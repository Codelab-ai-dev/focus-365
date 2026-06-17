# Búsqueda en el chat — Plan de implementación (Rebanada 21)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Buscar dentro del asistente: una barra en `/asistente` que, al escribir, reemplaza la lista de hilos por resultados — hilos cuyo título coincide y mensajes cuyo contenido coincide, insensible a acentos y mayúsculas.

**Architecture:** Búsqueda por subcadena con `unaccent(lower(...)) LIKE` (extensión `unaccent`, sin tabla ni índice nuevos). Dos queries (títulos de hilo, contenido de mensajes), un endpoint `GET /ai/search`, escape de comodines en Go. Frontend: input con debounce que renderiza dos secciones de resultados.

**Tech Stack:** Go (chi, sqlc, pgx/v5, goose), Postgres (extensión `unaccent`), React + Vite + TanStack Query/Router + Vitest.

**Contexto del repo (leer antes de empezar):**
- sqlc mapea `uuid`→`github.com/google/uuid.UUID`, `timestamptz`→`time.Time` (override en `api/sqlc.yaml`). Tras editar SQL: `cd api && sqlc generate` (sqlc en `/opt/homebrew/bin/sqlc`, config en `api/sqlc.yaml`).
- `testutil.NewDB(t)` aplica TODAS las migraciones a una DB limpia (incluida la nueva 0018, que crea la extensión `unaccent`). DB dev/test en `localhost:5544`.
- Comandos Go: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" <cmd>`. DB de tests: `TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`.
- La R20 dejó `ai_threads` (id, user_id, title, created_at, updated_at) y `ai_messages` con `thread_id NOT NULL`. El paquete `ai` tiene `ChatService`, la interfaz `messageStore` (chat.go), `pgChatStore` (chatstore.go), el fake `memStore` (chat_test.go) y los handlers (handler.go) con helpers de test `getJSON`/`patchJSON`/`deleteReq` (en `package ai_test`).
- Las migraciones van **additivas**: agregar queries y métodos a la interfaz NO rompe el build salvo en el paso intermedio en que la interfaz tiene un método que `pgChatStore`/`memStore` aún no implementan — por eso esos cambios van juntos en la Task 2.

---

## Estructura de archivos

**Backend**
- Crear `api/db/migrations/0018_unaccent.sql` — habilita la extensión.
- Crear `api/db/queries/ai_search.sql` — `SearchThreadsByTitle`, `SearchMessages`.
- Regenerar `api/internal/store/*` (sqlc).
- Crear `api/internal/store/ai_search_test.go` — tests de las queries (DB real).
- Modificar `api/internal/ai/types.go` — `ThreadHitView`, `MessageHitView`, `SearchResults`.
- Modificar `api/internal/ai/chat.go` — interfaz `messageStore` (+2 métodos), `escapeLike`, `ChatService.Search`, constantes.
- Modificar `api/internal/ai/chatstore.go` — `pgChatStore` implementa las 2 queries.
- Modificar `api/internal/ai/handler.go` — `GET /search` + handler + validación.
- Modificar `api/internal/ai/chat_test.go` — fake `memStore` implementa los 2 métodos.
- Crear `api/internal/ai/search_test.go` — `escapeLike` unit + handler integración.

**Frontend**
- Modificar `web/src/lib/ai.ts` — tipos `ThreadHit`/`MessageHit`/`SearchResults` + `searchChat`.
- Modificar `web/src/lib/ai.test.ts` — test de `searchChat`.
- Modificar `web/src/routes/asistente.index.tsx` — input de búsqueda + debounce + vista de resultados.
- Modificar `web/src/routes/asistente.index.test.tsx` — tests de la búsqueda en la lista.

---

## Task 1: Migración `unaccent` + queries de búsqueda + tests de store

**Files:**
- Create: `api/db/migrations/0018_unaccent.sql`
- Create: `api/db/queries/ai_search.sql`
- Create: `api/internal/store/ai_search_test.go`
- Regenerate: `api/internal/store/`

