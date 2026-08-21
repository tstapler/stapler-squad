# Backend DI Refactor — Requirements

## Problem Statement

The backend has three compounding maintainability problems that compound each other:

1. **Setter injection with no validation**: `server/dependencies.go` has 16+ `Set*` calls across `BuildServiceDeps` and `BuildRuntimeDeps` with zero validation. A missing setter silently produces a nil pointer that panics at runtime — potentially minutes after startup — rather than failing fast at boot.

2. **Monolithic `session/instance.go`**: 3168 lines, 136 functions, spanning session lifecycle, tmux management, git worktrees, approval handling, status/state machine, terminal content, serialization, checkpoints, and tags. It is the single hardest file to review or test in the codebase.

3. **Concrete type coupling at the server/session boundary**: `server/services` consumes `*session.Instance`, `*session.Storage`, `*session.ReviewQueue`, and `*session.ReviewQueuePoller` as concrete types in most service constructors. This makes unit testing services in isolation difficult — you must construct or heavily mock the full session package.

The project already has `pkg/warren` — a purpose-built DI coordinator with `Wire`/`Set` validators — but it is used only in `main.go` for phased startup. The Wire validation layer is not applied to the setters in `BuildServiceDeps` or `BuildRuntimeDeps`.

## Goals

### G1 — Warren Wire Coverage (high value, low risk)
Every `Set*` call in `server/dependencies.go` must be wrapped in a `warren.Wire` validator. Missing or nil setters must return an error at startup, not panic at runtime.

**Acceptance criteria:**
- `BuildServiceDeps` and `BuildRuntimeDeps` both call `w.Validate()` (or `MustValidate()`) after wiring
- Removing any `warren.Set(...)` call causes the application to exit with a clear error on startup
- `server/dependencies_test.go` tests that nil setters produce a descriptive error (not just a nil CoreDeps check)

### G2 — Split `session/instance.go` (moderate risk, high readability payoff)
Split `session/instance.go` into domain-focused files within the same `session` package. No API changes — Go allows a type's methods to span multiple files in the same package.

**Target file layout:**
- `session/instance.go` — struct definition, constructors (`NewInstance`, `NewInstanceWithCleanup`), core lifecycle (`Start`, `Stop`, `Pause`, `Resume`, `Destroy`, `Kill`)
- `session/instance_status.go` — status, state machine (`setStatus`, `transitionTo`, `GetCategoryPath`, `MarkViewed`, `MarkAcknowledged`, etc.)
- `session/instance_tmux.go` — tmux session management (`initTmuxSession`, `buildLaunchCommand`, `KillSession`, `GetTmuxSessionName`, PTY access helpers)
- `session/instance_worktree.go` — git worktree management (`setupFirstTimeWorktree`, `resolveStartPath`, `GetEffectiveRootDir`, `Workspace`, `CleanupWorktree`, `RepoName`)
- `session/instance_approval.go` — approval and review queue (`SetReviewQueue`, `SetStatusManager`, `MarkNeedsApproval`, approval-related setters)
- `session/instance_serialization.go` — `ToInstanceData`, `FromInstanceData`, `InstanceData` type
- `session/instance_terminal.go` — terminal content (`Preview`, `CaptureCurrentState`, `GetDiffStats`, scrollback-adjacent methods)
- `session/instance_checkpoint.go` — checkpoint methods
- `session/instance_tags.go` — tag management methods

**Acceptance criteria:**
- `go build ./session/...` passes
- All existing `session/...` tests pass unchanged
- No method moved to wrong file (verified by reviewer)
- Each sub-file is under 400 lines

### G3 — Interface extraction at the server/session boundary (aggressive, highest long-term payoff)
Extract narrow interfaces for the concrete session types most consumed by `server/services`. The goal is to make service unit tests not require a real `*session.Instance`, `*session.Storage`, or `*session.ReviewQueue`.

**Priority interfaces to extract:**
- `session.InstanceReader` — the read-only subset of `*session.Instance` needed by most services (Title, Status, Branch, Path, Tags, etc.)
- Confirm `session.InstanceStore` (already exists in `session/storage.go:159`) is used consistently in service constructors instead of `*session.Storage`
- `session.ReviewQueueWriter` — the write-only subset used by services that only add items to the queue

**Acceptance criteria:**
- At least one service in `server/services` previously taking `*session.Storage` directly now takes `session.InstanceStore`
- At least one service previously taking `*session.ReviewQueue` directly now takes a narrower interface
- New interface is tested via a mock in that service's `_test.go`
- No service interface is wider than what the service actually uses (Interface Segregation)

### G4 — Test coverage for the wiring layer
The current `server/dependencies_test.go` has 3 tests that only check nil-guard panics. This should grow to cover the Warren Wire validation behaviour.

**Acceptance criteria:**
- At least 5 new tests in `server/dependencies_test.go` or `server/wiring_test.go`
- Tests cover: nil value skips Set and fails Validate; all required setters applied produces no error; phase ordering is enforced
- `pkg/warren` package test coverage stays at or above current level

## Non-Goals

- Migrating to constructor injection (Warren setter validation achieves the same safety net with a much smaller changeset)
- Moving packages (e.g., splitting `session/` into sub-packages) — same-package file splits only
- Changes to the frontend or proto definitions
- Changes to the ent ORM schema
- Splitting `server/services/session_service.go` (2837 lines) — this is a follow-on candidate but out of scope for this PR

## Constraints

- **No breaking API changes**: all public types and functions remain at their current import paths
- **Must pass `make ci`**: includes lint, tests, and build
- **Session package tests must all pass** — these are the primary regression guard
- **Branch**: `stapler-squad-backend-refactor` (already checked out)
- **No changes to `.claude/rules/session-creation-registry.md` touchpoints** — session creation flow is not being changed

## Prioritization (bang-for-buck order)

| Priority | Change | Risk | Payoff |
|---|---|---|---|
| 1 | Warren Wire validators in `BuildServiceDeps`/`BuildRuntimeDeps` | Low | Immediate: nil panics become startup errors |
| 2 | Split `session/instance.go` into sub-files | Moderate | High: reviewability, focused test scoping |
| 3 | Interface extraction for `*session.ReviewQueue` consumer(s) | Moderate | High: enables isolated unit tests |
| 4 | Widen wiring tests in `server/dependencies_test.go` | Low | Medium: regression safety for G1 |
| 5 | Confirm `session.InstanceStore` used consistently | Low | Medium: consistency, easier future mocking |
