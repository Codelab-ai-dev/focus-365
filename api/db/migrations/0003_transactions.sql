-- +goose Up
CREATE TABLE transactions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT NOT NULL CHECK (type IN ('income','expense','transfer')),
    amount      BIGINT NOT NULL CHECK (amount >= 0),  -- centavos (MXN)
    occurred_on DATE NOT NULL,
    cycle       DATE NOT NULL,            -- primer día del mes financiero
    category    TEXT NOT NULL DEFAULT '',
    remark      TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','import')),
    external_id TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_transactions_user_cycle
    ON transactions (user_id, cycle DESC, occurred_on DESC);
CREATE UNIQUE INDEX uq_transactions_user_external
    ON transactions (user_id, external_id) WHERE external_id IS NOT NULL;

-- +goose Down
DROP TABLE transactions;
