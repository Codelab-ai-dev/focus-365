-- +goose Up
CREATE TABLE ai_threads (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_threads_user_updated ON ai_threads (user_id, updated_at DESC);

-- Un hilo "General" por cada usuario que ya tenga mensajes.
INSERT INTO ai_threads (user_id, title, created_at, updated_at)
SELECT user_id, 'General', MIN(created_at), MAX(created_at)
FROM ai_messages
GROUP BY user_id;

ALTER TABLE ai_messages ADD COLUMN thread_id UUID REFERENCES ai_threads(id) ON DELETE CASCADE;
UPDATE ai_messages m
SET thread_id = t.id
FROM ai_threads t
WHERE t.user_id = m.user_id;
ALTER TABLE ai_messages ALTER COLUMN thread_id SET NOT NULL;

DROP INDEX IF EXISTS idx_ai_messages_user_created;
CREATE INDEX idx_ai_messages_thread_created ON ai_messages (thread_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_ai_messages_thread_created;
CREATE INDEX idx_ai_messages_user_created ON ai_messages (user_id, created_at);
ALTER TABLE ai_messages DROP COLUMN thread_id;
DROP TABLE ai_threads;
