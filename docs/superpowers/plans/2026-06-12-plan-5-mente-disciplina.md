# Plan 5 — Mente / Disciplina (hábitos + rachas) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Construir el módulo 4 "Mente/Disciplina": hábitos/retos unificados con registro binario diario, rachas calculadas desde los logs (racha actual + récord), ventana de gracia hoy/ayer, archivado manual; backend Go + frontend React.

**Architecture:** Paquete Go `internal/habits` calcado de `internal/training` (service + handler + types), montado bajo `RequireAuth` en `/api/v1/habits`. Dos tablas (`habits`, `habit_logs`) scoped por `user_id`. Las rachas se derivan en Go con un helper puro `computeStreaks` (sin contadores desnormalizados). El día "hoy" llega del cliente vía `?today=YYYY-MM-DD` (zona local), igual que `finance`. Frontend: lib `habits.ts` + página `/disciplina`.

**Tech Stack:** Go 1.23 (chi v5, pgx/v5, sqlc, goose, validator/v10, google/uuid), PostgreSQL 16, React 18 + Vite + TanStack Router/Query + Tailwind + Vitest.

---

## Convenciones de entorno (leer antes de empezar)

- **Go:** todos los comandos `go`/`sqlc` corren desde `api/` y con `GOTOOLCHAIN=local`. Nunca editar `go.mod`/`go.sum` a mano.
- **DB de test / migraciones:** `postgres://focus:changeme@localhost:5544/focus365?sslmode=disable`.
- **Tests Go comparten una sola DB:** correr con `-p 1` (usar `make test` / `make check`).
- **sqlc:** `cd api && sqlc generate`. Config en `api/sqlc.yaml`: nullable INT → `*int32`, nullable TIMESTAMPTZ → `*time.Time`, DATE/TIMESTAMPTZ NOT NULL → `time.Time`, UUID → `uuid.UUID`. Los archivos generados en `internal/store/` se commitean.
- **Migraciones:** aplicar con `cd api && DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go run ./cmd/migrate`.
- **Frontend:** desde `web/`. Para build correr primero `npx vite build` solo (regenera `routeTree.gen.ts`), después `npm run build`. En tests usar `vi.fn((_url: string, _opts?: RequestInit) => ...)` con parámetros tipados (un `vi.fn(() => ...)` sin parámetros rompe `tsc` al inferir tupla vacía en `.mock.calls`).
- **Commits y comentarios en español.**

---

## Estructura de archivos

**Backend (paquete nuevo `internal/habits`)**
- Crear `api/db/migrations/0005_habits.sql` — tablas `habits` + `habit_logs` e índices.
- Crear `api/db/queries/habits.sql` — queries sqlc.
- Generado (sqlc): `api/internal/store/habits.sql.go`, structs en `models.go` (commitear).
- Crear `api/internal/store/habits_test.go` — test de store.
- Crear `api/internal/habits/types.go` — `Habit`, `HabitInput`, `dateLayout`.
- Crear `api/internal/habits/service.go` — `Service`, métodos, `computeStreaks` puro.
- Crear `api/internal/habits/streaks_test.go` — tabla de casos de `computeStreaks`.
- Crear `api/internal/habits/handler.go` — `Routes`, handlers, `parseTodayParam`, `sameDay`.
- Crear `api/internal/habits/handler_test.go` — tests de integración HTTP.
- Modificar `api/internal/httpx/httpx.go` — labels `TargetDays`, `Day`, `Done`.
- Modificar `api/internal/server/server.go` — montar `/habits`.

**Frontend**
- Crear `web/src/lib/habits.ts` — tipos + funciones de API.
- Crear `web/src/lib/habits.test.ts` — test de la lib.
- Crear `web/src/routes/disciplina.tsx` — página `/disciplina`.
- Crear `web/src/routes/disciplina.test.tsx` — test de la página.
- Modificar `web/src/routes/index.tsx` — enlace "Disciplina" en el home.

---

### Task 1: Migración `0005_habits.sql`

**Files:**
- Create: `api/db/migrations/0005_habits.sql`

- [ ] **Step 1: Escribir la migración**

Crear `api/db/migrations/0005_habits.sql`:

```sql
-- +goose Up
CREATE TABLE habits (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    target_days INT,                      -- meta de N días (challenge); NULL = hábito abierto
    archived_at TIMESTAMPTZ,              -- archivado manual; NULL = activo
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Único parcial: no se permiten dos hábitos ACTIVOS con el mismo nombre (sin
-- distinguir mayúsculas); un nombre archivado no bloquea recrearlo.
CREATE UNIQUE INDEX uq_habits_user_name
    ON habits (user_id, lower(name)) WHERE archived_at IS NULL;
CREATE INDEX idx_habits_user_active
    ON habits (user_id, created_at DESC);

CREATE TABLE habit_logs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    habit_id   UUID NOT NULL REFERENCES habits(id) ON DELETE CASCADE,
    day        DATE NOT NULL,             -- el día cumplido
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Un registro por hábito por día. Marcar = upsert; desmarcar = delete.
CREATE UNIQUE INDEX uq_habit_logs_habit_day
    ON habit_logs (habit_id, day);
CREATE INDEX idx_habit_logs_habit_day
    ON habit_logs (habit_id, day DESC);

-- +goose Down
DROP TABLE habit_logs;
DROP TABLE habits;
```

- [ ] **Step 2: Aplicar la migración y verificar**

Run:
```bash
cd api && DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go run ./cmd/migrate
```
Expected: aplica `0005_habits` sin errores (sale 0). Si la DB no está levantada, primero `docker compose up -d db`.

- [ ] **Step 3: Commit**

```bash
git add api/db/migrations/0005_habits.sql
git commit -m "feat(habits): migración 0005 con tablas habits y habit_logs"
```

---

### Task 2: Queries sqlc `habits.sql` + generar

**Files:**
- Create: `api/db/queries/habits.sql`
- Generated: `api/internal/store/habits.sql.go`, `api/internal/store/models.go` (sqlc)

- [ ] **Step 1: Escribir las queries**

Crear `api/db/queries/habits.sql`:

