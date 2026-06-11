package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/finance"
	"github.com/google/uuid"
)

// fakeSnap se reutiliza desde service_test.go (mismo paquete).

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
		{ID: "c1", Date: "2026-06-10", Mood: 7, Energy: 6, Discipline: 8},
	}
	lister := &fakeLister{list: cks}

	b := newChatContextBuilder(fakeSnap{snap: snap}, fakeCycler{cycles: cyc}, lister)
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
}

func TestChatContextPropagatesError(t *testing.T) {
	b := newChatContextBuilder(
		fakeSnap{err: errors.New("db caída")},
		fakeCycler{},
		&fakeLister{},
	)
	if _, err := b.build(context.Background(), uuid.New(), time.Now()); err == nil {
		t.Fatal("esperaba propagar el error de Snapshot")
	}
}
