package ai

import (
	"strings"
	"testing"
)

func TestBuildChatSystemPrompt(t *testing.T) {
	ctxJSON := `{"snapshot":{"streak":{"best_current":12}}}`
	got := buildChatSystemPrompt(ctxJSON)

	// Incrusta el JSON de contexto literal.
	if !strings.Contains(got, ctxJSON) {
		t.Errorf("el prompt no incrusta el contexto:\n%s", got)
	}
	// Instruye español, concisión y no inventar.
	for _, want := range []string{"español", "ÚNICAMENTE", "inventes"} {
		if !strings.Contains(got, want) {
			t.Errorf("el prompt no menciona %q:\n%s", want, got)
		}
	}
}
