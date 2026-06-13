# Plan 17 — Check-in diario de Capitanes de Dios — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reemplazar el check-in genérico (ánimo/energía/disciplina/nota) por el de Capitanes de Dios: mood/energy + reflexión 4D + win + qué evité + compromisos.

**Architecture:** Migración 0013 cambia las columnas de `check_ins`; el servicio gana `Upsert` completo (form) y `UpsertMetrics` parcial (IA solo mood/energy, sin pisar reflexiones); dashboard y la acción IA `registrar_checkin` se ajustan (adiós disciplina, hola win); el formulario `/check-in` se reescribe con las 4 dimensiones.

**Tech Stack:** Go + chi + sqlc/pgx (api), React + TanStack Query + Vitest (web).

**Spec:** `docs/superpowers/specs/2026-06-13-plan-17-checkin-capitanes-design.md`

**Entorno:** Go desde `api/` con `GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`; `sqlc generate` desde `api/`. Frontend `cd web && npx vitest run && npm run build`. Commits en español con `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Rama: `plan-17-checkin-capitanes` desde `main`.

**Call sites de `discipline`/`note` a tocar (verificados):**
- `api/db/queries/check_ins.sql` (UpsertCheckIn)
- `api/internal/checkin/{service.go,handler.go}` (Input/CheckIn/Upsert/toView/upsertReq)
- `api/internal/dashboard/{types.go,service.go}` (CheckinView.Discipline, checkinView)
- `api/internal/ai/actions.go` (checkinPayload, validación, summary, executor execute+undo, tool schema)
- `api/internal/httpx/httpx.go:87` (`case "Discipline"` del traductor de errores — se elimina, queda muerto)
- `web/src/lib/{checkins.ts,dashboard.ts}` (tipos), `web/src/routes/{check-in.tsx,index.tsx}` (form, CheckinCard)

**Regla:** el frontend se rompe entre la Task 1 y la Task 6 (cambia la forma del check-in). La suite web se exige verde a partir de la Task 6; el backend, verde en cada task.

---

### Task 1: Migración 0013 + queries + servicio de check-in

**Files:**
- Create: `api/db/migrations/0013_checkin_capitanes.sql`
- Modify: `api/db/queries/check_ins.sql`, `api/internal/checkin/service.go`
- Test: `api/internal/checkin/service_test.go`
- Generated: `api/internal/store/`

- [ ] **Step 1: Migración** `api/db/migrations/0013_checkin_capitanes.sql`:

```sql
-- +goose Up
ALTER TABLE check_ins
    DROP COLUMN discipline,
    DROP COLUMN note,
    ADD COLUMN dim_espiritual TEXT NOT NULL DEFAULT '',
    ADD COLUMN dim_emocional  TEXT NOT NULL DEFAULT '',
    ADD COLUMN dim_fisica     TEXT NOT NULL DEFAULT '',
    ADD COLUMN dim_financiera TEXT NOT NULL DEFAULT '',
    ADD COLUMN win            TEXT NOT NULL DEFAULT '',
    ADD COLUMN avoided        TEXT NOT NULL DEFAULT '',
    ADD COLUMN commitments    JSONB NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE check_ins
    DROP COLUMN dim_espiritual,
    DROP COLUMN dim_emocional,
    DROP COLUMN dim_fisica,
    DROP COLUMN dim_financiera,
    DROP COLUMN win,
    DROP COLUMN avoided,
    DROP COLUMN commitments,
    ADD COLUMN discipline INT NOT NULL DEFAULT 0,
    ADD COLUMN note TEXT NOT NULL DEFAULT '';
```

- [ ] **Step 2: Queries.** Reemplazar `UpsertCheckIn` y agregar `UpsertCheckInMetrics` en `api/db/queries/check_ins.sql` (`GetCheckInByDate`, `ListCheckIns`, `DeleteCheckIn` quedan igual — usan `SELECT *`):

```sql
-- name: UpsertCheckIn :one
INSERT INTO check_ins (user_id, date, mood, energy,
    dim_espiritual, dim_emocional, dim_fisica, dim_financiera, win, avoided, commitments)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (user_id, date)
