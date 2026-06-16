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

// ErrThreadNotFound indica que el hilo no existe o no es del usuario.
// El handler lo traduce a 404.
var ErrThreadNotFound = errors.New("hilo no encontrado")

// chatHistoryLimit es cuántos turnos previos enviamos a Groq como contexto
// conversacional (la cola del historial).
const chatHistoryLimit = 10

// maxThreadTitle es el largo máximo del título autogenerado/renombrado (runes).
const maxThreadTitle = 60

// deriveTitle arma el título de un hilo nuevo a partir del primer mensaje:
// recorta espacios y limita a maxThreadTitle runes. Si queda vacío, "Nuevo hilo".
func deriveTitle(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return "Nuevo hilo"
	}
	r := []rune(t)
	if len(r) > maxThreadTitle {
		return string(r[:maxThreadTitle])
	}
	return t
}

// chatCompleter abstrae la llamada de chat a Groq (testeable con fake).
type chatCompleter interface {
	Chat(ctx context.Context, system string, history []ChatMsg) (string, error)
}

// chatStreamer abstrae la llamada de chat en streaming (testeable con fake).
type chatStreamer interface {
	ChatStream(ctx context.Context, system string, history []ChatMsg, tools []Tool, onDelta func(string)) (string, []ToolCall, error)
}

// messageStore lee el historial y persiste el par usuario+asistente del chat.
// La implementación de producción (pgChatStore) hace la escritura en una
// transacción para no dejar mensajes huérfanos.
type messageStore interface {
	// Hilos
	ListThreads(ctx context.Context, userID uuid.UUID) ([]store.ListThreadsRow, error)
	GetThread(ctx context.Context, threadID, userID uuid.UUID) (store.AiThread, error)
	RenameThread(ctx context.Context, threadID, userID uuid.UUID, title string) (store.AiThread, error)
	DeleteThread(ctx context.Context, threadID, userID uuid.UUID) (int64, error)
	ListThreadMessages(ctx context.Context, threadID uuid.UUID) ([]store.AiMessage, error)

	// CreateTurn persiste el par usuario+asistente (y las acciones propuestas)
	// en una transacción. Si threadID es nil, crea primero el hilo con `title`.
	// Devuelve el id del hilo resuelto y la fila del asistente.
	CreateTurn(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID, title, userText, assistantText string, actions []ProposedAction) (uuid.UUID, store.AiMessage, []store.AiAction, error)

	// Acciones (sin cambios)
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

// Threads devuelve los hilos del usuario para la lista (ordenados por actividad).
func (s *ChatService) Threads(ctx context.Context, userID uuid.UUID) ([]ThreadView, error) {
	rows, err := s.store.ListThreads(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ThreadView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ThreadView{
			ID: r.ID.String(), Title: r.Title, Preview: r.Preview, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

// RenameThread cambia el título (validando dueño y no-vacío). 404 si no es del usuario.
func (s *ChatService) RenameThread(ctx context.Context, userID, threadID uuid.UUID, title string) (*ThreadView, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, ErrActionInvalid // se traduce a 400; reusamos el error de validación
	}
	if r := []rune(title); len(r) > maxThreadTitle {
		title = string(r[:maxThreadTitle])
	}
	row, err := s.store.RenameThread(ctx, threadID, userID, title)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrThreadNotFound
		}
		return nil, err
	}
	v := ThreadView{ID: row.ID.String(), Title: row.Title, UpdatedAt: row.UpdatedAt}
	return &v, nil
}

// DeleteThread borra el hilo (cascada de mensajes y acciones). 404 si no es del usuario.
func (s *ChatService) DeleteThread(ctx context.Context, userID, threadID uuid.UUID) error {
	n, err := s.store.DeleteThread(ctx, threadID, userID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrThreadNotFound
	}
	return nil
}

// HistoryByThread devuelve los mensajes de un hilo (con acciones colgadas).
// Valida que el hilo sea del usuario (404 si no).
func (s *ChatService) HistoryByThread(ctx context.Context, userID, threadID uuid.UUID) ([]Message, error) {
	if _, err := s.store.GetThread(ctx, threadID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrThreadNotFound
		}
		return nil, err
	}
	rows, err := s.store.ListThreadMessages(ctx, threadID)
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
		k := uuid.UUID(a.MessageID.Bytes).String()
		byMsg[k] = append(byMsg[k], toActionView(a))
	}
	for i := range msgs {
		msgs[i].Actions = byMsg[msgs[i].ID]
	}
	return msgs, nil
}

