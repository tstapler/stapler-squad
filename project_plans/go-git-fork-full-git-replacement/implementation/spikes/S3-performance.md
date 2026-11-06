# S3: go-git v5.19.2 vs git CLI performance (Story 0.2.3, gate G3)

Measured 2026-10-08, macOS arm64 18-core, `/usr/bin/git` (pinned; PATH git is the `~/.local/bin/git` dotfiles wrapper). Harness: `/tmp/s3/main.go` (throwaway module, `github.com/go-git/go-git/v5 v5.19.2`). READ-ONLY operations only; nothing written to any real repo.

## Method
`cd /tmp/s3 && go build -o s3 . && ./s3 <repo> <n> <n-slow> [op-filter|!op-exclude]`. Each op: 1 discarded warm-up, then n timed iterations (n=30 unless noted). CLI = `exec /usr/bin/git -C <repo> ...` (includes spawn). go-git = `PlainOpenWithOptions(EnableDotGitCommonDir)` **per iteration** plus the op (the open is part of the cost, as `OpenRepo` per call would be). p50/p95/min reported. Op filter and p95 index were patched mid-run (n=1 crash); results below are from the final binary.

## Repos
- **Large**: `~/code/github.com/tstapler/stapler-squad` (this repo; the managed repo with the 27 GB `.git`: du 27.9 GB, `size-pack` 15.44 GiB, 8083 tracked files, `status --porcelain` shows 2 lines). Sessions are not stored in a file (`~/.stapler-squad/sessions.json` does not exist), so "largest managed" was found by scanning `.git` sizes under `~/code` and `~/ws`; this one is by far the largest (next: vscode-docs 1.6 GB, shallow).
- **Medium**: `~/ws/tn-nftitus` (`size-pack` 87.8 MiB, 2557 files, 8961 commits, clean). Plan asked for "5 GB or smaller": vscode-docs, kubernetes/website and deno clones were tried and are 1-commit shallow clones, which crash/skew history ops, so a full-history repo was used. Whether tn-nftitus is stapler-squad-managed is not verified.

## Results, large repo (stapler-squad)
Load average during runs: 5.5 to 9 for status (n=5), 21 for the first partial run, 7 for the n=30 run. 

| Op (CLI / go-git) | CLI p50 | CLI p95 | go-git p50 | go-git p95 | G3 |
|---|---|---|---|---|---|
| open only (`rev-parse --git-dir` / PlainOpen) | 18.1 ms | 30.7 ms | 0.48 ms | 0.82 ms | pass |
| `rev-parse HEAD` / `Head()` | 17.4 ms | 19.7 ms | 0.54 ms | 0.89 ms | pass |
| `rev-parse HEAD~5` / `ResolveRevision` | 18.6 ms | 20.3 ms | 8.0 ms | 10.3 ms | pass |
| `merge-base HEAD HEAD~50` / `Commit.MergeBase` | 20.4 ms | 24.0 ms | 14.9 ms | 17.5 ms | pass (thin margin) |
| `rev-list --count HEAD~200..HEAD` / `Log` walk | 13.7 ms | 17.6 ms | 24.2 ms | 49.3 ms | FAIL (also not equivalent: CLI 278 vs go-git 200, see caveats) |
| `diff --shortstat HEAD~1 HEAD` / `Patch.Stats` | 16.6 ms | 17.2 ms | 102.7 ms | 116.9 ms | FAIL (6x) |
| `diff --shortstat HEAD~200 HEAD` / `DiffTree`+`Patch.Stats` | 233.5 ms | 411.7 ms | 852.4 ms | 1205 ms | FAIL (3.6x) |
| `status --porcelain` / `Worktree.Status` (n=5) | 238.2 ms | 420.3 ms | **57,207 ms** | 70,891 ms | **FAIL (240x)** |

Status detail: a second independent CLI sample at a quieter moment gave `status --porcelain` p50 66.7 ms (python timing loop, n=30), so CLI status ranges 67 to 238 ms depending on load; go-git status is 57 s regardless. go-git also returned **896 entries** vs 2 porcelain lines from the CLI: it does not honor the same ignore rules here (`.claude/worktrees`, `node_modules`, global excludes are the suspects; cause NOT investigated). An earlier n=30 attempt had to be killed after >10 min because each go-git status takes about 1 min.

