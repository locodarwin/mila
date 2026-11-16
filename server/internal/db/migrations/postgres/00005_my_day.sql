-- +goose Up
-- My Day (M4): the tasks each person has picked for a day. Entries are
-- private to their user. Column order and types match the SQLite
-- migration.

CREATE TABLE my_day_entries (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day          TEXT NOT NULL,
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX my_day_entries_user ON my_day_entries (user_id);

-- +goose Down
DROP TABLE my_day_entries;
