-- +goose Up
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    project_id VARCHAR(64),
    principal VARCHAR(1024),
    resource VARCHAR(1024),
    operation VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'success',
    ip_address INET,
    user_agent TEXT,
    request_id VARCHAR(128),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_themis_audit_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE SET NULL
);

COMMENT ON TABLE audit_logs IS 'Audit trail for IAM operations';
COMMENT ON COLUMN audit_logs.operation IS 'Operation type: create, get, update, delete, list, attach, detach, authorize, token';
COMMENT ON COLUMN audit_logs.principal IS 'User or role performing the operation';

CREATE INDEX IF NOT EXISTS idx_themis_audit_account ON audit_logs (account_id);
CREATE INDEX IF NOT EXISTS idx_themis_audit_project ON audit_logs (project_id);
CREATE INDEX IF NOT EXISTS idx_themis_audit_operation ON audit_logs (operation);
CREATE INDEX IF NOT EXISTS idx_themis_audit_created_at ON audit_logs (created_at);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
