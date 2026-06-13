package ai

import (
	"context"
	"encoding/json"
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

// --- Fakes para el ejecutor (uno por servicio, implementan la interfaz compuesta) ---

type fakeCheckinSvc struct {
	in      *checkin.Input  // último Upsert
	today   *checkin.CheckIn // lo que devuelve Today()
	todayN  int             // # llamadas a Today
	deleted bool            // Delete() fue llamado
	delDate time.Time       // fecha del Delete
	delHit  bool            // Delete devuelve (true/false, ...)
	err     error           // error real de DB para Upsert/Delete
}

func (f *fakeCheckinSvc) Upsert(ctx context.Context, userID uuid.UUID, in checkin.Input) (*checkin.CheckIn, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.in = &in
	return &checkin.CheckIn{}, nil
}

func (f *fakeCheckinSvc) Today(ctx context.Context, userID uuid.UUID, date time.Time) (*checkin.CheckIn, error) {
	f.todayN++
	return f.today, nil
}

func (f *fakeCheckinSvc) Delete(ctx context.Context, userID uuid.UUID, date time.Time) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.deleted, f.delDate = true, date
	return f.delHit, nil
}

type fakeFinanceSvc struct {
	in       *finance.Input
	txID     string // ID que devuelve Create
	deletedID uuid.UUID
	deleted  bool
	delHit   bool // Delete devuelve este bool
	err      error
}

func (f *fakeFinanceSvc) Create(ctx context.Context, userID uuid.UUID, in finance.Input) (*finance.Transaction, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.in = &in
	return &finance.Transaction{ID: f.txID}, nil
}

func (f *fakeFinanceSvc) Delete(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.deleted, f.deletedID = true, id
	return f.delHit, nil
}

type fakeHabitsSvc struct {
	createIn  *habits.HabitInput
	createdID string // ID que devuelve Create
	habitID   uuid.UUID
	done      bool // último valor de SetCheck.done
	setN      int  // # llamadas a SetCheck
	setDate   time.Time
	notFound  bool // SetCheck devuelve (nil, nil)
	deletedID uuid.UUID
	deleted   bool
	delHit    bool
	err       error
}

func (f *fakeHabitsSvc) SetCheck(ctx context.Context, userID, habitID uuid.UUID, day time.Time, done bool, today time.Time) (*habits.Habit, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.notFound {
		return nil, nil
	}
	f.habitID, f.done, f.setDate = habitID, done, day
	f.setN++
	return &habits.Habit{}, nil
}

func (f *fakeHabitsSvc) Create(ctx context.Context, userID uuid.UUID, in habits.HabitInput, today time.Time) (*habits.Habit, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.createIn = &in
	return &habits.Habit{ID: f.createdID}, nil
}

func (f *fakeHabitsSvc) Delete(ctx context.Context, userID, habitID uuid.UUID) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.deleted, f.deletedID = true, habitID
	return f.delHit, nil
}

type fakeGoalsSvc struct {
	createIn  *goals.GoalInput
	createdID string // ID que devuelve Create
	goalID    uuid.UUID
	progress  *int32 // último Patch.Progress
	notFound  bool   // Patch devuelve (nil, nil)
	list      []goals.Goal // lo que devuelve List()
	listN     int
	deletedID uuid.UUID
	deleted   bool
	delHit    bool
	err       error
}

func (f *fakeGoalsSvc) Patch(ctx context.Context, userID, id uuid.UUID, p goals.GoalPatch, today time.Time) (*goals.Goal, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.notFound {
		return nil, nil
	}
	f.goalID, f.progress = id, p.Progress
	return &goals.Goal{}, nil
}

func (f *fakeGoalsSvc) Create(ctx context.Context, userID uuid.UUID, in goals.GoalInput, today time.Time) (*goals.Goal, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.createIn = &in
	return &goals.Goal{ID: f.createdID}, nil
}

func (f *fakeGoalsSvc) Delete(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.deleted, f.deletedID = true, id
	return f.delHit, nil
}

