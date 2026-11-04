# Why Bazel

Bazel (bzlmod, version 9.x) is the build of record for Go, proto, ent, tmux/tymux and the web app. This page
explains the reasoning; the decision itself is
[ADR-001](../../project_plans/bazel-conversion/decisions/ADR-001-adopt-bazel-bzlmod-build-of-record.md).
The migration is provisional until the parity gate and cutover (Epics 7 and 8 of the conversion plan).

## The three drivers

The three drivers are co-equal, and each has a measured baseline in
`project_plans/bazel-conversion/implementation/baselines.md`.

1. **CI speed.** CI selected work with three hand-maintained mechanisms (a `grep -qE` regex in `build.yml`,
   `paths:` filters, `tools/ci/detect-feature-changes.sh`). A target graph makes "what is affected" a query
   (`rdeps`) instead of a rule someone must keep in sync.
2. **Hermeticity.** `go test` reads `$HOME`, the real tmux socket and the working directory. Bazel runs tests in
   a sandbox, which turns those into explicit declared inputs or failures with a known remedy (see
   [troubleshoot-bazel](../how-to/troubleshoot-bazel.md#state-4)).
3. **Worktree cost.** Over a hundred agent worktrees (120 were registered when the plan was written) each rebuild everything. One shared disk cache and
   repository cache across output bases is the lever; the cost target is T3 in the plan's improvement-targets table.

## Supersession of the ci-speed ADR

The earlier ci-speed project rejected Bazel in
[ADR-001](../../project_plans/ci-speed/decisions/ADR-001-reject-bazel-bzlmod-adoption.md). The conversion's
ADR-001 supersedes it and answers each rejection reason point by point; the owner's decision is recorded in
`project_plans/bazel-conversion/requirements.md` (2026-11-01). A narrow `rules_foreign_cc` tmux build had been
removed in `b51b60eb1` because it ran in WORKSPACE mode, which Bazel 9 broke; the new build is bzlmod only.

## Why some things stay outside Bazel

[ADR-005](../../project_plans/bazel-conversion/decisions/ADR-005-permanent-exceptions-outside-bazel.md) lists the
permanent exceptions: the otel auto-instrumentation variant, macOS codesigning and TCC identity, gates that need
git history or a whole-tree scan (`dupl --new-from-rev`, `proto-breaking`, jscpd), golangci-lint, release-please,
benchmarks, goreleaser and the Homebrew formula. The rationale is the same in each case: the tool either rewrites
the toolchain, needs the host keychain, or reads history, none of which a hermetic action can see. Each keeps a
defined interface to Bazel (usually "run after `make sync`"). The reference page
[bazel-targets-and-configs](../reference/bazel-targets-and-configs.md) holds the table.

## Why native Go tooling stays

Bazel is the build of record, but `go build`, `go vet`, `go test` and gopls remain supported after `make sync` for
editor tooling and quick loops, because the cost of breaking muscle memory is higher than the cost of keeping a
second path. A native result is never authoritative; see
[ADR-002](../../project_plans/bazel-conversion/decisions/ADR-002-generated-code-bazel-only-with-sync-gen.md).

## Limits on custom build code

Repo-owned Starlark is capped at about 500 lines and a rules fork is a last resort with a recorded removal
condition ([ADR-008](../../project_plans/bazel-conversion/decisions/ADR-008-rule-repo-fork-policy.md)), so the
build stays reviewable by one owner.
