-- +goose Up
-- Notifications (M5): background jobs, the notifications people get, and
-- when each person's daily digest last went out. Column order and types
-- match the SQLite migration.

-- Work done outside a request: sending email, delivering notifications.
-- A runner claims a due job by setting locked_until, so two runners never
-- take the same one; a job that keeps failing stops after six attempts
-- with its last error kept.
CREATE TABLE jobs (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL,
    payload      TEXT NOT NULL,
    run_at       TIMESTAMPTZ NOT NULL,
    attempts     BIGINT NOT NULL DEFAULT 0,
    last_error   TEXT,
    locked_until TIMESTAMPTZ,
    done_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX jobs_due ON jobs (done_at, run_at);

-- in_app is false for a notification the person turned off in the app
-- but still gets by email or push: the row records the delivery.
CREATE TABLE notifications (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    task_id      TEXT REFERENCES tasks (id) ON DELETE CASCADE,
    actor_id     TEXT REFERENCES users (id) ON DELETE SET NULL,
    title        TEXT NOT NULL,
    body         TEXT NOT NULL,
    in_app       BOOLEAN NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    read_at      TIMESTAMPTZ,
    emailed_at   TIMESTAMPTZ,
    pushed_at    TIMESTAMPTZ
);
CREATE INDEX notifications_user ON notifications (user_id, created_at);

-- The local date of each person's last digest, so it goes out once a day.
CREATE TABLE digests (
    user_id   TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    last_date TEXT NOT NULL,
    sent_at   TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE digests;
DROP TABLE notifications;
DROP TABLE jobs;
