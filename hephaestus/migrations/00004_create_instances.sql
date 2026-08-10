-- +goose Up
CREATE TABLE IF NOT EXISTS instances (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    instance_type VARCHAR(64) NOT NULL,
    image_id VARCHAR(128) NOT NULL DEFAULT 'olympus-ami-linux-2',
    state VARCHAR(32) NOT NULL DEFAULT 'pending',
    private_ip VARCHAR(45),
    public_ip VARCHAR(45),
    key_pair_name VARCHAR(255),
    provider_ref VARCHAR(255),
    launched_by VARCHAR(255),
    launched_at TIMESTAMP,
    terminated_at TIMESTAMP,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_instances_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE instances IS 'Virtual compute instances (state machine controlled by Provisioner)';
COMMENT ON COLUMN instances.state IS 'Instance state: pending, running, stopped, terminated';
COMMENT ON COLUMN instances.provider_ref IS 'Backend identifier returned by the Provisioner (e.g. hypervisor id)';

CREATE UNIQUE INDEX IF NOT EXISTS idx_instances_project_name ON instances (project_id, name);
CREATE INDEX IF NOT EXISTS idx_instances_state ON instances (state);
CREATE INDEX IF NOT EXISTS idx_instances_type ON instances (instance_type);
CREATE INDEX IF NOT EXISTS idx_instances_created_at ON instances (created_at);

-- +goose Down
DROP TABLE IF EXISTS instances;