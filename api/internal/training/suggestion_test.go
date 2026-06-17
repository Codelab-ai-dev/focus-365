package training_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestSuggestHappyPathPersists(t *testing.T) {
	c := &fakeCompleter{out: "Hacé sentadillas 4x8…"}
	e := newEnvWith(t, c, true)
	tok := e.token(t, "sug@b.com")

	// guardar un perfil y un entreno para que entren al contexto
	do(t, e.h, http.MethodPut, "/training/profile", tok, map[string]any{"objective": "hipertrofia", "equipment": []string{"mancuernas"}})
	do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-15", "type": "Pierna",
		"sets": []map[string]any{{"exercise": "Sentadilla", "reps": 8, "weight_grams": 80000}},
	})

	rec := do(t, e.h, http.MethodPost, "/training/suggestion", tok, map[string]any{"focus": "pierna"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST suggestion code = %d, body=%s", rec.Code, rec.Body.String())
	}
	var s map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s["content"] != "Hacé sentadillas 4x8…" || s["focus"] != "pierna" {
		t.Fatalf("sugerencia = %+v", s)
	}
	// el prompt incluyó perfil, historial y enfoque
	for _, want := range []string{"hipertrofia", "Sentadilla", "pierna", "mancuernas"} {
		if !strings.Contains(c.lastUser, want) {
			t.Errorf("el contexto del prompt no contiene %q:\n%s", want, c.lastUser)
		}
	}

	// GET devuelve la última
	rec = do(t, e.h, http.MethodGet, "/training/suggestion", tok, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s["content"] != "Hacé sentadillas 4x8…" {
		t.Fatalf("GET suggestion = %+v", s)
	}
}

func TestGetSuggestionEmpty(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "sugempty@b.com")
	rec := do(t, e.h, http.MethodGet, "/training/suggestion", tok, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("GET vacío = %d %q", rec.Code, rec.Body.String())
	}
}

func TestSuggestNoKeyIs503(t *testing.T) {
	e := newEnvWith(t, &fakeCompleter{out: "x"}, false) // hasKey=false
	tok := e.token(t, "sugnokey@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/suggestion", tok, map[string]any{})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin clave code = %d, want 503", rec.Code)
	}
}

func TestSuggestGroqErrorIs503(t *testing.T) {
	e := newEnvWith(t, &fakeCompleter{err: errors.New("groq caído")}, true)
	tok := e.token(t, "sugerr@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/suggestion", tok, map[string]any{})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("error groq code = %d, want 503", rec.Code)
	}
}

func TestSuggestFocusTooLong(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "suglong@b.com")
	long := strings.Repeat("a", 201)
	rec := do(t, e.h, http.MethodPost, "/training/suggestion", tok, map[string]any{"focus": long})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("focus largo code = %d, want 400", rec.Code)
	}
}
