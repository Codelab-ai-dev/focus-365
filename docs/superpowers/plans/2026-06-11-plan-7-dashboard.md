# Plan 7 — Dashboard (Centro de mando) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Construir un dashboard en `/` que reúne en un snapshot agregado el estado del día de las 5 dimensiones (racha, superávit, ánimo/energía, check-in, entreno, metas), más una barra superior persistente para navegar entre módulos.

**Architecture:** Un endpoint backend `GET /api/v1/dashboard` compone los 5 servicios existentes (checkin, finance, training, habits, goals) en un único snapshot todo-o-nada. El frontend reemplaza el home por un dashboard de tarjetas (Layout B: 2 grandes + 4 chicas + banda IA placeholder) que consume ese snapshot con una sola query, y agrega una `TopBar` en el root shell.

**Tech Stack:** Go 1.23 (chi v5, sqlc, pgx), PostgreSQL 16, React 18 + Vite + TanStack Router/Query + Tailwind + Vitest. Sin migraciones ni queries sqlc nuevas.

**Spec:** `docs/superpowers/specs/2026-06-11-plan-7-dashboard-design.md`

---

## Convenciones del repo (leer antes de empezar)

- **Go:** todos los comandos `go`/`sqlc` corren desde `api/` con `GOTOOLCHAIN=local`. Nunca editar `go.mod`/`go.sum`.
- **make check** (desde `api/`): `GOTOOLCHAIN=local go vet ./...` + `go test -p 1 ./...`. Los tests de integración necesitan `TEST_DATABASE_URL` (`postgres://focus:changeme@localhost:5544/focus365?sslmode=disable`); si no está seteada, `testutil.NewDB` hace skip.
- **Frontend:** desde `web/`. Tests con `npx vitest run`. El árbol de rutas (`routeTree.gen.ts`) se regenera con `npx vite build`.
- **Commits y comentarios en español.**
- **File structure** (nuevo paquete `api/internal/dashboard`):
  - `types.go` — vistas JSON (`Snapshot`, `StreakView`, etc.) + helper puro `countActive`.
  - `service.go` — `Service` que compone los 5 servicios y arma el `Snapshot`.
  - `handler.go` — `Routes` + handler HTTP + `parseTodayParam`.
  - `types_test.go` — unit tests del helper puro `countActive` y el mapeo.
  - `handler_test.go` — tests de integración con `testutil.NewDB`.
- Frontend nuevos: `web/src/lib/dashboard.ts`, `web/src/components/TopBar.tsx`. Modificados: `web/src/routes/__root.tsx`, `web/src/routes/index.tsx`. Tests: `web/src/lib/dashboard.test.ts`, `web/src/components/TopBar.test.tsx`, `web/src/routes/index.test.tsx`.

---

## Task 1: Vistas del dashboard + helper `countActive`

**Files:**
- Create: `api/internal/dashboard/types.go`
- Test: `api/internal/dashboard/types_test.go`

Tipos puros (sin DB) que modelan la respuesta JSON, y el helper `countActive` que cuenta cuántas de las 5 dimensiones tienen algo que mostrar.

- [ ] **Step 1: Escribir el test que falla**

Crear `api/internal/dashboard/types_test.go`:

```go
package dashboard

import "testing"

func TestCountActiveEmpty(t *testing.T) {
	s := &Snapshot{}
	if got := countActive(s); got != 0 {
		t.Errorf("countActive(vacío) = %d, want 0", got)
	}
}

func TestCountActiveAll(t *testing.T) {
	s := &Snapshot{
		Checkin:  &CheckinView{Present: true, Mood: 8, Energy: 6, Discipline: 9},
		Streak:   StreakView{BestCurrent: 12, DoneToday: 2, Total: 4},
		Finance:  FinanceView{Cycle: "2026-06", Net: 320000, Status: "verde"},
		Training: TrainingView{TrainedToday: true, Type: "Fuerza"},
		Goals:    GoalsView{Active: 3, AvgProgress: 40, Overdue: 1},
	}
	if got := countActive(s); got != 5 {
		t.Errorf("countActive(todo) = %d, want 5", got)
	}
}

func TestCountActiveSubset(t *testing.T) {
	// Solo hábitos activos y metas activas → 2.
	s := &Snapshot{
		Streak:  StreakView{Total: 4},
		Finance: FinanceView{Status: "pendiente"},
		Goals:   GoalsView{Active: 1},
	}
	if got := countActive(s); got != 2 {
		t.Errorf("countActive(subset) = %d, want 2", got)
	}
}
```

- [ ] **Step 2: Correr el test para verificar que falla**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/dashboard/ -run TestCountActive -v`
Expected: FAIL — no compila (`Snapshot`, `countActive` indefinidos).

- [ ] **Step 3: Escribir `types.go`**

Crear `api/internal/dashboard/types.go`:

```go
// Package dashboard compone un snapshot agregado del día a partir de los cinco
// servicios de dominio (checkin, finance, training, habits, goals). No tiene
// tablas ni queries propias: es una vista de solo lectura.
package dashboard

// StreakView resume las rachas de hábitos.
// BestCurrent = mayor current_streak entre hábitos activos.
// DoneToday = cuántos activos están marcados hoy. Total = nº de hábitos activos.
type StreakView struct {
	BestCurrent int  `json:"best_current"`
	DoneToday   int  `json:"done_today"`
	Total       int  `json:"total"`
}

// FinanceView resume el ciclo de pago vigente (net en centavos).
type FinanceView struct {
	Cycle  string `json:"cycle"`
	Net    int64  `json:"net"`
	Status string `json:"status"`
}

// CheckinView resume el check-in de hoy. El campo `checkin` del Snapshot
// serializa null cuando no hay check-in (puntero nil).
type CheckinView struct {
	Present    bool `json:"present"`
	Mood       int  `json:"mood"`
	Energy     int  `json:"energy"`
	Discipline int  `json:"discipline"`
}

// TrainingView indica si entrenó hoy y de qué tipo (vacío si no entrenó).
type TrainingView struct {
	TrainedToday bool   `json:"trained_today"`
	Type         string `json:"type"`
}

