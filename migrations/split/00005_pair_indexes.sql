-- +goose Up
-- Friend ledger / settle-with-friend look up rows between exactly two users.
CREATE INDEX idx_debts_pair ON debts (from_user, to_user);
CREATE INDEX idx_settlements_pair ON settlements (from_user, to_user);

-- +goose Down
DROP INDEX idx_settlements_pair;
DROP INDEX idx_debts_pair;
