-- name: CreateGoalNote :one
INSERT INTO goal_notes (goal_id, user_id, note_date, body)
SELECT @goal_id, @user_id, @note_date, @body
WHERE EXISTS (SELECT 1 FROM goals WHERE id = @goal_id AND user_id = @user_id)
RETURNING *;

-- name: ListGoalNotes :many
SELECT n.* FROM goal_notes n
JOIN goals g ON g.id = n.goal_id
WHERE n.goal_id = @goal_id AND g.user_id = @user_id
ORDER BY n.note_date DESC, n.created_at DESC;

-- name: DeleteGoalNote :execrows
DELETE FROM goal_notes WHERE id = @id AND user_id = @user_id;
