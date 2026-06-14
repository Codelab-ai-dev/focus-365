# Plan 18 — Compromisos rastreables — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Los compromisos del check-in se vuelven rastreables: lo que escribes para mañana aparece mañana en el check-in con un check para marcar cumplido.

**Architecture:** Los compromisos salen del JSONB del check-in a una tabla propia `commitments` (target_date + done). Un paquete `commitments` nuevo (servicio + rutas: due/toggle); el guardado del check-in escribe los de mañana vía una interfaz estrecha; el contexto del chat los incluye con su cumplimiento; el formulario muestra «ayer te comprometiste a» arriba.

**Tech Stack:** Go + chi + sqlc/pgx (api), React + TanStack Query + Vitest (web).

**Spec:** `docs/superpowers/specs/2026-06-14-plan-18-compromisos-rastreables-design.md`

**Entorno:** Go desde `api/` con `GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`; `sqlc generate` desde `api/`. Frontend `cd web && npx vitest run && npm run build`. Commits en español con `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Rama: `plan-18-compromisos-rastreables` desde `main`.

**Hechos del código (verificados):**
- Rutas de módulo: `func Routes(svc *Service) http.Handler` con chi, montadas en server.go bajo `RequireAuth` (`r.Mount("/checkins", checkin.Routes(checkinSvc))`).
- `checkin` (post R17): `Input`/`CheckIn` tienen `Commitments []string`; `Upsert` los marshalea a JSONB; `toView` los unmarshalea; `handleUpsert` parsea `req.Commitments` y los pasa al `Input`. `UpsertCheckInMetrics` (parcial) no los toca.
- `NewChatContextBuilder(d snapshotter, f cycler, c checkinLister, h habitLister, g goalLister)`; `build` arma `map[string]any{"snapshot","cycles","checkins","habits","goals"}`.
- `check_ins.commitments` es columna JSONB (migración 0013).

---

### Task 1: Migración 0015 (tabla commitments + sacar del check-in)

**Files:**
- Create: `api/db/migrations/0015_commitments.sql`, `api/db/queries/commitments.sql`
- Modify: `api/db/queries/check_ins.sql` (UpsertCheckIn pierde commitments), `api/internal/checkin/service.go`, `api/internal/checkin/handler.go`
- Test: `api/internal/checkin/service_test.go` (quitar asserts de commitments), `api/internal/store/check_ins_test.go` (params), `api/internal/store/commitments_test.go` (nuevo: migración + round-trip)
- Generated: `api/internal/store/`

- [ ] **Step 1: Migración** `api/db/migrations/0015_commitments.sql`:

```sql
-- +goose Up
CREATE TABLE commitments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_date DATE NOT NULL,
    text        TEXT NOT NULL,
    done        BOOLEAN NOT NULL DEFAULT false,
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_commitments_user_target ON commitments (user_id, target_date);

-- Migra los compromisos del JSONB de cada check-in (fecha F) a la tabla con
-- target = F+1 y la posición del array.
INSERT INTO commitments (user_id, target_date, text, position)
SELECT c.user_id, c.date + INTERVAL '1 day',
       elem.value #>> '{}', elem.ordinality - 1
FROM check_ins c,
     jsonb_array_elements(c.commitments) WITH ORDINALITY elem(value, ordinality)
WHERE jsonb_array_length(c.commitments) > 0;

ALTER TABLE check_ins DROP COLUMN commitments;

-- +goose Down
ALTER TABLE check_ins ADD COLUMN commitments JSONB NOT NULL DEFAULT '[]';
DROP TABLE commitments;
```

- [ ] **Step 2: Queries** `api/db/queries/commitments.sql`:

```sql
-- name: CreateCommitment :one
INSERT INTO commitments (user_id, target_date, text, position)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteCommitmentsForDate :execrows
DELETE FROM commitments WHERE user_id = $1 AND target_date = $2;

