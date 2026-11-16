-- +goose Up
-- Signing in to the desktop app through the person's own browser (docs/PLAN.md
-- section 3): a code the signed-in browser hands to the app, good once, for a few
-- minutes, and only with the secret whose hash it was asked for with.
CREATE TABLE desktop_codes (
    code_hash  TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    challenge  TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE desktop_codes;
