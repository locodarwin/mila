-- +goose Up
-- When a browser was last told that its session, and with it its pushes,
-- would soon end.
ALTER TABLE push_subscriptions ADD COLUMN warned_at TIMESTAMP;

-- +goose Down
ALTER TABLE push_subscriptions DROP COLUMN warned_at;
