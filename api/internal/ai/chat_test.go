package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// fakeChatGroq registra lo que recibió y devuelve out/err.
type fakeChatGroq struct {
	out         string
	err         error
	called      int
	lastSystem  string
	lastHistory []ChatMsg
	lastTools   []Tool
	chatDeltas  []string
	toolCall    *ToolCall
}

func (f *fakeChatGroq) Chat(ctx context.Context, system string, history []ChatMsg) (string, error) {
	f.called++
	f.lastSystem = system
	f.lastHistory = history
	return f.out, f.err
}

// ChatStream del fake: emite chatDeltas en orden y devuelve su concatenación,
// o err si está seteado (simula corte a medias tras emitir los deltas).
func (f *fakeChatGroq) ChatStream(ctx context.Context, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, *ToolCall, error) {
	f.called++
	f.lastSystem = system
	f.lastHistory = history
	f.lastTools = tools
	var full string
	for _, d := range f.chatDeltas {
		full += d
		onDelta(d)
	}
	if f.err != nil {
		return "", nil, f.err
	}
	return full, f.toolCall, nil
}

// fakeCtx devuelve un JSON fijo.
type fakeCtx struct {
	out string
	err error
}

func (f fakeCtx) build(ctx context.Context, userID uuid.UUID, today time.Time) (string, error) {
	return f.out, f.err
}

// memStore es un messageStore en memoria (sin DB) por usuario.
type memStore struct {
	rows []store.AiMessage
}

func (m *memStore) ListMessages(ctx context.Context, userID uuid.UUID) ([]store.AiMessage, error) {
	out := make([]store.AiMessage, 0, len(m.rows))
	for _, r := range m.rows {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memStore) CreatePair(ctx context.Context, userID uuid.UUID, userText, assistantText string) (store.AiMessage, error) {
	user := store.AiMessage{
		ID: uuid.New(), UserID: userID, Role: "user", Content: userText,
		CreatedAt: time.Now().Add(time.Duration(len(m.rows)) * time.Millisecond),
	}
	m.rows = append(m.rows, user)
	assistant := store.AiMessage{
		ID: uuid.New(), UserID: userID, Role: "assistant", Content: assistantText,
		CreatedAt: time.Now().Add(time.Duration(len(m.rows)) * time.Millisecond),
	}
	m.rows = append(m.rows, assistant)
	return assistant, nil
}

func TestChatSendPersistsPairAndReturnsAssistant(t *testing.T) {
	groq := &fakeChatGroq{out: "Vas verde este ciclo."}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: `{"snapshot":{}}`}, st, groq, groq, nil, true)
	uid := uuid.New()

	msg, err := svc.Send(context.Background(), uid, "¿cómo voy?", time.Now())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if msg.Role != "assistant" || msg.Content != "Vas verde este ciclo." {
		t.Errorf("reply = %+v", msg)
	}
	if len(st.rows) != 2 {
		t.Fatalf("persistió %d filas, want 2", len(st.rows))
	}
	if st.rows[0].Role != "user" || st.rows[0].Content != "¿cómo voy?" {
		t.Errorf("fila 0 = %+v", st.rows[0])
	}
	if st.rows[1].Role != "assistant" {
		t.Errorf("fila 1 = %+v", st.rows[1])
	}
	if groq.lastSystem == "" {
		t.Error("system vacío")
	}
}

func TestChatSendMultiTurnPassesHistory(t *testing.T) {
	groq := &fakeChatGroq{out: "ok"}
	uid := uuid.New()
	st := &memStore{rows: []store.AiMessage{
		{ID: uuid.New(), UserID: uid, Role: "user", Content: "hola", CreatedAt: time.Now()},
		{ID: uuid.New(), UserID: uid, Role: "assistant", Content: "qué tal", CreatedAt: time.Now().Add(time.Millisecond)},
	}}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)

	if _, err := svc.Send(context.Background(), uid, "¿cómo voy?", time.Now()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(groq.lastHistory) != 3 {
		t.Fatalf("history len = %d, want 3", len(groq.lastHistory))
	}
	last := groq.lastHistory[2]
	if last.Role != "user" || last.Content != "¿cómo voy?" {
		t.Errorf("último turno = %+v", last)
	}
}

