package dashboard_test

import (
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

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("uuid %s: %v", s, err)
	}
	return id
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

	h, err := e.habits.Create(ctx, uid, habits.HabitInput{Name: "Leer"}, td)
	if err != nil {
		t.Fatalf("crear hábito: %v", err)
	}
	if _, err := e.habits.SetCheck(ctx, uid, mustUUID(t, h.ID), td, true, td); err != nil {
		t.Fatalf("marcar hábito: %v", err)
	}
	if _, err := e.finance.Create(ctx, uid, finance.Input{
		Type: "income", Amount: 320000, OccurredOn: td, Category: "Sueldo",
	}); err != nil {
		t.Fatalf("crear transacción: %v", err)
	}
	if _, err := e.checkins.Upsert(ctx, uid, checkin.Input{
		Date: td, Mood: 8, Energy: 6, Win: "gran victoria",
	}); err != nil {
		t.Fatalf("check-in: %v", err)
	}
	if _, err := e.training.CreateWorkout(ctx, uid, training.WorkoutInput{
		Date: td, Type: "Fuerza",
	}); err != nil {
		t.Fatalf("workout: %v", err)
	}
	if _, err := e.goals.Create(ctx, uid, goals.GoalInput{
		Title: "Correr 10k", Dimension: "fisica",
	}, td); err != nil {
		t.Fatalf("meta: %v", err)
	}

	rec, body := get(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	// Las 5 dimensiones tienen movimiento hoy: hábito marcado, ingreso en el
	// ciclo (net != 0), check-in, entreno y meta activa.
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
		Title: "Entrega", Dimension: "espiritual", Deadline: &dl,
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
		Title: "Privada", Dimension: "espiritual",
	}, td); err != nil {
		t.Fatalf("meta A: %v", err)
	}
	_, body := get(t, e.h, tokB)
	gl := body["goals"].(map[string]any)
	if gl["active"].(float64) != 0 {
		t.Errorf("B ve %v metas activas de A, want 0", gl["active"])
	}
}
