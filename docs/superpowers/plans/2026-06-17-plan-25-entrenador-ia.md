# Entrenador IA: sugerencias — Plan de implementación (Entrenamiento slice B, Rebanada 25)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un panel "Entrenador IA" en `/entrenamiento` con un botón "Sugerir" (y enfoque opcional) que genera, vía Groq, una rutina/ejercicios en texto según el perfil de fitness + el historial reciente, y guarda la última sugerencia.

**Architecture:** Tabla `training_suggestions` (1 por usuario, upsert). El paquete `training` gana un `Completer` de Groq inyectado y un método `SuggestTraining` que arma un prompt desde el perfil (slice A) + los últimos 8 entrenos con sus series + el enfoque, llama a Groq una vez y persiste. Endpoints `GET/POST /training/suggestion`. Frontend: `lib/trainingSuggestion.ts` + un panel en `entrenamiento.tsx`.

**Tech Stack:** Go (chi, sqlc, pgx/v5, goose), Groq (Completer), Postgres, React + Vite + TanStack Query + Vitest.

**Contexto del repo (leer antes de empezar):**
- sqlc: nullable→puntero, `text[]`→`[]string`, `date`/`timestamptz`→`time.Time`. Tras editar SQL: `cd api && sqlc generate`. DB test en `localhost:5544`. Comandos Go: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" <cmd>`, `TEST_DATABASE_URL=...`; **suite con `-p 1`**.
- `ai.Completer` = `Complete(ctx, system, user string) (string, error)`; `*ai.GroqClient` lo implementa. **`ai` importa `training`** (vía `NewActionExecutor`), así que `training` NO puede importar `ai` (ciclo) → `training` define su propia interfaz `completer` con esa firma; `GroqClient` la satisface estructuralmente.
- `training.Service` hoy: `type Service struct { q *store.Queries; pool *pgxpool.Pool }`, `NewService(q, pool)`. Llamadores de `NewService`: `server.go:38` y el `newEnv` de `handler_test.go`.
- Queries existentes reutilizadas: `GetFitnessProfile(ctx, userID) (store.FitnessProfile, error)` (slice A); `ListWorkouts(ctx, store.ListWorkoutsParams{UserID, From, To}) ([]store.Workout, error)` (orden date DESC; From/To nullable, dejar en cero = todos); `ListSetsByWorkoutIDs(ctx, []uuid.UUID) ([]store.ListSetsByWorkoutIDsRow, error)` con campos `WorkoutID uuid.UUID, Position int32, Reps *int32, WeightGrams *int32, ExerciseName string`. `store.Workout{ID, UserID uuid.UUID, Date time.Time, Type, Note string, CreatedAt time.Time}`. `store.FitnessProfile{Birthdate *time.Time, Sex *string, HeightCm *int32, WeightGrams *int32, Objective *string, Location *string, Level *string, WeeklyDays *int32, Equipment []string, Limitations string, ...}`.
- `httpx.DecodeAndValidate`, `httpx.WriteErr`, `httpx.WriteJSON` como en el resto. Última migración: `0020_fitness_profiles.sql` → la nueva es `0021`.

---

## Estructura de archivos

**Backend**
- Crear `api/db/migrations/0021_training_suggestions.sql`.
- Crear `api/db/queries/training_suggestions.sql`.
- Regenerar `api/internal/store/*` (sqlc).
- Crear `api/internal/store/training_suggestions_test.go`.
- Crear `api/internal/training/suggestion.go` — interfaz `completer`, `ErrUnavailable`, vista `Suggestion`, prompt + `Suggestion`/`SuggestTraining`.
- Modificar `api/internal/training/service.go` — `Service` gana `groq`/`hasKey`; `NewService(q, pool, groq, hasKey)`.
- Modificar `api/internal/training/handler.go` — rutas + handlers `GET/POST /suggestion`.
- Modificar `api/internal/server/server.go` — crear `groq` antes de `trainingSvc` y pasarlo.
- Modificar `api/internal/training/handler_test.go` y crear/ajustar `api/internal/training/suggestion_test.go` — fake completer + tests.

