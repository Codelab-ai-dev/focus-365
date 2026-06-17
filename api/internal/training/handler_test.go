package training_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/focus365/api/internal/training"
	"github.com/go-chi/chi/v5"
)

type fakeCompleter struct {
	out        string
	err        error
	lastSystem string
	lastUser   string
}

func (f *fakeCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	f.lastSystem, f.lastUser = system, user
	return f.out, f.err
}

type env struct {
	h    http.Handler
	auth *auth.Service
}

func newEnv(t *testing.T) *env {
	return newEnvWith(t, &fakeCompleter{out: "rutina sugerida"}, true)
}

func newEnvWith(t *testing.T, c *fakeCompleter, hasKey bool) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	tm := auth.NewTokenManager("secret")
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/training", training.Routes(training.NewService(q, pool, c, hasKey)))
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
