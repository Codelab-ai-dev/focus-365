# Plan 4 — Entrenamiento Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Añadir el módulo de Entrenamiento a Focus 365: catálogo de ejercicios por usuario, sesiones de gym con series (reps + peso en gramos) y un historial, todo bajo `/api/v1/training` y una página `/entrenamiento`.

**Architecture:** Mismo patrón que Finanzas: migración goose → queries sqlc → paquete de dominio `internal/training` (service con transacción + handler chi) montado bajo `RequireAuth` → lib + página React con TanStack Query. El servicio de training, a diferencia de finance, recibe el `*pgxpool.Pool` para abrir la transacción que crea sesión + series de forma atómica.

**Tech Stack:** Go 1.23 (chi v5, pgx/v5 + pgxpool, sqlc v1.31.1, goose, validator/v10, uuid), PostgreSQL 16, React 18 + Vite + TanStack Router/Query + Vitest. Todo dockerizado; comentarios y commits en español.

**Convenciones del repo (no olvidar):**
- Todos los comandos `go` se corren desde `api/` y con `GOTOOLCHAIN=local`. Nunca editar `go.mod`/`go.sum`.
- Tests de backend con `make check` / `make test` (encapsulan `-p 1`). Requieren Postgres de pruebas en `:5544` (`docker compose up -d db`).
- `TEST_DATABASE_URL` y `DATABASE_URL` = `postgres://focus:changeme@localhost:5544/focus365?sslmode=disable`.
- `sqlc generate` desde `api/`; los archivos generados en `internal/store/` SÍ se commitean.
- Peso en **gramos** (entero). El frontend convierte kg↔gramos.
- Spec de referencia: `docs/superpowers/specs/2026-06-11-plan-4-entrenamiento-design.md`.

---

### Task 1: Migración de las tablas de entrenamiento

**Files:**
- Create: `api/db/migrations/0004_training.sql`

- [ ] **Step 1: Escribir la migración**

Crear `api/db/migrations/0004_training.sql`:

```sql
-- +goose Up
CREATE TABLE exercises (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Único por usuario sin distinguir mayúsculas: "Sentadilla" == "sentadilla".
CREATE UNIQUE INDEX uq_exercises_user_name
    ON exercises (user_id, lower(name));

CREATE TABLE workouts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date       DATE NOT NULL,
    type       TEXT NOT NULL DEFAULT '',   -- libre: "Fuerza", "Pierna"…
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_workouts_user_date
    ON workouts (user_id, date DESC, created_at DESC);

CREATE TABLE workout_sets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workout_id   UUID NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    exercise_id  UUID NOT NULL REFERENCES exercises(id),
    position     INT NOT NULL,            -- orden de la serie dentro de la sesión
    reps         INT,                     -- opcional
    weight_grams INT,                     -- opcional; peso en gramos (80kg = 80000)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_workout_sets_workout ON workout_sets (workout_id, position);
CREATE INDEX idx_workout_sets_exercise ON workout_sets (exercise_id);

-- +goose Down
DROP TABLE workout_sets;
DROP TABLE workouts;
DROP TABLE exercises;
```

- [ ] **Step 2: Levantar la DB de pruebas y aplicar la migración**

```bash
docker compose up -d db
cd api && DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go run ./cmd/migrate
```

Esperado: log de goose aplicando `0004_training` (o "successfully migrated ... to version 4").

- [ ] **Step 3: Verificar que las tablas existen**

```bash
docker compose exec db psql -U focus -d focus365 -c '\dt'
```

Esperado: aparecen `exercises`, `workouts`, `workout_sets` además de `users`, `check_ins`, `transactions`, `goose_db_version`.

- [ ] **Step 4: Commit**

```bash
git add api/db/migrations/0004_training.sql
git commit -m "feat(entrenamiento): migración de exercises, workouts y workout_sets"
```

---

### Task 2: Queries sqlc del módulo de entrenamiento

**Files:**
- Create: `api/db/queries/training.sql`
- Modify (generado): `api/internal/store/*` (sqlc)

- [ ] **Step 1: Escribir las queries**

Crear `api/db/queries/training.sql`:

```sql
-- name: ListExercises :many
SELECT * FROM exercises
WHERE user_id = $1
ORDER BY name ASC;

-- name: UpsertExercise :one
-- Crea el ejercicio o, si ya existe (mismo user + lower(name)), devuelve el actual.
INSERT INTO exercises (user_id, name)
VALUES ($1, $2)
ON CONFLICT (user_id, lower(name)) DO UPDATE SET name = exercises.name
RETURNING *;

-- name: CreateWorkout :one
INSERT INTO workouts (user_id, date, type, note)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateWorkoutSet :one
INSERT INTO workout_sets (workout_id, exercise_id, position, reps, weight_grams)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListWorkouts :many
SELECT * FROM workouts
WHERE user_id = sqlc.arg('user_id')
  AND (sqlc.narg('from')::date IS NULL OR date >= sqlc.narg('from'))
  AND (sqlc.narg('to')::date   IS NULL OR date <= sqlc.narg('to'))
ORDER BY date DESC, created_at DESC;

-- name: GetWorkout :one
SELECT * FROM workouts
WHERE id = $1 AND user_id = $2;

-- name: ListSetsByWorkout :many
SELECT ws.position, ws.reps, ws.weight_grams, e.name AS exercise_name
FROM workout_sets ws
JOIN exercises e ON e.id = ws.exercise_id
WHERE ws.workout_id = $1
ORDER BY ws.position ASC;

-- name: ListSetsByWorkoutIDs :many
SELECT ws.workout_id, ws.position, ws.reps, ws.weight_grams, e.name AS exercise_name
FROM workout_sets ws
JOIN exercises e ON e.id = ws.exercise_id
WHERE ws.workout_id = ANY(sqlc.arg('workout_ids')::uuid[])
ORDER BY ws.position ASC;

-- name: DeleteWorkout :execrows
DELETE FROM workouts
WHERE id = $1 AND user_id = $2;
```

