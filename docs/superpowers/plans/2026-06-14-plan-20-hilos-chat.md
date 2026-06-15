# Hilos en el asistente — Plan de implementación (Rebanada 20)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dividir el chat plano del asistente en hilos (conversaciones independientes) que se crean, renombran y borran, con creación lazy auto-titulada y navegación lista → hilo.

**Architecture:** Nueva tabla `ai_threads` (1:N con `ai_messages` vía `thread_id NOT NULL ON DELETE CASCADE`). El backend del paquete `ai` cambia como una unidad: el `messageStore` y `ChatService` pasan a operar por hilo; un único método transaccional `CreateTurn` persiste el par (+acciones) creando el hilo si hace falta. El frontend gana rutas `/asistente` (lista), `/asistente/$threadId` y `/asistente/new`.

**Tech Stack:** Go (chi, sqlc, pgx/v5, goose), Postgres, React + Vite + TanStack Query/Router + Vitest.

**Contexto del repo (leer antes de empezar):**
- sqlc mapea `uuid`→`github.com/google/uuid.UUID` y `timestamptz`→`time.Time` (override en `api/sqlc.yaml`). Tras editar SQL hay que regenerar con sqlc.
- Las migraciones se aplican en los tests vía `testutil.NewDB(t)` (aplica todas las migraciones a una DB limpia) y en producción al arrancar el server.
- La DB de desarrollo/test corre en `localhost:5544` (`postgres://focus:changeme@localhost:5544/focus365?sslmode=disable`).
- Comandos Go en este repo: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" <cmd>`.
- **Patrón de ejecución por capas:** el build del paquete `ai` queda roto a propósito entre la Task 1 y la Task 2 (la Task 1 cambia las firmas generadas por sqlc y el paquete `ai` todavía no se adapta). La Task 1 verifica solo `./internal/store/`. El build completo y los tests de `ai` se exigen al final de la Task 2. El frontend queda roto entre Task 3 y Task 4; la suite web se exige verde al final de la Task 4.

---

## Estructura de archivos

**Backend**
- Crear `api/db/migrations/0017_ai_threads.sql` — tabla `ai_threads`, FK `thread_id` en `ai_messages`, backfill, índices.
- Crear `api/db/queries/ai_threads.sql` — CRUD de hilos + preview.
- Modificar `api/db/queries/ai_messages.sql` — `CreateMessage` gana `thread_id`; reemplazar `ListMessages` por `ListThreadMessages`.
- Regenerar `api/internal/store/*` con sqlc.
- Crear `api/internal/store/ai_threads_test.go` — tests de migración + CRUD + ownership a nivel store.
- Modificar `api/internal/ai/chatstore.go` — `pgChatStore`: métodos de hilo + `CreateTurn` transaccional.
- Modificar `api/internal/ai/chat.go` — interfaz `messageStore`, `ChatService` por hilo, `ErrThreadNotFound`, `ThreadView`, helper de título.
- Modificar `api/internal/ai/handler.go` — endpoints de hilo, `thread_id` en chat y en el evento `done`.
- Modificar `api/internal/ai/types.go` — `ThreadView`.
- Modificar tests de `ai`: `chat_test.go` (fake `memStore` + unit), `handler_test.go` y `chat_handler_test.go` (integración).
- `api/internal/server/server.go` — sin cambios (los constructores conservan su firma).

**Frontend**
- Modificar `web/src/lib/ai.ts` — `Thread`, `getThreads`, `getThreadMessages`, `renameThread`, `deleteThread`; `sendMessageStream(message, threadId?, onDelta)` devuelve `{ reply, threadId }`.
- Modificar `web/src/lib/ai.test.ts` — tests de las nuevas funciones.
- Crear `web/src/routes/asistente.index.tsx` — la lista de hilos en `/asistente`. (Ver nota de routing en Task 4.)
- Crear `web/src/routes/asistente.$threadId.tsx` — chat de un hilo.
- Crear `web/src/routes/asistente.new.tsx` — chat de hilo nuevo (lazy).
- Borrar/reemplazar `web/src/routes/asistente.tsx` por las anteriores.
- Tests de rutas en `web/src/routes/`.

---

## Task 1: Migración 0017 + queries + regeneración sqlc + tests de store

**Files:**
- Create: `api/db/migrations/0017_ai_threads.sql`
- Create: `api/db/queries/ai_threads.sql`
- Modify: `api/db/queries/ai_messages.sql`
- Create: `api/internal/store/ai_threads_test.go`
- Regenerate: `api/internal/store/` (sqlc)

- [ ] **Step 1: Escribir la migración**

Crear `api/db/migrations/0017_ai_threads.sql`:

```sql
-- +goose Up
CREATE TABLE ai_threads (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_threads_user_updated ON ai_threads (user_id, updated_at DESC);

-- Un hilo "General" por cada usuario que ya tenga mensajes.
INSERT INTO ai_threads (user_id, title, created_at, updated_at)
SELECT user_id, 'General', MIN(created_at), MAX(created_at)
FROM ai_messages
GROUP BY user_id;

ALTER TABLE ai_messages ADD COLUMN thread_id UUID REFERENCES ai_threads(id) ON DELETE CASCADE;
UPDATE ai_messages m
SET thread_id = t.id
FROM ai_threads t
WHERE t.user_id = m.user_id;
ALTER TABLE ai_messages ALTER COLUMN thread_id SET NOT NULL;

DROP INDEX IF EXISTS idx_ai_messages_user_created;
CREATE INDEX idx_ai_messages_thread_created ON ai_messages (thread_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_ai_messages_thread_created;
CREATE INDEX idx_ai_messages_user_created ON ai_messages (user_id, created_at);
ALTER TABLE ai_messages DROP COLUMN thread_id;
DROP TABLE ai_threads;
```

- [ ] **Step 2: Escribir las queries de hilos**

Crear `api/db/queries/ai_threads.sql`:

```sql
-- name: CreateThread :one
INSERT INTO ai_threads (user_id, title)
VALUES ($1, $2)
RETURNING *;

-- name: GetThread :one
SELECT * FROM ai_threads
WHERE id = $1 AND user_id = $2;

-- name: ListThreads :many
SELECT t.id, t.user_id, t.title, t.created_at, t.updated_at,
       COALESCE(lm.content, '') AS preview
FROM ai_threads t
LEFT JOIN LATERAL (
    SELECT content FROM ai_messages m
    WHERE m.thread_id = t.id
    ORDER BY m.created_at DESC
    LIMIT 1
) lm ON true
WHERE t.user_id = $1
ORDER BY t.updated_at DESC;

-- name: RenameThread :one
UPDATE ai_threads SET title = $3
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteThread :execrows
DELETE FROM ai_threads
WHERE id = $1 AND user_id = $2;

-- name: TouchThread :exec
UPDATE ai_threads SET updated_at = now()
WHERE id = $1;
```

- [ ] **Step 3: Ajustar las queries de mensajes**

Reemplazar el contenido de `api/db/queries/ai_messages.sql` por:

```sql
-- name: ListThreadMessages :many
SELECT * FROM ai_messages
WHERE thread_id = $1
ORDER BY created_at ASC;

-- name: CreateMessage :one
INSERT INTO ai_messages (user_id, thread_id, role, content)
VALUES ($1, $2, $3, $4)
RETURNING *;
```

- [ ] **Step 4: Regenerar sqlc**

Run: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" sqlc generate` (si `sqlc` no está en PATH, usar el binario del proyecto; ver cómo se generó `store` antes — el comando vive en el toolchain del repo).
Expected: se actualizan `api/internal/store/models.go` (struct `AiThread`, campo `ThreadID uuid.UUID` en `AiMessage`), `api/internal/store/ai_threads.sql.go` (nuevo) y `api/internal/store/ai_messages.sql.go` (`CreateMessageParams` gana `ThreadID`, aparece `ListThreadMessages`, desaparece `ListMessages`). Verificá con:
`grep -n "ThreadID\|ListThreadMessages\|type AiThread\|ListThreadsRow" internal/store/*.go`