func (f *fakeGoalsSvc) List(ctx context.Context, userID uuid.UUID, status string, today time.Time) ([]goals.Goal, error) {
	f.listN++
	return f.list, nil
}

type fakeTrainingSvc struct {
	createIn  *training.WorkoutInput
	createdID string // ID que devuelve CreateWorkout
	deletedID uuid.UUID
	deleted   bool
	delHit    bool
	err       error
}

func (f *fakeTrainingSvc) CreateWorkout(ctx context.Context, userID uuid.UUID, in training.WorkoutInput) (*training.Workout, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.createIn = &in
	return &training.Workout{ID: f.createdID}, nil
}

func (f *fakeTrainingSvc) DeleteWorkout(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.deleted, f.deletedID = true, id
	return f.delHit, nil
}

func newTestExecutor(c *fakeCheckinSvc, fin *fakeFinanceSvc, h *fakeHabitsSvc, g *fakeGoalsSvc, tr *fakeTrainingSvc) *actionExecutor {
	return &actionExecutor{checkin: c, finance: fin, habits: h, goals: g, training: tr}
}

func TestExecutorCheckin(t *testing.T) {
	c := &fakeCheckinSvc{}
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	today := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	_, err := ex.execute(context.Background(), uuid.New(), "checkin",
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
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	today := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	if _, err := ex.execute(context.Background(), uuid.New(), "movimiento",
		[]byte(`{"type":"expense","amount_centavos":2500000,"category":"comida"}`), today); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fin.in == nil || fin.in.Type != "expense" || fin.in.Amount != 2500000 || !fin.in.OccurredOn.Equal(today) {
		t.Errorf("input = %+v", fin.in)
	}
}

func TestExecutorHabitoNotFound(t *testing.T) {
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{notFound: true}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	_, err := ex.execute(context.Background(), uuid.New(), "habito",
		[]byte(`{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`), time.Now())
	if !errors.Is(err, ErrActionInvalid) {
		t.Errorf("err = %v, want ErrActionInvalid", err)
	}
}

func TestExecutorMeta(t *testing.T) {
	g := &fakeGoalsSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, g, &fakeTrainingSvc{})
	if _, err := ex.execute(context.Background(), uuid.New(), "meta",
		[]byte(`{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":60}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if g.progress == nil || *g.progress != 60 {
		t.Errorf("progress = %v", g.progress)
	}
}

func TestExecutorPayloadInvalido(t *testing.T) {
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	_, err := ex.execute(context.Background(), uuid.New(), "checkin", []byte(`{"mood":99}`), time.Now())
	if !errors.Is(err, ErrActionInvalid) {
		t.Errorf("err = %v, want ErrActionInvalid", err)
	}
}

func TestExecutorHabitoNuevo(t *testing.T) {
	hc := &fakeHabitsSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, hc, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	if _, err := ex.execute(context.Background(), uuid.New(), "habito_nuevo",
		[]byte(`{"name":"Leer 30 min","target_days":21}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if hc.createIn == nil || hc.createIn.Name != "Leer 30 min" || hc.createIn.TargetDays == nil || *hc.createIn.TargetDays != 21 {
		t.Errorf("input = %+v", hc.createIn)
	}
}

func TestExecutorMetaNuevaConDeadline(t *testing.T) {
	gc := &fakeGoalsSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, gc, &fakeTrainingSvc{})
	if _, err := ex.execute(context.Background(), uuid.New(), "meta_nueva",
		[]byte(`{"title":"Ahorrar 50k","dimension":"finanzas","deadline":"2026-12-01"}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gc.createIn == nil || gc.createIn.Title != "Ahorrar 50k" || gc.createIn.Dimension != "finanzas" {
		t.Fatalf("input = %+v", gc.createIn)
	}
	if gc.createIn.Deadline == nil || gc.createIn.Deadline.Format("2006-01-02") != "2026-12-01" {
		t.Errorf("deadline = %v", gc.createIn.Deadline)
	}
}

func TestExecutorEntrenamientoConvierteKgAGramos(t *testing.T) {
	wc := &fakeTrainingSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, wc)
	today := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	if _, err := ex.execute(context.Background(), uuid.New(), "entrenamiento",
		[]byte(`{"type":"fuerza","sets":[{"exercise":"press banca","reps":8,"weight_kg":60},{"exercise":"plancha"}]}`), today); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if wc.createIn == nil || wc.createIn.Type != "fuerza" || !wc.createIn.Date.Equal(today) || len(wc.createIn.Sets) != 2 {
		t.Fatalf("input = %+v", wc.createIn)
	}
	s0 := wc.createIn.Sets[0]
	if s0.Exercise != "press banca" || s0.Reps == nil || *s0.Reps != 8 || s0.WeightGrams == nil || *s0.WeightGrams != 60000 {
		t.Errorf("set 0 = %+v", s0)
	}
	s1 := wc.createIn.Sets[1]
	if s1.Reps != nil || s1.WeightGrams != nil {
		t.Errorf("set 1 debe ir sin reps/peso: %+v", s1)
	}
}

func TestExecutorEntrenamientoRedondeaGramos(t *testing.T) {
	wc := &fakeTrainingSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, wc)
	if _, err := ex.execute(context.Background(), uuid.New(), "entrenamiento",
		[]byte(`{"type":"fuerza","sets":[{"exercise":"x","weight_kg":60.5499}]}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if g := wc.createIn.Sets[0].WeightGrams; g == nil || *g != 60550 {
		t.Errorf("gramos = %v, want 60550 (redondeo, no truncado)", g)
	}
}

// --- execute devuelve result por kind ---

func TestExecutorCheckinResultGuardaPrevYFecha(t *testing.T) {
	today := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	// Caso A: sin check-in previo → prev null.
	c := &fakeCheckinSvc{}
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	res, err := ex.execute(context.Background(), uuid.New(), "checkin",
		[]byte(`{"mood":8,"energy":7,"discipline":9}`), today)
	if err != nil {
		t.Fatalf("execute A: %v", err)
	}
	var rA checkinResult
	if err := json.Unmarshal(res, &rA); err != nil {
		t.Fatalf("unmarshal A: %v", err)
	}
	if rA.Prev != nil {
		t.Errorf("prev A = %+v, want nil", rA.Prev)
	}
	if rA.Date != "2026-06-12" {
		t.Errorf("date A = %q", rA.Date)
	}
	if c.todayN != 1 {
		t.Errorf("Today llamado %d veces, want 1", c.todayN)
	}

	// Caso B: con check-in previo → prev poblado.
	c2 := &fakeCheckinSvc{today: &checkin.CheckIn{Mood: 5, Energy: 4, Discipline: 6, Note: "meh"}}
	ex2 := newTestExecutor(c2, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	res2, err := ex2.execute(context.Background(), uuid.New(), "checkin",
		[]byte(`{"mood":8,"energy":7,"discipline":9}`), today)
	if err != nil {
		t.Fatalf("execute B: %v", err)
	}
	var rB checkinResult
	if err := json.Unmarshal(res2, &rB); err != nil {
		t.Fatalf("unmarshal B: %v", err)
	}
	if rB.Prev == nil || rB.Prev.Mood != 5 || rB.Prev.Energy != 4 || rB.Prev.Discipline != 6 || rB.Prev.Note != "meh" {
		t.Errorf("prev B = %+v", rB.Prev)
	}
}

func TestExecutorMovimientoResultGuardaTxID(t *testing.T) {
	fin := &fakeFinanceSvc{txID: "tx-1"}
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	res, err := ex.execute(context.Background(), uuid.New(), "movimiento",
		[]byte(`{"type":"expense","amount_centavos":100,"category":"x"}`), time.Now())
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var r idResult
	if err := json.Unmarshal(res, &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if r.ID != "tx-1" {
		t.Errorf("tx_id = %q, want tx-1", r.ID)
	}
}

// --- undo por kind ---

const undoUUID = "3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"

func TestUndoCheckinRestauraPrevio(t *testing.T) {
	c := &fakeCheckinSvc{}
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	result := []byte(`{"prev":{"mood":5,"energy":4,"discipline":6,"note":"meh"},"date":"2026-06-10"}`)
	if err := ex.undo(context.Background(), uuid.New(), "checkin", nil, result); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if c.in == nil || c.in.Mood != 5 || c.in.Energy != 4 || c.in.Discipline != 6 || c.in.Note != "meh" {
		t.Errorf("upsert prev = %+v", c.in)
	}
	if c.in.Date.Format("2006-01-02") != "2026-06-10" {
		t.Errorf("fecha = %s, want fecha del result", c.in.Date)
	}
	if c.deleted {
		t.Error("no debía borrar habiendo previo")
	}
}

func TestUndoCheckinSinPrevioBorra(t *testing.T) {
	c := &fakeCheckinSvc{}
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	result := []byte(`{"prev":null,"date":"2026-06-10"}`)
	if err := ex.undo(context.Background(), uuid.New(), "checkin", nil, result); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if !c.deleted || c.delDate.Format("2006-01-02") != "2026-06-10" {
		t.Errorf("delete = %v, fecha %s", c.deleted, c.delDate)
	}
	if c.in != nil {
		t.Error("no debía hacer upsert sin previo")
	}
}

func TestUndoMovimientoBorraYToleraInexistente(t *testing.T) {
	fin := &fakeFinanceSvc{delHit: false} // Delete devuelve false (ya no existe)
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	result := []byte(`{"id":"` + undoUUID + `"}`)
	if err := ex.undo(context.Background(), uuid.New(), "movimiento", nil, result); err != nil {
		t.Fatalf("undo: %v (false debe tolerarse)", err)
	}
	if !fin.deleted || fin.deletedID.String() != undoUUID {
		t.Errorf("delete = %v id %s", fin.deleted, fin.deletedID)
	}
}

func TestUndoHabitoDesmarca(t *testing.T) {
	h := &fakeHabitsSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, h, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	result := []byte(`{"habit_id":"` + undoUUID + `","date":"2026-06-10"}`)
	if err := ex.undo(context.Background(), uuid.New(), "habito", nil, result); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if h.done {
		t.Error("debía desmarcar (done=false)")
	}
	if h.setN != 1 || h.habitID.String() != undoUUID || h.setDate.Format("2006-01-02") != "2026-06-10" {
		t.Errorf("SetCheck n=%d id=%s date=%s", h.setN, h.habitID, h.setDate)
	}
}

func TestUndoMetaRestauraProgreso(t *testing.T) {
	g := &fakeGoalsSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, g, &fakeTrainingSvc{})
	result := []byte(`{"prev_progress":42,"goal_id":"` + undoUUID + `"}`)
	if err := ex.undo(context.Background(), uuid.New(), "meta", nil, result); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if g.progress == nil || *g.progress != 42 {
		t.Errorf("progress restaurado = %v, want 42", g.progress)
	}

	// Meta inexistente (Patch nil,nil) → undone igual.
	g2 := &fakeGoalsSvc{notFound: true}
	ex2 := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, g2, &fakeTrainingSvc{})
	if err := ex2.undo(context.Background(), uuid.New(), "meta", nil, result); err != nil {
		t.Errorf("undo meta inexistente = %v, want nil", err)
	}
}

func TestUndoCreacionesBorran(t *testing.T) {
	result := []byte(`{"id":"` + undoUUID + `"}`)
	t.Run("habito_nuevo", func(t *testing.T) {
		h := &fakeHabitsSvc{delHit: true}
		ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, h, &fakeGoalsSvc{}, &fakeTrainingSvc{})
		if err := ex.undo(context.Background(), uuid.New(), "habito_nuevo", nil, result); err != nil {
			t.Fatalf("undo: %v", err)
		}
		if !h.deleted || h.deletedID.String() != undoUUID {
			t.Errorf("delete = %v id %s", h.deleted, h.deletedID)
		}
	})
	t.Run("meta_nueva", func(t *testing.T) {
		g := &fakeGoalsSvc{delHit: true}
		ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, g, &fakeTrainingSvc{})
		if err := ex.undo(context.Background(), uuid.New(), "meta_nueva", nil, result); err != nil {
			t.Fatalf("undo: %v", err)
		}
		if !g.deleted || g.deletedID.String() != undoUUID {
			t.Errorf("delete = %v id %s", g.deleted, g.deletedID)
		}
	})
	t.Run("entrenamiento", func(t *testing.T) {
		tr := &fakeTrainingSvc{delHit: false} // inexistente tolerado
		ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, tr)
		if err := ex.undo(context.Background(), uuid.New(), "entrenamiento", nil, result); err != nil {
			t.Fatalf("undo: %v", err)
		}
		if !tr.deleted || tr.deletedID.String() != undoUUID {
			t.Errorf("delete = %v id %s", tr.deleted, tr.deletedID)
		}
	})
}

