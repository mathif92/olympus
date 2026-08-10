-- +goose Up
CREATE TABLE IF NOT EXISTS cache_snapshots (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    cluster_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    size_mb INTEGER NOT NULL DEFAULT 0,
    state VARCHAR(32) NOT NULL DEFAULT 'creating',
    provider_ref VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_mneme_cache_snapshots_cluster
        FOREIGN KEY (cluster_id)
        REFERENCES cache_clusters(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE cache_snapshots IS 'Point-in-time snapshots (backups) of a managed cache cluster';
COMMENT ON COLUMN cache_snapshots.state IS 'Snapshot state: creating, available, deleting, deleted, failed';
COMMENT ON COLUMN cache_snapshots.provider_ref IS 'Backend identifier returned by the Provisioner';

CREATE UNIQUE INDEX IF NOT EXISTS idx_mneme_cache_snapshots_cluster_name ON cache_snapshots (cluster_id, name);
CREATE INDEX IF NOT EXISTS idx_mneme_cache_snapshots_state ON cache_snapshots (state);
CREATE INDEX IF NOT EXISTS idx_mneme_cache_snapshots_created_at ON cache_snapshots (created_at);

-- +goose Down
DROP TABLE IF EXISTS cache_snapshots;