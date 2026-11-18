-- +goose Up
-- Payment handles for "pay back" deep links (Venmo, PayPal.me, Zelle). Empty = not set.
ALTER TABLE users
    ADD COLUMN venmo  TEXT NOT NULL DEFAULT '',
    ADD COLUMN paypal TEXT NOT NULL DEFAULT '',
    ADD COLUMN zelle  TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN venmo, DROP COLUMN paypal, DROP COLUMN zelle;
