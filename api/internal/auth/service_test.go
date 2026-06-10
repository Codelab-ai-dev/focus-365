package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/google/uuid"
)

func newService(t *testing.T) *auth.Service {
	pool := testutil.NewDB(t)
	return auth.NewService(store.New(pool), auth.NewTokenManager("test-secret"))
}

func TestRegisterThenLogin(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	user, err := svc.Register(ctx, "user@focus.com", "p4ssword", "Gus")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.Email != "user@focus.com" {
		t.Errorf("email = %q", user.Email)
	}

	logged, err := svc.Login(ctx, "user@focus.com", "p4ssword")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if logged.ID != user.ID {
		t.Errorf("login devolvió otro usuario")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "dup@focus.com", "p4ssword", "A")
	if _, err := svc.Register(ctx, "dup@focus.com", "p4ssword", "B"); !errors.Is(err, auth.ErrEmailTaken) {
		t.Errorf("err = %v, want ErrEmailTaken", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "x@focus.com", "right", "X")
	if _, err := svc.Login(ctx, "x@focus.com", "wrong"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestIssueTokens(t *testing.T) {
	svc := newService(t)
	access, refresh, err := svc.IssueTokens(uuid.New())
	if err != nil {
		t.Fatalf("IssueTokens: %v", err)
	}
	if access == "" || refresh == "" {
		t.Error("access y refresh no deben estar vacíos")
	}
	if access == refresh {
		t.Error("access y refresh deben ser distintos")
	}
}
