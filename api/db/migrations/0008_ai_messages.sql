-- +goose Up
CREATE TABLE ai_messages (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,            -- 'user' | 'assistant'
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ai_messages_role_valid CHECK (role IN ('user','assistant'))
);
CREATE INDEX idx_ai_messages_user_created ON ai_messages (user_id, created_at);

-- +goose Down
DROP TABLE ai_messages;
