-- +goose Up
-- What each op of a batch written through the API changed from (docs/PLAN.md
-- section 11, Agent access), so the batch can be undone as a whole: the
-- fields' values before it, the clock it stamped them with, whether it made
-- the row, and its change number, which orders them. Kept as long as the
-- trash keeps rows.
CREATE TABLE batch_changes (
    op_id        TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL,
    batch        TEXT NOT NULL,
    entity       TEXT NOT NULL,
    entity_id    TEXT NOT NULL,
    made         BOOLEAN NOT NULL,
    before       TEXT NOT NULL,
    hlc          TEXT NOT NULL,
    seq          BIGINT NOT NULL,
    created_at   TIMESTAMP NOT NULL
);
CREATE INDEX batch_changes_batch ON batch_changes (workspace_id, user_id, batch, seq);
CREATE INDEX batch_changes_age ON batch_changes (created_at);

-- +goose Down
DROP INDEX batch_changes_age;
DROP INDEX batch_changes_batch;
DROP TABLE batch_changes;
