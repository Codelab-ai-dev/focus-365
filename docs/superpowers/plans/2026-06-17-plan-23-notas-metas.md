# Notas de avance por meta — Plan de implementación (Rebanada 23)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cada meta gana una bitácora de notas fechadas (agregar/borrar) que se ve y se edita en un modal desde la card de la meta.

**Architecture:** Nueva tabla `goal_notes` (1:N con `goals`, cascada). Endpoints anidados `GET/POST /goals/{id}/notes` y `DELETE /goals/{id}/notes/{noteId}` en el paquete `goals`; el create inserta solo si la meta es del usuario (`WHERE EXISTS`). Frontend: `lib/goalNotes.ts` + un `GoalNotesModal` (reusa `ui/Modal.tsx`) abierto desde un botón 📝 en cada card. Cambios additivos: el build se mantiene verde entre tasks.

**Tech Stack:** Go (chi, sqlc, pgx/v5, goose), Postgres, React + Vite + TanStack Query/Router + Vitest.

**Contexto del repo (leer antes de empezar):**
- sqlc: `uuid`→`uuid.UUID`, `timestamptz`/`date`→`time.Time` (override en `api/sqlc.yaml`). Tras editar SQL: `cd api && sqlc generate` (sqlc en `/opt/homebrew/bin/sqlc`).
- `testutil.NewDB(t)` aplica todas las migraciones (incl. la nueva 0019). DB dev/test en `localhost:5544`. Comandos Go: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" <cmd>`, `TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`.
- `goals.Service` (`api/internal/goals/service.go`) tiene `q *store.Queries`. `goals/types.go` define `const dateLayout = "2006-01-02"`. Las rutas (`Routes`) ya están bajo `RequireAuth`. La vista `Goal` serializa `deadline` como `*time.Time`.
- `CreateGoalParams{UserID, Title, Dimension, Deadline *time.Time}`; la columna `goals.dimension` tiene CHECK de las 4D (espiritual/emocional/fisica/financiera) desde la R19 — en tests usar `"fisica"`.
- Helper de tests del store `newUser(t, q)` existe en `api/internal/store/ai_threads_test.go` (mismo `package store_test`).
- El front tiene `ui/Modal.tsx` (`Modal({open, onClose, title, children})`) y `todayString()` en `lib/goals.ts`.
- Última migración: `0018_unaccent.sql` → la nueva es `0019`.

---

## Estructura de archivos

**Backend**
- Crear `api/db/migrations/0019_goal_notes.sql` — tabla + índice.
- Crear `api/db/queries/goal_notes.sql` — Create/List/Delete.
- Regenerar `api/internal/store/*` (sqlc).
- Crear `api/internal/store/goal_notes_test.go` — tests de queries.
- Modificar `api/internal/goals/types.go` — vista `Note` + `ErrGoalNotFound`.
- Modificar `api/internal/goals/service.go` — `Notes`/`AddNote`/`DeleteNote` + `buildNote`.
- Modificar `api/internal/goals/handler.go` — rutas + handlers de notas.
- Modificar `api/internal/goals/handler_test.go` — tests de los endpoints.

**Frontend**
- Crear `web/src/lib/goalNotes.ts` — tipo + funciones.
- Crear `web/src/lib/goalNotes.test.ts` — tests de la lib.
- Modificar `web/src/routes/metas.tsx` — botón 📝 + `GoalNotesModal`.
- Modificar `web/src/routes/metas.test.tsx` — tests del modal de notas.

---

## Task 1: Migración 0019 + queries + tests de store

**Files:**
- Create: `api/db/migrations/0019_goal_notes.sql`
- Create: `api/db/queries/goal_notes.sql`
- Create: `api/internal/store/goal_notes_test.go`
- Regenerate: `api/internal/store/`

- [ ] **Step 1: Migración**

Crear `api/db/migrations/0019_goal_notes.sql`:

```sql
-- +goose Up
CREATE TABLE goal_notes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    goal_id    UUID NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note_date  DATE NOT NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_goal_notes_goal ON goal_notes (goal_id, note_date DESC, created_at DESC);

-- +goose Down
DROP TABLE goal_notes;
```

