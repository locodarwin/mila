-- +goose Up
-- Time logged to a project itself, not to one of its tasks, as
-- time_entries is to a task: typed in, or a timer, which runs while
-- running is true (its minutes are 0 until it stops).
CREATE TABLE project_time_entries (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id   TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day          TEXT NOT NULL,
    minutes      BIGINT NOT NULL DEFAULT 0,
    note         TEXT NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ,
    running      BOOLEAN NOT NULL DEFAULT FALSE,
    active_at    TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX project_time_entries_project ON project_time_entries (project_id);
CREATE INDEX project_time_entries_user ON project_time_entries (user_id);

-- +goose Down
DROP TABLE project_time_entries;
