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
	baseURL     string
	apiKey      string
	model       string
	visionModel string
	http        *http.Client
	streamHTTP  *http.Client
}

// NewGroqClient crea el cliente real contra la API pública de Groq.
func NewGroqClient(apiKey, model, visionModel string) *GroqClient {
	return newGroqClient(defaultGroqBaseURL, apiKey, model, visionModel)
}

// newGroqClient permite inyectar baseURL (httptest.Server) en los tests.
func newGroqClient(baseURL, apiKey, model, visionModel string) *GroqClient {
	return &GroqClient{
		baseURL:     baseURL,
		apiKey:      apiKey,
		model:       model,
		visionModel: visionModel,
		http:        &http.Client{Timeout: 10 * time.Second},
		// El timeout del cliente cubre la lectura completa del body; un stream
		// necesita más holgura que una respuesta bloqueante.
		streamHTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Tool es la definición de una function (estilo OpenAI) que se ofrece al modelo.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema de los argumentos
}

// ToolCall es la llamada a función que el modelo decidió hacer.
type ToolCall struct {
	Name      string
	Arguments string // JSON crudo, lo valida el servicio
}

type openaiToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openaiTool struct {
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	MaxTokens      int             `json:"max_tokens"`
	Stream         bool            `json:"stream,omitempty"`
	Tools          []openaiTool    `json:"tools,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
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
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int `json:"index"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

// tcAccum acumula nombre y argumentos fragmentados de un tool call por index.
type tcAccum struct {
	name string
	args strings.Builder
}

const funcTagOpen = "<function="
const funcTagClose = "</function>"

// extractFunctionToolCalls rescata las llamadas a función que el modelo (Llama en
// Groq) a veces emite como TEXTO —formato `<function=NOMBRE>{json}</function>`—
// en lugar de por el campo estructurado tool_calls. Devuelve el contenido sin
// esas etiquetas y las ToolCall extraídas. Solo extrae cuando el nombre no está
// vacío y los argumentos son JSON válido; una etiqueta mal formada se deja tal
// cual (mejor mostrar algo que romper el turno).
func extractFunctionToolCalls(content string) (string, []ToolCall) {
	var calls []ToolCall
	var out strings.Builder
	rest := content
	for {
		i := strings.Index(rest, funcTagOpen)
		if i < 0 {
			out.WriteString(rest)
			break
		}
		gt := strings.Index(rest[i:], ">")
		end := strings.Index(rest[i:], funcTagClose)
		if gt < 0 || end < 0 || end < gt {
			// Etiqueta incompleta o mal formada: emitir el resto sin tocar.
			out.WriteString(rest)
			break
		}
		name := strings.TrimSpace(rest[i+len(funcTagOpen) : i+gt])
		args := strings.TrimSpace(rest[i+gt+1 : i+end])
		after := i + end + len(funcTagClose)
		if name != "" && json.Valid([]byte(args)) {
			out.WriteString(rest[:i]) // texto conversacional antes de la etiqueta
			calls = append(calls, ToolCall{Name: name, Arguments: args})
		} else {
			// No es una llamada válida: conservar la etiqueta entera.
			out.WriteString(rest[:after])
		}
		rest = rest[after:]
	}
	return out.String(), calls
}

// funcTagHoldLen devuelve cuántos bytes del FINAL de s hay que retener porque
// podrían ser el comienzo de una etiqueta `<function=` aún incompleta (para no
// streamear una etiqueta cruda a medias). Es el sufijo más largo de s que es un
// prefijo propio de funcTagOpen.
func funcTagHoldLen(s string) int {
	max := len(funcTagOpen) - 1
	if max > len(s) {
		max = len(s)
	}
	for l := max; l >= 1; l-- {
		if strings.HasPrefix(funcTagOpen, s[len(s)-l:]) {
			return l
		}
	}
	return 0
}

// ChatStream envía el chat con "stream": true (y tools si hay) y re-emite cada
// delta de texto vía onDelta. Devuelve el texto acumulado y, si el modelo
// decidió llamar funciones, todos los ToolCall reensamblados ordenados por
// index. Corte antes de [DONE] → error.
func (c *GroqClient) ChatStream(ctx context.Context, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, []ToolCall, error) {
	msgs := make([]chatMessage, 0, len(history)+1)
	msgs = append(msgs, chatMessage{Role: "system", Content: system})
	for _, m := range history {
		msgs = append(msgs, chatMessage{Role: m.Role, Content: m.Content})
	}

	req := chatRequest{
		Model:       c.model,
		Messages:    msgs,
		Temperature: 0.7,
		MaxTokens:   400,
		Stream:      true,
	}
	for _, t := range tools {
		req.Tools = append(req.Tools, openaiTool{Type: "function", Function: openaiToolFunction{
			Name: t.Name, Description: t.Description, Parameters: t.Parameters,
		}})
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	res, err := c.streamHTTP.Do(httpReq)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return "", nil, fmt.Errorf("groq status %d: %s", res.StatusCode, string(body))
	}

	var full strings.Builder
	accums := map[int]*tcAccum{}
	maxIdx := -1
	sawDone := false

	// Buffer de retención del stream: si el modelo emite una etiqueta de función
	// como texto (`<function=...>...</function>`), no la mandamos cruda al usuario;
	// retenemos los bytes potencialmente parte de una etiqueta y descartamos las
	// etiquetas completas del stream en vivo (se rescatan del texto acumulado al
	// final). force=true vacía lo que quede al cerrar el stream.
	pending := ""
	emitClean := func(force bool) {
		for {
			i := strings.Index(pending, funcTagOpen)
			if i < 0 {
				hold := 0
				if !force {
					hold = funcTagHoldLen(pending)
				}
				cut := len(pending) - hold
				if cut > 0 {
					onDelta(pending[:cut])
				}
				pending = pending[cut:]
				return
			}
			if i > 0 {
				onDelta(pending[:i])
				pending = pending[i:]
			}
			end := strings.Index(pending, funcTagClose)
			if end < 0 {
				if force {
					pending = "" // etiqueta sin cerrar al final: no emitirla cruda
				}
				return
			}
			pending = pending[end+len(funcTagClose):] // descartar la etiqueta del stream en vivo
		}
	}

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
			return "", nil, fmt.Errorf("groq chunk inválido: %w", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			full.WriteString(delta.Content)
			pending += delta.Content
			emitClean(false)
		}
		for _, tc := range delta.ToolCalls {
			a, ok := accums[tc.Index]
			if !ok {
				a = &tcAccum{}
				accums[tc.Index] = a
				if tc.Index > maxIdx {
					maxIdx = tc.Index
				}
			}
			if tc.Function.Name != "" {
				a.name = tc.Function.Name
			}
			a.args.WriteString(tc.Function.Arguments)
		}
	}
	emitClean(true) // vaciar lo retenido al cerrar el stream
	if err := scanner.Err(); err != nil {
		return "", nil, err
	}
	if !sawDone {
		return "", nil, fmt.Errorf("groq stream cortado antes de [DONE]")
	}
	var calls []ToolCall
	for i := 0; i <= maxIdx; i++ {
		if a, ok := accums[i]; ok && a.name != "" {
			calls = append(calls, ToolCall{Name: a.name, Arguments: a.args.String()})
		}
	}
	content := full.String()
	// Fallback: si no hubo tool_calls estructurados, el modelo pudo emitir la
	// llamada como texto (`<function=...>`). La rescatamos y limpiamos el texto.
	if len(calls) == 0 {
		cleaned, textCalls := extractFunctionToolCalls(content)
		if len(textCalls) > 0 {
			content = cleaned
			calls = textCalls
		}
	}
	if len(calls) > 0 {
		return content, calls, nil
	}
	// Una respuesta vacía con [DONE] se trata como fallo a propósito: persistir
	// un mensaje de asistente vacío no le sirve de nada al usuario.
	if content == "" {
		return "", nil, fmt.Errorf("groq stream sin contenido")
	}
	return content, nil, nil
}

// --- Tipos para mensajes de visión (content array) ---

type visionContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *visionImageURL `json:"image_url,omitempty"`
}

