-- Bulk data for load tests (make load-up). Runs as the database superuser,
-- which Row-Level Security doesn't restrict. Only for the throwaway load
-- stack: never run it against a real database.
--
-- Adds to orc-shack: 2,000 dishes and 1,000 users who between them write
-- 80,000 reviews (40 per dish). A menu this size makes the list, search
-- and rating queries do real work.
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

DO $$
DECLARE
    shack restaurants;
BEGIN
    SELECT * INTO STRICT shack FROM restaurants WHERE slug = 'orc-shack';
    IF EXISTS (SELECT 1 FROM users WHERE email LIKE 'loadtest-%') THEN
        RAISE NOTICE 'load-test data already present';
        RETURN;
    END IF;

    -- These users never log in; 'x' is not a valid hash, so login fails closed.
    INSERT INTO users (email, password_hash)
    SELECT format('loadtest-%s@shire.example', g), 'x'
    FROM generate_series(1, 1000) g;

    INSERT INTO dishes (restaurant_id, name, description, price_amount, price_currency, image_url)
    SELECT shack.id,
           format('%s %s #%s',
                  (ARRAY['Mushroom', 'Seed', 'Honey', 'Lembas', 'Rabbit', 'Potato', 'Bacon', 'Apple', 'Cheese', 'Onion'])[1 + g % 10],
                  (ARRAY['Pie', 'Cake', 'Stew', 'Bread', 'Tart', 'Soup', 'Roast', 'Pudding'])[1 + (g / 10) % 8],
                  g),
           'A hearty dish from the Shire, cooked slowly and served warm with plenty of butter.',
           1000 + (g * 37) % 20000,
           shack.currency,
           'https://example.com/dish.jpg'
    FROM generate_series(1, 2000) g;

    WITH d AS (
        SELECT id, row_number() OVER (ORDER BY id) AS n FROM dishes
        WHERE restaurant_id = shack.id AND name LIKE '% #%'
    ), u AS (
        SELECT id, row_number() OVER (ORDER BY id) AS n FROM users WHERE email LIKE 'loadtest-%'
    )
    INSERT INTO reviews (restaurant_id, dish_id, user_id, rating, body)
    SELECT shack.id, d.id, u.id, 1 + (d.n * 7 + u.n * 3) % 5, 'Really tasty, would eat again.'
    FROM d JOIN u ON (d.n + u.n) % 25 = 0;
END $$;

ANALYZE;

SELECT (SELECT count(*) FROM dishes) AS dishes, (SELECT count(*) FROM reviews) AS reviews;
