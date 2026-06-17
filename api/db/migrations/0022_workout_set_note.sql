-- +goose Up
ALTER TABLE workout_sets ADD COLUMN note TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE workout_sets DROP COLUMN note;
