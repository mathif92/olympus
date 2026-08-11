-- +goose Up
CREATE TABLE IF NOT EXISTS projects (
    id VARCHAR(64) PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    resource_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    metadata JSONB,

    CONSTRAINT fk_themis_projects_account
        FOREIGN KEY (account_id)
        REFERENCES accounts(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE projects IS 'Projects (namespaces) for each account';
COMMENT ON COLUMN projects.name IS 'Project name (e.g., checkout-service, orders-api)';
COMMENT ON COLUMN projects.resource_count IS 'Number of IAM resources currently stored';

CREATE INDEX IF NOT EXISTS idx_themis_projects_account_id ON projects (account_id);
CREATE INDEX IF NOT EXISTS idx_themis_projects_name ON projects (name);
CREATE INDEX IF NOT EXISTS idx_themis_projects_status ON projects (status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_themis_projects_account_name_unique ON projects (account_id, name);

-- +goose Down
DROP TABLE IF EXISTS projects;
