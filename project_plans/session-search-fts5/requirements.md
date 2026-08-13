# Requirements: session-search-fts5

**Date**: 2026-08-02
**Type**: feature addition (extends existing subsystem — NOT a greenfield build)
**Complexity**: 3 — system design (existing search subsystem needs new modes + a new consumer surface)

## Problem Statement

When triaging a new backlog item, the operator (human or triage agent) has no visibility into whether similar work has been attempted before, what decisions were made, or what failed. The backlog item as filed proposes building cross-session full-text search from scratch using SQLite FTS5, modeled on Hermes Agent's `tools/session_search_tool.py`.

**Codebase reality check (invalidates part of the original proposal):** stapler-squad already has a working, integrated full-text search subsystem over Claude conversation history — it does not need to be built from zero:

- `session/search/` — a hand-rolled inverted index + BM25 scorer (`engine.go`, `inverted_index.go`, `bm25.go`, `document_store.go`, `snippet.go`, `tokenizer.go`), with disk persistence (`index_store.go`) and incremental sync off `session.ClaudeSessionHistory`.
- `SearchClaudeHistory` RPC (`proto/session/v1/session.proto:80`, handler in `server/services/search_service.go:459`) — keyword query → per-message scored hits with generated snippets + highlight ranges.
- `ListClaudeHistory` RPC (`server/services/search_service.go:245`) — already supports project/text filtering, and **already has proper cursor pagination** (`page_token`/`page_size`, `historyCursor` in `search_service.go:29`), which functionally covers the item's "Browse mode" acceptance criterion today.
- Frontend: `useHistoryFullTextSearch.ts` hook, `HistorySearchInput.tsx` / `HistorySearchResults.tsx` components already consume `SearchClaudeHistory`.
- Index updates are already wired to `session/history_watcher.go` via `IncrementalSync(hist)`, called at the top of every `SearchClaudeHistory` request (pull-based incremental sync, not a watcher-push callback, but net effect is "index reflects current JSONL state on next search").

**What's actually missing**, verified by reading `session/search/engine.go` `Search()` (`session/search/engine.go:191`) and `search_service.go:459`:

1. **No session-level dedup.** `Search()` returns one `SearchResult` per matching *message*, not per session. A session with 5 matching messages returns 5 rows. The item's own acceptance criteria explicitly call this out as required behavior it currently lacks.
2. **No ±5 message context window or bookend (first 3 / last 3) messages** around a hit — only a per-message snippet via `SnippetGenerator`.
3. **No Scroll mode** — no anchor-on-message-ID forward/backward navigation without re-running the query.
4. **No `source=tool` / background-automation exclusion** — nothing in `search/` or `search_service.go` filters by session origin; background reviewer / automation sessions are indistinguishable from user sessions in search results today.
5. **No triage-panel integration** — `TriageReviewPanel.tsx` and friends (`web-app/src/components/backlog/Triage*.tsx`) have no "find related past work" search box; the existing search UI lives only in the history page/omnibar surfaces.
6. **BM25 vs. FTS5**: the original proposal assumes SQLite FTS5. The codebase's existing engine is a custom in-memory BM25 index, not SQLite-backed. Whether to (a) extend the existing engine with the missing modes, or (b) migrate to SQLite FTS5 as the ticket proposes, is an open architecture question for Phase 2 research / Phase 3 planning — the existing engine already does ranked full-text search reasonably, and replacing it is a bigger, riskier change than extending it.

## Baseline

Today, a user or triage agent must manually navigate to a session's history page and run `SearchClaudeHistory`/`ListClaudeHistory` by hand (via the existing history UI) to find related past sessions — there is no discovery entry point from the backlog triage flow itself, no session-grouped results (a busy session's 5 hits are 5 separate rows to scan), and no way to review a hit's surrounding conversation without opening the full session transcript.

## Users / Consumers

- The triage agent / triage panel (`TriageReviewPanel.tsx` and the backlog triage flow) — primary new consumer, needs a "search box pre-populated with backlog item title" entry point.
- Human users on the session detail page and omnibar — secondary consumer, extending the existing search UX with scroll/browse modes.
- Internal: `session/search` engine, `SearchService` (backend), `history_watcher.go` (index freshness trigger).

## Success Metrics

