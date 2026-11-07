-- +goose Up
-- Target for dishes' (restaurant_id, price_currency) foreign key, so a price
-- can only be in its restaurant's currency.
ALTER TABLE restaurants ADD CONSTRAINT restaurants_id_currency_key UNIQUE (id, currency);

CREATE TABLE dishes (
    restaurant_id  uuid   NOT NULL,
    id             uuid   NOT NULL DEFAULT uuidv7(),
    name           text   NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    description    text   NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 2000),
    -- Minor units (cents) of price_currency, never floating point.
    price_amount   bigint NOT NULL CHECK (price_amount > 0),
    price_currency text   NOT NULL,
    image_url      text   NOT NULL CHECK (length(btrim(image_url)) BETWEEN 1 AND 2048),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    -- Tenant first: every lookup is within one restaurant, and tables that
    -- reference a dish (reviews) use (restaurant_id, id), so they can never
    -- point at another restaurant's dish.
    PRIMARY KEY (restaurant_id, id),
    -- Also makes the restaurant's currency impossible to change while
    -- dishes are priced in it.
    FOREIGN KEY (restaurant_id, price_currency)
        REFERENCES restaurants (id, currency) ON DELETE CASCADE
);

GRANT SELECT, INSERT, UPDATE, DELETE ON dishes TO pony_app;

-- +goose Down
DROP TABLE dishes;
ALTER TABLE restaurants DROP CONSTRAINT restaurants_id_currency_key;