**Frontend**
- Crear `web/src/lib/trainingSuggestion.ts` + `web/src/lib/trainingSuggestion.test.ts`.
- Modificar `web/src/routes/entrenamiento.tsx` — panel "Entrenador IA".
- Modificar `web/src/routes/entrenamiento.test.tsx` — test del panel.

---

## Task 1: Migración 0021 + queries + tests de store

**Files:**
- Create: `api/db/migrations/0021_training_suggestions.sql`
- Create: `api/db/queries/training_suggestions.sql`
- Create: `api/internal/store/training_suggestions_test.go`
- Regenerate: `api/internal/store/`

- [ ] **Step 1: Migración**

Crear `api/db/migrations/0021_training_suggestions.sql`:

```sql
-- +goose Up
CREATE TABLE training_suggestions (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    focus      TEXT NOT NULL DEFAULT '',
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE training_suggestions;
```

- [ ] **Step 2: Queries**

Crear `api/db/queries/training_suggestions.sql`:

```sql
-- name: GetTrainingSuggestion :one
SELECT * FROM training_suggestions WHERE user_id = $1;

-- name: UpsertTrainingSuggestion :one
INSERT INTO training_suggestions (user_id, focus, content, created_at)
VALUES (@user_id, @focus, @content, now())
ON CONFLICT (user_id) DO UPDATE SET
    focus      = EXCLUDED.focus,
    content    = EXCLUDED.content,
    created_at = now()
RETURNING *;
```

- [ ] **Step 3: Regenerar sqlc**

Run: `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`
Verificá: `grep -n "TrainingSuggestion\|UpsertTrainingSuggestionParams" internal/store/*.go`
Esperado: modelo `TrainingSuggestion{ UserID uuid.UUID; Focus string; Content string; CreatedAt time.Time }`; `UpsertTrainingSuggestionParams{ UserID uuid.UUID; Focus string; Content string }`; `GetTrainingSuggestion(ctx, userID) (TrainingSuggestion, error)`.

- [ ] **Step 4: Test de store (que falla)**

Crear `api/internal/store/training_suggestions_test.go`. Reusá `newUser`.

```go
package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestTrainingSuggestionUpsert(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)

	// no existe
	if _, err := q.GetTrainingSuggestion(ctx, u); err != pgx.ErrNoRows {
		t.Fatalf("sin sugerencia: err = %v, want ErrNoRows", err)
	}

	// insert
	s1, err := q.UpsertTrainingSuggestion(ctx, store.UpsertTrainingSuggestionParams{
		UserID: u, Focus: "pierna", Content: "rutina A",
	})
	if err != nil || s1.Content != "rutina A" || s1.Focus != "pierna" {
		t.Fatalf("insert: %v %+v", err, s1)
	}

	// upsert reemplaza (sigue una fila)
	s2, err := q.UpsertTrainingSuggestion(ctx, store.UpsertTrainingSuggestionParams{
		UserID: u, Focus: "", Content: "rutina B",
	})
	if err != nil || s2.Content != "rutina B" || s2.Focus != "" {
		t.Fatalf("update: %v %+v", err, s2)
	}

	got, err := q.GetTrainingSuggestion(ctx, u)
	if err != nil || got.Content != "rutina B" {
		t.Fatalf("get tras upsert: %v %+v", err, got)
	}
}
```

- [ ] **Step 5: Correr (falla→pasa)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ -run TestTrainingSuggestion -v`
Expected: PASS. (El build completo sigue verde: additivo.)

- [ ] **Step 6: Commit**

```bash
git add api/db/migrations/0021_training_suggestions.sql api/db/queries/training_suggestions.sql api/internal/store
git commit -m "feat(store): tabla training_suggestions + upsert (migración 0021)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Servicio + endpoints + wiring

