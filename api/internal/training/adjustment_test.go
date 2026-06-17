package training_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAdjustHappyPathPersists(t *testing.T) {
	c := &fakeCompleter{out: "Subí 2.5 kg en sentadilla la próxima."}
	e := newEnvWith(t, c, true)
	tok := e.token(t, "adj@b.com")

	do(t, e.h, http.MethodPut, "/training/profile", tok, map[string]any{"objective": "fuerza"})
	do(t, e.h, http.MethodPost, "/training/workouts", tok, map[string]any{
		"date": "2026-06-15", "type": "Pierna",
		"sets": []map[string]any{{"exercise": "Sentadilla", "reps": 5, "weight_grams": 100000, "note": "fácil"}},
	})

	rec := do(t, e.h, http.MethodPost, "/training/adjustment", tok, map[string]any{"scope": "last"})
	if rec.Code != http.StatusOK {
		t.Fatalf("POST adjustment code = %d, body=%s", rec.Code, rec.Body.String())
	}
	var a map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if a["content"] != "Subí 2.5 kg en sentadilla la próxima." || a["scope"] != "last" {
		t.Fatalf("adjustment = %+v", a)
	}
	for _, want := range []string{"Sentadilla", "fácil", "fuerza"} {
		if !strings.Contains(c.lastUser, want) {
			t.Errorf("el contexto no contiene %q:\n%s", want, c.lastUser)
		}
	}

	rec = do(t, e.h, http.MethodGet, "/training/adjustment", tok, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if a["content"] != "Subí 2.5 kg en sentadilla la próxima." {
		t.Fatalf("GET adjustment = %+v", a)
	}
}

func TestGetAdjustmentEmpty(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "adjempty@b.com")
	rec := do(t, e.h, http.MethodGet, "/training/adjustment", tok, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("GET vacío = %d %q", rec.Code, rec.Body.String())
	}
}

func TestAdjustInvalidScope(t *testing.T) {
	e := newEnv(t)
	tok := e.token(t, "adjscope@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/adjustment", tok, map[string]any{"scope": "mes"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("scope inválido code = %d, want 400", rec.Code)
	}
}

func TestAdjustNoKeyIs503(t *testing.T) {
	e := newEnvWith(t, &fakeCompleter{out: "x"}, false)
	tok := e.token(t, "adjnokey@b.com")
	rec := do(t, e.h, http.MethodPost, "/training/adjustment", tok, map[string]any{"scope": "week"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin clave code = %d, want 503", rec.Code)
	}
}
