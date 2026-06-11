package habits

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, _ := time.Parse(dateLayout, s)
	return t
}

func TestComputeStreaks(t *testing.T) {
	today := d("2026-06-14") // referencia fija

	cases := []struct {
		name      string
		days      []time.Time
		wantCur   int
		wantBest  int
		wantToday bool
		wantYest  bool
	}{
		{
			name: "historial vacío",
			days: nil,
			wantCur: 0, wantBest: 0, wantToday: false, wantYest: false,
		},
		{
			name: "un solo día hoy",
			days: []time.Time{d("2026-06-14")},
			wantCur: 1, wantBest: 1, wantToday: true, wantYest: false,
		},
		{
			name: "corrida consecutiva hasta hoy",
			days: []time.Time{d("2026-06-12"), d("2026-06-13"), d("2026-06-14")},
			wantCur: 3, wantBest: 3, wantToday: true, wantYest: true,
		},
		{
			name: "racha viva anclada en ayer (hoy pendiente)",
			days: []time.Time{d("2026-06-12"), d("2026-06-13")},
			wantCur: 2, wantBest: 2, wantToday: false, wantYest: true,
		},
		{
			name: "racha cortada (ni hoy ni ayer)",
			days: []time.Time{d("2026-06-10"), d("2026-06-11")},
			wantCur: 0, wantBest: 2, wantToday: false, wantYest: false,
		},
		{
			name: "récord mayor que la actual",
			days: []time.Time{d("2026-06-09"), d("2026-06-10"), d("2026-06-11"), d("2026-06-13"), d("2026-06-14")},
			wantCur: 2, wantBest: 3, wantToday: true, wantYest: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cur, best, doneToday, doneYest := computeStreaks(c.days, today)
			if cur != c.wantCur {
				t.Errorf("current = %d, want %d", cur, c.wantCur)
			}
			if best != c.wantBest {
				t.Errorf("best = %d, want %d", best, c.wantBest)
			}
			if doneToday != c.wantToday {
				t.Errorf("doneToday = %v, want %v", doneToday, c.wantToday)
			}
			if doneYest != c.wantYest {
				t.Errorf("doneYesterday = %v, want %v", doneYest, c.wantYest)
			}
		})
	}
}
