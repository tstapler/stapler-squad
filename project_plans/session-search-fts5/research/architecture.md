# Architecture Research: session-search-fts5

Builds on `project_plans/session-search-fts5/requirements.md` — does not re-derive
the baseline inventory there. Findings below are from reading
`server/services/search_service.go:1-598` (full), `session/search/engine.go:1-693`
(full), `session/search/document_store.go:1-211` (full), `session/history.go:79-199`,
`proto/session/v1/session.proto:846-1030`, `session/instance.go`, and
`session/storage.go` in full.

## 1. Where session-dedup logic belongs

**Verdict: RPC handler (`SearchService.SearchClaudeHistory`), not `SearchEngine.Search`.**

`SearchEngine.Search` (`session/search/engine.go:191`) returns one `SearchResult`
per matching *document* (message), scored independently by BM25 via
`e.scorer.ScoreAll(queryTokens)`. Its contract is "rank documents," which is the
right layer for term scoring — collapsing to session-level is a *presentation*
concern layered on top of ranked documents, not an indexing/scoring concern.
Two structural reasons to keep it out of the engine:

- `SearchOptions` (`engine.go:23`) already has a `SessionID` field used to scope
  a search *to* one session (used by nothing yet, but clearly intended for a
  future "search within this session" mode) — overloading `Search()` to also
  *group by* session would conflate two unrelated options on the same struct.
- The engine has no concept of "top hit" tie-breaking policy (highest score
  first? most recent match first?) — that's a product decision that belongs
  with the RPC/response-shaping code in `search_service.go`, which already does
  comparable post-processing (snippet generation, entry enrichment via
  `hist.GetByID`, lines 541-590).

Concretely: after `ss.searchEngine.Search(...)` returns `searchResults.Results`
(flat, already sorted by score), the handler groups by `result.SessionID`,
keeps the highest-scored hit per session as the representative result, and
counts the rest as `MoreMatchesInSession`. This is a simple post-pass over an
already-sorted slice — O(n), no new engine method required. `TotalMatches` in
the response needs a decision: does it mean "matching messages" (current
semantics, unchanged) or "matching sessions" (new)? Recommend keeping
`total_matches` message-level (backward compatible) and adding a new
`total_sessions` field for the deduped count — see §2.

## 2. Does `Document`/`DocumentStore` already have enough for ±5 window + bookends?

**Partially — SessionID + MessageIndex are present and sufficient to *identify*
the window, but nothing today extracts it, and doing so cheaply requires two
changes:**

`Document` (`document_store.go:9`) has exactly `SessionID`, `MessageIndex`,
`MessageRole`, `Content`, `WordCount`, `Timestamp` — no back/forward links.
`DocumentStore.GetBySession(sessionID)` (`document_store.go:85`) returns all
docs for a session but:

- **only messages that survived tokenization.** `IndexMessage`/`indexSessionLocked`
  skip messages where `tokenizer.Tokenize(content)` yields zero tokens
  (`engine.go:156-159`, `544-547`) — e.g., empty messages, pure-whitespace, or
  content that's entirely stopwords/punctuation. This means `GetBySession`
  is **not a complete, contiguous transcript** — it silently has holes at
  whatever `MessageIndex` values got skipped. A ±5-message window built purely
  from `DocumentStore` would jump over skipped indices without knowing it.
- **document order within `SessionIndex[sessionID]` is insertion order**
  (append-only slice, `Add()` at `document_store.go:47-58`), which happens to
  match `MessageIndex` order today because indexing walks `messages` in order,
  but it's an incidental property, not an enforced invariant — nothing sorts
  by `MessageIndex` before use.

**Conclusion:** for a *correct* ±5 window and bookend messages (first 3 / last
3), don't reconstruct from `DocumentStore`. Instead reuse the existing
`GetClaudeHistoryMessages` path (`search_service.go:404-456`), which calls
`hist.GetMessagesFromConversationFile(id, 0)` (`session/history.go:558`) — this
reads the *actual* JSONL conversation file and returns every message in true
chronological order with no tokenizer-driven gaps. `MessageIndex` on a
`SearchResult` is the index into *that* same slice (see `engine.go:104`,
`msgIdx` from `range messages` in `BuildIndex`/`indexSessionLocked` — the same
loop `GetClaudeHistoryMessages` uses), so it's directly usable as a slice
index: `messages[max(0,idx-5) : min(len,idx+6)]` for the context window, plus
`messages[:3]` / `messages[len-3:]` for bookends. This is a few lines added
to the existing `GetClaudeHistoryMessages`-adjacent code path, not new
storage. No new `Document` field needed — `MessageIndex` is enough *as long as
context extraction re-reads the conversation file* rather than walking
`DocumentStore`.

