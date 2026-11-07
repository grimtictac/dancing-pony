# The Orc Shack API: plan

This plan covers all four tasks in the brief, plus observability. Work is delivered in **rounds**. Each round is a series of small commits, and each commit builds and passes the tests.

## Guiding decisions

| Area | Decision | Why |
|------|----------|-----|
| Language | Go (latest stable), standard library `net/http` routing | Matches the team's stack; no framework needed for the routing we have |
| Persistence | PostgreSQL, `pressly/goose` migrations built into the binary | One binary handles serving, migrations and admin set-up |
| Query layer | `sqlc` + `pgx` | Plain SQL files you can run in `psql`, with type-safe generated Go |
| Multi-tenancy | Shared schema, `restaurant_id` on every tenant-owned table, composite foreign keys, Postgres Row-Level Security ([ADR-001](docs/adr/0001-multi-tenancy.md)) | Each restaurant's data is isolated by the database itself, not just by the application code |
| Tenant routing | Path-based: `/restaurants/{slug}/…` | Explicit and easy to test; no wildcard TLS certificate needed |
| Auth | Our own registration, login and session flows built on vetted libraries: argon2id, `crypto/rand`, `x/oauth2`, `go-oidc` | The brief asks for these flows; all cryptography comes from standard, audited libraries |
| Sessions | Random opaque tokens sent as `Authorization: Bearer`, stored hashed in Postgres | Can be revoked immediately; a single service doesn't need JWTs |
| Money | Integer minor units plus an ISO 4217 currency code | Floating-point numbers can't represent money exactly |
| Errors | RFC 9457 `application/problem+json` | A standard, machine-readable error format |
| Images | Uploaded and validated by the API; stored in S3-compatible storage (MinIO locally, DigitalOcean Spaces in production) | Size and type checks; images are re-encoded, which strips metadata such as GPS location |
| Sentiment | `SentimentScorer` interface; a word-list scorer by default | Testable and deterministic; an LLM scorer can be plugged in later |
| API collection | Bruno, stored in the repo under `bruno/` | Requests are versioned with the code and double as smoke tests |

## Security notes

- **Tenant isolation:** a dedicated test suite checks that no endpoint can read or write another restaurant's data.
- **Brute-force defence:** failed logins are throttled per email, per IP address and per email+IP pair, with responses of `429` plus `Retry-After`. Accounts are not locked, because locking would let an attacker lock real users out.
- **No account enumeration:** login uses the same error message for unknown emails and wrong passwords, and checks a dummy hash for unknown users so response times match.
- **Password hashes:** stored in the self-describing PHC format and parsed strictly; anything malformed is rejected (fail closed). Password length limits are tested at the boundaries. This draws on recent OpenSSH issues: CVE-2016-6210 (timing) and CVE-2026-123158 (hash-format confusion).
- **Client IP behind the proxy:** the app trusts `X-Real-IP` / `X-Forwarded-For` only when the request comes from a known proxy. Tests show that a spoofed header cannot bypass rate limits.
- **HTTP hardening:** server timeouts, request size limits, security headers, a deny-by-default CORS policy, and an internal-only listener for metrics and profiling.
- **Pipeline checks:** `govulncheck`, `gosec`, `gitleaks` and fuzz tests for validators and parsers.

## Observability

- Prometheus metrics per route (rate, errors and latency, labelled by route pattern), plus Go runtime, database pool and business metrics.
- Structured JSON logs (`log/slog`) carrying a request ID.
- Health endpoints `/livez` and `/readyz`.
- Grafana dashboards and alert rules provisioned from the repo, including a login-failure spike alert.

## Roadmap

| Round | Scope | Status |
|-------|-------|--------|
| 0 | Foundations: integer money, project layout, configuration, timeouts, routing, error format, CI, Bruno, hardened Compose file | Done |
| 1 | Postgres, migrations, multi-tenancy with Row-Level Security, tenant-isolation tests | Done |
| 2 | Task 1: search (full-text and typo-tolerant), filters, pagination, reviews and ratings | Planned |
| 3 | Metrics, logs, health checks, Grafana | Planned |
| 4 | Task 2: users, sessions, roles, brute-force defence | Planned |
| 5 | Image upload and blurred placeholder previews | Planned |
| 6 | Task 3: load tests (k6), profiling, measured optimisations | Planned |
| 7 | Task 4: per-user rate limiting, Google sign-in, sentiment scoring and reviewer trust | Planned |
| 8 | Security sweep | Planned |
| 9 | CI/CD deployment and documentation | Planned |

## Trade-offs

- **Login is required everywhere,** including browsing the menu, because the spec requires it (a real restaurant would usually show its menu publicly). Only registration, login, the OAuth endpoints and the health checks are public.
- **Registration returns `409 Conflict` for an already-registered email,** which reveals that the email exists. Per-IP throttling limits abuse; a generic response with email verification is planned as a stretch goal.
- **Sentiment scoring runs synchronously,** because the word-list scorer is fast. A slower scorer such as an LLM would move to background scoring.
