-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
    id VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    plan VARCHAR(32) NOT NULL DEFAULT 'free',
    instance_limit INTEGER NOT NULL DEFAULT 16,
    used_instances INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    metadata JSONB
);

COMMENT ON TABLE accounts IS 'Tenant accounts for multi-tenant compute isolation';
COMMENT ON COLUMN accounts.id IS 'Unique account identifier (UUID or email-based)';
COMMENT ON COLUMN accounts.instance_limit IS 'Max running instances allowed for the account';
COMMENT ON COLUMN accounts.used_instances IS 'Running instances currently counted against the limit';

CREATE INDEX IF NOT EXISTS idx_compute_accounts_email ON accounts (email);
CREATE INDEX IF NOT EXISTS idx_compute_accounts_status ON accounts (status);
CREATE INDEX IF NOT EXISTS idx_compute_accounts_plan ON accounts (plan);

-- +goose Down
DROP TABLE IF EXISTS accounts;