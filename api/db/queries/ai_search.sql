-- name: SearchThreadsByTitle :many
SELECT t.id, t.title, t.updated_at,
       COALESCE(lm.content, '') AS preview
FROM ai_threads t
LEFT JOIN LATERAL (
    SELECT content FROM ai_messages m
    WHERE m.thread_id = t.id
    ORDER BY m.created_at DESC
    LIMIT 1
) lm ON true
WHERE t.user_id = @user_id
  AND unaccent(lower(t.title)) LIKE '%' || unaccent(lower(@term::text)) || '%'
ORDER BY t.updated_at DESC
LIMIT @lim;

-- name: SearchMessages :many
SELECT m.id, m.thread_id, m.role, m.content, m.created_at,
       t.title AS thread_title
FROM ai_messages m
JOIN ai_threads t ON t.id = m.thread_id
WHERE m.user_id = @user_id
  AND unaccent(lower(m.content)) LIKE '%' || unaccent(lower(@term::text)) || '%'
ORDER BY m.created_at DESC
LIMIT @lim;
