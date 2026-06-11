-- name: CreateHabit :one
-- Idempotente por (user_id, lower(name)) entre hábitos ACTIVOS: si ya existe
-- uno activo con ese nombre, devuelve el actual (no toca target_days).
INSERT INTO habits (user_id, name, target_days)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, lower(name)) WHERE archived_at IS NULL
DO UPDATE SET name = habits.name
RETURNING *;

-- name: ListHabits :many
SELECT * FROM habits
WHERE user_id = $1 AND archived_at IS NULL
ORDER BY created_at DESC;

-- name: ListArchivedHabits :many
SELECT * FROM habits
WHERE user_id = $1 AND archived_at IS NOT NULL
ORDER BY archived_at DESC;

-- name: GetHabit :one
SELECT * FROM habits
WHERE id = $1 AND user_id = $2;

-- name: ArchiveHabit :one
UPDATE habits
SET archived_at = now()
WHERE id = $1 AND user_id = $2 AND archived_at IS NULL
RETURNING *;

-- name: DeleteHabit :execrows
DELETE FROM habits
WHERE id = $1 AND user_id = $2;

-- name: UpsertHabitLog :exec
INSERT INTO habit_logs (habit_id, day)
VALUES ($1, $2)
ON CONFLICT (habit_id, day) DO NOTHING;

-- name: DeleteHabitLog :exec
DELETE FROM habit_logs
WHERE habit_id = $1 AND day = $2;

-- name: ListLogsByHabitIDs :many
SELECT habit_id, day FROM habit_logs
WHERE habit_id = ANY(sqlc.arg('habit_ids')::uuid[])
ORDER BY day ASC;