// GoalsView resume las metas activas. AvgProgress es el promedio entero del
// progreso de las activas (0 si no hay). Overdue = cuántas activas están vencidas.
type GoalsView struct {
	Active      int `json:"active"`
	AvgProgress int `json:"avg_progress"`
	Overdue     int `json:"overdue"`
}

// Snapshot es la vista agregada que devuelve el endpoint.
type Snapshot struct {
	Streak           StreakView   `json:"streak"`
	Finance          FinanceView  `json:"finance"`
	Checkin          *CheckinView `json:"checkin"`
	Training         TrainingView `json:"training"`
	Goals            GoalsView    `json:"goals"`
	DimensionsActive int          `json:"dimensions_active"`
}

// countActive cuenta cuántas de las 5 dimensiones tienen algo que mostrar hoy.
func countActive(s *Snapshot) int {
	n := 0
	if s.Checkin != nil {
		n++
	}
	if s.Streak.Total > 0 {
		n++
	}
	if s.Finance.Status != "pendiente" {
		n++
	}
	if s.Training.TrainedToday {
		n++
	}
	if s.Goals.Active > 0 {
		n++
	}
	return n
}
```

- [ ] **Step 4: Correr el test para verificar que pasa**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/dashboard/ -run TestCountActive -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/dashboard/types.go internal/dashboard/types_test.go
git commit -m "feat(dashboard): vistas JSON del snapshot y helper countActive"
```

---

## Task 2: Servicio agregador (`Snapshot`)

**Files:**
- Create: `api/internal/dashboard/service.go`
- Test: `api/internal/dashboard/types_test.go` (agregar tests de mapeo puro)

El servicio recibe punteros a los 5 servicios existentes y arma el `Snapshot`. El mapeo de cada slice de dominio a su vista se hace con helpers puros (testeable sin DB).

- [ ] **Step 1: Escribir los tests de mapeo (puros) que fallan**

Agregar a `api/internal/dashboard/types_test.go`:

```go
import (
	"testing"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/training"
)

func TestStreakViewFromHabits(t *testing.T) {
	hs := []habits.Habit{
		{CurrentStreak: 12, DoneToday: true},
		{CurrentStreak: 3, DoneToday: false},
	}
	v := streakView(hs)
	if v.BestCurrent != 12 || v.DoneToday != 1 || v.Total != 2 {
		t.Errorf("streakView = %+v, want {12 1 2}", v)
	}
}

func TestCheckinViewNilWhenAbsent(t *testing.T) {
	if v := checkinView(nil); v != nil {
		t.Errorf("checkinView(nil) = %+v, want nil", v)
	}
	c := &checkin.CheckIn{Mood: 8, Energy: 6, Discipline: 9}
	v := checkinView(c)
	if v == nil || !v.Present || v.Mood != 8 || v.Energy != 6 || v.Discipline != 9 {
		t.Errorf("checkinView = %+v, want present 8/6/9", v)
	}
}

func TestTrainingViewFirstWorkout(t *testing.T) {
	if v := trainingView(nil); v.TrainedToday || v.Type != "" {
		t.Errorf("trainingView(vacío) = %+v, want no entrenó", v)
	}
	ws := []training.Workout{{Type: "Fuerza"}, {Type: "Cardio"}}
	v := trainingView(ws)
	if !v.TrainedToday || v.Type != "Fuerza" {
		t.Errorf("trainingView = %+v, want Fuerza ✓", v)
	}
}

func TestGoalsViewAverages(t *testing.T) {
	if v := goalsView(nil); v.Active != 0 || v.AvgProgress != 0 || v.Overdue != 0 {
		t.Errorf("goalsView(vacío) = %+v, want ceros", v)
	}
	gs := []goals.Goal{
		{Progress: 20, Overdue: true},
		{Progress: 60, Overdue: false},
	}
	v := goalsView(gs)
	if v.Active != 2 || v.AvgProgress != 40 || v.Overdue != 1 {
		t.Errorf("goalsView = %+v, want {2 40 1}", v)
	}
}

func TestFinanceViewFromSummary(t *testing.T) {
	cs := &finance.CycleSummary{Cycle: "2026-06", Net: 320000, Status: "verde"}
	v := financeView(cs)
	if v.Cycle != "2026-06" || v.Net != 320000 || v.Status != "verde" {
		t.Errorf("financeView = %+v", v)
	}
}

var _ = time.Now // evita import no usado si se reordena
```

- [ ] **Step 2: Correr para verificar que falla**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/dashboard/ -run "TestStreakView|TestCheckinView|TestTrainingView|TestGoalsView|TestFinanceView" -v`
Expected: FAIL — `streakView`, `checkinView`, etc. indefinidos.

- [ ] **Step 3: Escribir `service.go`**

Crear `api/internal/dashboard/service.go`:

```go
package dashboard

import (
	"context"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/training"
	"github.com/google/uuid"
)

// Service compone los cinco servicios de dominio para armar el snapshot.
type Service struct {
	checkins *checkin.Service
	finance  *finance.Service
	training *training.Service
	habits   *habits.Service
	goals    *goals.Service
}

// NewService inyecta los servicios existentes (punteros compartidos con server.go).
func NewService(c *checkin.Service, f *finance.Service, t *training.Service,
	h *habits.Service, g *goals.Service) *Service {
	return &Service{checkins: c, finance: f, training: t, habits: h, goals: g}
}

// Snapshot consulta cada servicio para today y arma la vista agregada. Es
// todo-o-nada: si cualquier sub-llamada falla, propaga el error (→ 500).
func (s *Service) Snapshot(ctx context.Context, userID uuid.UUID, today time.Time) (*Snapshot, error) {
	hs, err := s.habits.List(ctx, userID, false, today)
	if err != nil {
		return nil, err
	}
	sum, err := s.finance.Summary(ctx, userID, finance.Cycle(today), today)
	if err != nil {
		return nil, err
	}
	ci, err := s.checkins.Today(ctx, userID, today)
	if err != nil {
		return nil, err
	}
	ws, err := s.training.ListWorkouts(ctx, userID, &today, &today)
	if err != nil {
		return nil, err
	}
	gs, err := s.goals.List(ctx, userID, "active", today)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{
		Streak:   streakView(hs),
		Finance:  financeView(sum),
		Checkin:  checkinView(ci),
		Training: trainingView(ws),
		Goals:    goalsView(gs),
	}
	snap.DimensionsActive = countActive(snap)
	return snap, nil
}

