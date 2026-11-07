-- name: GetDishPhotoKey :one
-- FOR UPDATE: two uploads to the same dish replace each other in turn,
-- each learning which file it replaced.
SELECT key
FROM dish_photos
WHERE restaurant_id = $1 AND dish_id = $2
FOR UPDATE;

-- name: UpsertDishPhoto :exec
INSERT INTO dish_photos (restaurant_id, dish_id, key, width, height, thumbhash, data_url)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (restaurant_id, dish_id) DO UPDATE
SET key        = EXCLUDED.key,
    width      = EXCLUDED.width,
    height     = EXCLUDED.height,
    thumbhash  = EXCLUDED.thumbhash,
    data_url   = EXCLUDED.data_url,
    created_at = now();

-- name: DeleteDishPhoto :one
DELETE FROM dish_photos
WHERE restaurant_id = $1 AND dish_id = $2
RETURNING key;
