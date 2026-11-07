-- +goose Up
-- One photo per dish, uploaded through the API. The image itself lives in
-- photo storage under key; this row holds what clients need without
-- fetching it: dimensions (to reserve layout space) and two placeholders.
CREATE TABLE dish_photos (
    restaurant_id uuid        NOT NULL,
    dish_id       uuid        NOT NULL,
    key           text        NOT NULL UNIQUE CHECK (key ~ '^[0-9A-Za-z]{22}/[0-9A-Za-z]{22}\.jpg$'),
    width         integer     NOT NULL CHECK (width > 0),
    height        integer     NOT NULL CHECK (height > 0),
    thumbhash     text        NOT NULL CHECK (length(thumbhash) <= 64),
    data_url      text        NOT NULL CHECK (length(data_url) <= 4096),
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (restaurant_id, dish_id),
    -- Composite: a photo can only belong to a dish in its own restaurant.
    FOREIGN KEY (restaurant_id, dish_id) REFERENCES dishes (restaurant_id, id) ON DELETE CASCADE
);

ALTER TABLE dish_photos ENABLE ROW LEVEL SECURITY;
ALTER TABLE dish_photos FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dish_photos
    USING (restaurant_id = current_restaurant_id())
    WITH CHECK (restaurant_id = current_restaurant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON dish_photos TO pony_app;

-- +goose Down
DROP TABLE dish_photos;
