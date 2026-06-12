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

func TestBuildChatToolsCuatro(t *testing.T) {
	tools := buildChatTools()
	if len(tools) != 4 {
		t.Fatalf("tools = %d, want 4", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
		if len(tl.Parameters) == 0 || tl.Description == "" {
			t.Errorf("tool %s incompleta", tl.Name)
		}
	}
	for _, want := range []string{"registrar_checkin", "registrar_movimiento", "marcar_habito", "actualizar_meta"} {
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

func newTestExecutor(c *fakeCheckinSvc, fin *fakeFinanceSvc, h *fakeHabitsSvc, g *fakeGoalsSvc) *actionExecutor {
	return &actionExecutor{checkin: c, finance: fin, habits: h, goals: g}
}

func TestExecutorCheckin(t *testing.T) {
	c := &fakeCheckinSvc{}
	ex := newTestExecutor(c, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{})
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
	ex := newTestExecutor(&fakeCheckinSvc{}, fin, &fakeHabitsSvc{}, &fakeGoalsSvc{})
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
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{notFound: true}, &fakeGoalsSvc{})
	err := ex.execute(context.Background(), uuid.New(), "habito",
		[]byte(`{"habit_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11"}`), time.Now())
	if !errors.Is(err, ErrActionInvalid) {
		t.Errorf("err = %v, want ErrActionInvalid", err)
	}
}

func TestExecutorMeta(t *testing.T) {
	g := &fakeGoalsSvc{}
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, g)
	if err := ex.execute(context.Background(), uuid.New(), "meta",
		[]byte(`{"goal_id":"3b39c1f1-58a6-4012-9b69-0a3f4f6f3a11","progress":60}`), time.Now()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if g.progress == nil || *g.progress != 60 {
		t.Errorf("progress = %v", g.progress)
	}
}

func TestExecutorPayloadInvalido(t *testing.T) {
	ex := newTestExecutor(&fakeCheckinSvc{}, &fakeFinanceSvc{}, &fakeHabitsSvc{}, &fakeGoalsSvc{})
	err := ex.execute(context.Background(), uuid.New(), "checkin", []byte(`{"mood":99}`), time.Now())
	if !errors.Is(err, ErrActionInvalid) {
		t.Errorf("err = %v, want ErrActionInvalid", err)
	}
}
