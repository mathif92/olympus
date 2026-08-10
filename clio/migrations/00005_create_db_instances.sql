-- +goose Up
CREATE TABLE IF NOT EXISTS db_instances (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    engine VARCHAR(32) NOT NULL,
    engine_version VARCHAR(32) NOT NULL,
    size VARCHAR(64) NOT NULL,
    allocated_storage_gb INTEGER NOT NULL DEFAULT 20,
    state VARCHAR(32) NOT NULL DEFAULT 'pending',
    endpoint VARCHAR(512),
    master_username VARCHAR(64),
    provider_ref VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_clio_db_instances_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE db_instances IS 'Managed relational database instances (engine, size, endpoint)';
COMMENT ON COLUMN db_instances.state IS 'Instance state: pending, creating, active, stopping, stopped, starting, deleting, deleted';
COMMENT ON COLUMN db_instances.endpoint IS 'Connection endpoint (host:port) of the provisioned database';
COMMENT ON COLUMN db_instances.provider_ref IS 'Backend identifier returned by the Provisioner (e.g. container id)';

CREATE UNIQUE INDEX IF NOT EXISTS idx_clio_db_instances_project_name ON db_instances (project_id, name);
CREATE INDEX IF NOT EXISTS idx_clio_db_instances_state ON db_instances (state);
CREATE INDEX IF NOT EXISTS idx_clio_db_instances_engine ON db_instances (engine);
CREATE INDEX IF NOT EXISTS idx_clio_db_instances_created_at ON db_instances (created_at);

-- +goose Down
DROP TABLE IF EXISTS db_instances;