-- name: CreateGoal :one
INSERT INTO goals (user_id, title, dimension, deadline)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListGoals :many
SELECT * FROM goals
WHERE user_id = $1 AND status = $2
ORDER BY created_at DESC;

-- name: GetGoal :one
SELECT * FROM goals
WHERE id = $1 AND user_id = $2;

-- name: UpdateGoal :one
UPDATE goals SET
    title     = COALESCE(sqlc.narg('title'), title),
    dimension = COALESCE(sqlc.narg('dimension'), dimension),
    status    = COALESCE(sqlc.narg('status'), status),
    progress  = COALESCE(sqlc.narg('progress'), progress),
    deadline  = CASE WHEN sqlc.arg('set_deadline')::bool
                     THEN sqlc.narg('deadline')
                     ELSE deadline END
WHERE id = sqlc.arg('id') AND user_id = sqlc.arg('user_id')
RETURNING *;

-- name: DeleteGoal :execrows
DELETE FROM goals
WHERE id = $1 AND user_id = $2;