func TestUndoResultCorrupto(t *testing.T) {
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	if err := ex.undo(context.Background(), uuid.New(), "movimiento", nil, []byte(`{"id":"no-uuid"}`)); !errors.Is(err, ErrActionInvalid) {
		t.Errorf("err = %v, want ErrActionInvalid", err)
	}
	if err := ex.undo(context.Background(), uuid.New(), "checkin", nil, []byte(`{bad`)); !errors.Is(err, ErrActionInvalid) {
		t.Errorf("checkin corrupto err = %v, want ErrActionInvalid", err)
	}
}

func TestUndoErrorDeDBSePropaga(t *testing.T) {
	dbErr := errors.New("db caída")
	fin := &fakeFinanceSvc{err: dbErr}
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	result := []byte(`{"id":"` + undoUUID + `"}`)
	if err := ex.undo(context.Background(), uuid.New(), "movimiento", nil, result); !errors.Is(err, dbErr) {
		t.Errorf("err = %v, want db caída", err)
	}
}

func TestParseMovimientoOccurredOn(t *testing.T) {
	// Válido con fecha.
	if _, err := parseActionPayload("movimiento", `{"type":"expense","amount_centavos":25000,"category":"comida","occurred_on":"2026-05-10"}`); err != nil {
		t.Errorf("con occurred_on válido: %v", err)
	}
	// Sin fecha (retrocompatible).
	if _, err := parseActionPayload("movimiento", `{"type":"income","amount_centavos":1,"category":"x"}`); err != nil {
		t.Errorf("sin occurred_on: %v", err)
	}
	// Fecha malformada.
	if _, err := parseActionPayload("movimiento", `{"type":"expense","amount_centavos":1,"category":"x","occurred_on":"mayo"}`); err == nil {
		t.Error("occurred_on malformado debe fallar")
	}
}

func TestExecutorMovimientoUsaFechaDelPayload(t *testing.T) {
	fin := &fakeFinanceSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	today := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	if _, err := ex.execute(context.Background(), uuid.New(), "movimiento",
		[]byte(`{"type":"expense","amount_centavos":25000,"category":"comida","occurred_on":"2026-05-10"}`), today); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	if !fin.in.OccurredOn.Equal(want) {
		t.Errorf("OccurredOn = %v, want %v (la fecha del payload)", fin.in.OccurredOn, want)
	}
}

func TestExecutorMovimientoSinFechaUsaToday(t *testing.T) {
	fin := &fakeFinanceSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{}, &fakeTrainingSvc{})
	today := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	if _, err := ex.execute(context.Background(), uuid.New(), "movimiento",
		[]byte(`{"type":"expense","amount_centavos":25000,"category":"comida"}`), today); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !fin.in.OccurredOn.Equal(today) {
		t.Errorf("OccurredOn = %v, want today %v", fin.in.OccurredOn, today)
	}
}