- [ ] **Step 1: Migración**

Crear `api/db/migrations/0018_unaccent.sql`:

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS unaccent;

-- +goose Down
-- No se elimina la extensión (puede usarla otra cosa).
```

- [ ] **Step 2: Queries**

Crear `api/db/queries/ai_search.sql` (parámetros nombrados con `@` para que sqlc genere campos limpios: `UserID`, `Term`, `Lim`):

```sql
-- name: SearchThreadsByTitle :many
SELECT t.id, t.title, t.updated_at,
       COALESCE(lm.content, '') AS preview
FROM ai_threads t
LEFT JOIN LATERAL (
    SELECT content FROM ai_messages m
    WHERE m.thread_id = t.id
    ORDER BY m.created_at DESC
    LIMIT 1
) lm ON true
WHERE t.user_id = @user_id
  AND unaccent(lower(t.title)) LIKE '%' || unaccent(lower(@term::text)) || '%'
ORDER BY t.updated_at DESC
LIMIT @lim;

-- name: SearchMessages :many
SELECT m.id, m.thread_id, m.role, m.content, m.created_at,
       t.title AS thread_title
FROM ai_messages m
JOIN ai_threads t ON t.id = m.thread_id
WHERE m.user_id = @user_id
  AND unaccent(lower(m.content)) LIKE '%' || unaccent(lower(@term::text)) || '%'
ORDER BY m.created_at DESC
LIMIT @lim;
```

- [ ] **Step 3: Regenerar sqlc**

Run: `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`
Expected: aparece `internal/store/ai_search.sql.go` con métodos `SearchThreadsByTitle(ctx, SearchThreadsByTitleParams)` y `SearchMessages(ctx, SearchMessagesParams)`. Verificá los nombres reales:
`grep -n "SearchThreadsByTitle\|SearchMessages\|Term\|Lim\b" internal/store/ai_search.sql.go`
Los structs esperados:
- `SearchThreadsByTitleParams{ UserID uuid.UUID; Term string; Lim int32 }`
- `SearchThreadsByTitleRow{ ID uuid.UUID; Title string; UpdatedAt time.Time; Preview string }`
- `SearchMessagesParams{ UserID uuid.UUID; Term string; Lim int32 }`
- `SearchMessagesRow{ ID uuid.UUID; ThreadID uuid.UUID; Role string; Content string; CreatedAt time.Time; ThreadTitle string }`

Si sqlc nombró algún campo distinto (p.ej. `Lim`→`Limit`), usá el nombre real en las tasks siguientes.

- [ ] **Step 4: Tests de store (que fallan)**

Crear `api/internal/store/ai_search_test.go`. Reusá el helper `newUser` del paquete (definido en `ai_threads_test.go`); si por scope no es visible, replicá el mismo patrón.

```go
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
```

- [ ] **Step 5: Correr y ver pasar**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ -run TestSearch -v`
Expected: los 5 PASS. (El build completo sigue verde: las queries son additivas.)

- [ ] **Step 6: Commit**

```bash
git add api/db/migrations/0018_unaccent.sql api/db/queries/ai_search.sql api/internal/store
git commit -m "feat(store): búsqueda de mensajes y títulos (unaccent + LIKE)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 2: Servicio + endpoint de búsqueda

Cambios en el paquete `ai`. La interfaz gana 2 métodos → `pgChatStore` y el fake `memStore` deben implementarlos en esta misma task (si no, no compila).

**Files:**
- Modify: `api/internal/ai/types.go`
- Modify: `api/internal/ai/chat.go`
- Modify: `api/internal/ai/chatstore.go`
- Modify: `api/internal/ai/handler.go`
- Modify: `api/internal/ai/chat_test.go` (fake memStore)
- Create: `api/internal/ai/search_test.go`

- [ ] **Step 1: Vistas en types.go**

Agregar a `api/internal/ai/types.go`:

```go
// ThreadHitView es un hilo cuyo título coincidió en la búsqueda.
type ThreadHitView struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Preview   string    `json:"preview"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MessageHitView es un mensaje cuyo contenido coincidió en la búsqueda.
type MessageHitView struct {
	ID          string    `json:"id"`
	ThreadID    string    `json:"thread_id"`
	ThreadTitle string    `json:"thread_title"`
	Role        string    `json:"role"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

