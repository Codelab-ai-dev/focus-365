package goals

import "time"

const dateLayout = "2006-01-02"

// Goal es la vista de dominio de una meta, con overdue ya calculado.
// deadline va como fecha YYYY-MM-DD (o null si no tiene).
type Goal struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Dimension string     `json:"dimension"`
	Status    string     `json:"status"`
	Progress  int32      `json:"progress"`
	Deadline  *time.Time `json:"deadline"`
	Overdue   bool       `json:"overdue"`
	CreatedAt time.Time  `json:"created_at"`
}

// GoalInput son los datos de dominio para crear una meta.
type GoalInput struct {
	Title     string
	Dimension string
	Deadline  *time.Time
}

// GoalPatch describe una actualización parcial. Los punteros nil = "no tocar".
// SetDeadline distingue limpiar (true + Deadline nil) de no tocar (false).
type GoalPatch struct {
	Title       *string
	Dimension   *string
	Status      *string
	Progress    *int32
	SetDeadline bool
	Deadline    *time.Time
}

// computeOverdue: una meta está vencida si está activa, tiene deadline y la
// fecha límite es anterior a hoy (comparando por día YYYY-MM-DD).
func computeOverdue(status string, deadline *time.Time, today time.Time) bool {
	if status != "active" || deadline == nil {
		return false
	}
	return deadline.Format(dateLayout) < today.Format(dateLayout)
}
