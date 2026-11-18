-- +goose Up
-- raw_name: text as printed on the receipt ("GV HH 320Z"); name holds the readable
-- version ("Great Value Half & Half") found by product code lookup.
ALTER TABLE receipt_items
    ADD COLUMN raw_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN code     TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE receipt_items DROP COLUMN raw_name, DROP COLUMN code;
