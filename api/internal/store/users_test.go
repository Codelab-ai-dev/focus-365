package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestCreateAndGetUser(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	created, err := q.CreateUser(ctx, store.CreateUserParams{
		Email:        "a@b.com",
		PasswordHash: "hash",
		Name:         "Ana",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.Email != "a@b.com" {
		t.Errorf("email = %q", created.Email)
	}

	byEmail, err := q.GetUserByEmail(ctx, "a@b.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if byEmail.ID != created.ID {
		t.Errorf("id mismatch")
	}

	byID, err := q.GetUserByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if byID.Name != "Ana" {
		t.Errorf("name = %q", byID.Name)
	}
}
