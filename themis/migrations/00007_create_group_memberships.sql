-- +goose Up
CREATE TABLE IF NOT EXISTS group_memberships (
    group_id VARCHAR(64) NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (group_id, user_id),

    CONSTRAINT fk_themis_memberships_group
        FOREIGN KEY (group_id)
        REFERENCES groups(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_themis_memberships_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE CASCADE
);

COMMENT ON TABLE group_memberships IS 'Many-to-many mapping of users to groups';

CREATE INDEX IF NOT EXISTS idx_themis_memberships_user ON group_memberships (user_id);

-- +goose Down
DROP TABLE IF EXISTS group_memberships;