- [ ] **Step 2: Generar el código sqlc**

```bash
cd api && sqlc generate
```

Esperado: sin errores; aparecen/actualizan archivos en `internal/store/` (p. ej. `training.sql.go`, `models.go`). Verificar que `Reps` y `WeightGrams` se generan como `*int32` (nullable) y `Position` como `int32`.

- [ ] **Step 3: Compilar para confirmar que el paquete store está bien**

```bash
cd api && GOTOOLCHAIN=local go build ./internal/store/
```

Esperado: compila sin errores.

- [ ] **Step 4: Commit**

```bash
git add api/db/queries/training.sql api/internal/store/
git commit -m "feat(entrenamiento): queries sqlc para catálogo, sesiones y series"
```

---

### Task 3: Tests de store del entrenamiento

**Files:**
- Create: `api/internal/store/training_test.go`

- [ ] **Step 1: Escribir el test de store (debe fallar al compilar primero si las queries no estuvieran)**

Crear `api/internal/store/training_test.go`:

```go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func ptrInt32(v int32) *int32 { return &v }

func TestTrainingStore(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "gym@b.com", PasswordHash: "h", Name: "Gus",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// UpsertExercise es idempotente por (user_id, lower(name)).
	ex1, err := q.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: user.ID, Name: "Sentadilla"})
	if err != nil {
		t.Fatalf("UpsertExercise: %v", err)
	}
	ex2, err := q.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: user.ID, Name: "sentadilla"})
	if err != nil {
		t.Fatalf("UpsertExercise dup: %v", err)
	}
	if ex1.ID != ex2.ID {
		t.Errorf("el upsert duplicó el catálogo: %s vs %s", ex1.ID, ex2.ID)
	}

	list, err := q.ListExercises(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListExercises: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(catálogo) = %d, want 1", len(list))
	}

	// Crear una sesión con dos series.
	day := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	w, err := q.CreateWorkout(ctx, store.CreateWorkoutParams{
		UserID: user.ID, Date: day, Type: "Fuerza", Note: "buen pump",
	})
	if err != nil {
		t.Fatalf("CreateWorkout: %v", err)
	}
	if _, err := q.CreateWorkoutSet(ctx, store.CreateWorkoutSetParams{
		WorkoutID: w.ID, ExerciseID: ex1.ID, Position: 0, Reps: ptrInt32(8), WeightGrams: ptrInt32(80000),
	}); err != nil {
		t.Fatalf("CreateWorkoutSet 0: %v", err)
	}
	if _, err := q.CreateWorkoutSet(ctx, store.CreateWorkoutSetParams{
		WorkoutID: w.ID, ExerciseID: ex1.ID, Position: 1, Reps: ptrInt32(6), WeightGrams: ptrInt32(80000),
	}); err != nil {
		t.Fatalf("CreateWorkoutSet 1: %v", err)
	}

	// ListSetsByWorkout: ordena por position y resuelve el nombre.
	sets, err := q.ListSetsByWorkout(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListSetsByWorkout: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("len(sets) = %d, want 2", len(sets))
	}
	if sets[0].Position != 0 || sets[1].Position != 1 {
		t.Errorf("series fuera de orden: %d, %d", sets[0].Position, sets[1].Position)
	}
	if sets[0].ExerciseName != "Sentadilla" {
		t.Errorf("nombre = %q, want Sentadilla", sets[0].ExerciseName)
	}

	// ListWorkouts filtra por rango.
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	ws, err := q.ListWorkouts(ctx, store.ListWorkoutsParams{UserID: user.ID, From: &from, To: &to})
	if err != nil {
		t.Fatalf("ListWorkouts: %v", err)
	}
	if len(ws) != 1 {
		t.Fatalf("len(workouts) = %d, want 1", len(ws))
	}

	// DeleteWorkout arrastra las series (cascade) y respeta dueño.
	n, err := q.DeleteWorkout(ctx, store.DeleteWorkoutParams{ID: w.ID, UserID: user.ID})
	if err != nil {
		t.Fatalf("DeleteWorkout: %v", err)
	}
	if n != 1 {
		t.Fatalf("DeleteWorkout afectó %d filas, want 1", n)
	}
	gone, err := q.ListSetsByWorkout(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListSetsByWorkout post-delete: %v", err)
	}
	if len(gone) != 0 {
		t.Errorf("las series no cayeron en cascada: quedan %d", len(gone))
	}
	// El catálogo no se borra al borrar la sesión.
	cat, err := q.ListExercises(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListExercises post-delete: %v", err)
	}
	if len(cat) != 1 {
		t.Errorf("el catálogo cambió al borrar la sesión: %d", len(cat))
	}
}
```

- [ ] **Step 2: Correr el test de store**

```bash
cd api && TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go test ./internal/store/ -run TestTrainingStore -v
```

Esperado: PASS. (Si falla por nombres de campos generados, ajustar a lo que produjo sqlc — p. ej. `From`/`To` en `ListWorkoutsParams`.)

- [ ] **Step 3: Commit**

```bash
git add api/internal/store/training_test.go
git commit -m "test(entrenamiento): store de catálogo, sesiones, series y cascade"
```

---

### Task 4: Dominio de training — tipos y servicio

**Files:**
- Create: `api/internal/training/types.go`
- Create: `api/internal/training/service.go`

- [ ] **Step 1: Escribir los tipos del dominio**

Crear `api/internal/training/types.go`:

```go
package training

import "time"

const dateLayout = "2006-01-02"

// Exercise es un ejercicio del catálogo del usuario.
type Exercise struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// SetInput es una serie recibida al capturar una sesión.
type SetInput struct {
	Exercise    string
	Reps        *int32
	WeightGrams *int32
}

// WorkoutInput son los datos de dominio para crear una sesión completa.
type WorkoutInput struct {
	Date time.Time
	Type string
	Note string
	Sets []SetInput
}

// WorkoutSet es la vista de una serie (con el nombre del ejercicio resuelto).
type WorkoutSet struct {
	Exercise    string `json:"exercise"`
	Reps        *int32 `json:"reps"`
	WeightGrams *int32 `json:"weight_grams"`
}

// Workout es la vista de dominio de una sesión que se serializa a JSON.
// date va como YYYY-MM-DD.
type Workout struct {
	ID        string       `json:"id"`
	Date      string       `json:"date"`
	Type      string       `json:"type"`
	Note      string       `json:"note"`
	Sets      []WorkoutSet `json:"sets"`
	CreatedAt time.Time    `json:"created_at"`
}
```

- [ ] **Step 2: Escribir el servicio (con transacción para crear la sesión)**

Crear `api/internal/training/service.go`:

```go
package training

import (
	"context"
	"errors"
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

// NewService recibe el pool además de las queries: CreateWorkout abre una
// transacción para insertar la sesión y todas sus series de forma atómica.
func NewService(q *store.Queries, pool *pgxpool.Pool) *Service {
	return &Service{q: q, pool: pool}
}

func (s *Service) ListExercises(ctx context.Context, userID uuid.UUID) ([]Exercise, error) {
	rows, err := s.q.ListExercises(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Exercise, 0, len(rows))
	for _, r := range rows {
		out = append(out, Exercise{ID: r.ID.String(), Name: r.Name, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Service) CreateExercise(ctx context.Context, userID uuid.UUID, name string) (*Exercise, error) {
	row, err := s.q.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: userID, Name: strings.TrimSpace(name)})
	if err != nil {
		return nil, err
	}
	return &Exercise{ID: row.ID.String(), Name: row.Name, CreatedAt: row.CreatedAt}, nil
}

// CreateWorkout inserta la sesión y sus series en una transacción. Por cada
// serie resuelve (o crea) el ejercicio del catálogo por nombre. Si algo falla,
// hace rollback y no deja sesiones a medias.
func (s *Service) CreateWorkout(ctx context.Context, userID uuid.UUID, in WorkoutInput) (*Workout, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	w, err := qtx.CreateWorkout(ctx, store.CreateWorkoutParams{
		UserID: userID, Date: in.Date, Type: in.Type, Note: in.Note,
	})
	if err != nil {
		return nil, err
	}

	// Cache de ejercicios resueltos por nombre (lower) para no repetir upserts.
	cache := make(map[string]uuid.UUID)
	for i, set := range in.Sets {
		name := strings.TrimSpace(set.Exercise)
		key := strings.ToLower(name)
		exID, ok := cache[key]
		if !ok {
			ex, err := qtx.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: userID, Name: name})
			if err != nil {
				return nil, err
			}
			exID = ex.ID
			cache[key] = exID
		}
		if _, err := qtx.CreateWorkoutSet(ctx, store.CreateWorkoutSetParams{
			WorkoutID: w.ID, ExerciseID: exID, Position: int32(i), Reps: set.Reps, WeightGrams: set.WeightGrams,
		}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetWorkout(ctx, userID, w.ID)
}

// GetWorkout devuelve la sesión si pertenece al usuario; (nil, nil) si no existe.
func (s *Service) GetWorkout(ctx context.Context, userID, id uuid.UUID) (*Workout, error) {
	w, err := s.q.GetWorkout(ctx, store.GetWorkoutParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rows, err := s.q.ListSetsByWorkout(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	sets := make([]WorkoutSet, 0, len(rows))
	for _, r := range rows {
		sets = append(sets, WorkoutSet{Exercise: r.ExerciseName, Reps: r.Reps, WeightGrams: r.WeightGrams})
	}
	return workoutView(w, sets), nil
}

// ListWorkouts trae el historial por rango (from/to opcionales) con sus series,
// evitando N+1 con una sola consulta de series para todas las sesiones.
func (s *Service) ListWorkouts(ctx context.Context, userID uuid.UUID, from, to *time.Time) ([]Workout, error) {
	rows, err := s.q.ListWorkouts(ctx, store.ListWorkoutsParams{UserID: userID, From: from, To: to})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Workout{}, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, w := range rows {
		ids[i] = w.ID
	}
	sets, err := s.q.ListSetsByWorkoutIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byWorkout := make(map[uuid.UUID][]WorkoutSet)
	for _, st := range sets {
		byWorkout[st.WorkoutID] = append(byWorkout[st.WorkoutID], WorkoutSet{
			Exercise: st.ExerciseName, Reps: st.Reps, WeightGrams: st.WeightGrams,
		})
	}
	out := make([]Workout, 0, len(rows))
	for _, w := range rows {
		out = append(out, *workoutView(w, byWorkout[w.ID]))
	}
	return out, nil
}

// DeleteWorkout borra la sesión si pertenece al usuario; devuelve si borró algo.
func (s *Service) DeleteWorkout(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	n, err := s.q.DeleteWorkout(ctx, store.DeleteWorkoutParams{ID: id, UserID: userID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func workoutView(w store.Workout, sets []WorkoutSet) *Workout {
	if sets == nil {
		sets = []WorkoutSet{}
	}
	return &Workout{
		ID:        w.ID.String(),
		Date:      w.Date.Format(dateLayout),
		Type:      w.Type,
		Note:      w.Note,
		Sets:      sets,
		CreatedAt: w.CreatedAt,
	}
}
```

- [ ] **Step 3: Compilar el paquete**

```bash
cd api && GOTOOLCHAIN=local go build ./internal/training/
```

Esperado: compila. (Si `ListWorkoutsParams` usa otros nombres que `From`/`To`, ajustar; si `ListSetsByWorkoutIDs` recibe un nombre distinto a `ids`, ajustar la llamada.)

- [ ] **Step 4: Commit**