// resolveThread valida el hilo destino: si threadID no es nil, confirma dueño
// (404 si no) y devuelve la cola de mensajes del hilo. Si es nil, devuelve cola
// vacía (hilo nuevo, se crea al persistir).
func (s *ChatService) resolveThread(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID) ([]store.AiMessage, error) {
	if threadID == nil {
		return nil, nil
	}
	if _, err := s.store.GetThread(ctx, *threadID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrThreadNotFound
		}
		return nil, err
	}
	return s.store.ListThreadMessages(ctx, *threadID)
}

// Send procesa una pregunta en un hilo (o crea uno nuevo si threadID es nil).
// Devuelve la respuesta y el id del hilo resuelto.
func (s *ChatService) Send(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID, text string, today time.Time) (*Message, uuid.UUID, error) {
	if !s.hasKey {
		return nil, uuid.Nil, ErrUnavailable
	}
	contextJSON, err := s.ctxb.build(ctx, userID, today)
	if err != nil {
		return nil, uuid.Nil, err
	}
	rows, err := s.resolveThread(ctx, userID, threadID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	history := buildHistory(rows, text)

	reply, err := s.groq.Chat(ctx, buildChatSystemPrompt(contextJSON), history)
	if err != nil {
		return nil, uuid.Nil, ErrUnavailable
	}
	tid, assistant, _, err := s.store.CreateTurn(ctx, userID, threadID, deriveTitle(text), text, reply, nil)
	if err != nil {
		return nil, uuid.Nil, err
	}
	v := toMessageView(assistant)
	return &v, tid, nil
}

// SendStream es la variante streaming de Send.
func (s *ChatService) SendStream(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID, text string, today time.Time, onDelta func(string)) (*Message, uuid.UUID, error) {
	if !s.hasKey {
		return nil, uuid.Nil, ErrUnavailable
	}
	contextJSON, err := s.ctxb.build(ctx, userID, today)
	if err != nil {
		return nil, uuid.Nil, err
	}
	rows, err := s.resolveThread(ctx, userID, threadID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	history := buildHistory(rows, text)

	reply, toolCalls, err := s.streamer.ChatStream(ctx, buildChatSystemPrompt(contextJSON), history, buildChatTools(), onDelta)
	if err != nil {
		return nil, uuid.Nil, ErrUnavailable
	}

	var proposed []ProposedAction
	content := reply
	if len(toolCalls) > 0 {
		if len(toolCalls) > maxActionsPerTurn {
			return nil, uuid.Nil, ErrUnavailable
		}
		proposed = make([]ProposedAction, 0, len(toolCalls))
		summaries := make([]string, 0, len(toolCalls))
		for _, tc := range toolCalls {
			kind, ok := toolNameToKind[tc.Name]
			if !ok {
				return nil, uuid.Nil, ErrUnavailable
			}
			payload, perr := parseActionPayload(kind, tc.Arguments)
			if perr != nil {
				return nil, uuid.Nil, ErrUnavailable
			}
			proposed = append(proposed, ProposedAction{Kind: kind, Payload: payload})
			summaries = append(summaries, actionSummary(kind, payload))
		}
		content = strings.TrimSpace(reply)
		if content == "" {
			content = strings.Join(summaries, " ")
		}
	}

	tid, assistant, actions, cerr := s.store.CreateTurn(ctx, userID, threadID, deriveTitle(text), text, content, proposed)
	if cerr != nil {
		return nil, uuid.Nil, cerr
	}
	v := toMessageView(assistant)
	for _, a := range actions {
		v.Actions = append(v.Actions, toActionView(a))
	}
	return &v, tid, nil
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
	result, err := s.exec.execute(ctx, userID, row.Kind, row.Payload, today)
	if err != nil {
		return nil, err
	}
	upd, err := s.store.SetActionStatusFrom(ctx, actionID, userID, "done", result, "proposed")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionConflict
		}
		return nil, err
	}
	v := toActionView(upd)
	return &v, nil
}

// UndoAction revierte una acción done y la marca undone. Solo transiciona si
// la reversa fue exitosa; un error real de DB deja la acción en done.
func (s *ChatService) UndoAction(ctx context.Context, userID, actionID uuid.UUID) (*ActionView, error) {
	row, err := s.store.GetAction(ctx, actionID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrActionNotFound
		}
		return nil, err
	}
	if row.Status != "done" {
		return nil, ErrActionConflict
	}
	if err := s.exec.undo(ctx, userID, row.Kind, row.Payload, row.Result); err != nil {
		return nil, err
	}
	upd, err := s.store.SetActionStatusFrom(ctx, actionID, userID, "undone", nil, "done")
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
