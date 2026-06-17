-- +goose Up
CREATE TABLE fitness_profiles (
    user_id      UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    birthdate    DATE,
    sex          TEXT,
    height_cm    INT,
    weight_grams INT,
    objective    TEXT,
    location     TEXT,
    level        TEXT,
    weekly_days  INT,
    equipment    TEXT[] NOT NULL DEFAULT '{}',
    limitations  TEXT NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE fitness_profiles;
