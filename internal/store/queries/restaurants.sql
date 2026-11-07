-- name: GetRestaurantBySlug :one
SELECT id, slug, name, currency
FROM restaurants
WHERE slug = $1;
