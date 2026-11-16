-- +goose Up
-- Calendar feeds (M5): read-only iCal links a calendar app subscribes to.
-- Column order and types match the Postgres migration.

-- The link carries a random token, of which only the SHA-256 is kept, so
-- it is shown once when made. A feed is someone's own: what it shows is
-- worked out again at every fetch from what they can see then, so leaving
-- the workspace or losing a project ends it. scope is me (tasks assigned
-- to them and their My Day), project or view; target_id names the project
-- or saved view, and is null for me.
CREATE TABLE ical_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    scope        TEXT NOT NULL,
    target_id    TEXT,
    token_hash   TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP,
    revoked_at   TIMESTAMP
);
CREATE INDEX ical_tokens_user ON ical_tokens (user_id, workspace_id);

-- +goose Down
DROP TABLE ical_tokens;
