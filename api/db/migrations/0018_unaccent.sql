-- +goose Up
CREATE EXTENSION IF NOT EXISTS unaccent;

-- +goose Down
-- No se elimina la extensión (puede usarla otra cosa).
