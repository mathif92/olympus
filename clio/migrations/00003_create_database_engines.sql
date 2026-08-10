-- +goose Up
CREATE TABLE IF NOT EXISTS database_engines (
    engine VARCHAR(32) NOT NULL,
    version VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    PRIMARY KEY (engine, version)
);

COMMENT ON TABLE database_engines IS 'Catalog of relational database engines and versions that Clio can manage';

INSERT INTO database_engines (engine, version, status) VALUES
    ('postgres', '16.8', 'active'),
    ('postgres', '15.12', 'active'),
    ('postgres', '14.17', 'deprecated'),
    ('mysql', '8.4.4', 'active')
ON CONFLICT (engine, version) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS database_engines;