-- name: GetRestaurantBySlug :one
SELECT id, slug, name, currency
FROM restaurants
WHERE slug = $1;

-- name: UpsertRestaurant :one
-- Schema owner only (pony seed): pony_app may not change the directory.
-- The no-op DO UPDATE makes RETURNING give the existing row on a conflict.
INSERT INTO restaurants (slug, name, currency)
VALUES ($1, $2, $3)
ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
RETURNING id, slug, name, currency;
