# Research: Features / edge cases for a per-repo shared cache (session/unfinished)

Paths below are relative to `session/unfinished/` unless noted. Line numbers are at branch `perf/scanner-fsnotify-filter` (HEAD 150168a4c + uncommitted edits). VERIFIED = source opened this session; INFERRED = reasoned, not executed. No code was run (machine load).

## 1. What is shared vs per-worktree (the cache-key boundary)

| Item | Scope | Evidence |
|---|---|---|
| Objects (loose + packs), pack indexes, decoded-object LRU | common dir | VERIFIED `gogitstore/open.go:26-40` (`reg.acquire(commonDirAbs, ...)`), `gogit_vcs_reader_objectcache_test.go:55` (2 worktrees -> 1 registry entry) |
| refs/, packed-refs, config, info/, shallow, worktrees/, logs/ | common dir | VERIFIED fork `storage/filesystem/dotgit/repository_filesystem.go:46` (`objectsPath, refsPath, packedRefsPath, configPath, ..., infoPath, ..., shallowPath, worktreesPath` are routed to commondir) |
| HEAD, index, per-worktree refs (refs/bisect, refs/worktree) | per-worktree gitdir `<common>/worktrees/<name>/` | VERIFIED `gogit_vcs_reader.go:1989-2016` (`worktreeGitDir`), used for the index mtime at `:1592`; test `gogit_vcs_reader_limits_test.go:948,980` |
| `.gitignore` files | the working tree (checked-out content) | INFERRED from `getOrBuildUntrackedMatcher` `:997-1033`: `gitignore.ReadPatterns(osfs.New(worktreePath))` |
| `info/exclude` | common dir (shared) | VERIFIED path routing above; but the matcher never reads it (see 3.9) |

Consequence: reachable set, parent memo, merge-base, ahead/behind inputs are pure functions of (commit hash) and are safe to key by (canonical common dir, hash). HEAD tree hashes are a pure function of HEAD commit hash (also safe, key by (common dir, commit hash), not by worktree). The gitignore matcher is NOT purely commit-derived (working tree `.gitignore`, untracked edits) so it must stay per worktree or be keyed by (worktree, headTreeHash) as now.

Today's keys (VERIFIED): `reachableSetCache` by worktree path (`:1392-1410`), `aheadBehindCache` by `path\0base` (`:1247`), `commitMessagesCache` by `path\0base\0max` (`:1314`), `entry.headTreeCache` per `cachedRepo` (`:324-340`), `matcherCache` by worktree path (`:1007-1030`), `repoCache` by worktree path (`:2053`). Only the object store and pack index are already shared (`gogitstore.Registry`).

## 2. Canonical key: two different "common dir" implementations (RISK)

- `gitCommonDir` (`gogit_vcs_reader.go:2021-2047`) does NOT `EvalSymlinks`, reads the gitdir line without trimming relative paths against `repoPath` the way `worktreeGitDir` does (`:2003-2008` handles relative; `gitCommonDir` `:2036` does not), and falls back to `filepath.Dir(wtGitDir)` when `commondir` is missing (which yields `<main>/.git/worktrees`, not the common dir). VERIFIED by reading.
- `gogitstore.resolveGitFilesystems` (`gogitstore/open.go:63-110`) canonicalizes with `filepath.EvalSymlinks` (`canonicalizeDir`, `:112-121`), documented for macOS `/tmp -> /private/tmp`. Registry key = this canonical string (`registry.go:159-175`).
- A new shared per-repo cache MUST reuse the registry's canonical key (export it from `gogitstore`, e.g. via the `WorktreeStorer`), not `gitCommonDir`, or macOS symlinked paths split one repo into two cache entries (functional, but defeats the dedupe and doubles memory). INFERRED consequence; the mechanism is VERIFIED.
- Test coverage of symlink canonicalization: none found in `session/unfinished` tests for the reader-level key (grep for `EvalSymlinks` in `*_test.go` returned nothing in this package; the registry-level comment cites it). GAP.

## 3. Edge cases and failure modes

