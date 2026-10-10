# Validation Plan: program-env-not-applied

**Date**: 2026-09-24

## Happy Path Scenario
Given a custom program registered via `UpsertProgramConfig` with a registered `env` var (e.g. `netflix-model-gateway` → `ANTHROPIC_BASE_URL`), when a new `SESSION_TYPE_NEW_WORKTREE` session is created with that program, then the registered env var is present both in `tmux show-environment`'s session-scoped table and in the spawned pane process's actual environment (verified via `printenv` inside the pane).

## Requirement → Test Mapping

This plan is dominated by one committed integration test (Epic 1.1) proving four of the five
ACs at once, plus a dedicated verification-only task for AC4 — not a uniform "1 unit + 1 error +
1 integration per requirement" grid. The table below reflects that shape rather than forcing
artificial rows.

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: registered env var reaches the spawned process (`printenv` inside the pane) | `server/services/session_service_create_test.go` | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (Task 1.1.1c's `send-keys "printenv ..."` + `capture-pane` assertion) | Integration | Register `netflix-model-gateway` with `ANTHROPIC_BASE_URL`, create a real `SESSION_TYPE_NEW_WORKTREE` session via the real `CreateSession` RPC, poll to `Active`, run `printenv` in the live pane via `tmux send-keys`/`capture-pane`, assert the value appears. |
| AC2: `tmux show-environment` shows the var in the session-scoped table | `server/services/session_service_create_test.go` | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (Task 1.1.1c's `show-environment` assertion) | Integration | Same fixture as AC1; shell out to `tmux -L <socket> show-environment -t <session>` and assert `ANTHROPIC_BASE_URL=http://127.0.0.1:47000` is present. |
| AC3: a regression test exists that would have caught the original bug (proven, not assumed) | `server/services/session_service_create_test.go` | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` + Task 1.1.1d's red/green procedure | Integration (+ manual red/green proof, not itself a committed test) | Task 1.1.1d reverts `session/instance_tmux.go` to the pre-fix `SetExtraEnv([]string{"STAPLER_SESSION_UUID=..."})`-only behavior in a scratch worktree/stash, confirms the test in Task 1.1.1c reports `FAIL`, restores HEAD, confirms `PASS` again. Contingency: if it does *not* fail pre-fix, halt and escalate — the test doesn't exercise the bug. |
| AC4: no regression to `claudeSettingsEnvOverrideArgs()` (#852) — custom program env still wins over global `~/.claude/settings.json` | `session/instance_tmux_test.go` | `TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars`, `TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars` (pre-existing, unmodified) | Verification (run existing suite, no new test) | Task 1.4.1a runs `go test ./session -run TestClaudeSettingsEnvOverrideArgs -v` after all Phase 1 changes land and records both subtests `PASS` as evidence — no code in this plan touches `resolveExtraEnvVars`/`claudeSettingsEnvOverrideArgs`. |
| AC5: root cause identified and documented | `server/services/session_service_create_test.go` | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession`'s doc comment (Task 1.1.1c) | Documentation (attached to the Epic 1.1 test) | Doc comment states the actual defect (pre-fix `initTmuxSession` called `SetExtraEnv` with only `STAPLER_SESSION_UUID`; no code path merged `ResolveProgramConfig(...).EnvVars` in at all, since `resolveExtraEnvVars`/`buildExtraEnv` didn't exist pre-fix) and names fix commit `cdfd4e5cf2` (merged 2026-09-21), not reproducible on HEAD as of `dd1848f9b`. |

### Beyond the literal ACs: hardening tests (plan.md Epic 1.2/1.3, per their own scope notes)

Not required to close AC1-5, but part of plan.md's committed task list — included here so the
validation matrix matches what actually ships.

| Story | Test File | Test Name | Type | Scenario |
|-------|-----------|-----------|------|----------|
| 1.2.1: resume-after-pause picks up an env var added *after* session creation | `server/services/session_service_create_test.go` | Not explicitly named in plan.md (Task 1.2.1a) — see Step 3 note below | Integration | Reuse Epic 1.1's fixture; `Pause()` the instance (kills tmux, forcing the dead-tmux rebuild branch), register a *new* env key via a second `UpsertProgramConfig` call, `Resume()`, assert `tmux show-environment` now includes the new key. |
| 1.2.2: `initTmuxSession`'s reuse guard is correct when only `IsBackendProcessAlive()` (not `IsAlive()`) is true | `session/instance_tmux_test.go` | `TestInitTmuxSession_ReuseViaBackendProcessAliveOnly` | Unit (characterization, `mockTmuxManager` + `NewTmuxBackend`) | `HasSession()==true, IsAlive()==false, IsBackendProcessAlive()==true` → `initTmuxSession()` reuses the session; assert `TmuxManager().SetSession` is not called a second time (i.e. `wireTmuxSession`/`buildExtraEnv` did not re-run). |
| 1.2.3: `-e KEY=VALUE` survives both tmux argv-construction paths | `session/tmux/tmux_session_start_test.go` (new file) | `TestTmuxNewSessionArgs_IncludesExtraEnv` | Unit (table-driven, `MockCmdExec` spy capturing `cmd.Args`) | Two table cases (`start()`'s inline loop, `newSessionArgs()`) both assert the same `-e ANTHROPIC_BASE_URL=...` pair appears in the constructed argv, guarding against the two loops silently diverging. |
| 1.2.4: `-e KEY=VALUE` survives `wrapRemoteCommand`'s SSH rewrite | `session/tmux/remote_env_test.go` | New case appended to existing `TestWrapRemoteCommand` table | Unit (table-driven, pure function) | `wrapRemoteCommand("tmux", cmdArgs)` with a `-e ANTHROPIC_BASE_URL=...` pair in `cmdArgs` returns it unchanged, in order, in `wantArgs`. |
| 1.3.1: client sends program `id`, not resolved `command`, to `CreateSession` | `web-app/src/lib/hooks/useAvailablePrograms.test.ts` | Existing fixture strengthened (no new `it` block) | Unit (Jest, React Testing Library `renderHook`) | Mock fixture changed so `id` (`"netflix-model-gateway"`) and `command` (`"claude"`) differ (previously both `"aider"`, unable to distinguish); assert `result.current[0].value === "netflix-model-gateway"`, not `"claude"`. |

## UX Acceptance Tests
N/A — no user-facing surface; this is a backend/test-only regression-coverage item.

## Test Stack
- **Unit**: Go `testing` + `testify` (`assert`/`require`) for Go-side unit/characterization tests (`session/instance_tmux_test.go`, `session/tmux/tmux_session_start_test.go`, `session/tmux/remote_env_test.go`), using this repo's existing `MockCmdExec`/`mockTmuxManager` test doubles. Jest + React Testing Library (`renderHook`/`waitFor`) for the frontend fixture (`web-app/src/lib/hooks/useAvailablePrograms.test.ts`).
- **Integration**: `gotestsum`-run Go tests (`server/services/session_service_create_test.go`) exercising the real `UpsertProgramConfig`/`CreateSession` RPC handlers, a real `git init`-ed repo fixture (`initGitRepoWithCommit`), a real tmux binary on an isolated per-test socket (`svc.testTmuxServerSocket`), and `envtest.NewIsolatedStateDir(t)` for a shared, isolated config directory across the write (`UpsertProgramConfig`) and read (`CreateSession` → `resolveExtraEnvVars` → `config.LoadConfig()`) sides.
- **E2E / UX**: N/A.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./server/services ./session ./session/tmux -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line (informational here — this plan's value is closing a *specific* regression gap, not raising aggregate line coverage) |
| TypeScript/Jest | `cd web-app && npx jest --testPathPatterns="useAvailablePrograms" --no-coverage` | N/A — one existing test file's fixture strengthened, no new coverage surface |

- All public service methods: happy path (Epic 1.1) + error/edge paths (Epic 1.2's resume, reuse-guard, argv-duplication, and remote-rewrite cases) covered.
- All external integrations: the one true external integration here (the real tmux binary) is covered by both a unit-level test double (Epic 1.2.3's `MockCmdExec` spy) and a real-binary integration test (Epic 1.1).
- Target commands to actually run before considering this plan done: `go test ./server/services -run TestCreateSession_CustomProgramEnv -v`, `go test ./session -run 'TestInitTmuxSession_ReuseViaBackendProcessAliveOnly|TestClaudeSettingsEnvOverrideArgs' -v`, `go test ./session/tmux -run 'TestTmuxNewSessionArgs_IncludesExtraEnv|TestWrapRemoteCommand' -v`, `cd web-app && npx jest --testPathPatterns="useAvailablePrograms" --no-coverage`.
