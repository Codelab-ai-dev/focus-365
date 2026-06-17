# Notas por serie — Plan de implementación (Entrenamiento slice C1, Rebanada 26)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Una nota opcional por serie del registro de entreno, guardada con la serie, visible en el historial y sumada al contexto del Entrenador IA.

**Architecture:** Se agrega la columna `note` a `workout_sets`. El paquete `training` propaga la nota en el create, la vista y las queries de series; `buildSuggestionContext` (slice B) la incluye. El frontend agrega un campo de nota por serie en el form y la muestra en el historial. Cambios additivos: el build se mantiene verde entre tasks.

**Tech Stack:** Go (chi, sqlc, pgx/v5, goose), Postgres, React + Vite + TanStack Query + Vitest.

**Contexto del repo (leer antes de empezar):**
- sqlc: tras editar SQL `cd api && sqlc generate`. DB test en `localhost:5544`. Comandos Go: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" <cmd>`, `TEST_DATABASE_URL=...`; **suite con `-p 1`**.
- `workout_sets(id, workout_id, exercise_id, position, reps, weight_grams, created_at)`. Las queries actuales: `CreateWorkoutSet` (INSERT de workout_id/exercise_id/position/reps/weight_grams), `ListSetsByWorkout` y `ListSetsByWorkoutIDs` (SELECT con `JOIN exercises`).
- `training/types.go`: `SetInput{Exercise string; Reps, WeightGrams *int32}`, `WorkoutSet{Exercise string; Reps, WeightGrams *int32}` (JSON `exercise/reps/weight_grams`).
- `training/service.go`: `CreateWorkout` inserta cada serie con `qtx.CreateWorkoutSet(...)`; `GetWorkout` mapea `ListSetsByWorkout` → `WorkoutSet`; `ListWorkouts` mapea `ListSetsByWorkoutIDs` → `WorkoutSet` (agrupado por workout).
- `training/suggestion.go` (slice B): `buildSuggestionContext` recorre `[]store.ListSetsByWorkoutIDsRow` (`WorkoutID, Position, Reps *int32, WeightGrams *int32, ExerciseName`) por workout.
- `training/handler.go`: `setReq{Exercise, Reps, WeightGrams}`; el create arma `[]SetInput` en un loop. `httpx.DecodeAndValidate`. El handler ya importa `time`/`auth`/`httpx`/`chi`/`uuid`; tras la R25 además `errors`/`strings`/`unicode/utf8`.
- `handler_test.go` es `package training_test`; helpers `newEnv(t)`/`token`/`do`. `createWorkout` se postea a `/training/workouts`.
- Front `lib/training.ts`: `WorkoutSet{exercise; reps; weight_grams}`, `SetInput{exercise; reps; weight_grams}`. `entrenamiento.tsx`: `SetRow{exercise; reps; weightKg}`, `emptyRow()`, el form de series (líneas 172-202), el historial (`w.sets.map`, líneas 311-329).
- Última migración: `0021_training_suggestions.sql` → la nueva es `0022`.

---

## Estructura de archivos

**Backend**
- Crear `api/db/migrations/0022_workout_set_note.sql`.
- Modificar `api/db/queries/training.sql` (CreateWorkoutSet + las dos ListSets).
- Regenerar `api/internal/store/*` (sqlc).
- Modificar `api/internal/store/training_test.go` (o crear test de set-note) — test de store.
- Modificar `api/internal/training/types.go` (`SetInput.Note`, `WorkoutSet.Note`).
- Modificar `api/internal/training/service.go` (pasar/mapear note).
- Modificar `api/internal/training/suggestion.go` (`buildSuggestionContext` incluye note).
- Modificar `api/internal/training/handler.go` (`setReq.Note` + validación + map).
- Modificar `api/internal/training/handler_test.go` (test del round-trip + nota larga).

**Frontend**
- Modificar `web/src/lib/training.ts` (tipos `WorkoutSet`/`SetInput` ganan `note`).
- Modificar `web/src/routes/entrenamiento.tsx` (campo de nota por serie + historial).
- Modificar `web/src/routes/entrenamiento.test.tsx` (test).

---

## Task 1: Migración 0022 + queries + test de store

**Files:**
- Create: `api/db/migrations/0022_workout_set_note.sql`
- Modify: `api/db/queries/training.sql`
- Modify: `api/internal/store/training_test.go`
- Regenerate: `api/internal/store/`

