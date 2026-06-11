package ai

import (
	"strings"
	"testing"
)

func TestBuildPrompt(t *testing.T) {
	snap := `{"streak":{"best_current":12},"finance":{"net":320000},"checkin":{"mood":8}}`
	system, user := buildPrompt(snap)

	if !strings.Contains(strings.ToLower(system), "español") {
		t.Errorf("system no fija el idioma: %q", system)
	}
	if !strings.Contains(system, "1 a 3 frases") {
		t.Errorf("system no fija el largo: %q", system)
	}
	if !strings.Contains(user, snap) {
		t.Errorf("user no incluye el snapshot: %q", user)
	}
}
