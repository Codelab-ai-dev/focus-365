package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

// fakeChatGroq registra lo que recibió y devuelve out/err.
type fakeChatGroq struct {
	out         string
	err         error
	called      int
	lastSystem  string
	lastHistory []ChatMsg
}

func (f *fakeChatGroq) Chat(ctx context.Context, system string, history []ChatMsg) (string, error) {
	f.called++
	f.lastSystem = system
	f.lastHistory = history
	return f.out, f.err
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

func (m *memStore) CreateMessage(ctx context.Context, arg store.CreateMessageParams) (store.AiMessage, error) {
	row := store.AiMessage{
		ID: uuid.New(), UserID: arg.UserID, Role: arg.Role, Content: arg.Content,
		CreatedAt: time.Now().Add(time.Duration(len(m.rows)) * time.Millisecond),
	}
	m.rows = append(m.rows, row)
	return row, nil
}

func TestChatSendPersistsPairAndReturnsAssistant(t *testing.T) {
	groq := &fakeChatGroq{out: "Vas verde este ciclo."}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: `{"snapshot":{}}`}, st, groq, true)
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
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, true)

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
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, false)

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
	svc := NewChatService(fakeCtx{out: "{}"}, st, groq, true)

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
	svc := NewChatService(fakeCtx{}, st, &fakeChatGroq{}, true)

	msgs, err := svc.History(context.Background(), uid)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "hola" {
		t.Errorf("history = %+v", msgs)
	}
}
