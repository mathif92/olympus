-- +goose Up
CREATE TABLE IF NOT EXISTS node_groups (
    id VARCHAR(64) PRIMARY KEY,
    cluster_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    node_size VARCHAR(64) NOT NULL,
    min_size INTEGER NOT NULL DEFAULT 1,
    desired_size INTEGER NOT NULL DEFAULT 1,
    max_size INTEGER NOT NULL DEFAULT 4,
    state VARCHAR(32) NOT NULL DEFAULT 'creating',
    provider_ref VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_orchestra_node_groups_cluster
        FOREIGN KEY (cluster_id)
        REFERENCES clusters(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE node_groups IS 'Worker node groups attached to a managed cluster (autoscaling ranges)';
COMMENT ON COLUMN node_groups.state IS 'Node group state: creating, active, updating, deleting, deleted';
COMMENT ON COLUMN node_groups.provider_ref IS 'Backend identifier returned by the Provisioner';

CREATE UNIQUE INDEX IF NOT EXISTS idx_orchestra_node_groups_cluster_name ON node_groups (cluster_id, name);
CREATE INDEX IF NOT EXISTS idx_orchestra_node_groups_state ON node_groups (state);
CREATE INDEX IF NOT EXISTS idx_orchestra_node_groups_size ON node_groups (node_size);

-- +goose Down
DROP TABLE IF EXISTS node_groups;