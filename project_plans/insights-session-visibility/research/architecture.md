# Architecture Research: insights-session-visibility

Verified against worktree HEAD (branch `worktree-agent-a4bb10c9caaa381e1`, based on commit `57c593d06`).

## 1. `buildSessionSummary` data flow

`buildSessionSummary` (`server/services/insights_service.go:82-137`) is a pure function with
signature:

```go
func buildSessionSummary(
	r *tokens.ParseResult,
	pt *tokens.PricingTable,
	associator *tokens.Associator,
	snapshot []tokens.SessionRecord,
) *sessionv1.SessionTokenSummary
```

It has exactly three call sites, all in `insights_service.go`:
- `GetInsightsSummary` (line 233) — inside a per-session loop, called once per `r := range results`
- `ListSessionTokens` (line 482) — same pattern
- `watchInsights` (line 623) — once per streamed `update` event

All three already prefetch `sessionSnapshot := s.associator.Snapshot()` **once per request/event**
(lines 178-181, 466-469, 617-619) rather than calling `ListSessionRecords()` per session — this is
the existing "snapshot-once, associate-many" pattern any new bulk-lookup work should mirror.

**What's in scope at the call site:** `r *tokens.ParseResult` (JSONL-derived transcript data —
`r.SessionUUID` is the *Claude conversation UUID*, not the stapler-squad session ID), the
`pt *tokens.PricingTable`, `associator *tokens.Associator`, and `snapshot []tokens.SessionRecord`.
Inside `buildSessionSummary`, `sessionID, isOrphan := associator.AssociateWithSnapshot(r, snapshot)`
resolves the **stapler-squad session ID** (`Instance.UUID`) — this is the ID needed to look up both
tags and persisted role, *not* `r.SessionUUID`.

**`tokens.SessionRecord` today** (`session/tokens/association.go:11-16`):
```go
type SessionRecord struct {
	SessionID      string
	ConversationID string // matches ParseResult.SessionUUID
	Path           string
	CreatedAt      time.Time
}
```
`SessionStorage` interface (`session/tokens/association.go:20-23`) has one method,
`ListSessionRecords() []SessionRecord`, implemented by `*session.Storage`
(`session/storage.go:516-533`), which builds records from `ListInstanceData()` —
and `InstanceData` (`session/storage.go:23-75`) **already has `Tags []string` at line 48**.
`ListSessionRecords` currently drops it when building `SessionRecord`.

### Sourcing `Tags`: extend `SessionRecord`, not a separate lookup path

Because the associator's snapshot is already built from `InstanceData` (which already carries
`Tags`), the cheapest path is:
1. Add `Tags []string` to `tokens.SessionRecord` (`session/tokens/association.go:11-16`).
2. Populate it in `Storage.ListSessionRecords()` (`session/storage.go:516-533`) from `d.Tags`.
3. In `buildSessionSummary`, after resolving `sessionID`, look up the matching `SessionRecord` in
   `snapshot` to read `.Tags`. **Caveat:** `AssociateWithSnapshot`/`associate()`
   (`session/tokens/association.go:63-107`) currently returns only `(sessionID, isOrphan)`, discarding
   *which* record matched. Two options, in order of preference:
   - **(a)** Add a sibling method, e.g. `AssociateRecordWithSnapshot(result, sessions) (SessionRecord, bool)`,
     returning the matched record directly (or a zero value + `isOrphan=true`), used only by
     `buildSessionSummary`. Avoids a second scan.
   - **(b)** After getting `sessionID`, linear-scan `snapshot` for `SessionID == sessionID` — works,
     but doubles the scan `associate()` already did internally, and `snapshot` is unsorted/unindexed
     (no map). For dashboards with hundreds of sessions this is O(n) per session (O(n²) total) —
     tolerable given `associate()` itself is already O(n) per call for the same reason, but not
     free. **Recommend (a).**

