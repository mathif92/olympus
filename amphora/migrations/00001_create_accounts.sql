-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
    id VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    plan VARCHAR(32) NOT NULL DEFAULT 'free',
    storage_limit_bytes BIGINT NOT NULL DEFAULT 0,
    used_storage_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    metadata JSONB
);

COMMENT ON TABLE accounts IS 'Tenant accounts for multi-tenant storage isolation';
COMMENT ON COLUMN accounts.id IS 'Unique account identifier (UUID or email-based)';
COMMENT ON COLUMN accounts.plan IS 'Storage plan tier: free, pro, enterprise';
COMMENT ON COLUMN accounts.storage_limit_bytes IS 'Quota in bytes for this account';
COMMENT ON COLUMN accounts.used_storage_bytes IS 'Total bytes used across all spaces';

CREATE INDEX IF NOT EXISTS idx_accounts_email ON accounts (email);
CREATE INDEX IF NOT EXISTS idx_accounts_status ON accounts (status);
CREATE INDEX IF NOT EXISTS idx_accounts_plan ON accounts (plan);

-- +goose Down
DROP TABLE IF EXISTS accounts;