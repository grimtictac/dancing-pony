-- +goose Up
-- Dish search: full-text search on names and descriptions, plus
-- trigram matching on names so a typo ("mushrom") still finds a dish.

-- pg_trgm is a "trusted" extension, so the database owner can create it
-- without being a superuser (as on a managed database).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- One definition of the search document, used by both the index and the
-- query, so they always match. Name words rank above description words.
-- +goose StatementBegin
CREATE FUNCTION dish_search_vector(name text, description text) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN setweight(to_tsvector('english', name), 'A')
    || setweight(to_tsvector('english', description), 'B');
-- +goose StatementEnd

CREATE INDEX dishes_search_idx ON dishes USING gin (dish_search_vector(name, description));
CREATE INDEX dishes_name_trgm_idx ON dishes USING gin (name gin_trgm_ops);

-- Keyset pagination for each sort order: (restaurant, sort key, id).
CREATE INDEX dishes_name_idx ON dishes (restaurant_id, name, id);
CREATE INDEX dishes_price_idx ON dishes (restaurant_id, price_amount, id);

-- +goose Down
DROP INDEX dishes_price_idx;
DROP INDEX dishes_name_idx;
DROP INDEX dishes_name_trgm_idx;
DROP INDEX dishes_search_idx;
DROP FUNCTION dish_search_vector(text, text);
DROP EXTENSION IF EXISTS pg_trgm;