> Nota: el nombre exacto del struct de `ListThreads` lo decide sqlc (típicamente `ListThreadsRow`). Usá el nombre real generado en los pasos siguientes y en la Task 2.

- [ ] **Step 5: Escribir el test de migración + CRUD (que falla)**

Crear `api/internal/store/ai_threads_test.go`. Mirá un test existente del paquete (`goals_dimension_test.go`) para el patrón de `testutil.NewDB` y la creación de un usuario. Patrón de usuario: insertá una fila en `users` o usá el helper que ya exista; revisá cómo `goals_test.go`/`commitments_test.go` crean el `user_id`.

```go
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
```

> Si `CreateUserParams` tiene otros campos (revisá `internal/store/users.sql.go`), ajustá `newUser`. Si ya existe un helper para crear usuarios en los tests de store, reusalo en vez de `newUser`.

- [ ] **Step 6: Correr los tests y verlos fallar, luego pasar**

Run: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ -run 'TestThread|TestListThreads|TestDeleteThread' -v`
Expected: primero fallan a compilar si los nombres generados difieren — ajustá a los nombres reales. Luego PASS los 3.

- [ ] **Step 7: Commit**

```bash
git add api/db/migrations/0017_ai_threads.sql api/db/queries api/internal/store
git commit -m "feat(store): tabla ai_threads + thread_id en mensajes (migración 0017)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

> Tras esta task, `go build ./...` está roto (el paquete `ai` usa `ListMessages`/`CreatePair` viejos). Se arregla en la Task 2.

---

## Task 2: Backend del paquete `ai` por hilo

Esta task cambia el paquete `ai` como una unidad (no compila a medias). Implementá todo y recién al final verificá build + tests del paquete.

**Files:**
- Modify: `api/internal/ai/types.go` (agregar `ThreadView`)
- Modify: `api/internal/ai/chat.go` (interfaz, servicio, errores, helper de título)
- Modify: `api/internal/ai/chatstore.go` (pgChatStore)
- Modify: `api/internal/ai/handler.go` (endpoints + thread_id)
- Modify: `api/internal/ai/chat_test.go` (fake memStore + unit tests)
- Modify: `api/internal/ai/handler_test.go`, `api/internal/ai/chat_handler_test.go` (integración)

- [ ] **Step 1: `ThreadView` en types.go**

Agregar a `api/internal/ai/types.go`:

```go
// ThreadView es la vista de un hilo en la lista (se serializa a JSON).
type ThreadView struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Preview   string    `json:"preview"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

- [ ] **Step 2: Interfaz `messageStore`, errores y helper en chat.go**

En `api/internal/ai/chat.go`:

a) Agregar el error de hilo junto a `ErrUnavailable`:

```go
// ErrThreadNotFound indica que el hilo no existe o no es del usuario.
// El handler lo traduce a 404.
var ErrThreadNotFound = errors.New("hilo no encontrado")
```

b) Reemplazar la interfaz `messageStore` por:

```go
type messageStore interface {
	// Hilos
	ListThreads(ctx context.Context, userID uuid.UUID) ([]store.ListThreadsRow, error)
	GetThread(ctx context.Context, threadID, userID uuid.UUID) (store.AiThread, error)
	RenameThread(ctx context.Context, threadID, userID uuid.UUID, title string) (store.AiThread, error)
	DeleteThread(ctx context.Context, threadID, userID uuid.UUID) (int64, error)
	ListThreadMessages(ctx context.Context, threadID uuid.UUID) ([]store.AiMessage, error)

	// CreateTurn persiste el par usuario+asistente (y las acciones propuestas)
	// en una transacción. Si threadID es nil, crea primero el hilo con `title`.
	// Devuelve el id del hilo resuelto y la fila del asistente.
	CreateTurn(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID, title, userText, assistantText string, actions []ProposedAction) (uuid.UUID, store.AiMessage, []store.AiAction, error)

	// Acciones (sin cambios)
	ListActionsByMessages(ctx context.Context, messageIDs []uuid.UUID) ([]store.AiAction, error)
	GetAction(ctx context.Context, id, userID uuid.UUID) (store.AiAction, error)
	SetActionStatusFrom(ctx context.Context, id, userID uuid.UUID, to string, result []byte, from string) (store.AiAction, error)
}
```

> Usá el nombre real del struct de `ListThreads` que generó sqlc (si no es `store.ListThreadsRow`, ajustá aquí y en todos lados).

c) Agregar el helper de título (debajo de las constantes):

```go
// maxThreadTitle es el largo máximo del título autogenerado/renombrado (runes).
const maxThreadTitle = 60

// deriveTitle arma el título de un hilo nuevo a partir del primer mensaje:
// recorta espacios y limita a maxThreadTitle runes. Si queda vacío, "Nuevo hilo".
func deriveTitle(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return "Nuevo hilo"
	}
	r := []rune(t)
	if len(r) > maxThreadTitle {
		return string(r[:maxThreadTitle])
	}
	return t
}
```

- [ ] **Step 3: Reescribir los métodos del servicio en chat.go**

Reemplazar `History`, `Send` y `SendStream` por las variantes por-hilo y agregar los métodos de hilo. (Las acciones `ConfirmAction`/`CancelAction`/`UndoAction` y `toMessageView`/`toActionView`/`mapMessages`/`buildHistory` quedan igual.)

