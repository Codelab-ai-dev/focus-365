# Perfil de fitness — Plan de implementación (Entrenamiento slice A, Rebanada 24)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un perfil de fitness por usuario (edad/fecha de nacimiento, peso, altura, sexo, objetivo, equipo, lugar, nivel, frecuencia, limitaciones) editable desde un modal en `/entrenamiento`.

**Architecture:** Nueva tabla `fitness_profiles` (1:1 con users, PK = user_id). Endpoints `GET/PUT /training/profile` (upsert) en el paquete `training`. Frontend: `lib/fitnessProfile.ts` + un `ProfileModal` (reusa `ui/Modal.tsx`) abierto desde un botón "Mi perfil". Solo CRUD del perfil; el consumo por la IA es de otro slice. Cambios additivos: el build se mantiene verde entre tasks.

**Tech Stack:** Go (chi, sqlc, pgx/v5, goose), Postgres, React + Vite + TanStack Query/Router + Vitest.

**Contexto del repo (leer antes de empezar):**
- sqlc: `uuid`→`uuid.UUID`, `date`/`timestamptz`→`time.Time`, columnas **nullable → puntero** (`*time.Time`, `*string`, `*int32`); `text[]`→`[]string`. Tras editar SQL: `cd api && sqlc generate` (sqlc en `/opt/homebrew/bin/sqlc`).
- `testutil.NewDB(t)` aplica todas las migraciones (incl. la nueva 0020). DB dev/test en `localhost:5544`. Comandos Go: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" <cmd>`, `TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`. **Correr la suite con `-p 1`** (la DB de test deadlockea en paralelo).
- `training.Service` tiene `q *store.Queries` y `pool *pgxpool.Pool` (`NewService(q, pool)`). Rutas en `training.Routes(svc)` ya bajo `RequireAuth`, montadas en `/training`.
- `httpx.DecodeAndValidate(w, r, &req) bool` decodifica y valida tags `validate:`.
- Front: `lib/training.ts` exporta `kgToGrams(kg)` y `gramsToKg(grams)` (reusarlos para el peso). `ui/Modal.tsx` = `Modal({open, onClose, title, children})`. Helper de tests del store `newUser(t, q)` en `api/internal/store/ai_threads_test.go` (`package store_test`).
- Última migración: `0019_goal_notes.sql` → la nueva es `0020`.

**Refinamiento sobre el spec:** la API usa `weight_grams` y `height_cm` (enteros) en el body y en la vista —consistente con el flujo de workouts, que ya convierte con `kgToGrams`/`gramsToKg` en el front— en vez de convertir en el backend. Comportamiento de usuario idéntico.

---

## Estructura de archivos

**Backend**
- Crear `api/db/migrations/0020_fitness_profiles.sql`.
- Crear `api/db/queries/fitness_profiles.sql`.
- Regenerar `api/internal/store/*` (sqlc).
- Crear `api/internal/store/fitness_profiles_test.go`.
- Crear `api/internal/training/profile.go` — vista `Profile`, `ProfileInput`, `buildProfile`, métodos `Profile`/`SaveProfile`.
- Modificar `api/internal/training/handler.go` — rutas + handlers `GET/PUT /profile`.
- Modificar `api/internal/training/handler_test.go` — tests de los endpoints.

**Frontend**
- Crear `web/src/lib/fitnessProfile.ts` + `web/src/lib/fitnessProfile.test.ts`.
- Modificar `web/src/routes/entrenamiento.tsx` — botón "Mi perfil" + `ProfileModal`.
- Modificar `web/src/routes/entrenamiento.test.tsx` — test del modal de perfil.

---

## Task 1: Migración 0020 + queries + tests de store

**Files:**
- Create: `api/db/migrations/0020_fitness_profiles.sql`
- Create: `api/db/queries/fitness_profiles.sql`
- Create: `api/internal/store/fitness_profiles_test.go`
- Regenerate: `api/internal/store/`

- [ ] **Step 1: Migración**

Crear `api/db/migrations/0020_fitness_profiles.sql`:

```sql
-- +goose Up
CREATE TABLE fitness_profiles (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    birthdate    DATE,
    sex          TEXT,
    height_cm    INT,
    weight_grams INT,
    objective    TEXT,
    location     TEXT,
    level        TEXT,
    weekly_days  INT,
    equipment    TEXT[] NOT NULL DEFAULT '{}',
    limitations  TEXT NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE fitness_profiles;
```

