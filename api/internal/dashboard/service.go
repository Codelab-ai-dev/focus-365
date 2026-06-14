package dashboard

import (
	"context"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/training"
	"github.com/google/uuid"
)

// Service compone los cinco servicios de dominio para armar el snapshot.
type Service struct {
	checkins *checkin.Service
	finance  *finance.Service
	training *training.Service
	habits   *habits.Service
	goals    *goals.Service
}

// NewService inyecta los servicios existentes (punteros compartidos con server.go).
func NewService(c *checkin.Service, f *finance.Service, t *training.Service,
	h *habits.Service, g *goals.Service) *Service {
	return &Service{checkins: c, finance: f, training: t, habits: h, goals: g}
}

// Snapshot consulta cada servicio para today y arma la vista agregada. Es
// todo-o-nada: si cualquier sub-llamada falla, propaga el error (→ 500).
func (s *Service) Snapshot(ctx context.Context, userID uuid.UUID, today time.Time) (*Snapshot, error) {
	hs, err := s.habits.List(ctx, userID, false, today)
	if err != nil {
		return nil, err
	}
	sum, err := s.finance.Summary(ctx, userID, finance.Cycle(today), today)
	if err != nil {
		return nil, err
	}
	ci, err := s.checkins.Today(ctx, userID, today)
	if err != nil {
		return nil, err
	}
	ws, err := s.training.ListWorkouts(ctx, userID, &today, &today)
	if err != nil {
		return nil, err
	}
	gs, err := s.goals.List(ctx, userID, "active", today)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{
		Streak:   streakView(hs),
		Finance:  financeView(sum),
		Checkin:  checkinView(ci),
		Training: trainingView(ws),
		Goals:    goalsView(gs),
	}
	snap.DimensionsActive = countActive(snap)
	return snap, nil
}

func streakView(hs []habits.Habit) StreakView {
	v := StreakView{Total: len(hs)}
	for _, h := range hs {
		if h.CurrentStreak > v.BestCurrent {
			v.BestCurrent = h.CurrentStreak
		}
		if h.DoneToday {
			v.DoneToday++
		}
	}
	return v
}

func financeView(cs *finance.CycleSummary) FinanceView {
	return FinanceView{Cycle: cs.Cycle, Net: cs.Net, Status: cs.Status}
}

func checkinView(c *checkin.CheckIn) *CheckinView {
	if c == nil {
		return nil
	}
	return &CheckinView{Present: true, Mood: c.Mood, Energy: c.Energy, Win: c.Win}
}

func trainingView(ws []training.Workout) TrainingView {
	if len(ws) == 0 {
		return TrainingView{}
	}
	return TrainingView{TrainedToday: true, Type: ws[0].Type}
}

func goalsView(gs []goals.Goal) GoalsView {
	v := GoalsView{Active: len(gs)}
	sum := 0
	for _, g := range gs {
		sum += int(g.Progress)
		if g.Overdue {
			v.Overdue++
		}
	}
	if len(gs) > 0 {
		v.AvgProgress = sum / len(gs)
	}
	return v
}
