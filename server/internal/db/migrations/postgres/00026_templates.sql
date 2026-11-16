-- +goose Up
-- Templates (docs/PLAN.md section 3): a project or a task saved to make
-- more from. data is the snapshot as JSON (web/src/lib/templates.ts).
-- Members and admins see a workspace's templates; guests never do.
CREATE TABLE templates (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    name         TEXT NOT NULL,
    data         TEXT NOT NULL,
    saved_by     TEXT REFERENCES users (id) ON DELETE SET NULL,
    saved_at     TIMESTAMPTZ,
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX templates_workspace ON templates (workspace_id);

-- +goose Down
DROP TABLE templates;
