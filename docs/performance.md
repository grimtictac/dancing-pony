# Performance (Task 3)

Measured first, then fixed only what the measurements pointed at.

## Set-up

- **"Shire server":** the app container is limited to 1 vCPU and 512 MB,
  the size of the production droplet. Postgres gets 1 vCPU and 1 GB, the
  smallest managed plan. Both run in an isolated compose stack
  (`make load-up`).
- **Data:** a separate `load-test` restaurant with 2,000 dishes, 1,000 users and
  80,000 reviews, 40 per dish (`loadtest/fixture.sql`).
- **Load:** k6 (`loadtest/browse.js`), a logged-in customer at a fixed
  arrival rate. 50% list a page (random sort), 30% search (including the
  typo "mushrom"), 20% open a dish. A fixed arrival rate means an overloaded
  server shows up as latency and dropped iterations, not as a politely lower
  request rate.
- **Tools:**
  - pprof on the internal listener (`/debug/pprof`, `127.0.0.1:9091`);
  - `pg_stat_statements` in the load stack (`make load-stats`);
  - `EXPLAIN ANALYZE` run as `pony_app` with the tenant set, so Row-Level
    Security applies exactly as it does in the app.
- The load generator runs on the same laptop, outside the containers'
  limits.

```sh
make load-up                          # build and start, load the data
make load-test RATE=600 DURATION=30s  # browse at 600 requests/s
make load-stats                       # where Postgres spent its time
make load-down
```

## Results

95th-percentile latency at a given offered rate, and the highest rate
served (the point where k6 starts dropping iterations).

| | list | search | dish | max served |
|---|---|---|---|---|
| **Baseline**, 100 req/s | 2.4 ms | 32 ms | 1.2 ms | **~250 req/s** |
| Baseline, 300 req/s (overloaded) | 1.29 s | 1.46 s | 1.30 s | |
| **1. Stored search vector**, 300 req/s | 2.0 ms | 9.2 ms | 0.9 ms | **~590 req/s** |
| **2 + 3. Full-text first; rating totals**, 300 req/s | 1.3 ms | 2.3 ms | 0.9 ms | |
| Same, 1,200 req/s | 1.1 ms | 1.7 ms | 0.7 ms | **~1,440 req/s** |

That's **about 5.8× the throughput**, with search latency at the old load
level down about 14× (32 ms → 2.3 ms). There were no errors in any run.

## What the measurements showed

### The bottleneck was Postgres, not Go

- Under load, the app container used about 11% of its CPU while Postgres sat
  at 100% of its one.
- The Go profile was mostly syscalls and pgx encoding/decoding: nothing to
  optimise in Go first.

### Search used about 90% of the database's time

- `pg_stat_statements`: the search query was 1,811 of 6,000 statements but
  20.7 of about 23 seconds of execution, at 11 ms a call.
- `EXPLAIN ANALYZE` showed why. Both GIN indexes (full-text and trigram) go
  unused, and every one of the restaurant's dishes is scanned, re-running
  `to_tsvector` on its text.

**Row-Level Security disables the search indexes.** Postgres only evaluates
a condition before the RLS policy (so it can use an index) if its operator
is `LEAKPROOF`, meaning it can't reveal anything about rows the policy would
hide, even through an error. `@@` (`ts_match_vq`) and pg_trgm's `<%` aren't
leakproof.

- The same query as the table owner, without RLS, uses both indexes and takes
  1.7 ms.
- As `pony_app`, under RLS, it takes 26 ms.

Ways round it, and why they weren't taken:
- **Mark the operators `LEAKPROOF`.** That needs superuser (not available on
  managed Postgres), and it's a promise we can't make for them.
- **Search through a `SECURITY DEFINER` function that bypasses RLS.** That
  gives up the second wall that the multi-tenancy design is built on
  (ADR-001).

So the fixes make the per-tenant scan cheap instead:

1. **Stored search vector** (migration 10): a generated `tsvector` column,
   so the text is parsed once per write, not once per dish per search.
   26 ms → about 4–7 ms per search (EXPLAIN), and **~250 → ~590 req/s**.
2. **Full-text first; fuzzy only as a fallback.** The trigram half cost
   about 3.8 ms against 0.46 ms for full-text. And under the generic plans
   that pgx's prepared statements use, `websearch_to_tsquery($2)` was being
   recomputed for every row (3.4 ms); as `(SELECT websearch_to_tsquery($2))`
   it's computed once (0.3 ms).
   - Search now matches full-text, and only if that finds nothing on the
     menu falls back to a close match on the name. So "mushrom" still finds
     Mushroom Pie.
   - A typo search takes about 0.7 ms and a normal one about 0.5 ms, against
     11 ms before.

### The rating summary cost 25× the page

- Every dish read counted and averaged its reviews: 40 index entries per
  dish, 20 dishes per page.
- A page with ratings took 1.09 ms; the same page without took 0.04 ms.

3. **Rating totals on the dish** (migration 11): `rating_count` and
   `rating_sum`, kept up to date by a trigger on reviews in the same
   transaction, so they can never be stale.
   - The trigger runs as the writing user, so RLS still confines it to that
     restaurant.
   - The migration backfills the totals; on the load data they matched
     exactly (80,000 reviews, rating sum 240,000).
   - The cost moves to review writes (one extra row update each), which are
     rare next to reads. Concurrent reviews of the same dish now queue on
     its row lock, which is negligible at this scale.

