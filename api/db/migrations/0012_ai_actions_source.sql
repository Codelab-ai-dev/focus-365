-- +goose Up
ALTER TABLE ai_actions ALTER COLUMN message_id DROP NOT NULL;
ALTER TABLE ai_actions
    ADD COLUMN source TEXT NOT NULL DEFAULT 'chat'
        CHECK (source IN ('chat','upload'));
CREATE INDEX idx_ai_actions_upload ON ai_actions (user_id, source, status);

-- +goose Down
DROP INDEX idx_ai_actions_upload;
ALTER TABLE ai_actions DROP COLUMN source;
-- message_id se deja nullable: revertir a NOT NULL fallaría si hay filas de upload.
