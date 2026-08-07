# Research: Technology Stack — session-search-fts5

**Scope**: confirm whether the existing `session/search/` BM25 engine can absorb session-dedup,
±5 context windows, bookend messages, scroll/anchor navigation, and `source=tool` filtering
without a SQLite FTS5 migration, and identify any persistence/dependency changes needed.

## 1. Does the existing engine's data model support the new modes?

Read in full: `session/search/engine.go`, `inverted_index.go`, `bm25.go`, `document_store.go`,
`snippet.go`.

**Verdict: yes — all five requested behaviors are query/response-shaping changes on top of data
that already exists in `Document`/`DocumentStore`/`SearchResult`. Nothing in the index or
posting-list data model fights against it.**

- **Session-level dedup** — `DocumentStore.SessionIndex map[string][]int32`
  (`document_store.go:30`) already groups every doc ID by `SessionID`, and `SearchResult.SessionID`
  (`engine.go:47`) is already on every hit. `Search()` (`engine.go:191-259`) returns one
  `SearchResult` per matching message with no session grouping — dedup is a post-processing step
  in `SearchClaudeHistory` (group `scoredDocs`/`results` by `SessionID`, keep the highest-scoring
  hit per session, expose the rest as a count) — no index or `Document` schema change needed.
- **±5 message context window / bookends (first 3 / last 3)** — `DocumentStore.GetBySession(sessionID)`
  (`document_store.go:85`) already returns every `Document` for a session, and each `Document`
  carries `MessageIndex` (`document_store.go:13`). A context window is: take the hit's
  `MessageIndex`, slice `GetBySession(sessionID)` (or re-read via
  `history.GetMessagesFromConversationFile`) to `[idx-5, idx+5]`. Bookends are the same slice
  operation against `[0:3]` and `[len-3:]`. No new index structures — this is pure read-path
  logic, most naturally implemented against the *history JSONL* (`GetMessagesFromConversationFile`,
  `session/history.go:558`) rather than the search index, since the index only contains
  tokenizable (non-empty) messages and would silently gap out empty/whitespace-only turns that a
  human reading raw context would still expect to see. Confirm this choice in Phase 3 planning.
- **Scroll/anchor mode** — no index change; it is direct use of the already-existing
  `GetMessagesFromConversationFile(sessionID, limit)` (`session/history.go:558`,
  `server/services/search_service.go` around `GetClaudeHistoryMessages`) with a `MessageIndex`
  anchor instead of head/tail. `PostingsList` (`inverted_index.go:9`) is irrelevant to this mode —
  it doesn't touch the inverted index at all, only history-file reads. This is the strongest signal
  that Scroll mode is a `GetClaudeHistoryMessages`-adjacent RPC extension, not a search-engine
  change (resolves Open Question 3 in `requirements.md`).
- **`source=tool` exclusion** — the *engine* has no opinion on session provenance today (`Document`,
  `ClaudeHistoryEntry` — see §4 — carry no such field), so this is a filter applied at the
  `SearchService`/`ListClaudeHistory` layer using whatever signal Phase 2's other research
  streams settle on (see Open Question 1), not something `bm25.go`/`inverted_index.go` need to
  know about. One weak existing precedent: `findConversationFilePath`
  (`session/history.go:336-351`) already excludes files whose basename contains `agent-` when
  walking `~/.claude/projects/` for JSONL files — that's a sub-agent/sidechain transcript
  exclusion convention already in the codebase, though it's a different mechanism (file-walk skip)
  than a session-level `source` field and doesn't by itself cover "background/automation session"
  in the sense this ticket means. `rawClaudeTurn.IsSidechain` (`session/claude_adapter.go:45`) is
  the only other adjacent signal found, but it lives in the *live-session* Claude adapter import
  path, not in the history/search read path (`historyJSONLEntry`, `conversationMessage` in
  `session/history.go:80-99` carry no `IsSidechain`/`source`/`userType` field). **Confirms Open
  Question 1 in requirements.md is still open** — no existing field on `ClaudeHistoryEntry` or the
  history JSONL parse path distinguishes user vs. background/tool sessions today; Phase 2's other
  research stream (or Phase 3 planning) needs to pick a signal (e.g., a project-path pattern, a new
  field added at index time, or reusing `IsSidechain`/`agent-` naming if it can be threaded through).

