-- +goose Up
CREATE TABLE training_suggestions (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    focus      TEXT NOT NULL DEFAULT '',
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE training_suggestions;