- [ ] **Step 2: Queries**

Crear `api/db/queries/fitness_profiles.sql`:

```sql
-- name: GetFitnessProfile :one
SELECT * FROM fitness_profiles WHERE user_id = $1;

-- name: UpsertFitnessProfile :one
INSERT INTO fitness_profiles (
    user_id, birthdate, sex, height_cm, weight_grams, objective,
    location, level, weekly_days, equipment, limitations, updated_at
) VALUES (
    @user_id, @birthdate, @sex, @height_cm, @weight_grams, @objective,
    @location, @level, @weekly_days, @equipment, @limitations, now()
)
ON CONFLICT (user_id) DO UPDATE SET
    birthdate    = EXCLUDED.birthdate,
    sex          = EXCLUDED.sex,
    height_cm    = EXCLUDED.height_cm,
    weight_grams = EXCLUDED.weight_grams,
    objective    = EXCLUDED.objective,
    location     = EXCLUDED.location,
    level        = EXCLUDED.level,
    weekly_days  = EXCLUDED.weekly_days,
    equipment    = EXCLUDED.equipment,
    limitations  = EXCLUDED.limitations,
    updated_at   = now()
RETURNING *;
```

- [ ] **Step 3: Regenerar sqlc**

Run: `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`
Verificá: `grep -n "FitnessProfile\|UpsertFitnessProfileParams\|type FitnessProfile" internal/store/*.go`
Esperado: modelo `FitnessProfile{ UserID uuid.UUID; Birthdate *time.Time; Sex *string; HeightCm *int32; WeightGrams *int32; Objective *string; Location *string; Level *string; WeeklyDays *int32; Equipment []string; Limitations string; UpdatedAt time.Time }`; `UpsertFitnessProfileParams` con esos mismos campos; `GetFitnessProfile(ctx, userID) (FitnessProfile, error)`. **Si algún tipo difiere (p. ej. `pgtype.Int4` en vez de `*int32`), usá el tipo real generado en las tasks siguientes y reportalo.**

- [ ] **Step 4: Test de store (que falla)**

Crear `api/internal/store/fitness_profiles_test.go`. Reusá `newUser`.

```go
package store_test

import (
	"context"
	"testing"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func ptrI(v int32) *int32   { return &v }
func ptrS(v string) *string { return &v }

func TestFitnessProfileUpsertAndGet(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)

	// no existe -> ErrNoRows
	if _, err := q.GetFitnessProfile(ctx, u); err != pgx.ErrNoRows {
		t.Fatalf("perfil inexistente: err = %v, want ErrNoRows", err)
	}

	// insert
	p, err := q.UpsertFitnessProfile(ctx, store.UpsertFitnessProfileParams{
		UserID: u, Sex: ptrS("masculino"), HeightCm: ptrI(178),
		WeightGrams: ptrI(80500), Objective: ptrS("hipertrofia"),
		Location: ptrS("casa"), Level: ptrS("intermedio"), WeeklyDays: ptrI(4),
		Equipment: []string{"mancuernas", "bandas"}, Limitations: "cuido la rodilla",
	})
	if err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	if p.Sex == nil || *p.Sex != "masculino" || len(p.Equipment) != 2 || p.Limitations != "cuido la rodilla" {
		t.Fatalf("perfil insertado = %+v", p)
	}

	// update (mismo user) -> sigue una sola fila, valores nuevos
	p2, err := q.UpsertFitnessProfile(ctx, store.UpsertFitnessProfileParams{
		UserID: u, Objective: ptrS("fuerza"), WeeklyDays: ptrI(5),
		Equipment: []string{"barra"}, Limitations: "",
	})
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if p2.Objective == nil || *p2.Objective != "fuerza" || len(p2.Equipment) != 1 || p2.Equipment[0] != "barra" {
		t.Fatalf("perfil actualizado = %+v", p2)
	}
	// los campos no pasados quedan en NULL tras el upsert (reemplazo completo)
	if p2.Sex != nil {
		t.Errorf("sex debería ser NULL tras el reemplazo, got %v", *p2.Sex)
	}

	// get devuelve el actualizado
	got, err := q.GetFitnessProfile(ctx, u)
	if err != nil || got.Objective == nil || *got.Objective != "fuerza" {
		t.Fatalf("get tras update: %v %+v", err, got)
	}
}
```

