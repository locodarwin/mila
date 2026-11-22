-- +goose Up
-- Drawings in wiki pages: a drawing, which the page's document names, and
-- its shapes, each a row of its own so that people editing different
-- shapes at once keep both.
CREATE TABLE page_drawings (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    page_id      TEXT NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    name         TEXT NOT NULL DEFAULT '',
    grid         BOOLEAN NOT NULL DEFAULT TRUE,
    snap         BOOLEAN NOT NULL DEFAULT TRUE,
    made_by      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX page_drawings_page ON page_drawings (page_id);

-- from_id and to_id name other shapes of the drawing; they hold no
-- reference, so the copier never has to order a drawing's shapes.
CREATE TABLE drawing_shapes (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    page_id      TEXT NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    drawing_id   TEXT NOT NULL REFERENCES page_drawings (id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    box          TEXT NOT NULL,
    points       TEXT,
    text         TEXT NOT NULL DEFAULT '',
    style        TEXT NOT NULL DEFAULT '{}',
    from_id      TEXT,
    to_id        TEXT,
    image_id     TEXT REFERENCES page_attachments (id) ON DELETE SET NULL,
    sort_key     TEXT NOT NULL,
    deleted_at   TIMESTAMPTZ,
    clocks       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX drawing_shapes_drawing ON drawing_shapes (drawing_id);
CREATE INDEX drawing_shapes_page ON drawing_shapes (page_id);

-- +goose Down
DROP TABLE drawing_shapes;
DROP TABLE page_drawings;
