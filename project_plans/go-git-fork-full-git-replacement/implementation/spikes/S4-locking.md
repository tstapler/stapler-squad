# S4: CLI-compatible locking spike (Story 0.2.4, gate G4)

Run date 2026-10-08. Throwaway module `/tmp/s4` (`go.mod`: `github.com/go-git/go-git/v5 v5.19.2`); key sources copied to `spikes/S4-src/` (`go.mod.txt` is the module file). Environment: darwin/arm64, go1.26.6, `/usr/bin/git` 2.50.1 (Apple Git-155). The tests call `/usr/bin/git` directly because `~/.local/bin/git` is the ssh-fallback wrapper and adds about 0.5 to 2 s per call. All commands: `cd /tmp/s4 && GOFLAGS=-mod=mod GOPROXY=off go test -count=1 -run '<name>' -v ./...`.

Scope: the Phase 0 go/no-go core (Revision 7 note) plus the read-your-writes matrix, census, abort and ref probes. Not run: see "Not verified".

## Gate G4 verdict

**PASS on the go/no-go core. The lock layer lives in-repo (Story 2.3.1); no fork patch F1; the fork stays an empty F0 as far as S4 is concerned.** Every hook needed is public API: `filesystem.NewStorage`/`PlainOpen` returns a `*filesystem.Storage`; embedding it in a wrapper that overrides `Index`, `SetIndex`, `SetReference`, `CheckAndSetReference` and `Reference`, then calling `git.Open(wrapper, worktreeFs)`, makes every `Worktree` method go through the wrapper (the census below shows all index access is `w.r.Storer.Index/SetIndex`). `localwrite` is not dropped. Caveats that narrow, but do not change, the verdict are in "Findings that change the plan".

## Design built (`S4-src/lock.go`, 250 lines)

`WithIndexLock(repoPath, opts, fn)`: open the repo, take `<gitdir>/index.lock` with `O_EXCL` before `fn`, run `fn` against a scoped storer. `SetIndex` rewrites the held lock file (never `index`). After the first `SetIndex`, `Index()` decodes the lock file, so later reads see the scope's writes. Ref writes (`SetReference`/`CheckAndSetReference`) take `<ref>.lock` `O_EXCL` on first write and are buffered, reads served from the buffer. On success: set `Wrote()` first, fsync, rename index lock over `index`, then rename ref locks (branches first, HEAD last). On error or panic: remove only locks this scope created (a list), re-panic. A pre-existing lock yields `*ErrLocked{Path, Age}`; it is never deleted or retried by the scope. Read-only scopes (no `SetIndex`) remove the lock and leave `index` byte-identical.

## Results