**Files:**
- Create: `api/internal/training/suggestion.go`
- Modify: `api/internal/training/service.go`
- Modify: `api/internal/training/handler.go`
- Modify: `api/internal/server/server.go`
- Create: `api/internal/training/suggestion_test.go`
- Modify: `api/internal/training/handler_test.go`

- [ ] **Step 1: `Service` gana `groq`/`hasKey`**

En `api/internal/training/service.go`, cambiar el struct y el constructor:

```go
type Service struct {
	q      *store.Queries
	pool   *pgxpool.Pool
	groq   completer
	hasKey bool
}

// NewService recibe además el cliente de Groq (para las sugerencias del
// entrenador) y si hay clave configurada. completer está definido en suggestion.go.
func NewService(q *store.Queries, pool *pgxpool.Pool, groq completer, hasKey bool) *Service {
	return &Service{q: q, pool: pool, groq: groq, hasKey: hasKey}
}
```

- [ ] **Step 2: `suggestion.go`**

Crear `api/internal/training/suggestion.go`:

```go
package training

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	suggestionHistoryLimit = 8
	maxFocusChars          = 200
)

// ErrUnavailable: el entrenador IA no está disponible (sin clave o fallo de Groq).
// El handler lo traduce a 503.
var ErrUnavailable = errors.New("entrenador no disponible")

// completer abstrae la llamada bloqueante a Groq (la satisface *ai.GroqClient).
// Definida acá para no importar el paquete ai (evita ciclo).
type completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// Suggestion es la vista de la última sugerencia del entrenador.
type Suggestion struct {
	Focus     string    `json:"focus"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func buildSuggestion(s store.TrainingSuggestion) Suggestion {
	return Suggestion{Focus: s.Focus, Content: s.Content, CreatedAt: s.CreatedAt}
}

const suggestionSystemPrompt = `Sos un entrenador personal. A partir del PERFIL y el HISTORIAL del usuario, proponé una rutina o ejercicios concretos para su próxima sesión.
Reglas:
- Priorizá el equipo disponible y el lugar (si entrena en casa, usá lo que tiene).
- Apuntá al objetivo y respetá las limitaciones/lesiones.
- Ajustá el volumen y la intensidad al nivel y la frecuencia.
- Si hay un ENFOQUE PEDIDO, centrate en eso.
- Si el perfil está incompleto, hacé una sugerencia general y recomendá completar el perfil.
- Respondé en español, con ejercicios, series×reps y descansos, y una breve explicación. Sé concreto y accionable.`