- [ ] **Step 5: Correr (falla→pasa)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ -run TestFitnessProfile -v`
Expected: PASS. El build completo sigue verde (additivo).

- [ ] **Step 6: Commit**

```bash
git add api/db/migrations/0020_fitness_profiles.sql api/db/queries/fitness_profiles.sql api/internal/store
git commit -m "feat(store): tabla fitness_profiles + upsert (migración 0020)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Servicio + endpoints `GET/PUT /training/profile`

**Files:**
- Create: `api/internal/training/profile.go`
- Modify: `api/internal/training/handler.go`
- Modify: `api/internal/training/handler_test.go`

- [ ] **Step 1: `profile.go` (vista, input, servicio)**

Crear `api/internal/training/profile.go`:

```go
package training

import (
	"context"
	"errors"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const profileDateLayout = "2006-01-02"

// Profile es la vista del perfil de fitness. birthdate va como YYYY-MM-DD; los
// opcionales no seteados van como null. weight_grams/height_cm en enteros (el
// front convierte kg/cm con sus helpers).
type Profile struct {
	Birthdate   *string   `json:"birthdate"`
	Sex         *string   `json:"sex"`
	HeightCm    *int32    `json:"height_cm"`
	WeightGrams *int32    `json:"weight_grams"`
	Objective   *string   `json:"objective"`
	Location    *string   `json:"location"`
	Level       *string   `json:"level"`
	WeeklyDays  *int32    `json:"weekly_days"`
	Equipment   []string  `json:"equipment"`
	Limitations string    `json:"limitations"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProfileInput son los datos de dominio para guardar el perfil.
type ProfileInput struct {
	Birthdate   *time.Time
	Sex         *string
	HeightCm    *int32
	WeightGrams *int32
	Objective   *string
	Location    *string
	Level       *string
	WeeklyDays  *int32
	Equipment   []string
	Limitations string
}

func buildProfile(p store.FitnessProfile) Profile {
	var bd *string
	if p.Birthdate != nil {
		s := p.Birthdate.Format(profileDateLayout)
		bd = &s
	}
	eq := p.Equipment
	if eq == nil {
		eq = []string{}
	}
	return Profile{
		Birthdate: bd, Sex: p.Sex, HeightCm: p.HeightCm, WeightGrams: p.WeightGrams,
		Objective: p.Objective, Location: p.Location, Level: p.Level,
		WeeklyDays: p.WeeklyDays, Equipment: eq, Limitations: p.Limitations,
		UpdatedAt: p.UpdatedAt,
	}
}