func streakView(hs []habits.Habit) StreakView {
	v := StreakView{Total: len(hs)}
	for _, h := range hs {
		if h.CurrentStreak > v.BestCurrent {
			v.BestCurrent = h.CurrentStreak
		}
		if h.DoneToday {
			v.DoneToday++
		}
	}
	return v
}

func financeView(cs *finance.CycleSummary) FinanceView {
	return FinanceView{Cycle: cs.Cycle, Net: cs.Net, Status: cs.Status}
}

func checkinView(c *checkin.CheckIn) *CheckinView {
	if c == nil {
		return nil
	}
	return &CheckinView{Present: true, Mood: c.Mood, Energy: c.Energy, Discipline: c.Discipline}
}

func trainingView(ws []training.Workout) TrainingView {
	if len(ws) == 0 {
		return TrainingView{}
	}
	return TrainingView{TrainedToday: true, Type: ws[0].Type}
}

func goalsView(gs []goals.Goal) GoalsView {
	v := GoalsView{Active: len(gs)}
	sum := 0
	for _, g := range gs {
		sum += int(g.Progress)
		if g.Overdue {
			v.Overdue++
		}
	}
	if len(gs) > 0 {
		v.AvgProgress = sum / len(gs)
	}
	return v
}
```

- [ ] **Step 4: Correr para verificar que pasa**

Run: `cd api && GOTOOLCHAIN=local go test ./internal/dashboard/ -v`
Expected: PASS (todos los unit tests).

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/dashboard/service.go internal/dashboard/types_test.go
git commit -m "feat(dashboard): servicio agregador que compone las 5 dimensiones"
```

---

## Task 3: Handler HTTP + montaje en server

**Files:**
- Create: `api/internal/dashboard/handler.go`
- Modify: `api/internal/server/server.go` (grupo `RequireAuth`)

- [ ] **Step 1: Escribir `handler.go`**

Crear `api/internal/dashboard/handler.go`:

```go
package dashboard

import (
	"net/http"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/httpx"
	"github.com/go-chi/chi/v5"
)

const dateLayout = "2006-01-02"

// Routes monta el endpoint del dashboard (bajo RequireAuth en server.go).
func Routes(svc *Service) http.Handler {
	r := chi.NewRouter()
	r.Get("/", handleSnapshot(svc))
	return r
}

func handleSnapshot(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			httpx.WriteErr(w, http.StatusUnauthorized, "no autorizado")
			return
		}
		snap, err := svc.Snapshot(r.Context(), userID, parseTodayParam(r))
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, "error interno")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, snap)
	}
}

// parseTodayParam lee ?today=YYYY-MM-DD (zona del cliente). Si falta o es
// inválido, cae al día UTC del server. Mismo patrón que metas/hábitos.
func parseTodayParam(r *http.Request) time.Time {
	if s := r.URL.Query().Get("today"); s != "" {
		if t, err := time.Parse(dateLayout, s); err == nil {
			return t
		}
	}
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
```

- [ ] **Step 2: Verificar que compila**

Run: `cd api && GOTOOLCHAIN=local go build ./internal/dashboard/`
Expected: sin errores.

- [ ] **Step 3: Montar en `server.go`**

En `api/internal/server/server.go`, dentro del grupo `RequireAuth` (donde están los `r.Mount("/goals", ...)` etc.), agregar el import `"github.com/focus365/api/internal/dashboard"` y, después de que `checkinSvc`, `financeSvc`, `trainingSvc`, `habitsSvc`, `goalsSvc` ya estén construidos:

```go
dashboardSvc := dashboard.NewService(checkinSvc, financeSvc, trainingSvc, habitsSvc, goalsSvc)
r.Mount("/dashboard", dashboard.Routes(dashboardSvc))
```

- [ ] **Step 4: Verificar build y vet**

Run: `cd api && GOTOOLCHAIN=local go vet ./...`
Expected: sin errores.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/dashboard/handler.go internal/server/server.go
git commit -m "feat(dashboard): handler GET /dashboard y montaje en server bajo RequireAuth"
```

---

## Task 4: Tests de integración del endpoint

**Files:**
- Create: `api/internal/dashboard/handler_test.go`

Usa `testutil.NewDB` y siembra datos vía los servicios reales (no por HTTP) para verificar el snapshot agregado.

- [ ] **Step 1: Escribir el test que falla**

Crear `api/internal/dashboard/handler_test.go`:

```go
package dashboard_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/dashboard"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/focus365/api/internal/training"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const today = "2026-06-11"

type env struct {
	h        http.Handler
	auth     *auth.Service
	checkins *checkin.Service
	finance  *finance.Service
	training *training.Service
	habits   *habits.Service
	goals    *goals.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	tm := auth.NewTokenManager("secret")

	ci := checkin.NewService(q)
	fi := finance.NewService(q)
	tr := training.NewService(q, pool)
	ha := habits.NewService(q)
	go_ := goals.NewService(q)
	svc := dashboard.NewService(ci, fi, tr, ha, go_)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/dashboard", dashboard.Routes(svc))
	})
	return &env{
		h: r, auth: auth.NewService(q, tm),
		checkins: ci, finance: fi, training: tr, habits: ha, goals: go_,
	}
}

func (e *env) user(t *testing.T, email string) (uuid.UUID, string) {
	t.Helper()
	u, err := e.auth.Register(context.Background(), email, "p4ssword", "User")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	access, _, err := e.auth.IssueTokens(u.ID)
	if err != nil {
		t.Fatalf("IssueTokens: %v", err)
	}
	return u.ID, access
}

