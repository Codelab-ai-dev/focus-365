# Análisis / ajustes del agente — Plan de implementación (Entrenamiento slice C2, Rebanada 27)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un panel "Análisis del agente" en `/entrenamiento` con un toggle de alcance (último entreno / última semana) y un botón "Analizar" que genera, vía Groq, ajustes concretos a partir del perfil + los entrenos recientes con sus notas por serie; guarda el último análisis.

**Architecture:** Espejo del slice B (R25). Tabla `training_adjustments` (1 por usuario, upsert). Un método `SuggestAdjustments(userID, scope, today)` que filtra el historial por alcance (`last` = últimas 3 sesiones; `week` = últimos 7 días), **reutiliza** `buildSuggestionContext` (perfil + historial + notas, de B) y llama a Groq con un prompt de ajustes. Endpoints `GET/POST /training/adjustment`. Frontend: `lib/trainingAdjustment.ts` + una Card en `entrenamiento.tsx`.

**Tech Stack:** Go (chi, sqlc, pgx/v5, goose), Groq (Completer ya inyectado en `training.Service`), Postgres, React + Vite + TanStack Query + Vitest.

**Contexto del repo (leer antes de empezar):**
- `training.Service` ya tiene `groq completer` + `hasKey` (slice B). `NewService(q, pool, groq, hasKey)` — **sin cambios** en `server.go`.
- `suggestion.go` (B) ya define, en el package `training`: `completer`, `ErrUnavailable`, `ageFrom`, y `buildSuggestionContext(p *store.FitnessProfile, workouts []store.Workout, sets []store.ListSetsByWorkoutIDsRow, focus string, today time.Time) string` (incluye las notas de serie tras C1). C2 los reutiliza.
- `store.Workout{ID uuid.UUID, Date time.Time, Type, Note string, ...}` (`Date` es `time.Time`, una DATE a medianoche). `ListWorkouts(ctx, store.ListWorkoutsParams{UserID}) ([]store.Workout, error)` ordena date DESC. `ListSetsByWorkoutIDs(ctx, []uuid.UUID) ([]store.ListSetsByWorkoutIDsRow, error)` (con `Note`).
- `GetFitnessProfile(ctx, userID) (store.FitnessProfile, error)` → `pgx.ErrNoRows` si no hay perfil.
- Tests de `training`: `handler_test.go` es `package training_test` y ya tiene `fakeCompleter` (campos `out`/`err`/`lastSystem`/`lastUser`), `newEnv(t)` (default fake completer + hasKey true) y `newEnvWith(t, c *fakeCompleter, hasKey bool)`, más `token`/`do`. `suggestion_test.go` (package `training_test`) es el modelo a copiar.
- `httpx.DecodeAndValidate` valida tags `validate:`. Última migración: `0022_workout_set_note.sql` → la nueva es `0023`.

**Refinamiento aprobado:** `scope=last` pasa al agente las **3 sesiones más recientes** (para comparar progresión), no solo la última.

---

## Estructura de archivos

**Backend**
- Crear `api/db/migrations/0023_training_adjustments.sql`.
- Crear `api/db/queries/training_adjustments.sql`.
- Regenerar `api/internal/store/*` (sqlc).
- Crear `api/internal/store/training_adjustments_test.go`.
- Crear `api/internal/training/adjustment.go` — vista `Adjustment`, `Adjustment`/`SuggestAdjustments`, `filterWorkoutsByScope`, prompt.
- Modificar `api/internal/training/handler.go` — rutas + handlers `GET/POST /adjustment`.
- Crear `api/internal/training/adjustment_test.go` (`package training_test`) — tests de handler.
- Crear `api/internal/training/adjustment_internal_test.go` (`package training`) — unit de `filterWorkoutsByScope`.

**Frontend**
- Crear `web/src/lib/trainingAdjustment.ts` + `web/src/lib/trainingAdjustment.test.ts`.
- Modificar `web/src/routes/entrenamiento.tsx` — panel "Análisis del agente".
- Modificar `web/src/routes/entrenamiento.test.tsx` — test del panel.

---

## Task 1: Migración 0023 + queries + test de store

**Files:**
- Create: `api/db/migrations/0023_training_adjustments.sql`
- Create: `api/db/queries/training_adjustments.sql`
- Create: `api/internal/store/training_adjustments_test.go`
- Regenerate: `api/internal/store/`

- [ ] **Step 1: Migración**

