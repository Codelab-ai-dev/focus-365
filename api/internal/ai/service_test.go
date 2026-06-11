package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeSnap struct {
	snap *dashboard.Snapshot
	err  error
}

func (f fakeSnap) Snapshot(ctx context.Context, userID uuid.UUID, today time.Time) (*dashboard.Snapshot, error) {
	return f.snap, f.err
}

type fakeStore struct {
	got       *store.AiInsight // nil → ErrNoRows en GetInsight
	created   *store.CreateInsightParams
	createErr error
}

func (f *fakeStore) GetInsight(ctx context.Context, arg store.GetInsightParams) (store.AiInsight, error) {
	if f.got == nil {
		return store.AiInsight{}, pgx.ErrNoRows
	}
	return *f.got, nil
}

func (f *fakeStore) CreateInsight(ctx context.Context, arg store.CreateInsightParams) (store.AiInsight, error) {
	if f.createErr != nil {
		return store.AiInsight{}, f.createErr
	}
	f.created = &arg
	return store.AiInsight{
		ID: uuid.New(), UserID: arg.UserID, InsightDate: arg.InsightDate,
		Kind: arg.Kind, Content: arg.Content, ContextSnapshot: arg.ContextSnapshot,
		GeneratedAt: time.Now(),
	}, nil
}

type fakeCompleter struct {
	out    string
	err    error
	called int
}

func (f *fakeCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	f.called++
	return f.out, f.err
}

var (
	testUser = uuid.New()
	testDay  = time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
)

func emptySnap() *dashboard.Snapshot { return &dashboard.Snapshot{} }

func TestDailyInsightCacheHit(t *testing.T) {
	st := &fakeStore{got: &store.AiInsight{Content: "cacheado", GeneratedAt: testDay}}
	comp := &fakeCompleter{out: "nuevo"}
	svc := NewService(fakeSnap{snap: emptySnap()}, st, comp, true)

	got, err := svc.DailyInsight(context.Background(), testUser, testDay)
	if err != nil {
		t.Fatalf("DailyInsight: %v", err)
	}
	if !got.Available || got.Content != "cacheado" {
		t.Errorf("esperaba cache hit, got %+v", got)
	}
	if comp.called != 0 {
		t.Errorf("no debía llamar a Groq con cache, called=%d", comp.called)
	}
}

func TestDailyInsightNoKey(t *testing.T) {
	st := &fakeStore{}
	comp := &fakeCompleter{out: "x"}
	svc := NewService(fakeSnap{snap: emptySnap()}, st, comp, false)

	got, err := svc.DailyInsight(context.Background(), testUser, testDay)
	if err != nil {
		t.Fatalf("DailyInsight: %v", err)
	}
	if got.Available {
		t.Errorf("sin clave debería degradar, got %+v", got)
	}
	if comp.called != 0 || st.created != nil {
		t.Errorf("sin clave no debe llamar Groq ni persistir")
	}
}

func TestDailyInsightGroqError(t *testing.T) {
	st := &fakeStore{}
	comp := &fakeCompleter{err: errors.New("boom")}
	svc := NewService(fakeSnap{snap: emptySnap()}, st, comp, true)

	got, err := svc.DailyInsight(context.Background(), testUser, testDay)
	if err != nil {
		t.Fatalf("DailyInsight: %v", err)
	}
	if got.Available {
		t.Errorf("fallo de Groq debería degradar, got %+v", got)
	}
	if st.created != nil {
		t.Errorf("fallo de Groq no debe persistir")
	}
}

func TestDailyInsightGeneratesAndPersists(t *testing.T) {
	st := &fakeStore{}
	comp := &fakeCompleter{out: "Aprovecha tu energía alta hoy."}
	svc := NewService(fakeSnap{snap: emptySnap()}, st, comp, true)

	got, err := svc.DailyInsight(context.Background(), testUser, testDay)
	if err != nil {
		t.Fatalf("DailyInsight: %v", err)
	}
	if !got.Available || got.Content != "Aprovecha tu energía alta hoy." {
		t.Errorf("esperaba insight generado, got %+v", got)
	}
	if comp.called != 1 {
		t.Errorf("debía llamar a Groq una vez, called=%d", comp.called)
	}
	if st.created == nil || st.created.Content != "Aprovecha tu energía alta hoy." {
		t.Errorf("no persistió el insight: %+v", st.created)
	}
}

func TestDailyInsightSnapshotError(t *testing.T) {
	st := &fakeStore{}
	comp := &fakeCompleter{out: "x"}
	svc := NewService(fakeSnap{err: errors.New("db caída")}, st, comp, true)

	if _, err := svc.DailyInsight(context.Background(), testUser, testDay); err == nil {
		t.Fatal("error de Snapshot debe propagarse (500)")
	}
}
