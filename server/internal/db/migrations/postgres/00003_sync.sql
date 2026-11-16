-- +goose Up
-- Sync core (M2): projects, statuses and tasks, and the change log that
-- clients pull from. Column order and types match the SQLite migration.
--
-- Every synced row carries clocks: a JSON map from field name to the HLC
-- of the write that set it, which is how concurrent edits are merged.
-- A delete is the deleted_at field like any other, so it syncs too.

-- Each workspace numbers its own changes. A write increments this inside
-- its transaction, holding the row lock until commit, so sequence order
-- is commit order and a client pulling "after N" can never miss a change.
ALTER TABLE workspaces ADD COLUMN change_seq BIGINT NOT NULL DEFAULT 0;

CREATE TABLE projects (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    icon         TEXT NOT NULL DEFAULT '',
    color        TEXT NOT NULL DEFAULT 'blue',
    description  TEXT NOT NULL DEFAULT '',
    sort_key     TEXT NOT NULL,
    archived_at  TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX projects_workspace ON projects (workspace_id);

CREATE TABLE statuses (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id   TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    category     TEXT NOT NULL,
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX statuses_project ON statuses (project_id);

CREATE TABLE tasks (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id   TEXT REFERENCES projects (id) ON DELETE CASCADE,
    status_id    TEXT REFERENCES statuses (id) ON DELETE SET NULL,
    title        TEXT NOT NULL,
    notes        TEXT NOT NULL DEFAULT '',
    sort_key     TEXT NOT NULL,
    completed_at TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX tasks_workspace ON tasks (workspace_id);
CREATE INDEX tasks_project ON tasks (project_id);

-- Guests see only the projects listed here for them.
CREATE TABLE project_access (
    project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (project_id, user_id)
);

-- One row per accepted op. data maps each field the op changed to its
-- new value and HLC. project_id lets guests' pulls skip what they can't
-- see.
CREATE TABLE change_log (
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    seq          BIGINT NOT NULL,
    entity       TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    project_id   TEXT,
    data         TEXT NOT NULL,
    actor_id     TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (workspace_id, seq)
);

-- Op IDs already applied, so a retried push changes nothing twice.
CREATE TABLE applied_ops (
    op_id        TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE applied_ops;
DROP TABLE change_log;
DROP TABLE project_access;
DROP TABLE tasks;
DROP TABLE statuses;
DROP TABLE projects;
ALTER TABLE workspaces DROP COLUMN change_seq;
