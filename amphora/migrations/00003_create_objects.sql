-- +goose Up
CREATE TABLE IF NOT EXISTS objects (
    id VARCHAR(64) PRIMARY KEY,
    space_id VARCHAR(64) NOT NULL,
    key_path VARCHAR(1024) NOT NULL,
    original_filename VARCHAR(512) NOT NULL,
    content_type VARCHAR(255),
    content_length BIGINT NOT NULL DEFAULT 0,
    etag VARCHAR(64) NOT NULL,
    version_id VARCHAR(128) NOT NULL DEFAULT 'LATEST',
    checksum_algorithm VARCHAR(32) NOT NULL DEFAULT 'sha256',
    checksum_value VARCHAR(64) NOT NULL,
    storage_path VARCHAR(1024),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    uploaded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(32) NOT NULL DEFAULT 'active',

    CONSTRAINT fk_objects_space
        FOREIGN KEY (space_id)
        REFERENCES spaces(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE objects IS 'Object metadata and file information';
COMMENT ON COLUMN objects.space_id IS 'Reference to parent space';
COMMENT ON COLUMN objects.key_path IS 'Logical object key (e.g., photos/vacation.jpg)';
COMMENT ON COLUMN objects.etag IS 'Content hash for versioning/optimistic locking';
COMMENT ON COLUMN objects.version_id IS 'Version identifier (UUID, timestamp, or LATEST)';
COMMENT ON COLUMN objects.storage_path IS 'Physical location of object data';

CREATE UNIQUE INDEX IF NOT EXISTS idx_objects_space_key ON objects (space_id, key_path);
CREATE INDEX IF NOT EXISTS idx_objects_key_path ON objects (key_path);
CREATE INDEX IF NOT EXISTS idx_objects_version ON objects (version_id);
CREATE INDEX IF NOT EXISTS idx_objects_status ON objects (status);
CREATE INDEX IF NOT EXISTS idx_objects_uploaded_at ON objects (uploaded_at);
CREATE INDEX IF NOT EXISTS idx_objects_version_space ON objects (space_id, version_id);

-- +goose Down
DROP TABLE IF EXISTS objects;