```go
// Threads devuelve los hilos del usuario para la lista (ordenados por actividad).
func (s *ChatService) Threads(ctx context.Context, userID uuid.UUID) ([]ThreadView, error) {
	rows, err := s.store.ListThreads(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ThreadView, 0, len(rows))
	for _, r := range rows {
		out = append(out, ThreadView{
			ID: r.ID.String(), Title: r.Title, Preview: r.Preview, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

// RenameThread cambia el título (validando dueño y no-vacío). 404 si no es del usuario.
func (s *ChatService) RenameThread(ctx context.Context, userID, threadID uuid.UUID, title string) (*ThreadView, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, ErrActionInvalid // se traduce a 400; reusamos el error de validación
	}
	if r := []rune(title); len(r) > maxThreadTitle {
		title = string(r[:maxThreadTitle])
	}
	row, err := s.store.RenameThread(ctx, threadID, userID, title)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrThreadNotFound
		}
		return nil, err
	}
	v := ThreadView{ID: row.ID.String(), Title: row.Title, UpdatedAt: row.UpdatedAt}
	return &v, nil
}

// DeleteThread borra el hilo (cascada de mensajes y acciones). 404 si no es del usuario.
func (s *ChatService) DeleteThread(ctx context.Context, userID, threadID uuid.UUID) error {
	n, err := s.store.DeleteThread(ctx, threadID, userID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrThreadNotFound
	}
	return nil
}

// HistoryByThread devuelve los mensajes de un hilo (con acciones colgadas).
// Valida que el hilo sea del usuario (404 si no).
func (s *ChatService) HistoryByThread(ctx context.Context, userID, threadID uuid.UUID) ([]Message, error) {
	if _, err := s.store.GetThread(ctx, threadID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrThreadNotFound
		}
		return nil, err
	}
	rows, err := s.store.ListThreadMessages(ctx, threadID)
	if err != nil {
		return nil, err
	}
	msgs := mapMessages(rows)
	if len(rows) == 0 {
		return msgs, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	acts, err := s.store.ListActionsByMessages(ctx, ids)
	if err != nil {
		return nil, err
	}
	byMsg := make(map[string][]ActionView)
	for _, a := range acts {
		k := uuid.UUID(a.MessageID.Bytes).String()
		byMsg[k] = append(byMsg[k], toActionView(a))
	}
	for i := range msgs {
		msgs[i].Actions = byMsg[msgs[i].ID]
	}
	return msgs, nil
}

// resolveThread valida el hilo destino: si threadID no es nil, confirma dueño
// (404 si no) y devuelve la cola de mensajes del hilo. Si es nil, devuelve cola
// vacía (hilo nuevo, se crea al persistir).
func (s *ChatService) resolveThread(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID) ([]store.AiMessage, error) {
	if threadID == nil {
		return nil, nil
	}
	if _, err := s.store.GetThread(ctx, *threadID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrThreadNotFound
		}
		return nil, err
	}
	return s.store.ListThreadMessages(ctx, *threadID)
}

// Send procesa una pregunta en un hilo (o crea uno nuevo si threadID es nil).
// Devuelve la respuesta y el id del hilo resuelto.
func (s *ChatService) Send(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID, text string, today time.Time) (*Message, uuid.UUID, error) {
	if !s.hasKey {
		return nil, uuid.Nil, ErrUnavailable
	}
	contextJSON, err := s.ctxb.build(ctx, userID, today)
	if err != nil {
		return nil, uuid.Nil, err
	}
	rows, err := s.resolveThread(ctx, userID, threadID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	history := buildHistory(rows, text)

	reply, err := s.groq.Chat(ctx, buildChatSystemPrompt(contextJSON), history)
	if err != nil {
		return nil, uuid.Nil, ErrUnavailable
	}
	tid, assistant, _, err := s.store.CreateTurn(ctx, userID, threadID, deriveTitle(text), text, reply, nil)
	if err != nil {
		return nil, uuid.Nil, err
	}
	v := toMessageView(assistant)
	return &v, tid, nil
}

// SendStream es la variante streaming de Send.
func (s *ChatService) SendStream(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID, text string, today time.Time, onDelta func(string)) (*Message, uuid.UUID, error) {
	if !s.hasKey {
		return nil, uuid.Nil, ErrUnavailable
	}
	contextJSON, err := s.ctxb.build(ctx, userID, today)
	if err != nil {
		return nil, uuid.Nil, err
	}
	rows, err := s.resolveThread(ctx, userID, threadID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	history := buildHistory(rows, text)

	reply, toolCalls, err := s.streamer.ChatStream(ctx, buildChatSystemPrompt(contextJSON), history, buildChatTools(), onDelta)
	if err != nil {
		return nil, uuid.Nil, ErrUnavailable
	}

	var proposed []ProposedAction
	content := reply
	if len(toolCalls) > 0 {
		if len(toolCalls) > maxActionsPerTurn {
			return nil, uuid.Nil, ErrUnavailable
		}
		proposed = make([]ProposedAction, 0, len(toolCalls))
		summaries := make([]string, 0, len(toolCalls))
		for _, tc := range toolCalls {
			kind, ok := toolNameToKind[tc.Name]
			if !ok {
				return nil, uuid.Nil, ErrUnavailable
			}
			payload, perr := parseActionPayload(kind, tc.Arguments)
			if perr != nil {
				return nil, uuid.Nil, ErrUnavailable
			}
			proposed = append(proposed, ProposedAction{Kind: kind, Payload: payload})
			summaries = append(summaries, actionSummary(kind, payload))
		}
		content = strings.TrimSpace(reply)
		if content == "" {
			content = strings.Join(summaries, " ")
		}
	}

	tid, assistant, actions, cerr := s.store.CreateTurn(ctx, userID, threadID, deriveTitle(text), text, content, proposed)
	if cerr != nil {
		return nil, uuid.Nil, cerr
	}
	v := toMessageView(assistant)
	for _, a := range actions {
		v.Actions = append(v.Actions, toActionView(a))
	}
	return &v, tid, nil
}
```

> Nota: reusamos `ErrActionInvalid` (ya se traduce a 400 en el handler) para el título vacío, evitando un error nuevo. Verificá que `ErrActionInvalid` exista en `actions.go`; si no, definí `var ErrThreadInvalidTitle = errors.New("el título no puede estar vacío")` y traducilo a 400 en el handler.

- [ ] **Step 4: `pgChatStore` en chatstore.go**

Reemplazar los métodos viejos (`ListMessages`, `CreatePair`, `CreatePairWithActions`) por los nuevos (mantener `ListActionsByMessages`, `GetAction`, `SetActionStatusFrom`, `CreateUploadActions`, `ListPendingUploadActions` tal cual):

```go
func (s *pgChatStore) ListThreads(ctx context.Context, userID uuid.UUID) ([]store.ListThreadsRow, error) {
	return s.q.ListThreads(ctx, userID)
}

func (s *pgChatStore) GetThread(ctx context.Context, threadID, userID uuid.UUID) (store.AiThread, error) {
	return s.q.GetThread(ctx, store.GetThreadParams{ID: threadID, UserID: userID})
}

func (s *pgChatStore) RenameThread(ctx context.Context, threadID, userID uuid.UUID, title string) (store.AiThread, error) {
	return s.q.RenameThread(ctx, store.RenameThreadParams{ID: threadID, UserID: userID, Title: title})
}

func (s *pgChatStore) DeleteThread(ctx context.Context, threadID, userID uuid.UUID) (int64, error) {
	return s.q.DeleteThread(ctx, store.DeleteThreadParams{ID: threadID, UserID: userID})
}

func (s *pgChatStore) ListThreadMessages(ctx context.Context, threadID uuid.UUID) ([]store.AiMessage, error) {
	return s.q.ListThreadMessages(ctx, threadID)
}

// CreateTurn crea (si hace falta) el hilo y persiste el par + acciones en una
// transacción. Devuelve el id del hilo resuelto y la fila del asistente.
func (s *pgChatStore) CreateTurn(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID, title, userText, assistantText string, actions []ProposedAction) (uuid.UUID, store.AiMessage, []store.AiAction, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, store.AiMessage{}, nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	var tid uuid.UUID
	if threadID == nil {
		th, terr := qtx.CreateThread(ctx, store.CreateThreadParams{UserID: userID, Title: title})
		if terr != nil {
			return uuid.Nil, store.AiMessage{}, nil, terr
		}
		tid = th.ID
	} else {
		tid = *threadID
	}

	if _, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, ThreadID: tid, Role: "user", Content: userText,
	}); err != nil {
		return uuid.Nil, store.AiMessage{}, nil, err
	}
	assistant, err := qtx.CreateMessage(ctx, store.CreateMessageParams{
		UserID: userID, ThreadID: tid, Role: "assistant", Content: assistantText,
	})
	if err != nil {
		return uuid.Nil, store.AiMessage{}, nil, err
	}
	rows := make([]store.AiAction, 0, len(actions))
	for i, a := range actions {
		row, aerr := qtx.CreateAction(ctx, store.CreateActionParams{
			MessageID: pgtype.UUID{Bytes: assistant.ID, Valid: true},
			UserID:    userID, Position: int32(i),
			Kind:      a.Kind, Payload: a.Payload, Status: "proposed",
		})
		if aerr != nil {
			return uuid.Nil, store.AiMessage{}, nil, aerr
		}
		rows = append(rows, row)
	}
	// Si el hilo ya existía, refrescamos su actividad para reordenar la lista.
	// (Si es nuevo, su updated_at ya es now() por el default.)
	if threadID != nil {
		if err := qtx.TouchThread(ctx, tid); err != nil {
			return uuid.Nil, store.AiMessage{}, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, store.AiMessage{}, nil, err
	}
	return tid, assistant, rows, nil
}
```

- [ ] **Step 5: Handlers en handler.go**

a) En `Routes`, reemplazar `r.Get("/messages", ...)` por los endpoints de hilo:

```go
func Routes(svc *Service, chat *ChatService, imp *ImportService) http.Handler {
	r := chi.NewRouter()
	r.Get("/insight", handleInsight(svc))
	r.Get("/threads", handleListThreads(chat))
	r.Get("/threads/{id}/messages", handleThreadMessages(chat))
	r.Patch("/threads/{id}", handleRenameThread(chat))
	r.Delete("/threads/{id}", handleDeleteThread(chat))
	r.Post("/chat", handleChat(chat))
	r.Post("/chat/stream", handleChatStream(chat))
	r.Post("/actions/{id}/confirm", handleActionConfirm(chat))
	r.Post("/actions/{id}/cancel", handleActionCancel(chat))
	r.Post("/actions/{id}/undo", handleActionUndo(chat))
	r.Post("/import", handleImport(imp))
	r.Get("/import/pending", handleImportPending(imp))
	return r
}
```

b) Reemplazar `handleMessages` y agregar los handlers de hilo. Borrar el tipo `messagesResponse` viejo si ya no se usa y reemplazarlo:

```go
type threadsResponse struct {
	Threads []ThreadView `json:"threads"`
}

func handleListThreads(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		threads, err := chat.Threads(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, threadsResponse{Threads: threads})
	}
}

type messagesResponse struct {
	Messages []Message `json:"messages"`
}

func handleThreadMessages(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		threadID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "hilo no encontrado")
			return
		}
		msgs, err := chat.HistoryByThread(r.Context(), userID, threadID)
		if err != nil {
			if errors.Is(err, ErrThreadNotFound) {
				httpx.WriteErr(w, http.StatusNotFound, "hilo no encontrado")
				return
			}
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, messagesResponse{Messages: msgs})
	}
}

type renameThreadBody struct {
	Title string `json:"title" validate:"required"`
}

type threadResponse struct {
	Thread ThreadView `json:"thread"`
}

func handleRenameThread(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		threadID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "hilo no encontrado")
			return
		}
		var body renameThreadBody
		if !httpx.DecodeAndValidate(w, r, &body) {
			return
		}
		view, err := chat.RenameThread(r.Context(), userID, threadID, body.Title)
		if err != nil {
			switch {
			case errors.Is(err, ErrThreadNotFound):
				httpx.WriteErr(w, http.StatusNotFound, "hilo no encontrado")
			case errors.Is(err, ErrActionInvalid):
				httpx.WriteErr(w, http.StatusBadRequest, "el título no puede estar vacío")
			default:
				httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			}
			return
		}
		httpx.WriteJSON(w, http.StatusOK, threadResponse{Thread: *view})
	}
}

func handleDeleteThread(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		threadID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "hilo no encontrado")
			return
		}
		if err := chat.DeleteThread(r.Context(), userID, threadID); err != nil {
			if errors.Is(err, ErrThreadNotFound) {
				httpx.WriteErr(w, http.StatusNotFound, "hilo no encontrado")
				return
			}
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
```

c) `thread_id` en el chat. Cambiar `chatRequestBody` y `decodeChatMessage` para devolver el hilo opcional:

```go
type chatRequestBody struct {
	Message  string `json:"message" validate:"required"`
	ThreadID string `json:"thread_id"`
}

// decodeChatMessage valida auth, decode, no-vacío y máximo de runes, y parsea el
// thread_id opcional (vacío -> nil; presente e inválido -> 400).
func decodeChatMessage(w http.ResponseWriter, r *http.Request) (uuid.UUID, *uuid.UUID, string, bool) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
		return uuid.Nil, nil, "", false
	}
	var req chatRequestBody
	if !httpx.DecodeAndValidate(w, r, &req) {
		return uuid.Nil, nil, "", false
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		httpx.WriteErr(w, http.StatusBadRequest, "Falta el mensaje")
		return uuid.Nil, nil, "", false
	}
	if utf8.RuneCountInString(req.Message) > maxChatChars {
		httpx.WriteErr(w, http.StatusBadRequest, "El mensaje es demasiado largo")
		return uuid.Nil, nil, "", false
	}
	var threadID *uuid.UUID
	if req.ThreadID != "" {
		id, err := uuid.Parse(req.ThreadID)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "thread_id inválido")
			return uuid.Nil, nil, "", false
		}
		threadID = &id
	}
	return userID, threadID, req.Message, true
}
```

d) Actualizar `handleChat` y `handleChatStream` a la nueva firma y agregar `thread_id` al `done`. Cambiar `doneEvent` y `chatReplyResponse`:

```go
type doneEvent struct {
	Reply    Message `json:"reply"`
	ThreadID string  `json:"thread_id"`
}

type chatReplyResponse struct {
	Reply    Message `json:"reply"`
	ThreadID string  `json:"thread_id"`
}
```

`handleChat`:

```go
func handleChat(chat *ChatService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, threadID, msg, ok := decodeChatMessage(w, r)
		if !ok {
			return
		}
		reply, tid, err := chat.Send(r.Context(), userID, threadID, msg, parseTodayParam(r))
		if err != nil {
			switch {
			case errors.Is(err, ErrThreadNotFound):
				httpx.WriteErr(w, http.StatusNotFound, "hilo no encontrado")
			case errors.Is(err, ErrUnavailable):
				httpx.WriteErr(w, http.StatusServiceUnavailable, "asistente no disponible por ahora")
			default:
				httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			}
			return
		}
		httpx.WriteJSON(w, http.StatusOK, chatReplyResponse{Reply: *reply, ThreadID: tid.String()})
	}
}
```

`handleChatStream` (cambia la llamada y el `done`; el resto del manejo SSE queda igual). El `chat.SendStream` ahora devuelve `(reply, tid, err)`; si `err` es `ErrThreadNotFound` antes de empezar el stream, responder 404:

```go
		reply, tid, err := chat.SendStream(r.Context(), userID, threadID, msg, parseTodayParam(r), func(delta string) {
			if !started {
				startSSE()
			}
			writeSSEEvent(w, flusher, "delta", deltaEvent{Text: delta})
		})
		if err != nil {
			if !started {
				switch {
				case errors.Is(err, ErrThreadNotFound):
					httpx.WriteErr(w, http.StatusNotFound, "hilo no encontrado")
				case errors.Is(err, ErrUnavailable):
					httpx.WriteErr(w, http.StatusServiceUnavailable, "asistente no disponible por ahora")
				default:
					httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
				}
				return
			}
			msgTxt := "error interno"
			if errors.Is(err, ErrUnavailable) {
				msgTxt = "asistente no disponible por ahora"
			}
			writeSSEEvent(w, flusher, "error", errorEvent{Error: msgTxt})
			return
		}
		if !started {
			startSSE()
		}
		writeSSEEvent(w, flusher, "done", doneEvent{Reply: *reply, ThreadID: tid.String()})
```

- [ ] **Step 6: Reescribir el fake `memStore` y los unit tests en chat_test.go**

El fake `memStore` debe implementar la nueva interfaz. Reemplazá el struct y sus métodos viejos (`ListMessages`, `CreatePair`, `CreatePairWithActions`) por:

