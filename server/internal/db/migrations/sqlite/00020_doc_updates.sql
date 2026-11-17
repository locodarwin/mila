-- +goose Up
-- Live documents: each row is one Yjs update to a
-- task's notes, as base64. Rows are never changed, only deleted when the
-- server merges a document's rows into one; the task's notes column holds
-- the document rendered as Markdown.
CREATE TABLE doc_updates (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    data         TEXT NOT NULL,
    deleted_at   TIMESTAMP,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX doc_updates_task ON doc_updates (task_id);

-- +goose Down
DROP TABLE doc_updates;
