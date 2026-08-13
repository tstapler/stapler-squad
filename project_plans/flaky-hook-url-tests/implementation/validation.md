# Validation Plan: flaky-hook-url-tests

**Date**: 2026-08-01
**Depends on**: `requirements.md`, `implementation/plan.md`, `decisions/ADR-001-p1-flag-over-isolated-invocation.md`

---

## Happy Path Scenario

Given the two named tests (`TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` and
`TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`,
`server/server_integration_test.go`) run under CI's gating `go test -race` invocation
(`.github/workflows/build.yml:155-157`, spanning `./server/... ./session/... ./config/...`),
when `waitForPermissionRequestHookCommand`'s two call-site budgets are widened from
30s to 60s and converted (along with the other three poll helpers) to
`require.Eventually`/`testutil/wait.WaitForCondition`, and `-p 1` is appended to the
gating invocation to serialize the three package binaries instead of running them
concurrently on the 4-vCPU runner, then both tests pass reliably (near-zero flakes
across the next 20 CI runs, per requirements.md's Success Metrics), the same
`coverage.out` artifact is still produced by the one gating command with `-race`
coverage unchanged for all three package trees, and the 60%-threshold coverage gate
still passes.

---

## Requirement → Test Mapping

| Requirement / Scope Item (requirements.md) | Plan Task(s) | Test Case(s) | Test Type |
|---|---|---|---|
| Diagnose why 30s waits are marginal under `-race` + full-suite load (In Scope #1) | N/A (research, not code) | `TestFlakeRepro_should_ReproduceRaceB_When_RunUnderArtificialCPUContention` (manual/documented repro, not an automated Go test — see Stress/Flake-Verification below) | Manual verification |
| Fix the stale 30s→60s call-site bug (Task 1.1.1) | 1.1.1 | `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` (regression, post-change) | Regression (existing test, unchanged assertions) |
| " | 1.1.1 | `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated` (regression, post-change) | Regression (existing test, unchanged assertions) |
| Convert `waitForPermissionRequestHookCommand` to `require.Eventually` (Task 1.2.1) | 1.2.1 | `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` / `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated` still pass post-conversion (regression); manual negative-path check: settings file never written → `require.Eventually` fails with `"Condition never satisfied"` + custom message, matching current `t.Fatalf` semantics | Regression + manual failure-mode check |
| Convert `waitForLiveInstance` to `require.Eventually` (Task 1.2.2) | 1.2.2 | Same two tests, regression pass; helper returns identical `*session.Instance` pointer behavior (implicit — both tests already assert on fields of `inst`, e.g. `inst.MCPServerURL`, so a wrong/nil `inst` fails those assertions directly) | Regression |
| Convert `waitForResolvedAddr` to `require.Eventually` (Task 1.2.3) | 1.2.3 | `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated` (only caller of `waitForResolvedAddr`) still passes post-conversion | Regression |
| Convert `waitForTmuxTeardown` to non-fatal `testutil/wait.WaitForCondition` equivalent (Task 1.2.4) | 1.2.4 | `TestServer_should_NonFatallyLogTeardownDelay_When_TmuxSessionExistsCheckIsArtificiallyDelayed` | New unit-style verification (manual local repro per plan's own Given/When/Then; see below) |
| Add `-p 1` to the gating invocation (Task 2.1.1) | 2.1.1 | `TestCI_should_PreserveRaceCoverageAndCoverageArtifact_When_P1FlagIsAdded` (workflow-log verification, not a Go test) | CI workflow verification |
| Measure wall-clock cost of `-p 1` (Task 2.1.2) | 2.1.2 | `Measurement_should_RecordWallClockDelta_When_P1IsAddedVsBaseline` (≥3-run averaged local timing, not a Go test) | Manual measurement |
| Add stress-repro comment (Task 3.1.1) | 3.1.1 | `TestFlakeRepro_should_ReproduceRaceB_When_RunUnderArtificialCPUContention` | Manual repro / documentation verification |
| Cross-link repro command from doc comment (Task 3.1.2) | 3.1.2 | Manual doc-comment review (grep for the cross-reference sentence) | Manual review |
| Must not weaken `-race` coverage for `./server/... ./session/... ./config/...` (Constraints) | 2.1.1 (ADR-001) | `TestCI_should_PreserveRaceCoverageAndCoverageArtifact_When_P1FlagIsAdded` | CI workflow verification |
| Must not require new external dependencies (Constraints) | 2.1.1, 1.2.1-1.2.4 | Manual `go.mod` diff review — confirm no new entries (testify already a direct dep; `testutil/wait` is in-repo) | Manual review |
| Adding/strengthening a regression check so this flake class can be re-diagnosed quickly (In Scope, last bullet) | 3.1.1, 3.1.2 | `TestFlakeRepro_should_ReproduceRaceB_When_RunUnderArtificialCPUContention` | Manual repro |
| No increase in runtime budget "beyond what's demonstrably needed" (Success Metrics) | 1.1.1, 2.1.2 | `Measurement_should_RecordWallClockDelta_When_P1IsAddedVsBaseline` | Manual measurement |
| Zero/near-zero intermittent failures across next 20 CI runs (Success Metrics) | All | CI run history over 20-run observation window (post-merge, not verifiable pre-merge) | Observational (post-ship) |

**Coverage**: 12 of 12 requirement/scope items mapped (100%). Two items
(20-run observation window; runner-sizing investigation, explicitly out-of-scope per
requirements.md) are **not verifiable before merge** — flagged below in "What Cannot
Be Verified Pre-Merge," not silently dropped.

---

## UX Acceptance Tests

N/A — no user-facing surface. This is a CI/test-infrastructure fix touching only
`server/server_integration_test.go` and `.github/workflows/build.yml`; no production
code path, UI component, or API contract changes (confirmed: no edits to
`approval_handler.go`, `session_service.go`, or `session/tmux/tmux.go` per plan.md's
Risk Control section).

---

## Test Stack

- **Language/framework**: Go 1.x, standard `testing` package + `github.com/stretchr/testify/require` (already a direct dependency, `go.mod`, `require.Eventually` — confirmed used elsewhere in this package at `server/services/approval_handler_integration_test.go`, 3 call sites).
- **In-repo polling helper**: `testutil/wait.WaitForCondition` (`testutil/wait/wait.go:53`) — confirmed to take `condition func() bool` and `WaitConfig{Timeout, PollInterval, Description}`; confirmed no import cycle (`server` already imports `session`; `testutil/wait` imports neither `server` nor `session`).
- **Race detector**: `go test -race` (ThreadSanitizer), unchanged scope (`./server/... ./session/... ./config/...`).
- **CI runner**: GitHub Actions `ubuntu-latest`, 4 vCPU, `.github/workflows/build.yml`'s `Test` job.
- **Coverage tooling**: `-covermode=atomic`, `coverage.out`, gated by `vladopajic/go-test-coverage@v2` action (60% global threshold, `local-threshold: 0`).
- **No new test frameworks, mocking libraries, or external dependencies** are introduced — this is a constraint from requirements.md and is honored (verified via `go.mod` review, see mapping table above).

---

## Stress / Flake-Verification Test (Integration-Equivalent)

This is the closest thing to an "integration test" for a CI-reliability fix — a
before/after stress comparison, per the plan's own repro command (Task 3.1.1) and
requirements.md's Constraints ("validate via ... a stress-run that demonstrates the
marginal timeout and then demonstrates it's resolved").

**Before the change** (establish current flake baseline, if reproducible):
```bash
git stash   # ensure baseline code (30s budgets, hand-rolled loops, no -p 1)
yes > /dev/null & yes > /dev/null & yes > /dev/null &
TMUX_BIN="$(pwd)/bin/tmux" go test -race -count=10 \
  -run 'TestServer_should_Write.*HookURL' ./server/...
kill %1 %2 %3
git stash pop
```
- **Expected (pre-change)**: some non-zero failure rate under artificial contention
  approximating CI's 4-vCPU runner under full `-race` load — this is the flake this
  ticket targets. (Per requirements.md's Feasibility Risks, this may not reproduce
  identically to the exact CI failure; "reasonably convincing local stress repro" is
  the bar, not exact reproduction.)

**After the change** (confirm improvement):
```bash
yes > /dev/null & yes > /dev/null & yes > /dev/null &
TMUX_BIN="$(pwd)/bin/tmux" go test -race -count=10 \
  -run 'TestServer_should_Write.*HookURL' ./server/...
kill %1 %2 %3
```
- **Expected (post-change)**: 10/10 passes across the `-count=10` repeat, with the
  widened 60s budget and `require.Eventually` conversion in place. Run this at least
  once locally before merge as the "green first, then done" gate for Epic 1; the true
  confirmation is the 20-CI-run observation window (Success Metrics), which is
  necessarily post-merge.

**Named test cases covered by this stress run**:
- `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`
- `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`

---

## Task 1.2.4 Verification: Non-Fatal `waitForTmuxTeardown` Behavior

Per plan.md's own Given/When/Then for Task 1.2.4, verify the converted teardown
helper stays non-fatal:

1. In a throwaway local branch, inject an artificial delay before
   `inst.TmuxSessionExists()` can return `false` (e.g. temporarily wrap/stub it to
   sleep 6 seconds before evaluating), exceeding the helper's 5s `timeout`.
2. Run `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` with the
   stub active.
3. **Expected**: the test still reports `PASS` overall, with a `t.Logf` line in the
   test output reading something like `"tmux session for ... still reported alive
   5s after DeleteSession; teardown may still be in flight"` — confirming the
   timeout in the cleanup path does not retroactively fail an already-decided PASS
   verdict. This mirrors the existing hand-rolled behavior exactly (same `t.Logf`,
   not `t.Fatalf`/`t.Errorf`, per the code's own comment at lines 522-526).
4. Revert the artificial stub after verification — it is a throwaway repro, not
   committed code.

---

## Task 2.1.1 Verification: `-p 1` Preserves Coverage Artifact + `-race` Scope

1. Run the gating command locally with `-p 1` added:
   ```bash
   TMUX_BIN="$(pwd)/bin/tmux" go test -race -p 1 -coverprofile=coverage.out \
     -covermode=atomic ./server/... ./session/... ./config/...
   ```
2. **Expected**: `coverage.out` is produced (single file, same `mode: atomic` header),
   and `go tool cover -func=coverage.out | tail -1` reports a total percentage not
   meaningfully different from a pre-`-p 1` baseline run (same command minus `-p 1`)
   — confirming the coverage gate's 60%-threshold check sees an equivalent artifact.
3. **Expected**: raw test output/log shows the three package binaries'
   `--- PASS`/`--- FAIL`/`ok`/`FAIL` summary lines for `./server/...`, `./session/...`,
   `./config/...` appearing sequentially rather than interleaved — confirming
   serialization took effect (this is the observable proxy for "`-race` still covers
   all three package trees, just sequentially not concurrently," since `-race`'s own
   instrumentation output isn't otherwise distinguishable per-package in the default
   log format).
4. Post-merge corroboration: confirm on the actual next CI run of the `Test` job that
   the "Coverage gate" step still passes at ≥60% and the step exit code for "Run tests
   with coverage" is 0 (or the existing `-v ./...` diagnostic fallback correctly
   triggers only on genuine failure, unchanged from today).

---

## Task 2.1.2 Verification: Wall-Clock Measurement (Averaged, Not Single-Sample)

Per the architecture review's noise concern (plan.md footnote, implicit in Task
2.1.2's "Size: 5 min" being a measurement task, not a one-shot timing), average over
**at least 3 runs each**, not a single before/after sample — a single sample cannot
distinguish signal from ordinary machine-load noise (background processes, thermal
throttling, disk cache state).

```bash
# Baseline (no -p 1), 3 runs:
for i in 1 2 3; do
  /usr/bin/time -p env TMUX_BIN="$(pwd)/bin/tmux" go test -race \
    -coverprofile=/tmp/cov-before-$i.out -covermode=atomic \
    ./server/... ./session/... ./config/... 2>> /tmp/before-timings.txt
done

# With -p 1, 3 runs:
for i in 1 2 3; do
  /usr/bin/time -p env TMUX_BIN="$(pwd)/bin/tmux" go test -race -p 1 \
    -coverprofile=/tmp/cov-after-$i.out -covermode=atomic \
    ./server/... ./session/... ./config/... 2>> /tmp/after-timings.txt
done
```

- **Expected**: record the mean (or median, to reduce single-outlier skew) real-time
  delta between the two sets of 3 runs in the shipping PR description as an explicit,
  accepted trade-off (per plan.md's Task 2.1.2 Given/When/Then — "not silently
  absorbed"). A single-sample A/B is explicitly insufficient per this validation
  plan's design constraint from the parent task.
- **Expected outcome shape**: some positive (slower) delta for the `-p 1` runs is
  expected and acceptable — the trade-off being validated is that this delta is
  "low-single-digit minutes," not that there is no delta at all (ADR-001's Negative
  consequence explicitly names this cost).

---

## Coverage Targets and How to Measure

- **Existing gate (unchanged by this ticket)**: 60% global threshold, `local-threshold: 0`, enforced by `vladopajic/go-test-coverage@v2` against `coverage.out` (`.github/workflows/build.yml:162-168`).
- **How to measure locally**:
  ```bash
  make build   # generates protos; required before go test per CLAUDE.md
  go test ./... -coverprofile=coverage.out
  go tool cover -func=coverage.out
  ```
  (CI's actual gating command additionally scopes to `./server/... ./session/... ./config/...` and adds `-race -p 1 -covermode=atomic`; the `go test ./...` form above is the general-purpose local coverage check per this repo's `make test-coverage` target.)
- **This ticket's coverage delta**: expected to be ~zero — no new production code is added (Epic 1 and Epic 2 are test-code and CI-workflow-only changes), so the 60% global threshold is not expected to move. The verification in Task 2.1.1 above (comparing `go tool cover -func` totals before/after `-p 1`) is the specific check that the *artifact* itself is unaffected by the `-p 1` flag, distinct from the ticket not being expected to change *what* is covered.

---

## What Cannot Be Verified Pre-Merge

- **Zero/near-zero flakes across 20 consecutive CI runs** (Success Metrics) is
  inherently a post-merge, observational verification — it requires the fix to be
  merged and running in real CI over time. Pre-merge, the best available proxy is
  the local stress-run (`-count=10` under artificial `yes`-loop contention, above)
  plus the reasoning already captured in ADR-001 and `research/stack.md`.
- **Actual GitHub Actions runner wall-clock delta** for `-p 1` (as opposed to the
  local approximation in Task 2.1.2) is explicitly flagged in plan.md's Unresolved
  Questions as "measure in CI, don't just trust the local estimate" — a known gap,
  not silently dropped, to be corroborated on the first post-merge CI run.
- **Whether GitHub Actions runner-level cross-*job* contention (not just cross-
  *package* contention within this one job) is a contributing factor** is explicitly
  out of scope per requirements.md's Rabbit Holes / Unresolved Questions and is not
  addressed or verified by this validation plan.

---

## Migration Test

N/A — no schema or data migration in this ticket (plan.md's own Migration Plan
section: "Omitted — no schema or data changes. This is a test-file and
CI-workflow-only change.").
