-- +goose Up
-- Recurrence (M5): a task's repeat rule (a subset of RRULE), whether the
-- next date follows the schedule or the completion, and the series it
-- belongs to; plus who created the task. Column order and types match the
-- Postgres migration.

ALTER TABLE tasks ADD COLUMN created_by TEXT REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE tasks ADD COLUMN recurrence TEXT;
ALTER TABLE tasks ADD COLUMN recurrence_mode TEXT NOT NULL DEFAULT 'schedule';
ALTER TABLE tasks ADD COLUMN series_id TEXT;

-- +goose Down
ALTER TABLE tasks DROP COLUMN series_id;
ALTER TABLE tasks DROP COLUMN recurrence_mode;
ALTER TABLE tasks DROP COLUMN recurrence;
ALTER TABLE tasks DROP COLUMN created_by;
