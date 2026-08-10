-- +goose Up
CREATE TABLE IF NOT EXISTS node_types (
    name VARCHAR(64) PRIMARY KEY,
    vcpus INTEGER NOT NULL,
    memory_gb INTEGER NOT NULL,
    price_per_hour_cents BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active'
);

COMMENT ON TABLE node_types IS 'Catalog of available cache node types';

INSERT INTO node_types (name, vcpus, memory_gb, price_per_hour_cents) VALUES
    ('mneme-micro',   1, 1,  80),
    ('mneme-small',   1, 2, 160),
    ('mneme-medium',  2, 4, 320),
    ('mneme-large',   4, 8, 640)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS node_types;