func dayTime(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return d
}

func get(t *testing.T, h http.Handler, tok string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/dashboard?today="+today, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestEmptyDashboard(t *testing.T) {
	e := newEnv(t)
	_, tok := e.user(t, "empty@b.com")
	rec, body := get(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body["checkin"] != nil {
		t.Errorf("checkin = %v, want null", body["checkin"])
	}
	if body["dimensions_active"].(float64) != 0 {
		t.Errorf("dimensions_active = %v, want 0", body["dimensions_active"])
	}
	streak := body["streak"].(map[string]any)
	if streak["total"].(float64) != 0 {
		t.Errorf("streak.total = %v, want 0", streak["total"])
	}
}

func TestPopulatedDashboard(t *testing.T) {
	e := newEnv(t)
	uid, tok := e.user(t, "full@b.com")
	ctx := context.Background()
	td := dayTime(t, today)

	// Hábito marcado hoy.
	h, err := e.habits.Create(ctx, uid, habits.HabitInput{Name: "Leer"}, td)
	if err != nil {
		t.Fatalf("crear hábito: %v", err)
	}
	if _, err := e.habits.SetCheck(ctx, uid, mustUUID(t, h.ID), td, true, td); err != nil {
		t.Fatalf("marcar hábito: %v", err)
	}
	// Transacción de ingreso en el ciclo de hoy.
	if _, err := e.finance.Create(ctx, uid, finance.Input{
		Type: "income", Amount: 320000, OccurredOn: today, Category: "Sueldo",
	}); err != nil {
		t.Fatalf("crear transacción: %v", err)
	}
	// Check-in de hoy.
	if _, err := e.checkins.Upsert(ctx, uid, checkin.Input{
		Date: today, Mood: 8, Energy: 6, Discipline: 9,
	}); err != nil {
		t.Fatalf("check-in: %v", err)
	}
	// Workout de hoy.
	if _, err := e.training.CreateWorkout(ctx, uid, training.WorkoutInput{
		Date: today, Type: "Fuerza",
	}); err != nil {
		t.Fatalf("workout: %v", err)
	}
	// Meta activa.
	if _, err := e.goals.Create(ctx, uid, goals.GoalInput{
		Title: "Correr 10k", Dimension: "entrenamiento",
	}, td); err != nil {
		t.Fatalf("meta: %v", err)
	}

	rec, body := get(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body["dimensions_active"].(float64) != 5 {
		t.Errorf("dimensions_active = %v, want 5", body["dimensions_active"])
	}
	streak := body["streak"].(map[string]any)
	if streak["total"].(float64) != 1 || streak["done_today"].(float64) != 1 {
		t.Errorf("streak = %v, want total 1 done 1", streak)
	}
	ci := body["checkin"].(map[string]any)
	if ci["mood"].(float64) != 8 {
		t.Errorf("checkin.mood = %v, want 8", ci["mood"])
	}
	tr := body["training"].(map[string]any)
	if tr["trained_today"] != true || tr["type"] != "Fuerza" {
		t.Errorf("training = %v, want Fuerza ✓", tr)
	}
	gl := body["goals"].(map[string]any)
	if gl["active"].(float64) != 1 {
		t.Errorf("goals.active = %v, want 1", gl["active"])
	}
}

func TestOverdueGoalsCounted(t *testing.T) {
	e := newEnv(t)
	uid, tok := e.user(t, "ov@b.com")
	ctx := context.Background()
	td := dayTime(t, today)
	dl := dayTime(t, "2026-06-01")
	if _, err := e.goals.Create(ctx, uid, goals.GoalInput{
		Title: "Entrega", Dimension: "general", Deadline: &dl,
	}, td); err != nil {
		t.Fatalf("meta: %v", err)
	}
	_, body := get(t, e.h, tok)
	gl := body["goals"].(map[string]any)
	if gl["overdue"].(float64) != 1 {
		t.Errorf("goals.overdue = %v, want 1", gl["overdue"])
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec, _ := get(t, e.h, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	uidA, _ := e.user(t, "uA@b.com")
	_, tokB := e.user(t, "uB@b.com")
	ctx := context.Background()
	td := dayTime(t, today)
	if _, err := e.goals.Create(ctx, uidA, goals.GoalInput{
		Title: "Privada", Dimension: "general",
	}, td); err != nil {
		t.Fatalf("meta A: %v", err)
	}
	_, body := get(t, e.h, tokB)
	gl := body["goals"].(map[string]any)
	if gl["active"].(float64) != 0 {
		t.Errorf("B ve %v metas activas de A, want 0", gl["active"])
	}
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("uuid %s: %v", s, err)
	}
	return id
}

var _ = bytes.NewReader
```

> **Nota para el implementador:** Confirmá las firmas exactas de los inputs de cada servicio antes de correr (`habits.HabitInput`, `finance.Input` con sus campos `Type/Amount/OccurredOn/Category`, `checkin.Input` con `Date/Mood/Energy/Discipline`, `training.WorkoutInput` con `Date/Type`, `goals.GoalInput` con `Title/Dimension/Deadline *time.Time`). Si algún nombre de campo difiere, ajustá el literal — la estructura del test no cambia. `training.NewService` toma `(q, pool)`. Quitá `var _ = bytes.NewReader` y el import `bytes` si no hacen falta.

- [ ] **Step 2: Correr para verificar que falla (o pasa si DB lista)**

Run: `cd api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test ./internal/dashboard/ -run "TestEmptyDashboard|TestPopulated|TestOverdue|TestRequiresAuth|TestUserIsolation" -v`
Expected: primero puede fallar por nombres de campos; ajustar hasta PASS (5 tests). Si la DB no está levantada, hace skip.

- [ ] **Step 3: Asegurar que pasan**

Iterar sobre los literales de input hasta que los 5 tests pasen.

- [ ] **Step 4: `make check` completo**

Run: `cd api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go vet ./... && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test -p 1 ./...`
Expected: todos los paquetes ok.

- [ ] **Step 5: Commit**

```bash
cd api && git add internal/dashboard/handler_test.go
git commit -m "test(dashboard): integración del endpoint agregado (vacío, poblado, vencidas, auth, aislamiento)"
```

---

## Task 5: Lib frontend `dashboard.ts`

**Files:**
- Create: `web/src/lib/dashboard.ts`
- Test: `web/src/lib/dashboard.test.ts`

- [ ] **Step 1: Escribir el test que falla**

Crear `web/src/lib/dashboard.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { getDashboard, type Snapshot } from "./dashboard";
import { setAccessToken } from "./api";

const snap: Snapshot = {
  streak: { best_current: 12, done_today: 2, total: 4 },
  finance: { cycle: "2026-06", net: 320000, status: "verde" },
  checkin: { present: true, mood: 8, energy: 6, discipline: 9 },
  training: { trained_today: true, type: "Fuerza" },
  goals: { active: 3, avg_progress: 40, overdue: 1 },
  dimensions_active: 5,
};

function okJson(data: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(data), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })
  );
}

