-- +goose Up
-- Comments pinned to text in a wiki page (docs/PLAN.md section 3): a thread
-- holds where it is pinned and whether it is resolved, which anyone who can
-- comment may change; its comments are their authors' alone.
CREATE TABLE page_threads (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    page_id      TEXT NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    created_by   TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    quote        TEXT NOT NULL,
    anchor       TEXT NOT NULL DEFAULT '',
    resolved_at  TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX page_threads_page ON page_threads (page_id);

CREATE TABLE page_comments (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    page_id      TEXT NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    thread_id    TEXT NOT NULL REFERENCES page_threads (id) ON DELETE CASCADE,
    author_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body         TEXT NOT NULL,
    edited_at    TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX page_comments_thread ON page_comments (thread_id);
CREATE INDEX page_comments_page ON page_comments (page_id);

-- A notification about a thread, as task_id is for one about a task.
ALTER TABLE notifications ADD COLUMN page_thread_id TEXT REFERENCES page_threads (id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE notifications DROP COLUMN page_thread_id;
DROP TABLE page_comments;
DROP TABLE page_threads;
