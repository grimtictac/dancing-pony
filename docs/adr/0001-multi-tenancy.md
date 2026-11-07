# ADR-001: Multi-tenancy model

- **Status:** Accepted
- **Date:** 2026-11-07

## Context

The API serves more than one restaurant. The Orc Shack from the brief is the first tenant, and every restaurant's dishes (and later reviews, staff and customers) must be invisible to every other restaurant. Adding tenancy after the fact means touching every table, query and permission check. A single forgotten `WHERE restaurant_id = …` would leak another restaurant's data. So tenancy is designed in from the start, even while the brief only describes one restaurant.

## Decision

### Shared database, shared schema

Every tenant-owned table has a `restaurant_id` column. Schema-per-tenant and database-per-tenant isolate more strongly, but they multiply migrations, connections and operational work, with no benefit at this scale.

### Keys that keep rows in their tenant

- Primary keys lead with the tenant: `dishes` is keyed `(restaurant_id, id)`.
- Tables that reference a dish (reviews, from Round 2) will use the composite foreign key `(restaurant_id, dish_id) → dishes (restaurant_id, id)`. The database then rejects a review that points at another restaurant's dish.
- The same technique ties each price to its restaurant's currency: `(restaurant_id, price_currency) → restaurants (id, currency)`.

### Row-Level Security as a second, independent guard

Every query names its restaurant explicitly. Postgres Row-Level Security (RLS) also enforces isolation, so a query that forgets the condition still can't cross tenants:

```sql
ALTER TABLE dishes ENABLE ROW LEVEL SECURITY;
ALTER TABLE dishes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON dishes
    USING (restaurant_id = current_restaurant_id())
    WITH CHECK (restaurant_id = current_restaurant_id());
```

- **Setting the tenant:** each store operation runs in a transaction that begins with `SELECT set_config('app.restaurant_id', $1, true)`. The `true` limits the setting to that transaction, so a pooled connection can never carry one request's tenant into the next. (`SET LOCAL` would do the same, but it can't take a bind parameter.)
- **`current_restaurant_id()`** returns `NULL` when nothing is set, and `restaurant_id = NULL` matches no rows. Forgetting to set the tenant therefore returns nothing rather than everything: it **fails closed**.
- **`USING`** limits which rows can be read, updated and deleted. **`WITH CHECK`** stops a row being inserted into, or moved to, another restaurant.
- **`FORCE`** applies the policy to the table owner as well.

### Database roles

RLS doesn't restrict superusers, roles with `BYPASSRLS` or, without `FORCE`, a table's owner. So there are two roles:

| Role | Used by | Can |
|------|---------|-----|
| Schema owner (e.g. the managed database's admin user) | `pony migrate`, `pony seed` | Create and alter tables and policies |
| `pony_app` | `pony serve` | Read the restaurant directory; read and write tenant tables, subject to RLS. Owns nothing |

`pony serve` checks its role at start-up and **refuses to run** as a superuser, a `BYPASSRLS` role, or a role that owns, or can act as the owner of, a table with RLS enabled. An owner could otherwise drop the policies. Pointing the API at the wrong credentials is a start-up error, not a silent hole.

The first migration creates `pony_app` without a password. Each environment gives it a login out of band. Locally, the Compose init script does it from `.env`.

### Resolving the tenant

The restaurant comes from the URL path: `/restaurants/{slug}/dishes/…`. Middleware looks up the slug (an unknown slug is a 404 `restaurant not found`), puts the restaurant in the request context, and passes it to the handler, which scopes every store call to it. Subdomains (`orc-shack.example.com`) were rejected for now: they need a wildcard TLS certificate, and therefore DNS-01 validation, which the current DNS host doesn't support.

The restaurant directory (`restaurants`) is readable by `pony_app`, because the slug must be resolved before any tenant is set. `pony_app` can't change it. Creating restaurants is an operator task (`pony seed` today; `pony admin` in Round 4).

### Identifiers

Rows have UUIDv7 primary keys. They're time-ordered, so they index well, and unguessable (about 74 random bits). In URLs they appear as 22-character base62 strings rather than 36-character UUIDs (`internal/id`). The encoding is fixed-length and order-preserving, and decoding is strict, so every ID has exactly one accepted spelling.

## Consequences

- **A dish of another restaurant is indistinguishable from a missing dish.** It gets the same 404 with the same body, so the API doesn't even confirm that it exists.
- **Every new tenant-owned table needs** a `restaurant_id`, a tenant-first primary key, a policy like the one above, grants to `pony_app`, and a place in the isolation tests. The HTTP isolation suite fails for any tenant route it doesn't cover.
- **Each operation is a short transaction plus one `set_config` round trip.** That's cheap next to the queries; Round 6 measures it.
- **Cross-tenant work** (platform admin, reporting) needs an explicit, separate path, such as a different role or a deliberately unscoped query that is visible in code review. It can't happen by accident.
- **Postgres 18 is required,** for the built-in `uuidv7()` default. An earlier version would need a polyfill function.

## Verification

- `internal/store/rls_test.go` uses raw SQL **with no `WHERE restaurant_id` at all** to show that RLS alone:
  - hides everything when no tenant is set;
  - shows a tenant only its own rows;
  - blocks inserts into, and moves to, another tenant;
  - limits unscoped `UPDATE` and `DELETE` to the tenant;
  - forgets the tenant when the transaction ends, even on the same connection;
  - binds the owner too, because of `FORCE`.

  Weakening the policy to `USING (true)` makes five of these tests fail.
- `internal/httpapi/isolation_test.go` attacks every tenant route over HTTP, from green-dragon against an orc-shack dish. Forcing every request to resolve to orc-shack makes six subtests fail.
- Migrations run in tests as a non-superuser owner with `CREATEROLE`, like a managed database's admin user. So the tests exercise production's privileges, not a superuser's.
- The Bruno collection's **Tenancy** folder shows the same thing interactively.

### Seeing it yourself

```sh
cp .env.example .env && make db
docker compose exec postgres psql -U pony_app -d dancingpony
```

```sql
SELECT count(*) FROM dishes;                    -- 0: no tenant set, fails closed
BEGIN;
SELECT set_config('app.restaurant_id', (SELECT id::text FROM restaurants WHERE slug = 'orc-shack'), true);
SELECT name FROM dishes;                        -- only The Orc Shack's dishes
DELETE FROM dishes;                             -- deletes only The Orc Shack's...
ROLLBACK;                                       -- ...and undoes it
```
