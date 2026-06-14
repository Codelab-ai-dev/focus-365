package checkin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/checkin"
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
		r.Mount("/checkins", checkin.Routes(checkin.NewService(q)))
	})
	return &env{h: r, auth: auth.NewService(q, tm)}
}

// token registra un usuario y devuelve su access token.
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

func TestUpsertCreatesAndUpdates(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "a@b.com")

	body := map[string]any{"date": "2026-06-10", "mood": 7, "energy": 6, "win": "buen día"}
	rec := do(t, e.h, http.MethodPost, "/checkins", tok, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var ci map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &ci)
	if ci["mood"].(float64) != 7 {
		t.Errorf("mood = %v, want 7", ci["mood"])
	}

	// Segundo POST mismo día → actualiza (mismo id).
	firstID := ci["id"]
	body["mood"] = 3
	rec2 := do(t, e.h, http.MethodPost, "/checkins", tok, body)
	var ci2 map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &ci2)
	if ci2["id"] != firstID {
		t.Errorf("el upsert creó una fila nueva: %v vs %v", ci2["id"], firstID)
	}
	if ci2["mood"].(float64) != 3 {
		t.Errorf("mood actualizado = %v, want 3", ci2["mood"])
	}
}

func TestTodayAndList(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "b@b.com")

	// Sin check-in → today devuelve null.
	rec := do(t, e.h, http.MethodGet, "/checkins/today?date=2026-06-10", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET today code = %d", rec.Code)
	}
	if got := bytes.TrimSpace(rec.Body.Bytes()); string(got) != "null" {
		t.Errorf("today vacío = %q, want null", got)
	}

	// Creamos uno y lo recuperamos.
	_ = do(t, e.h, http.MethodPost, "/checkins", tok, map[string]any{
		"date": "2026-06-10", "mood": 5, "energy": 5,
	})
	rec2 := do(t, e.h, http.MethodGet, "/checkins/today?date=2026-06-10", tok, nil)
	var ci map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &ci)
	if ci["date"] != "2026-06-10" {
		t.Errorf("today date = %v", ci["date"])
	}

	// List.
	rec3 := do(t, e.h, http.MethodGet, "/checkins", tok, nil)
	var list []map[string]any
	_ = json.Unmarshal(rec3.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Errorf("len(list) = %d, want 1", len(list))
	}
}

func TestValidationOutOfRange(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "c@b.com")
	rec := do(t, e.h, http.MethodPost, "/checkins", tok, map[string]any{
		"date": "2026-06-10", "mood": 11, "energy": 5,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "El ánimo debe ser como máximo 10" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/checkins", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	tokA := e.token(t, "userA@b.com")
	tokB := e.token(t, "userB@b.com")

	_ = do(t, e.h, http.MethodPost, "/checkins", tokA, map[string]any{
		"date": "2026-06-10", "mood": 9, "energy": 9, "win": "de A",
	})

	rec := do(t, e.h, http.MethodGet, "/checkins", tokB, nil)
	var list []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("user B ve %d check-ins de A; debería ver 0", len(list))
	}
}
