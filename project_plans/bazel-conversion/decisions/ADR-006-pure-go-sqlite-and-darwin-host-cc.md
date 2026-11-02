# ADR-006: Pure-Go builds on Linux (modernc sqlite); host Xcode toolchain for darwin cgo

**Date**: 2026-11-01
**Status**: Accepted (resolved by research; owner open question closed in requirements.md).

## Evidence

- Production code imports only `modernc.org/sqlite` (`session/ent_repository.go:38`, `server/services/*.go`). `mattn/go-sqlite3` is imported by two test files only: `session/ent_repository_backlog_test.go:16`, `session/workflow_enabled_field_migration_test.go:8` (VERIFIED, `research/stack.md` section 0.2).
- Real `import "C"` exists only in three darwin files: `infoplist_darwin.go`, `session/tmux/socket_owner_darwin.go`, `session/procinfo/openfiles_darwin.go`.
- `.goreleaser.yaml` already ships `CGO_ENABLED=0` binaries, so dev and release builds already differ.

## Decision

1. Move the two test files to `modernc.org/sqlite` and drop `mattn/go-sqlite3` from `go.mod` (Task 1.1b). Linux and CI use `--@io_bazel_rules_go//go/config:pure` with no C toolchain for Go code.
2. darwin builds use native cgo with the host Xcode toolchain on macOS runners (`--config=macos`); no hermetic C toolchain, no macOS cross-compilation from Linux. Exception recorded: hermeticity of the darwin binary depends on Xcode CLT.
3. tmux (a C project) is built per spike S5; if it uses host `cc` that is a recorded single-target hermeticity exception.
4. `toolchains_llvm` stays out of v1; revisit only if Linux glibc drift between Manjaro and CI shows up in P5 comparisons.

## Consequences

- Removes the cgo requirement from the Linux dependency graph and from `-race` runs on non-darwin (race still needs a C compiler on Linux; recorded in Story 1.4).
- Windows cross-compile (ADR-005) is trivially pure Go.
