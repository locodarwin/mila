-- +goose Up
-- Admin and security (M6): the security log, the devices people sign in
-- from, and resetting a guest's copy when their access changes. Column
-- order and types match the SQLite migration.

-- Where a session was last used from: the client address as the server
-- saw it (behind a trusted proxy, the address the proxy reported).
ALTER TABLE sessions ADD COLUMN ip TEXT NOT NULL DEFAULT '';

-- A member's devices load the workspace afresh when their access to its
-- projects changes (a guest given or denied a project, a role to or from
-- guest): a pull from before reset_seq answers 410, so a device that was
-- offline at the time hears of it too. Changing access raises the
-- workspace's change number and stores it here.
ALTER TABLE memberships ADD COLUMN reset_seq BIGINT NOT NULL DEFAULT 0;

-- What happened to an account that its owner should be able to see:
-- sign-ins (method: passkey, magic_link, setup or invite; new_device when
-- the browser had not signed in to this account before), accepted
-- invitations, passkeys added and removed, devices signed out, and admin
-- rights given or taken. detail depends on the kind: the workspace, the
-- passkey's name, the signed-out device's user agent, the admin's name.
-- ip and user_agent are the request's that caused it. Kept for a year.
CREATE TABLE security_events (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    method     TEXT NOT NULL DEFAULT '',
    new_device BOOLEAN NOT NULL DEFAULT FALSE,
    detail     TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX security_events_user ON security_events (user_id, id);
CREATE INDEX security_events_created ON security_events (created_at);

-- Browsers an account has signed in from. The browser keeps a random
-- token in a long-lived cookie (mila_device), shared by everyone who
-- signs in on it; only its SHA-256 is stored, per account, so a sign-in
-- from a browser the account has not used before can be told apart.
CREATE TABLE known_devices (
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_hash  TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, device_hash)
);

-- +goose Down
DROP TABLE known_devices;
DROP TABLE security_events;
ALTER TABLE memberships DROP COLUMN reset_seq;
ALTER TABLE sessions DROP COLUMN ip;
