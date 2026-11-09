# Research: Session Artifacts integration

Scope (per owner): "Artifacts" = the existing Artifacts tab in the stapler-squad web UI, fed by `session/artifacts/`. NOT the Claude Artifact tool (see "Claude Artifact tool" below). Confidence labels: VERIFIED = read in source this session; INFERRED = reasoned, not run. No code was executed; nothing here is runtime-verified.

## 1. How it works today (VERIFIED)

| Stage | What | Where |
|---|---|---|
| Trigger | `HistoryLinker` file callback on any changed `~/.claude/projects/**/*.jsonl` plus a startup walk (backfill, paced by `artifactBackfillDelay` = 30s) | `session/artifacts/store.go` (`OnHistoryFileChanged`, `walkAndEnqueue`); wiring `server/dependencies.go:1851-1895` |
| Skipped files | non-`.jsonl`, and any file named `agent-*` (subagent transcripts) | `store.go` `OnHistoryFileChanged`, `walkAndEnqueue` |
| Session match | `FindInstanceByHistoryPath`: linear scan comparing `inst.HistoryFilePath == path`; unlinked file => scan silently skipped | `session/artifact_lookup.go:10`, `scan.go` (`lookupTitle` !ok => return) |
| Extraction | Incremental byte-offset scan. `user` entries: only `tool_result` text -> regexes. `assistant` entries: only Bash `tool_use` -> `ExtractFromBashCommand` | `scan.go` `addLine`; `extractor.go` |
| Extracted | `PRURLs` (`https://github.com/o/r/pull/N`, github.com only), `CommitSHAs` (only `commit <40hex>`), `ExternalURLs` (any http(s), cap 50), `Commands` (types `gh_pr_create`, `gh_pr_merge`, `git_commit`, `package_install`; cap 30) | `extractor.go`, `types.go` |
| Storage | One JSON TEXT column `session_artifacts` on the ent `session` row; merge-on-write per title under a mutex; `ScanOffsetBytes` lives in the same blob; per-file scan positions separately in `scanstate` ("artifacts" consumer) | `session/ent/schema/session.go:156`, `session/ent_repository.go:1170-1245`, `store.go` `mergeAndPersist` |
| Load | `Storage.LoadInstances` bulk-loads all blobs into `inst.Artifacts` | `session/storage.go:488-500` |
| Live push | `OnScanComplete` -> `inst.SetArtifacts` + `SessionUpdatedEvent(["artifacts"])`; also seeds legacy PR number (below) | `server/dependencies.go:1867-1893` |
| Proto | `Session.artifacts = 70`; `SessionArtifacts{pr_urls, commit_shas, external_urls, last_scanned_at}`. `Commands` is NOT in proto | `proto/session/v1/types.proto:279,415-425` |
| Adapter | `snap.Artifacts` -> proto (`PrUrls`, `CommitShas`, `ExternalUrls`, `LastScannedAt`) | `server/adapters/instance_adapter.go:152-159` |
| UI | Tab id `artifacts` in `SessionDetailView.tsx:643,1723`; component `web-app/src/components/sessions/ArtifactsTab.tsx` (120 lines); data comes from `session.artifacts` on the Session message, no dedicated RPC | |
| Other consumer | Debug bundle exports the whole blob (incl. Commands) | `server/services/session_bundle_service.go:405-420` |

There is no artifacts-specific RPC; the tab only reads whatever the session list/watch stream carries.

## 2. Why the tab "sometimes works" (diagnosis)

Hypotheses ranked by how directly the code supports them. All VERIFIED as code behavior; whether each fires for the owner's sessions is INFERRED (needs a live check, see "How to confirm").

