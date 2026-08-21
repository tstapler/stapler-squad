# Build vs. Buy: Extend BM25 vs. Migrate to SQLite FTS5

**Date**: 2026-08-02
**Scope**: Evaluates the four missing pieces from `requirements.md` (session dedup, ±5 context window + bookend messages, scroll mode, `source=tool` filter) against two architectures: extending the existing hand-rolled BM25 engine (`session/search/`) vs. migrating to SQLite FTS5 as the original backlog item proposed.

## Grounding: what was actually read

- `session/search/engine.go` (694 lines) — `SearchEngine.Search()`, `IncrementalSync()`, `BuildIndex()`.
- `session/search/document_store.go` (212 lines) — `Document`, `DocumentStore`.
- `server/services/search_service.go:459` (`SearchClaudeHistory` handler) and its request/response wiring.
- `web-app/src/lib/hooks/useHistoryFullTextSearch.ts` — existing frontend consumer contract.
- `proto/session/v1/session.proto:75-91, 924-943` — `GetClaudeHistoryMessages` RPC (already has `offset`/`limit`/`tail`).
- `session/history.go:18-33` — `ClaudeHistoryEntry` struct (no `source`/origin field today).
- `go.mod:24` — `github.com/mattn/go-sqlite3 v1.14.40`.
- `session/ent_repository.go:48,76,89` — ent's SQLite DB path (`~/.stapler-squad/sessions.db`) and `entsql.OpenDB(dialect.SQLite, db)`.
- `Makefile:130-141` (build target), `Makefile:14` (`CGO_ENABLED := 1`) — no `sqlite_fts5`/`fts5` build tag present anywhere in the repo (confirmed via `grep -rn "sqlite_fts5\|fts5" .` returning no build-tag usage, only this ticket's own docs).

## Option 1: Extend the existing BM25 engine

### What the data model already gives you for free

`DocumentStore.SessionIndex map[string][]int32` (`document_store.go:30`) accumulates doc IDs per session in the order `Add()` is called. Since `indexSessionLocked`/`buildIndexLocked` (`engine.go:537-572`, `607-657`) iterate `messages` by `msgIdx` ascending and call `docStore.Add()` in that order, `DocumentStore.GetBySession(sessionID)` (`document_store.go:85-97`) already returns docs pre-sorted by `MessageIndex`. Context-window and bookend slicing is a `sort`-free array slice over data that already exists.

One caveat: `indexSessionLocked` skips messages where `tokenizer.Tokenize(msg.Content)` is empty (`engine.go:546-548`) — so `DocumentStore` is a *filtered* view (indexable messages only), not the full transcript. Bookend/context-window logic that wants the literal first-3/last-3 or ±5 raw messages (including empty/tool-only messages) should read through `history.GetMessagesFromConversationFile(sessionID, 0)` (already used by `GetClaudeHistoryMessages`, `session_service.go`/`search_service.go`) rather than `DocumentStore`, and use `DocumentStore` only to know *which* message index scored a hit. This is a one-line source substitution, not a redesign.

### Per missing piece

1. **Session dedup** — `Search()` (`engine.go:191-259`) returns one `SearchResult` per matching message. Grouping by `SessionID`, keeping the top-scored hit and counting the rest, is a single post-processing pass over `searchResults.Results` — no index/scorer change needed. Fits as new logic in `search_service.go` (or a `GroupBySession(results []SearchResult) []SessionGroup` helper in `session/search/`) without touching `InvertedIndex`/`BM25Scorer`.
2. **±5 context window / bookend messages** — as above: slice `history.GetMessagesFromConversationFile(sessionID, 0)` around the hit's `MessageIndex`, or take `[:3]`/`[-3:]` for bookends. No index change.
3. **Scroll mode** — `GetClaudeHistoryMessages` (`proto/session/v1/session.proto:924-936`) already accepts `offset`/`limit`/`tail`. An "anchor on message ID, page forward/backward" scroll mode is either (a) client-side: compute `offset = anchorIndex - N` and call the existing RPC, or (b) a small server-side convenience (`anchor_index` field) that does the same math once. Either way this is additive to an RPC that already does 90% of the work; it is not new indexing/storage capability.
4. **`source=tool` filter** — confirmed by reading `session/history.go:18-33`: `ClaudeHistoryEntry` has no source/origin field today (`ID`, `Name`, `Project`, `CreatedAt`, `UpdatedAt`, `Model`, `MessageCount` only). This is a genuine gap, but it's a data-model gap independent of BM25 vs. FTS5 — whichever engine is chosen, the signal has to be added at the same layer (history entry or session metadata) and threaded through indexing as a filterable field. FTS5 doesn't make this problem easier; it's arguably harder there (see Option 2, filtered-column handling in a virtual table).

### Pros
- All four gaps are query/response-shaping additions over data the engine already produces — no changes to `InvertedIndex`, `bm25.go`, or the on-disk index format (`index_store.go`).
- Zero migration/dual-write risk; the existing `IncrementalSync` pull-on-request model (`engine.go:399-496`) is untouched.
- No new build tag, no CGO surface change, no new failure mode in `make build`/CI.
- Existing test suites (`engine_test.go`, `document_store.go` has no direct test file but is covered via `engine_test.go`/`inverted_index_test.go`, `bm25_test.go`, `snippet_test.go`) keep covering the scoring/indexing core; new tests only need to cover the new grouping/slicing/filtering functions.
- Existing frontend contract (`useHistoryFullTextSearch.ts`) is unaffected unless the response shape changes — and the requirements doc already flags this as something to do additively (new field, not a breaking shape change).

### Cons
- The engine is in-memory and rebuilds/holds state per-process; this was already true before this ticket and is out of scope to fix here — not a new cost imposed by "extend," just an existing property inherited either way.
- `source=tool` filter needs a new field added to `ClaudeHistoryEntry` (or equivalent) and threaded through JSONL parsing — real work, but orthogonal to the BM25/FTS5 choice.

### Verdict: **Recommended**

## Option 2: Migrate to SQLite FTS5

### Build tag reality check

`mattn/go-sqlite3` compiles FTS5 support only when built with the `sqlite_fts5` build tag (FTS5 is not compiled into the driver by default). `grep -rn "sqlite_fts5\|fts5" .` across the repo returns nothing except this ticket's own docs — confirming the tag is not present anywhere today. Adopting FTS5 would require adding `-tags sqlite_fts5` (or extending the existing tag set, e.g. alongside `embed_tmux` at `Makefile:257-269`) to *every* build/test/CI invocation that touches the search path: `make build` (`Makefile:130-141`), `make test`/`make ci`, `go build .` (README/CLAUDE.md's documented direct-build path), and any `go test ./session/search/...` invocation run outside `make`. Missing the tag on any one of these silently compiles a binary where `CREATE VIRTUAL TABLE ... USING fts5(...)` fails at runtime with "no such module: fts5" — a footgun with no compile-time signal, cross-cutting every build entry point in the repo, not just the search package.

