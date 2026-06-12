package ai

import (
	"context"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
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

// CreatePairWithAction es CreatePair pero el mensaje del asistente lleva una
// acción propuesta (kind + payload + status 'proposed'). Misma transacción.
func (s *pgChatStore) CreatePairWithAction(ctx context.Context, userID uuid.UUID, userText, assistantText, kind string, payload []byte) (store.AiMessage, error) {
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
	status := "proposed"
	assistant, err := qtx.CreateMessageWithAction(ctx, store.CreateMessageWithActionParams{
		UserID: userID, Role: "assistant", Content: assistantText,
		ActionKind: &kind, ActionPayload: payload, ActionStatus: &status,
	})
	if err != nil {
		return store.AiMessage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return store.AiMessage{}, err
	}
	return assistant, nil
}

// GetMessageForAction y SetActionStatus exponen las queries con el pool simple
// (sin transacción): la transición atómica la garantiza el WHERE de la query.
func (s *pgChatStore) GetMessageForAction(ctx context.Context, id, userID uuid.UUID) (store.AiMessage, error) {
	return s.q.GetMessageForAction(ctx, store.GetMessageForActionParams{ID: id, UserID: userID})
}

func (s *pgChatStore) SetActionStatus(ctx context.Context, id, userID uuid.UUID, status string) (store.AiMessage, error) {
	return s.q.SetActionStatus(ctx, store.SetActionStatusParams{ID: id, UserID: userID, ActionStatus: &status})
}
