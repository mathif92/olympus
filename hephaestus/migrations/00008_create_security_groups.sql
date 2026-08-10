-- +goose Up
CREATE TABLE IF NOT EXISTS security_groups (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    rules JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_security_groups_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE security_groups IS 'Firewall rulesets scoped to instances in a project';
COMMENT ON COLUMN security_groups.rules IS 'JSON array of ingress rules, e.g. [{"port":22,"cidr":"0.0.0.0/0"}]';

CREATE UNIQUE INDEX IF NOT EXISTS idx_security_groups_project_name ON security_groups (project_id, name);
CREATE INDEX IF NOT EXISTS idx_security_groups_name ON security_groups (name);

-- +goose Down
DROP TABLE IF EXISTS security_groups;