**Primary user for design purposes: the human triage operator**, viewing the "Find related past work" box in the triage panel UI. (A fully autonomous triage agent is a secondary consumer of the same `SearchClaudeHistory` RPC — it can call the RPC directly with the same flags; it does not need the UI surface this feature adds. This resolves an ambiguity flagged in Phase 4 triad review: the UI/UX work in Epic 2.2 is scoped to the human path, not the agent path.)

- Triaging a backlog item surfaces related past sessions (if any exist) without the operator leaving the triage panel or hand-crafting a search query.
- A multi-hit session appears exactly once in discovery results (currently: once per matching message) — closes the AC gap directly.
- A user can page through a session's messages around a hit (scroll mode) without re-issuing the FTS query — currently impossible.
- Background/tool-sourced sessions no longer clutter end-user search results.

**Measurable outcome target and post-ship signal** (added post-triad-review — the above are capability checks, not outcome metrics on their own): within the first week after shipping, at least one real triage session results in the operator opening a `SessionHitCard` (click-through, Story 2.2.3) at least once — measured via the `search.sessions_returned`/query-fired span attributes already planned in the Observability Plan, cross-referenced with click-through events (add a lightweight span/log on card activation if not already covered by page-navigation telemetry). Zero click-throughs in the first two weeks of real usage is a signal the feature isn't earning its keep and is worth a follow-up UX check, not a silent miss. (No baseline exists today for how often "prior related work would have helped during triage" occurs, since there is no way to observe that today — this metric is a proxy: it measures usage of the new surface, not the counterfactual value of having found something. A stronger outcome measure — e.g., "operator applies a decision informed by a surfaced session" — would require additional instrumentation out of scope for this feature's v1.)

## Appetite

Medium (1–2 weeks) — most of the plumbing (index, RPC, frontend hook, UI shell) already exists; work is additive (dedup, context window, scroll mode, session-source filter, one new UI surface), not foundational.

## Constraints

- Must reuse the existing `session/search` engine and `SearchClaudeHistory`/`ListClaudeHistory` RPCs rather than introducing a parallel SQLite FTS5 system, unless Phase 2 research concludes the BM25 engine cannot reasonably support scroll/dedup (unlikely — both are query/response-shaping changes, not indexing changes).
- Must not break existing consumers of `SearchClaudeHistory` (`useHistoryFullTextSearch.ts`, `HistorySearchResults.tsx`) — changes to response shape (e.g., session grouping) need to be additive or versioned carefully.
- Per repo convention, any new RPC/UI touchpoint requires a `docs/registry/features/` entry and e2e test (`.claude/rules/feature-registry.md`).

## Non-functional Requirements

- **Performance SLO**: discovery query should return in the same order of magnitude as today's `SearchClaudeHistory` calls (currently in-memory BM25, sub-100ms typical for this repo's history volume — confirm in research, don't regress).
- **Scalability**: not applicable — history volume is single-user/local, same as today.
- **Security classification**: internal (local Claude history, already handled by existing RPCs — no new trust boundary).
- **Data residency**: no special requirements — data never leaves the local machine, same as today.

## Scope

### In Scope

