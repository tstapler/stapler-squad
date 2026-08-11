# ADR-003: Frontend Performance Measurement Strategy

**Status**: Proposed
**Date**: 2026-04-02
**Context**: Performance Benchmarking feature

## Context

The claude-squad web UI is a React 19 / Next.js 15 application that renders terminal output via xterm.js with a WebGL addon. Performance measurement for the frontend faces several constraints:

1. **WebGL is unmeasurable in CI**: GitHub Actions runners have no GPU. xterm.js falls back to SwiftShader (software rendering), which is 10-50x slower than production. Frame rate measurements are meaningless.
2. **Playwright rAF throttling**: `requestAnimationFrame` callbacks are throttled in headless Chrome, making FPS measurement unreliable.
3. **Bundle size is the most stable metric**: Deterministic, not affected by runner CPU/GPU variability.
4. **Terminal throughput is measurable**: Writing ANSI data to xterm.js and measuring time-to-settle is CPU-bound and reproducible even in headless CI.

The existing Playwright stress tests (`web-app/tests/e2e/terminal-stress/`) are pass/fail correctness tests, not regression-tracking benchmarks.

## Decision

Measure three frontend dimensions in CI, skip GPU-dependent metrics:

### 1. Bundle Size Regression Gate (size-limit)

Hard limits on JavaScript bundle output. Most deterministic frontend CI metric. Fails PR if bundle grows beyond threshold.

### 2. Terminal Throughput Benchmark (Playwright + User Timing API)

Convert one existing stress test into a regression-tracking benchmark using `performance.mark()` / `performance.measure()` inside a single `page.evaluate()` call. Output in `customBiggerIsBetter` format for `github-action-benchmark`.

Discard first 2 warmup runs. Collect 10 samples. Gate on P95.

### 3. Lighthouse CI Scores (advisory, not blocking)

Run Lighthouse CI against `next build && next start` for page-level metrics (LCP, TTI, CLS). Advisory only -- not a merge blocker -- because Lighthouse scores vary 5-15% between runs on shared runners.

### Explicitly Skipped

- **WebGL FPS**: Cannot be measured accurately without a real GPU. Not included in CI.
- **Core Web Vitals (FID/INP)**: Require real user interaction events that Playwright synthetic clicks do not reliably trigger.
- **React render profiling**: React DevTools profiling requires headed Chrome with React DevTools extension. Deferred to manual profiling workflow.

## Rationale

### Why size-limit over webpack-bundle-analyzer

`size-limit` integrates directly into CI with pass/fail semantics. `webpack-bundle-analyzer` produces visual reports useful for manual investigation but cannot gate CI. Both can coexist.

### Why terminal throughput is the proxy for rendering performance

The ANSI-to-screen pipeline (ConnectRPC stream to React state to xterm.js write to render) is the application's primary performance-sensitive path. Measuring the CPU-bound portion captures regressions in the data pipeline even without GPU measurement.

### Why Lighthouse is advisory only

Lighthouse score variance on GitHub Actions runners (5-15% between runs with identical code) means a tight gate would produce false positives. Valuable for tracking trends, not for blocking merges.

## Alternatives Considered

1. **Skip all frontend benchmarks**: Rely on Go-side benchmarks only. Rejected because terminal rendering and bundle size are critical user-facing concerns.
2. **Self-hosted runner with GPU**: Would enable real WebGL measurement but adds infrastructure maintenance burden. Deferred.
3. **Chrome DevTools Protocol metrics in CI**: Useful but coarse-grained and noisy on shared runners. Retained as a secondary investigation tool.

## Consequences

- `size-limit` added as a devDependency in `web-app/package.json`.
- One Playwright test converted from pass/fail stress test to regression-tracking benchmark with JSON output.
- Lighthouse CI added as an optional CI step (non-blocking).
- Bundle size limits must be updated when intentional feature additions increase size.
- No WebGL performance data in CI -- GPU regressions require manual testing.
