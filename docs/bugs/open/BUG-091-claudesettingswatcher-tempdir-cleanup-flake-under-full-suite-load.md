# BUG-091: TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable's `t.TempDir()` cleanup flakes under full `server/services` suite load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-27 (during the `async-session-creation` spec-compliance sweep — unrelated to
that diff; surfaced by running the full `server/services` suite as a verification step, the same way
BUG-090 was discovered the day before)
**Impact**: Test-only. `go test ./server/services/...` intermittently reports one failure among ~2000
tests:

```
--- FAIL: TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable
    testing.go:1464: TempDir RemoveAll cleanup: unlinkat /var/folders/.../001/...: <truncated>
```

Re-running the same test in isolation (`-run TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable
-count=20 -race`) passes reliably (20/20, race-clean).

## Problem Description

`TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable` (`server/services/session_service_test.go:3247`)
does:

```go
home := t.TempDir()
t.Setenv("HOME", home)
storage := createTestStorage(t)
eventBus := events.NewEventBus(100)
svc := NewSessionService(storage, eventBus)
t.Cleanup(func() { svc.Shutdown() })
assert.NotNil(t, svc.GetClaudeSettingsWatcher())
```

It only asserts the watcher is non-nil — it never calls `claudeSettingsWatcher.Start(ctx)` (per the
test's own doc comment, that's `wireDepsIntoServer`'s job in production, not this test's), so no
fsnotify goroutine actually watches `home` here. `t.Cleanup` is LIFO, so `svc.Shutdown()` (registered
second) runs before `t.TempDir()`'s own `RemoveAll` (registered first, when `t.TempDir()` was called) —
if `Shutdown()` is fully synchronous, that ordering should already be race-free.

The observed failure is Go's own `t.TempDir()` cleanup calling `RemoveAll` on `home` and hitting a
mid-directory `unlinkat` error — meaning something was still creating/writing an entry under `home` at
the moment `RemoveAll` walked it. This is the same "background goroutine outlives Shutdown() and
touches a test's now-being-removed directory" failure family as
`.claude/rules/fix-flaky-tests-dont-defer.md`'s worked examples, and structurally similar to BUG-089
(`server-shutdown-joins-background-tickers-flake`).

## Reproduction Steps

1. `go test ./server/services/...` (full package, ~2050 tests) — occasionally reports this test as
   `FAIL` with a `TempDir RemoveAll cleanup: unlinkat ...` error.
2. `go test ./server/services/... -run TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable
   -count=20 -race -v` — passes every time, race-clean.
3. Not yet reproduced with a tight repeat loop or artificial load in isolation; like BUG-090, it appears
   to need genuine full-suite scheduler/filesystem contention to manifest.

## Root Cause (partial — not yet confirmed)

**Hypothesis, not yet verified**: `NewSessionService` starts at least one component
(`analyticsStore.Start(context.Background())` at `server/services/session_service.go:619` is one
candidate — notably keyed off `context.Background()` rather than a context `Shutdown()` is known to
cancel, though its backing store in this test is the in-memory ent `concStorage`, not the filesystem,
so it may be a red herring) whose write path is not fully joined by `svc.Shutdown()` before that call
returns. A background goroutine writing under `$HOME` (set to this test's `t.TempDir()` result) even a
few milliseconds after `Shutdown()` returns would race the LIFO-deferred `t.TempDir()` `RemoveAll`,
producing exactly this symptom. Every test run in this package also logs a recurring, unrelated-looking
`WARN failed to save default config err="...open .../config.json.<tmp>.tmp: no such file or directory"`
pattern (a create-temp-then-rename config save racing its own directory's lifecycle) — worth checking
whether that save path is reachable from `NewSessionService`'s construction and targets `$HOME` rather
than only the fixed `~/.stapler-squad/test/test-<pid>/` global test dir.

## Files Affected

- `server/services/session_service_test.go` (test itself, ~line 3247)
- Likely `server/services/session_service.go`'s `NewSessionService`/`NewSessionServiceWithSearchEngine`
  construction path (whichever sub-service's `Start`/background goroutine isn't joined by `Shutdown()`)
- Possibly the "save default config" path referenced above, if it turns out to target `$HOME` rather
  than a fixed test dir

## Fix Approach

1. Reproduce reliably: run the full suite in a loop, or under `GOMAXPROCS=1`/artificial CPU load, until
   it flakes again — ideally capturing which goroutine (via `-race` or a `pprof` goroutine dump at the
   moment of failure) is still touching `home` after `Shutdown()` returns.
2. Once identified, join that goroutine into `Shutdown()`'s existing wait mechanism (mirroring however
   BUG-089's background-ticker join was fixed) rather than widening a timeout — this is a real
   synchronization gap, not a timing budget issue like BUG-090's current leading hypothesis.

## Verification

After fix: `go test ./server/services/... -race -count=10 -run
TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable` must pass every iteration, and repeated
full-suite runs must not reproduce the `TempDir RemoveAll cleanup` failure for this test.

## Related

- **2026-09-08: `TestTriggerTriage_RunsInIsolatedWorktree_When_RepoPathIsARealGitRepo` instance
  root-caused and fixed** (unlike this bug's own still-unconfirmed ClaudeSettingsWatcher hypothesis).
  Hit during `make quick-check` on PR #735 (`fix-control-mode-input-silent-drop`, an unrelated
  control-mode diff) after merging `main`. Confirmed root cause via reading
  `server/services/backlog_service_triage.go`'s `TriggerTriage`: the test's only synchronization was
  `require.Eventually(pool.callCount() == 1)`, which only proves `CallBlocking` was *invoked* — the
  background goroutine keeps running past that point (commit + `retitleTriageWorktreeToFinalBranch`
  writing into the worktree under `repoPath/.git/worktrees/`, i.e. exactly the `t.TempDir()` the test
  is about to tear down), so the test could return — starting `RemoveAll` — while that goroutine was
  still writing files. This is the *general* mechanism this bug's title names ("something outlives
  test teardown and still touches the TempDir"), now confirmed for one concrete instance instead of
  hypothesized. Fixed both this test and its sibling `TestTriggerTriage_FallsBackToRepoPathDirectly_When_RepoPathIsNotAGitRepo`
  by polling `storage.ListItemSessions(...)[0].EndedAt != nil` instead (the trailing write that lands
  after all worktree file writes are done) — the same pattern `TestTriggerTriage_Success` already used
  for the identical reason. Did not use the package's `testTriageCompleteHook` (used by other tests for
  this same wait) because both fixed tests call `t.Parallel()`, and that hook is a single
  package-global function var — a sibling parallel test's own registration would silently clobber this
  test's, dropping its completion signal (see the hook's own doc comment warning). This closes the
  TriggerTriage instance specifically; BUG-091 stays open for the still-unconfirmed
  ClaudeSettingsWatcher root cause below.
