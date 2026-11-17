-- +goose Up
-- One change to many tasks (selecting several): the
-- batch its ops carried. A rule's runs from one batch count once toward its
-- hourly pause, and the assignments one batch makes are pushed as one.
ALTER TABLE automation_runs ADD COLUMN batch TEXT;
ALTER TABLE notifications ADD COLUMN batch TEXT;
CREATE INDEX notifications_batch ON notifications (user_id, batch);

-- +goose Down
DROP INDEX notifications_batch;
ALTER TABLE notifications DROP COLUMN batch;
ALTER TABLE automation_runs DROP COLUMN batch;