- [ ] **Step 1: Migración**

Crear `api/db/migrations/0022_workout_set_note.sql`:

```sql
-- +goose Up
ALTER TABLE workout_sets ADD COLUMN note TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE workout_sets DROP COLUMN note;
```

- [ ] **Step 2: Queries**

En `api/db/queries/training.sql`, agregar `note`:

`CreateWorkoutSet`:
```sql
-- name: CreateWorkoutSet :one
INSERT INTO workout_sets (workout_id, exercise_id, position, reps, weight_grams, note)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;
```

`ListSetsByWorkout` (agregar `ws.note`):
```sql
-- name: ListSetsByWorkout :many
SELECT ws.position, ws.reps, ws.weight_grams, ws.note, e.name AS exercise_name
FROM workout_sets ws
JOIN exercises e ON e.id = ws.exercise_id
WHERE ws.workout_id = $1
ORDER BY ws.position ASC;
```

`ListSetsByWorkoutIDs` (agregar `ws.note`):
```sql
-- name: ListSetsByWorkoutIDs :many
SELECT ws.workout_id, ws.position, ws.reps, ws.weight_grams, ws.note, e.name AS exercise_name
FROM workout_sets ws
JOIN exercises e ON e.id = ws.exercise_id
WHERE ws.workout_id = ANY(sqlc.arg('workout_ids')::uuid[])
ORDER BY ws.position ASC;
```

- [ ] **Step 3: Regenerar sqlc**

Run: `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`
Verificá: `grep -n "Note" internal/store/training.sql.go`
Esperado: `CreateWorkoutSetParams` gana `Note string`; `ListSetsByWorkoutRow` y `ListSetsByWorkoutIDsRow` ganan `Note string`. El modelo `WorkoutSet` (models.go) gana `Note string`.

- [ ] **Step 4: Test de store (que falla)**

En `api/internal/store/training_test.go` (si no existe, crearlo en `package store_test`), agregar un test que cree un workout + un set con nota y verifique que `ListSetsByWorkout` la devuelve. Reusá `newUser`. Si el archivo ya tiene helpers de training, reusalos.

```go
func TestWorkoutSetNote(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)

	ex, err := q.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: u, Name: "Sentadilla"})
	if err != nil {
		t.Fatalf("UpsertExercise: %v", err)
	}
	w, err := q.CreateWorkout(ctx, store.CreateWorkoutParams{
		UserID: u, Date: date("2026-06-17"), Type: "Pierna", Note: "",
	})
	if err != nil {
		t.Fatalf("CreateWorkout: %v", err)
	}
	if _, err := q.CreateWorkoutSet(ctx, store.CreateWorkoutSetParams{
		WorkoutID: w.ID, ExerciseID: ex.ID, Position: 0,
		Reps: nil, WeightGrams: nil, Note: "leí pesado",
	}); err != nil {
		t.Fatalf("CreateWorkoutSet: %v", err)
	}

	rows, err := q.ListSetsByWorkout(ctx, w.ID)
	if err != nil || len(rows) != 1 || rows[0].Note != "leí pesado" {
		t.Fatalf("ListSetsByWorkout: %v rows=%+v", err, rows)
	}
}
```

> `date(...)` es el helper de `goal_notes_test.go`/`fitness_profiles_test.go` (`time.Parse("2006-01-02", ...)`); si no es visible por estar en otro archivo del mismo `package store_test`, está accesible — todos comparten package. Si no existiera, definí uno local `func date(s string) time.Time { d, _ := time.Parse("2006-01-02", s); return d }` (cuidando no duplicar si ya está).

- [ ] **Step 5: Correr (falla→pasa)**

Run: `cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ -run TestWorkoutSetNote -v`
Expected: PASS.

> Tras esta task, `go build ./...` queda **roto** (el paquete `training` llama a `CreateWorkoutSet` sin `Note` y mapea rows sin `Note`). Es esperado; se arregla en la Task 2. La Task 1 solo verifica `./internal/store/`.

- [ ] **Step 6: Commit**