```go
type memThread struct {
	id        uuid.UUID
	userID    uuid.UUID
	title     string
	updatedAt time.Time
}

type memStore struct {
	threads []memThread
	rows    []store.AiMessage
	actions []store.AiAction
	seq     int
}

func (m *memStore) ListThreads(ctx context.Context, userID uuid.UUID) ([]store.ListThreadsRow, error) {
	out := []store.ListThreadsRow{}
	for _, t := range m.threads {
		if t.userID != userID {
			continue
		}
		preview := ""
		for _, r := range m.rows {
			if r.ThreadID == t.id {
				preview = r.Content // el último en orden de inserción
			}
		}
		out = append(out, store.ListThreadsRow{
			ID: t.id, UserID: t.userID, Title: t.title, UpdatedAt: t.updatedAt, Preview: preview,
		})
	}
	// orden por updatedAt desc
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (m *memStore) GetThread(ctx context.Context, threadID, userID uuid.UUID) (store.AiThread, error) {
	for _, t := range m.threads {
		if t.id == threadID && t.userID == userID {
			return store.AiThread{ID: t.id, UserID: t.userID, Title: t.title, UpdatedAt: t.updatedAt}, nil
		}
	}
	return store.AiThread{}, pgx.ErrNoRows
}

func (m *memStore) RenameThread(ctx context.Context, threadID, userID uuid.UUID, title string) (store.AiThread, error) {
	for i := range m.threads {
		if m.threads[i].id == threadID && m.threads[i].userID == userID {
			m.threads[i].title = title
			return store.AiThread{ID: threadID, UserID: userID, Title: title, UpdatedAt: m.threads[i].updatedAt}, nil
		}
	}
	return store.AiThread{}, pgx.ErrNoRows
}

func (m *memStore) DeleteThread(ctx context.Context, threadID, userID uuid.UUID) (int64, error) {
	kept := m.threads[:0]
	var n int64
	for _, t := range m.threads {
		if t.id == threadID && t.userID == userID {
			n++
			continue
		}
		kept = append(kept, t)
	}
	m.threads = kept
	if n > 0 {
		// cascada
		rows := m.rows[:0]
		for _, r := range m.rows {
			if r.ThreadID != threadID {
				rows = append(rows, r)
			}
		}
		m.rows = rows
	}
	return n, nil
}

func (m *memStore) ListThreadMessages(ctx context.Context, threadID uuid.UUID) ([]store.AiMessage, error) {
	out := []store.AiMessage{}
	for _, r := range m.rows {
		if r.ThreadID == threadID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memStore) nextTime() time.Time {
	m.seq++
	return time.Now().Add(time.Duration(m.seq) * time.Millisecond)
}

func (m *memStore) CreateTurn(ctx context.Context, userID uuid.UUID, threadID *uuid.UUID, title, userText, assistantText string, actions []ProposedAction) (uuid.UUID, store.AiMessage, []store.AiAction, error) {
	var tid uuid.UUID
	if threadID == nil {
		tid = uuid.New()
		m.threads = append(m.threads, memThread{id: tid, userID: userID, title: title, updatedAt: m.nextTime()})
	} else {
		tid = *threadID
		for i := range m.threads {
			if m.threads[i].id == tid {
				m.threads[i].updatedAt = m.nextTime()
			}
		}
	}
	user := store.AiMessage{ID: uuid.New(), UserID: userID, ThreadID: tid, Role: "user", Content: userText, CreatedAt: m.nextTime()}
	m.rows = append(m.rows, user)
	assistant := store.AiMessage{ID: uuid.New(), UserID: userID, ThreadID: tid, Role: "assistant", Content: assistantText, CreatedAt: m.nextTime()}
	m.rows = append(m.rows, assistant)
	out := make([]store.AiAction, 0, len(actions))
	for i, a := range actions {
		row := store.AiAction{
			ID: uuid.New(), UserID: userID,
			MessageID: pgtype.UUID{Bytes: assistant.ID, Valid: true},
			Position:  int32(i), Kind: a.Kind, Payload: a.Payload, Status: "proposed",
		}
		m.actions = append(m.actions, row)
		out = append(out, row)
	}
	return tid, assistant, out, nil
}
```

Agregá los imports que falten (`sort`). El resto de métodos del fake (`ListActionsByMessages`, `GetAction`, `SetActionStatusFrom`) quedan igual.

Luego **actualizá los unit tests existentes** de `chat_test.go` a las nuevas firmas (esto es mecánico y determinista):
- `svc.Send(ctx, uid, text, today)` → `svc.Send(ctx, uid, nil, text, today)` y recibí `(msg, tid, err)`.
- `svc.SendStream(...)` análogo (agregar `nil` como threadID, recibir `tid`).
- Donde un test contaba `st.rows`, sigue válido. Donde llamaba `svc.History(ctx, uid)`, cambialo a crear/usar un hilo: usá el `tid` devuelto por el primer `Send` y luego `svc.HistoryByThread(ctx, uid, tid)`.

Agregá estos **tests nuevos** para el comportamiento de hilos:

```go
func TestSendCreatesThreadLazilyWithTitle(t *testing.T) {
	groq := &fakeChatGroq{out: "ok"}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: `{}`}, st, groq, groq, nil, true)
	uid := uuid.New()

	_, tid, err := svc.Send(context.Background(), uid, nil, "¿cuánto gasté este mes?", time.Now())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if tid == uuid.Nil {
		t.Fatal("no devolvió thread id")
	}
	threads, _ := svc.Threads(context.Background(), uid)
	if len(threads) != 1 || threads[0].Title != "¿cuánto gasté este mes?" {
		t.Fatalf("threads = %+v", threads)
	}
}

func TestSendToExistingThreadKeepsIt(t *testing.T) {
	groq := &fakeChatGroq{out: "ok"}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: `{}`}, st, groq, groq, nil, true)
	uid := uuid.New()
	_, tid, _ := svc.Send(context.Background(), uid, nil, "primero", time.Now())
	_, tid2, err := svc.Send(context.Background(), uid, &tid, "segundo", time.Now())
	if err != nil || tid2 != tid {
		t.Fatalf("tid2=%v tid=%v err=%v", tid2, tid, err)
	}
	threads, _ := svc.Threads(context.Background(), uid)
	if len(threads) != 1 {
		t.Fatalf("se crearon %d hilos, want 1", len(threads))
	}
}

func TestSendToForeignThreadIs404(t *testing.T) {
	groq := &fakeChatGroq{out: "ok"}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: `{}`}, st, groq, groq, nil, true)
	owner, stranger := uuid.New(), uuid.New()
	_, tid, _ := svc.Send(context.Background(), owner, nil, "mío", time.Now())
	_, _, err := svc.Send(context.Background(), stranger, &tid, "intruso", time.Now())
	if !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("err = %v, want ErrThreadNotFound", err)
	}
}

func TestDeleteThreadRemovesMessages(t *testing.T) {
	groq := &fakeChatGroq{out: "ok"}
	st := &memStore{}
	svc := NewChatService(fakeCtx{out: `{}`}, st, groq, groq, nil, true)
	uid := uuid.New()
	_, tid, _ := svc.Send(context.Background(), uid, nil, "hola", time.Now())
	if err := svc.DeleteThread(context.Background(), uid, tid); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	if err := svc.DeleteThread(context.Background(), uid, tid); !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("segundo delete = %v, want ErrThreadNotFound", err)
	}
}
```

- [ ] **Step 7: Actualizar los tests de integración (handler_test.go, chat_handler_test.go)**

Estos tests usan DB real (`newEnv`). Cambios mecánicos + nuevos casos:
- El helper `getMessages(t, h, tok)` que pega a `GET /ai/messages` ya no aplica. Reemplazalo por helpers `listThreads(t,h,tok)` (`GET /ai/threads`) y `threadMessages(t,h,tok,id)` (`GET /ai/threads/{id}/messages`).
- `postChat`/post al stream: agregá `thread_id` opcional al body cuando el test lo necesite. Para los tests existentes que mandaban `{"message":"..."}` sin hilo, el primer envío ahora crea un hilo — leé el `thread_id` de la respuesta (`chatReplyResponse`/evento `done`) para los asserts que después listan mensajes.
- Donde un test verificaba el historial con `GET /ai/messages`, cambialo a: enviar (obtener `thread_id`) y luego `GET /ai/threads/{thread_id}/messages`.

Agregá estos **tests de handler nuevos** (en `package ai_test`, usando `newEnv`):

