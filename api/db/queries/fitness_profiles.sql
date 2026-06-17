-- name: GetFitnessProfile :one
SELECT * FROM fitness_profiles WHERE user_id = $1;

-- name: UpsertFitnessProfile :one
INSERT INTO fitness_profiles (
    user_id, birthdate, sex, height_cm, weight_grams, objective,
    location, level, weekly_days, equipment, limitations, updated_at
) VALUES (
    @user_id, @birthdate, @sex, @height_cm, @weight_grams, @objective,
    @location, @level, @weekly_days, @equipment, @limitations, now()
)
ON CONFLICT (user_id) DO UPDATE SET
    birthdate    = EXCLUDED.birthdate,
    sex          = EXCLUDED.sex,
    height_cm    = EXCLUDED.height_cm,
    weight_grams = EXCLUDED.weight_grams,
    objective    = EXCLUDED.objective,
    location     = EXCLUDED.location,
    level        = EXCLUDED.level,
    weekly_days  = EXCLUDED.weekly_days,
    equipment    = EXCLUDED.equipment,
    limitations  = EXCLUDED.limitations,
    updated_at   = now()
RETURNING *;