describe("getDashboard", () => {
  beforeEach(() => setAccessToken("tok"));
  afterEach(() => vi.restoreAllMocks());

  it("hace GET a /api/v1/dashboard con ?today=", async () => {
    const fetchMock = vi.fn(() => okJson(snap));
    vi.stubGlobal("fetch", fetchMock);

    const result = await getDashboard();

    expect(result.dimensions_active).toBe(5);
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toMatch(/^\/api\/v1\/dashboard\?today=\d{4}-\d{2}-\d{2}$/);
    const opts = fetchMock.mock.calls[0][1] as RequestInit | undefined;
    expect(opts?.method ?? "GET").toBe("GET");
  });

  it("acepta checkin null", async () => {
    const fetchMock = vi.fn(() => okJson({ ...snap, checkin: null, dimensions_active: 4 }));
    vi.stubGlobal("fetch", fetchMock);
    const result = await getDashboard();
    expect(result.checkin).toBeNull();
  });
});
```

- [ ] **Step 2: Correr para verificar que falla**

Run: `cd web && npx vitest run src/lib/dashboard.test.ts`
Expected: FAIL — `./dashboard` no existe.

- [ ] **Step 3: Escribir `dashboard.ts`**

Crear `web/src/lib/dashboard.ts`:

```ts
import { apiFetch } from "./api";

export type StreakView = {
  best_current: number;
  done_today: number;
  total: number;
};

export type FinanceView = {
  cycle: string; // YYYY-MM
  net: number; // centavos
  status: "pendiente" | "verde" | "rojo";
};

export type CheckinView = {
  present: boolean;
  mood: number;
  energy: number;
  discipline: number;
};

export type TrainingView = {
  trained_today: boolean;
  type: string;
};

export type GoalsView = {
  active: number;
  avg_progress: number;
  overdue: number;
};

export type Snapshot = {
  streak: StreakView;
  finance: FinanceView;
  checkin: CheckinView | null;
  training: TrainingView;
  goals: GoalsView;
  dimensions_active: number;
};

// todayString calcula la fecha local del usuario como YYYY-MM-DD (sin UTC).
export function todayString(date = new Date()): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export function getDashboard(): Promise<Snapshot> {
  return apiFetch<Snapshot>(`/api/v1/dashboard?today=${todayString()}`);
}
```

- [ ] **Step 4: Correr para verificar que pasa**

Run: `cd web && npx vitest run src/lib/dashboard.test.ts`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
cd web && git add src/lib/dashboard.ts src/lib/dashboard.test.ts
git commit -m "feat(dashboard): lib frontend getDashboard y tipos del snapshot"
```

---

## Task 6: Componente `TopBar`

**Files:**
- Create: `web/src/components/TopBar.tsx`
- Modify: `web/src/routes/__root.tsx`
- Test: `web/src/components/TopBar.test.tsx`

Barra superior persistente. Sólo se renderiza con usuario (`useAuth`). Links a los módulos, resalta el activo, botón "Salir".

- [ ] **Step 1: Escribir el test que falla**

Crear `web/src/components/TopBar.test.tsx`:

```tsx
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";

const mockAuth = { user: null as null | { id: string; email: string; name: string } };
vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: mockAuth.user,
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { TopBar } from "./TopBar";

function renderBar() {
  const rootRoute = createRootRoute({ component: TopBar });
  const home = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: () => <div>home</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([home]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  // @ts-ignore router de prueba
  render(<RouterProvider router={router} />);
}

describe("TopBar", () => {
  afterEach(() => vi.restoreAllMocks());

  it("no muestra nada sin usuario", () => {
    mockAuth.user = null;
    renderBar();
    expect(screen.queryByText("Focus 365")).not.toBeInTheDocument();
  });

  it("muestra links con usuario", () => {
    mockAuth.user = { id: "u1", email: "a@b.com", name: "Ana" };
    renderBar();
    expect(screen.getByText("Focus 365")).toBeInTheDocument();
    expect(screen.getByText("Finanzas")).toBeInTheDocument();
    expect(screen.getByText("Salir")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Correr para verificar que falla**

Run: `cd web && npx vitest run src/components/TopBar.test.tsx`
Expected: FAIL — `./TopBar` no existe.

- [ ] **Step 3: Escribir `TopBar.tsx`**

Crear `web/src/components/TopBar.tsx`:

```tsx
import { Link, useRouterState } from "@tanstack/react-router";
import { useAuth } from "@/lib/auth";

const LINKS: { to: string; label: string }[] = [
  { to: "/", label: "Inicio" },
  { to: "/check-in", label: "Check-in" },
  { to: "/finanzas", label: "Finanzas" },
  { to: "/entrenamiento", label: "Entreno" },
  { to: "/disciplina", label: "Disciplina" },
  { to: "/metas", label: "Metas" },
];