### 3.1 Linked vs main worktree
- Main worktree: `.git` is a dir, `commonFs == dotFs` (`open.go:73-77`). Linked: `.git` file -> gitdir -> `commondir` (`open.go:79-109`). VERIFIED.
- `ListWorktrees` enumerates `<common>/worktrees/*` and returns a per-worktree HEAD/branch (`:783-852`); it works from either the main or a linked path via `gitCommonDir`. VERIFIED. Tests: `scanner_test.go:17-92` cover `ParseAllWorktrees` (a helper), `gogit_vcs_reader_objectcache_test.go:55` covers the shared open. No test found asserting `ListWorktrees` from a linked path. GAP (INFERRED).
- fsnotify watches only `<repoPath>/.git` (`scanner.go:495-505`). For a linked-worktree repoPath, `.git` is a FILE, so `w.Add` watches that pointer file, not the per-worktree gitdir or the common dir. INFERRED: events for a worktree registered as its own repo are lost, and the debounced scope escalation (`gitEventAllWorktrees`, `scanner.go:594-607`) only helps repos registered at their main path. Existing test `scanner_eventdriven_test.go:74,90` only covers a normal `.git` dir. GAP: needs verification with a linked-worktree repoPath (cheap to test).
- Loose refs are not watched (comment at `scanner.go:588-590`: "loose refs under .git/refs are not watched, so these are the proxy"). A `git commit` on a linked worktree writes `<common>/refs/heads/<branch>` (loose) and the worktree's `HEAD`/`index` under `<common>/worktrees/<name>/`; none of these are inside the watched `.git` dir top level, so linked worktree commits depend on the backstop tick (`scanner.go:249`). INFERRED.

### 3.2 Worktree deletion / pause_session (worktree removed, branch kept)
- `pruneMissingRepos` every `pruneRepoInterval` = 5 min stats each registered repo path and calls `RemoveRepo` (`scanner.go:1089-1115`, test `scanner_eventdriven_test.go:280,308`). VERIFIED.
- `RemoveRepo` -> `forgetRepoWorktrees` -> `ForgetWorktree` for the repo and every indexed worktree (`scanner.go:1057-1075`, `gogit_vcs_reader.go:1453`); `ForgetWorktree` drops the reachable set and invalidation gen, but does NOT delete `repoCache` entry (so the gogitstore ref stays until TTL/prune: `repoCacheTTL` 30 min, `:402-458`), nor `matcherCache`, `aheadBehindCache` ... (the latter by prefix in `evictWorktreeResults`, `:1418`). VERIFIED by reading; `matcherCache` retention after forget INFERRED (not touched in `ForgetWorktree`).
- A deleted worktree leaves a dangling `<common>/worktrees/<name>/` until `git worktree prune`; `ListWorktrees` flags `IsPrunable` when the path is missing (`:843-847`) and `scanRepo` skips prunable (`scanner.go:757-759`). VERIFIED.
- Shared-cache implication: a per-repo cache must be refcounted by live worktrees or purely hash-keyed with budgeted eviction; dropping it when ONE worktree is forgotten would penalize siblings (today `ForgetWorktree` is per-path, which is safe for path keys but unsafe for repo keys). Branch kept means the branch tip hash stays valid; a paused-then-resumed worktree should hit the shared cache cold-free. INFERRED.
- Test: `gogit_vcs_reader_limits_test.go:1059` (RemoveRepo drops reachable sets), `scanner_test.go:373` (persistCache evicts gone worktree). VERIFIED.

### 3.3 Shallow clones
- `cachedReachableSet` doc: "Known limit: a shallow clone deepened in place keeps its truncated set until the base moves or the process restarts" (`:1387-1391`). VERIFIED. Key is base hash only; `shallow` file lives in the common dir (routing above), so a deepen changes the same common dir but not the base hash. A repo-keyed cache needs (base hash, shallow-file mtime/hash) or invalidate on `shallow` change.
- `findMergeBase` on a shallow boundary: parent memo returns `ObjectNotFound` for grafted-away parents? Not verified (INFERRED); there is no shallow test in the package (grep for "shallow" in `session/unfinished` returned nothing). GAP.

### 3.4 gc / repack / prune invalidating cached data
- Commit-derived caches (reachable set, parent memo, merge base) are immutable per hash; a repack/gc never changes ancestry. Safe, VERIFIED by `reachableSetEntry` comment `:208-211` and test `limits_test.go:996` (no `time.Time` field allowed).
- Hazard: `git prune` can delete UNREACHABLE commits (e.g. an old rebased branch tip). A cached entry keyed by a pruned hash is never looked up again (harmless), but a cached set containing such hashes only wastes memory. If a cache stores decoded commit objects, a pruned object is still served from memory (stale but equal). INFERRED.
- Object-store staleness after repack: handled in gogitstore. Non-mmap mode relies on Registry TTL; mmap mode has fsnotify on `objects/pack` with debounce (`gogitstore/mmapwatch.go:43-110`), `RefreshIndexes` (`registry.go:330`). Tests: `mmap_stage2_test.go:183` (RefreshDetectsRepack), `:293` (FsnotifyTriggersRefresh), `:358` (pinned readers survive real repack). VERIFIED (names). Not run.
- Loose-object cache hit guard: `registry_eviction_test.go:765,804`.

