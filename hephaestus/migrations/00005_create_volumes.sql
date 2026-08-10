-- +goose Up
CREATE TABLE IF NOT EXISTS volumes (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    instance_id VARCHAR(64),
    size_gb INTEGER NOT NULL,
    volume_type VARCHAR(32) NOT NULL DEFAULT 'gp2',
    state VARCHAR(32) NOT NULL DEFAULT 'available',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_volumes_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_volumes_instance
        FOREIGN KEY (instance_id)
        REFERENCES instances(id)
        ON DELETE SET NULL
);

COMMENT ON TABLE volumes IS 'Attachable block storage volumes for instances';
COMMENT ON COLUMN volumes.state IS 'Volume state: available, in-use, creating, deleted';

CREATE UNIQUE INDEX IF NOT EXISTS idx_volumes_project_name ON volumes (project_id, name);
CREATE INDEX IF NOT EXISTS idx_volumes_instance_id ON volumes (instance_id);
CREATE INDEX IF NOT EXISTS idx_volumes_status ON volumes (status);

-- +goose Down
DROP TABLE IF EXISTS volumes;