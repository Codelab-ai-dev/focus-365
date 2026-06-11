package habits

import "time"

const dateLayout = "2006-01-02"

// Habit es la vista de dominio de un hábito con las rachas ya calculadas.
// archived_at va como ISO timestamp (o null si está activo).
type Habit struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	TargetDays    *int32     `json:"target_days"`
	CurrentStreak int        `json:"current_streak"`
	BestStreak    int        `json:"best_streak"`
	DoneToday     bool       `json:"done_today"`
	DoneYesterday bool       `json:"done_yesterday"`
	ArchivedAt    *time.Time `json:"archived_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// HabitInput son los datos de dominio para crear un hábito.
type HabitInput struct {
	Name       string
	TargetDays *int32
}
