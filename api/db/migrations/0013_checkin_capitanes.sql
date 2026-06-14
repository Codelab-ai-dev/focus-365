-- +goose Up
ALTER TABLE check_ins
    DROP COLUMN discipline,
    DROP COLUMN note,
    ADD COLUMN dim_espiritual TEXT NOT NULL DEFAULT '',
    ADD COLUMN dim_emocional  TEXT NOT NULL DEFAULT '',
    ADD COLUMN dim_fisica     TEXT NOT NULL DEFAULT '',
    ADD COLUMN dim_financiera TEXT NOT NULL DEFAULT '',
    ADD COLUMN win            TEXT NOT NULL DEFAULT '',
    ADD COLUMN avoided        TEXT NOT NULL DEFAULT '',
    ADD COLUMN commitments    JSONB NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE check_ins
    DROP COLUMN dim_espiritual,
    DROP COLUMN dim_emocional,
    DROP COLUMN dim_fisica,
    DROP COLUMN dim_financiera,
    DROP COLUMN win,
    DROP COLUMN avoided,
    DROP COLUMN commitments,
    ADD COLUMN discipline INT NOT NULL DEFAULT 0,
    ADD COLUMN note TEXT NOT NULL DEFAULT '';
