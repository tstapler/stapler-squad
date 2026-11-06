# Slow CI suites and how each is selected (ADR-009, Story 12.7)

Per-suite p50 comes from `tools/bzl/baseline/measured.json` (`ci_per_workflow`, whole workflow wall time); `-` means
no per-suite measurement exists yet. Classification values: `rdeps` (a Bazel test selected by `bazel query rdeps`),
`manifest` (a Playwright spec selected by the Epic 12 observed-dependency manifest), or `full: <reason>`.
`e2eslice check-slow-suites` fails when a suite over 300 s p50 has no classification.

`go run ./tools/bzl/e2e/slowsuites` (or `bazel run //tools/bzl/e2e/slowsuites`) fails when a suite stops running in its
own CI job. The "Own CI job" column is `workflow.yml#job`, optionally `~text` for a command that must still appear in
a non-comment workflow line, or `-` for a suite with no workflow of its own (not checked). The checker fails when:

- the workflow file or the named job is missing, or the `~text` marker is gone;
- that workflow invokes `bazel`/`bazelisk` in a non-comment line (the suite moved into the Bazel path);
- a `bazel*.yml` workflow reuses it with `uses:`.

Jest has no workflow of its own: it runs as the `Jest` step of the `lint` job in `lint.yml`, so that is its row.

| Suite | p50 s | Classification | Status | Own CI job |
|---|---|---|---|---|
| build.yml Go unit and integration tests | 2230 | rdeps | Bazel-native tests from Epics 3-4; selection by `affected-targets.sh` (Story 7.3, not merged) | `build.yml#test-affected~gotestsum` |
| build.yml e2e (Bun build) shards | 2230 | manifest | Epic 12 shadow workflow `bazel-e2e.yml`; manifest not yet captured | `build.yml#e2e-bun-shard` |
| lint.yml Jest step | - | rdeps | Bazel Jest targets from Epics 3-4 | `lint.yml#lint~npx jest` |
| lint.yml | 1033 | full: golangci-lint and custom analyzers stay outside Bazel (ADR-005) | unchanged | `lint.yml#lint` |
| mcp-integration.yml | 590 | rdeps | NOT wired: needs `affected-targets.sh` (Story 7.3); the workflow's `paths:` filter stays the pre-gate | `mcp-integration.yml#mcp-integration` |
| ux-analysis.yml axe sweeps | - | manifest | Playwright specs (`a11y-*.spec.ts`); NOT wired, same dependency | `ux-analysis.yml#ux-analysis` |
| ux-analysis.yml Lighthouse | - | full: performance score needs a full page set, not a slice | unchanged | `ux-analysis.yml#ux-analysis` |
| benchmark.yml | - | full: benchmarks measure the whole binary | unchanged | `benchmark.yml#go-tier1` |
| nightly.yml secondary browsers and visual regression | - | full: run only nightly by design (ADR-009 amendment) | unchanged | `nightly.yml#e2e-secondary-browsers-bun` |
| RealClaude harness | - | full: external service, no declared inputs | unchanged | - |