-- name: ListCommitmentsByTarget :many
SELECT * FROM commitments
WHERE user_id = $1 AND target_date = $2
ORDER BY position;

-- name: ToggleCommitment :one
UPDATE commitments
SET done = NOT done, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: ListRecentCommitments :many
SELECT * FROM commitments
WHERE user_id = $1 AND target_date >= $2
ORDER BY target_date DESC, position;
```

En `api/db/queries/check_ins.sql`, quitar `commitments` de `UpsertCheckIn`
(columnas, VALUES, y el `DO UPDATE SET`). El resto de la query queda igual.
`UpsertCheckInMetrics` no se toca. Correr `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`. `store.CheckIn` y `UpsertCheckInParams` pierden `Commitments`; aparece `store.Commitment` + sus params.

- [ ] **Step 3: Test que falla** `api/internal/store/commitments_test.go` (verifica la migración de datos + round-trip):

```go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCommitmentRoundTrip(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{Email: "com@b.com", PasswordHash: "h", Name: "C"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	target := pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true}
	c, err := q.CreateCommitment(ctx, store.CreateCommitmentParams{
		UserID: u.ID, TargetDate: target, Text: "Tender la cama", Position: 0,
	})
	if err != nil {
		t.Fatalf("CreateCommitment: %v", err)
	}
	if c.Done {
		t.Error("nuevo commitment no debe estar done")
	}
	toggled, err := q.ToggleCommitment(ctx, store.ToggleCommitmentParams{ID: c.ID, UserID: u.ID})
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if !toggled.Done {
		t.Error("toggle debe poner done=true")
	}
	list, err := q.ListCommitmentsByTarget(ctx, store.ListCommitmentsByTargetParams{UserID: u.ID, TargetDate: target})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Text != "Tender la cama" {
		t.Errorf("list = %+v", list)
	}
}
```

(Los tipos sqlc para `DATE` con pgx pueden salir como `pgtype.Date` — ajustar
el test y el servicio a lo generado; si sale `time.Time`, usar `time.Time`.)

- [ ] **Step 4: Verificar que falla** (compila tras sqlc; el INSERT de migración se ejecuta solo). Correr `go test ./internal/store/ -run TestCommitmentRoundTrip`.

- [ ] **Step 5: Quitar commitments del check-in** en `service.go`:
  - `Input`: quitar `Commitments []string`.
  - `CheckIn`: quitar `Commitments []string json:"commitments"`.
  - `Upsert`: quitar el `json.Marshal(cleanCommitments(...))` y el campo
    `Commitments` del `UpsertCheckInParams`; quitar `cleanCommitments` (se
    mueve al paquete commitments en la Task 2 — aquí simplemente se elimina del
    checkin).
  - `toView`: quitar el bloque de `Commitments`.
  - Si `encoding/json`/`strings` quedan sin uso en service.go, quitarlos.

  En `handler.go`: `upsertReq` **conserva** `Commitments []string json:"commitments"` (el body sigue trayéndolos; se escribirán en la Task 3). En `handleUpsert`, quitar `Commitments: req.Commitments` del `Input{...}` (ya no es campo de Input). `req.Commitments` queda parseado pero sin usar aún (es un campo de struct, no rompe el build).

- [ ] **Step 6: Adaptar tests del checkin/store** que asser­taban commitments
  (`service_test.go` TestUpsertFullRoundTrip — quitar las aserciones de
  Commitments y el campo del Input; `check_ins_test.go` — quitar `Commitments`
  de los params si lo ponían).

- [ ] **Step 7: Verificar (checkin + store) + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/checkin/ ./internal/store/ -count=1
git add api/db api/internal/store api/internal/checkin
git commit -m "feat(commitments): tabla propia y migración del JSONB del check-in"
```

(El build completo queda roto si algo más usaba `CheckIn.Commitments` — el frontend sí, se arregla en la Task 5; el backend no debería usarlo fuera de checkin.)

---

