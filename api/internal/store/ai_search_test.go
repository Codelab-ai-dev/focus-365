package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/google/uuid"
)

// seedThreadMsg crea un hilo con título y un mensaje de contenido dado.
func seedThreadMsg(t *testing.T, q *store.Queries, user uuid.UUID, title, role, content string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	th, err := q.CreateThread(ctx, store.CreateThreadParams{UserID: user, Title: title})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	m, err := q.CreateMessage(ctx, store.CreateMessageParams{
		UserID: user, ThreadID: th.ID, Role: role, Content: content,
	})
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	return th.ID, m.ID
}

func TestSearchMessagesAccentAndCaseInsensitive(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)
	seedThreadMsg(t, q, u, "Hilo", "user", "Este mes gasté mucho en café")

	// 'gaste' sin acento y en minúscula debe encontrar 'gasté'.
	rows, err := q.SearchMessages(ctx, store.SearchMessagesParams{UserID: u, Term: "gaste", Lim: 50})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len = %d, want 1", len(rows))
	}
	if rows[0].ThreadTitle != "Hilo" {
		t.Errorf("thread_title = %q", rows[0].ThreadTitle)
	}
	// 'CAFÉ' en mayúscula con acento también.
	rows, _ = q.SearchMessages(ctx, store.SearchMessagesParams{UserID: u, Term: "CAFÉ", Lim: 50})
	if len(rows) != 1 {
		t.Fatalf("CAFÉ: len = %d, want 1", len(rows))
	}
}

func TestSearchMessagesScopedToUser(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	owner := newUser(t, q)
	stranger := newUser(t, q)
	seedThreadMsg(t, q, owner, "H", "user", "secreto del owner")

	rows, _ := q.SearchMessages(ctx, store.SearchMessagesParams{UserID: stranger, Term: "secreto", Lim: 50})
	if len(rows) != 0 {
		t.Fatalf("el extraño vio %d mensajes ajenos", len(rows))
	}
}

func TestSearchMessagesWildcardLiteral(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)
	seedThreadMsg(t, q, u, "H", "user", "subió 50% el dólar")
	seedThreadMsg(t, q, u, "H2", "user", "comí 50 empanadas")

	// El término escapado '50\%' debe matchear solo el literal '50%'.
	rows, _ := q.SearchMessages(ctx, store.SearchMessagesParams{UserID: u, Term: `50\%`, Lim: 50})
	if len(rows) != 1 {
		t.Fatalf("'50\\%%' encontró %d, want 1 (solo el literal 50%%)", len(rows))
	}
}

func TestSearchMessagesRespectsLimit(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)
	for i := 0; i < 5; i++ {
		seedThreadMsg(t, q, u, "H", "user", "repetido foo")
	}
	rows, _ := q.SearchMessages(ctx, store.SearchMessagesParams{UserID: u, Term: "foo", Lim: 3})
	if len(rows) != 3 {
		t.Fatalf("limit: len = %d, want 3", len(rows))
	}
}

func TestSearchThreadsByTitle(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)
	seedThreadMsg(t, q, u, "Finanzas del mes", "user", "hola")
	seedThreadMsg(t, q, u, "Entrenamiento", "user", "hola")

	rows, err := q.SearchThreadsByTitle(ctx, store.SearchThreadsByTitleParams{UserID: u, Term: "finanzas", Lim: 20})
	if err != nil {
		t.Fatalf("SearchThreadsByTitle: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "Finanzas del mes" {
		t.Fatalf("rows = %+v, want 1 'Finanzas del mes'", rows)
	}
	if rows[0].Preview != "hola" {
		t.Errorf("preview = %q want 'hola'", rows[0].Preview)
	}
}