Crear `api/db/migrations/0023_training_adjustments.sql`:

```sql
-- +goose Up
CREATE TABLE training_adjustments (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    scope      TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE training_adjustments;
```

- [ ] **Step 2: Queries**

Crear `api/db/queries/training_adjustments.sql`:

```sql
-- name: GetTrainingAdjustment :one
SELECT * FROM training_adjustments WHERE user_id = $1;

-- name: UpsertTrainingAdjustment :one
INSERT INTO training_adjustments (user_id, scope, content, created_at)
VALUES (@user_id, @scope, @content, now())
ON CONFLICT (user_id) DO UPDATE SET
    scope      = EXCLUDED.scope,
    content    = EXCLUDED.content,
    created_at = now()
RETURNING *;
```

- [ ] **Step 3: Regenerar sqlc**

Run: `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`
Verificá: `grep -n "TrainingAdjustment\|UpsertTrainingAdjustmentParams" internal/store/*.go`
Esperado: modelo `TrainingAdjustment{UserID uuid.UUID, Scope string, Content string, CreatedAt time.Time}`; `UpsertTrainingAdjustmentParams{UserID, Scope, Content}`; `GetTrainingAdjustment(ctx, userID) (TrainingAdjustment, error)`.

- [ ] **Step 4: Test de store (que falla)**

Crear `api/internal/store/training_adjustments_test.go`. Reusá `newUser`.

```go
package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestTrainingAdjustmentUpsert(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)

	if _, err := q.GetTrainingAdjustment(ctx, u); err != pgx.ErrNoRows {
		t.Fatalf("sin análisis: err = %v, want ErrNoRows", err)
	}
	a1, err := q.UpsertTrainingAdjustment(ctx, store.UpsertTrainingAdjustmentParams{
		UserID: u, Scope: "last", Content: "ajuste A",
	})
	if err != nil || a1.Scope != "last" || a1.Content != "ajuste A" {
		t.Fatalf("insert: %v %+v", err, a1)
	}
	a2, err := q.UpsertTrainingAdjustment(ctx, store.UpsertTrainingAdjustmentParams{
		UserID: u, Scope: "week", Content: "ajuste B",
	})
	if err != nil || a2.Scope != "week" || a2.Content != "ajuste B" {
		t.Fatalf("update: %v %+v", err, a2)
	}
	got, err := q.GetTrainingAdjustment(ctx, u)
	if err != nil || got.Content != "ajuste B" {
		t.Fatalf("get: %v %+v", err, got)
	}
}
```

- [ ] **Step 5: Correr (falla→pasa)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ -run TestTrainingAdjustment -v`
Expected: PASS. El build completo sigue verde (additivo).

- [ ] **Step 6: Commit**

```bash
git add api/db/migrations/0023_training_adjustments.sql api/db/queries/training_adjustments.sql api/internal/store
git commit -m "feat(store): tabla training_adjustments + upsert (migración 0023)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Servicio + endpoints `GET/POST /training/adjustment`

**Files:**
- Create: `api/internal/training/adjustment.go`
- Modify: `api/internal/training/handler.go`
- Create: `api/internal/training/adjustment_test.go`
- Create: `api/internal/training/adjustment_internal_test.go`

- [ ] **Step 1: `adjustment.go`**

Crear `api/internal/training/adjustment.go`:

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
)

const (
	adjustmentLastSessions = 3
	adjustmentWeekDays     = 7
)