### Task 2: Paquete `commitments` — servicio

**Files:**
- Create: `api/internal/commitments/service.go`, `api/internal/commitments/service_test.go`

- [ ] **Step 1: Tests que fallan** `api/internal/commitments/service_test.go` (patrón testutil como otros paquetes):

```go
package commitments_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/commitments"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/google/uuid"
)

func newSvc(t *testing.T) (*commitments.Service, uuid.UUID) {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	u, err := q.CreateUser(context.Background(), store.CreateUserParams{Email: "s@b.com", PasswordHash: "h", Name: "S"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return commitments.NewService(q, pool), u.ID
}

func TestReplaceForDateAndDueOn(t *testing.T) {
	svc, uid := newSvc(t)
	ctx := context.Background()
	d := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := svc.ReplaceForDate(ctx, uid, d, []string{"Tender la cama", "  ", "Pasear a Ruffo"}); err != nil {
		t.Fatalf("ReplaceForDate: %v", err)
	}
	due, err := svc.DueOn(ctx, uid, d)
	if err != nil {
		t.Fatalf("DueOn: %v", err)
	}
	if len(due) != 2 || due[0].Text != "Tender la cama" || due[1].Text != "Pasear a Ruffo" {
		t.Fatalf("due = %+v (esperaba 2, vacío filtrado)", due)
	}
	// Reemplazar de nuevo no duplica.
	if err := svc.ReplaceForDate(ctx, uid, d, []string{"Solo uno"}); err != nil {
		t.Fatalf("Replace 2: %v", err)
	}
	due2, _ := svc.DueOn(ctx, uid, d)
	if len(due2) != 1 || due2[0].Text != "Solo uno" {
		t.Errorf("replace duplicó: %+v", due2)
	}
}

func TestToggle(t *testing.T) {
	svc, uid := newSvc(t)
	ctx := context.Background()
	d := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	_ = svc.ReplaceForDate(ctx, uid, d, []string{"X"})
	due, _ := svc.DueOn(ctx, uid, d)
	id := due[0].ID
	c, err := svc.Toggle(ctx, uid, uuid.MustParse(id))
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if !c.Done {
		t.Error("toggle debe marcar done")
	}
	// Toggle de otro usuario → (nil, nil).
	other, _ := newSvc(t)
	c2, err := other.Toggle(ctx, uuid.New(), uuid.MustParse(id))
	if err != nil || c2 != nil {
		t.Errorf("toggle ajeno = (%v, %v), want (nil, nil)", c2, err)
	}
}

func TestRecent(t *testing.T) {
	svc, uid := newSvc(t)
	ctx := context.Background()
	_ = svc.ReplaceForDate(ctx, uid, time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC), []string{"viejo"})
	_ = svc.ReplaceForDate(ctx, uid, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), []string{"nuevo"})
	rec, err := svc.Recent(ctx, uid, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(rec) != 1 || rec[0].Text != "nuevo" {
		t.Errorf("recent = %+v (since 15 debe excluir el 14)", rec)
	}
}
```

- [ ] **Step 2: Verificar que fallan.**

- [ ] **Step 3: Implementar** `api/internal/commitments/service.go`:

