-- +goose Up
CREATE TABLE IF NOT EXISTS policies (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    document JSONB NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_themis_policies_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE policies IS 'Named JSON policy documents attachable to principals';
COMMENT ON COLUMN policies.document IS 'IAM-style policy document: { "Version", "Statement": [...] }';

CREATE INDEX IF NOT EXISTS idx_themis_policies_project ON policies (project_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_themis_policies_project_name_unique ON policies (project_id, name);

-- +goose Down
DROP TABLE IF EXISTS policies;
