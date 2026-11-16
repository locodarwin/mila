-- +goose Up
-- Reminders (M5): a person's reminders on tasks, synced and private to
-- them, and what the server has already sent: reminders fired and tasks
-- whose due moment was announced. Column order and types match the
-- SQLite migration.

-- offset_minutes is set for a reminder made relative to the due moment
-- ("1 hour before"); it then follows the task's due date and time, and
-- remind_at is only the time it worked out to when last saved.
CREATE TABLE reminders (
    id             TEXT PRIMARY KEY,
    workspace_id   TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    task_id        TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id        TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    remind_at      TIMESTAMPTZ NOT NULL,
    offset_minutes BIGINT,
    deleted_at     TIMESTAMPTZ,
    clocks         TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL
);
CREATE INDEX reminders_task ON reminders (task_id);
CREATE INDEX reminders_remind_at ON reminders (remind_at);

-- A reminder fired for one moment. Kept apart from the synced row, so a
-- reminder moved to another time fires again.
CREATE TABLE reminder_deliveries (
    reminder_id TEXT NOT NULL REFERENCES reminders (id) ON DELETE CASCADE,
    remind_at   TIMESTAMPTZ NOT NULL,
    sent_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (reminder_id, remind_at)
);

-- A task's due moment announced to one person. due is the due date, and
-- the time if it has one, so a rescheduled task is announced again.
CREATE TABLE due_notices (
    task_id TEXT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    due     TEXT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (task_id, user_id, due)
);

-- +goose Down
DROP TABLE due_notices;
DROP TABLE reminder_deliveries;
DROP TABLE reminders;