Practical implication for §3 (RPC shape): computing a context window per hit
means an extra `hist.GetMessagesFromConversationFile` call per session in the
result set (session-deduped, so at most `limit` calls, e.g. ≤20 by default).
Each call re-reads one JSONL file from disk — same cost `GetClaudeHistoryMessages`
already pays per request today, just multiplied by result count. Worth a
latency check in Phase 4 validation but not an architectural blocker (local
disk, small files, existing SLO is "same order of magnitude as today").

## 3. New RPC vs. extending `SearchClaudeHistoryRequest`/`Response`

**Verdict: extend the existing request/response with optional fields (no new
RPC for dedup + context window); Scroll mode reuses `GetClaudeHistoryMessages`
with a new anchor param rather than inventing an RPC.**

Concretely, following the proto's existing `optional`/default-false-bool
conventions (`SearchClaudeHistoryRequest` already uses `optional string
project`, `optional string model`, etc. at lines 958-973):

- `SearchClaudeHistoryRequest`: add `optional bool group_by_session = 8;`
  and `optional bool include_context = 9;` (next available field numbers —
  confirm exact next-free number at implementation time). Both default `false`
  so existing callers (`useHistoryFullTextSearch.ts`, `HistorySearchResults.tsx`)
  are byte-for-byte unaffected — this directly satisfies the requirements
  doc's constraint ("changes to response shape need to be additive").
- `SearchResult`: add `repeated SearchResult more_matches_in_session = 8;` (or
  an `int32 more_matches_in_session_count = 8` if only a count is needed —
  cheaper, avoids duplicating full result payloads; recommend the count-only
  form unless UI needs to list the other hits inline) and
  `repeated ClaudeMessage context_window = 9` / `bookend_first = 10` /
  `bookend_last = 11` for context, populated only when the corresponding
  request flag is set. Existing consumers ignore unknown-to-them fields by
  construction (protobuf field addition is wire-compatible), and since the
  fields are only *populated* when the new request flags are set, behavior
  for existing callers is unchanged even at the Go struct level.
- **Scroll mode** does not need a new RPC. `GetClaudeHistoryMessagesRequest`
  (`session.proto:924`) already has `id`, `limit`, `offset`, `tail` — anchor-based
  forward/backward paging is `offset` arithmetic around a known `MessageIndex`
  (the anchor), which the client already has from a `SearchResult`. No proto
  change is strictly required; at most, consider adding an `optional int32
  anchor_index = 5` convenience field so the client doesn't have to compute
  `offset = anchor - N` itself, but that's a UX-polish decision for Phase 3,
  not a new integration point.

This resolves Open Question #2 from requirements.md: **extend
`SearchClaudeHistoryRequest`/`Response` with optional fields; no `DiscoverSessions`
RPC.** And Open Question #3: **Scroll mode extends `GetClaudeHistoryMessages`**,
which already exists and already does offset/limit/tail paging — it's the
"exact existing message-fetch RPC surface" the question asked about confirming.

## 4. Does `IncrementalSync` still need to run for the new modes?

**Yes for search/discovery (unchanged); no for scroll mode — confirmed
architecturally, not just assumed.**

`IncrementalSync(hist)` (`engine.go:399`, called at `search_service.go:481`)
exists to keep the BM25 index consistent with on-disk JSONL state before
scoring a query — it's inherent to *searching*, so `group_by_session` and
`include_context` (both riding on `SearchClaudeHistory`) still need it: they
still call `Search()` and need a fresh index to score against.

Scroll mode, per §3, is implemented as `GetClaudeHistoryMessages` calls, which
**already skip the search index and sync entirely** — that handler
(`search_service.go:404-456`) only touches `getOrRefreshHistoryCache` (the
lightweight `history.jsonl` cache, TTL-based) and
`hist.GetMessagesFromConversationFile`, never `ss.searchEngine`. So scroll
mode's lower latency isn't something to *build* — it's a free consequence of
reusing the RPC that was never coupled to the search index in the first place.
No engine change, no new "skip sync" flag needed anywhere.

## 5. Simple CRUD or multi-actor business logic? (EventStorming table)

**Skip it — confirmed.** All five in-scope changes are single-actor,
read-path query/response shaping:

- Session dedup: pure function over an already-computed `[]SearchResult`.
- Context window/bookends: a second read (`GetMessagesFromConversationFile`)
  keyed off data already in hand.
- Scroll mode: existing RPC, new client usage pattern.
- `source=tool` filter: a predicate applied during/after `Search()` (see §6).
- Triage panel search box: a new frontend caller of an existing RPC.

