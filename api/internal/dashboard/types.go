// Package dashboard compone un snapshot agregado del día a partir de los cinco
// servicios de dominio (checkin, finance, training, habits, goals). No tiene
// tablas ni queries propias: es una vista de solo lectura.
package dashboard

// StreakView resume las rachas de hábitos.
// BestCurrent = mayor current_streak entre hábitos activos.
// DoneToday = cuántos activos están marcados hoy. Total = nº de hábitos activos.
type StreakView struct {
	BestCurrent int `json:"best_current"`
	DoneToday   int `json:"done_today"`
	Total       int `json:"total"`
}

// FinanceView resume el ciclo de pago vigente (net en centavos).
type FinanceView struct {
	Cycle  string `json:"cycle"`
	Net    int64  `json:"net"`
	Status string `json:"status"`
}

// CheckinView resume el check-in de hoy. El campo `checkin` del Snapshot
// serializa null cuando no hay check-in (puntero nil).
type CheckinView struct {
	Present bool   `json:"present"`
	Mood    int    `json:"mood"`
	Energy  int    `json:"energy"`
	Win     string `json:"win"`
}

// TrainingView indica si entrenó hoy y de qué tipo (vacío si no entrenó).
type TrainingView struct {
	TrainedToday bool   `json:"trained_today"`
	Type         string `json:"type"`
}

// GoalsView resume las metas activas. AvgProgress es el promedio entero del
// progreso de las activas (0 si no hay). Overdue = cuántas activas están vencidas.
type GoalsView struct {
	Active      int `json:"active"`
	AvgProgress int `json:"avg_progress"`
	Overdue     int `json:"overdue"`
}

// Snapshot es la vista agregada que devuelve el endpoint.
type Snapshot struct {
	Streak           StreakView   `json:"streak"`
	Finance          FinanceView  `json:"finance"`
	Checkin          *CheckinView `json:"checkin"`
	Training         TrainingView `json:"training"`
	Goals            GoalsView    `json:"goals"`
	DimensionsActive int          `json:"dimensions_active"`
}

// countActive cuenta cuántas de las 5 dimensiones tienen algo que mostrar hoy.
func countActive(s *Snapshot) int {
	n := 0
	if s.Checkin != nil {
		n++
	}
	if s.Streak.Total > 0 {
		n++
	}
	// El ciclo vigente siempre está "pendiente" (no cierra hasta el próximo
	// día de pago), así que el estado no sirve como señal: contamos finanzas
	// como activa cuando el ciclo tuvo movimiento (net distinto de cero).
	if s.Finance.Net != 0 {
		n++
	}
	if s.Training.TrainedToday {
		n++
	}
	if s.Goals.Active > 0 {
		n++
	}
	return n
}
