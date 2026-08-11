-- +goose Up
CREATE TABLE IF NOT EXISTS functions (
    id VARCHAR(64) PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    runtime VARCHAR(32) NOT NULL,
    handler VARCHAR(255) NOT NULL DEFAULT 'handler',
    timeout_ms INTEGER NOT NULL DEFAULT 30000,
    memory_mb INTEGER NOT NULL DEFAULT 128,
    cpus DOUBLE PRECISION NOT NULL DEFAULT 0.5,
    env_vars JSONB,
    current_version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_prometheus_functions_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE functions IS 'Serverless functions (AWS Lambda equivalent)';
COMMENT ON COLUMN functions.runtime IS 'Language runtime: python3.12, nodejs20, typescript5, java21, go1.25, rust1.80, dotnet9, ruby3.3';
COMMENT ON COLUMN functions.handler IS 'Entrypoint exposed by the runtime (informational; each runtime fixes its handler file/name)';
COMMENT ON COLUMN functions.timeout_ms IS 'Max execution time before the invocation is killed';
COMMENT ON COLUMN functions.memory_mb IS 'Memory limit applied to the execution container';
COMMENT ON COLUMN functions.cpus IS 'CPU share (vCPU) granted to the execution container';
COMMENT ON COLUMN functions.current_version IS 'Version number of the currently deployed code';

CREATE INDEX IF NOT EXISTS idx_prometheus_functions_account ON functions (account_id);
CREATE INDEX IF NOT EXISTS idx_prometheus_functions_project ON functions (project_id);
CREATE INDEX IF NOT EXISTS idx_prometheus_functions_runtime ON functions (runtime);
CREATE UNIQUE INDEX IF NOT EXISTS idx_prometheus_functions_project_name_unique ON functions (account_id, project_id, name);

-- +goose Down
DROP TABLE IF EXISTS functions;
