-- +goose Up
-- Task depth (M3): subtasks, priority and dates on tasks; labels,
-- assignees, checklists, comments and attachments as synced rows of their
-- own; and an activity history. Column order and types match the SQLite
-- migration.

ALTER TABLE tasks ADD COLUMN parent_id TEXT REFERENCES tasks (id) ON DELETE CASCADE;
ALTER TABLE tasks ADD COLUMN priority TEXT NOT NULL DEFAULT 'none';
ALTER TABLE tasks ADD COLUMN start_date TEXT;
ALTER TABLE tasks ADD COLUMN due_date TEXT;
ALTER TABLE tasks ADD COLUMN due_time TEXT;
CREATE INDEX tasks_parent ON tasks (parent_id);

CREATE TABLE labels (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    color        TEXT NOT NULL DEFAULT 'blue',
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX labels_workspace ON labels (workspace_id);

-- A label on a task, and a person assigned to a task, are rows whose ID is
-- derived from the pair, so two devices adding the same one offline make
-- the same row instead of two.
CREATE TABLE task_labels (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    label_id     TEXT NOT NULL REFERENCES labels (id) ON DELETE CASCADE,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX task_labels_task ON task_labels (task_id);

CREATE TABLE task_assignees (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX task_assignees_task ON task_assignees (task_id);

CREATE TABLE checklist_items (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    text         TEXT NOT NULL,
    done         BOOLEAN NOT NULL DEFAULT FALSE,
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX checklist_items_task ON checklist_items (task_id);

CREATE TABLE comments (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    author_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body         TEXT NOT NULL,
    edited_at    TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX comments_task ON comments (task_id);

-- An attachment's file is stored by the server (storage_key names it);
-- only the server creates these rows, when the file is uploaded.
CREATE TABLE attachments (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    filename     TEXT NOT NULL,
    mime         TEXT NOT NULL,
    size         BIGINT NOT NULL,
    storage_key  TEXT NOT NULL,
    uploaded_by  TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX attachments_task ON attachments (task_id);

-- What happened to each task, in words a person can read; kept forever,
-- unlike the change log.
CREATE TABLE activity (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL,
    actor_id     TEXT NOT NULL,
    entity       TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    kind         TEXT NOT NULL,
    data         TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX activity_task ON activity (task_id, created_at);

-- +goose Down
DROP TABLE activity;
DROP TABLE attachments;
DROP TABLE comments;
DROP TABLE checklist_items;
DROP TABLE task_assignees;
DROP TABLE task_labels;
DROP TABLE labels;
DROP INDEX tasks_parent;
ALTER TABLE tasks DROP COLUMN due_time;
ALTER TABLE tasks DROP COLUMN due_date;
ALTER TABLE tasks DROP COLUMN start_date;
ALTER TABLE tasks DROP COLUMN priority;
ALTER TABLE tasks DROP COLUMN parent_id;