1. **Empty scans are never persisted, so the "no artifacts found" state is unreachable and the tab shows "Extraction pending" forever.** `scan.go` `if found.empty() { ae.setPosition(...); return }` returns before `storeFn` and `OnScanComplete`. For a session with no PR/SHA/URL/command matches, `inst.Artifacts` stays nil -> adapter omits the field -> `ArtifactsTab.tsx:22-28` renders "Extraction pending — will populate automatically once the session starts". The second empty state (`ArtifactsTab.tsx:39-45`, "scanned, nothing found") can only occur if a blob exists with all-empty lists, which the scanner never writes. Strongest explanation for "sometimes".
2. **Unlinked sessions are skipped silently.** `scanFile` needs `HistoryFilePath` set by `HistoryLinker`; before the link (or for non-Claude programs such as aider/other agents, which have no Claude JSONL) `lookupTitle` fails and the scan returns without advancing. Nothing re-enqueues the file when the link is made later (only file-change events or a restart walk do). INFERRED: a quiet session linked after its last write stays empty until restart.
3. **Subagent transcripts are excluded.** `agent-*` files are skipped (`store.go` `OnHistoryFileChanged`, `walkAndEnqueue`). PRs/commits produced inside Task/Agent subagents appear only if the parent transcript echoes them in a tool_result. INFERRED partial coverage.
4. **Narrow patterns.** SHAs only match `commit <40hex>` (git log style); a normal `git commit` prints `[branch abc1234]`, which is not matched, so commits come only from the `git_commit` Command (message only, no SHA) and Commands are not exposed in proto/UI. `gh pr create ... --title "..."` regex requires double quotes and `--title` after create; heredoc/`--body-file` styles miss. PR regex is github.com only (no GHE host).
5. **Caps keep the oldest, drop the newest.** `cap50` and `mergedCmds[:maxCommands]` truncate the tail (newest), so a long session stops showing recent URLs/commands (`store.go` `mergeAndPersist`, `cap50`).
6. **Startup lag.** Backfill is delayed 30s and throttled (`dependencies.go:47`); right after a restart the tab can show pending for all sessions without a live event.
7. **Stale in-memory cache vs. DB.** `inst.Artifacts` is set on load and on scan completion only; if `readFn`/`storeFn` error (logged `[ArtifactExtractor] persist failed`) the position is not advanced and the scan retries on next change only.
8. **Noisy PR list.** Any PR URL in any tool_result is recorded (e.g. `gh pr list`, `gh pr view` of other PRs, web fetch output, CI logs), so the tab can show PRs the session did not author. This is a correctness problem more than a "works sometimes" one.

### How to confirm (not yet run)
- Log grep in `~/.stapler-squad/logs/staplersquad.log` for `[ArtifactExtractor]` warnings (`seek failed`, `persist failed`, `scanner error`, `fingerprint failed`).
- `sqlite3` the DB: count sessions where `session_artifacts = ''` vs non-empty, and compare to sessions with a non-empty `history_file_path`/Claude program. If most "pending" sessions have a linked history file and a nonempty transcript, hypothesis 1 is confirmed.
- Unit test: scan a fixture JSONL with no matches and assert `inst.Artifacts != nil` (currently fails by design).

## 3. Is `PRURLs` a usable source of PR history?

Short answer: a decent *seed/backfill hint*, not a source of truth.

Strengths
- Captures PRs actually created via `gh pr create` in the session transcript, with URL (host, owner, repo, number all present in the URL) and merge signal via `gh_pr_merge` Command.
- Survives branch deletion and does not cost GitHub rate limit.
- Stored for all sessions with a Claude transcript, including pre-feature history (backfill walk).

Weaknesses (VERIFIED in code)
- Unordered semantics: `dedup(append(existing, new))` keeps discovery order but there is no timestamp per URL, no role (created/merged/mentioned/viewed). JSONL lines carry timestamps; the scanner discards them.
- False positives (hypothesis 8) and no association to author/branch.
- github.com only; GHE URLs never match the regex (conflicts with host-aware identity requirement in `requirements.md` Rabbit Holes).
- Gaps: PRs created by subagents, via MCP/API tools, by Jules, or via non-Claude programs are absent.
- Only written when something is found; a lost/rotated transcript loses history.

Existing coupling to the single-PR bug (VERIFIED): `server/dependencies.go:1876-1889`, `OnScanComplete` takes the FIRST parseable URL in `blob.PRURLs` when `!inst.HasGitHubPR()` and writes only `PRNumber` (`storage.UpdateInstancePRNumber`, `session/storage.go:798`), dropping owner/repo/host. It never updates when a later PR appears, and "first discovered" can be a mentioned, not authored, PR. This is one concrete input to the "wrong PR tracked" problem.