```sql
-- name: CreateHabit :one
-- Idempotente por (user_id, lower(name)) entre hábitos ACTIVOS: si ya existe
-- uno activo con ese nombre, devuelve el actual (no toca target_days).
INSERT INTO habits (user_id, name, target_days)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, lower(name)) WHERE archived_at IS NULL
DO UPDATE SET name = habits.name
RETURNING *;

-- name: ListHabits :many
SELECT * FROM habits
WHERE user_id = $1 AND archived_at IS NULL
ORDER BY created_at DESC;

-- name: ListArchivedHabits :many
SELECT * FROM habits
WHERE user_id = $1 AND archived_at IS NOT NULL
ORDER BY archived_at DESC;

-- name: GetHabit :one
SELECT * FROM habits
WHERE id = $1 AND user_id = $2;

-- name: ArchiveHabit :one
UPDATE habits
SET archived_at = now()
WHERE id = $1 AND user_id = $2 AND archived_at IS NULL
RETURNING *;

-- name: DeleteHabit :execrows
DELETE FROM habits
WHERE id = $1 AND user_id = $2;

-- name: UpsertHabitLog :exec
INSERT INTO habit_logs (habit_id, day)
VALUES ($1, $2)
ON CONFLICT (habit_id, day) DO NOTHING;

-- name: DeleteHabitLog :exec
DELETE FROM habit_logs
WHERE habit_id = $1 AND day = $2;

-- name: ListLogsByHabitIDs :many
SELECT habit_id, day FROM habit_logs
WHERE habit_id = ANY(sqlc.arg('habit_ids')::uuid[])
ORDER BY day ASC;
```

- [ ] **Step 2: Generar el código sqlc**

Run:
```bash
cd api && sqlc generate
```
Expected: sin errores. Aparecen `internal/store/habits.sql.go` y nuevos structs `Habit`, `HabitLog` en `internal/store/models.go`. Verificar tipos esperados: `Habit.TargetDays *int32`, `Habit.ArchivedAt *time.Time`, `HabitLog.Day time.Time`, `CreateHabitParams{UserID uuid.UUID; Name string; TargetDays *int32}`, `ListLogsByHabitIDsRow{HabitID uuid.UUID; Day time.Time}`.

- [ ] **Step 3: Compilar para confirmar que todo encaja**

Run:
```bash
cd api && GOTOOLCHAIN=local go build ./...
```
Expected: compila sin errores.

- [ ] **Step 4: Commit**

```bash
git add api/db/queries/habits.sql api/internal/store/
git commit -m "feat(habits): queries sqlc de hábitos y logs"
```

---

### Task 3: Test de store

**Files:**
- Create: `api/internal/store/habits_test.go`

- [ ] **Step 1: Escribir el test fallido**

Crear `api/internal/store/habits_test.go`:

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
```

- [ ] **Step 2: Correr el test y verificar que pasa**

Run:
```bash
cd api && TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go test ./internal/store/ -run TestHabitsStore -v
```
Expected: PASS. (Las queries y el schema ya existen de las tasks 1–2, así que este test valida la capa store completa.)

- [ ] **Step 3: Commit**

```bash
git add api/internal/store/habits_test.go
git commit -m "test(habits): cobertura del store (idempotencia, archivar, cascade)"
```

---

### Task 4: Tipos de dominio + servicio + `computeStreaks`

**Files:**
- Create: `api/internal/habits/types.go`
- Create: `api/internal/habits/service.go`
- Create: `api/internal/habits/streaks_test.go`

- [ ] **Step 1: Escribir los tipos de dominio**

Crear `api/internal/habits/types.go`:

```go
package habits

import "time"

const dateLayout = "2006-01-02"

// Habit es la vista de dominio de un hábito con las rachas ya calculadas.
// archived_at va como ISO timestamp (o null si está activo).
type Habit struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	TargetDays    *int32     `json:"target_days"`
	CurrentStreak int        `json:"current_streak"`
	BestStreak    int        `json:"best_streak"`
	DoneToday     bool       `json:"done_today"`
	DoneYesterday bool       `json:"done_yesterday"`
	ArchivedAt    *time.Time `json:"archived_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// HabitInput son los datos de dominio para crear un hábito.
type HabitInput struct {
	Name       string
	TargetDays *int32
}
```

- [ ] **Step 2: Escribir el test fallido de `computeStreaks`**

Crear `api/internal/habits/streaks_test.go`:

```go
package habits

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, _ := time.Parse(dateLayout, s)
	return t
}

func TestComputeStreaks(t *testing.T) {
	today := d("2026-06-14") // referencia fija

	cases := []struct {
		name      string
		days      []time.Time
		wantCur   int
		wantBest  int
		wantToday bool
		wantYest  bool
	}{
		{
			name: "historial vacío",
			days: nil,
			wantCur: 0, wantBest: 0, wantToday: false, wantYest: false,
		},
		{
			name: "un solo día hoy",
			days: []time.Time{d("2026-06-14")},
			wantCur: 1, wantBest: 1, wantToday: true, wantYest: false,
		},
		{
			name: "corrida consecutiva hasta hoy",
			days: []time.Time{d("2026-06-12"), d("2026-06-13"), d("2026-06-14")},
			wantCur: 3, wantBest: 3, wantToday: true, wantYest: true,
		},
		{
			name: "racha viva anclada en ayer (hoy pendiente)",
			days: []time.Time{d("2026-06-12"), d("2026-06-13")},
			wantCur: 2, wantBest: 2, wantToday: false, wantYest: true,
		},
		{
			name: "racha cortada (ni hoy ni ayer)",
			days: []time.Time{d("2026-06-10"), d("2026-06-11")},
			wantCur: 0, wantBest: 2, wantToday: false, wantYest: false,
		},
		{
			name: "récord mayor que la actual",
			days: []time.Time{d("2026-06-09"), d("2026-06-10"), d("2026-06-11"), d("2026-06-13"), d("2026-06-14")},
			wantCur: 2, wantBest: 3, wantToday: true, wantYest: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cur, best, doneToday, doneYest := computeStreaks(c.days, today)
			if cur != c.wantCur {
				t.Errorf("current = %d, want %d", cur, c.wantCur)
			}
			if best != c.wantBest {
				t.Errorf("best = %d, want %d", best, c.wantBest)
			}
			if doneToday != c.wantToday {
				t.Errorf("doneToday = %v, want %v", doneToday, c.wantToday)
			}
			if doneYest != c.wantYest {
				t.Errorf("doneYesterday = %v, want %v", doneYest, c.wantYest)
			}
		})
	}
}
```

