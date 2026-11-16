-- +goose Up
-- People's own themes: each a validated theme document (compact JSON) that
-- overrides one of the built-in themes. The name is kept beside the body
-- for listing; the body holds it too.
CREATE TABLE user_themes (
  id         TEXT PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  body       TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX user_themes_user ON user_themes (user_id);

-- +goose Down
DROP TABLE user_themes;
