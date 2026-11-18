-- +goose Up
-- user ids come from the auth service: no cross-service foreign keys.

-- Expo push tokens. A token belongs to exactly one user (the last one who
-- registered it on that device).
CREATE TABLE devices (
    token        TEXT PRIMARY KEY,
    user_id      UUID        NOT NULL,
    platform     TEXT        NOT NULL CHECK (platform IN ('ios', 'android', 'web')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_devices_user ON devices (user_id);

-- In-app inbox: the source of truth (push is best effort).
CREATE TABLE notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    type       TEXT        NOT NULL,
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL DEFAULT '',
    data       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    read       BOOLEAN     NOT NULL DEFAULT false,
    -- Idempotency for at-least-once Kafka delivery: one row per (message, user).
    dedupe_key TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_user ON notifications (user_id, created_at DESC);
CREATE INDEX idx_notifications_unread ON notifications (user_id) WHERE NOT read;

-- +goose Down
DROP TABLE notifications;
DROP TABLE devices;
