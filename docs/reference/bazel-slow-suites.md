# Slow CI suites and how each is selected (ADR-009, Story 12.7)

Per-suite p50 comes from `tools/bzl/baseline/measured.json` (`ci_per_workflow`, whole workflow wall time); `-` means
no per-suite measurement exists yet. Classification values: `rdeps` (a Bazel test selected by `bazel query rdeps`),
`manifest` (a Playwright spec selected by the Epic 12 observed-dependency manifest), or `full: <reason>`.
`e2eslice check-slow-suites` fails when a suite over 300 s p50 has no classification.

| Suite | p50 s | Classification | Status |
|---|---|---|---|
| build.yml Go unit and integration tests | 2230 | rdeps | Bazel-native tests from Epics 3-4; selection by `affected-targets.sh` (Story 7.3, not merged) |
| build.yml e2e (Bun build) shards | 2230 | manifest | Epic 12 shadow workflow `bazel-e2e.yml`; manifest not yet captured |
| build.yml Jest | - | rdeps | Bazel Jest targets from Epics 3-4 |
| lint.yml | 1033 | full: golangci-lint and custom analyzers stay outside Bazel (ADR-005) | unchanged |
| mcp-integration.yml | 590 | rdeps | NOT wired: needs `affected-targets.sh` (Story 7.3); the workflow's `paths:` filter stays the pre-gate |
| ux-analysis.yml axe sweeps | - | manifest | Playwright specs (`a11y-*.spec.ts`); NOT wired, same dependency |
| ux-analysis.yml Lighthouse | - | full: performance score needs a full page set, not a slice | unchanged |
| benchmark.yml | - | full: benchmarks measure the whole binary | unchanged |
| nightly.yml secondary browsers and visual regression | - | full: run only nightly by design (ADR-009 amendment) | unchanged |
| RealClaude harness | - | full: external service, no declared inputs | unchanged |