## What's left, at ~1,440 req/s

Postgres is still the limit, but no statement dominates any more:

- Search is about 0.9 ms a call and the largest share. It still scans the
  tenant's dishes, which is fine for a menu, but wouldn't be for a catalogue
  of 100,000 items.
- Every request makes **six round trips**: session lookup, restaurant
  lookup, `BEGIN`, set tenant, the query and `COMMIT`. Each is cheap
  (3–35 µs of execution), but the protocol overhead adds up.

Next steps, if needed (not done: measure first):

- Cache the session → user and slug → restaurant lookups in memory for a
  few seconds, so most requests make 4 round trips instead of 6.
- Send `BEGIN; set_config(...)` and the query as one pipelined batch (pgx
  supports batches).
- HTTP caching of list pages (`ETag` / `Cache-Control: private, max-age`)
  would avoid repeat work entirely for the same client.
- At much larger tenants: a search table without RLS, reached only through a
  function that takes the tenant as a required argument, or an external
  search index.

## Production (silson.dev), 8 November 2026

The droplet (1 vCPU, 1 GB, London) runs Nginx and the app; Postgres is
DigitalOcean's managed service on the private network. Load came from a
laptop in South Africa, so every client time includes a ~150 ms round trip;
the app's own time comes from its metrics (`loadtest/latency.py`), and the
host's from Grafana Alloy (`deploy/alloy`). The data is the same fixture, in
its own `load-test` restaurant (`make load-fixture-prod`).

### Two limits that weren't the app

1. **The kernel's connection table.** At ~200 req/s about 1% of requests got
   no answer, and the kernel logged `nf_conntrack: table full, dropping
   packet`. Nginx opened a new connection to the app for every request, and
   each closed one stays in the table (8,192 entries on this droplet) for two
   minutes. **Fix:** an `upstream` block with `keepalive`
   (`deploy/nginx/pony-upstream.conf`), so Nginx reuses a pool of
   connections. With it, the table peaked at about half full even at
   500 req/s, with no drops.
2. **Nginx's connection limit.** At 500 req/s Nginx logged "768
   worker_connections are not enough" and reset connections. Raised to 4096
   in `/etc/nginx/nginx.conf`. This doesn't add capacity; it makes overload
   queue instead of fail.

The firewall (ufw) has no rate limits; it was involved only because its
stateful rules put every connection in the kernel's table.

### Capacity, with keepalive and the cache (60 s steps)

| Offered | Served | k6 errors | Client p95 | Notes |
|---|---|---|---|---|
| 200 req/s | 195 | 0 of 11,981 | 171 ms | |
| 300 req/s | 295 | 0 of 17,957 | 201 ms | |
| 400 req/s | 378 | 0 of 23,346 | 923 ms | saturated: queueing, dropped iterations |
| 500 req/s | | connection resets | | Nginx limit (since raised); CPU ~88% |

**About 300–350 req/s with no errors and a sane tail** on a 1 vCPU droplet.

### The session and restaurant cache (`PONY_CACHE`), 300 req/s for 90 s

Same rate, same data, the app restarted with the cache off, then on:

| | cache off | cache on |
|---|---|---|
| k6 errors | 4 of 26,501 | 0 of 26,939 |
| Dropped iterations | 502 | 63 |
| Client p95 | 1.54 s | 230 ms |
| App time, list: mean / p95 | 233 ms / 1.81 s | 20 ms / 95 ms |
| App time, dish: mean / p95 | 228 ms / 1.81 s | 14 ms / 81 ms |
| Host CPU busy | 45% | 39% |

The cache turns an overloaded server into a comfortable one at this rate:
**about 12× lower mean latency in the app and 19× lower p95.**

Why so much more than "2 round trips out of 6" suggests: with the cache off
the host was only 45% busy, so the app was *waiting*, not computing. Each
request holds a database connection for six round trips across the network,
and the pool is small (pgxpool's default is 4 connections). At 300 req/s
the pool runs out and requests queue for a connection. Taking two round
trips out of every request frees the pool, and the queue disappears.

The next levers, measured in this order if needed: a larger pool
(`pool_max_conns` in the URL, within the managed database's connection
limit), then pipelining `BEGIN`, `set_config` and the query in one batch.

Caveats: the cache serves a session for up to 30 s and a restaurant for 60 s
after a change made outside this process (logout in this process evicts at
once; see ADR-002). One run each, from one client location.

### Per-customer rate limit (8 November, later)

Each logged-in customer may make 30 requests at once, then 10 a second
(token bucket; `PONY_USER_RATE`, `PONY_USER_BURST`). One k6 user at
300 req/s would now measure the limit, not the server, so production runs
use 1,000 customers: `loadtest/sessions.sql` gives the load-test users
sessions whose tokens come from a secret prefix, and `browse.js` picks one
per request when `TOKEN_PREFIX` is set. The local load stack turns the
limit off (`loadtest/compose.yml`).

Checked on production:

- **One customer, 40 requests at once:** 30 answered, 10 refused with `429`
  (`pony_rate_limited_total` 0 → 10).
- **300 req/s for 60 s over 1,000 customers:** 0 errors of 17,796 and no
  refusals. Client p95 was 660 ms, higher than the single-customer run
  (230 ms). Not yet investigated; a likely part is the session cache: 1,000
  sessions with a 30 s lifetime means roughly one request in nine misses and
  goes to the database, against almost none with one customer. One run.