### Where it could live

`session/ent_repository.go:48,76,89` shows `~/.stapler-squad/sessions.db` is an existing SQLite file (ent-backed, `dialect.SQLite`) already used for `backlog_item`, `session`, etc. (`session/ent/schema/*.go`). An FTS5 virtual table (or a satellite `sessions_search.db` `ATTACH`ed to it) could technically live alongside that file. But:
- ent's schema is entirely code-generated (`session/ent/schema/` → `go generate` per `.claude/rules/ent-schema-generation.md`, requiring the `--feature sql/upsert` flag). FTS5 virtual tables are not first-class ent schema objects — ent doesn't generate FTS5 `CREATE VIRTUAL TABLE`/`fts5vocab` bindings, so this would need hand-written SQL migrations living outside the generated ent layer, a second migration mechanism next to ent's.
- Claude history entries (`session/history.go`) are JSONL-backed, not ent-backed at all — there's no existing ent schema for `ClaudeHistoryEntry`/messages. Standing up FTS5 means either (a) adding a new ent entity + generated code for something that today is a flat-file read, or (b) hand-rolling a parallel SQLite table+trigger setup outside ent entirely, duplicating the "keep index in sync with JSONL" logic `IncrementalSync` already solves in `engine.go`.

### Migration cost inventory (concrete, not hand-wavy)
- Reindex: every session's messages need a one-time bulk `INSERT INTO fts5table`, replacing the working `BuildIndex`/`IncrementalSync` pull-based flow.
- Cutover: `SearchClaudeHistory`/`ListClaudeHistory` handlers (`search_service.go`) would need a parallel code path (BM25 query builder → FTS5 `MATCH` query builder), snippet generation would move from the hand-rolled `snippet.go` `SnippetGenerator` to FTS5's `snippet()`/`highlight()` SQL functions (different highlighting semantics — a rewrite of `HighlightRange` computation, not a port).
- Dual-write risk during cutover: any window where both engines are live and could disagree on results is new operational risk that doesn't exist today.
- Full regression surface: `bm25_test.go`, `engine_test.go`, `inverted_index_test.go`, `index_store_test.go`, `snippet_test.go` (5 test files, ~50KB of test code) all test the engine being replaced — none of it carries over to an FTS5 implementation.

### Pros
- FTS5 has built-in phrase queries (`"exact phrase"`), `NEAR()`, and BM25 ranking (`bm25()` SQL function) out of the box — capability the hand-rolled engine would have to implement itself if ever needed. **Not currently needed**: nothing in `requirements.md`'s scope (dedup, context window, scroll, source filter) requires phrase queries or `NEAR()`; the existing tokenizer+inverted-index already does BM25 ranking (`bm25.go`).
- Offloads storage/durability to SQLite's WAL rather than the hand-rolled `index_store.go` serialization format.

