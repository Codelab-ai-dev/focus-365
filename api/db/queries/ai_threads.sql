-- name: CreateThread :one
INSERT INTO ai_threads (user_id, title)
VALUES ($1, $2)
RETURNING *;

-- name: GetThread :one
SELECT * FROM ai_threads
WHERE id = $1 AND user_id = $2;

-- name: ListThreads :many
SELECT t.id, t.user_id, t.title, t.created_at, t.updated_at,
       COALESCE(lm.content, '') AS preview
FROM ai_threads t
LEFT JOIN LATERAL (
    SELECT content FROM ai_messages m
    WHERE m.thread_id = t.id
    ORDER BY m.created_at DESC
    LIMIT 1
) lm ON true
WHERE t.user_id = $1
ORDER BY t.updated_at DESC;

-- name: RenameThread :one
UPDATE ai_threads SET title = $3
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteThread :execrows
DELETE FROM ai_threads
WHERE id = $1 AND user_id = $2;

-- name: TouchThread :exec
UPDATE ai_threads SET updated_at = now()
WHERE id = $1;
