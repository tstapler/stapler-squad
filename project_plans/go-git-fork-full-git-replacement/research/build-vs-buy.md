# Build vs. Buy: replacing the git CLI in stapler-squad

**Date**: 2026-10-08. **Labels**: VERIFIED = source/URL/command output shown here; INFERRED = reasoning, not checked.

## Headline findings

1. **The requirements' premise is partly stale.** The repo already has an in-process linked-worktree lifecycle (add / remove / prune / list) plus a three-way merge, merged in [#730](https://github.com/tstapler/stapler-squad/pull/730) (commit `0084660da`, 2026-09-07); the feature flags were removed in `65edb5c73` (#849). VERIFIED: `session/git/native_worktree_{add,remove,prune,list,common}.go` and `native_admin_writer.go` are ~560 lines (`wc -l`); `native_merge*.go` adds ~2.4k more (6,984 lines total across non-test `native_*.go`). It is wired in at `session/git/worktree_ops.go:176` (`nativeSetupNewWorktreeWithSelfHeal`) and `:461` (`nativeRemoveWorktree`). The CLI `worktree add/remove` that remains is `session/git/remote_worktree.go:86,103` (remote runner) plus `worktree list` in `server/services/path_completion_service.go:217`. So "fork go-git to add linked worktrees" is mostly already done *outside* a fork, as a bespoke layer on go-git v5.19.2.
2. **Upstream go-git has linked-worktree support, but only in v6.** `x/plumbing/worktree` (`Add`, `Remove`, `List`, `Open`, `Init`) exists on `main` and in tag `v6.0.0-beta.1` (2026-10-04), and is absent from v5.19.3. VERIFIED: `gh api repos/go-git/go-git/contents/x/plumbing/worktree/worktree.go?ref=v6.0.0-beta.1` returns 11,610 bytes; the same path at `ref=v5.19.3` is HTTP 404. Source: [worktree.go @ c40b645](https://github.com/go-git/go-git/blob/c40b645093fcd18c696a0cef6a26d7bbdfa5b173/x/plumbing/worktree/worktree.go). It lives under `x/` (experimental, API may change).
3. **No product code uses go-git networking.** VERIFIED: `git grep -E 'transport/(http|ssh)'` over non-test `.go` finds nothing. All fetch/push/clone are CLI. The "ssh-fallback" wrapper is a dotfiles convenience, not product behaviour (see Option 4).

## Cost side (measured)

**Call sites** (`git grep`, counting `"git",` command-construction tokens; helper-indirected calls are not all captured, so these are floors):

| Scope | Sites | Files |
|---|---|---|
| Non-test `.go` | 62 | 29 files, mostly `server/services/*`, `session/git/*`, `session/vcs/*`, `pkg/classifier/*` |
| Test `.go` | 223 (121 under `session/`) | many |
| `exec.Command(...,"git"` direct in tests | 108 | |

Product subcommand histogram (sed over `exec/runner` lines): `rev-parse` 19, `worktree` 3, `status` 3, `fetch` 3, `diff` 3, `remote` 2, `for-each-ref` 2, `config` 2, `clone` 2, one each of `push`, `merge-base`, `log`, `commit`, `symbolic-ref`, `rev-list`, `branch`. (A separate grep of `"push"|"fetch"|"clone"` also found `push -u` at `server/services/unfinished_work_service.go:476`, `session/git/worktree_git.go:194,539`, and fetch at `session/git/ops.go:36,190`.)

**Per-exec cost**, 200 sequential `subprocess.run` calls on this machine (macOS, Apple Git 2.50.1, repo `.git` = 27 GB), mean per call:

| Command | ms/call |
|---|---|
| `/usr/bin/true` (spawn floor, includes Python overhead) | 7.0 |
| `~/.local/bin/git -C repo rev-parse HEAD` (PATH wrapper) | 52.1 |
| `/usr/bin/git -C repo rev-parse HEAD` | 39.1 |
| `/usr/bin/git --no-optional-locks -C repo rev-parse HEAD` | 60.0 (noise: slower than the plain run; single-sample, ignore) |
| `/usr/bin/git status --porcelain` | 181.8 |
| `/usr/bin/git worktree list --porcelain` | 48.8 |

VERIFIED as measurements; single run each, so +/-10 ms noise is plausible. The wrapper adds ~13 ms (~33%) over `/usr/bin/git` for `rev-parse` (INFERRED from one sample). Real `git` costs ~32 ms above the spawn floor, which matches EDR/exec-latency overhead dominating over git's own work. A `rev-parse HEAD` is a few file reads; in-process it is microseconds (INFERRED). Rough scale: 300 execs x ~40-52 ms = ~12-16 s of serial spawn time per package run (INFERRED; parallelism and the fork-lock mutex from #955 change the wall-clock effect).

## What the ssh-fallback wrapper is (VERIFIED)

`~/.local/bin/git` is a symlink to `~/dotfiles/stapler-scripts/git-ssh-fallback` (450-line POSIX sh). It intercepts `clone|fetch|pull|push|ls-remote|submodule|remote` (line 110), looks up the remote URL, probes for a live SSH agent across 1Password/GPG/systemd paths, and sets `GIT_SSH_COMMAND` with `BatchMode`/`ConnectTimeout=10`. No Go code references it except a test comment (`session/git/ops_test.go:192-195`, a hang reproduction). It is a **developer-machine convenience**, not product behaviour: a user without it gets plain `git` and their own credential helper. The requirement "keep ssh-fallback parity" therefore only matters for the maintainer's own box. It also means wrapper cost applies to every git call, including non-network ones (it is invoked first, then `exec`s real git).

## Options

### (1) Fork go-git, add linked worktrees + network parity

- **Pros**: one dependency graph; full control of storage layer; could carry custom fixes (e.g. status perf).
- **Cons**: the worktree half is already solved in-repo (finding 1) and upstream v6 (finding 2), so the fork's marginal value is mainly network parity. go-git's own transport already supports HTTP/SSH, SSH agent and `Auth` objects; it does **not** call `git credential` helpers (INFERRED from go-git's `transport` API; not verified against source). Reproducing keychain/GHE credential-helper parity means implementing the `git credential fill` protocol in-process (or shelling to it, which defeats the goal). A single maintainer rebasing a fork across the v5 -> v6 API break (issue [#910](https://github.com/go-git/go-git/issues/910), "API and Behaviour Changes for next major release", open) is a recurring cost. Upstream is active (release v5.19.3 on 2026-10-04; many open worktree/status perf bugs, e.g. [#181](https://github.com/go-git/go-git/issues/181), [#2441](https://github.com/go-git/go-git/issues/2441), [#1896](https://github.com/go-git/go-git/issues/1896), all open), so a fork would carry divergence against a moving target.
- **Verdict: Not recommended** as a full fork. A *thin* patch carried against upstream for a single concrete bug is fine, but a long-lived "full git replacement" fork has high cost for little marginal gain.

### (2) Contribute linked-worktree support upstream

- **State** (VERIFIED via `gh`): this is already upstream. Issue [#1812](https://github.com/go-git/go-git/issues/1812) "worktree: Review API for opening linked-worktrees" (closed); [#1842](https://github.com/go-git/go-git/issues/1842) PlainOpen with DetectDotGit not following `commondir` (closed); [#1580](https://github.com/go-git/go-git/issues/1580) `repo.Head()` on a worktree fails (closed); [#2324](https://github.com/go-git/go-git/issues/2324) relative worktree extensions, fixed by merged PR [#2336](https://github.com/go-git/go-git/pull/2336); merged PR [#2401](https://github.com/go-git/go-git/pull/2401) bounds worktree names. `x/plumbing/worktree` commit history: `a57d09289` Add Init (2025-12-18), `5ec382632` WithDetachedHead (2025-12-18), `c40b64509` bound name length (2026-09-17, latest). Maintainer stance: [#1560](https://github.com/go-git/go-git/issues/1560) (earlier "implement git worktree" request) was closed as wontfix by `pjbgf` on 2025-06-26 as a misunderstanding of bare-repo support, not a rejection of linked worktrees.
- **Gaps in upstream `x/plumbing/worktree`** (VERIFIED by reading `worktree.go:140-260`): `Remove` deletes only `.git/worktrees/<name>` metadata, not the working directory (the doc comment says so); `Add` checks existence with `Lstat` then `Mkdir` (TOCTOU race, which this repo's `native_worktree_add.go` header calls out and avoids with atomic `os.Mkdir` + numeric-suffix retry). `Add` always checks out via go-git's `Checkout` (inherits go-git checkout performance, [#1956](https://github.com/go-git/go-git/issues/1956) open). No `prune`/lock/move.
- **Cons**: `x/` is experimental, v6 only (beta.1), and this repo is on `go-git/v5` (`go.mod:30`); adopting it means a v6 migration for 30 files that import go-git (VERIFIED count: 30 non-test files matched). Waiting is a poor fit for the "cut scope, not deadline" appetite.
- **Verdict: Viable, as a follow-up not a prerequisite.** Contributing the race fix and a working-dir-removing `Remove` upstream is cheap and reduces the bespoke surface later; do not block on it.

### (3) Another pure-Go implementation or libgit2 bindings

- **git2go** (libgit2): VERIFIED `libgit2/git2go` latest release v34.0.0 (2022-10-06), last push 2024-03-04, i.e. stale; libgit2 itself is active (v1.8.7, 2026-08-13). It requires cgo plus a libgit2 build (static link or system lib). This repo's release builds pin `CGO_ENABLED=0` for all three binaries (`.goreleaser.yaml:14,28,40`) and CI enforces it (`.github/workflows/goreleaser-check.yml:74-101`, a "build + smoke test (CGO_ENABLED=0, matches release build)" job); the Makefile sets `CGO_ENABLED := 1` for local builds (`Makefile:14`), but release is what matters. Adopting git2go would break the single-static-binary, cross-compiled goreleaser matrix (darwin/linux/windows x amd64/arm64) and needs a cgo cross-toolchain. libgit2 also lacks some git features (e.g. no `git worktree` parity guarantee; INFERRED, not verified).
- **Other pure-Go**: `gogs/git-module` (VERIFIED: pushed 2026-09-01) wraps the git CLI, so it does not remove the dependency. I found no other maintained pure-Go full git implementation (INFERRED from a limited search; not exhaustive). Gitoxide is Rust, not applicable.
- **Verdict: Not recommended.** cgo violates the release constraint; the pure-Go alternative is go-git itself.

### (4) Keep the CLI, cut its cost

- Levers and what each buys (INFERRED unless noted):
  - **Call `/usr/bin/git` (or a resolved absolute path) instead of PATH `git`**: saves ~13 ms/call on the maintainer's box (52.1 -> 39.1 ms, VERIFIED single sample). Zero effect on other users, since the wrapper is not installed for them. Risk: bypasses the ssh-fallback for network calls, so apply only to non-network subcommands (the wrapper's own line 110 list shows which ones it touches), or set `GIT_SSH_FALLBACK_ACTIVE=1` (the wrapper's own pass-through switch, line ~13) to skip its logic while still exec'ing real git.
  - **Persistent `git cat-file --batch`**: only helps object reads (`rev-list`/`log`/blob reads). The histogram is dominated by `rev-parse` (19), which is not served by `cat-file --batch` except `--batch-check` for object names; ref resolution still needs a ref read. Narrow benefit.
  - **Caching** (`rev-parse --show-toplevel/--git-dir` results are stable per path): removes many calls cheaply, but cache invalidation across moved/removed worktrees needs care.
  - **Embedded/bundled git binary**: lets a machine with no git work, but needs per-OS/arch binaries in a CGO-free release; larger artifacts and a CVE-update burden. The repo already bundles tmux (`docs/how-to/bundle-tmux.md`), so there is a precedent, but git is far larger and has runtime-helper dependencies (`git-remote-https`, `git-core` exec path). INFERRED.
- **Cons**: does not meet the "zero git spawns" metric or the "no git binary installed" metric by construction (except the bundled variant). Per-call floor stays near the 7 ms spawn cost.
- **Verdict: Viable** as the safety net and for the network operations, **not** as the end state for reads.

### (5) Hybrid: in-process reads and simple writes, CLI for network (and anything go-git cannot match)

- This is effectively where the repo already is, plus continuing the migration. Worktree add/remove/prune/list and three-way merge are already native; test fixtures partly moved in-process (`b5af54fb8`, #955/#956). Remaining product CLI sites (62) split as: **cheap in-process**: `rev-parse` family (19), `symbolic-ref`, `for-each-ref`, `config`, `remote`, `rev-list`, `merge-base` (already native at `native_merge_base.go`), `log`; **risky**: `diff`/`status` (go-git status is slow and mismatches CLI on edge cases: issues [#181](https://github.com/go-git/go-git/issues/181), [#436](https://github.com/go-git/go-git/issues/436), [#1896](https://github.com/go-git/go-git/issues/1896) all open; CLI `status` here takes ~182 ms on a 27 GB-`.git` repo, and go-git is known slower on large trees, INFERRED); **keep CLI**: `fetch`/`push`/`clone` (credential helpers, GHE, SSH agent) behind the existing per-operation switch.
- **Pros**: matches the requirements' own Risk Control (per-operation switch, staged rollout); retires most of the spawn cost because `rev-parse` is the bulk; no fork; stays on stable v5.
- **Cons**: "zero spawns" is not reached while network ops remain; two code paths to keep in parity.
- **Verdict: Recommended**, combined with Option 4's cheap levers for the network/status/diff residue.

## LLM-generated / bespoke vs battle-tested: worktree metadata correctness

- **What the format demands** (INFERRED from git's documented layout, cross-checked with this repo's own code): linked worktree = `<wt>/.git` file `gitdir: <main>/.git/worktrees/<name>`; admin dir holds `HEAD`, `commondir`, `gitdir` (back-pointer), `index`, optional `locked`; allocation uses atomic mkdir with numeric-suffix retry; prune must treat missing back-pointers and `locked` correctly. Mistakes here corrupt state silently (a worktree that `git` itself will later report as prunable/broken) and only show up when a *real* `git` touches the repo.
- **Bespoke risk is real but already bounded in this repo**: the native implementation cites git's own `builtin/worktree.c` at a pinned commit for the allocation loop, avoids the TOCTOU race that upstream's `x/plumbing/worktree.Add` has (`native_worktree_add.go:14-34`), and has fuzz (`native_worktree_add_fuzz_test.go`), differential (`native_merge_differential_test.go`), golden and concurrency tests. VERIFIED those files exist; I did not run them.
- **Where bespoke is weakest**: the three-way merge (`native_merge*.go`, ~2.4k lines incl. diff3) and any bespoke status/diff. Merge semantics (renames, mode changes, conflict markers) are a long tail that git has hardened over 15+ years; differential tests against the real CLI are the right control, and should stay in CI as long as the CLI path exists.
- **Library risk is not zero either**: go-git's worktree/status area carries open correctness bugs (#1896 nested worktrees reported as untracked; #2297 `Status()` writes to disk for uncloned submodules; #2322 regression since v5.19.1). "Battle-tested" for go-git means widely used for clone/commit/log, not for linked-worktree-heavy flows (linked-worktree open/HEAD bugs #1580/#1842 were only closed in 2025-26).
- **Conclusion**: for each remaining operation, prefer (a) go-git's own implementation where the repo already trusts it, (b) the bespoke layer only with differential tests against the real CLI, and (c) the CLI where neither is trustworthy (network, merge edge cases). Do not hand-write new on-disk formats without a differential test against `git` 2.x.

## Recommendation

Adopt Option 5 (hybrid), supported by Option 4's cheap levers (absolute git path / pass-through env for non-network calls, caching of `rev-parse`), keep the per-operation CLI-fallback switch and the spawn counter from the requirements, drop the full-fork plan, and treat upstream contributions (Option 2) as optional hygiene. Re-scope the goal metric from "zero git spawns" to "zero spawns except fetch/push/clone and any diff/status path proven slower in-process".

## Open items I could not settle

- go-git source for credential-helper support (INFERRED absent; not checked in `plumbing/transport`).
- Actual count of git execs per `session` test run (the requirements say "hundreds"; I did not instrument a PATH shim).
- Whether go-git status/diff are slower than CLI on the repos this tool manages (needs a benchmark; CLI `status` here is 182 ms).
