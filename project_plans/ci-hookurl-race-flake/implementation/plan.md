# Implementation Plan: ci-hookurl-race-flake

**Date**: 2026-08-01
**Type**: CI reliability / test-infra fix (no new user-facing feature) — **adoption plan**
**Depends on**: `project_plans/ci-hookurl-race-flake/requirements.md`, `research/{stack,architecture,pitfalls,build-vs-buy}.md`

---

## Provenance

This backlog item (`ci-hookurl-race-flake`, requirements migrated from GitHub issue #192 /
backlog item `7271b842-7139-46ef-8c99-8083107b4d97`) and an earlier SDD project,
**`project_plans/flaky-hook-url-tests/`**, independently target the **identical bug**: the
same two tests (`TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`,
`TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`
in `server/server_integration_test.go`), the same root cause (`go test -race` cross-package
CPU contention on a 4-vCPU `ubuntu-latest` runner, compounded by a stale 30s-vs-60s
timeout comment), and the same "Race A vs. Race B" distinction.

All four of this project's own research passes (`research/stack.md`, `research/architecture.md`,
`research/pitfalls.md`, `research/build-vs-buy.md`) independently discovered
`flaky-hook-url-tests` mid-research and recommended **not** re-deriving a second, parallel
plan. `flaky-hook-url-tests` already has:

- `requirements.md`
- `research/{stack,features,architecture,pitfalls,build-vs-buy,ux}.md`
- `decisions/ADR-001-p1-flag-over-isolated-invocation.md` (Status: Proposed — never applied)
- `implementation/plan.md` (full Epic/Story/Task breakdown)
- `implementation/pre-mortem.md` (5 failure modes)
- `implementation/adversarial-review.md` (verdict: CONCERNS, no blockers)
- `implementation/architecture-review.md` (verdict: CONCERNS, no blockers)
- `implementation/validation.md` (requirement→test mapping, stress-repro procedure,
  averaged wall-clock measurement methodology)

**None of it has been implemented.** Verified directly against the current tree as part of
writing this plan (2026-08-01):

- `server/server_integration_test.go:336,338,407,409` still pass `30*time.Second` to
  `waitForLiveInstance`/`waitForPermissionRequestHookCommand` — Task 1.1.1 (bump the hook-wait
  call sites to 60s) is unapplied.
- None of the four poll helpers use `require.Eventually`/`testutil/wait.WaitForCondition` yet —
  all four are still hand-rolled `for`/`time.Sleep` loops (Tasks 1.2.1–1.2.4 unapplied).
- `.github/workflows/build.yml:155-157` still reads
  `go test -race -coverprofile=coverage.out -covermode=atomic ./server/... ./session/... ./config/...`
  with no `-p 1` flag (Task 2.1.1 / ADR-001 unapplied).
- ADR-001's `## Status` field still reads `Proposed`.

**This plan is a consolidation, not a re-plan.** Rather than duplicate the planning effort
that already produced a reviewed, pre-mortemed design, this plan **adopts
`flaky-hook-url-tests`'s implementation plan and ADR-001 wholesale**, reproducing the full
task breakdown below (with attribution) so this document is self-contained and directly
executable without a reader needing to open the other project's directory. **No new ADR is
written here** — this plan cites and reuses
`project_plans/flaky-hook-url-tests/decisions/ADR-001-p1-flag-over-isolated-invocation.md`
by reference; do not create a competing `ADR-001` under this project's `decisions/`.

### Line-reference verification (2026-08-01)

Every file:line reference in the adopted plan/ADR was re-verified against the current tree
before being reproduced below. **All of them are still accurate** — no drift found:

| Reference | Adopted plan says | Verified current value |
|---|---|---|
| `waitForResolvedAddr` def | `server/server_integration_test.go:424-437` | Confirmed: func at line 424, closing brace line 437 |
| `waitForLiveInstance` def | `:441-452` | Confirmed: func at line 441, closing brace line 452 |
| `waitForLiveInstance` call sites | `:336, 407` | Confirmed |
| `waitForPermissionRequestHookCommand` doc comment | `:454-462` ("callers pass 60s") | Confirmed: comment spans 454-462, stale claim still present |
| `waitForPermissionRequestHookCommand` def | `:463-501` (signature at 463, `func` body through line 501) | Confirmed |
| `waitForPermissionRequestHookCommand` call sites | `:338, 409` | Confirmed, still `30*time.Second` at both |
| `waitForTmuxTeardown` comment + def | `:503-540` (comment 503-526, `func` signature 527, closing brace 540) | Confirmed |
| `waitForTmuxTeardown` call sites (in `t.Cleanup`) | `:332, 404` | Confirmed |
| `installFakeClaudeBinary` | `:36-60` (doc comment 36-51, func 52-60) | Confirmed |
| Two test function signatures | `:281` and `:373` | Confirmed |
| `.github/workflows/build.yml` gating step | `:153-157` (step name `:153`, command `:155-157`) | Confirmed, no `-p 1` |
| `.github/workflows/build.yml` "Integration coverage (advisory)" step | `:244-255` | Confirmed, `continue-on-error: true`, unchanged |
| `.github/workflows/build.yml` Coverage gate step | `:162-168` | Confirmed |
| `test` job has no `timeout-minutes` | — | Confirmed via `grep -n timeout-minutes .github/workflows/build.yml`: only `benchmark-gate` (line 365) sets one (30 min) |
| `build`/`install-check`/`benchmark-gate`/`web-build-smoke` all `needs: prepare`, not `needs: test` | — | Confirmed by reading each job header; `benchmark-gate` (line 362) is the one job that does depend on `test`, and it only runs on `main` push, not concurrently |

