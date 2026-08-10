-- +goose Up
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    project_id VARCHAR(64),
    entity VARCHAR(64),
    operation VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'success',
    ip_address INET,
    user_agent TEXT,
    request_id VARCHAR(128),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_iris_audit_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE SET NULL
);

COMMENT ON TABLE audit_logs IS 'Audit trail for all Iris operations';
COMMENT ON COLUMN audit_logs.operation IS 'Operation type: create, list, delete, send, pull, publish, subscribe, unsubscribe';
COMMENT ON COLUMN audit_logs.entity IS 'Affected entity type: queue, message, topic, subscriber, project';

CREATE INDEX IF NOT EXISTS idx_iris_audit_account ON audit_logs (account_id);
CREATE INDEX IF NOT EXISTS idx_iris_audit_project ON audit_logs (project_id);
CREATE INDEX IF NOT EXISTS idx_iris_audit_operation ON audit_logs (operation);
CREATE INDEX IF NOT EXISTS idx_iris_audit_created_at ON audit_logs (created_at);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;