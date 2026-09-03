# Architecture Research: vcs-tab-redesign

## (a) Current architecture — two independent data paths

```
                         ┌─────────────────────────────────────────┐
                         │  PATH 1: Local git status (pull, sync)  │
                         └─────────────────────────────────────────┘
web-app useSessionVcs.ts ──RPC──▶ WorkspaceService.GetVCSStatus
  (SessionVcsContext.tsx)          server/services/workspace_service.go:132
                                          │
                                          │ 15s in-process TTL cache,
                                          │ keyed by workDir
                                          │ (workspace_service.go:56-60,152-160)
                                          ▼
                                  vc.NewGitProvider(workDir) / NewJujutsuProvider
                                          │
                                          ▼
                                  GitProvider.GetStatus()   session/vc/git_provider.go:66
                                    - branch (30s atomic cache)
                                    - HEAD short SHA, last commit subject
                                    - ahead/behind vs @{upstream}   (git CLI, NOT go-git)
                                    - staged/unstaged/untracked/conflict files (porcelain v2)
                                          │
                                          ▼
                                  vcsStatusToProto()   workspace_service.go:414
                                          │
                                          ▼
                                  proto VCSStatus  (proto/session/v1/types.proto:949)


                         ┌─────────────────────────────────────────┐
                         │  PATH 2: GitHub PR poller (push, async)  │
                         └─────────────────────────────────────────┘
session/pr_status_poller.go  PRStatusPoller.pollLoop()  (session/pr_status_poller.go:184)
  ticker @ PollInterval = 60s (DefaultPRStatusPollerConfig, pr_status_poller.go:39-47)
  fires immediately on Start(), then every tick; concurrency-limited (default 5) via
  a semaphore, one goroutine per monitored *Instance (checkAllSessions, :204)
        │
        │ github.GetPRInfoConditional (ETag If-None-Match; 304 costs 0 rate-limit
        │ quota and returns the CACHED *PRInfo, so any new field added to PRInfo
        │ survives across 304 ticks — see etag_cache.go:78-83)
        ▼
  applyPRUpdate(inst, prInfo)   pr_status_poller.go:396
        │
        ▼
  Instance.UpdatePRStatus(...)   session/instance_terminal.go:389
    - writes inst.GitHubPRState/Priority/IsDraft/ApprovedCount/ChangesReqCount/
      CheckConclusion/PRStatusTerminal/LastPRStatusCheck under inst.mu, via the
      actor (sendSyncErr), then rebuilds+stores an atomic snapshot
        │
        ▼
  adapters.InstanceToProto(instance, ...)  →  Session proto (githubPrState,
  githubCheckConclusion, githubApprovedCount, githubChangesReqCount, ...)


                         ┌─────────────────────────────────────────┐
                         │  WHERE THE TWO PATHS MERGE (frontend)    │
                         └─────────────────────────────────────────┘
web-app/src/lib/vcs/adapters.ts:83  fromSessionVcs(status: VCSStatus, session?: Session)
  - status  → local git fields (branch, isClean, fileChanges, aheadOfMain/behindMain,
              commits: [] hardcoded)
  - session → fromSessionGithub(session) (adapters.ts:68) reads the Session proto's
              githubPrState/githubCheckConclusion/githubApprovedCount/... fields
  - the two are stapled together into one VcsWidgetData with NO shared "as of"
    timestamp and NO reconciliation — they can be arbitrarily stale relative to
    each other (see (c) below)
        │
        ▼
  VcsWidget.tsx (mode="full" for the session tab; mode="compact" for backlog
  ship-status) — VcsWidget.tsx:106 only renders data.aggregateStats in compact
  mode; full mode never does. commits is always [] from fromSessionVcs, so
  VcsWidgetCommitList (:118) renders empty in the session tab today.
```

**Key finding — the two paths are NOT synchronized.** `GetVCSStatus` is a pull,
answered per-RPC (with a 15s cache) directly from the live git working tree.
The GitHub fields on `Session` are push-updated by a background poller on its
own 60s cadence and simply read off whatever `Session` snapshot the RPC layer
already has in memory. There is no code path that fetches both "atomically" —
a `GetVCSStatus` response can reflect a git tree that is seconds old while the
`Session`'s GitHub fields are up to a full poll interval (60s, or longer with the
5-minute `NoPRBackoff` before PR discovery) stale, or vice versa.

