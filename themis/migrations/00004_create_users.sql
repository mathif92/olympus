-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    path VARCHAR(255) NOT NULL DEFAULT '/',
    tags JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_themis_users_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE users IS 'IAM users that can hold access keys';
COMMENT ON COLUMN users.path IS 'Path like /service-accounts/ for grouping';

CREATE INDEX IF NOT EXISTS idx_themis_users_project ON users (project_id);
CREATE INDEX IF NOT EXISTS idx_themis_users_status ON users (status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_themis_users_project_name_unique ON users (project_id, name);

-- +goose Down
DROP TABLE IF EXISTS users;