- [ ] **Step 2: Queries**

Crear `api/db/queries/goal_notes.sql`:

```sql
-- name: CreateGoalNote :one
INSERT INTO goal_notes (goal_id, user_id, note_date, body)
SELECT @goal_id, @user_id, @note_date, @body
WHERE EXISTS (SELECT 1 FROM goals WHERE id = @goal_id AND user_id = @user_id)
RETURNING *;

-- name: ListGoalNotes :many
SELECT n.* FROM goal_notes n
JOIN goals g ON g.id = n.goal_id
WHERE n.goal_id = @goal_id AND g.user_id = @user_id
ORDER BY n.note_date DESC, n.created_at DESC;

-- name: DeleteGoalNote :execrows
DELETE FROM goal_notes WHERE id = @id AND user_id = @user_id;
```

- [ ] **Step 3: Regenerar sqlc**

Run: `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`
Expected: aparece `internal/store/goal_notes.sql.go`. Verificá nombres reales:
`grep -n "CreateGoalNote\|ListGoalNotes\|DeleteGoalNote\|type GoalNote" internal/store/*.go`
Esperado:
- modelo `GoalNote{ ID, GoalID, UserID uuid.UUID; NoteDate time.Time; Body string; CreatedAt time.Time }`
- `CreateGoalNoteParams{ GoalID, UserID uuid.UUID; NoteDate time.Time; Body string }` → `GoalNote`
- `ListGoalNotesParams{ GoalID, UserID uuid.UUID }` → `[]GoalNote` (si sqlc generó `ListGoalNotesRow`, usá ese nombre en los pasos siguientes)
- `DeleteGoalNoteParams{ ID, UserID uuid.UUID }` → `int64`

- [ ] **Step 4: Tests de store (que fallan)**

Crear `api/internal/store/goal_notes_test.go`. Reusá `newUser` del paquete.

```go
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
```

- [ ] **Step 5: Correr (fallan→pasan)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ -run TestGoalNote -v` (ajustá el `-run` a los nombres reales: `TestCreateGoalNote|TestListGoalNotes|TestDeleteGoalNote`).
Expected: 3 PASS. El build completo sigue verde (additivo).

- [ ] **Step 6: Commit**

```bash
git add api/db/migrations/0019_goal_notes.sql api/db/queries/goal_notes.sql api/internal/store
git commit -m "feat(store): tabla goal_notes + queries (migración 0019)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Servicio + endpoints de notas en el paquete `goals`

**Files:**
- Modify: `api/internal/goals/types.go`
- Modify: `api/internal/goals/service.go`
- Modify: `api/internal/goals/handler.go`
- Modify: `api/internal/goals/handler_test.go`

- [ ] **Step 1: Vista `Note` + error en types.go**

Agregar a `api/internal/goals/types.go`:

```go
import "errors"  // agregar al bloque de imports si no está

// ErrGoalNotFound: la meta no existe o no es del usuario (al colgar una nota).
var ErrGoalNotFound = errors.New("meta no encontrada")

// Note es la vista de una nota de avance. note_date va como YYYY-MM-DD.
type Note struct {
	ID        string    `json:"id"`
	GoalID    string    `json:"goal_id"`
	NoteDate  string    `json:"note_date"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
```
(El archivo ya importa `time`. Agregá `errors`.)

- [ ] **Step 2: Métodos del servicio**

Agregar a `api/internal/goals/service.go` (ya importa `context`, `errors`, `strings`, `time`, `store`, `uuid`, `pgx`):

```go
func buildNote(n store.GoalNote) Note {
	return Note{
		ID:        n.ID.String(),
		GoalID:    n.GoalID.String(),
		NoteDate:  n.NoteDate.Format(dateLayout),
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
	}
}

