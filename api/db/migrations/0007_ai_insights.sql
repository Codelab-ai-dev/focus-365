-- +goose Up
CREATE TABLE ai_insights (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    insight_date     DATE NOT NULL,                     -- día del insight (cache key)
    kind             TEXT NOT NULL DEFAULT 'proactive', -- proactive|on_demand (futuro)
    content          TEXT NOT NULL,                     -- el párrafo generado
    context_snapshot JSONB NOT NULL,                    -- snapshot enviado a Groq (auditoría)
    generated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ai_insights_kind_valid  CHECK (kind IN ('proactive','on_demand')),
    CONSTRAINT ai_insights_unique_day  UNIQUE (user_id, insight_date, kind)
);
CREATE INDEX idx_ai_insights_user_date ON ai_insights (user_id, insight_date DESC);

-- +goose Down
DROP TABLE ai_insights;