| # | Criterion | Test | Result (VERIFIED, test run) |
|---|---|---|---|
| T1 | Stock lost update reproduced | `TestT1_StockLostUpdate` (20 iterations, 3,000-entry repo, CLI `git add a.txt` vs stock go-git `Add("b.txt")`) | **6 of 20 iterations lost an entry** (277 s). Reproduced. |
| T2 | Scoped: no lost update | `TestT2_ScopedNoLostUpdate` (200 iterations, CLI add vs `WithIndexLock` add, retry on lock) | 600 of 600 entries (200 base + 400), no `*.lock`, `git fsck` clean. **Weak as a contention test**: 0 collisions occurred (CLI process start dominates the timing), so T13 and T6 supply the contention evidence. |
| T3a | CLI reader never sees a partial index | `TestT3a_PartialRead_Scoped`: reader loops `git status --porcelain=v1`, `git ls-files -s`, `git add` while 200 scoped writes run on a 5,000-entry index | 220 reader invocations, **0 anomalies** (no corrupt/short/non-zero exits other than lock contention), 5,200 entries at the end, no locks left. |
| T3b | Stock in-place truncation (reproduction) | `TestT3b_PartialRead_StockInPlace`, same loop with stock `Worktree.Add` | 416 invocations, **23 anomalies**: `git status`, `git ls-files -s` and `git add` each died with `signal: segmentation fault` on the half-written index. Apple Git 2.50.1 crashes rather than printing `index file corrupt`. Reproduced, and worse than the plan expected. |
| T13 | Lock excludes the CLI | `TestT13_CLIBlockedWhileScopeHolds` | CLI `git add` inside a held scope exits 128: `fatal: Unable to create '.../.git/index.lock': File exists.`; after release it succeeds; both entries present. |
| T4 | Error and panic do not leak | `TestT4_ErrorAndPanicNoLeak` | Error after `SetIndex`, panic after `SetIndex`, error in `BeforeCommitPhase`, and a read-only scope: no `*.lock`, `index` bytes identical to before in all four. Panic propagates. |
| T5 | Foreign lock never auto-deleted | `TestT5_ForeignLockNeverDeleted` | Fresh (age 0) and 10-minute-old foreign `index.lock`: `ErrLocked{Path, Age}`, `fn` not run, file content untouched and still present. |
| T6 | Deadlock | `TestT6_NoDeadlock` with `-race` | 24 goroutines taking an outer mutex (stand-in for `repoWorktreeLock`) then `WithIndexLock`: finished, 24 entries; then 24 goroutines contending `index.lock` directly with retry on `ErrLocked`: finished, 148 of 148 entries, no locks. No deadlock (scope never blocks; it fails fast, so lock-order deadlock cannot occur by construction). Total 99.9 s under `-race` on this slow machine; not a performance number. |
| T8 | Stale-read hazard is real | `TestT8_StaleReadHazard`: same scope but `Index()` always reads the on-disk index | `Commit(All)` tree `c5a73202...` vs CLI `713c3bff...`: **differs**. A lock without read-your-writes silently commits the wrong tree and returns success. |
| T9a | Abort leaves ref/index/HEAD consistent | `TestT9_RefsAbortAndContention` (a): `Commit(All)`, injected error before the commit phase | `refs/heads/master`, `index`, `HEAD` byte-identical to pre-state, no locks, `Wrote()==false`. |
| T9b | Foreign ref lock | (b): pre-existing `refs/heads/master.lock` | Scope returns `ErrLocked` on the ref; its own `index.lock` removed; foreign ref lock, ref and index untouched. |
| T9c | Success path | (c) | New commit visible to CLI, `git fsck` clean, no locks. |
| T9d | CLI commit inside the index-rename-to-ref-rename window | (d): hook runs `git commit --allow-empty` between the two renames | CLI exits 128 `fatal: cannot lock ref 'HEAD': Unable to create '.../refs/heads/master.lock': File exists`; no leftover lock. Matches the ADR-003 index-before-ref window analysis. |
| T10 | Census | `TestT10_Census` (go/ast over the module dir, `<x>.Storer.Index/SetIndex` calls, excluding tests, `storage/`, `plumbing/`) | **23 call sites**, equal to the plan's list: `status.go:122` (preloadStatus), `submodule.go:61,259,394`, `worktree.go:375,436,452,484`, `worktree_commit.go:62,109,125`, `worktree_status.go:134,250,354,390,412,441,586,603,682,708,723,741`. Output saved to `/tmp/s4/census.txt` equivalent; belongs in `gates.md`. |

## Read-your-writes matrix (`TestT7_ReadYourWritesMatrix`)

Twin fixtures (modified, deleted, untracked, staged rename; 2 commits; branch `other`); go-git op in one `WithIndexLock` scope vs CLI on the twin; compared `HEAD^{tree}`, `ls-files -s`, `status --porcelain=v1 --untracked-files=all`, HEAD ref, commit count, full worktree content; `git fsck` on the go-git twin.