// TopBar es la barra de navegación persistente. Sólo se muestra con usuario;
// en /login y /register useAuth devuelve user null y no se renderiza.
export function TopBar() {
  const { user, logout } = useAuth();
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  if (!user) return null;

  return (
    <nav className="flex items-center justify-between border-b border-ink-700 bg-ink-900 px-4 py-3">
      <div className="flex items-center gap-4">
        <Link to="/" className="text-sm font-extrabold text-amber-brand">
          Focus 365
        </Link>
        <div className="flex gap-3 text-sm">
          {LINKS.map((l) => (
            <Link
              key={l.to}
              to={l.to}
              className={
                pathname === l.to
                  ? "font-bold text-amber-brand"
                  : "text-sand-400 hover:text-sand-100"
              }
            >
              {l.label}
            </Link>
          ))}
        </div>
      </div>
      <button onClick={logout} className="text-sm text-sand-400 hover:text-sand-100">
        Salir
      </button>
    </nav>
  );
}
```

- [ ] **Step 4: Correr para verificar que pasa**

Run: `cd web && npx vitest run src/components/TopBar.test.tsx`
Expected: PASS (2 tests).

- [ ] **Step 5: Montar en `__root.tsx`**

Reemplazar `web/src/routes/__root.tsx` por:

```tsx
import { createRootRoute, Outlet } from "@tanstack/react-router";
import { TopBar } from "@/components/TopBar";

export const Route = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-ink-950 text-sand-100">
      <TopBar />
      <Outlet />
    </div>
  ),
});
```

- [ ] **Step 6: Verificar build del frontend**

Run: `cd web && npx vite build`
Expected: build OK (regenera `routeTree.gen.ts`).

- [ ] **Step 7: Commit**

```bash
cd web && git add src/components/TopBar.tsx src/components/TopBar.test.tsx src/routes/__root.tsx src/routeTree.gen.ts
git commit -m "feat(dashboard): TopBar persistente montada en el root shell"
```

---

## Task 7: Página dashboard (`index.tsx`, Layout B)

**Files:**
- Modify: `web/src/routes/index.tsx` (reemplazo completo)
- Test: `web/src/routes/index.test.tsx`

El home pasa de lista de links a dashboard. Una sola query, un loading y un error con reintento. Layout B: banda IA + saludo + 2 tarjetas grandes (Racha, Superávit) + 4 chicas (Ánimo/Energía, Check-in, Entreno, Metas).

- [ ] **Step 1: Escribir el test que falla**

Crear `web/src/routes/index.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  createMemoryHistory,
} from "@tanstack/react-router";
import type { Snapshot } from "@/lib/dashboard";

vi.mock("@/lib/auth", () => ({
  useAuth: () => ({
    user: { id: "u1", email: "a@b.com", name: "Ana" },
    login: vi.fn(),
    register: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}));

import { Route as IndexRoute } from "./index";

function makeSnap(overrides: Partial<Snapshot> = {}): Snapshot {
  return {
    streak: { best_current: 12, done_today: 2, total: 4 },
    finance: { cycle: "2026-06", net: 320000, status: "verde" },
    checkin: { present: true, mood: 8, energy: 6, discipline: 9 },
    training: { trained_today: true, type: "Fuerza" },
    goals: { active: 3, avg_progress: 40, overdue: 1 },
    dimensions_active: 4,
    ...overrides,
  };
}

function okJson(data: unknown) {
  return Promise.resolve(
    new Response(JSON.stringify(data), { status: 200 })
  );
}

function renderPage() {
  const rootRoute = createRootRoute();
  const route = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: IndexRoute.options.component,
  });
  const login = createRoute({
    getParentRoute: () => rootRoute,
    path: "/login",
    component: () => <div>login</div>,
  });
  const module = createRoute({
    getParentRoute: () => rootRoute,
    path: "/disciplina",
    component: () => <div>disciplina</div>,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([route, login, module]),
    history: createMemoryHistory({ initialEntries: ["/"] }),
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      {/* @ts-ignore router de prueba */}
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}

describe("DashboardPage", () => {
  beforeEach(() => vi.stubGlobal("fetch", vi.fn(() => okJson(makeSnap()))));
  afterEach(() => vi.restoreAllMocks());

  it("muestra el saludo con el nombre y las dimensiones", async () => {
    renderPage();
    expect(await screen.findByText(/Hola, Ana/)).toBeInTheDocument();
    expect(screen.getByText(/4 dimensiones en marcha/)).toBeInTheDocument();
  });

  it("muestra la racha y el superávit en MXN", async () => {
    renderPage();
    expect(await screen.findByText(/12/)).toBeInTheDocument();
    // 320000 centavos = $3,200.00
    expect(screen.getByText(/\$3,200\.00/)).toBeInTheDocument();
  });

  it("muestra la banda de IA placeholder", async () => {
    renderPage();
    expect(await screen.findByText(/Tu insight del día llega pronto/)).toBeInTheDocument();
  });

  it("muestra aviso de metas vencidas", async () => {
    renderPage();
    expect(await screen.findByText(/1 vencida/)).toBeInTheDocument();
  });

  it("muestra 'Sin check-in hoy' cuando checkin es null", async () => {
    vi.stubGlobal("fetch", vi.fn(() => okJson(makeSnap({ checkin: null, dimensions_active: 3 }))));
    renderPage();
    expect(await screen.findByText(/Sin check-in hoy/)).toBeInTheDocument();
  });

  it("cada tarjeta linkea a su módulo (racha → /disciplina)", async () => {
    renderPage();
    const link = await screen.findByRole("link", { name: /Racha/ });
    expect(link.getAttribute("href")).toBe("/disciplina");
  });
});
```

- [ ] **Step 2: Correr para verificar que falla**

Run: `cd web && npx vitest run src/routes/index.test.tsx`
Expected: FAIL — el index actual no tiene dashboard.

- [ ] **Step 3: Reemplazar `index.tsx`**

Reemplazar `web/src/routes/index.tsx` por:

```tsx
import { createFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/lib/auth";
import { getDashboard, todayString, type Snapshot } from "@/lib/dashboard";
import { formatMXN } from "@/lib/finances";

export const Route = createFileRoute("/")({ component: DashboardPage });

function DashboardPage() {
  const { user } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (!user) navigate({ to: "/login" });
  }, [user, navigate]);

  const query = useQuery({
    queryKey: ["dashboard", todayString()],
    queryFn: getDashboard,
    enabled: !!user,
  });

  if (!user) return null;

  if (query.isLoading) {
    return <p className="p-6 text-sand-400">Cargando tu día…</p>;
  }

  if (query.isError || !query.data) {
    return (
      <div className="p-6">
        <p className="text-streak">No pudimos cargar tu día.</p>
        <button
          onClick={() => query.refetch()}
          className="mt-3 rounded-lg border border-ink-700 px-4 py-2 text-sm font-bold text-sand-400"
        >
          Reintentar
        </button>
      </div>
    );
  }

  const s = query.data;
  const fecha = new Date().toLocaleDateString("es-MX", {
    weekday: "long",
    day: "numeric",
    month: "long",
  });

  return (
    <div className="p-6">
      <AIBand />
      <p className="mt-4 text-sm text-sand-400">
        Hola, <span className="text-amber-brand">{user.name}</span> · {fecha} ·{" "}
        {s.dimensions_active} dimensiones en marcha
      </p>

      <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
        <StreakCard s={s} />
        <FinanceCard s={s} />
      </div>

      <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <MoodCard s={s} />
        <CheckinCard s={s} />
        <TrainingCard s={s} />
        <GoalsCard s={s} />
      </div>
    </div>
  );
}

function AIBand() {
  return (
    <div className="rounded-lg border border-dashed border-amber-brand bg-amber-brand/10 px-4 py-3 text-sm font-bold text-amber-brand">
      ✦ Tu insight del día llega pronto
    </div>
  );
}

function Card({
  to,
  title,
  big,
  children,
}: {
  to: string;
  title: string;
  big?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Link
      to={to}
      className={`flex flex-col gap-1 rounded-lg border border-ink-700 bg-ink-800 p-4 ${
        big ? "min-h-[88px]" : "min-h-[64px]"
      }`}
    >
      <span className="text-sm font-bold text-sand-100">{title}</span>
      {children}
    </Link>
  );
}

function StreakCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/disciplina" title="🔥 Racha" big>
      {s.streak.total === 0 ? (
        <span className="text-sm text-sand-400">Sin hábitos aún</span>
      ) : (
        <>
          <span className="text-2xl font-extrabold text-streak">
            {s.streak.best_current} días
          </span>
          <span className="text-xs text-sand-400">
            {s.streak.done_today}/{s.streak.total} hábitos hoy
          </span>
        </>
      )}
    </Card>
  );
}

