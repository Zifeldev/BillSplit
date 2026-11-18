-- +goose Up
-- Manual account balances for net worth: cash, cards, savings, credit cards,
-- loans and property. Liabilities store the amount owed as a positive number.
CREATE TABLE accounts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        NOT NULL,
    name          TEXT        NOT NULL,
    kind          SMALLINT    NOT NULL CHECK (kind BETWEEN 1 AND 6),
    currency      CHAR(3)     NOT NULL,
    balance_cents BIGINT      NOT NULL DEFAULT 0 CHECK (balance_cents >= 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_accounts_user ON accounts (user_id, created_at);

-- One net worth point per user per day (home currency at that day's rates).
CREATE TABLE net_worth_snapshots (
    user_id           UUID    NOT NULL,
    day               DATE    NOT NULL,
    currency          CHAR(3) NOT NULL,
    assets_cents      BIGINT  NOT NULL,
    liabilities_cents BIGINT  NOT NULL,
    PRIMARY KEY (user_id, day)
);

-- A goal can follow an investment (e.g. a deposit) instead of a manual amount.
ALTER TABLE savings_goals
    ADD COLUMN investment_id UUID REFERENCES investments (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE savings_goals DROP COLUMN investment_id;
DROP TABLE net_worth_snapshots;
DROP TABLE accounts;
