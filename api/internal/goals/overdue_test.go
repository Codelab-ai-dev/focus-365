package goals

import (
	"testing"
	"time"
)

func day(s string) time.Time {
	t, _ := time.Parse(dateLayout, s)
	return t
}

func ptrDay(s string) *time.Time {
	t := day(s)
	return &t
}

func TestComputeOverdue(t *testing.T) {
	today := day("2026-06-11")
	cases := []struct {
		name     string
		status   string
		deadline *time.Time
		want     bool
	}{
		{"activa con deadline pasado", "active", ptrDay("2026-06-10"), true},
		{"activa con deadline hoy", "active", ptrDay("2026-06-11"), false},
		{"activa con deadline futuro", "active", ptrDay("2026-06-12"), false},
		{"activa sin deadline", "active", nil, false},
		{"completada con deadline pasado", "done", ptrDay("2026-06-10"), false},
		{"pausada con deadline pasado", "paused", ptrDay("2026-06-10"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := computeOverdue(c.status, c.deadline, today); got != c.want {
				t.Errorf("computeOverdue(%s) = %v, want %v", c.status, got, c.want)
			}
		})
	}
}
