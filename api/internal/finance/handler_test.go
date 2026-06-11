package finance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/finance"
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
		r.Mount("/finances", finance.Routes(finance.NewService(q)))
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

func TestCreateListSummary(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "a@b.com")

	rec := do(t, e.h, http.MethodPost, "/finances/transactions", tok, map[string]any{
		"type": "income", "amount": 500000, "occurred_on": "2026-06-10", "category": "sueldo",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var tx map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tx)
	if tx["cycle"] != "2026-06" {
		t.Errorf("cycle = %v, want 2026-06", tx["cycle"])
	}

	_ = do(t, e.h, http.MethodPost, "/finances/transactions", tok, map[string]any{
		"type": "expense", "amount": 200000, "occurred_on": "2026-06-12",
	})

	// List del ciclo actual (today en junio).
	recL := do(t, e.h, http.MethodGet, "/finances/transactions?today=2026-06-15", tok, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}

	// Summary del ciclo actual.
	recS := do(t, e.h, http.MethodGet, "/finances/summary?today=2026-06-15", tok, nil)
	var sum map[string]any
	_ = json.Unmarshal(recS.Body.Bytes(), &sum)
	if sum["net"].(float64) != 300000 {
		t.Errorf("net = %v, want 300000", sum["net"])
	}
	if sum["status"] != "pendiente" {
		t.Errorf("status = %v, want pendiente", sum["status"])
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "del@b.com")
	rec := do(t, e.h, http.MethodPost, "/finances/transactions", tok, map[string]any{
		"type": "expense", "amount": 1000, "occurred_on": "2026-06-10",
	})
	var tx map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tx)
	id := tx["id"].(string)

	recD := do(t, e.h, http.MethodDelete, "/finances/transactions/"+id, tok, nil)
	if recD.Code != http.StatusNoContent {
		t.Fatalf("DELETE code = %d, want 204", recD.Code)
	}
	recD2 := do(t, e.h, http.MethodDelete, "/finances/transactions/"+id, tok, nil)
	if recD2.Code != http.StatusNotFound {
		t.Errorf("segundo DELETE code = %d, want 404", recD2.Code)
	}
}

func TestValidationTipo(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "v@b.com")
	rec := do(t, e.h, http.MethodPost, "/finances/transactions", tok, map[string]any{
		"type": "bogus", "amount": 1000, "occurred_on": "2026-06-10",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "El tipo no es válido" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/finances/transactions?today=2026-06-15", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	e := newEnv(t)
	tokA := e.token(t, "userA@b.com")
	tokB := e.token(t, "userB@b.com")

	recA := do(t, e.h, http.MethodPost, "/finances/transactions", tokA, map[string]any{
		"type": "income", "amount": 9999, "occurred_on": "2026-06-10",
	})
	var txA map[string]any
	_ = json.Unmarshal(recA.Body.Bytes(), &txA)
	idA := txA["id"].(string)

	// B no ve las transacciones de A.
	recL := do(t, e.h, http.MethodGet, "/finances/transactions?today=2026-06-15", tokB, nil)
	var list []map[string]any
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("user B ve %d transacciones de A; debería ver 0", len(list))
	}

	// B no puede borrar la transacción de A.
	recD := do(t, e.h, http.MethodDelete, "/finances/transactions/"+idA, tokB, nil)
	if recD.Code != http.StatusNotFound {
		t.Errorf("B borró la tx de A: code = %d, want 404", recD.Code)
	}
}
