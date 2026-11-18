-- +goose Up
-- group/user ids come from social/auth services: no cross-service foreign keys.

CREATE TABLE receipts (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id       UUID        NOT NULL,
    payer_id       UUID        NOT NULL,
    store          TEXT        NOT NULL DEFAULT '',
    status         SMALLINT    NOT NULL DEFAULT 1,
    subtotal_cents BIGINT      NOT NULL DEFAULT 0,
    tax_cents      BIGINT      NOT NULL DEFAULT 0,
    tip_cents      BIGINT      NOT NULL DEFAULT 0,
    total_cents    BIGINT      NOT NULL DEFAULT 0,
    currency       CHAR(3)     NOT NULL,
    parse_error    TEXT        NOT NULL DEFAULT '',
    ocr_provider   TEXT        NOT NULL DEFAULT '',
    purchased_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_receipts_group ON receipts (group_id, created_at DESC);

CREATE TABLE receipt_files (
    receipt_id   UUID PRIMARY KEY REFERENCES receipts (id) ON DELETE CASCADE,
    content_type TEXT NOT NULL DEFAULT '',
    data         BYTEA,
    raw_text     TEXT NOT NULL DEFAULT ''
);

CREATE TABLE receipt_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    receipt_id  UUID   NOT NULL REFERENCES receipts (id) ON DELETE CASCADE,
    position    INT    NOT NULL,
    name        TEXT   NOT NULL,
    quantity    INT    NOT NULL DEFAULT 1,
    price_cents BIGINT NOT NULL
);
CREATE INDEX idx_receipt_items_receipt ON receipt_items (receipt_id, position);

CREATE TABLE item_claims (
    item_id UUID NOT NULL REFERENCES receipt_items (id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    shares  INT  NOT NULL CHECK (shares > 0),
    PRIMARY KEY (item_id, user_id)
);

-- Debts are stored in the group currency (converted at the purchase-date rate).
CREATE TABLE debts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID        NOT NULL,
    receipt_id UUID REFERENCES receipts (id) ON DELETE CASCADE,
    from_user  UUID        NOT NULL,
    to_user    UUID        NOT NULL,
    amount     BIGINT      NOT NULL CHECK (amount > 0),
    currency   CHAR(3)     NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_debts_group ON debts (group_id);
CREATE INDEX idx_debts_from ON debts (from_user);
CREATE INDEX idx_debts_to ON debts (to_user);

CREATE TABLE settlements (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID        NOT NULL,
    from_user  UUID        NOT NULL,
    to_user    UUID        NOT NULL,
    amount     BIGINT      NOT NULL CHECK (amount > 0),
    currency   CHAR(3)     NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_settlements_group ON settlements (group_id);
CREATE INDEX idx_settlements_from ON settlements (from_user);
CREATE INDEX idx_settlements_to ON settlements (to_user);

-- +goose Down
DROP TABLE settlements, debts, item_claims, receipt_items, receipt_files, receipts;