// Profile devuelve el perfil del usuario, o nil si nunca lo guardó.
func (s *Service) Profile(ctx context.Context, userID uuid.UUID) (*Profile, error) {
	p, err := s.q.GetFitnessProfile(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v := buildProfile(p)
	return &v, nil
}

// SaveProfile hace upsert del perfil y devuelve el resultado.
func (s *Service) SaveProfile(ctx context.Context, userID uuid.UUID, in ProfileInput) (*Profile, error) {
	eq := in.Equipment
	if eq == nil {
		eq = []string{}
	}
	p, err := s.q.UpsertFitnessProfile(ctx, store.UpsertFitnessProfileParams{
		UserID: userID, Birthdate: in.Birthdate, Sex: in.Sex, HeightCm: in.HeightCm,
		WeightGrams: in.WeightGrams, Objective: in.Objective, Location: in.Location,
		Level: in.Level, WeeklyDays: in.WeeklyDays, Equipment: eq, Limitations: in.Limitations,
	})
	if err != nil {
		return nil, err
	}
	v := buildProfile(p)
	return &v, nil
}
```

> Si sqlc generó tipos distintos de `*int32`/`*string` (p. ej. `pgtype.Int4`), adaptá los campos de `Profile`/`ProfileInput`/`buildProfile` a esos tipos.

- [ ] **Step 2: Handlers + rutas**

En `api/internal/training/handler.go`:

a) Agregar a `Routes` (antes del `return r`):

```go
	r.Get("/profile", handleGetProfile(svc))
	r.Put("/profile", handleSaveProfile(svc))
```

b) Agregar `"errors"` al bloque de imports (ya están `net/http`, `time`, `auth`, `httpx`, `chi`, `uuid`).

c) Agregar el body, la validación y los handlers:

```go
type profileReq struct {
	Birthdate   *string  `json:"birthdate"`
	Sex         *string  `json:"sex" validate:"omitempty,oneof=masculino femenino otro"`
	HeightCm    *int32   `json:"height_cm" validate:"omitempty,min=1"`
	WeightGrams *int32   `json:"weight_grams" validate:"omitempty,min=1"`
	Objective   *string  `json:"objective" validate:"omitempty,oneof=perder_grasa hipertrofia fuerza resistencia salud"`
	Location    *string  `json:"location" validate:"omitempty,oneof=casa gym ambos"`
	Level       *string  `json:"level" validate:"omitempty,oneof=principiante intermedio avanzado"`
	WeeklyDays  *int32   `json:"weekly_days" validate:"omitempty,min=1,max=7"`
	Equipment   []string `json:"equipment" validate:"omitempty,dive,oneof=peso_corporal mancuernas barra banco bandas kettlebell dominadas gym"`
	Limitations *string  `json:"limitations"`
}

func handleGetProfile(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		p, err := svc.Profile(r.Context(), userID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		// p puede ser nil -> se serializa como null (200).
		httpx.WriteJSON(w, http.StatusOK, p)
	}
}

func handleSaveProfile(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		var req profileReq
		if !httpx.DecodeAndValidate(w, r, &req) {
			return
		}
		var birthdate *time.Time
		if req.Birthdate != nil && *req.Birthdate != "" {
			d, err := time.Parse(profileDateLayout, *req.Birthdate)
			if err != nil {
				httpx.WriteErr(w, http.StatusBadRequest, "la fecha de nacimiento no tiene un formato válido (YYYY-MM-DD)")
				return
			}
			birthdate = &d
		}
		limitations := ""
		if req.Limitations != nil {
			limitations = *req.Limitations
		}
		out, err := svc.SaveProfile(r.Context(), userID, ProfileInput{
			Birthdate: birthdate, Sex: req.Sex, HeightCm: req.HeightCm,
			WeightGrams: req.WeightGrams, Objective: req.Objective, Location: req.Location,
			Level: req.Level, WeeklyDays: req.WeeklyDays, Equipment: req.Equipment,
			Limitations: limitations,
		})
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}
```

- [ ] **Step 3: Tests de handler (que fallan)**

En `api/internal/training/handler_test.go`, agregar (usan `newEnv`/`token`/`do` existentes):

```go
func TestProfileGetEmptyThenSave(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "prof@b.com")

	// sin perfil -> 200 null
	rec := do(t, e.h, http.MethodGet, "/training/profile", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET vacío code = %d", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "null" {
		t.Fatalf("GET vacío body = %q, want null", body)
	}

	// guardar
	rec = do(t, e.h, http.MethodPut, "/training/profile", tok, map[string]any{
		"sex": "masculino", "height_cm": 178, "weight_grams": 80500,
		"objective": "hipertrofia", "location": "casa", "level": "intermedio",
		"weekly_days": 4, "equipment": []string{"mancuernas", "bandas"},
		"limitations": "cuido la rodilla", "birthdate": "1990-05-01",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT code = %d, body=%s", rec.Code, rec.Body.String())
	}
	var p map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p["objective"] != "hipertrofia" || p["birthdate"] != "1990-05-01" {
		t.Fatalf("perfil guardado = %+v", p)
	}

	// GET ahora devuelve el perfil
	rec = do(t, e.h, http.MethodGet, "/training/profile", tok, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p["weight_grams"].(float64) != 80500 {
		t.Fatalf("weight_grams = %v", p["weight_grams"])
	}
}

func TestProfileValidation(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "profval@b.com")
	cases := []map[string]any{
		{"sex": "x"},                         // enum inválido
		{"weekly_days": 8},                    // fuera de rango
		{"weight_grams": -1},                  // negativo
		{"objective": "ganar"},                // enum inválido
		{"equipment": []string{"cohete"}},     // item inválido
		{"birthdate": "01/05/1990"},           // fecha mal formada
	}
	for i, c := range cases {
		rec := do(t, e.h, http.MethodPut, "/training/profile", tok, c)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("caso %d (%v): code = %d, want 400", i, c, rec.Code)
		}
	}
}

func TestProfileIsolation(t *testing.T) {
	e := newEnv(t)
	a := e.token(t, "pa@b.com")
	b := e.token(t, "pb@b.com")
	do(t, e.h, http.MethodPut, "/training/profile", a, map[string]any{"objective": "fuerza"})
	rec := do(t, e.h, http.MethodGet, "/training/profile", b, nil)
	if strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("el usuario B vio un perfil ajeno: %s", rec.Body.String())
	}
}
```

> Asegurate de que `handler_test.go` importe `strings` y `encoding/json` (probablemente ya). Si no, agregalos.

- [ ] **Step 4: Verificar build + tests + suite completa**

Run:
```
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
```
Expected: todo verde.

- [ ] **Step 5: Commit**

```bash
git add api/internal/training
git commit -m "feat(training): endpoints GET/PUT /training/profile (perfil de fitness)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Frontend — lib `fitnessProfile.ts`

