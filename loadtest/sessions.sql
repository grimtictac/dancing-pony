-- Sessions for the 1,000 load-test users, so a production load test spreads
-- across many customers, as real traffic does, instead of running into one
-- customer's rate limit. Runs as pony_app (users and sessions aren't
-- tenant-scoped):
--
--   psql "$PONY_DATABASE_URL" -v prefix=SECRET < loadtest/sessions.sql
--
-- Token n is "SECRET-n". Only its SHA-256 hash is stored, as for real
-- sessions. Keep the prefix secret and out of git (loadtest/.token-prefix):
-- anyone with it can act as these users. The sessions expire in 7 days.
INSERT INTO sessions (token_hash, user_id, expires_at)
SELECT sha256(convert_to(:'prefix' || '-' || n, 'UTF8')), id, now() + interval '7 days'
FROM (
    SELECT id, row_number() OVER (ORDER BY email) AS n
    FROM users
    WHERE email LIKE 'loadtest-%@shire.example'
) u
ON CONFLICT (token_hash) DO NOTHING;
