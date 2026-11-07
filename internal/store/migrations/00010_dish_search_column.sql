-- +goose Up
-- Search scanned every dish of the restaurant, re-parsing its text with
-- to_tsvector each time: under Row-Level Security the GIN index on
-- dish_search_vector(...) can't be used, because @@ and pg_trgm's <% are not
-- LEAKPROOF, so Postgres won't evaluate them before the tenant policy (see
-- docs/performance.md). Storing the vector keeps the per-tenant scan but
-- does the parsing once per write instead of once per row per search.
ALTER TABLE dishes
    ADD COLUMN search tsvector NOT NULL
    GENERATED ALWAYS AS (dish_search_vector(name, description)) STORED;

-- Same index, now on the column: still used by queries without RLS
-- (reports, psql as the owner).
DROP INDEX dishes_search_idx;
CREATE INDEX dishes_search_idx ON dishes USING gin (search);

-- +goose Down
DROP INDEX dishes_search_idx;
ALTER TABLE dishes DROP COLUMN search;
CREATE INDEX dishes_search_idx ON dishes USING gin (dish_search_vector(name, description));
