-- +goose Up
-- Operations (M6): compacting the sync log. Column order and types match
-- the SQLite migration.

-- The change log is compacted after 30 days. log_floor is the highest
-- change number deleted so far: a device that has pulled less than that
-- has missed changes the log no longer holds, so it bootstraps again.
ALTER TABLE workspaces ADD COLUMN log_floor BIGINT NOT NULL DEFAULT 0;

CREATE INDEX change_log_created ON change_log (workspace_id, created_at);
CREATE INDEX applied_ops_created ON applied_ops (created_at);

-- +goose Down
DROP INDEX applied_ops_created;
DROP INDEX change_log_created;
ALTER TABLE workspaces DROP COLUMN log_floor;