- [ ] **Step 3: Correr el test para verificar que falla a compilar**

Run:
```bash
cd api && GOTOOLCHAIN=local go test ./internal/habits/ -run TestComputeStreaks
```
Expected: FAIL — `undefined: computeStreaks` (todavía no existe el servicio).

- [ ] **Step 4: Escribir el servicio con `computeStreaks`**

Crear `api/internal/habits/service.go`:

```go
package habits

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q    *store.Queries
	pool *pgxpool.Pool
}

func NewService(q *store.Queries, pool *pgxpool.Pool) *Service {
	return &Service{q: q, pool: pool}
}

// List devuelve los hábitos del usuario (activos o archivados) con sus rachas
// calculadas. today se pasa desde el cliente (su zona local). Evita N+1
// trayendo todos los logs en una sola query y agrupándolos por hábito.
func (s *Service) List(ctx context.Context, userID uuid.UUID, archived bool, today time.Time) ([]Habit, error) {
	var rows []store.Habit
	var err error
	if archived {
		rows, err = s.q.ListArchivedHabits(ctx, userID)
	} else {
		rows, err = s.q.ListHabits(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Habit{}, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, h := range rows {
		ids[i] = h.ID
	}
	logs, err := s.q.ListLogsByHabitIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byHabit := make(map[uuid.UUID][]time.Time)
	for _, l := range logs {
		byHabit[l.HabitID] = append(byHabit[l.HabitID], l.Day)
	}
	out := make([]Habit, 0, len(rows))
	for _, h := range rows {
		out = append(out, *buildHabit(h, byHabit[h.ID], today))
	}
	return out, nil
}

// Create crea (o devuelve, idempotente por nombre activo) el hábito.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, in HabitInput, today time.Time) (*Habit, error) {
	h, err := s.q.CreateHabit(ctx, store.CreateHabitParams{
		UserID: userID, Name: strings.TrimSpace(in.Name), TargetDays: in.TargetDays,
	})
	if err != nil {
		return nil, err
	}
	return s.habitView(ctx, h, today)
}

// SetCheck marca (done) o desmarca (!done) el día y devuelve el hábito
// recalculado. (nil, nil) si el hábito no es del usuario → 404 en el handler.
func (s *Service) SetCheck(ctx context.Context, userID, habitID uuid.UUID, day time.Time, done bool, today time.Time) (*Habit, error) {
	h, err := s.q.GetHabit(ctx, store.GetHabitParams{ID: habitID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if done {
		if err := s.q.UpsertHabitLog(ctx, store.UpsertHabitLogParams{HabitID: h.ID, Day: day}); err != nil {
			return nil, err
		}
	} else {
		if err := s.q.DeleteHabitLog(ctx, store.DeleteHabitLogParams{HabitID: h.ID, Day: day}); err != nil {
			return nil, err
		}
	}
	return s.habitView(ctx, h, today)
}

// Archive marca el hábito como archivado. (nil, nil) si no es del usuario o ya
// estaba archivado.
func (s *Service) Archive(ctx context.Context, userID, habitID uuid.UUID, today time.Time) (*Habit, error) {
	h, err := s.q.ArchiveHabit(ctx, store.ArchiveHabitParams{ID: habitID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s.habitView(ctx, h, today)
}

// Delete borra el hábito y sus logs (cascade). Devuelve si borró algo.
func (s *Service) Delete(ctx context.Context, userID, habitID uuid.UUID) (bool, error) {
	n, err := s.q.DeleteHabit(ctx, store.DeleteHabitParams{ID: habitID, UserID: userID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// habitView recarga los logs de un solo hábito y arma la vista con rachas.
func (s *Service) habitView(ctx context.Context, h store.Habit, today time.Time) (*Habit, error) {
	logs, err := s.q.ListLogsByHabitIDs(ctx, []uuid.UUID{h.ID})
	if err != nil {
		return nil, err
	}
	days := make([]time.Time, 0, len(logs))
	for _, l := range logs {
		days = append(days, l.Day)
	}
	return buildHabit(h, days, today), nil
}

func buildHabit(h store.Habit, days []time.Time, today time.Time) *Habit {
	current, best, doneToday, doneYesterday := computeStreaks(days, today)
	return &Habit{
		ID:            h.ID.String(),
		Name:          h.Name,
		TargetDays:    h.TargetDays,
		CurrentStreak: current,
		BestStreak:    best,
		DoneToday:     doneToday,
		DoneYesterday: doneYesterday,
		ArchivedAt:    h.ArchivedAt,
		CreatedAt:     h.CreatedAt,
	}
}

// computeStreaks deriva, a partir de los días con log, la racha actual, el
// récord histórico y si hoy/ayer están marcados. Es pura (no toca DB). today
// se pasa desde afuera para no depender del reloj del server.
func computeStreaks(days []time.Time, today time.Time) (current, best int, doneToday, doneYesterday bool) {
	if len(days) == 0 {
		return 0, 0, false, false
	}
	// Set de días normalizados a YYYY-MM-DD para lookups O(1).
	set := make(map[string]bool, len(days))
	for _, day := range days {
		set[day.Format(dateLayout)] = true
	}
	yesterday := today.AddDate(0, 0, -1)
	doneToday = set[today.Format(dateLayout)]
	doneYesterday = set[yesterday.Format(dateLayout)]

	// Racha actual: ancla en hoy si está hecho; si no, en ayer si está hecho;
	// si ninguno, la racha se cortó (0). Cuenta consecutivos hacia atrás.
	var anchor time.Time
	switch {
	case doneToday:
		anchor = today
	case doneYesterday:
		anchor = yesterday
	}
	if !anchor.IsZero() {
		for c := anchor; set[c.Format(dateLayout)]; c = c.AddDate(0, 0, -1) {
			current++
		}
	}

	// Récord: corrida consecutiva más larga sobre los días únicos ordenados.
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys) // YYYY-MM-DD ordena cronológicamente como string
	run := 0
	var prev time.Time
	for i, k := range keys {
		day, _ := time.Parse(dateLayout, k)
		if i > 0 && day.Sub(prev) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		if run > best {
			best = run
		}
		prev = day
	}
	return current, best, doneToday, doneYesterday
}
```

- [ ] **Step 5: Correr el test de `computeStreaks` y verificar que pasa**

