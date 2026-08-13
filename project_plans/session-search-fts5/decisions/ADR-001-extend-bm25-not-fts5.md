# ADR-001: Extend the Existing BM25 Search Engine, Do Not Migrate to SQLite FTS5

**Date**: 2026-08-02
**Status**: Accepted
**Context**: session-search-fts5 feature — the originating backlog item proposed building cross-session search from scratch on SQLite FTS5, modeled on Hermes Agent's `tools/session_search_tool.py`.

---

## Context

The backlog item that seeded this project assumed the codebase had no existing full-text search and proposed building one on SQLite FTS5. Reading `session/search/` (`engine.go`, `document_store.go`, `bm25.go`, `inverted_index.go`, `index_store.go`) shows this assumption is false: stapler-squad already has a working, integrated, hand-rolled BM25 engine wired to `SearchClaudeHistory` and `session/history_watcher.go`. The actual gap is four missing *response-shaping* behaviors (session dedup, ±5 context window + bookends, scroll-mode paging, automation-session exclusion), not a missing search capability.

The question this ADR resolves: given those four gaps, should they be built by extending the existing BM25 engine, or does closing them justify migrating to SQLite FTS5 as originally proposed?

## Evidence

- `session/search/document_store.go:30` (`DocumentStore.SessionIndex map[string][]int32`) and `engine.go:47` (`SearchResult.SessionID`) already carry every field needed for session dedup — grouping is a post-processing pass over an already-sorted slice, no index change.
- `session/history.go:558` (`GetMessagesFromConversationFile`) already provides raw, gap-free JSONL access needed for context windows/bookends — no new storage primitive required.
- `proto/session/v1/session.proto:924` (`GetClaudeHistoryMessagesRequest`) already has `offset`/`limit`/`tail`; scroll mode is `offset` arithmetic around an already-known `MessageIndex`, not a new indexing capability.
- `go.mod:24` requires `github.com/mattn/go-sqlite3 v1.14.40`, but FTS5 is not compiled in — it requires the `sqlite_fts5` cgo build tag, confirmed absent from every build/test/CI invocation in the repo (`grep -rn "sqlite_fts5\|fts5" .` returns nothing but this project's own docs).
- Standing up FTS5 would require: (a) adding `-tags sqlite_fts5` to every `go build`/`go test`/CI entry point with a silent "no such module: fts5" runtime failure if any one is missed; (b) a parallel, hand-written SQLite schema/migration path outside ent's generated schema (ent doesn't generate FTS5 virtual tables); (c) a full reindex + cutover from `IncrementalSync`'s working pull-based sync; (d) rewriting `snippet.go`'s highlighting to FTS5's `snippet()`/`highlight()` SQL functions; (e) discarding the working coverage of 5 existing test files (`bm25_test.go`, `engine_test.go`, `inverted_index_test.go`, `index_store_test.go`, `snippet_test.go`) with no FTS5-side replacement.
- None of the four in-scope gaps require FTS5-specific capability (phrase queries, `NEAR()`) — the existing tokenizer + inverted index already performs BM25 ranking (`bm25.go`).

## Decision

Extend `session/search/` and `server/services/search_service.go` with plain Go functions (`groupResultsBySession`, `contextWindowAndBookends`, anchor-index arithmetic, `isAutomationSession`) operating on data the engine and `GetMessagesFromConversationFile` already produce. No SQLite FTS5 migration, no `sqlite_fts5` build tag, no new indexing/storage layer.

## Consequences

- Zero migration/dual-write risk; `IncrementalSync`'s existing pull-on-request model is untouched.
- No CI/build-tag surface change; no cgo footgun introduced.
- Existing test coverage of the BM25 core (`bm25_test.go` et al.) continues to apply unchanged; new tests only need to cover the new grouping/slicing/filtering functions (see `implementation/plan.md` Phase 1).
- If a future requirement genuinely needs FTS5-only capability (e.g., phrase/`NEAR()` queries at a scale the in-memory index can't handle), that would be a new decision made against a concrete requirement, not inherited from this one — this ADR does not foreclose FTS5 forever, it just establishes that none of this project's four gaps are that requirement.

## Alternatives Considered

- **Migrate fully to SQLite FTS5, as the originating backlog item proposed** — rejected. High effort (build-tag plumbing, reindex, cutover, snippet-generation rewrite), medium-high risk (silent build-tag footgun, discarded test coverage), and directly contradicts `requirements.md`'s own constraint ("must reuse the existing engine... unless research concludes the BM25 engine cannot reasonably support scroll/dedup" — research found no such limitation).
- **Hybrid: BM25 for existing search, FTS5 for the four new modes** — rejected. The four new modes are response-shaping/filtering operations over a *single* ranked-match input; splitting that input across two engines risks the two backends disagreeing about which session's hit is "the top one" (a correctness bug), doubles the index-freshness/sync surface for zero functional gain, and none of the four gaps need FTS5-specific capability in the first place.

Full evidence trail: `project_plans/session-search-fts5/research/build-vs-buy.md`, `project_plans/session-search-fts5/research/stack.md` §1–2.
