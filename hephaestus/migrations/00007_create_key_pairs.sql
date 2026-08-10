-- +goose Up
CREATE TABLE IF NOT EXISTS key_pairs (
    id VARCHAR(64) PRIMARY KEY,
    project_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    fingerprint VARCHAR(128) NOT NULL,
    public_key TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_key_pairs_project
        FOREIGN KEY (project_id)
        REFERENCES projects(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE key_pairs IS 'SSH key pairs for instance access. Only the public key is stored; the private key is shown once at creation.';

CREATE UNIQUE INDEX IF NOT EXISTS idx_key_pairs_project_name ON key_pairs (project_id, name);
CREATE INDEX IF NOT EXISTS idx_key_pairs_fingerprint ON key_pairs (fingerprint);

-- +goose Down
DROP TABLE IF EXISTS key_pairs;