Run:
```bash
cd api && GOTOOLCHAIN=local go test ./internal/habits/ -run TestComputeStreaks -v
```
Expected: PASS (los 6 sub-casos).

- [ ] **Step 6: Compilar todo el paquete**

Run:
```bash
cd api && GOTOOLCHAIN=local go build ./...
```
Expected: compila sin errores.

- [ ] **Step 7: Commit**

```bash
git add api/internal/habits/types.go api/internal/habits/service.go api/internal/habits/streaks_test.go
git commit -m "feat(habits): servicio de dominio y cálculo puro de rachas"
```

---

### Task 5: Handlers HTTP + labels + montaje

**Files:**
- Create: `api/internal/habits/handler.go`
- Modify: `api/internal/httpx/httpx.go` (función `fieldLabel`)
- Modify: `api/internal/server/server.go`

- [ ] **Step 1: Escribir los handlers**

Crear `api/internal/habits/handler.go`:

```go
package habits

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type habitReq struct {
	Name       string `json:"name" validate:"required"`
	TargetDays *int32 `json:"target_days" validate:"omitempty,min=1"`
}

type checkReq struct {
	Day  string `json:"day"`
	Done *bool  `json:"done" validate:"required"`
}

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/", handleList(svc))
	r.Post("/", handleCreate(svc))
	r.Post("/{id}/check", handleCheck(svc))
	r.Post("/{id}/archive", handleArchive(svc))
	r.Delete("/{id}", handleDelete(svc))
	return r
}

func handleList(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		archived := r.URL.Query().Get("archived") == "true"
		list, err := svc.List(r.Context(), userID, archived, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
	}
}

func handleCreate(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req habitReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		out, err := svc.Create(r.Context(), userID, HabitInput{Name: req.Name, TargetDays: req.TargetDays}, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, out)
	}
}

func handleCheck(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		var req checkReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		today := parseTodayParam(r)
		day := today
		if req.Day != "" {
			parsed, err := time.Parse(dateLayout, req.Day)
			if err != nil {
				httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
				return
			}
			day = parsed
		}
		// Ventana de gracia: solo se acepta marcar hoy o ayer.
		if !sameDay(day, today) && !sameDay(day, today.AddDate(0, 0, -1)) {
			httpx.WriteErr(w, http.StatusBadRequest, "solo podés marcar hoy o ayer")
			return
		}
		out, err := svc.SetCheck(r.Context(), userID, id, day, *req.Done, today)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if out == nil {
			httpx.WriteErr(w, http.StatusNotFound, "no encontrado")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

func handleArchive(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		out, err := svc.Archive(r.Context(), userID, id, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if out == nil {
			httpx.WriteErr(w, http.StatusNotFound, "no encontrado")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

func handleDelete(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "id inválido")
			return
		}
		deleted, err := svc.Delete(r.Context(), userID, id)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		if !deleted {
			httpx.WriteErr(w, http.StatusNotFound, "no encontrado")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// parseTodayParam lee ?today=YYYY-MM-DD (zona del cliente). Si falta o es
// inválido, cae al día UTC del server.
func parseTodayParam(r *http.Request) time.Time {
	if s := r.URL.Query().Get("today"); s != "" {
		if t, err := time.Parse(dateLayout, s); err == nil {
			return t
		}
	}
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// sameDay compara dos fechas por su día (YYYY-MM-DD).
func sameDay(a, b time.Time) bool {
	return a.Format(dateLayout) == b.Format(dateLayout)
}
```

- [ ] **Step 2: Agregar los labels nuevos en httpx**

En `api/internal/httpx/httpx.go`, dentro de `fieldLabel`, agregar los casos nuevos justo antes de `case "Exercise":` (mantener el resto igual):

```go
	case "TargetDays":
		return "la meta de días"
	case "Day":
		return "el día"
	case "Done":
		return "el estado"
```

- [ ] **Step 3: Montar el módulo en el server**

En `api/internal/server/server.go`:

1. Agregar el import (en orden alfabético, después de `finance`):
```go
	"github.com/focus365/api/internal/habits"
```

2. Crear el servicio junto a los otros (después de `trainingSvc := ...`):
```go
	habitsSvc := habits.NewService(q, d.Pool)
```

3. Montar la ruta dentro del grupo `RequireAuth` (después de la línea de `training`):
```go
			r.Mount("/habits", habits.Routes(habitsSvc))
```

- [ ] **Step 4: Compilar**

Run:
```bash
cd api && GOTOOLCHAIN=local go build ./...
```
Expected: compila sin errores.

- [ ] **Step 5: Commit**

```bash
git add api/internal/habits/handler.go api/internal/httpx/httpx.go api/internal/server/server.go
git commit -m "feat(habits): handlers HTTP, labels de validación y montaje en /habits"
```

---

### Task 6: Tests de handler (integración HTTP)

**Files:**
- Create: `api/internal/habits/handler_test.go`

- [ ] **Step 1: Escribir los tests**

Crear `api/internal/habits/handler_test.go`:

