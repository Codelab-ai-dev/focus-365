package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestTrainingSuggestionUpsert(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)

	// no existe
	if _, err := q.GetTrainingSuggestion(ctx, u); err != pgx.ErrNoRows {
		t.Fatalf("sin sugerencia: err = %v, want ErrNoRows", err)
	}

	// insert
	s1, err := q.UpsertTrainingSuggestion(ctx, store.UpsertTrainingSuggestionParams{
		UserID: u, Focus: "pierna", Content: "rutina A",
	})
	if err != nil || s1.Content != "rutina A" || s1.Focus != "pierna" {
		t.Fatalf("insert: %v %+v", err, s1)
	}

	// upsert reemplaza (sigue una fila)
	s2, err := q.UpsertTrainingSuggestion(ctx, store.UpsertTrainingSuggestionParams{
		UserID: u, Focus: "", Content: "rutina B",
	})
	if err != nil || s2.Content != "rutina B" || s2.Focus != "" {
		t.Fatalf("update: %v %+v", err, s2)
	}

	got, err := q.GetTrainingSuggestion(ctx, u)
	if err != nil || got.Content != "rutina B" {
		t.Fatalf("get tras upsert: %v %+v", err, got)
	}
}
