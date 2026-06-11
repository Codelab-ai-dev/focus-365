-- +goose Up
CREATE TABLE habits (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    target_days INT,                      -- meta de N días (challenge); NULL = hábito abierto
    archived_at TIMESTAMPTZ,              -- archivado manual; NULL = activo
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Único parcial: no se permiten dos hábitos ACTIVOS con el mismo nombre (sin
-- distinguir mayúsculas); un nombre archivado no bloquea recrearlo.
CREATE UNIQUE INDEX uq_habits_user_name
    ON habits (user_id, lower(name)) WHERE archived_at IS NULL;
CREATE INDEX idx_habits_user_active
    ON habits (user_id, created_at DESC);

CREATE TABLE habit_logs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    habit_id   UUID NOT NULL REFERENCES habits(id) ON DELETE CASCADE,
    day        DATE NOT NULL,             -- el día cumplido
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Un registro por hábito por día. Marcar = upsert; desmarcar = delete.
CREATE UNIQUE INDEX uq_habit_logs_habit_day
    ON habit_logs (habit_id, day);
CREATE INDEX idx_habit_logs_habit_day
    ON habit_logs (habit_id, day DESC);

-- +goose Down
DROP TABLE habit_logs;
DROP TABLE habits;
