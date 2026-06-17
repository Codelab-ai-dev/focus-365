-- +goose Up
CREATE TABLE goal_notes (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    goal_id    UUID NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note_date  DATE NOT NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_goal_notes_goal ON goal_notes (goal_id, note_date DESC, created_at DESC);

-- +goose Down
DROP TABLE goal_notes;
