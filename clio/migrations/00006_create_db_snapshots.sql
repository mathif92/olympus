-- +goose Up
CREATE TABLE IF NOT EXISTS db_snapshots (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    instance_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    size_gb INTEGER NOT NULL DEFAULT 0,
    state VARCHAR(32) NOT NULL DEFAULT 'creating',
    provider_ref VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_clio_db_snapshots_instance
        FOREIGN KEY (instance_id)
        REFERENCES db_instances(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE db_snapshots IS 'Point-in-time snapshots (backups) of a managed database instance';
COMMENT ON COLUMN db_snapshots.state IS 'Snapshot state: creating, available, deleting, deleted, failed';
COMMENT ON COLUMN db_snapshots.provider_ref IS 'Backend identifier returned by the Provisioner';

CREATE UNIQUE INDEX IF NOT EXISTS idx_clio_db_snapshots_instance_name ON db_snapshots (instance_id, name);
CREATE INDEX IF NOT EXISTS idx_clio_db_snapshots_state ON db_snapshots (state);
CREATE INDEX IF NOT EXISTS idx_clio_db_snapshots_created_at ON db_snapshots (created_at);

-- +goose Down
DROP TABLE IF EXISTS db_snapshots;