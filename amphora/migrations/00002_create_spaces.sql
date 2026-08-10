-- +goose Up
CREATE TABLE IF NOT EXISTS spaces (
    id VARCHAR(64) PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    max_size_bytes BIGINT NOT NULL DEFAULT 0,
    current_size_bytes BIGINT NOT NULL DEFAULT 0,
    object_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    metadata JSONB,

    CONSTRAINT fk_spaces_account
        FOREIGN KEY (account_id)
        REFERENCES accounts(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE spaces IS 'Spaces (buckets) for each account';
COMMENT ON COLUMN spaces.account_id IS 'Reference to parent account';
COMMENT ON COLUMN spaces.name IS 'Space name (e.g., photos, documents)';
COMMENT ON COLUMN spaces.max_size_bytes IS 'Quota limit for this specific space';
COMMENT ON COLUMN spaces.current_size_bytes IS 'Total bytes used in this space';

CREATE INDEX IF NOT EXISTS idx_spaces_account_id ON spaces (account_id);
CREATE INDEX IF NOT EXISTS idx_spaces_name ON spaces (name);
CREATE INDEX IF NOT EXISTS idx_spaces_status ON spaces (status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_spaces_account_name_unique ON spaces (account_id, name);

-- +goose Down
DROP TABLE IF EXISTS spaces;