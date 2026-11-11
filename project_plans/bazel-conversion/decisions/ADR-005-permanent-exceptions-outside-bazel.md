# ADR-005: What stays outside Bazel, and how Windows is handled

**Date**: 2026-11-01
**Status**: Accepted (Windows is an owner decision; the rest is the plan's recommendation from `research/features.md` section 4).

## Decision

Permanent exceptions (documented in `docs/reference/bazel-targets-and-configs.md`, tested by the weekly canary where applicable):

| Item | Why it stays | Interface to Bazel |
|---|---|---|
| macOS codesign, TCC identity | Keychain access, non-sandboxable | `install-service`/`make _codesign-binary` signs a copy of the Bazel output at a stable path |
| Diff-based and history-based gates: `dupl --new-from-rev`, gocyclo/gocognit/funlen/revive new-code gate, `proto-breaking`, jscpd threshold, actionlint, shellcheck | Need git history or whole-tree scans | Wrapped; run after `make sync` |
| golangci-lint | Needs a normal Go tree | Wrapped; the 17 custom analyzers may move to `nogo` (Story 8.4) with the wrapped `make lint-custom` retained until the retention window ends |
| release-please, website/demo deploy workflows, benchmarks/pprof, harness (RealClaude) tests | Not build-graph concerns | Unchanged |
| goreleaser | Keeps its own `CGO_ENABLED=0 go build`, routed through the otelc weave shim `scripts/goreleaser-go-tool.sh` (Task 3.7a, "Goreleaser decision" below): `builder: prebuilt` is GoReleaser Pro only | Parity check via `goreleaser-check.yml` |
| Homebrew `Formula/stapler-squad.rb` | Written by goreleaser (`brews.directory: Formula`) from release assets | Stays goreleaser-owned; Task 3.7c snapshot dry run asserts asset names, URLs and sha256 match |
| Feature registry (`docs/registry/features/*.json`, `make registry-generate`) | Committed scan-derived artifact, not gitignored codegen | Stays a make target; CI fails on a non-empty `git diff -- docs/registry` (ADR-002 item 6, Task 2.4c) |

**otel auto-instrumentation is not an exception any more (owner decision 2026-11-03, PROVISIONAL on branch `sdd/bazel-otel-default`).** The one `stapler-squad` binary is always the `otelc`-woven build, in make, Bazel and releases; the opt-in `stapler-squad-otel` split, `build-otel-auto*` and `otel-auto-isolation-guard` are retired. Under Bazel the default `//:stapler-squad` is an `otel_weave_hermetic` rule (`tools/bzl/otel_weave_hermetic.bzl`, spike S15b, verdict FallbackWorkaround): a **sandboxed, remote-cacheable** action that runs the pinned `otelc` toolexec build from a committed `otelc setup` overlay (`tools/bzl/otel_overlay/`) with declared inputs only (Go SDK, sources from the `:stapler-squad_lib` closure, generated proto/ent, web dist, embed staging, a module-cache tarball). It replaces the S15 `otel_weave` action, which was unsandboxed and never cached. Residual non-hermetic parts: the module cache comes from a networked, go.sum-verified `go mod download` action, cgo uses the host `gcc`, and every cache miss is a cold Go build cache. Windows cross builds use the same rule (`--platforms=`, cgo off).

**Windows (owner decision 2026-11-01): cross-compile only.** `windows_amd64` binaries via a platform transition (`go_cross_binary`); no Windows tests under Bazel; windows-only packages carry `target_compatible_with` so `//...` on Linux/macOS excludes them. This corrects `research/architecture.md` section 3, which said "dropped".

**Amended by ADR-009 (Proposed, 2026-11-02)**: Playwright e2e is no longer outside the build graph; execution and selection move into Bazel (Epic 12). Everything in the table above still stays outside Bazel. What remains outside for e2e is only the pinned Playwright browser image, `bun install` of `tests/e2e`, and the nightly-only visual/perf/secondary-browser projects.

## Goreleaser decision (Task 3.7a, spike run 2026-11-03)

Decision (goreleaser): keep-go-build

Evidence:
- `builder: prebuilt` is documented under the Pro heading: the page <https://goreleaser.com/customization/builds/prebuilt/> opens with "GoReleaser Pro" (fetched 2026-11-03). `.goreleaser.yaml` is `version: 2.9` and `goreleaser-check.yml` and `release.yml` use the OSS `goreleaser/goreleaser-action@v6` with no license key, so consuming Bazel binaries would need a paid license, an owner decision this plan does not make.
- The cross-compile half is available without goreleaser: `go_cross_binary` already produces `windows_amd64` (Epic 6, `tools/bzl/windows/BUILD.bazel`), so a later switch to Bazel-built release binaries is technically possible once a Pro license exists.
- Goreleaser builds all three binaries (`stapler-squad`, `ssq-mux`, `ssq-hooks`) for 3 OS x 2 arch with `CGO_ENABLED=0`; the OSS path keeps that unchanged, so the CGO=0 guard in `goreleaser-check.yml` stays valid and Task 3.7b reduces to this exception note.

Revisit only if the owner buys GoReleaser Pro; then switch to `builder: prebuilt` fed from `//:stapler-squad` cross builds and amend this ADR.

## Consequences

- "Bazel is the build of record" means the repo's binaries and tests; the table above is the complete list of exceptions. Adding to it requires amending this ADR.
- Installing/running the live service never goes through `bazel run` (CLAUDE.md rule on `:8543`).

## Changelog

- 2026-11-03: otel auto-instrumentation removed from the permanent-exception table; the single `stapler-squad` binary is always `otelc`-woven (make, Bazel `//:stapler-squad` via the S15b `otel_weave_hermetic` action, GoReleaser). Supersedes the "opt-in, structurally isolated" decision in `project_plans/go-auto-instrumentation/decisions/ADR-003-structural-opt-in-separate-binary.md`. Owner decision, provisional on branch `sdd/bazel-otel-default`.