**Files:**
- Create: `web/src/lib/fitnessProfile.ts`
- Create: `web/src/lib/fitnessProfile.test.ts`

- [ ] **Step 1: Test (que falla)**

Crear `web/src/lib/fitnessProfile.test.ts`. **Importante:** tipá los mocks de fetch con `(_url: string, _opts?: RequestInit)` para que el typecheck (`npm run build`) no rompa al indexar `mock.calls`.

```ts
import { describe, it, expect, vi, afterEach } from "vitest";
import { getProfile, saveProfile } from "./fitnessProfile";

afterEach(() => vi.restoreAllMocks());

describe("fitnessProfile", () => {
  it("getProfile devuelve null cuando el backend responde null", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response("null", { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    expect(await getProfile()).toBeNull();
    expect(String(fetchMock.mock.calls[0][0])).toContain("/api/v1/training/profile");
  });

  it("getProfile devuelve el perfil", async () => {
    const prof = { birthdate: "1990-05-01", sex: "masculino", height_cm: 178, weight_grams: 80500, objective: "hipertrofia", location: "casa", level: "intermedio", weekly_days: 4, equipment: ["mancuernas"], limitations: "", updated_at: "" };
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify(prof), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    const p = await getProfile();
    expect(p?.objective).toBe("hipertrofia");
  });

  it("saveProfile hace PUT con el body", async () => {
    const fetchMock = vi.fn((_url: string, _opts?: RequestInit) =>
      Promise.resolve(new Response(JSON.stringify({ equipment: [], limitations: "", updated_at: "" }), { status: 200 }))
    );
    vi.stubGlobal("fetch", fetchMock);
    await saveProfile({ objective: "fuerza", weekly_days: 5, equipment: ["barra"], limitations: "" });
    const opts = fetchMock.mock.calls[0][1] as RequestInit;
    expect(opts.method).toBe("PUT");
    expect(String(opts.body)).toContain("fuerza");
  });
});
```

- [ ] **Step 2: Verlo fallar**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/fitnessProfile.test.ts`
Expected: FAIL (módulo no existe).

- [ ] **Step 3: Implementar `web/src/lib/fitnessProfile.ts`**

```ts
import { apiFetch } from "./api";

export type FitnessProfile = {
  birthdate: string | null;
  sex: string | null;
  height_cm: number | null;
  weight_grams: number | null;
  objective: string | null;
  location: string | null;
  level: string | null;
  weekly_days: number | null;
  equipment: string[];
  limitations: string;
  updated_at: string;
};

// Lo que se envía al guardar: todos los campos opcionales (null/ausente = limpiar).
export type FitnessProfileInput = {
  birthdate?: string | null;
  sex?: string | null;
  height_cm?: number | null;
  weight_grams?: number | null;
  objective?: string | null;
  location?: string | null;
  level?: string | null;
  weekly_days?: number | null;
  equipment?: string[];
  limitations?: string;
};

export function getProfile(): Promise<FitnessProfile | null> {
  return apiFetch<FitnessProfile | null>("/api/v1/training/profile");
}

export function saveProfile(input: FitnessProfileInput): Promise<FitnessProfile> {
  return apiFetch<FitnessProfile>("/api/v1/training/profile", {
    method: "PUT",
    body: JSON.stringify(input),
  });
}
```

- [ ] **Step 4: Verde + build (typecheck)**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/fitnessProfile.test.ts && npm run build`
Expected: tests PASS y build OK. (Correr `npm run build` acá evita que un error de typecheck en el test se filtre a la Task 4.)

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/fitnessProfile.ts web/src/lib/fitnessProfile.test.ts
git commit -m "feat(web): lib del perfil de fitness (get/save)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Frontend — botón "Mi perfil" + `ProfileModal` en `entrenamiento.tsx`

**Files:**
- Modify: `web/src/routes/entrenamiento.tsx`
- Modify: `web/src/routes/entrenamiento.test.tsx`

- [ ] **Step 1: Test (que falla)**

