-- +goose Up
-- Apps that connect to the MCP endpoint by OAuth. An app registers itself,
-- with no secret, and the addresses it may be sent back to. A person saying
-- yes makes a code good for ten minutes, once, which the app trades with
-- its PKCE verifier for an API token marked as the app's: only that token
-- reaches anything.
CREATE TABLE oauth_clients (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    redirect_uris TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL,
    used_at       TIMESTAMP
);
CREATE INDEX oauth_clients_age ON oauth_clients (created_at);

-- Only the code's SHA-256 is kept. token_id is the token it was traded
-- for, revoked if the code comes back a second time.
CREATE TABLE oauth_codes (
    code_hash       TEXT PRIMARY KEY,
    client_id       TEXT NOT NULL REFERENCES oauth_clients (id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id    TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    can_write       BOOLEAN NOT NULL,
    expires_in_days BIGINT NOT NULL,
    redirect_uri    TEXT NOT NULL,
    challenge       TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL,
    expires_at      TIMESTAMP NOT NULL,
    used_at         TIMESTAMP,
    token_id        TEXT
);
CREATE INDEX oauth_codes_age ON oauth_codes (expires_at);

-- A token an app was given, which reaches the MCP endpoint and nothing else.
ALTER TABLE api_tokens ADD COLUMN oauth_client_id TEXT REFERENCES oauth_clients (id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE api_tokens DROP COLUMN oauth_client_id;
DROP INDEX oauth_codes_age;
DROP TABLE oauth_codes;
DROP INDEX oauth_clients_age;
DROP TABLE oauth_clients;
