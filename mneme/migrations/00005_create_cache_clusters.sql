-- +goose Up
CREATE TABLE IF NOT EXISTS cache_clusters (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    engine VARCHAR(32) NOT NULL,
    engine_version VARCHAR(32) NOT NULL,
    node_type VARCHAR(64) NOT NULL,
    num_nodes INTEGER NOT NULL DEFAULT 1,
    state VARCHAR(32) NOT NULL DEFAULT 'pending',
    endpoint VARCHAR(512),
    provider_ref VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_mneme_cache_clusters_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE cache_clusters IS 'Managed in-memory cache clusters (engine, node type, endpoint)';
COMMENT ON COLUMN cache_clusters.state IS 'Cluster state: pending, creating, active, deleting, deleted';
COMMENT ON COLUMN cache_clusters.endpoint IS 'Connection endpoint (host:port) of the provisioned cache';
COMMENT ON COLUMN cache_clusters.provider_ref IS 'Backend identifier returned by the Provisioner (e.g. container id)';

CREATE UNIQUE INDEX IF NOT EXISTS idx_mneme_cache_clusters_project_name ON cache_clusters (project_id, name);
CREATE INDEX IF NOT EXISTS idx_mneme_cache_clusters_state ON cache_clusters (state);
CREATE INDEX IF NOT EXISTS idx_mneme_cache_clusters_engine ON cache_clusters (engine);
CREATE INDEX IF NOT EXISTS idx_mneme_cache_clusters_created_at ON cache_clusters (created_at);

-- +goose Down
DROP TABLE IF EXISTS cache_clusters;