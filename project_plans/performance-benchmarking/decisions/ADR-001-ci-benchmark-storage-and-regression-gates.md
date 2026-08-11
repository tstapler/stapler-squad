# ADR-001: CI Benchmark Storage and Regression Gate Strategy

**Status**: Superseded
**Date**: 2026-04-02
**Superseded by**: File-in-main baseline approach (see Implementation Note below)
**Context**: Performance Benchmarking feature

## Context

Claude-squad has 62 Go benchmark functions across 12 packages but none are run in CI. The `build.yml` workflow runs `go test -v ./...` only. There is no baseline storage, no regression detection, and no PR-blocking gates. The codebase needs a CI strategy that:

1. Stores benchmark baselines persistently across runs
2. Compares PR benchmarks against baselines with configurable thresholds
3. Comments on PRs and optionally fails CI on significant regressions
4. Supports both Go benchmarks and frontend metrics in a unified workflow
5. Runs on GitHub-hosted runners (2-vCPU Ubuntu, no GPU)

## Decision

Use `benchmark-action/github-action-benchmark@v1` with a dedicated `benchmarks` branch for baseline storage. Use dual thresholds (alert at 120%, fail at 150%) appropriate for shared CI runners. Run benchmarks on both `push:main` (baseline update) and `pull_request:main` (regression gate).

For statistical rigor on local A/B comparisons, use `benchstat` (golang.org/x/perf) with `-count=8` minimum. `benchstat` is not integrated into CI gates directly -- it is a developer tool for investigating regressions flagged by the action.

Frontend metrics (bundle size, Playwright throughput) use a second `benchmark-action` step with a separate `benchmark-data-dir-path` in the same job, following the Grafana dual-step pattern.

## Rationale

### Why `benchmarks` branch over Actions cache

- **Persistence**: Actions cache expires in 7-90 days; the `benchmarks` branch is permanent and git-auditable.
- **Visualization**: The action generates a Chart.js time-series page that can be served via GitHub Pages.
- **Debugging**: Historical data is browsable in git log when investigating regressions.
- **Cost**: One auto-push per main merge; negligible storage.

### Why 120%/150% thresholds (not 105%/110%)

Research findings (findings-pitfalls.md) document 10-30% coefficient of variation on GitHub-hosted runners due to CPU throttling and shared-resource contention. A 5% gate would produce constant false positives. The 120%/150% thresholds are appropriate for application code on shared runners:
- `alert-threshold: '120%'` -- post warning comment, catch moderate regressions
- `fail-threshold: '150%'` -- block merge, catch catastrophic regressions

### Why separate workflow file (not added to build.yml)

- Benchmarks can run 10-30 minutes with `-count=8` across 62 functions. Adding this to the build matrix (6 OS/arch combinations) would multiply CI time unacceptably.
- Benchmarks only need to run on `ubuntu-latest` (not cross-platform).
- Separation allows different trigger conditions and caching strategies.

## Alternatives Considered

1. **Actions cache only** (`use-github-action-cache: true`): Simpler setup, but cache expiration means baselines are lost after inactivity. Rejected for a project that may have weeks between commits.

2. **Workflow artifacts + benchstat**: Upload raw benchmark output as artifacts, download main's latest, run `benchstat` to compare. Most statistically rigorous, but requires custom scripting for PR comments and fail gates. Retained as a complementary local developer tool, not the primary CI mechanism.

3. **No CI benchmarks (status quo)**: Regressions are invisible until a developer manually profiles. Rejected as the primary motivation for this feature.

## Consequences

- A new `benchmarks` branch will be created (auto-pushed by the action on first run).
- The workflow requires `contents: write` permission to push baseline data.
- PRs from forks cannot push baselines (GitHub Actions security model); fork PRs will compare against the last main baseline only.
- Bundle size gates require `next build` in CI, adding Node.js setup to the benchmark workflow.

---

## Implementation Note — Deviation from This ADR

During implementation (commit `3211c0f`), `benchmark-action/github-action-benchmark` was replaced with a simpler file-in-main approach:

**What changed:**
- Baseline files committed directly to `main` under `benchmarks/go/`, `benchmarks/frontend/`, `benchmarks/e2e/` (no separate `benchmarks` branch).
- Go regression comparison uses `benchstat` inline in the CI shell step (not the action's built-in comparison).
- Threshold changed from the planned 120%/150% dual thresholds to a 10% `benchstat`-based exit-1 gate. This was increased to match GitHub runner noise more accurately in a follow-up revision.
- Chart.js visualization page (GitHub Pages) not implemented; comparisons are posted as PR comments in plain text.

**Why:**
- The `benchmarks` branch approach (gh-pages style) required an additional branch + Pages setup that added repo complexity for minimal gain at this project's scale.
- `benchstat` is already a required developer tool; using it directly in CI keeps the comparison logic transparent and auditable without a third-party action.
- File-in-main baselines are simpler to inspect, diff, and recover from (standard `git log` on main).

**Trade-offs accepted:**
- No time-series visualization (acceptable; regressions visible in PR comments).
- Slightly more brittle `benchstat` grep in the regression gate (mitigated by pinning benchstat version).
