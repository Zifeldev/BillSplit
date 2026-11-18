-- +goose Up
-- kind: 1 needs, 2 wants, 3 savings, 4 income (50/30/20 rule buckets)
CREATE TABLE categories (
    id   TEXT PRIMARY KEY,
    name TEXT     NOT NULL,
    kind SMALLINT NOT NULL,
    icon TEXT     NOT NULL DEFAULT ''
);

-- user_id NULL = global default rule
CREATE TABLE category_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID,
    pattern     TEXT NOT NULL, -- lower-case substring of merchant/description
    category_id TEXT NOT NULL REFERENCES categories (id),
    UNIQUE NULLS NOT DISTINCT (user_id, pattern)
);

-- Amounts are kept in the original currency; reports convert to the home currency.
CREATE TABLE transactions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL,
    amount_cents BIGINT      NOT NULL,
    currency     CHAR(3)     NOT NULL,
    description  TEXT        NOT NULL DEFAULT '',
    merchant     TEXT        NOT NULL DEFAULT '',
    category_id  TEXT        NOT NULL REFERENCES categories (id),
    source       TEXT        NOT NULL DEFAULT 'manual',
    dedup_key    TEXT,
    occurred_at  TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, dedup_key)
);
CREATE INDEX idx_transactions_user_time ON transactions (user_id, occurred_at DESC);

CREATE TABLE statement_imports (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'pending',
    format     SMALLINT    NOT NULL DEFAULT 0,
    bank       TEXT        NOT NULL DEFAULT '',
    currency   CHAR(3)     NOT NULL,
    file       BYTEA       NOT NULL,
    imported   INT         NOT NULL DEFAULT 0,
    duplicates INT         NOT NULL DEFAULT 0,
    error      TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Budgets and goals are in the user's home currency.
CREATE TABLE budgets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID   NOT NULL,
    category_id TEXT   NOT NULL REFERENCES categories (id),
    limit_cents BIGINT NOT NULL CHECK (limit_cents >= 0),
    UNIQUE (user_id, category_id)
);

CREATE TABLE savings_goals (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL,
    name         TEXT        NOT NULL,
    target_cents BIGINT      NOT NULL CHECK (target_cents > 0),
    saved_cents  BIGINT      NOT NULL DEFAULT 0,
    deadline     TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO categories (id, name, kind, icon) VALUES
    ('groceries',     'Groceries',      1, '🛒'),
    ('housing',       'Housing & Rent', 1, '🏠'),
    ('utilities',     'Utilities',      1, '💡'),
    ('transport',     'Transport',      1, '🚗'),
    ('health',        'Health',         1, '💊'),
    ('insurance',     'Insurance',      1, '🛡'),
    ('dining',        'Restaurants',    2, '🍔'),
    ('entertainment', 'Entertainment',  2, '🎬'),
    ('shopping',      'Shopping',       2, '🛍'),
    ('subscriptions', 'Subscriptions',  2, '📺'),
    ('travel',        'Travel',         2, '✈️'),
    ('other',         'Other',          2, '❔'),
    ('savings',       'Savings',        3, '🏦'),
    ('investments',   'Investments',    3, '📈'),
    ('income',        'Income',         4, '💰'),
    ('transfer',      'Transfers',      2, '🔁');

INSERT INTO category_rules (user_id, pattern, category_id) VALUES
    (NULL, 'walmart', 'groceries'), (NULL, 'wal-mart', 'groceries'), (NULL, 'kroger', 'groceries'),
    (NULL, 'costco', 'groceries'), (NULL, 'whole foods', 'groceries'), (NULL, 'trader joe', 'groceries'),
    (NULL, 'aldi', 'groceries'), (NULL, 'safeway', 'groceries'), (NULL, 'publix', 'groceries'),
    (NULL, 'lidl', 'groceries'), (NULL, 'tesco', 'groceries'),
    (NULL, 'target', 'shopping'), (NULL, 'amazon', 'shopping'), (NULL, 'best buy', 'shopping'), (NULL, 'ikea', 'shopping'),
    (NULL, 'uber eats', 'dining'), (NULL, 'doordash', 'dining'), (NULL, 'mcdonald', 'dining'),
    (NULL, 'starbucks', 'dining'), (NULL, 'chipotle', 'dining'), (NULL, 'grubhub', 'dining'),
    (NULL, 'uber', 'transport'), (NULL, 'lyft', 'transport'), (NULL, 'shell', 'transport'),
    (NULL, 'chevron', 'transport'), (NULL, 'exxon', 'transport'), (NULL, 'parking', 'transport'),
    (NULL, 'netflix', 'subscriptions'), (NULL, 'spotify', 'subscriptions'), (NULL, 'hulu', 'subscriptions'),
    (NULL, 'apple.com', 'subscriptions'), (NULL, 'youtube', 'subscriptions'), (NULL, 'chatgpt', 'subscriptions'),
    (NULL, 'cvs', 'health'), (NULL, 'walgreens', 'health'), (NULL, 'pharmacy', 'health'),
    (NULL, 'geico', 'insurance'), (NULL, 'state farm', 'insurance'), (NULL, 'progressive', 'insurance'),
    (NULL, 'comcast', 'utilities'), (NULL, 'verizon', 'utilities'), (NULL, 'at&t', 'utilities'),
    (NULL, 'electric', 'utilities'), (NULL, 'rent', 'housing'), (NULL, 'mortgage', 'housing'),
    (NULL, 'airbnb', 'travel'), (NULL, 'airlines', 'travel'), (NULL, 'hotel', 'travel'), (NULL, 'booking.com', 'travel'),
    (NULL, 'payroll', 'income'), (NULL, 'direct dep', 'income'), (NULL, 'salary', 'income'),
    (NULL, 'venmo', 'transfer'), (NULL, 'zelle', 'transfer'), (NULL, 'robinhood', 'investments'),
    (NULL, 'vanguard', 'investments')
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE savings_goals, budgets, statement_imports, transactions, category_rules, categories;
