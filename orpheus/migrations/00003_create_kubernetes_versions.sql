-- +goose Up
CREATE TABLE IF NOT EXISTS kubernetes_versions (
    version VARCHAR(16) PRIMARY KEY,
    channel VARCHAR(16) NOT NULL DEFAULT 'stable',
    status VARCHAR(32) NOT NULL DEFAULT 'active'
);

COMMENT ON TABLE kubernetes_versions IS 'Catalog of Kubernetes control-plane versions that Orpheus can manage';

INSERT INTO kubernetes_versions (version, channel, status) VALUES
    ('1.31', 'stable', 'active'),
    ('1.30', 'stable', 'active'),
    ('1.29', 'stable', 'deprecated')
ON CONFLICT (version) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS kubernetes_versions;