-- +goose Up
-- Workspace themes: themes a workspace offers
-- its members and guests, never imposes. body is the theme document as
-- JSON (web/src/lib/theme/document.ts), a copy of one of the publisher's
-- own themes; source_id is that theme's id, for Update from my theme.
-- The theme suggested to everyone is the one with the latest
-- suggested_at. Owners and admins change them; everyone sees them.
CREATE TABLE workspace_themes (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    body         TEXT NOT NULL,
    source_id    TEXT,
    suggested_at TIMESTAMP,
    published_by TEXT REFERENCES users (id) ON DELETE SET NULL,
    published_at TIMESTAMP,
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMP,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX workspace_themes_workspace ON workspace_themes (workspace_id);

-- +goose Down
DROP TABLE workspace_themes;
