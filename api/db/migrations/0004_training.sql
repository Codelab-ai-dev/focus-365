-- +goose Up
CREATE TABLE exercises (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Único por usuario sin distinguir mayúsculas: "Sentadilla" == "sentadilla".
CREATE UNIQUE INDEX uq_exercises_user_name
    ON exercises (user_id, lower(name));

CREATE TABLE workouts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date       DATE NOT NULL,
    type       TEXT NOT NULL DEFAULT '',   -- libre: "Fuerza", "Pierna"…
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_workouts_user_date
    ON workouts (user_id, date DESC, created_at DESC);

CREATE TABLE workout_sets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workout_id   UUID NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    exercise_id  UUID NOT NULL REFERENCES exercises(id),
    position     INT NOT NULL,            -- orden de la serie dentro de la sesión
    reps         INT,                     -- opcional
    weight_grams INT,                     -- opcional; peso en gramos (80kg = 80000)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_workout_sets_workout ON workout_sets (workout_id, position);
CREATE INDEX idx_workout_sets_exercise ON workout_sets (exercise_id);

-- +goose Down
DROP TABLE workout_sets;
DROP TABLE workouts;
DROP TABLE exercises;
