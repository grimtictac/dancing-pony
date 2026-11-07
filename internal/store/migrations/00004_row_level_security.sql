-- +goose Up
-- Row-Level Security: a second guard for tenant isolation, independent of
-- the WHERE restaurant_id = ... in every query. Even a query that forgets
-- it can only see the current restaurant's rows.
--
-- The API starts every transaction with
--   SELECT set_config('app.restaurant_id', '<uuid>', true)
-- and the setting ends with the transaction. When it is not set, the
-- policies match no rows: isolation fails closed.

-- +goose StatementBegin
CREATE FUNCTION current_restaurant_id() RETURNS uuid
LANGUAGE sql STABLE PARALLEL SAFE
AS $$ SELECT NULLIF(current_setting('app.restaurant_id', true), '')::uuid $$;
-- +goose StatementEnd

ALTER TABLE dishes ENABLE ROW LEVEL SECURITY;
-- FORCE applies the policy to the table owner too, so pointing the API at
-- the owner role by mistake still can't cross tenants. (Superusers always
-- bypass RLS; the API refuses to start as one.)
ALTER TABLE dishes FORCE ROW LEVEL SECURITY;

-- USING filters what can be read, updated and deleted; WITH CHECK stops a
-- row being written into, or moved to, another restaurant.
CREATE POLICY tenant_isolation ON dishes
    USING (restaurant_id = current_restaurant_id())
    WITH CHECK (restaurant_id = current_restaurant_id());

-- +goose Down
DROP POLICY tenant_isolation ON dishes;
ALTER TABLE dishes NO FORCE ROW LEVEL SECURITY;
ALTER TABLE dishes DISABLE ROW LEVEL SECURITY;
DROP FUNCTION current_restaurant_id();
