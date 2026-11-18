-- +goose Up
-- Transactional outbox: events are written here in the same tx as business data
-- and relayed to Kafka by internal/platform/outbox.Relay.
CREATE TABLE outbox (
    id         BIGSERIAL PRIMARY KEY,
    topic      TEXT        NOT NULL,
    key        TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE outbox;
