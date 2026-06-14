-- +goose Up
UPDATE goals SET dimension = CASE dimension
    WHEN 'finanzas'      THEN 'financiera'
    WHEN 'entrenamiento' THEN 'fisica'
    WHEN 'mente'         THEN 'emocional'
    WHEN 'checkin'       THEN 'emocional'
    WHEN 'general'       THEN 'espiritual'
    ELSE dimension
END;
ALTER TABLE goals ADD CONSTRAINT goals_dimension_valid
    CHECK (dimension IN ('espiritual','emocional','fisica','financiera'));

-- +goose Down
ALTER TABLE goals DROP CONSTRAINT goals_dimension_valid;
