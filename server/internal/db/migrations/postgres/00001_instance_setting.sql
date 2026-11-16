-- +goose Up
-- Instance-wide settings the server generates or the admin changes,
-- such as the instance ID and, later, the web push keys.
CREATE TABLE instance_setting (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE instance_setting;
