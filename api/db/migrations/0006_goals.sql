-- +goose Up
CREATE TABLE goals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    dimension   TEXT NOT NULL,                    -- checkin|finanzas|entrenamiento|mente|general
    status      TEXT NOT NULL DEFAULT 'active',   -- active|done|paused
    progress    INT  NOT NULL DEFAULT 0,          -- 0..100
    deadline    DATE,                             -- opcional (NULL = sin fecha)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT goals_progress_range CHECK (progress BETWEEN 0 AND 100),
    CONSTRAINT goals_status_valid   CHECK (status IN ('active','done','paused'))
);
CREATE INDEX idx_goals_user_status ON goals (user_id, status, created_at DESC);

-- +goose Down
DROP TABLE goals;
