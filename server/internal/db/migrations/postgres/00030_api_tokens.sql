-- +goose Up
-- Personal API tokens (docs/PLAN.md section 11): a person's way in for a script
-- or another app. Only the token's SHA-256 is kept. No workspace_id means every
-- workspace the person is in, now and later.
CREATE TABLE api_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    can_write    BOOLEAN NOT NULL,
    workspace_id TEXT REFERENCES workspaces (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX api_tokens_user ON api_tokens (user_id, created_at);

-- +goose Down
DROP TABLE api_tokens;
