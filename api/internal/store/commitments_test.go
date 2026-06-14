package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestCommitmentRoundTrip(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{Email: "com@b.com", PasswordHash: "h", Name: "C"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	target := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	c, err := q.CreateCommitment(ctx, store.CreateCommitmentParams{
		UserID: u.ID, TargetDate: target, Text: "Tender la cama", Position: 0,
	})
	if err != nil {
		t.Fatalf("CreateCommitment: %v", err)
	}
	if c.Done {
		t.Error("nuevo commitment no debe estar done")
	}
	toggled, err := q.ToggleCommitment(ctx, store.ToggleCommitmentParams{ID: c.ID, UserID: u.ID})
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if !toggled.Done {
		t.Error("toggle debe poner done=true")
	}
	list, err := q.ListCommitmentsByTarget(ctx, store.ListCommitmentsByTargetParams{UserID: u.ID, TargetDate: target})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Text != "Tender la cama" {
		t.Errorf("list = %+v", list)
	}
}
