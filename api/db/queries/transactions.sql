-- name: CreateTransaction :one
INSERT INTO transactions (user_id, type, amount, occurred_on, cycle, category, remark)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListTransactionsByCycle :many
SELECT * FROM transactions
WHERE user_id = $1 AND cycle = $2
ORDER BY occurred_on DESC, created_at DESC;

-- name: DeleteTransaction :execrows
DELETE FROM transactions
WHERE id = $1 AND user_id = $2;

-- name: SummarizeCycle :one
SELECT
    COALESCE(SUM(amount) FILTER (WHERE type = 'income'), 0)::bigint  AS income,
    COALESCE(SUM(amount) FILTER (WHERE type = 'expense'), 0)::bigint AS expense
FROM transactions
WHERE user_id = $1 AND cycle = $2;

-- name: SummarizeCycles :many
SELECT
    cycle,
    COALESCE(SUM(amount) FILTER (WHERE type = 'income'), 0)::bigint  AS income,
    COALESCE(SUM(amount) FILTER (WHERE type = 'expense'), 0)::bigint AS expense
FROM transactions
WHERE user_id = $1
GROUP BY cycle
ORDER BY cycle DESC;

-- name: UpsertImportedTransaction :one
INSERT INTO transactions (user_id, type, amount, occurred_on, cycle, category, remark, source, external_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'import', $8)
ON CONFLICT (user_id, external_id) WHERE external_id IS NOT NULL
DO UPDATE SET
    type = EXCLUDED.type,
    amount = EXCLUDED.amount,
    occurred_on = EXCLUDED.occurred_on,
    cycle = EXCLUDED.cycle,
    category = EXCLUDED.category,
    remark = EXCLUDED.remark,
    updated_at = now()
RETURNING *;