None of these involve commands that mutate shared state, multiple actors
racing on the same aggregate, or domain events with side effects — the
defining triggers for reaching for EventStorming. This matches the
requirements doc's own "Risk Control: not needed — low risk" call and the
"Complexity: 3 — system design" framing (structural work across layers, not
domain-event complexity).

## 6. Where should `source=tool` exclusion live, and does `Document` need a new field?

**This is the one open architectural gap requirements.md correctly flagged
(Feasibility Risk, Open Question #1) — confirmed here with the actual schema,
and the answer is more constrained than "add a field to `Document`."**

Traced the full data lineage for a search hit:

1. `~/.claude/history.jsonl` (`historyJSONLEntry`, `session/history.go:95-100`)
   — **owned and written by the Claude Code CLI itself, not by stapler-squad.**
   Fields: `display`, `timestamp`, `project`, `sessionId`. No source/origin field.
2. Per-session conversation JSONL (`conversationMessage`,
   `session/history.go:80-91`, read via `findConversationFilePath` +
   `GetMessagesFromConversationFile`) — also Claude-CLI-owned. Fields per
   message: `type`, `uuid`, `sessionId`, `timestamp`, `cwd`,
   `message.{role,model,content}`. No source/origin field either.
3. `session.search.Document` (`document_store.go:9`) is built entirely from
   fields synthesized in (1)+(2) at index time (`engine.go:112-119`,
   `550-557`) — there is no origin signal available *to add* into `Document`,
   because the upstream Claude-owned files don't carry one. Adding a field to
   `Document` would just be a place to *store* a signal that has to come from
   somewhere else.

**The only origin signal that exists anywhere in this repo is
`session.Instance.AutonomousMode` (`session/instance.go:166` /
`session/storage.go:68`, persisted alongside `ConversationUUID`
(`storage.go:163`) in the live session store.** This lets the RPC handler
cross-reference a search hit's `SessionID` (= Claude conversation UUID)
against `Storage`'s persisted instance records to check `AutonomousMode` —
**but only for sessions whose `Instance` record still exists.**
`Storage.DeleteInstance` (`storage.go:458`) removes a session's persisted
record once the user (or reconciliation) deletes/archives it — there's no
"closed sessions" archive that retains `AutonomousMode` indefinitely. So this
signal is **best-effort**: reliable for recent/still-tracked sessions,
silently unavailable for older history entries whose `Instance` was cleaned
up — at that point the search hit has no way to know whether it came from a
human or an autonomous/background run.

**Recommendation for Phase 3 planning** (this is a decision point, not fully
resolved here):

- **Placement**: the filter must live in the RPC handler
  (`SearchService.SearchClaudeHistory`), *not* in `SearchEngine`/`Document` —
  the engine has no access to `Storage`/`Instance` state and shouldn't gain a
  dependency on it (would violate the existing layering where `session/search`
  only depends on `session.ClaudeSessionHistory`). The handler already
  cross-references `hist.GetByID` and `ss.getInstances()` for
  `liveSessionStatus` (`search_service.go:157-167`) — this is the same
  pattern, extended to look up `AutonomousMode` via `Storage` instead of/in
  addition to live instances.
- **No new `Document` field** — there's nothing upstream to populate it with
  for the general case. If exhaustive coverage (including sessions with
  deleted `Instance` records) turns out to be a hard requirement, the fallback
  is a path-based heuristic (e.g., worktree paths matching the backlog
  automation directory convention) applied to `entry.Project` at filter time —
  cheap, no schema change, but a heuristic, not a guarantee. Flag this
  explicitly as a known gap in Phase 3 rather than silently shipping a filter
  that "mostly" works.
- This directly resolves Open Question #1: **no existing field distinguishes
  user vs. background/tool sessions; the closest available signal is
  `Instance.AutonomousMode`, and it is not durable past `DeleteInstance`.**

## Summary of resolved Open Questions (from requirements.md)

1. No existing field distinguishes source; `Instance.AutonomousMode` (cross-referenced
   by `ConversationUUID`) is the best available signal and is non-durable past
   session deletion — needs an explicit "best-effort" call-out in the plan.
2. Extend `SearchClaudeHistoryRequest`/`Response` with optional fields
   (`group_by_session`, `include_context`, plus response fields for
   more-matches-count and context/bookend messages) — no new RPC.
3. Scroll mode extends `GetClaudeHistoryMessages` (existing RPC, offset/limit/tail
   already present); it never touched the search index, so it already skips
   `IncrementalSync` by construction.
