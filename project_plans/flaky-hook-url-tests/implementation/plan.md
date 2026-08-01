# Implementation Plan: flaky-hook-url-tests

**Date**: 2026-08-01
**Type**: CI reliability / test-infra fix (no new user-facing feature)
**Depends on**: `project_plans/flaky-hook-url-tests/requirements.md`, `research/{stack,features,architecture,pitfalls,build-vs-buy}.md`

---

## Step 0.5 — Creative Pass: Candidate Approaches

| Option | Description | Strength | Weakness |
|---|---|---|---|
| **A** | Fix the stale 30s-vs-60s comment/timeout mismatch at the two `waitForPermissionRequestHookCommand` call sites (`server/server_integration_test.go:338,409`) + convert all 4 hand-rolled poll loops to `require.Eventually`. No CI workflow change. | Zero new CI surface, zero risk, matches the repo's 3-for-3 historical precedent (build-vs-buy.md) of fixing this exact test pair via wait-tuning only. | Does not touch the diagnosed root cause (cross-package `-race` CPU contention on the 4-vCPU runner) — if runner load is the dominant factor, even 60s can still occasionally blow under a sufficiently loaded run. |
| **B** | A + add `-p 1` to the existing single gating `go test -race` invocation in `.github/workflows/build.yml:155-157` (same command, same `-coverprofile=coverage.out`, no new step, no new invocation). | Directly targets the researched root cause (stack.md: "`go test` runs up to `GOMAXPROCS`(=4) package binaries concurrently, each independently `-race`-taxed, competing for the same 4 cores") with a single flag, zero new dependencies, zero coverage-merge risk — `coverage.out` is produced by the exact same command as today, so the coverage gate and `-race` scope are provably unchanged. | Increases total CI wall-clock for the `Test` job (the 3 package trees now build/test sequentially instead of up to 4-way concurrently) — a real, measurable cost, not just a hypothetical one. |
| **C** | A + isolate `server_integration_test.go` (or the whole `server` package) into its own **second**, still-gating `go test -race` invocation, merging its `-coverprofile` into `coverage.out` before the coverage gate runs. | Would most completely decouple this file's `-race` run from the other 3 package trees' contention — the "purest" fix to the diagnosed contention. | Violates the spirit of "no new external dependencies" for safe profile merging (no `gocovmerge` dependency exists in `go.mod`; simple text concatenation of two `mode: atomic` profiles is merge-correctness-fragile and unverified here), duplicates job-setup steps (tmux build/cache, Go setup) as a drift risk, and — per pitfalls.md — is explicitly flagged as "a heavier, riskier lever than it looks." Stands 0-for-0 against a 3-for-3 precedent (build-vs-buy.md) of this exact test pair being fixed via narrower wait-tuning, never CI-topology changes. |
| **D** *(considered, not scored as a real contender)* | A + add a new production `EventBus` "hook injected" signal to replace polling entirely. | Would give a true happens-before instead of a poll. | Rejected outright: adds test-only production surface (violates `.claude/rules/interface-pollution-checklist.md`'s "speculative interface" / unjustified-new-abstraction smells), and doesn't even fully solve the problem — no equivalent "tmux session is live" event exists today for `waitForLiveInstance`'s wait, so polling would remain regardless (build-vs-buy.md, "Rejected" entry). |

**Chosen: B** (A + `-p 1`). Rationale: A alone is the correct floor (fixes a real, cheap, low-risk bug) but research explicitly names CPU contention on a 4-vCPU runner as a live contributing cause of Race B, and doing nothing about it leaves the fix incomplete. `-p 1` is the cheapest possible lever that targets that cause without any of C's merge-correctness or job-topology risk, and it's reversible with a one-line revert. C is rejected as disproportionate to the evidence (see ADR-001). D is rejected per the interface-pollution checklist and because it doesn't fully solve the problem it claims to.

This choice and the rejected alternatives are recorded in ADR-001 (`decisions/ADR-001-p1-flag-over-isolated-invocation.md`).

---

## Step 1 — System Type

CI reliability / test-infrastructure bug fix. No new feature, no schema change, no new production code path (the one candidate production change — a new EventBus signal — was evaluated and rejected in the Creative Pass). Touches: one test file (`server/server_integration_test.go`), one CI workflow file (`.github/workflows/build.yml`).

---

## Step 2 — Domain Glossary

| Term | Definition |
|---|---|
| **Race A** | The already-mitigated "early-exit hang": if the fake `claude` stand-in binary (`installFakeClaudeBinary`, `server/server_integration_test.go:36-60`) exits before tmux's readiness check observes the session as live, `CreateSession`'s async goroutine returns early — *before ever calling* `InjectHookConfig` — so the waiters time out no matter how long their budget is (observed failing at both 30s and 60s). Currently worked around by having the fake binary `sleep 60`. **Out of scope for this ticket** — do not touch. |
| **Race B** | This ticket's target: a genuinely eventually-succeeding wait (`InjectHookConfig` *does* run and *does* write the file) that is delayed past its budget by scheduling/CPU contention under `go test -race` + full-suite load, producing an intermittent (not deterministic) CI-only failure. Distinguishable from Race A by server logs: Race A logs `"[CreateSession] async start failed"`; Race B does not. |
| `waitForResolvedAddr` | Poll helper (`server/server_integration_test.go:424-437`) — polls `srv.GetAddr()` until it reports a real, non-zero bound address. 10s budget, not implicated in the reported flakiness, converted for consistency only. |
| `waitForLiveInstance` | Poll helper (`server/server_integration_test.go:441-452`) — polls `SessionService.FindLiveInstance` until the session created by `CreateSession` is registered in the live poller. 30s budget at both call sites (lines 336, 407); unchanged by this plan. |
| `waitForPermissionRequestHookCommand` | Poll helper (`server/server_integration_test.go:454-501`) — polls `.claude/settings.local.json` until `InjectHookConfig`'s async write lands, then returns the `PermissionRequest` hook's command string. Its own doc comment (lines 458-462) already says "callers pass 60s," but both call sites (lines 338, 409) still pass `30*time.Second` — the stale-comment/never-applied-fix bug this plan fixes. |
| `waitForTmuxTeardown` | Cleanup-phase poll helper (`server/server_integration_test.go:503-539`) — polls `inst.TmuxSessionExists()` after `t.Cleanup(DeleteSession)`, deliberately non-fatal (`t.Logf`, not `t.Fatalf`) because it runs after the test's own pass/fail verdict is decided. Documents the shared-tmux-socket root cause (see below). |
| `sessionCreateTimeout` | The 10s hard deadline inside `instance.Start(true)`'s own tmux-spin-up readiness-check loop (`session/tmux/tmux.go:182-183, 985-1040`). This is Race A's trigger surface — **this plan must not touch it or any poll interval inside that loop.** |
| **Gating coverage** | `coverage.out`, produced by the single `go test -race -coverprofile=coverage.out -covermode=atomic ./server/... ./session/... ./config/...` step (`.github/workflows/build.yml:153-157`) and consumed by the `vladopajic/go-test-coverage@v2` threshold gate (60% global). Failing here blocks merge. |
| **Advisory coverage** | The separate `-tags integration` lane (`.github/workflows/build.yml:244-255`, "Integration coverage (advisory)") — runs with `continue-on-error: true`, writes `integration.out`, and is uploaded as an artifact but never fed to the coverage gate. Moving tests here would silently stop them from gating merges — explicitly rejected (pitfalls.md). |
| `testSocketOnce` | The process-scoped, PID-keyed `sync.OnceValue` tmux server socket singleton (`session/tmux.go:336`, gated by `config.IsTestMode()`) shared by every integration test in one `go test` binary. Root cause of the *separate*, already-documented "monotonically increasing teardown latency" finding (`waitForTmuxTeardown`'s comment) — not fully fixed by this plan's `-p 1` change (that's a cross-package fix; this is within-package/within-binary). Noted as a residual risk, not re-solved here. |
| `InjectHookConfig` | `server/services/approval_handler.go:672-792` — writes `.claude/settings.local.json` atomically (temp file + `os.Rename`, lines 785-788) after `instance.Start(true)` returns. Confirmed not the bottleneck (no torn-read window); the bottleneck is entirely the preceding tmux spin-up. |
| `require.Eventually` | `github.com/stretchr/testify/require` (already a direct dependency, `go.mod:31`, `v1.11.1`) polling assertion: `require.Eventually(t, func() bool {...}, timeout, pollInterval, msgAndArgs...)`. Already the established pattern for polling this exact same hook file elsewhere in the package (`server/services/approval_handler_integration_test.go`, 3 call sites) — `server_integration_test.go` is the only file in this area still hand-rolling `for/time.Sleep/t.Fatalf` loops. |
| `-p 1` | `go test`'s flag controlling how many package build/test binaries run in parallel (default = `GOMAXPROCS` = `NumCPU` = 4 on the `ubuntu-latest` runner). Setting `-p 1` serializes `./server/...`, `./session/...`, `./config/...` package binaries instead of running up to 4 concurrently, freeing CPU for whichever package binary is currently running (including `-race`'s 2-20x tax). |

---

## Step 3 — Pattern Decisions

| Decision | Chosen | Rejected | Why rejected |
|---|---|---|---|
| Poll mechanism in `server_integration_test.go` | `require.Eventually` (matches sibling `approval_handler_integration_test.go` pattern; testify already a direct dep) | Hand-rolled `for time.Now().Before(deadline) { ...; time.Sleep(x) }` loop (status quo) | This file is the *only* holdout of this pattern in the area; build-vs-buy.md's build-vs-buy analysis found zero justification to keep it — no cycle risk, no missing capability `require.Eventually` lacks for this use case. |
| `waitForPermissionRequestHookCommand`'s timeout value | `60*time.Second` at both call sites (lines 338, 409), matching the helper's own doc comment (lines 458-462) | (a) Leave at 30s (status quo); (b) widen `waitForLiveInstance` too | (a) is the exact stale bug this ticket exists to fix — the comment already documents the justification, it was simply never applied. (b) is unjustified: research found no documented mismatch for `waitForLiveInstance`'s budget specifically, and widening it too would grow the tests' runtime budget beyond what's "demonstrably needed" (requirements.md Success Metrics). |
| CI contention mitigation mechanism | Add `-p 1` to the existing single gating invocation (`.github/workflows/build.yml:155-157`) | Isolate this file into a second, still-gating invocation with a merged `-coverprofile` (via `go tool covdata` or text concatenation) | Heavier lever than it looks (pitfalls.md): no `gocovmerge` dependency exists (violates "no new external dependencies"), profile-merge correctness for two `mode: atomic` outputs is unverified here, and it duplicates job-setup steps (tmux build/cache, Go setup) as an ongoing drift risk. `-p 1` achieves the same "less CPU contention" goal with a single flag, zero new invocations, and zero coverage-merge surface. See ADR-001. |
| Where isolation/gating lives | Same single `go test -race -coverprofile=coverage.out` command, `-p 1` appended | Move to the `-tags integration` advisory lane (`build.yml:244-255`) | That lane is `continue-on-error: true` and does not feed `coverage.out`/the 60%-threshold gate — moving these tests there stops them from gating merges, a real regression against the explicit constraint "must not weaken `-race` coverage for `./server/... ./session/... ./config/...`" (requirements.md). |
| Determinism source for "hook injected" | Continue polling the file (`require.Eventually` wrapping the existing read-and-parse logic) | Add a new production `EventBus` "hook injected" signal | No such signal exists today (architecture.md); adding one is new production surface for a test-only need (interface-pollution-checklist.md's "speculative interface"/"forwarding-only wrapper" smells), and it still wouldn't remove `waitForLiveInstance`'s poll (no analogous "tmux session is live" event exists either) — so it only partially solves the problem at a disproportionate cost. |
| Scope boundary vs. Race A | Leave `sessionCreateTimeout` and all poll intervals inside `session/tmux/tmux.go`'s readiness-check loop untouched | Tune `sessionCreateTimeout` or the tmux-side poll interval "while we're in here" | Pitfalls.md is explicit: that's Race A's territory (the already-mitigated early-exit hang), a causally-chained but *decoupled* failure mode from Race B. Touching it risks reintroducing/interacting with a race this ticket is not scoped to re-verify. |

---

## Step 4 — Task Breakdown

### Epic 1: Deterministic, correctly-budgeted test-side waits

#### Story 1.1: Fix the stale 30s→60s call-site bug

**Task 1.1.1** — Widen `waitForPermissionRequestHookCommand`'s two call-site timeouts from `30*time.Second` to `60*time.Second`.
- Files: `server/server_integration_test.go` (lines 338, 409)
- Size: 2 min
- Detail: Change `hookCmd := waitForPermissionRequestHookCommand(t, settingsPath, 30*time.Second)` → `... 60*time.Second)` at both call sites. Update the trailing inline comment at each call site (if any references "30s") to match. Do not touch the helper's own doc comment (lines 458-462) — it already states the correct intent; this task makes the code match the comment, not vice versa.
- **Given/When/Then**: Given `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` runs under `go test -race -count=5 -run TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort ./server/...` on a CI runner where `InjectHookConfig`'s write completes at, say, 42s under heavy `-race` contention (a value that would have failed the old 30s budget), when `waitForPermissionRequestHookCommand(t, settingsPath, 60*time.Second)` polls, then the test passes because 42s < 60s, whereas before this task it would have failed at the 30s deadline with `"expected .../.claude/settings.local.json to contain a PermissionRequest command hook within 30s"`.

#### Story 1.2: Convert hand-rolled poll loops to `require.Eventually`

**Task 1.2.1** — Add the `testify/require` import and convert `waitForPermissionRequestHookCommand` to `require.Eventually`.
- Files: `server/server_integration_test.go` (import block ~lines 1-18; function body 454-501)
- Size: 5 min
- Detail: Add `"github.com/stretchr/testify/require"` to the import block. Rewrite the function body to capture the resolved `hookCmd` string in a closure variable and assert with `require.Eventually(t, func() bool { ... return hookCmd != "" }, timeout, 50*time.Millisecond, "expected %s to contain a PermissionRequest command hook", settingsPath)`, preserving the exact same JSON-parsing logic (read file → unmarshal `hooks.PermissionRequest` → find first `type=="command"` entry) and the 50ms poll interval already used today. Return `hookCmd` after the `require.Eventually` call succeeds (it fails the test via `t.FailNow()` internally on timeout, matching the current `t.Fatalf` semantics).
- **Given/When/Then**: Given the file `settingsPath` never gets written (simulating a real hang, i.e. Race A), when `require.Eventually` exhausts its `60*time.Second` budget, then the test fails with testify's standard `"Condition never satisfied"` message plus the custom message `"expected <path> to contain a PermissionRequest command hook"` — equivalent fail-fast behavior to today's `t.Fatalf`, just testify's message format instead of the hand-rolled one.

**Task 1.2.2** — Convert `waitForLiveInstance` to `require.Eventually`.
- Files: `server/server_integration_test.go` (lines 441-452)
- Size: 3 min
- Detail: Same pattern: capture `inst` in a closure, `require.Eventually(t, func() bool { inst = deps.SessionService.FindLiveInstance(sessionID); return inst != nil }, timeout, 20*time.Millisecond, "expected session %q to appear in the live poller", sessionID)`, preserving the existing 20ms poll interval and unchanged 30s call-site budgets (lines 336, 407 — not modified by this task, only 1.1.1 touches a timeout value).
- **Given/When/Then**: Given `FindLiveInstance(sessionID)` returns non-nil after 12s of tmux spin-up (well under the unchanged 30s budget), when `waitForLiveInstance` is called from either test, then it returns the same `*session.Instance` pointer it would have under the old hand-rolled loop, with no behavior change beyond the polling mechanism.

**Task 1.2.3** — Convert `waitForResolvedAddr` to `require.Eventually`.
- Files: `server/server_integration_test.go` (lines 424-437)
- Size: 2 min
- Detail: Same pattern, preserving the 10s timeout and 10ms poll interval and the exact non-zero/non-`:0` address check.

**Task 1.2.4** — Convert `waitForTmuxTeardown` to a `require.Eventually`-*equivalent* that stays non-fatal.
- Files: `server/server_integration_test.go` (lines 503-539)
- Size: 3 min
- Detail: **Do not use `require.Eventually` here directly** — `require.Eventually` calls `t.FailNow()`/`t.Errorf()` internally on timeout, which would break this helper's documented deliberate non-fatal behavior (it runs inside `t.Cleanup`, after the test's own pass/fail verdict is already decided; see the comment at lines 522-526). Instead, replace the hand-rolled loop with the already-available `testutil/wait.WaitForCondition` (`testutil/wait/wait.go`, purpose-built for "condition polling with a timeout" and — per its own package doc — for exactly this "can't import top-level `testutil` due to an import cycle with `session`" situation) and log via `t.Logf` on its returned error instead of failing. If `server` package can cleanly import `testutil/wait` (verify no cycle — `server` already imports `session`, and `testutil/wait` imports neither `server` nor `session`, only `context`/`fmt`/`time`), use `wait.WaitForCondition(func() bool { return !inst.TmuxSessionExists() }, wait.WaitConfig{Timeout: timeout, PollInterval: 20 * time.Millisecond, Description: fmt.Sprintf("tmux teardown for %q", inst.Title)})` and `t.Logf` its error. This is the one helper of the four where "convert for consistency" (per requirements scope item 2, "ideally") means reaching for the *other* existing no-new-deps polling helper in the repo, not `require.Eventually`, precisely because the fatal/non-fatal distinction matters here.
- **Given/When/Then**: Given `DeleteSession`'s teardown goroutine has not finished tearing down tmux by the time `t.Cleanup` runs (a slow-but-eventually-successful teardown), when `wait.WaitForCondition` exhausts its 5s budget, then `t.Logf("tmux session for %q still reported alive %s after DeleteSession; teardown may still be in flight", inst.Title, timeout)` is logged and the *already-decided* test result (pass) is unaffected — identical to today's `t.Logf`-only behavior, confirmed by re-running `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` and asserting it still reports `PASS` even when a synthetic 6-second sleep is injected before `TmuxSessionExists()` returns false in a throwaway local repro.

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
  Leave the `-v ./...` fallback diagnostic re-run (which only fires on failure, for verbose output) unchanged — it is not the gating step and adding `-p 1` there is unnecessary for a diagnostic-only path that already only runs after the primary command failed.
- **Given/When/Then**: Given the `Test` job today runs `./server/...`, `./session/...`, `./config/...` as up to 3 concurrent package binaries competing for 4 vCPUs, when `-p 1` is added, then GitHub Actions logs for the step show the three package binaries' `--- PASS/FAIL` blocks appearing sequentially (not interleaved) in the raw log, and `coverage.out` is still produced by the same single command with the same `-covermode=atomic` — verified by diffing the coverage gate's reported total percentage across two CI runs (before/after) and confirming it does not drop below the existing 60% global threshold.

**Task 2.1.2** — Measure and record the wall-clock cost of `-p 1` before merging.
- Files: none (verification-only; record finding in `project_plans/flaky-hook-url-tests/implementation/validation.md`, created in Phase 4)
- Size: 5 min
- Detail: Run the gating command locally twice — once as-is, once with `-p 1` — and record the wall-clock delta (`time TMUX_BIN="$(pwd)/bin/tmux" go test -race -coverprofile=/tmp/cov-before.out -covermode=atomic ./server/... ./session/... ./config/...` vs. the same with `-p 1` appended and `/tmp/cov-after.out`). This is the evidence backing the "no increase in the tests' own runtime budget beyond what's demonstrably needed" success metric being satisfied at the *test* level (Epic 1) while being honest that Epic 2's CI-level wall-clock cost is a separate, acknowledged trade-off, not hidden.
- **Given/When/Then**: Given a pre-change local run measures, say, 4m10s, when the post-`-p 1` run is measured, then the delta (e.g. +1m50s) is written into the PR description as an explicit, accepted trade-off — not silently absorbed.

### Epic 3: Regression check / observability artifact

#### Story 3.1: Document a reusable stress-repro command

**Task 3.1.1** — Add a local stress-repro command as a code comment above the two affected tests.
- Files: `server/server_integration_test.go` (immediately above `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`, and above `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`)
- Size: 3 min
- Detail: Add a short comment block (reusing the existing doc-comment convention already used throughout this file, e.g. lines 358-372) documenting the exact repro command a future flake investigation can copy-paste, using only shell builtins (no new external dependency, per requirements.md's Constraints):
  ```go
  // Flake repro: to reproduce Race B (marginal-timeout-under-load, see
  // project_plans/flaky-hook-url-tests/requirements.md) locally with
  // artificial CPU contention approximating CI's 4-vCPU runner under full
  // `-race` load:
  //
  //   yes > /dev/null & yes > /dev/null & yes > /dev/null &
  //   TMUX_BIN="$(pwd)/bin/tmux" go test -race -count=10 \
  //     -run 'TestServer_should_Write.*HookURL' ./server/...
  //   kill %1 %2 %3
  ```
  This gives a future investigator both the correctness stress-run (`-count=10`, matching the precedent in `waitForTmuxTeardown`'s own comment, which cites "`-count=10`" as the reproduction methodology already used for the shared-socket finding) and an artificial-load variant, without adding a Makefile target, script, or dependency.
- **Given/When/Then**: Given a future engineer sees this test pair flake again in CI, when they copy-paste the comment's repro command into a local shell, then they can reproduce Race B's failure mode locally (elevated, contended, but eventually-successful completion) without needing CI access or guessing at a repro methodology from scratch.

**Task 3.1.2** — Cross-link the repro command from `waitForPermissionRequestHookCommand`'s doc comment.
- Files: `server/server_integration_test.go` (lines 458-462)
- Size: 2 min
- Detail: Append one sentence to the existing doc comment: "See the repro-command comments above the two call sites for a local stress-repro of the load-sensitive failure mode this budget accounts for." Do not rewrite the rest of the comment — it remains accurate after Task 1.1.1 (the call sites now do pass 60s, so "callers pass 60s" is no longer stale).

---

## Migration Plan

Omitted — no schema or data changes. This is a test-file and CI-workflow-only change.

---

## Observability Plan

- **Primary observability artifact**: the stress-repro command added in Task 3.1.1, living directly in `server/server_integration_test.go` next to the tests it documents (not a separate doc file, to guarantee it's discovered by anyone editing these tests again) — `yes > /dev/null &`-based artificial CPU contention + `go test -race -count=10 -run 'TestServer_should_Write.*HookURL' ./server/...`.
- **Secondary signal**: CI's own pass/fail history for the `Test` job on `main` and on PRs over the 20-run window named in requirements.md's Success Metrics — no new dashboard or alerting needed; GitHub Actions' existing run history is sufficient to confirm the near-zero-flake outcome.
- **What we are explicitly not adding**: no new metrics, no new EventBus event, no new logging statement in production code (`approval_handler.go`/`session_service.go`/`tmux.go` are untouched) — this stays a test-and-CI-config-only change per the Pattern Decisions table.

---

## Risk Control

- Standard PR revert. No feature flag needed — this is a test-file and CI-workflow-only change with no production code path affected (confirmed: no edits to `approval_handler.go`, `session_service.go`, or `session/tmux/tmux.go`).
- If `-p 1` (Task 2.1.1) causes the `Test` job to exceed CI's timeout budget (check `.github/workflows/build.yml` for a job-level `timeout-minutes`; if none is set, GitHub Actions' default is 360 minutes, effectively not a concern here given the measured delta in Task 2.1.2 is expected to be low-single-digit minutes), revert Task 2.1.1 independently of Epic 1 — the two epics are decoupled and either can ship without the other.
- If widening to 60s (Task 1.1.1) still shows residual flakiness after the 20-run observation window, the next lever (not this ticket's scope) would be investigating the shared-tmux-socket / `testSocketOnce` root cause already documented in `waitForTmuxTeardown`'s comment — file that as a separate ticket per requirements.md's Out of Scope ("Fixing unrelated flakiness discovered along the way (file separately)").

---

## Unresolved Questions

- **Exact wall-clock cost of `-p 1`** on the actual GitHub Actions `ubuntu-latest` runner (as opposed to a local approximation) is not knowable until Task 2.1.2's local measurement is corroborated by an actual CI run post-merge — flagged as a "measure in CI, don't just trust the local estimate" follow-up, not a blocker to shipping.
- **Whether 60s (vs. some smaller, adaptively-computed value) is the *right* number for `waitForPermissionRequestHookCommand`**, independent of `-p 1`'s mitigation, is not conclusively provable without exact CI contention data (requirements.md's own Feasibility Risks flags this) — 60s is chosen because it is what the code's own pre-existing doc comment already committed to, the cheapest defensible number, and consistent with the "tune the wait" 3-for-3 precedent; it is not derived from a controlled experiment.
- All three of requirements.md's original Open Questions are otherwise resolved by research and reflected in the Pattern Decisions table:
  - "Does `-race` need to cover this file specifically...?" → Resolved: yes, coverage must be preserved, and `-p 1` preserves it exactly (same command, same flags, same profile).
  - "Is there a quick win in `approval_handler.go`'s hook-injection path?" → Resolved: no — `InjectHookConfig`'s file write is confirmed not the bottleneck (atomic write, no torn-read window); the bottleneck is entirely the preceding tmux spin-up, which is Race A's/`tmux.go`'s territory and out of scope.
  - "Are other jobs in `build.yml` running concurrently...contributing to runner contention?" → Not separately investigated (out of scope per requirements.md — "Changing CI provider, runner OS...` is out of scope, and cross-*job* contention within the same workflow run was not named as in-scope); flagging as a residual open question if flakiness persists after this plan ships.
