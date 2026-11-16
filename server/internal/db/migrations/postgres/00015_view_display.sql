-- +goose Up
-- Sort, grouping, board columns and visible fields of a saved view, as
-- JSON. Null means the defaults. Kept apart from config so edits to the
-- filter and to these merge separately.
ALTER TABLE saved_views ADD COLUMN display TEXT;

-- +goose Down
ALTER TABLE saved_views DROP COLUMN display;
