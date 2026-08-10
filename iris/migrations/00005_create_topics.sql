-- +goose Up
CREATE TABLE IF NOT EXISTS topics (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    state VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_iris_topics_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE topics IS 'Managed pub-sub topics (SNS-style)';
COMMENT ON COLUMN topics.state IS 'Topic state: active, deleting, deleted';

CREATE UNIQUE INDEX IF NOT EXISTS idx_iris_topics_project_name ON topics (project_id, name);
CREATE INDEX IF NOT EXISTS idx_iris_topics_state ON topics (state);

-- +goose Down
DROP TABLE IF EXISTS topics;