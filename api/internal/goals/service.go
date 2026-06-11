package goals

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	q *store.Queries
}

func NewService(q *store.Queries) *Service {
	return &Service{q: q}
}

// List devuelve las metas del usuario en el estado dado, con overdue calculado.
func (s *Service) List(ctx context.Context, userID uuid.UUID, status string, today time.Time) ([]Goal, error) {
	rows, err := s.q.ListGoals(ctx, store.ListGoalsParams{UserID: userID, Status: status})
	if err != nil {
		return nil, err
	}
	out := make([]Goal, 0, len(rows))
	for _, g := range rows {
		out = append(out, *buildGoal(g, today))
	}
	return out, nil
}

// Create crea una meta activa con progreso 0.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, in GoalInput, today time.Time) (*Goal, error) {
	g, err := s.q.CreateGoal(ctx, store.CreateGoalParams{
		UserID:    userID,
		Title:     strings.TrimSpace(in.Title),
		Dimension: in.Dimension,
		Deadline:  in.Deadline,
	})
	if err != nil {
		return nil, err
	}
	return buildGoal(g, today), nil
}

// Patch aplica los campos presentes. (nil, nil) si la meta no es del usuario.
func (s *Service) Patch(ctx context.Context, userID, id uuid.UUID, p GoalPatch, today time.Time) (*Goal, error) {
	if p.Title != nil {
		t := strings.TrimSpace(*p.Title)
		p.Title = &t
	}
	g, err := s.q.UpdateGoal(ctx, store.UpdateGoalParams{
		ID:          id,
		UserID:      userID,
		Title:       p.Title,
		Dimension:   p.Dimension,
		Status:      p.Status,
		Progress:    p.Progress,
		SetDeadline: p.SetDeadline,
		Deadline:    p.Deadline,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return buildGoal(g, today), nil
}

// Delete borra la meta. Devuelve si borró algo.
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	n, err := s.q.DeleteGoal(ctx, store.DeleteGoalParams{ID: id, UserID: userID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func buildGoal(g store.Goal, today time.Time) *Goal {
	return &Goal{
		ID:        g.ID.String(),
		Title:     g.Title,
		Dimension: g.Dimension,
		Status:    g.Status,
		Progress:  g.Progress,
		Deadline:  g.Deadline,
		Overdue:   computeOverdue(g.Status, g.Deadline, today),
		CreatedAt: g.CreatedAt,
	}
}
