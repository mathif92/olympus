-- +goose Up
CREATE TABLE IF NOT EXISTS queues (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    visibility_timeout_sec INTEGER NOT NULL DEFAULT 30,
    message_retention_sec INTEGER NOT NULL DEFAULT 86400,
    state VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_iris_queues_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE queues IS 'Managed message queues (SQS-style)';
COMMENT ON COLUMN queues.visibility_timeout_sec IS 'Seconds a delivered message stays hidden before it becomes visible to other consumers';
COMMENT ON COLUMN queues.message_retention_sec IS 'Seconds an undelivered message is kept before expiry';
COMMENT ON COLUMN queues.state IS 'Queue state: active, deleting, deleted';

CREATE UNIQUE INDEX IF NOT EXISTS idx_iris_queues_project_name ON queues (project_id, name);
CREATE INDEX IF NOT EXISTS idx_iris_queues_state ON queues (state);

-- +goose Down
DROP TABLE IF EXISTS queues;