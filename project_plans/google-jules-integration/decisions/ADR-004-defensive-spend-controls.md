# ADR-004: Defensive spend controls — proactive caps plus a reservation-based idempotency key

**Status**: Accepted
**Date**: 2026-09-01
**Project**: google-jules-integration

## Context

Jules is a metered product. Public pricing (`research/build-vs-buy.md` §2) bundles
it into Google AI subscription tiers — Free: 15 tasks/day, 3 concurrent; Pro
($19.99/mo): 100/day, 15 concurrent; Ultra ($124.99/mo): 300/day, 60 concurrent.
The **API's own** rate limits are not published: the docs mention a `429` but no
thresholds, headers, or backoff guidance. This remained unresolved after Phase 2
and is carried forward as an explicit Unresolved Question in `plan.md`.

What this repo has today (`research/pitfalls.md` §5):

- `github.DefaultRateLimiter` (`github/rate_limit.go:25`) — a good reactive
  pattern (parse response headers, back off once the *server* says so), but purely
  reactive. Nothing prevents 500 create calls before the first `429` returns. For
  GitHub (rate-limited, not billed) that is fine; for a billed API the first sign
  of trouble would be a bill.
- `MaxConcurrentBacklogWorkItems` (`config/config.go:381-384`, hard ceiling 10 at
  `:954`) — the repo's **only** proactive cap, on concurrently in-progress backlog
  items. Structurally the right shape to extend.
- `session/detection/ratelimit/detector.go` — not relevant; it pattern-matches
  *local agent terminal output* for the agent's own provider limits, and has no
  bearing on stapler-squad's outbound calls.

Two concrete abuse paths were identified: a retry loop (stale-work reconciler, a
webhook firing twice, a double-clicked button) spawning duplicate billed
sessions; and a create-then-immediately-complete loop that a concurrency ceiling
alone would not catch because nothing is ever concurrently in flight.

## Decision

Three layers, in order, all enforced **before** any billed API call.

**1. Proactive ceilings, extending `MaxConcurrentBacklogWorkItems`' shape.**

| Setting | Default | Hard ceiling | Guards against |
|---|---|---|---|
| `MaxConcurrentJulesSessions` | 2 | 10 | Fan-out — many items dispatched at once |
| `MaxJulesSessionsPerDay` | 15 | 300 | Creation *rate* — a tight create/complete loop |

Both are clamped by `*OrDefault()` accessors, never read raw, mirroring
`config/config.go:954`. Both are evaluated from durable state — the two narrow
queries `ListOpenJulesItemSessions` and `CountJulesItemSessionsSince(now-24h)` —
so a process restart cannot reset the budget. Defaults are deliberately at or
below Jules' free tier (3 concurrent, 15 tasks/day) so the out-of-box behavior is
safe for the weakest plan; a Pro/Ultra user raises them explicitly.

**2. Reactive backoff on top.** `julesRateLimitTransport` records `429` and
`Retry-After` and exposes `IsLimited()`, which the poller checks to skip an entire
tick rather than firing calls guaranteed to fail — the `github/rate_limit.go`
`WaitIfLimited` pattern, as an `http.RoundTripper` decorator so no endpoint can
forget it.

**3. A reservation row as the idempotency key.** Before the `CreateSession`
POST, `DispatchToJules` writes an `ItemSession` with
`session_uuid = "jules-pending-" + uuid`; after a successful create it swaps in
`"jules-" + sessions/{id}` via the existing `UpdateItemSessionSessionUUID`
(`session/storage.go:1275`). The pre-call duplicate guard is then simply "does
this item already have an open `jules_work` `ItemSession`", closed against
double-clicks by a per-item lock.

Reservations that never got confirmed (process crash between POST and DB write)
are swept by the poller after 10 minutes: the row is ended with reason
`dispatch_incomplete`, the item returns to `ready`, and a **visible progress
note** tells the user a session may exist at jules.google.com — the automation
never fails silently (per the "document AI decisions in edge cases" standing
instruction).

Every rejection is logged as `jules dispatch rejected` with a `reason` field at
`Info` — these are expected user-facing outcomes, not errors — and surfaces to
the caller as `connect.CodeFailedPrecondition` so the UI can explain it inline.
`JulesUsageCounter` totals are surfaced in the settings panel, so spend is
visible in-tool rather than only in log lines nobody reads until there is a bill.

## Alternatives Considered

- **Reactive-only, like `github.DefaultRateLimiter`** — Rejected. Adequate for a
  free API; for a billed one it makes the bill the first signal.
- **Wait for Google to publish real quotas, then tune** — Rejected as a blocker.
  The figures may never be published for an alpha API, and defensive defaults with
  config overrides cost nothing to ship now. This is recorded as a non-blocking
  Unresolved Question with a named owner.
- **Concurrency cap only, no daily cap** — Rejected. A create-then-complete loop
  never exceeds one concurrent session while burning the whole daily quota.
- **A dedicated dedup/idempotency table** — Rejected. A schema migration for
  information the `ItemSession` row already carries; the reservation makes the
  local row the single source of truth for "a dispatch is in flight".
- **Dedup after the fact on the returned `JulesSessionName`** — Rejected. A billed
  session created between the POST and a crashed DB write would be invisible and
  unreconcilable.

## Consequences

**Positive**

- A retry-loop bug costs at most one day's cap, not an unbounded bill.
- No duplicate billed session for the same item, even under a double-click or a
  mid-dispatch crash.
- Every rejection has a specific, user-facing reason rather than a generic error.

**Negative**

- A user on Ultra must raise both defaults before they can use their plan's
  headroom. Accepted: safe-by-default beats convenient-by-default for spend.
- The daily cap needs one extra indexed count query per dispatch. Negligible at
  this scale, and it only runs on the dispatch path, never on the poll path.
- The reservation adds a write before the API call and a swap after — two writes
  per dispatch instead of one. Accepted for the durability it buys.

## References

- `research/pitfalls.md` §5, §6
- `research/build-vs-buy.md` §2
- `config/config.go:381-384`, `:954`
- `github/rate_limit.go:25,153`, `github/http_client.go`
- `session/storage.go:1275`
