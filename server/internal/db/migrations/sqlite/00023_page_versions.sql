-- +goose Up
-- A wiki page's history (docs/PLAN.md section 3): its body after each
-- editing session, kept for good. A session is one person's edits with
-- no gap of more than five minutes; a body written whole (a restore,
-- a REST write) is a version of its own.
-- actor_id is null for the body a page had before its history began.
CREATE TABLE page_versions (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    page_id      TEXT NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    actor_id     TEXT REFERENCES users (id) ON DELETE SET NULL,
    body         TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX page_versions_page ON page_versions (page_id, created_at);

-- +goose Down
DROP TABLE page_versions;
