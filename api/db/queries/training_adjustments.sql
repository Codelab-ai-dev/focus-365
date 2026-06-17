-- name: GetTrainingAdjustment :one
SELECT * FROM training_adjustments WHERE user_id = $1;

-- name: UpsertTrainingAdjustment :one
INSERT INTO training_adjustments (user_id, scope, content, created_at)
VALUES (@user_id, @scope, @content, now())
ON CONFLICT (user_id) DO UPDATE SET
    scope      = EXCLUDED.scope,
    content    = EXCLUDED.content,
    created_at = now()
RETURNING *;
