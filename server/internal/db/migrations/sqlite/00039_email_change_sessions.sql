-- +goose Up
-- The session that asked for an email change: the change goes through only
-- while it lasts, so ending it (signing out, revoking it from Security, an
-- admin signing the person out everywhere) ends the change too. Changes
-- from before have none, and expire within a day.
ALTER TABLE email_changes ADD COLUMN session_id TEXT;

-- +goose Down
ALTER TABLE email_changes DROP COLUMN session_id;
