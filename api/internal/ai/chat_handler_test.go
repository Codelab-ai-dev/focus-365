package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/focus365/api/internal/ai"
	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
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

func postChatStream(t *testing.T, h http.Handler, tok, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ai/chat/stream?today="+today, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestChatStreamHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatDeltas: []string{"Vas ", "verde."}}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "stream@b.com")

	rec := postChatStream(t, e.h, tok, `{"message":"¿cómo voy?"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	if ab := rec.Header().Get("X-Accel-Buffering"); ab != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no (nginx)", ab)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: delta", `{"text":"Vas "}`, `{"text":"verde."}`,
		"event: done", `"content":"Vas verde."`, `"role":"assistant"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body SSE no contiene %q:\n%s", want, body)
		}
	}

	rec2, body2 := getMessages(t, e.h, tok)
	if rec2.Code != http.StatusOK {
		t.Fatalf("messages code = %d", rec2.Code)
	}
	msgs, _ := body2["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2 (par persistido)", len(msgs))
	}
}

func TestChatStreamGroqFailureMidwayEmitsErrorEvent(t *testing.T) {
	comp := &fakeCompleter{chatDeltas: []string{"Vas "}, chatStreamErr: errors.New("stream cortado")}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "streamcut@b.com")

	rec := postChatStream(t, e.h, tok, `{"message":"hola"}`)
	// Los headers ya salieron con el primer delta: el código es 200.
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: delta") || !strings.Contains(body, "event: error") {
		t.Errorf("esperaba delta y error en el body:\n%s", body)
	}
	if strings.Contains(body, "event: done") {
		t.Errorf("no debe haber done tras un corte:\n%s", body)
	}

	_, body2 := getMessages(t, e.h, tok)
	msgs, _ := body2["messages"].([]any)
	if len(msgs) != 0 {
		t.Errorf("corte a medias no debe persistir, got %d mensajes", len(msgs))
	}
}

func TestChatStreamFailureBeforeFirstDeltaIs503(t *testing.T) {
	comp := &fakeCompleter{chatStreamErr: errors.New("groq caído")} // cero deltas
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "streamdown@b.com")

	rec := postChatStream(t, e.h, tok, `{"message":"hola"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503 (HTTP normal, sin SSE)", rec.Code)
	}
}

func TestChatStreamNoKey503(t *testing.T) {
	e := newEnv(t, false, &fakeCompleter{chatDeltas: []string{"no usar"}})
	_, tok := e.user(t, "streamnokey@b.com")

	rec := postChatStream(t, e.h, tok, `{"message":"hola"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", rec.Code)
	}
}