// Suggestion devuelve la última sugerencia guardada, o nil si no hay.
func (s *Service) Suggestion(ctx context.Context, userID uuid.UUID) (*Suggestion, error) {
	row, err := s.q.GetTrainingSuggestion(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v := buildSuggestion(row)
	return &v, nil
}

// SuggestTraining genera una sugerencia con Groq desde el perfil + historial +
// enfoque, la persiste (upsert) y la devuelve. ErrUnavailable sin clave o ante
// fallo de Groq.
func (s *Service) SuggestTraining(ctx context.Context, userID uuid.UUID, focus string, today time.Time) (*Suggestion, error) {
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
	if len(workouts) > suggestionHistoryLimit {
		workouts = workouts[:suggestionHistoryLimit]
	}
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

	userCtx := buildSuggestionContext(profile, workouts, sets, focus, today)
	content, err := s.groq.Complete(ctx, suggestionSystemPrompt, userCtx)
	if err != nil {
		return nil, ErrUnavailable
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrUnavailable
	}
	row, err := s.q.UpsertTrainingSuggestion(ctx, store.UpsertTrainingSuggestionParams{
		UserID: userID, Focus: focus, Content: content,
	})
	if err != nil {
		return nil, err
	}
	v := buildSuggestion(row)
	return &v, nil
}

// buildSuggestionContext arma el texto del usuario para el prompt: perfil +
// historial reciente (con series) + enfoque.
func buildSuggestionContext(p *store.FitnessProfile, workouts []store.Workout, sets []store.ListSetsByWorkoutIDsRow, focus string, today time.Time) string {
	var b strings.Builder
	b.WriteString("PERFIL:\n")
	if p == nil {
		b.WriteString("(sin perfil cargado)\n")
	} else {
		if p.Birthdate != nil {
			fmt.Fprintf(&b, "- edad: %d años\n", ageFrom(*p.Birthdate, today))
		}
		if p.Sex != nil {
			b.WriteString("- sexo: " + *p.Sex + "\n")
		}
		if p.HeightCm != nil {
			fmt.Fprintf(&b, "- altura: %d cm\n", *p.HeightCm)
		}
		if p.WeightGrams != nil {
			fmt.Fprintf(&b, "- peso: %.1f kg\n", float64(*p.WeightGrams)/1000)
		}
		if p.Objective != nil {
			b.WriteString("- objetivo: " + *p.Objective + "\n")
		}
		if p.Location != nil {
			b.WriteString("- lugar: " + *p.Location + "\n")
		}
		if p.Level != nil {
			b.WriteString("- nivel: " + *p.Level + "\n")
		}
		if p.WeeklyDays != nil {
			fmt.Fprintf(&b, "- días por semana: %d\n", *p.WeeklyDays)
		}
		if len(p.Equipment) > 0 {
			b.WriteString("- equipo: " + strings.Join(p.Equipment, ", ") + "\n")
		}
		if p.Limitations != "" {
			b.WriteString("- limitaciones: " + p.Limitations + "\n")
		}
	}

	b.WriteString("\nHISTORIAL RECIENTE:\n")
	if len(workouts) == 0 {
		b.WriteString("(sin entrenos registrados)\n")
	} else {
		byWorkout := map[uuid.UUID][]store.ListSetsByWorkoutIDsRow{}
		for _, st := range sets {
			byWorkout[st.WorkoutID] = append(byWorkout[st.WorkoutID], st)
		}
		for _, w := range workouts {
			b.WriteString("- " + w.Date.Format("2006-01-02"))
			if w.Type != "" {
				b.WriteString(" (" + w.Type + ")")
			}
			b.WriteString(":\n")
			for _, st := range byWorkout[w.ID] {
				b.WriteString("    · " + st.ExerciseName)
				if st.Reps != nil {
					fmt.Fprintf(&b, " %d reps", *st.Reps)
				}
				if st.WeightGrams != nil {
					fmt.Fprintf(&b, " @ %.1f kg", float64(*st.WeightGrams)/1000)
				}
				b.WriteString("\n")
			}
		}
	}

	if strings.TrimSpace(focus) != "" {
		b.WriteString("\nENFOQUE PEDIDO: " + strings.TrimSpace(focus) + "\n")
	}
	return b.String()
}

// ageFrom calcula la edad en años a la fecha `today`.
func ageFrom(birth, today time.Time) int {
	y := today.Year() - birth.Year()
	if today.Month() < birth.Month() || (today.Month() == birth.Month() && today.Day() < birth.Day()) {
		y--
	}
	return y
}
```

- [ ] **Step 3: Rutas + handlers**

En `api/internal/training/handler.go`:

a) Agregar a `Routes` (antes del `return r`):

```go
	r.Get("/suggestion", handleGetSuggestion(svc))
	r.Post("/suggestion", handleSuggest(svc))
```

b) Agregar `"errors"`, `"strings"`, `"unicode/utf8"` a los imports si faltan.

c) Agregar los handlers:

```go
type suggestReq struct {
	Focus string `json:"focus"`
}

func handleGetSuggestion(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		s, err := svc.Suggestion(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		// s puede ser nil -> null (200).
		httpx.WriteJSON(w, http.StatusOK, s)
	}
}

func handleSuggest(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req suggestReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		focus := strings.TrimSpace(req.Focus)
		if utf8.RuneCountInString(focus) > maxFocusChars {
			httpx.WriteErr(w, http.StatusBadRequest, "el enfoque es demasiado largo")
			return
		}
		now := time.Now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		out, err := svc.SuggestTraining(r.Context(), userID, focus, today)
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

- [ ] **Step 4: Wiring en `server.go`**

En `api/internal/server/server.go`: mover la creación de `groq` a antes de `trainingSvc` y pasarla. Reemplazar la línea 38 y eliminar el `groq := ...` duplicado de la línea 63:

```go
	groq := ai.NewGroqClient(d.GroqAPIKey, d.GroqModel, d.GroqVisionModel)
	trainingSvc := training.NewService(q, d.Pool, groq, d.GroqAPIKey != "")
```
…y en el bloque del grupo, **borrar** la línea `groq := ai.NewGroqClient(...)` (ahora ya existe `groq`), dejando intactas las que la usan (`aiSvc`, `chatSvc`, `importSvc`).

> Verificá que el orden de declaración compile (groq se declara una sola vez, antes de su primer uso). `ai` se sigue importando.

- [ ] **Step 5: Fake completer + ajustar `newEnv` + tests**

a) En `api/internal/training/handler_test.go`, agregar un fake completer y parametrizar `newEnv`:

```go
type fakeCompleter struct {
	out        string
	err        error
	lastSystem string
	lastUser   string
}

func (f *fakeCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	f.lastSystem, f.lastUser = system, user
	return f.out, f.err
}
```

Cambiar el `newEnv(t)` actual para que delegue en un helper parametrizado, conservando su firma:

```go
func newEnv(t *testing.T) *env {
	return newEnvWith(t, &fakeCompleter{out: "rutina sugerida"}, true)
}

func newEnvWith(t *testing.T, c *fakeCompleter, hasKey bool) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	tm := auth.NewTokenManager("secret")
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/training", training.Routes(training.NewService(q, pool, c, hasKey)))
	})
	return &env{h: r, auth: auth.NewService(q, tm)}
}
```

> **Detalle de tipo (confirmado):** `handler_test.go` es `package training_test` (externo) y `completer` es **no exportada** en `training`. Por eso el parámetro es el tipo concreto **`*fakeCompleter`** (que satisface la interfaz estructuralmente al pasarlo a `NewService`). El test ya importa `context`/`encoding/json`/`strings`; agregá **`errors`** (lo usa `TestSuggestGroqErrorIs503`).

b) Crear `api/internal/training/suggestion_test.go` (mismo package que `handler_test.go`) con los casos:

```go
func TestSuggestHappyPathPersists(t *testing.T) {
	c := &fakeCompleter{out: "Hacé sentadillas 4x8…"}
	e := newEnvWith(t, c, true)
	tok := e.token(t, "sug@b.com")

	// guardar un perfil y un entreno para que entren al contexto
	do(t, e.h, http.MethodPut, "/training/profile", tok, map[string]any{"objective": "hipertrofia", "equipment": []string{"mancuernas"}})
	do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-15", "type": "Pierna",
		"sets": []map[string]any{{"exercise": "Sentadilla", "reps": 8, "weight_grams": 80000}},
	})

	rec := do(t, e.h, http.MethodPost, "/training/suggestion", tok, map[string]any{"focus": "pierna"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST suggestion code = %d, body=%s", rec.Code, rec.Body.String())
	}
	var s map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s["content"] != "Hacé sentadillas 4x8…" || s["focus"] != "pierna" {
		t.Fatalf("sugerencia = %+v", s)
	}
	// el prompt incluyó perfil, historial y enfoque
	for _, want := range []string{"hipertrofia", "Sentadilla", "pierna", "mancuernas"} {
		if !strings.Contains(c.lastUser, want) {
			t.Errorf("el contexto del prompt no contiene %q:\n%s", want, c.lastUser)
		}
	}

	// GET devuelve la última
	rec = do(t, e.h, http.MethodGet, "/training/suggestion", tok, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s["content"] != "Hacé sentadillas 4x8…" {
		t.Fatalf("GET suggestion = %+v", s)
	}
}

func TestGetSuggestionEmpty(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "sugempty@b.com")
	rec := do(t, e.h, http.MethodGet, "/training/suggestion", tok, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("GET vacío = %d %q", rec.Code, rec.Body.String())
	}
}

func TestSuggestNoKeyIs503(t *testing.T) {
	e := newEnvWith(t, &fakeCompleter{out: "x"}, false) // hasKey=false
	tok := e.token(t, "sugnokey@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/suggestion", tok, map[string]any{})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin clave code = %d, want 503", rec.Code)
	}
}

func TestSuggestGroqErrorIs503(t *testing.T) {
	e := newEnvWith(t, &fakeCompleter{err: errors.New("groq caído")}, true)
	tok := e.token(t, "sugerr@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/suggestion", tok, map[string]any{})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("error groq code = %d, want 503", rec.Code)
	}
}

func TestSuggestFocusTooLong(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "suglong@b.com")
	long := strings.Repeat("a", 201)
	rec := do(t, e.h, http.MethodPost, "/training/suggestion", tok, map[string]any{"focus": long})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("focus largo code = %d, want 400", rec.Code)
	}
}
```