Backfill recommendation for a `SessionPR` relation
- Seed from `PRURLs` only as `source=artifact, role=unknown/inferred` candidates, key `(host, owner, repo, number)` from `ParseGitHubURL` (existing helper). Prefer entries that also match a `gh_pr_create` Command in the same blob, or whose head branch matches the session branch (verify once via GitHub, ETag-cached) to reduce false positives.
- Run as an idempotent one-time migration plus incremental upsert from `OnScanComplete`; never delete relation rows because a URL disappears from the blob.
- Treat the legacy `GitHubPRURL/Number` as the authoritative "primary" row; artifacts add secondary rows.

## 4. Gaps: what else would help users, especially across multiple PRs

Priority order (value vs. cost):
1. **Per-PR timeline events with timestamps**: created, merged (`gh pr merge N`), closed, plus the transcript timestamp and the Bash command that did it. Enables "PR #12 (merged) -> PR #15 (open)" ordering without GitHub calls.
2. **Branches**: `git checkout -b`, `git push -u origin <branch>`, `gh pr create --head/--base`. Links each PR to a branch and exposes stacked base relationships (`--base` = previous PR's head).
3. **Commit SHAs + subjects** from git commit output (`[branch sha] subject`) and `git push` ranges; link a commit to the PR whose branch contains it.
4. **Files changed** per PR/session (Write/Edit tool_use `file_path` is already in the transcript but ignored).
5. **Issues** (`#123`, `gh issue ...`, `Fixes #N` in PR body) and **plan docs** (Write/Edit to `project_plans/**`, `docs/**`, `*.md`): helps navigate a session that spans several PRs.
6. **Deploy / CI links**: Actions run URLs (`/actions/runs/N`), preview/deploy URLs; classify instead of dumping into one 50-URL bucket.
7. **Classify `ExternalURLs`** (docs vs. CI vs. issue vs. deploy) and stop capping oldest-first.

## 5. Ways to deepen the integration

Capture timing
- Persist an empty-but-scanned blob (or a `last_scanned_at` marker) so "pending" vs "none" is distinguishable (fixes hypothesis 1).
- Re-enqueue the transcript on `SetHistoryInfo` (link established) and on session start; include `agent-*` subagent files mapped to their parent session (derive parent from path/UUID).
- Hook tool-result events from live sessions (e.g. the existing `report_pr_created`-style MCP tool path) in addition to transcript scanning, so a PR is recorded at creation time, not at next file event.

Dedup / identity
- Replace `[]string` with structured records `{host, owner, repo, number, url, firstSeenAt, source(created|merged|mentioned), commandRef}`; key on `(host, owner, repo, number)`; make the regex host-aware (reuse the GHE host logic from `github/`).
- Keep newest, not oldest, when capping; or drop caps for typed PR/commit records and cap only noise.
- Distinguish authored vs mentioned: authored = appears in `gh pr create` tool_result (pair `tool_use_id` with the Bash command, which `artifactContentBlock` already carries) or matches session branch.

Link artifacts to a specific PR
- Pair `tool_use` (command) with its `tool_result` via `tool_use_id` (field exists in `jsonl.go` but is unused) so `gh pr create` -> resulting URL is one record; attach branch/commits/files seen between that create and the next create (temporal window) to that PR.
- Store the association as columns on `SessionPR` (or an `artifacts` JSON field per PR) rather than in the session-wide blob.

Storage
- Keep the blob for scan state and noise; promote high-value typed items (PRs, commits, branches, issues) to ent tables if cross-session query is required (see search below). Do not commit generated ent output.

UI surfacing
- Group by PR in the Artifacts tab (PR header with state chip, then its commits/files/links); an "unassigned" group for the rest. Session card: PR count badge from the relation.
- Expose `Commands` and timestamps in proto (new fields are additive; check mobile client).
- Distinguish tab states: not linked / scanning / none found / has data, with a "rescan" action (RPC) for the "sometimes works" complaint.
- Mobile: collapse sections, 44px targets (existing `ArtifactsTab.css.ts` has no responsive rules; INFERRED, not read in full).

## 6. Search across artifacts to find sessions

Existing index (VERIFIED from `project_plans/session-search-fts5/decisions/ADR-001-extend-bm25-not-fts5.md`): a hand-rolled in-memory BM25 engine in `session/search/` (`engine.go`, `bm25.go`, `inverted_index.go`, `document_store.go`, `index_store.go`), indexing Claude history messages and exposed via `SearchClaudeHistory`/`search_service.go`; FTS5 was explicitly rejected (no `sqlite_fts5` build tag). Artifacts are NOT in that index; `session_artifacts` is an opaque JSON string, so today the only way to find a session by PR/commit URL is substring luck inside message text (PR URLs do appear in tool_result messages, so BM25 over history might incidentally hit them, INFERRED, untested).

Options
- A. **Index artifacts as extra documents in the existing BM25 engine**: one document per artifact (type=pr|commit|branch|issue|url, text = URL + number + owner/repo + title/subject, `SessionID` = session). `DocumentStore.SessionIndex` already groups by session, so a query like `owner/repo#42`, `abc1234`, or a branch name returns sessions. Cheapest; stays consistent with ADR-001. Feed from `OnScanComplete` (incremental) and a one-time backfill from stored blobs. Needs a tokenizer rule so `owner/repo#42` and short SHAs match (INFERRED; check `bm25.go` tokenizer).
- B. **Relational lookup for exact keys**: a `SessionPR` table indexed on `(host, owner, repo, number)` answers "which sessions touched this PR" exactly and cheaply (also fixes the dedupe need); use for the omnibar "paste a PR URL" detector (`web-app` Omnibar `detector.ts`, per CLAUDE.md DetectorRegistry) and an RPC `FindSessionsByArtifact`. Pair with A for fuzzy/text search.
- C. SQL `LIKE` over `session_artifacts` blob: works for a handful of sessions, no index, brittle JSON; use only as a stopgap.
Recommendation: B for PR/commit exact lookup (feeds the multi-PR feature anyway) + A for free-text across artifact kinds, surfaced in the existing session search UI with a facet "matched artifact".

## 7. Risks
- Behavior drift: changing "empty blob persisted" alters UI states and the `nil` checks in `instance_adapter.go:152` and `session_bundle_service.go:412` (bundle currently reports "omitted" for none).
- Backfill cost: re-scanning all transcripts (10 MB token cap, `maxScannerTokenSize`) after a regex/schema change; use the existing paced backfill queue and bump a scan-version in `scanstate` so positions reset deliberately.
- Concurrency: `inst.SetArtifacts` and the PR poller both mutate instance state; use the actor setters / Snapshot rules (`.claude/rules/instance-lock-free-reads.md`). `Instance.Artifacts` is a raw pointer shared into snapshots (`instance_snapshot.go:172,265`): treat blob as immutable after publish.
- False-positive seeding creating phantom PRs in the new relation and then being polled (cost, wrong "pr_pending"); mitigate with `role=inferred` and verification before polling.
- Archived sessions: any new automatic path that re-enqueues/rescans must not revive sessions (`noarchivedrevival`); scanning itself is read-only, so fine.
- Privacy/size: URLs and commands (first 200 chars) can contain tokens in query strings; the blob is also exported by the debug bundle. Consider redacting before widening capture.
- Proto/mobile: adding fields is additive, but changing `pr_urls` semantics would break older clients; add new typed fields, keep `pr_urls` populated.
- Feature registry: any new RPC/component needs `make registry-generate` and a `// +feature:` marker (ArtifactsTab already has `session-artifacts`).

## 8. Claude Artifact tool (published HTML pages)
No hook found. In the greps run (`PrUrls|SessionArtifacts|NewArtifactExtractor|...` over Go/proto/TS excluding gen/docs/plans) nothing references the Claude `Artifact` tool or `claude.ai/artifact` URLs; the extractor only sees `Bash` tool_use and generic tool_result text, so a published artifact URL would at most land in `ExternalURLs` as an unclassified link (INFERRED: only if the tool_result text contains the URL). A deliberate hook would be: detect `tool_use` name `Artifact` and `claude.ai/(code/)?artifact/` URLs in tool_result, store as typed `deploy/doc` artifacts. Not requested; flagged as an optional extension. (Not exhaustively searched beyond those greps.)

## Open questions for planning
- Persist empty-scan marker vs add an RPC returning scan status.
- Authored-vs-mentioned rule for seeding `SessionPR`.
- Whether to add a `SessionPR`-linked artifact table now or keep per-PR attachments in JSON initially.
