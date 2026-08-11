-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
    id VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    plan VARCHAR(32) NOT NULL DEFAULT 'free',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    metadata JSONB
);

COMMENT ON TABLE accounts IS 'Tenant accounts for multi-tenant IAM isolation';
COMMENT ON COLUMN accounts.plan IS 'Plan tier: free, pro, enterprise';
COMMENT ON COLUMN accounts.status IS 'active, suspended, deleted';

CREATE INDEX IF NOT EXISTS idx_themis_accounts_email ON accounts (email);
CREATE INDEX IF NOT EXISTS idx_themis_accounts_status ON accounts (status);

-- +goose Down
DROP TABLE IF EXISTS accounts;
