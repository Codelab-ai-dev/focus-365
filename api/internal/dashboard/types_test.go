package dashboard

import (
	"testing"

	"github.com/focus365/api/internal/checkin"
	"github.com/focus365/api/internal/finance"
	"github.com/focus365/api/internal/goals"
	"github.com/focus365/api/internal/habits"
	"github.com/focus365/api/internal/training"
)

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

func TestStreakViewFromHabits(t *testing.T) {
	hs := []habits.Habit{
		{CurrentStreak: 12, DoneToday: true},
		{CurrentStreak: 3, DoneToday: false},
	}
	v := streakView(hs)
	if v.BestCurrent != 12 || v.DoneToday != 1 || v.Total != 2 {
		t.Errorf("streakView = %+v, want {12 1 2}", v)
	}
}

func TestCheckinViewNilWhenAbsent(t *testing.T) {
	if v := checkinView(nil); v != nil {
		t.Errorf("checkinView(nil) = %+v, want nil", v)
	}
	c := &checkin.CheckIn{Mood: 8, Energy: 6, Discipline: 9}
	v := checkinView(c)
	if v == nil || !v.Present || v.Mood != 8 || v.Energy != 6 || v.Discipline != 9 {
		t.Errorf("checkinView = %+v, want present 8/6/9", v)
	}
}

func TestTrainingViewFirstWorkout(t *testing.T) {
	if v := trainingView(nil); v.TrainedToday || v.Type != "" {
		t.Errorf("trainingView(vacío) = %+v, want no entrenó", v)
	}
	ws := []training.Workout{{Type: "Fuerza"}, {Type: "Cardio"}}
	v := trainingView(ws)
	if !v.TrainedToday || v.Type != "Fuerza" {
		t.Errorf("trainingView = %+v, want Fuerza ✓", v)
	}
}

func TestGoalsViewAverages(t *testing.T) {
	if v := goalsView(nil); v.Active != 0 || v.AvgProgress != 0 || v.Overdue != 0 {
		t.Errorf("goalsView(vacío) = %+v, want ceros", v)
	}
	gs := []goals.Goal{
		{Progress: 20, Overdue: true},
		{Progress: 60, Overdue: false},
	}
	v := goalsView(gs)
	if v.Active != 2 || v.AvgProgress != 40 || v.Overdue != 1 {
		t.Errorf("goalsView = %+v, want {2 40 1}", v)
	}
}

func TestFinanceViewFromSummary(t *testing.T) {
	cs := &finance.CycleSummary{Cycle: "2026-06", Net: 320000, Status: "verde"}
	v := financeView(cs)
	if v.Cycle != "2026-06" || v.Net != 320000 || v.Status != "verde" {
		t.Errorf("financeView = %+v", v)
	}
}