Leé `web/src/routes/entrenamiento.test.tsx` (harness: mock de `@/lib/auth`, stub de fetch, router en memoria). Mockeá `@/lib/fitnessProfile` con `vi.mock` y agregá un test: hay un botón "Mi perfil"; al tocarlo se abre el modal y, con un perfil mockeado (`getProfile` devuelve objetivo "hipertrofia"), el select de objetivo queda en ese valor; cambiar y "Guardar" llama `saveProfile`. Escribí los `expect` concretos adaptados al harness real (cómo siembra `listWorkouts`/`listExercises`).

```tsx
// vi.mock("@/lib/fitnessProfile", () => ({
//   getProfile: vi.fn(async () => ({ birthdate: null, sex: null, height_cm: null, weight_grams: null, objective: "hipertrofia", location: "casa", level: null, weekly_days: 4, equipment: ["mancuernas"], limitations: "", updated_at: "" })),
//   saveProfile: vi.fn(async () => ({ equipment: [], limitations: "", updated_at: "" })),
// }));
// ...
// await userEvent.click(await screen.findByRole("button", { name: "Mi perfil" }));
// expect(await screen.findByLabelText("Objetivo")).toHaveValue("hipertrofia");
```

- [ ] **Step 2: Imports + estado en `entrenamiento.tsx`**

Agregar imports:

```tsx
import { Modal } from "@/ui/Modal";
import { getProfile, saveProfile, type FitnessProfile } from "@/lib/fitnessProfile";
import { gramsToKg, kgToGrams } from "@/lib/training"; // si no están ya importados de "@/lib/training"
```
(`useQuery`/`useMutation`/`useQueryClient`/`useState` ya están. `kgToGrams`/`gramsToKg` puede que ya estén en el import de `@/lib/training` — agregalos a ese import existente en vez de duplicarlo.)

Dentro del componente de la página, junto a los otros `useState`:

```tsx
  const [profileOpen, setProfileOpen] = useState(false);
```

- [ ] **Step 3: Botón "Mi perfil" en el header**

En el `<header>` (junto al `<Link>` "Volver"), agregar antes o después del link:

```tsx
          <Button
            type="button"
            variant="ghost"
            onClick={() => setProfileOpen(true)}
            className="px-3 py-1 text-xs"
          >
            Mi perfil
          </Button>
```

- [ ] **Step 4: Montar el modal**

Antes de cerrar el contenedor de la página, agregar:

```tsx
        <ProfileModal open={profileOpen} onClose={() => setProfileOpen(false)} />
```

- [ ] **Step 5: Implementar `ProfileModal`**

Agregar al final de `entrenamiento.tsx`. Listas de enums con etiquetas legibles:

