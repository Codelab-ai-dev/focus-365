package commitments_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/focus365/api/internal/auth"
	"github.com/focus365/api/internal/commitments"
	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type env struct {
	h    http.Handler
	svc  *commitments.Service
	auth *auth.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	tm := auth.NewTokenManager("secret")
	svc := commitments.NewService(q, pool)
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/commitments", commitments.Routes(svc))
	})
	return &env{h: r, svc: svc, auth: auth.NewService(q, tm)}
}

// register registra un usuario y devuelve (id, access token).
func (e *env) register(t *testing.T, email string) (uuid.UUID, string) {
	t.Helper()
	user, err := e.auth.Register(context.Background(), email, "p4ssword", "User")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	access, _, err := e.auth.IssueTokens(user.ID)
	if err != nil {
		t.Fatalf("IssueTokens: %v", err)
	}
	return user.ID, access
}

func do(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDueAndToggle(t *testing.T) {
	e := newEnv(t)
	uid, tok := e.register(t, "h@b.com")
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := e.svc.ReplaceForDate(context.Background(), uid, date, []string{"Tender la cama", "Pasear a Ruffo"}); err != nil {
		t.Fatalf("ReplaceForDate: %v", err)
	}

	// GET /due?date=... → 200 con los 2 sembrados.
	rec := do(t, e.h, http.MethodGet, "/commitments/due?date=2026-06-15", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET due code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var dueResp struct {
		Commitments []commitments.Commitment `json:"commitments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &dueResp); err != nil {
		t.Fatalf("unmarshal due: %v", err)
	}
	if len(dueResp.Commitments) != 2 {
		t.Fatalf("due = %d, want 2: %+v", len(dueResp.Commitments), dueResp.Commitments)
	}
	if dueResp.Commitments[0].Text != "Tender la cama" || dueResp.Commitments[1].Text != "Pasear a Ruffo" {
		t.Errorf("due textos = %+v", dueResp.Commitments)
	}

	// POST /{id}/toggle → 200, done=true.
	id := dueResp.Commitments[0].ID
	rec2 := do(t, e.h, http.MethodPost, "/commitments/"+id+"/toggle", tok)
	if rec2.Code != http.StatusOK {
		t.Fatalf("POST toggle code = %d, body = %s", rec2.Code, rec2.Body.String())
	}
	var togResp struct {
		Commitment commitments.Commitment `json:"commitment"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &togResp); err != nil {
		t.Fatalf("unmarshal toggle: %v", err)
	}
	if !togResp.Commitment.Done {
		t.Errorf("toggle done = %v, want true", togResp.Commitment.Done)
	}
}

func TestToggleUnknownIsNotFound(t *testing.T) {
	e := newEnv(t)
	_, tok := e.register(t, "nf@b.com")
	rec := do(t, e.h, http.MethodPost, "/commitments/"+uuid.New().String()+"/toggle", tok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("toggle desconocido code = %d, want 404", rec.Code)
	}
}

func TestRequiresAuth(t *testing.T) {
	e := newEnv(t)
	rec := do(t, e.h, http.MethodGet, "/commitments/due?date=2026-06-15", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestPendingEndpoint(t *testing.T) {
	e := newEnv(t)
	uid, tok := e.register(t, "pending@b.com")
	ctx := context.Background()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	ayer := today.AddDate(0, 0, -1)
	if err := e.svc.ReplaceForDate(ctx, uid, ayer, []string{"Vencido"}); err != nil {
		t.Fatalf("Replace ayer: %v", err)
	}
	if err := e.svc.ReplaceForDate(ctx, uid, today, []string{"De hoy"}); err != nil {
		t.Fatalf("Replace hoy: %v", err)
	}

	rec := do(t, e.h, http.MethodGet, "/commitments/pending", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperaba 200", rec.Code)
	}
	var body struct {
		Commitments []commitments.Commitment `json:"commitments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Commitments) != 2 || body.Commitments[0].Text != "Vencido" {
		t.Fatalf("commitments = %+v (esperaba [Vencido, De hoy])", body.Commitments)
	}

	rec401 := do(t, e.h, http.MethodGet, "/commitments/pending", "")
	if rec401.Code != http.StatusUnauthorized {
		t.Fatalf("sin auth status = %d, esperaba 401", rec401.Code)
	}
}