// Adjustment es la vista del último análisis/ajustes del agente.
type Adjustment struct {
	Scope     string    `json:"scope"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func buildAdjustment(a store.TrainingAdjustment) Adjustment {
	return Adjustment{Scope: a.Scope, Content: a.Content, CreatedAt: a.CreatedAt}
}

const adjustmentsSystemPrompt = `Sos un entrenador personal. ANALIZÁ el PERFIL y el HISTORIAL reciente del usuario (incluí las notas de cada serie) y proponé AJUSTES concretos para la próxima sesión o la próxima semana.
Reglas:
- Centrate en lo más reciente; usá las sesiones anteriores para comparar progresión.
- Proponé progresión o descarga concreta (peso/reps) por ejercicio.
- Atendé lo que digan las notas (molestias, "fácil", "pesado") y las limitaciones del perfil.
- Sugerí correcciones de técnica o cambios de ejercicio si corresponde.
- Cerrá con un resumen breve de qué ajustar.
- Si no hay entrenos en el período, decilo y sugerí empezar a registrar.
- Respondé en español, concreto y accionable.`

// Adjustment devuelve el último análisis guardado, o nil.
func (s *Service) Adjustment(ctx context.Context, userID uuid.UUID) (*Adjustment, error) {
	row, err := s.q.GetTrainingAdjustment(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v := buildAdjustment(row)
	return &v, nil
}

// SuggestAdjustments genera un análisis con Groq desde el perfil + el historial
// del alcance + las notas, lo persiste (upsert) y lo devuelve. ErrUnavailable
// sin clave o ante fallo de Groq.
func (s *Service) SuggestAdjustments(ctx context.Context, userID uuid.UUID, scope string, today time.Time) (*Adjustment, error) {
	if !s.hasKey {
		return nil, ErrUnavailable
	}
	var profile *store.FitnessProfile
	if p, err := s.q.GetFitnessProfile(ctx, userID); err == nil {
		profile = &p
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	workouts, err := s.q.ListWorkouts(ctx, store.ListWorkoutsParams{UserID: userID})
	if err != nil {
		return nil, err
	}
	workouts = filterWorkoutsByScope(workouts, scope, today)

	var sets []store.ListSetsByWorkoutIDsRow
	if len(workouts) > 0 {
		ids := make([]uuid.UUID, len(workouts))
		for i, w := range workouts {
			ids[i] = w.ID
		}
		if sets, err = s.q.ListSetsByWorkoutIDs(ctx, ids); err != nil {
			return nil, err
		}
	}

	userCtx := buildSuggestionContext(profile, workouts, sets, "", today)
	content, err := s.groq.Complete(ctx, adjustmentsSystemPrompt, userCtx)
	if err != nil {
		return nil, ErrUnavailable
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrUnavailable
	}
	row, err := s.q.UpsertTrainingAdjustment(ctx, store.UpsertTrainingAdjustmentParams{
		UserID: userID, Scope: scope, Content: content,
	})
	if err != nil {
		return nil, err
	}
	v := buildAdjustment(row)
	return &v, nil
}

// filterWorkoutsByScope recorta el historial (que viene ordenado date DESC)
// según el alcance: "week" → sesiones de los últimos adjustmentWeekDays días;
// cualquier otro (incl. "last") → las primeras adjustmentLastSessions.
func filterWorkoutsByScope(workouts []store.Workout, scope string, today time.Time) []store.Workout {
	if scope == "week" {
		cutoff := today.AddDate(0, 0, -(adjustmentWeekDays - 1))
		out := make([]store.Workout, 0, len(workouts))
		for _, w := range workouts {
			if !w.Date.Before(cutoff) {
				out = append(out, w)
			}
		}
		return out
	}
	if len(workouts) > adjustmentLastSessions {
		return workouts[:adjustmentLastSessions]
	}
	return workouts
}
```

- [ ] **Step 2: Rutas + handlers**

En `api/internal/training/handler.go`:

a) Agregar a `Routes` (junto a las rutas de `/suggestion`):

```go
	r.Get("/adjustment", handleGetAdjustment(svc))
	r.Post("/adjustment", handleAdjust(svc))
```

b) Agregar el body y los handlers (los imports `errors`/`time`/`auth`/`httpx`/`uuid` ya están):

```go
type adjustReq struct {
	Scope string `json:"scope" validate:"required,oneof=last week"`
}

func handleGetAdjustment(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		a, err := svc.Adjustment(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, a) // nil -> null
	}
}

func handleAdjust(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req adjustReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		now := time.Now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		out, err := svc.SuggestAdjustments(r.Context(), userID, req.Scope, today)
		if err != nil {
			if errors.Is(err, ErrUnavailable) {
				httpx.WriteErr(w, http.StatusServiceUnavailable, "el entrenador no está disponible por ahora")
				return
			}
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}
```

- [ ] **Step 3: Unit de `filterWorkoutsByScope` (que falla)**

Crear `api/internal/training/adjustment_internal_test.go`:

```go
package training

import (
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

func TestFilterWorkoutsByScope(t *testing.T) {
	today := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	mk := func(d string) store.Workout {
		dt, _ := time.Parse("2006-01-02", d)
		return store.Workout{ID: uuid.New(), Date: dt}
	}
	// ordenadas date DESC, como las devuelve ListWorkouts
	ws := []store.Workout{mk("2026-06-17"), mk("2026-06-15"), mk("2026-06-12"), mk("2026-06-05"), mk("2026-05-20")}

	last := filterWorkoutsByScope(ws, "last", today)
	if len(last) != 3 {
		t.Fatalf("last = %d, want 3", len(last))
	}

	// últimos 7 días: corte en 2026-06-11 → 17, 15, 12 entran; 05 y 20-may quedan fuera
	week := filterWorkoutsByScope(ws, "week", today)
	if len(week) != 3 {
		t.Fatalf("week = %d, want 3 (17,15,12)", len(week))
	}
	cutoff := today.AddDate(0, 0, -6)
	for _, w := range week {
		if w.Date.Before(cutoff) {
			t.Errorf("week incluyó una sesión vieja: %s", w.Date.Format("2006-01-02"))
		}
	}
}
```

- [ ] **Step 4: Tests de handler (que fallan)**

Crear `api/internal/training/adjustment_test.go` (`package training_test`):

```go
package training_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAdjustHappyPathPersists(t *testing.T) {
	c := &fakeCompleter{out: "Subí 2.5 kg en sentadilla la próxima."}
	e := newEnvWith(t, c, true)
	tok := e.token(t, "adj@b.com")

	do(t, e.h, http.MethodPut, "/training/profile", tok, map[string]any{"objective": "fuerza"})
	do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-15", "type": "Pierna",
		"sets": []map[string]any{{"exercise": "Sentadilla", "reps": 5, "weight_grams": 100000, "note": "fácil"}},
	})

	rec := do(t, e.h, http.MethodPost, "/training/adjustment", tok, map[string]any{"scope": "last"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST adjustment code = %d, body=%s", rec.Code, rec.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if a["content"] != "Subí 2.5 kg en sentadilla la próxima." || a["scope"] != "last" {
		t.Fatalf("adjustment = %+v", a)
	}
	for _, want := range []string{"Sentadilla", "fácil", "fuerza"} {
		if !strings.Contains(c.lastUser, want) {
			t.Errorf("el contexto no contiene %q:\n%s", want, c.lastUser)
		}
	}

	rec = do(t, e.h, http.MethodGet, "/training/adjustment", tok, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if a["content"] != "Subí 2.5 kg en sentadilla la próxima." {
		t.Fatalf("GET adjustment = %+v", a)
	}
}

func TestGetAdjustmentEmpty(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "adjempty@b.com")
	rec := do(t, e.h, http.MethodGet, "/training/adjustment", tok, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("GET vacío = %d %q", rec.Code, rec.Body.String())
	}
}

func TestAdjustInvalidScope(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "adjscope@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/adjustment", tok, map[string]any{"scope": "mes"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("scope inválido code = %d, want 400", rec.Code)
	}
}

func TestAdjustNoKeyIs503(t *testing.T) {
	e := newEnvWith(t, &fakeCompleter{out: "x"}, false)
	tok := e.token(t, "adjnokey@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/adjustment", tok, map[string]any{"scope": "week"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin clave code = %d, want 503", rec.Code)
	}
}
```

- [ ] **Step 5: Verificar build + suite**

Run:
```
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
```
Expected: todo verde.

- [ ] **Step 6: Commit**

```bash
git add api/internal/training
git commit -m "feat(training): análisis/ajustes del agente — GET/POST /training/adjustment

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Frontend — lib `trainingAdjustment.ts`

**Files:**
- Create: `web/src/lib/trainingAdjustment.ts`
- Create: `web/src/lib/trainingAdjustment.test.ts`

- [ ] **Step 1: Test (que falla)**

Crear `web/src/lib/trainingAdjustment.test.ts` (mocks de fetch tipados `(_url: string, _opts?: RequestInit)`):

```ts
import { describe, it, expect, vi, afterEach } from "vitest";
import { getAdjustment, generateAdjustment } from "./trainingAdjustment";

afterEach(() => vi.restoreAllMocks());

describe("trainingAdjustment", () => {
  it("getAdjustment devuelve null cuando no hay", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response("null", { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    expect(await getAdjustment()).toBeNull();
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/training/adjustment");
  });

  it("generateAdjustment hace POST con el scope", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ scope: "week", content: "ajustá", created_at: "" }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    const a = await generateAdjustment("week");
    expect(a.content).toBe("ajustá");
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("POST");
    expect(String(opts.body)).toContain("week");
  });
});
```

- [ ] **Step 2: Verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/trainingAdjustment.test.ts`
Expected: FAIL.

