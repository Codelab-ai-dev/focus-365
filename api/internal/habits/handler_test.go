package habits_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/habits"
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
		r.Mount("/habits", habits.Routes(habits.NewService(q)))
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

func TestCreateAndList(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "a@b.com")

	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{
		"name": "Leer", "target_days": 21,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST habit code = %d, body = %s", rec.Code, rec.Body.String())
	}

	recL := do(t, e.h, http.MethodGet, "/habits?today=2026-06-14", tok, nil)
	if recL.Code != http.StatusOK {
		t.Fatalf("GET habits code = %d", recL.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("lista = %d, want 1", len(list))
	}
	if list[0]["name"] != "Leer" {
		t.Errorf("name = %v, want Leer", list[0]["name"])
	}
	if list[0]["current_streak"].(float64) != 0 {
		t.Errorf("current_streak = %v, want 0", list[0]["current_streak"])
	}
}

func TestCheckTodayAndYesterday(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "c@b.com")
	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{"name": "Flexiones"})
	var h map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &h)
	id := h["id"].(string)

	// Marcar hoy → racha 1.
	recT := do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{
		"day": "2026-06-14", "done": true,
	})
	if recT.Code != http.StatusOK {
		t.Fatalf("check hoy code = %d, body = %s", recT.Code, recT.Body.String())
	}
	var afterToday map[string]any
	_ = json.Unmarshal(recT.Body.Bytes(), &afterToday)
	if afterToday["current_streak"].(float64) != 1 {
		t.Errorf("racha tras hoy = %v, want 1", afterToday["current_streak"])
	}
	if afterToday["done_today"] != true {
		t.Errorf("done_today = %v, want true", afterToday["done_today"])
	}

	// Marcar ayer → racha 2.
	recY := do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{
		"day": "2026-06-13", "done": true,
	})
	var afterYest map[string]any
	_ = json.Unmarshal(recY.Body.Bytes(), &afterYest)
	if afterYest["current_streak"].(float64) != 2 {
		t.Errorf("racha tras ayer = %v, want 2", afterYest["current_streak"])
	}
}

func TestCheckRejectsBeforeYesterday(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "r@b.com")
	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{"name": "Meditar"})
	var h map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &h)
	id := h["id"].(string)

	recB := do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{
		"day": "2026-06-12", "done": true,
	})
	if recB.Code != http.StatusBadRequest {
		t.Errorf("marcar anteayer code = %d, want 400", recB.Code)
	}
}

func TestUncheckLowersStreak(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "u@b.com")
	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{"name": "Correr"})
	var h map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &h)
	id := h["id"].(string)

	_ = do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{"day": "2026-06-14", "done": true})
	recU := do(t, e.h, http.MethodPost, "/habits/"+id+"/check?today=2026-06-14", tok, map[string]any{"day": "2026-06-14", "done": false})
	var after map[string]any
	_ = json.Unmarshal(recU.Body.Bytes(), &after)
	if after["current_streak"].(float64) != 0 {
		t.Errorf("racha tras desmarcar = %v, want 0", after["current_streak"])
	}
}

func TestArchiveMovesOutOfActive(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "ar@b.com")
	rec := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tok, map[string]any{"name": "Diario"})
	var h map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &h)
	id := h["id"].(string)

	if recA := do(t, e.h, http.MethodPost, "/habits/"+id+"/archive?today=2026-06-14", tok, nil); recA.Code != http.StatusOK {
		t.Fatalf("archive code = %d", recA.Code)
	}
	recAct := do(t, e.h, http.MethodGet, "/habits?today=2026-06-14", tok, nil)
	var active []map[string]any
	_ = json.Unmarshal(recAct.Body.Bytes(), &active)
	if len(active) != 0 {
		t.Errorf("activos = %d, want 0", len(active))
	}
	recArch := do(t, e.h, http.MethodGet, "/habits?today=2026-06-14&archived=true", tok, nil)
	var arch []map[string]any
	_ = json.Unmarshal(recArch.Body.Bytes(), &arch)
	if len(arch) != 1 {
		t.Errorf("archivados = %d, want 1", len(arch))
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/habits", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	tokA := e.token(t, "uA@b.com")
	tokB := e.token(t, "uB@b.com")

	recA := do(t, e.h, http.MethodPost, "/habits?today=2026-06-14", tokA, map[string]any{"name": "Privado"})
	var hA map[string]any
	_ = json.Unmarshal(recA.Body.Bytes(), &hA)
	idA := hA["id"].(string)

	// B no ve hábitos de A.
	recL := do(t, e.h, http.MethodGet, "/habits?today=2026-06-14", tokB, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("B ve %d hábitos de A; debería ver 0", len(list))
	}
	// B no puede marcar ni borrar el de A.
	if rec := do(t, e.h, http.MethodPost, "/habits/"+idA+"/check?today=2026-06-14", tokB, map[string]any{"day": "2026-06-14", "done": true}); rec.Code != http.StatusNotFound {
		t.Errorf("B marcó el hábito de A: code = %d, want 404", rec.Code)
	}
	if rec := do(t, e.h, http.MethodDelete, "/habits/"+idA, tokB, nil); rec.Code != http.StatusNotFound {
		t.Errorf("B borró el hábito de A: code = %d, want 404", rec.Code)
	}
}
