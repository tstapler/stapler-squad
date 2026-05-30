# Implementation Plan: shell-tabs-completion

**Date**: 2026-05-30
**Status**: Ready for implementation

---

## Epic: Complete the Shell Tabs Feature

Three parallel work streams that can proceed independently after Stream A (path picker) and Stream B (UX entries) are reviewed.

---

## Stream A — UX Improvements (frontend only, no backend changes)

### Task A1 — Replace plain input with RepoPathInput in NewShellDialog
**File**: `web-app/src/components/sessions/NewShellDialog.tsx`
**Change**: Replace `<input id="shell-working-dir" .../>` (lines 91-102) with `<RepoPathInput id="shell-working-dir" directoriesOnly={true} value={workingDir} onChange={setWorkingDir} placeholder="Defaults to session directory" />`
**Import**: Add `import { RepoPathInput } from "@/components/ui/RepoPathInput";`
**Risk**: Dropdown z-index inside dialog — verify no `overflow: hidden` on dialog container (Pitfall P3)
**Acceptance**: Path autocomplete appears; selecting a directory populates the field; invalid paths show no match (not a hard validation error — field is optional)

### Task A2 — Keyboard shortcut: Ctrl+T in terminal context
**File**: `web-app/src/components/sessions/SessionDetailView.tsx`
**Change**: Add `useShortcut("terminal.spawn-shell", { key: "t", modifiers: { ctrl: true }, label: "Spawn new shell", context: "terminal", action: useCallback(() => setShowNewShellDialog(true), []) })` inside the component
**Import**: Add `import { useShortcut } from "@/lib/shortcuts/useShortcut";`
**Note**: If Ctrl+T is captured by xterm.js before registry (Pitfall P1), fall back to Alt+T and update label
**Acceptance**: Pressing Ctrl+T (or Alt+T) while focused inside the terminal opens NewShellDialog

### Task A3 — Omnibar: ">shell" command via CommandDetector
**Files** (in order):
1. `web-app/src/lib/omnibar/types.ts` — add `SpawnShell = "spawn_shell"` to `InputType` enum; add entry to `INPUT_TYPE_INFO`
2. `web-app/src/lib/omnibar/detectors/CommandDetector.ts` — add pattern `{ pattern: /^>shell(?:\s+(.+))?$/i, commandType: "spawn_shell", ... }` to COMMANDS array
3. `web-app/src/lib/omnibar/actions/types.ts` — add `| { type: "spawn_shell"; sessionId?: string; workingDir?: string; command?: string }` to `OmnibarAction`
4. `web-app/src/lib/omnibar/actions/dispatch.ts` — add `spawnShell?: (sessionId?: string, workingDir?: string) => void` to `ActionDeps`; add exhaustive switch case
5. `web-app/src/lib/contexts/OmnibarContext.tsx` — wire `spawnShell` callback into ActionDeps using active session ID from session list
**Acceptance**: Typing `>shell` in the omnibar shows a "Spawn new shell" suggestion; selecting it opens NewShellDialog for the active session

### Task A4 — Context/action menu entry in session detail
**File**: `web-app/src/components/sessions/SessionDetailView.tsx` (locate the `⋮` overflow/action menu)
**Change**: Add "Spawn new shell" menu item that calls `setShowNewShellDialog(true)`
**Acceptance**: ⋮ menu in the session header contains "Spawn new shell" item

### Task A5 — Error state in ShellTab when shell exits with non-zero code
**File**: `web-app/src/components/sessions/ShellTab.tsx` (and `.css.ts`)
**Change**: When `shell.status === "exited"` and `shell.exitCode !== 0`, show a warning icon and style the tab with an error color token from the theme
**Also**: In `useShells.ts` or `SessionDetailView.tsx`, show an error toast via `addNotification` when a shell exits unexpectedly (exitCode !== 0, status === "exited")
**Acceptance**: Shell tab turns red/orange with exit icon when process dies unexpectedly; toast appears with shell name and exit code

---

## Stream B — Go Unit Tests (backend, no frontend changes)

### Task B1 — Go unit tests for SpawnShell RPC handler
**File**: `server/services/session_service_shells_test.go` (new file)
**Pattern**: Follow `setupForkTestFixture` + `addInstanceToPoller` from `session_service_fork_test.go`
**Test cases**:
- `TestSpawnShell_Success` — valid session, running → returns Shell proto with ID
- `TestSpawnShell_SessionNotFound` → `CodeNotFound`
- `TestSpawnShell_SessionNotRunning` → `CodeFailedPrecondition`
- `TestSpawnShell_EmptySessionId` → `CodeInvalidArgument`
**Note**: Mock the instance `SpawnShell` call — do not actually create tmux processes (Pitfall P6). Use `addInstanceToPoller` with a test instance that returns a mock shell.

