package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestGoalsDimensionCheck(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{Email: "g4d@b.com", PasswordHash: "h", Name: "G"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	// Crear una meta con una 4D válida pasa.
	if _, err := q.CreateGoal(ctx, store.CreateGoalParams{
		UserID: u.ID, Title: "Ahorrar", Dimension: "financiera", Deadline: nil,
	}); err != nil {
		t.Fatalf("dimensión 4D válida rechazada: %v", err)
	}
	// Una dimensión fuera de las 4D (vieja) viola el CHECK.
	if _, err := q.CreateGoal(ctx, store.CreateGoalParams{
		UserID: u.ID, Title: "X", Dimension: "general", Deadline: nil,
	}); err == nil {
		t.Error("dimensión 'general' debería violar el CHECK")
	}
}
