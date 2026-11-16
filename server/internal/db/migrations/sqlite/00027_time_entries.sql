-- +goose Up
-- Time tracking and estimates (docs/PLAN.md section 3). A task's estimate
-- is its own work in minutes, or null for none. Each piece of time logged
-- is a row: typed in, or a timer, which runs while running is true (its
-- minutes are 0 until it stops) and counts from started_at. active_at is
-- when its person last did something in Mila while it ran.
ALTER TABLE tasks ADD COLUMN estimate_minutes BIGINT;

CREATE TABLE time_entries (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day          TEXT NOT NULL,
    minutes      BIGINT NOT NULL DEFAULT 0,
    note         TEXT NOT NULL DEFAULT '',
    started_at   TIMESTAMP,
    running      BOOLEAN NOT NULL DEFAULT FALSE,
    active_at    TIMESTAMP,
    deleted_at   TIMESTAMP,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX time_entries_task ON time_entries (task_id);
CREATE INDEX time_entries_user ON time_entries (user_id);

-- +goose Down
DROP TABLE time_entries;
ALTER TABLE tasks DROP COLUMN estimate_minutes;
