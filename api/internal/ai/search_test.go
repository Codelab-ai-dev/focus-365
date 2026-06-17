package ai_test

import (
	"net/http"
	"testing"
)

func TestSearchHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatOut: "ok"}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "search@b.com")

	// Crear un hilo "Finanzas" con un mensaje (el primer envío titula el hilo
	// con el texto del mensaje, así que renombramos para tener un título claro).
	_, body := postChat(t, e.h, tok, `{"message":"gasté 200 en libros"}`)
	tid, _ := body["thread_id"].(string)
	patchJSON(t, e.h, tok, "/ai/threads/"+tid, `{"title":"Finanzas"}`)

	// Buscar por contenido (sin acento).
	rec, out := getJSON(t, e.h, tok, "/ai/search?q=gaste")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	msgs, _ := out["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}

	// Buscar por título.
	rec, out = getJSON(t, e.h, tok, "/ai/search?q=finanzas")
	threads, _ := out["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("threads = %d, want 1", len(threads))
	}
}

func TestSearchTooShortIs400(t *testing.T) {
	comp := &fakeCompleter{chatOut: "ok"}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "short@b.com")
	rec, _ := getJSON(t, e.h, tok, "/ai/search?q=a")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestSearchWildcardLiteralAndIsolation(t *testing.T) {
	comp := &fakeCompleter{chatOut: "ok"}
	e := newEnv(t, true, comp)
	_, owner := e.user(t, "owner-s@b.com")
	_, stranger := e.user(t, "stranger-s@b.com")

	postChat(t, e.h, owner, `{"message":"subió 50% el dólar"}`)
	postChat(t, e.h, owner, `{"message":"comí 50 empanadas"}`)

	// '50%' (con comodín) debe matchear solo el literal "50%".
	_, out := getJSON(t, e.h, owner, "/ai/search?q=50%25") // %25 = '%' urlencoded
	msgs, _ := out["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("'50%%' encontró %d, want 1", len(msgs))
	}

	// El extraño no ve nada del owner.
	_, out = getJSON(t, e.h, stranger, "/ai/search?q=dolar")
	msgs, _ = out["messages"].([]any)
	if len(msgs) != 0 {
		t.Fatalf("aislamiento: extraño vio %d", len(msgs))
	}
}
