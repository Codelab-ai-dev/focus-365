package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/training"
	"github.com/google/uuid"
)

func TestParseActionPayloadValid(t *testing.T) {
	cases := []struct {
		kind, args string
	}{
		{"checkin", `{"mood":8,"energy":6,"discipline":9,"note":"bien"}`},
		{"checkin", `{"mood":1,"energy":10,"discipline":5}`},
		{"movimiento", `{"type":"expense","amount_centavos":2500000,"category":"comida"}`},
		{"movimiento", `{"type":"income","amount_centavos":1,"category":"sueldo","remark":"junio"}`},
		{"habito", `{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`},
		{"meta", `{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":60}`},
	}
	for _, c := range cases {
		if _, err := parseActionPayload(c.kind, c.args); err != nil {
			t.Errorf("%s %s: %v", c.kind, c.args, err)
		}
	}
}

func TestParseActionPayloadInvalid(t *testing.T) {
	cases := []struct {
		name, kind, args string
	}{
		{"kind desconocido", "borrar_todo", `{}`},
		{"json roto", "checkin", `{mood:`},
		{"mood fuera de rango", "checkin", `{"mood":11,"energy":6,"discipline":9}`},
		{"mood faltante", "checkin", `{"energy":6,"discipline":9}`},
		{"type inválido", "movimiento", `{"type":"transfer","amount_centavos":100,"category":"x"}`},
		{"monto cero", "movimiento", `{"type":"expense","amount_centavos":0,"category":"x"}`},
		{"categoría vacía", "movimiento", `{"type":"expense","amount_centavos":100,"category":"  "}`},
		{"uuid inválido", "habito", `{"habit_id":"no-es-uuid"}`},
		{"progress fuera de rango", "meta", `{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":101}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseActionPayload(c.kind, c.args); err == nil {
				t.Errorf("esperaba error para %s %s", c.kind, c.args)
			}
		})
	}
}

func TestActionSummaryPorKind(t *testing.T) {
	got := actionSummary("checkin", []byte(`{"mood":8,"energy":6,"discipline":9}`))
	for _, want := range []string{"check-in", "8", "6", "9"} {
		if !strings.Contains(strings.ToLower(got), want) {
			t.Errorf("summary %q no contiene %q", got, want)
		}
	}
	if s := actionSummary("habito", []byte(`{"habit_id":"x"}`)); s == "" {
		t.Error("summary de hábito vacío")
	}
}

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

// --- Fakes para el ejecutor ---

type fakeCheckinSvc struct{ in *checkin.Input }

func (f *fakeCheckinSvc) Upsert(ctx context.Context, userID uuid.UUID, in checkin.Input) (*checkin.CheckIn, error) {
	f.in = &in
	return &checkin.CheckIn{}, nil
}

type fakeFinanceSvc struct{ in *finance.Input }

func (f *fakeFinanceSvc) Create(ctx context.Context, userID uuid.UUID, in finance.Input) (*finance.Transaction, error) {
	f.in = &in
	return &finance.Transaction{}, nil
}

type fakeHabitsSvc struct {
	habitID  uuid.UUID
	done     bool
	notFound bool
}

func (f *fakeHabitsSvc) SetCheck(ctx context.Context, userID, habitID uuid.UUID, day time.Time, done bool, today time.Time) (*habits.Habit, error) {
	if f.notFound {
		return nil, nil
	}
	f.habitID, f.done = habitID, done
	return &habits.Habit{}, nil
}

type fakeGoalsSvc struct {
	goalID   uuid.UUID
	progress *int32
	notFound bool
}

func (f *fakeGoalsSvc) Patch(ctx context.Context, userID, id uuid.UUID, p goals.GoalPatch, today time.Time) (*goals.Goal, error) {
	if f.notFound {
		return nil, nil
	}
	f.goalID, f.progress = id, p.Progress
	return &goals.Goal{}, nil
}

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

func newTestExecutor(c *fakeCheckinSvc, fin *fakeFinanceSvc, h *fakeHabitsSvc, g *fakeGoalsSvc,
	hc *fakeHabitCreate, gc *fakeGoalCreate, wc *fakeWorkoutCreate) *actionExecutor {
	return &actionExecutor{checkin: c, finance: fin, habits: h, goals: g,
		habitCreate: hc, goalCreate: gc, workouts: wc}
}

func TestExecutorCheckin(t *testing.T) {
	c := &fakeCheckinSvc{}
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{})
	today := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	err := ex.execute(context.Background(), uuid.New(), "checkin",
		[]byte(`{"mood":8,"energy":6,"discipline":9,"note":"ok"}`), today)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if c.in == nil || c.in.Mood != 8 || c.in.Note != "ok" || !c.in.Date.Equal(today) {
		t.Errorf("input = %+v", c.in)
	}
}

func TestExecutorMovimiento(t *testing.T) {
	fin := &fakeFinanceSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{})
	today := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	if err := ex.execute(context.Background(), uuid.New(), "movimiento",
		[]byte(`{"type":"expense","amount_centavos":2500000,"category":"comida"}`), today); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fin.in == nil || fin.in.Type != "expense" || fin.in.Amount != 2500000 || !fin.in.OccurredOn.Equal(today) {
		t.Errorf("input = %+v", fin.in)
	}
}

func TestExecutorHabitoNotFound(t *testing.T) {
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{notFound: true}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{})
	err := ex.execute(context.Background(), uuid.New(), "habito",
		[]byte(`{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`), time.Now())
	if !errors.Is(err, ErrActionInvalid) {
		t.Errorf("err = %v, want ErrActionInvalid", err)
	}
}

func TestExecutorMeta(t *testing.T) {
	g := &fakeGoalsSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, g, &fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{})
	if err := ex.execute(context.Background(), uuid.New(), "meta",
		[]byte(`{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":60}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if g.progress == nil || *g.progress != 60 {
		t.Errorf("progress = %v", g.progress)
	}
}

func TestExecutorPayloadInvalido(t *testing.T) {
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, &fakeWorkoutCreate{})
	err := ex.execute(context.Background(), uuid.New(), "checkin", []byte(`{"mood":99}`), time.Now())
	if !errors.Is(err, ErrActionInvalid) {
		t.Errorf("err = %v, want ErrActionInvalid", err)
	}
}

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

func TestExecutorEntrenamientoRedondeaGramos(t *testing.T) {
	wc := &fakeWorkoutCreate{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeHabitCreate{}, &fakeGoalCreate{}, wc)
	if err := ex.execute(context.Background(), uuid.New(), "entrenamiento",
		[]byte(`{"type":"fuerza","sets":[{"exercise":"x","weight_kg":60.5499}]}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if g := wc.in.Sets[0].WeightGrams; g == nil || *g != 60550 {
		t.Errorf("gramos = %v, want 60550 (redondeo, no truncado)", g)
	}
}
