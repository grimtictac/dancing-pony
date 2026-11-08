# ADR-002: Opaque sessions, not JWTs

- **Status:** Accepted
- **Date:** 2026-11-08

## Context

Every endpoint except registration, login and the health checks needs a
logged-in user. The two usual ways to carry that login between requests are:

- **A JWT:** a signed token that contains the user's ID. The server checks the
  signature and trusts the contents, with no lookup.
- **An opaque session token:** a random string that means nothing by itself.
  The server looks it up to find the user.

## Decision

Opaque session tokens.

- Login returns 256 bits from `crypto/rand`, sent by the client as
  `Authorization: Bearer <token>`.
- Postgres stores only the token's SHA-256, so a database leak doesn't hand out
  working tokens. A fast hash is enough here: the token is random, unlike a
  password, so it can't be guessed offline.
- Sessions last 30 days. Logout deletes the row, which revokes the session at
  once.
- Clients must treat the token as opaque. Switching to JWTs later would only
  change the code that issues and checks tokens.

## Why not JWTs

- **Revocation.** A JWT stays valid until it expires. Making logout, password
  changes or a banned account take effect means short expiries plus refresh
  tokens, and refresh tokens have to be stored and revocable. That rebuilds
  server-side session state, with more moving parts.
- **What JWTs are good at doesn't apply.** Checking a token without a lookup
  pays off when many services, or third parties, must verify it. Here one
  binary on one box issues and checks every token.
- **Multiple instances don't need JWTs either.** Sessions live in the shared
  Postgres, which every instance can read.

## Consequence: knowing who is calling costs a lookup

A JWT tells the server who is calling for free. An opaque token doesn't: every
authenticated request looks its session up in Postgres before doing anything
else. In production that is one of six database round trips per request
(session, restaurant, `BEGIN`, set tenant, query, `COMMIT`).

This becomes sharp with **per-user rate limiting** (Task 4). A rate limiter's
job under abuse is to refuse excess requests cheaply. But a per-user limit
has to know the user first, so without help every request, including those
about to be refused with `429`, would cost a database round trip just to find
out who is asking. The abuse would land on the database, the component the
limiter is meant to protect.

So rejecting JWTs moved the cost of identity onto every request, and the
limiter makes that cost matter. The plan is to pay it back with a cache.

### Options for the rate limiter's key

| Key | Needs a lookup? | Limits per | Notes |
|---|---|---|---|
| User ID, looked up every time | Yes, every request | User | Correct, but abuse costs a database round trip each |
| User ID, from an in-memory session cache | Only on a cache miss | User | **Chosen.** Cheap rejections; revocation needs care (below) |
| User ID, from a JWT | No | User | The option rejected above |
| The token hash itself | No | Session | No lookup at all, but someone with several sessions gets several allowances |

Keying on the token hash is a legitimate cheap alternative and worth naming.
It was not chosen because the brief asks for a limit **per logged-in
customer**. Opening more sessions would multiply the allowance, although each
new session costs a login, and login is already throttled per email and per IP.

### The cache brings back part of the JWT problem

A cached session can outlive its database row. That is exactly the
revocation problem that ruled out JWTs, so the cache must not reintroduce it:

- **One process (today):** logout deletes the row *and* evicts the cache
  entry, so revocation is still immediate. Entries also expire after a short
  time (seconds to a minute), which bounds any other way a session can end,
  such as expiry.
- **Several instances (later):** an eviction on one instance doesn't reach the
  others. The options are a short cache lifetime (revocation takes up to that
  long), broadcasting evictions (for example Redis pub/sub or Postgres
  `LISTEN/NOTIFY`), or moving both the cache and the rate-limit counters to a
  shared store such as Redis.

Unknown or invalid tokens are not cached. Those requests still reach the
database, and the per-IP throttle on login covers token guessing.

### Why in memory, not Redis

The app runs as one process, and the login throttle already keeps its state in
memory. A Redis round trip costs about the same as the Postgres one it would
replace. Redis earns its place when there is more than one instance, and then
the session cache and the rate-limit counters move to it together.
