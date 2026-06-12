// Package ai genera el insight proactivo diario a partir del snapshot del
// dashboard, usando Groq (clave server-side) detrás de una interfaz testeable.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultGroqBaseURL = "https://api.groq.com/openai/v1"

// Completer abstrae la llamada al LLM para testear el servicio con un fake
// (sin red). GroqClient es la implementación real.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// GroqClient habla con el endpoint OpenAI-compatible de Groq.
type GroqClient struct {
	baseURL    string
	apiKey     string
	model      string
	http       *http.Client
	streamHTTP *http.Client
}

// NewGroqClient crea el cliente real contra la API pública de Groq.
func NewGroqClient(apiKey, model string) *GroqClient {
	return newGroqClient(defaultGroqBaseURL, apiKey, model)
}

// newGroqClient permite inyectar baseURL (httptest.Server) en los tests.
func newGroqClient(baseURL, apiKey, model string) *GroqClient {
	return &GroqClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 10 * time.Second},
		// El timeout del cliente cubre la lectura completa del body; un stream
		// necesita más holgura que una respuesta bloqueante.
		streamHTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// ChatMsg es un turno de la conversación (rol + contenido) para el modo chat.
type ChatMsg struct {
	Role    string
	Content string
}

// Complete envía system+user a Groq y devuelve choices[0].message.content.
func (c *GroqClient) Complete(ctx context.Context, system, user string) (string, error) {
	return c.send(ctx, []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, 200)
}

// Chat envía el system + el historial completo (estilo OpenAI) y devuelve la
// respuesta. max_tokens un poco mayor que Complete para respuestas conversacionales.
func (c *GroqClient) Chat(ctx context.Context, system string, history []ChatMsg) (string, error) {
	msgs := make([]chatMessage, 0, len(history)+1)
	msgs = append(msgs, chatMessage{Role: "system", Content: system})
	for _, m := range history {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}
	return c.send(ctx, msgs, 400)
}

// send hace el POST a /chat/completions y devuelve el contenido del primer choice.
func (c *GroqClient) send(ctx context.Context, msgs []chatMessage, maxTokens int) (string, error) {
	reqBody, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    msgs,
		Temperature: 0.7,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("groq status %d: %s", res.StatusCode, string(body))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("groq sin choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

// chatStreamChunk es un chunk SSE de Groq en modo stream (estilo OpenAI).
type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// ChatStream envía el chat con "stream": true y re-emite cada delta vía
// onDelta. Devuelve el contenido completo acumulado. Si el stream se corta
// antes del data: [DONE], devuelve error (el caller no debe persistir).
func (c *GroqClient) ChatStream(ctx context.Context, system string, history []ChatMsg, onDelta func(string)) (string, error) {
	msgs := make([]chatMessage, 0, len(history)+1)
	msgs = append(msgs, chatMessage{Role: "system", Content: system})
	for _, m := range history {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}

	reqBody, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    msgs,
		Temperature: 0.7,
		MaxTokens:   400,
		Stream:      true,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	res, err := c.streamHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("groq status %d: %s", res.StatusCode, string(body))
	}

	var full strings.Builder
	sawDone := false
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			sawDone = true
			break
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return "", fmt.Errorf("groq chunk inválido: %w", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		if delta := chunk.Choices[0].Delta.Content; delta != "" {
			full.WriteString(delta)
			onDelta(delta)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if !sawDone {
		return "", fmt.Errorf("groq stream cortado antes de [DONE]")
	}
	if full.Len() == 0 {
		return "", fmt.Errorf("groq stream sin contenido")
	}
	return full.String(), nil
}
