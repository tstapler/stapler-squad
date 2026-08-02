# Verification Report: detector-plugins

**Date**: 2026-08-02
**Diff**: 32f504c8..36a951acb (16 files, +2640/-23), `session/detection/{plugins,detector_snapshot,plugin_watcher,registry,binary_detector,detector}.go` + tests, `session/claude_controller.go`, `session/instance_terminal.go`, `main.go`

## Technology Surface

| Technology | Files | Review approach |
|---|---|---|
| Go | all touched files | `go-development` skill (idiom) + `code-architecture-best-practices` skill (architecture) + `code-refactoring` skill (refactor candidates), 3 parallel agents |

## Layer 1 — Idioms

0 MUST FIX. 4 SUGGEST, 4 NITPICK. Two fixed inline (see Layer 2 below — the naming
collision was independently flagged by both the idiom and architecture agents).
Remaining SUGGEST/NITPICK items, all non-blocking follow-ups:

- `PluginLoadError.Field` overloads a human-readable key path with `"directory"`/
  `"file_count"` control-flow sentinels (stringly-typed). A `Fatal bool` field would
  be more typo-proof. Deliberate per plan.md's Hardening Addendum item 3, well-tested
  today.
- `rebuildSnapshot`/`LoadPluginDir` don't re-check `ctx` between files — a
  maximally pathological 200-file directory (each hitting the 500ms compile
  budget) could delay shutdown up to ~100s. Theoretical; the caps make this
  unrealistic in practice.
- Test-goroutine-wait gap noted in an early draft of `Test_InitPlugins` was
  resolved by the `InitPlugins_should_beNoOp_When_calledTwice` test added below,
  which exercises the watcher lifecycle across two calls directly.

## Layer 2 — Architecture

0 BLOCKER. 2 CONCERN, both addressed:

- **Fixed**: `DetectForProgram` (existing method) vs. new `DetectorForProgram`
  (new package func) — one-letter, same-package name collision, independently
  flagged by both the idiom and architecture review agents. Renamed the new
  func to `ResolveDetectorForProgram` across `detector_snapshot.go`,
  `detector_snapshot_test.go`, and `claude_controller.go`.
- **Fixed**: `DetectorProvenance`'s manual defensive-copy loop simplified to
  `maps.Clone` (stdlib, already imported elsewhere in this diff's tests).
- Follow-up, not fixed: `MergedRegistry`'s override-log line could mislabel a
  plugin-vs-plugin collision as a built-in override — not currently
  triggerable, since `LoadPluginDir` already rejects duplicate `binary_names`
  across plugin files before `MergedRegistry` ever sees them. Log-text-only,
  no behavior bug.

Plan/ADR compliance spot-checked and confirmed matching (ADR-002 copy-on-write
snapshot shape, Epic 2.4 wiring actually implemented not just claimed, ADR-003
schema v1, ADR-004 trust boundary and resource caps, `Register`'s panic
invariant untouched).

## Layer 3 — Correctness

All 8 acceptance criteria verified against the diff (see `report_progress`
evidence on the backlog item). `gofmt -l`, `go build`, `go vet`,
`golangci-lint run --enable=nilnil,staticcheck,ineffassign,govet`, and the
repo's custom linter all clean on every touched file. Full
`go test ./session/... -count=1`: 23 packages, 0 FAIL.
`go test -race ./session/detection/... -count=1`: clean.

**Known gap**: `make nil-safety`'s `nilaway` binary is broken in this
environment (built with go1.25, environment runs go1.26 — reproducible on
files this diff never touched, e.g. `main.go` itself pre-diff). Pre-existing
toolchain mismatch, not introduced by this feature. `go vet` (included in
golangci-lint's `govet`) is clean; the nil-guard pattern in
`lookupBinaryDetector` (`activeSnapshot.Load()`) was manually verified present.

**Security**: no auth/HTTP/DB/shell-command surface. No secrets in diff. RE2
regex compilation is ReDoS-safe by construction; file paths are fixed to
`os.ReadDir` entries of a known directory (no path traversal from plugin
content).

**Observability**: matches plan.md's Observability Plan — 16 structured
`log.Info`/`log.Warn` call sites verified present with the plan's specified
key names (load success/rejection/override/directory-scan-failure/
symlink-skip/watcher-fallback).

## Post-review addendum (2026-08-02)

The first `request_review` pass came back **PARTIAL**. The reviewer
cross-referenced `validation.md`'s Requirement → Test Mapping table against
the actual test files and found two promised tests genuinely absent (grep
confirmed):

- `EnsurePluginDir_should_stillReturnDirectory_When_seedFileWriteFails`
  (Hardening 2, backs AC5) — added to `Test_EnsurePluginDir` in
  `session/detection/plugins_test.go`. Forces the write failure by chmod'ing
  the plugin directory read-only after seeding a real plugin file, so the
  test also proves the directory stays scannable afterward, not just that
  `EnsurePluginDir` swallows the error.
- `InitPlugins_should_beNoOp_When_calledTwice` (Hardening 5, backs AC8) —
  added to `Test_InitPlugins`. Repoints `STAPLER_SQUAD_TEST_DIR` at a
  brand-new directory before the second `InitPlugins` call and asserts that
  directory's `detectors/` subfolder is never created — a concrete,
  observable proof the second call's body never runs, rather than trusting
  the `sync.Once` doc comment. Also confirms the *original* watcher from the
  first call is still the only one running (a plugin dropped in the first
  call's directory is still hot-reloaded).

Both tests pass; full `session/detection` suite and `-race` remain clean
after the addition. This file itself is the artifact the reviewer noted was
missing — it exists now, alongside this addendum recording what changed
after the first review cycle.
