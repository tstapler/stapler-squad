# Validation Plan: cold-restart-uuid-recovery

**Date**: 2026-08-10

## Happy Path Scenario
Given a revived session (`Start(firstTimeSetup=false)`) with a dead tmux pane, an empty in-memory `ConversationUUID`, and a same-path conversation JSONL present on disk under `~/.claude/projects/<encoded-path>/`, when the session cold-restarts, then it launches with `--resume <uuid-from-jsonl>` instead of silently starting a brand-new conversation.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1 (AC1): pre-launch recovery runs before `initTmuxSession()`/`buildLaunchCommand()`, so a disk-recovered UUID is used to build `--resume` (Epic 1.1 / Story 1.1.1 / Task 1.1.1a-b) | `session/instance_cold_restore_test.go` | `TestColdRestore_WithoutUUID_RecoversFromJSONL` | Integration | Happy path — dead tmux, empty in-memory UUID, matching JSONL on disk → `LaunchCommand` contains `--resume '<uuid>'` and `GetConversationUUID() == "<uuid>"` |
| REQ-2 (AC2): if recovery finds nothing, the session still starts fresh — not a hard failure (Story 1.1.1, second Given/When/Then) | `session/instance_cold_restore_test.go` | `TestColdRestore_WithoutUUID` (existing, unmodified — re-run post-fix to confirm no regression) | Integration | Error/negative path — dead tmux, empty in-memory UUID, no matching JSONL → `LaunchCommand` has no `--resume`, `Start` returns `nil`, `Status == Running` |
| REQ-3 (AC3): guard suppresses resurrecting a JSONL that predates an explicit `ClearConversationState()`, with a diagnosable log line (Epic 1.1 / Story 1.1.2 / Task 1.1.2d) | `session/instance_cold_restore_test.go` | `TestTryExtractConversationUUID_SkipsStaleJSONLAfterClear` | Unit | Error/negative path — fixture JSONL mtime `conversationClearedAt - 1h` → `tryExtractConversationUUID()` leaves `claudeSession` nil or `ConversationUUID == ""`; DEBUG log `"tryextractconversationuuid: found jsonl predates last explicit clear, skipping recovery"` emitted, distinguishable from the pre-existing `"no jsonl file found"` line |
| REQ-3 (AC3) — guard is one-sided, not a permanent recovery kill switch (Story 1.1.2, second Given/When/Then) | `session/instance_cold_restore_test.go` | `TestTryExtractConversationUUID_RecoversJSONLNewerThanClear` | Unit | Happy path — fixture JSONL mtime `conversationClearedAt + 1h` → `tryExtractConversationUUID()` sets `ConversationUUID` to the fixture's UUID |
| REQ-3 (AC3) — supporting mechanism: `DetectByPath` must expose the mtime the guard compares against (Task 1.1.2c) | `session/history_detector_test.go` | `TestHistoryFileDetector_DetectByPath_PicksMostRecentWhenMultiple` (extended with one added assertion) | Unit | Happy path — `DetectByPath` populates `HistoryFileInfo.ModTime` with the winning candidate's on-disk mtime (`assert.WithinDuration`) |
| REQ-4 (AC4): regression test closing the `TestColdRestore_WithoutUUID` coverage gap — tmux dead, empty in-memory UUID, same-path JSONL on disk → revived session launches with `--resume <uuid-from-jsonl>` (Epic 1.2 / Story 1.2.1 / Task 1.2.1a) | `session/instance_cold_restore_test.go` | `TestColdRestore_WithoutUUID_RecoversFromJSONL` (same test as REQ-1 — this is the exact regression the coverage gap in requirements.md names) | Integration | Happy path — see REQ-1 |
| REQ-5 (AC5): no change to `session-resume-uuid-fix` behavior — a session with an already-stored UUID must not have it overwritten by a newer JSONL from a different session (Story 1.2.2, first Given/When/Then) | `session/instance_workspace_test.go` | `TestTryExtractConversationUUID_SkipsWhenAlreadyHasID` (existing, unmodified — Task 1.2.3a re-runs it to confirm Story 1.1.2's changes don't regress it) | Unit | Happy path — `Instance{claudeSession: &ClaudeSessionData{ConversationUUID: existingID}}` → `tryExtractConversationUUID()` leaves `ConversationUUID == existingID` unchanged |
| REQ-6 (AC6): `make quick-check` (build + test + lint) stays green (Epic 1.2 / Story 1.2.3 / Task 1.2.3a-b) | N/A — validation gate, not a test file | Targeted run: `go test ./session -run "TestTryExtractConversationUUID\|TestColdRestore\|TestHotRestore\|TestHistoryFileDetector" -v`, then `make quick-check` | Integration / CI gate | Full path — all targeted tests pass, then build+test+lint exits `0` with no new lint findings |

## UX Acceptance Tests
N/A — backend reliability fix, no user-facing UI surface in scope (see requirements.md Out of Scope: full UI "conversation could not be resumed" banner is a stretch goal, not in this plan).

## Test Stack
- **Unit**: Go stdlib testing + testify (assert/require)
- **Integration**: Go stdlib testing + real tmux (see `instance_cold_restore_test.go`'s `checkTmuxAvailable`/`coldRestoreSocket` helpers)
- **E2E / UX**: N/A

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- All public service methods: happy path + error paths covered
- All external integrations: unit mocked + at least one integration test