> Asegurá los imports del test (`context`, `errors`, `strings`, `encoding/json`, `net/http`). Si `handler_test.go` ya importa algunos, no dupliques.

- [ ] **Step 6: Verificar build + vet + suite**

Run:
```
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
```
Expected: todo verde. Iterar hasta verde.

- [ ] **Step 7: Commit**

```bash
git add api/internal/training api/internal/server/server.go
git commit -m "feat(training): entrenador IA — GET/POST /training/suggestion

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Frontend — lib `trainingSuggestion.ts`

**Files:**
- Create: `web/src/lib/trainingSuggestion.ts`
- Create: `web/src/lib/trainingSuggestion.test.ts`

- [ ] **Step 1: Test (que falla)**

Crear `web/src/lib/trainingSuggestion.test.ts`. Tipá los mocks `(_url: string, _opts?: RequestInit)`.

```ts
import { describe, it, expect, vi, afterEach } from "vitest";
import { getSuggestion, generateSuggestion } from "./trainingSuggestion";

afterEach(() => vi.restoreAllMocks());

describe("trainingSuggestion", () => {
  it("getSuggestion devuelve null cuando no hay", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response("null", { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    expect(await getSuggestion()).toBeNull();
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/training/suggestion");
  });

  it("generateSuggestion hace POST con el enfoque", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ focus: "pierna", content: "rutina", created_at: "" }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    const s = await generateSuggestion("pierna");
    expect(s.content).toBe("rutina");
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("POST");
    expect(String(opts.body)).toContain("pierna");
  });
});
```

- [ ] **Step 2: Verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/trainingSuggestion.test.ts`
Expected: FAIL.

- [ ] **Step 3: Implementar `web/src/lib/trainingSuggestion.ts`**

```ts
import { apiFetch } from "./api";

export type TrainingSuggestion = {
  focus: string;
  content: string;
  created_at: string;
};

export function getSuggestion(): Promise<TrainingSuggestion | null> {
  return apiFetch<TrainingSuggestion | null>("/api/v1/training/suggestion");
}

export function generateSuggestion(focus: string): Promise<TrainingSuggestion> {
  return apiFetch<TrainingSuggestion>("/api/v1/training/suggestion", {
    method: "POST",
    body: JSON.stringify({ focus }),
  });
}
```