```go
func TestThreadsEndpointsHappyPath(t *testing.T) {
	comp := &fakeCompleter{chatOut: "respuesta"}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "threads@b.com")

	// Crear un hilo enviando un mensaje sin thread_id.
	rec, body := postChat(t, e.h, tok, `{"message":"hola mundo"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat code = %d", rec.Code)
	}
	tid, _ := body["thread_id"].(string)
	if tid == "" {
		t.Fatal("chat no devolvió thread_id")
	}

	// Listar hilos: 1, con preview y título derivado.
	rec, body = getJSON(t, e.h, tok, "/ai/threads")
	if rec.Code != http.StatusOK {
		t.Fatalf("threads code = %d", rec.Code)
	}
	threads, _ := body["threads"].([]any)
	if len(threads) != 1 {
		t.Fatalf("len threads = %d", len(threads))
	}

	// Renombrar.
	rec, _ = patchJSON(t, e.h, tok, "/ai/threads/"+tid, `{"title":"Mi hilo"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename code = %d", rec.Code)
	}

	// Mensajes del hilo.
	rec, body = getJSON(t, e.h, tok, "/ai/threads/"+tid+"/messages")
	if rec.Code != http.StatusOK {
		t.Fatalf("messages code = %d", rec.Code)
	}

	// Borrar.
	rec = deleteReq(t, e.h, tok, "/ai/threads/"+tid)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete code = %d", rec.Code)
	}
}

func TestThreadOwnership404(t *testing.T) {
	comp := &fakeCompleter{chatOut: "x"}
	e := newEnv(t, true, comp)
	_, ownerTok := e.user(t, "owner@b.com")
	_, strangerTok := e.user(t, "stranger@b.com")

	_, body := postChat(t, e.h, ownerTok, `{"message":"propio"}`)
	tid, _ := body["thread_id"].(string)

	rec, _ := getJSON(t, e.h, strangerTok, "/ai/threads/"+tid+"/messages")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ajeno messages code = %d, want 404", rec.Code)
	}
	rec = deleteReq(t, e.h, strangerTok, "/ai/threads/"+tid)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ajeno delete code = %d, want 404", rec.Code)
	}
}
```

Agregá los helpers que falten siguiendo el patrón de `postChat`/`getMessages` (mismo archivo): `getJSON` (GET con token → rec, map), `patchJSON` (PATCH con body), `deleteReq` (DELETE → rec). Ejemplo:

```go
func getJSON(t *testing.T, h http.Handler, tok, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func patchJSON(t *testing.T, h http.Handler, tok, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func deleteReq(t *testing.T, h http.Handler, tok, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
```

- [ ] **Step 8: Verificar build + tests del paquete**

Run:
```
cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./internal/ai/ ./internal/server/ -count=1
```
Expected: build limpio; tests verde. Iterá hasta verde (los nombres generados por sqlc o campos de structs pueden requerir ajustes puntuales).

- [ ] **Step 9: Verificar la suite completa del backend**

Run: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1`
Expected: todos los paquetes verde.

- [ ] **Step 10: Commit**

```bash
git add api/internal/ai
git commit -m "feat(ai): chat por hilos (servicio, store y endpoints)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 3: Frontend — lib `ai.ts` por hilo

**Files:**
- Modify: `web/src/lib/ai.ts`
- Modify: `web/src/lib/ai.test.ts`

- [ ] **Step 1: Tests de la lib (que fallan)**

En `web/src/lib/ai.test.ts`, seguí el patrón existente (`vi.stubGlobal("fetch", ...)`, `okJson`). Agregá:

```ts
it("getThreads pide la lista y devuelve threads", async () => {
  const fetchMock = vi.fn(() =>
    okJson({ threads: [{ id: "t1", title: "A", preview: "hola", updated_at: "2026-06-14T00:00:00Z" }] })
  );
  vi.stubGlobal("fetch", fetchMock);
  const threads = await getThreads();
  expect(threads).toHaveLength(1);
  expect(threads[0].title).toBe("A");
  expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/ai/threads");
});

it("getThreadMessages pega al endpoint del hilo", async () => {
  const fetchMock = vi.fn(() => okJson({ messages: [] }));
  vi.stubGlobal("fetch", fetchMock);
  await getThreadMessages("t1");
  expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/ai/threads/t1/messages");
});

it("renameThread hace PATCH con el título", async () => {
  const fetchMock = vi.fn(() => okJson({ thread: { id: "t1", title: "Nuevo", preview: "", updated_at: "" } }));
  vi.stubGlobal("fetch", fetchMock);
  const th = await renameThread("t1", "Nuevo");
  expect(th.title).toBe("Nuevo");
  const opts = fetchMock.mock.calls[0][1] as RequestInit;
  expect(opts.method).toBe("PATCH");
  expect(String(opts.body)).toContain("Nuevo");
});

it("deleteThread hace DELETE", async () => {
  const fetchMock = vi.fn(() => new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetchMock);
  await deleteThread("t1");
  const opts = fetchMock.mock.calls[0][1] as RequestInit;
  expect(opts.method).toBe("DELETE");
  expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/ai/threads/t1");
});
```

Agregá los imports al tope del archivo de test: `getThreads, getThreadMessages, renameThread, deleteThread`.

- [ ] **Step 2: Verlos fallar**

Run: `cd web && npx vitest run src/lib/ai.test.ts`
Expected: FAIL (funciones no exportadas).

- [ ] **Step 3: Implementar en `web/src/lib/ai.ts`**

Agregar el tipo y las funciones; modificar `sendMessageStream`. Reemplazá `getMessages`/`sendMessage` (ya no se usan tras la Task 4) por las versiones por hilo:

```ts
export type Thread = {
  id: string;
  title: string;
  preview: string;
  updated_at: string;
};

export function getThreads(): Promise<Thread[]> {
  return apiFetch<{ threads: Thread[] }>("/api/v1/ai/threads").then((r) => r.threads);
}

export function getThreadMessages(threadId: string): Promise<Message[]> {
  return apiFetch<{ messages: Message[] }>(
    `/api/v1/ai/threads/${threadId}/messages`
  ).then((r) => r.messages);
}

export function renameThread(id: string, title: string): Promise<Thread> {
  return apiFetch<{ thread: Thread }>(`/api/v1/ai/threads/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ title }),
  }).then((r) => r.thread);
}

