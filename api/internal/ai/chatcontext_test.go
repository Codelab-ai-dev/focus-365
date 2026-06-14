package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/commitments"
	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/google/uuid"
)

type fakeCommitments struct {
	list []commitments.Commitment
}

func (f fakeCommitments) Recent(ctx context.Context, userID uuid.UUID, since time.Time) ([]commitments.Commitment, error) {
	return f.list, nil
}

// fakeSnap se reutiliza desde service_test.go (mismo paquete).

type fakeHabits struct {
	list []habits.Habit
	err  error
}

func (f fakeHabits) List(ctx context.Context, userID uuid.UUID, archived bool, today time.Time) ([]habits.Habit, error) {
	return f.list, f.err
}

type fakeGoals struct {
	list []goals.Goal
	err  error
}

func (f fakeGoals) List(ctx context.Context, userID uuid.UUID, status string, today time.Time) ([]goals.Goal, error) {
	return f.list, f.err
}

type fakeCycler struct {
	cycles []finance.CycleSummary
	err    error
}

func (f fakeCycler) Cycles(ctx context.Context, userID uuid.UUID, now time.Time) ([]finance.CycleSummary, error) {
	return f.cycles, f.err
}

type fakeLister struct {
	list  []checkin.CheckIn
	err   error
	limit int
}

func (f *fakeLister) List(ctx context.Context, userID uuid.UUID, limit int) ([]checkin.CheckIn, error) {
	f.limit = limit
	return f.list, f.err
}

func TestChatContextComposesJSON(t *testing.T) {
	snap := &dashboard.Snapshot{
		Streak:  dashboard.StreakView{BestCurrent: 12, DoneToday: 1, Total: 3},
		Finance: dashboard.FinanceView{Cycle: "2026-06", Net: 5000, Status: "pendiente"},
	}
	cyc := []finance.CycleSummary{
		{Cycle: "2026-05", Income: 10000, Expense: 8000, Net: 2000, Status: "verde"},
	}
	cks := []checkin.CheckIn{
		{ID: "c1", Date: "2026-06-10", Mood: 7, Energy: 6, Espiritual: "reto día 2"},
	}
	lister := &fakeLister{list: cks}
	hab := fakeHabits{list: []habits.Habit{{ID: "h1", Name: "Meditar", DoneToday: false}}}
	gls := fakeGoals{list: []goals.Goal{{ID: "g1", Title: "Ahorrar", Progress: 40, Status: "activa"}}}
	commits := fakeCommitments{list: []commitments.Commitment{{ID: "x", TargetDate: "2026-06-14", Text: "tender la cama", Done: true}}}

	b := newChatContextBuilder(fakeSnap{snap: snap}, fakeCycler{cycles: cyc}, lister, hab, gls, commits)
	out, err := b.build(context.Background(), uuid.New(), time.Now())
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if lister.limit != 14 {
		t.Errorf("limit = %d, want 14", lister.limit)
	}

	var payload struct {
		Snapshot *dashboard.Snapshot    `json:"snapshot"`
		Cycles   []finance.CycleSummary `json:"cycles"`
		Checkins []checkin.CheckIn      `json:"checkins"`
		Habits   []habits.Habit         `json:"habits"`
		Goals    []goals.Goal           `json:"goals"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json inválido: %v\n%s", err, out)
	}
	if payload.Snapshot == nil || payload.Snapshot.Streak.BestCurrent != 12 {
		t.Errorf("snapshot mal compuesto: %s", out)
	}
	if len(payload.Cycles) != 1 || payload.Cycles[0].Cycle != "2026-05" {
		t.Errorf("cycles mal compuesto: %s", out)
	}
	if len(payload.Checkins) != 1 || payload.Checkins[0].Date != "2026-06-10" {
		t.Errorf("checkins mal compuesto: %s", out)
	}
	if len(payload.Habits) != 1 || payload.Habits[0].ID != "h1" {
		t.Errorf("habits mal compuesto: %s", out)
	}
	if len(payload.Goals) != 1 || payload.Goals[0].ID != "g1" {
		t.Errorf("goals mal compuesto: %s", out)
	}
	if !strings.Contains(out, "reto día 2") {
		t.Errorf("el contexto debe incluir la reflexión espiritual: %s", out)
	}
	if !strings.Contains(out, "tender la cama") {
		t.Errorf("el contexto debe incluir los compromisos: %s", out)
	}
}

func TestChatContextPropagatesError(t *testing.T) {
	b := newChatContextBuilder(
		fakeSnap{err: errors.New("db caída")},
		fakeCycler{},
		&fakeLister{},
		fakeHabits{},
		fakeGoals{},
		fakeCommitments{},
	)
	if _, err := b.build(context.Background(), uuid.New(), time.Now()); err == nil {
		t.Fatal("esperaba propagar el error de Snapshot")
	}
}
