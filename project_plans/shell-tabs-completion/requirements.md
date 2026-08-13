# Requirements: shell-tabs-completion

**Date**: 2026-05-30
**Type**: Feature completion (UX + testing + quality gates)

## Problem Statement

The shell tabs / SpawnShell feature is implemented end-to-end (5 RPCs, NewShellDialog, ShellTab UI, useShells hook) but is not considered done:

1. **Poor discoverability** — only a small `+` button in the shell tab bar; no keyboard shortcut, no omnibar entry, no context/action menu entry
2. **Zero test coverage** — all shell feature registry entries show `tested: false` with empty `testIds`; no e2e tests exist in `tests/e2e/`
3. **Unknown correctness under error conditions** — shell exit, spawn failure, session-not-running cases are untested; error UX is unverified

Users who need to quickly spawn a second shell (a common task when running Claude Code alongside a dev server) cannot do so quickly and have no confidence in the error path.

## Users / Consumers

End users (human operators) managing AI agent sessions in stapler-squad. Specifically: users who want to run multiple shells within a single session (e.g., agent in one tab, log tail in another).

## Success Metrics

- All four quality gates met:
  1. E2E Playwright test covering the full spawn → use → close flow
  2. Go unit tests for `SpawnShell` RPC handler (success + validation errors + session-not-found)
  3. Jest tests for `NewShellDialog` component and `useShells` hook (form submission, error state, tab switching)
  4. Feature registry updated to `tested: true` for all 7 shell entries (5 backend RPCs + 2 frontend components)
- All four UX entry points work:
  1. Keyboard shortcut (Ctrl+T or similar) opens the new shell dialog from anywhere in the session view
  2. `+` button in the shell tab bar is visible and accessible
  3. Omnibar / command palette entry "new shell" or "spawn shell" works
  4. Context menu or action menu on the session surfaces "Spawn new shell"
- Error state: when a shell fails to spawn or exits unexpectedly, an error toast appears and the tab remains visible with an error indicator

## Constraints

- No hard deadline
- No performance SLA
- Backend implementation (5 RPCs in `session_service_shells.go`, `instance_shells.go`) is complete and should not require significant changes — UX and test work is the focus
- Must follow existing CSS architecture rules (vanilla-extract `.css.ts`, no raw hex colors)
- Keyboard shortcut must be registered in the shortcut registry and appear in the keyboard shortcuts settings tab
- Omnibar entry must follow the existing OmnibarAction + DetectorRegistry patterns (`feature-testing-registry.md`)

## Scope

### In Scope

- **UX: keyboard shortcut** — register a shortcut (e.g., `Ctrl+T` in `terminal` context) to open `NewShellDialog`
- **UX: + button polish** — ensure existing button is accessible, has correct aria-label and title, is keyboard-focusable
- **UX: path picker in NewShellDialog** — replace the raw text input for "working directory" with the existing path selection component (same picker/autocomplete used in session creation / omnibar), including path existence validation before submit is enabled
- **UX: omnibar entry** — add a "spawn shell" / "new shell" `OmnibarAction` variant and register it in `createDefaultRegistry()` or as a command palette action
- **UX: context/action menu** — add "Spawn new shell" to the session action menu (overflow / `⋮` menu)
- **UX: error state** — error toast on spawn failure; tab shows error indicator on shell exit with non-zero code
- **Tests: Go unit** — `SpawnShell`, `StopShell`, `RestartShell`, `ListShells`, `DeleteShell` RPC handlers; cover success, validation errors, session-not-found, session-not-running
- **Tests: Jest** — `NewShellDialog` (form fields, submit, Escape to close, click-outside); `useShells` hook (spawnShell, stopShell, deleteShell, error handling)
- **Tests: E2E Playwright** — full spawn → type → close flow against the live server
- **Registry: update** — set `tested: true` and populate `testIds` for all 7 shell feature registry entries

### Out of Scope

- Changes to the shell backend architecture (tmux sibling-session model is final)
- Multi-pane / split-terminal layout within a single shell tab
- Shell persistence across session restarts
- Shell-to-session promotion (turning a shell into a full session)

## Open Questions

- What keyboard shortcut key should be used? `Ctrl+T` is conventional (new tab) but may conflict with terminal pass-through; `Alt+T` or a custom chord may be safer — needs conflict check against `shortcutRegistry.ts`
- Should "new shell" in the omnibar work globally (from any page) or only when a session is active?
- What is the visual treatment for a shell tab in error state — red border, warning icon, both?
- Does the context/action menu already exist on the session detail view, or does one need to be created?
- Which path picker component is canonical? (Omnibar path input, `LocalPathDetector`, or a separate `PathInput` component?) — needs exploration before implementation
