-- +goose Up
-- How a session was signed in (passkey, magic_link, setup, invite), so
-- what needs a recent sign-in can ask for a passkey from an account that
-- has one. Sessions from before this say ''.
ALTER TABLE sessions ADD COLUMN method TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE sessions DROP COLUMN method;
