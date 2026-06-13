# Plan 14 — Acciones de la IA parte 2 (crear hábito/meta + entrenamiento) — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** El asistente puede proponer (y el usuario confirmar) crear hábitos, crear metas y registrar entrenamientos completos con series.

**Architecture:** Extensión pura del mecanismo R11: 3 kinds nuevos (`habito_nuevo`, `meta_nueva`, `entrenamiento`), migración que amplía el CHECK, 3 tools con validación, 3 casos del ejecutor sobre `habits.Create`/`goals.Create`/`training.CreateWorkout`, y los textos de tarjeta en el frontend. Sin cambios de flujo.

**Tech Stack:** Go (paquete `api/internal/ai`), React (`asistente.tsx`), Groq tool-use.

**Spec:** `docs/superpowers/specs/2026-06-12-plan-14-acciones-ia-2-design.md`

**Entorno:** Go desde `api/` con `GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable"`; frontend `cd web && npx vitest run && npm run build`. Commits en español con `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Rama: `plan-14-acciones-ia-2` desde `main`.

---

### Task 1: Migración 0010 (kinds nuevos en el CHECK)

**Files:**
- Create: `api/db/migrations/0010_ai_action_kinds.sql`
- Test: `api/internal/store/ai_messages_test.go` (ampliar)

- [ ] **Step 1: Test que falla.** Agregar a `api/internal/store/ai_messages_test.go` (el archivo ya tiene `ptr` y el patrón `CreateUser`):

```go
func TestAiMessageNewActionKinds(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "kinds-rt@b.com", PasswordHash: "h", Name: "Ada",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for _, kind := range []string{"habito_nuevo", "meta_nueva", "entrenamiento"} {
		k := kind
		if _, err := q.CreateMessageWithAction(ctx, store.CreateMessageWithActionParams{
			UserID: u.ID, Role: "assistant", Content: "x",
			ActionKind: &k, ActionPayload: []byte(`{}`), ActionStatus: ptr("proposed"),
		}); err != nil {
			t.Errorf("kind %s rechazado por el CHECK: %v", kind, err)
		}
	}
}
```

- [ ] **Step 2: Verificar que falla** (constraint violation: la 0009 no admite esos kinds):

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/store/ -run TestAiMessageNewActionKinds -v
```

- [ ] **Step 3: Migración** `api/db/migrations/0010_ai_action_kinds.sql`:

```sql
-- +goose Up
ALTER TABLE ai_messages
    DROP CONSTRAINT ai_messages_action_kind_valid,
    ADD CONSTRAINT ai_messages_action_kind_valid CHECK (
        action_kind IS NULL OR action_kind IN (
            'checkin','movimiento','habito','meta',
            'habito_nuevo','meta_nueva','entrenamiento'
        )
    );

-- +goose Down
ALTER TABLE ai_messages
    DROP CONSTRAINT ai_messages_action_kind_valid,
    ADD CONSTRAINT ai_messages_action_kind_valid CHECK (
        action_kind IS NULL OR action_kind IN ('checkin','movimiento','habito','meta')
    );
```

- [ ] **Step 4: Verificar que pasa** (mismo comando; `testutil.NewDB` aplica la 0010 sola) y correr el paquete `store` completo.

- [ ] **Step 5: Commit**

```bash
git add api/db/migrations/0010_ai_action_kinds.sql api/internal/store/ai_messages_test.go
git commit -m "feat(ai): migración 0010 — kinds habito_nuevo, meta_nueva y entrenamiento"
```

---

### Task 2: Payloads, validación, summaries y tools

**Files:**
- Modify: `api/internal/ai/actions.go`
- Test: `api/internal/ai/actions_test.go`

- [ ] **Step 1: Tests que fallan.** Agregar a `api/internal/ai/actions_test.go`:

