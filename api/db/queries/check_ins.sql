-- name: UpsertCheckIn :one
INSERT INTO check_ins (user_id, date, mood, energy, discipline, note)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, date)
DO UPDATE SET
    mood = EXCLUDED.mood,
    energy = EXCLUDED.energy,
    discipline = EXCLUDED.discipline,
    note = EXCLUDED.note,
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
