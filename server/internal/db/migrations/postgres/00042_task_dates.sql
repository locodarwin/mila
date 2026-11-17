-- +goose Up
-- When a task was made, and when it first went to a status of kind
-- in-progress: what reports read for trends. Both are synced; the
-- server fills them in as tasks are made and moved, and migration 43
-- fills them in for tasks from before, from their history.
ALTER TABLE tasks ADD COLUMN made_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN started_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE tasks DROP COLUMN started_at;
ALTER TABLE tasks DROP COLUMN made_at;
