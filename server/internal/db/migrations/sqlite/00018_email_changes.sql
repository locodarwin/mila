-- +goose Up
-- A change of email address waiting for its new address to confirm it.
-- Only the hash of the link's token is kept. A newer request ends the
-- older ones (expires_at moves to the moment of the newer one), and rows
-- stay a day after they expire so the hourly limit can count them.
CREATE TABLE email_changes (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email      TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used_at    TIMESTAMP
);
CREATE INDEX email_changes_user ON email_changes (user_id, created_at);

-- +goose Down
DROP TABLE email_changes;
