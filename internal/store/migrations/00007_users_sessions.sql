-- +goose Up
-- Users are global: one identity across restaurants. Their role at each
-- restaurant comes from memberships (next migration).
CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT uuidv7(),
    -- Stored normalised (trimmed, lower-case) so the unique index matches
    -- however the address was typed.
    email             text NOT NULL UNIQUE
                      CHECK (email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 254),
    -- argon2id in PHC format; parsed strictly by the application.
    password_hash     text NOT NULL,
    is_platform_admin boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now()
);

-- pony_app registers users and reads them at login. The column list on
-- INSERT means the API can never create a platform admin, even if a bug
-- tried to: only the schema owner (pony admin create) can.
GRANT SELECT ON users TO pony_app;
GRANT INSERT (email, password_hash) ON users TO pony_app;

-- Opaque session tokens. Only a SHA-256 hash of each token is stored, so a
-- database leak doesn't hand out live sessions.
CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY CHECK (length(token_hash) = 32),
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
GRANT SELECT, INSERT, DELETE ON sessions TO pony_app;

-- Reviews now belong to real users. Reviews written with the temporary
-- X-Demo-User header have no user, so they're removed first. FORCE row
-- security would hide them from the owner too (no tenant is set here), so
-- it's lifted for the clean-up.
ALTER TABLE reviews NO FORCE ROW LEVEL SECURITY;
DELETE FROM reviews WHERE user_id NOT IN (SELECT id FROM users);
ALTER TABLE reviews FORCE ROW LEVEL SECURITY;
ALTER TABLE reviews ADD CONSTRAINT reviews_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE reviews DROP CONSTRAINT reviews_user_id_fkey;
DROP TABLE sessions;
DROP TABLE users;
