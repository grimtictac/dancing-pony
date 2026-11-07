-- +goose Up
-- One review per user per dish: a 1-5 rating plus optional text. The text's
-- sentiment is scored when the review is written, and the scorer's model
-- name is stored with it.
CREATE TABLE reviews (
    restaurant_id   uuid     NOT NULL,
    id              uuid     NOT NULL DEFAULT uuidv7(),
    dish_id         uuid     NOT NULL,
    -- References users once they exist (Round 4).
    user_id         uuid     NOT NULL,
    rating          smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body            text     NOT NULL DEFAULT '' CHECK (length(body) <= 2000),
    sentiment_score real     CHECK (sentiment_score BETWEEN -1 AND 1),
    sentiment_label text     CHECK (sentiment_label IN ('positive', 'neutral', 'negative')),
    sentiment_model text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (restaurant_id, id),
    -- One review per user per dish; also serves "reviews of this dish".
    UNIQUE (restaurant_id, dish_id, user_id),
    -- Composite: a review can only point at a dish in its own restaurant.
    FOREIGN KEY (restaurant_id, dish_id) REFERENCES dishes (restaurant_id, id) ON DELETE CASCADE
);

ALTER TABLE reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE reviews FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON reviews
    USING (restaurant_id = current_restaurant_id())
    WITH CHECK (restaurant_id = current_restaurant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON reviews TO pony_app;

-- +goose Down
DROP TABLE reviews;
