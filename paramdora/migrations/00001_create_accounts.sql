-- +goose Up
CREATE TABLE IF NOT EXISTS accounts (
    id VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    plan VARCHAR(32) NOT NULL DEFAULT 'free',
    parameter_limit INTEGER NOT NULL DEFAULT 250,
    used_parameters INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    metadata JSONB
);

COMMENT ON TABLE accounts IS 'Tenant accounts for multi-tenant parameter isolation';
COMMENT ON COLUMN accounts.id IS 'Unique account identifier (UUID or email-based)';
COMMENT ON COLUMN accounts.plan IS 'Plan tier: free, pro, enterprise';
COMMENT ON COLUMN accounts.parameter_limit IS 'Max parameters allowed across all projects';
COMMENT ON COLUMN accounts.used_parameters IS 'Total parameters stored across all projects';

CREATE INDEX IF NOT EXISTS idx_params_accounts_email ON accounts (email);
CREATE INDEX IF NOT EXISTS idx_params_accounts_status ON accounts (status);
CREATE INDEX IF NOT EXISTS idx_params_accounts_plan ON accounts (plan);

-- +goose Down
DROP TABLE IF EXISTS accounts;