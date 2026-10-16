# BUG-069: `TestSharedIndex_SecondAndLaterWorktreesCostLessThanFirst` hangs past a 2-minute timeout when run locally (non-CI) [SEVERITY: Low]

**Status**: ✅ Fixed
**Discovered**: 2026-08-12
**Fixed**: 2026-08-16
**Impact**: Fails/times out `go test ./session/...` locally whenever this prototype package's test binary runs outside CI (the test explicitly skips itself under `CI=1`, so this cannot fire in GitHub Actions). Makes `go test ./session/...` unreliable as a local pre-push gate — a developer running the full suite locally sees a spurious `FAIL` in an unrelated package after 600s+.

## Problem Description

`session/unfinished/gogitstore.TestSharedIndex_SecondAndLaterWorktreesCostLessThanFirst` builds a fixture repo with `numCommits = 250`, each commit driving two `git` subprocess invocations (`git add .`, `git commit -q -m ...`) through `gitRunErr` (`gogitstore_test.go:92`), then forces `git gc --aggressive` to pack it, repeated across `numWorktrees = 5` worktrees for both a shared-storer and a control measurement. Locally this run took over 600s in a full-suite run and, isolated with `-timeout 120s`, panicked with `panic: test timed out after 2m0s` while blocked inside `os/exec.(*Cmd).Wait` (called from `gitRunErr`'s `cmd.CombinedOutput()` at `gogitstore_test.go:103`, via `buildPackedFixtureOnce` → `buildPackedFixture` → the test itself).

This package is explicitly marked as a prototype (`session/unfinished/gogitstore`), and the test already self-skips in CI with an inline comment citing a related, distinct issue:

```go
if os.Getenv("CI") != "" {
    // ponytail: skipped in CI — git gc --aggressive under this repo's current CI load reliably
    // corrupts the fixture repo (see PR #162); needs either a lighter non-aggressive gc or
    // serialized/non-parallel test execution to fix properly, not attempted here
    t.Skip("skipped in CI — see PR #162")
}
```

PR #162's issue was **corruption** under CI load; this bug's symptom is different — a **hang/extreme slowness** when run locally (not under CI, so the `CI` skip doesn't apply) — but is very plausibly the same underlying class of problem the file's own extensive comments already document at length (see `gogitstore_test.go:70-91` and `:345-387`): `git gc --auto` / `maintenance.auto` background auto-housekeeping racing the test's own explicit `git gc --aggressive`/commit loop, even after both are explicitly disabled per-repo (`gc.auto=0`, `maintenance.auto=false`) — 250 commits × 2 subprocess spawns × 5 worktrees × (shared + control) is also simply a lot of `git` subprocess overhead on its own, so ordinary local machine load/contention could push total wall-clock time past 120–600s without any race being involved at all.

## Reproduction Steps

1. Full suite: `go test ./session/...` — `gogitstore` package reported `FAIL github.com/tstapler/stapler-squad/session/unfinished/gogitstore 605.122s`.
2. Isolated: `go test ./session/unfinished/gogitstore/... -v -timeout 120s` — deterministically reproduced:
   ```
   === RUN   TestSharedIndex_SecondAndLaterWorktreesCostLessThanFirst
   panic: test timed out after 2m0s
       running tests:
           TestSharedIndex_SecondAndLaterWorktreesCostLessThanFirst (1m54s)
   ```
   with the blocked goroutine's stack rooted in `gitRunErr`'s `cmd.CombinedOutput()` (`gogitstore_test.go:103`), reached via `buildPackedFixtureOnce` (`:397`, inside the per-commit `git add .` call in the fixture-build loop) ← `buildPackedFixture` (`:190`) ← `TestSharedIndex_SecondAndLaterWorktreesCostLessThanFirst` (`:502`).
3. `CI` env var was unset in both repro runs, so the test's own CI skip did not apply — this is a **local-only** manifestation, distinct from PR #162's CI-only corruption symptom.

## Root Cause

Not fully isolated. Two non-exclusive candidates, both already partially documented inline in this file by prior authors:

1. **Genuine slowness, not a true hang**: 250 commits × 2 subprocess spawns × 5 worktrees × 2 measurement passes (shared + control) is ~2,500+ `git` subprocess invocations total; on a loaded local machine this could plausibly exceed even a 120s single-test timeout without any deadlock.
2. **Residual auto-gc race**: despite `gc.auto=0` and `maintenance.auto=false` being set per-repo (`gogitstore_test.go:368,385`), the extensive comments at `:345-387` describe a documented history of new auto-trigger surfaces being discovered one at a time (PR #190 closed `gc.auto`, a later CI run found `maintenance.auto` was a second, independent trigger) — it is plausible a third surface (or a lock left over from a previous partial run under CI, if `t.TempDir()` collisions or leftover state were ever in play) still exists and causes an indefinite subprocess block rather than a race that merely produces bad output.

Confirmed **unrelated** to the concurrent `RepairCorruptedGitRepo` fix (`session/repo_path.go`, `session/instance_worktree.go`) — no file overlap, and this package's fixture-build code predates that change.

## Files Likely Affected

- `session/unfinished/gogitstore/gogitstore_test.go` — `buildPackedFixture`/`buildPackedFixtureOnce`/`gitRunErr`/`TestSharedIndex_SecondAndLaterWorktreesCostLessThanFirst`.

## Fix Approach

Implemented candidate (c): `gitRunErr` (`gogitstore_test.go`) now wires each subprocess through `context.WithTimeout(context.Background(), gitCommandTimeout(args))` instead of the previous unbounded `context.Background()`. `gitCommandTimeout(args []string) time.Duration` returns 90s for `gc` subcommands (which can legitimately run longer under contention) and 30s otherwise — see commit `da52d9c8`. A wedged git subprocess now fails fast with a clear timeout error instead of relying on `go test -timeout` to eventually panic the whole binary 2+ minutes later. `mmap_stage2_test.go`'s `runGitCmd` is a thin pass-through wrapper over `gitRunErr` and inherits the fix with no separate change needed (verified via a targeted run of its callers — all 6 passed). `mmap_adversarial_test.go`'s own inline fuzz-seed-builder call site was also updated to use `gitCommandTimeout(args)` (commit `9a7fd788`) — it had been missed in the initial pass.

Candidates (a)/(b)/(d) were not needed once (c) closed the actual hang.

## Verification

- `go test ./session/unfinished/gogitstore/... -v -run TestSharedIndex_SecondAndLaterWorktreesCostLessThanFirst -timeout 120s`, CI unset: 5 consecutive passing runs, each well under the timeout.
- `gc.auto=0`/`maintenance.auto=false` behavior unchanged; `countPacks()==1` still passes.
- Targeted runs covering the fix's surface area all pass cleanly: the above 5x, plus `TestPackWatch_FsnotifyTriggersRefresh` and `TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack` (the only test exercising `runGitCmd`, the other `gitRunErr` call site) in isolation. A single unbroken full-package run under one `-timeout` was not obtained this session — bundled runs across multiple fixture-heavy tests hit the outer `-timeout` while inside legitimate (documented above) fixture-construction work, not a subprocess hang. An earlier draft of this note cited a specific full-suite pass/fail count and a "BUG-077" tracking bug for `TestGogitstore_SoakUnderSustainedLoad`; no `BUG-077` file exists in `docs/bugs/`, so that citation is retracted as unverified rather than restated.

## Related Tasks

Discovered while verifying the `RepairCorruptedGitRepo` fix (backlog-triage worktree creation, `/errors/` page investigation) via `go test ./session/...`. Filed per `.claude/rules/fix-flaky-tests-dont-defer.md` rather than silently re-excused as "known pre-existing, unrelated" — root-caused to the extent possible without a dedicated profiling session, and explicitly not fixed in this session because it is out of scope for the `RepairCorruptedGitRepo` change and belongs to an already-flagged prototype package (`session/unfinished/gogitstore`, itself already self-skipping in CI for a related reason).
