-- +goose Up
-- Every dish read computed its rating from its reviews (count + avg over
-- ~40 index entries per dish, x20 per page): 25x the cost of the page
-- itself (docs/performance.md). Instead, each dish keeps a running count
-- and sum, maintained by a trigger in the same transaction as the review
-- write, so it is never stale. The average is derived when read.
ALTER TABLE dishes
    ADD COLUMN rating_count integer NOT NULL DEFAULT 0 CHECK (rating_count >= 0),
    ADD COLUMN rating_sum   integer NOT NULL DEFAULT 0 CHECK (rating_sum >= 0);

-- Runs as the user writing the review (pony_app), so Row-Level Security
-- still confines it to that review's restaurant.
-- +goose StatementBegin
CREATE FUNCTION dishes_rating_sync() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        UPDATE dishes
        SET rating_count = rating_count - 1, rating_sum = rating_sum - OLD.rating
        WHERE restaurant_id = OLD.restaurant_id AND id = OLD.dish_id;
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        UPDATE dishes
        SET rating_count = rating_count + 1, rating_sum = rating_sum + NEW.rating
        WHERE restaurant_id = NEW.restaurant_id AND id = NEW.dish_id;
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd

CREATE TRIGGER reviews_rating_sync
    AFTER INSERT OR DELETE OR UPDATE OF rating, dish_id ON reviews
    FOR EACH ROW EXECUTE FUNCTION dishes_rating_sync();

-- Backfill. The owner is subject to Row-Level Security (FORCE), which
-- would hide every row here, so lift it for this statement only.
ALTER TABLE dishes NO FORCE ROW LEVEL SECURITY;
ALTER TABLE reviews NO FORCE ROW LEVEL SECURITY;
UPDATE dishes d
SET rating_count = r.n, rating_sum = r.total
FROM (SELECT restaurant_id, dish_id, count(*) AS n, sum(rating) AS total
      FROM reviews GROUP BY restaurant_id, dish_id) r
WHERE d.restaurant_id = r.restaurant_id AND d.id = r.dish_id;
ALTER TABLE dishes FORCE ROW LEVEL SECURITY;
ALTER TABLE reviews FORCE ROW LEVEL SECURITY;

-- +goose Down
DROP TRIGGER reviews_rating_sync ON reviews;
DROP FUNCTION dishes_rating_sync();
ALTER TABLE dishes DROP COLUMN rating_count, DROP COLUMN rating_sum;
