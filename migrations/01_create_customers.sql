-- +goose Up
CREATE TABLE IF NOT EXISTS customers
(
    id         BIGSERIAL PRIMARY KEY,
    api_key    TEXT    NOT NULL UNIQUE,
    balance    BIGINT  NOT NULL DEFAULT 0 CHECK (balance >= 0)
);

-- +goose Down
DROP TABLE IF EXISTS customers;
