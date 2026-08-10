-- +goose Up
CREATE TABLE IF NOT EXISTS instance_types (
    name VARCHAR(64) PRIMARY KEY,
    vcpus INTEGER NOT NULL,
    memory_gb INTEGER NOT NULL,
    storage_gb INTEGER NOT NULL DEFAULT 0,
    price_per_hour_cents BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active'
);

COMMENT ON TABLE instance_types IS 'Catalog of available instance type sizes';

INSERT INTO instance_types (name, vcpus, memory_gb, storage_gb, price_per_hour_cents) VALUES
    ('olympus-nano',   1,  1,  8,  100),
    ('olympus-small',  1,  2, 16,  200),
    ('olympus-medium', 2,  4, 32,  400),
    ('olympus-large',  4,  8, 64,  800)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS instance_types;