## Results, medium repo (tn-nftitus)
| Op | CLI p50 | CLI p95 | go-git p50 | go-git p95 | G3 |
|---|---|---|---|---|---|
| open only | 10.9 ms | 16.8 ms | 0.054 ms | 0.061 ms | pass |
| `rev-parse HEAD` | 10.5 ms | 12.0 ms | 0.096 ms | 0.108 ms | pass |
| `rev-parse HEAD~5` | 11.1 ms | 11.4 ms | 7.9 ms | 8.3 ms | pass |
| `merge-base HEAD HEAD~50` | 14.1 ms | 15.3 ms | 15.5 ms | 20.5 ms | FAIL (marginal, +10%) |
| `rev-list --count HEAD~200..HEAD` / Log | 15.5 ms | 22.4 ms | 22.0 ms | 24.0 ms | FAIL (CLI 591 vs go-git 200, not equivalent) |
| `diff --shortstat HEAD~1 HEAD` | 63.1 ms | 65.5 ms | 270.5 ms | 421.6 ms | FAIL (4.3x) |
| `diff --shortstat HEAD~200 HEAD` | 133.0 ms | 138.4 ms | 1201 ms | 1553 ms | FAIL (9x) |
| `status --porcelain` / `Worktree.Status` | 57.4 ms | 71.4 ms | 227.1 ms | 269.3 ms | FAIL (4x) |

(kubernetes/website, a 1-commit clone with 13,095 files, corroborates status: CLI 81.3 ms vs go-git 445.4 ms; its history ops are meaningless.)

## PATH-git vs /usr/bin/git (CLI baseline sensitivity, large repo, n=30)
| Op | /usr/bin/git p50 | PATH git (wrapper) p50 |
|---|---|---|
| `rev-parse HEAD` | 10.8 ms | 23.1 ms |
| `status --porcelain` | 66.7 ms | 79.7 ms |
| `merge-base HEAD HEAD~50` | 15.2 ms | 27.5 ms |

The wrapper adds about 12 ms per spawn. With PATH git as the baseline, `merge-base` would pass on both repos (27.5 ms vs 14.9 to 15.5 ms) and `rev-parse` gets a larger win; status, diff and Log verdicts do not change.

## G3 verdict (rule: an op flips to in-process only if go-git p50 <= CLI p50)
- **PASS, flip candidates**: open, `rev-parse HEAD` (about 30x to 100x faster), `ResolveRevision` (rev-parse of a revision expression), and `merge-base` (passes on the large repo, ties/fails by 10% on the medium one against /usr/bin/git; passes against the PATH wrapper). Marginal; keep behind the per-op flag.
- **FAIL, stay CLI** (or need a fork hot-path patch under 200 lines per G3/F4): `status` (the worst: 240x on the large repo and 4x on a 2.5k-file repo; pre-existing plan already routes boolean dirtiness to `worktree_dirty_fast.go`, which this confirms is necessary, and status as an enumerated list should stay CLI), `diff`/`Patch.Stats` (3.6x to 9x slower; line-diff cost in go-git `Patch`), and `Log`-based rev-list count.
- Hot-path patch candidates: `Worktree.Status` (needs ignore-matcher pruning of ignored directories and stat-cache use against the index mtime; currently walks everything) and `Patch.Stats` (diff engine). Neither is plausibly under 200 lines for a 240x gap; status patch is the one worth sizing, since it is the op with the largest absolute pain and correctness divergence (896 vs 2 entries).
- Not measured (plan lists them): worktree add (existing branch), commit (writes), index v3/v4 probes, rev-list count equivalent. Writes need a scratch clone (not done).

## Caveats
- Machine load was 5 to 31 (EDR agent, other agent sessions, a live stapler-squad service); CLI numbers swing 3.5x on status between runs (67 vs 238 ms), so only large ratios are conclusive. Single machine, single day.
- Log comparison is not like for like: go-git walks until it meets `HEAD~200` in its own order, counting 200, while `A..B` excludes everything reachable from A (278/591 on merge-heavy history). A true comparison needs `rev-list --count --first-parent`, or the same traversal semantics.
- go-git status on the large repo used n=5 (30 would take about 30 min); 5 samples spread 56.5 to 70.9 s.
- No Status/Checkout edge-case probes (rename, CRLF, LFS, shallow) and no write benchmarks were run.
