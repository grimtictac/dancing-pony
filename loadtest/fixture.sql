-- Bulk data for load tests. Creates a separate restaurant, load-test, with
-- 2,000 dishes, and 1,000 users who between them write 80,000 reviews (40 per
-- dish). A menu this size makes the list, search and rating queries do real
-- work, and keeping it in its own tenant leaves the demo restaurants as they
-- are.
--
-- Runs as the schema owner: the load stack's superuser (make load-up), or
-- doadmin in production (make load-fixture-prod). Where the owner is subject
-- to Row-Level Security (FORCE, no BYPASSRLS), the block needs the tenant
-- set, so it sets it like the API does. Safe to run twice: it stops if the
-- restaurant already exists.
DO $$
DECLARE
    r restaurants;
BEGIN
    IF EXISTS (SELECT 1 FROM restaurants WHERE slug = 'load-test') THEN
        RAISE NOTICE 'load-test data already present';
        RETURN;
    END IF;

    INSERT INTO restaurants (slug, name) VALUES ('load-test', 'The Prancing Pony (load test)')
    RETURNING * INTO r;
    PERFORM set_config('app.restaurant_id', r.id::text, true);

    -- These users never log in; 'x' is not a valid hash, so login fails closed.
    INSERT INTO users (email, password_hash)
    SELECT format('loadtest-%s@shire.example', g), 'x'
    FROM generate_series(1, 1000) g
    ON CONFLICT DO NOTHING;

    INSERT INTO dishes (restaurant_id, name, description, price_amount, price_currency, image_url)
    SELECT r.id,
           format('%s %s #%s',
                  (ARRAY['Mushroom', 'Seed', 'Honey', 'Lembas', 'Rabbit', 'Potato', 'Bacon', 'Apple', 'Cheese', 'Onion'])[1 + g % 10],
                  (ARRAY['Pie', 'Cake', 'Stew', 'Bread', 'Tart', 'Soup', 'Roast', 'Pudding'])[1 + (g / 10) % 8],
                  g),
           'A hearty dish from the Shire, cooked slowly and served warm with plenty of butter.',
           1000 + (g * 37) % 20000,
           r.currency,
           'https://example.com/dish.jpg'
    FROM generate_series(1, 2000) g;

    WITH d AS (
        SELECT id, row_number() OVER (ORDER BY id) AS n FROM dishes WHERE restaurant_id = r.id
    ), u AS (
        SELECT id, row_number() OVER (ORDER BY id) AS n FROM users WHERE email LIKE 'loadtest-%'
    )
    INSERT INTO reviews (restaurant_id, dish_id, user_id, rating, body)
    SELECT r.id, d.id, u.id, 1 + (d.n * 7 + u.n * 3) % 5, 'Really tasty, would eat again.'
    FROM d JOIN u ON (d.n + u.n) % 25 = 0;
END $$;

ANALYZE restaurants, users, dishes, reviews;

SELECT name AS restaurant, (SELECT count(*) FROM users WHERE email LIKE 'loadtest-%') AS load_users
FROM restaurants WHERE slug = 'load-test';
