package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func TestUpsertGetListCheckIns(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "c@b.com", PasswordHash: "h", Name: "Caro",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	d10 := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	d11 := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	// Insert inicial.
	ci, err := q.UpsertCheckIn(ctx, store.UpsertCheckInParams{
		UserID: user.ID, Date: d10, Mood: 7, Energy: 6, Win: "buen día",
	})
	if err != nil {
		t.Fatalf("UpsertCheckIn insert: %v", err)
	}
	if ci.Mood != 7 || ci.Win != "buen día" {
		t.Errorf("valores insertados incorrectos: %+v", ci)
	}

	// Upsert el mismo día actualiza (no duplica): mismo ID, valores nuevos.
	ci2, err := q.UpsertCheckIn(ctx, store.UpsertCheckInParams{
		UserID: user.ID, Date: d10, Mood: 3, Energy: 4, Win: "regular",
	})
	if err != nil {
		t.Fatalf("UpsertCheckIn update: %v", err)
	}
	if ci2.ID != ci.ID {
		t.Errorf("el upsert debería actualizar la misma fila, IDs: %s vs %s", ci2.ID, ci.ID)
	}
	if ci2.Mood != 3 {
		t.Errorf("mood actualizado = %d, want 3", ci2.Mood)
	}

	// Otro día → fila distinta.
	if _, err := q.UpsertCheckIn(ctx, store.UpsertCheckInParams{
		UserID: user.ID, Date: d11, Mood: 9, Energy: 9, Win: "",
	}); err != nil {
		t.Fatalf("UpsertCheckIn d11: %v", err)
	}

	// GetCheckInByDate.
	got, err := q.GetCheckInByDate(ctx, store.GetCheckInByDateParams{UserID: user.ID, Date: d10})
	if err != nil {
		t.Fatalf("GetCheckInByDate: %v", err)
	}
	if got.Mood != 3 {
		t.Errorf("get mood = %d, want 3", got.Mood)
	}

	// ListCheckIns: descendente por fecha, respeta limit.
	list, err := q.ListCheckIns(ctx, store.ListCheckInsParams{UserID: user.ID, Limit: 30})
	if err != nil {
		t.Fatalf("ListCheckIns: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if !list[0].Date.After(list[1].Date) {
		t.Errorf("la lista no está descendente: %v, %v", list[0].Date, list[1].Date)
	}

	limited, err := q.ListCheckIns(ctx, store.ListCheckInsParams{UserID: user.ID, Limit: 1})
	if err != nil {
		t.Fatalf("ListCheckIns limit: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("limit=1 devolvió %d filas", len(limited))
	}
}