### 3.5 Force-push / rebase of base
- Base ref moves to a new hash: ahead/behind entry is validated by `(headHash, baseHash)` (`:1286-1294`), reachable set by base hash (`:1396`). Correct. Old (superseded) sets stay in memory until `ForgetWorktree`: `reachableSetCache` holds exactly one entry per worktree path (asserted `limits_test.go:1046-1050`), so a repo-keyed cache must evict superseded base hashes (LRU by budget) or it grows by up to R entries per force-push. INFERRED.
- Rebased head: `findMergeBase` lockstep walk still correct; `behind` can hit `mergeBaseBFSLimit` (L=2000) and is silently truncated (`AGENTS.md` row countCommitsTo). Existing test: `vcsreader_test.go:368` BehindCount; `gogit_vcs_reader_mergebase_test.go` (decode count). VERIFIED for existence.
- Parity risk: a shared cache must not change truncated results (L cap, R cap) when two worktrees ask with different heads. INFERRED.

### 3.6 Detached HEAD, unborn branch
- Detached: `ListWorktrees` marks `IsDetached`; `scanRepo` skips detached (`scanner.go:757`). So a shared cache will never serve detached worktrees through the scanner. `AheadBehind` itself works on detached HEAD (uses `repo.Head()` hash). VERIFIED. Test: `scanner_test.go:52` (parse only).
- Unborn: `resolveHeadTreeHashes` returns `(nil, nil)` on `ErrReferenceNotFound` (`:894-899`), documented sentinel; `AheadBehind` returns an error on `repo.Head()` failure (`:1266-1268`). Tests: not found specifically for unborn in this package (INFERRED GAP). Shared cache must not cache the error (currently it does not store on error).

### 3.7 Submodules
- Untracked walker skips only the literal name `.git` (`:1957-1960`), so a submodule dir (which contains a `.git` FILE) is walked as a normal directory and its files count as untracked unless ignored; tracked gitlink entries are not walked (index has a Dir/gitlink mode). `resolveHeadTreeHashes` skips only `filemode.Dir` (`:929`), so a gitlink (`filemode.Submodule`) is recorded in `headHashes` as if it were a blob. INFERRED parity bug, not specific to caching; a hash-keyed shared cache must not make this worse. No submodule tests found (grep). GAP.

### 3.8 Sparse checkout
- No handling found (grep for sparse/skip-worktree returned nothing). Skip-worktree index entries absent from the worktree would look "deleted" to the stat phase. INFERRED; out of scope for caching but a watcher-driven dirty tracker (changed-paths-only) must not assume every index entry exists on disk.

### 3.9 Bare repos
- `ListWorktrees` branch `wt.IsBare` is skipped in `scanRepo` (`scanner.go:757`), but `GoGitVCSReader.ListWorktrees` never sets `IsBare` (VERIFIED: no `IsBare` assignment in the reader; the `ParseAllWorktrees` test `scanner_test.go:39` covers the parser only). A bare repo path has no `.git` entry: `gogitstore.Open` stats `.git` and fails (`open.go:60-62`). So a bare repo registered with the scanner yields an open error each tick. INFERRED; low priority.

### 3.10 gitignore / info/exclude / global excludes
- Matcher = `gitignore.ReadPatterns(osfs.New(worktreePath), nil)` only (`:1024`): it does not read `.git/info/exclude`, `core.excludesFile`, or nested negation edge cases beyond go-git's. `info/exclude` is common-dir shared, so a repo-wide matcher would need to merge per-worktree `.gitignore` with shared `info/exclude`. INFERRED existing divergence from native git behavior (parity oracle tests: `walkers_oracle_test.go`).
- Matcher invalidation: HEAD move + changed `.gitignore` blob only (`:957-990`, tests `gogit_vcs_reader_gitignore_invalidation_test.go:71,113,146`). Documented staleness: a `.gitignore` EDIT in the working tree between HEAD moves is stale up to a poll cycle (`:337-345`). A shared matcher must stay per worktree because working-tree edits are per worktree.

