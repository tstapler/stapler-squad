# Research: Pitfalls & Risks

## P1 — Ctrl+T passes through to the terminal emulator
**Risk**: When focus is inside xterm.js, Ctrl+T may be intercepted by the terminal before the shortcut registry sees it. xterm.js captures all keydown events by default.
**Mitigation**: Test empirically. The existing `usePaneShortcuts` cockpit shortcuts (Ctrl+\, Ctrl+-) use the same approach and work; confirm Ctrl+T behaves the same. If not, use `Alt+T` as fallback.

## P2 — Omnibar lacks active session context at dispatch time
**Risk**: A ">shell" command in the omnibar has no way to know which session to spawn a shell in without changes to ActionDeps or the omnibar component tree.
**Mitigation**: Use the simpler `onSpawnShell(sessionId)` callback prop pattern (same as `onCreateSession`). OmnibarContext already has access to the active session list. Gate the ">shell" suggestion to only appear when there's exactly one active session or let the user pick from a list.

## P3 — RepoPathInput dropdown z-index inside a modal
**Risk**: RepoPathInput renders its completion dropdown `position: absolute` — inside a dialog with `overflow: hidden` or a stacking context, the dropdown may be clipped.
**Mitigation**: Check `NewShellDialog.tsx` CSS; ensure the dialog container does NOT set `overflow: hidden`. The dropdown uses `zIndex.dropdown` from the theme contract, which should be above modal z-index if the modal itself uses `createPortal`.

## P4 — Shell exit detection vs error toast
**Risk**: Shell processes exit asynchronously; the tab must transition from "running" to "error" state without a page reload. The existing `watchShellExit` goroutine (instance_shells.go) emits events — verify these reach the frontend via the streaming subscription.
**Mitigation**: Check that `useShells` hook re-fetches or receives a push update when shell status changes. Add error-state rendering to ShellTab for non-zero exit codes.

## P5 — Feature registry testIds must match actual test describe/it names
**Risk**: Registry testIds are matched against `describe > test` names; if names drift, the registry shows stale coverage.
**Mitigation**: Use snake_case naming convention consistent with existing tests. Update registry immediately after writing each test.

## P6 — Go shell tests require a live tmux process
**Risk**: `SpawnShell` implementation creates a real tmux sibling session; unit tests on the RPC handler may fail in CI where tmux is not available.
**Mitigation**: Test only the service handler layer (validation, session lookup, error codes) without actually calling `inst.SpawnShell()` — mock the instance or use a test double. Follow the `addInstanceToPoller` pattern from fork tests which injects mock instances.
