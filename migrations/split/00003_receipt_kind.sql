-- +goose Up
-- kind: 1 = itemized receipt (photo or manual), 2 = single-amount expense
-- (rent, taxi...). For expenses `store` holds the description.
ALTER TABLE receipts ADD COLUMN kind SMALLINT NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE receipts DROP COLUMN kind;
