-- +goose Up
CREATE TABLE IF NOT EXISTS parameters (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(1024) NOT NULL,
    value TEXT,
    data_type VARCHAR(32) NOT NULL DEFAULT 'string',
    description TEXT,
    is_encrypted BOOLEAN NOT NULL DEFAULT FALSE,
    tier VARCHAR(16) NOT NULL DEFAULT 'standard',
    version INTEGER NOT NULL DEFAULT 1,
    key_id VARCHAR(128),
    tags JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_modified_by VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_parameters_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE parameters IS 'Parameter key/value entries with versioning';
COMMENT ON COLUMN parameters.name IS 'Logical parameter path (e.g., /app/db/host)';
COMMENT ON COLUMN parameters.data_type IS 'Parameter type: string, string_list, secure_string';
COMMENT ON COLUMN parameters.is_encrypted IS 'True when value is AES-GCM ciphertext';
COMMENT ON COLUMN parameters.version IS 'Current version, incremented on every update';
COMMENT ON COLUMN parameters.key_id IS 'Reference to the encryption key used for the value';
COMMENT ON COLUMN parameters.tier IS 'Parameter tier: standard, advanced';

CREATE UNIQUE INDEX IF NOT EXISTS idx_parameters_project_name ON parameters (project_id, name);
CREATE INDEX IF NOT EXISTS idx_parameters_name ON parameters (name);
CREATE INDEX IF NOT EXISTS idx_parameters_status ON parameters (status);
CREATE INDEX IF NOT EXISTS idx_parameters_updated_at ON parameters (updated_at);

-- +goose Down
DROP TABLE IF EXISTS parameters;