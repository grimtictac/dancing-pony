-- name: SetTenant :exec
-- Scopes Row-Level Security to one restaurant until the transaction ends.
SELECT set_config('app.restaurant_id', sqlc.arg(restaurant_id)::uuid::text, true);
