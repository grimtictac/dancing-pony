-- Users and sessions are global, not tenant-scoped: no restaurant is set.

-- name: CreateUser :one
INSERT INTO users (name, email, password_hash)
VALUES ($1, $2, $3)
RETURNING id, name, email, is_platform_admin;

-- name: GetUserByEmail :one
SELECT id, name, email, password_hash, is_platform_admin
FROM users
WHERE email = $1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at)
VALUES ($1, $2, $3);

-- name: GetSessionUser :one
SELECT u.id, u.name, u.email, u.is_platform_admin
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = $1;

-- name: UpsertUser :one
-- Schema owner only (pony seed, pony admin). Keeps an existing user's
-- password; only grants admin, never revokes it.
INSERT INTO users (name, email, password_hash, is_platform_admin)
VALUES ($1, $2, $3, $4)
ON CONFLICT (email) DO UPDATE
SET is_platform_admin = users.is_platform_admin OR EXCLUDED.is_platform_admin
RETURNING id;