```go
package habits_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/go-chi/chi/v5"
)

type env struct {
	h    http.Handler
	auth *auth.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	tm := auth.NewTokenManager("secret")
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/habits", habits.Routes(habits.NewService(q, pool)))
	})
	return &env{h: r, auth: auth.NewService(q, tm)}
}

func (e *env) token(t *testing.T, email string) string {
	t.Helper()
	user, err := e.auth.Register(context.Background(), email, "p4ssword", "User")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	access, _, err := e.auth.IssueTokens(user.ID)
	if err != nil {
		t.Fatalf("IssueTokens: %v", err)
	}
	return access
}

func do(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		raw, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateAndList(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "a@b.com")

	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{
		"name": "Leer", "target_days": 21,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST habit code = %d, body = %s", rec.Code, rec.Body.String())
	}

	recL := do(t, e.h, http.MethodGet, "/habits?today=2026-06-14", tok, nil)
	if recL.Code != http.StatusOK {
		t.Fatalf("GET habits code = %d", recL.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("lista = %d, want 1", len(list))
	}
	if list[0]["name"] != "Leer" {
		t.Errorf("name = %v, want Leer", list[0]["name"])
	}
	if list[0]["current_streak"].(float64) != 0 {
		t.Errorf("current_streak = %v, want 0", list[0]["current_streak"])
	}
}

func TestCheckTodayAndYesterday(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "c@b.com")
	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{"name": "Flexiones"})
	var h map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &h)
	id := h["id"].(string)

	// Marcar hoy → racha 1.
	recT := do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{
		"day": "2026-06-14", "done": true,
	})
	if recT.Code != http.StatusOK {
		t.Fatalf("check hoy code = %d, body = %s", recT.Code, recT.Body.String())
	}
	var afterToday map[string]any
	_ = json.Unmarshal(recT.Body.Bytes(), &afterToday)
	if afterToday["current_streak"].(float64) != 1 {
		t.Errorf("racha tras hoy = %v, want 1", afterToday["current_streak"])
	}
	if afterToday["done_today"] != true {
		t.Errorf("done_today = %v, want true", afterToday["done_today"])
	}

	// Marcar ayer → racha 2.
	recY := do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{
		"day": "2026-06-13", "done": true,
	})
	var afterYest map[string]any
	_ = json.Unmarshal(recY.Body.Bytes(), &afterYest)
	if afterYest["current_streak"].(float64) != 2 {
		t.Errorf("racha tras ayer = %v, want 2", afterYest["current_streak"])
	}
}

func TestCheckRejectsBeforeYesterday(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "r@b.com")
	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{"name": "Meditar"})
	var h map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &h)
	id := h["id"].(string)

	recB := do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{
		"day": "2026-06-12", "done": true,
	})
	if recB.Code != http.StatusBadRequest {
		t.Errorf("marcar anteayer code = %d, want 400", recB.Code)
	}
}

func TestUncheckLowersStreak(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "u@b.com")
	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{"name": "Correr"})
	var h map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &h)
	id := h["id"].(string)

	_ = do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{"day": "2026-06-14", "done": true})
	recU := do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{"day": "2026-06-14", "done": false})
	var after map[string]any
	_ = json.Unmarshal(recU.Body.Bytes(), &after)
	if after["current_streak"].(float64) != 0 {
		t.Errorf("racha tras desmarcar = %v, want 0", after["current_streak"])
	}
}

func TestArchiveMovesOutOfActive(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "ar@b.com")
	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{"name": "Diario"})
	var h map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &h)
	id := h["id"].(string)

	if recA := do(t, e.h, http.MethodPost, "/habits/"+id+"/archive?today=2026-06-14", tok, nil); recA.Code != http.StatusOK {
		t.Fatalf("archive code = %d", recA.Code)
	}
	recAct := do(t, e.h, http.MethodGet, "/habits?today=2026-06-14", tok, nil)
	var active []map[string]any
	_ = json.Unmarshal(recAct.Body.Bytes(), &active)
	if len(active) != 0 {
		t.Errorf("activos = %d, want 0", len(active))
	}
	recArch := do(t, e.h, http.MethodGet, "/habits?today=2026-06-14&archived=true", tok, nil)
	var arch []map[string]any
	_ = json.Unmarshal(recArch.Body.Bytes(), &arch)
	if len(arch) != 1 {
		t.Errorf("archivados = %d, want 1", len(arch))
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/habits", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	tokA := e.token(t, "uA@b.com")
	tokB := e.token(t, "uB@b.com")

	recA := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tokA, map[string]any{"name": "Privado"})
	var hA map[string]any
	_ = json.Unmarshal(recA.Body.Bytes(), &hA)
	idA := hA["id"].(string)

	// B no ve hábitos de A.
	recL := do(t, e.h, http.MethodGet, "/habits?today=2026-06-14", tokB, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("B ve %d hábitos de A; debería ver 0", len(list))
	}
	// B no puede marcar ni borrar el de A.
	if rec := do(t, e.h, http.MethodPost, "/habits/"+idA+"/check?today=2026-06-14", tokB, map[string]any{"day": "2026-06-14", "done": true}); rec.Code != http.StatusNotFound {
		t.Errorf("B marcó el hábito de A: code = %d, want 404", rec.Code)
	}
	if rec := do(t, e.h, http.MethodDelete, "/habits/"+idA, tokB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("B borró el hábito de A: code = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Correr los tests del paquete habits y verificar que pasan**

Run:
```bash
cd api && TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go test ./internal/habits/ -v
```
Expected: PASS (todos los tests de handler + `TestComputeStreaks`).

- [ ] **Step 3: Correr la suite Go completa (`make check`)**

Run:
```bash
cd api && make check
```
Expected: `go vet` limpio y todos los paquetes en `ok` (incl. `internal/store`, `internal/habits`). `make check` ya usa `-p 1`.

- [ ] **Step 4: Commit**

```bash
git add api/internal/habits/handler_test.go
git commit -m "test(habits): integración HTTP (rachas, ventana de gracia, aislamiento)"
```

---

### Task 7: Frontend lib `habits.ts`

**Files:**
- Create: `web/src/lib/habits.ts`
- Test: `web/src/lib/habits.test.ts`

- [ ] **Step 1: Escribir el test fallido**

Crear `web/src/lib/habits.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  listHabits,
  createHabit,
  checkHabit,
  archiveHabit,
  removeHabit,
  todayString,
  yesterdayString,
} from "./habits";
import { setAccessToken } from "./api";

function okJson(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), { status }));
}

