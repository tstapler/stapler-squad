# session/unfinished — agent guidance

The background scanner that decides which worktrees have unfinished work. It is the largest single
CPU and allocation consumer of the `stapler-squad` process (39% of a 30-minute CPU profile, 2026-10-15),
so every change here is a performance change.

## go-git is the path forward — never native git

- **Never use or suggest shelling out to `git`** (`exec`, `safeexec`, `rev-list`, `diff --shortstat`,
  `merge-base`, ...) as a fix, fallback, or "faster alternative" in this package. Do not propose it in
  reviews, plans, or PerfFix write-ups either.
- If go-git is slow, **make go-git better**. We run our own fork (`tstapler/go-git`, branch `ssq/v5`,
  consumed by the `replace` directive in the root `go.mod`; see `docs/how-to/roll-back-go-git-fork.md`
  and `project_plans/go-git-fork-full-git-replacement/decisions/ADR-002*`). Extending the fork is
  explicitly preferred over working around upstream limits in this package.
- Order of preference for a hotspot: (1) a better algorithm in this package, (2) cache or avoid the call,
  (3) a fork patch that adds the missing primitive. Native git is not on the list.
- `gogit_vcs_reader_shellout_bench_test.go` is a historical go-git-vs-native comparison. It is not a
  reason to switch, and no new native-git comparisons should be added.

## Extending the fork

Fork checkout: `~/code/github.com/tstapler/go-git` (`origin` = fork, `upstream` = go-git/go-git; never PR
upstream from it). Workflow: patch on `ssq/v5` with a test, wait for `ssq-ci`, tag `vX.Y.Z-ssq.N`, bump
the `replace` line here, run `make ci`. Keep patches small and additive (new functions rather than changed
behaviour) so upstream rebases stay cheap. Import paths never change.

## Algorithm analysis (update when you change a hot path)

Notation: n = tracked files, f = non-ignored files walked, W = worktrees, L = `mergeBaseBFSLimit` (2000),
R = `maxReachableSetCommits` (50,000), d = commits between a tip and the merge base, a/b = ahead/behind.
Costs are per cache miss; a miss happens when the `diffStatCacheTTL` entry expires or a `.git`/worktree
event invalidates it, and the scanner multiplies it by W.

| Operation | Cost | Notes and improvement levers |
|---|---|---|
| `findMergeBase` | O(d1 + d2) commit decodes, capped at L per side | Was O(min(L, history)) before the lockstep walk: ~2000 decodes per call. Measured on a 2500-commit history: 114,780 → 657 allocs/op, 7.8 MB → 54 KB/op. |
| `countCommitsTo` ×2 | O(a) + O(b), capped at L | `b` can reach L when base moves far ahead. `parentMemo` removes re-decodes, not the walk. Lever: cache `behind` per (base hash, merge base). |
| Commit decode (go-git) | one inflate + full message/signature parse per commit | Allocs profile: `bytes.growSlice` 30.8%, `MemoryObject.Write` 10.8%, `bufio.ReadBytes` 10.3%, `continuationCont` 9.2%. Only parents are needed here. **Fork lever:** add a header-only parents reader that stops at the first blank line and skips the message buffer and gpgsig parsing. The fork already ships `plumbing/format/commitgraph`; whether these repos have a commit-graph file is UNVERIFIED. |
| `HasUncommitted` | O(n) index/HEAD compares + O(n) `lstat` + O(f) untracked walk | Cheapest exits first (staged, deleted, size/mtime). Lever: use watcher events to stat only changed paths, O(changed) instead of O(n). |
| `diffShortstatUncached` | per modified file, LCS DP O(m·k) time, guarded by `maxLCSLines` | Lever: Myers diff, O((m+k)·D), in the fork or here. |
| `CommitMessages` / `reachableSet` | O(R) commit decodes per (worktree, base hash) | Cache key is the worktree path, so W worktrees of one repo recompute the identical set when base moves: O(W·R). Lever: key by (common git dir, base hash). Not hot in the 2026-10-15 profile; latent. |
| Scanner total | Σ over W worktrees of the costs above × event rate | The biggest multiplier is the event rate. Filtering and debouncing `.git` events (`watcher.go`) cuts the miss count for every row at once. |

## Rules for changes here

- Profile before and after. Quote `-benchmem` numbers, using `b.Run` variants against the old
  implementation, and keep a regression test that fails on the old behaviour (see
  `gogit_vcs_reader_mergebase_test.go`: decode count scales with distance, not history depth).
- Hold `entry.mu` only around go-git object access, never across stat walks or cache stores (see
  `hasUncommittedGoGitPhase`).
- Singleflight and TTL-cache expensive results (`sfDo`, `*Cache` fields); a cache hit must stay
  allocation-free.
- Do not add `time.Sleep`-based tests; follow the `deterministic-fast-tests` skill.
