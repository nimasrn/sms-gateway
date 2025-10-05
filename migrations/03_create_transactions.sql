-- +goose Up
CREATE TABLE IF NOT EXISTS transactions
(
    id          BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    amount      BIGINT NOT NULL,
    "type"      TEXT   NOT NULL, -- e.g., 'debit' | 'credit'
    message_id  BIGINT REFERENCES messages (id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_transactions_customer ON transactions (customer_id);
CREATE INDEX IF NOT EXISTS idx_transactions_message ON transactions (message_id);

-- +goose Down
DROP TABLE IF EXISTS transactions;
