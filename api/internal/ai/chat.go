package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// chatStreamer abstrae la llamada de chat en streaming (testeable con fake).
type chatStreamer interface {
	ChatStream(ctx context.Context, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, *ToolCall, error)
}

// messageStore lee el historial y persiste el par usuario+asistente del chat.
// La implementación de producción (pgChatStore) hace la escritura en una
// transacción para no dejar mensajes huérfanos.
type messageStore interface {
	ListMessages(ctx context.Context, userID uuid.UUID) ([]store.AiMessage, error)
	CreatePair(ctx context.Context, userID uuid.UUID, userText, assistantText string) (store.AiMessage, error)
	CreatePairWithActions(ctx context.Context, userID uuid.UUID, userText, assistantText string, actions []ProposedAction) (store.AiMessage, []store.AiAction, error)
	ListActionsByMessages(ctx context.Context, messageIDs []uuid.UUID) ([]store.AiAction, error)
	GetAction(ctx context.Context, id, userID uuid.UUID) (store.AiAction, error)
	SetActionStatusFrom(ctx context.Context, id, userID uuid.UUID, to string, result []byte, from string) (store.AiAction, error)
}

// contextBuilder abstrae el armado del contexto (lo implementa chatContextBuilder).
type contextBuilder interface {
	build(ctx context.Context, userID uuid.UUID, today time.Time) (string, error)
}

// ChatService orquesta la conversación: contexto + historial + Groq + persistencia.
type ChatService struct {
	ctxb     contextBuilder
	store    messageStore
	groq     chatCompleter
	streamer chatStreamer
	exec     *actionExecutor
	hasKey   bool
}

// NewChatService inyecta el constructor de contexto, el store de mensajes, los
// clientes de chat bloqueante y streaming (GroqClient implementa ambos), el
// ejecutor de acciones y si hay clave configurada.
func NewChatService(ctxb contextBuilder, q messageStore, c chatCompleter, s chatStreamer, exec *actionExecutor, hasKey bool) *ChatService {
	return &ChatService{ctxb: ctxb, store: q, groq: c, streamer: s, exec: exec, hasKey: hasKey}
}

// History devuelve el historial completo del usuario, mapeado a la vista, con
// las acciones de cada mensaje colgadas (un solo query para evitar N+1).
func (s *ChatService) History(ctx context.Context, userID uuid.UUID) ([]Message, error) {
	rows, err := s.store.ListMessages(ctx, userID)
	if err != nil {
		return nil, err
	}
	msgs := mapMessages(rows)
	if len(rows) == 0 {
		return msgs, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	acts, err := s.store.ListActionsByMessages(ctx, ids)
	if err != nil {
		return nil, err
	}
	byMsg := make(map[string][]ActionView)
	for _, a := range acts {
		k := a.MessageID.String()
		byMsg[k] = append(byMsg[k], toActionView(a))
	}
	for i := range msgs {
		msgs[i].Actions = byMsg[msgs[i].ID]
	}
	return msgs, nil
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
	v := toMessageView(assistant)
	return &v, nil
}

// SendStream es la variante streaming de Send: re-emite cada delta vía onDelta
// y, solo si el stream de Groq terminó sin error, persiste el par completo de
// forma atómica. Corte a medias → ErrUnavailable y nada persistido.
func (s *ChatService) SendStream(ctx context.Context, userID uuid.UUID, text string, today time.Time, onDelta func(string)) (*Message, error) {
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

	reply, toolCall, err := s.streamer.ChatStream(ctx, buildChatSystemPrompt(contextJSON), history, buildChatTools(), onDelta)
	if err != nil {
		return nil, ErrUnavailable
	}

	if toolCall != nil {
		kind, ok := toolNameToKind[toolCall.Name]
		if !ok {
			return nil, ErrUnavailable
		}
		payload, perr := parseActionPayload(kind, toolCall.Arguments)
		if perr != nil {
			return nil, ErrUnavailable
		}
		content := strings.TrimSpace(reply)
		if content == "" {
			content = actionSummary(kind, payload)
		}
		assistant, actions, cerr := s.store.CreatePairWithActions(ctx, userID, text, content,
			[]ProposedAction{{Kind: kind, Payload: payload}})
		if cerr != nil {
			return nil, cerr
		}
		v := toMessageView(assistant)
		for _, a := range actions {
			v.Actions = append(v.Actions, toActionView(a))
		}
		return &v, nil
	}

	assistant, err := s.store.CreatePair(ctx, userID, text, reply)
	if err != nil {
		return nil, err
	}
	v := toMessageView(assistant)
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
		out = append(out, toMessageView(r))
	}
	return out
}

// toMessageView mapea la fila a la vista (sin acciones; estas se cuelgan aparte).
func toMessageView(r store.AiMessage) Message {
	return Message{ID: r.ID.String(), Role: r.Role, Content: r.Content, CreatedAt: r.CreatedAt}
}

// toActionView mapea una fila de ai_actions a la vista.
func toActionView(a store.AiAction) ActionView {
	return ActionView{ID: a.ID.String(), Kind: a.Kind, Payload: json.RawMessage(a.Payload), Status: a.Status}
}

// ConfirmAction ejecuta la acción propuesta y la marca done.
// Solo transiciona si la ejecución fue exitosa.
func (s *ChatService) ConfirmAction(ctx context.Context, userID, actionID uuid.UUID, today time.Time) (*ActionView, error) {
	row, err := s.store.GetAction(ctx, actionID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionNotFound
		}
		return nil, err
	}
	if row.Status != "proposed" {
		return nil, ErrActionConflict
	}
	if err := s.exec.execute(ctx, userID, row.Kind, row.Payload, today); err != nil {
		return nil, err
	}
	upd, err := s.store.SetActionStatusFrom(ctx, actionID, userID, "done", nil, "proposed")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionConflict
		}
		return nil, err
	}
	v := toActionView(upd)
	return &v, nil
}

// CancelAction marca la propuesta como cancelada sin ejecutar nada.
func (s *ChatService) CancelAction(ctx context.Context, userID, actionID uuid.UUID) (*ActionView, error) {
	row, err := s.store.GetAction(ctx, actionID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionNotFound
		}
		return nil, err
	}
	if row.Status != "proposed" {
		return nil, ErrActionConflict
	}
	upd, err := s.store.SetActionStatusFrom(ctx, actionID, userID, "cancelled", nil, "proposed")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionConflict
		}
		return nil, err
	}
	v := toActionView(upd)
	return &v, nil
}