### Cons
- New build tag (`sqlite_fts5`) needed across every build/test/CI entry point, with a silent-failure footgun if any invocation misses it.
- No ent-generated path for FTS5 virtual tables — a second, hand-written migration mechanism alongside ent's generated one.
- Full reindex + cutover + dual-write risk for a system that already works, to solve requirements (dedup, context window, scroll, source filter) that are unrelated to search algorithm/storage choice — none of the four gaps are things BM25-in-memory structurally can't do.
- Discards 5 existing test files' coverage of the current engine with no direct replacement value for the actual scope of this ticket.
- Directly contradicts the requirements doc's own constraint (`requirements.md:51`): "Must reuse the existing `session/search` engine... unless Phase 2 research concludes the BM25 engine cannot reasonably support scroll/dedup" — this research finds no such limitation.

### Verdict: **Not recommended**

## Option 3: Hybrid — BM25 for existing modes, FTS5 for new modes

Two search backends over the same underlying JSONL data source, kept eventually consistent independently, queried through different code paths depending on which "mode" (search vs. scroll vs. browse) the caller is in.

### Why this doesn't hold up
- The four "new modes" (dedup, context window, scroll, source filter) are not independent search algorithms — they're response-shaping and filtering operations *on top of* a single ranked-match input. Session dedup needs to dedup the *same* ranked list a plain search returns; splitting that ranked list's source across two engines means two different relevance orderings could disagree about which session's hit is "the top one," a correctness bug with no clean resolution.
- Scroll mode isn't search at all — it's `GetClaudeHistoryMessages` with an anchor, already reading straight from JSONL via `history.GetMessagesFromConversationFile`. It doesn't need *any* search backend, BM25 or FTS5.
- Splitting index-freshness guarantees across two engines doubles the sync-correctness surface (`IncrementalSync` for BM25 + a second trigger-or-poll sync for FTS5) for zero functional gain — nothing in scope needs FTS5-only capability (phrase queries, `NEAR()`).
- Operationally this is strictly worse than either pure option: all of FTS5's migration/build-tag cost (Option 2) *plus* the ongoing maintenance burden of two indexing pipelines, with no scope item that actually requires the second backend.

### Verdict: **Not recommended** — confirmed, not just presumed. There is no scope item in `requirements.md` that needs FTS5-specific capability, so paying for two backends buys nothing.

## Option 4: Hand-rolled Go slicing/grouping vs. a library, for the 4 missing pieces

Concretely: dedup-by-session (map + max-by-score), ±5 context window (index arithmetic + slice bounds), bookend messages (`messages[:3]` / `messages[len-3:]`), scroll pagination (offset arithmetic, already mirrors `GetClaudeHistoryMessages`'s existing `offset`/`limit`/`tail` params), and a source-type equality filter.

### Assessment
These are textbook slice/map operations over data already in memory (`[]SearchResult`, `[]ClaudeMessage`) — `O(n)` grouping, bounds-checked slicing, a boolean predicate filter. None of them are: sorting-with-custom-comparators-at-scale, string/text algorithms, concurrent data structures, or anything with a non-trivial correctness subtlety that a well-tested library would meaningfully de-risk. Go's standard library (`sort`, `slices` package since Go 1.21) covers every primitive these operations would need.

Per this repo's own instruction to prefer stdlib/existing deps over new dependencies (`~/.claude/CLAUDE.md` "Engineering Discipline" + this repo's absence of any grouping/collections library in `go.mod`), reaching for a library here would be a violation of that convention, not an application of it — there is no unmet capability need, only a few dozen lines of straightforward Go.

### Verdict: **Confirmed — hand-rolled Go functions over existing `DocumentStore`/`SearchResult` data, no library.** This is the "lazy read" and it's correct: `go.mod` has no collections/generics-helper library today (no `samber/lo`, no `thoas/go-funk`), and introducing one for four simple operations would be new-dependency overhead with no capability gain.

## Summary Table

| Option | Effort | Risk | Verdict |
|---|---|---|---|
| 1. Extend existing BM25 engine | Low — additive functions over existing data model, no index/storage changes | Low — no migration, existing tests keep covering the core | **Recommended** |
| 2. Migrate to SQLite FTS5 | High — new build tag repo-wide, reindex, cutover, snippet-generation rewrite, dual-write window | Medium-High — silent build-tag footgun, discards 5 test files' coverage, contradicts requirements' own constraint | **Not recommended** |
| 3. Hybrid BM25 + FTS5 | Highest — pays FTS5's full cost plus ongoing dual-pipeline maintenance | High — relevance-ordering disagreement between backends, doubled sync surface | **Not recommended** |
| 4. Hand-rolled Go vs. library for dedup/context/bookend/scroll | Trivial — stdlib slice/map ops | None | **Recommended (hand-rolled, no library)** |

## Bottom line

The existing `session/search` engine's `Document`/`DocumentStore`/`InvertedIndex` model already produces everything the four missing pieces need as inputs (per-session-ordered docs, per-message scores, raw JSONL access via `history.GetMessagesFromConversationFile`). None of the four gaps are algorithmic limitations of BM25-in-memory — they're response-shaping and filtering layers that belong in `session/search/` (new helper functions) and `server/services/search_service.go` (wiring), implemented as plain Go, not a new search backend or a new dependency. The one genuine gap independent of this decision — no `source`/origin field on `ClaudeHistoryEntry` — needs to be solved at the data layer regardless of which engine is chosen, and is not made easier by FTS5.
