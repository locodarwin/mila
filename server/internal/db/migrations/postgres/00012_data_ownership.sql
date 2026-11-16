-- +goose Up
-- Data ownership (M6): workspace exports and imports, and deleted
-- accounts. Column order and types match the SQLite migration.

-- A deleted account's row stays, emptied and named "Deleted user", so
-- what the person wrote in shared workspaces keeps an author; deleted_at
-- marks it.
ALTER TABLE users ADD COLUMN deleted_at TIMESTAMPTZ;

-- An export of one workspace as one member sees it, made by a job. The
-- zip is <data_dir>/exports/<id>.zip until expires_at. status is queued,
-- running, ready or failed; error says why a failed one failed.
CREATE TABLE exports (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    format       TEXT NOT NULL,
    status       TEXT NOT NULL,
    file_name    TEXT NOT NULL,
    size         BIGINT NOT NULL DEFAULT 0,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL,
    finished_at  TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX exports_user ON exports (user_id, created_at);

-- A JSON export being restored into a new workspace, made by a job. The
-- upload waits in <data_dir>/imports/<id>.zip until the job is done.
-- workspace_id is the workspace it is making, kept out of sight (its
-- deleted_at set) until the import completes. report counts what came
-- in and what was left out, as JSON.
CREATE TABLE imports (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id TEXT REFERENCES workspaces (id) ON DELETE SET NULL,
    file_name    TEXT NOT NULL,
    keep_ids     BOOLEAN NOT NULL,
    status       TEXT NOT NULL,
    report       TEXT NOT NULL DEFAULT '{}',
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL,
    finished_at  TIMESTAMPTZ
);
CREATE INDEX imports_user ON imports (user_id, created_at);

-- +goose Down
DROP TABLE imports;
DROP TABLE exports;
ALTER TABLE users DROP COLUMN deleted_at;
