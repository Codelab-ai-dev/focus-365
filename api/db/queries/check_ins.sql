-- name: UpsertCheckIn :one
INSERT INTO check_ins (user_id, date, mood, energy,
    dim_espiritual, dim_emocional, dim_fisica, dim_financiera, win, avoided, commitments)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (user_id, date)
DO UPDATE SET
    mood = EXCLUDED.mood,
    energy = EXCLUDED.energy,
    dim_espiritual = EXCLUDED.dim_espiritual,
    dim_emocional = EXCLUDED.dim_emocional,
    dim_fisica = EXCLUDED.dim_fisica,
    dim_financiera = EXCLUDED.dim_financiera,
    win = EXCLUDED.win,
    avoided = EXCLUDED.avoided,
    commitments = EXCLUDED.commitments,
    updated_at = now()
RETURNING *;

-- name: UpsertCheckInMetrics :one
-- Upsert parcial: solo mood/energy, sin tocar reflexiones (lo usa la IA).
INSERT INTO check_ins (user_id, date, mood, energy)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, date)
DO UPDATE SET
    mood = EXCLUDED.mood,
    energy = EXCLUDED.energy,
    updated_at = now()
RETURNING *;

-- name: GetCheckInByDate :one
SELECT * FROM check_ins
WHERE user_id = $1 AND date = $2;

-- name: ListCheckIns :many
SELECT * FROM check_ins
WHERE user_id = $1
ORDER BY date DESC
LIMIT $2;

-- name: DeleteCheckIn :execrows
DELETE FROM check_ins
WHERE user_id = $1 AND date = $2;
