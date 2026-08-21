# ADR-001: `-p 1` on the Existing Gating Invocation, Not a Second Isolated `-race` Job

## Status
Accepted — implemented in `.github/workflows/build.yml` via the `ci-hookurl-race-flake` PR.

## Context

Two integration tests in `server/server_integration_test.go` intermittently time out in CI's `Test` job (`.github/workflows/build.yml`), which runs:

```
TMUX_BIN="$(pwd)/bin/tmux" go test -race -coverprofile=coverage.out \
  -covermode=atomic ./server/... ./session/... ./config/... \
  || (TMUX_BIN="$(pwd)/bin/tmux" go test -race -v ./...; exit 1)
```

on a 4-vCPU `ubuntu-latest` runner. Research (`project_plans/flaky-hook-url-tests/research/stack.md`) confirmed:

- `-race` (ThreadSanitizer) imposes ~2-20x CPU/wall-time overhead.
- `go test` runs up to `GOMAXPROCS` (= `NumCPU` = 4 on this runner) package binaries concurrently. This single invocation spans 3 package trees (`./server/...`, `./session/...`, `./config/...`), each independently `-race`-taxed, all competing for the same 4 cores.
- The affected tests wait (via `waitForLiveInstance` / `waitForPermissionRequestHookCommand`) for a real tmux spin-up + async file write to complete within a fixed budget — a budget that is marginal specifically under this contention, not broken outright (this is "Race B" — see `requirements.md`'s glossary distinction from the already-mitigated "Race A" early-exit hang).

Requirements.md's explicit constraints for any fix:
- Must not weaken `-race` coverage for `./server/... ./session/... ./config/...`.
- Must not require new external dependencies or a different CI provider.
- If isolating this file from `-race` contention is pursued, the rest of that scope must still run under `-race` — race coverage must not regress package-wide to fix two tests.

Three CI-side levers were on the table (this ADR covers only the CI-workflow-change option; the test-code-side fix — `require.Eventually` + the 30s→60s stale-comment fix — is covered separately in `implementation/plan.md` Epic 1 and is adopted regardless of this ADR's outcome):

1. **Do nothing at the CI level** — rely solely on the test-code fix.
2. **Add `-p 1`** to the existing single gating invocation.
3. **Isolate `server_integration_test.go` (or the whole `server` package) into a second, still-gating `go test -race` invocation**, with its `-coverprofile` output merged into `coverage.out` before the coverage gate runs.

## Decision

**Adopt option 2: append `-p 1` to the existing single gating invocation** (`.github/workflows/build.yml:155`, changing `go test -race -coverprofile=...` to `go test -race -p 1 -coverprofile=...`). Reject option 3 (isolated second invocation + coverage merge) and reject option 1 (CI-side no-op) as insufficient on its own.

### Why not option 1 (CI no-op, test-fix only)

The test-code fix (Epic 1: `require.Eventually` + the 60s budget) addresses the *symptom budget* but does nothing about the *diagnosed cause* (cross-package CPU contention). Research explicitly names this contention as a live contributor; leaving it untouched means the fix is a strictly narrower bet than the evidence supports; there's a cheap, safe lever available (`-p 1`) that directly targets the named cause, so declining to use it would be leaving a known, low-risk mitigation on the table for a bug whose success metric is "near-zero flake rate across 20 CI runs," not "narrower but still marginal."

### Why not option 3 (isolated second invocation)

1. **Coverage-merge correctness is unverified and risky.** `go.mod` has no `gocovmerge` dependency, and adding one would violate the explicit "no new external dependencies" constraint. The alternative — naively concatenating two `mode: atomic` text profiles (skipping the duplicate `mode:` header line) — is a well-known-fragile technique: it is only safe when the two profiles cover disjoint source lines. Here they would *not* be fully disjoint in general (both invocations would still need to cover `./server/...` if `server_integration_test.go` and its sibling tests in the same package are isolated together, or the split itself becomes a maintenance hazard if it's *file*-level rather than *package*-level, since `go test` operates at package granularity, not file granularity). Verifying this correctness is out of proportion to the size of the bug being fixed.
2. **Job-setup duplication is an ongoing drift risk.** A second invocation needs its own tmux build/cache step and Go setup (or must be squeezed into the same job in a way that risks the two `go test` invocations interfering with each other, e.g. via the shared `TMUX_BIN` binary or `config.IsTestMode()`'s process-PID-keyed state). Every future change to the primary test step (Go version bump, tmux version bump, new package added to the gating scope) now has two places to update, and CI workflow drift of this kind is exactly the class of bug this ticket exists to prevent, not introduce.
3. **Disproportionate to the evidence.** `research/build-vs-buy.md` found this exact test pair has been patched 3 separate times before for CI-only timing flakiness, **all three times via wait-tuning, never via CI topology changes** — a 3-for-3 precedent against option 3, and 0-for-0 for it. `research/pitfalls.md` independently and explicitly flags "splitting `-race` into a separate CI job/invocation" as "a heavier, riskier lever than it looks."
4. **It does not even fully solve the diagnosed problem.** `research/features.md` notes that the *shared tmux socket* contention (`testSocketOnce`, documented in `waitForTmuxTeardown`'s own comment) is a *within-binary* problem — isolating just these 2 tests into their own invocation would not fix it, because other tests in the same package/binary still share the socket. Option 3's complexity buys less correctness improvement than it appears to at first glance.

### Why `-p 1` satisfies the constraints cleanly

- **No new dependency**: `-p` is a built-in `go test`/`go build` flag.
- **No coverage-merge risk**: `coverage.out` is still produced by exactly one `go test -coverprofile=... -covermode=atomic` invocation, identical in every respect except serialized rather than concurrent package execution — the coverage gate sees the same artifact shape it always has.
- **No weakened `-race` coverage**: the same three package trees run under the same `-race` flag; nothing is excluded, skipped, or moved to a non-gating lane.
- **Directly targets the named cause**: serializing package binaries means whichever package is running (including `server`, where the flaky tests live) has the full 4 vCPUs to itself instead of competing with up to 3 other `-race`-taxed binaries.
- **Trivially reversible**: a one-line revert if the wall-clock cost (Task 2.1.2 in `implementation/plan.md`) proves unacceptable, fully decoupled from Epic 1's test-code fix.

## Consequences

### Positive
- CI's gating step becomes strictly less internally contended, which should reduce the "marginal timeout under load" failure mode across not just these two tests but any other test in the same three package trees that shares the same class of budget-marginal wait (a bonus not claimed as a primary goal, but a plausible side benefit).
- Zero new CI surface area, zero new dependency, fully reversible independent of the test-code changes.

### Negative
- Total wall-clock time for the `Test` job's primary step increases, because the three package trees can no longer overlap their build/test time. The exact delta is measured locally in Task 2.1.2 and must be recorded (not silently absorbed) in the shipping PR's description.
- Does not address the separate, already-documented `testSocketOnce` shared-socket contention within a single package binary — that remains a residual risk noted in `implementation/plan.md`'s Risk Control section as a candidate follow-up ticket, not solved here.

### Neutral
- If, after the 20-CI-run observation window named in requirements.md's Success Metrics, flakiness persists despite both the 60s budget fix and `-p 1`, the next escalation is investigating the shared-socket root cause directly (a different, larger ticket) — not revisiting option 3, which this ADR has already evaluated and rejected on cost/risk grounds independent of whether `-p 1` alone proves sufficient.
