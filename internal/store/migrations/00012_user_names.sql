-- +goose Up
-- The brief: users have a name, an email address and a password. Existing
-- users (created before names were asked for) get an empty name.
ALTER TABLE users
    ADD COLUMN name text NOT NULL DEFAULT '' CHECK (length(name) <= 100 AND name = btrim(name));

GRANT INSERT (name) ON users TO pony_app;

-- +goose Down
REVOKE INSERT (name) ON users FROM pony_app;
ALTER TABLE users DROP COLUMN name;