```go
// Package commitments gestiona los compromisos rastreables del check-in:
// lo que el usuario se compromete a hacer un día, marcable como cumplido.
package commitments

import (
	"context"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q    *store.Queries
	pool *pgxpool.Pool
}

func NewService(q *store.Queries, pool *pgxpool.Pool) *Service {
	return &Service{q: q, pool: pool}
}

// Commitment es la vista de dominio (target_date como YYYY-MM-DD).
type Commitment struct {
	ID         string `json:"id"`
	TargetDate string `json:"target_date"`
	Text       string `json:"text"`
	Done       bool   `json:"done"`
}

const dateLayout = "2006-01-02"

func toView(c store.Commitment) Commitment {
	return Commitment{
		ID: c.ID.String(), TargetDate: c.TargetDate.Time.Format(dateLayout),
		Text: c.Text, Done: c.Done,
	}
}

func pgDate(d time.Time) pgtype.Date {
	return pgtype.Date{Time: d, Valid: true}
}

// DueOn devuelve los compromisos cuyo objetivo es `date` (para marcar ese día).
func (s *Service) DueOn(ctx context.Context, userID uuid.UUID, date time.Time) ([]Commitment, error) {
	rows, err := s.q.ListCommitmentsByTarget(ctx, store.ListCommitmentsByTargetParams{UserID: userID, TargetDate: pgDate(date)})
	if err != nil {
		return nil, err
	}
	return mapViews(rows), nil
}

// ReplaceForDate reemplaza los compromisos del usuario para `target` (borra y
// re-inserta, filtrando vacíos), en una transacción.
func (s *Service) ReplaceForDate(ctx context.Context, userID uuid.UUID, target time.Time, texts []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)
	if _, err := qtx.DeleteCommitmentsForDate(ctx, store.DeleteCommitmentsForDateParams{UserID: userID, TargetDate: pgDate(target)}); err != nil {
		return err
	}
	pos := 0
	for _, t := range texts {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, err := qtx.CreateCommitment(ctx, store.CreateCommitmentParams{
			UserID: userID, TargetDate: pgDate(target), Text: t, Position: int32(pos),
		}); err != nil {
			return err
		}
		pos++
	}
	return tx.Commit(ctx)
}

// Toggle invierte el cumplimiento. (nil, nil) si no es del usuario.
func (s *Service) Toggle(ctx context.Context, userID, id uuid.UUID) (*Commitment, error) {
	row, err := s.q.ToggleCommitment(ctx, store.ToggleCommitmentParams{ID: id, UserID: userID})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

// Recent devuelve los compromisos con target >= since (contexto de la IA).
func (s *Service) Recent(ctx context.Context, userID uuid.UUID, since time.Time) ([]Commitment, error) {
	rows, err := s.q.ListRecentCommitments(ctx, store.ListRecentCommitmentsParams{UserID: userID, TargetDate: pgDate(since)})
	if err != nil {
		return nil, err
	}
	return mapViews(rows), nil
}

func mapViews(rows []store.Commitment) []Commitment {
	out := make([]Commitment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toView(r))
	}
	return out
}
```

(Si sqlc generó `TargetDate` como `time.Time` en vez de `pgtype.Date`, quitar
`pgDate`/`.Time` y usar `time.Time` directo. Ajustar a lo generado.)

- [ ] **Step 4: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/commitments/ -count=1
git add api/internal/commitments
git commit -m "feat(commitments): servicio DueOn/ReplaceForDate/Toggle/Recent"
```

---

### Task 3: Rutas de commitments + el check-in escribe los de mañana + wiring

**Files:**
- Create: `api/internal/commitments/handler.go`, `api/internal/commitments/handler_test.go`
- Modify: `api/internal/checkin/handler.go` (escribir mañana), `api/internal/server/server.go`
- Test: el handler del checkin (si hay) o un test de integración del commitments handler

- [ ] **Step 1: Tests que fallan** `api/internal/commitments/handler_test.go` (montar con RequireAuth como en los otros; reusar el patrón de `checkin`/`finance` handler tests):

```go
// Test del handler: GET /due lista los del target; POST /{id}/toggle invierte;
// toggle ajeno → 404. Construir el router con el servicio real (testutil.NewDB)
// + auth, registrar un usuario, sembrar con svc.ReplaceForDate, y verificar:
//  - GET /commitments/due?date=2026-06-15 → 200 con los 2 compromisos.
//  - POST /commitments/{id}/toggle → 200, done=true.
//  - POST /commitments/{otro-uuid}/toggle → 404.
//  - sin token → 401.
```

(Escribir el test completo siguiendo el harness de `checkin`/`finance` handler
tests: `auth.RequireAuth` + `commitments.Routes(svc)` montado en `/commitments`,
helper de token. El plan da las aserciones obligatorias.)

- [ ] **Step 2: Implementar** `api/internal/commitments/handler.go`:

```go
package commitments

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const dateParam = "2006-01-02"

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/due", handleDue(svc))
	r.Post("/{id}/toggle", handleToggle(svc))
	return r
}

