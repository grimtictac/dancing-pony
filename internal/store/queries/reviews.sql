-- name: UpsertReview :one
-- Creates the user's review of a dish, or replaces it: one per user per dish.
INSERT INTO reviews (restaurant_id, dish_id, user_id, rating, body,
                     sentiment_score, sentiment_label, sentiment_model)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (restaurant_id, dish_id, user_id) DO UPDATE
SET rating          = EXCLUDED.rating,
    body            = EXCLUDED.body,
    sentiment_score = EXCLUDED.sentiment_score,
    sentiment_label = EXCLUDED.sentiment_label,
    sentiment_model = EXCLUDED.sentiment_model,
    updated_at      = now()
RETURNING *;

-- name: DeleteReview :execrows
DELETE FROM reviews
WHERE restaurant_id = $1 AND dish_id = $2 AND user_id = $3;

-- name: ListReviews :many
SELECT *
FROM reviews
WHERE restaurant_id = $1 AND dish_id = $2
ORDER BY id DESC
LIMIT $3;

-- name: DishExists :one
SELECT EXISTS (SELECT 1 FROM dishes WHERE restaurant_id = $1 AND id = $2);
