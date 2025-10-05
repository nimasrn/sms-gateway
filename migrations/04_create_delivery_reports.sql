-- +goose Up
CREATE TABLE IF NOT EXISTS delivery_reports
(
    id           BIGSERIAL PRIMARY KEY,
    message_id   BIGINT NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    status       TEXT   NOT NULL,
    delivered_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_delivery_reports_message ON delivery_reports (message_id);
CREATE INDEX IF NOT EXISTS idx_delivery_reports_status ON delivery_reports (status);

-- +goose Down
DROP TABLE IF EXISTS delivery_reports;
