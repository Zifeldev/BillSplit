-- +goose Up
-- Investment portfolio: deposits are valued by formula (rate, capitalization,
-- monthly top-ups), market assets by quantity x live quote.
CREATE TABLE investments (
    id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                    UUID        NOT NULL,
    kind                       SMALLINT    NOT NULL CHECK (kind BETWEEN 1 AND 7),
    name                       TEXT        NOT NULL,
    symbol                     TEXT        NOT NULL DEFAULT '',
    currency                   CHAR(3)     NOT NULL,
    quantity                   NUMERIC(28, 10) NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    invested_cents             BIGINT      NOT NULL DEFAULT 0 CHECK (invested_cents >= 0),
    rate_percent               NUMERIC(7, 4) NOT NULL DEFAULT 0,
    capitalization             SMALLINT    NOT NULL DEFAULT 1,
    monthly_contribution_cents BIGINT      NOT NULL DEFAULT 0 CHECK (monthly_contribution_cents >= 0),
    opened_at                  DATE        NOT NULL DEFAULT CURRENT_DATE,
    term_months                INT         NOT NULL DEFAULT 0 CHECK (term_months >= 0),
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_investments_user ON investments (user_id, created_at);

-- +goose Down
DROP TABLE investments;