func handleDue(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		date := time.Now().UTC().Truncate(24 * time.Hour)
		if s := r.URL.Query().Get("date"); s != "" {
			if d, err := time.Parse(dateParam, s); err == nil {
				date = d
			}
		}
		due, err := svc.DueOn(r.Context(), userID, date)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"commitments": due})
	}
}

func handleToggle(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusNotFound, "compromiso no encontrado")
			return
		}
		c, err := svc.Toggle(r.Context(), userID, id)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if c == nil {
			httpx.WriteErr(w, http.StatusNotFound, "compromiso no encontrado")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"commitment": c})
	}
}
```

- [ ] **Step 3: El check-in escribe los de mañana.** En `api/internal/checkin/handler.go`:
  - Definir una interfaz estrecha en el paquete checkin (evita depender de commitments):

```go
// commitmentWriter es lo que el check-in usa para guardar los compromisos de
// mañana (lo implementa commitments.Service).
type commitmentWriter interface {
	ReplaceForDate(ctx context.Context, userID uuid.UUID, target time.Time, texts []string) error
}
```

  - `Routes` pasa a `func Routes(svc *Service, commits commitmentWriter) http.Handler` y `handleUpsert(svc, commits)`.
  - En `handleUpsert`, tras el `svc.Upsert` exitoso, antes del `WriteJSON`:

```go
// Los compromisos del body son para MAÑANA (target = fecha del check-in + 1).
if err := commits.ReplaceForDate(r.Context(), userID, date.AddDate(0, 0, 1), req.Commitments); err != nil {
	httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
	return
}
```

  (Imports `context`, `time`, `uuid` ya están o agregarlos.)

- [ ] **Step 4: Wiring** en `server.go`:

```go
commitmentsSvc := commitments.NewService(q, d.Pool)
// ...
r.Mount("/checkins", checkin.Routes(checkinSvc, commitmentsSvc))
r.Mount("/commitments", commitments.Routes(commitmentsSvc))
```

(import del paquete `commitments`. `commitmentsSvc` se declara antes de los mounts.)

- [ ] **Step 5: Verificar backend completo + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
git add api/internal/commitments api/internal/checkin/handler.go api/internal/server/server.go
git commit -m "feat(commitments): endpoints due/toggle y el check-in guarda los de mañana"
```

(El `go build ./...` debe pasar; si el handler_test del checkin construía `checkin.Routes(checkinSvc)` con un arg, actualizarlo a 2 args pasando un fake `commitmentWriter`.)

---

### Task 4: La IA ve los compromisos (chatcontext)

**Files:**
- Modify: `api/internal/ai/chatcontext.go`, `api/internal/server/server.go`, `api/internal/ai/handler_test.go` (wiring)
- Test: `api/internal/ai/chatcontext_test.go`

- [ ] **Step 1: Test que falla** en `chatcontext_test.go`: el builder gana un fake `commitmentLister` y el JSON incluye los compromisos. Agregar fake + assert:

```go
type fakeCommitments struct {
	list []commitments.Commitment
}

func (f fakeCommitments) Recent(ctx context.Context, userID uuid.UUID, since time.Time) ([]commitments.Commitment, error) {
	return f.list, nil
}
```

En el test de composición (`TestChatContextComposesJSON`), construir con el
fake nuevo (`commits := fakeCommitments{list: []commitments.Commitment{{ID:"x",
TargetDate:"2026-06-14", Text:"tender la cama", Done:true}}}`) y assert:

