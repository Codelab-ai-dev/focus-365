package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGroqCompleteOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Buen ritmo hoy."}}]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile", "vision-model")
	got, err := c.Complete(context.Background(), "sys", "usr")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "Buen ritmo hoy." {
		t.Errorf("content = %q", got)
	}
}

func TestGroqCompleteHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error en HTTP 500")
	}
}

func TestGroqCompleteNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error sin choices")
	}
}

func TestGroqCompleteInvalidBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`no-json`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error con body inválido")
	}
}

func TestGroqChatOK(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Vas bien."}}]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile", "vision-model")
	got, err := c.Chat(context.Background(), "sys", []ChatMsg{
		{Role: "user", Content: "hola"},
		{Role: "assistant", Content: "qué tal"},
		{Role: "user", Content: "¿cómo voy?"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "Vas bien." {
		t.Errorf("content = %q", got)
	}
	// El body debe llevar el system primero y luego el history en orden.
	body := string(gotBody)
	for _, want := range []string{`"role":"system"`, `"content":"sys"`, `"content":"hola"`, `"content":"¿cómo voy?"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body no contiene %q: %s", want, body)
		}
	}
}

func TestGroqChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	if _, err := c.Chat(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("esperaba error en HTTP 500")
	}
}

// sseChunk escribe un evento data: de Groq y hace flush.
func sseChunk(w http.ResponseWriter, payload string) {
	_, _ = w.Write([]byte("data: " + payload + "\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func TestGroqChatStreamOK(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"content":"Vas "}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"content":"bien."}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile", "vision-model")
	var deltas []string
	got, tcs, err := c.ChatStream(context.Background(), "sys", []ChatMsg{
		{Role: "user", Content: "¿cómo voy?"},
	}, nil, func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if got != "Vas bien." {
		t.Errorf("content = %q, want %q", got, "Vas bien.")
	}
	if len(tcs) != 0 {
		t.Errorf("tcs = %+v, want empty en turno de texto puro", tcs)
	}
	if len(deltas) != 2 || deltas[0] != "Vas " || deltas[1] != "bien." {
		t.Errorf("deltas = %v", deltas)
	}
	body := string(gotBody)
	for _, want := range []string{`"stream":true`, `"role":"system"`, `"content":"sys"`, `"content":"¿cómo voy?"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body no contiene %q: %s", want, body)
		}
	}
}

func TestGroqChatStreamCutMidwayFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"content":"Vas "}}]}`)
		// Cierra sin [DONE]: simula corte a medias.
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	var deltas []string
	_, _, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}},
		nil, func(d string) { deltas = append(deltas, d) })
	if err == nil {
		t.Fatal("esperaba error al cortarse el stream sin [DONE]")
	}
	_ = deltas
}

func TestGroqChatStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	if _, _, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}},
		nil, func(string) {}); err == nil {
		t.Fatal("esperaba error en HTTP 500")
	}
}

func TestGroqChatStreamToolCall(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		// El name llega en el primer fragmento; arguments llega partido.
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"registrar_checkin","arguments":""}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"mood\":8,"}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"energy\":6}"}}]}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	tools := []Tool{{Name: "registrar_checkin", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}}
	text, tcs, err := c.ChatStream(context.Background(), "sys", []ChatMsg{{Role: "user", Content: "registra"}}, tools, func(string) {})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if text != "" {
		t.Errorf("text = %q, want vacío en turno de tool call", text)
	}
	if len(tcs) != 1 || tcs[0].Name != "registrar_checkin" || tcs[0].Arguments != `{"mood":8,"energy":6}` {
		t.Errorf("toolCalls = %+v", tcs)
	}
	body := string(gotBody)
	for _, want := range []string{`"tools":[{"type":"function"`, `"name":"registrar_checkin"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body no contiene %q: %s", want, body)
		}
	}
}

func TestGroqChatStreamTextWithToolsReturnsNilToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"content":"Hola."}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	text, tcs, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "hola"}},
		[]Tool{{Name: "x", Description: "d", Parameters: json.RawMessage(`{}`)}}, func(string) {})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if text != "Hola." || len(tcs) != 0 {
		t.Errorf("text = %q, tcs = %+v", text, tcs)
	}
}

func TestGroqChatStreamMultipleToolCallsAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"registrar_checkin","arguments":"{\"mood\":8,"}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"name":"marcar_habito","arguments":""}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"energy\":6,\"discipline\":9}"}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"habit_id\":\"h1\"}"}}]}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	_, tcs, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}}, nil, func(string) {})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if len(tcs) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(tcs))
	}
	if tcs[0].Name != "registrar_checkin" || tcs[0].Arguments != `{"mood":8,"energy":6,"discipline":9}` {
		t.Errorf("tc0 = %+v", tcs[0])
	}
	if tcs[1].Name != "marcar_habito" || tcs[1].Arguments != `{"habit_id":"h1"}` {
		t.Errorf("tc1 = %+v", tcs[1])
	}
}

