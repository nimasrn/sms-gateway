-- +goose Up
INSERT INTO customers (api_key, balance)
VALUES ('default-api-key', 1000000)
ON CONFLICT (api_key) DO NOTHING;

-- +goose Down
DELETE FROM customers WHERE api_key = 'default-api-key';
