-- +goose Up
-- ListStatementImports: newest imports of a user.
CREATE INDEX idx_statement_imports_user_time ON statement_imports (user_id, created_at DESC);
-- Recurring reminders job: users with recent expenses (scanned by time, batched by user_id).
CREATE INDEX idx_transactions_time_user ON transactions (occurred_at, user_id) WHERE amount_cents < 0;

-- +goose Down
DROP INDEX idx_transactions_time_user;
DROP INDEX idx_statement_imports_user_time;