function FinanceCard({ s }: { s: Snapshot }) {
  const color =
    s.finance.status === "verde"
      ? "text-money"
      : s.finance.status === "rojo"
        ? "text-streak"
        : "text-sand-400";
  return (
    <Card to="/finanzas" title="Superávit del ciclo" big>
      <span className={`text-2xl font-extrabold ${color}`}>{formatMXN(s.finance.net)}</span>
      <span className="text-xs text-sand-400">
        {s.finance.cycle} · {s.finance.status}
      </span>
    </Card>
  );
}

function Bar({ value }: { value: number }) {
  // value 1-10 → ancho proporcional.
  return (
    <div className="h-2 w-full rounded bg-ink-700">
      <div className="h-2 rounded bg-amber-brand" style={{ width: `${value * 10}%` }} />
    </div>
  );
}

function MoodCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/check-in" title="Ánimo / Energía">
      {s.checkin == null ? (
        <span className="text-xs text-sand-400">Sin check-in hoy</span>
      ) : (
        <div className="flex flex-col gap-1">
          <Bar value={s.checkin.mood} />
          <Bar value={s.checkin.energy} />
        </div>
      )}
    </Card>
  );
}

function CheckinCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/check-in" title="Check-in de hoy">
      {s.checkin?.present ? (
        <span className="text-xs text-money">
          Hecho ✓ · disciplina {s.checkin.discipline}
        </span>
      ) : (
        <span className="text-xs text-sand-400">Pendiente</span>
      )}
    </Card>
  );
}

function TrainingCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/entrenamiento" title="Entreno de hoy">
      <span className="text-xs text-sand-400">
        {s.training.trained_today ? `${s.training.type} ✓` : "Sin entreno hoy"}
      </span>
    </Card>
  );
}

function GoalsCard({ s }: { s: Snapshot }) {
  return (
    <Card to="/metas" title="Metas activas">
      <span className="text-xs text-sand-400">
        {s.goals.active} activas · {s.goals.avg_progress}% prom.
      </span>
      {s.goals.overdue > 0 && (
        <span className="text-xs text-streak">{s.goals.overdue} vencida(s)</span>
      )}
    </Card>
  );
}
```

> **Nota:** la tarjeta de Racha usa el título "🔥 Racha"; el test la busca por `name: /Racha/` (accessible name del link incluye el texto del título). Verificá que el rol `link` con ese nombre resuelva; si el emoji interfiere, el regex `/Racha/` igual matchea el substring.

- [ ] **Step 4: Correr para verificar que pasa**

Run: `cd web && npx vitest run src/routes/index.test.tsx`
Expected: PASS (6 tests).

- [ ] **Step 5: Suite completa + build**

Run: `cd web && npx vitest run && npx vite build`
Expected: todos los tests verdes; build OK.

- [ ] **Step 6: Commit**

```bash
cd web && git add src/routes/index.tsx src/routes/index.test.tsx src/routeTree.gen.ts
git commit -m "feat(dashboard): home como centro de mando (Layout B) con banda IA, racha, superávit y tarjetas"
```

---

## Task 8: Smoke E2E con Docker

**Files:**
- Create (temporal): `/tmp/smoke_dashboard.sh`

Levantar el stack, registrar usuario, sembrar datos vía endpoints existentes (check-in, hábito+check, transacción, workout, meta), pedir `GET /dashboard` y verificar el snapshot + aislamiento.

> **Importante (zsh):** correr el script con `bash /tmp/smoke_dashboard.sh` explícitamente. No pegarlo inline en la terminal (zsh rompe con "bad math expression"). Antes de los comandos docker, exportar el PATH en la misma línea.

- [ ] **Step 1: Reconstruir el stack (incluye binario API con el nuevo endpoint)**

Run (con `dangerouslyDisableSandbox: true`):
```bash
export PATH="$PATH:$HOME/.docker/bin:/Applications/Docker.app/Contents/Resources/bin" && cd /Users/gustavo/Desktop/focus-365 && docker compose up -d --build
```
Expected: api (8088), db (5544), web (5174) up.

- [ ] **Step 2: Escribir `/tmp/smoke_dashboard.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail
API=http://localhost:8088
TODAY=2026-06-11
EMAIL="dash_$(date +%s)@b.com"