### 3.11 fsnotify event loss / overflow
- `w.Errors` is only logged (`scanner.go:564-569`); there is no fallback rescan on overflow (`ErrEventOverflow`) beyond the periodic backstop tick. VERIFIED. A watcher-driven dirty tracker therefore needs: on any error or overflow, mark all repos "needs full walk" (not present today). No test for Errors channel (grep: no match in tests). GAP.
- Debounce window `fsnotifyDebounce = 2s`, fixed (non-sliding) window, widest scope wins (`scanner.go:573-649`); tests `scanner_eventdriven_test.go:381-430`. VERIFIED.
- Only `Write|Create` are handled (`scanner.go:542`); `Rename` (git's lockfile -> final file rename produces Create on the target on most platforms; macOS kqueue/FSEvents may surface Rename) is dropped. INFERRED risk on macOS for `packed-refs` and `HEAD`.
- Index events for linked worktrees are not watched (3.1).

### 3.12 Many worktrees of one repo at once (thundering herd)
- Already collapsed per key by singleflight (`sfDo`, `:763`; groups at `:544-552`) but keyed by worktree path / `path\0base`, so W worktrees = W independent flights computing the same reachable set. Test: `limits_test.go:270` (parallel same-key callers collapse). VERIFIED. A repo-keyed cache needs singleflight keyed by (common dir, base hash) and must not hold `entry.mu` (per-WORKTREE mutex, `:324-326`) as the cache lock: two worktrees have different `entry.mu`, yet `cachedRepo`s share a `SharedObjectStore` whose `ensureIndex`/reads have their own lock (`gogitstore/store.go:187`). Whether go-git object reads through two `*git.Repository` over the same `SharedObjectStore` are concurrency-safe is covered by `gogitstore_test.go:1196` (ConcurrentReadsAcrossWorktrees_NoDataRace). VERIFIED (name; not run).
- `invalidateRepo` iterates `repoWorktrees` and invalidates all worktrees on any `gitEventAllWorktrees` event (`scanner.go:1267-1280`). Combined with `scanRepo` scanning ALL worktrees per event (`:746-787`), a single commit in one worktree triggers W rescans each with W-way duplicated reachability work. Burst of W worktree creations (e.g. backlog dispatch) means W cold opens, W reachable-set builds of the same base: O(W*R) (the AGENTS.md "latent" row). VERIFIED by reading.
- `openRepoEntry` race on first open: duplicates discarded and ref released (`:2085-2100`), test `gogit_vcs_reader_gogitstore_eviction_test.go:188`. VERIFIED.

### 3.13 Cache / invalidation machinery already in place
- `guardInvalidation`/`pathInvalidationGen` (`:1432-1450`) detects invalidate-during-compute and re-evicts; test `limits_test.go:1103`. Key is worktree path; a repo-level generation would be needed if a shared entry can be computed while a ref moves. Hash keying makes this unnecessary for commit-derived data (immutable) but still required for ref-resolved results (`headRef`, `baseHash` resolution).
- `repoCache`: LRU/TTL (30 min) plus memory-budget prune tiers `approxBytesPerCachedRepo` 96 MB, budget 3 GB, pressure thresholds `:225-322`, tests `gogit_vcs_reader_memory_test.go:138-173`, `gogitstore_eviction_test.go`. A shared per-repo cache must plug into `PruneToMemoryBudget`/`UnderSeverePressure` (`:315,459`) and `ClearCache` (`:475`), otherwise it is an unbounded memory sink outside the budget. VERIFIED call sites exist; integration INFERRED.

## 4. Unstated needs (inferred)

1. One canonical repo key, exported from `gogitstore` (EvalSymlinks'd), shared by the object store and the new caches. Do not use `gitCommonDir`.
2. Per-repo cache memory must be accounted under the existing budget (`PruneToMemoryBudget`, pressure tiers) and surfaced in `RepoCacheStats`/metrics (requirements.md Observability). A 50k-hash set is ~ a few MB (comment `:1356-1362`); W worktrees times several bases adds up.
3. Cache entries need a lifetime tied to the repo (refcount of live worktrees or TTL), because `ForgetWorktree` today is per-path and a repo-keyed `ForgetWorktree` would evict siblings' data.
4. A shallow-boundary stamp (the `shallow` file mtime/content hash) in the key or an invalidation hook, to remove the documented "deepened in place" limit.
5. Truncation parity: the R and L caps produce truncated, base-relative results; a shared set computed to depth R from one base must be identical for every worktree. It is, because it depends on base only; but a cached partial result computed under a lowered test cap (`maxReachableSetCommits` is a var, `:1368`) must not leak across tests (use per-reader state, not package globals).
6. Event-loss safety net: on `fsnotify` error/overflow or unwatched worktree gitdirs, fall back to full walk (needed for any watcher-driven dirty tracking per requirements.md rabbit hole).
7. Per-repo feature flag (requirements Risk Control) requires cache access behind an interface so the old path-keyed behavior stays selectable; today caches are plain `sync.Map` fields on `GoGitVCSReader` (`:491-604`).
8. Deterministic tests: package already uses `readHeapInUse` var override, `fsDebounce` field, `pruneRepoInterval` var, `wait.WaitForCondition`; follow these seams (no sleeps).

## 5. Test inventory (existing coverage map)

| Area | Test (file:line) | Status |
|---|---|---|
| Shared object store across worktrees | `gogit_vcs_reader_objectcache_test.go:55` | covered |
| Registry refcount / TTL / budget | `gogitstore/registry_eviction_test.go:54,100,156,248,316,365` | covered |
| Concurrent reads across worktrees | `gogitstore/gogitstore_test.go:1196` | covered |
| Reachable set reuse / cap | `gogit_vcs_reader_limits_test.go:817,996` | covered |
| invalidateRepo keeps reachable set; RemoveRepo drops it | `limits_test.go:1059` | covered |
| Invalidate-during-compute race | `limits_test.go:1103` | covered |
| Singleflight collapse | `limits_test.go:270` | covered |
| AheadBehind TTL reuse when hashes unchanged | `limits_test.go:394` | covered |
| Merge base decode scaling | `gogit_vcs_reader_mergebase_test.go` | covered |
| worktreeGitDir (linked, relative) | `limits_test.go:948,980` | covered |
| Debounce / scope | `scanner_eventdriven_test.go:381-430` | covered |
| Prune missing repos | `scanner_eventdriven_test.go:280,308` | covered |
| Mmap index repack refresh | `gogitstore/mmap_stage2_test.go:183,293,358` | covered |
| Matcher invalidation | `gogit_vcs_reader_gitignore_invalidation_test.go:71,113,146`; `matchercache_test.go:17` | covered |
| Shallow clone, symlinked path key, unborn, submodule, sparse, bare, fsnotify Errors/overflow, linked-worktree `.git` file watch, ListWorktrees from linked path | none found | GAP |

## 6. Comparable existing features (reuse / pattern sources)

- `gogitstore.Registry` + `SharedObjectStore` (`gogitstore/registry.go`, `store.go`): per-commondir sharing with refcount (`acquire`/`release`), TTL (`registryStoreTTL`), budget/pressure-aware `Prune`. The natural home or template for a per-repo derived-data cache; a `SharedObjectStore` already is the per-repo object (extend with a hash-keyed derived-data section so refcount/prune/budget come for free). INFERRED recommendation.
- `sfDo[T]` generic singleflight wrapper (`gogit_vcs_reader.go:763`) and the four `singleflight.Group` fields (`:544-552`).
- TTL caches: `diffStatCacheTTL` 30 s (`:37`), `repoCacheTTL` 30 min (`:407`), `matcherCache` (decoupled, survives `cachedRepo` eviction), `blobCacheLRU` (`:114-175`, content-addressed LRU by bytes: the closest model for a hash-keyed budgeted cache).
- Hash-validated no-TTL cache: `reachableSetEntry` (`:210`) and `aheadBehindEntry` re-validation (`:1286`).
- `guardInvalidation` generation counter (`:1432`).
- Scanner-level: `repoWorktrees` index (`scanner.go:1285`), `circuitBreaker` per repo (`:1319`), `worktreeCache` TTL (`:1234`).
- Repo-wide singleflight use elsewhere: `server/dependencies.go`, `github/http_client.go`, `server/services/hook_context_cache.go` (checked by grep, names only).
- Metrics: `metrics.go` (77 lines), `RepoCacheStats`/`BlobCacheStats`/`GitignoreMatcherStats` (`:623-735`) are the established stat-snapshot pattern for the Observability requirement.
