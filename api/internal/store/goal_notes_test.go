package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/google/uuid"
)

func mkGoal(t *testing.T, q *store.Queries, user uuid.UUID) uuid.UUID {
	t.Helper()
	g, err := q.CreateGoal(context.Background(), store.CreateGoalParams{
		UserID: user, Title: "Meta", Dimension: "fisica", Deadline: nil,
	})
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	return g.ID
}

func date(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}

func TestCreateGoalNoteOwnershipGuard(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	owner := newUser(t, q)
	stranger := newUser(t, q)
	goalID := mkGoal(t, q, owner)

	// dueño: inserta
	n, err := q.CreateGoalNote(ctx, store.CreateGoalNoteParams{
		GoalID: goalID, UserID: owner, NoteDate: date("2026-06-17"), Body: "avancé",
	})
	if err != nil || n.Body != "avancé" {
		t.Fatalf("create propio: %v n=%+v", err, n)
	}
	// extraño sobre la meta del dueño: 0 filas -> ErrNoRows
	if _, err := q.CreateGoalNote(ctx, store.CreateGoalNoteParams{
		GoalID: goalID, UserID: stranger, NoteDate: date("2026-06-17"), Body: "intruso",
	}); err == nil {
		t.Fatal("crear nota en meta ajena debería fallar")
	}
}

func TestListGoalNotesOrderAndScope(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)
	goalID := mkGoal(t, q, u)
	for _, d := range []string{"2026-06-10", "2026-06-15", "2026-06-12"} {
		if _, err := q.CreateGoalNote(ctx, store.CreateGoalNoteParams{
			GoalID: goalID, UserID: u, NoteDate: date(d), Body: d,
		}); err != nil {
			t.Fatalf("create %s: %v", d, err)
		}
	}
	rows, err := q.ListGoalNotes(ctx, store.ListGoalNotesParams{GoalID: goalID, UserID: u})
	if err != nil {
		t.Fatalf("ListGoalNotes: %v", err)
	}
	if len(rows) != 3 || rows[0].Body != "2026-06-15" || rows[2].Body != "2026-06-10" {
		t.Fatalf("orden por fecha desc incorrecto: %+v", rows)
	}
	// scope: otro usuario no ve nada
	other := newUser(t, q)
	r2, _ := q.ListGoalNotes(ctx, store.ListGoalNotesParams{GoalID: goalID, UserID: other})
	if len(r2) != 0 {
		t.Fatalf("scope: el otro vio %d notas", len(r2))
	}
}

func TestDeleteGoalNoteAndCascade(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	owner := newUser(t, q)
	stranger := newUser(t, q)
	goalID := mkGoal(t, q, owner)
	n, _ := q.CreateGoalNote(ctx, store.CreateGoalNoteParams{
		GoalID: goalID, UserID: owner, NoteDate: date("2026-06-17"), Body: "x",
	})
	// extraño no borra
	got, _ := q.DeleteGoalNote(ctx, store.DeleteGoalNoteParams{ID: n.ID, UserID: stranger})
	if got != 0 {
		t.Fatalf("borrado ajeno afectó %d filas", got)
	}
	// dueño borra
	got, _ = q.DeleteGoalNote(ctx, store.DeleteGoalNoteParams{ID: n.ID, UserID: owner})
	if got != 1 {
		t.Fatalf("borrado propio afectó %d, want 1", got)
	}
	// cascada: nueva nota y borro la meta -> notas eliminadas
	n2, _ := q.CreateGoalNote(ctx, store.CreateGoalNoteParams{
		GoalID: goalID, UserID: owner, NoteDate: date("2026-06-18"), Body: "y",
	})
	_ = n2
	if _, err := q.DeleteGoal(ctx, store.DeleteGoalParams{ID: goalID, UserID: owner}); err != nil {
		t.Fatalf("DeleteGoal: %v", err)
	}
	left, _ := q.ListGoalNotes(ctx, store.ListGoalNotesParams{GoalID: goalID, UserID: owner})
	if len(left) != 0 {
		t.Fatalf("cascada falló: quedaron %d notas", len(left))
	}
}
