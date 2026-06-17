package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/google/uuid"
)

// crea un usuario y devuelve su id (seguí el patrón de los otros *_test.go del
// paquete store; si hay un helper compartido usalo en su lugar).
func newUser(t *testing.T, q *store.Queries) uuid.UUID {
	t.Helper()
	u, err := q.CreateUser(context.Background(), store.CreateUserParams{
		Email: "u-" + uuid.NewString() + "@t.com", PasswordHash: "x", Name: "U",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u.ID
}

func TestThreadCrudOwnership(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	owner := newUser(t, q)
	stranger := newUser(t, q)

	th, err := q.CreateThread(ctx, store.CreateThreadParams{UserID: owner, Title: "Finanzas"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	// GetThread ajeno -> ErrNoRows
	if _, err := q.GetThread(ctx, store.GetThreadParams{ID: th.ID, UserID: stranger}); err == nil {
		t.Fatal("GetThread ajeno debería fallar")
	}
	// GetThread propio -> ok
	if _, err := q.GetThread(ctx, store.GetThreadParams{ID: th.ID, UserID: owner}); err != nil {
		t.Fatalf("GetThread propio: %v", err)
	}

	// Rename ajeno -> ErrNoRows; propio -> ok
	if _, err := q.RenameThread(ctx, store.RenameThreadParams{ID: th.ID, UserID: stranger, Title: "x"}); err == nil {
		t.Fatal("RenameThread ajeno debería fallar")
	}
	r, err := q.RenameThread(ctx, store.RenameThreadParams{ID: th.ID, UserID: owner, Title: "Plata"})
	if err != nil || r.Title != "Plata" {
		t.Fatalf("RenameThread: %v title=%q", err, r.Title)
	}

	// Delete ajeno -> 0 filas; propio -> 1
	n, _ := q.DeleteThread(ctx, store.DeleteThreadParams{ID: th.ID, UserID: stranger})
	if n != 0 {
		t.Fatalf("DeleteThread ajeno borró %d", n)
	}
	n, _ = q.DeleteThread(ctx, store.DeleteThreadParams{ID: th.ID, UserID: owner})
	if n != 1 {
		t.Fatalf("DeleteThread propio borró %d, want 1", n)
	}
}

func TestListThreadsPreviewAndOrder(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)

	t1, _ := q.CreateThread(ctx, store.CreateThreadParams{UserID: u, Title: "A"})
	t2, _ := q.CreateThread(ctx, store.CreateThreadParams{UserID: u, Title: "B"})

	// Mensaje en t1 y luego tocar t1 para que quede arriba.
	if _, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: u, ThreadID: t1.ID, Role: "user", Content: "hola t1",
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if err := q.TouchThread(ctx, t1.ID); err != nil {
		t.Fatalf("TouchThread: %v", err)
	}

	rows, err := q.ListThreads(ctx, u)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len = %d, want 2", len(rows))
	}
	// t1 fue tocado último -> primero.
	if rows[0].ID != t1.ID {
		t.Errorf("orden: rows[0]=%v want %v", rows[0].ID, t1.ID)
	}
	if rows[0].Preview != "hola t1" {
		t.Errorf("preview = %q want 'hola t1'", rows[0].Preview)
	}
	if rows[1].ID != t2.ID || rows[1].Preview != "" {
		t.Errorf("rows[1] = %+v", rows[1])
	}
}

func TestDeleteThreadCascadesMessages(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)
	th, _ := q.CreateThread(ctx, store.CreateThreadParams{UserID: u, Title: "T"})
	if _, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: u, ThreadID: th.ID, Role: "user", Content: "m",
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if _, err := q.DeleteThread(ctx, store.DeleteThreadParams{ID: th.ID, UserID: u}); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	msgs, _ := q.ListThreadMessages(ctx, th.ID)
	if len(msgs) != 0 {
		t.Fatalf("quedaron %d mensajes tras borrar el hilo (cascada falló)", len(msgs))
	}
}
