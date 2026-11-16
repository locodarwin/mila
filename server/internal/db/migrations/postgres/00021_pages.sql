-- +goose Up
-- The wiki (docs/PLAN.md section 3): pages of a project, or of the whole
-- workspace when project_id is null. A wiki's main page has no parent and
-- an ID made from its project's or workspace's; every other page has a
-- parent. body is the page's live document rendered as Markdown; the
-- document itself is page_updates, as a task's notes are doc_updates.
CREATE TABLE pages (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id   TEXT REFERENCES projects (id) ON DELETE CASCADE,
    parent_id    TEXT REFERENCES pages (id) ON DELETE CASCADE,
    title        TEXT NOT NULL DEFAULT '',
    body         TEXT NOT NULL DEFAULT '',
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX pages_workspace ON pages (workspace_id);
CREATE INDEX pages_parent ON pages (parent_id);

CREATE TABLE page_updates (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    page_id      TEXT NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    data         TEXT NOT NULL,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX page_updates_page ON page_updates (page_id);

-- +goose Down
DROP TABLE page_updates;
DROP TABLE pages;
