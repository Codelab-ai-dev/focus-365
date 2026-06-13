package ai

import (
	"context"
	"encoding/json"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgChatStore implementa messageStore sobre Postgres. Persiste el par
// usuario+asistente dentro de una transacción para no dejar un mensaje de
// usuario huérfano si fallara el segundo insert.
type pgChatStore struct {
	q    *store.Queries
	pool *pgxpool.Pool
}

// NewChatStore arma el store del chat con las queries y el pool (el pool es
// necesario para abrir la transacción de CreatePair).
func NewChatStore(q *store.Queries, pool *pgxpool.Pool) *pgChatStore {
	return &pgChatStore{q: q, pool: pool}
}

func (s *pgChatStore) ListMessages(ctx context.Context, userID uuid.UUID) ([]store.AiMessage, error) {
	return s.q.ListMessages(ctx, userID)
}

// CreatePair inserta la pregunta del usuario y la respuesta del asistente en una
// sola transacción y devuelve la fila del asistente. Si algo falla hace rollback
// y no deja mensajes a medias.
func (s *pgChatStore) CreatePair(ctx context.Context, userID uuid.UUID, userText, assistantText string) (store.AiMessage, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.AiMessage{}, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	if _, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "user", Content: userText,
	}); err != nil {
		return store.AiMessage{}, err
	}
	assistant, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "assistant", Content: assistantText,
	})
	if err != nil {
		return store.AiMessage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AiMessage{}, err
	}
	return assistant, nil
}

// ProposedAction es una acción validada lista para persistir como proposed.
type ProposedAction struct {
	Kind    string
	Payload json.RawMessage
}

// CreatePairWithActions persiste el par usuario+asistente y las N acciones
// propuestas del turno en una sola transacción.
func (s *pgChatStore) CreatePairWithActions(ctx context.Context, userID uuid.UUID, userText, assistantText string, actions []ProposedAction) (store.AiMessage, []store.AiAction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return store.AiMessage{}, nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	if _, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "user", Content: userText,
	}); err != nil {
		return store.AiMessage{}, nil, err
	}
	assistant, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, Role: "assistant", Content: assistantText,
	})
	if err != nil {
		return store.AiMessage{}, nil, err
	}
	rows := make([]store.AiAction, 0, len(actions))
	for i, a := range actions {
		row, err := qtx.CreateAction(ctx, store.CreateActionParams{
			MessageID: pgtype.UUID{Bytes: assistant.ID, Valid: true},
			UserID: userID, Position: int32(i),
			Kind: a.Kind, Payload: a.Payload, Status: "proposed",
		})
		if err != nil {
			return store.AiMessage{}, nil, err
		}
		rows = append(rows, row)
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AiMessage{}, nil, err
	}
	return assistant, rows, nil
}

func (s *pgChatStore) ListActionsByMessages(ctx context.Context, messageIDs []uuid.UUID) ([]store.AiAction, error) {
	return s.q.ListActionsByMessages(ctx, messageIDs)
}

func (s *pgChatStore) GetAction(ctx context.Context, id, userID uuid.UUID) (store.AiAction, error) {
	return s.q.GetAction(ctx, store.GetActionParams{ID: id, UserID: userID})
}

// SetActionStatusFrom transiciona el estado solo si está en from (atómico) y
// guarda result si no es nil.
func (s *pgChatStore) SetActionStatusFrom(ctx context.Context, id, userID uuid.UUID, to string, result []byte, from string) (store.AiAction, error) {
	return s.q.SetActionStatusFrom(ctx, store.SetActionStatusFromParams{
		ID: id, UserID: userID, Status: to, Result: result, Status_2: from,
	})
}

// CreateUploadActions persiste N movimientos extraídos de un archivo como
// acciones source='upload' (sin mensaje de chat), en una transacción.
func (s *pgChatStore) CreateUploadActions(ctx context.Context, userID uuid.UUID, actions []ProposedAction) ([]store.AiAction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	rows := make([]store.AiAction, 0, len(actions))
	for i, a := range actions {
		row, err := qtx.CreateUploadAction(ctx, store.CreateUploadActionParams{
			UserID: userID, Position: int32(i), Kind: a.Kind, Payload: a.Payload,
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *pgChatStore) ListPendingUploadActions(ctx context.Context, userID uuid.UUID) ([]store.AiAction, error) {
	return s.q.ListPendingUploadActions(ctx, userID)
}
