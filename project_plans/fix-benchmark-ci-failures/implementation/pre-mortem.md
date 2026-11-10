# Pre-Mortem: fix-benchmark-ci-failures

**Date**: 2026-11-04
**Scenario**: Imagine this fix has shipped and failed. What went wrong?

---

## Failure Mode Inventory

### FM-1 — test.setTimeout fires on a slow CI runner (zero overhead slack)

**Failure**: The terminal-throughput spec still times out intermittently because `test.setTimeout(600_000)` = exactly 60s × 10 runs leaves no room for per-iteration setup overhead (~2s × 10 = 20s), so a full-load CI run takes ~620s and the outer timeout fires before all 10 iterations complete.

**First symptom**: The `frontend-throughput` job fails with a Playwright outer-test-timeout error on slow-runner days while passing on fast ones — a flaky failure, not a consistent fix.

**Prevention**: Change `test.setTimeout(600_000)` to `test.setTimeout(700_000)` (11m40s) in plan.md Story 1.2.1a, matching the adversarial review CONCERN 1 recommendation. The pitfalls research already prescribed "at least 650s"; 700s adds 100s of CI-variance headroom. The `frontend-throughput` job cap of `timeout-minutes: 45` is unaffected (700s ≈ 12 min, well inside the job ceiling after browser install).

**Severity**: P1 — Likely (adversarial review confirmed the gap; the same zero-slack design broke the original 300s timeout) AND catastrophic (CI is still broken after the fix ships; the team loses confidence in the entire repair).

---

### FM-2 — go-tier2 newly-compilable benchmark packages hit the 30-min timeout

**Failure**: After adding buf generate + ent generate, packages that previously failed silently now compile and run their `count=8` benchmarks for the first time; if any carry non-trivial per-iteration cost, the first post-fix main push hits the `-timeout=30m` go test flag or the 50-minute job cap, and go-tier2 fails with what looks like a new regression introduced by the fix.

**First symptom**: The go-tier2 job on the very first post-fix main push times out with `panic: test timed out after 30m0s` or is killed by GitHub Actions at 50 minutes — a failure that did not exist before the PR merged.

**Prevention**: Before merging, add a pre-merge audit task to plan.md Story 1.1.2: enumerate packages importing generated protobuf or ent types, run `go test -bench=. -count=1 -timeout=5m <package>` locally to bound their cost, and if any add materially to the full sweep, raise `-timeout=30m` to `-timeout=40m` and `timeout-minutes: 50` to `timeout-minutes: 60`. This is a one-time check, not maintenance.

**Severity**: P2 — Likely (the adversarial review rated this Medium-High for the first post-fix run; previously-silenced packages are unknown quantities) but recoverable (a second PR raising the timeout fixes it in one push; no data is lost).

---

### FM-3 — frontend-bundle on ubuntu-latest breaks the Playwright job chain

**Failure**: The three Playwright jobs are pinned to ubuntu-24.04, but their upstream dependency `frontend-bundle` stays on `ubuntu-latest`; when ubuntu-latest resolves to a future Ubuntu release and `frontend-bundle`'s apt dependencies or Bun install step breaks, all three Playwright jobs fail at the artifact-download step — even though each one's own runner is correct.

**First symptom**: All three Playwright jobs (`frontend-throughput`, `e2e-latency`, `e2e-perf`) simultaneously fail with an artifact-not-found or upload-failure error traceable to `frontend-bundle`, not to any of the three pinned jobs themselves — confusing to diagnose because the runner labels look right.

**Prevention**: Add an optional task to plan.md Story 1.1.1 to also pin `frontend-bundle` (line 187) to `ubuntu-24.04`. The plan correctly notes this is not required by any AC today, but the architecture review MINOR 2 called it a known deferred maintenance item. Pinning it now costs one line and eliminates the cross-job dependency risk. Alternatively, add a tracking comment in benchmark.yml above `frontend-bundle`'s `runs-on` noting the deferred pin.

**Severity**: P2 — Medium-term likelihood (ubuntu-latest is now on Ubuntu 26; the next jump to Ubuntu 28 is ~April 2028, 18 months away) but recoverable (a one-line fix when the breakage surfaces; no data corruption).

---

### FM-4 — buf binary integrity unverified; supply-chain substitution produces silent bad codegen