No corrections were needed. The task breakdown below reproduces the adopted plan's task
descriptions, sizes, and Given/When/Then criteria verbatim (with attribution), using these
re-verified line numbers.

### Adopted review feedback (non-blocking, already resolved or explicitly deferred)

Both `flaky-hook-url-tests/implementation/adversarial-review.md` and `architecture-review.md`
returned **CONCERNS, no blockers**. Two of their concerns are already closed by
`flaky-hook-url-tests/implementation/validation.md` (reproduced in this project's own
Phase 4 validation, not duplicated here):

- The adversarial review's concern that no task actually *runs* the before/after stress-repro
  is closed by `validation.md`'s "Stress / Flake-Verification Test" section (baseline run via
  `git stash` + `-count=10` under artificial `yes`-loop contention, then the same run against
  patched code).
- The architecture review's concern that Task 2.1.2's single-sample wall-clock measurement is
  noisy is closed by `validation.md`'s "Task 2.1.2 Verification" section (≥3 runs each side,
  averaged/median delta, alternating order).

The remaining concerns (ADR-001 not naming `go tool covdata` as a considered-and-rejected
sub-option for Option 3; the "3-for-3 precedent" being comment-sourced rather than verified
against unsquashed git history; Tasks 1.2.2/1.2.3 touching helpers not implicated in the
reported flakiness) are accepted as-is per this plan's adoption stance — they do not change
the chosen approach (`-p 1` + `require.Eventually` + 60s budget) and are not re-litigated
here. If a future reviewer wants to act on them, they are documented in
`project_plans/flaky-hook-url-tests/implementation/adversarial-review.md` and
`architecture-review.md`.

---

## Step 1 — System Type

CI reliability / test-infrastructure bug fix. No new feature, no schema change, no new
production code path. Touches: one test file (`server/server_integration_test.go`), one CI
workflow file (`.github/workflows/build.yml`). *(Carried over verbatim from
`flaky-hook-url-tests/implementation/plan.md` Step 1.)*

---

## Step 2 — Domain Glossary

*(Reproduced in full from `flaky-hook-url-tests/implementation/plan.md` Step 2, attributed;
line numbers re-verified current as of 2026-08-01 — see table above.)*