```go
if !strings.Contains(out, "tender la cama") {
	t.Errorf("el contexto debe incluir los compromisos: %s", out)
}
```

(import `github.com/focus365/api/internal/commitments`.) Actualizar TODAS las
construcciones de `newChatContextBuilder` del archivo con el arg nuevo.

- [ ] **Step 2: Verificar que falla** (aridad del constructor).

- [ ] **Step 3: Implementar** en `chatcontext.go`:
  - Interfaz nueva:

```go
// commitmentLister es la porción de commitments.Service que usamos.
type commitmentLister interface {
	Recent(ctx context.Context, userID uuid.UUID, since time.Time) ([]commitments.Commitment, error)
}
```

  - `chatContextBuilder` gana `commits commitmentLister`; `NewChatContextBuilder`
    y `newChatContextBuilder` ganan el parámetro (al final). Import `commitments`.
  - En `build`, tras los goals, antes del marshal:

```go
comms, err := b.commits.Recent(ctx, userID, today.AddDate(0, 0, -7))
if err != nil {
	return "", err
}
```

  y el map gana `"commitments": comms`.

- [ ] **Step 4: Wiring** en `server.go`: `chatCtx := ai.NewChatContextBuilder(dashboardSvc, financeSvc, checkinSvc, habitsSvc, goalsSvc, commitmentsSvc)`. En `handler_test.go` (`newEnv`): construir un `commitments.NewService(q, pool)` (o un fake) y pasarlo a `NewChatContextBuilder`.

- [ ] **Step 5: Verificar backend completo + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
git add api/internal/ai api/internal/server/server.go
git commit -m "feat(ai): el contexto del chat incluye los compromisos recientes y su cumplimiento"
```

---

### Task 5: Frontend — sección «ayer» + toggle + mañana

**Files:**
- Create: `web/src/lib/commitments.ts`, `web/src/lib/commitments.test.ts`
- Modify: `web/src/lib/checkins.ts` (CheckIn pierde commitments), `web/src/routes/check-in.tsx`, `web/src/routes/check-in.test.tsx`

- [ ] **Step 1: Lib (TDD).** `web/src/lib/commitments.ts`:

```ts
import { apiFetch } from "./api";

export type Commitment = {
  id: string;
  target_date: string;
  text: string;
  done: boolean;
};

export function getDue(date: string): Promise<Commitment[]> {
  return apiFetch<{ commitments: Commitment[] }>(
    `/api/v1/commitments/due?date=${encodeURIComponent(date)}`
  ).then((r) => r.commitments);
}

