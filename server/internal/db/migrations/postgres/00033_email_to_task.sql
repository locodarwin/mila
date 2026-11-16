-- +goose Up
-- Email to task (docs/PLAN.md section 11): a person's private addresses, each
-- going to the Inbox or a project, and the mail that reached them. An address
-- can only add tasks, so its token stays readable for the person to copy again.
-- senders is a JSON list of the addresses accepted beside the person's own.
CREATE TABLE inbound_addresses (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    token        TEXT NOT NULL UNIQUE,
    project_id   TEXT REFERENCES projects (id) ON DELETE CASCADE,
    assign_self  BOOLEAN NOT NULL,
    senders      TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX inbound_addresses_user ON inbound_addresses (user_id, workspace_id);

-- Who mail came from and its subject, never the message of one refused. Kept
-- 30 days. Outlives the address it came to, which is why that is no reference.
CREATE TABLE inbound_mail (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    address_id   TEXT NOT NULL,
    from_address TEXT NOT NULL,
    subject      TEXT NOT NULL,
    outcome      TEXT NOT NULL,
    task_id      TEXT REFERENCES tasks (id) ON DELETE SET NULL,
    size         BIGINT NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX inbound_mail_user ON inbound_mail (user_id, workspace_id, received_at);
CREATE INDEX inbound_mail_address ON inbound_mail (address_id, outcome, received_at);

-- +goose Down
DROP TABLE inbound_mail;
DROP TABLE inbound_addresses;
