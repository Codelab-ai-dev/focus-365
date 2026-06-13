-- name: CreateAction :one
INSERT INTO ai_actions (message_id, user_id, position, kind, payload, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListActionsByMessages :many
SELECT * FROM ai_actions
WHERE message_id = ANY($1::uuid[])
ORDER BY message_id, position;

-- name: GetAction :one
SELECT * FROM ai_actions
WHERE id = $1 AND user_id = $2;

-- name: SetActionStatusFrom :one
UPDATE ai_actions
SET status = $3, result = COALESCE($4, result)
WHERE id = $1 AND user_id = $2 AND status = $5
RETURNING *;

-- name: CreateUploadAction :one
INSERT INTO ai_actions (user_id, position, kind, payload, status, source)
VALUES ($1, $2, $3, $4, 'proposed', 'upload')
RETURNING *;

-- name: ListPendingUploadActions :many
SELECT * FROM ai_actions
WHERE user_id = $1 AND source = 'upload' AND status = 'proposed'
ORDER BY created_at, position;