```go
func TestParseActionPayloadNewKindsValid(t *testing.T) {
	cases := []struct{ kind, args string }{
		{"habito_nuevo", `{"name":"Leer 30 min","target_days":21}`},
		{"habito_nuevo", `{"name":"Meditar"}`},
		{"meta_nueva", `{"title":"Ahorrar 50k","dimension":"finanzas","deadline":"2026-12-01"}`},
		{"meta_nueva", `{"title":"Leer 12 libros","dimension":"general"}`},
		{"entrenamiento", `{"type":"fuerza","sets":[{"exercise":"press banca","reps":8,"weight_kg":60},{"exercise":"sentadilla","reps":5,"weight_kg":80.5}]}`},
		{"entrenamiento", `{"type":"cardio","note":"suave","sets":[{"exercise":"correr"}]}`},
	}
	for _, c := range cases {
		if _, err := parseActionPayload(c.kind, c.args); err != nil {
			t.Errorf("%s %s: %v", c.kind, c.args, err)
		}
	}
}

func TestParseActionPayloadNewKindsInvalid(t *testing.T) {
	cases := []struct{ name, kind, args string }{
		{"hábito sin nombre", "habito_nuevo", `{"name":"  "}`},
		{"target_days cero", "habito_nuevo", `{"name":"Leer","target_days":0}`},
		{"meta sin título", "meta_nueva", `{"title":"","dimension":"general"}`},
		{"dimension inválida", "meta_nueva", `{"title":"X","dimension":"dinero"}`},
		{"deadline malformado", "meta_nueva", `{"title":"X","dimension":"general","deadline":"diciembre"}`},
		{"entreno sin tipo", "entrenamiento", `{"type":" ","sets":[{"exercise":"x"}]}`},
		{"entreno sin series", "entrenamiento", `{"type":"fuerza","sets":[]}`},
		{"serie sin ejercicio", "entrenamiento", `{"type":"fuerza","sets":[{"exercise":"  "}]}`},
		{"reps cero", "entrenamiento", `{"type":"fuerza","sets":[{"exercise":"x","reps":0}]}`},
		{"peso negativo", "entrenamiento", `{"type":"fuerza","sets":[{"exercise":"x","weight_kg":-1}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseActionPayload(c.kind, c.args); err == nil {
				t.Errorf("esperaba error para %s %s", c.kind, c.args)
			}
		})
	}
}

func TestParseActionPayloadEntrenamientoTooManySets(t *testing.T) {
	sets := make([]string, 21)
	for i := range sets {
		sets[i] = `{"exercise":"x"}`
	}
	args := `{"type":"fuerza","sets":[` + strings.Join(sets, ",") + `]}`
	if _, err := parseActionPayload("entrenamiento", args); err == nil {
		t.Error("esperaba error con 21 series")
	}
}

func TestActionSummaryNewKinds(t *testing.T) {
	cases := []struct {
		kind, payload string
		wants         []string
	}{
		{"habito_nuevo", `{"name":"Leer 30 min","target_days":21}`, []string{"hábito", "Leer 30 min"}},
		{"meta_nueva", `{"title":"Ahorrar 50k","dimension":"finanzas","deadline":"2026-12-01"}`, []string{"meta", "Ahorrar 50k", "2026-12-01"}},
		{"entrenamiento", `{"type":"fuerza","sets":[{"exercise":"a"},{"exercise":"b"}]}`, []string{"entrenamiento", "fuerza", "2"}},
	}
	for _, c := range cases {
		got := actionSummary(c.kind, []byte(c.payload))
		for _, w := range c.wants {
			if !strings.Contains(strings.ToLower(got), strings.ToLower(w)) {
				t.Errorf("summary de %s %q no contiene %q", c.kind, got, w)
			}
		}
	}
}

