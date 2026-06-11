// Package ai genera el insight proactivo diario a partir del snapshot del
// dashboard, usando Groq (clave server-side) detrás de una interfaz testeable.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
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
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete envía system+user a Groq y devuelve choices[0].message.content.
func (c *GroqClient) Complete(ctx context.Context, system, user string) (string, error) {
	reqBody, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0.7,
		MaxTokens:   200,
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
