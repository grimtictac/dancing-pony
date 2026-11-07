-- name: GetMembershipRole :one
SELECT role
FROM memberships
WHERE restaurant_id = $1 AND user_id = $2;

-- name: UpsertMembership :exec
-- Schema owner only (pony admin grant, pony seed).
INSERT INTO memberships (restaurant_id, user_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (restaurant_id, user_id) DO UPDATE SET role = EXCLUDED.role;

-- name: GetUserIDByEmail :one
SELECT id FROM users WHERE email = $1;
