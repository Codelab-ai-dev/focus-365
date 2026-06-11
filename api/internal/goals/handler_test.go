package goals_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
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
		r.Mount("/goals", goals.Routes(goals.NewService(q)))
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

func createGoal(t *testing.T, e *env, tok string, body map[string]any) map[string]any {
	t.Helper()
	rec := do(t, e.h, http.MethodPost, "/goals?today=2026-06-11", tok, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST goal code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var g map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &g)
	return g
}

func TestCreateAndList(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "a@b.com")
	createGoal(t, e, tok, map[string]any{"title": "Correr 10k", "dimension": "entrenamiento"})

	rec := do(t, e.h, http.MethodGet, "/goals?status=active&today=2026-06-11", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET goals code = %d", rec.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("lista = %d, want 1", len(list))
	}
	if list[0]["title"] != "Correr 10k" {
		t.Errorf("title = %v, want Correr 10k", list[0]["title"])
	}
	if list[0]["progress"].(float64) != 0 {
		t.Errorf("progress = %v, want 0", list[0]["progress"])
	}
	if list[0]["status"] != "active" {
		t.Errorf("status = %v, want active", list[0]["status"])
	}
	if list[0]["overdue"] != false {
		t.Errorf("overdue = %v, want false", list[0]["overdue"])
	}
}

func TestPatchProgress(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "p@b.com")
	g := createGoal(t, e, tok, map[string]any{"title": "Leer", "dimension": "mente"})
	id := g["id"].(string)

	rec := do(t, e.h, http.MethodPatch, "/goals/"+id+"?today=2026-06-11", tok, map[string]any{"progress": 40})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var after map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &after)
	if after["progress"].(float64) != 40 {
		t.Errorf("progress = %v, want 40", after["progress"])
	}
	if after["status"] != "active" {
		t.Errorf("status = %v, want active (independiente del progreso)", after["status"])
	}
}

func TestProgress100DoesNotChangeStatus(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "p100@b.com")
	g := createGoal(t, e, tok, map[string]any{"title": "Meta", "dimension": "general"})
	id := g["id"].(string)

	rec := do(t, e.h, http.MethodPatch, "/goals/"+id+"?today=2026-06-11", tok, map[string]any{"progress": 100})
	var after map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &after)
	if after["status"] != "active" {
		t.Errorf("status tras 100%% = %v, want active", after["status"])
	}
}

func TestPatchStatusTransitions(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "st@b.com")
	g := createGoal(t, e, tok, map[string]any{"title": "Proyecto", "dimension": "general"})
	id := g["id"].(string)

	do(t, e.h, http.MethodPatch, "/goals/"+id+"?today=2026-06-11", tok, map[string]any{"status": "done"})
	if n := listLen(t, e, tok, "active"); n != 0 {
		t.Errorf("activas = %d, want 0", n)
	}
	if n := listLen(t, e, tok, "done"); n != 1 {
		t.Errorf("completadas = %d, want 1", n)
	}
	do(t, e.h, http.MethodPatch, "/goals/"+id+"?today=2026-06-11", tok, map[string]any{"status": "active"})
	if n := listLen(t, e, tok, "active"); n != 1 {
		t.Errorf("activas tras reactivar = %d, want 1", n)
	}
}

func listLen(t *testing.T, e *env, tok, status string) int {
	t.Helper()
	rec := do(t, e.h, http.MethodGet, "/goals?status="+status+"&today=2026-06-11", tok, nil)
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	return len(list)
}

func TestOverdue(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "ov@b.com")
	g := createGoal(t, e, tok, map[string]any{
		"title": "Entrega", "dimension": "general", "deadline": "2026-06-01",
	})
	id := g["id"].(string)

	rec := do(t, e.h, http.MethodGet, "/goals?status=active&today=2026-06-11", tok, nil)
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["overdue"] != true {
		t.Fatalf("overdue = %v, want true", list)
	}
	do(t, e.h, http.MethodPatch, "/goals/"+id+"?today=2026-06-11", tok, map[string]any{"status": "done"})
	recD := do(t, e.h, http.MethodGet, "/goals?status=done&today=2026-06-11", tok, nil)
	var done []map[string]any
	_ = json.Unmarshal(recD.Body.Bytes(), &done)
	if done[0]["overdue"] != false {
		t.Errorf("overdue tras completar = %v, want false", done[0]["overdue"])
	}
}

func TestDeadlineClear(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "dl@b.com")
	g := createGoal(t, e, tok, map[string]any{
		"title": "Con fecha", "dimension": "general", "deadline": "2026-12-01",
	})
	id := g["id"].(string)

	recKeep := do(t, e.h, http.MethodPatch, "/goals/"+id+"?today=2026-06-11", tok, map[string]any{"progress": 10})
	var keep map[string]any
	_ = json.Unmarshal(recKeep.Body.Bytes(), &keep)
	if keep["deadline"] == nil {
		t.Errorf("deadline se borró sin pedirlo")
	}
	recClr := do(t, e.h, http.MethodPatch, "/goals/"+id+"?today=2026-06-11", tok, map[string]any{"deadline": nil})
	var clr map[string]any
	_ = json.Unmarshal(recClr.Body.Bytes(), &clr)
	if clr["deadline"] != nil {
		t.Errorf("deadline = %v, want null", clr["deadline"])
	}
}

func TestValidation(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "v@b.com")

	cases := []map[string]any{
		{"title": "X", "dimension": "inexistente"},
		{"dimension": "general"},
	}
	for _, body := range cases {
		if rec := do(t, e.h, http.MethodPost, "/goals?today=2026-06-11", tok, body); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %v code = %d, want 400", body, rec.Code)
		}
	}
	g := createGoal(t, e, tok, map[string]any{"title": "ok", "dimension": "general"})
	id := g["id"].(string)
	if rec := do(t, e.h, http.MethodPatch, "/goals/"+id, tok, map[string]any{"progress": 150}); rec.Code != http.StatusBadRequest {
		t.Errorf("progress 150 code = %d, want 400", rec.Code)
	}
	if rec := do(t, e.h, http.MethodPatch, "/goals/"+id, tok, map[string]any{"status": "raro"}); rec.Code != http.StatusBadRequest {
		t.Errorf("status raro code = %d, want 400", rec.Code)
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "d@b.com")
	g := createGoal(t, e, tok, map[string]any{"title": "Borrar", "dimension": "general"})
	id := g["id"].(string)

	if rec := do(t, e.h, http.MethodDelete, "/goals/"+id, tok, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE code = %d, want 204", rec.Code)
	}
	if rec := do(t, e.h, http.MethodDelete, "/goals/"+id, tok, nil); rec.Code != http.StatusNotFound {
		t.Errorf("segundo DELETE code = %d, want 404", rec.Code)
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/goals", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	tokA := e.token(t, "uA@b.com")
	tokB := e.token(t, "uB@b.com")
	g := createGoal(t, e, tokA, map[string]any{"title": "Privada", "dimension": "general"})
	idA := g["id"].(string)

	if n := listLen(t, e, tokB, "active"); n != 0 {
		t.Errorf("B ve %d metas de A; want 0", n)
	}
	if rec := do(t, e.h, http.MethodPatch, "/goals/"+idA, tokB, map[string]any{"progress": 5}); rec.Code != http.StatusNotFound {
		t.Errorf("B parcheó meta de A: code = %d, want 404", rec.Code)
	}
	if rec := do(t, e.h, http.MethodDelete, "/goals/"+idA, tokB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("B borró meta de A: code = %d, want 404", rec.Code)
	}
}
