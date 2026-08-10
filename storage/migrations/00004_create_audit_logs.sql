-- +goose Up
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    space_id VARCHAR(64),
    object_key VARCHAR(1024),
    account_id VARCHAR(64) NOT NULL,
    operation VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'success',
    ip_address INET,
    user_agent TEXT,
    request_id VARCHAR(128),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_audit_space
        FOREIGN KEY (space_id)
        REFERENCES spaces(id)
        ON DELETE SET NULL
);

COMMENT ON TABLE audit_logs IS 'Audit trail for all storage operations';
COMMENT ON COLUMN audit_logs.operation IS 'Operation type: upload, download, delete, list, metadata_read';
COMMENT ON COLUMN audit_logs.status IS 'Operation result: success, failed, throttled';

CREATE INDEX IF NOT EXISTS idx_audit_account ON audit_logs (account_id);
CREATE INDEX IF NOT EXISTS idx_audit_space ON audit_logs (space_id);
CREATE INDEX IF NOT EXISTS idx_audit_operation ON audit_logs (operation);
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_logs (created_at);
CREATE INDEX IF NOT EXISTS idx_audit_status ON audit_logs (status);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;