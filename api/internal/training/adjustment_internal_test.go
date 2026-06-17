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
	// ordenadas date DESC, como las devuelve ListWorkouts
	ws := []store.Workout{mk("2026-06-17"), mk("2026-06-15"), mk("2026-06-12"), mk("2026-06-05"), mk("2026-05-20")}

	last := filterWorkoutsByScope(ws, "last", today)
	if len(last) != 3 {
		t.Fatalf("last = %d, want 3", len(last))
	}

	// últimos 7 días: corte en 2026-06-11 → 17, 15, 12 entran; 05 y 20-may quedan fuera
	week := filterWorkoutsByScope(ws, "week", today)
	if len(week) != 3 {
		t.Fatalf("week = %d, want 3 (17,15,12)", len(week))
	}
	cutoff := today.AddDate(0, 0, -6)
	for _, w := range week {
		if w.Date.Before(cutoff) {
			t.Errorf("week incluyó una sesión vieja: %s", w.Date.Format("2006-01-02"))
		}
	}
}
