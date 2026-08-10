-- +goose Up
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id BIGSERIAL PRIMARY KEY,
    subscriber_id VARCHAR(64) NOT NULL,
    message_id VARCHAR(64),
    payload TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_iris_webhook_deliveries_subscriber
        FOREIGN KEY (subscriber_id)
        REFERENCES topic_subscribers(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE webhook_deliveries IS 'HTTP push deliveries of a published message to a webhook subscriber (with retries)';
COMMENT ON COLUMN webhook_deliveries.status IS 'Delivery status: pending, in_flight, delivered, failed';
COMMENT ON COLUMN webhook_deliveries.attempts IS 'Number of push attempts so far';
COMMENT ON COLUMN webhook_deliveries.last_error IS 'Error text of the most recent failed attempt';

CREATE INDEX IF NOT EXISTS idx_iris_webhook_deliveries_status ON webhook_deliveries (status);
CREATE INDEX IF NOT EXISTS idx_iris_webhook_deliveries_subscriber ON webhook_deliveries (subscriber_id);

-- +goose Down
DROP TABLE IF EXISTS webhook_deliveries;