func TestChatStreamValidationAndAuth(t *testing.T) {
	e := newEnv(t, true, &fakeCompleter{chatDeltas: []string{"x"}})
	_, tok := e.user(t, "streamval@b.com")

	if rec := postChatStream(t, e.h, tok, `{"message":"   "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("solo espacios code = %d, want 400", rec.Code)
	}
	if rec := postChatStream(t, e.h, "", `{"message":"hola"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func postAction(t *testing.T, h http.Handler, tok, id, verb string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ai/actions/"+id+"/"+verb+"?today="+today, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

// proposeViaChat dispara el chat/stream con un fake que devuelve un tool call
// y extrae el id del mensaje propuesto desde el historial.
func proposeViaChat(t *testing.T, e *env, tok string) string {
	t.Helper()
	rec := postChatStream(t, e.h, tok, `{"message":"registra mi check-in: 8 6 9"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat/stream code = %d, body = %s", rec.Code, rec.Body.String())
	}
	_, body := getMessages(t, e.h, tok)
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("sin mensajes tras proponer")
	}
	last, _ := msgs[len(msgs)-1].(map[string]any)
	action, _ := last["action"].(map[string]any)
	if action == nil || action["status"] != "proposed" {
		t.Fatalf("último mensaje sin acción proposed: %v", last)
	}
	id, _ := last["id"].(string)
	if id == "" {
		t.Fatal("mensaje sin id")
	}
	return id
}

func checkinToolCall() *ai.ToolCall {
	return &ai.ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}
}

func TestActionConfirmHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatToolCall: checkinToolCall()}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "action-ok@b.com")
	id := proposeViaChat(t, e, tok)

	rec, body := postAction(t, e.h, tok, id, "confirm")
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm code = %d, body = %s", rec.Code, rec.Body.String())
	}
	msg, _ := body["message"].(map[string]any)
	action, _ := msg["action"].(map[string]any)
	if action["status"] != "done" {
		t.Errorf("status = %v", action)
	}

	// El check-in quedó escrito de verdad.
	ci, err := e.q.GetCheckInByDate(context.Background(), store.GetCheckInByDateParams{
		UserID: uid, Date: dayTime(t),
	})
	if err != nil {
		t.Fatalf("check-in no escrito: %v", err)
	}
	if ci.Mood != 8 || ci.Energy != 6 || ci.Discipline != 9 {
		t.Errorf("check-in = %+v", ci)
	}

	// Doble confirm → 409.
	if rec2, _ := postAction(t, e.h, tok, id, "confirm"); rec2.Code != http.StatusConflict {
		t.Errorf("doble confirm code = %d, want 409", rec2.Code)
	}
}

func TestActionCancel(t *testing.T) {
	comp := &fakeCompleter{chatToolCall: checkinToolCall()}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "action-cancel@b.com")
	id := proposeViaChat(t, e, tok)

	rec, body := postAction(t, e.h, tok, id, "cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel code = %d", rec.Code)
	}
	msg, _ := body["message"].(map[string]any)
	action, _ := msg["action"].(map[string]any)
	if action["status"] != "cancelled" {
		t.Errorf("status = %v", action)
	}
	// Nada se escribió.
	if _, err := e.q.GetCheckInByDate(context.Background(), store.GetCheckInByDateParams{
		UserID: uid, Date: dayTime(t),
	}); err == nil {
		t.Error("cancelar no debe escribir el check-in")
	}
}

func TestActionConfirmInvalidPayloadIs400AndStaysProposed(t *testing.T) {
	comp := &fakeCompleter{chatToolCall: &ai.ToolCall{
		Name: "marcar_habito", Arguments: `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`,
	}}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "action-400@b.com")
	id := proposeViaChat(t, e, tok)

	rec, _ := postAction(t, e.h, tok, id, "confirm")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("confirm code = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}

	// La acción sigue proposed (se puede cancelar o reintentar).
	_, body := getMessages(t, e.h, tok)
	msgs, _ := body["messages"].([]any)
	last, _ := msgs[len(msgs)-1].(map[string]any)
	action, _ := last["action"].(map[string]any)
	if action["status"] != "proposed" {
		t.Errorf("status = %v, want proposed", action["status"])
	}
}

func TestActionCrearHabitoEndToEnd(t *testing.T) {
	comp := &fakeCompleter{chatToolCall: &ai.ToolCall{
		Name: "crear_habito", Arguments: `{"name":"Leer 30 min","target_days":21}`,
	}}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "habito-nuevo@b.com")
	id := proposeViaChat(t, e, tok)

	rec, body := postAction(t, e.h, tok, id, "confirm")
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm code = %d, body = %s", rec.Code, rec.Body.String())
	}
	msg, _ := body["message"].(map[string]any)
	action, _ := msg["action"].(map[string]any)
	if action["status"] != "done" || action["kind"] != "habito_nuevo" {
		t.Errorf("action = %v", action)
	}

	// El hábito existe de verdad en la DB.
	habs, err := e.q.ListHabits(context.Background(), uid)
	if err != nil {
		t.Fatalf("ListHabits: %v", err)
	}
	found := false
	for _, h := range habs {
		if h.Name == "Leer 30 min" {
			found = true
		}
	}
	if !found {
		t.Errorf("el hábito no se creó: %+v", habs)
	}
}

func TestActionErrors(t *testing.T) {
	comp := &fakeCompleter{chatToolCall: checkinToolCall()}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "action-err@b.com")

	// id inexistente → 404; id malformado → 404; sin token → 401.
	if rec, _ := postAction(t, e.h, tok, uuid.New().String(), "confirm"); rec.Code != http.StatusNotFound {
		t.Errorf("inexistente code = %d, want 404", rec.Code)
	}
	if rec, _ := postAction(t, e.h, tok, "no-uuid", "confirm"); rec.Code != http.StatusNotFound {
		t.Errorf("uuid malformado code = %d, want 404", rec.Code)
	}
	if rec, _ := postAction(t, e.h, "", uuid.New().String(), "confirm"); rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}

	// El mensaje de otro usuario no se puede confirmar.
	idA := proposeViaChat(t, e, tok)
	_, tokB := e.user(t, "action-eve@b.com")
	if rec, _ := postAction(t, e.h, tokB, idA, "confirm"); rec.Code != http.StatusNotFound {
		t.Errorf("cross-user code = %d, want 404", rec.Code)
	}
}