reg() {
  curl -s -X POST "$API/api/v1/auth/register" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$1\",\"password\":\"p4ssword\",\"name\":\"Dash\"}"
}

# Registrar usuario A y extraer access token.
TOKA=$(reg "$EMAIL" | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')
AUTH=(-H "Authorization: Bearer $TOKA" -H 'Content-Type: application/json')

# Sembrar las 5 dimensiones.
curl -s -X POST "$API/api/v1/check-ins" "${AUTH[@]}" \
  -d "{\"date\":\"$TODAY\",\"mood\":8,\"energy\":6,\"discipline\":9}" >/dev/null

HID=$(curl -s -X POST "$API/api/v1/habits?today=$TODAY" "${AUTH[@]}" \
  -d '{"name":"Leer"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
curl -s -X POST "$API/api/v1/habits/$HID/checks?today=$TODAY" "${AUTH[@]}" \
  -d "{\"date\":\"$TODAY\",\"done\":true}" >/dev/null

curl -s -X POST "$API/api/v1/finances/transactions" "${AUTH[@]}" \
  -d "{\"type\":\"income\",\"amount\":320000,\"occurred_on\":\"$TODAY\",\"category\":\"Sueldo\"}" >/dev/null

curl -s -X POST "$API/api/v1/training/workouts" "${AUTH[@]}" \
  -d "{\"date\":\"$TODAY\",\"type\":\"Fuerza\"}" >/dev/null

curl -s -X POST "$API/api/v1/goals?today=$TODAY" "${AUTH[@]}" \
  -d '{"title":"Correr 10k","dimension":"entrenamiento"}' >/dev/null

# Pedir el dashboard.
SNAP=$(curl -s "$API/api/v1/dashboard?today=$TODAY" "${AUTH[@]}")
echo "$SNAP" | python3 - "$SNAP" <<'PY'
import sys, json
snap = json.loads(sys.argv[1])
assert snap["dimensions_active"] == 5, snap
assert snap["streak"]["total"] == 1 and snap["streak"]["done_today"] == 1, snap
assert snap["checkin"]["mood"] == 8, snap
assert snap["training"]["trained_today"] is True and snap["training"]["type"] == "Fuerza", snap
assert snap["goals"]["active"] == 1, snap
print("snapshot OK")
PY

# Aislamiento: usuario B no ve nada de A.
EMAILB="dashb_$(date +%s)@b.com"
TOKB=$(reg "$EMAILB" | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')
SNAPB=$(curl -s "$API/api/v1/dashboard?today=$TODAY" -H "Authorization: Bearer $TOKB")
echo "$SNAPB" | python3 - "$SNAPB" <<'PY'
import sys, json
snap = json.loads(sys.argv[1])
assert snap["dimensions_active"] == 0, snap
assert snap["checkin"] is None, snap
assert snap["goals"]["active"] == 0, snap
print("isolation OK")
PY

# Sin token → 401.
CODE=$(curl -s -o /dev/null -w '%{http_code}' "$API/api/v1/dashboard?today=$TODAY")
[ "$CODE" = "401" ] && echo "auth OK" || { echo "auth FAIL: $CODE"; exit 1; }

echo "SMOKE OK"
```

> **Nota:** confirmá las rutas reales de los endpoints de seeding contra `server.go` (p. ej. `/api/v1/check-ins`, `/api/v1/habits/{id}/checks`, `/api/v1/training/workouts`, `/api/v1/finances/transactions`, `/api/v1/goals`). Ajustá paths/campos si difieren; la lógica de aserción no cambia. El access token puede llamarse `access_token` o `token` en la respuesta de register/login — verificá y ajustá el extractor `python3`.

- [ ] **Step 3: Correr el smoke**

Run (con `dangerouslyDisableSandbox: true`):
```bash
bash /tmp/smoke_dashboard.sh
```
Expected: `snapshot OK`, `isolation OK`, `auth OK`, `SMOKE OK`.

- [ ] **Step 4: Verificación final completa**

Run:
```bash
cd /Users/gustavo/Desktop/focus-365/api && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go vet ./... && TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" GOTOOLCHAIN=local go test -p 1 ./...
```
y
```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run
```
Expected: backend todos los paquetes ok; frontend todos los tests verdes.

- [ ] **Step 5: Commit final (si quedó algún ajuste de smoke en archivos versionados)**

El smoke vive en `/tmp` (no versionado). Si durante el smoke ajustaste código (paths, campos), agregá esos archivos y commiteá:

```bash
cd /Users/gustavo/Desktop/focus-365 && git add -A
git commit -m "test(dashboard): smoke e2e del endpoint agregado con docker"
```

---

## Criterios de aceptación (verificar al final)

- [ ] `GET /api/v1/dashboard` agrega las 5 dimensiones, scopeado por usuario, con `dimensions_active` correcto (5 poblado / 0 vacío).
- [ ] `checkin` serializa `null` cuando no hay check-in hoy.
- [ ] TopBar persistente en páginas autenticadas; oculta sin usuario.
- [ ] Dashboard Layout B: banda IA placeholder + saludo + 2 tarjetas grandes + 4 chicas, cada una con link a su módulo.
- [ ] Estados de carga ("Cargando tu día…"), error (con reintento) y vacíos por tarjeta correctos.
- [ ] `make check` verde + frontend Vitest verde + smoke `SMOKE OK`.
