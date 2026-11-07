-- Every query names its restaurant explicitly. Row-Level Security is a
-- second, independent guard, not a replacement for these conditions.

-- name: CreateDish :one
INSERT INTO dishes (restaurant_id, name, description, price_amount, price_currency, image_url)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: UpdateDish :one
UPDATE dishes
SET name           = $3,
    description    = $4,
    price_amount   = $5,
    price_currency = $6,
    image_url      = $7,
    updated_at     = now()
WHERE restaurant_id = $1 AND id = $2
RETURNING id;

-- name: DeleteDish :execrows
DELETE FROM dishes
WHERE restaurant_id = $1 AND id = $2;

-- name: CountDishes :one
SELECT count(*)
FROM dishes
WHERE restaurant_id = $1;
