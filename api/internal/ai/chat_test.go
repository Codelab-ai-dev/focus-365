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
	toolCalls   []ToolCall
}

func (f *fakeChatGroq) Chat(ctx context.Context, system string, history []ChatMsg) (string, error) {
	f.called++
	f.lastSystem = system
	f.lastHistory = history
	return f.out, f.err
}

// ChatStream del fake: emite chatDeltas en orden y devuelve su concatenación,
// o err si está seteado (simula corte a medias tras emitir los deltas).
func (f *fakeChatGroq) ChatStream(ctx context.Context, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, []ToolCall, error) {
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
	return full, f.toolCalls, nil
}

// fakeCtx devuelve un JSON fijo.
type fakeCtx struct {
	out string
	err error
}

func (f fakeCtx) build(ctx context.Context, userID uuid.UUID, today time.Time) (string, error) {
	return f.out, f.err
}

// memStore es un messageStore en memoria (sin DB) por usuario. Guarda los
// mensajes y sus acciones por separado, como la tabla ai_actions.
type memStore struct {
	rows    []store.AiMessage
	actions []store.AiAction
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

func (m *memStore) CreatePairWithActions(ctx context.Context, userID uuid.UUID, userText, assistantText string, actions []ProposedAction) (store.AiMessage, []store.AiAction, error) {
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
	out := make([]store.AiAction, 0, len(actions))
	for i, a := range actions {
		act := store.AiAction{
			ID: uuid.New(), MessageID: assistant.ID, UserID: userID,
			Position: int32(i), Kind: a.Kind, Payload: a.Payload, Status: "proposed",
			CreatedAt: time.Now(),
		}
		m.actions = append(m.actions, act)
		out = append(out, act)
	}
	return assistant, out, nil
}

func (m *memStore) ListActionsByMessages(ctx context.Context, messageIDs []uuid.UUID) ([]store.AiAction, error) {
	want := make(map[uuid.UUID]bool, len(messageIDs))
	for _, id := range messageIDs {
		want[id] = true
	}
	out := make([]store.AiAction, 0, len(m.actions))
	for _, a := range m.actions {
		if want[a.MessageID] {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *memStore) GetAction(ctx context.Context, id, userID uuid.UUID) (store.AiAction, error) {
	for _, a := range m.actions {
		if a.ID == id && a.UserID == userID {
			return a, nil
		}
	}
	return store.AiAction{}, pgx.ErrNoRows
}

func (m *memStore) SetActionStatusFrom(ctx context.Context, id, userID uuid.UUID, to string, result []byte, from string) (store.AiAction, error) {
	for i, a := range m.actions {
		if a.ID == id && a.UserID == userID && a.Status == from {
			m.actions[i].Status = to
			if result != nil {
				m.actions[i].Result = result
			}
			return m.actions[i], nil
		}
	}
	return store.AiAction{}, pgx.ErrNoRows
}

func TestChatSendStreamToolCallPersistsProposal(t *testing.T) {
	groq := &fakeChatGroq{toolCalls: []ToolCall{{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	uid := uuid.New()

	msg, err := svc.SendStream(context.Background(), uid, "registra mi check-in", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if len(msg.Actions) != 1 || msg.Actions[0].Kind != "checkin" || msg.Actions[0].Status != "proposed" {
		t.Fatalf("actions = %+v", msg.Actions)
	}
	if msg.ID == "" {
		t.Error("falta ID en el mensaje")
	}
	if msg.Actions[0].ID == "" {
		t.Error("falta ID en la acción")
	}
	if msg.Content == "" {
		t.Error("el contenido no debe quedar vacío (resumen generado)")
	}
	if len(groq.lastTools) != 7 {
		t.Errorf("tools enviadas = %d, want 7", len(groq.lastTools))
	}
	if len(st.rows) != 2 || len(st.actions) != 1 {
		t.Errorf("persistencia = rows=%+v actions=%+v", st.rows, st.actions)
	}
}

func TestChatSendStreamUnknownToolDegrades(t *testing.T) {
	groq := &fakeChatGroq{toolCalls: []ToolCall{{Name: "borrar_todo", Arguments: `{}`}}}
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
	groq := &fakeChatGroq{toolCalls: []ToolCall{{Name: "marcar_habito", Arguments: `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`}}}
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
	if len(last.Actions) != 1 || last.Actions[0].Kind != "habito" || last.ID == "" {
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
	groq := &fakeChatGroq{toolCalls: []ToolCall{{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}}
	st := &memStore{}
	c := &fakeCheckinSvc{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{}), true)
	uid := uuid.New()
	msg := proposeCheckin(t, svc, uid)
	actionID := uuid.MustParse(msg.Actions[0].ID)

	got, err := svc.ConfirmAction(context.Background(), uid, actionID, time.Now())
	if err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	if got.Status != "done" {
		t.Errorf("status = %+v", got)
	}
	if c.in == nil || c.in.Mood != 8 {
		t.Errorf("no ejecutó el check-in: %+v", c.in)
	}

	// Doble confirm → conflicto.
	if _, err := svc.ConfirmAction(context.Background(), uid, actionID, time.Now()); !errors.Is(err, ErrActionConflict) {
		t.Errorf("doble confirm err = %v, want ErrActionConflict", err)
	}
}

func TestCancelActionTransitionsWithoutExecuting(t *testing.T) {
	groq := &fakeChatGroq{toolCalls: []ToolCall{{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}}
	st := &memStore{}
	c := &fakeCheckinSvc{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{}), true)
	uid := uuid.New()
	msg := proposeCheckin(t, svc, uid)

	got, err := svc.CancelAction(context.Background(), uid, uuid.MustParse(msg.Actions[0].ID))
	if err != nil {
		t.Fatalf("CancelAction: %v", err)
	}
	if got.Status != "cancelled" {
		t.Errorf("status = %+v", got)
	}
	if c.in != nil {
		t.Error("cancelar no debe ejecutar nada")
	}
}

func TestConfirmActionNotFound(t *testing.T) {
	svc := NewChatService(fakeCtx{out: "{}"}, &memStore{}, &fakeChatGroq{}, &fakeChatGroq{}, newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{}), true)
	if _, err := svc.ConfirmAction(context.Background(), uuid.New(), uuid.New(), time.Now()); !errors.Is(err, ErrActionNotFound) {
		t.Errorf("err = %v, want ErrActionNotFound", err)
	}
}

func TestChatSendStreamMultipleActions(t *testing.T) {
	groq := &fakeChatGroq{toolCalls: []ToolCall{
		{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":7,"discipline":9}`},
		{Name: "marcar_habito", Arguments: `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`},
	}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	msg, err := svc.SendStream(context.Background(), uuid.New(), "check-in y meditación", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if len(msg.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(msg.Actions))
	}
	if msg.Actions[0].Kind != "checkin" || msg.Actions[1].Kind != "habito" {
		t.Errorf("kinds = %s, %s", msg.Actions[0].Kind, msg.Actions[1].Kind)
	}
	if msg.Content == "" {
		t.Error("contenido de fallback vacío")
	}
}

func TestChatSendStreamTooManyActionsDegrades(t *testing.T) {
	var calls []ToolCall
	for i := 0; i < 6; i++ {
		calls = append(calls, ToolCall{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":7,"discipline":9}`})
	}
	st := &memStore{}
	groq := &fakeChatGroq{toolCalls: calls}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	if _, err := svc.SendStream(context.Background(), uuid.New(), "x", time.Now(), func(string) {}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("no debe persistir nada")
	}
}

func TestChatSendStreamOneInvalidActionDiscardsAll(t *testing.T) {
	groq := &fakeChatGroq{toolCalls: []ToolCall{
		{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":7,"discipline":9}`},
		{Name: "tool_inexistente", Arguments: `{}`},
	}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, nil, true)
	if _, err := svc.SendStream(context.Background(), uuid.New(), "x", time.Now(), func(string) {}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if len(st.rows) != 0 {
		t.Error("all-or-nothing: nada persistido")
	}
}

func TestConfirmActionOnPlainMessageIsNotFound(t *testing.T) {
	groq := &fakeChatGroq{chatDeltas: []string{"hola"}}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, groq, newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{}), true)
	uid := uuid.New()
	msg, err := svc.SendStream(context.Background(), uid, "hola", time.Now(), func(string) {})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}
	if _, err := svc.ConfirmAction(context.Background(), uid, uuid.MustParse(msg.ID), time.Now()); !errors.Is(err, ErrActionNotFound) {
		t.Errorf("err = %v, want ErrActionNotFound", err)
	}
}

// confirmCheckin propone y confirma un check-in, devolviendo el service, uid y actionID.
func confirmCheckin(t *testing.T, c *fakeCheckinSvc) (*ChatService, uuid.UUID, uuid.UUID) {
	t.Helper()
	groq := &fakeChatGroq{toolCalls: []ToolCall{{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}}
	svc := NewChatService(fakeCtx{out: "{}"}, &memStore{}, groq, groq, newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{}), true)
	uid := uuid.New()
	msg := proposeCheckin(t, svc, uid)
	actionID := uuid.MustParse(msg.Actions[0].ID)
	if _, err := svc.ConfirmAction(context.Background(), uid, actionID, time.Now()); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}
	return svc, uid, actionID
}

func TestUndoActionRevierteYTransiciona(t *testing.T) {
	c := &fakeCheckinSvc{} // sin previo → undo borra
	svc, uid, actionID := confirmCheckin(t, c)

	got, err := svc.UndoAction(context.Background(), uid, actionID)
	if err != nil {
		t.Fatalf("UndoAction: %v", err)
	}
	if got.Status != "undone" {
		t.Errorf("status = %s, want undone", got.Status)
	}
	if !c.deleted {
		t.Error("undo sin previo debía borrar el check-in")
	}
}

func TestUndoActionSoloUnaVez(t *testing.T) {
	c := &fakeCheckinSvc{}
	svc, uid, actionID := confirmCheckin(t, c)
	if _, err := svc.UndoAction(context.Background(), uid, actionID); err != nil {
		t.Fatalf("primer undo: %v", err)
	}
	if _, err := svc.UndoAction(context.Background(), uid, actionID); !errors.Is(err, ErrActionConflict) {
		t.Errorf("segundo undo err = %v, want ErrActionConflict", err)
	}
}

func TestUndoActionDeProposedEsConflicto(t *testing.T) {
	groq := &fakeChatGroq{toolCalls: []ToolCall{{Name: "registrar_checkin", Arguments: `{"mood":8,"energy":6,"discipline":9}`}}}
	svc := NewChatService(fakeCtx{out: "{}"}, &memStore{}, groq, groq, newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{}), true)
	uid := uuid.New()
	msg := proposeCheckin(t, svc, uid)
	actionID := uuid.MustParse(msg.Actions[0].ID)
	// Sin confirmar: la acción sigue proposed.
	if _, err := svc.UndoAction(context.Background(), uid, actionID); !errors.Is(err, ErrActionConflict) {
		t.Errorf("undo de proposed err = %v, want ErrActionConflict", err)
	}
}

func TestUndoActionNotFound(t *testing.T) {
	svc := NewChatService(fakeCtx{out: "{}"}, &memStore{}, &fakeChatGroq{}, &fakeChatGroq{}, newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{}), true)
	if _, err := svc.UndoAction(context.Background(), uuid.New(), uuid.New()); !errors.Is(err, ErrActionNotFound) {
		t.Errorf("err = %v, want ErrActionNotFound", err)
	}
}

func TestUndoActionErrorDeDBNoTransiciona(t *testing.T) {
	// El check-in se confirma sin error; el undo (Delete) falla → sigue done.
	c := &fakeCheckinSvc{}
	svc, uid, actionID := confirmCheckin(t, c)
	c.err = errors.New("db caída") // afecta al Delete del undo
	if _, err := svc.UndoAction(context.Background(), uid, actionID); err == nil {
		t.Fatal("esperaba error de DB en el undo")
	}
	// La acción sigue done: un segundo undo (sin error) debe poder revertirla.
	c.err = nil
	if got, err := svc.UndoAction(context.Background(), uid, actionID); err != nil || got.Status != "undone" {
		t.Errorf("reintento undo: got %+v err %v", got, err)
	}
}