```tsx
const SEXES = [
  { value: "masculino", label: "Masculino" },
  { value: "femenino", label: "Femenino" },
  { value: "otro", label: "Otro" },
];
const OBJECTIVES = [
  { value: "perder_grasa", label: "Perder grasa" },
  { value: "hipertrofia", label: "Hipertrofia" },
  { value: "fuerza", label: "Fuerza" },
  { value: "resistencia", label: "Resistencia" },
  { value: "salud", label: "Salud general" },
];
const LOCATIONS = [
  { value: "casa", label: "Casa" },
  { value: "gym", label: "Gimnasio" },
  { value: "ambos", label: "Ambos" },
];
const LEVELS = [
  { value: "principiante", label: "Principiante" },
  { value: "intermedio", label: "Intermedio" },
  { value: "avanzado", label: "Avanzado" },
];
const EQUIPMENT = [
  { value: "peso_corporal", label: "Peso corporal" },
  { value: "mancuernas", label: "Mancuernas" },
  { value: "barra", label: "Barra" },
  { value: "banco", label: "Banco" },
  { value: "bandas", label: "Bandas" },
  { value: "kettlebell", label: "Kettlebell" },
  { value: "dominadas", label: "Barra de dominadas" },
  { value: "gym", label: "Gimnasio completo" },
];

function ProfileModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const profileQuery = useQuery({
    queryKey: ["fitness-profile"],
    queryFn: getProfile,
    enabled: open,
  });

  const [birthdate, setBirthdate] = useState("");
  const [sex, setSex] = useState("");
  const [heightCm, setHeightCm] = useState("");
  const [weightKg, setWeightKg] = useState("");
  const [objective, setObjective] = useState("");
  const [location, setLocation] = useState("");
  const [level, setLevel] = useState("");
  const [weeklyDays, setWeeklyDays] = useState("");
  const [equipment, setEquipment] = useState<string[]>([]);
  const [limitations, setLimitations] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Precarga el form cuando llega el perfil (o lo deja en blanco si es null).
  useEffect(() => {
    if (!open) return;
    const p = profileQuery.data;
    setBirthdate(p?.birthdate ?? "");
    setSex(p?.sex ?? "");
    setHeightCm(p?.height_cm != null ? String(p.height_cm) : "");
    setWeightKg(p?.weight_grams != null ? String(gramsToKg(p.weight_grams)) : "");
    setObjective(p?.objective ?? "");
    setLocation(p?.location ?? "");
    setLevel(p?.level ?? "");
    setWeeklyDays(p?.weekly_days != null ? String(p.weekly_days) : "");
    setEquipment(p?.equipment ?? []);
    setLimitations(p?.limitations ?? "");
  }, [open, profileQuery.data]);

  const saveMutation = useMutation({
    mutationFn: () =>
      saveProfile({
        birthdate: birthdate || null,
        sex: sex || null,
        height_cm: heightCm ? Number(heightCm) : null,
        weight_grams: weightKg ? kgToGrams(Number(weightKg)) : null,
        objective: objective || null,
        location: location || null,
        level: level || null,
        weekly_days: weeklyDays ? Number(weeklyDays) : null,
        equipment,
        limitations,
      }),
    onSuccess: () => {
      setError(null);
      qc.invalidateQueries({ queryKey: ["fitness-profile"] });
      onClose();
    },
    onError: (e) => setError(e instanceof Error ? e.message : "No se pudo guardar"),
  });

  const toggleEquip = (v: string) =>
    setEquipment((prev) => (prev.includes(v) ? prev.filter((x) => x !== v) : [...prev, v]));

  const selectCls = "w-full rounded-lg border-2 border-ink bg-surface px-2 py-1";

  return (
    <Modal open={open} onClose={onClose} title="Mi perfil">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          saveMutation.mutate();
        }}
        className="space-y-3 text-sm"
      >
        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Fecha de nacimiento</span>
          <input type="date" aria-label="Fecha de nacimiento" value={birthdate}
            onChange={(e) => setBirthdate(e.target.value)} className={selectCls} />
        </label>

        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Sexo</span>
          <select aria-label="Sexo" value={sex} onChange={(e) => setSex(e.target.value)} className={selectCls}>
            <option value="">—</option>
            {SEXES.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </label>

        <div className="flex gap-2">
          <label className="block flex-1 space-y-1">
            <span className="text-xs font-bold text-muted">Altura (cm)</span>
            <input type="number" aria-label="Altura (cm)" value={heightCm} min={1}
              onChange={(e) => setHeightCm(e.target.value)} className={selectCls} />
          </label>
          <label className="block flex-1 space-y-1">
            <span className="text-xs font-bold text-muted">Peso (kg)</span>
            <input type="number" aria-label="Peso (kg)" value={weightKg} min={1} step={0.1}
              onChange={(e) => setWeightKg(e.target.value)} className={selectCls} />
          </label>
        </div>

        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Objetivo</span>
          <select aria-label="Objetivo" value={objective} onChange={(e) => setObjective(e.target.value)} className={selectCls}>
            <option value="">—</option>
            {OBJECTIVES.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </label>

        <div className="flex gap-2">
          <label className="block flex-1 space-y-1">
            <span className="text-xs font-bold text-muted">Lugar</span>
            <select aria-label="Lugar" value={location} onChange={(e) => setLocation(e.target.value)} className={selectCls}>
              <option value="">—</option>
              {LOCATIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
            </select>
          </label>
          <label className="block flex-1 space-y-1">
            <span className="text-xs font-bold text-muted">Nivel</span>
            <select aria-label="Nivel" value={level} onChange={(e) => setLevel(e.target.value)} className={selectCls}>
              <option value="">—</option>
              {LEVELS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
            </select>
          </label>
        </div>

        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Días por semana</span>
          <input type="number" aria-label="Días por semana" value={weeklyDays} min={1} max={7}
            onChange={(e) => setWeeklyDays(e.target.value)} className={selectCls} />
        </label>

        <fieldset className="space-y-1">
          <legend className="text-xs font-bold text-muted">Equipo</legend>
          <div className="flex flex-wrap gap-2">
            {EQUIPMENT.map((o) => (
              <button key={o.value} type="button" onClick={() => toggleEquip(o.value)}
                aria-pressed={equipment.includes(o.value)}
                className={`rounded-lg border-2 border-ink px-2 py-1 text-xs font-bold shadow-brutal-sm ${equipment.includes(o.value) ? "bg-accent" : "bg-surface"}`}>
                {o.label}
              </button>
            ))}
          </div>
        </fieldset>

        <label className="block space-y-1">
          <span className="text-xs font-bold text-muted">Lesiones / limitaciones</span>
          <textarea aria-label="Lesiones o limitaciones" value={limitations} rows={2}
            onChange={(e) => setLimitations(e.target.value)} className={selectCls} />
        </label>

        {error && (
          <p className="rounded-md border-2 border-ink bg-danger-bg px-3 py-2 text-xs font-bold text-danger-fg shadow-brutal-sm">{error}</p>
        )}

        <Button type="submit" disabled={saveMutation.isPending} className="w-full">
          {saveMutation.isPending ? "Guardando…" : "Guardar"}
        </Button>
      </form>
    </Modal>
  );
}
```