func TestBuildChatToolsSiete(t *testing.T) {
	tools := buildChatTools()
	if len(tools) != 7 {
		t.Fatalf("tools = %d, want 7", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
	}
	for _, want := range []string{"crear_habito", "crear_meta", "registrar_entrenamiento"} {
		if !names[want] {
			t.Errorf("falta tool %s", want)
		}
	}
}
```

Además, **actualizar** `TestBuildChatToolsCuatro` → renombrarlo no: eliminarlo (lo reemplaza `TestBuildChatToolsSiete`; verificar que ningún otro test asuma 4 tools — `TestChatSendStreamToolCallPersistsProposal` en chat_test.go asserta `len(groq.lastTools) != 4`: cambiarlo a `!= 7`).

- [ ] **Step 2: Verificar que fallan.**

- [ ] **Step 3: Implementar en `actions.go`.**

1. Kinds y mapeo:

```go
const (
	actionCheckin       = "checkin"
	actionMovimiento    = "movimiento"
	actionHabito        = "habito"
	actionMeta          = "meta"
	actionHabitoNuevo   = "habito_nuevo"
	actionMetaNueva     = "meta_nueva"
	actionEntrenamiento = "entrenamiento"
)

var toolNameToKind = map[string]string{
	"registrar_checkin":       actionCheckin,
	"registrar_movimiento":    actionMovimiento,
	"marcar_habito":           actionHabito,
	"actualizar_meta":         actionMeta,
	"crear_habito":            actionHabitoNuevo,
	"crear_meta":              actionMetaNueva,
	"registrar_entrenamiento": actionEntrenamiento,
}
```

2. Payloads (debajo de los existentes):

```go
type habitoNuevoPayload struct {
	Name       string `json:"name"`
	TargetDays *int32 `json:"target_days,omitempty"`
}

type metaNuevaPayload struct {
	Title     string `json:"title"`
	Dimension string `json:"dimension"`
	Deadline  string `json:"deadline,omitempty"` // YYYY-MM-DD, "" = sin fecha
}

type setPayload struct {
	Exercise string   `json:"exercise"`
	Reps     *int32   `json:"reps,omitempty"`
	WeightKg *float64 `json:"weight_kg,omitempty"`
}

type entrenamientoPayload struct {
	Type string       `json:"type"`
	Note string       `json:"note,omitempty"`
	Sets []setPayload `json:"sets"`
}

// goalDimensions replica el oneof del handler HTTP de metas.
var goalDimensions = map[string]bool{
	"checkin": true, "finanzas": true, "entrenamiento": true, "mente": true, "general": true,
}

const maxWorkoutSets = 20
```

3. Casos nuevos en el switch de `parseActionPayload` (antes del `}` final del switch):

```go
case actionHabitoNuevo:
	var p habitoNuevoPayload
	if err := dec(&p); err != nil {
		return nil, err
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return nil, fmt.Errorf("falta name")
	}
	if p.TargetDays != nil && *p.TargetDays < 1 {
		return nil, fmt.Errorf("target_days debe ser positivo")
	}
	return json.Marshal(p)
case actionMetaNueva:
	var p metaNuevaPayload
	if err := dec(&p); err != nil {
		return nil, err
	}
	p.Title = strings.TrimSpace(p.Title)
	if p.Title == "" {
		return nil, fmt.Errorf("falta title")
	}
	if !goalDimensions[p.Dimension] {
		return nil, fmt.Errorf("dimension inválida: %s", p.Dimension)
	}
	if p.Deadline != "" {
		if _, err := time.Parse("2006-01-02", p.Deadline); err != nil {
			return nil, fmt.Errorf("deadline inválido (YYYY-MM-DD)")
		}
	}
	return json.Marshal(p)
case actionEntrenamiento:
	var p entrenamientoPayload
	if err := dec(&p); err != nil {
		return nil, err
	}
	p.Type = strings.TrimSpace(p.Type)
	if p.Type == "" {
		return nil, fmt.Errorf("falta type")
	}
	if len(p.Sets) == 0 || len(p.Sets) > maxWorkoutSets {
		return nil, fmt.Errorf("sets debe tener entre 1 y %d series", maxWorkoutSets)
	}
	for i, s := range p.Sets {
		if strings.TrimSpace(s.Exercise) == "" {
			return nil, fmt.Errorf("serie %d sin exercise", i+1)
		}
		if s.Reps != nil && *s.Reps < 1 {
			return nil, fmt.Errorf("serie %d: reps debe ser positivo", i+1)
		}
		if s.WeightKg != nil && *s.WeightKg <= 0 {
			return nil, fmt.Errorf("serie %d: weight_kg debe ser positivo", i+1)
		}
	}
	return json.Marshal(p)
