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

	// Upsert para la misma fecha actualiza la misma fila (sin duplicar).
	ci2, err := svc.Upsert(ctx, user.ID, checkin.Input{
		Date: date, Mood: 3, Energy: 6, Discipline: 8, Note: "ok",
	})
	if err != nil {
		t.Fatalf("Upsert mismo día: %v", err)
	}
	if ci2.ID != ci.ID {
		t.Errorf("ID = %q, want %q (no debe crear fila nueva)", ci2.ID, ci.ID)
	}
	if ci2.Mood != 3 {
		t.Errorf("Mood = %d, want 3 (actualizado)", ci2.Mood)
	}

	// Segundo check-in en fecha anterior para verificar orden descendente.
	earlier := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	if _, err := svc.Upsert(ctx, user.ID, checkin.Input{
		Date: earlier, Mood: 5, Energy: 5, Discipline: 5, Note: "ayer",
	}); err != nil {
		t.Fatalf("Upsert fecha anterior: %v", err)
	}

	// List devuelve el historial en orden descendente por fecha.
	list, err := svc.List(ctx, user.ID, 30)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
	if list[0].Date != "2026-06-10" {
		t.Errorf("list[0].Date = %q, want 2026-06-10 (orden date DESC)", list[0].Date)
	}
}

func TestDeleteCheckIn(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	svc := checkin.NewService(q)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{Email: "del@b.com", PasswordHash: "h", Name: "Del"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	date := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)

	// 1. Upsert de un check-in del día.
	if _, err := svc.Upsert(ctx, user.ID, checkin.Input{
		Date: date, Mood: 8, Energy: 7, Discipline: 9, Note: "test",
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// 2. Delete → deleted == true.
	deleted, err := svc.Delete(ctx, user.ID, date)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Error("Delete debería devolver true cuando existe el check-in")
	}

	// 3. Today → (nil, nil).
	got, err := svc.Today(ctx, user.ID, date)
	if err != nil {
		t.Fatalf("Today tras Delete: %v", err)
	}
	if got != nil {
		t.Errorf("Today debería ser nil tras Delete, got %+v", got)
	}

	// 4. Delete de nuevo → deleted == false (idempotente).
	deleted2, err := svc.Delete(ctx, user.ID, date)
	if err != nil {
		t.Fatalf("Delete idempotente: %v", err)
	}
	if deleted2 {
		t.Error("Delete repetido debería devolver false (ya no existe)")
	}

	// 5. Delete con OTRO usuario sobre el mismo día → false (scoping).
	other, err := q.CreateUser(ctx, store.CreateUserParams{Email: "other@b.com", PasswordHash: "h", Name: "Other"})
	if err != nil {
		t.Fatalf("CreateUser otro: %v", err)
	}
	// Primero crear un check-in para el primer usuario en esa fecha (ya fue borrado), no hay nada.
	// El otro usuario no tiene check-in: Delete debe devolver false.
	deletedOther, err := svc.Delete(ctx, other.ID, date)
	if err != nil {
		t.Fatalf("Delete otro usuario: %v", err)
	}
	if deletedOther {
		t.Error("Delete de otro usuario sin check-in debería devolver false")
	}
}
