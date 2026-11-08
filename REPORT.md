# The Dancing Pony: report for reviewers

A multi-restaurant REST API in Go. The Orc Shack from the brief is the first
tenant; its menu lives at `/restaurants/orc-shack/dishes`.

- **Production:** https://silson.dev
- **Deployment:** [deploy/README.md](deploy/README.md)
- **Decisions:** [ADR-001 multi-tenancy](docs/adr/0001-multi-tenancy.md),
  [ADR-002 sessions, not JWTs](docs/adr/0002-sessions-not-jwts.md)
- **Performance:** [docs/performance.md](docs/performance.md)

## Try it

- **Bruno:** open the `bruno/` collection, choose the `production` (or
  `local`) environment, run **Auth → Log in (Frodo)**, then any request.
  Requests act as whoever logged in last.
- **Demo users** (password `pony-demo-password`, published on purpose):
  `owner@orc-shack.example` (owner), `owner@green-dragon.example` (owner of a
  second restaurant), `frodo@shire.example` and `sam@shire.example`
  (customers).
- **Locally:** `make db` (Postgres, Prometheus and Grafana in Docker,
  migrations and demo data), then `make run`. `make check` runs lint, race
  tests and `govulncheck`; `make api-test` runs the whole Bruno collection
  against a fresh stack.

## What was built

| Brief | Delivered |
|---|---|
| **Task 1:** dishes CRUD; customers search, view and rate | Dishes under each restaurant; full-text search with a typo-tolerant fallback; filters, sorting and keyset pagination; one review per customer per dish, with rating totals on the dish |
| **Task 2:** users, auth, permissions, validation, brute force | Registration and login with argon2id; revocable sessions; per-restaurant roles; strict input validation; login throttling per email, IP and email + IP |
| **Task 3:** performance on the same hardware | Measured, then fixed: about 250 → 1,440 requests/s on a droplet-sized stack |
| **Task 3:** multi-tenant SaaS | Shared schema, composite keys and Postgres Row-Level Security |
| **Task 4:** sentiment | A local word-list scorer behind a swappable interface |
| **Task 4:** per-customer rate limiting | A token bucket per logged-in customer (burst 30, then 10/s), checked right after the cached session lookup; `429` with `Retry-After` |
| **Task 4:** OAuth SSO | **Explicitly not done** (see Trade-offs) |

Beyond the brief: photo upload with blurred placeholders, Prometheus metrics
and alerts, a Grafana dashboard, a k6 load-test harness, and CI that deploys
every green push to production with automatic rollback.

## Key decisions

### Multi-tenancy: two walls, not one

Every query names its restaurant, and Postgres Row-Level Security enforces the
same rule independently, so a query that forgets still can't cross tenants.
The tenant is set per transaction and **fails closed**: if it isn't set, a
query sees nothing. An isolation test calls every tenant route across
restaurants; another restaurant's dish returns a 404 identical to a missing
one. Details: [ADR-001](docs/adr/0001-multi-tenancy.md).

**The cost we found and kept:** under RLS, Postgres can't use the search
indexes, because the full-text and fuzzy-match operators aren't `LEAKPROOF`.
Search scans one restaurant's menu instead. We made that scan cheap (about
0.5–0.9 ms) rather than weaken isolation. It's the right trade for menus; a
tenant with 100,000+ items would need a different design.

### Sessions, not JWTs, and what that costs

> **This is the trade-off with the most consequences.**
>
> We chose opaque, revocable session tokens over JWTs: logout takes effect
> at once, and one service doesn't need self-contained tokens. The price is
> that **the server can't know who is calling without a lookup**, so every
> request asks Postgres.
>
> Per-user rate limiting makes that price matter. To refuse an abusive
> customer cheaply, the limiter must know who they are **before** touching
> the database. So the sessions decision leads directly to an **in-memory
> session cache** as the enabler for the rate limiter.
>
> Two honest caveats:
>
> - **It isn't strictly forced.** Keying the limiter on the token itself
>   needs no lookup, but it limits per session rather than per customer, and
>   the brief asks for per customer.
> - **The cache brings back part of the JWT problem:** a cached session can
>   outlive logout. With one process, logout evicts the entry, so revocation
>   stays immediate. With several instances it would need broadcast
>   evictions or a shared store such as Redis, and that is the point where
>   Redis earns its place.
>
> Full reasoning: [ADR-002](docs/adr/0002-sessions-not-jwts.md).

