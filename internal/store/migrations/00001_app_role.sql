-- +goose Up
-- pony_app is the role the API connects as. It owns nothing, so Row-Level
-- Security always applies to it. Roles are cluster-wide, so it may already
-- exist (for example created with a password by the local compose init
-- script).
--
-- A new role is NOSUPERUSER NOBYPASSRLS by default. The migration can't
-- enforce that on an existing role: changing those attributes needs a
-- superuser, which the owner of a managed database is not. The API checks
-- them at startup instead and refuses to run with a privileged role.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'pony_app') THEN
        CREATE ROLE pony_app NOLOGIN;
    END IF;
END
$$;
-- +goose StatementEnd
GRANT USAGE ON SCHEMA public TO pony_app;

-- +goose Down
-- The role itself is kept: it is cluster-wide and may be in use as a login.
REVOKE USAGE ON SCHEMA public FROM pony_app;
