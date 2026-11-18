-- +goose Up
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    name          TEXT        NOT NULL,
    currency      CHAR(3)     NOT NULL DEFAULT 'USD',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