func TestGroqChatStreamNoToolsOmitsField(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		sseChunk(w, `{"choices":[{"delta":{"content":"ok"}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	if _, _, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}}, nil, func(string) {}); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if strings.Contains(string(gotBody), `"tools"`) {
		t.Errorf("sin tools el body no debe llevar el campo: %s", gotBody)
	}
	_ = gotBody
}

func TestGroqExtractTextJSONMode(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"movimientos\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m", "vm")
	got, err := c.ExtractText(context.Background(), "sys", "datos csv")
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if got != `{"movimientos":[]}` {
		t.Errorf("content = %q", got)
	}
	body := string(gotBody)
	if !strings.Contains(body, `"response_format":{"type":"json_object"}`) {
		t.Errorf("falta response_format json_object: %s", body)
	}
}

func TestGroqExtractVisionSendsImage(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"movimientos\":[]}"}}]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "text-model", "vision-model")
	got, err := c.ExtractVision(context.Background(), "sys", "aGVsbG8=", "image/png")
	if err != nil {
		t.Fatalf("ExtractVision: %v", err)
	}
	if got != `{"movimientos":[]}` {
		t.Errorf("content = %q", got)
	}
	body := string(gotBody)
	for _, want := range []string{`"vision-model"`, `"type":"image_url"`, `data:image/png;base64,aGVsbG8=`, `"response_format"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body no contiene %q: %s", want, body)
		}
	}
}

func TestGroqExtractVisionHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()
	c := newGroqClient(srv.URL, "k", "m", "vm")
	if _, err := c.ExtractVision(context.Background(), "s", "x", "image/png"); err == nil {
		t.Fatal("esperaba error en HTTP 400")
	}
}

func TestExtractFunctionToolCalls(t *testing.T) {
	// Caso del bug: el modelo emitió la llamada como TEXTO embebida en una frase.
	cleaned, calls := extractFunctionToolCalls(
		`Podríamos <function=registrar_checkin>{"mood": 9, "energy": 9}</function> para reflejar.`)
	if len(calls) != 1 || calls[0].Name != "registrar_checkin" ||
		calls[0].Arguments != `{"mood": 9, "energy": 9}` {
		t.Fatalf("calls = %+v", calls)
	}
	if strings.Contains(cleaned, "<function") {
		t.Errorf("cleaned aún tiene la etiqueta: %q", cleaned)
	}
	if !strings.Contains(cleaned, "Podríamos") || !strings.Contains(cleaned, "para reflejar.") {
		t.Errorf("cleaned perdió el texto conversacional: %q", cleaned)
	}

	// Sin etiqueta: contenido intacto.
	c2, k2 := extractFunctionToolCalls("Hola, todo bien.")
	if c2 != "Hola, todo bien." || len(k2) != 0 {
		t.Errorf("c2=%q k2=%+v", c2, k2)
	}

	// Args con JSON inválido: no se extrae (no se rompe el turno).
	_, k3 := extractFunctionToolCalls("x <function=foo>esto no es json</function> y")
	if len(k3) != 0 {
		t.Errorf("no debería extraer con JSON inválido: %+v", k3)
	}

	// Solo la etiqueta (sin texto alrededor): cleaned vacío, un call.
	c4, k4 := extractFunctionToolCalls(`<function=crear_meta>{"titulo":"x","dimension":"fisica"}</function>`)
	if len(k4) != 1 || strings.TrimSpace(c4) != "" {
		t.Errorf("c4=%q k4=%+v", c4, k4)
	}
}

func TestGroqChatStreamTextFunctionCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// El modelo emite la llamada como TEXTO, partida en varios chunks SSE.
		sseChunk(w, `{"choices":[{"delta":{"content":"Podríamos "}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"content":"<function=registrar_che"}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"content":"ckin>{\"mood\": 9, \"energy\": 9}</function>"}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"content":" para reflejar."}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "llama-3.3-70b-versatile", "vm")
	var deltas strings.Builder
	tools := []Tool{{Name: "registrar_checkin", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}}
	text, tcs, err := c.ChatStream(context.Background(), "sys",
		[]ChatMsg{{Role: "user", Content: "registra"}}, tools,
		func(d string) { deltas.WriteString(d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if len(tcs) != 1 || tcs[0].Name != "registrar_checkin" ||
		tcs[0].Arguments != `{"mood": 9, "energy": 9}` {
		t.Fatalf("tcs = %+v", tcs)
	}
	if strings.Contains(text, "<function") {
		t.Errorf("el texto persistido aún tiene la etiqueta: %q", text)
	}
	if strings.Contains(deltas.String(), "<function") {
		t.Errorf("se streameó la etiqueta cruda al usuario: %q", deltas.String())
	}
}
