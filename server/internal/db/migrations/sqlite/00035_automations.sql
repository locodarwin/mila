-- +goose Up
-- Automations: when something happens to a task, and it
-- fits, the changes a person chose. rule is the rule as JSON (syncer/automation.go).
-- Members see a workspace's rules; guests never do.
CREATE TABLE automations (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    project_id   TEXT REFERENCES projects (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    enabled      BOOLEAN NOT NULL,
    enabled_at   TIMESTAMP,
    rule         TEXT NOT NULL,
    made_by      TEXT,
    paused_why   TEXT,
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMP,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX automations_workspace ON automations (workspace_id);

-- What each rule did, kept 30 days. Outlives what it names, so these are no references.
CREATE TABLE automation_runs (
    id            TEXT PRIMARY KEY,
    workspace_id  TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    automation_id TEXT NOT NULL,
    task_id       TEXT NOT NULL,
    task_title    TEXT NOT NULL,
    outcome       TEXT NOT NULL,
    detail        TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL
);
CREATE INDEX automation_runs_workspace ON automation_runs (workspace_id, created_at);
CREATE INDEX automation_runs_automation ON automation_runs (automation_id, created_at);

-- A rule on a due date runs once for each date a task has.
CREATE TABLE automation_fired (
    automation_id TEXT NOT NULL,
    task_id       TEXT NOT NULL,
    due_date      TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL,
    PRIMARY KEY (automation_id, task_id, due_date)
);

-- +goose Down
DROP TABLE automation_fired;
DROP TABLE automation_runs;
DROP TABLE automations;
