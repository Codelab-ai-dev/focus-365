package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/google/uuid"
)

func TestHabitsStore(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "hab@b.com", PasswordHash: "h", Name: "Gus",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// CreateHabit es idempotente por (user_id, lower(name)) entre activos.
	target := int32(21)
	h1, err := q.CreateHabit(ctx, store.CreateHabitParams{
		UserID: user.ID, Name: "Leer", TargetDays: &target,
	})
	if err != nil {
		t.Fatalf("CreateHabit: %v", err)
	}
	h2, err := q.CreateHabit(ctx, store.CreateHabitParams{
		UserID: user.ID, Name: "leer", TargetDays: nil,
	})
	if err != nil {
		t.Fatalf("CreateHabit dup: %v", err)
	}
	if h1.ID != h2.ID {
		t.Errorf("el create duplicó el hábito activo: %s vs %s", h1.ID, h2.ID)
	}

	// UpsertHabitLog es idempotente por (habit_id, day).
	day := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	if err := q.UpsertHabitLog(ctx, store.UpsertHabitLogParams{HabitID: h1.ID, Day: day}); err != nil {
		t.Fatalf("UpsertHabitLog: %v", err)
	}
	if err := q.UpsertHabitLog(ctx, store.UpsertHabitLogParams{HabitID: h1.ID, Day: day}); err != nil {
		t.Fatalf("UpsertHabitLog dup: %v", err)
	}
	logs, err := q.ListLogsByHabitIDs(ctx, []uuid.UUID{h1.ID})
	if err != nil {
		t.Fatalf("ListLogsByHabitIDs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %d, want 1 (no duplica por día)", len(logs))
	}

	// DeleteHabitLog quita el día.
	if err := q.DeleteHabitLog(ctx, store.DeleteHabitLogParams{HabitID: h1.ID, Day: day}); err != nil {
		t.Fatalf("DeleteHabitLog: %v", err)
	}
	logs2, _ := q.ListLogsByHabitIDs(ctx, []uuid.UUID{h1.ID})
	if len(logs2) != 0 {
		t.Errorf("el log no se borró: quedan %d", len(logs2))
	}

	// ListHabits trae activos; ArchiveHabit lo saca y aparece en archivados.
	active, _ := q.ListHabits(ctx, user.ID)
	if len(active) != 1 {
		t.Fatalf("activos = %d, want 1", len(active))
	}
	if _, err := q.ArchiveHabit(ctx, store.ArchiveHabitParams{ID: h1.ID, UserID: user.ID}); err != nil {
		t.Fatalf("ArchiveHabit: %v", err)
	}
	active2, _ := q.ListHabits(ctx, user.ID)
	if len(active2) != 0 {
		t.Errorf("activos tras archivar = %d, want 0", len(active2))
	}
	arch, _ := q.ListArchivedHabits(ctx, user.ID)
	if len(arch) != 1 {
		t.Errorf("archivados = %d, want 1", len(arch))
	}

	// El único parcial permite recrear el nombre tras archivar.
	h3, err := q.CreateHabit(ctx, store.CreateHabitParams{
		UserID: user.ID, Name: "Leer", TargetDays: nil,
	})
	if err != nil {
		t.Fatalf("CreateHabit tras archivar: %v", err)
	}
	if h3.ID == h1.ID {
		t.Errorf("debería crear un hábito nuevo, no devolver el archivado")
	}

	// DeleteHabit borra el hábito y sus logs (cascade), scoped por usuario.
	if err := q.UpsertHabitLog(ctx, store.UpsertHabitLogParams{HabitID: h3.ID, Day: day}); err != nil {
		t.Fatalf("UpsertHabitLog h3: %v", err)
	}
	n, err := q.DeleteHabit(ctx, store.DeleteHabitParams{ID: h3.ID, UserID: user.ID})
	if err != nil {
		t.Fatalf("DeleteHabit: %v", err)
	}
	if n != 1 {
		t.Fatalf("DeleteHabit afectó %d filas, want 1", n)
	}
	gone, _ := q.ListLogsByHabitIDs(ctx, []uuid.UUID{h3.ID})
	if len(gone) != 0 {
		t.Errorf("los logs no cayeron en cascada: quedan %d", len(gone))
	}
}
