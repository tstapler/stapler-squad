# ADR-029: `session_role` Is Sourced From a Batched Per-Request Snapshot, Not a Live Join

**Date**: 2026-09-12
**Status**: Accepted

## Context

`SessionTokenSummary.session_role` (new field 22, `proto/session/v1/insights.proto`)
needs to reflect the persisted `ItemSession.session_role` for the session's
stapler-squad `Instance.UUID`. `buildSessionSummary` (`server/services/insights_service.go:82-137`)
runs once per session inside three call sites — `GetInsightsSummary`,
`ListSessionTokens` (bulk, up to every parsed session in the store), and
`watchInsights` (one session per streamed event) — so the sourcing mechanism's
shape has real performance consequences (research/pitfalls.md §2).

Two live-persistence questions bear directly on whether a "batch once per request,
treat as point-in-time fact" design can go stale:

1. **Can a session's role change after its `ItemSession` row is created?**
   Verified via `grep -rn "SetSessionRole" --include='*.go' .` (excluding test
   files): exactly two production call sites, both inside `EntRepository.CreateItemSession`
   (`session/storage_backlog.go:238-251`) and `CreateItemSessionWithVerdict`
   (`session/storage_backlog.go:788-802`) — both `.Create()` builders. **No
   production code path ever calls an `UpdateOne()`/`Update()` mutation on
   `session_role` after creation.** The only other `SetSessionRole` call sites
   in the repo are ent test-builder setup in `session/backlog_lifecycle_test.go`
   and `session/backlog_lifecycle_stuck_test.go`, which construct fixture rows
   directly, not a production mutation path. So the underlying fact ("this
   `ItemSession`'s role") is write-once and immutable for the row's lifetime —
   there is no "role changed after the fact" case to design around.
2. **Can the row disappear?** Yes — `DeleteBacklogItem` (`session/ent_repository_backlog.go:1438-1490`)
   hard-cascades a delete of all `ItemSession` rows for a deleted item
   (research/pitfalls.md §5, architecture.md §2). Archival does not delete the
   row (confirmed independently via the ent schema's edge annotations and the
   status-transition code path).

## Options considered

1. **Live per-session query at build time** (`Storage.GetItemSessionBySessionUUID`,
   one query per session). Rejected: reintroduces the exact N+1 bug class the
   `Associator.Snapshot()` pattern was already built to eliminate for
   `sessionId`/`isOrphan` resolution (research/pitfalls.md §2) — `GetInsightsSummary`
   and `ListSessionTokens` are unfiltered over every parsed session before
   pagination/time filters apply.
2. **Batched snapshot, once per request/event** (chosen). `InsightsService`
   calls `Storage.GetAllItemSessionsWithBacklogInfo(ctx)` once per
   `GetInsightsSummary`/`ListSessionTokens` request (mirroring the existing
   `associator.Snapshot()` prefetch) or once per `watchInsights` streamed
   event (a single extra query per event, not per session — acceptable), folds
   the result into a `map[sessionUUID]role`, and passes that map into
   `buildSessionSummary` for every session in the batch.
3. **Cache the role map across requests** (e.g. TTL cache, invalidated on
   backlog mutation). Rejected as unnecessary complexity: `WatchInsights`
   already has an established staleness profile for every other field
   `buildSessionSummary` computes (architecture.md §6) — `is_orphan` and
   `activity_type` are only recomputed when the session's own JSONL transcript
   changes, not when something external (backlog status, tags) changes
   elsewhere. `session_role` inheriting that same "eventually consistent,
   refreshed on next JSONL write or full page reload" profile is consistent
   with the rest of the message, not a new inconsistency.

## Decision

**Option 2.** Fetch `GetAllItemSessionsWithBacklogInfo` once per
`GetInsightsSummary`/`ListSessionTokens` request and once per `watchInsights`
streamed update, build an in-memory `map[string]string` keyed by session UUID,
and pass it into `buildSessionSummary` as a plain parameter — the same
"prefetch once, associate many" shape `Associator.Snapshot()` already
established for `sessionId`/`isOrphan`. Because a given session UUID can have
more than one `ItemSession` row (`GetItemSessionBySessionUUID`'s own doc
comment: "session_uuid is not unique across records"), the query gets an
explicit `.Order(ent.Desc(itemsession.FieldCreatedAt))` (previously absent —
see plan Task 1.3.1) so the map-building loop can deterministically keep the
first (most recent) role it sees per UUID, rather than depending on
undefined ent iteration order.

`session_role` is therefore treated as a **fact captured once per request/event**,
not a value with independent live-update semantics of its own.

## Consequences

- **No staleness risk from role mutation**: since §Context point 1 shows role
  is never mutated after `ItemSession` creation in production code, "the
  batched snapshot went stale because someone changed the role" is not a real
  failure mode today. If a future change ever adds a role-correction/reassignment
  RPC, this ADR's assumption must be revisited — the batching design would
  still work (same request-scoped staleness profile as everything else in
  `SessionTokenSummary`), but the "immutable once written" framing above would
  no longer hold and should be re-verified.
- **Real staleness vector is deletion, not mutation**: once `DeleteBacklogItem`
  hard-cascades a session's `ItemSession` row, `session_role` permanently
  reads `""` for that session from then on — this is expected data loss (the
  source fact no longer exists anywhere), not a bug in this batching design,
  and should be stated plainly in the PR description (see plan).
- **Performance**: one extra query per bulk request (not O(n) per session) and
  one extra query per `watchInsights` event (acceptable, matches the existing
  per-event `associator.Snapshot()` re-fetch already happening there).
- **Collateral fix**: adding the `.Order()` clause to `GetAllItemSessionsWithBacklogInfo`
  also fixes a latent, previously-undocumented ambiguity in `BacklogService.GetSessionBacklogIndex`
  (the existing frontend `backlogIndex` join), which shares this same query and
  had no explicit ordering guarantee for sessions with multiple `ItemSession`
  rows.
- **Known limitation — one session UUID legitimately attached to two different backlog
  items**: `AttachSessionToItem` (`server/services/backlog_service_sync.go:103-109`,
  backing the `link_session_to_item` MCP tool) can attach an already-running session
  UUID to a second, different backlog item later. Since `SessionTokenSummary` is one row
  per session, the batched "most-recent-`created_at`-wins" role lookup will show the
  second item's role even when inspecting data generated during the first item's
  attachment. This is a known, accepted limitation — not a regression versus today's
  frontend join, which has the same undefined-order ambiguity, just without the new
  `.Order()` fix.

## Known inefficiencies

- **The full-table scan now runs twice per page load, and once per streamed event.**
  `GetAllItemSessionsWithBacklogInfo` is called independently by both
  `BacklogService.GetSessionBacklogIndex` (existing, powers the frontend's own
  `backlogIndex` fetch) and the new `InsightsService` role lookup — so a single Insights
  page load triggers this unfiltered scan twice, and each `WatchInsights` streamed event
  triggers it again. This is accepted as a known inefficiency, not silently introduced,
  because: (a) this is a personal single-user tool (per requirements.md's Users/Consumers
  section) where the absolute `ItemSession` row count is small, and (b) this repo's own
  backlog-automation WIP-limit convention (cap of 2 concurrent backlog work sessions, per
  project memory) means realistic concurrent-session/event load is low. If this ever
  becomes a measured problem, the fix is a short-TTL cache or request-scoped memoization
  shared between the two call sites — not attempted now, to avoid speculative complexity.
