package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/focus365/api/internal/store"
	"github.com/focus365/api/internal/testutil"
)

func ptrInt32(v int32) *int32 { return &v }

func TestTrainingStore(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()

	user, err := q.CreateUser(ctx, store.CreateUserParams{
		Email: "gym@b.com", PasswordHash: "h", Name: "Gus",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// UpsertExercise es idempotente por (user_id, lower(name)).
	ex1, err := q.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: user.ID, Name: "Sentadilla"})
	if err != nil {
		t.Fatalf("UpsertExercise: %v", err)
	}
	ex2, err := q.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: user.ID, Name: "sentadilla"})
	if err != nil {
		t.Fatalf("UpsertExercise dup: %v", err)
	}
	if ex1.ID != ex2.ID {
		t.Errorf("el upsert duplicó el catálogo: %s vs %s", ex1.ID, ex2.ID)
	}

	list, err := q.ListExercises(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListExercises: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(catálogo) = %d, want 1", len(list))
	}

	// Crear una sesión con dos series.
	day := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)
	w, err := q.CreateWorkout(ctx, store.CreateWorkoutParams{
		UserID: user.ID, Date: day, Type: "Fuerza", Note: "buen pump",
	})
	if err != nil {
		t.Fatalf("CreateWorkout: %v", err)
	}
	if _, err := q.CreateWorkoutSet(ctx, store.CreateWorkoutSetParams{
		WorkoutID: w.ID, ExerciseID: ex1.ID, Position: 0, Reps: ptrInt32(8), WeightGrams: ptrInt32(80000),
	}); err != nil {
		t.Fatalf("CreateWorkoutSet 0: %v", err)
	}
	if _, err := q.CreateWorkoutSet(ctx, store.CreateWorkoutSetParams{
		WorkoutID: w.ID, ExerciseID: ex1.ID, Position: 1, Reps: ptrInt32(6), WeightGrams: ptrInt32(80000),
	}); err != nil {
		t.Fatalf("CreateWorkoutSet 1: %v", err)
	}

	// ListSetsByWorkout: ordena por position y resuelve el nombre.
	sets, err := q.ListSetsByWorkout(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListSetsByWorkout: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("len(sets) = %d, want 2", len(sets))
	}
	if sets[0].Position != 0 || sets[1].Position != 1 {
		t.Errorf("series fuera de orden: %d, %d", sets[0].Position, sets[1].Position)
	}
	if sets[0].ExerciseName != "Sentadilla" {
		t.Errorf("nombre = %q, want Sentadilla", sets[0].ExerciseName)
	}

	// ListWorkouts filtra por rango. From/To son *time.Time.
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	ws, err := q.ListWorkouts(ctx, store.ListWorkoutsParams{UserID: user.ID, From: &from, To: &to})
	if err != nil {
		t.Fatalf("ListWorkouts: %v", err)
	}
	if len(ws) != 1 {
		t.Fatalf("len(workouts) = %d, want 1", len(ws))
	}

	// DeleteWorkout arrastra las series (cascade) y respeta dueño.
	n, err := q.DeleteWorkout(ctx, store.DeleteWorkoutParams{ID: w.ID, UserID: user.ID})
	if err != nil {
		t.Fatalf("DeleteWorkout: %v", err)
	}
	if n != 1 {
		t.Fatalf("DeleteWorkout afectó %d filas, want 1", n)
	}
	gone, err := q.ListSetsByWorkout(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListSetsByWorkout post-delete: %v", err)
	}
	if len(gone) != 0 {
		t.Errorf("las series no cayeron en cascada: quedan %d", len(gone))
	}
	// El catálogo no se borra al borrar la sesión.
	cat, err := q.ListExercises(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListExercises post-delete: %v", err)
	}
	if len(cat) != 1 {
		t.Errorf("el catálogo cambió al borrar la sesión: %d", len(cat))
	}
}

func TestWorkoutSetNote(t *testing.T) {
	pool := testutil.NewDB(t)
	q := store.New(pool)
	ctx := context.Background()
	u := newUser(t, q)

	ex, err := q.UpsertExercise(ctx, store.UpsertExerciseParams{UserID: u, Name: "Sentadilla"})
	if err != nil {
		t.Fatalf("UpsertExercise: %v", err)
	}
	w, err := q.CreateWorkout(ctx, store.CreateWorkoutParams{
		UserID: u, Date: date("2026-06-17"), Type: "Pierna", Note: "",
	})
	if err != nil {
		t.Fatalf("CreateWorkout: %v", err)
	}
	if _, err := q.CreateWorkoutSet(ctx, store.CreateWorkoutSetParams{
		WorkoutID: w.ID, ExerciseID: ex.ID, Position: 0,
		Reps: nil, WeightGrams: nil, Note: "leí pesado",
	}); err != nil {
		t.Fatalf("CreateWorkoutSet: %v", err)
	}

	rows, err := q.ListSetsByWorkout(ctx, w.ID)
	if err != nil || len(rows) != 1 || rows[0].Note != "leí pesado" {
		t.Fatalf("ListSetsByWorkout: %v rows=%+v", err, rows)
	}
}
