-- +goose Up
-- A pair's row (a label on a task, an assignee, a dependency, a task in
-- someone's day, a field's value) took the two IDs XORed as its own, so
-- anyone could choose two of theirs that made another's and take it first.
-- Its ID is now a hash of the two, and migration 38 moves each row to it.
-- old_id is what a row of the entity was called, for edits still on their
-- way from before and REST callers that kept it. One pair can have rows of
-- two entities (an assignee and a My Day entry), under the same IDs.
CREATE TABLE pair_ids (
    entity TEXT NOT NULL,
    old_id TEXT NOT NULL,
    new_id TEXT NOT NULL,
    PRIMARY KEY (entity, old_id)
);

-- +goose Down
DROP TABLE pair_ids;