```bash
git add api/internal/training/types.go api/internal/training/service.go
git commit -m "feat(entrenamiento): dominio y servicio con transacción de sesión"
```

---

### Task 5: Handlers HTTP, labels de validación y montaje

**Files:**
- Create: `api/internal/training/handler.go`
- Modify: `api/internal/httpx/httpx.go` (añadir labels)
- Modify: `api/internal/server/server.go` (montar rutas)

- [ ] **Step 1: Escribir los handlers**

Crear `api/internal/training/handler.go`:

```go
package training

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type exerciseReq struct {
	Name string `json:"name" validate:"required"`
}

type setReq struct {
	Exercise    string `json:"exercise" validate:"required"`
	Reps        *int32 `json:"reps" validate:"omitempty,min=0"`
	WeightGrams *int32 `json:"weight_grams" validate:"omitempty,min=0"`
}

type workoutReq struct {
	Date string   `json:"date" validate:"required"`
	Type string   `json:"type"`
	Note string   `json:"note"`
	Sets []setReq `json:"sets" validate:"required,min=1,dive"`
}

func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/exercises", handleListExercises(svc))
	r.Post("/exercises", handleCreateExercise(svc))
	r.Post("/workouts", handleCreateWorkout(svc))
	r.Get("/workouts", handleListWorkouts(svc))
	r.Get("/workouts/{id}", handleGetWorkout(svc))
	r.Delete("/workouts/{id}", handleDeleteWorkout(svc))
	return r
}

func handleListExercises(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		list, err := svc.ListExercises(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
	}
}

func handleCreateExercise(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req exerciseReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		ex, err := svc.CreateExercise(r.Context(), userID, req.Name)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, ex)
	}
}

func handleCreateWorkout(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req workoutReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		date, err := time.Parse(dateLayout, req.Date)
		if err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "la fecha no tiene un formato válido (YYYY-MM-DD)")
			return
		}
		sets := make([]SetInput, 0, len(req.Sets))
		for _, s := range req.Sets {
			sets = append(sets, SetInput{Exercise: s.Exercise, Reps: s.Reps, WeightGrams: s.WeightGrams})
		}
		out, err := svc.CreateWorkout(r.Context(), userID, WorkoutInput{
			Date: date, Type: req.Type, Note: req.Note, Sets: sets,
		})
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, out)
	}
}

func handleListWorkouts(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		from := parseDateParam(r, "from")
		to := parseDateParam(r, "to")
		list, err := svc.ListWorkouts(r.Context(), userID, from, to)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, list)
	}
}

func handleGetWorkout(svc *Service) http.HandlerFunc {
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
		out, err := svc.GetWorkout(r.Context(), userID, id)
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

func handleDeleteWorkout(svc *Service) http.HandlerFunc {
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
		deleted, err := svc.DeleteWorkout(r.Context(), userID, id)
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

// parseDateParam lee ?name=YYYY-MM-DD y devuelve *time.Time (nil si falta o es inválido).
func parseDateParam(r *http.Request, name string) *time.Time {
	s := r.URL.Query().Get(name)
	if s == "" {
		return nil
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return nil
	}
	return &t
}
```

- [ ] **Step 2: Añadir labels de validación en español**

En `api/internal/httpx/httpx.go`, dentro de `fieldLabel`, añadir casos antes del `default:` (después del case `"Remark"`):

```go
	case "Exercise":
		return "el ejercicio"
	case "Sets":
		return "las series"
	case "Reps":
		return "las repeticiones"
	case "WeightGrams":
		return "el peso"
```

(Los campos `Name` y `Date` ya tienen label.)

- [ ] **Step 3: Montar las rutas en el servidor**

En `api/internal/server/server.go`:

1. Añadir el import `"github.com/focus365/api/internal/training"` junto a los otros imports de módulos.
2. Crear el servicio tras `financeSvc := finance.NewService(q)`:

```go
	trainingSvc := training.NewService(q, d.Pool)
```

3. Dentro del grupo con `auth.RequireAuth(tm)`, junto a los otros `r.Mount`, añadir:

```go
			r.Mount("/training", training.Routes(trainingSvc))
```

- [ ] **Step 4: Compilar todo**

```bash
cd api && GOTOOLCHAIN=local go build ./...
```

Esperado: compila sin errores.

- [ ] **Step 5: Commit**

```bash
git add api/internal/training/handler.go api/internal/httpx/httpx.go api/internal/server/server.go
git commit -m "feat(entrenamiento): handlers HTTP, labels de validación y montaje de rutas"
```

---

### Task 6: Tests de handlers del entrenamiento

**Files:**
- Create: `api/internal/training/handler_test.go`

- [ ] **Step 1: Escribir los tests de handler**

Crear `api/internal/training/handler_test.go`:

```go
package training_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/focus365/api/internal/training"
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
		r.Mount("/training", training.Routes(training.NewService(q, pool)))
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

func TestCreateWorkoutAndList(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "a@b.com")

	rec := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-11", "type": "Fuerza", "note": "buen pump",
		"sets": []map[string]any{
			{"exercise": "Sentadilla", "reps": 8, "weight_grams": 80000},
			{"exercise": "Sentadilla", "reps": 6, "weight_grams": 80000},
			{"exercise": "Press banca", "reps": 10, "weight_grams": 60000},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST workout code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var w map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &w)
	sets := w["sets"].([]any)
	if len(sets) != 3 {
		t.Fatalf("sets = %d, want 3", len(sets))
	}
	first := sets[0].(map[string]any)
	if first["exercise"] != "Sentadilla" {
		t.Errorf("exercise[0] = %v, want Sentadilla", first["exercise"])
	}

	// Crear ejercicios on-the-fly: el catálogo ahora tiene 2.
	recE := do(t, e.h, http.MethodGet, "/training/exercises", tok, nil)
	var cat []map[string]any
	_ = json.Unmarshal(recE.Body.Bytes(), &cat)
	if len(cat) != 2 {
		t.Errorf("catálogo = %d, want 2", len(cat))
	}

	// Historial por rango.
	recL := do(t, e.h, http.MethodGet, "/training/workouts?from=2026-06-01&to=2026-06-30", tok, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("historial = %d, want 1", len(list))
	}
}

func TestExerciseIdempotente(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "ex@b.com")
	_ = do(t, e.h, http.MethodPost, "/training/exercises", tok, map[string]any{"name": "Peso muerto"})
	_ = do(t, e.h, http.MethodPost, "/training/exercises", tok, map[string]any{"name": "peso muerto"})
	recE := do(t, e.h, http.MethodGet, "/training/exercises", tok, nil)
	var cat []map[string]any
	_ = json.Unmarshal(recE.Body.Bytes(), &cat)
	if len(cat) != 1 {
		t.Errorf("catálogo = %d, want 1 (no duplica por capitalización)", len(cat))
	}
}

func TestGetAndDeleteWorkout(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "d@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-11",
		"sets": []map[string]any{{"exercise": "Dominadas", "reps": 10}},
	})
	var w map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &w)
	id := w["id"].(string)

	recG := do(t, e.h, http.MethodGet, "/training/workouts/"+id, tok, nil)
	if recG.Code != http.StatusOK {
		t.Fatalf("GET code = %d, want 200", recG.Code)
	}

	recD := do(t, e.h, http.MethodDelete, "/training/workouts/"+id, tok, nil)
	if recD.Code != http.StatusNoContent {
		t.Fatalf("DELETE code = %d, want 204", recD.Code)
	}
	recG2 := do(t, e.h, http.MethodGet, "/training/workouts/"+id, tok, nil)
	if recG2.Code != http.StatusNotFound {
		t.Errorf("GET tras borrar code = %d, want 404", recG2.Code)
	}
}

func TestValidationWorkout(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "v@b.com")
	// sets vacío → 400.
	rec := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-11", "sets": []map[string]any{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("sets vacío code = %d, want 400", rec.Code)
	}
	// set sin exercise → 400.
	rec2 := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-11", "sets": []map[string]any{{"reps": 8}},
	})
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("set sin ejercicio code = %d, want 400", rec2.Code)
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/training/exercises", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	tokA := e.token(t, "userA@b.com")
	tokB := e.token(t, "userB@b.com")

	recA := do(t, e.h, http.MethodPost, "/training/workouts", tokA, map[string]any{
		"date": "2026-06-11", "sets": []map[string]any{{"exercise": "Curl", "reps": 12}},
	})
	var wA map[string]any
	_ = json.Unmarshal(recA.Body.Bytes(), &wA)
	idA := wA["id"].(string)

	// B no ve el historial de A.
	recL := do(t, e.h, http.MethodGet, "/training/workouts", tokB, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("B ve %d sesiones de A; debería ver 0", len(list))
	}
	// B no puede ver ni borrar la sesión de A.
	if rec := do(t, e.h, http.MethodGet, "/training/workouts/"+idA, tokB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("B vio la sesión de A: code = %d, want 404", rec.Code)
	}
	if rec := do(t, e.h, http.MethodDelete, "/training/workouts/"+idA, tokB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("B borró la sesión de A: code = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Correr los tests del paquete training**

```bash
cd api && TEST_DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go test ./internal/training/ -v
```

Esperado: PASS en todos los tests.

- [ ] **Step 3: Correr la verificación completa del backend**

```bash
cd api && make check
```

Esperado: `vet` limpio y todos los paquetes `ok` (con `-p 1`).

- [ ] **Step 4: Commit**

```bash
git add api/internal/training/handler_test.go
git commit -m "test(entrenamiento): handlers de catálogo, sesión, historial, aislamiento y validación"
```

---

### Task 7: Lib del frontend (training.ts) + tests

**Files:**
- Create: `web/src/lib/training.ts`
- Create: `web/src/lib/training.test.ts`

- [ ] **Step 1: Escribir la lib**

Crear `web/src/lib/training.ts`:

```ts
import { apiFetch } from "./api";

export type Exercise = {
  id: string;
  name: string;
  created_at: string;
};

export type WorkoutSet = {
  exercise: string;
  reps: number | null;
  weight_grams: number | null;
};

export type Workout = {
  id: string;
  date: string; // YYYY-MM-DD
  type: string;
  note: string;
  sets: WorkoutSet[];
  created_at: string;
};

export type SetInput = {
  exercise: string;
  reps: number | null;
  weight_grams: number | null;
};

export type WorkoutInput = {
  date: string;
  type: string;
  note: string;
  sets: SetInput[];
};

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

// El peso se guarda en gramos (entero) para evitar líos de coma flotante.
export function kgToGrams(kg: number): number {
  return Math.round(kg * 1000);
}

export function gramsToKg(grams: number): number {
  return grams / 1000;
}

export function listExercises(): Promise<Exercise[]> {
  return apiFetch<Exercise[]>("/api/v1/training/exercises");
}

