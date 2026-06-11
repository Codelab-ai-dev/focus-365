package training_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/focus365/api/internal/training"
	"github.com/go-chi/chi/v5"
)

type env struct {
	h    http.Handler
	auth *auth.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	tm := auth.NewTokenManager("secret")
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/training", training.Routes(training.NewService(q, pool)))
	})
	return &env{h: r, auth: auth.NewService(q, tm)}
}

func (e *env) token(t *testing.T, email string) string {
	t.Helper()
	user, err := e.auth.Register(context.Background(), email, "p4ssword", "User")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	access, _, err := e.auth.IssueTokens(user.ID)
	if err != nil {
		t.Fatalf("IssueTokens: %v", err)
	}
	return access
}

func do(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		raw, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateWorkoutAndList(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "a@b.com")

	rec := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-11", "type": "Fuerza", "note": "buen pump",
		"sets": []map[string]any{
			{"exercise": "Sentadilla", "reps": 8, "weight_grams": 80000},
			{"exercise": "Sentadilla", "reps": 6, "weight_grams": 80000},
			{"exercise": "Press banca", "reps": 10, "weight_grams": 60000},
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST workout code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var w map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &w)
	sets := w["sets"].([]any)
	if len(sets) != 3 {
		t.Fatalf("sets = %d, want 3", len(sets))
	}
	first := sets[0].(map[string]any)
	if first["exercise"] != "Sentadilla" {
		t.Errorf("exercise[0] = %v, want Sentadilla", first["exercise"])
	}

	// Crear ejercicios on-the-fly: el catálogo ahora tiene 2.
	recE := do(t, e.h, http.MethodGet, "/training/exercises", tok, nil)
	var cat []map[string]any
	_ = json.Unmarshal(recE.Body.Bytes(), &cat)
	if len(cat) != 2 {
		t.Errorf("catálogo = %d, want 2", len(cat))
	}

	// Historial por rango.
	recL := do(t, e.h, http.MethodGet, "/training/workouts?from=2026-06-01&to=2026-06-30", tok, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("historial = %d, want 1", len(list))
	}
}

func TestExerciseIdempotente(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "ex@b.com")
	_ = do(t, e.h, http.MethodPost, "/training/exercises", tok, map[string]any{"name": "Peso muerto"})
	_ = do(t, e.h, http.MethodPost, "/training/exercises", tok, map[string]any{"name": "peso muerto"})
	recE := do(t, e.h, http.MethodGet, "/training/exercises", tok, nil)
	var cat []map[string]any
	_ = json.Unmarshal(recE.Body.Bytes(), &cat)
	if len(cat) != 1 {
		t.Errorf("catálogo = %d, want 1 (no duplica por capitalización)", len(cat))
	}
}

func TestGetAndDeleteWorkout(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "d@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-11",
		"sets": []map[string]any{{"exercise": "Dominadas", "reps": 10}},
	})
	var w map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &w)
	id := w["id"].(string)

	recG := do(t, e.h, http.MethodGet, "/training/workouts/"+id, tok, nil)
	if recG.Code != http.StatusOK {
		t.Fatalf("GET code = %d, want 200", recG.Code)
	}

	recD := do(t, e.h, http.MethodDelete, "/training/workouts/"+id, tok, nil)
	if recD.Code != http.StatusNoContent {
		t.Fatalf("DELETE code = %d, want 204", recD.Code)
	}
	recG2 := do(t, e.h, http.MethodGet, "/training/workouts/"+id, tok, nil)
	if recG2.Code != http.StatusNotFound {
		t.Errorf("GET tras borrar code = %d, want 404", recG2.Code)
	}
}

func TestValidationWorkout(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "v@b.com")
	// sets vacío → 400.
	rec := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-11", "sets": []map[string]any{},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("sets vacío code = %d, want 400", rec.Code)
	}
	// set sin exercise → 400.
	rec2 := do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-11", "sets": []map[string]any{{"reps": 8}},
	})
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("set sin ejercicio code = %d, want 400", rec2.Code)
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/training/exercises", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	tokA := e.token(t, "userA@b.com")
	tokB := e.token(t, "userB@b.com")

	recA := do(t, e.h, http.MethodPost, "/training/workouts", tokA, map[string]any{
		"date": "2026-06-11", "sets": []map[string]any{{"exercise": "Curl", "reps": 12}},
	})
	var wA map[string]any
	_ = json.Unmarshal(recA.Body.Bytes(), &wA)
	idA := wA["id"].(string)

	// B no ve el historial de A.
	recL := do(t, e.h, http.MethodGet, "/training/workouts", tokB, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("B ve %d sesiones de A; debería ver 0", len(list))
	}
	// B no puede ver ni borrar la sesión de A.
	if rec := do(t, e.h, http.MethodGet, "/training/workouts/"+idA, tokB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("B vio la sesión de A: code = %d, want 404", rec.Code)
	}
	if rec := do(t, e.h, http.MethodDelete, "/training/workouts/"+idA, tokB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("B borró la sesión de A: code = %d, want 404", rec.Code)
	}
}
