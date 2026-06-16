-- name: ListThreadMessages :many
SELECT * FROM ai_messages
WHERE thread_id = $1
ORDER BY created_at ASC;

-- name: CreateMessage :one
INSERT INTO ai_messages (user_id, thread_id, role, content)
VALUES ($1, $2, $3, $4)
RETURNING *;
