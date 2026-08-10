-- +goose Up
CREATE TABLE IF NOT EXISTS clusters (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    kubernetes_version VARCHAR(16) NOT NULL,
    region VARCHAR(32) NOT NULL DEFAULT 'eu-west-1',
    state VARCHAR(32) NOT NULL DEFAULT 'pending',
    endpoint VARCHAR(512),
    ca_cert TEXT,
    kubeconfig TEXT,
    provider_ref VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_orchestra_clusters_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE clusters IS 'Managed Kubernetes clusters (control plane + networking)';
COMMENT ON COLUMN clusters.state IS 'Cluster state: pending, creating, active, deleting, deleted';
COMMENT ON COLUMN clusters.endpoint IS 'Kubernetes API server endpoint of the cluster';
COMMENT ON COLUMN clusters.ca_cert IS 'Base64-encoded API server CA certificate (for kubeconfig)';
COMMENT ON COLUMN clusters.kubeconfig IS 'Rendered kubeconfig for the cluster (returned on request)';
COMMENT ON COLUMN clusters.provider_ref IS 'Backend identifier returned by the Provisioner (e.g. registry id)';

CREATE UNIQUE INDEX IF NOT EXISTS idx_orchestra_clusters_project_name ON clusters (project_id, name);
CREATE INDEX IF NOT EXISTS idx_orchestra_clusters_state ON clusters (state);
CREATE INDEX IF NOT EXISTS idx_orchestra_clusters_version ON clusters (kubernetes_version);
CREATE INDEX IF NOT EXISTS idx_orchestra_clusters_created_at ON clusters (created_at);

-- +goose Down
DROP TABLE IF EXISTS clusters;