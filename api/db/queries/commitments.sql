-- name: CreateCommitment :one
INSERT INTO commitments (user_id, target_date, text, position)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteCommitmentsForDate :execrows
DELETE FROM commitments WHERE user_id = $1 AND target_date = $2;

-- name: ListCommitmentsByTarget :many
SELECT * FROM commitments
WHERE user_id = $1 AND target_date = $2
ORDER BY position;

-- name: ToggleCommitment :one
UPDATE commitments
SET done = NOT done, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: ListRecentCommitments :many
SELECT * FROM commitments
WHERE user_id = $1 AND target_date >= $2
ORDER BY target_date DESC, position;

-- name: ListPendingCommitments :many
SELECT * FROM commitments
WHERE user_id = $1 AND done = false AND target_date <= $2
ORDER BY target_date ASC, position ASC;
