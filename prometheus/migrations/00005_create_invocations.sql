-- +goose Up
CREATE TABLE IF NOT EXISTS invocations (
    id VARCHAR(64) PRIMARY KEY,
    function_id VARCHAR(64) NOT NULL,
    version INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    request JSONB,
    response TEXT,
    error TEXT,
    exit_code INTEGER,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    invoked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_prometheus_invocations_function
        FOREIGN KEY (function_id)
        REFERENCES functions(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE invocations IS 'One row per handler execution';
COMMENT ON COLUMN invocations.status IS 'success, error, timeout or oom';
COMMENT ON COLUMN invocations.request IS 'Event payload delivered to the handler (JSON)';
COMMENT ON COLUMN invocations.response IS 'Handler stdout (JSON result)';

CREATE INDEX IF NOT EXISTS idx_prometheus_invocations_function ON invocations (function_id);
CREATE INDEX IF NOT EXISTS idx_prometheus_invocations_status ON invocations (status);
CREATE INDEX IF NOT EXISTS idx_prometheus_invocations_invoked_at ON invocations (invoked_at);

-- +goose Down
DROP TABLE IF EXISTS invocations;
