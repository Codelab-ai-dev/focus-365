-- +goose Up
ALTER TABLE ai_messages
    ADD COLUMN action_kind    TEXT,
    ADD COLUMN action_payload JSONB,
    ADD COLUMN action_status  TEXT,
    ADD CONSTRAINT ai_messages_action_kind_valid CHECK (
        action_kind IS NULL OR action_kind IN ('checkin','movimiento','habito','meta')
    ),
    ADD CONSTRAINT ai_messages_action_status_valid CHECK (
        action_status IS NULL OR action_status IN ('proposed','done','cancelled')
    ),
    ADD CONSTRAINT ai_messages_action_consistente CHECK (
        (action_kind IS NULL AND action_payload IS NULL AND action_status IS NULL)
        OR (action_kind IS NOT NULL AND action_payload IS NOT NULL AND action_status IS NOT NULL)
    );

-- +goose Down
ALTER TABLE ai_messages
    DROP CONSTRAINT ai_messages_action_consistente,
    DROP CONSTRAINT ai_messages_action_status_valid,
    DROP CONSTRAINT ai_messages_action_kind_valid,
    DROP COLUMN action_status,
    DROP COLUMN action_payload,
    DROP COLUMN action_kind;
