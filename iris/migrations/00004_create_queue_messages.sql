-- +goose Up
CREATE TABLE IF NOT EXISTS queue_messages (
    id VARCHAR(64) PRIMARY KEY,
    queue_id VARCHAR(64) NOT NULL,
    body TEXT NOT NULL,
    attributes JSONB,
    state VARCHAR(32) NOT NULL DEFAULT 'pending',
    visible_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_iris_queue_messages_queue
        FOREIGN KEY (queue_id)
        REFERENCES queues(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE queue_messages IS 'Messages sitting in a queue, with visibility (SQS-style delivery)';
COMMENT ON COLUMN queue_messages.state IS 'Message state: pending, available, in_flight, delivered';
COMMENT ON COLUMN queue_messages.visible_at IS 'When this message becomes visible to consumers (visibility timeout)';
COMMENT ON COLUMN queue_messages.expires_at IS 'When this message expires and is purged (retention deadline)';

CREATE INDEX IF NOT EXISTS idx_iris_queue_messages_queue_state ON queue_messages (queue_id, state, visible_at);
CREATE INDEX IF NOT EXISTS idx_iris_queue_messages_expires ON queue_messages (expires_at);

-- +goose Down
DROP TABLE IF EXISTS queue_messages;