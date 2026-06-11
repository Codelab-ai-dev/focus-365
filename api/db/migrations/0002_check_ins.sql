-- +goose Up
CREATE TABLE check_ins (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date        DATE NOT NULL,
    mood        INT  NOT NULL CHECK (mood       BETWEEN 1 AND 10),
    energy      INT  NOT NULL CHECK (energy     BETWEEN 1 AND 10),
    discipline  INT  NOT NULL CHECK (discipline BETWEEN 1 AND 10),
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, date)
);
CREATE INDEX idx_check_ins_user_date ON check_ins (user_id, date DESC);

-- +goose Down
DROP TABLE check_ins;