```

4. Casos nuevos en `actionSummary` (antes del `}` final del switch):

```go
case actionHabitoNuevo:
	var p habitoNuevoPayload
	_ = json.Unmarshal(payload, &p)
	return fmt.Sprintf("Propongo crear el hábito %q.", p.Name)
case actionMetaNueva:
	var p metaNuevaPayload
	_ = json.Unmarshal(payload, &p)
	if p.Deadline != "" {
		return fmt.Sprintf("Propongo crear la meta %q (%s) para %s.", p.Title, p.Dimension, p.Deadline)
	}
	return fmt.Sprintf("Propongo crear la meta %q (%s).", p.Title, p.Dimension)
case actionEntrenamiento:
	var p entrenamientoPayload
	_ = json.Unmarshal(payload, &p)
	return fmt.Sprintf("Propongo registrar un entrenamiento de %s con %d series.", p.Type, len(p.Sets))
```

5. Tools nuevas al final del slice de `buildChatTools`:

```go
{
	Name:        "crear_habito",
	Description: "Crea un hábito nuevo. Úsala solo si el usuario pide explícitamente crear/empezar un hábito. target_days es el objetivo de racha en días (opcional).",
	Parameters: json.RawMessage(`{"type":"object","properties":{
		"name":{"type":"string","description":"nombre del hábito"},
		"target_days":{"type":"integer","minimum":1,"description":"objetivo de días (opcional)"}},
		"required":["name"]}`),
},
{
	Name:        "crear_meta",
	Description: "Crea una meta nueva. dimension: checkin, finanzas, entrenamiento, mente o general — infiérela del tema (ahorro→finanzas) y usa general si no es claro. deadline opcional en YYYY-MM-DD.",
	Parameters: json.RawMessage(`{"type":"object","properties":{
		"title":{"type":"string"},
		"dimension":{"type":"string","enum":["checkin","finanzas","entrenamiento","mente","general"]},
		"deadline":{"type":"string","description":"YYYY-MM-DD, opcional"}},
		"required":["title","dimension"]}`),
},
{
	Name:        "registrar_entrenamiento",
	Description: "Registra el entrenamiento de HOY con sus series. Úsala solo si el usuario pide registrar un entreno. El peso va en KILOGRAMOS (weight_kg). No inventes reps ni pesos que el usuario no dijo: las series pueden ir solo con el ejercicio.",
	Parameters: json.RawMessage(`{"type":"object","properties":{
		"type":{"type":"string","description":"tipo de sesión: fuerza, cardio, movilidad..."},
		"note":{"type":"string","description":"nota opcional"},
		"sets":{"type":"array","minItems":1,"maxItems":20,"items":{"type":"object","properties":{
			"exercise":{"type":"string"},
			"reps":{"type":"integer","minimum":1},
			"weight_kg":{"type":"number","exclusiveMinimum":0}},
			"required":["exercise"]}}},
		"required":["type","sets"]}`),
},
```

6. Ajustar el comentario de `buildChatTools` («define las functions que se ofrecen al modelo» — quitar el «4») y el de chat_test (`lastTools != 4` → `!= 7`, está en Task 2 Step 1).

- [ ] **Step 4: Verificar** (`go test ./internal/ai/ -count=1` completo) **+ commit**

```bash
git add api/internal/ai/actions.go api/internal/ai/actions_test.go api/internal/ai/chat_test.go
git commit -m "feat(ai): tools y payloads de crear hábito, crear meta y registrar entrenamiento"
```

---

### Task 3: Ejecutor con los 3 servicios nuevos

**Files:**
- Modify: `api/internal/ai/actions.go`, `api/internal/server/server.go`, `api/internal/ai/handler_test.go`
- Test: `api/internal/ai/actions_test.go`

- [ ] **Step 1: Tests que fallan.** En `actions_test.go`, fakes nuevos y tests (imports nuevos: `training`):

```go
type fakeHabitCreate struct{ in *habits.HabitInput }

