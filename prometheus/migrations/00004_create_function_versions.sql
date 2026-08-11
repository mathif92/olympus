-- +goose Up
CREATE TABLE IF NOT EXISTS function_versions (
    id VARCHAR(64) PRIMARY KEY,
    function_id VARCHAR(64) NOT NULL,
    version INTEGER NOT NULL,
    code BYTEA NOT NULL,
    code_sha256 VARCHAR(64) NOT NULL,
    code_size INTEGER NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_prometheus_versions_function
        FOREIGN KEY (function_id)
        REFERENCES functions(id)
        ON DELETE CASCADE,
    CONSTRAINT uq_prometheus_versions_function_version UNIQUE (function_id, version)
);

COMMENT ON TABLE function_versions IS 'Immutable snapshots of deployed function code (zip archives)';
COMMENT ON COLUMN function_versions.code IS 'Source archive uploaded for this version';
COMMENT ON COLUMN function_versions.is_active IS 'The version served by invoke calls';

CREATE INDEX IF NOT EXISTS idx_prometheus_versions_function ON function_versions (function_id);
CREATE INDEX IF NOT EXISTS idx_prometheus_versions_active ON function_versions (function_id, is_active);

-- +goose Down
DROP TABLE IF EXISTS function_versions;
