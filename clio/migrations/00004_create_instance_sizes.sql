-- +goose Up
CREATE TABLE IF NOT EXISTS instance_sizes (
    name VARCHAR(64) PRIMARY KEY,
    vcpus INTEGER NOT NULL,
    memory_gb INTEGER NOT NULL,
    storage_gb INTEGER NOT NULL DEFAULT 20,
    price_per_hour_cents BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active'
);

COMMENT ON TABLE instance_sizes IS 'Catalog of available database instance sizes';

INSERT INTO instance_sizes (name, vcpus, memory_gb, storage_gb, price_per_hour_cents) VALUES
    ('clio-nano',   1, 2,   20, 100),
    ('clio-small',  1, 4,   40, 200),
    ('clio-medium', 2, 8,   80, 400),
    ('clio-large',  4, 16, 160, 800)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS instance_sizes;