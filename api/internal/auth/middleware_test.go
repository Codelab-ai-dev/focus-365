package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/focus365/api/internal/auth"
	"github.com/google/uuid"
)

func TestRequireAuthAllowsValidToken(t *testing.T) {
	tm := auth.NewTokenManager("secret")
	id := uuid.New()
	tok, _ := tm.IssueAccess(id)

	var gotID uuid.UUID
	h := auth.RequireAuth(tm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, _ = auth.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if gotID != id {
		t.Errorf("user id no inyectado")
	}
}

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	tm := auth.NewTokenManager("secret")
	h := auth.RequireAuth(tm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", rec.Code)
	}
}
