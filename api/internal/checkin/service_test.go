package checkin_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestServiceUpsertTodayList(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	svc := checkin.NewService(q)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{Email: "s@b.com", PasswordHash: "h", Name: "Sol"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	date := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)

	// Today sin check-in → nil, sin error.
	none, err := svc.Today(ctx, user.ID, date)
	if err != nil {
		t.Fatalf("Today vacío: %v", err)
	}
	if none != nil {
		t.Errorf("Today debería ser nil cuando no hay check-in, got %+v", none)
	}

	// Upsert y verificar formato de fecha.
	ci, err := svc.Upsert(ctx, user.ID, checkin.Input{
		Date: date, Mood: 7, Energy: 6, Discipline: 8, Note: "ok",
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if ci.Date != "2026-06-10" {
		t.Errorf("Date = %q, want 2026-06-10", ci.Date)
	}
	if ci.Mood != 7 {
		t.Errorf("Mood = %d, want 7", ci.Mood)
	}

	// Today ahora devuelve el check-in.
	got, err := svc.Today(ctx, user.ID, date)
	if err != nil {
		t.Fatalf("Today: %v", err)
	}
	if got == nil || got.Note != "ok" {
		t.Errorf("Today = %+v, want note=ok", got)
	}

	// List devuelve el historial.
	list, err := svc.List(ctx, user.ID, 30)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len(list) = %d, want 1", len(list))
	}
}