// Notes lista las notas de una meta del usuario (orden del store: fecha desc).
func (s *Service) Notes(ctx context.Context, userID, goalID uuid.UUID) ([]Note, error) {
	rows, err := s.q.ListGoalNotes(ctx, store.ListGoalNotesParams{GoalID: goalID, UserID: userID})
	if err != nil {
		return nil, err
	}
	out := make([]Note, 0, len(rows))
	for _, n := range rows {
		out = append(out, buildNote(n))
	}
	return out, nil
}

// AddNote cuelga una nota de la meta. ErrGoalNotFound si la meta no es del usuario.
func (s *Service) AddNote(ctx context.Context, userID, goalID uuid.UUID, noteDate time.Time, body string) (*Note, error) {
	n, err := s.q.CreateGoalNote(ctx, store.CreateGoalNoteParams{
		GoalID: goalID, UserID: userID, NoteDate: noteDate, Body: strings.TrimSpace(body),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGoalNotFound
		}
		return nil, err
	}
	v := buildNote(n)
	return &v, nil
}

// DeleteNote borra una nota del usuario. Devuelve si borró algo.
func (s *Service) DeleteNote(ctx context.Context, userID, noteID uuid.UUID) (bool, error) {
	n, err := s.q.DeleteGoalNote(ctx, store.DeleteGoalNoteParams{ID: noteID, UserID: userID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
```

> Si sqlc generó `ListGoalNotesRow` en vez de `GoalNote` para el List, cambiá la firma de `buildNote`/`Notes` a ese tipo (los campos son los mismos).

- [ ] **Step 3: Rutas + handlers**

En `api/internal/goals/handler.go`:

a) Agregar a `Routes` (después de `r.Delete("/{id}", ...)`):

```go
	r.Get("/{id}/notes", handleListNotes(svc))
	r.Post("/{id}/notes", handleCreateNote(svc))
	r.Delete("/{id}/notes/{noteId}", handleDeleteNote(svc))
```

b) Agregar `"errors"` y `"strings"` y `"unicode/utf8"` al bloque de imports si faltan (ya están `net/http`, `time`, `auth`, `httpx`, `chi`, `uuid`, `encoding/json`).

c) Agregar los handlers y tipos:

```go
const maxNoteChars = 1000

type noteReq struct {
	NoteDate string `json:"note_date" validate:"required"`
	Body     string `json:"body" validate:"required"`
}

type notesResponse struct {
	Notes []Note `json:"notes"`
}

type noteResponse struct {
	Note Note `json:"note"`
}

func handleListNotes(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		goalID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "meta no encontrada")
			return
		}
		notes, err := svc.Notes(r.Context(), userID, goalID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, notesResponse{Notes: notes})
	}
}

func handleCreateNote(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		goalID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "meta no encontrada")
			return
		}
		var req noteReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		body := strings.TrimSpace(req.Body)
		if body == "" {
			httpx.WriteErr(w, http.StatusBadRequest, "la nota no puede estar vacía")
			return
		}
		if utf8.RuneCountInString(body) > maxNoteChars {
			httpx.WriteErr(w, http.StatusBadRequest, "la nota es demasiado larga")
			return
		}
		noteDate, err := time.Parse(dateLayout, req.NoteDate)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
			return
		}
		out, err := svc.AddNote(r.Context(), userID, goalID, noteDate, body)
		if err != nil {
			if errors.Is(err, ErrGoalNotFound) {
				httpx.WriteErr(w, http.StatusNotFound, "meta no encontrada")
				return
			}
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, noteResponse{Note: *out})
	}
}