## (b) Integration seam per new data point

| # | Data point | Join this path | Why |
|---|---|---|---|
| 1 | Commit list (branch vs. base) | **Path 1** (`GetVCSStatus` → `GitProvider.GetStatus`) | It's a property of the local git tree, computable without any GitHub API call. Add a call to `session/git.ListShippedCommits(repoRoot, baseSHA, headSHA)` (session/git/ops.go:369) inside (or alongside) `GetStatus()`, gated behind resolving a base ref (see open question below on what "base" means for a live, unmerged branch — likely `@{upstream}` or the configured main branch, not a PR base, since a live session may not have a PR yet). Do NOT reuse the GitHub poller — commit history has nothing to do with PR state and the poller's 60s/backoff cadence would needlessly stale it. |
| 2 | Aggregate diff-stat line | **Path 1** | `FileStatsBetween`/diff-stat logic already lives in `session/git/ops.go` next to `ListShippedCommits`, operating on the same two SHAs. Compute once per `GetVCSStatus` call and add `AggregateStats{FilesChanged, Additions, Deletions}` to the `VCSStatus` proto — mirrors what `VcsWidget.tsx:106-111` already expects for `aggregateStats`, just gated on `mode === "full"` too (a one-line frontend fix, see (d) below is not needed — this is a proto+adapter change, not a mode-gating bug requiring extra investigation). |
| 3 | Itemized CI checks | **Path 2** (poller) | The data (`ghStatusCheckItem` list, `resp.StatusCheckRollup`) is already fetched by `GetPRInfo`/`GetPRInfoConditional` inside every non-304 poll tick (github/client.go:113,126-132) and then thrown away by `getCheckConclusion()` (client.go:357) after deriving a single string. No new GitHub API call is needed — this is a "stop discarding data already in hand" change: add a `Checks []CheckItem` field to `PRInfo`, populate it in `GetPRInfo`, thread it through `applyPRUpdate`→`Instance.UpdatePRStatus`→`Session` proto (or a new nested message) exactly like `GithubCheckConclusion` today. |
| 4 | "Why blocked" rollup (all reasons) | **Frontend, derived** — but depends on Path 2 fields (3) and (5) plus Path 1's `HasConflicts`/`aheadBy`/`behindBy` | This is not a new backend fetch; it's a new frontend derivation (`web-app/src/lib/vcs/mergeability.ts`'s `deriveMergeabilityState`, which today collapses everything to ONE state) that needs to become "list every blocking reason" instead of "pick one." It becomes possible once (3) itemized checks and (5) review body exist to enumerate against, plus existing `HasConflicts`. No new backend RPC field is strictly required beyond what (3)/(5) already add. |
| 5 | Reviewer's changes-requested body text | **Path 2** (poller) | Same story as (3): `ghReviewItem.Body` (client.go:117-123) is already fetched in the same `gh pr view --json reviews,...` response and discarded by `parseReviewCounts()` (client.go:330) after tallying counts. Add `Reviews []ReviewItem` (author, state, body) to `PRInfo`, thread through the same poller path. |
| 6 | Live "as of" staleness timestamp | **Both paths, separately** | Because the two paths are independently timestamped (see (a)), the honest UI needs TWO staleness values, not one: (i) local-git freshness = when `GetVCSStatus`'s cache entry was populated (`vcsStatusCacheEntry.cachedAt`, workspace_service.go:51-54 — already tracked server-side, just not surfaced in the proto response) and (ii) GitHub freshness = `Instance.LastPRStatusCheck` (already set by `UpdatePRStatus`, instance_terminal.go:406, and already exposed as... check: not currently on the `Session` proto — needs adding). Surfacing a single merged "as of" would be misleading; recommend exposing both timestamps and letting the frontend show the older/more relevant one, or two small labels. |
| 7 | Ahead/behind-vs-base | **Path 1, but reconsider the implementation** | `GitProvider.GetStatus()` already computes ahead/behind (session/vc/git_provider.go:87-100) via a git CLI subprocess (`git rev-list --left-right --count`) against `@{upstream}`, and that value already reaches the proto (`status.AheadBy`/`BehindBy`) and the frontend (`fromSessionVcs` maps them to `aheadOfMain`/`behindMain`, adapters.ts:89-90) — **this is already wired end-to-end**, contrary to requirements.md's framing that it's only wired to the unfinished-worktree scanner. What requirements.md's `AheadBehind` (session/unfinished/gogit_vcs_reader.go:1140) offers ADDITIONALLY is: (i) a go-git-native implementation with an in-memory cache + singleflight de-dup instead of a subprocess per call, and (ii) ahead/behind against an arbitrary `base` ref, not just `@{upstream}`. If the redesign wants ahead/behind against the PR's actual base branch (not just tracking upstream), that requires either the existing git-CLI path with a different ref argument, or reaching for `GoGitVCSReader.AheadBehind` — see concurrency caveat in (c) below before choosing the latter. |

## (c) Concurrency / locking considerations

Two very different concurrency models exist in this codebase for git repo access, and the redesign must pick consciously:

1. **`session/git/ops.go`'s `ListShippedCommits`/`CommitInfo`/`FileStatsBetween`** — each call does `git.PlainOpenWithOptions(repoPath, ...)` fresh, with **no caching and no explicit mutex**. go-git's `*git.Repository`/`Storer` types are documented (see `code-go-git` skill) as not safe for concurrent use in general, but because each call here opens its own throwaway `*Repository` object with no shared state across calls, there is no actual data race — the cost is instead **redundant work**: every VCS-tab render for every open session re-parses the repo's packfile indexes from scratch, once per SHA-pair lookup. For N sessions on the same physical repo (a shared upstream with multiple worktrees) open concurrently, this means N independent index-parses hitting the same `.git/objects/pack/*.idx` files with no cache reuse. This is the "no lock, but no sharing" model — safe, but slow at scale, and exactly what `session/unfinished/gogitstore` (item 2 below) exists to fix.

2. **`session/unfinished/gogit_vcs_reader.go`'s `GoGitVCSReader`** (used only by the unfinished-worktree scanner today, `session/unfinished/scanner.go:716-734`) — has a **fully-built concurrency-safe caching layer**: a per-worktree `cachedRepo` entry (`entry.mu sync.Mutex`, held via `defer` around every read, gogit_vcs_reader.go:1155-1156) serializing access to that one repo object, plus TTL result caches (`aheadBehindCache`, `commitMessagesCache`, etc.) and `singleflight.Group` (`sfDo(&g.aheadBehindSF, cacheKey, ...)`, :1149) so N concurrent callers asking for the same `(worktreePath, base)` pair collapse into one actual git walk. Additionally, `session/unfinished/gogitstore` (a separate, more advanced prototype not yet wired into `GoGitVCSReader`) shares the parsed packfile index and decoded-object cache *across worktrees of the same commondir* under one `SharedObjectStore.mu`, specifically because upstream go-git's `idxfile.MemoryIndex` and `dotgit.DotGit` mutate internal caches even on reads (go-git issue #1121, documented in gogitstore/store.go:70-82) and are therefore unsafe to share without a mutex, even for read-only access.

**Recommendation:** if the VCS tab is opened for many concurrent sessions against a small number of distinct physical repos (the common case — worktrees of one upstream), reusing `session/git/ops.go`'s current "open fresh, no cache" functions for the *new* commit-list/diff-stat calls will multiply subprocess-free-but-still-expensive index parses linearly with open tabs. The safer/faster seam is to either (i) add the same TTL-cache + singleflight pattern `GoGitVCSReader` already has to `session/git/ops.go`'s new call sites (keyed by `repoPath`, with a `sync.Mutex`-guarded `cachedRepo`-equivalent), or (ii) route commit-list/diff-stat/ahead-behind through `session/unfinished`'s existing `GoGitVCSReader` instead of introducing parallel logic in `session/git`. Option (ii) reuses code that has already been hardened (its test suite includes `TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers`, gogit_vcs_reader_limits_test.go:268) but currently lives in a package literally named `unfinished` and is scoped to the backlog scanner's use cases (`HasUncommitted`, `DiffShortstat`, `AheadBehind`, `CommitMessages`) — it does not yet have a `ListShippedCommits`-equivalent returning full commit metadata (SHA/summary/author/date) the way `session/git/ops.go`'s does. Either promoting/renaming `session/unfinished`'s reader out of that package, or backporting its locking pattern into `session/git/ops.go`, is a real design decision for the planning phase — not resolved by this research pass.

No existing per-repo-path mutex/registry was found in `session/git/` itself outside `GoGitVCSReader`'s own package-local `cachedRepo` map — confirmed by grep across `session/git/` and `session/unfinished/` for `sync.Mutex`/`repoLock`/`perRepoMutex` (only hits were inside `gogit_vcs_reader.go` and `gogitstore/store.go`).

## (d) Answers to Open Questions #2 and #3

**#2 — Can itemized CI checks / review body text piggyback on the existing poll cycle, or do they need a new on-demand fetch path?**

They can piggyback — **VERIFIED, no new fetch needed**. `GetPRInfo` (github/client.go, the `gh pr view --json ...` non-conditional path used for full/uncached PR fetches) and `GetPRInfoConditional` (etag_cache.go:53, used by the poller's steady-state ticks) already retrieve `resp.Reviews []ghReviewItem` (with `.Body`) and `resp.StatusCheckRollup []ghStatusCheckItem` in the same HTTP response used today — see the `ghPRResponse` struct (client.go:88-114) which already unmarshals both fields. `getCheckConclusion()` (client.go:357) and `parseReviewCounts()` (client.go:330) are pure post-processing functions that collapse those already-fetched slices down to a conclusion string and two counts, discarding the rest. Adding `Checks []CheckItem` / `Reviews []ReviewItem` fields to the `PRInfo` struct and populating them alongside the existing derived fields requires zero additional GitHub API calls, zero changes to the poller's cadence, and zero changes to the ETag conditional-request logic — a 304 response already returns the *cached* `*PRInfo` (etag_cache.go:78-83), so the itemized fields would be preserved across 304 ticks automatically, same as every other `PRInfo` field today.

**#3 — What is the poller's actual polling interval, and what triggers a poll?**

`PollInterval: 60 * time.Second` — `DefaultPRStatusPollerConfig()`, session/pr_status_poller.go:39-47 (VERIFIED, literal default; a caller can override via `PRStatusPollerConfig` but no override was found wired into `server/` in this repo pass — worth a follow-up grep before assuming 60s is the live production value everywhere). Triggers, per `pollLoop()` (pr_status_poller.go:184-201):
  - An **immediate check on `Start()`** (`p.checkAllSessions()` runs once before entering the ticker loop, :190-191) so sessions don't show stale/empty status on server startup.
  - A **`time.Ticker` firing every `PollInterval`** thereafter (60s steady-state), unconditionally checking all monitored sessions (subject to global rate-limit and per-session backoff gates below) — not event-driven, not triggered by e.g. a webhook or user action opening the VCS tab.
  - Per-session skips inside `checkAllSessions()` (pr_status_poller.go:230-259): sessions with no GitHub owner/repo, sessions already in a terminal PR state (`GitHubPRStatusTerminal`), fork sessions (Phase-2 deferred), and sessions still inside their `NoPRBackoff` window (default 5 minutes, `DefaultPRStatusPollerConfig`) are all skipped that tick.
  - A global `github.DefaultRateLimiter.IsLimited()` check (:205) can skip an entire tick for all sessions if GitHub API rate limiting is in effect.

This means a "staleness timestamp" for the GitHub-derived half of the VCS tab (open question #6) should be understood as "up to ~60s stale in the common case, but potentially much staler (minutes) for a session still in `NoPRBackoff`, or indefinitely stale if rate-limited" — not a fixed, tight bound.
