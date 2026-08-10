-- +goose Up
CREATE TABLE IF NOT EXISTS node_sizes (
    name VARCHAR(64) PRIMARY KEY,
    vcpus INTEGER NOT NULL,
    memory_gb INTEGER NOT NULL,
    price_per_hour_cents BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active'
);

COMMENT ON TABLE node_sizes IS 'Catalog of available worker node sizes for node groups';

INSERT INTO node_sizes (name, vcpus, memory_gb, price_per_hour_cents) VALUES
    ('olympus-nano',   1,  1,  100),
    ('olympus-small',  1,  2,  200),
    ('olympus-medium', 2,  4,  400),
    ('olympus-large',  4,  8,  800)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS node_sizes;