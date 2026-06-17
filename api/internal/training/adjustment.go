package training

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	adjustmentLastSessions = 3
	adjustmentWeekDays     = 7
)

// Adjustment es la vista del último análisis/ajustes del agente.
type Adjustment struct {
	Scope     string    `json:"scope"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func buildAdjustment(a store.TrainingAdjustment) Adjustment {
	return Adjustment{Scope: a.Scope, Content: a.Content, CreatedAt: a.CreatedAt}
}

const adjustmentsSystemPrompt = `Sos un entrenador personal. ANALIZÁ el PERFIL y el HISTORIAL reciente del usuario (incluí las notas de cada serie) y proponé AJUSTES concretos para la próxima sesión o la próxima semana.
Reglas:
- Centrate en lo más reciente; usá las sesiones anteriores para comparar progresión.
- Proponé progresión o descarga concreta (peso/reps) por ejercicio.
- Atendé lo que digan las notas (molestias, "fácil", "pesado") y las limitaciones del perfil.
- Sugerí correcciones de técnica o cambios de ejercicio si corresponde.
- Cerrá con un resumen breve de qué ajustar.
- Si no hay entrenos en el período, decilo y sugerí empezar a registrar.
- Respondé en español, concreto y accionable.`

// Adjustment devuelve el último análisis guardado, o nil.
func (s *Service) Adjustment(ctx context.Context, userID uuid.UUID) (*Adjustment, error) {
	row, err := s.q.GetTrainingAdjustment(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v := buildAdjustment(row)
	return &v, nil
}

// SuggestAdjustments genera un análisis con Groq desde el perfil + el historial
// del alcance + las notas, lo persiste (upsert) y lo devuelve. ErrUnavailable
// sin clave o ante fallo de Groq.
func (s *Service) SuggestAdjustments(ctx context.Context, userID uuid.UUID, scope string, today time.Time) (*Adjustment, error) {
	if !s.hasKey {
		return nil, ErrUnavailable
	}
	var profile *store.FitnessProfile
	if p, err := s.q.GetFitnessProfile(ctx, userID); err == nil {
		profile = &p
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	workouts, err := s.q.ListWorkouts(ctx, store.ListWorkoutsParams{UserID: userID})
	if err != nil {
		return nil, err
	}
	workouts = filterWorkoutsByScope(workouts, scope, today)

	var sets []store.ListSetsByWorkoutIDsRow
	if len(workouts) > 0 {
		ids := make([]uuid.UUID, len(workouts))
		for i, w := range workouts {
			ids[i] = w.ID
		}
		if sets, err = s.q.ListSetsByWorkoutIDs(ctx, ids); err != nil {
			return nil, err
		}
	}

	userCtx := buildSuggestionContext(profile, workouts, sets, "", today)
	content, err := s.groq.Complete(ctx, adjustmentsSystemPrompt, userCtx)
	if err != nil {
		return nil, ErrUnavailable
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrUnavailable
	}
	row, err := s.q.UpsertTrainingAdjustment(ctx, store.UpsertTrainingAdjustmentParams{
		UserID: userID, Scope: scope, Content: content,
	})
	if err != nil {
		return nil, err
	}
	v := buildAdjustment(row)
	return &v, nil
}

// filterWorkoutsByScope recorta el historial (que viene ordenado date DESC)
// según el alcance: "week" → sesiones de los últimos adjustmentWeekDays días;
// cualquier otro (incl. "last") → las primeras adjustmentLastSessions.
func filterWorkoutsByScope(workouts []store.Workout, scope string, today time.Time) []store.Workout {
	if scope == "week" {
		cutoff := today.AddDate(0, 0, -(adjustmentWeekDays - 1))
		out := make([]store.Workout, 0, len(workouts))
		for _, w := range workouts {
			if !w.Date.Before(cutoff) {
				out = append(out, w)
			}
		}
		return out
	}
	if len(workouts) > adjustmentLastSessions {
		return workouts[:adjustmentLastSessions]
	}
	return workouts
}
