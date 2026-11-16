-- +goose Up
-- Web push (M5): the devices that asked for push notifications. Column
-- order and types match the SQLite migration.

-- One row per browser subscription, keyed by its endpoint, the push
-- service address the browser gave. It belongs to the session that made
-- it: a device that signs out, or whose session is revoked or expires,
-- gets no more pushes, and the row goes with its session.
CREATE TABLE push_subscriptions (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    session_id   TEXT NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    endpoint     TEXT NOT NULL UNIQUE,
    p256dh       TEXT NOT NULL,
    auth         TEXT NOT NULL,
    user_agent   TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ
);
CREATE INDEX push_subscriptions_user ON push_subscriptions (user_id);
CREATE INDEX push_subscriptions_session ON push_subscriptions (session_id);

-- +goose Down
DROP TABLE push_subscriptions;