**Nothing here requires changing `PostingsList`, `InvertedIndex`, or `BM25Scorer`.** Those three
types only need to keep doing what they do (score documents, return `ScoredDocument{DocID, Score}`)
— every new mode is implemented by *what the service layer does with* `DocumentStore` lookups and
`ClaudeSessionHistory` reads before/after calling `Search()`, confirming the requirements.md
constraint ("query/response-shaping changes, not indexing changes") against actual code, not just
assertion.

## 2. Is SQLite FTS5 available today via `mattn/go-sqlite3` as a fallback?

`go.mod:24` already requires `github.com/mattn/go-sqlite3 v1.14.40`, and it's imported (driver-only,
`_ "github.com/mattn/go-sqlite3"`) in four places: `server/analytics/db.go:13`,
`server/services/database_service.go:17`, `server/services/capacity_monitor.go:14`,
`session/agy_adapter.go:15`, `session/ent_repository.go:26`. All four uses are plain relational
storage (ent ORM backend, analytics tables, capacity counters) — **none of them enable or use
FTS5.**

**FTS5 is NOT currently available/enabled.** `mattn/go-sqlite3` compiles SQLite via cgo and does
**not** enable the FTS5 virtual-table module by default — it requires the cgo build tag
`sqlite_fts5` (`go build -tags sqlite_fts5 ...` / `go test -tags sqlite_fts5 ...`), which
recompiles the vendored SQLite amalgamation with `-DSQLITE_ENABLE_FTS5`. Checked every build
invocation in the repo:

- `Makefile` — no `sqlite_fts5` tag anywhere (`grep -n "sqlite_fts5\|-tags" Makefile` only turns up
  `embed_tmux`, `integration`, `harness` tags — see build/test targets at `Makefile:257-269,
  498-525`).
- `.github/workflows/*.yml` — same, no `sqlite_fts5` tag in CI (`build.yml`, `mcp-integration.yml`
  only pass `-tags integration` / `-tags=integration`).
- No `//go:build sqlite_fts5` constraint files anywhere in the tree.

So if Phase 2/3 concluded FTS5 were actually needed, it would require: (a) adding
`-tags sqlite_fts5` to every `go build`/`go test`/CI invocation that needs it (a global build-flag
change, not additive), and (b) confirming the cgo toolchain in CI/dev images can rebuild the
amalgamation with that flag (untested here). Given §1 shows the BM25 engine already supports every
requested mode without new indexing primitives, **this repo does not need to exercise that path** —
consistent with `requirements.md`'s constraint to avoid the FTS5 migration rabbit hole. Flagging the
concrete tag-plumbing cost here only so it's documented if a future need arises.

## 3. Does `index_store.go`'s persistence format need schema changes?

No. `IndexStore` (`session/search/index_store.go`) gob-encodes `*InvertedIndex` and `*DocumentStore`
wholesale (`saveGob`/`loadGob`, lines 65-92, 96-123) plus a small JSON version/sync-metadata
sidecar (`IndexVersion` at line 21, `IndexSyncMetadata` in `sync_types.go`). Since §1 established
that session-dedup, context windows, and scroll mode are all read-path logic over fields
(`Document.SessionID`, `Document.MessageIndex`) that already exist and are already persisted,
**no field needs to be added to `Document`, `InvertedIndex`, or `PostingsList`, so
`CurrentIndexVersion` (`index_store.go:31`, currently `1`) does not need to bump.**

The one field that *would* force a version bump is if `source=tool` filtering (§1, Open Question 1)
is resolved by adding a new field to `Document` itself (e.g., `Document.Source string`) rather than
filtering via `ClaudeHistoryEntry`/session metadata at the `SearchService` layer. That's a
Phase 3 planning decision, not a foregone conclusion — filtering via `ClaudeHistoryEntry` metadata
(joined via `SessionID`, no index change) is the lower-risk option and should be preferred unless
Phase 2's other research explicitly favors indexing the source per-document (e.g., to support
`source:tool` as a query-time filter term rather than a response post-filter).

