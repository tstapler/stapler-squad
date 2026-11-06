# Validation Plan: ci-hookurl-race-flake

**Date**: 2026-08-01
**Depends on**: `requirements.md`, `implementation/plan.md`
**Methodology reuse**: this project's stress-repro procedure, averaged wall-clock
measurement methodology, and requirement→test mapping format are adapted directly from
`project_plans/flaky-hook-url-tests/implementation/validation.md` (cited inline below at
each reused section) — per plan.md's Provenance section, this project adopts that
project's plan wholesale, so its validation methodology is reused rather than
re-derived. This document adds project-specific coverage for **this project's own 5
acceptance criteria**, which differ from the sibling's requirement list — most notably
AC #4 ("measure, don't assume" the hook-injection pipeline's actual latency), which the
sibling's validation.md does not cover and which plan.md's new Task 2.1.2b exists
specifically to close.

---

## Happy Path Scenario

Given the current flaky state (`TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`
and `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`
intermittently time out at 30s under `go test -race` + full-suite CI load), when the
plan's tasks are applied (60s budget at both `waitForPermissionRequestHookCommand` call
sites, `require.Eventually`/`testutil/wait.WaitForCondition` conversions for all four poll
helpers, and `-p 1` added to the gating `go test -race` invocation), then the two named
tests stop intermittently timing out under `-race` + full-suite CI load, with the
trade-off (config-change rationale, `-race`-coverage preservation, and measured — not
assumed — pipeline latency) explicitly documented rather than silently absorbed.

---

## Requirement → Test Mapping

