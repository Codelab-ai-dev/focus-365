-- +goose Up
ALTER TABLE ai_messages
    DROP CONSTRAINT ai_messages_action_kind_valid,
    ADD CONSTRAINT ai_messages_action_kind_valid CHECK (
        action_kind IS NULL OR action_kind IN (
            'checkin','movimiento','habito','meta',
            'habito_nuevo','meta_nueva','entrenamiento'
        )
    );

-- +goose Down
ALTER TABLE ai_messages
    DROP CONSTRAINT ai_messages_action_kind_valid,
    ADD CONSTRAINT ai_messages_action_kind_valid CHECK (
        action_kind IS NULL OR action_kind IN ('checkin','movimiento','habito','meta')
    );