> `useEffect` ya está importado (la página lo usa para el redirect a /login). Confirmá que esté en el import de `react`.

- [ ] **Step 6: Verde + suite + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build`
Expected: todo verde; build OK (typecheck incluido).

- [ ] **Step 7: Commit**

```bash
git add web/src/routes/entrenamiento.tsx web/src/routes/entrenamiento.test.tsx
git commit -m "feat(web): modal 'Mi perfil' en entrenamiento

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Cierre — review, merge y smoke

**Files:** verificación + `scripts/smoke-r24.sh` + bitácora.

- [ ] **Step 1: Review final** del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-17-plan-24-perfil-fitness-design.md`. Aplicar nits.

- [ ] **Step 2: Suites verdes**
Backend: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && TEST_DATABASE_URL=... go test -p 1 ./... -count=1`
Frontend: `cd web && npx vitest run && npm run build`

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`.

- [ ] **Step 4: Deploy manual (Coolify) + smoke.** Crear `scripts/smoke-r24.sh` (patrón de `scripts/smoke-r20.sh`: token Bearer del register): `GET /training/profile` → `null`; `PUT /training/profile` con un perfil válido → 200 con los valores; `GET` de nuevo → el perfil (un solo registro); `PUT` con `weekly_days=8` → 400; `PUT` con `sex` inválido → 400. (Extraer campos con `grep -o`, no `sed` greedy — ver la lección de la R23.)

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-17-sesion-plan-24-perfil-fitness.md` (mencionar que es el slice A de la expansión de entrenamiento; B/C/D pendientes).

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 modelo (tabla fitness_profiles 1:1, columnas) → Task 1. ✓
- §3 backend (Get/Upsert; Profile/SaveProfile; rutas GET/PUT; validación enums+rangos+fecha; ownership por PK) → Task 1 (queries) + Task 2 (servicio+handler). ✓
- §3 unidades: refinamiento documentado (API usa weight_grams/height_cm; el front convierte con kgToGrams/gramsToKg) → Task 2 (vista) + Task 4 (modal). ✓
- §4 frontend (lib get/save; botón "Mi perfil"; ProfileModal con todos los campos + precarga + guardar) → Task 3 (lib) + Task 4 (UI). ✓
- §5 errores (400 enum/rango/fecha; GET null; ownership) → Task 2 handler. ✓
- §6 testing → Tasks 1–4; E2E → Task 5. ✓
- §7 aceptación → smoke Task 5. ✓

**Placeholders:** los «ajustá al tipo generado por sqlc / al harness real de entrenamiento.test» son adaptaciones deterministas con instrucción de qué inspeccionar; sin TODOs de diseño.

**Consistencia de tipos/firmas:** store `UpsertFitnessProfileParams`/`FitnessProfile` (nullable → `*T`, equipment `[]string`) ↔ servicio `ProfileInput`/`Profile`/`buildProfile` ↔ handler `profileReq` (validación) ↔ endpoints `GET/PUT /training/profile` ↔ lib `FitnessProfile`/`getProfile`/`saveProfile` ↔ modal. Enums idénticos en handler (`oneof`) y en el front (listas). weight en `weight_grams` en toda la API; kg solo en la UI vía `kgToGrams`/`gramsToKg`. ✓

**Lección aplicada (R21/R23):** la Task 3 (lib) corre `npm run build` además del test, para atajar errores de typecheck.