- **2026-09-07 sighting (second, same day)**: the identical `TempDir RemoveAll cleanup: unlinkat ...
  directory not empty` symptom recurred again on the *same* test,
  `TestTriggerTriage_RunsInIsolatedWorktree_When_RepoPathIsARealGitRepo` (`server/services`), CI run
  [34157839904](https://github.com/tstapler/stapler-squad/actions/runs/34157839904/attempts/1) (job
  101853656914), on PR #733 (`fix-worktree-browse-button-e2e-timeout`) — an unrelated diff. Re-running
  the same job with no code changes (attempt 2) passed cleanly. Confirms this recurrence is the same
  shared teardown-ordering gap, not something PR #733's diff introduced; re-ran rather than investigating
  further, consistent with this bug's own scope boundary and the precedent set by the entry below.
- **2026-09-07 sighting**: the identical `TempDir RemoveAll cleanup: unlinkat ... directory not empty`
  symptom recurred on a third test, `TestTriggerTriage_RunsInIsolatedWorktree_When_RepoPathIsARealGitRepo`
  (`server/services`), CI run [34075111973](https://github.com/tstapler/stapler-squad/actions/runs/34075111973/job/101599943522),
  on PR #718 (`session/instance_worktree.go` race fixes) — passed 10/10 in isolation with `-race`
  immediately after (one iteration logged a transient, already-tolerated `not a git repository` staging
  error, unrelated to the cleanup failure). Confirms this is once again the shared teardown-ordering gap,
  not something PR #718's diff introduced. Re-ran the failed CI job rather than investigating further,
  consistent with this bug's own scope boundary.
- **2026-09-02 sighting**: the identical `TempDir RemoveAll cleanup: unlinkat ... directory not empty`
  symptom recurred on a *different* test, `TestCreateSession_Autonomous_ExplicitPath_DoesNotGenerateScratchDir`
  (`server/services`), during a full `./session/... ./server/services/...` run — passed 3/3 in isolation
  immediately after. Confirms this failure family isn't specific to
  `TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable`'s construction path; it's a broader
  "something outlives test teardown and still touches the TempDir" gap, strengthening the case for
  root-causing the shared mechanism (whatever `Shutdown()`/cleanup fails to join) rather than patching
  per-test. Observed while verifying an unrelated perf-fix branch's test suite; not investigated further
  here (out of scope for that work).
- `.claude/rules/fix-flaky-tests-dont-defer.md` — this repo's standing rule against re-excusing a known
  flake without root-causing or filing it; this bug is that filing for the instance discovered during
  the `async-session-creation` spec-compliance sweep.
- `docs/bugs/open/BUG-090-hubregistry-restartpump-reconnect-flake-under-full-suite-load.md` — sibling
  full-suite-only flake discovered the day before, same discovery method (running the full package as a
  verification step for unrelated work).
- `docs/bugs/open/BUG-089-server-shutdown-joins-background-tickers-flake.md` — prior art for the
  "Shutdown() doesn't join every background goroutine" failure family this bug's leading hypothesis
  belongs to.
