package training

import "time"

const dateLayout = "2006-01-02"

// Exercise es un ejercicio del catálogo del usuario.
type Exercise struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// SetInput es una serie recibida al capturar una sesión.
type SetInput struct {
	Exercise    string
	Reps        *int32
	WeightGrams *int32
	Note        string
}

// WorkoutInput son los datos de dominio para crear una sesión completa.
type WorkoutInput struct {
	Date time.Time
	Type string
	Note string
	Sets []SetInput
}

// WorkoutSet es la vista de una serie (con el nombre del ejercicio resuelto).
type WorkoutSet struct {
	Exercise    string `json:"exercise"`
	Reps        *int32 `json:"reps"`
	WeightGrams *int32 `json:"weight_grams"`
	Note        string `json:"note"`
}

// Workout es la vista de dominio de una sesión que se serializa a JSON.
// date va como YYYY-MM-DD.
type Workout struct {
	ID        string       `json:"id"`
	Date      string       `json:"date"`
	Type      string       `json:"type"`
	Note      string       `json:"note"`
	Sets      []WorkoutSet `json:"sets"`
	CreatedAt time.Time    `json:"created_at"`
}
