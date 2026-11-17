-- +goose Up
-- Files on a wiki page, as attachments are on a
-- task: the images a page shows are among them.
CREATE TABLE page_attachments (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    page_id      TEXT NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    filename     TEXT NOT NULL,
    mime         TEXT NOT NULL,
    size         BIGINT NOT NULL,
    storage_key  TEXT NOT NULL,
    uploaded_by  TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    deleted_at   TIMESTAMP,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX page_attachments_page ON page_attachments (page_id);

-- +goose Down
DROP TABLE page_attachments;