No new `SessionStorage` interface method is needed beyond the existing `ListSessionRecords()` —
just widening its return element. No separate `BacklogRepository` reference is needed for tags.

### Sourcing `session_role`: a genuinely new lookup, but the exact primitive already exists

Session role is **not** derivable from `tokens.SessionRecord`/`Associator` at all — it lives in a
completely different subsystem (backlog persistence, ent-backed), unrelated to the
JSONL-transcript/`Instance` matching `Associator` does. Requires a **new dependency** on backlog
storage.

- `ItemSession.session_uuid` (`session/ent/schema/item_session.go:23-24`, comment: "Loose FK to
  Session; not an ent edge") is populated from the **stapler-squad session's own `Instance.UUID`**,
  not the Claude conversation UUID — verified at the call site `session/review_gate.go:444-452`:
  `ItemSessionData{..., SessionUUID: reviewInst.UUID, SessionRole: SessionRoleReview, ...}`. This
  means the lookup key is the **same `sessionID` `buildSessionSummary` already resolves via the
  associator** (`Instance.UUID`), not `r.SessionUUID`. Getting this wrong (using the conversation
  UUID instead) would silently return zero matches for every session.
- The exact-match lookup already exists: `Storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
  (ItemSessionSummary, error)` (`session/storage.go:1196`, backed by
  `session/storage_backlog.go:351-364`). It queries `ItemSession` directly with **no join/filter on
  the parent `BacklogItem`'s status** — so it already returns a match regardless of whether the
  parent item is open, done, or archived (see §2).
- `ItemSessionSummary.Role` (`session/repository.go:166`) is the field name (not `SessionRole`) —
  its value is one of the `session.SessionRoleWork/Triage/Review/JulesWork` string constants
  (`session/backlog.go:50-53`).

**Bulk lookup, not N sequential queries.** `buildSessionSummary` runs inside a per-session loop
called for every `ParseResult` (potentially hundreds), and `GetItemSessionBySessionUUID` is a real
DB query. Calling it once per session per request would be an N+1 query pattern the codebase has
already solved once for exactly this shape of problem: `EntRepository.GetAllItemSessionsWithBacklogInfo(ctx)`
(`session/ent_repository_backlog.go:2780-2804`, exposed via `Storage.GetAllItemSessionsWithBacklogInfo`,
`session/storage.go:1394-1396`) does a **single** full-table join (`//nolint:entfullscan` — its own
comment says "Used by the Insights dashboard index") and returns `[]ItemSessionBacklogEntry{SessionUUID,
SessionRole, ItemID, ItemTitle, ItemStatus}` for every `ItemSession` row. This is the same mechanism
`BacklogService.GetSessionBacklogIndex` (`server/services/backlog_service_query.go:531-560`) already
uses to serve the frontend's `useBacklogSessionIndex()` client-side join (see §3).

**Recommendation:** prefetch once per request/event via `GetAllItemSessionsWithBacklogInfo`, build a
`map[string]string` (session UUID → role) analogous to the existing `sessionSnapshot` prefetch, and
pass it into `buildSessionSummary` alongside the existing `snapshot` parameter. This avoids adding a
second per-session DB round-trip and keeps the "prefetch once, associate many" shape the file
already uses everywhere else.

## 2. Does role survive backlog item archival? (ent cascade/soft-delete check)

**Yes — archival never touches `ItemSession` rows.** Two independent code paths confirm this:

- **Archiving is a plain status write, not a delete.** `BacklogStatusArchived` transitions go through
  `TransitionBacklogItemStatus` → `.SetStatus(string(BacklogStatusArchived))`
  (`session/ent_repository_backlog.go:1326`) — no cascading delete anywhere in that path.
- **The `item_sessions` edge has no DB-level cascade annotation.** `BacklogItem.Edges()`
  (`session/ent/schema/backlog_item.go:194-228`) explicitly annotates `entsql.OnDelete(entsql.Cascade)`
  on `status_events`, `stuck_states`, `progress_notes`, `activity_notes`, and both dependency edges
  — but **`edge.To("item_sessions", ItemSession.Type)` at line 196 carries no such annotation.**
  Confirming this is deliberate (not an oversight): the only code path that actually removes
  `ItemSession` rows is `EntRepository.DeleteBacklogItem` (`session/ent_repository_backlog.go:1438-1490`),
  which **manually** deletes `ReviewVerdict` then `ItemSession` rows before deleting the `BacklogItem`
  itself (lines 1461-1485) — redundant work if a DB cascade already existed, confirming none does.
  `DeleteBacklogItem` is a **permanent, explicit hard-delete** (comment: "permanently removes an item
  and all its child records"), reachable only via the user-initiated `BacklogService.DeleteBacklogItem`
  RPC (`server/services/backlog_service_lifecycle.go:540`) and a debug mutate handler
  (`server/services/backlog_debug_mutate_handler.go:286`) — **never** an automatic sweep. Grepping the
  whole worktree for `DeleteBacklogItem(` call sites turns up only those two.

So: an archived (or "done") backlog item's `ItemSession` rows remain queryable via
`GetItemSessionBySessionUUID`/`GetAllItemSessionsWithBacklogInfo` indefinitely, until/unless someone
explicitly hard-deletes the item. **`session/ent_repository_backlog.go` already has the "find
ItemSession by SessionUUID across all items regardless of status" query the open question in
requirements.md asks for — no new query needs to be added, it already ignores status entirely.**

One correction to the requirements doc's stated root cause worth flagging for planning: the
existing `GetSessionBacklogIndex`/`GetAllItemSessionsWithBacklogInfo` path is *also* a full,
unfiltered scan (no `WHERE item.status IN (...)` clause) — so the frontend's current client-side
`backlogIndex` join is not literally "built only from currently open items" at the RPC layer. The
more likely actual failure mode (not chased further here — a root-cause question for Phase
2/implementation, not architecture) is that the *session* (`Instance`) itself can disappear from
`Storage.ListSessionRecords()`'s backing `ListInstanceData()` once `reconcileTerminalItemSessions`
(`session/backlog_lifecycle_archive.go:80-126`) archives+kills a terminal item's work/review session
— at that point `s.sessionId` on the `SessionTokenSummary` resolves to `""` (orphan), and
`backlogIndex.get(s.sessionId)` (`SessionsTable.tsx:281,294`) can never match an empty key, regardless
of whether the backlog item's row is still present. This distinction matters for the fix: sourcing
`session_role` by keying off `r.SessionUUID`/conversation-based matching wouldn't help either — the
fix must key off the **persisted `ItemSession.session_uuid`** independent of whether the `Instance`
is still live, which is exactly what `GetItemSessionBySessionUUID`/`GetAllItemSessionsWithBacklogInfo`
already do (they query `ItemSession` directly, no dependency on `Storage.ListInstanceData()` at all).

## 3. `session.Storage` surface for tags and role — cheapest existing methods

| Need | Method | Notes |
|---|---|---|
| Instance tags by session ID | `Instance.GetTags()` (`session/instance_tags.go:53-64`) | Only works for a *live* in-memory `*Instance`, not a session ID string. For `buildSessionSummary`'s use case (bulk, ID-keyed, includes non-resident sessions) use `Storage.ListInstanceData()` → `InstanceData.Tags` instead (already flows through `ListSessionRecords`, see §1). |
| Persisted role by session UUID, any item status | `Storage.GetItemSessionBySessionUUID(ctx, sessionUUID)` (`session/storage.go:1196` → `session/storage_backlog.go:351-364`) | Single-row, no status filter. Adequate for `GetSessionTurnTimeline`-style on-demand lookups but an N+1 risk in the bulk summary-list path (see §1). |
| Persisted role for *all* sessions in one query | `Storage.GetAllItemSessionsWithBacklogInfo(ctx)` (`session/storage.go:1394-1396` → `session/ent_repository_backlog.go:2780-2804`) | Full unfiltered scan, already used by `BacklogService.GetSessionBacklogIndex`. **This is the right primitive for `buildSessionSummary`'s bulk path** — prefetch once per request/event, same shape as `associator.Snapshot()`. |

`session/ent_repository_backlog.go` already effectively has the "find ItemSession by SessionUUID
across all items regardless of status" query requirements.md's open question asks about — both
`GetItemSessionBySessionUUID` (single) and `GetAllItemSessionsWithBacklogInfo` (bulk) already ignore
`BacklogItem.Status` entirely (see §2). No new ent query is required.

## 4. `InsightsService` constructor — what's already wired, minimal-blast-radius extension

`NewInsightsService` (`server/services/insights_service.go:44-56`) currently takes exactly three
params: `store tokens.TokenStoreReader`, `pricing *tokens.PricingTable`, `associator
*tokens.Associator`. **It receives nothing backlog-related today.**

Wiring site: `server/dependencies.go:1493-1494`:
```go
associator := tokens.NewAssociator(storage)
insightsSvc = services.NewInsightsService(tokenStore, pricing, associator)
```
`storage` here is the same `*session.Storage` instance already passed to `tokens.NewAssociator` —
it is already in scope at this exact call site and already has `GetAllItemSessionsWithBacklogInfo`
and `ListInstanceData`/`ListSessionRecords` (needed for tags) as methods. **The minimal-blast-radius
change is to pass this same `storage` value as a fourth `NewInsightsService` argument** — no new
dependency needs to be constructed or threaded through `dependencies.go`, since it already exists
in the exact scope where the constructor is called.

Recommend defining a narrow interface in `server/services/insights_service.go` (mirroring
`tokens.SessionStorage`'s narrow-interface style) rather than accepting `*session.Storage` directly,
e.g.:
```go
// insightsBacklogReader is the narrow interface InsightsService uses to source
// persisted session_role at summary-build time. Satisfied by *session.Storage.
type insightsBacklogReader interface {
	GetAllItemSessionsWithBacklogInfo(ctx context.Context) ([]session.ItemSessionBacklogEntry, error)
}
```
This keeps `insights_service.go`'s existing pattern (it already depends on `tokens.TokenStoreReader`
and implicitly on `tokens.SessionStorage` via `Associator`, both narrow interfaces, not concrete
types) and avoids a new import cycle risk from importing the full `session` package's `Storage` type
if one exists (needs a quick build check once implemented — `session/tokens` already avoids
importing `session` for exactly this reason, per `association.go`'s doc comment at line 8-10).

One consequence: `buildSessionSummary`, `GetInsightsSummary`, and `ListSessionTokens` do not
currently thread a real `context.Context` in the places that would need one — `GetInsightsSummary`
and `ListSessionTokens` both discard their incoming context as `_ context.Context`
(`insights_service.go:141, 449`), and `buildSessionSummary` takes no context at all. Adding a
backlog-role prefetch means `GetInsightsSummary`/`ListSessionTokens` must start honoring `ctx`
(rename `_` → `ctx`, pass it to the new `GetAllItemSessionsWithBacklogInfo` call), and
`watchInsights` (which already has `ctx` in scope at line 585) must pass it through into
`buildSessionSummary`'s new role-map parameter path (or just prefetch the role map itself, sibling to
its existing `sessionSnapshot` prefetch at line 617-619).

## 5. Frontend architecture — no shared mapper, direct prop-drilling of the generated type

`useInsightsService.ts` has **no intermediate DTO/mapping layer**. `SessionTokenSummary` (the
protobuf-ES generated type from `@/gen/session/v1/insights_pb`) is fetched via
`client.getInsightsSummary()`/`client.watchInsights()` and stored directly in React state
(`useInsightsSummary`'s `summary: GetInsightsSummaryResponse | null`, line 30); no `mapSessionSummary`
or similar transform exists anywhere in `useInsightsService.ts`. Consumers (`SessionsTable.tsx` props
`sessions: SessionTokenSummary[]`, line 36; `SessionDetailContent.tsx` similarly) read fields directly
off the generated type (`s.primaryModel`, `s.cacheHitRate`, etc. — grep confirms no wrapper/adapter
type between the RPC response and the table/detail components).

**Consequence:** adding `tags`/`session_role` to the proto message requires updating exactly one
place per concern — `make proto-gen` regenerates the TS type with new `tags: string[]` /
`sessionRole: string` fields (protobuf-ES camelCases proto `snake_case` automatically) — and every
consumer (`SessionsTable.tsx`, `SessionDetailContent.tsx`, `SessionDetailDrawer.tsx`,
`InsightsDashboard.tsx`) can read `s.tags`/`s.sessionRole` directly with no shared-layer edit needed.
This is lower-risk than a codebase with a central mapper (nothing to keep in sync), but it also means
each consuming component's own logic (Fuse.js search doc-building in `SessionsTable.tsx:88-97`, the
existing `backlogEntry`/`backlogIndex` join at lines 281/294/298/301) needs its **own** point edit —
there's no single choke point where adding the new field once covers every surface.

Specifically for role display: `SessionsTable.tsx:281` (`const backlogEntry =
backlogIndex?.get(s.sessionId)`) and `SessionDetailContent.tsx` (`backlogEntry?.sessionRole` via its
`backlogEntry` prop) would switch from reading `backlogEntry.sessionRole` (client join, requires a
non-empty `s.sessionId` and a still-present `ItemSession`-index row for that session UUID) to reading
`s.sessionRole` directly off the new proto field — dropping the `backlogIndex` dependency for role
specifically (tag/title/status display via `backlogIndex` can stay as-is; requirements.md's Scope
item #3 only asks for role, not the badge's title/link, to move off the join).

## 6. Live-stream vs. fetch-once, and staleness precedent

`SessionTokenSummary` is **both**: `ListSessionTokens`/`GetInsightsSummary` are fetch-once RPCs;
`WatchInsights` (`insights_service.go:570-641`) streams incremental `update` events that **re-run
`buildSessionSummary` for one session** each time the `TokenStore` reparses that session's JSONL file
(`insights_service.go:616-625`), which the frontend merges into its in-memory `summary.sessions` array
by `conversationId || sessionId` key (`useInsightsService.ts:115-124`).

**Existing staleness precedent, applicable to `tags`/`session_role`:** every field
`buildSessionSummary` computes — including `is_orphan` (associator match) and `activity_type`
(`ClassifyActivity`) — is only recomputed when a `ParseResult` update event fires (i.e., the
session's JSONL transcript file changes on disk), **not** when something external to the
transcript changes (e.g. the `Instance`'s tags, or the backlog item's status). There is no existing
mechanism that pushes a `WatchInsights` update purely because a tag or backlog status changed
elsewhere. `is_orphan` in particular already has exactly the staleness profile `tags`/`session_role`
would inherit: if a session gets associated with (or orphaned from) a stapler-squad `Instance`
*after* its last JSONL write, that change is invisible to Insights until either another JSONL write
triggers a fresh `TokenStore` event for that file, or the user does a full reload
(`fetchSummary()`, called on mount and on every `parse_complete` event).

**Recommendation:** treat `tags`/`session_role` staleness the same way — eventual/on-refresh
consistency, matching `is_orphan`/`activity_type`'s existing behavior, not a new invalidation
trigger. This is consistent with the requirements doc's Out of Scope framing (no backlog lifecycle
changes, no redesign of computation triggers) and avoids adding a second event source (backlog
mutation → force-refresh) to `WatchInsights`'s single-trigger (`TokenStore` reparse) design.
