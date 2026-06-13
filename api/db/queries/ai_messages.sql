-- name: ListMessages :many
SELECT * FROM ai_messages
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: CreateMessage :one
INSERT INTO ai_messages (user_id, role, content)
VALUES ($1, $2, $3)
RETURNING *;