describe("lib/habits", () => {
  beforeEach(() => setAccessToken(null));
  afterEach(() => vi.restoreAllMocks());

  it("yesterdayString es el día anterior a today", () => {
    const base = new Date(2026, 5, 12); // 2026-06-12 local
    expect(todayString(base)).toBe("2026-06-12");
    expect(yesterdayString(base)).toBe("2026-06-11");
  });

  it("listHabits pega a GET /habits con today y sin archived", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson([]));
    vi.stubGlobal("fetch", fetchMock);
    await listHabits();
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url.startsWith("/api/v1/habits?")).toBe(true);
    expect(url).toContain(`today=${todayString()}`);
    expect(url).not.toContain("archived");
  });

  it("listHabits(true) agrega archived=true", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) => okJson([]));
    vi.stubGlobal("fetch", fetchMock);
    await listHabits(true);
    expect(fetchMock.mock.calls[0][0] as string).toContain("archived=true");
  });

  it("createHabit hace POST con el body y manda Bearer si hay token", async () => {
    setAccessToken("tok123");
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "h1" }, 201)
    );
    vi.stubGlobal("fetch", fetchMock);
    await createHabit({ name: "Leer", target_days: 21 });
    const [url, opts] = fetchMock.mock.calls[0];
    expect((url as string).startsWith("/api/v1/habits?today=")).toBe(true);
    expect(opts?.method).toBe("POST");
    expect(JSON.parse(opts?.body as string)).toEqual({ name: "Leer", target_days: 21 });
    const headers = opts?.headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok123");
  });

  it("checkHabit arma el body con day y done", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "h1" }, 200)
    );
    vi.stubGlobal("fetch", fetchMock);
    await checkHabit("h1", "2026-06-12", true);
    const [url, opts] = fetchMock.mock.calls[0];
    expect((url as string).startsWith("/api/v1/habits/h1/check?today=")).toBe(true);
    expect(opts?.method).toBe("POST");
    expect(JSON.parse(opts?.body as string)).toEqual({ day: "2026-06-12", done: true });
  });

  it("archiveHabit hace POST a /archive", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      okJson({ id: "h1" }, 200)
    );
    vi.stubGlobal("fetch", fetchMock);
    await archiveHabit("h1");
    const [url, opts] = fetchMock.mock.calls[0];
    expect((url as string).startsWith("/api/v1/habits/h1/archive?today=")).toBe(true);
    expect(opts?.method).toBe("POST");
  });

  it("removeHabit hace DELETE al hábito", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(null, { status: 204 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    await removeHabit("h9");
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/habits/h9");
    expect(opts?.method).toBe("DELETE");
  });
});
```

- [ ] **Step 2: Correr el test para verificar que falla**

Run:
```bash
cd web && npx vitest run src/lib/habits.test.ts
```
Expected: FAIL — no resuelve `./habits` (el archivo no existe todavía).

- [ ] **Step 3: Escribir la lib**

Crear `web/src/lib/habits.ts`:

```ts
import { apiFetch } from "./api";

export type Habit = {
  id: string;
  name: string;
  target_days: number | null;
  current_streak: number;
  best_streak: number;
  done_today: boolean;
  done_yesterday: boolean;
  archived_at: string | null;
  created_at: string;
};

export type HabitInput = {
  name: string;
  target_days: number | null;
};

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

// yesterdayString es el día anterior a la fecha dada (ventana de gracia).
export function yesterdayString(date = new Date()): string {
  const d = new Date(date);
  d.setDate(d.getDate() - 1);
  return todayString(d);
}

export function listHabits(archived = false): Promise<Habit[]> {
  const params = new URLSearchParams();
  params.set("today", todayString());
  if (archived) params.set("archived", "true");
  return apiFetch<Habit[]>(`/api/v1/habits?${params.toString()}`);
}

export function createHabit(input: HabitInput): Promise<Habit> {
  return apiFetch<Habit>(`/api/v1/habits?today=${todayString()}`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function checkHabit(id: string, day: string, done: boolean): Promise<Habit> {
  return apiFetch<Habit>(`/api/v1/habits/${id}/check?today=${todayString()}`, {
    method: "POST",
    body: JSON.stringify({ day, done }),
  });
}

export function archiveHabit(id: string): Promise<Habit> {
  return apiFetch<Habit>(`/api/v1/habits/${id}/archive?today=${todayString()}`, {
    method: "POST",
  });
}

export function removeHabit(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/habits/${id}`, { method: "DELETE" });
}
```

- [ ] **Step 4: Correr el test y verificar que pasa**

Run:
```bash
cd web && npx vitest run src/lib/habits.test.ts
```
Expected: PASS (7 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/habits.ts web/src/lib/habits.test.ts
git commit -m "feat(habits): lib frontend de hábitos con today por zona del cliente"
```

---

### Task 8: Página `/disciplina` + enlace en el home

**Files:**
- Create: `web/src/routes/disciplina.tsx`
- Test: `web/src/routes/disciplina.test.tsx`
- Modify: `web/src/routes/index.tsx`

- [ ] **Step 1: Escribir la página**

Crear `web/src/routes/disciplina.tsx`:

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  listHabits,
  createHabit,
  checkHabit,
  archiveHabit,
  removeHabit,
  todayString,
  yesterdayString,
  type Habit,
} from "@/lib/habits";

export const Route = createFileRoute("/disciplina")({ component: DisciplinaPage });

function DisciplinaPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const [showArchived, setShowArchived] = useState(false);
  const habitsQuery = useQuery({
    queryKey: ["habits", showArchived],
    queryFn: () => listHabits(showArchived),
    enabled: !!user,
  });

  const [name, setName] = useState("");
  const [target, setTarget] = useState("");
  const [error, setError] = useState<string | null>(null);

  function invalidate() {
    qc.invalidateQueries({ queryKey: ["habits"] });
  }

  const createMutation = useMutation({
    mutationFn: () =>
      createHabit({
        name: name.trim(),
        target_days: target === "" ? null : Number(target),
      }),
    onSuccess: () => {
      setError(null);
      setName("");
      setTarget("");
      invalidate();
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "Error al crear"),
  });

  const checkMutation = useMutation({
    mutationFn: (v: { id: string; day: string; done: boolean }) =>
      checkHabit(v.id, v.day, v.done),
    onSuccess: invalidate,
  });

  const archiveMutation = useMutation({
    mutationFn: (id: string) => archiveHabit(id),
    onSuccess: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => removeHabit(id),
    onSuccess: invalidate,
  });

  if (!user) return null;

  return (
    <div className="mx-auto max-w-xl p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Disciplina</h1>
        <Link to="/" className="text-sm text-sand-400">Volver</Link>
      </header>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          createMutation.mutate();
        }}
        className="mt-6 space-y-4 rounded-xl border border-ink-700 bg-ink-900 p-6"
      >
        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Hábito o reto</span>
          <input
            type="text"
            aria-label="Nombre del hábito"
            placeholder="Leer 20 min, 100 flexiones…"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>
        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Meta de días (opcional)</span>
          <input
            type="number"
            aria-label="Meta de días"
            placeholder="21"
            min="1"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>
        {error && <p className="text-sm text-red-400">{error}</p>}
        <button
          type="submit"
          disabled={createMutation.isPending}
          className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          {createMutation.isPending ? "Creando…" : "Crear"}
        </button>
      </form>

      <div className="mt-6 flex gap-3 text-sm">
        <button
          type="button"
          onClick={() => setShowArchived(false)}
          className={!showArchived ? "font-bold text-amber-brand" : "text-sand-400"}
        >
          Activos
        </button>
        <button
          type="button"
          onClick={() => setShowArchived(true)}
          className={showArchived ? "font-bold text-amber-brand" : "text-sand-400"}
        >
          Archivados
        </button>
      </div>

      <section className="mt-4">
        {habitsQuery.data && habitsQuery.data.length > 0 ? (
          <ul className="space-y-3">
            {habitsQuery.data.map((h: Habit) => (
              <li
                key={h.id}
                className="rounded-xl border border-ink-700 bg-ink-900 p-4 text-sm"
              >
                <div className="flex items-center justify-between">
                  <span className="font-bold">{h.name}</span>
                  <span className="text-streak">🔥 {h.current_streak} días</span>
                </div>
                <p className="mt-1 text-xs text-sand-400">
                  Récord {h.best_streak}
                  {h.target_days != null && ` · meta ${h.target_days}`}
                </p>
                {h.target_days != null && (
                  <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-ink-800">
                    <div
                      className="h-full bg-streak"
                      style={{
                        width: `${Math.min(100, (h.current_streak / h.target_days) * 100)}%`,
                      }}
                    />
                  </div>
                )}
                {!showArchived && (
                  <div className="mt-3 flex flex-wrap gap-2">
                    <button
                      type="button"
                      aria-label={`Marcar hoy ${h.name}`}
                      onClick={() =>
                        checkMutation.mutate({ id: h.id, day: todayString(), done: !h.done_today })
                      }
                      className={
                        h.done_today
                          ? "rounded-lg bg-streak px-3 py-1 text-xs font-bold text-ink-950"
                          : "rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400"
                      }
                    >
                      {h.done_today ? "Hecho hoy ✓" : "Marcar hoy"}
                    </button>
                    {!h.done_yesterday && (
                      <button
                        type="button"
                        aria-label={`Marcar ayer ${h.name}`}
                        onClick={() =>
                          checkMutation.mutate({ id: h.id, day: yesterdayString(), done: true })
                        }
                        className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400"
                      >
                        Marcar ayer
                      </button>
                    )}
                    <button
                      type="button"
                      aria-label={`Archivar ${h.name}`}
                      onClick={() => archiveMutation.mutate(h.id)}
                      className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400"
                    >
                      Archivar
                    </button>
                    <button
                      type="button"
                      aria-label={`Borrar ${h.name}`}
                      onClick={() => deleteMutation.mutate(h.id)}
                      className="rounded-lg border border-ink-700 px-3 py-1 text-xs text-sand-400 hover:text-red-400"
                    >
                      Borrar
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-sand-400">
            {showArchived ? "No hay hábitos archivados." : "Aún no hay hábitos."}
          </p>
        )}
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Agregar el enlace en el home**

En `web/src/routes/index.tsx`, después del bloque `<Link to="/entrenamiento">…</Link>` agregar:

```tsx
      <Link
        to="/disciplina"
        className="mt-4 ml-2 inline-block rounded-lg border border-ink-700 px-4 py-2 text-sm font-bold text-sand-400"
      >
        Disciplina
      </Link>
```

- [ ] **Step 3: Escribir el test de la página**

Crear `web/src/routes/disciplina.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";

vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "a@b.com", name: "Ana" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { Route as DisciplinaRoute } from "./disciplina";

function fetchMock() {
  return vi.fn((url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ id: "h9" }), { status: 201 }));
    }
    if (opts?.method === "DELETE") {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    // GET /habits
    return Promise.resolve(
      new Response(
        JSON.stringify([
          {
            id: "h1",
            name: "Leer 20 min",
            target_days: 21,
            current_streak: 5,
            best_streak: 8,
            done_today: false,
            done_yesterday: true,
            archived_at: null,
            created_at: "",
          },
        ]),
        { status: 200 }
      )
    );
  });
}