func (f *fakeHabitCreate) Create(ctx context.Context, userID uuid.UUID, in habits.HabitInput, today time.Time) (*habits.Habit, error) {
	f.in = &in
	return &habits.Habit{}, nil
}

type fakeGoalCreate struct{ in *goals.GoalInput }

func (f *fakeGoalCreate) Create(ctx context.Context, userID uuid.UUID, in goals.GoalInput, today time.Time) (*goals.Goal, error) {
	f.in = &in
	return &goals.Goal{}, nil
}

type fakeWorkoutCreate struct{ in *training.WorkoutInput }

func (f *fakeWorkoutCreate) CreateWorkout(ctx context.Context, userID uuid.UUID, in training.WorkoutInput) (*training.Workout, error) {
	f.in = &in
	return &training.Workout{}, nil
}
```

Actualizar `newTestExecutor` para recibir los 7 fakes:

```go
func newTestExecutor(c *fakeCheckinSvc, fin *fakeFinanceSvc, h *fakeHabitsSvc, g *fakeGoalsSvc,
	hc *fakeHabitCreate, gc *fakeGoalCreate, wc *fakeWorkoutCreate) *actionExecutor {
	return &actionExecutor{checkin: c, finance: fin, habits: h, goals: g,
		habitCreate: hc, goalCreate: gc, workouts: wc}
}
```

(y actualizar TODAS las llamadas existentes a `newTestExecutor` en `actions_test.go` y `chat_test.go` agregando `&fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{}`).

Tests nuevos:

```go
func TestExecutorHabitoNuevo(t *testing.T) {
	hc := &fakeHabitCreate{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, hc, &fakeGoalCreate{}, &fakeWorkoutCreate{})
	if err := ex.execute(context.Background(), uuid.New(), "habito_nuevo",
		[]byte(`{"name":"Leer 30 min","target_days":21}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if hc.in == nil || hc.in.Name != "Leer 30 min" || hc.in.TargetDays == nil || *hc.in.TargetDays != 21 {
		t.Errorf("input = %+v", hc.in)
	}
}

func TestExecutorMetaNuevaConDeadline(t *testing.T) {
	gc := &fakeGoalCreate{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, gc, &fakeWorkoutCreate{})
	if err := ex.execute(context.Background(), uuid.New(), "meta_nueva",
		[]byte(`{"title":"Ahorrar 50k","dimension":"finanzas","deadline":"2026-12-01"}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gc.in == nil || gc.in.Title != "Ahorrar 50k" || gc.in.Dimension != "finanzas" {
		t.Fatalf("input = %+v", gc.in)
	}
	if gc.in.Deadline == nil || gc.in.Deadline.Format("2006-01-02") != "2026-12-01" {
		t.Errorf("deadline = %v", gc.in.Deadline)
	}
}

func TestExecutorEntrenamientoConvierteKgAGramos(t *testing.T) {
	wc := &fakeWorkoutCreate{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, wc)
	today := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	if err := ex.execute(context.Background(), uuid.New(), "entrenamiento",
		[]byte(`{"type":"fuerza","sets":[{"exercise":"press banca","reps":8,"weight_kg":60},{"exercise":"plancha"}]}`), today); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if wc.in == nil || wc.in.Type != "fuerza" || !wc.in.Date.Equal(today) || len(wc.in.Sets) != 2 {
		t.Fatalf("input = %+v", wc.in)
	}
	s0 := wc.in.Sets[0]
	if s0.Exercise != "press banca" || s0.Reps == nil || *s0.Reps != 8 || s0.WeightGrams == nil || *s0.WeightGrams != 60000 {
		t.Errorf("set 0 = %+v", s0)
	}
	s1 := wc.in.Sets[1]
	if s1.Reps != nil || s1.WeightGrams != nil {
		t.Errorf("set 1 debe ir sin reps/peso: %+v", s1)
	}
}
```

- [ ] **Step 2: Verificar que fallan** (compilación).

- [ ] **Step 3: Implementar en `actions.go`.**

1. Interfaces nuevas (debajo de `goalPatcher`; import nuevo `training`):

```go
type habitCreator interface {
	Create(ctx context.Context, userID uuid.UUID, in habits.HabitInput, today time.Time) (*habits.Habit, error)
}

type goalCreator interface {
	Create(ctx context.Context, userID uuid.UUID, in goals.GoalInput, today time.Time) (*goals.Goal, error)
}

type workoutCreator interface {
	CreateWorkout(ctx context.Context, userID uuid.UUID, in training.WorkoutInput) (*training.Workout, error)
}
```

2. Struct y constructor:

```go
type actionExecutor struct {
	checkin     checkinUpserter
	finance     txCreator
	habits      habitChecker
	goals       goalPatcher
	habitCreate habitCreator
	goalCreate  goalCreator
	workouts    workoutCreator
}

// NewActionExecutor arma el ejecutor con los servicios reales (wiring en server.go).
func NewActionExecutor(c checkinUpserter, f txCreator, h habitChecker, g goalPatcher,
	hc habitCreator, gc goalCreator, w workoutCreator) *actionExecutor {
	return &actionExecutor{checkin: c, finance: f, habits: h, goals: g,
		habitCreate: hc, goalCreate: gc, workouts: w}
}
```

3. Casos nuevos en el switch de `execute` (antes del `}` final):

```go
case actionHabitoNuevo:
	var p habitoNuevoPayload
	_ = json.Unmarshal(normalized, &p)
	_, err := e.habitCreate.Create(ctx, userID, habits.HabitInput{Name: p.Name, TargetDays: p.TargetDays}, today)
	return err
case actionMetaNueva:
	var p metaNuevaPayload
	_ = json.Unmarshal(normalized, &p)
	var deadline *time.Time
	if p.Deadline != "" {
		d, _ := time.Parse("2006-01-02", p.Deadline) // ya validado en parse
		deadline = &d
	}
	_, err := e.goalCreate.Create(ctx, userID, goals.GoalInput{Title: p.Title, Dimension: p.Dimension, Deadline: deadline}, today)
	return err
case actionEntrenamiento:
	var p entrenamientoPayload
	_ = json.Unmarshal(normalized, &p)
	sets := make([]training.SetInput, 0, len(p.Sets))
	for _, s := range p.Sets {
		set := training.SetInput{Exercise: s.Exercise, Reps: s.Reps}
		if s.WeightKg != nil {
			g := int32(*s.WeightKg * 1000)
			set.WeightGrams = &g
		}
		sets = append(sets, set)
	}
	_, err := e.workouts.CreateWorkout(ctx, userID, training.WorkoutInput{
		Date: today, Type: p.Type, Note: p.Note, Sets: sets,
	})
	return err
```

4. Wiring:
- `api/internal/server/server.go`: `actionExec := ai.NewActionExecutor(checkinSvc, financeSvc, habitsSvc, goalsSvc, habitsSvc, goalsSvc, trainingSvc)` (los mismos servicios implementan las interfaces nuevas; `trainingSvc` ya existe en el archivo).
- `api/internal/ai/handler_test.go` (`newEnv`): `actionExec := ai.NewActionExecutor(ci, fi, ha, go_, ha, go_, tr)` (`tr` ya existe).

- [ ] **Step 4: Verificar paquete + build completo + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go build ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test ./internal/ai/ -count=1
git add api/internal/ai api/internal/server/server.go
git commit -m "feat(ai): ejecutor de crear hábito, crear meta y registrar entrenamiento"
```

---

### Task 4: Prompt + test de integración end-to-end

**Files:**
- Modify: `api/internal/ai/chatprompt.go`
- Test: `api/internal/ai/chat_handler_test.go`

- [ ] **Step 1: Test que falla.** Agregar a `chat_handler_test.go` (helpers `postChatStream`, `getMessages`, `postAction`, `proposeViaChat` y `fakeCompleter.chatToolCall` ya existen):

```go
func TestActionCrearHabitoEndToEnd(t *testing.T) {
	comp := &fakeCompleter{chatToolCall: &ai.ToolCall{
		Name: "crear_habito", Arguments: `{"name":"Leer 30 min","target_days":21}`,
	}}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "habito-nuevo@b.com")
	id := proposeViaChat(t, e, tok)

	rec, body := postAction(t, e.h, tok, id, "confirm")
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm code = %d, body = %s", rec.Code, rec.Body.String())
	}
	msg, _ := body["message"].(map[string]any)
	action, _ := msg["action"].(map[string]any)
	if action["status"] != "done" || action["kind"] != "habito_nuevo" {
		t.Errorf("action = %v", action)
	}

	// El hábito existe de verdad en la DB.
	habs, err := e.q.ListHabits(context.Background(), uid)
	if err != nil {
		t.Fatalf("ListHabits: %v", err)
	}
	found := false
	for _, h := range habs {
		if h.Name == "Leer 30 min" {
			found = true
		}
	}
	if !found {
		t.Errorf("el hábito no se creó: %+v", habs)
	}
}
```

(Verificar la firma real generada: `grep -n "func (q \*Queries) ListHabits" api/internal/store/habits.sql.go` — si recibe params struct en vez de `uuid.UUID`, ajustar la llamada a lo generado.)

- [ ] **Step 2: Verificar que falla.** Con el código de las Tasks 2-3 ya mergeado en la rama, este test debería PASAR directamente — si es así, verificar el rojo solo del assert (cambiar temporalmente "Leer 30 min" por otro nombre para ver que el assert muerde, y revertir). Si falla por otra razón, investigar antes de seguir.

- [ ] **Step 3: Prompt.** En `chatprompt.go`, ampliar la línea de acciones existente agregando (mismo bloque, después de la oración de las herramientas):

```
También puedes proponer crear hábitos, crear metas (con su dimension y deadline si lo dio) y registrar el entrenamiento de hoy con sus series, siempre solo ante pedido explícito y con los datos que el usuario dio.
```

- [ ] **Step 4: Verificar todo el backend + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/api && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" go vet ./... && GOTOOLCHAIN=local PATH="$PATH:/usr/local/go/bin" TEST_DATABASE_URL="postgres://focus:changeme@localhost:5544/focus365?sslmode=disable" go test -p 1 ./... -count=1
git add api/internal/ai/chatprompt.go api/internal/ai/chat_handler_test.go
git commit -m "feat(ai): prompt y test end-to-end de las acciones nuevas"
```

---

### Task 5: Tarjetas del frontend

**Files:**
- Modify: `web/src/routes/asistente.tsx`
- Test: `web/src/routes/asistente.test.tsx`

- [ ] **Step 1: Tests que fallan.** Agregar a `asistente.test.tsx` (patrón `proposedMessages` existente):

```tsx
it("la tarjeta de entrenamiento muestra tipo y series", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            messages: [
              {
                id: "w1",
                role: "assistant",
                content: "Propongo registrar un entrenamiento.",
                action: {
                  kind: "entrenamiento",
                  payload: {
                    type: "fuerza",
                    sets: [
                      { exercise: "press banca", reps: 8, weight_kg: 60 },
                      { exercise: "plancha" },
                    ],
                  },
                  status: "proposed",
                },
                created_at: "2026-06-12T10:00:01Z",
              },
            ],
          }),
          { status: 200 }
        )
      )
    )
  );
  renderPage();
  expect(await screen.findByText("Entrenamiento")).toBeInTheDocument();
  expect(screen.getByText(/fuerza/)).toBeInTheDocument();
  expect(screen.getByText(/press banca 3?×?8.*@60kg|press banca 8.*@60kg|press banca ×8 @60kg/)).toBeInTheDocument();
});