**Failure**: The `Install buf` step downloads `buf-Linux-x86_64` via curl with no sha256 integrity check; a compromised or accidentally-replaced GitHub release artifact produces a buf binary that silently generates incorrect protobuf stubs, which compile without error but produce wrong wire-format encodings at runtime.

**First symptom**: Go benchmarks pass and CI goes green, but benchmark data written by protobuf-serialized types is corrupted or truncated; the first sign is a downstream consumer (e.g. benchmark result parser) reporting unexpected struct values or empty fields, not a CI failure.

**Prevention**: Add a sha256 checksum verification step immediately after the curl download in plan.md Story 1.1.2a, matching the pattern already recommended by the adversarial review MINOR 3. For buf v1.41.0, pin the expected SHA (obtainable from buf's release page or `sha256sum buf-Linux-x86_64`) and fail the step if it doesn't match.

**Severity**: P3 — Unlikely (GitHub release artifacts are rarely substituted; the attack surface requires GitHub or Buf's release infrastructure to be compromised) but catastrophic (corrupted benchmark output is data-integrity damage that may not surface for days; the fix requires rerunning all affected benchmarks and auditing stored results).

---

### FM-5 — ubuntu-24.04 runner retirement causes the pinned jobs to fail with no warning system

**Failure**: GitHub Actions retires ubuntu-24.04 runners (60-day notice per GHA policy, expected ~2027) and all three Playwright jobs fail with "No hosted runners matched the label: ubuntu-24.04"; because no tracking issue, no Renovate rule, and no calendar reminder was created when the pin was added, the team is surprised when it surfaces.

**First symptom**: All three Playwright jobs fail simultaneously with a runner-label-not-found error on the same day GitHub removes the ubuntu-24.04 image — the same class of breakage as the original ubuntu-latest migration, now on a known fixed date.

**Prevention**: Add a plan.md post-merge task: open a GitHub issue titled "Track ubuntu-24.04 runner retirement (benchmark.yml)" tagged `maintenance`, citing GHA's 60-day deprecation notice policy and listing the three affected jobs. Optionally, add a Renovate rule or dependabot config for GHA runner pinning. The plan's Risk Control table acknowledges this risk but takes no action; a tracking issue converts acknowledgment into a durable reminder.

**Severity**: P3 — Unlikely within the next year (ubuntu-24.04 is an LTS; retirement pressure is 1-2 years away) but catastrophic when it happens (all three Playwright benchmark jobs go dark simultaneously; the same benchmark failure-class recurs, now harder to diagnose because the team has forgotten why the runner was pinned).

---

## Risk Summary

| ID | Failure Mode | Severity |
|---|---|---|
| FM-1 | test.setTimeout zero-slack causes intermittent timeout | P1 |
| FM-2 | go-tier2 newly-compilable packages hit job timeout | P2 |
| FM-3 | frontend-bundle on ubuntu-latest breaks Playwright job chain | P2 |
| FM-4 | buf binary integrity unverified; silent bad codegen | P3 |
| FM-5 | ubuntu-24.04 retirement causes pinned jobs to fail | P3 |

**Counts**: 1× P1, 2× P2, 2× P3

**Top failure mode**: FM-1 (test.setTimeout zero overhead slack). It is the most likely failure — the adversarial review confirmed the arithmetic gap, the pitfalls research prescribed a 650s minimum, and the plan chose exactly 600s. Unlike FM-2 through FM-5, this failure does not require an external event (future OS retirement, supply-chain incident, slow first run); it is a latent flake baked into the timeout math that fires on any CI runner that takes its full 60s budget per iteration.

---

## Recommended Plan Changes

1. **FM-1 (must fix before merging)**: Story 1.2.1a — change `test.setTimeout(700_000)` (not 600_000).
2. **FM-2 (must do before merging)**: Add a pre-merge audit sub-task to Story 1.1.2: run representative newly-unblocked benchmark packages locally with `count=1` and bound their cost; raise go-tier2's `-timeout` and `timeout-minutes` if needed.
3. **FM-3 (optional, low effort)**: Pin `frontend-bundle` to `ubuntu-24.04` in the same PR; one line, eliminates cross-job dependency risk.
4. **FM-4 (optional, low effort)**: Add sha256 verification after the buf curl download in Story 1.1.2a.
5. **FM-5 (post-merge)**: Open a tracking GitHub issue for ubuntu-24.04 runner retirement monitoring.