type visionImageURL struct {
	URL string `json:"url"`
}

type visionMessage struct {
	Role    string              `json:"role"`
	Content []visionContentPart `json:"content"`
}

type visionRequest struct {
	Model          string            `json:"model"`
	Messages       []json.RawMessage `json:"messages"`
	Temperature    float64           `json:"temperature"`
	MaxTokens      int               `json:"max_tokens"`
	ResponseFormat *responseFormat   `json:"response_format,omitempty"`
}

// ExtractText manda system+user al modelo de TEXTO con JSON mode (CSV / PDF-texto).
func (c *GroqClient) ExtractText(ctx context.Context, system, user string) (string, error) {
	return c.sendJSON(ctx, c.model, []json.RawMessage{
		rawMsg("system", system), rawMsg("user", user),
	})
}

// ExtractVision manda system + imagen base64 al modelo de VISIÓN con JSON mode.
func (c *GroqClient) ExtractVision(ctx context.Context, system, b64, mime string) (string, error) {
	userMsg := visionMessage{Role: "user", Content: []visionContentPart{
		{Type: "text", Text: "Extrae los movimientos del comprobante."},
		{Type: "image_url", ImageURL: &visionImageURL{URL: "data:" + mime + ";base64," + b64}},
	}}
	raw, _ := json.Marshal(userMsg)
	return c.sendJSON(ctx, c.visionModel, []json.RawMessage{rawMsg("system", system), raw})
}

func rawMsg(role, content string) json.RawMessage {
	b, _ := json.Marshal(chatMessage{Role: role, Content: content})
	return b
}

// sendJSON hace el POST con response_format json_object y max_tokens amplio
// (las extracciones pueden devolver muchos movimientos). Reusa el cliente de
// 60s porque la visión puede tardar.
func (c *GroqClient) sendJSON(ctx context.Context, model string, msgs []json.RawMessage) (string, error) {
	reqBody, err := json.Marshal(visionRequest{
		Model: model, Messages: msgs, Temperature: 0.2, MaxTokens: 2000,
		ResponseFormat: &responseFormat{Type: "json_object"},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
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