func TestChatSendNoKeyDegrades(t *testing.T) {
	groq := &fakeChatGroq{out: "no usar"}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, false)

	_, err := svc.Send(context.Background(), uuid.New(), "hola", time.Now())
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if groq.called != 0 {
		t.Error("sin clave no debe llamar a Groq")
	}
	if len(st.rows) != 0 {
		t.Error("sin clave no debe persistir nada")
	}
}

func TestChatSendGroqFailureDoesNotPersist(t *testing.T) {
	groq := &fakeChatGroq{err: errors.New("groq caído")}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)

	_, err := svc.Send(context.Background(), uuid.New(), "hola", time.Now())
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("fallo de Groq no debe dejar mensajes huérfanos")
	}
}

func TestChatHistoryMapsRows(t *testing.T) {
	uid := uuid.New()
	st := &memStore{rows: []store.AiMessage{
		{ID: uuid.New(), UserID: uid, Role: "user", Content: "hola", CreatedAt: time.Now()},
	}}
	f := &fakeChatGroq{}
	svc := NewChatService(fakeCtx{}, st, f, f, nil, true)

	msgs, err := svc.History(context.Background(), uid)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "hola" {
		t.Errorf("history = %+v", msgs)
	}
}

func TestChatSendStreamEmitsDeltasAndPersists(t *testing.T) {
	groq := &fakeChatGroq{chatDeltas: []string{"Vas ", "bien."}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	uid := uuid.New()

	var deltas []string
	msg, err := svc.SendStream(context.Background(), uid, "¿cómo voy?", time.Now(),
		func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if len(deltas) != 2 || deltas[0] != "Vas " || deltas[1] != "bien." {
		t.Errorf("deltas = %v", deltas)
	}
	if msg.Role != "assistant" || msg.Content != "Vas bien." {
		t.Errorf("reply = %+v", msg)
	}
	if len(st.rows) != 2 || st.rows[0].Content != "¿cómo voy?" || st.rows[1].Content != "Vas bien." {
		t.Errorf("persistencia = %+v", st.rows)
	}
}

func TestChatSendStreamFailureDoesNotPersist(t *testing.T) {
	groq := &fakeChatGroq{chatDeltas: []string{"Vas "}, err: errors.New("stream cortado")}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)

	_, err := svc.SendStream(context.Background(), uuid.New(), "hola", time.Now(), func(string) {})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("corte a medias no debe persistir nada")
	}
}

func TestChatSendStreamNoKeyDegrades(t *testing.T) {
	groq := &fakeChatGroq{chatDeltas: []string{"no usar"}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, false)

	_, err := svc.SendStream(context.Background(), uuid.New(), "hola", time.Now(), func(string) {})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if groq.called != 0 {
		t.Error("sin clave no debe llamar a Groq")
	}
}

func (m *memStore) CreatePairWithAction(ctx context.Context, userID uuid.UUID, userText, assistantText, kind string, payload []byte) (store.AiMessage, error) {
	user := store.AiMessage{
		ID: uuid.New(), UserID: userID, Role: "user", Content: userText,
		CreatedAt: time.Now().Add(time.Duration(len(m.rows)) * time.Millisecond),
	}
	m.rows = append(m.rows, user)
	status := "proposed"
	assistant := store.AiMessage{
		ID: uuid.New(), UserID: userID, Role: "assistant", Content: assistantText,
		ActionKind: &kind, ActionPayload: payload, ActionStatus: &status,
		CreatedAt: time.Now().Add(time.Duration(len(m.rows)) * time.Millisecond),
	}
	m.rows = append(m.rows, assistant)
	return assistant, nil
}

func (m *memStore) GetMessageForAction(ctx context.Context, id, userID uuid.UUID) (store.AiMessage, error) {
	for _, r := range m.rows {
		if r.ID == id && r.UserID == userID {
			return r, nil
		}
	}
	return store.AiMessage{}, pgx.ErrNoRows
}

func (m *memStore) SetActionStatus(ctx context.Context, id, userID uuid.UUID, status string) (store.AiMessage, error) {
	for i, r := range m.rows {
		if r.ID == id && r.UserID == userID && r.ActionStatus != nil && *r.ActionStatus == "proposed" {
			s := status
			m.rows[i].ActionStatus = &s
			return m.rows[i], nil
		}
	}
	return store.AiMessage{}, pgx.ErrNoRows
}