it("la tarjeta de hábito nuevo y meta nueva muestran sus datos", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            messages: [
              {
                id: "h1", role: "assistant", content: "x",
                action: { kind: "habito_nuevo", payload: { name: "Leer 30 min", target_days: 21 }, status: "proposed" },
                created_at: "2026-06-12T10:00:01Z",
              },
              {
                id: "g1", role: "assistant", content: "y",
                action: { kind: "meta_nueva", payload: { title: "Ahorrar 50k", dimension: "finanzas", deadline: "2026-12-01" }, status: "proposed" },
                created_at: "2026-06-12T10:00:02Z",
              },
            ],
          }),
          { status: 200 }
        )
      )
    )
  );
  renderPage();
  expect(await screen.findByText("Nuevo hábito")).toBeInTheDocument();
  expect(screen.getByText(/Leer 30 min · objetivo 21 días/)).toBeInTheDocument();
  expect(screen.getByText("Nueva meta")).toBeInTheDocument();
  expect(screen.getByText(/Ahorrar 50k · finanzas · para 2026-12-01/)).toBeInTheDocument();
});
```

(Nota: el assert del entrenamiento usa regex flexible; al implementar, fijar el formato exacto `press banca ×8 @60kg` y simplificar el regex a ese literal.)

- [ ] **Step 2: Verificar que fallan.**

- [ ] **Step 3: Implementar en `asistente.tsx`.**

`ACTION_TITLES` gana:

```ts
habito_nuevo: "Nuevo hábito",
meta_nueva: "Nueva meta",
entrenamiento: "Entrenamiento",
```

`actionDetails` gana (antes del `default`):

```ts
case "habito_nuevo":
  return `${p.name}${p.target_days ? ` · objetivo ${p.target_days} días` : ""}`;
