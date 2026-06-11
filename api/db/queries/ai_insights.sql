-- name: GetInsight :one
SELECT * FROM ai_insights
WHERE user_id = $1 AND insight_date = $2 AND kind = $3;

-- name: CreateInsight :one
INSERT INTO ai_insights (user_id, insight_date, kind, content, context_snapshot)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