func TestChatSendStreamToolCallPersistsProposal(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	uid := uuid.New()

	msg, err := svc.SendStream(context.Background(), uid, "registra mi check-in", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if msg.Action == nil || msg.Action.Kind != "checkin" || msg.Action.Status != "proposed" {
		t.Fatalf("action = %+v", msg.Action)
	}
	if msg.ID == "" {
		t.Error("falta ID en el mensaje")
	}
	if msg.Content == "" {
		t.Error("el contenido no debe quedar vacío (resumen generado)")
	}
	if len(groq.lastTools) != 7 {
		t.Errorf("tools enviadas = %d, want 7", len(groq.lastTools))
	}
	if len(st.rows) != 2 || st.rows[1].ActionKind == nil {
		t.Errorf("persistencia = %+v", st.rows)
	}
}

func TestChatSendStreamUnknownToolDegrades(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "borrar_todo", Arguments: `{}`}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)

	_, err := svc.SendStream(context.Background(), uuid.New(), "x", time.Now(), func(string) {})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("tool desconocido no debe persistir nada")
	}
}

func TestChatHistoryIncludesAction(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "marcar_habito", Arguments: `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	uid := uuid.New()
	if _, err := svc.SendStream(context.Background(), uid, "marca meditar", time.Now(), func(string) {}); err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	msgs, err := svc.History(context.Background(), uid)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	last := msgs[len(msgs)-1]
	if last.Action == nil || last.Action.Kind != "habito" || last.ID == "" {
		t.Errorf("history sin acción: %+v", last)
	}
}

func proposeCheckin(t *testing.T, svc *ChatService, uid uuid.UUID) *Message {
	t.Helper()
	msg, err := svc.SendStream(context.Background(), uid, "registra mi check-in", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	return msg
}

func TestConfirmActionExecutesAndTransitions(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}
	st := &memStore{}
	c := &fakeCheckinSvc{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{}), true)
	uid := uuid.New()
	msg := proposeCheckin(t, svc, uid)

	got, err := svc.ConfirmAction(context.Background(), uid, uuid.MustParse(msg.ID), time.Now())
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if got.Action == nil || got.Action.Status != "done" {
		t.Errorf("status = %+v", got.Action)
	}
	if c.in == nil || c.in.Mood != 8 {
		t.Errorf("no ejecutó el check-in: %+v", c.in)
	}

	// Doble confirm → conflicto.
	if _, err := svc.ConfirmAction(context.Background(), uid, uuid.MustParse(msg.ID), time.Now()); !errors.Is(err, ErrActionConflict) {
		t.Errorf("doble confirm err = %v, want ErrActionConflict", err)
	}
}

func TestCancelActionTransitionsWithoutExecuting(t *testing.T) {
	groq := &fakeChatGroq{toolCall: &ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}
	st := &memStore{}
	c := &fakeCheckinSvc{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{}), true)
	uid := uuid.New()
	msg := proposeCheckin(t, svc, uid)

	got, err := svc.CancelAction(context.Background(), uid, uuid.MustParse(msg.ID))
	if err != nil {
		t.Fatalf("CancelAction: %v", err)
	}
	if got.Action == nil || got.Action.Status != "cancelled" {
		t.Errorf("status = %+v", got.Action)
	}
	if c.in != nil {
		t.Error("cancelar no debe ejecutar nada")
	}
}

func TestConfirmActionNotFound(t *testing.T) {
	svc := NewChatService(fakeCtx{out: "{}"}, &memStore{}, &fakeChatGroq{}, &fakeChatGroq{}, newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{}), true)
	if _, err := svc.ConfirmAction(context.Background(), uuid.New(), uuid.New(), time.Now()); !errors.Is(err, ErrActionNotFound) {
		t.Errorf("err = %v, want ErrActionNotFound", err)
	}
}

func TestConfirmActionOnPlainMessageIsNotFound(t *testing.T) {
	groq := &fakeChatGroq{chatDeltas: []string{"hola"}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{}), true)
	uid := uuid.New()
	msg, err := svc.SendStream(context.Background(), uid, "hola", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if _, err := svc.ConfirmAction(context.Background(), uid, uuid.MustParse(msg.ID), time.Now()); !errors.Is(err, ErrActionNotFound) {
		t.Errorf("err = %v, want ErrActionNotFound", err)
	}
}