- [ ] **Step 3: Implementar `web/src/lib/trainingAdjustment.ts`**

```ts
import { apiFetch } from "./api";

export type TrainingAdjustment = {
  scope: string;
  content: string;
  created_at: string;
};

export function getAdjustment(): Promise<TrainingAdjustment | null> {
  return apiFetch<TrainingAdjustment | null>("/api/v1/training/adjustment");
}

export function generateAdjustment(scope: "last" | "week"): Promise<TrainingAdjustment> {
  return apiFetch<TrainingAdjustment>("/api/v1/training/adjustment", {
    method: "POST",
    body: JSON.stringify({ scope }),
  });
}
```

- [ ] **Step 4: Verde + build (typecheck)**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/trainingAdjustment.test.ts && npm run build`
Expected: tests PASS y build OK. (Correr `npm run build` acá es obligatorio para atajar typecheck antes de la Task 4.)

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/trainingAdjustment.ts web/src/lib/trainingAdjustment.test.ts
git commit -m "feat(web): lib de análisis del agente (get/generate adjustment)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Frontend — panel "Análisis del agente" en `entrenamiento.tsx`

**Files:**
- Modify: `web/src/routes/entrenamiento.tsx`
- Modify: `web/src/routes/entrenamiento.test.tsx`

- [ ] **Step 1: Test (que falla)**

En `web/src/routes/entrenamiento.test.tsx`, mockeá `@/lib/trainingAdjustment` con `vi.mock` y agregá un test: hay un botón "Analizar"; al tocarlo se llama `generateAdjustment` y se muestra el `content` devuelto.

```tsx
// vi.mock("@/lib/trainingAdjustment", () => ({
//   getAdjustment: vi.fn(async () => null),
//   generateAdjustment: vi.fn(async () => ({ scope: "last", content: "Subí 2.5 kg en sentadilla", created_at: "" })),
// }));
// ...
// await userEvent.click(await screen.findByRole("button", { name: "Analizar" }));
// expect(await screen.findByText(/Subí 2.5 kg en sentadilla/)).toBeInTheDocument();
```
Adaptá al harness real (ya mockea `@/lib/fitnessProfile` y `@/lib/trainingSuggestion`). Watch fail.

- [ ] **Step 2: Imports + estado**

En `web/src/routes/entrenamiento.tsx`:

```tsx
import { getAdjustment, generateAdjustment, type TrainingAdjustment } from "@/lib/trainingAdjustment";
```

Junto a los otros `useState`:

```tsx
  const [adjustScope, setAdjustScope] = useState<"last" | "week">("last");
  const [adjustError, setAdjustError] = useState<string | null>(null);
```

- [ ] **Step 3: Query + mutación**

Junto a las otras queries/mutaciones:

```tsx
  const adjustmentQuery = useQuery({
    queryKey: ["training-adjustment"],
    queryFn: getAdjustment,
    enabled: !!user,
  });
  const adjustMutation = useMutation({
    mutationFn: () => generateAdjustment(adjustScope),
    onSuccess: (a) => {
      setAdjustError(null);
      qc.setQueryData<TrainingAdjustment | null>(["training-adjustment"], a);
    },
    onError: (e) =>
      setAdjustError(e instanceof Error ? e.message : "No se pudo generar el análisis"),
  });