export function deleteThread(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/ai/threads/${id}`, { method: "DELETE" });
}
```

> Verificá que `apiFetch` tolere respuestas 204 sin body (mirá su implementación en `web/src/lib/api.ts`). Si no, usá un `fetch` directo como en `importFile` para el DELETE, devolviendo `void` cuando `res.ok`.

Modificar `sendMessageStream` para aceptar `threadId?` y devolver `{ reply, threadId }`:

```ts
export async function sendMessageStream(
  message: string,
  threadId: string | undefined,
  onDelta: (text: string) => void
): Promise<{ reply: Message; threadId: string }> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const token = getAccessToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch("/api/v1/ai/chat/stream", {
    method: "POST",
    headers,
    body: JSON.stringify(threadId ? { message, thread_id: threadId } : { message }),
    credentials: "include",
  });

  if (!res.ok) {
    let msg = `Error ${res.status}`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* respuesta sin JSON */
    }
    throw new ApiError(msg, res.status);
  }
  if (!res.body) throw new ApiError("streaming no soportado", 500);

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let reply: Message | null = null;
  let doneThreadId = "";

  const handleEvent = (raw: string) => {
    let event = "";
    let data = "";
    for (const line of raw.split("\n")) {
      if (line.startsWith("event: ")) event = line.slice(7).trim();
      else if (line.startsWith("data: ")) data += line.slice(6);
    }
    if (!event || !data) return;
    if (event === "delta") {
      onDelta((JSON.parse(data) as { text: string }).text);
    } else if (event === "done") {
      const d = JSON.parse(data) as { reply: Message; thread_id: string };
      reply = d.reply;
      doneThreadId = d.thread_id;
    } else if (event === "error") {
      throw new ApiError((JSON.parse(data) as { error: string }).error, 503);
    }
  };

  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let sep: number;
      while ((sep = buffer.indexOf("\n\n")) !== -1) {
        const raw = buffer.slice(0, sep);
        buffer = buffer.slice(sep + 2);
        handleEvent(raw);
      }
    }
  } finally {
    await reader.cancel().catch(() => {});
  }

  if (!reply) throw new ApiError("la respuesta se cortó, intenta de nuevo", 502);
  return { reply, threadId: doneThreadId };
}
```

Eliminá `getMessages` y `sendMessage` (las reemplaza lo de arriba; confirmá que nada más las importe con `grep -rn "getMessages\|sendMessage\b" web/src`).

- [ ] **Step 4: Verde**

Run: `cd web && npx vitest run src/lib/ai.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/ai.ts web/src/lib/ai.test.ts
git commit -m "feat(web): lib del chat por hilos (getThreads/getThreadMessages/rename/delete)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

> Tras esta task, `asistente.tsx` no compila (usa `getMessages`/firma vieja de `sendMessageStream`). Se reemplaza en la Task 4.

---

## Task 4: Frontend — rutas de hilos (lista, hilo, nuevo)

**Files:**
- Delete: `web/src/routes/asistente.tsx`
- Create: `web/src/routes/asistente.index.tsx` (lista en `/asistente`)
- Create: `web/src/routes/asistente.$threadId.tsx` (chat del hilo)
- Create: `web/src/routes/asistente.new.tsx` (chat de hilo nuevo)
- Modify/Create: tests de rutas en `web/src/routes/`

> **Routing (TanStack):** revisá cómo está configurado el router (¿`@tanstack/router-plugin` con generación de árbol a partir de archivos, o rutas declaradas a mano?). Mirá `web/src/routeTree.gen.ts` y `web/src/main.tsx`/`router.tsx`. Si es file-based, los nombres `asistente.index.tsx`, `asistente.$threadId.tsx`, `asistente.new.tsx` generan `/asistente`, `/asistente/$threadId` y `/asistente/new` automáticamente (regenerá el árbol). Si las rutas se declaran a mano, declaralas igual y seguí el patrón de una ruta existente con parámetro (buscá `useParams` en el repo). Adaptá los nombres/registro a lo que use el proyecto; lo importante son las 3 vistas y sus paths.

- [ ] **Step 1: Test de la lista (que falla)**

Mirá `web/src/routes/asistente.test.tsx` (actual) para el harness (`renderWithProviders`, mocks de `@/lib/ai`, router de prueba). Creá `web/src/routes/asistente.index.test.tsx` que monte la lista y verifique que renderiza los hilos devueltos por `getThreads` (mockeado) con su título y preview, y que el botón nuevo enlaza a `/asistente/new`.

```tsx
// patrón: mockear "@/lib/ai" -> getThreads devuelve 2 hilos; render; assert títulos visibles.
```

(Escribí el test concreto siguiendo el harness real del archivo existente: `vi.mock("@/lib/ai", ...)`, `renderWithProviders(<ruta/>)`, `await screen.findByText("Finanzas")`.)

- [ ] **Step 2: Implementar `asistente.index.tsx` (lista)**

Lista de hilos con el design system neo-brutalista (mirá `asistente.tsx` viejo para tokens: `border-2 border-ink`, `shadow-brutal-sm`, `font-display`). Estructura:

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getThreads, type Thread } from "@/lib/ai";
import { PageTransition } from "@/ui/PageTransition";

export const Route = createFileRoute("/asistente/")({ component: ThreadListPage });

function ThreadListPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const threadsQuery = useQuery({ queryKey: ["ai-threads"], queryFn: getThreads, enabled: !!user });
  if (!user) return null;
  const threads = threadsQuery.data ?? [];

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

        {threads.length === 0 ? (
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

// relativeDate: "hoy"/"Nd"/fecha. Si ya existe un helper de fechas relativas en
// el repo (buscá en web/src/lib), reusalo en lugar de redefinirlo.
function relativeDate(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  const days = Math.floor((Date.now() - d.getTime()) / 86_400_000);
  if (days <= 0) return "hoy";
  if (days < 7) return `${days}d`;
  return d.toLocaleDateString();
}
```

- [ ] **Step 3: Implementar `asistente.$threadId.tsx` (chat del hilo)**

Tomá como base el `asistente.tsx` viejo (cuerpo del chat: historial, streaming, ActionCard, form). Cambios:
- `useParams` para `threadId`.
- `historyQuery` usa `["ai-thread", threadId, "messages"]` con `getThreadMessages(threadId)`.
- `sendMessageStream(t, threadId, onDelta)`; en `onSuccess` actualizá el caché de ese hilo.
- Header con el título del hilo + botones renombrar (✏️) y borrar (🗑️). Renombrar: `renameThread(threadId, nuevoTitulo)` (prompt simple o input inline) e invalidar `["ai-threads"]`. Borrar: `deleteThread(threadId)`, invalidar `["ai-threads"]`, `navigate({ to: "/asistente" })`.
- Si `getThreadMessages` da 404 (hilo borrado/ajeno), redirigí a `/asistente`.

```tsx
import { createFileRoute, useNavigate, useParams, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  getThreadMessages, sendMessageStream, renameThread, deleteThread,
  confirmAction, cancelAction, undoAction, type Message,
} from "@/lib/ai";
import { Button } from "@/ui/Button";
import { Input } from "@/ui/Input";
import { PageTransition } from "@/ui/PageTransition";
import { ActionCard } from "@/ui/ActionCard";

export const Route = createFileRoute("/asistente/$threadId")({ component: ThreadChatPage });

function ThreadChatPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { threadId } = useParams({ from: "/asistente/$threadId" });

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const historyQuery = useQuery({
    queryKey: ["ai-thread", threadId, "messages"],
    queryFn: () => getThreadMessages(threadId),
    enabled: !!user,
    retry: false,
  });
  useEffect(() => {
    // hilo inexistente/ajeno -> volver a la lista
    if (historyQuery.isError) navigate({ to: "/asistente" });
  }, [historyQuery.isError, navigate]);

  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [streaming, setStreaming] = useState<{ question: string; partial: string } | null>(null);

  const actionMutation = useMutation({
    mutationFn: ({ id, verb }: { id: string; verb: "confirm" | "cancel" | "undo" }) =>
      verb === "confirm" ? confirmAction(id) : verb === "cancel" ? cancelAction(id) : undoAction(id),
    onSuccess: (updated) => {
      setError(null);
      qc.setQueryData<Message[]>(["ai-thread", threadId, "messages"], (prev) =>
        (prev ?? []).map((m) =>
          m.actions?.some((a) => a.id === updated.id)
            ? { ...m, actions: m.actions.map((a) => (a.id === updated.id ? updated : a)) }
            : m
        )
      );
    },
    onError: (err) => setError(err instanceof Error ? err.message : "No se pudo resolver la acción"),
  });

  const mutation = useMutation({
    mutationFn: (message: string) => {
      setStreaming({ question: message, partial: "" });
      return sendMessageStream(message, threadId, (delta) =>
        setStreaming((s) => (s ? { ...s, partial: s.partial + delta } : s))
      );
    },
    onSuccess: ({ reply }, message) => {
      setError(null);
      setText("");
      qc.setQueryData<Message[]>(["ai-thread", threadId, "messages"], (prev) => [
        ...(prev ?? []),
        { id: "", role: "user", content: message, created_at: reply.created_at },
        reply,
      ]);
      qc.invalidateQueries({ queryKey: ["ai-threads"] });
      setStreaming(null);
    },
    onError: (err) => {
      setStreaming(null);
      setError(err instanceof Error ? err.message : "No se pudo enviar");
    },
  });

  const renameMut = useMutation({
    mutationFn: (title: string) => renameThread(threadId, title),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ai-threads"] }),
  });
  const deleteMut = useMutation({
    mutationFn: () => deleteThread(threadId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["ai-threads"] });
      navigate({ to: "/asistente" });
    },
  });

  if (!user) return null;
  const messages = historyQuery.data ?? [];

  return (
    <PageTransition>
      <div className="mx-auto flex max-w-xl flex-col gap-4 p-6">
        <header className="flex items-center justify-between gap-2">
          <Link to="/asistente" className="font-bold underline decoration-accent decoration-2 underline-offset-2">← Hilos</Link>
          <div className="flex items-center gap-2">
            <button aria-label="Renombrar hilo" onClick={() => {
              const t = window.prompt("Nuevo título");
              if (t && t.trim()) renameMut.mutate(t.trim());
            }} className="rounded-md border-2 border-ink px-2 py-1 shadow-brutal-sm">✏️</button>
            <button aria-label="Borrar hilo" onClick={() => {
              if (window.confirm("¿Borrar este hilo?")) deleteMut.mutate();
            }} className="rounded-md border-2 border-ink px-2 py-1 shadow-brutal-sm">🗑️</button>
          </div>
        </header>

        {/* …reusar el bloque de mensajes/streaming/form del asistente.tsx viejo,
            apuntando setQueryData a ["ai-thread", threadId, "messages"]… */}
      </div>
    </PageTransition>
  );
}
```

Completá el bloque de mensajes/streaming/form copiándolo del `asistente.tsx` viejo (sección `<section>` con el `.map` de mensajes, el bloque `streaming`, el `error` y el `<form>`), sin cambios salvo las queryKeys ya indicadas.

- [ ] **Step 4: Implementar `asistente.new.tsx` (hilo nuevo lazy)**

Igual que el chat del hilo pero sin `threadId`: el historial arranca vacío, no hay header de renombrar/borrar, y al enviar el primer mensaje se llama `sendMessageStream(message, undefined, onDelta)`; en `onSuccess` se lee `threadId` y se navega con replace:

```tsx
export const Route = createFileRoute("/asistente/new")({ component: NewThreadPage });

function NewThreadPage() {
  // …igual estructura, historyQuery NO existe (mensajes locales en estado),
  // o simplemente mostrá solo el streaming y, al done, navegá:
  const mutation = useMutation({
    mutationFn: (message: string) => {
      setStreaming({ question: message, partial: "" });
      return sendMessageStream(message, undefined, (delta) =>
        setStreaming((s) => (s ? { ...s, partial: s.partial + delta } : s))
      );
    },
    onSuccess: ({ threadId }) => {
      qc.invalidateQueries({ queryKey: ["ai-threads"] });
      navigate({ to: "/asistente/$threadId", params: { threadId }, replace: true });
    },
    onError: (err) => {
      setStreaming(null);
      setError(err instanceof Error ? err.message : "No se pudo enviar");
    },
  });
  // …header con "← Hilos", el form, y el bloque streaming (pregunta + parcial).
}
```

- [ ] **Step 5: Borrar `asistente.tsx` y regenerar el árbol de rutas**

```bash
rm web/src/routes/asistente.tsx
```
Si el router es file-based, regenerá `routeTree.gen.ts` (corré el dev/build una vez o el comando de generación del plugin). Si es manual, actualizá el registro de rutas.

- [ ] **Step 6: Tests de rutas**

Adaptá/creá tests para las tres vistas siguiendo el harness de `asistente.test.tsx` (mock de `@/lib/ai`, `renderWithProviders`). Como mínimo:
- Lista: renderiza hilos mockeados (título + preview); estado vacío.
- Hilo: renderiza mensajes mockeados de `getThreadMessages`; enviar agrega la burbuja.
- Nuevo: al `done` (mock de `sendMessageStream` que resuelve `{reply, threadId:"t9"}`) navega a `/asistente/t9` (verificá con un router de prueba o un spy de `navigate`).

Borrá `asistente.test.tsx` viejo si quedó obsoleto.

- [ ] **Step 7: Suite web completa + build**

Run: `cd web && npx vitest run && npm run build`
Expected: todo verde; build OK. Arreglá imports/typing pendientes (p.ej. cualquier referencia residual a `getMessages`).

- [ ] **Step 8: Commit**

```bash
git add web/src/routes web/src/routeTree.gen.ts
git commit -m "feat(web): rutas de hilos del asistente (lista, hilo, nuevo)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Task 5: Cierre — review final, deploy y smoke de producción

**Files:** ninguno de código (verificación + scripts/docs)

- [ ] **Step 1: Review final**

Pedir la review holística (spec-compliance + correctness + calidad) del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-14-plan-20-hilos-chat-design.md`. Aplicar nits.

- [ ] **Step 2: Suites verdes (gate de finishing-a-development-branch)**

Backend: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && TEST_DATABASE_URL=... go test -p 1 ./... -count=1`
Frontend: `cd web && npx vitest run && npm run build`

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push**

Seguir la skill `finishing-a-development-branch` (opción merge local). Mensaje de merge describiendo la rebanada 20.

- [ ] **Step 4: Deploy + smoke**

Tras el deploy (Coolify), correr un smoke `scripts/smoke-r20.sh` que: registra un usuario; envía un mensaje sin `thread_id` (espera `thread_id` en la respuesta); `GET /ai/threads` (espera 1 hilo con preview); envía un 2.º mensaje con ese `thread_id` (sigue 1 hilo); `PATCH` el título (200); `GET /ai/threads/{id}/messages` (espera 4 mensajes); `DELETE` (204); `GET` del hilo borrado → 404. Patrón de auth: token Bearer del body de `register` (ver `scripts/smoke-r19.sh`).

- [ ] **Step 5: Bitácora**

Escribir `docs/superpowers/sesiones/2026-06-14-sesion-plan-20-hilos-chat.md` (qué se entregó, arquitectura, commits, decisiones/hallazgos, verificación, backlog restante: búsqueda R21 + auto-deploy + backups + OCR + recordatorios).

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 modelo de datos → Task 1 (migración 0017, backfill, índices). ✓
- §3 backend (queries, servicio por hilo, lazy create, endpoints, contexto global intacto) → Task 1 (queries) + Task 2 (servicio, store, handlers). El `chatcontext` no se toca. ✓
- §4 frontend (lista, hilo, nuevo, lib) → Task 3 (lib) + Task 4 (rutas). ✓
- §5 errores (404 ownership, lazy atómico, título vacío 400) → Task 2 (resolveThread/CreateTurn/RenameThread + handlers). ✓
- §6 testing → tests en Tasks 1–4; E2E en Task 5. ✓
- §7 aceptación → smoke en Task 5. ✓

**Placeholders:** los puntos marcados con «seguí el patrón/ajustá al nombre generado» son adaptaciones deterministas a artefactos del repo (nombres sqlc, harness de tests, file-based routing), no decisiones abiertas; cada uno indica exactamente qué inspeccionar. Sin TODOs de diseño.

**Consistencia de tipos/firmas:** `messageStore` (chat.go) ↔ `pgChatStore` (chatstore.go) ↔ `memStore` (chat_test.go) comparten exactamente: `ListThreads→[]store.ListThreadsRow`, `GetThread(threadID,userID)→store.AiThread`, `RenameThread(...,title)→store.AiThread`, `DeleteThread→int64`, `ListThreadMessages(threadID)→[]store.AiMessage`, `CreateTurn(...,threadID *uuid.UUID, title, userText, assistantText, []ProposedAction)→(uuid.UUID, store.AiMessage, []store.AiAction, error)`. Servicio: `Send`/`SendStream(...,threadID *uuid.UUID,...)→(*Message, uuid.UUID, error)`; `HistoryByThread`, `Threads`, `RenameThread`, `DeleteThread`. Frontend: `sendMessageStream(message, threadId?, onDelta)→{reply, threadId}`; `getThreads`/`getThreadMessages`/`renameThread`/`deleteThread`. ✓