```bash
git add api/db/migrations/0022_workout_set_note.sql api/db/queries/training.sql api/internal/store
git commit -m "feat(store): columna note en workout_sets (migración 0022)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Backend `training` — propagar la nota

**Files:**
- Modify: `api/internal/training/types.go`
- Modify: `api/internal/training/service.go`
- Modify: `api/internal/training/suggestion.go`
- Modify: `api/internal/training/handler.go`
- Modify: `api/internal/training/handler_test.go`

- [ ] **Step 1: Tipos**

En `api/internal/training/types.go`, agregar `Note` a `SetInput` y `WorkoutSet`:

```go
// SetInput es una serie recibida al capturar una sesión.
type SetInput struct {
	Exercise    string
	Reps        *int32
	WeightGrams *int32
	Note        string
}
```
```go
// WorkoutSet es la vista de una serie (con el nombre del ejercicio resuelto).
type WorkoutSet struct {
	Exercise    string `json:"exercise"`
	Reps        *int32 `json:"reps"`
	WeightGrams *int32 `json:"weight_grams"`
	Note        string `json:"note"`
}
```

- [ ] **Step 2: Servicio (pasar y mapear la nota)**

En `api/internal/training/service.go`:

a) En `CreateWorkout`, el `qtx.CreateWorkoutSet(...)` pasa `Note`:

```go
		if _, err := qtx.CreateWorkoutSet(ctx, store.CreateWorkoutSetParams{
			WorkoutID: w.ID, ExerciseID: exID, Position: int32(i),
			Reps: set.Reps, WeightGrams: set.WeightGrams, Note: set.Note,
		}); err != nil {
			return nil, err
		}
```

b) En `GetWorkout`, el map de `ListSetsByWorkout` incluye `Note`:

```go
	sets := make([]WorkoutSet, 0, len(rows))
	for _, r := range rows {
		sets = append(sets, WorkoutSet{Exercise: r.ExerciseName, Reps: r.Reps, WeightGrams: r.WeightGrams, Note: r.Note})
	}
```

c) En `ListWorkouts`, el map de `ListSetsByWorkoutIDs` (el `byWorkout[...] = append(...)`) incluye `Note`. Buscá la línea donde se construye el `WorkoutSet` desde el row de `ListSetsByWorkoutIDs` y agregá `Note: st.Note` (mismo patrón que en (b)).

- [ ] **Step 3: Contexto del agente (slice B)**

En `api/internal/training/suggestion.go`, dentro de `buildSuggestionContext`, en el bloque que escribe cada serie (`b.WriteString("    · " + st.ExerciseName)` … reps … kg), agregar al final, antes del `"\n"`:

```go
				if st.Note != "" {
					b.WriteString(" (" + st.Note + ")")
				}
```

- [ ] **Step 4: Handler (body + validación + map)**

En `api/internal/training/handler.go`:

a) `setReq` gana `Note`:

```go
type setReq struct {
	Exercise    string `json:"exercise" validate:"required"`
	Reps        *int32 `json:"reps" validate:"omitempty,min=0"`
	WeightGrams *int32 `json:"weight_grams" validate:"omitempty,min=0"`
	Note        string `json:"note"`
}
```

b) En el create handler, el loop que arma `[]SetInput` valida el largo de la nota (máx 200 runes) y la pasa:

```go
		sets := make([]SetInput, 0, len(req.Sets))
		for _, s := range req.Sets {
			note := strings.TrimSpace(s.Note)
			if utf8.RuneCountInString(note) > 200 {
				httpx.WriteErr(w, http.StatusBadRequest, "la nota de la serie es demasiado larga")
				return
			}
			sets = append(sets, SetInput{Exercise: s.Exercise, Reps: s.Reps, WeightGrams: s.WeightGrams, Note: note})
		}
```
(`strings` y `unicode/utf8` ya están importados desde la R25.)

- [ ] **Step 5: Tests de handler (que fallan)**

En `api/internal/training/handler_test.go`, agregar:

```go
func TestWorkoutSetNoteRoundTrip(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "setnote@b.com")

	rec := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-17", "type": "Pierna",
		"sets": []map[string]any{
			{"exercise": "Sentadilla", "reps": 8, "weight_grams": 80000, "note": "leí pesado"},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST workout code = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = do(t, e.h, http.MethodGet, "/training/workouts", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET workouts code = %d", rec.Code)
	}
	var ws []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &ws)
	if len(ws) != 1 {
		t.Fatalf("workouts = %d", len(ws))
	}
	sets, _ := ws[0]["sets"].([]any)
	if len(sets) != 1 {
		t.Fatalf("sets = %d", len(sets))
	}
	s0, _ := sets[0].(map[string]any)
	if s0["note"] != "leí pesado" {
		t.Fatalf("set note = %v", s0["note"])
	}
}

