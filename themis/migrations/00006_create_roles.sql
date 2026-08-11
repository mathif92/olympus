-- +goose Up
CREATE TABLE IF NOT EXISTS roles (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    tags JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_themis_roles_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE roles IS 'IAM roles; roles hold policies and can be assumed via tokens';

CREATE INDEX IF NOT EXISTS idx_themis_roles_project ON roles (project_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_themis_roles_project_name_unique ON roles (project_id, name);

-- +goose Down
DROP TABLE IF EXISTS roles;
