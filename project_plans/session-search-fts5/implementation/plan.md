# Implementation Plan: session-search-fts5

**Feature**: Extend the existing BM25 history-search engine with session dedup, ±5/bookend context windows, scroll-mode anchor paging, and best-effort automation-session exclusion; surface it as a new "Find related past work" search box in the backlog triage panel.
**Date**: 2026-08-02
**Status**: Ready for implementation
**ADRs**: ADR-001-extend-bm25-not-fts5, ADR-002-automation-session-exclusion-best-effort

---

## Creative Pass (Step 0.5)

Three high-level shapes for how dedup + context window + scroll mode + source filtering + triage UI fit together were considered:

**(a) Extend `SearchClaudeHistoryRequest`/`Response` with additive optional fields, reuse `GetClaudeHistoryMessages` for scroll mode.**
Strength: zero new RPCs/registry entries, wire-compatible with the two existing consumers (`useHistoryFullTextSearch.ts`, `HistorySearchResults.tsx`), matches the proto's own `optional`-field convention already used for `project`/`model`.
Weakness: `SearchResult` grows conditional fields (`context_window`, `bookend_first/last`) that are empty payload weight for callers who never set `include_context`.

**(b) New `DiscoverSessions` RPC wrapping `SearchClaudeHistory` + `GetClaudeHistoryMessages` internally.**
Strength: purpose-built response shape from the start, zero risk of ever touching the existing RPC's wire format.
Weakness: duplicates ~90% of `SearchClaudeHistory`'s handler logic (index sync, snippet generation, entry enrichment) behind a second RPC that needs its own proto messages, its own `docs/registry/features/backend/` entry, and its own test suite — paying full new-RPC overhead for functionality that's already expressible as additive fields on the existing one, with no correctness benefit (mirrors why `build-vs-buy.md`'s Option 3 "hybrid backends" was rejected — splitting the plumbing buys nothing when nothing in scope needs a second code path).

**(c) Dedup/context computed purely client-side in `TriageRelatedWorkSection`, existing RPCs unmodified.**
Strength: zero backend/proto changes, fastest to ship.
Weakness: pushes one `GetClaudeHistoryMessages` round-trip per session-group to the browser (N+1 over the network) for context windows, and — critically — pushes `source=tool` filtering to the client, which means the raw (unfiltered) automation-session data would have to reach the browser first for the client to hide it; that's a filtering-in-the-wrong-layer smell, not just a performance one.

**Chosen: (a).** It is what `architecture.md` §3 already concluded from reading the actual handler code, it satisfies `requirements.md`'s explicit constraint ("changes to response shape... need to be additive"), and per `pitfalls.md` §1/§7 it is the only option that keeps `HistorySearchResults.tsx`'s `totalMatches - results.length` arithmetic and per-message list keying untouched by construction (new fields simply aren't read by old code). (b) and (c) are recorded as rejected alternatives in the Pattern Decisions table below.

---

## Step 1: System Type

Read/query extension over an existing search subsystem (`session/search/` BM25 engine + `SearchService` RPCs), plus one new UI surface (`TriageRelatedWorkSection.tsx`). Not a new service, not a new storage layer, not a greenfield build.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `SessionHit` | The single, highest-scored `SearchResult` retained per session after dedup; represents "this session matched." | Not a new Go/TS type — it's the *role* an existing `SearchResult`/`SearchResultItem` plays once `group_by_session` is on. |
| `ContextWindow` | The ±5 messages surrounding a hit's `MessageIndex`, read from `GetMessagesFromConversationFile` (raw JSONL), not from `DocumentStore`. | Proto: `SearchResult.context_window`. Populated only when `include_context=true`. |
| `BookendMessages` | The first 3 and last 3 messages of a session's full transcript, giving scene-setting context regardless of where the hit lands. | Proto: `SearchResult.bookend_first` / `bookend_last`. Suppressed (empty) when the context window already spans the whole session — see Story 1.3.1. |
| `MoreMatchesInSessionCount` | Count of additional matching messages in the same session beyond the retained `SessionHit`. | Proto: `SearchResult.more_matches_in_session_count` (`int32`). Directly answers requirements.md's "N more matches in this session" AC. |
| `ScrollAnchor` | The `MessageIndex` used as a stable position to center a `GetClaudeHistoryMessages` page, enabling forward/backward paging without re-running search. | Proto: `GetClaudeHistoryMessagesRequest.anchor_index` (`optional int32`). Not a new cursor type — `MessageIndex` is already a stable positional integer, so no opaque token is needed (see Pattern Decisions). |
| `GroupBySession` | Request flag that turns on session-level dedup post-processing. | Proto: `SearchClaudeHistoryRequest.group_by_session` (`optional bool`, default false). |
| `IncludeContext` | Request flag that turns on `ContextWindow`/`BookendMessages` population per retained hit. | Proto: `SearchClaudeHistoryRequest.include_context` (`optional bool`, default false). |
| `ExcludeAutomationSessions` | Request flag enabling the best-effort automation-session filter. | Proto: `SearchClaudeHistoryRequest.exclude_automation_sessions` (`optional bool`, default false). |
| `isAutomationSession` | Go predicate cross-referencing a `SearchResult.SessionId` against live `session.Instance.Hidden` via `ConversationUUID` — the same field `ListSessions`' `include_hidden` flag already uses to hide background/triage/review sessions (`session/instance.go:220-223`, `server/services/session_service.go:1089`). | Best-effort only — silently unavailable for sessions whose `Instance` was deleted. See ADR-002. |
| `groupResultsBySession` | Go helper: walks score-sorted `[]*sessionv1.SearchResult`, keeps the first (highest-scored) result per `SessionId`, accumulates `MoreMatchesInSessionCount` on the kept result for subsequent same-session hits. | New file: `server/services/search_related_work.go`. |
| `contextWindowAndBookends` | Go helper: given a session's full message slice and a hit index, returns the clamped ±5 window plus first-3/last-3 bookends (or empty bookends when the window already covers the full transcript). | Same file. |
| `RelatedWorkQuery` | The triage panel's default search invocation: `query=item.title`, `project=item.repoPath`, `groupBySession=true`, `includeContext=true`, `excludeAutomationSessions=true`, `limit=5`. | Not a type — the fixed option bundle `TriageRelatedWorkSection` passes to `useHistoryFullTextSearch().search(...)`. |
| `TriageRelatedWorkSection` | New React component, sibling to `TriageDiffSection`/`TriageErrorBanner`, hosting the "Find related past work" search box + session-deduped result cards inside `TriageReviewPanel`. | `web-app/src/components/backlog/TriageRelatedWorkSection.tsx`. |
| `SessionHitCard` | Small internal presentational component (within `TriageRelatedWorkSection.tsx`) rendering one `SessionHit`: title, date, match count, top snippet, "N more matches" affordance. | Not extracted to its own file — single call site, keeps task count reasonable (see Pattern Decisions #7). |

**Glossary term count: 12.**

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Session dedup (`groupResultsBySession`) | Transaction Script — plain `O(n)` function over an already-sorted slice | PoEAA (Transaction Script) | `SessionGrouper` strategy interface with pluggable tie-break policy | Exactly one tie-break policy exists (highest score wins, stable order on ties) — a `Strategy` interface for one implementation is the "speculative interface" smell this repo's `interface-pollution-checklist.md` explicitly flags. |
| Context window / bookend extraction (`contextWindowAndBookends`) | Transaction Script — pure function over a message slice | PoEAA | `ContextWindowBuilder` object | No configuration varies across calls; a builder adds indirection with zero behavior gain (checklist smell #4, forwarding-only wrapper). |
| Automation-session filter (`isAutomationSession`) | Plain predicate function, not an interface | Go idiom (accept concrete types) | Pluggable `SourceFilter` interface with multiple implementations | Only one signal exists today (`Instance.Hidden` cross-ref, the same field `ListSessions`' `include_hidden` already filters on) — same "exactly one implementation" smell as above. If a second signal (e.g. a persisted `conversation_uuid` column) is added later, promote to an interface *then*, in the consumer package, per the checklist's own prescription. |
| Scroll-mode anchor (`anchor_index`) | Extend existing RPC with one optional field + server-side offset arithmetic | Existing `GetClaudeHistoryMessages` code | New opaque base64 cursor type mirroring `historyCursor` | `MessageIndex` is already a stable, directly-addressable integer (unlike `historyCursor`'s `UpdatedAt+ID`, which exists because history *entries* have no stable ordinal) — inventing an opaque token for data that's already positionally addressable is unjustified indirection. |
| RPC/response shape (dedup + context) | Additive `optional` fields on existing `SearchClaudeHistoryRequest`/`Response` | Existing proto convention (`optional string project`, etc.) | New `DiscoverSessions` RPC / new `SessionGroup` message | See Creative Pass (a) vs (b) above — duplicate plumbing for zero correctness gain. |
| Frontend search invocation | Extend `useHistoryFullTextSearch`'s existing `SearchOptions`/`SearchResultItem`, no new hook | Existing hook | New `useRelatedWorkSearch` hook | Would be a forwarding-only wrapper around the same debounce/abort/state logic (checklist smell #4) — the only real delta is which fields are populated in the request/response. |
| Triage UI component | New leaf component composed into `TriageReviewPanel`, matching `TriageDiffSection` precedent | Existing repo convention (one file + `.css.ts` per panel section) | Inline JSX in `TriageReviewPanel.tsx` | `TriageReviewPanel.tsx` already decomposes every section into its own file; inlining would bloat an already-large component and contradicts that precedent. |
| `SessionHitCard` | Internal function component within `TriageRelatedWorkSection.tsx`, not its own file | N/A | Extract to `SessionHitCard.tsx` | Single call site, no reuse elsewhere yet — premature extraction adds a file/import for no benefit; matches "don't over-pattern a single implementation." |

---

## Migration Plan

N/A — no persisted schema or data changes. All proto field additions (§ below) are wire-compatible (new field numbers, `optional`/default-false semantics) and require no backfill, no reindex, and no `IndexStore`/gob version bump (confirmed in `research/stack.md` §3: nothing is added to `Document`, `InvertedIndex`, or `PostingsList`). `make proto-gen` regenerates committed generated code in the same commit as the `.proto` edit (Task 1.1.1b) — this is a codegen refresh, not a migration.

## Observability Plan

- **Logs**: extend the existing `search index sync` info log (`search_service.go` inside `SearchClaudeHistory`) with no new log line required for dedup/context — those are deterministic, low-risk transforms. Add one new `log.Info` in the automation-session filter path (`server/services/search_related_work.go`) only when `exclude_automation_sessions=true` **and** at least one result was actually excluded, logging the excluded count — mirrors the existing `syncResult.HasChanges()` conditional-logging pattern (avoid unconditional per-request noise).
- **Metrics**: none new — reuse the existing OTel spans (`SearchEngine.IncrementalSync`, `SearchEngine.Search`) and add `search.group_by_session`, `search.include_context`, `search.exclude_automation_sessions` (bool) and `search.sessions_returned` (int, post-dedup count) as attributes on the existing `SearchClaudeHistory` request span, so discovery-mode usage is visible in existing traces without a new span.
- **Alerts**: none — matches requirements.md's Observability Requirements ("standard request logging/tracing sufficient").

## Risk Control

- **Feature flag**: none needed. All new behavior is opt-in per request (`group_by_session`/`include_context`/`exclude_automation_sessions` default `false`), so the flags themselves are the staged-rollout mechanism — existing callers get byte-identical behavior until they explicitly opt in. Matches requirements.md's explicit "Risk Control: Not needed."
- **Rollback procedure**: revert the PR. Because all changes are additive (new proto fields, new Go functions, new React component, new panel section gated on `!readOnly`), a revert leaves zero orphaned data or partially-migrated state — nothing to clean up.
- **Staged rollout**: not applicable — single-user local tool, no fleet/canary concept (matches requirements.md's Scalability N/A framing).

## Unresolved Questions

None block this plan. One item is explicitly deferred as a fast-follow, not a blocker: cross-referencing a `SessionHit` to a backlog `LinkedSession.reviewVerdict.overallOutcome` for a PASS/FAIL outcome badge on the result card (`research/ux.md` §3, Open Question 2) requires a session-ID → backlog-item join that does not exist on the `SearchClaudeHistory` path today and is out of scope for this plan (see Phase 2 Out-of-Scope note in Story 2.2.1) — v1 ships snippet-only cards, which `ux.md` itself recommends as the fallback if the join isn't free.

## Dependency Visualization

```
proto/session/v1/session.proto (Task 1.1.1a)
        |
        v
make proto-gen (Task 1.1.1b) ---------------------------------+
        |                                                     |
        v                                                     v
gen/proto/go/session/v1/*.go                    web-app/src/gen/session/v1/*_pb.ts
        |                                                     |
        v                                                     v
server/services/search_related_work.go          web-app/src/lib/hooks/useHistoryFullTextSearch.ts
  groupResultsBySession      (Story 1.2.1)         SearchOptions + SearchResultItem  (Story 2.1.1)
  contextWindowAndBookends   (Story 1.3.1)                          |
  isAutomationSession        (Story 1.5.1)                          v
  filterByProject            (Story 1.6.1)         web-app/src/components/backlog/TriageRelatedWorkSection.tsx
        |                                                    (Story 2.2.1, 2.2.2, 2.2.3)
        v                                                            |
server/services/search_service.go handlers wired                    v
  SearchClaudeHistory   (Stories 1.2.1-1.6.1,      web-app/src/components/backlog/TriageReviewPanel.tsx
   raw-limit oversampling in Task 1.2.1b)              (insertion point, Task 2.2.1d)
  GetClaudeHistoryMessages (Story 1.4.1, consumed                    |
   end-to-end via Story 2.2.3's anchor_index call)                   v
        |                                          web-app/src/app/history/page.tsx
        v                                              (?sessionId=/?messageIndex= handling, Task 2.2.3b)
docs/registry/features/backend/history/search.json                  |
  (Story 3.1.1)                                                      v
        |                                    docs/registry/features/frontend/ui/triage-related-work.json
        |                                                (Story 3.1.1)
        +---------------------------+---------------------------+
                                     v
                    tests/e2e/triage-related-work.spec.ts (Story 3.2.1)
```

---

## Phase 1: Backend Response-Shaping Extensions

### Epic 1.1: Proto Schema Extension
**Goal**: Add every new request/response field needed by Phase 1's other epics as a single, additive proto change, regenerated once so downstream tasks compile against final types.

#### Story 1.1.1: Extend `SearchClaudeHistoryRequest`/`Response` and `GetClaudeHistoryMessagesRequest`
**As a** backend handler implementer, **I want** the new fields to exist in generated code before I wire any logic, **so that** Stories 1.2.1–1.5.1 can be implemented and compiled independently.
**Acceptance Criteria**:
- New fields are additive (next available field numbers), default to `false`/`0`/empty, and existing callers' wire payloads are unaffected.
  - *Given* a `SearchClaudeHistoryRequest{Query: "auth refactor", Limit: 20}` sent by today's `useHistoryFullTextSearch.ts` (no new fields set), *When* the server processes it, *Then* `group_by_session`, `include_context`, and `exclude_automation_sessions` all evaluate to `false` and the response shape (message-per-hit, `total_matches` = message count) is byte-identical to pre-change behavior.
**Files**: `proto/session/v1/session.proto`, `gen/proto/go/session/v1/*.go` (regenerated), `web-app/src/gen/session/v1/*_pb.ts` (regenerated)

##### Task 1.1.1a: Add proto fields (~4 min)
- In `proto/session/v1/session.proto`, inside `message SearchClaudeHistoryRequest` (after existing field 7 `offset`), add:
  ```protobuf
  // When true, collapse results to one entry per session (highest-scored
  // hit kept; others counted via more_matches_in_session_count). Default false.
  optional bool group_by_session = 8;
  // When true, populate context_window/bookend_first/bookend_last on each
  // retained result. Default false.
  optional bool include_context = 9;
  // When true, best-effort exclude sessions whose live Instance has
  // AutonomousMode=true. Sessions with no live Instance record are NOT
  // excluded (signal unavailable, not assumed absent). Default false.
  optional bool exclude_automation_sessions = 10;
  ```
- Inside `message SearchResult` (after existing field 7 `metadata`), add:
  ```protobuf
  // Count of additional matching messages in this session beyond this hit.
  // Only meaningful when the request set group_by_session=true.
  int32 more_matches_in_session_count = 8;
  // ±5 messages around message_index, read from the raw conversation file.
  // Populated only when the request set include_context=true.
  repeated ClaudeMessage context_window = 9;
  // First 3 messages of the session. Empty when context_window already
  // spans the full session (see contextWindowAndBookends).
  repeated ClaudeMessage bookend_first = 10;
  // Last 3 messages of the session. Empty when context_window already
  // spans the full session.
  repeated ClaudeMessage bookend_last = 11;
  ```
- Inside `message GetClaudeHistoryMessagesRequest` (after existing field 4 `tail`), add:
  ```protobuf
  // When set, overrides offset: the server centers the returned page on
  // this message index (offset = max(0, anchor_index - limit/2)),
  // enabling forward/backward scroll paging without re-running search.
  // Mutually exclusive with tail.
  optional int32 anchor_index = 5;
  ```
- Files: `proto/session/v1/session.proto`

##### Task 1.1.1b: Regenerate and commit generated code (~3 min)
- Run `make proto-gen`.
- Verify `git status` shows changes only in `gen/proto/go/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` (no unrelated diffs).
- Run `go build ./...` to confirm the backend still compiles with the new (unused) fields.
- Files: `gen/proto/go/session/v1/session.pb.go` (or equivalent generated filename), `web-app/src/gen/session/v1/session_pb.ts`

---

### Epic 1.2: Session-Level Dedup
**Goal**: When `group_by_session=true`, collapse `SearchClaudeHistory` results to one entry per session — and, critically, do so over an oversampled *raw* candidate set fetched from the engine *before* the caller's requested `limit` is applied, not over the already-`limit`-truncated message-level hits. Without this, a small `limit` (e.g. the triage panel's `limit=5`) lets a single busy session's messages fill the entire raw result set, starving out genuinely distinct sessions dedup exists to surface.

#### Story 1.2.1: Group results by session, keep top-scored hit, count the rest
**As a** triage operator, **I want** a session that matched 5 times to appear once — and other distinct matching sessions not to get crowded out by that one busy session — **so that** I can scan discovery results per-session instead of per-message.
**Acceptance Criteria**:
- With `group_by_session=true`, a session with multiple matching messages appears exactly once in `results`, with `more_matches_in_session_count` set to the number of collapsed hits.
  - *Given* a search for `"dark mode toggle"` where session `a3f5c8d2-7b1e-4a90-9c3d-1f6e8b2a4c7d` has 3 matching messages (scores 8.2, 6.1, 4.0) and session `b7e1a204-...` has 1 matching message (score 5.5), *When* `SearchClaudeHistory` is called with `group_by_session=true`, *Then* `results` contains exactly 2 entries: session `a3f5c8d2...` with `score=8.2` and `more_matches_in_session_count=2`, and session `b7e1a204...` with `score=5.5` and `more_matches_in_session_count=0`.
- With `group_by_session=false` (default), behavior is unchanged (one row per message, as today).
  - *Given* the same search with `group_by_session` unset, *When* the handler runs, *Then* `results` contains all 4 message-level hits, matching today's `SearchClaudeHistory` behavior byte-for-byte.
- **Dedup does not starve on a small `limit` (the triage default is `limit=5`).** When `group_by_session=true`, the engine is queried with an oversampled raw limit (`requestedLimit * 5`, capped at the existing 100-result ceiling) so dedup has enough raw candidates to collapse *before* the caller's `limit` is applied to the final, session-deduped list.
  - *Given* a search for `"dark mode toggle"` where the raw top-25 message-level hits (sorted by score) consist of 20 messages from session `busy-session-1` (scores 9.9 down to 5.0) and 1 message each from 5 other distinct sessions (`s2`..`s6`, scores 4.9 down to 4.5), and the request sets `group_by_session=true, limit=5`, *When* `SearchClaudeHistory` runs, *Then* it queries the engine with `Limit=25` (5 × 5, per the oversampling rule), dedup collapses `busy-session-1`'s 20 messages to 1 row (`more_matches_in_session_count=19`), and the final truncation to the requested `limit=5` yields exactly 5 session rows: `busy-session-1`, `s2`, `s3`, `s4`, `s5` — not 1 row (which is what truncating to `limit=5` *before* dedup would have produced, since all 5 raw slots would have been consumed by `busy-session-1`).
**Files**: `server/services/search_related_work.go` (new), `server/services/search_related_work_test.go` (new), `server/services/search_service.go`

##### Task 1.2.1a: Implement `groupResultsBySession` (~4 min)
- Create `server/services/search_related_work.go` with package `services` and:
  ```go
  // groupResultsBySession collapses results to one entry per SessionId,
  // keeping the first-encountered (highest-scored, since results arrives
  // pre-sorted by score) result per session and accumulating
  // MoreMatchesInSessionCount on it for every subsequent same-session hit.
  func groupResultsBySession(results []*sessionv1.SearchResult) []*sessionv1.SearchResult {
  	kept := make(map[string]*sessionv1.SearchResult, len(results))
  	order := make([]*sessionv1.SearchResult, 0, len(results))
  	for _, r := range results {
  		if existing, ok := kept[r.SessionId]; ok {
  			existing.MoreMatchesInSessionCount++
  			continue
  		}
  		kept[r.SessionId] = r
  		order = append(order, r)
  	}
  	return order
  }
  ```
- Files: `server/services/search_related_work.go`

##### Task 1.2.1b: Oversample the raw engine fetch, wire dedup, truncate to the requested limit (~6 min)
- This task owns the fix for the filter/dedup-vs-truncation ordering bug: today (verified at `search_service.go:501-521`) `limit` is clamped once (`≤0→20`, `>100→100`) and passed straight through as `searchOpts.Limit` to `ss.searchEngine.Search(...)`, so the engine itself truncates `Results` to `limit` *before* any post-processing ever runs. When `group_by_session`/`exclude_automation_sessions`/`project` post-processing is requested (Stories 1.2.1, 1.5.1, 1.6.1), that truncation must happen *after* post-processing, not before.
- In `server/services/search_service.go`, replace the existing block:
  ```go
  limit := int(req.Msg.Limit)
  if limit <= 0 {
  	limit = 20
  }
  if limit > 100 {
  	limit = 100
  }

  offset := int(req.Msg.Offset)
  if offset < 0 {
  	offset = 0
  }

  searchOpts := search.SearchOptions{
  	Limit:  limit,
  	Offset: offset,
  }
  ```
  with:
  ```go
  requestedLimit := int(req.Msg.Limit)
  if requestedLimit <= 0 {
  	requestedLimit = 20
  }
  if requestedLimit > 100 {
  	requestedLimit = 100
  }

  offset := int(req.Msg.Offset)
  if offset < 0 {
  	offset = 0
  }

  // needsPostProcessing is true whenever a post-fetch filter/dedup step will
  // run on protoResults below. In that case, truncating the *raw* engine
  // fetch to requestedLimit first would let a single busy or filtered-out
  // session consume the entire raw window before dedup/filtering ever gets
  // a chance to run — so over-fetch a larger raw candidate set instead, and
  // apply requestedLimit only after post-processing (see the truncation
  // step at the end of this function).
  needsPostProcessing := req.Msg.GetGroupBySession() || req.Msg.GetExcludeAutomationSessions() || req.Msg.GetProject() != ""
  rawLimit := requestedLimit
  if needsPostProcessing {
  	rawLimit = requestedLimit * 5
  	if rawLimit > 100 {
  		rawLimit = 100
  	}
  }

  searchOpts := search.SearchOptions{
  	Limit:  rawLimit,
  	Offset: offset,
  }
  ```
- After `protoResults` is fully built (end of the existing per-result loop) and after the project filter (Task 1.6.1b), automation filter (Task 1.5.1b), and dedup all run in that order (see those tasks for why this order matters — project/automation filtering must precede dedup so an excluded/out-of-project hit never contributes to `more_matches_in_session_count` on a kept session), add the dedup call and the final truncation:
  ```go
  if req.Msg.GetGroupBySession() {
  	protoResults = groupResultsBySession(protoResults)
  }

  if needsPostProcessing && len(protoResults) > requestedLimit {
  	protoResults = protoResults[:requestedLimit]
  }
  ```
- `TotalMatches`/`HasMore` in the final response continue to reflect the raw, pre-post-processing message-level count from `searchResults.TotalMatches` (unchanged from today) — they are documented as message-level counts, not post-processing session counts, which is an existing, separately-tracked concern (architecture-review.md's "total_matches/has_more become ambiguous" note) and is not part of this fix.
- Files: `server/services/search_service.go`

##### Task 1.2.1c: Unit tests (~7 min)
- `server/services/search_related_work_test.go`: `TestGroupResultsBySession_KeepsHighestScoredHitPerSession` (3-message session collapses to 1, count=2, per GWT above), `TestGroupResultsBySession_LeavesSingleHitSessionsUntouched` (count stays 0), `TestGroupResultsBySession_PreservesInputOrderWhenNoGrouping` (empty input, single-session input identity checks).
- `server/services/search_service_test.go` (or the new `search_related_work_test.go`, whichever already has handler-level test scaffolding): `TestSearchClaudeHistory_DedupOversamplesBeforeTruncatingToRequestedLimit` — the busy-session-vs-5-distinct-sessions scenario from Story 1.2.1's third GWT, asserting the engine is called with `Limit=25` and the final response has exactly 5 session rows, not 1.
- Files: `server/services/search_related_work_test.go`
- Files: `server/services/search_related_work_test.go`

---

### Epic 1.3: Context Window & Bookend Messages
**Goal**: When `include_context=true`, attach a ±5-message window and first-3/last-3 bookends to each retained hit, sourced from the raw JSONL transcript (not the tokenizer-filtered index — per `pitfalls.md` §4).

#### Story 1.3.1: Attach context window and bookends per retained hit
**As a** triage operator, **I want** to see the conversation around a match, **so that** the snippet is enough evidence to judge "what happened" without opening the full session.
**Acceptance Criteria**:
- With `include_context=true`, each retained `SearchResult` carries up to 11 messages in `context_window` (±5 around `message_index`, clamped at session boundaries).
  - *Given* session `a3f5c8d2-7b1e-4a90-9c3d-1f6e8b2a4c7d` has 20 messages total and the hit's `message_index=10`, *When* `contextWindowAndBookends` runs, *Then* `context_window` is `messages[5:16]` (11 messages, indices 5–15) and `bookend_first`/`bookend_last` are `messages[0:3]`/`messages[17:20]`.
- When the ±5 window already spans the entire session (short session), bookends are suppressed (empty) rather than duplicating content already in `context_window`.
  - *Given* session `b7e1a204-...` has 8 messages total and the hit's `message_index=4`, *When* `contextWindowAndBookends` runs, *Then* `context_window` is `messages[0:8]` (windowStart=max(0,4-5)=0, windowEnd=min(8,4+6)=8, i.e. the full transcript) and both `bookend_first` and `bookend_last` are empty.
- Context/bookends are read via `hist.GetMessagesFromConversationFile`, never via `DocumentStore`, so tokenizer-skipped messages (e.g. a bare "ok") still appear in the window.
  - *Given* a session where message index 7 is the single word "ok" (zero tokens, skipped by `IndexMessage`) and the hit is at index 9, *When* the ±5 window `[4:15]` is computed, *Then* the "ok" message at index 7 is present in `context_window` because the source is the raw conversation file, not `DocumentStore.GetBySession`.
**Files**: `server/services/search_related_work.go`, `server/services/search_service.go`, `server/services/search_related_work_test.go`

##### Task 1.3.1a: Extract shared `toProtoClaudeMessages` helper (~3 min)
- In `server/services/search_related_work.go`, extract the existing per-message proto-conversion loop (currently inline in `GetClaudeHistoryMessages`, `search_service.go` ~lines 442–450: `Role`, `Content`, `Timestamp: timestamppb.New(...)`, `Model`) into:
  ```go
  func toProtoClaudeMessages(messages []session.ClaudeConversationMessage) []*sessionv1.ClaudeMessage {
  	out := make([]*sessionv1.ClaudeMessage, 0, len(messages))
  	for _, msg := range messages {
  		out = append(out, &sessionv1.ClaudeMessage{
  			Role:      msg.Role,
  			Content:   msg.Content,
  			Timestamp: timestamppb.New(msg.Timestamp),
  			Model:     msg.Model,
  		})
  	}
  	return out
  }
  ```
- Update `GetClaudeHistoryMessages` in `search_service.go` to call `toProtoClaudeMessages(messages)` instead of its inline loop.
- Files: `server/services/search_related_work.go`, `server/services/search_service.go`

##### Task 1.3.1b: Implement `contextWindowAndBookends` (~5 min)
- In `server/services/search_related_work.go`:
  ```go
  // contextWindowAndBookends returns the clamped ±5 window around hitIndex
  // and the session's first-3/last-3 bookend messages. Bookends are empty
  // when the window already spans the full transcript, to avoid returning
  // duplicate content across the two fields.
  func contextWindowAndBookends(messages []session.ClaudeConversationMessage, hitIndex int) (window, bookendFirst, bookendLast []session.ClaudeConversationMessage) {
  	if len(messages) == 0 {
  		return nil, nil, nil
  	}
  	start := hitIndex - 5
  	if start < 0 {
  		start = 0
  	}
  	end := hitIndex + 6
  	if end > len(messages) {
  		end = len(messages)
  	}
  	window = messages[start:end]

  	if start == 0 && end == len(messages) {
  		return window, nil, nil
  	}
  	firstEnd := 3
  	if firstEnd > len(messages) {
  		firstEnd = len(messages)
  	}
  	lastStart := len(messages) - 3
  	if lastStart < 0 {
  		lastStart = 0
  	}
  	return window, messages[:firstEnd], messages[lastStart:]
  }
  ```
- Files: `server/services/search_related_work.go`

##### Task 1.3.1c: Wire into `SearchClaudeHistory` (~4 min)
- In `search_service.go`'s `SearchClaudeHistory`, after the full Task 1.2.1b block (project filter → automation filter → dedup → truncation to `requestedLimit`, in that order) — i.e. context is computed only for the final, post-truncation result set, not wastefully on raw hits that get discarded by truncation — add:
  ```go
  if req.Msg.GetIncludeContext() {
  	for _, r := range protoResults {
  		msgs, err := hist.GetMessagesFromConversationFile(r.SessionId, 0)
  		if err != nil {
  			continue // best-effort: leave context fields empty rather than failing the whole search
  		}
  		window, first, last := contextWindowAndBookends(msgs, int(r.MessageIndex))
  		r.ContextWindow = toProtoClaudeMessages(window)
  		r.BookendFirst = toProtoClaudeMessages(first)
  		r.BookendLast = toProtoClaudeMessages(last)
  	}
  }
  ```
- Files: `server/services/search_service.go`

##### Task 1.3.1d: Unit tests (~5 min)
- `server/services/search_related_work_test.go`: `TestContextWindowAndBookends_ClampsAtSessionBoundaries` (20-message case from GWT above), `TestContextWindowAndBookends_SuppressesBookendsWhenWindowCoversFullSession` (8-message ≤11 case from GWT above — this is the exact pitfall `pitfalls.md` §"Huge message counts / bookend on a 2-message session" calls out), `TestContextWindowAndBookends_EmptySessionReturnsNil`.
- Files: `server/services/search_related_work_test.go`

---

### Epic 1.4: Scroll Mode (Anchor Paging)
**Goal**: `GetClaudeHistoryMessages` accepts `anchor_index` to center a page on a message without the client computing offset math or re-running search.

#### Story 1.4.1: `anchor_index` centers the returned page
**As a** triage operator, **I want** to page forward/backward through a session's messages from a hit, **so that** I can read more context without re-issuing the search query.
**Acceptance Criteria**:
- When `anchor_index` is set and `limit > 0`, the server computes `offset = max(0, anchor_index - limit/2)` and ignores the request's own `offset`/`tail`.
  - *Given* session `a3f5c8d2-7b1e-4a90-9c3d-1f6e8b2a4c7d` has 40 messages, and a client calls `GetClaudeHistoryMessages{Id: "a3f5c8d2...", AnchorIndex: 20, Limit: 10}`, *When* the handler runs, *Then* it returns `messages[15:25]` (offset = max(0, 20-5) = 15), regardless of any `Offset`/`Tail` field also present on the request.
- When `anchor_index` is unset, behavior is byte-identical to today's offset/limit/tail semantics.
  - *Given* a request with `Offset: 10, Limit: 5` and `AnchorIndex` unset, *When* the handler runs, *Then* it returns `messages[10:15]`, matching current behavior exactly.
**Files**: `server/services/search_service.go`, `server/services/search_related_work_test.go`

##### Task 1.4.1a: Add anchor handling to `GetClaudeHistoryMessages`, and fix the out-of-bounds offset bug it makes easier to trigger (~5 min)
- **⚠ Architecture-review correction (Concern, addressed here):** the existing slicing guard `if offset > 0 && offset < len(messages) { messages = messages[offset:] }` (`search_service.go:435-437`) silently falls through to the *full unsliced list* when `offset >= len(messages)`, instead of returning an empty page. This is a pre-existing latent bug for ordinary `offset`-based callers, but `anchor_index` (`offset = anchor - limit/2`) makes it much easier to hit in practice: a stale/cached search result's `messageIndex` used as an anchor after the session has since been truncated/re-synced is a realistic path to an out-of-range anchor, unlike today's callers which typically derive `offset` from a `totalCount` fetched in the same request cycle. Fix the guard while adding anchor support, since both paths share the same slicing code:
  ```go
  offset := int(req.Msg.Offset)
  limit := int(req.Msg.Limit)
  if req.Msg.AnchorIndex != nil {
  	anchor := int(*req.Msg.AnchorIndex)
  	offset = anchor - limit/2
  	if offset < 0 {
  		offset = 0
  	}
  }
  if offset >= len(messages) {
  	messages = messages[:0]
  } else if offset > 0 {
  	messages = messages[offset:]
  }
  ```
  (This replaces both the existing `offset := int(req.Msg.Offset)` / `limit := int(req.Msg.Limit)` declarations *and* the existing `if offset > 0 && offset < len(messages) { messages = messages[offset:] }` guard at their current location — keep the existing `fileLimit`/`tail` logic above this block unchanged, since `tail` is documented as mutually exclusive with the new field and takes no special interaction here.)
- Files: `server/services/search_service.go`

##### Task 1.4.1b: Unit tests (~5 min)
- `server/services/search_related_work_test.go`: `TestGetClaudeHistoryMessages_AnchorIndexCentersWindow` (40-message case from GWT above), `TestGetClaudeHistoryMessages_ReturnsEmptyPage_When_OffsetExceedsMessageCount` (regression test for the out-of-bounds fix — both a plain out-of-range `offset` and an out-of-range `anchor_index` on a stale/shorter session), `TestGetClaudeHistoryMessages_OffsetLimitUnchanged_When_AnchorIndexUnset` (existing-behavior byte-identical check). These live alongside the other new handler-adjacent tests since `GetClaudeHistoryMessages` itself has no existing dedicated test file to extend (confirmed: only `search_pagination_test.go` exists, covering `ListClaudeHistory`'s cursor, a different RPC).
- Files: `server/services/search_related_work_test.go`

---

### Epic 1.5: Automation-Session Exclusion (Best-Effort)
**Goal**: When `exclude_automation_sessions=true`, filter out results whose session is cross-referenced to a live `Instance` with `Hidden=true` — the same field `ListSessions`' `include_hidden` flag already uses to hide background/triage/review sessions from the default session list (`session/instance.go:220-223`, `server/services/session_service.go:1089,1129`). Per ADR-002 (corrected below), this is explicitly best-effort — sessions with no live `Instance` record are left in the results, not assumed to be human sessions.
**Correction**: an earlier version of this epic/ADR-002 filtered on `Instance.AutonomousMode` instead. `AutonomousMode` is the user-facing "Fix Autonomously (Beta)" opt-in toggle (`.claude/rules/session-creation-registry.md`'s `autonomous` mode) — a *human* deliberately choosing autonomous execution, not a background/system session marker. Background/triage sessions are created via `CreateDirectorySession(..., hidden=true)` (`server/services/backlog_service.go:30`; call sites at `backlog_service_triage.go:2439-2440`, `session_service.go:827`) and do not set `AutonomousMode`. Filtering on `AutonomousMode` would both (a) fail to exclude the actual background sessions this feature exists to hide, and (b) incorrectly exclude legitimate human "Fix Autonomously" sessions. `Instance.Hidden` is the correct signal.

#### Story 1.5.1: Filter results by live `Instance.Hidden`
**As a** human user browsing history search, **I want** background/hidden sessions excluded from my results, **so that** they don't clutter results meant for reviewing my own work.
**Acceptance Criteria**:
- A result whose `SessionId` matches a currently-tracked `Instance` with `Hidden=true` is excluded.
  - *Given* search results include session `c9d2e611-...`, and `ss.getInstances()` returns an `Instance` with `GetConversationUUID() == "c9d2e611-..."` and `Hidden == true`, *When* `SearchClaudeHistory` runs with `exclude_automation_sessions=true`, *Then* session `c9d2e611-...` is absent from `results`.
- A result whose session has no matching live `Instance` (e.g., a session from 3 weeks ago whose process has long exited) is **kept**, not excluded — the filter never assumes absence-of-signal means "human."
  - *Given* search results include session `d4e3f722-...` with no matching entry in `ss.getInstances()`, *When* `SearchClaudeHistory` runs with `exclude_automation_sessions=true`, *Then* session `d4e3f722-...` remains in `results` (best-effort limitation, documented in ADR-002).
- A result whose `Instance` has `Hidden=false` but `AutonomousMode=true` (a legitimate human "Fix Autonomously" session) is **kept**, not excluded — confirming the fix targets the right field.
  - *Given* search results include session `e5f4a833-...`, and `ss.getInstances()` returns an `Instance` with `GetConversationUUID() == "e5f4a833-..."`, `Hidden == false`, and `AutonomousMode == true`, *When* `SearchClaudeHistory` runs with `exclude_automation_sessions=true`, *Then* session `e5f4a833-...` remains in `results`.
**Files**: `server/services/search_related_work.go`, `server/services/search_service.go`, `server/services/search_related_work_test.go`

##### Task 1.5.1a: Implement `isAutomationSession` / `filterAutomationSessions` (~4 min)
- In `server/services/search_related_work.go`:
  ```go
  // isAutomationSession returns true only when sessionID matches a
  // currently-live Instance with Hidden=true — the same field ListSessions'
  // include_hidden flag uses to hide background/triage/review sessions.
  // Best-effort: returns false (not automation) for any session with no
  // live Instance match, since absence of the live record is not evidence
  // either way.
  func isAutomationSession(sessionID string, instances []*session.Instance) bool {
  	for _, inst := range instances {
  		if inst.GetConversationUUID() == sessionID {
  			return inst.Hidden
  		}
  	}
  	return false
  }

  func filterAutomationSessions(results []*sessionv1.SearchResult, instances []*session.Instance) []*sessionv1.SearchResult {
  	out := results[:0]
  	for _, r := range results {
  		if isAutomationSession(r.SessionId, instances) {
  			continue
  		}
  		out = append(out, r)
  	}
  	return out
  }
  ```
- Files: `server/services/search_related_work.go`

##### Task 1.5.1b: Wire into `SearchClaudeHistory` (~3 min)
- In `search_service.go`'s `SearchClaudeHistory`, after the project filter (Task 1.6.1b) and before the `groupResultsBySession` call (Task 1.2.1b) — so filtering happens before dedup counts are computed; an excluded automation session's messages must not contribute to `more_matches_in_session_count` on a kept human session — add:
  ```go
  if req.Msg.GetExcludeAutomationSessions() && ss.getInstances != nil {
  	excludedBefore := len(protoResults)
  	protoResults = filterAutomationSessions(protoResults, ss.getInstances())
  	if excluded := excludedBefore - len(protoResults); excluded > 0 {
  		log.Info("search: excluded automation sessions", "count", excluded)
  	}
  }
  ```
- Files: `server/services/search_service.go`

##### Task 1.5.1c: Unit tests (~4 min)
- `server/services/search_related_work_test.go`: `TestFilterAutomationSessions_ExcludesSessionsWithHiddenTrue`, `TestFilterAutomationSessions_KeepsSessionsWithNoLiveInstanceMatch` (both from GWT above), `TestFilterAutomationSessions_KeepsSessionsWithHiddenFalse`, `TestFilterAutomationSessions_KeepsAutonomousModeSessionsThatAreNotHidden` (the `AutonomousMode=true, Hidden=false` case above — locks in the field-selection fix).
- Files: `server/services/search_related_work_test.go`

---

### Epic 1.6: Project Scoping (`project` Filter)
**Goal**: `SearchClaudeHistoryRequest.project` (field 2, already defined on the proto — `proto/session/v1/session.proto:962`) is currently read by `ListClaudeHistory` but silently ignored by `SearchClaudeHistory` (verified: no reference to `req.Msg.Project`/`req.Msg.GetProject()` anywhere in the existing `SearchClaudeHistory` handler, `search_service.go:459-598`, and `search.SearchOptions` has no project field). `RelatedWorkQuery` (Epic 2.2) sends `project: repoPath` on every triage search assuming it scopes results — today it's a no-op, so a triage search returns matches from every project in the user's history, not just the current repo. This epic adds a post-fetch project filter to `SearchClaudeHistory`, reusing `ClaudeHistoryEntry.Project` — the same source of truth `ListClaudeHistory` already uses — rather than teaching the BM25 engine a project concept it doesn't have.

#### Story 1.6.1: Filter `SearchClaudeHistory` results by `project` when set
**As a** triage operator, **I want** "Find related past work" to only surface sessions from the current repo, **so that** results are relevant instead of drawn from my entire cross-project history.

**⚠ Pre-mortem correction (P1, resolved below):** an earlier version of this story compared `entry.Project` (a session's live working-directory path — which for a worktree session is the *worktree* path, not the repo root, per `session/instance.go`'s distinct `Path`/`MainRepoPath`/`ClonedRepoPath` fields) against `item.RepoPath` (the canonical main-repo path) via raw string equality. Since `RelatedWorkQuery` always sends `project: item.RepoPath` by default, and any backlog item whose sessions ran in worktrees would have `entry.Project != item.RepoPath` for every one of them, that version of the filter would silently return zero results for most real items — masked as the legitimate "this looks like new territory" empty state (Story 2.2.2). Fixed by resolving worktree paths to their main repo before comparing, exactly mirroring how Epic 1.5's `isAutomationSession` already cross-references live `Instance` state instead of trusting `entry.Project` at face value.

**Acceptance Criteria**:
- When `project` is set and non-empty, only results whose session's *resolved* repo path matches exactly are returned.
  - *Given* search results include session `f1a2b3c4-...` (`entry.Project == "/home/tstapler/code/github.com/tstapler/stapler-squad"`, no live `Instance` — already a main-repo path) and session `a9b8c7d6-...` (`entry.Project == "/home/tstapler/code/github.com/tstapler/other-repo"`), *When* `SearchClaudeHistory` runs with `project: "/home/tstapler/code/github.com/tstapler/stapler-squad"`, *Then* `results` contains only session `f1a2b3c4-...`.
- **Worktree case (the P1 fix):** a session whose `entry.Project` is a worktree path is still matched against the *main* repo path via its live `Instance.MainRepoPath`.
  - *Given* search results include session `g2h3i4j5-...` with `entry.Project == "/home/tstapler/.stapler-squad/worktrees/stapler-squad-abc123"` and a live `Instance` for that session with `MainRepoPath == "/home/tstapler/code/github.com/tstapler/stapler-squad"`, *When* `SearchClaudeHistory` runs with `project: "/home/tstapler/code/github.com/tstapler/stapler-squad"`, *Then* session `g2h3i4j5-...` **is included** in `results` (not excluded, which is what naive `entry.Project == project` string equality would have done).
  - *Given* the same session `g2h3i4j5-...` but **no live `Instance`** record (worktree long since cleaned up — resolution signal unavailable), *When* the same search runs, *Then* the session is **kept** rather than excluded — matching Epic 1.5's "signal unavailable ⇒ don't assume a negative" precedent, not silently dropped as a false exclusion.
- When `project` is unset or empty, behavior is unchanged (no filtering by project, matching today).
  - *Given* the same sessions, *When* `SearchClaudeHistory` runs with `project` unset, *Then* `results` contains all of them (subject to the other filters/limits already in play).
**Files**: `server/services/search_related_work.go`, `server/services/search_service.go`, `server/services/search_related_work_test.go`

**⚠ Implementation-time correction (found during sdd:6-verify architecture review, deliberately shipped this way):** the second GWT bullet above ("no live `Instance` record ... session is kept") describes the *intended* outcome but is not what shipped. The literal Task 1.6.1a code sample (below) has `resolvedProject` fall back to the session's raw `entry.Project` when no live `Instance` matches — and `filterByProject` then compares that raw (still-worktree) path against the target `project` via plain string equality, which does **not** match, so the session is **excluded**, not kept. Keeping it unconditionally would also flip the *first* GWT bullet's outcome (`a9b8c7d6`'s genuinely-different-repo session, which likewise may have no live `Instance`, would incorrectly stay in results) — the two GWTs are not simultaneously satisfiable from `entry.Project` alone without a live `Instance` to disambiguate "unresolvable worktree" from "different project." Shipped behavior favors excluding over readmitting cross-project noise; the gap is called out explicitly in `resolvedProject`'s doc comment and locked in by `TestFilterByProject_ExcludesWorktreeSessionWithNoLiveInstance` (see `server/services/search_related_work_test.go`), which supersedes the never-implemented `TestFilterByProject_KeepsWorktreeSessionWithNoLiveInstance` this plan originally specified.

##### Task 1.6.1a: Implement `filterByProject` with worktree resolution (~6 min)
- In `server/services/search_related_work.go`:
  ```go
  // resolvedProject returns the main-repo path for a session's Project.
  // If the session has a live Instance whose Path is a worktree (i.e.
  // MainRepoPath is set), that main-repo path is returned instead of the
  // raw (worktree) Project string — mirroring isAutomationSession's
  // pattern of cross-referencing live Instance state rather than trusting
  // entry.Project at face value. Falls back to the raw project string
  // when no live Instance match exists (signal unavailable, not assumed
  // to be a mismatch) or when the session isn't a worktree.
  func resolvedProject(sessionID, project string, instances []*session.Instance) string {
  	for _, inst := range instances {
  		if inst.GetConversationUUID() == sessionID && inst.MainRepoPath != "" {
  			return inst.MainRepoPath
  		}
  	}
  	return project
  }

  // filterByProject keeps only results whose session's resolved repo path
  // (see resolvedProject) exactly matches project.
  func filterByProject(results []*sessionv1.SearchResult, project string, instances []*session.Instance) []*sessionv1.SearchResult {
  	if project == "" {
  		return results
  	}
  	out := results[:0]
  	for _, r := range results {
  		if resolvedProject(r.SessionId, r.Project, instances) == project {
  			out = append(out, r)
  		}
  	}
  	return out
  }
  ```
- Files: `server/services/search_related_work.go`

##### Task 1.6.1b: Wire into `SearchClaudeHistory` (~2 min)
- In `search_service.go`'s `SearchClaudeHistory`, immediately after `protoResults` is fully built (end of the existing per-result loop) and before the automation filter (Task 1.5.1b) — project scoping is the broadest filter and should narrow the candidate set first, so an out-of-project hit can never contribute to `more_matches_in_session_count` on a kept in-project session — add:
  ```go
  if project := req.Msg.GetProject(); project != "" {
  	var instances []*session.Instance
  	if ss.getInstances != nil {
  		instances = ss.getInstances()
  	}
  	protoResults = filterByProject(protoResults, project, instances)
  }
  ```
- Files: `server/services/search_service.go`

##### Task 1.6.1c: Unit tests (~6 min)
- `server/services/search_related_work_test.go`: `TestFilterByProject_KeepsOnlyMatchingProject` (per GWT above), `TestFilterByProject_NoOpWhenProjectEmpty`, `TestFilterByProject_ResolvesWorktreePathViaLiveInstanceMainRepoPath` (the worktree GWT above — this is the regression test for the P1 pre-mortem finding), `TestFilterByProject_KeepsWorktreeSessionWithNoLiveInstance` (signal-unavailable GWT above).
- Before Phase 5 implementation begins, additionally validate against one real (not hand-seeded) backlog item created from an actual worktree session, per pre-mortem.md item #1's prevention note — a passing unit test alone does not confirm the fix behaves correctly against real `Instance` data shapes.
- Files: `server/services/search_related_work_test.go`

---

## Phase 2: Frontend Hook + Triage UI

### Epic 2.1: Extend `useHistoryFullTextSearch`
**Goal**: Thread the three new request flags and four new response fields through the existing hook, additively.

#### Story 2.1.1: `SearchOptions`/`SearchResultItem` carry the new fields
**As a** frontend caller (the new triage component), **I want** to pass `groupBySession`/`includeContext`/`excludeAutomationSessions` through the existing hook, **so that** I don't need a second hook duplicating debounce/abort/state logic.
**Acceptance Criteria**:
- Calling `search({ query: "dark mode toggle", groupBySession: true, includeContext: true, excludeAutomationSessions: true })` results in a `SearchClaudeHistoryRequest` with those three fields set to `true`.
  - *Given* a component calls `search({ query: "dark mode toggle", project: "/home/tstapler/code/github.com/tstapler/stapler-squad", groupBySession: true, includeContext: true, excludeAutomationSessions: true, limit: 5 })`, *When* the ConnectRPC call fires, *Then* the request payload has `group_by_session: true, include_context: true, exclude_automation_sessions: true, limit: 5`.
- Existing callers that don't set the new options are unaffected (fields omitted → default `false` server-side, per Story 1.1.1).
  - *Given* `HistorySearchResults.tsx`'s existing usage (`search({ query })`, no new fields), *When* the request is sent, *Then* `group_by_session`/`include_context`/`exclude_automation_sessions` are omitted from the payload exactly as they are today (no behavior change).
- `SearchResultItem` exposes `moreMatchesInSessionCount`, `contextWindow`, `bookendFirst`, `bookendLast`, defaulting to `0`/`[]` when not requested.
**Files**: `web-app/src/lib/hooks/useHistoryFullTextSearch.ts`

##### Task 2.1.1a: Extend `SearchOptions` and `SearchResultItem` types (~3 min)
- In `useHistoryFullTextSearch.ts`, add to `SearchOptions`: `groupBySession?: boolean; includeContext?: boolean; excludeAutomationSessions?: boolean;`.
- Add a new `SearchMessageItem` interface: `{ role: string; content: string; timestamp: Date | null; model: string }`.
- Add to `SearchResultItem`: `moreMatchesInSessionCount: number; contextWindow: SearchMessageItem[]; bookendFirst: SearchMessageItem[]; bookendLast: SearchMessageItem[];`.
- Files: `web-app/src/lib/hooks/useHistoryFullTextSearch.ts`

##### Task 2.1.1b: Wire options into the RPC call and `convertResult` (~4 min)
- In the `search` callback's `clientRef.current.searchClaudeHistory({...})` call, add: `groupBySession: searchOptions.groupBySession ?? false, includeContext: searchOptions.includeContext ?? false, excludeAutomationSessions: searchOptions.excludeAutomationSessions ?? false`.
- In `convertResult`, add a small local `toSearchMessageItems(msgs: ClaudeMessage[]): SearchMessageItem[]` mapper (`role`, `content`, `timestamp: m.timestamp ? timestampDate(m.timestamp) : null`, `model`) and populate the four new `SearchResultItem` fields from `result.moreMatchesInSessionCount`, `result.contextWindow`, `result.bookendFirst`, `result.bookendLast`.
- Files: `web-app/src/lib/hooks/useHistoryFullTextSearch.ts`

---

### Epic 2.2: Triage "Find Related Past Work" Section
**Goal**: New search box in `TriageReviewPanel`, pre-populated with the backlog item title, defaulting to session-deduped, context-included, automation-excluded results scoped to the item's repo.

#### Story 2.2.1: `TriageRelatedWorkSection` renders pre-populated, editable search
**As a** triage operator, **I want** a search box pre-filled with the item's title, **so that** I see related past work without hand-writing a query.
**Acceptance Criteria**:
- On mount with a non-empty `itemTitle`, the component auto-searches using `RelatedWorkQuery` (title as query, `repoPath` as project filter, `groupBySession`/`includeContext`/`excludeAutomationSessions` all true, `limit=5`).
  - *Given* `itemTitle="Add dark mode toggle to settings page"` and `repoPath="/home/tstapler/code/github.com/tstapler/stapler-squad"`, *When* `TriageRelatedWorkSection` mounts, *Then* within one debounce interval (300ms) it calls `search({ query: "Add dark mode toggle to settings page", project: "/home/tstapler/code/github.com/tstapler/stapler-squad", groupBySession: true, includeContext: true, excludeAutomationSessions: true, limit: 5 })`.
- The query is editable; edits re-search debounced, same as `HistorySearchInput`'s pattern.
- On an empty/whitespace-only `itemTitle`, the component does not auto-search (mirrors `useHistoryFullTextSearch`'s own empty-query guard) and shows the box empty rather than firing an empty query.
  - *Given* `itemTitle=""`, *When* the component mounts, *Then* no RPC call is made and the input renders empty, unfocused.
- The section is omitted entirely when `readOnly` (matches the Actions-block precedent at `TriageReviewPanel.tsx:266`).
- **Out of scope for v1** (see Unresolved Questions): outcome badges cross-referencing `LinkedSession.reviewVerdict` — cards render snippet-only.
**Files**: `web-app/src/components/backlog/TriageRelatedWorkSection.tsx` (new), `web-app/src/components/backlog/TriageRelatedWorkSection.css.ts` (new), `web-app/src/components/backlog/TriageReviewPanel.tsx`

##### Task 2.2.1a: Create `TriageRelatedWorkSection.css.ts` (~3 min)
- New file, vanilla-extract styles per ADR-009/`.claude/rules/css-architecture.md`: `section`, `input`, `resultList` (`display: "flex", flexDirection: "column", gap: vars.space["2"]`), `resultCard` (styles a real interactive element — a `<button>` per this task's original draft, superseded by Task 2.2.3a's `<a href>` anchor element; keep the class name generic, not tied to either tag), `moreMatchesText` (renamed from an earlier `moreMatchesLink` — it is deliberately plain, non-interactive text, per `ux.md` §3/§7; do not name it "Link"), `emptyState`, `errorState` — reuse `vars.color.*`/`vars.space.*`/`vars.radii.*` tokens from `@/styles/theme.css`, matching `TriageReviewPanel.css.ts`'s token usage exactly (no hardcoded hex, no `var()` strings).
- Files: `web-app/src/components/backlog/TriageRelatedWorkSection.css.ts`

##### Task 2.2.1b: Component shell + debounced search wiring (~5 min)
- New file `TriageRelatedWorkSection.tsx`, `"use client"` + `// +feature: triage-related-work` marker on line 2.
- Props: `interface TriageRelatedWorkSectionProps { itemTitle: string; repoPath?: string }`.
- `const { results, loading, error, search, clearSearch } = useHistoryFullTextSearch({ autoSearch: false })` — ignore the hook's own `query`/`setQuery`/internal debounce since fixed options (`project`, `groupBySession`, etc.) must ride along with every search call.
- Local `const [query, setQuery] = useState(itemTitle)`; `const debouncedQuery = useDebounce(query, 300)` (import from `@/lib/hooks/useDebounce`).
- `useEffect(() => { if (!debouncedQuery.trim()) { clearSearch(); return; } search({ query: debouncedQuery, project: repoPath, groupBySession: true, includeContext: true, excludeAutomationSessions: true, limit: 5 }); }, [debouncedQuery, repoPath, search, clearSearch])`.
- Input: `<input type="search" aria-label={`Search past sessions for ${itemTitle || "this item"}`} value={query} onChange={(e) => setQuery(e.target.value)} data-testid="triage-related-work-input" className={styles.input} />` — no auto-focus (per `ux.md` §4 Focus Management finding).
- Files: `web-app/src/components/backlog/TriageRelatedWorkSection.tsx`

##### Task 2.2.1c: `SessionHitCard` rendering + result list (~5 min)
- Within the same file, an internal `SessionHitCard({ hit }: { hit: SearchResultItem })` function component rendering, in order: session title (`hit.sessionName`), date (`hit.metadata.createdAt`, reuse a small local `formatDate` or inline `toLocaleDateString`), match count (`hit.moreMatchesInSessionCount > 0` → `"+{n} more matches in this session"`), top snippet (`hit.snippets[0]?.text`, truncate to ~200 chars), as a real `<button type="button" data-testid={`triage-related-work-hit-${hit.sessionId}`}>` (not `role="button" div`).
- Wrap the list in `<ul className={styles.resultList} data-testid="triage-related-work-results">` / `<li>` per `ux.md` §4's "result list semantics" finding (native list, not a bare `<div>`).
- No nested `aria-live` — `TriageReviewPanel.tsx`'s ancestor `<section aria-live="polite">` already covers announcement (per `ux.md` §4); do not add a second one here.
- Files: `web-app/src/components/backlog/TriageRelatedWorkSection.tsx`

##### Task 2.2.1d: Insert into `TriageReviewPanel.tsx` (~3 min)
- Import `TriageRelatedWorkSection` in `TriageReviewPanel.tsx`.
- Insert between the Summary block (ends at the closing `</div>` after `styles.summaryText`) and the `{hasSuggestions && (` block:
  ```tsx
  {!readOnly && (
    <>
      <hr className={styles.divider} aria-hidden="true" />
      <TriageRelatedWorkSection itemTitle={item.title} repoPath={item.repoPath} />
    </>
  )}
  ```
- Files: `web-app/src/components/backlog/TriageReviewPanel.tsx`

#### Story 2.2.2: Triage-specific empty/error/loading copy
**As a** triage operator, **I want** a "no matches" result to read as reassuring rather than as a failure, **so that** I correctly interpret silence as "this is genuinely new work."
**Acceptance Criteria**:
- Zero results for a non-empty, completed query shows "No related past sessions found — this looks like new territory." (not the generic "No results found" copy from `HistorySearchResults.tsx`).
  - *Given* `search()` resolves with `results: [], totalMatches: 0` for query `"Add dark mode toggle to settings page"`, *When* the component re-renders, *Then* it shows the reassuring copy above, with `data-testid="triage-related-work-empty"`.
- A failed search shows an inline `role="alert"` message with a `[Retry]` action scoped to the search box, distinct from `TriageErrorBanner`'s panel-wide Apply/Skip actions.
  - *Given* `search()` rejects with a non-abort error, *When* the component re-renders, *Then* it shows `<div role="alert" data-testid="triage-related-work-error">Search failed — <button onClick={retry}>Retry</button></div>`, and this error does not surface `TriageErrorBanner`'s "Reload item"/"Skip without applying" actions.
- A fresh search (not "load more") shows a spinner only when `loading && results.length === 0`, matching `HistorySearchResults`'s existing rule.
**Files**: `web-app/src/components/backlog/TriageRelatedWorkSection.tsx`, `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx` (new)

##### Task 2.2.2a: Implement empty/error/loading states (~4 min)
- In `TriageRelatedWorkSection.tsx`, add the four-state render order (empty query → hint text only, no search fired; loading with zero results → spinner; error → inline alert + retry; zero results for a completed non-empty query → reassuring copy) before the result list render, following `HistorySearchResults.tsx`'s state-ordering precedent but with the triage-specific copy above.
- `retry` re-invokes `search(...)` with the current `debouncedQuery`/fixed options.
- Files: `web-app/src/components/backlog/TriageRelatedWorkSection.tsx`

##### Task 2.2.2b: Jest tests (~5 min)
- New `TriageRelatedWorkSection.test.tsx`: `pre-populates query with backlog item title on mount`, `does not auto-search when itemTitle is empty`, `shows reassuring copy when zero matches found`, `shows inline alert with retry on search failure`, `omits the section when readOnly is not applicable` (n/a — parent-level test, see Story 2.2.1's insertion; this file only tests the standalone component). Mock `useHistoryFullTextSearch` (or the underlying ConnectRPC client) per existing test conventions in `web-app/src/components/backlog/*.test.tsx` (check `TriageDiffSection.test.tsx` if present for the mocking pattern; otherwise mock the hook module directly via `jest.mock`).
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="TriageRelatedWorkSection"` to verify.
- Files: `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx`

#### Story 2.2.3: Click-through navigation from a `SessionHitCard`

**⚠ Consistency-check correction (BLOCKER, resolved below):** this story was referenced by name in the Dependency Visualization diagram (`Task 2.2.3b`) and required by `requirements.md`'s scroll-mode Success Metric and `design/ux.md`'s UX Acceptance Criterion #4, but was never actually written up — `SessionHitCard` (Task 2.2.1c) was specified as a `<button>` with no `onClick`. This story closes that gap, implementing the design decision `design/ux.md` §7 already made: clicking a card opens the session's history page in a new tab, anchored on the hit's message index when the route supports it.

**⚠ Triad-review correction (Engineering + UX, resolved below):** the first draft of this story assumed (a) `window.open` from a `<button>` was sufficient, and (b) `history/page.tsx` already reads a `?sessionId=` query param "per existing history-page navigation." Both were wrong: **verified** by reading `web-app/src/app/history/page.tsx` in full — session selection there happens entirely through in-page clicks calling `loadEntryDetail(result.sessionId)` (line 138); there is no `useSearchParams`/URL-driven session selection anywhere in that 408-line file today. A `?sessionId=` deep link is new plumbing this story must add, not an existing capability to merely extend — Task 2.2.3b below is rewritten accordingly. Separately, `window.open()` on a `<button>` loses native middle-click/ctrl-click/right-click "open in new tab" affordances and screen-reader advance warning of a new window (WCAG G201) that a real `<a href target="_blank">` gets for free — Task 2.2.3a is rewritten to use an anchor element instead.

**As a** triage operator, **I want** to open a hit's full surrounding conversation from its result card, **so that** I can review prior art in depth without losing my place in the triage panel.
**Acceptance Criteria**:
- Activating a `SessionHitCard` (click, middle-click, ctrl-click, Enter, or Space — all native anchor-element behaviors) opens `web-app/src/app/history/page.tsx` in a **new tab**, scoped to that session, without altering the triage panel's own state.
  - *Given* a rendered `SessionHitCard` for session `a3f5c8d2-...` with `messageIndex=10`, *When* the user activates the card by any of the above means, *Then* a new tab opens at `/history?sessionId=a3f5c8d2-...&messageIndex=10`, the history page loads that session (not its default landing state), and the triage panel's search box value, results, and any in-progress Apply/Skip/Refine state are unchanged in the original tab.
- If `messageIndex` centering isn't wired at implementation time, the fallback is opening the session's default view (not erroring, not blocking Story 2.2.3) — matching `design/ux.md` §7's "ship the fallback, don't block on a new deep-link route" framing. This fallback applies **only** to `messageIndex` centering, not to `sessionId` selection itself, which is required for this story to do anything useful.
  - *Given* the anchor/centering piece isn't wired yet, *When* the same card is activated, *Then* the new tab opens at `/history?sessionId=a3f5c8d2-...`, the correct session loads (unanchored), and no error occurs.
- The "+N more matches in this session" text (Task 2.2.1c) is documented as informational only, not a separate navigation target: activating anywhere on the card — including over that text — navigates to the session's history page as above, where all matching messages (not just the top-scored one) are visible by scrolling/browsing that session. This is stated explicitly so the affordance isn't mistaken for a broken secondary link.
**Files**: `web-app/src/components/backlog/TriageRelatedWorkSection.tsx`, `web-app/src/app/history/page.tsx`

##### Task 2.2.3a: Render `SessionHitCard` as a real anchor element (~4 min)
- In `TriageRelatedWorkSection.tsx`, change `SessionHitCard`'s root element from `<button type="button">` (Task 2.2.1c) to `<a href={`/history?sessionId=${hit.sessionId}&messageIndex=${hit.messageIndex}`} target="_blank" rel="noopener noreferrer" data-testid={`triage-related-work-hit-${hit.sessionId}`} className={styles.resultCard}>`, styled identically to the button variant (per `TriageRelatedWorkSection.css.ts`, Task 2.2.1a — `resultCard` already targets a generic interactive-element selector, confirm it isn't `button`-specific). This preserves Tab-order + Enter/Space activation natively, and additionally gets middle-click/ctrl-click "open in new tab" and screen-reader new-window announcement for free — none of which `window.open()` from a `<button>` provides.
- Files: `web-app/src/components/backlog/TriageRelatedWorkSection.tsx`, `web-app/src/components/backlog/TriageRelatedWorkSection.css.ts`

##### Task 2.2.3b: Add `?sessionId=`/`?messageIndex=` deep-link handling to `history/page.tsx` (~15 min — larger than originally estimated; this is new plumbing, not a confirm-and-tweak)
- `history/page.tsx` today has no URL-driven session selection (verified: only in-page click handlers call `loadEntryDetail`, e.g. line 138). Add, near the page's existing data-loading effect: read `sessionId`/`messageIndex` via `useSearchParams()` (Next.js), and on mount, if `sessionId` is present, call the existing `loadEntryDetail(sessionId)` path directly (bypassing the normal list-click flow) instead of waiting for a user click.
- If `messageIndex` is also present, pass it through as `GetClaudeHistoryMessagesRequest.anchor_index` (Task 1.1.1a/1.4.1a) on that same initial fetch; if wiring this additional piece proves nontrivial at implementation time, ship the `sessionId`-only fallback (session loads at its default, unanchored view) rather than blocking the story on it — per this story's second acceptance criterion. The `sessionId` piece itself is not optional: without it, Story 2.2.3 has no observable effect at all.
- Files: `web-app/src/app/history/page.tsx`

##### Task 2.2.3c: Jest test (~4 min)
- `TriageRelatedWorkSection.test.tsx`: `SessionHitCard renders as a link targeting the session's history page with sessionId and messageIndex` (asserts the rendered `<a>`'s `href`, `target`, and `rel` attributes — no `window.open` mocking needed now that it's a real anchor).
- `history/page.test.tsx` (or equivalent, if one exists — check first): `loads the session named in ?sessionId= on mount without requiring a list click`.
- Files: `web-app/src/components/backlog/TriageRelatedWorkSection.test.tsx`, `web-app/src/app/history/page.test.tsx` (new if absent)

---

## Phase 3: Registry & E2E

### Epic 3.1: Feature Registry Updates
**Goal**: Register the extended backend RPC and the new frontend surface per `.claude/rules/feature-registry.md`.

#### Story 3.1.1: Update/add per-feature registry entries
**As a** repo maintainer, **I want** the registry to reflect the extended `SearchClaudeHistory` RPC and the new triage UI, **so that** `make registry-generate` doesn't report a coverage gap.
**Acceptance Criteria**:
- `docs/registry/features/backend/history/search.json` has an updated `lastModified` and the new test IDs from Phase 1.
- A new `docs/registry/features/frontend/ui/triage-related-work.json` exists with `markerFound`-equivalent `markerLine: 2` pointing at the `// +feature: triage-related-work` comment added in Task 2.2.1b.
- `make registry-generate` produces no unexplained net increase in `docs/registry/coverage-gaps.json`.
**Files**: `docs/registry/features/backend/history/search.json`, `docs/registry/features/frontend/ui/triage-related-work.json`

##### Task 3.1.1a: Update `search.json` (~2 min)
- Edit `docs/registry/features/backend/history/search.json`: update `lastModified` to the implementation date, append `testIds`: `["TestGroupResultsBySession_KeepsHighestScoredHitPerSession", "TestContextWindowAndBookends_SuppressesBookendsWhenWindowCoversFullSession", "TestFilterAutomationSessions_ExcludesSessionsWithHiddenTrue", "TestGetClaudeHistoryMessages_AnchorIndexCentersWindow", "TestFilterByProject_ResolvesWorktreePathViaLiveInstanceMainRepoPath"]`, set `"tested": true`.
- Files: `docs/registry/features/backend/history/search.json`

##### Task 3.1.1b: Create `triage-related-work.json` (~2 min)
- New file, following the `history-search.json` shape:
  ```json
  {
    "id": "triage-related-work",
    "type": "frontend",
    "component": "TriageRelatedWorkSection",
    "path": "web-app/src/components/backlog/TriageRelatedWorkSection.tsx",
    "markerLine": 2,
    "tested": true,
    "testIds": [
      "pre-populates query with backlog item title on mount",
      "shows reassuring copy when zero matches found",
      "e2e:triage-related-work - Find related past work surfaces prior sessions"
    ],
    "lastModified": "2026-08-02T00:00:00Z"
  }
  ```
- Files: `docs/registry/features/frontend/ui/triage-related-work.json`

##### Task 3.1.1c: Regenerate aggregates (~2 min)
- Run `make registry-generate`; run `make registry-diff` to confirm no unexpected changes; commit the regenerated aggregate files alongside the per-feature JSON edits.
- Files: (generated) `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json`

---

### Epic 3.2: E2E Coverage
**Goal**: A Playwright spec exercising the triage search box end-to-end, per `.claude/rules/e2e-test-conventions.md`.

#### Story 3.2.1: `triage-related-work.spec.ts`
**As a** repo maintainer, **I want** e2e coverage of the pre-populated search box, **so that** a regression in the RPC wiring or component is caught in CI.
**Acceptance Criteria**:
- Spec starts with `// @feature triage-related-work`.
- Uses only `data-testid`/ARIA locators (`triage-related-work-input`, `triage-related-work-results`, `triage-related-work-hit-*`).
- No `waitForTimeout` — waits on `expect(locator).toHaveValue(...)` / `waitForSelector`.
  - *Given* a seeded backlog item with `title="Add dark mode toggle to settings page"` in triage-completed state (reusing the existing `seedHeadlessTriageSession` debug endpoint pattern from `BacklogItemDetailPage.ts`), *When* the e2e test navigates to that item's detail page, *Then* `page.getByTestId("triage-related-work-input")` eventually has value `"Add dark mode toggle to settings page"` (auto-populated), verified via `toHaveValue`, not a timeout.
- Activating a result card (Story 2.2.3) opens a new tab/page targeting the session's history route.
  - *Given* at least one result renders (`page.getByTestId(/^triage-related-work-hit-/)`), *When* the test clicks the first card and awaits the `context.waitForEvent("page")` popup, *Then* the new page's URL contains `/history?sessionId=`.
**Files**: `tests/e2e/triage-related-work.spec.ts` (new), `tests/e2e/pages/BacklogItemDetailPage.ts`

##### Task 3.2.1a: Add locator methods to `BacklogItemDetailPage.ts` (~3 min)
- Add `relatedWorkInput(): Locator { return this.page.getByTestId("triage-related-work-input"); }` and `relatedWorkResults(): Locator { return this.page.getByTestId("triage-related-work-results"); }`, following the existing `triageReviewPanel()` method's pattern at line 73.
- Files: `tests/e2e/pages/BacklogItemDetailPage.ts`

##### Task 3.2.1b: Write the spec (~5 min)
- New `tests/e2e/triage-related-work.spec.ts`: `// @feature triage-related-work` header, import `FEATURE_CATALOG['triage-related-work']` per the existing `history-search.spec.ts` pattern, seed a headless-triage item via the existing debug endpoint, navigate to its detail page, assert the input auto-populates with the item's title (GWT above), assert the results region renders without error (loading → resolved state, no fixed timeout).
- Files: `tests/e2e/triage-related-work.spec.ts`
