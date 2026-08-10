-- +goose Up
CREATE TABLE IF NOT EXISTS snapshots (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    volume_id VARCHAR(64),
    size_gb INTEGER NOT NULL,
    state VARCHAR(32) NOT NULL DEFAULT 'pending',
    provider_ref VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_snapshots_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_snapshots_volume
        FOREIGN KEY (volume_id)
        REFERENCES volumes(id)
        ON DELETE SET NULL
);

COMMENT ON TABLE snapshots IS 'Point-in-time backups of volumes';
COMMENT ON COLUMN snapshots.state IS 'Snapshot state: pending, completed, failed';

CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_project_name ON snapshots (project_id, name);
CREATE INDEX IF NOT EXISTS idx_snapshots_volume_id ON snapshots (volume_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_state ON snapshots (state);

-- +goose Down
DROP TABLE IF EXISTS snapshots;