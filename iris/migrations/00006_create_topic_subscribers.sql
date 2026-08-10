-- +goose Up
CREATE TABLE IF NOT EXISTS topic_subscribers (
    id VARCHAR(64) PRIMARY KEY,
    topic_id VARCHAR(64) NOT NULL,
    kind VARCHAR(32) NOT NULL DEFAULT 'queue',
    queue_id VARCHAR(64),
    webhook_url VARCHAR(2048),
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_iris_topic_subscribers_topic
        FOREIGN KEY (topic_id)
        REFERENCES topics(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_iris_topic_subscribers_queue
        FOREIGN KEY (queue_id)
        REFERENCES queues(id)
        ON DELETE CASCADE,
    CONSTRAINT chk_iris_topic_subscribers_kind
        CHECK ((kind = 'queue' AND queue_id IS NOT NULL AND webhook_url IS NULL)
            OR (kind = 'webhook' AND queue_id IS NULL AND webhook_url IS NOT NULL))
);

COMMENT ON TABLE topic_subscribers IS 'Subscribers attached to a topic: queues (fan-out target) or webhooks (HTTP push target)';
COMMENT ON COLUMN topic_subscribers.kind IS 'Subscriber kind: queue (fan-out into an SQS-style queue) or webhook (HTTP POST push)';
COMMENT ON COLUMN topic_subscribers.queue_id IS 'Target queue when kind = queue';
COMMENT ON COLUMN topic_subscribers.webhook_url IS 'Target endpoint when kind = webhook';

CREATE UNIQUE INDEX IF NOT EXISTS idx_iris_topic_subscribers_queue ON topic_subscribers (topic_id, queue_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_iris_topic_subscribers_webhook ON topic_subscribers (topic_id, webhook_url);
CREATE INDEX IF NOT EXISTS idx_iris_topic_subscribers_kind ON topic_subscribers (kind);

-- +goose Down
DROP TABLE IF EXISTS topic_subscribers;