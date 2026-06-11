package ai

import (
	"context"
	"errors"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

// ErrUnavailable indica que la IA no está disponible (sin clave o fallo de Groq).
// El handler lo traduce a 503.
var ErrUnavailable = errors.New("asistente no disponible")

// chatHistoryLimit es cuántos turnos previos enviamos a Groq como contexto
// conversacional (la cola del historial).
const chatHistoryLimit = 10

// chatCompleter abstrae la llamada de chat a Groq (testeable con fake).
type chatCompleter interface {
	Chat(ctx context.Context, system string, history []ChatMsg) (string, error)
}

// messageStore lee el historial y persiste el par usuario+asistente del chat.
// La implementación de producción (pgChatStore) hace la escritura en una
// transacción para no dejar mensajes huérfanos.
type messageStore interface {
	ListMessages(ctx context.Context, userID uuid.UUID) ([]store.AiMessage, error)
	CreatePair(ctx context.Context, userID uuid.UUID, userText, assistantText string) (store.AiMessage, error)
}

// contextBuilder abstrae el armado del contexto (lo implementa chatContextBuilder).
type contextBuilder interface {
	build(ctx context.Context, userID uuid.UUID, today time.Time) (string, error)
}

// ChatService orquesta la conversación: contexto + historial + Groq + persistencia.
type ChatService struct {
	ctxb   contextBuilder
	store  messageStore
	groq   chatCompleter
	hasKey bool
}

// NewChatService inyecta el constructor de contexto, el store de mensajes, el
// cliente de chat (Groq o fake) y si hay clave configurada.
func NewChatService(ctxb contextBuilder, q messageStore, c chatCompleter, hasKey bool) *ChatService {
	return &ChatService{ctxb: ctxb, store: q, groq: c, hasKey: hasKey}
}

// History devuelve el historial completo del usuario, mapeado a la vista.
func (s *ChatService) History(ctx context.Context, userID uuid.UUID) ([]Message, error) {
	rows, err := s.store.ListMessages(ctx, userID)
	if err != nil {
		return nil, err
	}
	return mapMessages(rows), nil
}

// Send procesa una pregunta: arma contexto, carga la cola del historial, llama a
// Groq y solo ante éxito persiste el par pregunta+respuesta. Devuelve la
// respuesta del asistente. Degrada a ErrUnavailable sin clave o ante fallo de IA.
func (s *ChatService) Send(ctx context.Context, userID uuid.UUID, text string, today time.Time) (*Message, error) {
	if !s.hasKey {
		return nil, ErrUnavailable
	}

	contextJSON, err := s.ctxb.build(ctx, userID, today)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.ListMessages(ctx, userID)
	if err != nil {
		return nil, err
	}
	history := buildHistory(rows, text)

	reply, err := s.groq.Chat(ctx, buildChatSystemPrompt(contextJSON), history)
	if err != nil {
		return nil, ErrUnavailable
	}

	// Solo ante éxito persistimos el par, y de forma atómica (evita mensajes de
	// usuario huérfanos si fallara el segundo insert).
	assistant, err := s.store.CreatePair(ctx, userID, text, reply)
	if err != nil {
		return nil, err
	}
	v := Message{Role: assistant.Role, Content: assistant.Content, CreatedAt: assistant.CreatedAt}
	return &v, nil
}

// buildHistory toma la cola del historial (últimos chatHistoryLimit) y agrega el
// mensaje nuevo del usuario al final, en formato ChatMsg para Groq.
func buildHistory(rows []store.AiMessage, newText string) []ChatMsg {
	start := 0
	if len(rows) > chatHistoryLimit {
		start = len(rows) - chatHistoryLimit
	}
	out := make([]ChatMsg, 0, chatHistoryLimit+1)
	for _, r := range rows[start:] {
		out = append(out, ChatMsg{Role: r.Role, Content: r.Content})
	}
	out = append(out, ChatMsg{Role: "user", Content: newText})
	return out
}

func mapMessages(rows []store.AiMessage) []Message {
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, Message{Role: r.Role, Content: r.Content, CreatedAt: r.CreatedAt})
	}
	return out
}
