package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestTrainingAdjustmentUpsert(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)

	if _, err := q.GetTrainingAdjustment(ctx, u); err != pgx.ErrNoRows {
		t.Fatalf("sin análisis: err = %v, want ErrNoRows", err)
	}
	a1, err := q.UpsertTrainingAdjustment(ctx, store.UpsertTrainingAdjustmentParams{
		UserID: u, Scope: "last", Content: "ajuste A",
	})
	if err != nil || a1.Scope != "last" || a1.Content != "ajuste A" {
		t.Fatalf("insert: %v %+v", err, a1)
	}
	a2, err := q.UpsertTrainingAdjustment(ctx, store.UpsertTrainingAdjustmentParams{
		UserID: u, Scope: "week", Content: "ajuste B",
	})
	if err != nil || a2.Scope != "week" || a2.Content != "ajuste B" {
		t.Fatalf("update: %v %+v", err, a2)
	}
	got, err := q.GetTrainingAdjustment(ctx, u)
	if err != nil || got.Content != "ajuste B" {
		t.Fatalf("get: %v %+v", err, got)
	}
}
