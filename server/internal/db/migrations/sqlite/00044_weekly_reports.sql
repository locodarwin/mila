-- +goose Up
-- The weekly report by email (docs/PLAN.md section 3, Reporting): the last
-- week reported to each person for each workspace, the day it began, so a
-- restart or a second server never sends one twice.
CREATE TABLE weekly_reports (
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    week         TEXT NOT NULL,
    sent_at      TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, workspace_id)
);

-- +goose Down
DROP TABLE weekly_reports;
