-- +goose Up
-- A person's picture: the key of a square image (256 px at most) in the
-- attachments folder, under a random key of its own. Null shows their
-- initials. Unique, so a key names one person.
ALTER TABLE users ADD COLUMN avatar TEXT;
CREATE UNIQUE INDEX users_avatar ON users (avatar);

-- +goose Down
DROP INDEX users_avatar;
ALTER TABLE users DROP COLUMN avatar;