- Session-level deduplication of discovery results (top hit per session, others available as "N more matches in this session").
- ±5 message context window per hit, plus bookend messages (first 3 / last 3 of the session) — new response fields on `SearchClaudeHistoryResponse` or a new RPC, TBD in planning.
- Scroll mode: anchor on a message ID, page forward/backward within a session without re-running the search.
- Exclusion of `source=tool` / background-automation sessions from end-user-facing search/browse results (Browse mode already effectively covered by `ListClaudeHistory` cursor pagination — extend its filter, don't rebuild it). Resolved in planning: no `source=tool`-equivalent field exists anywhere upstream; the filter uses live `Instance.Hidden` instead (ADR-002).
- Scope `SearchClaudeHistory` to the current repo via its existing but currently-ignored `project` request field (ADR/plan discovered this field is defined on the proto but silently unused by the handler today — needed for the triage query, which always scopes to the item's repo).
- Triage panel "Find related past work" search box, pre-populated with the backlog item title, wired to the (extended) discovery RPC.
- Unit tests: session dedup, snippet/context-window extraction, source-exclusion filter.

### Out of Scope

- Migrating the existing BM25 engine to SQLite FTS5 (unless Phase 2 research finds a concrete reason the current engine can't support the above — treat as a rabbit hole, not a default).
- Cross-session search UI in the omnibar (item mentions this as a "surface in two places" nice-to-have; the triage panel is the acceptance-criteria-mandated surface — omnibar/session-detail search-within-session is already partially covered by existing UI and is not blocking).
- Any change to how history JSONL files are parsed or watched (`history.go`, `history_watcher.go` internals) beyond confirming the existing incremental sync is sufficient.

## Rabbit Holes

- **Full FTS5 migration**: large, unnecessary rewrite of a working subsystem if the BM25 engine can be extended instead — explicitly flagged for Phase 2/3 to resolve, not assumed.
- **"Push" reindexing on watcher event**: current design does pull-based incremental sync on every search request, which is already correct/sufficient (index is always fresh at query time). Don't build a separate push-reindex pipeline unless research shows a real staleness problem — it would add a background goroutine + lifecycle management for no measurable benefit.
- **Session grouping response shape**: changing `SearchClaudeHistoryResponse` from "N rows per session" to "1 row per session with nested hits" is an API shape change with existing consumers (`useHistoryFullTextSearch.ts`) — needs a plan that doesn't break them (new field vs. new RPC vs. versioned response).

## Alternatives Considered

- **Build SQLite FTS5 from scratch as the ticket proposes** — rejected as default path; the repo already has an equivalent, integrated, tested search engine. Redoing it in FTS5 duplicates working functionality unless research surfaces a concrete limitation of the BM25 engine (e.g., can't do phrase queries, can't scale — TBD in research).
- **Port Hermes's three-mode design (`tools/session_search_tool.py`) 1:1** — useful as a UX reference for Scroll/Browse mode shapes, but the underlying storage/index layer differs enough that a literal port isn't appropriate; adapt the *modes*, not the *implementation*.

## Feasibility Risks

- Response-shape change to `SearchClaudeHistoryResponse` (session dedup) could break the two existing frontend consumers if not additive — needs explicit compat check in planning.
- `source=tool` / background-session exclusion needs a reliable signal on `ClaudeHistoryEntry` or session metadata to filter on — confirm such a field exists (or needs adding) in research; the item's Notes section assumes it exists (referencing Hermes's `source=tool` tag) but this repo's schema may not have an equivalent field yet.
- "Pre-populate with backlog item title" in the triage panel is a small UI change but touches `TriageReviewPanel.tsx`, which per `.claude/rules/feature-testing-registry.md`-adjacent conventions may need registry/e2e updates.

## Observability Requirements

Standard request logging/tracing sufficient — `SearchClaudeHistory` already emits OTel spans (`telemetry.StartSpan`) for sync and search phases; extend the same spans for new modes rather than adding a separate observability path.

## Risk Control

Not needed — low risk, additive change to an existing, already-shipped subsystem. No feature flag needed; ship behind normal PR review + e2e coverage per repo convention.

## Terminology Note

This document uses "discovery" for the search-and-surface capability throughout (see Success Metrics, Scope, Open Questions below). Planning settled on "related work" as the shipped name (`RelatedWorkQuery`, `TriageRelatedWorkSection` — see `implementation/plan.md`'s Domain Glossary) — same capability, read "discovery" here as "related work" wherever the two diverge.

## Open Questions

1. Does any existing field (on `ClaudeHistoryEntry`, session metadata, or the JSONL entries themselves) already distinguish user-initiated sessions from background/tool/automation sessions? If not, what's the cheapest reliable signal to add? *(for Phase 2 research)* — **Resolved**: no such field exists upstream; `implementation/plan.md` Epic 1.5 / `decisions/ADR-002` use live `Instance.Hidden` as a best-effort signal instead.
2. Should session-grouped discovery be a new response field on `SearchClaudeHistoryResponse`, a new RPC (e.g., `DiscoverSessions`), or a `group_by_session` request flag? *(for Phase 3 planning)*
3. Should Scroll mode be a new RPC or an extension of `GetClaudeHistoryMessages` (if such exists) with an anchor parameter? *(for Phase 2 research — confirm exact existing message-fetch RPC surface)*
