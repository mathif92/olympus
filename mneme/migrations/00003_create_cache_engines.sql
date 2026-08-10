-- +goose Up
CREATE TABLE IF NOT EXISTS cache_engines (
    engine VARCHAR(32) NOT NULL,
    version VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    PRIMARY KEY (engine, version)
);

COMMENT ON TABLE cache_engines IS 'Catalog of in-memory cache engines and versions that Mneme can manage';

INSERT INTO cache_engines (engine, version, status) VALUES
    ('redis', '7.4', 'active'),
    ('redis', '7.2', 'active'),
    ('redis', '6.2', 'deprecated'),
    ('memcached', '1.6.22', 'active')
ON CONFLICT (engine, version) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS cache_engines;