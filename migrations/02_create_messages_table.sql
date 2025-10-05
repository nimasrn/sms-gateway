-- +goose Up
CREATE TABLE IF NOT EXISTS messages
(
    id          BIGSERIAL PRIMARY KEY,
    customer_id BIGINT       NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    mobile      VARCHAR(32)  NOT NULL,
    content     VARCHAR(255) NOT NULL,
    priority    VARCHAR(32)  NOT NULL DEFAULT 'normal',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_messages_customer_created_at
    ON messages (customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_mobile ON messages (mobile);

-- +goose Down
DROP TABLE IF EXISTS messages;