function renderPage() {
  const rootRoute = createRootRoute();
  const route = createRoute({
    getParentRoute: () => rootRoute,
    path: "/disciplina",
    component: DisciplinaRoute.options.component,
  });
  const loginRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    component: () => <div>login</div>,
  });
  const homeRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => <div>home</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([route, loginRoute, homeRoute]),
    history: createMemoryHistory({ initialEntries: ["/disciplina"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("DisciplinaPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock()));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el hábito con su racha", async () => {
    renderPage();
    expect(await screen.findByText("Leer 20 min")).toBeInTheDocument();
    expect(screen.getByText("🔥 5 días")).toBeInTheDocument();
  });

  it("marcar hoy dispara un POST a /check con done:true", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Marcar hoy Leer 20 min" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const post = calls.find(
        ([url, opts]) =>
          (url as string).startsWith("/api/v1/habits/h1/check") && opts?.method === "POST"
      );
      expect(post).toBeTruthy();
      const body = JSON.parse(post![1].body as string);
      expect(body.done).toBe(true);
    });
  });

  it("crear hábito dispara un POST a /habits", async () => {
    renderPage();
    await userEvent.type(await screen.findByLabelText("Nombre del hábito"), "Meditar");
    await userEvent.click(screen.getByRole("button", { name: "Crear" }));
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const post = calls.find(
        ([url, opts]) =>
          (url as string).startsWith("/api/v1/habits?today=") && opts?.method === "POST"
      );
      expect(post).toBeTruthy();
      const body = JSON.parse(post![1].body as string);
      expect(body.name).toBe("Meditar");
    });
  });

  it("archivar dispara un POST a /archive", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Archivar Leer 20 min" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const post = calls.some(
        ([url, opts]) =>
          (url as string).startsWith("/api/v1/habits/h1/archive") && opts?.method === "POST"
      );
      expect(post).toBe(true);
    });
  });
});
```

- [ ] **Step 4: Correr los tests del frontend y verificar que pasan**

Run:
```bash
cd web && npx vitest run src/routes/disciplina.test.tsx src/lib/habits.test.ts
```
Expected: PASS (4 tests de página + 7 de lib).

- [ ] **Step 5: Verificar el build completo del frontend**

Run:
```bash
cd web && npx vite build && npm run build
```
Expected: `npx vite build` regenera `routeTree.gen.ts` (ahora con `/disciplina`); `npm run build` (`tsc -b && vite build`) compila sin errores de tipos.

- [ ] **Step 6: Correr toda la suite de tests del frontend**

Run:
```bash
cd web && npx vitest run
```
Expected: PASS (todas las suites, incluidas las nuevas).

- [ ] **Step 7: Commit**

```bash
git add web/src/routes/disciplina.tsx web/src/routes/disciplina.test.tsx web/src/routes/index.tsx web/src/routeTree.gen.ts
git commit -m "feat(habits): página /disciplina con rachas y enlace en el home"
```

---

### Task 9: Smoke e2e dockerizado

**Files:** (ninguno — validación end-to-end del stack)

> Nota de entorno: los comandos `docker` necesitan `dangerouslyDisableSandbox: true` y exportar el PATH de Docker en la misma línea:
> `export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"`.