### Task B2 — Go unit tests for remaining shell RPCs
**File**: same `session_service_shells_test.go`
**Test cases** (happy path + not-found for each):
- `TestStopShell_Success`, `TestStopShell_NotFound`
- `TestRestartShell_Success`, `TestRestartShell_NotFound`
- `TestListShells_ReturnsAll`, `TestListShells_EmptySession`
- `TestDeleteShell_Success`, `TestDeleteShell_NotFound`

---

## Stream C — Frontend Tests (Jest + E2E)

### Task C1 — Jest tests for NewShellDialog
**File**: `web-app/src/components/sessions/__tests__/NewShellDialog.test.tsx` (new file)
**Mock**: `useShells` hook (mock `spawnShell`, `stopShell`, etc.)
**Test cases**:
- `renders_with_default_values` — name, command, workingDir fields visible
- `submits_with_correct_params_When_formFilled` — verifies `spawnShell` called with correct args
- `closes_When_escapePressed` — `onClose` called
- `closes_When_backdropClicked` — `onClose` called
- `disables_submit_While_spawning` — button disabled during async call
- `shows_error_toast_When_spawnFails` — error displayed when `spawnShell` rejects

### Task C2 — Jest tests for useShells hook
**File**: `web-app/src/lib/hooks/__tests__/useShells.test.ts` (new file)
**Mock**: ConnectRPC client (`@connectrpc/connect`, `@connectrpc/connect-web`)
**Test cases**:
- `spawnShell_calls_rpc_With_correct_params`
- `spawnShell_updates_shells_list_on_success`
- `stopShell_calls_rpc_With_shellId`
- `deleteShell_removes_shell_from_list`
- `listShells_returns_all_shells_for_session`

### Task C3 — E2E Playwright test for full shell flow
**File**: `tests/e2e/shell-tabs.spec.ts` (new file)
**Header**: `// @feature session:shell-tabs`
**Requires**: Live server (`STAPLER_SQUAD_INSTANCE=e2e-local`) with a running session
**Test cases**:
- `spawn_new_shell_via_button` — click `+`, fill dialog, verify new tab appears
- `spawn_new_shell_via_keyboard_shortcut` — focus terminal, Ctrl+T, fill dialog, verify tab
- `close_shell_tab` — spawn shell, click delete, tab disappears
- `shell_tab_shows_error_on_exit` — spawn shell with `exit 1` command, verify error state
**Page objects**: Extend or create `ShellPage.ts` with helpers for tab interaction

---

## Stream D — Registry Updates

### Task D1 — Update feature registry for all shell entries
**Files**: 
- `docs/registry/features/backend/SpawnShell.json`
- `docs/registry/features/backend/StopShell.json`
- `docs/registry/features/backend/RestartShell.json`
- `docs/registry/features/backend/ListShells.json`
- `docs/registry/features/backend/DeleteShell.json`
- `docs/registry/features/frontend/ui/new-shell-dialog.json`
- `docs/registry/features/frontend/ui/shell-tabs.json`
**Change**: Set `"tested": true`, populate `"testIds"` with actual test function/describe names, update `"lastModified"`
**Do after**: All Stream B and C tests are written and passing

---

## Sequencing

```
A1 (path picker)   ──┐
A2 (shortcut)      ──┤
A3 (omnibar)       ──┼── All parallel → QA verify → 
A4 (action menu)   ──┤
A5 (error state)   ──┘

B1 (SpawnShell test)  ──┐
B2 (other RPC tests)  ──┼── All parallel →
C1 (Dialog Jest)      ──┤
C2 (useShells Jest)   ──┤
C3 (E2E Playwright)   ──┘

D1 (Registry) ── after B1, B2, C1, C2, C3 all pass
```

## Acceptance Criteria

1. Ctrl+T (or Alt+T) opens NewShellDialog from within the terminal view
2. `>shell` in the omnibar opens NewShellDialog
3. `⋮` action menu contains "Spawn new shell"
4. `+` button in shell tab bar is visible, keyboard-focusable, accessible
5. Working directory field uses RepoPathInput with filesystem autocomplete
6. Shell tab shows error indicator when shell exits with non-zero code; error toast fires
7. All 12+ Go test cases pass with `go test ./server/services/ -run TestSpawnShell -race`
8. All Jest tests pass with `npx jest --no-coverage --testPathPatterns="NewShellDialog|useShells"`
9. E2E Playwright tests pass against live server
10. Feature registry: 7 entries show `tested: true` with populated `testIds`
