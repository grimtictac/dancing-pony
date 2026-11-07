-- +goose Up
-- The tenant directory. Every tenant-owned table carries a restaurant_id.
CREATE TABLE restaurants (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    slug       text NOT NULL UNIQUE
               CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) <= 63),
    name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    -- ISO 4217 code; every price at this restaurant is in this currency.
    currency   text NOT NULL DEFAULT 'ZAR' CHECK (currency ~ '^[A-Z]{3}$'),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- The API resolves the slug in the URL to a restaurant before any tenant is
-- set, so it may read the directory, but not change it.
GRANT SELECT ON restaurants TO pony_app;

-- +goose Down
DROP TABLE restaurants;