export function toggle(id: string): Promise<Commitment> {
  return apiFetch<{ commitment: Commitment }>(
    `/api/v1/commitments/${id}/toggle`,
    { method: "POST" }
  ).then((r) => r.commitment);
}
```

Tests en `commitments.test.ts`: `getDue` hace GET con la fecha y devuelve el
array; `toggle` hace POST a `/{id}/toggle` y devuelve el commitment.

`web/src/lib/checkins.ts`: el tipo `CheckIn` y `CheckInInput` ya NO tienen
`commitments`. **Pero** el `upsert` sigue mandando `commitments: string[]` en el
body (son los de mañana) — mantener `commitments` en `CheckInInput` (es lo que
se postea), solo quitarlo de `CheckIn` (la respuesta ya no los trae). Actualizar
`checkins.test.ts` (la respuesta `CheckIn` sin commitments; el input los
conserva).

- [ ] **Step 2: Página (TDD).** En `check-in.test.tsx`, agregar/adaptar tests:
  - «ayer te comprometiste a» aparece cuando `getDue(today)` devuelve
    compromisos; cada uno con un checkbox; click llama el toggle.
  - sin due de hoy, la sección no se muestra.
  - la lista «mañana» precarga con `getDue(tomorrow)` y se guarda con el check-in.
  El mock de fetch debe responder a `/commitments/due?date=<today>` (los de
  ayer), `/commitments/due?date=<tomorrow>` (precarga de mañana),
  `/commitments/<id>/toggle`, `/checkins/today` y `POST /checkins`.

- [ ] **Step 3: Implementar** en `check-in.tsx`:
  - Imports: `getDue`, `toggle` de `@/lib/commitments`, tipo `Commitment`.
  - `today` ya existe; calcular `tomorrow` (today + 1 día, formato YYYY-MM-DD).
  - `dueQuery = useQuery(["commitments","due",today], () => getDue(today))`.
  - **Sección «ayer»** arriba del form (dentro de `PageTransition`, antes de la
    Card del check-in): si `dueQuery.data?.length`, una `Card` con título
    «📋 Ayer te comprometiste a» + contador `N/M ✓` + por cada commitment un
    botón-checkbox que llama una `toggleMutation`:

```tsx
const toggleMutation = useMutation({
  mutationFn: (id: string) => toggle(id),
  onSuccess: () => qc.invalidateQueries({ queryKey: ["commitments", "due", today] }),
});
```

    Cada item: `<button aria-label={\`Marcar: \${c.text}\`} onClick={() => toggleMutation.mutate(c.id)} ...>` con el check (✓ si `c.done`) y el texto (line-through si done).
  - **Precarga de «mañana»:** la lista `commitments` (estado existente) se
    inicializa desde `getDue(tomorrow)` — `useQuery(["commitments","due",tomorrow])`
    en un `useEffect` que setea `setCommitments(data.map(c => c.text))` una sola
    vez (guard con un ref como ya hace el preload del check-in). El guardado del
    check-in (`upsert({...commitments})`) ya escribe los de mañana en el backend.
  - El check-in ya no precarga `commitments` desde `ci.commitments` (ya no
    existe); quitar esa línea del preload del check-in.

- [ ] **Step 4: Suite completa + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src
git commit -m "feat(web): compromisos rastreables — sección de ayer con check y precarga de mañana"
```

---

### Task 6: Cierre — review, merge, deploy, smoke de producción

- [ ] **Step 1:** Suites completas (backend `-p 1 ./...` + frontend + build) y smoke local de acciones (`/tmp/smoke_actions.sh`).
- [ ] **Step 2:** Rebuild docker; smoke local del flujo: `POST /checkins` con `commitments:["A","B"]` para una fecha F → `GET /commitments/due?date=F+1` devuelve A,B → `POST /commitments/{id}/toggle` → done=true.
- [ ] **Step 3:** Review final holística (subagente), nits.
- [ ] **Step 4:** Merge `--no-ff` a `main` + push. **Verificar el deploy** (auto-deploy o Deploy manual de Coolify; las migraciones 0015 se aplican al arrancar — verifican que el JSONB migró sin perder datos).
- [ ] **Step 5:** Smoke de producción: guardar un check-in con 2 compromisos → `GET /commitments/due` del día siguiente los muestra → toggle uno → done.
- [ ] **Step 6:** Bitácora en `docs/superpowers/sesiones/` y push.

---

## Notas para el ejecutor

- Los tipos de `DATE` generados por sqlc con pgx pueden ser `pgtype.Date` o `time.Time` — ajustar `pgDate`/conversiones a lo generado (el plan asume `pgtype.Date`).
- El backend queda con `go build ./...` posiblemente roto entre las Tasks 1 y 3 (el handler del checkin necesita el `commitmentWriter` que se cablea en la Task 3); cada task verifica su(s) paquete(s), y el build completo se exige en la Task 3.
- El frontend se rompe en la Task 1 (CheckIn pierde commitments) y se exige verde en la Task 5.
- Las 7 acciones del chat, el undo y el check-in (4D/win) son INVARIANTES.
- Compat: la migración 0015 mueve los compromisos del JSONB a la tabla con target = fecha+1; verificar en el smoke de producción que los compromisos viejos del usuario aparecen.
