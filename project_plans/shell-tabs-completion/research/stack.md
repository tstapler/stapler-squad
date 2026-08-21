# Research: Stack & Existing Infrastructure

## Path Picker Component

**Canonical component**: `web-app/src/components/ui/RepoPathInput.tsx`
- Drop-in replacement for the plain `<input>` in `NewShellDialog.tsx:91-102`
- Props: `id`, `value`, `onChange`, `disabled`, `error`, `placeholder`, `required`, `directoriesOnly`
- Features: filesystem autocomplete (150ms debounce, LRU cache), history matching, tilde abbreviation, keyboard navigation, ARIA-compliant
- Backend: `ListPathCompletions` RPC validates path existence server-side (`path_exists` field in response)
- Hook: `usePathCompletions` + `usePathHistory` — both already used by RepoPathInput internally

**Change in NewShellDialog**: Replace `<input id="shell-working-dir" .../>` (line 91-102) with `<RepoPathInput id="shell-working-dir" directoriesOnly={true} ... />`.

## Keyboard Shortcut Infrastructure

- Registry: `web-app/src/lib/shortcuts/shortcutRegistry.ts`
- Hook: `useShortcut(id, shortcut)` — auto-registers and auto-cleans up on unmount
- `terminal` context: **already exists**, applied via `data-context="terminal"` on XtermTerminal div (XtermTerminal.tsx:455)
- **Ctrl+T is available** — zero conflicts in the current registry
- Context hierarchy: when focus is inside terminal, `terminal` + `global` + `cockpit` shortcuts all fire

**Registration pattern:**
```typescript
useShortcut("terminal.spawn-shell", {
  key: "t", modifiers: { ctrl: true },
  label: "Spawn new shell",
  context: "terminal",
  action: useCallback(() => setShowNewShellDialog(true), [])
});
```

## Omnibar Integration

**CommandDetector** (priority 5) is the right extensibility point for a command-palette–style "spawn shell" entry:
- File: `web-app/src/lib/omnibar/detectors/CommandDetector.ts`
- Matches `>shell` or `>spawn shell` prefix
- Add `InputType.SpawnShell` to `web-app/src/lib/omnibar/types.ts`
- Add `spawn_shell` variant to `OmnibarAction` union (types.ts)
- Add `spawnShell` to `ActionDeps` and case to dispatch.ts exhaustive switch
- **Session context gap**: dispatch has no `currentSessionId` — pass `sessionId` in the action payload; OmnibarContext must supply it from the active session

**Simpler omnibar path**: Add direct `onSpawnShell` callback prop to Omnibar (like `onCreateSession`) rather than going through the full dispatch pattern. Resolves session context without breaking ActionDeps.

## Test Infrastructure

| Layer | Harness | Key helpers |
|---|---|---|
| Go service | `setupForkTestFixture(t)` in session_service_fork_test.go | `addInstanceToPoller`, `addPausedSession`, `createTestStorage` |
| Jest/RTL | @testing-library/react + Redux Provider wrapper | `makeSession()`, `makeStore()`, `jest.mock(...)` |
| E2E/Playwright | Full framework in tests/e2e/ | `SessionsPage`, RPC interception via `page.waitForRequest()` |
