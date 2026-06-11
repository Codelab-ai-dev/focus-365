package ai_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postChat(t *testing.T, h http.Handler, tok, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ai/chat?today="+today, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func getMessages(t *testing.T, h http.Handler, tok string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ai/messages", nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestChatHappyPathPersists(t *testing.T) {
	comp := &fakeCompleter{chatOut: "Vas verde este ciclo."}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "chat@b.com")

	rec, body := postChat(t, e.h, tok, `{"message":"¿cómo voy?"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	reply, _ := body["reply"].(map[string]any)
	if reply["role"] != "assistant" || reply["content"] != "Vas verde este ciclo." {
		t.Errorf("reply = %v", body["reply"])
	}

	rec2, body2 := getMessages(t, e.h, tok)
	if rec2.Code != http.StatusOK {
		t.Fatalf("messages code = %d", rec2.Code)
	}
	msgs, _ := body2["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "user" || first["content"] != "¿cómo voy?" {
		t.Errorf("primer mensaje = %v", msgs[0])
	}
}

func TestChatRequiresAuth(t *testing.T) {
	e := newEnv(t, true, &fakeCompleter{chatOut: "x"})
	if rec, _ := postChat(t, e.h, "", `{"message":"hola"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("POST sin token code = %d, want 401", rec.Code)
	}
	if rec, _ := getMessages(t, e.h, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("GET sin token code = %d, want 401", rec.Code)
	}
}

func TestChatValidationRejectsEmpty(t *testing.T) {
	e := newEnv(t, true, &fakeCompleter{chatOut: "x"})
	_, tok := e.user(t, "empty@b.com")

	if rec, _ := postChat(t, e.h, tok, `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("body vacío code = %d, want 400", rec.Code)
	}
	if rec, _ := postChat(t, e.h, tok, `{"message":"   "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("solo espacios code = %d, want 400", rec.Code)
	}
}

func TestChatValidationLengthCountsRunes(t *testing.T) {
	comp := &fakeCompleter{chatOut: "ok"}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "len@b.com")

	// 2000 acentos = 4000 bytes pero 2000 caracteres: debe pasar (no 400 por bytes).
	okMsg := strings.Repeat("á", 2000)
	if rec, _ := postChat(t, e.h, tok, `{"message":"`+okMsg+`"}`); rec.Code != http.StatusOK {
		t.Errorf("2000 caracteres code = %d, want 200", rec.Code)
	}

	// 2001 caracteres: debe rechazarse con 400.
	tooLong := strings.Repeat("a", 2001)
	if rec, _ := postChat(t, e.h, tok, `{"message":"`+tooLong+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("2001 caracteres code = %d, want 400", rec.Code)
	}
}

func TestChatNoKeyDegrades(t *testing.T) {
	e := newEnv(t, false, &fakeCompleter{chatOut: "no usar"})
	_, tok := e.user(t, "nokeychat@b.com")

	rec, _ := postChat(t, e.h, tok, `{"message":"hola"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("sin clave code = %d, want 503", rec.Code)
	}
	_, body := getMessages(t, e.h, tok)
	msgs, _ := body["messages"].([]any)
	if len(msgs) != 0 {
		t.Errorf("degradado no debe persistir, got %d mensajes", len(msgs))
	}
}

func TestChatEmptyHistory(t *testing.T) {
	e := newEnv(t, true, &fakeCompleter{chatOut: "x"})
	_, tok := e.user(t, "fresh@b.com")

	rec, body := getMessages(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) != 0 {
		t.Errorf("historial fresco = %v, want []", body["messages"])
	}
}