| # | Acceptance Criterion (requirements.md) | Plan Task(s) | Test / Verification Case(s) | Test Type |
|---|---|---|---|---|
| 1 | Hook-URL/MCP-URL integration tests do not intermittently time out under normal CI load (verified by N consecutive green CI runs post-fix, or a documented equivalent stress-test result) | 1.1.1, 1.2.1, 2.1.1, 3.1.1 | `stress_repro_should_PassConsistently_When_RunWithArtificialContention` (local `-count=10` stress run, before/after); `ci_run_history_should_ShowNearZeroFlakes_When_Observed20RunsPostMerge` (post-merge observational, per Observability Plan's "Secondary signal") | Stress/regression test + post-merge observational |
| 2 | Any change to test/CI configuration (timeout values, `-race` scope, job parallelism, runner sizing) is captured with a rationale, not a bare number bump | 1.1.1, 2.1.1, 3.1.2, 3.1.3 | `config_change_rationale_should_BePresentInComments_When_TimeoutOrPFlagIsModified` (grep-based doc-comment review) | Manual review (grep-verifiable) |
| 3 | If the fix narrows or removes `-race` coverage for any package, that trade-off is explicit and justified | 2.1.1, 2.2.1 | `coverage_gate_should_PreserveRaceScopeAndThreshold_When_P1FlagIsAdded` (before/after `coverage.out` diff) | CI workflow verification |
| 4 | The hook-injection pipeline's actual latency under `-race` + concurrent-suite load is measured (not assumed) before deciding between "raise the timeout further" and "make the pipeline faster/more deterministic" | 2.1.2b | `hook_injection_latency_should_BeMeasuredViaJSONCapture_When_StressRunExecutedWithGotestsum` (concrete `-json`/`gotestsum` capture, see dedicated section below) | Manual measurement (JSON-captured, not eyeballed) |
| 5 | No source behavior in `session/services/approval_handler.go` or the hook-injection path is changed unless the investigation shows the pipeline itself (not just test infrastructure) is the bottleneck | 2.1.2b (gate), all Epic 1/2 tasks (diff scope) | `approval_handler_diff_should_BeEmpty_When_PRIsShipped_UnlessLatencyMeasurementJustifiesChange` (git diff review, conditioned on AC #4's finding) | Manual review (git diff, conditional) |

**Coverage**: 5 of 5 acceptance criteria mapped (100%). AC #1's 20-run CI observation
window is the one item **not verifiable before merge** (see "What Cannot Be Verified
Pre-Merge" below, adapted from the sibling's identical caveat) — flagged, not silently
dropped.

---

## Test Stack

*(Adapted from `flaky-hook-url-tests/implementation/validation.md`'s Test Stack section
— unchanged, since this project adopts the same plan/code surface.)*

- **Language/framework**: Go 1.x, standard `testing` package + `github.com/stretchr/testify/require` (already a direct dependency, `go.mod:31`, `v1.11.1`; `require.Eventually` already used at 3 call sites in `server/services/approval_handler_integration_test.go`).
- **In-repo polling helper**: `testutil/wait.WaitForCondition` (`testutil/wait/wait.go`) — non-fatal, synchronous-condition alternative used only for `waitForTmuxTeardown` (Task 1.2.4); confirmed no import cycle with `server`/`session`.
- **Race detector**: `go test -race` (ThreadSanitizer), unchanged scope (`./server/... ./session/... ./config/...`).
- **CI runner**: GitHub Actions `ubuntu-latest`, 4 vCPU, `.github/workflows/build.yml`'s `test` job.
- **Coverage tooling**: `-covermode=atomic`, `coverage.out`, gated by `vladopajic/go-test-coverage@v2` (60% global threshold, `.github/workflows/build.yml:162-168`).
- **New for this project's AC #4 only**: `go test -json` and/or `gotestsum` (`gotestsum --jsonfile=<path>` + `gotestsum tool slowest`) — used strictly as a one-off diagnostic capture (Task 2.1.2b), not wired into CI or `make test`. If `gotestsum` is not already installed locally, `go test -json` alone (piped through `tparse` or manually inspected) is an equivalent zero-install fallback — no new dependency is added to `go.mod` either way.
- **No new test frameworks, mocking libraries, or `go.mod` dependencies** are introduced — consistent with requirements.md's implicit constraint (carried over from the sibling project) and re-confirmed by manual `go.mod` diff review.

---

## Stress / Flake-Verification Test (AC #1)

Reused verbatim from `flaky-hook-url-tests/implementation/validation.md`'s "Stress /
Flake-Verification Test" section — same repro command, same before/after structure, same
named test cases. Cited rather than re-derived because this project's AC #1 and the
sibling's requirement ("validate via a stress-run that demonstrates the marginal timeout
and then demonstrates it's resolved") are the same underlying claim.

**`stress_repro_should_PassConsistently_When_RunWithArtificialContention`**

Before (baseline, pre-fix code):
```bash
git stash   # ensure baseline code (30s budgets, hand-rolled loops, no -p 1)
yes > /dev/null & yes > /dev/null & yes > /dev/null &
TMUX_BIN="$(pwd)/bin/tmux" go test -race -count=10 \
  -run 'TestServer_should_Write.*HookURL' ./server/...
kill %1 %2 %3
git stash pop
```
- **Expected (pre-change)**: some non-zero failure rate under artificial contention — the
  flake this ticket targets.

After (patched code):
```bash
yes > /dev/null & yes > /dev/null & yes > /dev/null &
TMUX_BIN="$(pwd)/bin/tmux" go test -race -count=10 \
  -run 'TestServer_should_Write.*HookURL' ./server/...
kill %1 %2 %3
```
- **Expected (post-change)**: 10/10 passes across the `-count=10` repeat. Run at least
  once locally before merge as the "green first, then done" gate for Epic 1.

**`ci_run_history_should_ShowNearZeroFlakes_When_Observed20RunsPostMerge`**

Not a runnable command — a post-merge observational check against GitHub Actions' own
run history for the `test` job on `main` and PRs, over the next 20 runs (requirements.md
Success Metrics; exact N is this project's own, not re-derived). No new dashboard or
alerting is added (Observability Plan, plan.md). **Cannot be verified pre-merge** — see
"What Cannot Be Verified Pre-Merge" below.

---

## `config_change_rationale_should_BePresentInComments_When_TimeoutOrPFlagIsModified` (AC #2)

Grep-verifiable doc-review, not a Go test:

```bash
# Confirm the -p 1 change carries its rationale comment (Task 2.1.1):
grep -n -A1 "\-p 1" .github/workflows/build.yml
# Expected: inline comment citing project_plans/flaky-hook-url-tests/decisions/ADR-001-...

# Confirm the 60s call sites' rationale is the helper's own doc comment, now made
# accurate rather than stale (Task 1.1.1 makes code match comment; Task 3.1.2 cross-links
# the repro command from it):
sed -n '454,462p' server/server_integration_test.go
```

- **Expected**: the `-p 1` line in `.github/workflows/build.yml` is preceded by the
  comment specified in Task 2.1.1 ("`-p 1`: serializes package binaries to reduce
  `-race` CPU contention ... see project_plans/flaky-hook-url-tests/decisions/ADR-001 ...
  Do not remove without re-reading that ADR"), and `waitForPermissionRequestHookCommand`'s
  doc comment (lines 454-462) states "callers pass 60s" **and is now true** (call sites
  at lines 338/409 actually pass `60*time.Second`, closing the stale-comment bug this
  ticket exists to fix), plus the Task 3.1.2 cross-link sentence pointing at the
  Task 3.1.1 repro comments.
- **Failure mode this catches**: a "bare number bump" — e.g. someone widening a timeout
  or adding `-p 1` in a future PR without an adjacent rationale comment, which AC #2
  explicitly disallows.

---

## `coverage_gate_should_PreserveRaceScopeAndThreshold_When_P1FlagIsAdded` (AC #3)

Adapted from `flaky-hook-url-tests/implementation/validation.md`'s "Task 2.1.1
Verification" section.

```bash
# Baseline (no -p 1):
TMUX_BIN="$(pwd)/bin/tmux" go test -race -coverprofile=/tmp/cov-before.out \
  -covermode=atomic ./server/... ./session/... ./config/...
go tool cover -func=/tmp/cov-before.out | tail -1

# With -p 1:
TMUX_BIN="$(pwd)/bin/tmux" go test -race -p 1 -coverprofile=/tmp/cov-after.out \
  -covermode=atomic ./server/... ./session/... ./config/...
go tool cover -func=/tmp/cov-after.out | tail -1
```

- **Expected**: both runs cover the identical package scope (`./server/...
  ./session/... ./config/...`) with `-race` active in both, `coverage.out` in both cases
  is a single well-formed `mode: atomic` profile, and the total percentage reported by
  `go tool cover -func` does not meaningfully differ between the two runs (confirming
  `-p 1` changes *concurrency*, not *what* is covered) — and neither total drops below
  the existing 60% global threshold gated by `vladopajic/go-test-coverage@v2`
  (`.github/workflows/build.yml:162-168`).
- **Expected (raw log)**: the three package binaries' `--- PASS`/`--- FAIL`/`ok`/`FAIL`
  summary lines appear sequentially, not interleaved, in the `-p 1` run's log —
  confirming serialization took effect.
- **Post-merge corroboration**: confirm on the actual next CI run that the "Coverage
  gate" step still passes at ≥60% and the "Run tests with coverage" step's exit code is
  0 (or the existing `-v ./...` diagnostic fallback triggers only on genuine failure,
  unchanged from today).
- **This is the concrete evidence for AC #3** ("if the fix narrows or removes `-race`
  coverage for any package, that trade-off is explicit and justified") — the answer
  here is that it does **not** narrow coverage at all (same package scope, same
  `-race`/`-covermode` flags, only `-p` changes), which is itself the justification: no
  narrowing occurred, so no separate trade-off write-up is owed beyond this
  verification's result being recorded in the shipping PR description.

---

## `hook_injection_latency_should_BeMeasuredViaJSONCapture_When_StressRunExecutedWithGotestsum` (AC #4)

This is the concrete procedure for Task 2.1.2b and directly closes acceptance criterion
4 — the one criterion that differs materially from the sibling project and is **not**
covered by the sibling's validation.md. "Measured, not assumed" requires an actual
captured-and-inspected number, not a task description promising one; this section is
that procedure, runnable as-is.

```bash
# Same artificial-contention setup as the AC #1 stress repro (Task 3.1.1's documented
# command), with -json output captured for latency inspection:
yes > /dev/null & yes > /dev/null & yes > /dev/null &

TMUX_BIN="$(pwd)/bin/tmux" gotestsum --jsonfile=/tmp/hookurl-latency.json \
  --format standard-verbose -- \
  -race -count=10 -run 'TestServer_should_Write.*HookURL' ./server/...

kill %1 %2 %3

# Inspect the slowest individual test runs from the captured JSON:
gotestsum tool slowest --jsonfile /tmp/hookurl-latency.json --threshold 500ms
```

**Zero-install fallback** (if `gotestsum` is unavailable and not worth installing for a
one-off measurement):
```bash
yes > /dev/null & yes > /dev/null & yes > /dev/null &
TMUX_BIN="$(pwd)/bin/tmux" go test -race -count=10 -json \
  -run 'TestServer_should_Write.*HookURL' ./server/... > /tmp/hookurl-latency.json
kill %1 %2 %3
# Manually filter "run"/"pass"/"fail" events with matching Test field and diff their
# Time timestamps, or pipe through `tparse -file=/tmp/hookurl-latency.json -smallscreen`.
```

- **Expected observable result**: `gotestsum tool slowest` (or manual `test2json`/`tparse`
  inspection) reports a per-repeat elapsed-time distribution for both
  `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` and
  `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`
  across the 10 repeats — a real distribution (e.g. "min 8s / median 14s / max 41s"),
  not a guess. Two possible outcomes, both actionable:
  1. **Confirms `research/stack.md` §4's static-reading conclusion** — the distribution's
     upper bound sits comfortably under the new 60s budget, and the elapsed time is
     dominated by tmux spin-up (not `InjectHookConfig`'s atomic write) — this is the
     expected, "no code change needed beyond Epic 1/2" outcome, and satisfies AC #4 by
     replacing "assumed generous headroom" with "measured headroom."
  2. **Surfaces an unexpected pipeline bottleneck** (e.g. `InjectHookConfig`'s write
     itself, not tmux spin-up, dominates elapsed time) — in this case AC #5's constraint
     activates: this finding is recorded as a new, separate investigation (a follow-up
     ticket), not silently folded into an `approval_handler.go` edit within this PR.
- **This is the artifact that distinguishes this validation.md from the sibling's**: the
  sibling's Task 2.1.2 verification measures `-p 1`'s wall-clock *cost* to the CI job
  (a different question); this section measures the hook-injection *pipeline's own
  latency* under load, which is what AC #4 specifically requires and what
  `research/build-vs-buy.md` (this project) recommended `gotestsum`/`-json` for.

---

## `approval_handler_diff_should_BeEmpty_When_PRIsShipped_UnlessLatencyMeasurementJustifiesChange` (AC #5)

Manual review gate, conditioned on the AC #4 measurement's outcome:

```bash
git diff --stat origin/main -- server/services/approval_handler.go session/services/
```

- **Expected (default case)**: empty diff — no lines changed in
  `server/services/approval_handler.go` or any file under `session/services/` (the
  hook-injection path). This is the expected outcome per `research/stack.md`'s
  static-reading conclusion that `InjectHookConfig`'s atomic write (temp file +
  `os.Rename`) is not the bottleneck.
- **Expected (only if AC #4's measurement surfaces a genuine pipeline bottleneck)**: a
  non-empty diff is acceptable **only if** the shipping PR description explicitly cites
  the AC #4 measurement's result (the specific elapsed-time evidence) as the
  justification, per AC #5's own wording ("unless the investigation shows the pipeline
  itself ... is the bottleneck"). A non-empty diff with no such citation is a finding
  this verification is designed to catch — it would mean a behavior change slipped in
  without the measurement-first gate AC #4 requires.
- **Failure mode this catches**: "opportunistic" tidying/refactoring of
  `approval_handler.go` bundled into this CI-reliability PR without the measured
  justification AC #5 demands — exactly the kind of undocumented scope creep the
  Risk Control section's "standard PR revert, no feature flag needed" framing assumes
  does not happen.

---

## Coverage Targets and How to Measure

This change touches **zero production code** — Epic 1 (`server/server_integration_test.go`,
a test file) and Epic 2 (`.github/workflows/build.yml`, a CI config file) are the only
files modified, per plan.md's Risk Control section ("no edits to `approval_handler.go`,
`session_service.go`, or `session/tmux/tmux.go`"). **No new coverage obligation is
introduced by this project** — there is no new function, branch, or code path requiring
a new test to reach the existing 60% global threshold.

The one thing that **must not regress** is the existing gate itself:

- **Existing gate (unchanged by this ticket)**: 60% global threshold, `local-threshold: 0`,
  enforced by `vladopajic/go-test-coverage@v2` against `coverage.out`
  (`.github/workflows/build.yml:162-168`).
- **Why Task 2.1.1's `-p 1` doesn't affect it**: `-p 1` only changes how many package
  build/test binaries run concurrently (`GOMAXPROCS`-bound parallelism across
  `./server/... ./session/... ./config/...`); it is the same underlying `go test`
  invocation, same `-coverprofile`/`-covermode=atomic` flags, same package scope,
  same test binary — just serialized rather than parallelized. There is no mechanism by
  which serializing package binaries changes which lines execute or which coverage
  profile entries are recorded; this is directly confirmed by the
  `coverage_gate_should_PreserveRaceScopeAndThreshold_When_P1FlagIsAdded` verification
  above (before/after `go tool cover -func` totals, expected to match).
- **How to measure locally**:
  ```bash
  make build   # generates protos; required before go test per CLAUDE.md
  TMUX_BIN="$(pwd)/bin/tmux" go test -race -p 1 -coverprofile=coverage.out \
    -covermode=atomic ./server/... ./session/... ./config/...
  go tool cover -func=coverage.out | tail -1
  ```
- **This ticket's coverage delta**: expected to be exactly zero on the numerator (no new
  covered/uncovered lines — no production code changed) and the denominator (no new
  lines added to the codebase). The only regression risk is the *mechanical* one Task
  2.1.1 checks for (verified above), not a *content* one.

---

## What Cannot Be Verified Pre-Merge

*(Adapted from the sibling's identical section — the same structural gap applies here,
plus one addition specific to this project's AC #4.)*

- **Zero/near-zero flakes across 20 consecutive CI runs** (AC #1) is inherently a
  post-merge, observational verification — it requires the fix to be merged and running
  in real CI over time. Pre-merge, the best available proxy is the local stress-run
  (`-count=10` under artificial `yes`-loop contention, above) plus the reasoning already
  captured in `flaky-hook-url-tests/decisions/ADR-001-p1-flag-over-isolated-invocation.md`
  and this project's `research/stack.md`.
- **Actual GitHub Actions runner wall-clock delta** for `-p 1` (as opposed to the local
  approximation) is flagged in plan.md's Unresolved Questions as "measure in CI, don't
  just trust the local estimate" — a known gap, to be corroborated on the first
  post-merge CI run.
- **Whether the AC #4 local/stress measurement's latency distribution matches actual CI
  runner behavior exactly**: the `gotestsum`/`-json` capture in this document is run
  locally (or on a throwaway CI dispatch) under *artificial* contention (`yes` loops)
  approximating, not reproducing, the exact `ubuntu-latest` 4-vCPU runner's real
  contention profile under a genuine concurrent full-suite `-race` run. This is the same
  "reasonably convincing local stress repro, not exact reproduction" bar the sibling's
  validation.md already accepts for AC #1; it applies identically to AC #4's measurement.
- **Whether GitHub Actions runner-level cross-*job* contention (not just cross-*package*
  contention within the `test` job) is a contributing factor**: resolved as "ruled out"
  by Task 2.2.1's read-only job-graph inspection (`build`/`install-check`/
  `web-build-smoke` run on separate VMs via `needs: prepare`; `benchmark-gate` runs
  strictly after `test`), but GitHub's account-level concurrent-job quota remains
  theoretically possible and unverifiable from this repo alone (plan.md's Unresolved
  Questions).

---

## Migration Test

N/A — no schema or data migration in this ticket (plan.md's own Migration Plan section:
"Omitted — no schema or data changes. This is a test-file and CI-workflow-only change.").