- [ ] **Step 4: Verde + build (typecheck)**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/trainingSuggestion.test.ts && npm run build`
Expected: tests PASS y build OK. (Correr `npm run build` acá es obligatorio para atajar typecheck antes de la Task 4.)

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/trainingSuggestion.ts web/src/lib/trainingSuggestion.test.ts
git commit -m "feat(web): lib del entrenador IA (get/generate suggestion)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Frontend — panel "Entrenador IA" en `entrenamiento.tsx`

**Files:**
- Modify: `web/src/routes/entrenamiento.tsx`
- Modify: `web/src/routes/entrenamiento.test.tsx`

- [ ] **Step 1: Test (que falla)**

En `web/src/routes/entrenamiento.test.tsx`, mockeá `@/lib/trainingSuggestion` con `vi.mock` y agregá un test: hay un campo de enfoque y un botón "Sugerir"; al tocar "Sugerir", se llama `generateSuggestion` y se muestra el `content` devuelto. Scopeá si hace falta para no chocar con otros botones. Adaptá al harness real (cómo siembra `listWorkouts`/`listExercises`).

```tsx
// vi.mock("@/lib/trainingSuggestion", () => ({
//   getSuggestion: vi.fn(async () => null),
//   generateSuggestion: vi.fn(async () => ({ focus: "", content: "Hacé sentadillas 4x8", created_at: "" })),
// }));
// ...
// await userEvent.click(await screen.findByRole("button", { name: "Sugerir" }));
// expect(await screen.findByText(/Hacé sentadillas 4x8/)).toBeInTheDocument();
```

- [ ] **Step 2: Imports + estado**

En `web/src/routes/entrenamiento.tsx`:

```tsx
import { getSuggestion, generateSuggestion, type TrainingSuggestion } from "@/lib/trainingSuggestion";
```
(El `Card`, `Button`, `Input`, `useQuery`, `useMutation`, `useQueryClient`, `useState` ya están.)

Dentro del componente de la página, junto a los otros `useState`:

```tsx
  const [focus, setFocus] = useState("");
  const [suggestError, setSuggestError] = useState<string | null>(null);
```

- [ ] **Step 3: Query + mutación**

Dentro del componente (junto a las otras queries/mutaciones):

```tsx
  const suggestionQuery = useQuery({
    queryKey: ["training-suggestion"],
    queryFn: getSuggestion,
    enabled: !!user,
  });
  const suggestMutation = useMutation({
    mutationFn: () => generateSuggestion(focus.trim()),
    onSuccess: (s) => {
      setSuggestError(null);
      qc.setQueryData<TrainingSuggestion | null>(["training-suggestion"], s);
    },
    onError: (e) =>
      setSuggestError(e instanceof Error ? e.message : "No se pudo generar la sugerencia"),
  });
```
(`qc` ya existe como `useQueryClient()`. `user` ya está de `useAuth()`.)

- [ ] **Step 4: Panel "Entrenador IA"**

Agregar una `Card` arriba del historial (después del form de registro, antes de `<section>` "Historial"):

```tsx
        <Card className="mt-8 p-6 space-y-3">
          <h2 className="font-display text-lg font-bold tracking-tight">Entrenador IA</h2>
          <div className="flex gap-2">
            <Input
              type="text"
              aria-label="Enfoque"
              placeholder="enfoque opcional: pierna, 30 min, sin saltos…"
              value={focus}
              onChange={(e) => setFocus(e.target.value)}
              className="flex-1"
            />
            <Button
              type="button"
              onClick={() => suggestMutation.mutate()}
              disabled={suggestMutation.isPending}
              className="px-3 py-1 text-xs"
            >
              {suggestMutation.isPending ? "Generando…" : "Sugerir"}
            </Button>
          </div>
          {suggestError && (
            <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-xs font-bold text-danger-fg shadow-brutal-sm">
              {suggestError}
            </p>
          )}
          {suggestionQuery.data ? (
            <div className="rounded-lg border-2 border-ink bg-surface px-3 py-2 shadow-brutal-sm">
              <p className="whitespace-pre-wrap text-sm">{suggestionQuery.data.content}</p>
              {suggestionQuery.data.created_at && (
                <p className="mt-2 text-[10px] uppercase tracking-[0.12em] text-muted">
                  {suggestionQuery.data.focus ? `enfoque: ${suggestionQuery.data.focus} · ` : ""}
                  {relativeDateTraining(suggestionQuery.data.created_at)}
                </p>
              )}
            </div>
          ) : (
            !suggestMutation.isPending && (
              <p className="text-sm text-muted">Pedí una sugerencia de entrenamiento.</p>
            )
          )}
        </Card>
