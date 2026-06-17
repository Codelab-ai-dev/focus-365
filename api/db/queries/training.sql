-- name: ListExercises :many
SELECT * FROM exercises
WHERE user_id = $1
ORDER BY name ASC;

-- name: UpsertExercise :one
-- Crea el ejercicio o, si ya existe (mismo user + lower(name)), devuelve el actual.
INSERT INTO exercises (user_id, name)
VALUES ($1, $2)
ON CONFLICT (user_id, lower(name)) DO UPDATE SET name = exercises.name
RETURNING *;

-- name: CreateWorkout :one
INSERT INTO workouts (user_id, date, type, note)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateWorkoutSet :one
INSERT INTO workout_sets (workout_id, exercise_id, position, reps, weight_grams, note)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListWorkouts :many
SELECT * FROM workouts
WHERE user_id = sqlc.arg('user_id')
  AND (sqlc.narg('from')::date IS NULL OR date >= sqlc.narg('from'))
  AND (sqlc.narg('to')::date   IS NULL OR date <= sqlc.narg('to'))
ORDER BY date DESC, created_at DESC;

-- name: GetWorkout :one
SELECT * FROM workouts
WHERE id = $1 AND user_id = $2;

-- name: ListSetsByWorkout :many
SELECT ws.position, ws.reps, ws.weight_grams, ws.note, e.name AS exercise_name
FROM workout_sets ws
JOIN exercises e ON e.id = ws.exercise_id
WHERE ws.workout_id = $1
ORDER BY ws.position ASC;

-- name: ListSetsByWorkoutIDs :many
SELECT ws.workout_id, ws.position, ws.reps, ws.weight_grams, ws.note, e.name AS exercise_name
FROM workout_sets ws
JOIN exercises e ON e.id = ws.exercise_id
WHERE ws.workout_id = ANY(sqlc.arg('workout_ids')::uuid[])
ORDER BY ws.position ASC;

-- name: DeleteWorkout :execrows
DELETE FROM workouts
WHERE id = $1 AND user_id = $2;