export function createExercise(name: string): Promise<Exercise> {
  return apiFetch<Exercise>("/api/v1/training/exercises", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export function createWorkout(input: WorkoutInput): Promise<Workout> {
  return apiFetch<Workout>("/api/v1/training/workouts", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function listWorkouts(from?: string, to?: string): Promise<Workout[]> {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  const qs = params.toString();
  return apiFetch<Workout[]>(`/api/v1/training/workouts${qs ? `?${qs}` : ""}`);
}

export function getWorkout(id: string): Promise<Workout> {
  return apiFetch<Workout>(`/api/v1/training/workouts/${id}`);
}

export function removeWorkout(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/training/workouts/${id}`, { method: "DELETE" });
}
```

- [ ] **Step 2: Escribir el test de la lib**

Crear `web/src/lib/training.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  listExercises,
  createExercise,
  createWorkout,
  listWorkouts,
  removeWorkout,
  kgToGrams,
  gramsToKg,
} from "./training";
import { setAccessToken } from "./api";

function okJson(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), { status }));
}

describe("lib/training", () => {
  beforeEach(() => setAccessToken(null));
  afterEach(() => vi.restoreAllMocks());

  it("convierte kg↔gramos (incl. 0.5 kg)", () => {
    expect(kgToGrams(80)).toBe(80000);
    expect(kgToGrams(0.5)).toBe(500);
    expect(gramsToKg(80000)).toBe(80);
    expect(gramsToKg(500)).toBe(0.5);
  });

  it("listExercises pega a GET /exercises", async () => {
    const fetchMock = vi.fn(() => okJson([]));
    vi.stubGlobal("fetch", fetchMock);
    await listExercises();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/training/exercises",
      expect.objectContaining({ })
    );
  });

  it("createExercise hace POST con el nombre", async () => {
    const fetchMock = vi.fn(() => okJson({ id: "e1", name: "Sentadilla" }, 201));
    vi.stubGlobal("fetch", fetchMock);
    await createExercise("Sentadilla");
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/training/exercises");
    expect(opts?.method).toBe("POST");
    expect(JSON.parse(opts?.body as string)).toEqual({ name: "Sentadilla" });
  });

  it("createWorkout hace POST con el body y manda Bearer si hay token", async () => {
    setAccessToken("tok123");
    const fetchMock = vi.fn(() => okJson({ id: "w1" }, 201));
    vi.stubGlobal("fetch", fetchMock);
    await createWorkout({
      date: "2026-06-11",
      type: "Fuerza",
      note: "",
      sets: [{ exercise: "Sentadilla", reps: 8, weight_grams: 80000 }],
    });
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/training/workouts");
    expect(opts?.method).toBe("POST");
    const headers = opts?.headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("Bearer tok123");
  });

  it("listWorkouts arma el querystring de rango", async () => {
    const fetchMock = vi.fn(() => okJson([]));
    vi.stubGlobal("fetch", fetchMock);
    await listWorkouts("2026-06-01", "2026-06-30");
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/training/workouts?from=2026-06-01&to=2026-06-30"
    );
  });

  it("removeWorkout hace DELETE a la sesión", async () => {
    const fetchMock = vi.fn(() => Promise.resolve(new Response(null, { status: 204 })));
    vi.stubGlobal("fetch", fetchMock);
    await removeWorkout("w9");
    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/training/workouts/w9");
    expect(opts?.method).toBe("DELETE");
  });
});
```

- [ ] **Step 3: Correr el test de la lib**

```bash
cd web && npx vitest run src/lib/training.test.ts
```

Esperado: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/training.ts web/src/lib/training.test.ts
git commit -m "feat(entrenamiento): lib del cliente con conversión kg/gramos y tests"
```

---

### Task 8: Página /entrenamiento + tests + enlace en home

**Files:**
- Create: `web/src/routes/entrenamiento.tsx`
- Create: `web/src/routes/entrenamiento.test.tsx`
- Modify: `web/src/routes/index.tsx` (enlace)

- [ ] **Step 1: Escribir la página**

Crear `web/src/routes/entrenamiento.tsx`:

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import {
  listExercises,
  listWorkouts,
  createWorkout,
  removeWorkout,
  kgToGrams,
  gramsToKg,
  todayString,
  type Exercise,
  type Workout,
} from "@/lib/training";

export const Route = createFileRoute("/entrenamiento")({ component: EntrenamientoPage });

type SetRow = { exercise: string; reps: string; weightKg: string };

function emptyRow(): SetRow {
  return { exercise: "", reps: "", weightKg: "" };
}

function EntrenamientoPage() {
  const { user } = useAuth();
  const navigate = useNavigate();
  const qc = useQueryClient();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const exercisesQuery = useQuery({
    queryKey: ["training", "exercises"],
    queryFn: () => listExercises(),
    enabled: !!user,
  });
  const historyQuery = useQuery({
    queryKey: ["training", "workouts"],
    queryFn: () => listWorkouts(),
    enabled: !!user,
  });

  const [date, setDate] = useState(todayString());
  const [type, setType] = useState("");
  const [note, setNote] = useState("");
  const [rows, setRows] = useState<SetRow[]>([emptyRow()]);
  const [error, setError] = useState<string | null>(null);

  function invalidate() {
    qc.invalidateQueries({ queryKey: ["training"] });
  }

  const createMutation = useMutation({
    mutationFn: () =>
      createWorkout({
        date,
        type,
        note,
        sets: rows
          .filter((r) => r.exercise.trim() !== "")
          .map((r) => ({
            exercise: r.exercise.trim(),
            reps: r.reps === "" ? null : Number(r.reps),
            weight_grams: r.weightKg === "" ? null : kgToGrams(Number(r.weightKg)),
          })),
      }),
    onSuccess: () => {
      setError(null);
      setType("");
      setNote("");
      setRows([emptyRow()]);
      invalidate();
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : "Error al guardar"),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => removeWorkout(id),
    onSuccess: invalidate,
  });

  function updateRow(i: number, patch: Partial<SetRow>) {
    setRows((rs) => rs.map((r, idx) => (idx === i ? { ...r, ...patch } : r)));
  }

  if (!user) return null;

  return (
    <div className="mx-auto max-w-xl p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-extrabold">Entrenamiento</h1>
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
          <span className="text-sm text-sand-400">Fecha</span>
          <input
            type="date"
            aria-label="Fecha"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Tipo</span>
          <input
            type="text"
            aria-label="Tipo"
            placeholder="Fuerza, Pierna…"
            value={type}
            onChange={(e) => setType(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        <div className="space-y-3">
          <span className="text-sm text-sand-400">Series</span>
          {rows.map((row, i) => (
            <div key={i} className="flex gap-2">
              <input
                type="text"
                aria-label={`Ejercicio ${i + 1}`}
                list="catalogo-ejercicios"
                placeholder="Ejercicio"
                value={row.exercise}
                onChange={(e) => updateRow(i, { exercise: e.target.value })}
                className="flex-1 rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
              />
              <input
                type="number"
                aria-label={`Reps ${i + 1}`}
                placeholder="Reps"
                min="0"
                value={row.reps}
                onChange={(e) => updateRow(i, { reps: e.target.value })}
                className="w-20 rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
              />
              <input
                type="number"
                aria-label={`Peso ${i + 1}`}
                placeholder="kg"
                min="0"
                step="0.5"
                value={row.weightKg}
                onChange={(e) => updateRow(i, { weightKg: e.target.value })}
                className="w-20 rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
              />
            </div>
          ))}
          <datalist id="catalogo-ejercicios">
            {exercisesQuery.data?.map((ex: Exercise) => (
              <option key={ex.id} value={ex.name} />
            ))}
          </datalist>
          <button
            type="button"
            onClick={() => setRows((rs) => [...rs, emptyRow()])}
            className="text-sm text-amber-brand"
          >
            + Agregar serie
          </button>
        </div>

        <label className="block space-y-1">
          <span className="text-sm text-sand-400">Nota</span>
          <input
            type="text"
            aria-label="Nota"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            className="w-full rounded-lg border border-ink-700 bg-ink-800 px-3 py-2 text-sm outline-none focus:border-amber-brand"
          />
        </label>

        {error && <p className="text-sm text-red-400">{error}</p>}

        <button
          type="submit"
          disabled={createMutation.isPending}
          className="w-full rounded-lg bg-amber-brand px-3 py-2 text-sm font-bold text-ink-950 disabled:opacity-60"
        >
          {createMutation.isPending ? "Guardando…" : "Guardar"}
        </button>
      </form>

      <section className="mt-8">
        <h2 className="text-lg font-bold">Historial</h2>
        {historyQuery.data && historyQuery.data.length > 0 ? (
          <ul className="mt-3 space-y-3">
            {historyQuery.data.map((w: Workout) => (
              <li
                key={w.id}
                className="rounded-xl border border-ink-700 bg-ink-900 p-4 text-sm"
              >
                <div className="flex items-center justify-between">
                  <span className="font-bold">
                    {w.date} {w.type && <span className="text-sand-400">· {w.type}</span>}
                  </span>
                  <button
                    type="button"
                    aria-label={`Borrar sesión ${w.date}`}
                    onClick={() => deleteMutation.mutate(w.id)}
                    className="text-xs text-sand-400 hover:text-red-400"
                  >
                    ✕
                  </button>
                </div>
                <ul className="mt-2 space-y-1 text-sand-400">
                  {w.sets.map((s, i) => (
                    <li key={i}>
                      {s.exercise}
                      {s.reps != null && ` · ${s.reps} reps`}
                      {s.weight_grams != null && ` · ${gramsToKg(s.weight_grams)} kg`}
                    </li>
                  ))}
                </ul>
                {w.note && <p className="mt-2 text-xs text-sand-400">{w.note}</p>}
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-sand-400">Aún no hay sesiones.</p>
        )}
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Escribir el test de la página**

Crear `web/src/routes/entrenamiento.test.tsx`:

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

import { Route as EntrenamientoRoute } from "./entrenamiento";

function fetchMock() {
  return vi.fn((url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ id: "w9" }), { status: 201 }));
    }
    if (opts?.method === "DELETE") {
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url.includes("/exercises")) {
      return Promise.resolve(
        new Response(
          JSON.stringify([{ id: "e1", name: "Sentadilla", created_at: "" }]),
          { status: 200 }
        )
      );
    }
    // GET /workouts
    return Promise.resolve(
      new Response(
        JSON.stringify([
          {
            id: "w1",
            date: "2026-06-11",
            type: "Fuerza",
            note: "",
            sets: [{ exercise: "Sentadilla", reps: 8, weight_grams: 80000 }],
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
    path: "/entrenamiento",
    component: EntrenamientoRoute.options.component,
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
    history: createMemoryHistory({ initialEntries: ["/entrenamiento"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("EntrenamientoPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock()));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el historial de sesiones", async () => {
    renderPage();
    expect(await screen.findByText("Sentadilla")).toBeInTheDocument();
  });

  it("al guardar dispara un POST con el peso en gramos", async () => {
    renderPage();
    await userEvent.type(await screen.findByLabelText("Ejercicio 1"), "Sentadilla");
    await userEvent.type(screen.getByLabelText("Reps 1"), "8");
    await userEvent.type(screen.getByLabelText("Peso 1"), "80");
    await userEvent.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const post = calls.find(
        ([url, opts]) =>
          url === "/api/v1/training/workouts" && opts?.method === "POST"
      );
      expect(post).toBeTruthy();
      const body = JSON.parse(post![1].body as string);
      expect(body.sets[0].weight_grams).toBe(80000);
      expect(body.sets[0].reps).toBe(8);
    });
  });

  it("al borrar una sesión dispara un DELETE", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: "Borrar sesión 2026-06-11" });
    await userEvent.click(btn);
    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const del = calls.some(
        ([url, opts]) =>
          url === "/api/v1/training/workouts/w1" && opts?.method === "DELETE"
      );
      expect(del).toBe(true);
    });
  });
});
```

- [ ] **Step 3: Añadir el enlace en el home**

En `web/src/routes/index.tsx`, tras el `<Link to="/finanzas">…</Link>`, añadir:

```tsx
      <Link
        to="/entrenamiento"
        className="mt-4 ml-2 inline-block rounded-lg border border-ink-700 px-4 py-2 text-sm font-bold text-sand-400"
      >
        Entrenamiento
      </Link>
```

- [ ] **Step 4: Regenerar el route tree y correr los tests del frontend**

```bash
cd web && npx vite build
```

Esperado: `routeTree.gen.ts` se regenera incluyendo `/entrenamiento`; build OK.

```bash
cd web && npx vitest run
```

Esperado: todos los archivos de test PASS (incl. `entrenamiento.test.tsx` y `training.test.ts`).

- [ ] **Step 5: Build completo del frontend**

```bash
cd web && npm run build
```

Esperado: `tsc -b && vite build` sin errores.

- [ ] **Step 6: Commit**

```bash
git add web/src/routes/entrenamiento.tsx web/src/routes/entrenamiento.test.tsx web/src/routes/index.tsx web/src/routeTree.gen.ts
git commit -m "feat(entrenamiento): página /entrenamiento con captura, historial y enlace en home"
```

---

### Task 9: Smoke e2e dockerizado y cierre

**Files:** (ninguno nuevo; solo validación)

- [ ] **Step 1: Levantar todo el stack y aplicar migraciones**

```bash
docker compose up -d --build
cd api && DATABASE_URL=postgres://focus:changeme@localhost:5544/focus365?sslmode=disable GOTOOLCHAIN=local go run ./cmd/migrate
```

Esperado: 3 contenedores arriba; migración en versión 4.

> Nota operativa: si algún comando con docker falla por el credential helper, exportar en la MISMA línea `export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"` y usar `dangerouslyDisableSandbox: true`. Para inspeccionar salidas largas, redirigir a `/tmp/*.log` y leer el archivo (no pipe a `tail`).

- [ ] **Step 2: Registrar usuario y capturar el token**

```bash
curl -s -X POST http://localhost:8088/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"gym@e2e.com","password":"p4ssword","name":"Gym"}' > /tmp/train-reg.json
```

Esperado: HTTP 201 con `access_token`. Extraer el token a una variable de shell (`TOKEN=$(... jq -r .access_token)` si hay jq, o leer el archivo).

- [ ] **Step 3: Crear una sesión con varias series**

```bash
curl -s -o /tmp/train-create.json -w '%{http_code}' -X POST http://localhost:8088/api/v1/training/workouts \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"date":"2026-06-11","type":"Fuerza","note":"e2e","sets":[
        {"exercise":"Sentadilla","reps":8,"weight_grams":80000},
        {"exercise":"sentadilla","reps":6,"weight_grams":80000},
        {"exercise":"Press banca","reps":10,"weight_grams":60000}]}'
```

Esperado: `201`. El cuerpo trae `sets` con 3 elementos y nombres resueltos.

- [ ] **Step 4: Verificar catálogo, historial, detalle y borrado**

```bash
curl -s http://localhost:8088/api/v1/training/exercises -H "Authorization: Bearer $TOKEN"      # 2 ejercicios (no duplica por capitalización)
curl -s 'http://localhost:8088/api/v1/training/workouts?from=2026-06-01&to=2026-06-30' -H "Authorization: Bearer $TOKEN"  # 1 sesión
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8088/api/v1/training/exercises   # 401 sin token
```

Esperado: catálogo con 2 ejercicios; historial con 1 sesión y sus 3 series; `401` sin token. Tomar el `id` de la sesión del paso 3 y borrarla:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE http://localhost:8088/api/v1/training/workouts/<ID> -H "Authorization: Bearer $TOKEN"  # 204
```

Esperado: `204`; un segundo DELETE da `404`.

- [ ] **Step 5: Verificación final completa**

```bash
cd api && make check
cd web && npx vitest run && npm run build
```

Esperado: backend `vet`+`test` verde; frontend tests + build verde.

- [ ] **Step 6: Commit de cierre (si hubo ajustes) o etiqueta de fin**

Si los pasos de validación no cambiaron código, no hay nada que commitear (Task de validación pura). Si hubo ajustes menores:

```bash
git add -A
git commit -m "chore(entrenamiento): ajustes tras smoke e2e del módulo"
```

---

## Self-Review

**Cobertura del spec:**
- §4 modelo de datos → Task 1 (migración con las 3 tablas e índices exactos del spec). ✓
- §5 queries sqlc → Task 2 (las 9 queries listadas). ✓
- §5 API (6 endpoints) → Task 5 handlers + Task 6 tests cubren exercises (list/create), workouts (create/list/get/delete). ✓
- §6 estructura de código (service con pool+transacción, types, handler, montaje, lib, página, enlace) → Tasks 4, 5, 7, 8. ✓
- §8 manejo de errores (400/401/404/500, rollback) → Task 5 handlers + Task 6 tests (validación, auth, isolation, 404). ✓
- §9 testing (store + handler; lib + página) → Tasks 3, 6, 7, 8. ✓
- §10 criterios de aceptación (tablas, crear/borrar, catálogo on-the-fly, idempotencia por capitalización, peso en gramos, aislamiento, 401 sin sesión, rollback, builds verdes) → cubiertos por Tasks 6 y 9. ✓

**Placeholders:** No hay TBD/TODO; todo el código está completo en cada step.

**Consistencia de tipos:** `*int32` para `Reps`/`WeightGrams` en store (sqlc nullable), dominio (`SetInput`, `WorkoutSet`) y handler (`setReq`); `From`/`To` como `*time.Time` en `ListWorkoutsParams`; nombres de funciones del servicio (`ListExercises`, `CreateExercise`, `CreateWorkout`, `ListWorkouts`, `GetWorkout`, `DeleteWorkout`) consistentes entre service, handler y lib (`listExercises`, `createExercise`, `createWorkout`, `listWorkouts`, `getWorkout`, `removeWorkout`). Endpoints `/api/v1/training/...` idénticos en handler, lib y smoke e2e.

> Riesgo conocido: los nombres exactos de structs/params generados por sqlc (p. ej. `ListWorkoutsParams.From`, `ListSetsByWorkoutIDsRow.ExerciseName`, parámetro `workoutIds`) deben confirmarse tras `sqlc generate` en Task 2 y ajustarse en Tasks 3–4 si difieren. El plan lo señala en los steps correspondientes.
