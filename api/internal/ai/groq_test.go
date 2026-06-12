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

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile")
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

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error en HTTP 500")
	}
}

func TestGroqCompleteNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("esperaba error sin choices")
	}
}

func TestGroqCompleteInvalidBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`no-json`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
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

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile")
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

	c := newGroqClient(srv.URL, "k", "m")
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

	c := newGroqClient(srv.URL, "test-key", "llama-3.3-70b-versatile")
	var deltas []string
	got, tc, err := c.ChatStream(context.Background(), "sys", []ChatMsg{
		{Role: "user", Content: "¿cómo voy?"},
	}, nil, func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if got != "Vas bien." {
		t.Errorf("content = %q, want %q", got, "Vas bien.")
	}
	if tc != nil {
		t.Errorf("tc = %+v, want nil en turno de texto puro", tc)
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

	c := newGroqClient(srv.URL, "k", "m")
	var deltas []string
	_, _, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}},
		nil, func(d string) { deltas = append(deltas, d) })
	if err == nil {
		t.Fatal("esperaba error al cortarse el stream sin [DONE]")
	}
}

func TestGroqChatStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
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
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"function":{"name":"registrar_checkin","arguments":""}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{\"mood\":8,"}}]}}]}`)
		sseChunk(w, `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"\"energy\":6}"}}]}}]}`)
		sseChunk(w, `[DONE]`)
	}))
	defer srv.Close()

	c := newGroqClient(srv.URL, "k", "m")
	tools := []Tool{{Name: "registrar_checkin", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}}
	text, tc, err := c.ChatStream(context.Background(), "sys", []ChatMsg{{Role: "user", Content: "registra"}}, tools, func(string) {})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if text != "" {
		t.Errorf("text = %q, want vacío en turno de tool call", text)
	}
	if tc == nil || tc.Name != "registrar_checkin" || tc.Arguments != `{"mood":8,"energy":6}` {
		t.Errorf("toolCall = %+v", tc)
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

	c := newGroqClient(srv.URL, "k", "m")
	text, tc, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "hola"}},
		[]Tool{{Name: "x", Description: "d", Parameters: json.RawMessage(`{}`)}}, func(string) {})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if text != "Hola." || tc != nil {
		t.Errorf("text = %q, tc = %+v", text, tc)
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

	c := newGroqClient(srv.URL, "k", "m")
	if _, _, err := c.ChatStream(context.Background(), "s", []ChatMsg{{Role: "user", Content: "x"}}, nil, func(string) {}); err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if strings.Contains(string(gotBody), `"tools"`) {
		t.Errorf("sin tools el body no debe llevar el campo: %s", gotBody)
	}
}
