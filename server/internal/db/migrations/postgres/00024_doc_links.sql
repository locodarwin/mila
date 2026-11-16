-- +goose Up
-- The links a task's notes or a page's body holds to pages and tasks
-- (docs/PLAN.md section 3), one row per document and target, kept by the
-- server whenever the notes or body are written. Renaming a page or task
-- reads it to find the documents whose links take the new name. Each row
-- names its document in task_id or page_id, never both, and goes with it.
CREATE TABLE doc_links (
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT REFERENCES tasks (id) ON DELETE CASCADE,
    page_id      TEXT REFERENCES pages (id) ON DELETE CASCADE,
    target_id    TEXT NOT NULL
);
CREATE UNIQUE INDEX doc_links_task ON doc_links (task_id, target_id);
CREATE UNIQUE INDEX doc_links_page ON doc_links (page_id, target_id);
CREATE INDEX doc_links_target ON doc_links (target_id);

-- +goose Down
DROP TABLE doc_links;
