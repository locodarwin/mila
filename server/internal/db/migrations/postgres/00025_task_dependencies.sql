-- +goose Up
-- Dependencies (docs/PLAN.md section 3): task_id waits for blocker_id
-- to be done. A row is named by its pair (PairID of task_id and
-- blocker_id), so adding one again undeletes it.
CREATE TABLE task_dependencies (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    blocker_id   TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX task_dependencies_task ON task_dependencies (task_id);
CREATE INDEX task_dependencies_blocker ON task_dependencies (blocker_id);

-- +goose Down
DROP TABLE task_dependencies;
