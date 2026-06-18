// Package commitments gestiona los compromisos rastreables del check-in:
// lo que el usuario se compromete a hacer un día, marcable como cumplido.
package commitments

import (
	"context"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service gestiona los compromisos rastreables.
type Service struct {
	q    *store.Queries
	pool *pgxpool.Pool
}

// NewService crea un Service con las queries y el pool de conexiones.
func NewService(q *store.Queries, pool *pgxpool.Pool) *Service {
	return &Service{q: q, pool: pool}
}

// Commitment es la vista de dominio (target_date como YYYY-MM-DD).
type Commitment struct {
	ID         string `json:"id"`
	TargetDate string `json:"target_date"`
	Text       string `json:"text"`
	Done       bool   `json:"done"`
}

const dateLayout = "2006-01-02"

func toView(c store.Commitment) Commitment {
	return Commitment{
		ID:         c.ID.String(),
		TargetDate: c.TargetDate.Format(dateLayout),
		Text:       c.Text,
		Done:       c.Done,
	}
}

// DueOn devuelve los compromisos cuyo objetivo es `date` (para marcar ese día).
func (s *Service) DueOn(ctx context.Context, userID uuid.UUID, date time.Time) ([]Commitment, error) {
	rows, err := s.q.ListCommitmentsByTarget(ctx, store.ListCommitmentsByTargetParams{
		UserID:     userID,
		TargetDate: date,
	})
	if err != nil {
		return nil, err
	}
	return mapViews(rows), nil
}

// Pending devuelve los compromisos sin cumplir con target_date <= today
// (vencidos + hoy), vencidos primero. Para el panel de recordatorios de la home.
func (s *Service) Pending(ctx context.Context, userID uuid.UUID, today time.Time) ([]Commitment, error) {
	rows, err := s.q.ListPendingCommitments(ctx, store.ListPendingCommitmentsParams{
		UserID:     userID,
		TargetDate: today,
	})
	if err != nil {
		return nil, err
	}
	return mapViews(rows), nil
}

// ReplaceForDate reemplaza los compromisos del usuario para `target` (borra y
// re-inserta, filtrando vacíos), en una transacción.
func (s *Service) ReplaceForDate(ctx context.Context, userID uuid.UUID, target time.Time, texts []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.q.WithTx(tx)
	if _, err := qtx.DeleteCommitmentsForDate(ctx, store.DeleteCommitmentsForDateParams{
		UserID:     userID,
		TargetDate: target,
	}); err != nil {
		return err
	}

	pos := 0
	for _, t := range texts {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, err := qtx.CreateCommitment(ctx, store.CreateCommitmentParams{
			UserID:     userID,
			TargetDate: target,
			Text:       t,
			Position:   int32(pos),
		}); err != nil {
			return err
		}
		pos++
	}
	return tx.Commit(ctx)
}

// Toggle invierte el cumplimiento. Devuelve (nil, nil) si el commitment no
// pertenece al usuario.
func (s *Service) Toggle(ctx context.Context, userID, id uuid.UUID) (*Commitment, error) {
	row, err := s.q.ToggleCommitment(ctx, store.ToggleCommitmentParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

// Recent devuelve los compromisos con target >= since (contexto de la IA).
func (s *Service) Recent(ctx context.Context, userID uuid.UUID, since time.Time) ([]Commitment, error) {
	rows, err := s.q.ListRecentCommitments(ctx, store.ListRecentCommitmentsParams{
		UserID:     userID,
		TargetDate: since,
	})
	if err != nil {
		return nil, err
	}
	return mapViews(rows), nil
}

func mapViews(rows []store.Commitment) []Commitment {
	out := make([]Commitment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toView(r))
	}
	return out
}
