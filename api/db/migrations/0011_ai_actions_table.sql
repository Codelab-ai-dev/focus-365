-- +goose Up
CREATE TABLE ai_actions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES ai_messages(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    position   INT  NOT NULL DEFAULT 0,
    kind       TEXT NOT NULL CHECK (kind IN (
        'checkin','movimiento','habito','meta',
        'habito_nuevo','meta_nueva','entrenamiento')),
    payload    JSONB NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('proposed','done','cancelled','undone')),
    result     JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_actions_message ON ai_actions (message_id, position);
CREATE INDEX idx_ai_actions_user ON ai_actions (user_id);

INSERT INTO ai_actions (message_id, user_id, position, kind, payload, status, created_at)
SELECT id, user_id, 0, action_kind, action_payload, action_status, created_at
FROM ai_messages
WHERE action_kind IS NOT NULL;

ALTER TABLE ai_messages
    DROP CONSTRAINT ai_messages_action_consistente,
    DROP CONSTRAINT ai_messages_action_status_valid,
    DROP CONSTRAINT ai_messages_action_kind_valid,
    DROP COLUMN action_status,
    DROP COLUMN action_payload,
    DROP COLUMN action_kind;

-- +goose Down
ALTER TABLE ai_messages
    ADD COLUMN action_kind    TEXT,
    ADD COLUMN action_payload JSONB,
    ADD COLUMN action_status  TEXT,
    ADD CONSTRAINT ai_messages_action_kind_valid CHECK (
        action_kind IS NULL OR action_kind IN (
            'checkin','movimiento','habito','meta',
            'habito_nuevo','meta_nueva','entrenamiento')
    ),
    ADD CONSTRAINT ai_messages_action_status_valid CHECK (
        action_status IS NULL OR action_status IN ('proposed','done','cancelled')
    ),
    ADD CONSTRAINT ai_messages_action_consistente CHECK (
        (action_kind IS NULL AND action_payload IS NULL AND action_status IS NULL)
        OR (action_kind IS NOT NULL AND action_payload IS NOT NULL AND action_status IS NOT NULL)
    );

UPDATE ai_messages m SET
    action_kind = a.kind,
    action_payload = a.payload,
    -- undone no existe en el modelo viejo: degrada a done
    action_status = CASE WHEN a.status = 'undone' THEN 'done' ELSE a.status END
FROM ai_actions a
WHERE a.message_id = m.id AND a.position = 0;

DROP TABLE ai_actions;