```

Agregar el helper de fecha relativa al final del archivo (si ya hay uno en este archivo, reusalo en vez de duplicar):

```tsx
function relativeDateTraining(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  const days = Math.floor((Date.now() - d.getTime()) / 86_400_000);
  if (days <= 0) return "hoy";
  if (days < 7) return `${days}d`;
  return d.toLocaleDateString();
}
```

- [ ] **Step 5: Verde + suite + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build`
Expected: todo verde; build OK (typecheck incluido).

- [ ] **Step 6: Commit**

```bash
git add web/src/routes/entrenamiento.tsx web/src/routes/entrenamiento.test.tsx
git commit -m "feat(web): panel Entrenador IA en entrenamiento

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Cierre — review, merge y smoke

**Files:** verificación + `scripts/smoke-r25.sh` + bitácora.

- [ ] **Step 1: Review final** del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-17-plan-25-entrenador-ia-design.md`. Aplicar nits.

- [ ] **Step 2: Suites verdes**
Backend: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && TEST_DATABASE_URL=... go test -p 1 ./... -count=1`
Frontend: `cd web && npx vitest run && npm run build`

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`.

- [ ] **Step 4: Deploy manual (Coolify) + smoke.** Crear `scripts/smoke-r25.sh` (patrón de `scripts/smoke-r24.sh`, extraer con `grep -o`): `GET /training/suggestion` → `null`; `POST /training/suggestion` con `{"focus":"pierna"}` → 200 con `content` no vacío (la IA real responde en prod); `GET` de nuevo → la misma sugerencia; `POST` con un `focus` de 201 chars → 400. (Groq real en prod puede tardar unos segundos; dar timeout holgado.)

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-17-sesion-plan-25-entrenador-ia.md` (slice B; C/D pendientes).

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 modelo (training_suggestions 1:1) → Task 1. ✓
- §3 backend (Get/Upsert; completer interface; Suggestion/SuggestTraining con perfil+historial+enfoque; system prompt; ErrUnavailable; rutas GET/POST; validación focus ≤200; wiring groq) → Task 1 (queries) + Task 2 (servicio+handler+server). ✓
- §4 frontend (lib get/generate; panel con enfoque+botón+render pre-wrap+precarga) → Task 3 (lib) + Task 4 (UI). ✓
- §5 errores (503 sin clave/fallo; 400 focus largo; perfil/historial vacío genera igual; ownership PK) → Task 2. ✓
- §6 testing → Tasks 1–4; E2E → Task 5. ✓
- §7 aceptación → smoke Task 5. ✓

**Placeholders:** los «ajustá al package del test / al harness real de entrenamiento.test» son adaptaciones deterministas con instrucción de qué inspeccionar. El detalle del tipo del parámetro de `newEnvWith` (usar `*fakeCompleter`) está resuelto explícitamente.

**Consistencia de tipos/firmas:** store `UpsertTrainingSuggestionParams{UserID,Focus,Content}`/`TrainingSuggestion` ↔ servicio `Suggestion`/`SuggestTraining(userID,focus,today)`/`Suggestion(userID)`/`buildSuggestion` ↔ handler `suggestReq`/rutas ↔ endpoints `GET/POST /training/suggestion` ↔ lib `TrainingSuggestion`/`getSuggestion`/`generateSuggestion`. `NewService(q,pool,groq,hasKey)` actualizado en server.go y en newEnv. `completer` definido en training (sin importar ai). ✓

**Lección aplicada (R21/R23):** la Task 3 (lib) corre `npm run build` además del test.