// SearchResults agrupa los dos tipos de coincidencia.
type SearchResults struct {
	Threads  []ThreadHitView  `json:"threads"`
	Messages []MessageHitView `json:"messages"`
}
```

- [ ] **Step 2: Interfaz, escape, constantes y Search en chat.go**

a) Agregar al final de la interfaz `messageStore` (en `chat.go`):

```go
	SearchThreadsByTitle(ctx context.Context, userID uuid.UUID, term string, limit int32) ([]store.SearchThreadsByTitleRow, error)
	SearchMessages(ctx context.Context, userID uuid.UUID, term string, limit int32) ([]store.SearchMessagesRow, error)
```

b) Agregar constantes (junto a las otras del archivo) y el helper de escape:

```go
const (
	searchThreadLimit  = 20
	searchMessageLimit = 50
	minSearchLen       = 2
)

// escapeLike escapa los comodines de LIKE para que el término se busque
// literalmente (\, %, _). Postgres usa \ como carácter de escape por defecto.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
```

c) Agregar el método `Search` al `ChatService`:

```go
// Search busca en los hilos (por título) y mensajes (por contenido) del usuario,
// insensible a acentos/mayúsculas. El término ya viene validado (≥minSearchLen).
func (s *ChatService) Search(ctx context.Context, userID uuid.UUID, query string) (*SearchResults, error) {
	term := escapeLike(query)
	threadRows, err := s.store.SearchThreadsByTitle(ctx, userID, term, searchThreadLimit)
	if err != nil {
		return nil, err
	}
	msgRows, err := s.store.SearchMessages(ctx, userID, term, searchMessageLimit)
	if err != nil {
		return nil, err
	}
	out := &SearchResults{
		Threads:  make([]ThreadHitView, 0, len(threadRows)),
		Messages: make([]MessageHitView, 0, len(msgRows)),
	}
	for _, t := range threadRows {
		out.Threads = append(out.Threads, ThreadHitView{
			ID: t.ID.String(), Title: t.Title, Preview: t.Preview, UpdatedAt: t.UpdatedAt,
		})
	}
	for _, m := range msgRows {
		out.Messages = append(out.Messages, MessageHitView{
			ID: m.ID.String(), ThreadID: m.ThreadID.String(), ThreadTitle: m.ThreadTitle,
			Role: m.Role, Content: m.Content, CreatedAt: m.CreatedAt,
		})
	}
	return out, nil
}
```

- [ ] **Step 3: `pgChatStore` en chatstore.go**

Agregar:

```go
func (s *pgChatStore) SearchThreadsByTitle(ctx context.Context, userID uuid.UUID, term string, limit int32) ([]store.SearchThreadsByTitleRow, error) {
	return s.q.SearchThreadsByTitle(ctx, store.SearchThreadsByTitleParams{UserID: userID, Term: term, Lim: limit})
}

func (s *pgChatStore) SearchMessages(ctx context.Context, userID uuid.UUID, term string, limit int32) ([]store.SearchMessagesRow, error) {
	return s.q.SearchMessages(ctx, store.SearchMessagesParams{UserID: userID, Term: term, Lim: limit})
}
```

> Si sqlc nombró el campo de límite distinto de `Lim`, ajustá aquí.

- [ ] **Step 4: Endpoint en handler.go**

a) En `Routes`, agregar junto a las rutas de threads:

```go
	r.Get("/search", handleSearch(chat))
