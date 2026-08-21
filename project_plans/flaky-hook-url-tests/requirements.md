# Requirements: flaky-hook-url-tests

**Date**: 2026-08-01
**Type**: bug fix (CI reliability)
**Complexity**: 2 — focused feature (touches CI workflow config, test timeout/synchronization design, and potentially the hook-injection pipeline's architecture)

## Problem Statement
Two integration tests in `server/server_integration_test.go` —
`TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` and
`TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`
— intermittently time out in CI under `go test -race ./...`. Both wait (via
`waitForPermissionRequestHookCommand` / `waitForLiveInstance`, defined in the same file)
for `CreateSession`'s asynchronous hook-injection path (real tmux session spin-up +
`.claude/settings.local.json` write, see `server/services/approval_handler.go`) to
complete within a 30s budget. Under `-race`'s 2-10x overhead plus concurrent
resource contention from the rest of `./...` running in the same CI job, that budget is
marginal and intermittently blown — 3 consecutive CI reruns during PR #190 (which never
touched this test file or the hook-injection path) failed here, with escalating failure
counts (1 test failed on run 2, 2 tests failed on run 3), consistent with a load-sensitive
timeout rather than a deterministic regression.

The test file's own top-of-file comment (`installFakeClaudeBinary`, lines 36-51) already
documents a related, more severe failure mode investigated previously: if the fake
`claude` stand-in binary exits before tmux's readiness check observes the session as
live, `CreateSession`'s async goroutine returns early — before ever calling
`InjectHookConfig` — and the waiters "time out no matter how long [their] budget is
(observed: failed at both 30s and 60s)." That specific race is currently worked around
by having the fake binary `sleep 60` so it outlives the readiness check; this item is
about the *remaining*, load-driven marginal-timeout flakiness on top of that already-
worked-around race, not a reintroduction of it.

## Baseline
Today, these two tests pass reliably in local `make test` (plain `go test -short`, no
`-race`), but fail intermittently in the `Test` job of `.github/workflows/build.yml`,
which runs `go test -race -coverprofile=coverage.out -covermode=atomic ./server/...
./session/... ./config/...`. When they fail, the job goes red on PRs that never touched
the affected code path, forcing an unrelated rerun (sometimes multiple) before merge —
burning CI minutes and reviewer patience, and training engineers to reflexively re-run
red CI instead of investigating, which risks masking a future real regression in the
hook-injection path.

## Users / Consumers
- stapler-squad contributors (github.com/TylerStaplerAtFanatics/stapler-squad and the
  tstapler/stapler-squad personal fork) whose PRs run this CI job.
- The `Test` GitHub Actions job itself (consumes wall-clock CI minutes on reruns).

## Success Metrics
- Zero (or near-zero — see Appetite) intermittent failures of these two tests across the
  next 20 consecutive CI runs of the `Test` job on `master`/PR branches, compared to the
  observed ~3-consecutive-failure baseline from PR #190.
- No increase in the tests' own runtime budget beyond what's demonstrably needed (i.e.
  avoid papering over the issue by blindly widening the timeout to an arbitrarily large
  number without addressing *why* it's marginal).
- If the fix involves isolating this test file from `-race`'s contention (e.g. running it
  in its own job/invocation), the rest of `./server/... ./session/... ./config/...` must
  still run under `-race` — race coverage must not regress package-wide to fix two tests.

## Appetite
Small (1–2 days). This is a CI-flake investigation and fix, not a redesign of the
hook-injection pipeline. If root-causing genuine determinism improvements in
`approval_handler.go` / tmux spin-up turns out to require deeper surgery, cut scope back
to timeout/isolation mitigations and file a follow-up backlog item for the deeper fix
rather than extending this item's appetite.

## Constraints
- Must not weaken `-race` coverage for the packages currently covered by the `Test` job
  (`./server/... ./session/... ./config/...`).
- Must not require new external dependencies or a different CI provider — this is a
  workflow config + test code change within the existing GitHub Actions Ubuntu runner.
- Fix must be verifiable without merge access to see it fail again live — validate via
  either reproducing the failure locally (e.g. running the affected tests under `-race`
  with artificial CPU contention) or via a stress-run (`go test -race -count=N`) that
  demonstrates the marginal timeout and then demonstrates it's resolved.

## Non-functional Requirements
- **Performance SLO**: not applicable (CI reliability, not a runtime SLO for the product).
- **Scalability**: not applicable.
- **Security classification**: internal (CI workflow + test code only).
- **Data residency**: not applicable.

## Scope
### In Scope
- Diagnosing why the current 30s waits (`waitForLiveInstance`,
  `waitForPermissionRequestHookCommand`) are marginal specifically under
  `go test -race` + full-suite concurrent load in CI (not under local `-short` runs).
- Evaluating and choosing among the three "suggested next steps" already named in the
  backlog item:
  1. Whether `-race` is strictly needed for this integration-test file, or whether it
     (or the whole `server` package) could run without `-race` or in isolation from the
     rest of `./...` to remove CPU/memory contention.
  2. Whether the hook-injection pipeline (tmux spin-up → `.claude/settings.local.json`
     write, `server/services/approval_handler.go`) has room to be made faster or more
     deterministic, rather than only widening the timeout further.
  3. Whether GitHub Actions runner sizing/concurrency is a contributing factor (e.g. this
     job sharing a small/shared runner with other concurrent jobs).
- Implementing the chosen mitigation(s) — this may be a CI workflow change
  (`.github/workflows/build.yml`), a test code change (`server/server_integration_test.go`
  waiters), and/or a production code change to `approval_handler.go` if a concrete
  determinism win is found there.