```

- [ ] **Step 4: Panel "Análisis del agente"**

Agregar una `Card` **después** del panel "Entrenador IA" y **antes** de la `<section>` "Historial":

```tsx
        <Card className="mt-8 p-6 space-y-3">
          <h2 className="font-display text-lg font-bold tracking-tight">Análisis del agente</h2>
          <div className="flex flex-wrap items-center gap-2">
            {([
              { v: "last", label: "Último entreno" },
              { v: "week", label: "Última semana" },
            ] as const).map((o) => (
              <button
                key={o.v}
                type="button"
                aria-pressed={adjustScope === o.v}
                onClick={() => setAdjustScope(o.v)}
                className={`rounded-lg border-2 border-ink px-3 py-1 text-xs font-bold shadow-brutal-sm ${adjustScope === o.v ? "bg-accent" : "bg-surface"}`}
              >
                {o.label}
              </button>
            ))}
            <Button
              type="button"
              onClick={() => adjustMutation.mutate()}
              disabled={adjustMutation.isPending}
              className="px-3 py-1 text-xs"
            >
              {adjustMutation.isPending ? "Analizando…" : "Analizar"}
            </Button>
          </div>
          {adjustError && (
            <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-xs font-bold text-danger-fg shadow-brutal-sm">
              {adjustError}
            </p>
          )}
          {adjustmentQuery.data ? (
            <div className="rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm">
              <p className="whitespace-pre-wrap text-sm">{adjustmentQuery.data.content}</p>
              {adjustmentQuery.data.created_at && (
                <p className="mt-2 text-[10px] uppercase tracking-[0.12em] text-muted">
                  {adjustmentQuery.data.scope === "week" ? "última semana" : "último entreno"} · {relativeDateTraining(adjustmentQuery.data.created_at)}
                </p>
              )}
            </div>
          ) : (
            !adjustMutation.isPending && (
              <p className="text-sm text-muted">Pedí un análisis de tus entrenos recientes.</p>
            )
          )}
        </Card>
```

> `relativeDateTraining` ya existe en el archivo (lo agregó la R25); reusalo.

- [ ] **Step 5: Verde + suite + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build`
Expected: todo verde; build OK (typecheck incluido).

- [ ] **Step 6: Commit**

```bash
git add web/src/routes/entrenamiento.tsx web/src/routes/entrenamiento.test.tsx
git commit -m "feat(web): panel Análisis del agente en entrenamiento

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Cierre — review, merge y smoke

**Files:** verificación + `scripts/smoke-r27.sh` + bitácora.

- [ ] **Step 1: Review final** del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-17-plan-27-ajustes-agente-design.md`. Aplicar nits.

- [ ] **Step 2: Suites verdes**
Backend: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && TEST_DATABASE_URL=... go test -p 1 ./... -count=1`
Frontend: `cd web && npx vitest run && npm run build`

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`.

- [ ] **Step 4: Deploy manual (Coolify) + smoke.** Crear `scripts/smoke-r27.sh` (patrón de `scripts/smoke-r25.sh`, extraer con `grep -o`; Groq real puede tardar): `GET /training/adjustment` → `null`; `POST {"scope":"last"}` → 200 con `content` no vacío; `GET` de nuevo → el análisis (scope "last"); `POST {"scope":"week"}` → 200; `POST {"scope":"mes"}` → 400.

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-17-sesion-plan-27-ajustes-agente.md` (cierra el agente de entrenamiento A→C2; queda D).

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 modelo (training_adjustments 1:1) → Task 1. ✓
- §3 backend (Get/Upsert; Adjustment/SuggestAdjustments con filtro por alcance reutilizando buildSuggestionContext; adjustmentsSystemPrompt; rutas GET/POST; validación scope oneof=last week → 400; 503) → Task 1 (queries) + Task 2. ✓
- §4 frontend (lib get/generate; panel con toggle de alcance + botón + render pre-wrap + precarga) → Task 3 (lib) + Task 4 (UI). ✓
- §5 errores (503; scope inválido 400; sin entrenos genera igual; ownership PK) → Task 2. ✓
- §6 testing → Tasks 1–4 (incl. unit de filterWorkoutsByScope para el alcance) ; E2E → Task 5. ✓
- §7 aceptación → smoke Task 5. ✓

**Placeholders:** las adaptaciones al harness de tests son deterministas con instrucción de qué inspeccionar. Reutiliza piezas existentes (`buildSuggestionContext`, `completer`, `ErrUnavailable`, `fakeCompleter`, `newEnvWith`, `relativeDateTraining`) verificadas en el contexto del repo.

**Consistencia de tipos/firmas:** store `UpsertTrainingAdjustmentParams{UserID,Scope,Content}`/`TrainingAdjustment` ↔ servicio `Adjustment`/`SuggestAdjustments(userID,scope,today)`/`Adjustment(userID)`/`buildAdjustment`/`filterWorkoutsByScope` ↔ handler `adjustReq` (`oneof=last week`)/rutas ↔ endpoints `GET/POST /training/adjustment` ↔ lib `TrainingAdjustment`/`getAdjustment`/`generateAdjustment("last"|"week")`. `SuggestAdjustments` reutiliza `buildSuggestionContext(..., "", today)` (focus vacío). ✓

**Lección aplicada (R21/R23):** la Task 3 (lib) corre `npm run build` además del test.