| Term | Definition |
|---|---|
| **Race A** | The already-mitigated "early-exit hang": if the fake `claude` stand-in binary (`installFakeClaudeBinary`, `server/server_integration_test.go:36-60`) exits before tmux's readiness check observes the session as live, `CreateSession`'s async goroutine returns early — *before ever calling* `InjectHookConfig` — so the waiters time out no matter how long their budget is (observed failing at both 30s and 60s). Currently worked around by having the fake binary `sleep 60`. **Out of scope for this ticket** — do not touch. |
| **Race B** | This ticket's target: a genuinely eventually-succeeding wait (`InjectHookConfig` *does* run and *does* write the file) that is delayed past its budget by scheduling/CPU contention under `go test -race` + full-suite load, producing an intermittent (not deterministic) CI-only failure. Distinguishable from Race A by server logs: Race A logs `"[CreateSession] async start failed"`; Race B does not. |
| `waitForResolvedAddr` | Poll helper (`server/server_integration_test.go:424-437`) — polls `srv.GetAddr()` until it reports a real, non-zero bound address. 10s budget, not implicated in the reported flakiness, converted for consistency only. |
| `waitForLiveInstance` | Poll helper (`server/server_integration_test.go:441-452`) — polls `SessionService.FindLiveInstance` until the session created by `CreateSession` is registered in the live poller. 30s budget at both call sites (lines 336, 407); unchanged by this plan. |
| `waitForPermissionRequestHookCommand` | Poll helper (`server/server_integration_test.go:463-501`, doc comment `:454-462`) — polls `.claude/settings.local.json` until `InjectHookConfig`'s async write lands, then returns the `PermissionRequest` hook's command string. Its own doc comment (lines 458-462) already says "callers pass 60s," but both call sites (lines 338, 409) still pass `30*time.Second` — the stale-comment/never-applied-fix bug this plan fixes. |
| `waitForTmuxTeardown` | Cleanup-phase poll helper (`server/server_integration_test.go:527-540`, doc comment `:503-526`) — polls `inst.TmuxSessionExists()` after `t.Cleanup(DeleteSession)`, deliberately non-fatal (`t.Logf`, not `t.Fatalf`) because it runs after the test's own pass/fail verdict is decided. Documents the shared-tmux-socket root cause (see below). |
| `sessionCreateTimeout` | The 10s hard deadline inside `instance.Start(true)`'s own tmux-spin-up readiness-check loop (`session/tmux/tmux.go:182-183, 985-1040`). This is Race A's trigger surface — **this plan must not touch it or any poll interval inside that loop.** |
| **Gating coverage** | `coverage.out`, produced by the single `go test -race -coverprofile=coverage.out -covermode=atomic ./server/... ./session/... ./config/...` step (`.github/workflows/build.yml:153-157`) and consumed by the `vladopajic/go-test-coverage@v2` threshold gate (60% global, `:162-168`). Failing here blocks merge. |
| **Advisory coverage** | The separate `-tags integration` lane (`.github/workflows/build.yml:244-255`, "Integration coverage (advisory)") — runs with `continue-on-error: true`, writes `integration.out`, and is uploaded as an artifact but never fed to the coverage gate. Moving tests here would silently stop them from gating merges — explicitly rejected. |
| `testSocketOnce` | The process-scoped, PID-keyed `sync.OnceValue` tmux server socket singleton (`session/tmux.go:336`, gated by `config.IsTestMode()`) shared by every integration test in one `go test` binary. Root cause of the *separate*, already-documented "monotonically increasing teardown latency" finding (`waitForTmuxTeardown`'s comment) — not fully fixed by this plan's `-p 1` change (that's a cross-package fix; this is within-package/within-binary). Noted as a residual risk, not re-solved here. |
| `InjectHookConfig` | `server/services/approval_handler.go:672-792` — writes `.claude/settings.local.json` atomically (temp file + `os.Rename`, lines 785-788) after `instance.Start(true)` returns. Confirmed not the bottleneck (no torn-read window); the bottleneck is entirely the preceding tmux spin-up. |
| `require.Eventually` | `github.com/stretchr/testify/require` (already a direct dependency, `go.mod:31`, `v1.11.1`) polling assertion: `require.Eventually(t, func() bool {...}, timeout, pollInterval, msgAndArgs...)`. Already the established pattern for polling this exact same hook file elsewhere in the package (`server/services/approval_handler_integration_test.go`, 3 call sites) — `server_integration_test.go` is the only file in this area still hand-rolling `for/time.Sleep/t.Fatalf` loops. Verified (architecture-review.md) that `assert.Eventually`'s condition callback runs on a background goroutine, not synchronously — see Task 1.2.1's remediation note below for the resulting closure-safety requirement. |
| `-p 1` | `go test`'s flag controlling how many package build/test binaries run in parallel (default = `GOMAXPROCS` = `NumCPU` = 4 on the `ubuntu-latest` runner). Setting `-p 1` serializes `./server/...`, `./session/...`, `./config/...` package binaries instead of running up to 4 concurrently, freeing CPU for whichever package binary is currently running (including `-race`'s 2-20x tax). |

---

## Step 3 — Pattern Decisions

*(Reproduced in full from `flaky-hook-url-tests/implementation/plan.md` Step 3, attributed.)*

| Decision | Chosen | Rejected | Why rejected |
|---|---|---|---|
| Poll mechanism in `server_integration_test.go` | `require.Eventually` (matches sibling `approval_handler_integration_test.go` pattern; testify already a direct dep) | Hand-rolled `for time.Now().Before(deadline) { ...; time.Sleep(x) }` loop (status quo) | This file is the *only* holdout of this pattern in the area; build-vs-buy analysis (in `flaky-hook-url-tests/research/build-vs-buy.md`) found zero justification to keep it — no cycle risk, no missing capability `require.Eventually` lacks for this use case. **Scope note**: Tasks 1.2.2/1.2.3 touch `waitForLiveInstance`/`waitForResolvedAddr`, which are *not* implicated in the reported flakiness. These two tasks are pure mechanism swaps — no budget, poll-interval, or behavior change, only replacing hand-rolled loops with an equivalent library call. If in doubt, defer Tasks 1.2.2/1.2.3 and ship only 1.2.1/1.2.4 (the two helpers actually on Race B's critical path) without weakening the fix. |
| `waitForPermissionRequestHookCommand`'s timeout value | `60*time.Second` at both call sites (lines 338, 409), matching the helper's own doc comment (lines 458-462) | (a) Leave at 30s (status quo); (b) widen `waitForLiveInstance` too | (a) is the exact stale bug this ticket exists to fix — the comment already documents the justification, it was simply never applied. (b) is unjustified: no documented mismatch found for `waitForLiveInstance`'s budget specifically, and widening it too would grow the tests' runtime budget beyond what's "demonstrably needed." |
| CI contention mitigation mechanism | Add `-p 1` to the existing single gating invocation (`.github/workflows/build.yml:155-157`) | Isolate this file into a second, still-gating invocation with a merged `-coverprofile` (via `go tool covdata` or text concatenation) | Heavier lever than it looks: no `gocovmerge` dependency exists (violates "no new external dependencies"), profile-merge correctness for two `mode: atomic` outputs is unverified here, and it duplicates job-setup steps (tmux build/cache, Go setup) as an ongoing drift risk. `-p 1` achieves the same "less CPU contention" goal with a single flag, zero new invocations, and zero coverage-merge surface. See ADR-001 (cited below, not duplicated). |
| Where isolation/gating lives | Same single `go test -race -coverprofile=coverage.out` command, `-p 1` appended | Move to the `-tags integration` advisory lane (`build.yml:244-255`) | That lane is `continue-on-error: true` and does not feed `coverage.out`/the 60%-threshold gate — moving these tests there stops them from gating merges. |
| Determinism source for "hook injected" | Continue polling the file (`require.Eventually` wrapping the existing read-and-parse logic) | Add a new production `EventBus` "hook injected" signal | No such signal exists today; adding one is new production surface for a test-only need (`.claude/rules/interface-pollution-checklist.md`'s "speculative interface"/"forwarding-only wrapper" smells), and it still wouldn't remove `waitForLiveInstance`'s poll (no analogous "tmux session is live" event exists either) — so it only partially solves the problem at a disproportionate cost. |
| Scope boundary vs. Race A | Leave `sessionCreateTimeout` and all poll intervals inside `session/tmux/tmux.go`'s readiness-check loop untouched | Tune `sessionCreateTimeout` or the tmux-side poll interval "while we're in here" | That's Race A's territory (the already-mitigated early-exit hang), a causally-chained but *decoupled* failure mode from Race B. Touching it risks reintroducing/interacting with a race this ticket is not scoped to re-verify. |

**ADR reference**: the CI-topology decision (reject a second isolated `-race` invocation,
adopt `-p 1`) is fully recorded in
`project_plans/flaky-hook-url-tests/decisions/ADR-001-p1-flag-over-isolated-invocation.md`.
This plan cites it rather than re-deriving or duplicating it as a new ADR under
`project_plans/ci-hookurl-race-flake/decisions/`.

---

## Migration Plan

Omitted — no schema or data changes. This is a test-file and CI-workflow-only change.

---

## Observability Plan

*(Carried over from the adopted plan, attributed.)*

- **Primary observability artifact**: the stress-repro command added in Task 3.1.1, living
  directly in `server/server_integration_test.go` next to the tests it documents (not a
  separate doc file, to guarantee it's discovered by anyone editing these tests again) —
  `yes > /dev/null &`-based artificial CPU contention + `go test -race -count=10 -run
  'TestServer_should_Write.*HookURL' ./server/...`.
- **Secondary signal**: CI's own pass/fail history for the `Test` job on `main` and on PRs
  over the 20-run window named in requirements.md's Success Metrics — no new dashboard or
  alerting needed; GitHub Actions' existing run history is sufficient to confirm the
  near-zero-flake outcome.
- **What we are explicitly not adding**: no new metrics, no new EventBus event, no new
  logging statement in production code (`approval_handler.go`/`session_service.go`/`tmux.go`
  are untouched) — this stays a test-and-CI-config-only change per the Pattern Decisions
  table.
- **Addition specific to this project's own requirements** (acceptance criterion 4 — "measure,
  don't assume" the pipeline's actual latency under `-race` + load): `research/build-vs-buy.md`
  (this project) additionally recommends `go test -json` (or `gotestsum --jsonfile` +
  `gotestsum tool slowest`) for a one-off measurement pass of hook-injection pipeline latency,
  piped to `$GITHUB_STEP_SUMMARY` (a pattern this repo's `benchmark.yml` already establishes).
  This is a diagnostic-only addition to the adopted plan's Task 2.1.2 (which measures `-p 1`'s
  wall-clock *cost*, not pipeline *latency*) — it does not change any task's chosen approach,
  it closes acceptance criterion 4's "measured, not assumed" bar with existing, zero-dependency
  tooling. See Task 2.1.2b below.

---

## Risk Control

*(Carried over from the adopted plan, attributed.)*

- Standard PR revert. No feature flag needed — this is a test-file and CI-workflow-only
  change with no production code path affected (confirmed: no edits to
  `approval_handler.go`, `session_service.go`, or `session/tmux/tmux.go`).
- If `-p 1` (Task 2.1.1) causes the `Test` job to exceed CI's timeout budget (confirmed via
  `grep -n timeout-minutes .github/workflows/build.yml`: the `test` job has no
  `timeout-minutes` set, so GitHub Actions' default 360-minute limit applies — effectively not
  a concern given the measured delta in Task 2.1.2 is expected to be low-single-digit
  minutes), revert Task 2.1.1 independently of Epic 1 — the two epics are decoupled and either
  can ship without the other.
- If widening to 60s (Task 1.1.1) still shows residual flakiness after the 20-run observation
  window, the next lever (not this ticket's scope) would be investigating the shared-tmux-socket
  / `testSocketOnce` root cause already documented in `waitForTmuxTeardown`'s comment — file
  that as a separate ticket.
- **Pre-registered presumptive cause of recurrence** (pre-mortem P1, Task 3.1.3 below): any
  post-merge recurrence of `TestServer_should_Write*HookURL` flakiness should first be checked
  against the already-known, explicitly-out-of-scope `testSocketOnce` shared-tmux-socket
  mechanism before reflexively re-tuning timeouts or CI topology again.

---

## Unresolved Questions

*(Carried over from the adopted plan, attributed, plus one item specific to this project.)*

- **Exact wall-clock cost of `-p 1`** on the actual GitHub Actions `ubuntu-latest` runner (as
  opposed to a local approximation) is not knowable until Task 2.1.2's local measurement is
  corroborated by an actual CI run post-merge — flagged as a "measure in CI, don't just trust
  the local estimate" follow-up, not a blocker to shipping.
- **Whether 60s (vs. some smaller, adaptively-computed value) is the *right* number** for
  `waitForPermissionRequestHookCommand`, independent of `-p 1`'s mitigation, is not
  conclusively provable without exact CI contention data — 60s is chosen because it is what
  the code's own pre-existing doc comment already committed to, the cheapest defensible
  number, and consistent with the "tune the wait" historical precedent; it is not derived from
  a controlled experiment.
- **Whether other jobs in `build.yml` contribute cross-job contention**: resolved — `build`,
  `install-check`, `benchmark-gate`, `web-build-smoke` all declare `needs: prepare`, not
  `needs: test` (re-verified above), so they run on separate GitHub-hosted VMs concurrently
  with `test` but do not share `test`'s 4 vCPUs. Account-level concurrent-job-quota contention
  (a GitHub org/billing-level resource) remains theoretically possible but is unverifiable
  from this repo alone.
- **Whether larger/paid GitHub-hosted runners are a viable alternative lever**: resolved by
  this project's own `research/build-vs-buy.md` — larger runner tiers are gated behind
  GitHub Team/Enterprise Cloud billing this repo (`tstapler/stapler-squad`, a personal
  account) does not have; self-hosting a bigger runner is technically possible but
  disproportionate (new maintenance/security surface for a test-timeout flake). Not pursued.
- **Divergent recommendation in this project's own `research/build-vs-buy.md`**: that research
  pass, evaluated independently of the adopted plan, recommends applying the repo's existing
  `//go:build integration` tag or a `testing.Short()` guard to isolate the two flaky tests from
  the gating `-race` run entirely, rather than `-p 1`. This is **not adopted** — moving these
  tests to a build-tagged or `-short`-skipped tier would stop them from running (and being
  covered) in the default gating invocation, which ADR-001 and this project's own
  `pitfalls.md`/`architecture.md` treat as an explicit non-goal ("must not weaken `-race`
  coverage for `./server/... ./session/... ./config/...`"). Flagged here for transparency
  rather than silently dropped: if a future reviewer prefers the build-tag/`-short` approach,
  it should be argued explicitly against ADR-001's reasoning (specifically, that isolating via
  build tag changes *what* is covered by the gating run, not just *how* concurrently it runs —
  a different trade-off than `-p 1`'s "same coverage, serialized" trade-off), not silently
  substituted.

---

## Dependency Visualization

```
Epic 1 (test-side waits)                Epic 2 (CI-side mitigation)         Epic 3 (observability)
─────────────────────────                ──────────────────────────         ───────────────────────
Task 1.1.1 (30s→60s)  ──┐
                        │
Task 1.2.1 (require.    │  (independent — different files,
  Eventually on hook     │   can ship in either order or
  helper)                │   in parallel)
                        │
Task 1.2.2 (require.    │                Task 2.1.1 (-p 1)  ──── Task 2.1.2 (measure
  Eventually on live     │                                        -p 1 wall-clock cost)
  instance helper)       │                                              │
                        │                Task 2.2.1 (inspect              │
Task 1.2.3 (require.    │                  job graph for                  │
  Eventually on addr     │                  concurrency —                 │
  helper)                │                  read-only)                    │
                        │                                              │
Task 1.2.4 (wait.        │                                              ▼
  WaitForCondition on    │                                        (feeds PR description
  teardown helper,       │                                         trade-off note)
  non-fatal)             │
                        │                                        Task 3.1.1 (stress-repro
All of Epic 1 is         │                                          comment)
independent of Epic 2 ───┘                                              │
(different files:                                                       ▼
server_integration_test.go                                        Task 3.1.2 (cross-link
  vs. build.yml) — either                                            repro from doc comment)
can ship, revert, or be
measured without the other.                                       Task 3.1.3 (pre-register
                                                                       testSocketOnce as
                                                                       presumptive cause of
                                                                       recurrence — P1 pre-
                                                                       mortem fix, ship before
                                                                       or with the PR)

Additional (this project's own acceptance criterion 4, not in the adopted plan):
Task 2.1.2b (gotestsum/`go test -json` pipeline-latency measurement) — independent
  diagnostic, feeds validation.md evidence only, no code change.
```

**Critical path for merge**: Task 1.1.1 + Task 1.2.1 (Epic 1's minimum fix for the reported
symptom) and Task 2.1.1 (Epic 2's CI mitigation) are the two load-bearing tasks; everything
else (1.2.2-1.2.4, 2.1.2, 2.2.1, all of Epic 3, 2.1.2b) is either consistency cleanup,
measurement/evidence-gathering, or documentation, and can be reviewed/merged in the same PR
without blocking on sequencing relative to each other.

---

## Step 4 — Task Breakdown

*(Reproduced in full from `flaky-hook-url-tests/implementation/plan.md` Step 4, attributed;
task numbering and file:line references unchanged and re-verified current as of 2026-08-01.)*

### Epic 1: Deterministic, correctly-budgeted test-side waits

#### Story 1.1: Fix the stale 30s→60s call-site bug

**Task 1.1.1** — Widen `waitForPermissionRequestHookCommand`'s two call-site timeouts from `30*time.Second` to `60*time.Second`.
- Files: `server/server_integration_test.go` (lines 338, 409)
- Size: 2 min
- Detail: Change `hookCmd := waitForPermissionRequestHookCommand(t, settingsPath, 30*time.Second)` → `... 60*time.Second)` at both call sites. Update the trailing inline comment at each call site (if any references "30s") to match. Do not touch the helper's own doc comment (lines 458-462) — it already states the correct intent; this task makes the code match the comment, not vice versa.
- **Given/When/Then**: Given `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` runs under `go test -race -count=5 -run TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort ./server/...` on a CI runner where `InjectHookConfig`'s write completes at, say, 42s under heavy `-race` contention (a value that would have failed the old 30s budget), when `waitForPermissionRequestHookCommand(t, settingsPath, 60*time.Second)` polls, then the test passes because 42s < 60s, whereas before this task it would have failed at the 30s deadline with `"expected .../.claude/settings.local.json to contain a PermissionRequest command hook within 30s"`.

#### Story 1.2: Convert hand-rolled poll loops to `require.Eventually`

**Task 1.2.1** — Add the `testify/require` import and convert `waitForPermissionRequestHookCommand` to `require.Eventually`.
- Files: `server/server_integration_test.go` (import block lines 3-18; function body 463-501, doc comment 454-462)
- Size: 5 min
- Detail: Add `"github.com/stretchr/testify/require"` to the import block. Rewrite the function body to capture the resolved `hookCmd` string in a closure variable and assert with `require.Eventually(t, func() bool { ... return hookCmd != "" }, timeout, 50*time.Millisecond, "expected %s to contain a PermissionRequest command hook", settingsPath)`, preserving the exact same JSON-parsing logic (read file → unmarshal `hooks.PermissionRequest` → find first `type=="command"` entry) and the 50ms poll interval already used today. Return `hookCmd` after the `require.Eventually` call succeeds (it fails the test via `t.FailNow()` internally on timeout, matching the current `t.Fatalf` semantics). **Closure-safety requirement** (from architecture-review.md's verified check against testify v1.11.1 source — `assert.Eventually`'s condition callback runs on a background goroutine, not synchronously): the closure must assign to `hookCmd` only in the `h.Type == "command" && h.Command != ""` success branch, immediately before `return true`; every other return path (`false`) must leave `hookCmd` untouched — this is what keeps the happens-before chain (write → channel send → channel receive → caller's `return hookCmd`) both race-safe and free of the "stale/partial value" ambiguity a naive unconditional-reassignment implementation could introduce.
- **Given/When/Then**:
  - Given the file `settingsPath` never gets written (simulating a real hang, i.e. Race A), when `require.Eventually` exhausts its `60*time.Second` budget, then the test fails with testify's standard `"Condition never satisfied"` message plus the custom message `"expected <path> to contain a PermissionRequest command hook"` — equivalent fail-fast behavior to today's `t.Fatalf`, just testify's message format instead of the hand-rolled one.
  - Given the file has `hooks.PermissionRequest` present but no `type=="command"` entry on the first 2 polls, and a valid entry appears on poll 3, when the closure runs across those 3 polls, then the returned `hookCmd` is exactly the poll-3 value, never `""` or a stale intermediate from a failed earlier poll.

**Task 1.2.2** — Convert `waitForLiveInstance` to `require.Eventually`.
- Files: `server/server_integration_test.go` (lines 441-452)
- Size: 3 min
- Detail: Same pattern: capture `inst` in a closure, `require.Eventually(t, func() bool { inst = deps.SessionService.FindLiveInstance(sessionID); return inst != nil }, timeout, 20*time.Millisecond, "expected session %q to appear in the live poller", sessionID)`, preserving the existing 20ms poll interval and unchanged 30s call-site budgets (lines 336, 407 — not modified by this task, only 1.1.1 touches a timeout value).
- **Given/When/Then**: Given `FindLiveInstance(sessionID)` returns non-nil after 12s of tmux spin-up (well under the unchanged 30s budget), when `waitForLiveInstance` is called from either test, then it returns the same `*session.Instance` pointer it would have under the old hand-rolled loop, with no behavior change beyond the polling mechanism.

**Task 1.2.3** — Convert `waitForResolvedAddr` to `require.Eventually`.
- Files: `server/server_integration_test.go` (lines 424-437)
- Size: 2 min
- Detail: Same pattern, preserving the 10s timeout and 10ms poll interval and the exact non-zero/non-`:0` address check.
- **Given/When/Then**: Given `srv.GetAddr()` returns a real bound address on poll N (N such that elapsed time is under the 10s budget), when `waitForResolvedAddr` is called, then it returns that address unchanged from the old hand-rolled-loop behavior.

**Task 1.2.4** — Convert `waitForTmuxTeardown` to a `require.Eventually`-*equivalent* that stays non-fatal.
- Files: `server/server_integration_test.go` (lines 503-540)
- Size: 3 min
- Detail: **Do not use `require.Eventually` here directly** — `require.Eventually` calls `t.FailNow()`/`t.Errorf()` internally on timeout, which would break this helper's documented deliberate non-fatal behavior (it runs inside `t.Cleanup`, after the test's own pass/fail verdict is already decided; see the comment at lines 522-526). Instead, replace the hand-rolled loop with the already-available `testutil/wait.WaitForCondition` (`testutil/wait/wait.go`, purpose-built for "condition polling with a timeout" and — per its own package doc — for exactly this "can't import top-level `testutil` due to an import cycle with `session`" situation) and log via `t.Logf` on its returned error instead of failing. `server` already imports `session`, and `testutil/wait` imports neither `server` nor `session`, only `context`/`fmt`/`time` — confirmed no cycle (also independently re-confirmed in architecture-review.md against the actual `testutil/wait/wait.go` source: `WaitForCondition` runs its condition synchronously in the calling goroutine, unlike `assert.Eventually`'s background goroutine, and never calls `t.FailNow()` — the right primitive for this deliberately-non-fatal case). Use `wait.WaitForCondition(func() bool { return !inst.TmuxSessionExists() }, wait.WaitConfig{Timeout: timeout, PollInterval: 20 * time.Millisecond, Description: fmt.Sprintf("tmux teardown for %q", inst.Title)})` and `t.Logf` its error.
- **Given/When/Then**: Given `DeleteSession`'s teardown goroutine has not finished tearing down tmux by the time `t.Cleanup` runs (a slow-but-eventually-successful teardown), when `wait.WaitForCondition` exhausts its 5s budget, then `t.Logf("tmux session for %q still reported alive %s after DeleteSession; teardown may still be in flight", inst.Title, timeout)` is logged and the *already-decided* test result (pass) is unaffected — identical to today's `t.Logf`-only behavior, confirmed by re-running `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` and asserting it still reports `PASS` even when a synthetic 6-second sleep is injected before `TmuxSessionExists()` returns false in a throwaway local repro (procedure spelled out in `flaky-hook-url-tests/implementation/validation.md`, "Task 1.2.4 Verification").

### Epic 2: CI-side contention mitigation

#### Story 2.1: Reduce cross-package `-race` CPU contention on the gating job

**Task 2.1.1** — Add `-p 1` to the gating `go test -race` invocation.
- Files: `.github/workflows/build.yml` (lines 153-157, step "Run tests with coverage (pinned tmux 3.4)")
- Size: 2 min
- Detail: Change:
  ```
  TMUX_BIN="$(pwd)/bin/tmux" go test -race -coverprofile=coverage.out \
    -covermode=atomic ./server/... ./session/... ./config/... \
    || (TMUX_BIN="$(pwd)/bin/tmux" go test -race -v ./...; exit 1)
  ```
  to:
  ```
  TMUX_BIN="$(pwd)/bin/tmux" go test -race -p 1 -coverprofile=coverage.out \
    -covermode=atomic ./server/... ./session/... ./config/... \
    || (TMUX_BIN="$(pwd)/bin/tmux" go test -race -v ./...; exit 1)
  ```
  Leave the `-v ./...` fallback diagnostic re-run (which only fires on failure, for verbose output) unchanged — it is not the gating step and adding `-p 1` there is unnecessary for a diagnostic-only path that already only runs after the primary command failed. Also add an inline comment directly above the modified line (pre-mortem failure #3, see Task 3.1.3-adjacent guidance): `# -p 1: serializes package binaries to reduce -race CPU contention that caused intermittent hook/MCP-URL test timeouts — see project_plans/flaky-hook-url-tests/decisions/ADR-001. Do not remove without re-reading that ADR and re-verifying flake rate.`
- **Given/When/Then**: Given the `Test` job today runs `./server/...`, `./session/...`, `./config/...` as up to 3 concurrent package binaries competing for 4 vCPUs, when `-p 1` is added, then GitHub Actions logs for the step show the three package binaries' `--- PASS/FAIL` blocks appearing sequentially (not interleaved) in the raw log, and `coverage.out` is still produced by the same single command with the same `-covermode=atomic` — verified by diffing the coverage gate's reported total percentage across two CI runs (before/after) and confirming it does not drop below the existing 60% global threshold.

**Task 2.1.2** — Measure and record the wall-clock cost of `-p 1` before merging.
- Files: none (verification-only; record finding in `project_plans/ci-hookurl-race-flake/implementation/validation.md`, to be created in this project's own Phase 4)
- Size: 5 min
- Detail: Run the gating command locally **at least 3 times each way** (not a single before/after sample — architecture-review.md flagged single-sample A/B as noisy due to Go build-cache warming and thermal/scheduling variance), alternating order, and record the median wall-clock delta. See `flaky-hook-url-tests/implementation/validation.md`'s "Task 2.1.2 Verification" section for the exact averaged-measurement script (3 runs baseline, 3 runs with `-p 1`, `/usr/bin/time -p`, median delta). This is the evidence backing the "no increase in the tests' own runtime budget beyond what's demonstrably needed" success metric being satisfied at the *test* level (Epic 1) while being honest that Epic 2's CI-level wall-clock cost is a separate, acknowledged trade-off, not hidden.
- **Given/When/Then**: Given 3 pre-change and 3 post-`-p 1` local runs, when the median of each set is computed, then the delta (e.g. +1m50s) is written into the PR description as an explicit, accepted trade-off — not silently absorbed.

**Task 2.1.2b** *(new — closes this project's own acceptance criterion 4, not present in the adopted plan)* — Capture a one-off hook-injection pipeline latency measurement under `-race` + contention.
- Files: none (diagnostic-only; record finding in `project_plans/ci-hookurl-race-flake/implementation/validation.md`)
- Size: 10 min
- Detail: Per this project's own `research/build-vs-buy.md` §4, run one local (or throwaway CI) invocation of the affected tests with `go test -json` output captured, optionally via `gotestsum --jsonfile=<path>` + `gotestsum tool slowest --jsonfile <path> --threshold 500ms`, under the same artificial-contention stress-repro conditions as Task 3.1.1's documented command (`yes > /dev/null &` ×3 + `-count=10`). This produces the "measured, not assumed" latency evidence acceptance criterion 4 requires, distinguishing "the wait budget is marginal under contention" (confirmed elsewhere in this plan) from "the hook-injection pipeline itself has a latent slowdown" (not expected, per `research/stack.md` §4's static-reading conclusion that `InjectHookConfig`'s atomic write is not the bottleneck — this task provides the dynamic confirmation of that static finding). No code change results from this task unless the measurement surfaces an unexpected pipeline bottleneck, in which case acceptance criterion 5's constraint applies (no behavior change to `approval_handler.go`/hook-injection path unless research shows it's the actual bottleneck).
- **Given/When/Then**: Given the stress-repro command runs with `-json`/`gotestsum` capture enabled, when `gotestsum tool slowest` (or manual `test2json` inspection) reports the elapsed time for `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`/`TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated` across the 10 repeats, then that distribution (not a guess) is recorded in `validation.md` as the evidence for "the 60s budget has more than headroom for a directly measured worst-case tmux-spin-up-plus-write time," or, if the measurement instead shows `InjectHookConfig`'s own write taking non-trivial time, that finding is flagged as a new, separate investigation rather than silently folded into this ticket's scope.

### Epic 2 (cont.): GitHub Actions runner/job concurrency check

**Task 2.2.1** — Inspect `build.yml`'s job graph for concurrent-job contention, as far as observable from the YAML alone.
- Files: `.github/workflows/build.yml` (whole-file `needs:`/`concurrency:` graph — read-only inspection, no file edit expected unless a concrete finding warrants one)
- Size: 3 min
- Detail: Confirm (a) whether the `test` job runs concurrently with other jobs in the same workflow run, and (b) whether any workflow-level `concurrency:` group serializes runs in a way relevant here. **Already answered by this project's own research** (`research/architecture.md` §3, `research/stack.md` §2, re-verified directly in this plan's Provenance section above): `build`, `install-check`, `benchmark-gate`, `web-build-smoke` all declare `needs: prepare` (not `needs: test`) — except `benchmark-gate`, which declares `needs: [prepare, test]` and only runs on `main` push, so it is serialized *after* `test`, not concurrent with it. This task is effectively closed by that research; retain it in the breakdown only as a checklist item confirming the finding is carried into the shipping PR description, not as unfinished investigative work.
- **Given/When/Then**: Given `build.yml`'s job headers (re-verified: `prepare` at line 47, `web-build-smoke` at 78, `test` at 119, `build` at 270, `install-check` at 323, `benchmark-gate` at 362), when this task inspects the job graph, then it confirms `build`/`install-check`/`web-build-smoke` run concurrently with `test` on separate GitHub-hosted VMs (not competing for `test`'s own 4 vCPUs) and `benchmark-gate` runs strictly after `test` — ruling out same-VM cross-job contention as a factor, while noting that GitHub's account-level concurrent-job quota (a separate, org-level resource this item cannot inspect or change) remains a theoretically possible but unverifiable residual contributor.

### Epic 3: Regression check / observability artifact

#### Story 3.1: Document a reusable stress-repro command

**Task 3.1.1** — Add a local stress-repro command as a code comment above the two affected tests.
- Files: `server/server_integration_test.go` (immediately above `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated` at line 281, and above `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` at line 373)
- Size: 3 min
- Detail: Add a short comment block (reusing the existing doc-comment convention already used throughout this file) documenting the exact repro command a future flake investigation can copy-paste, using only shell builtins (no new external dependency):
  ```go
  // Flake repro: to reproduce Race B (marginal-timeout-under-load, see
  // project_plans/flaky-hook-url-tests/requirements.md and
  // project_plans/ci-hookurl-race-flake/requirements.md) locally with
  // artificial CPU contention approximating CI's 4-vCPU runner under full
  // `-race` load:
  //
  //   yes > /dev/null & yes > /dev/null & yes > /dev/null &
  //   TMUX_BIN="$(pwd)/bin/tmux" go test -race -count=10 \
  //     -run 'TestServer_should_Write.*HookURL' ./server/...
  //   kill %1 %2 %3
  ```
  This gives a future investigator both the correctness stress-run (`-count=10`, matching the precedent cited in `waitForTmuxTeardown`'s own comment) and an artificial-load variant, without adding a Makefile target, script, or dependency.
- **Given/When/Then**: Given a future engineer sees this test pair flake again in CI, when they copy-paste the comment's repro command into a local shell, then they can reproduce Race B's failure mode locally (elevated, contended, but eventually-successful completion) without needing CI access or guessing at a repro methodology from scratch.

**Task 3.1.2** — Cross-link the repro command from `waitForPermissionRequestHookCommand`'s doc comment.
- Files: `server/server_integration_test.go` (lines 454-462)
- Size: 2 min
- Detail: Append one sentence to the existing doc comment: "See the repro-command comments above the two call sites for a local stress-repro of the load-sensitive failure mode this budget accounts for." Do not rewrite the rest of the comment — it remains accurate after Task 1.1.1 (the call sites now do pass 60s, so "callers pass 60s" is no longer stale).

**Task 3.1.3** — Pre-register `testSocketOnce` as the presumptive cause of any post-merge recurrence (pre-mortem P1 fix — the one item both adopted reviews treat as address-before-implementation).
- Files: `server/server_integration_test.go` (comment near `waitForTmuxTeardown`, lines 503-526), shipping PR description (not a file, but a required step)
- Size: 2 min
- Detail: Because this plan explicitly does NOT fix the already-documented `testSocketOnce` shared-tmux-socket contention, a post-merge recurrence of `TestServer_should_Write*HookURL` flakiness could be misdiagnosed as "the 60s/`-p 1` fix didn't work" rather than "a different, already-known, out-of-scope mechanism is still live" — triggering a reflexive further timeout bump the file's own comments already warn against. Add one sentence next to `waitForTmuxTeardown`'s existing comment: "If `TestServer_should_Write*HookURL` flakes again after this fix, check whether it's this shared-tmux-socket contention before re-tuning timeouts or CI topology — file a follow-up ticket for that root cause instead of widening a budget further." Include the same sentence in the shipping PR description.
- **Given/When/Then**: Given a future CI run fails `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` after this PR merges, when the investigating engineer reads either the PR description or the comment near `waitForTmuxTeardown`, then they are pointed at `testSocketOnce` as the first thing to check (via server logs' absence of `"[CreateSession] async start failed"`, ruling out Race A, then via elevated CreateSession/DeleteSession cycle count in the same test binary) before assuming this PR's fix regressed.

---

## Summary of Task Counts

- **Epics**: 3
- **Stories**: 4 (1.1, 1.2, 2.1, 3.1) + 1 unnumbered read-only checklist item under Epic 2 (Task 2.2.1) + 1 project-specific addition (Task 2.1.2b) not organized under a numbered story
- **Tasks**: 11 total — 5 in Epic 1 (1.1.1, 1.2.1-1.2.4), 4 in Epic 2 (2.1.1, 2.1.2, 2.1.2b [new], 2.2.1), 3 in Epic 3 (3.1.1, 3.1.2, 3.1.3)
- **Glossary terms**: 13 (Race A, Race B, `waitForResolvedAddr`, `waitForLiveInstance`, `waitForPermissionRequestHookCommand`, `waitForTmuxTeardown`, `sessionCreateTimeout`, Gating coverage, Advisory coverage, `testSocketOnce`, `InjectHookConfig`, `require.Eventually`, `-p 1`)
