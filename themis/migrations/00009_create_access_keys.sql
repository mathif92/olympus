-- +goose Up
CREATE TABLE IF NOT EXISTS access_keys (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    secret_hash VARCHAR(128) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    last_used_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_themis_keys_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_themis_keys_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE access_keys IS 'AWS-style access keys; only the SHA-256 of the secret is stored';
COMMENT ON COLUMN access_keys.id IS 'Access key id, e.g. AKIA followed by 16 base32 chars';
COMMENT ON COLUMN access_keys.secret_hash IS 'SHA-256 hex of the secret access key';
COMMENT ON COLUMN access_keys.status IS 'active or inactive';

CREATE INDEX IF NOT EXISTS idx_themis_keys_user ON access_keys (user_id);
CREATE INDEX IF NOT EXISTS idx_themis_keys_status ON access_keys (status);

-- +goose Down
DROP TABLE IF EXISTS access_keys;