func TestWorkoutSetNoteTooLong(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "setnotelong@b.com")
	long := strings.Repeat("a", 201)
	rec := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-17",
		"sets": []map[string]any{{"exercise": "Sentadilla", "note": long}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("nota larga code = %d, want 400", rec.Code)
	}
}
```

> `handler_test.go` ya importa `encoding/json`, `strings`, `net/http`. Si falta alguno, agregalo.

- [ ] **Step 6: Verificar build + suite**

Run:
```
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && \
GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
```
Expected: todo verde.

- [ ] **Step 7: Commit**

```bash
git add api/internal/training
git commit -m "feat(training): nota por serie (registro, vista y contexto del agente)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Frontend — nota por serie en el form y el historial

**Files:**
- Modify: `web/src/lib/training.ts`
- Modify: `web/src/routes/entrenamiento.tsx`
- Modify: `web/src/routes/entrenamiento.test.tsx`

- [ ] **Step 1: Tipos en `lib/training.ts`**

Agregar `note` a `WorkoutSet` y `SetInput`:

```ts
export type WorkoutSet = {
  exercise: string;
  reps: number | null;
  weight_grams: number | null;
  note: string;
};
```
```ts
export type SetInput = {
  exercise: string;
  reps: number | null;
  weight_grams: number | null;
  note: string;
};
```

- [ ] **Step 2: Test (que falla)**

En `web/src/routes/entrenamiento.test.tsx`, agregar (o ampliar) tests: (a) el form muestra un input "Nota serie 1"; (b) el historial muestra la nota de una serie. Para (b), el `fetchMock` del historial debe devolver un workout cuya serie tenga `note`. Revisá cómo `entrenamiento.test.tsx` arma `listWorkouts` (su `fetchMock`); incluí `note` en la serie mockeada. Ejemplo de asserts:

```tsx
// (a) input presente
// expect(await screen.findByLabelText("Nota serie 1")).toBeInTheDocument();
//
// (b) historial: con un workout mockeado cuya serie tenga note "molestia rodilla"
// expect(await screen.findByText("molestia rodilla")).toBeInTheDocument();
```
Adaptá al harness real (el fetchMock que ya devuelve workouts). Watch fail.

- [ ] **Step 3: `SetRow` + form**

En `web/src/routes/entrenamiento.tsx`:

a) `SetRow` y `emptyRow` ganan `note`:

```tsx
type SetRow = { exercise: string; reps: string; weightKg: string; note: string };

function emptyRow(): SetRow {
  return { exercise: "", reps: "", weightKg: "", note: "" };
}
```

b) En el payload de guardar (donde arma `sets: rows.filter(...).map(...)`), agregar `note`:

```tsx
        sets: rows
          .filter((r) => r.exercise.trim() !== "")
          .map((r) => ({
            exercise: r.exercise.trim(),
            reps: r.reps === "" ? null : Number(r.reps),
            weight_grams: r.weightKg === "" ? null : kgToGrams(Number(r.weightKg)),
            note: r.note.trim(),
          })),
```
(Confirmá el nombre del helper de conversión usado allí — `kgToGrams` — y mantené el resto igual.)

c) Cada serie del form pasa a dos líneas: envolver la fila actual (`<div className="flex gap-2">…</div>`) y agregar debajo el input de nota. Reemplazar el bloque `rows.map((row, i) => ( <div key={i} className="flex gap-2"> … </div> ))` por:

```tsx
              {rows.map((row, i) => (
                <div key={i} className="space-y-1">
                  <div className="flex gap-2">
                    <Input
                      type="text"
                      aria-label={`Ejercicio ${i + 1}`}
                      list="catalogo-ejercicios"
                      placeholder="Ejercicio"
                      value={row.exercise}
                      onChange={(e) => updateRow(i, { exercise: e.target.value })}
                      className="flex-1"
                    />
                    <Input
                      type="number"
                      aria-label={`Reps ${i + 1}`}
                      placeholder="Reps"
                      min="0"
                      value={row.reps}
                      onChange={(e) => updateRow(i, { reps: e.target.value })}
                      className="w-20"
                    />
                    <Input
                      type="number"
                      aria-label={`Peso ${i + 1}`}
                      placeholder="kg"
                      min="0"
                      step="0.5"
                      value={row.weightKg}
                      onChange={(e) => updateRow(i, { weightKg: e.target.value })}
                      className="w-20"
                    />
                  </div>
                  <Input
                    type="text"
                    aria-label={`Nota serie ${i + 1}`}
                    placeholder="nota de la serie (opcional)"
                    value={row.note}
                    onChange={(e) => updateRow(i, { note: e.target.value })}
                  />
                </div>
              ))}
```

