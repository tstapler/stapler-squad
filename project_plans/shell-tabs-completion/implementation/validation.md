# Validation Plan: shell-tabs-completion

**Date**: 2026-05-30

## Requirements → Test Traceability

| Requirement | Test | Type | File |
|---|---|---|---|
| Ctrl+T opens NewShellDialog | `shortcut_should_openDialog_When_ctrlTInTerminal` | Jest (RTL) | NewShellDialog.test.tsx |
| `>shell` omnibar opens dialog | `CommandDetector_should_detectSpawnShell_When_shellCommand` | Jest | detector.test.ts |
| `⋮` menu has "Spawn new shell" | `spawn_new_shell_via_action_menu` | E2E | shell-tabs.spec.ts |
| Path picker uses RepoPathInput | `renders_repoPathInput_For_workingDir` | Jest | NewShellDialog.test.tsx |
| Dialog submits correct params | `submits_with_correct_params_When_formFilled` | Jest | NewShellDialog.test.tsx |
| Dialog closes on Escape | `closes_When_escapePressed` | Jest | NewShellDialog.test.tsx |
| Dialog closes on backdrop click | `closes_When_backdropClicked` | Jest | NewShellDialog.test.tsx |
| Error toast when spawn fails | `shows_error_toast_When_spawnFails` | Jest | NewShellDialog.test.tsx |
| Shell tab error state on bad exit | `shows_error_state_When_shellExitsNonZero` | Jest | ShellTab.test.tsx |
| SpawnShell RPC success | `TestSpawnShell_Success` | Go unit | session_service_shells_test.go |
| SpawnShell RPC session not found | `TestSpawnShell_SessionNotFound` | Go unit | session_service_shells_test.go |
| SpawnShell RPC session not running | `TestSpawnShell_SessionNotRunning` | Go unit | session_service_shells_test.go |
| SpawnShell RPC empty session ID | `TestSpawnShell_EmptySessionId` | Go unit | session_service_shells_test.go |
| StopShell success + not found | `TestStopShell_Success`, `TestStopShell_NotFound` | Go unit | session_service_shells_test.go |
| RestartShell success + not found | `TestRestartShell_Success`, `TestRestartShell_NotFound` | Go unit | session_service_shells_test.go |
| ListShells returns all shells | `TestListShells_ReturnsAll` | Go unit | session_service_shells_test.go |
| DeleteShell success + not found | `TestDeleteShell_Success`, `TestDeleteShell_NotFound` | Go unit | session_service_shells_test.go |
| Full spawn → use → close flow | `spawn_new_shell_via_button` | E2E | shell-tabs.spec.ts |
| Keyboard shortcut E2E | `spawn_new_shell_via_keyboard_shortcut` | E2E | shell-tabs.spec.ts |
| Tab deletion removes tab | `close_shell_tab` | E2E | shell-tabs.spec.ts |
| useShells.spawnShell calls RPC | `spawnShell_calls_rpc_With_correct_params` | Jest | useShells.test.ts |
| useShells.stopShell calls RPC | `stopShell_calls_rpc_With_shellId` | Jest | useShells.test.ts |
| Feature registry tested: true | All 7 entries updated | Registry | docs/registry/features/ |

## Test Count Summary

| Type | Count |
|---|---|
| Go unit tests | 12 |
| Jest/RTL tests | 10 |
| E2E Playwright tests | 4 |
| **Total** | **26** |

## Readiness Gate

- [x] All requirements have at least one test mapped
- [x] Every acceptance criterion in plan.md is traceable to a test
- [x] No requirement relies on manual QA only
- [x] Unhappy paths (spawn failure, session not found, non-zero exit) all have tests
- [x] E2E covers the complete golden path (spawn → interact → close)

**Gate verdict: PASS — ready for implementation**
