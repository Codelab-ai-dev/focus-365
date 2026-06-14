-- +goose Up
CREATE TABLE commitments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_date DATE NOT NULL,
    text        TEXT NOT NULL,
    done        BOOLEAN NOT NULL DEFAULT false,
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_commitments_user_target ON commitments (user_id, target_date);

-- Migra los compromisos del JSONB de cada check-in (fecha F) a la tabla con
-- target = F+1 y la posición del array.
INSERT INTO commitments (user_id, target_date, text, position)
SELECT c.user_id, c.date + INTERVAL '1 day',
       elem.value #>> '{}', elem.ordinality - 1
FROM check_ins c,
     jsonb_array_elements(c.commitments) WITH ORDINALITY elem(value, ordinality)
WHERE jsonb_array_length(c.commitments) > 0;

ALTER TABLE check_ins DROP COLUMN commitments;

-- +goose Down
ALTER TABLE check_ins ADD COLUMN commitments JSONB NOT NULL DEFAULT '[]';
DROP TABLE commitments;
