-- name: ListMessages :many
SELECT * FROM ai_messages
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: CreateMessage :one
INSERT INTO ai_messages (user_id, role, content)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CreateMessageWithAction :one
INSERT INTO ai_messages (user_id, role, content, action_kind, action_payload, action_status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetMessageForAction :one
SELECT * FROM ai_messages
WHERE id = $1 AND user_id = $2;

-- name: SetActionStatus :one
UPDATE ai_messages
SET action_status = $3
WHERE id = $1 AND user_id = $2 AND action_status = 'proposed'
RETURNING *;