- [ ] **Step 1: Levantar el stack y aplicar migraciones**

Run:
```bash
export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin" && cd /Users/gustavo/Desktop/focus-365 && docker compose up -d --build
```
Luego aplicar migraciones (incluye `0005_habits`):
```bash
cd /Users/gustavo/Desktop/focus-365/api && DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go run ./cmd/migrate
```
Expected: contenedores `db`, `api`, `web` arriba; migración `0005_habits` aplicada (o "ya aplicada").

- [ ] **Step 2: Ejecutar el smoke contra la API real**

Run (registra usuario, crea hábito con meta, marca hoy y ayer, verifica racha, lista, archiva, borra):
```bash
cd /Users/gustavo/Desktop/focus-365 && bash -c '
set -e
BASE=http://localhost:8088/api/v1
EMAIL="disc-$(date +%s)@b.com"
TODAY=$(date +%F)
YEST=$(date -v-1d +%F 2>/dev/null || date -d "yesterday" +%F)

TOKEN=$(curl -s -X POST $BASE/auth/register -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"p4ssword\",\"name\":\"Disc\"}" | python3 -c "import sys,json;print(json.load(sys.stdin)[\"access_token\"])")
echo "token ok"

HID=$(curl -s -X POST "$BASE/habits?today=$TODAY" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"name\":\"Leer\",\"target_days\":21}" | python3 -c "import sys,json;print(json.load(sys.stdin)[\"id\"])")
echo "habit $HID"

curl -s -X POST "$BASE/habits/$HID/check?today=$TODAY" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "{\"day\":\"$TODAY\",\"done\":true}" >/dev/null
STREAK=$(curl -s -X POST "$BASE/habits/$HID/check?today=$TODAY" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "{\"day\":\"$YEST\",\"done\":true}" | python3 -c "import sys,json;print(json.load(sys.stdin)[\"current_streak\"])")
echo "racha=$STREAK (esperado 2)"
test "$STREAK" = "2"

# Marcar anteayer debe dar 400.
ANTEAYER=$(date -v-2d +%F 2>/dev/null || date -d "2 days ago" +%F)
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/habits/$HID/check?today=$TODAY" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -d "{\"day\":\"$ANTEAYER\",\"done\":true}")
echo "anteayer code=$CODE (esperado 400)"
test "$CODE" = "400"

# Sin token: 401.
CODE401=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/habits?today=$TODAY")
echo "sin token code=$CODE401 (esperado 401)"
test "$CODE401" = "401"

# Archivar saca de activos.
curl -s -X POST "$BASE/habits/$HID/archive?today=$TODAY" -H "Authorization: Bearer $TOKEN" >/dev/null
ACT=$(curl -s "$BASE/habits?today=$TODAY" -H "Authorization: Bearer $TOKEN" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))")
ARCH=$(curl -s "$BASE/habits?today=$TODAY&archived=true" -H "Authorization: Bearer $TOKEN" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))")
echo "activos=$ACT archivados=$ARCH (esperado 0 y 1)"
test "$ACT" = "0" && test "$ARCH" = "1"

# Borrar (204) y segundo borrado (404).
DEL=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/habits/$HID" -H "Authorization: Bearer $TOKEN")
DEL2=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/habits/$HID" -H "Authorization: Bearer $TOKEN")
echo "delete=$DEL delete2=$DEL2 (esperado 204 y 404)"
test "$DEL" = "204" && test "$DEL2" = "404"
echo "SMOKE OK"
'
```
Expected: termina con `SMOKE OK`; todas las aserciones (`test ...`) pasan.

- [ ] **Step 3: Verificación final de suites (`make check` + frontend)**

Run:
```bash
cd /Users/gustavo/Desktop/focus-365/api && make check
```
Expected: vet limpio + todos los paquetes `ok`.

Run:
```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npx vite build && npm run build
```
Expected: tests verdes y build sin errores.

- [ ] **Step 4: Sin cambios de código → no hay commit**

Este task es solo validación. Si algún paso revela un bug, corregirlo con un commit dedicado (`fix(habits): …`) y volver a correr el smoke.

---

## Self-Review (cobertura del spec)

- **§2 Modelo de datos** → Task 1 (tablas + índices, único parcial, cascade). ✔
- **§3 Cálculo de rachas** (`computeStreaks`, ancla hoy/ayer, récord) → Task 4 + casos en `streaks_test.go`. ✔
- **§3 Zona horaria** (`?today=` del cliente) → `parseTodayParam` en Task 5; lib en Task 7. ✔
- **§4 API** (5 endpoints, idempotencia create, toggle check, ventana hoy/ayer, archive, delete) → Tasks 2, 5; cubiertos por Tasks 3, 6. ✔
- **§5 Forma JSON** (`Habit`, `HabitInput`, `CheckInput`) → Task 4 (`types.go`) + `habitReq`/`checkReq` en Task 5. ✔
- **§6 Estructura de código** (migración, queries, types/service/handler, httpx labels, server mount, lib, página, home link) → Tasks 1–8. ✔
- **§7 Manejo de errores** (400/401/404/500) → handlers en Task 5; verificados en Task 6 + smoke en Task 9. ✔
- **§8 Testing** (store, computeStreaks tabla, handler, lib, página) → Tasks 3, 4, 6, 7, 8. ✔
- **§9 Criterios de aceptación** (1–8) → cubiertos; el #8 (make check + frontend + smoke dockerizado) es Task 9. ✔

Consistencia de tipos verificada: `Habit`/`HabitInput` (Go y TS), `computeStreaks(days, today)` firma única, `CreateHabitParams.TargetDays *int32`, `ListLogsByHabitIDs([]uuid.UUID)`, rutas `/habits` montadas en `/` (patrón confirmado en `checkin`).