- Adding or strengthening a regression check (e.g. a stress-test invocation, or a
  documented repro command) so this flake class can be re-diagnosed quickly if it
  recurs, per the "no fix without root cause / green first, then done" project norm.

### Out of Scope
- A general redesign of the tmux session lifecycle or hook-injection architecture beyond
  what's needed to fix this specific flake.
- Changing CI provider, runner OS, or Go version.
- Any change to `-race` usage for packages/tests outside
  `server_integration_test.go`'s hook/MCP-URL tests unless the chosen fix is "isolate this
  file's `-race` run," in which case the *rest* of the existing `-race` coverage must be
  preserved as noted in Constraints.
- Fixing unrelated flakiness (if any is discovered) in other tests during this
  investigation — file separate backlog items instead, per "do what has been asked;
  nothing more, nothing less."

## Rabbit Holes
- **tmux spin-up latency profiling**: measuring exactly where time goes in a real tmux
  session start + `InjectHookConfig` write under `-race` could expand into a broader
  performance-profiling exercise (see `.claude/docs/profiling.md`,
  `.claude/docs/concurrency-patterns.md`). Time-box this to a quick check (does the
  existing `--profile --trace` tooling show an obvious hotspot?) rather than a full
  profiling pass, unless it directly explains the marginal timeout.
- **GitHub Actions runner investigation**: verifying actual runner sizing/concurrency
  requires org-level GitHub settings access that may not be available from this session;
  treat this as a research question to answer "as far as observable" (workflow YAML,
  concurrent job count in `build.yml`) rather than blocking the fix on infrastructure
  changes outside this repo's control.
- **Reproducing the exact CI failure locally**: `-race` + full-suite contention is
  runner-specific and may not reproduce identically on a dev machine; budget for
  "reasonably convincing local stress repro," not exact reproduction.

## Alternatives Considered
- **Widen the timeout further** (e.g. 30s → 60s or 90s): cheapest possible change, but
  the file's own comment about the *other* related race ("failed at both 30s and 60s")
  is a specific warning against treating timeout-widening as a reliable fix in this file
  — it can mask the same symptom recurring at a new, larger threshold. Considered but not
  preferred as the sole fix; may still be part of a combined mitigation if paired with a
  root-cause improvement.
- **Isolate this test file (or `TestServer_should_Write*`) to run without `-race` or in
  its own job**: removes the specific overhead multiplier implicated by the backlog
  item's own diagnosis, at the cost of losing race coverage for this file's code paths
  unless carefully scoped (e.g. `-race` in a separate, less-contended job just for this
  file, rather than dropping `-race` entirely).
- **Speed up hook injection itself**: addresses the actual bottleneck (tmux spin-up +
  file write) rather than the timeout budget around it — most durable fix if a concrete,
  low-risk speedup exists (e.g. avoiding an unnecessary poll interval, using go-git-style
  direct primitives instead of shelling out per
  `.claude/rules/prefer-go-git-over-subshells.md`-style guidance, though that rule is
  git-specific and may not directly apply to tmux spin-up).
- **Increase GitHub Actions runner size for this job**: outside this repo's control from
  within this session (requires org-level runner config); noted as a possible
  contributing factor to flag for the user/CI-owner rather than something this item can
  implement directly.

## Feasibility Risks
- The marginal-timeout hypothesis may not be conclusively provable without triggering the
  exact CI contention pattern; the fix may need to ship as a "most likely mitigation" and
  be verified empirically over the next N CI runs (see Success Metrics) rather than
  proven correct before merge.
- Any change to `approval_handler.go`'s hook-injection timing touches an async path with
  its own already-documented race (the `installFakeClaudeBinary` early-exit race) —
  changes here risk reintroducing or interacting with that race if not careful; this is a
  concurrency-sensitive Go code path and should invoke the `go-concurrency` /
  `go-development` skills per this project's CLAUDE.md guidance on Go work.
- If the true root cause turns out to be GitHub Actions runner-level contention outside
  this repo's control, the best achievable fix from this repo alone may only reduce (not
  eliminate) the failure rate — Success Metrics should be read as "near-zero," not
  "provably zero."

## Observability Requirements
Standard CI failure visibility is sufficient (GitHub Actions run status + logs). If the
chosen fix includes a stress-test invocation or repro script, document the exact command
in a code comment or `.claude/docs/` reference so a future flake investigation doesn't
have to rediscover it from scratch.

## Risk Control
Not needed — low risk. This change touches test code and/or CI workflow config, not a
production runtime path exposed to real users (or, if `approval_handler.go` timing is
touched, the change should be small and covered by the existing test suite; no feature
flag or staged rollout is warranted for a CI-only or narrowly-scoped production timing
change). Standard rollback via PR revert if the fix itself introduces new instability.

## Open Questions
- Does `-race` actually need to cover `server_integration_test.go`'s hook/MCP-URL tests
  specifically, or would isolating just this file (not the whole `server` package) from
  `-race` preserve most of the race-detection value while removing the contention?
- Is there a quick, low-risk win in `approval_handler.go`'s hook-injection path (e.g.
  polling interval tuning) that would tighten the actual latency rather than just the
  timeout budget around it?
- Are other jobs in `.github/workflows/build.yml` running concurrently with the `Test`
  job in a way that's plausibly contributing to runner contention (visible from the
  workflow YAML's `needs:`/concurrency config), or is this isolated to `-race` overhead
  within the single `Test` job itself?