| Operation | Result |
|---|---|
| `Commit(All)` | equal to CLI |
| `Add` then `Commit(Amend)` | equal |
| `Add(m.txt)` then `Commit` | equal |
| `Restore(Staged)` | equal |
| `Reset(Mixed)` | equal |
| `Reset(Soft)` | equal |
| `AddWithOptions(All)` | equal |
| `Remove` then in-scope `Status` (sees staged delete) | equal (probe-only) |
| `RemoveGlob` then `Commit` | equal (probe-only) |
| `Move` then in-scope `Status` | equal (probe-only) |
| `Commit(All+Amend)` | **go-git refuses**: `all and amend cannot be used together`. Not a lock problem. Needs an emulation (`AddWithOptions` of tracked paths then `Commit(Amend)`, not tested) or stays on the CLI. |
| `Reset(Hard)` | **differs**: go-git deletes the untracked `u.txt`; CLI keeps it. |
| `Checkout(Branch other, Keep)`, `Checkout(Hash, Keep)` | **differs**: go-git leaves `c.txt` staged as `A`; CLI removes it. |

`TestT12_StockResetHardAndCheckout` runs the same two operations with stock go-git and no lock layer: untracked `u.txt` is deleted (`survives=false`) and the `Checkout` status is identical to the scoped result (`A  c.txt | D d.txt | M m.txt | R r.txt -> r2.txt | ?? u.txt`). So both divergences are stock go-git semantics, independent of the lock layer.

## Findings that change the plan

1. **Operation list, atomic vs CLI** (Revision 5 note applied). Can be made atomic through the scope (index and ref writes buffered, nothing touches the worktree before the rename): `Commit(All)`, `Commit(Amend)`, `Add` then `Commit`, `Restore(Staged)`, `Reset(Mixed|Soft)`, `AddWithOptions(All)`, plain `Add`. Must stay CLI (`unsafe_worktree_write`): `Checkout`, `Reset(Hard|Merge)`, `Restore(Worktree)` (write the worktree before `SetIndex`, and the first two also diverge from CLI semantics, measured above), plus `Remove`/`RemoveGlob`/`Move` (index results equal on the happy path but go-git deletes or renames files before `SetIndex`; the abort case was not fault-injected here, so they stay CLI per Story 2.3.1). `Commit(All+Amend)` is not a go-git feature.
2. **`Reset(Hard)` data-loss hazard independent of this work**: stock go-git `Reset(Hard)` deletes untracked files (T12). Any existing use in the product should be audited in Story 0.1.1c; `Reset(Hard)` must not be promoted without a CLI-parity test.
3. **`CommitOptions.Validate` mutates the options** (sets `Parents` to the current HEAD when empty). Reusing one options value across repos produced `object not found` in my first matrix run (my test bug, found and fixed). A production wrapper must build `CommitOptions` per call.
4. **Stock truncation is worse than corruption messages**: Apple Git 2.50.1 segfaults on a partially written index (T3b). This makes the scoped write (T3a) a correctness fix, not a nicety, for any agent running `git status` while the server stages.
5. T2's 0-collision outcome shows a probabilistic CLI-vs-go-git race rarely overlaps here; use T13 (deterministic hold) and T6 (in-process contention) as the standing regression tests, not T2 alone.

## Not verified (do not read the PASS as covering these)

- Linked worktrees (`<CommonDir>/worktrees/<name>`, `EnableDotGitCommonDir`): all tests used a main repo. `scopedStorer.fs` is `Storage.Filesystem()`; ref lock paths and index location for linked worktrees were not tested. Required before Story 2.3.1 is accepted.
- Ref delete, `packed-refs.lock`, `HEAD.lock`, `config.lock`, and the 60 s `pack-refs`/`branch -d` stress loop: not implemented. Reflog lines: not written (go-git writes none), so `git reflog` after a scoped commit lacks the entry; the plan's reflog criterion is open.
- Journal/token/`xattr` crash recovery, `SIGKILL` leftovers, 60 s age rule, instance flock: not implemented (Story 2.3.1 scope).
- Worktree-write fault injection (billy decorator, every N): not run; hence `Checkout`/`Reset(Hard)` stay CLI.
- `Wrote()` after a failed index rename: set before the rename by construction, not fault-injected.
- Index-version coverage (v3/v4 round-trip through the scoped `Index()`/`SetIndex`), `index.lock` mtime and ModTime (racy-git) behavior: not tested.
- Census test compares only the count (23), not the list; the list needs committing to `gates.md`.
- Test durations are inflated by the sandbox (CLI git calls 0.1 to 2.4 s); no perf claim is made.
