-- +goose Up
CREATE TABLE IF NOT EXISTS policy_attachments (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    principal_type VARCHAR(16) NOT NULL,
    principal_id VARCHAR(64) NOT NULL,
    policy_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_themis_attachments_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_themis_attachments_policy
        FOREIGN KEY (policy_id)
        REFERENCES policies(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE policy_attachments IS 'Policies attached to users, groups or roles';
COMMENT ON COLUMN policy_attachments.principal_type IS 'user, group or role';
COMMENT ON COLUMN policy_attachments.principal_id IS 'id of the principal row (validated by the service layer)';

CREATE INDEX IF NOT EXISTS idx_themis_attachments_project ON policy_attachments (project_id);
CREATE INDEX IF NOT EXISTS idx_themis_attachments_principal ON policy_attachments (principal_type, principal_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_themis_attachments_uniq ON policy_attachments (principal_type, principal_id, policy_id);

-- +goose Down
DROP TABLE IF EXISTS policy_attachments;
