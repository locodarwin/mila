-- +goose Up
-- Webhooks (docs/PLAN.md section 11): a workspace's addresses to tell of changes,
-- and each delivery with its body, so it can be tried again and resent. The secret
-- is kept as it is, since signing needs it. No project_id means every project.
CREATE TABLE webhooks (
    id            TEXT PRIMARY KEY,
    workspace_id  TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    created_by    TEXT NOT NULL,
    url           TEXT NOT NULL,
    secret        TEXT NOT NULL,
    events        TEXT NOT NULL,
    project_id    TEXT,
    created_at    TIMESTAMP NOT NULL,
    paused_at     TIMESTAMP,
    disabled_at   TIMESTAMP,
    failing_since TIMESTAMP
);
CREATE INDEX webhooks_workspace ON webhooks (workspace_id, created_at);

CREATE TABLE webhook_deliveries (
    id            TEXT PRIMARY KEY,
    webhook_id    TEXT NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
    event         TEXT NOT NULL,
    summary       TEXT NOT NULL,
    body          TEXT NOT NULL,
    status        TEXT NOT NULL,
    attempts      BIGINT NOT NULL,
    response_code BIGINT,
    response_body TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL,
    attempted_at  TIMESTAMP,
    next_at       TIMESTAMP
);
CREATE INDEX webhook_deliveries_webhook ON webhook_deliveries (webhook_id, created_at);
CREATE INDEX webhook_deliveries_created ON webhook_deliveries (created_at);

-- +goose Down
DROP TABLE webhook_deliveries;
DROP TABLE webhooks;