```

b) Agregar el handler:

```go
func handleSearch(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if utf8.RuneCountInString(q) < minSearchLen {
			httpx.WriteErr(w, http.StatusBadRequest, "el término de búsqueda es demasiado corto")
			return
		}
		res, err := chat.Search(r.Context(), userID, q)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, res)
	}
}
```

(`strings` y `utf8` ya están importados en handler.go.)

- [ ] **Step 5: Fake `memStore` en chat_test.go**

Agregar al fake (búsqueda naïve case-insensitive sobre el término ya escapado; alcanza para los unit tests del paquete, el comportamiento real con acentos/comodines se cubre en store y en la integración del handler). Importá `strings` si falta:

```go
func (m *memStore) SearchThreadsByTitle(ctx context.Context, userID uuid.UUID, term string, limit int32) ([]store.SearchThreadsByTitleRow, error) {
	needle := strings.ToLower(strings.NewReplacer(`\%`, "%", `\_`, "_", `\\`, `\`).Replace(term))
	out := []store.SearchThreadsByTitleRow{}
	for _, t := range m.threads {
		if t.userID == userID && strings.Contains(strings.ToLower(t.title), needle) {
			out = append(out, store.SearchThreadsByTitleRow{ID: t.id, Title: t.title, UpdatedAt: t.updatedAt})
			if int32(len(out)) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memStore) SearchMessages(ctx context.Context, userID uuid.UUID, term string, limit int32) ([]store.SearchMessagesRow, error) {
	needle := strings.ToLower(strings.NewReplacer(`\%`, "%", `\_`, "_", `\\`, `\`).Replace(term))
	out := []store.SearchMessagesRow{}
	for _, r := range m.rows {
		if r.UserID == userID && strings.Contains(strings.ToLower(r.Content), needle) {
			out = append(out, store.SearchMessagesRow{
				ID: r.ID, ThreadID: r.ThreadID, Role: r.Role, Content: r.Content, CreatedAt: r.CreatedAt,
			})
			if int32(len(out)) >= limit {
				break
			}
		}
	}
	return out, nil
}
```

- [ ] **Step 6: Tests (escapeLike unit + handler integración) en search_test.go**

Crear `api/internal/ai/search_test.go`. El unit de `escapeLike` va en `package ai`; la integración del handler en `package ai_test` con `newEnv`. **Como un archivo Go tiene un solo `package`, poné el unit de `escapeLike` dentro de `chat_test.go` (que es `package ai`)** y dejá `search_test.go` como `package ai_test` solo con la integración. Es decir:

En `chat_test.go` agregá:

```go
func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"hola":  "hola",
		"50%":   `50\%`,
		"a_b":   `a\_b`,
		`x\y`:   `x\\y`,
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
```

Crear `search_test.go` (`package ai_test`):

```go
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
```

> Nota: `getJSON`/`patchJSON`/`postChat` ya existen en `package ai_test` (creados en la R20). El `today` param no hace falta para `/search`.

- [ ] **Step 7: Verificar build + tests del paquete**

Run:
```
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
```
Expected: todo verde.

- [ ] **Step 8: Commit**

```bash
git add api/internal/ai
git commit -m "feat(ai): endpoint GET /ai/search (hilos por título, mensajes por contenido)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: Frontend — lib `searchChat`

**Files:**
- Modify: `web/src/lib/ai.ts`
- Modify: `web/src/lib/ai.test.ts`

- [ ] **Step 1: Test (que falla)**

En `web/src/lib/ai.test.ts`, siguiendo el harness (`vi.stubGlobal("fetch", ...)`, `okJson`), agregar:

```ts
it("searchChat pega a /ai/search con el término y devuelve threads+messages", async () => {
  const fetchMock = vi.fn(() =>
    okJson({
      threads: [{ id: "t1", title: "Finanzas", preview: "hola", updated_at: "" }],
      messages: [{ id: "m1", thread_id: "t1", thread_title: "Finanzas", role: "user", content: "gasté", created_at: "" }],
    })
  );
  vi.stubGlobal("fetch", fetchMock);
  const res = await searchChat("gaste");
  expect(res.threads).toHaveLength(1);
  expect(res.messages).toHaveLength(1);
  const url = String(fetchMock.mock.calls[0][0]);
  expect(url).toContain("/api/v1/ai/search?q=gaste");
});

it("searchChat urlencodea el término", async () => {
  const fetchMock = vi.fn(() => okJson({ threads: [], messages: [] }));
  vi.stubGlobal("fetch", fetchMock);
  await searchChat("50% más");
  const url = String(fetchMock.mock.calls[0][0]);
  expect(url).toContain("q=50%25%20m%C3%A1s");
});
```

Agregá `searchChat` al import del archivo de test.

- [ ] **Step 2: Verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/ai.test.ts`
Expected: FAIL (no exportada).

- [ ] **Step 3: Implementar en `web/src/lib/ai.ts`**

```ts
export type ThreadHit = {
  id: string;
  title: string;
  preview: string;
  updated_at: string;
};

export type MessageHit = {
  id: string;
  thread_id: string;
  thread_title: string;
  role: string;
  content: string;
  created_at: string;
};

export type SearchResults = { threads: ThreadHit[]; messages: MessageHit[] };

export function searchChat(q: string): Promise<SearchResults> {
  return apiFetch<SearchResults>(`/api/v1/ai/search?q=${encodeURIComponent(q)}`);
}
```

- [ ] **Step 4: Verde**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/ai.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/ai.ts web/src/lib/ai.test.ts
git commit -m "feat(web): lib searchChat (búsqueda en el chat)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 4: Frontend — barra de búsqueda en `/asistente`

**Files:**
- Modify: `web/src/routes/asistente.index.tsx`
- Modify: `web/src/routes/asistente.index.test.tsx`

- [ ] **Step 1: Tests (que fallan)**

En `web/src/routes/asistente.index.test.tsx`, seguí el harness existente (mock de `@/lib/auth`, mock de `@/lib/ai`, router en memoria). Mockeá también `searchChat`. Agregá tests:
- Escribir ≥2 caracteres en el input de búsqueda muestra los resultados (un hilo "Finanzas" y un mensaje "gasté…"), ocultando la lista normal.
- Con el input vacío se ve la lista de hilos normal.
- Búsqueda sin resultados muestra el texto "Sin resultados".

Patrón (adaptá al harness real del archivo):

```tsx
// vi.mock("@/lib/ai", () => ({
//   getThreads: vi.fn(async () => [{ id: "t1", title: "Hilo viejo", preview: "p", updated_at: "" }]),
//   searchChat: vi.fn(async () => ({
//     threads: [{ id: "t9", title: "Finanzas", preview: "hola", updated_at: "" }],
//     messages: [{ id: "m1", thread_id: "t9", thread_title: "Finanzas", role: "user", content: "gasté 200", created_at: "" }],
//   })),
// }));
// ...
// const input = await screen.findByPlaceholderText("Buscar…");
// await userEvent.type(input, "gaste");
// expect(await screen.findByText("Finanzas")).toBeInTheDocument();
// expect(screen.getByText(/gasté 200/)).toBeInTheDocument();
```

Como hay debounce, usá `await screen.findByText(...)` (espera) o `vi.useFakeTimers()`/`waitFor`. Lo más simple: `findBy*` que reintenta hasta el timeout.

- [ ] **Step 2: Implementar el input + vista de resultados en `asistente.index.tsx`**

Reemplazar el componente por la versión con búsqueda. Mantener `relativeDate` al final.

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  getThreads, searchChat,
  type Thread, type MessageHit, type ThreadHit,
} from "@/lib/ai";
import { Input } from "@/ui/Input";
import { PageTransition } from "@/ui/PageTransition";

export const Route = createFileRoute("/asistente/")({ component: ThreadListPage });

// useDebounced devuelve el valor tras `ms` sin cambios.
function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

function ThreadListPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const [query, setQuery] = useState("");
  const debounced = useDebounced(query.trim(), 250);
  const searching = debounced.length >= 2;

  const threadsQuery = useQuery({ queryKey: ["ai-threads"], queryFn: getThreads, enabled: !!user && !searching });
  const searchQuery = useQuery({
    queryKey: ["ai-search", debounced],
    queryFn: () => searchChat(debounced),
    enabled: !!user && searching,
  });

  if (!user) return null;
  const threads = threadsQuery.data ?? [];
  const results = searchQuery.data;

  return (
    <PageTransition>
      <div className="mx-auto flex max-w-xl flex-col gap-4 p-6">
        <header className="flex items-center justify-between">
          <h1 className="font-display text-xl font-bold tracking-tight">Asistente</h1>
          <div className="flex items-center gap-3">
            <Link to="/asistente/new" aria-label="Nuevo hilo"
              className="rounded-md border-2 border-ink bg-accent px-3 py-1 font-bold shadow-brutal-sm">+ Nuevo</Link>
            <Link to="/" className="font-bold text-ink underline decoration-accent decoration-2 underline-offset-2">Volver</Link>
          </div>
        </header>

        <Input
          type="search"
          aria-label="Buscar en el chat"
          placeholder="Buscar…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />

        {searching ? (
          <SearchResultsView results={results} query={debounced} navigate={navigate} />
        ) : threads.length === 0 ? (
          <p className="text-sm text-muted">Todavía no tenés hilos. Empezá uno nuevo.</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {threads.map((t: Thread) => (
              <li key={t.id}>
                <Link to="/asistente/$threadId" params={{ threadId: t.id }}
                  className="block rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm">
                  <div className="flex items-baseline justify-between gap-2">
                    <span className="truncate font-bold">{t.title || "Sin título"}</span>
                    <span className="shrink-0 text-xs text-muted">{relativeDate(t.updated_at)}</span>
                  </div>
                  {t.preview && <p className="truncate text-sm text-muted">{t.preview}</p>}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </PageTransition>
  );
}

function SearchResultsView({
  results, query, navigate,
}: {
  results: { threads: ThreadHit[]; messages: MessageHit[] } | undefined;
  query: string;
  navigate: ReturnType<typeof useNavigate>;
}) {
  if (!results) return <p className="text-sm text-muted">Buscando…</p>;
  if (results.threads.length === 0 && results.messages.length === 0) {
    return <p className="text-sm text-muted">Sin resultados para «{query}».</p>;
  }
  const open = (threadId: string) => navigate({ to: "/asistente/$threadId", params: { threadId } });
  return (
    <div className="flex flex-col gap-4">
      {results.threads.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="font-display text-sm font-bold uppercase tracking-wide text-muted">Hilos</h2>
          {results.threads.map((t) => (
            <button key={t.id} onClick={() => open(t.id)}
              className="block w-full rounded-lg border-2 border-ink bg-surface px-3 py-2 text-left shadow-brutal-sm">
              <div className="flex items-baseline justify-between gap-2">
                <span className="truncate font-bold">{t.title || "Sin título"}</span>
                <span className="shrink-0 text-xs text-muted">{relativeDate(t.updated_at)}</span>
              </div>
              {t.preview && <p className="truncate text-sm text-muted">{t.preview}</p>}
            </button>
          ))}
        </section>
      )}
      {results.messages.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="font-display text-sm font-bold uppercase tracking-wide text-muted">Mensajes</h2>
          {results.messages.map((m) => (
            <button key={m.id} onClick={() => open(m.thread_id)}
              className="block w-full rounded-lg border-2 border-ink bg-surface px-3 py-2 text-left shadow-brutal-sm">
              <div className="flex items-baseline justify-between gap-2">
                <span className="truncate text-xs font-bold text-muted">{m.thread_title || "Sin título"}</span>
                <span className="shrink-0 text-xs text-muted">{m.role === "user" ? "Vos" : "Asistente"}</span>
              </div>
              <p className="text-sm">{highlight(m.content, query)}</p>
            </button>
          ))}
        </section>
      )}
    </div>
  );
}

// highlight resalta (best-effort, case-insensitive) la primera aparición del
// término. Si no la encuentra (p.ej. por acentos), devuelve el texto tal cual.
function highlight(text: string, query: string): React.ReactNode {
  const i = text.toLowerCase().indexOf(query.toLowerCase());
  if (i < 0) return text;
  return (
    <>
      {text.slice(0, i)}
      <mark className="bg-accent">{text.slice(i, i + query.length)}</mark>
      {text.slice(i + query.length)}
    </>
  );
}

function relativeDate(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  const days = Math.floor((Date.now() - d.getTime()) / 86_400_000);
  if (days <= 0) return "hoy";
  if (days < 7) return `${days}d`;
  return d.toLocaleDateString();
}
```

> Verificá que `@/ui/Input` exista y acepte `type="search"` (lo usa el chat). Importá `React` si el `tsconfig` lo requiere para JSX con `React.ReactNode` (el resto de los archivos del repo indican el patrón; si usan `import type { ReactNode } from "react"`, seguí ese estilo y cambiá `React.ReactNode`→`ReactNode`).

- [ ] **Step 3: Verde + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build`
Expected: todo verde; build OK.

- [ ] **Step 4: Commit**

```bash
git add web/src/routes/asistente.index.tsx web/src/routes/asistente.index.test.tsx
git commit -m "feat(web): barra de búsqueda en el asistente (hilos + mensajes)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 5: Cierre — review, deploy y smoke de producción

**Files:** verificación + `scripts/smoke-r21.sh` + bitácora.

- [ ] **Step 1: Review final** contra el spec `docs/superpowers/specs/2026-06-16-plan-21-busqueda-chat-design.md`. Aplicar nits.

- [ ] **Step 2: Suites verdes**
Backend: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && TEST_DATABASE_URL=... go test -p 1 ./... -count=1`
Frontend: `cd web && npx vitest run && npm run build`

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`.

- [ ] **Step 4: Deploy + smoke.** Crear `scripts/smoke-r21.sh` (patrón de `scripts/smoke-r20.sh`: token Bearer del register): enviar un mensaje (crea hilo), renombrar el hilo a "Finanzas"; `GET /ai/search?q=<palabra del mensaje sin acento>` → 1+ mensajes; `GET /ai/search?q=finanzas` → 1 hilo; `GET /ai/search?q=a` → 400. Correr tras el deploy manual (el auto-deploy de Coolify no dispara — ver memoria/bitácoras).

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-16-sesion-plan-21-busqueda-chat.md`.

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 backend: migración unaccent → T1; queries → T1; escape comodines → T2 (`escapeLike`); servicio+endpoint+validación ≥2 → T2; límites 20/50 → T2 constantes. ✓
- §3 frontend: input+debounce → T4; dos secciones (Hilos/Mensajes) → T4 `SearchResultsView`; resaltado best-effort → T4 `highlight`; vuelve a la lista con término vacío → T4 (`searching` flag); lib `searchChat` → T3. ✓
- §4 errores: q<2 → 400 (T2 handler) y front no dispara (T4 `searching`); comodines escapados (T2); cero resultados (T4); scope user (T1 queries/WHERE). ✓
- §5 testing: store (T1), escapeLike+handler (T2), front (T4), E2E (T5). ✓
- §6 aceptación: smoke (T5). ✓

**Placeholders:** los «verificá el nombre generado por sqlc / el estilo de import de React» son adaptaciones deterministas a artefactos del repo, con instrucción exacta de qué mirar. Sin TODOs de diseño.

**Consistencia de tipos/firmas:** interfaz `messageStore` ↔ `pgChatStore` ↔ `memStore` comparten `SearchThreadsByTitle(ctx, userID, term string, limit int32) ([]store.SearchThreadsByTitleRow, error)` y `SearchMessages(...) ([]store.SearchMessagesRow, error)`. Servicio `Search(ctx, userID, query) (*SearchResults, error)`. Vistas `ThreadHitView`/`MessageHitView`/`SearchResults` ↔ frontend `ThreadHit`/`MessageHit`/`SearchResults`. Endpoint `GET /ai/search?q=` ↔ `searchChat`. ✓
