# Requirements: Flaky CI — hook-URL/MCP-URL integration tests timeout under `go test -race`

## Source

Migrated from GitHub issue #192 (originally filed by @tstapler, 2026-07-25), backlog
item `7271b842-7139-46ef-8c99-8083107b4d97`. This document was authored directly from
the backlog item's description; no interactive ideation interview was run (non-interactive
triage session).

## Problem Statement

`server/server_integration_test.go`'s hook-URL/MCP-URL integration tests intermittently
fail in CI with 30-second timeouts, unrelated to any specific code change:

- `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`
- `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`

Both tests fail the same way: `waitForPermissionRequestHookCommand` / `waitForLiveInstance`
(defined in the same file) time out after 30s waiting for `CreateSession`'s asynchronous
hook-injection pipeline (spin up a real tmux session via the pinned tmux 3.4 binary, then
write `.claude/settings.local.json`) to complete.

Observed: 3 consecutive CI reruns of the same job, on an unrelated PR (#190, which touched
only `session/backlog_*.go` and `server/services/backlog_service.go`), all failed on this
file, with an escalating failure count (1 test on run 2, 2 tests on run 3) — consistent
with a marginal-timeout issue that worsens under variable CI load, not a deterministic bug
introduced by that PR's diff.

## Root Cause (diagnosed while shipping PR #190, confirmed in this triage pass)

- CI's `test` job (`.github/workflows/build.yml`, `Run tests with coverage (pinned tmux
  3.4)` step) runs `go test -race -coverprofile=... ./server/... ./session/... ./config/...`
  — i.e., the full server/session/config suite runs under the race detector in one process,
  competing for CPU/memory on a shared `ubuntu-latest` runner. `-race` instrumentation adds
  substantial (documented as 2-10x) CPU/memory overhead industry-wide.
- The test file's own comment already flags this exact risk (`server_integration_test.go`,
  near the `waitForPermissionRequestHookCommand`/timeout helpers): timeouts are set
  "generous" specifically because CI runs this under `-race` alongside the rest of the
  suite, pushing tmux spin-up + hook injection close to the deadline under load.
- Locally, `make test` runs `go test -short` (no `-race`) and these tests reportedly pass
  reliably — `-race` plus shared-runner contention is what intermittently pushes the
  already-generous 30s timeout over the edge.

## Evidence This Is Pre-Existing / Environmental, Not a Regression

- Neither the failing tests, `server/server_integration_test.go`, nor the hook-injection
  code path (`server/services/approval_handler.go`) was touched by PR #190.
- PR #190's full local test suite (including the non-`-race` `make test`) passed cleanly
  multiple times.
- 3 consecutive CI reruns of the same job all failed on this same file with an escalating
  (not constant) failure count.

## Acceptance Criteria

1. The CI `test` job's hook-URL/MCP-URL integration tests do not intermittently time out
   under normal CI load. **Resolved during planning (this project's own Success Metric, not
   borrowed from the sibling project below): N = 20 consecutive green CI runs of the `test`
   job post-fix on `main`/PRs, OR, as a pre-merge stand-in when 20 CI runs haven't yet
   accumulated, a local stress-test result of `-count=10` runs of both named tests under
   artificial CPU contention (the repro command added in Task 3.1.1) passing consistently.**
2. Any change to test/CI configuration (timeout values, `-race` scope, job parallelism,
   runner sizing) is captured with a rationale, not a bare number bump.
3. If the fix narrows or removes `-race` coverage for any package, that trade-off is
   explicit and justified (this is a correctness-tooling regression risk, not just a
   flake-suppression knob).
4. The hook-injection pipeline's actual latency under `-race` + concurrent-suite load is
   measured (not assumed) before deciding between "raise the timeout further" and "make
   the pipeline faster/more deterministic."
5. No source behavior in `server/services/approval_handler.go` or the hook-injection path
   is changed unless the investigation shows the pipeline itself (not just test
   infrastructure) is the bottleneck.

## Success Metrics

- **N = 20** consecutive green CI runs of the `test` job (`.github/workflows/build.yml`) on
  `main`/PRs post-fix, with zero recurrences of
  `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` or
  `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`
  timing out. This is this project's own decision (see AC #1) — not inherited from the
  sibling `flaky-hook-url-tests` project, though it happens to match that project's number.
- Pre-merge stand-in (since 20 CI runs can't accumulate before merge): the stress-repro
  command from plan.md Task 3.1.1 (`-count=10` under artificial `yes`-loop CPU contention)
  passing consistently across at least 2 separate local invocations.

## Out of Scope

- Rewriting the hook-injection pipeline's architecture (tmux spin-up + settings.json write)
  unless research shows a small, targeted change is the right fix.
- General CI speedup/cost work unrelated to this specific flake.
- This triage session does not implement a fix — it produces requirements, research, a
  plan, and a validation pass per the SDD pipeline for a human/future session to act on.

## Suggested Next Steps (carried over from the backlog item, for research phase to evaluate)

- Whether `-race` is actually needed for this specific integration-test package, or
  whether it could run without `-race` (or isolated from the rest of `./...`) to remove
  the CPU/memory contention pushing it over the timeout.
- Whether the hook-injection pipeline (tmux spin-up → `.claude/settings.local.json` write)
  has room to be made faster/more deterministic rather than just widening the timeout.
- GitHub Actions runner sizing/concurrency — if this job runs alongside many other jobs on
  a shared/smaller runner, that's a likely contributor.
