-- +goose Up
-- A user's role at a restaurant. No row means "customer": any logged-in
-- user may browse and review, so registration stays global and simple.
CREATE TABLE memberships (
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role          text NOT NULL CHECK (role IN ('owner', 'staff')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (restaurant_id, user_id)
);

ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON memberships
    USING (restaurant_id = current_restaurant_id())
    WITH CHECK (restaurant_id = current_restaurant_id());

-- The API only reads roles; granting them is an operator task for now
-- (pony admin grant).
GRANT SELECT ON memberships TO pony_app;

-- +goose Down
DROP TABLE memberships;
