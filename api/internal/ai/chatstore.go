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
// necesario para abrir la transacción de CreateTurn).
func NewChatStore(q *store.Queries, pool *pgxpool.Pool) *pgChatStore {
	return &pgChatStore{q: q, pool: pool}
}

func (s *pgChatStore) ListThreads(ctx context.Context, userID uuid.UUID) ([]store.ListThreadsRow, error) {
	return s.q.ListThreads(ctx, userID)
}

func (s *pgChatStore) GetThread(ctx context.Context, threadID, userID uuid.UUID) (store.AiThread, error) {
	return s.q.GetThread(ctx, store.GetThreadParams{ID: threadID, UserID: userID})
}

func (s *pgChatStore) RenameThread(ctx context.Context, threadID, userID uuid.UUID, title string) (store.AiThread, error) {
	return s.q.RenameThread(ctx, store.RenameThreadParams{ID: threadID, UserID: userID, Title: title})
}

func (s *pgChatStore) DeleteThread(ctx context.Context, threadID, userID uuid.UUID) (int64, error) {
	return s.q.DeleteThread(ctx, store.DeleteThreadParams{ID: threadID, UserID: userID})
}

func (s *pgChatStore) ListThreadMessages(ctx context.Context, threadID uuid.UUID) ([]store.AiMessage, error) {
	return s.q.ListThreadMessages(ctx, threadID)
}

// ProposedAction es una acción validada lista para persistir como proposed.
type ProposedAction struct {
	Kind    string
	Payload json.RawMessage
}

// CreateTurn crea (si hace falta) el hilo y persiste el par + acciones en una
// transacción. Devuelve el id del hilo resuelto y la fila del asistente.
func (s *pgChatStore) CreateTurn(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID, title, userText, assistantText string, actions []ProposedAction) (uuid.UUID, store.AiMessage, []store.AiAction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, store.AiMessage{}, nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	var tid uuid.UUID
	if threadID == nil {
		th, terr := qtx.CreateThread(ctx, store.CreateThreadParams{UserID: userID, Title: title})
		if terr != nil {
			return uuid.Nil, store.AiMessage{}, nil, terr
		}
		tid = th.ID
	} else {
		tid = *threadID
	}

	if _, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, ThreadID: tid, Role: "user", Content: userText,
	}); err != nil {
		return uuid.Nil, store.AiMessage{}, nil, err
	}
	assistant, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, ThreadID: tid, Role: "assistant", Content: assistantText,
	})
	if err != nil {
		return uuid.Nil, store.AiMessage{}, nil, err
	}
	rows := make([]store.AiAction, 0, len(actions))
	for i, a := range actions {
		row, aerr := qtx.CreateAction(ctx, store.CreateActionParams{
			MessageID: pgtype.UUID{Bytes: assistant.ID, Valid: true},
			UserID:    userID, Position: int32(i),
			Kind:      a.Kind, Payload: a.Payload, Status: "proposed",
		})
		if aerr != nil {
			return uuid.Nil, store.AiMessage{}, nil, aerr
		}
		rows = append(rows, row)
	}
	// Si el hilo ya existía, refrescamos su actividad para reordenar la lista.
	// (Si es nuevo, su updated_at ya es now() por el default.)
	if threadID != nil {
		if err := qtx.TouchThread(ctx, tid); err != nil {
			return uuid.Nil, store.AiMessage{}, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, store.AiMessage{}, nil, err
	}
	return tid, assistant, rows, nil
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
