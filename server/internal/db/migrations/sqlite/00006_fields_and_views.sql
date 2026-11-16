-- +goose Up
-- Custom fields and saved views (M4). Column order and types match the
-- Postgres migration.

-- A field for every task in the workspace (project_id null) or in one
-- project. options holds a select's choices as JSON.
CREATE TABLE custom_fields (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id   TEXT REFERENCES projects (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL,
    options      TEXT NOT NULL DEFAULT '[]',
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMP,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);

-- A task's value for a field, as JSON; the row's ID comes from the pair.
CREATE TABLE custom_field_values (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id      TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    field_id     TEXT NOT NULL REFERENCES custom_fields (id) ON DELETE CASCADE,
    value        TEXT NOT NULL,
    deleted_at   TIMESTAMP,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX custom_field_values_task ON custom_field_values (task_id);

-- A saved view: shared when owner_id is null, private to its owner
-- otherwise. config holds the filter, grouping and sort as JSON.
CREATE TABLE saved_views (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id   TEXT REFERENCES projects (id) ON DELETE CASCADE,
    owner_id     TEXT REFERENCES users (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL,
    config       TEXT NOT NULL,
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMP,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE saved_views;
DROP TABLE custom_field_values;
DROP TABLE custom_fields;