DO UPDATE SET
    mood = EXCLUDED.mood,
    energy = EXCLUDED.energy,
    dim_espiritual = EXCLUDED.dim_espiritual,
    dim_emocional = EXCLUDED.dim_emocional,
    dim_fisica = EXCLUDED.dim_fisica,
    dim_financiera = EXCLUDED.dim_financiera,
    win = EXCLUDED.win,
    avoided = EXCLUDED.avoided,
    commitments = EXCLUDED.commitments,
    updated_at = now()
RETURNING *;

-- name: UpsertCheckInMetrics :one
-- Upsert parcial: solo mood/energy, sin tocar reflexiones (lo usa la IA).
INSERT INTO check_ins (user_id, date, mood, energy)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, date)
DO UPDATE SET
    mood = EXCLUDED.mood,
    energy = EXCLUDED.energy,
    updated_at = now()
RETURNING *;
```

Correr `cd /Users/gustavo/Desktop/focus-365/api && sqlc generate`. `store.CheckIn` gana `DimEspiritual/DimEmocional/DimFisica/DimFinanciera/Win/Avoided string` y `Commitments []byte` (jsonb); pierde `Discipline`/`Note`. `UpsertCheckInParams` gana los campos nuevos.

- [ ] **Step 3: Tests que fallan** en `api/internal/checkin/service_test.go` (leer el setup existente y reusarlo):

```go
func TestUpsertFullRoundTrip(t *testing.T) {
	// setup: pool, queries, svc, user, date.
	in := Input{
		Date: date, Mood: 8, Energy: 7,
		Espiritual: "día 3 del reto", Emocional: "llegaron mis hijas",
		Fisica: "rutina de piernas", Financiera: "0 gastos",
		Win: "ver a mis hijas", Avoided: "0 alcohol",
		Commitments: []string{"Tender la cama", "Pasear a Ruffo"},
	}
	ci, err := svc.Upsert(ctx, userID, in)
	if err != nil { t.Fatalf("Upsert: %v", err) }
	if ci.Mood != 8 || ci.Espiritual != "día 3 del reto" || ci.Win != "ver a mis hijas" {
		t.Errorf("ci = %+v", ci)
	}
	if len(ci.Commitments) != 2 || ci.Commitments[0] != "Tender la cama" {
		t.Errorf("commitments = %v", ci.Commitments)
	}
}

func TestUpsertMetricsPreservaReflexiones(t *testing.T) {
	// 1. Upsert completo con reflexiones.
	_, err := svc.Upsert(ctx, userID, Input{
		Date: date, Mood: 5, Energy: 5, Espiritual: "reflexión previa",
		Commitments: []string{"x"},
	})
	if err != nil { t.Fatalf("Upsert: %v", err) }
	// 2. UpsertMetrics solo cambia mood/energy.
	ci, err := svc.UpsertMetrics(ctx, userID, date, 9, 8)
	if err != nil { t.Fatalf("UpsertMetrics: %v", err) }
	if ci.Mood != 9 || ci.Energy != 8 {
		t.Errorf("métricas no actualizadas: %+v", ci)
	}
	if ci.Espiritual != "reflexión previa" || len(ci.Commitments) != 1 {
		t.Errorf("UpsertMetrics pisó las reflexiones: %+v", ci)
	}
}

func TestUpsertMetricsCreaMinimo(t *testing.T) {
	ci, err := svc.UpsertMetrics(ctx, userID, date, 6, 6)
	if err != nil { t.Fatalf("UpsertMetrics: %v", err) }
	if ci.Mood != 6 || ci.Espiritual != "" || len(ci.Commitments) != 0 {
		t.Errorf("registro mínimo mal: %+v", ci)
	}
}
```

(Escribir el setup real según el archivo. `ci.Commitments` es `[]string`.)

- [ ] **Step 4: Verificar que fallan** (compilación: Input/CheckIn cambian).

- [ ] **Step 5: Implementar** en `api/internal/checkin/service.go`:

`Input` y `CheckIn`:

```go
type Input struct {
	Date                                      time.Time
	Mood, Energy                              int
	Espiritual, Emocional, Fisica, Financiera string
	Win, Avoided                              string
	Commitments                               []string
}