func handleDeleteNote(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		if _, err := uuid.Parse(chi.URLParam(r, "id")); err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "meta no encontrada")
			return
		}
		noteID, err := uuid.Parse(chi.URLParam(r, "noteId"))
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "nota no encontrada")
			return
		}
		deleted, err := svc.DeleteNote(r.Context(), userID, noteID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if !deleted {
			httpx.WriteErr(w, http.StatusNotFound, "nota no encontrada")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
```

- [ ] **Step 4: Tests de handler (que fallan)**

En `api/internal/goals/handler_test.go`, agregar (usan `newEnv`, `token`, `do`, `createGoal` ya existentes):

```go
func TestGoalNotesHappyPath(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "notes@b.com")
	g := createGoal(t, e, tok, map[string]any{"title": "Correr", "dimension": "fisica"})
	gid := g["id"].(string)

	// crear nota
	rec := do(t, e.h, http.MethodPost, "/goals/"+gid+"/notes", tok,
		map[string]any{"note_date": "2026-06-17", "body": "5k hoy"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST nota code = %d, body=%s", rec.Code, rec.Body.String())
	}

	// listar
	rec = do(t, e.h, http.MethodGet, "/goals/"+gid+"/notes", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET notas code = %d", rec.Code)
	}
	var body struct {
		Notes []map[string]any `json:"notes"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Notes) != 1 || body.Notes[0]["note_date"] != "2026-06-17" || body.Notes[0]["body"] != "5k hoy" {
		t.Fatalf("notas = %+v", body.Notes)
	}
	noteID := body.Notes[0]["id"].(string)

	// borrar
	rec = do(t, e.h, http.MethodDelete, "/goals/"+gid+"/notes/"+noteID, tok, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE nota code = %d", rec.Code)
	}
}

func TestGoalNotesValidation(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "notesval@b.com")
	g := createGoal(t, e, tok, map[string]any{"title": "M", "dimension": "fisica"})
	gid := g["id"].(string)

	// body vacío -> 400
	rec := do(t, e.h, http.MethodPost, "/goals/"+gid+"/notes", tok,
		map[string]any{"note_date": "2026-06-17", "body": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("body vacío code = %d, want 400", rec.Code)
	}
	// fecha inválida -> 400
	rec = do(t, e.h, http.MethodPost, "/goals/"+gid+"/notes", tok,
		map[string]any{"note_date": "17/06/2026", "body": "x"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("fecha inválida code = %d, want 400", rec.Code)
	}
}

func TestGoalNotesForeignGoal404(t *testing.T) {
	e := newEnv(t)
	owner := e.token(t, "owner-n@b.com")
	stranger := e.token(t, "stranger-n@b.com")
	g := createGoal(t, e, owner, map[string]any{"title": "M", "dimension": "fisica"})
	gid := g["id"].(string)

	// el extraño intenta colgar una nota en la meta ajena -> 404
	rec := do(t, e.h, http.MethodPost, "/goals/"+gid+"/notes", stranger,
		map[string]any{"note_date": "2026-06-17", "body": "intruso"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("nota en meta ajena code = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 5: Verificar build + tests + suite completa**

Run:
```
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
```
Expected: todo verde.

- [ ] **Step 6: Commit**

```bash
git add api/internal/goals
git commit -m "feat(goals): notas de avance por meta (endpoints anidados)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Frontend — lib `goalNotes.ts`

**Files:**
- Create: `web/src/lib/goalNotes.ts`
- Create: `web/src/lib/goalNotes.test.ts`

- [ ] **Step 1: Test (que falla)**

Crear `web/src/lib/goalNotes.test.ts` (seguí el patrón de `web/src/lib/ai.test.ts`: `vi.stubGlobal("fetch", ...)` y un helper `okJson`):

```ts
import { describe, it, expect, vi, afterEach } from "vitest";
import { listGoalNotes, createGoalNote, deleteGoalNote } from "./goalNotes";

function okJson(data: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(data), { status }));
}

describe("goalNotes", () => {
  afterEach(() => vi.restoreAllMocks());

  it("listGoalNotes pega al endpoint de la meta", async () => {
    const fetchMock = vi.fn(() =>
      okJson({ notes: [{ id: "n1", goal_id: "g1", note_date: "2026-06-17", body: "x", created_at: "" }] })
    );
    vi.stubGlobal("fetch", fetchMock);
    const notes = await listGoalNotes("g1");
    expect(notes).toHaveLength(1);
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/goals/g1/notes");
  });

  it("createGoalNote hace POST con note_date y body", async () => {
    const fetchMock = vi.fn(() =>
      okJson({ note: { id: "n1", goal_id: "g1", note_date: "2026-06-17", body: "5k", created_at: "" } }, 201)
    );
    vi.stubGlobal("fetch", fetchMock);
    const note = await createGoalNote("g1", { note_date: "2026-06-17", body: "5k" });
    expect(note.body).toBe("5k");
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("POST");
    expect(String(opts.body)).toContain("2026-06-17");
  });

  it("deleteGoalNote hace DELETE a la nota", async () => {
    const fetchMock = vi.fn(() => Promise.resolve(new Response(null, { status: 204 })));
    vi.stubGlobal("fetch", fetchMock);
    await deleteGoalNote("g1", "n1");
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("DELETE");
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/goals/g1/notes/n1");
  });
});
```

- [ ] **Step 2: Verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/goalNotes.test.ts`
Expected: FAIL (no existe `./goalNotes`).

- [ ] **Step 3: Implementar `web/src/lib/goalNotes.ts`**

```ts
import { apiFetch } from "./api";

export type GoalNote = {
  id: string;
  goal_id: string;
  note_date: string;
  body: string;
  created_at: string;
};

export function listGoalNotes(goalId: string): Promise<GoalNote[]> {
  return apiFetch<{ notes: GoalNote[] }>(`/api/v1/goals/${goalId}/notes`).then(
    (r) => r.notes
  );
}

export function createGoalNote(
  goalId: string,
  input: { note_date: string; body: string }
): Promise<GoalNote> {
  return apiFetch<{ note: GoalNote }>(`/api/v1/goals/${goalId}/notes`, {
    method: "POST",
    body: JSON.stringify(input),
  }).then((r) => r.note);
}

export function deleteGoalNote(goalId: string, noteId: string): Promise<void> {
  return apiFetch<void>(`/api/v1/goals/${goalId}/notes/${noteId}`, {
    method: "DELETE",
  });
}
```

- [ ] **Step 4: Verde**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/goalNotes.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/goalNotes.ts web/src/lib/goalNotes.test.ts
git commit -m "feat(web): lib de notas de meta (list/create/delete)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Frontend — botón 📝 + `GoalNotesModal` en `metas.tsx`

**Files:**
- Modify: `web/src/routes/metas.tsx`
- Modify: `web/src/routes/metas.test.tsx`

- [ ] **Step 1: Tests (que fallan)**

En `web/src/routes/metas.test.tsx`, mirá el harness existente (mock de `@/lib/auth`, mock o stub de fetch, router en memoria). Agregá un test que: con una meta en el listado, hay un botón "Notas"; al tocarlo se abre el modal; mockeás `listGoalNotes` para devolver una nota y se ve su body; agregar postea (`createGoalNote`) y borrar llama `deleteGoalNote`. Lo más simple es **mockear `@/lib/goalNotes`** con `vi.mock`:

```tsx
// Al tope del archivo, junto a los otros vi.mock:
// vi.mock("@/lib/goalNotes", () => ({
//   listGoalNotes: vi.fn(async () => [
//     { id: "n1", goal_id: "g1", note_date: "2026-06-17", body: "5k hoy", created_at: "" },
//   ]),
//   createGoalNote: vi.fn(async () => ({ id: "n2", goal_id: "g1", note_date: "2026-06-18", body: "nueva", created_at: "" })),
//   deleteGoalNote: vi.fn(async () => undefined),
// }));
//
// it("abre el modal de notas y lista las notas de la meta", async () => {
//   ...render con una meta cuyo id sea "g1"...
//   await userEvent.click(await screen.findByLabelText(/Notas de/));
//   expect(await screen.findByText("5k hoy")).toBeInTheDocument();
// });
```
Adaptá el render al harness real (cómo `metas.test.tsx` siembra metas — probablemente vía el fetch mock de `/goals`; hacé que devuelva una meta con `id: "g1"`). Escribí los `expect` concretos. Watch fail.

- [ ] **Step 2: Imports y estado en `metas.tsx`**

Agregar imports:

```tsx
import { Modal } from "@/ui/Modal";
import { listGoalNotes, createGoalNote, deleteGoalNote, type GoalNote } from "@/lib/goalNotes";
```
(`todayString` ya se importa desde `@/lib/goals`; `useQuery`, `useMutation`, `useQueryClient`, `useState` ya están.)

Dentro de `MetasPage`, junto a los otros `useState`:

```tsx
  const [notesGoal, setNotesGoal] = useState<Goal | null>(null);
```

- [ ] **Step 3: Botón 📝 Notas en la card**

En el bloque de acciones de cada card (el `<div className="mt-3 flex flex-wrap gap-2">`, junto a los botones existentes), agregar como primer botón:

```tsx
                      <Button
                        type="button"
                        variant="ghost"
                        aria-label={`Notas de ${g.title}`}
                        onClick={() => setNotesGoal(g)}
                        className="px-3 py-1 text-xs"
                      >
                        📝 Notas
                      </Button>
```

- [ ] **Step 4: Montar el modal**

Antes de cerrar el contenedor de la página (al final del JSX, junto al resto), agregar:

```tsx
        <GoalNotesModal goal={notesGoal} onClose={() => setNotesGoal(null)} />
```

- [ ] **Step 5: Implementar `GoalNotesModal` y `formatDay`**

Agregar al final de `metas.tsx`:

```tsx
function GoalNotesModal({
  goal,
  onClose,
}: {
  goal: Goal | null;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [body, setBody] = useState("");
  const [noteDate, setNoteDate] = useState(todayString());
  const [error, setError] = useState<string | null>(null);

  const notesQuery = useQuery({
    queryKey: ["goal-notes", goal?.id],
    queryFn: () => listGoalNotes(goal!.id),
    enabled: goal !== null,
  });

  const addMutation = useMutation({
    mutationFn: () => createGoalNote(goal!.id, { note_date: noteDate, body }),
    onSuccess: () => {
      setBody("");
      setError(null);
      qc.invalidateQueries({ queryKey: ["goal-notes", goal!.id] });
    },
    onError: (e) => setError(e instanceof Error ? e.message : "No se pudo guardar"),
  });

  const delMutation = useMutation({
    mutationFn: (noteId: string) => deleteGoalNote(goal!.id, noteId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["goal-notes", goal!.id] }),
  });

  const notes = notesQuery.data ?? [];

  return (
    <Modal open={goal !== null} onClose={onClose} title={goal ? goal.title : ""}>
      <div className="space-y-4 text-sm">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (body.trim()) addMutation.mutate();
          }}
          className="space-y-2"
        >
          <textarea
            aria-label="Nueva nota"
            placeholder="¿qué avanzaste?"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            className="w-full rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm"
            rows={3}
          />
          <div className="flex items-center gap-2">
            <input
              type="date"
              aria-label="Fecha de la nota"
              value={noteDate}
              onChange={(e) => setNoteDate(e.target.value)}
              className="rounded-lg border-2 border-ink bg-surface px-2 py-1"
            />
            <Button type="submit" disabled={addMutation.isPending || body.trim() === ""} className="px-3 py-1 text-xs">
              {addMutation.isPending ? "Guardando…" : "Agregar"}
            </Button>
          </div>
          {error && (
            <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-xs font-bold text-danger-fg shadow-brutal-sm">
              {error}
            </p>
          )}
        </form>

        {notes.length === 0 ? (
          <p className="text-muted">Sin notas todavía.</p>
        ) : (
          <ul className="space-y-2">
            {notes.map((n: GoalNote) => (
              <li key={n.id} className="rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm">
                <div className="flex items-start justify-between gap-2">
                  <span className="text-xs font-bold text-muted">{formatDay(n.note_date)}</span>
                  <button
                    type="button"
                    aria-label={`Borrar nota del ${n.note_date}`}
                    onClick={() => delMutation.mutate(n.id)}
                    className="shrink-0 text-xs font-bold text-ink"
                  >
                    🗑️
                  </button>
                </div>
                <p className="mt-1 whitespace-pre-wrap">{n.body}</p>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}

// formatDay muestra "lun 16 jun" parseando YYYY-MM-DD como fecha LOCAL.
function formatDay(iso: string): string {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString("es", {
    weekday: "short",
    day: "numeric",
    month: "short",
  });
}
```

> `noteDate` no se resetea al cerrar/cambiar de meta; como arranca en hoy y es un detalle menor, es aceptable. Si querés resetearlo al abrir otra meta, podés añadir un `useEffect` que lo reinicie con `goal?.id` — opcional, no requerido.

- [ ] **Step 6: Verde + suite + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build`
Expected: todo verde; build OK (typecheck incluido). Verificá que `@/ui/Button` acepte `type="submit"` y `disabled` (lo usa el resto de la app).

- [ ] **Step 7: Commit**

```bash
git add web/src/routes/metas.tsx web/src/routes/metas.test.tsx
git commit -m "feat(web): bitácora de notas por meta (modal)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Cierre — review, merge y smoke

**Files:** verificación + `scripts/smoke-r23.sh` + bitácora.

- [ ] **Step 1: Review final** del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-17-plan-23-notas-metas-design.md`. Aplicar nits.

- [ ] **Step 2: Suites verdes**
Backend: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && TEST_DATABASE_URL=... go test -p 1 ./... -count=1`
Frontend: `cd web && npx vitest run && npm run build`

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`.

- [ ] **Step 4: Deploy manual (Coolify) + smoke.** Crear `scripts/smoke-r23.sh` (patrón de `scripts/smoke-r20.sh`: token Bearer del register): crear una meta (`POST /goals`); agregarle 2 notas con fechas distintas (`POST /goals/{id}/notes`); `GET /goals/{id}/notes` → 2, orden por fecha desc; body vacío → 400; borrar una nota → 204; `POST` nota en una meta ajena/inexistente (uuid random) → 404. Correr tras el deploy manual.

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-17-sesion-plan-23-notas-metas.md`.

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 modelo (tabla goal_notes + índice + cascada) → Task 1. ✓
- §3 backend (CreateGoalNote con `WHERE EXISTS`, ListGoalNotes ordenado, DeleteGoalNote; servicio Notes/AddNote/DeleteNote; rutas anidadas; validación body trim/≤1000/fecha) → Task 1 (queries) + Task 2 (servicio+handler). ✓
- §3 vista `note_date` como YYYY-MM-DD → `buildNote` formatea con `dateLayout` (Task 2). ✓
- §4 frontend (lib + botón 📝 + GoalNotesModal con textarea+date default hoy + lista con fecha legible + borrar + vacío) → Task 3 (lib) + Task 4 (UI). ✓
- §5 errores (404 cruzado, 400 body vacío/largo, 400 fecha, cascada) → Task 2 handler + Task 1 cascada. ✓
- §6 testing → Tasks 1–4; E2E → Task 5. ✓
- §7 aceptación → smoke Task 5. ✓

**Placeholders:** los «ajustá al nombre generado por sqlc / al harness real de metas.test» son adaptaciones deterministas con instrucción exacta de qué inspeccionar; sin TODOs de diseño.

**Consistencia de tipos/firmas:** store `CreateGoalNoteParams{GoalID,UserID,NoteDate,Body}` / `ListGoalNotesParams{GoalID,UserID}` / `DeleteGoalNoteParams{ID,UserID}` ↔ servicio `Notes(userID,goalID)`, `AddNote(userID,goalID,noteDate,body)→*Note`, `DeleteNote(userID,noteID)→bool` ↔ vista `Note{id,goal_id,note_date,body,created_at}` ↔ frontend `GoalNote{id,goal_id,note_date,body,created_at}` ↔ endpoints `GET/POST /goals/{id}/notes`, `DELETE /goals/{id}/notes/{noteId}` ↔ lib `listGoalNotes/createGoalNote/deleteGoalNote`. `ErrGoalNotFound` (servicio) → 404 (handler). ✓
