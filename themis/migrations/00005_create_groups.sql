-- +goose Up
CREATE TABLE IF NOT EXISTS groups (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    tags JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_themis_groups_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE groups IS 'IAM groups; users are members and inherit attached policies';

CREATE INDEX IF NOT EXISTS idx_themis_groups_project ON groups (project_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_themis_groups_project_name_unique ON groups (project_id, name);

-- +goose Down
DROP TABLE IF EXISTS groups;