type CheckIn struct {
	ID         string    `json:"id"`
	Date       string    `json:"date"`
	Mood       int       `json:"mood"`
	Energy     int       `json:"energy"`
	Espiritual string    `json:"espiritual"`
	Emocional  string    `json:"emocional"`
	Fisica     string    `json:"fisica"`
	Financiera string    `json:"financiera"`
	Win        string    `json:"win"`
	Avoided    string    `json:"avoided"`
	Commitments []string `json:"commitments"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
```

`Upsert` arma los params nuevos (commitments → JSON bytes):

```go
func (s *Service) Upsert(ctx context.Context, userID uuid.UUID, in Input) (*CheckIn, error) {
	commits, err := json.Marshal(cleanCommitments(in.Commitments))
	if err != nil {
		return nil, err
	}
	row, err := s.q.UpsertCheckIn(ctx, store.UpsertCheckInParams{
		UserID: userID, Date: in.Date, Mood: int32(in.Mood), Energy: int32(in.Energy),
		DimEspiritual: in.Espiritual, DimEmocional: in.Emocional,
		DimFisica: in.Fisica, DimFinanciera: in.Financiera,
		Win: in.Win, Avoided: in.Avoided, Commitments: commits,
	})
	if err != nil {
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

// UpsertMetrics actualiza solo mood/energy del día (la IA), sin pisar las
// reflexiones que el usuario haya escrito en el formulario.
func (s *Service) UpsertMetrics(ctx context.Context, userID uuid.UUID, date time.Time, mood, energy int) (*CheckIn, error) {
	row, err := s.q.UpsertCheckInMetrics(ctx, store.UpsertCheckInMetricsParams{
		UserID: userID, Date: date, Mood: int32(mood), Energy: int32(energy),
	})
	if err != nil {
		return nil, err
	}
	v := toView(row)
	return &v, nil
}

// cleanCommitments quita strings vacíos tras trim.
func cleanCommitments(in []string) []string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		if t := strings.TrimSpace(c); t != "" {
			out = append(out, t)
		}
	}
	return out
}
```

`toView`:

```go
func toView(row store.CheckIn) CheckIn {
	var commits []string
	if len(row.Commitments) > 0 {
		_ = json.Unmarshal(row.Commitments, &commits)
	}
	if commits == nil {
		commits = []string{}
	}
	return CheckIn{
		ID: row.ID.String(), Date: row.Date.Format(dateLayout),
		Mood: int(row.Mood), Energy: int(row.Energy),
		Espiritual: row.DimEspiritual, Emocional: row.DimEmocional,
		Fisica: row.DimFisica, Financiera: row.DimFinanciera,
		Win: row.Win, Avoided: row.Avoided, Commitments: commits,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
```

(Imports: `encoding/json`, `strings`.)

- [ ] **Step 6: Verificar (solo paquete checkin + store, el resto compila roto aún) + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/checkin/ ./internal/store/ -count=1
git add api/db api/internal/store api/internal/checkin/service.go api/internal/checkin/service_test.go
git commit -m "feat(checkin): modelo de Capitanes (4D + win + compromisos) con upsert completo y parcial"
```

(El build de `./...` queda roto hasta la Task 4 — el handler, dashboard y AI usan los campos viejos. Esta task verifica solo checkin+store.)

---

### Task 2: Handler de check-in (validación + body nuevo)

**Files:**
- Modify: `api/internal/checkin/handler.go`, `api/internal/httpx/httpx.go`
- Test: el test del handler de checkin (si existe; si no, los del servicio cubren)

- [ ] **Step 1: Reescribir `upsertReq` y `handleUpsert`** en `handler.go`:

```go
type upsertReq struct {
	Date        string   `json:"date" validate:"required"`
	Mood        int      `json:"mood" validate:"required,min=1,max=10"`
	Energy      int      `json:"energy" validate:"required,min=1,max=10"`
	Espiritual  string   `json:"espiritual"`
	Emocional   string   `json:"emocional"`
	Fisica      string   `json:"fisica"`
	Financiera  string   `json:"financiera"`
	Win         string   `json:"win"`
	Avoided     string   `json:"avoided"`
	Commitments []string `json:"commitments"`
}
```

y en `handleUpsert`, el `svc.Upsert(... Input{...})`:

```go
ci, err := svc.Upsert(r.Context(), userID, Input{
	Date: date, Mood: req.Mood, Energy: req.Energy,
	Espiritual: req.Espiritual, Emocional: req.Emocional,
	Fisica: req.Fisica, Financiera: req.Financiera,
	Win: req.Win, Avoided: req.Avoided, Commitments: req.Commitments,
})
```

- [ ] **Step 2: Quitar el `case "Discipline"` de `httpx.go`** (líneas ~87-88): borrar las dos líneas `case "Discipline": return "la disciplina"`.

- [ ] **Step 3: Verificar (checkin + httpx) + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/checkin/ ./internal/httpx/ -count=1
git add api/internal/checkin/handler.go api/internal/httpx/httpx.go
git commit -m "feat(checkin): handler con el body nuevo (4D, win, compromisos)"
```

---

### Task 3: Dashboard (adiós disciplina, hola win)

**Files:**
- Modify: `api/internal/dashboard/types.go`, `api/internal/dashboard/service.go`
- Test: `api/internal/dashboard/service_test.go` (si asserta discipline, adaptar)

- [ ] **Step 1: `CheckinView`** en `types.go`: `Discipline int` → `Win string`:

```go
type CheckinView struct {
	Present bool   `json:"present"`
	Mood    int    `json:"mood"`
	Energy  int    `json:"energy"`
	Win     string `json:"win"`
}
```

- [ ] **Step 2: `checkinView`** en `service.go`:

```go
func checkinView(c *checkin.CheckIn) *CheckinView {
	if c == nil {
		return nil
	}
	return &CheckinView{Present: true, Mood: c.Mood, Energy: c.Energy, Win: c.Win}
}
```

- [ ] **Step 3:** Si `dashboard/service_test.go` asserta `discipline`, cambiar a `win` (sembrar un check-in con win y verificar `snap.Checkin.Win`). Si el fake de checkin construye `checkin.CheckIn{...Discipline...}`, quitar discipline y poner Win.

- [ ] **Step 4: Verificar + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/dashboard/ -count=1
git add api/internal/dashboard
git commit -m "feat(dashboard): el check-in del snapshot muestra win en vez de disciplina"
```

---

### Task 4: IA — `registrar_checkin` solo métricas (mood/energy)

**Files:**
- Modify: `api/internal/ai/actions.go`
- Test: `api/internal/ai/actions_test.go`

- [ ] **Step 1: Tests que fallan** en `actions_test.go`. El fake del servicio de checkin (`fakeCheckinSvc`) gana `UpsertMetrics` y un campo para simular el check-in previo:

```go
// En fakeCheckinSvc: nuevos campos y método.
// metricsIn captura la última llamada a UpsertMetrics; today devuelve el check-in actual.
func (f *fakeCheckinSvc) UpsertMetrics(ctx context.Context, userID uuid.UUID, date time.Time, mood, energy int) (*checkin.CheckIn, error) {
	f.metricsMood, f.metricsEnergy = mood, energy
	return &checkin.CheckIn{Mood: mood, Energy: energy}, nil
}
```

Tests:

```go
func TestExecutorCheckinSoloMetricas(t *testing.T) {
	c := &fakeCheckinSvc{} // today devuelve nil (no había check-in)
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	today := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	res, err := ex.execute(context.Background(), uuid.New(), "checkin", []byte(`{"mood":8,"energy":7}`), today)
	if err != nil { t.Fatalf("execute: %v", err) }
	if c.metricsMood != 8 || c.metricsEnergy != 7 {
		t.Errorf("UpsertMetrics no llamado con 8/7: %+v", c)
	}
	// result guarda existed=false (no había check-in).
	var r checkinResult
	_ = json.Unmarshal(res, &r)
	if r.Existed {
		t.Errorf("existed debería ser false: %+v", r)
	}
}

func TestParseCheckinSoloMoodEnergy(t *testing.T) {
	if _, err := parseActionPayload("checkin", `{"mood":8,"energy":7}`); err != nil {
		t.Errorf("válido: %v", err)
	}
	// discipline ya no es campo conocido → rechazado por DisallowUnknownFields.
	if _, err := parseActionPayload("checkin", `{"mood":8,"energy":7,"discipline":9}`); err == nil {
		t.Error("discipline ya no debería aceptarse")
	}
	if _, err := parseActionPayload("checkin", `{"mood":11,"energy":7}`); err == nil {
		t.Error("mood fuera de rango debe fallar")
	}
}
```

Para el undo (con/sin previo), agregar al fake un campo `cur *checkin.CheckIn` que devuelva `Today`, y tests:

```go
func TestUndoCheckinSinPrevioBorra(t *testing.T) {
	c := &fakeCheckinSvc{} // sin previo
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	// result con existed=false → undo borra el día.
	err := ex.undo(context.Background(), uuid.New(), "checkin",
		[]byte(`{"mood":8,"energy":7}`), []byte(`{"existed":false,"date":"2026-06-13"}`))
	if err != nil { t.Fatalf("undo: %v", err) }
	if !c.deleted {
		t.Error("undo sin previo debe borrar el día")
	}
}

func TestUndoCheckinConPrevioRestaura(t *testing.T) {
	c := &fakeCheckinSvc{}
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	err := ex.undo(context.Background(), uuid.New(), "checkin",
		[]byte(`{"mood":8,"energy":7}`), []byte(`{"existed":true,"prev_mood":5,"prev_energy":4,"date":"2026-06-13"}`))
	if err != nil { t.Fatalf("undo: %v", err) }
	if c.metricsMood != 5 || c.metricsEnergy != 4 {
		t.Errorf("undo debe restaurar 5/4 vía UpsertMetrics: %+v", c)
	}
}
```

(Adaptar los tests viejos de checkin que usaban discipline/note — eliminarlos o reescribirlos a mood/energy.)

- [ ] **Step 2: Verificar que fallan.**

- [ ] **Step 3: Implementar** en `actions.go`.

`checkinPayload` y `checkinResult`:

```go
type checkinPayload struct {
	Mood   int `json:"mood"`
	Energy int `json:"energy"`
}

type checkinResult struct {
	Existed   bool   `json:"existed"`
	PrevMood  int    `json:"prev_mood"`
	PrevEnergy int   `json:"prev_energy"`
	Date      string `json:"date"`
}
```

`parseActionPayload` caso `actionCheckin` (solo mood/energy 1-10):

```go
case actionCheckin:
	var p checkinPayload
	if err := dec(&p); err != nil {
		return nil, err
	}
	if err := rango(p.Mood, 1, 10, "mood"); err != nil {
		return nil, err
	}
	if err := rango(p.Energy, 1, 10, "energy"); err != nil {
		return nil, err
	}
	return json.Marshal(p)
```

`actionSummary` caso checkin:

```go
case actionCheckin:
	var p checkinPayload
	_ = json.Unmarshal(payload, &p)
	return fmt.Sprintf("Propongo registrar tus métricas de hoy: ánimo %d, energía %d.", p.Mood, p.Energy)
```

Interfaz `checkinUpserter` (renombrar conceptualmente) — el ejecutor necesita
`Today`, `UpsertMetrics` y `Delete`. Ajustar la interfaz compuesta `checkinSvc`
para incluir `UpsertMetrics` (ya tiene Today y Delete del undo de la R15):

```go
type checkinMetrics interface {
	UpsertMetrics(ctx context.Context, userID uuid.UUID, date time.Time, mood, energy int) (*checkin.CheckIn, error)
}
// checkinSvc compone: Today + Delete (de antes) + UpsertMetrics.
// (Quitar el viejo Upsert completo de la interfaz si solo lo usaba el execute de checkin.)
```

Ejecutor `execute` caso checkin (lee el previo, hace UpsertMetrics, guarda result):

```go
case actionCheckin:
	var p checkinPayload
	_ = json.Unmarshal(normalized, &p)
	dateStr := today.Format("2006-01-02")
	cur, _ := e.checkin.Today(ctx, userID, today)
	r := checkinResult{Date: dateStr}
	if cur != nil {
		r.Existed = true
		r.PrevMood = cur.Mood
		r.PrevEnergy = cur.Energy
	}
	if _, err := e.checkin.UpsertMetrics(ctx, userID, today, p.Mood, p.Energy); err != nil {
		return nil, err
	}
	return json.Marshal(r)
```

Ejecutor `undo` caso checkin:

```go
case actionCheckin:
	var r checkinResult
	if err := json.Unmarshal(result, &r); err != nil {
		return fmt.Errorf("%w: result corrupto", ErrActionInvalid)
	}
	date, err := time.Parse("2006-01-02", r.Date)
	if err != nil {
		return fmt.Errorf("%w: fecha corrupta", ErrActionInvalid)
	}
	if !r.Existed {
		// La acción creó el registro: borrarlo (best-effort).
		_, err := e.checkin.Delete(ctx, userID, date)
		return err
	}
	// Restaurar los números previos sin tocar reflexiones.
	_, err = e.checkin.UpsertMetrics(ctx, userID, date, r.PrevMood, r.PrevEnergy)
	return err
```

Tool `registrar_checkin` (solo mood/energy):

```go
{
	Name:        "registrar_checkin",
	Description: "Registra solo tus métricas del día (ánimo y energía, 1-10). Las reflexiones de las 4 dimensiones, el win y los compromisos se escriben en el formulario de check-in, no por chat. Úsala solo si el usuario da explícitamente sus dos números.",
	Parameters: json.RawMessage(`{"type":"object","properties":{
		"mood":{"type":"integer","minimum":1,"maximum":10,"description":"ánimo 1-10"},
		"energy":{"type":"integer","minimum":1,"maximum":10,"description":"energía 1-10"}},
		"required":["mood","energy"]}`),
},
```

(Si la interfaz compuesta `checkinSvc` ya incluye `Upsert` completo y nadie más
lo usa desde el ejecutor, quitarlo de la interfaz para no exigirlo del fake;
verificar que el wiring en `server.go` pasa `checkinSvc` real que SÍ implementa
`UpsertMetrics` — lo hace tras la Task 1.)

- [ ] **Step 4: Verificar paquete ai + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -count=1
git add api/internal/ai/actions.go api/internal/ai/actions_test.go
git commit -m "feat(ai): registrar_checkin solo métricas (mood/energy) con upsert parcial y undo"
```

(Aquí el `go build ./...` ya debe pasar: checkin/dashboard/ai están alineados.)

---

### Task 5: Contexto del chat con la reflexión 4D + verificación backend completa

**Files:**
- Modify: `api/internal/ai/chatcontext_test.go` (si asserta la forma vieja del checkin)
- Verificación de todo el backend

- [ ] **Step 1:** El `chatcontext.go` incluye los check-ins recientes vía
`checkinLister.List`; como `checkin.CheckIn` ahora trae las 4 reflexiones + win +
avoided, el JSON de contexto **ya las incluye** sin cambios de código (se
serializan los campos del struct). Verificar en `chatcontext_test.go`: si el
test construye `checkin.CheckIn{...Discipline/Note...}` o asserta esos campos,
adaptarlo a los nuevos (sembrar uno con `Espiritual: "x"` y verificar que
aparece en el JSON del contexto).

```go
// En el test de composición del contexto, el fake de checkins:
cks := []checkin.CheckIn{
	{ID: "c1", Date: "2026-06-12", Mood: 7, Energy: 6, Espiritual: "reto día 2", Win: "cerré un trato"},
}
// ...y un assert nuevo:
if !strings.Contains(out, "reto día 2") {
	t.Errorf("el contexto debe incluir la reflexión espiritual: %s", out)
}
```

- [ ] **Step 2: Verificar TODO el backend (vet + suite con -p 1) + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
git add api/internal/ai/chatcontext_test.go
git commit -m "test(ai): el contexto del chat incluye la reflexión 4D del check-in"
```

---

### Task 6: Frontend — lib y dashboard card

**Files:**
- Modify: `web/src/lib/checkins.ts`, `web/src/lib/checkins.test.ts`, `web/src/lib/dashboard.ts`, `web/src/routes/index.tsx`
- Test: `web/src/routes/index.test.tsx` (si asserta «disciplina»)

- [ ] **Step 1: Tipos de la lib.** En `web/src/lib/checkins.ts`, el tipo `CheckIn` y el input pierden `discipline`/`note` y ganan los nuevos:

```ts
export type CheckIn = {
  id: string;
  date: string;
  mood: number;
  energy: number;
  espiritual: string;
  emocional: string;
  fisica: string;
  financiera: string;
  win: string;
  avoided: string;
  commitments: string[];
  created_at: string;
  updated_at: string;
};

export type CheckInInput = {
  date: string;
  mood: number;
  energy: number;
  espiritual: string;
  emocional: string;
  fisica: string;
  financiera: string;
  win: string;
  avoided: string;
  commitments: string[];
};
```

`upsert(input: CheckInInput)` mantiene el POST a `/api/v1/checkins` con el body nuevo. Actualizar `checkins.test.ts` (los literales `CheckIn`/input ganan los campos nuevos y pierden discipline/note).

- [ ] **Step 2: `dashboard.ts`** — el tipo del checkin del snapshot: `discipline: number` → `win: string`.

- [ ] **Step 3: `index.tsx` CheckinCard** (línea ~213): de
`Hecho ✓ · disciplina {s.checkin.discipline}` a
`Hecho ✓{s.checkin.win ? ` · ${s.checkin.win}` : ""}`. Si `index.test.tsx`
asserta «disciplina», adaptar el assert (sembrar `win` en el snap del mock y
verificar que aparece).

- [ ] **Step 4: Verificar (lib + dashboard tests) + build**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run src/lib/checkins.test.ts src/routes/index.test.tsx
```

(El build aún puede fallar por `check-in.tsx` que usa los campos viejos — se arregla en la Task 7. No commitear hasta la Task 7 si el build no pasa, o commitear solo lib+dashboard+index y dejar la página para la 7. **Commitear aquí solo si `npm run build` pasa**; si la página `check-in.tsx` rompe el build, hacer las Tasks 6 y 7 juntas y commitear al final de la 7.)

---

### Task 7: Frontend — formulario `/check-in`

**Files:**
- Modify: `web/src/routes/check-in.tsx`, `web/src/routes/check-in.test.tsx`

- [ ] **Step 1: Tests que fallan** en `check-in.test.tsx` (adaptar los existentes + agregar). Leer el harness actual; los tests deben cubrir: el form muestra los campos nuevos (4 dimensiones, win, evité, compromisos); guardar hace el POST con todos los campos; precarga del check-in de hoy. Ejemplo de assert de guardado:

```tsx
it("guarda el check-in completo con las 4 dimensiones y compromisos", async () => {
  const fetchMock = vi.fn((url: string, opts?: RequestInit) => {
    if (opts?.method === "POST") {
      return Promise.resolve(new Response(JSON.stringify({ id: "c1", date: "2026-06-13", mood: 8, energy: 7, espiritual: "reto", emocional: "", fisica: "", financiera: "", win: "win!", avoided: "", commitments: ["uno"], created_at: "", updated_at: "" }), { status: 200 }));
    }
    return Promise.resolve(new Response("null", { status: 200 })); // getToday → sin check-in
  });
  vi.stubGlobal("fetch", fetchMock);
  renderPage();
  await userEvent.type(await screen.findByLabelText("Espiritual"), "reto");
  await userEvent.type(screen.getByLabelText("Win del día"), "win!");
  // agregar un compromiso
  await userEvent.click(screen.getByRole("button", { name: /agregar compromiso/i }));
  await userEvent.type(screen.getByLabelText("Compromiso 1"), "uno");
  await userEvent.click(screen.getByRole("button", { name: "Guardar" }));
  await waitFor(() => {
    const posted = fetchMock.mock.calls.find(([u, o]) => u === "/api/v1/checkins" && (o as RequestInit)?.method === "POST");
    expect(posted).toBeTruthy();
    const body = JSON.parse((posted![1] as RequestInit).body as string);
    expect(body.espiritual).toBe("reto");
    expect(body.win).toBe("win!");
    expect(body.commitments).toEqual(["uno"]);
  });
});
```

- [ ] **Step 2: Reescribir `check-in.tsx`.** Mantener la lógica (useQuery getToday, useMutation upsert, estados); cambiar los campos:
- Estados: `mood`, `energy` (conservar Sliders), y `espiritual/emocional/fisica/financiera/win/avoided` (string) + `commitments: string[]`.
- Precarga en el `useEffect`/onSuccess de getToday: setear todos los campos desde el check-in (incluidos `commitments`).
- Render (estilo neo-brutalista, reusando `Card`, `Input`, `Button`, `PageTransition`):
  - **¿Cómo estoy?** dos `Slider` (Ánimo/Energía) — como hoy.
  - **Las 4 dimensiones:** 4 `Input` con `aria-label` exacto `Espiritual`,
    `Emocional`, `Fisica`, `Financiera` (label visible con chip de color y el
    nombre), placeholder «¿qué hiciste hoy?».
  - **Win del día** (`aria-label="Win del día"`) y **¿Qué evité hoy?**
    (`aria-label="Qué evité"`).
  - **Compromisos:** lista de `Input` (`aria-label={`Compromiso ${i+1}`}`) con
    botón quitar por fila y un botón **«+ agregar compromiso»** que hace
    `setCommitments([...commitments, ""])`; al guardar se filtran vacíos.
  - Botón **Guardar** (`Button`).
- `mutationFn`: `upsert({ date: today, mood, energy, espiritual, emocional, fisica, financiera, win, avoided, commitments })`.
- Quitar el `Slider` y estado de **Disciplina** y el campo **Nota**.
- El historial (si la página lista check-ins previos con `Á{ci.mood}·E·D{ci.discipline}`) cambia a `Á{ci.mood} · E{ci.energy}` (sin disciplina).

- [ ] **Step 3: Suite completa + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src
git commit -m "feat(web): formulario de check-in de Capitanes (4D, win, evité, compromisos)"
```

(Incluir aquí los archivos de la Task 6 si no se commitearon antes.)

---

### Task 8: Cierre — review, merge, deploy, smoke de producción

- [ ] **Step 1:** Suites completas (backend `-p 1 ./...` + frontend + build) y smoke local de acciones (`/tmp/smoke_actions.sh`).
- [ ] **Step 2:** Rebuild docker; smoke local del check-in nuevo: `POST /checkins` con 4D+compromisos → `GET /checkins/today` lo devuelve; registrar mood/energy por chat (acción) → confirmar → `GET /checkins/today` muestra los números nuevos sin pisar las reflexiones.
- [ ] **Step 3:** Review final holística (subagente), nits.
- [ ] **Step 4:** Merge `--no-ff` a `main` + push. **Verificar el deploy** (auto-deploy o Deploy manual de Coolify; la migración 0013 se aplica al arrancar).
- [ ] **Step 5:** Smoke de producción: guardar un check-in completo con las 4 dimensiones y un compromiso → `GET /checkins/today` lo devuelve íntegro.
- [ ] **Step 6:** Bitácora en `docs/superpowers/sesiones/` y push.

---

## Notas para el ejecutor

- Los nombres de columnas/params generados por sqlc mandan (`DimEspiritual`, `Commitments []byte`). Ajustar a lo generado.
- El backend queda con `go build ./...` roto entre las Tasks 1 y 4 (campos viejos en handler/dashboard/ai); cada una verifica solo su paquete; el build completo se exige verde en la Task 4. El frontend, roto entre 1 y 7; suite web verde desde la 7.
- Las 7 acciones del chat y el deshacer son INVARIANTES salvo la de check-in (que cambia a solo-métricas a propósito). Los tests viejos de la acción check-in con discipline se reescriben a mood/energy, no se "arreglan" al revés.
- Compat: no hay propuestas de check-in pendientes en producción; los `result` de acciones de check-in ya confirmadas pre-migración tienen forma vieja — su undo es best-effort y no debe romper (el `json.Unmarshal` a `checkinResult` nuevo simplemente ignora campos viejos; si `existed` sale false por defecto, intentará borrar el día, lo cual es inocuo o no encuentra nada).
