-- Every query names its restaurant explicitly. Row-Level Security is a
-- second, independent guard, not a replacement for these conditions.

-- name: ListDishes :many
SELECT *
FROM dishes
WHERE restaurant_id = $1
ORDER BY id;

-- name: GetDish :one
SELECT *
FROM dishes
WHERE restaurant_id = $1 AND id = $2;

-- name: CreateDish :one
INSERT INTO dishes (restaurant_id, name, description, price_amount, price_currency, image_url)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateDish :one
UPDATE dishes
SET name           = $3,
    description    = $4,
    price_amount   = $5,
    price_currency = $6,
    image_url      = $7,
    updated_at     = now()
WHERE restaurant_id = $1 AND id = $2
RETURNING *;

-- name: DeleteDish :execrows
DELETE FROM dishes
WHERE restaurant_id = $1 AND id = $2;

-- name: CountDishes :one
SELECT count(*)
FROM dishes
WHERE restaurant_id = $1;
