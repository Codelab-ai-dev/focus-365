package training

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	suggestionHistoryLimit = 8
	maxFocusChars          = 200
)

// ErrUnavailable: el entrenador IA no está disponible (sin clave o fallo de Groq).
// El handler lo traduce a 503.
var ErrUnavailable = errors.New("entrenador no disponible")

// completer abstrae la llamada bloqueante a Groq (la satisface *ai.GroqClient).
// Definida acá para no importar el paquete ai (evita ciclo).
type completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// Suggestion es la vista de la última sugerencia del entrenador.
type Suggestion struct {
	Focus     string    `json:"focus"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func buildSuggestion(s store.TrainingSuggestion) Suggestion {
	return Suggestion{Focus: s.Focus, Content: s.Content, CreatedAt: s.CreatedAt}
}

const suggestionSystemPrompt = `Sos un entrenador personal. A partir del PERFIL y el HISTORIAL del usuario, proponé una rutina o ejercicios concretos para su próxima sesión.
Reglas:
- Priorizá el equipo disponible y el lugar (si entrena en casa, usá lo que tiene).
- Apuntá al objetivo y respetá las limitaciones/lesiones.
- Ajustá el volumen y la intensidad al nivel y la frecuencia.
- Si hay un ENFOQUE PEDIDO, centrate en eso.
- Si el perfil está incompleto, hacé una sugerencia general y recomendá completar el perfil.
- Respondé en español, con ejercicios, series×reps y descansos, y una breve explicación. Sé concreto y accionable.`

// Suggestion devuelve la última sugerencia guardada, o nil si no hay.
func (s *Service) Suggestion(ctx context.Context, userID uuid.UUID) (*Suggestion, error) {
	row, err := s.q.GetTrainingSuggestion(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	v := buildSuggestion(row)
	return &v, nil
}

// SuggestTraining genera una sugerencia con Groq desde el perfil + historial +
// enfoque, la persiste (upsert) y la devuelve. ErrUnavailable sin clave o ante
// fallo de Groq.
func (s *Service) SuggestTraining(ctx context.Context, userID uuid.UUID, focus string, today time.Time) (*Suggestion, error) {
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
	if len(workouts) > suggestionHistoryLimit {
		workouts = workouts[:suggestionHistoryLimit]
	}
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

	userCtx := buildSuggestionContext(profile, workouts, sets, focus, today)
	content, err := s.groq.Complete(ctx, suggestionSystemPrompt, userCtx)
	if err != nil {
		return nil, ErrUnavailable
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrUnavailable
	}
	row, err := s.q.UpsertTrainingSuggestion(ctx, store.UpsertTrainingSuggestionParams{
		UserID: userID, Focus: focus, Content: content,
	})
	if err != nil {
		return nil, err
	}
	v := buildSuggestion(row)
	return &v, nil
}

// buildSuggestionContext arma el texto del usuario para el prompt: perfil +
// historial reciente (con series) + enfoque.
func buildSuggestionContext(p *store.FitnessProfile, workouts []store.Workout, sets []store.ListSetsByWorkoutIDsRow, focus string, today time.Time) string {
	var b strings.Builder
	b.WriteString("PERFIL:\n")
	if p == nil {
		b.WriteString("(sin perfil cargado)\n")
	} else {
		if p.Birthdate != nil {
			fmt.Fprintf(&b, "- edad: %d años\n", ageFrom(*p.Birthdate, today))
		}
		if p.Sex != nil {
			b.WriteString("- sexo: " + *p.Sex + "\n")
		}
		if p.HeightCm != nil {
			fmt.Fprintf(&b, "- altura: %d cm\n", *p.HeightCm)
		}
		if p.WeightGrams != nil {
			fmt.Fprintf(&b, "- peso: %.1f kg\n", float64(*p.WeightGrams)/1000)
		}
		if p.Objective != nil {
			b.WriteString("- objetivo: " + *p.Objective + "\n")
		}
		if p.Location != nil {
			b.WriteString("- lugar: " + *p.Location + "\n")
		}
		if p.Level != nil {
			b.WriteString("- nivel: " + *p.Level + "\n")
		}
		if p.WeeklyDays != nil {
			fmt.Fprintf(&b, "- días por semana: %d\n", *p.WeeklyDays)
		}
		if len(p.Equipment) > 0 {
			b.WriteString("- equipo: " + strings.Join(p.Equipment, ", ") + "\n")
		}
		if p.Limitations != "" {
			b.WriteString("- limitaciones: " + p.Limitations + "\n")
		}
	}

	b.WriteString("\nHISTORIAL RECIENTE:\n")
	if len(workouts) == 0 {
		b.WriteString("(sin entrenos registrados)\n")
	} else {
		byWorkout := map[uuid.UUID][]store.ListSetsByWorkoutIDsRow{}
		for _, st := range sets {
			byWorkout[st.WorkoutID] = append(byWorkout[st.WorkoutID], st)
		}
		for _, w := range workouts {
			b.WriteString("- " + w.Date.Format("2006-01-02"))
			if w.Type != "" {
				b.WriteString(" (" + w.Type + ")")
			}
			b.WriteString(":\n")
			for _, st := range byWorkout[w.ID] {
				b.WriteString("    · " + st.ExerciseName)
				if st.Reps != nil {
					fmt.Fprintf(&b, " %d reps", *st.Reps)
				}
				if st.WeightGrams != nil {
					fmt.Fprintf(&b, " @ %.1f kg", float64(*st.WeightGrams)/1000)
				}
				b.WriteString("\n")
			}
		}
	}

	if strings.TrimSpace(focus) != "" {
		b.WriteString("\nENFOQUE PEDIDO: " + strings.TrimSpace(focus) + "\n")
	}
	return b.String()
}

// ageFrom calcula la edad en años a la fecha `today`.
func ageFrom(birth, today time.Time) int {
	y := today.Year() - birth.Year()
	if today.Month() < birth.Month() || (today.Month() == birth.Month() && today.Day() < birth.Day()) {
		y--
	}
	return y
}
