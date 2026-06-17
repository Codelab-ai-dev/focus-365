-- name: GetTrainingSuggestion :one
SELECT * FROM training_suggestions WHERE user_id = $1;

-- name: UpsertTrainingSuggestion :one
INSERT INTO training_suggestions (user_id, focus, content, created_at)
VALUES (@user_id, @focus, @content, now())
ON CONFLICT (user_id) DO UPDATE SET
    focus      = EXCLUDED.focus,
    content    = EXCLUDED.content,
    created_at = now()
RETURNING *;
