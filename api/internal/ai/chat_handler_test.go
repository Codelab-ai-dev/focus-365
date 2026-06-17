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

func getJSON(t *testing.T, h http.Handler, tok, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func patchJSON(t *testing.T, h http.Handler, tok, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body))
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

func deleteReq(t *testing.T, h http.Handler, tok, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// threadMessages lista los mensajes de un hilo por su id.
func threadMessages(t *testing.T, h http.Handler, tok, tid string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	return getJSON(t, h, tok, "/ai/threads/"+tid+"/messages")
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
	tid, _ := body["thread_id"].(string)
	if tid == "" {
		t.Fatal("chat no devolvió thread_id")
	}

	rec2, body2 := threadMessages(t, e.h, tok, tid)
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
	if rec, _ := getJSON(t, e.h, "", "/ai/threads"); rec.Code != http.StatusUnauthorized {
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
	_, body := getJSON(t, e.h, tok, "/ai/threads")
	threads, _ := body["threads"].([]any)
	if len(threads) != 0 {
		t.Errorf("degradado no debe crear hilos, got %d", len(threads))
	}
}

func TestChatEmptyHistory(t *testing.T) {
	e := newEnv(t, true, &fakeCompleter{chatOut: "x"})
	_, tok := e.user(t, "fresh@b.com")

	rec, body := getJSON(t, e.h, tok, "/ai/threads")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	threads, ok := body["threads"].([]any)
	if !ok || len(threads) != 0 {
		t.Errorf("usuario fresco = %v, want []", body["threads"])
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

// threadIDFromDoneSSE extrae el thread_id del evento `done` de un body SSE.
func threadIDFromDoneSSE(t *testing.T, sse string) string {
	t.Helper()
	for _, line := range strings.Split(sse, "\n") {
		data, found := strings.CutPrefix(line, "data: ")
		if !found {
			continue
		}
		var ev struct {
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err == nil && ev.ThreadID != "" {
			return ev.ThreadID
		}
	}
	return ""
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
	tid := threadIDFromDoneSSE(t, body)
	if tid == "" {
		t.Fatal("done SSE sin thread_id")
	}

	rec2, body2 := threadMessages(t, e.h, tok, tid)
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

	_, body2 := getJSON(t, e.h, tok, "/ai/threads")
	threads, _ := body2["threads"].([]any)
	if len(threads) != 0 {
		t.Errorf("corte a medias no debe crear hilos, got %d", len(threads))
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
	tid := threadIDFromDoneSSE(t, rec.Body.String())
	if tid == "" {
		t.Fatal("done SSE sin thread_id")
	}
	_, body := threadMessages(t, e.h, tok, tid)
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("sin mensajes tras proponer")
	}
	last, _ := msgs[len(msgs)-1].(map[string]any)
	actions, _ := last["actions"].([]any)
	if len(actions) == 0 {
		t.Fatalf("último mensaje sin acciones: %v", last)
	}
	action, _ := actions[0].(map[string]any)
	if action == nil || action["status"] != "proposed" {
		t.Fatalf("último mensaje sin acción proposed: %v", last)
	}
	id, _ := action["id"].(string)
	if id == "" {
		t.Fatal("acción sin id")
	}
	return id
}

func checkinToolCall() []ai.ToolCall {
	return []ai.ToolCall{{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6}`}}
}

func TestActionConfirmHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatToolCalls: checkinToolCall()}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "action-ok@b.com")
	id := proposeViaChat(t, e, tok)

	rec, body := postAction(t, e.h, tok, id, "confirm")
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm code = %d, body = %s", rec.Code, rec.Body.String())
	}
	action, _ := body["action"].(map[string]any)
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
	if ci.Mood != 8 || ci.Energy != 6 {
		t.Errorf("check-in = %+v", ci)
	}

	// Doble confirm → 409.
	if rec2, _ := postAction(t, e.h, tok, id, "confirm"); rec2.Code != http.StatusConflict {
		t.Errorf("doble confirm code = %d, want 409", rec2.Code)
	}
}

func TestActionCancel(t *testing.T) {
	comp := &fakeCompleter{chatToolCalls: checkinToolCall()}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "action-cancel@b.com")
	id := proposeViaChat(t, e, tok)

	rec, body := postAction(t, e.h, tok, id, "cancel")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel code = %d", rec.Code)
	}
	action, _ := body["action"].(map[string]any)
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
	comp := &fakeCompleter{chatToolCalls: []ai.ToolCall{
		{Name: "marcar_habito", Arguments: `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`},
	}}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "action-400@b.com")
	id := proposeViaChat(t, e, tok)

	rec, _ := postAction(t, e.h, tok, id, "confirm")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("confirm code = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}

	// La acción sigue proposed (se puede cancelar o reintentar).
	_, body := threadMessages(t, e.h, tok, onlyThreadID(t, e, tok))
	msgs, _ := body["messages"].([]any)
	last, _ := msgs[len(msgs)-1].(map[string]any)
	actions, _ := last["actions"].([]any)
	action, _ := actions[0].(map[string]any)
	if action["status"] != "proposed" {
		t.Errorf("status = %v, want proposed", action["status"])
	}
}

// onlyThreadID devuelve el id del único hilo del usuario (helper de tests).
func onlyThreadID(t *testing.T, e *env, tok string) string {
	t.Helper()
	_, body := getJSON(t, e.h, tok, "/ai/threads")
	threads, _ := body["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("se esperaba 1 hilo, got %d", len(threads))
	}
	th, _ := threads[0].(map[string]any)
	id, _ := th["id"].(string)
	if id == "" {
		t.Fatal("hilo sin id")
	}
	return id
}

func TestActionCrearHabitoEndToEnd(t *testing.T) {
	comp := &fakeCompleter{chatToolCalls: []ai.ToolCall{
		{Name: "crear_habito", Arguments: `{"name":"Leer 30 min","target_days":21}`},
	}}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "habito-nuevo@b.com")
	id := proposeViaChat(t, e, tok)

	rec, body := postAction(t, e.h, tok, id, "confirm")
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm code = %d, body = %s", rec.Code, rec.Body.String())
	}
	action, _ := body["action"].(map[string]any)
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

func TestActionUndoHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatToolCalls: checkinToolCall()}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "undo-ok@b.com")
	id := proposeViaChat(t, e, tok)
	if rec, _ := postAction(t, e.h, tok, id, "confirm"); rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d", rec.Code)
	}
	// El check-in existe…
	if _, err := e.q.GetCheckInByDate(context.Background(), store.GetCheckInByDateParams{UserID: uid, Date: dayTime(t)}); err != nil {
		t.Fatalf("check-in no escrito: %v", err)
	}
	// …deshacer…
	rec, body := postAction(t, e.h, tok, id, "undo")
	if rec.Code != http.StatusOK {
		t.Fatalf("undo = %d, body = %s", rec.Code, rec.Body.String())
	}
	action, _ := body["action"].(map[string]any)
	if action["status"] != "undone" {
		t.Errorf("status = %v", action["status"])
	}
	// …y el check-in del día desapareció (no había previo).
	if _, err := e.q.GetCheckInByDate(context.Background(), store.GetCheckInByDateParams{UserID: uid, Date: dayTime(t)}); err == nil {
		t.Error("el check-in debía borrarse al deshacer")
	}
	// Doble undo → 409.
	if rec2, _ := postAction(t, e.h, tok, id, "undo"); rec2.Code != http.StatusConflict {
		t.Errorf("doble undo = %d, want 409", rec2.Code)
	}
}

func TestActionUndoDeProposedEs409(t *testing.T) {
	comp := &fakeCompleter{chatToolCalls: checkinToolCall()}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "undo-409@b.com")
	id := proposeViaChat(t, e, tok)
	if rec, _ := postAction(t, e.h, tok, id, "undo"); rec.Code != http.StatusConflict {
		t.Errorf("undo de proposed = %d, want 409", rec.Code)
	}
}

func TestActionErrors(t *testing.T) {
	comp := &fakeCompleter{chatToolCalls: checkinToolCall()}
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

func TestThreadsEndpointsHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatOut: "respuesta"}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "threads@b.com")

	// Crear un hilo enviando un mensaje sin thread_id.
	rec, body := postChat(t, e.h, tok, `{"message":"hola mundo"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat code = %d", rec.Code)
	}
	tid, _ := body["thread_id"].(string)
	if tid == "" {
		t.Fatal("chat no devolvió thread_id")
	}

	// Listar hilos: 1, con preview y título derivado.
	rec, body = getJSON(t, e.h, tok, "/ai/threads")
	if rec.Code != http.StatusOK {
		t.Fatalf("threads code = %d", rec.Code)
	}
	threads, _ := body["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("len threads = %d", len(threads))
	}

	// Renombrar.
	rec, _ = patchJSON(t, e.h, tok, "/ai/threads/"+tid, `{"title":"Mi hilo"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename code = %d", rec.Code)
	}

	// Mensajes del hilo.
	rec, body = getJSON(t, e.h, tok, "/ai/threads/"+tid+"/messages")
	if rec.Code != http.StatusOK {
		t.Fatalf("messages code = %d", rec.Code)
	}

	// Borrar.
	rec = deleteReq(t, e.h, tok, "/ai/threads/"+tid)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete code = %d", rec.Code)
	}
}

func TestThreadOwnership404(t *testing.T) {
	comp := &fakeCompleter{chatOut: "x"}
	e := newEnv(t, true, comp)
	_, ownerTok := e.user(t, "owner@b.com")
	_, strangerTok := e.user(t, "stranger@b.com")

	_, body := postChat(t, e.h, ownerTok, `{"message":"propio"}`)
	tid, _ := body["thread_id"].(string)

	rec, _ := getJSON(t, e.h, strangerTok, "/ai/threads/"+tid+"/messages")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ajeno messages code = %d, want 404", rec.Code)
	}
	rec = deleteReq(t, e.h, strangerTok, "/ai/threads/"+tid)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ajeno delete code = %d, want 404", rec.Code)
	}
}
