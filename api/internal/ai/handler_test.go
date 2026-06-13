package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/focus365/api/internal/ai"
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

type fakeCompleter struct {
	out    string
	err    error
	called int

	chatOut    string
	chatErr    error
	chatCalled int

	chatDeltas    []string
	chatStreamErr error
	chatToolCalls []ai.ToolCall
}

func (f *fakeCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	f.called++
	return f.out, f.err
}

func (f *fakeCompleter) Chat(ctx context.Context, system string, history []ai.ChatMsg) (string, error) {
	f.chatCalled++
	return f.chatOut, f.chatErr
}

func (f *fakeCompleter) ChatStream(ctx context.Context, system string, history []ai.ChatMsg, tools []ai.Tool, onDelta func(string)) (string, []ai.ToolCall, error) {
	f.chatCalled++
	var full string
	for _, d := range f.chatDeltas {
		full += d
		onDelta(d)
	}
	if f.chatStreamErr != nil {
		return "", nil, f.chatStreamErr
	}
	return full, f.chatToolCalls, nil
}

type env struct {
	h    http.Handler
	auth *auth.Service
	comp *fakeCompleter
	q    *store.Queries
}

func newEnv(t *testing.T, hasKey bool, comp *fakeCompleter) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	q := store.New(pool)
	tm := auth.NewTokenManager("secret")

	ci := checkin.NewService(q)
	fi := finance.NewService(q)
	tr := training.NewService(q, pool)
	ha := habits.NewService(q)
	go_ := goals.NewService(q)
	dash := dashboard.NewService(ci, fi, tr, ha, go_)

	svc := ai.NewService(dash, q, comp, hasKey)
	chatCtx := ai.NewChatContextBuilder(dash, fi, ci, ha, go_)
	chatStore := ai.NewChatStore(q, pool)
	actionExec := ai.NewActionExecutor(ci, fi, ha, go_, ha, go_, tr)
	chatSvc := ai.NewChatService(chatCtx, chatStore, comp, comp, actionExec, hasKey)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(tm))
		r.Mount("/ai", ai.Routes(svc, chatSvc))
	})
	return &env{h: r, auth: auth.NewService(q, tm), comp: comp, q: q}
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

func get(t *testing.T, h http.Handler, tok string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ai/insight?today="+today, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func dayTime(t *testing.T) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", today)
	if err != nil {
		t.Fatalf("parse today: %v", err)
	}
	return d
}

func TestGeneratesAndCaches(t *testing.T) {
	comp := &fakeCompleter{out: "Aprovecha tu racha hoy."}
	e := newEnv(t, true, comp)
	_, tok := e.user(t, "gen@b.com")

	rec, body := get(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body["available"] != true || body["content"] != "Aprovecha tu racha hoy." {
		t.Errorf("primera respuesta = %v", body)
	}
	if comp.called != 1 {
		t.Errorf("Groq llamado %d veces, want 1", comp.called)
	}

	rec2, body2 := get(t, e.h, tok)
	if rec2.Code != http.StatusOK {
		t.Fatalf("segunda code = %d", rec2.Code)
	}
	if body2["content"] != "Aprovecha tu racha hoy." {
		t.Errorf("cache content = %v", body2)
	}
	if comp.called != 1 {
		t.Errorf("Groq llamado %d veces tras cache, want 1", comp.called)
	}
}

func TestNoKeyDegrades(t *testing.T) {
	comp := &fakeCompleter{out: "no debería usarse"}
	e := newEnv(t, false, comp)
	uid, tok := e.user(t, "nokey@b.com")

	rec, body := get(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if body["available"] != false || body["content"] != nil {
		t.Errorf("degradado esperado, got %v", body)
	}
	if comp.called != 0 {
		t.Errorf("sin clave no debe llamar Groq")
	}
	if _, err := e.q.GetInsight(context.Background(), store.GetInsightParams{
		UserID: uid, InsightDate: dayTime(t), Kind: "proactive",
	}); err == nil {
		t.Errorf("no debería existir insight en DB")
	}
}

func TestGroqFailureDegrades(t *testing.T) {
	comp := &fakeCompleter{err: errors.New("groq caído")}
	e := newEnv(t, true, comp)
	uid, tok := e.user(t, "fail@b.com")

	rec, body := get(t, e.h, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if body["available"] != false {
		t.Errorf("fallo de Groq debería degradar, got %v", body)
	}
	if _, err := e.q.GetInsight(context.Background(), store.GetInsightParams{
		UserID: uid, InsightDate: dayTime(t), Kind: "proactive",
	}); err == nil {
		t.Errorf("no debería persistir tras fallo de Groq")
	}
}

func TestRequiresAuth(t *testing.T) {
	comp := &fakeCompleter{out: "x"}
	e := newEnv(t, true, comp)
	rec, _ := get(t, e.h, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("sin token code = %d, want 401", rec.Code)
	}
}

func TestUserIsolation(t *testing.T) {
	comp := &fakeCompleter{out: "insight fresco"}
	e := newEnv(t, true, comp)
	_, tokA := e.user(t, "iA@b.com")
	_, tokB := e.user(t, "iB@b.com")

	if rec, _ := get(t, e.h, tokA); rec.Code != http.StatusOK {
		t.Fatalf("A code = %d", rec.Code)
	}
	if comp.called != 1 {
		t.Errorf("tras A, called=%d want 1", comp.called)
	}

	rec, body := get(t, e.h, tokB)
	if rec.Code != http.StatusOK {
		t.Fatalf("B code = %d", rec.Code)
	}
	if body["available"] != true || body["content"] != "insight fresco" {
		t.Errorf("B respuesta = %v", body)
	}
	if comp.called != 2 {
		t.Errorf("B debió generar el suyo, called=%d want 2", comp.called)
	}
}
