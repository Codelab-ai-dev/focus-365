-- +goose Up
CREATE TABLE training_adjustments (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    scope      TEXT NOT NULL,
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE training_adjustments;
