package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestCreateAndGetInsight(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "ai@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	day := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	snap := []byte(`{"streak":{"best_current":12}}`)

	created, err := q.CreateInsight(ctx, store.CreateInsightParams{
		UserID:          user.ID,
		InsightDate:     day,
		Kind:            "proactive",
		Content:         "Tu racha de 12 días está en juego: cierra el hábito hoy.",
		ContextSnapshot: snap,
	})
	if err != nil {
		t.Fatalf("CreateInsight: %v", err)
	}
	if created.Content == "" || created.GeneratedAt.IsZero() {
		t.Errorf("insight creado incompleto: %+v", created)
	}

	got, err := q.GetInsight(ctx, store.GetInsightParams{
		UserID: user.ID, InsightDate: day, Kind: "proactive",
	})
	if err != nil {
		t.Fatalf("GetInsight: %v", err)
	}
	if got.ID != created.ID || got.Content != created.Content {
		t.Errorf("get != create: %+v vs %+v", got, created)
	}

	other := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	if _, err := q.GetInsight(ctx, store.GetInsightParams{
		UserID: user.ID, InsightDate: other, Kind: "proactive",
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("esperaba ErrNoRows para día sin insight, got %v", err)
	}
}
