package training

import (
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
)

func TestFilterWorkoutsByScope(t *testing.T) {
	today := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	mk := func(d string) store.Workout {
		dt, _ := time.Parse("2006-01-02", d)
		return store.Workout{ID: uuid.New(), Date: dt}
	}
	// ordenadas date DESC, como las devuelve ListWorkouts.
	// 06-11 está EXACTAMENTE en el corte (today-6) → debe entrar (ventana inclusiva);
	// 06-10 queda justo afuera. Esto blinda el límite contra un off-by-one.
	ws := []store.Workout{
		mk("2026-06-17"), mk("2026-06-15"), mk("2026-06-12"),
		mk("2026-06-11"), mk("2026-06-10"), mk("2026-05-20"),
	}

	last := filterWorkoutsByScope(ws, "last", today)
	if len(last) != 3 {
		t.Fatalf("last = %d, want 3", len(last))
	}

	// últimos 7 días: corte en 2026-06-11 → 17, 15, 12, 11 entran (11 = borde inclusivo);
	// 10 y 20-may quedan fuera.
	week := filterWorkoutsByScope(ws, "week", today)
	if len(week) != 4 {
		t.Fatalf("week = %d, want 4 (17,15,12,11)", len(week))
	}
	cutoff := today.AddDate(0, 0, -6) // 2026-06-11
	for _, w := range week {
		if w.Date.Before(cutoff) {
			t.Errorf("week incluyó una sesión vieja: %s", w.Date.Format("2006-01-02"))
		}
	}
	// el borde exacto (06-11) está incluido
	gotBoundary := false
	for _, w := range week {
		if w.Date.Equal(cutoff) {
			gotBoundary = true
		}
	}
	if !gotBoundary {
		t.Error("la sesión en el corte exacto (2026-06-11) debería estar incluida")
	}
}
