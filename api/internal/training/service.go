package training

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q    *store.Queries
	pool *pgxpool.Pool
}

// NewService recibe el pool además de las queries: CreateWorkout abre una
// transacción para insertar la sesión y todas sus series de forma atómica.
func NewService(q *store.Queries, pool *pgxpool.Pool) *Service {
	return &Service{q: q, pool: pool}
}

func (s *Service) ListExercises(ctx context.Context, userID uuid.UUID) ([]Exercise, error) {
	rows, err := s.q.ListExercises(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Exercise, 0, len(rows))
	for _, r := range rows {
		out = append(out, Exercise{ID: r.ID.String(), Name: r.Name, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *Service) CreateExercise(ctx context.Context, userID uuid.UUID, name string) (*Exercise, error) {
	row, err := s.q.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: userID, Name: strings.TrimSpace(name)})
	if err != nil {
		return nil, err
	}
	return &Exercise{ID: row.ID.String(), Name: row.Name, CreatedAt: row.CreatedAt}, nil
}

// CreateWorkout inserta la sesión y sus series en una transacción. Por cada
// serie resuelve (o crea) el ejercicio del catálogo por nombre. Si algo falla,
// hace rollback y no deja sesiones a medias.
func (s *Service) CreateWorkout(ctx context.Context, userID uuid.UUID, in WorkoutInput) (*Workout, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	w, err := qtx.CreateWorkout(ctx, store.CreateWorkoutParams{
		UserID: userID, Date: in.Date, Type: in.Type, Note: in.Note,
	})
	if err != nil {
		return nil, err
	}

	// Cache de ejercicios resueltos por nombre (lower) para no repetir upserts.
	cache := make(map[string]uuid.UUID)
	for i, set := range in.Sets {
		name := strings.TrimSpace(set.Exercise)
		key := strings.ToLower(name)
		exID, ok := cache[key]
		if !ok {
			ex, err := qtx.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: userID, Name: name})
			if err != nil {
				return nil, err
			}
			exID = ex.ID
			cache[key] = exID
		}
		if _, err := qtx.CreateWorkoutSet(ctx, store.CreateWorkoutSetParams{
			WorkoutID: w.ID, ExerciseID: exID, Position: int32(i), Reps: set.Reps, WeightGrams: set.WeightGrams,
		}); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetWorkout(ctx, userID, w.ID)
}

// GetWorkout devuelve la sesión si pertenece al usuario; (nil, nil) si no existe.
func (s *Service) GetWorkout(ctx context.Context, userID, id uuid.UUID) (*Workout, error) {
	w, err := s.q.GetWorkout(ctx, store.GetWorkoutParams{ID: id, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rows, err := s.q.ListSetsByWorkout(ctx, w.ID)
	if err != nil {
		return nil, err
	}
	sets := make([]WorkoutSet, 0, len(rows))
	for _, r := range rows {
		sets = append(sets, WorkoutSet{Exercise: r.ExerciseName, Reps: r.Reps, WeightGrams: r.WeightGrams})
	}
	return workoutView(w, sets), nil
}

// ListWorkouts trae el historial por rango (from/to opcionales) con sus series,
// evitando N+1 con una sola consulta de series para todas las sesiones.
func (s *Service) ListWorkouts(ctx context.Context, userID uuid.UUID, from, to *time.Time) ([]Workout, error) {
	rows, err := s.q.ListWorkouts(ctx, store.ListWorkoutsParams{
		UserID: userID, From: from, To: to,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Workout{}, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, w := range rows {
		ids[i] = w.ID
	}
	sets, err := s.q.ListSetsByWorkoutIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byWorkout := make(map[uuid.UUID][]WorkoutSet)
	for _, st := range sets {
		byWorkout[st.WorkoutID] = append(byWorkout[st.WorkoutID], WorkoutSet{
			Exercise: st.ExerciseName, Reps: st.Reps, WeightGrams: st.WeightGrams,
		})
	}
	out := make([]Workout, 0, len(rows))
	for _, w := range rows {
		out = append(out, *workoutView(w, byWorkout[w.ID]))
	}
	return out, nil
}

// DeleteWorkout borra la sesión si pertenece al usuario; devuelve si borró algo.
func (s *Service) DeleteWorkout(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	n, err := s.q.DeleteWorkout(ctx, store.DeleteWorkoutParams{ID: id, UserID: userID})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func workoutView(w store.Workout, sets []WorkoutSet) *Workout {
	if sets == nil {
		sets = []WorkoutSet{}
	}
	return &Workout{
		ID:        w.ID.String(),
		Date:      w.Date.Format(dateLayout),
		Type:      w.Type,
		Note:      w.Note,
		Sets:      sets,
		CreatedAt: w.CreatedAt,
	}
}

