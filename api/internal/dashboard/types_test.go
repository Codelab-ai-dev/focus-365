package dashboard

import "testing"

func TestCountActiveEmpty(t *testing.T) {
	s := &Snapshot{}
	if got := countActive(s); got != 0 {
		t.Errorf("countActive(vacío) = %d, want 0", got)
	}
}

func TestCountActiveAll(t *testing.T) {
	s := &Snapshot{
		Checkin:  &CheckinView{Present: true, Mood: 8, Energy: 6, Discipline: 9},
		Streak:   StreakView{BestCurrent: 12, DoneToday: 2, Total: 4},
		Finance:  FinanceView{Cycle: "2026-06", Net: 320000, Status: "verde"},
		Training: TrainingView{TrainedToday: true, Type: "Fuerza"},
		Goals:    GoalsView{Active: 3, AvgProgress: 40, Overdue: 1},
	}
	if got := countActive(s); got != 5 {
		t.Errorf("countActive(todo) = %d, want 5", got)
	}
}

func TestCountActiveSubset(t *testing.T) {
	// Solo hábitos activos y metas activas → 2.
	s := &Snapshot{
		Streak:  StreakView{Total: 4},
		Finance: FinanceView{Status: "pendiente"},
		Goals:   GoalsView{Active: 1},
	}
	if got := countActive(s); got != 2 {
		t.Errorf("countActive(subset) = %d, want 2", got)
	}
}
