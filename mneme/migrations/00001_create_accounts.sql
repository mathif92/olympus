-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
    id VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    plan VARCHAR(32) NOT NULL DEFAULT 'free',
    cluster_limit INTEGER NOT NULL DEFAULT 16,
    used_clusters INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    metadata JSONB
);

COMMENT ON TABLE accounts IS 'Tenant accounts for multi-tenant managed in-memory caching';
COMMENT ON COLUMN accounts.id IS 'Unique account identifier (UUID or email-based)';
COMMENT ON COLUMN accounts.cluster_limit IS 'Max concurrent cache clusters allowed for the account';
COMMENT ON COLUMN accounts.used_clusters IS 'Cache clusters currently counted against the limit';

CREATE INDEX IF NOT EXISTS idx_mneme_accounts_email ON accounts (email);
CREATE INDEX IF NOT EXISTS idx_mneme_accounts_status ON accounts (status);
CREATE INDEX IF NOT EXISTS idx_mneme_accounts_plan ON accounts (plan);

-- +goose Down
DROP TABLE IF EXISTS accounts;