- [ ] **Step 4: Historial muestra la nota**

En el historial, dentro del `w.sets.map((s, i) => ( <li key={i}> … </li> ))`, mostrar la nota de la serie si existe. Reemplazar el `<li>` por:

```tsx
                      {w.sets.map((s, i) => (
                        <li key={i}>
                          {s.exercise}
                          {s.reps != null && (
                            <>
                              {" · "}
                              <Chip variant="sky" size="sm">{s.reps} reps</Chip>
                            </>
                          )}
                          {s.weight_grams != null && (
                            <>
                              {" · "}
                              <Chip variant="sky" size="sm">{gramsToKg(s.weight_grams)} kg</Chip>
                            </>
                          )}
                          {s.note && (
                            <span className="block text-xs text-muted">{s.note}</span>
                          )}
                        </li>
                      ))}
```

- [ ] **Step 5: Verde + suite + build**

Run: `cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build`
Expected: todo verde; build OK (typecheck incluido).

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/training.ts web/src/routes/entrenamiento.tsx web/src/routes/entrenamiento.test.tsx
git commit -m "feat(web): nota por serie en el registro y el historial

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Cierre — review, merge y smoke

**Files:** verificación + `scripts/smoke-r26.sh` + bitácora.

- [ ] **Step 1: Review final** del diff `main..HEAD` contra el spec `docs/superpowers/specs/2026-06-17-plan-26-notas-serie-design.md`. Aplicar nits.

- [ ] **Step 2: Suites verdes**
Backend: `cd api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && TEST_DATABASE_URL=... go test -p 1 ./... -count=1`
Frontend: `cd web && npx vitest run && npm run build`

- [ ] **Step 3: Merge a `main` (no-ff), borrar rama, push** vía `finishing-a-development-branch`.

- [ ] **Step 4: Deploy manual (Coolify) + smoke.** Crear `scripts/smoke-r26.sh` (patrón de `scripts/smoke-r24.sh`, extraer con `grep -o`): registrar; `POST /training/workouts` con una serie con `note` → 201; `GET /training/workouts` → la serie trae `note`; `POST` con una serie con nota de 201 chars → 400.

- [ ] **Step 5: Bitácora** `docs/superpowers/sesiones/2026-06-17-sesion-plan-26-notas-serie.md` (slice C1; C2 pendiente).

---

## Self-review (checklist del autor)

**Cobertura del spec:**
- §2 modelo (columna note en workout_sets) → Task 1. ✓
- §3 backend (CreateWorkoutSet + ListSets con note; SetInput/WorkoutSet.Note; CreateWorkout pasa note; mapeos; validación ≤200; buildSuggestionContext incluye la nota) → Task 1 (queries) + Task 2. ✓
- §4 frontend (tipos lib; SetRow.note; input de nota por serie en dos líneas; payload; historial muestra la nota) → Task 3. ✓
- §5 errores (nota >200 → 400; vacía → '') → Task 2 handler. ✓
- §6 testing → Tasks 1–3; E2E → Task 4. ✓
- §7 aceptación → smoke Task 4. ✓

**Placeholders:** los «adaptá al harness real de entrenamiento.test / confirmá el nombre del helper» son adaptaciones deterministas con instrucción de qué inspeccionar. Sin TODOs de diseño.

**Consistencia de tipos/firmas:** store `CreateWorkoutSetParams.Note` / `ListSetsByWorkoutRow.Note` / `ListSetsByWorkoutIDsRow.Note` ↔ servicio `SetInput.Note` / `WorkoutSet.Note` / mapeos ↔ handler `setReq.Note` (validación) ↔ vista `WorkoutSet.note` ↔ frontend `WorkoutSet.note` / `SetInput.note` / `SetRow.note`. `buildSuggestionContext` usa `st.Note` (el row de ListSetsByWorkoutIDs ahora lo trae). ✓

**Patrón por capas:** la Task 1 deja el build roto a propósito (el paquete `training` usa las firmas viejas); la Task 1 verifica solo `./internal/store/`, el build completo se exige en la Task 2.