case "meta_nueva":
  return `${p.title} · ${p.dimension}${p.deadline ? ` · para ${p.deadline}` : ""}`;
case "entrenamiento": {
  const sets = (p.sets as Array<Record<string, unknown>>) ?? [];
  const detalle = sets
    .map((s) => `${s.exercise}${s.reps ? ` ×${s.reps}` : ""}${s.weight_kg ? ` @${s.weight_kg}kg` : ""}`)
    .join(" · ");
  return `${p.type} · ${detalle}`;
}
```

Ajustar el test del entrenamiento al formato exacto (`press banca ×8 @60kg`).

- [ ] **Step 4: Suite completa + build + commit**

```bash
cd /Users/gustavo/Desktop/focus-365/web && npx vitest run && npm run build
git add web/src/routes/asistente.tsx web/src/routes/asistente.test.tsx
git commit -m "feat(web): tarjetas de hábito nuevo, meta nueva y entrenamiento"
```

---

### Task 6: Cierre — review, merge, deploy y smoke de producción

- [ ] **Step 1:** Suites completas (backend `-p 1 ./...` + frontend + build) y smoke local `/tmp/smoke_actions.sh` (8/8) con docker reconstruido.
- [ ] **Step 2:** Review final holística (subagente), aplicar nits.
- [ ] **Step 3:** Merge `--no-ff` a `main`, borrar rama, **push** (dispara el auto-deploy de Coolify).
- [ ] **Step 4:** Smoke de **producción**: registrar usuario → pedir «crea un hábito de leer 30 minutos con objetivo de 21 días» → confirmar → verificar en `GET /api/v1/habits` que existe. (La migración 0010 se aplica sola al arrancar el api.)
- [ ] **Step 5:** Bitácora en `docs/superpowers/sesiones/` y push.

---

## Notas para el ejecutor

- Los tipos de dominio mandan: `habits.HabitInput{Name, TargetDays *int32}`, `goals.GoalInput{Title, Dimension string, Deadline *time.Time}`, `training.WorkoutInput{Date, Type, Note string, Sets []SetInput}`, `training.SetInput{Exercise string, Reps *int32, WeightGrams *int32}`. Si una firma difiere, ajustar a lo real.
- Las 4 acciones de la R11 son INVARIANTES: si un test previo falla, es regresión.
- `chat_test.go` asserta `len(groq.lastTools)`: pasa de 4 a 7 (Task 2).