## 4. Existing pagination cursor pattern — reuse for new modes?

Yes, there's a directly reusable pattern already in `server/services/search_service.go`:

```go
// historyCursor is the opaque token encoded into page_token / next_page_token.
type historyCursor struct {
    UpdatedAtNs int64  `json:"u"`
    ID          string `json:"i"`
}
func encodeHistoryCursor(c historyCursor) string   // json.Marshal → base64.RawURLEncoding
func decodeHistoryCursor(token string) (historyCursor, bool)
```

(`search_service.go:29-56`, consumed by `ListClaudeHistory` at `search_service.go:245-330`.) It's a
small struct capturing "position of last row returned," JSON-marshaled, then base64url-encoded into
an opaque `page_token`/`next_page_token` string pair — exactly the shape needed for:

- **Scroll mode's anchor**: a `scrollCursor{SessionID string; MessageIndex int}` (or similar)
  encoded/decoded the same way, so "resume scrolling from message N in session S" is a drop-in
  reuse of the existing `encode*/decode*Cursor` helper pattern rather than a new pagination
  mechanism. `ListClaudeHistory`'s cursor-recovery behavior — if the cursor can't be found (e.g.
  index changed), fall back to page 1 rather than erroring (`search_service.go:293-296`, "If
  startIdx == -1 the cursor wasn't found ... return from the beginning") — is also a pattern worth
  copying for scroll-mode robustness against a session being re-indexed mid-scroll.
- **Session-grouped discovery pagination**: same struct shape, keyed on session identity + score
  instead of `UpdatedAt`+`ID`, if session-level results need their own page token distinct from the
  underlying per-message `SearchResults.Offset`/`Limit` (`engine.go:23-30`).

No new pagination library or cursor-encoding scheme is needed — Phase 3 planning should specify the
new cursor struct(s)' fields, not a new mechanism.

## 5. New dependencies/versions needed

**None expected.** Confirmed:

- `github.com/mattn/go-sqlite3 v1.14.40` — already present (`go.mod:24`), no version bump needed;
  not used for FTS5 (see §2), so no build-tag or version changes required for the in-scope work.
- No new Go module is needed for cursor encoding (stdlib `encoding/json` + `encoding/base64`,
  already used by `historyCursor`), context-window slicing (plain slice ops over
  `[]*Document`/`[]ClaudeConversationMessage`), or session dedup (plain map grouping over
  `[]SearchResult`).
- Frontend: no new npm package expected — `useHistoryFullTextSearch.ts` /
  `HistorySearchResults.tsx` / `TriageReviewPanel.tsx` are existing React components; the new
  triage-panel search box and any scroll-mode UI are additive props/hooks work, not new
  dependencies. (Not independently re-verified against `package.json` in this pass — flag for
  Phase 3 planning if a virtualized-list library is deemed necessary for scroll mode's UI, but
  nothing in the backend research suggests one is required.)

## Summary of implications for Phase 3 planning

1. Extend `session/search` engine's *response shaping*, not its index — `Document`,
   `InvertedIndex`, `PostingsList`, `BM25Scorer` all stay as-is.
2. No FTS5 migration; no new `sqlite_fts5` build tag; no cgo/CI changes.
3. No `IndexStore`/gob schema version bump needed unless `source` filtering is deliberately chosen
   to live on `Document` rather than on session metadata (recommend it not — filter at
   `SearchService` layer using `ClaudeHistoryEntry`).
4. Reuse the `historyCursor` encode/decode-with-graceful-fallback pattern
   (`search_service.go:29-56`) for scroll-mode anchors and any session-grouped pagination cursor —
   don't invent a new cursor mechanism.
5. No new Go module dependencies; frontend dependency need (if any) is a Phase 3/UI question, not
   a backend stack question.
