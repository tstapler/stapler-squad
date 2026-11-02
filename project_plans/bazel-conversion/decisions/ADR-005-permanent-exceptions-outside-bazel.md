# ADR-005: What stays outside Bazel, and how Windows is handled

**Date**: 2026-11-01
**Status**: Accepted (Windows is an owner decision; the rest is the plan's recommendation from `research/features.md` section 4).

## Decision

Permanent exceptions (documented in `docs/reference/bazel-targets-and-configs.md`, tested by the weekly canary where applicable):

| Item | Why it stays | Interface to Bazel |
|---|---|---|
| otel auto-instrumentation variant (`build-otel-auto*`, tag `otelcauto`) | `otelc` rewrites the Go toolchain build; no Bazel analogue | Runs on `go build` after `make sync`; guard that the main build is unaffected |
| macOS codesign, TCC identity | Keychain access, non-sandboxable | `install-service`/`make _codesign-binary` signs a copy of the Bazel output at a stable path |
| Diff-based and history-based gates: `dupl --new-from-rev`, gocyclo/gocognit/funlen/revive new-code gate, `proto-breaking`, jscpd threshold, actionlint, shellcheck | Need git history or whole-tree scans | Wrapped; run after `make sync` |
| golangci-lint | Needs a normal Go tree | Wrapped; the 17 custom analyzers may move to `nogo` (Story 8.4) with the wrapped `make lint-custom` retained until the retention window ends |
| release-please, website/demo deploy workflows, benchmarks/pprof, harness (RealClaude) tests | Not build-graph concerns | Unchanged |
| goreleaser | Decision deferred to spike 3.7a: feed it Bazel binaries if `builder: prebuilt` is available, else keep its own `CGO_ENABLED=0 go build` and record it here as a permanent exception | Parity check via `goreleaser-check.yml` |
| Homebrew `Formula/stapler-squad.rb` | Written by goreleaser (`brews.directory: Formula`) from release assets | Stays goreleaser-owned; Task 3.7c snapshot dry run asserts asset names, URLs and sha256 match |
| Feature registry (`docs/registry/features/*.json`, `make registry-generate`) | Committed scan-derived artifact, not gitignored codegen | Stays a make target; CI fails on a non-empty `git diff -- docs/registry` (ADR-002 item 6, Task 2.4c) |

**Windows (owner decision 2026-11-01): cross-compile only.** `windows_amd64` binaries via a platform transition (`go_cross_binary`); no Windows tests under Bazel; windows-only packages carry `target_compatible_with` so `//...` on Linux/macOS excludes them. This corrects `research/architecture.md` section 3, which said "dropped".

## Consequences

- "Bazel is the build of record" means the repo's binaries and tests; the table above is the complete list of exceptions. Adding to it requires amending this ADR.
- Installing/running the live service never goes through `bazel run` (CLAUDE.md rule on `:8543`).