### Brute force: slow down, never lock out

Locking an account after N failures would let the Evil Orc lock Frodo out.
Instead, failures are counted per email + IP, per email and per IP; after a
few free attempts the wait doubles each time, answered at once with `429` and
`Retry-After`. The per-email wait is capped at 30 seconds, so the real Frodo
is only ever slowed. Client IPs come from proxy headers only when the request
comes from our own Nginx, so faking `X-Forwarded-For` doesn't reset the count.
Unknown emails take the same time as wrong passwords (a dummy hash), so
timing doesn't reveal who has an account.

### Sentiment: a local scorer behind an interface

Reviews call a `sentiment.Scorer` interface. Today it's backed by a **local
word list** (`wordlist-v1`): no API key, no network, deterministic and fast
enough (about 9 µs) to score inline. A cloud NLP API or an LLM can replace it
later by configuration; each score records the model that produced it, so old
and new scores are never mixed. Sentiment measures negativity, not whether a
reviewer is a sockpuppet; that needs trust signals such as account age,
review rate and shared IPs.

### Performance: measure first

On a droplet-sized stack (1 vCPU, 512 MB), Postgres was the bottleneck and
the Go app sat at 11% CPU. Search took 90% of database time, for the RLS
reason above. Three changes, each measured: a stored search vector,
full-text first with fuzzy matching as a fallback, and rating totals kept by
a trigger. Result: about 250 → 1,440 requests/s.
Details: [docs/performance.md](docs/performance.md).

**In production** (1 vCPU droplet, managed Postgres across the network):

- **About 300–350 requests/s with no errors.** Getting there meant fixing
  two limits outside the app, found through host metrics and kernel logs
  shipped to Grafana Cloud: Nginx opened a connection per request, which
  filled the kernel's connection table (fixed with upstream keepalive), and
  Nginx's own connection limit (raised).
- **The session and restaurant cache** (`PONY_CACHE=on|off`, so it can be
  measured): at 300 requests/s, the app's mean time for a menu page fell
  from 233 ms to 20 ms and the client p95 from 1.5 s to 230 ms. With the
  cache off the CPU was half idle: requests were queueing for the small
  database pool, and two fewer round trips per request freed it.

Details: [docs/performance.md](docs/performance.md#production-silsondev-8-november-2026).

**Per-customer rate limit:** each logged-in customer gets a burst of 30
requests, then 10 a second (`PONY_USER_RATE`, `PONY_USER_BURST`). It's
checked straight after the session lookup, which the cache answers from
memory, so turning an abusive customer away costs no database work: the
reason ADR-002 gives for the cache. Refusals are `429` with `Retry-After`,
counted in `pony_rate_limited_total`. Production load tests now spread over
1,000 customers (`loadtest/sessions.sql`), as real traffic would, so they
measure the server rather than one customer's limit.

## Trade-offs and what was left out

- **OAuth: explicitly not done.** It's a whole external dependency (provider
  set-up, redirect flows, linking accounts to existing users) and brings too
  much scope risk for what it adds. Sessions are opaque tokens, so an SSO
  login could later issue the same kind of session.
- **Throttle state and the session cache live in memory:** right for one
  instance; several instances would move them to a shared store.
- **Registration returns `409` for an existing email,** which reveals the
  account exists. It's throttled per IP; hiding it properly needs email
  verification.
- **Login is required even to browse the menu,** as the brief says.
- **Photos are stored on the droplet's disk,** behind an interface that an S3
  implementation could fill.
- **Demo accounts share a published password,** so reviewers can log in.
