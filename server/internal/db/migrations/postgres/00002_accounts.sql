-- +goose Up
-- Accounts (M1): users, sign-in by passkey or magic link, sessions,
-- workspaces, memberships and invites. Column order and types match the
-- SQLite migration exactly, so both dialects generate the same Go types.

CREATE TABLE users (
    id                TEXT PRIMARY KEY,
    email             TEXT NOT NULL UNIQUE,
    name              TEXT NOT NULL,
    is_instance_admin BOOLEAN NOT NULL DEFAULT FALSE,
    settings          TEXT NOT NULL DEFAULT '{}',
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL
);

-- A passkey's full WebAuthn credential is kept as JSON, so library
-- upgrades that add fields need no migration.
CREATE TABLE passkeys (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL UNIQUE,
    credential    TEXT NOT NULL,
    name          TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    last_used_at  TIMESTAMPTZ
);
CREATE INDEX passkeys_user ON passkeys (user_id);

-- The browser holds the token; only its SHA-256 is stored.
CREATE TABLE sessions (
    id           TEXT PRIMARY KEY,
    token_hash   TEXT NOT NULL UNIQUE,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_agent   TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX sessions_user ON sessions (user_id);

CREATE TABLE magic_links (
    token_hash TEXT PRIMARY KEY,
    email      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);
CREATE INDEX magic_links_email ON magic_links (email, created_at);

-- Server-side state between the two halves of a WebAuthn ceremony.
CREATE TABLE webauthn_challenges (
    id         TEXT PRIMARY KEY,
    user_id    TEXT REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    data       TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE workspaces (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    is_personal BOOLEAN NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,
    deleted_at  TIMESTAMPTZ
);

CREATE TABLE memberships (
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role         TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX memberships_user ON memberships (user_id);

CREATE TABLE invites (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    role         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    invited_by   TEXT NOT NULL REFERENCES users (id),
    created_at   TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    accepted_at  TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX invites_workspace ON invites (workspace_id);

-- +goose Down
DROP TABLE invites;
DROP TABLE memberships;
DROP TABLE workspaces;
DROP TABLE webauthn_challenges;
DROP TABLE magic_links;
DROP TABLE sessions;
DROP TABLE passkeys;
DROP TABLE users;
