-- +goose Up
CREATE TABLE IF NOT EXISTS parameter_versions (
    id BIGSERIAL PRIMARY KEY,
    parameter_id VARCHAR(64) NOT NULL,
    version INTEGER NOT NULL,
    value TEXT,
    data_type VARCHAR(32) NOT NULL DEFAULT 'string',
    description TEXT,
    is_encrypted BOOLEAN NOT NULL DEFAULT FALSE,
    key_id VARCHAR(128),
    tags JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_modified_by VARCHAR(255),

    CONSTRAINT fk_parameter_versions_parameter
        FOREIGN KEY (parameter_id)
        REFERENCES parameters(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE parameter_versions IS 'Immutable history of a parameter across updates';

CREATE UNIQUE INDEX IF NOT EXISTS idx_param_versions_uniq ON parameter_versions (parameter_id, version);
CREATE INDEX IF NOT EXISTS idx_param_versions_parameter_id ON parameter_versions (parameter_id);

-- +goose Down
DROP TABLE IF EXISTS parameter_versions;