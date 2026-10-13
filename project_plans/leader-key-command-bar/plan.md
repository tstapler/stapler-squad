# Implementation Plan: Leader-Key Command Bar

## Summary

Extend the existing `ShortcutRegistry` singleton with sequence + group metadata, add a capture-phase leader-key hook, and build a modal command bar with two modes (which-key + fuzzy palette). Fold the existing `Alt+W` window leader in as the `w` branch. Add notification-tray actions under `n`. Five Playwright tests cover arm, navigate, execute, cancel, and PTY-safety.

## Architecture

```
lib/shortcuts/
  shortcutRegistry.ts      ← extend Shortcut with sequence?, group?, hidden?
  useShortcut.ts           ← no change needed
  useLeaderMode.ts         ← NEW: leader arm/disarm, mode state (whichkey | fuzzy | closed)
  leaderConfig.ts          ← NEW: default chord + user-configurable override

components/command-bar/
  CommandBar.tsx           ← NEW: modal overlay (which-key + fuzzy palette)
  CommandBar.css.ts        ← NEW: vanilla-extract styles
  CommandBarGroup.tsx      ← NEW: grouped row in which-key mode
  CommandBarItem.tsx       ← NEW: single action row
  useCommandBarFilter.ts   ← NEW: fuzzy filter logic

app/settings/
  KeyboardShortcutsTab.tsx ← update: render sequence field when present

tests/e2e/
  leader-command-bar.spec.ts ← NEW: 5 Playwright scenarios (AC-8)
```

## Task Breakdown

### T1: Extend ShortcutRegistry (1.5 h, frontend)
File: `web-app/src/lib/shortcuts/shortcutRegistry.ts`
- Add optional fields to `Shortcut`: `sequence?: string[]`, `group?: string`, `hidden?: boolean`.
- Add `getTree()` method: returns `ShortcutTree` (nested groups for which-key rendering).
- `getAll()` signature unchanged; backward compatible.
- Update `ShortcutContext` to add `"command-bar"` context for bar-internal shortcuts (j/k/Enter/Esc).
- Update `KeyboardShortcutsTab.tsx` to render `sequence` field when present (format: `<leader> w 1`).

### T2: Leader chord config (0.5 h, frontend)
File: `web-app/src/lib/shortcuts/leaderConfig.ts` (new)
- `LeaderConfig`: `{ code: string; label: string }` — uses `event.code` not `event.key`.
- Default: `{ code: "AltLeft+Space", label: "Alt+Space" }` — **placeholder pending byte probe (AC-1)**.
- Config reads from `localStorage` key `ssq.leaderKey` (settable in Settings > Keyboard Shortcuts).
- Export `getLeaderConfig()` and `setLeaderConfig(config)`.
- Document the probe requirement in a TODO comment; block shipping on real probe result.

### T3: `useLeaderMode` hook (2 h, frontend)
File: `web-app/src/lib/shortcuts/useLeaderMode.ts` (new)
- Attaches a capture-phase `document.addEventListener("keydown", ...)` listener.
- On leader chord match: calls `stopPropagation()` and `preventDefault()`, sets state to `"whichkey"`.
- Exposes: `mode: "closed" | "whichkey" | "fuzzy"`, `path: string[]`, `query: string`, `open()`, `close()`, `navigate(key)`, `setQuery(q)`.
- Disarms if: `isComposing`, any `[role="dialog"]` is open, or timeout fires.
- Swallows unrecognised keys while armed (they never reach bubble phase).
- On `Esc` at root: closes. On `Esc` with path depth > 0: backs up one level.

### T4: Fold `useWindowShortcuts` into leader `w` branch (1 h, frontend)
File: `web-app/src/lib/window/useWindowShortcuts.ts`
- Register window actions (switch 1-9, next, prev, rename) in `shortcutRegistry` with `sequence: ["w", "1"]` etc. and `group: "Windows"`.
- Keep `Alt+W` as the direct leader shortcut for backward compatibility (users who memorised Alt+W can still use it as before; it just also appears in the command bar).
- The `armLeader` / `disarmLeader` pattern in `useWindowShortcuts` remains; the global leader now also shows the `w` submenu.

### T5: Register notification-tray actions (1 h, frontend)
Files: `web-app/src/components/ui/NotificationPanel.tsx` (or a new `useNotificationShortcuts.ts`)
- Register in `shortcutRegistry`: tray toggle (`sequence: ["n", "t"]`), mark all read (`["n", "m"]`), Quiet mode toggle (`["n", "q"]`), Move all to tray (`["n", "a"]`).
- Reply options 1..N: dynamic registration (N from current notifications), `sequence: ["n", "r", "1"]`…
- All under `group: "Notifications"`.

### T6: `CommandBar` component (3 h, frontend)
Files: `web-app/src/components/command-bar/CommandBar.tsx`, `.css.ts`
- Renders as `<dialog role="dialog" aria-modal="true">` inside a React portal.
- Uses `useFocusTrap` (existing hook) for focus management.
- Two sub-views driven by `mode`:
  - **which-key**: renders `ShortcutTree` from `registry.getTree()` at current `path`; highlights active item with `aria-activedescendant`.
  - **fuzzy**: renders filtered flat list from `useCommandBarFilter`.
- `j`/`k`/`ArrowUp`/`ArrowDown` move selection. `Enter` executes. `Esc` closes/backs up.
- Entrance/exit: CSS transition. `@media (prefers-reduced-motion: reduce)` suppresses it.
- Touch targets: `min-height: 44px` on each item.
- Colour tokens from design system; contrast checked ≥ 4.5:1.

### T7: `useCommandBarFilter` hook (1 h, frontend)
File: `web-app/src/components/command-bar/useCommandBarFilter.ts`
- Accepts query string + flat list of actions.
- Returns filtered + ranked list (consecutive-char bonus).
- Lazy: only filters when query length ≥ 1.
- No library dependency — inline 30-line fuzzy scorer.

### T8: Wire `useLeaderMode` into app root (0.5 h, frontend)
File: `web-app/src/app/page.tsx` (or `Providers.tsx`)
- Call `useLeaderMode()` at app root.
- Render `<CommandBar>` portal when `mode !== "closed"`.

### T9: Settings UI — leader key configuration (1 h, frontend)
File: `web-app/src/app/settings/KeyboardShortcutsTab.tsx`
- Add "Leader Key" field: shows current leader chord, button to re-probe / change.
- Writes to `leaderConfig`.
- Render `sequence` column in the shortcuts table (e.g. `⌥Space w 1`).

### T10: Chord safety probe documentation (0.5 h, docs)
File: `docs/reference/leader-key-chord-probe.md`
- Document the byte probe methodology (test matrix: macOS Safari, macOS Chrome, Linux Chrome).
- Record the chosen default and any collisions found.
- This is a **human spike** — mark AC-1 as blocked until the probe is run.

### T11: Playwright E2E tests (2 h, test)
File: `tests/e2e/leader-command-bar.spec.ts`
- `// @feature session:keyboard-leader`
- Test 1 (Arm): press leader → bar opens.
- Test 2 (Navigate): j/k moves selection; `aria-activedescendant` updates.
- Test 3 (Execute): select "Windows: Next" → window actually switches.
- Test 4 (Cancel): Esc closes bar; focus returns to previously focused element.
- Test 5 (PTY safety): terminal pane focused → press leader → `xterm.onData` spy not called.
- No `waitForTimeout`; all locators via `data-testid` or ARIA roles.

### T12: Unit tests for `shortcutRegistry` extension (1 h, test)
File: `web-app/src/lib/shortcuts/__tests__/shortcutRegistry.test.ts`
- Existing tests must pass (no regression).
- New: `getTree()` groups actions by `group` field correctly.
- New: `sequence` field roundtrips through register/getAll.
- New: `hidden: true` actions excluded from `getAll()` output (they're still callable).

## Sequencing

```
T1 (registry extend) → T2 (leader config) → T3 (useLeaderMode)
                                              ↓
T4 (fold windowShortcuts) ────────────────→ T6 (CommandBar)
T5 (notif actions)        ────────────────→ T6
T7 (fuzzy filter)         ────────────────→ T6
                                              ↓
T8 (wire into app) → T9 (settings UI) → T10 (docs) → T11 (E2E) → T12 (unit tests)
```

T1, T2, T3 are serial (each builds on the prior). T4, T5, T7 can be parallelised once T1 is done. T6 depends on T3, T4, T5, T7. T8 depends on T3 and T6.

## Total estimate

~14 h of implementation work, split across frontend (T1-T9: ~11 h), docs (T10: 0.5 h), and tests (T11-T12: 3 h). Note T10 (the byte probe) is a mandatory human step that blocks AC-1; it can run in parallel with T1-T5.

## Files not to touch

- `web-app/src/lib/omnibar/` — no omnibar changes needed.
- `session/` or `server/` Go code — pure frontend feature.
- Any existing `.test.ts`/`.spec.ts` — only add, never modify existing test expectations.

## Implementation notes (as built)

- `useLeaderMode`, `CommandBarGroup`, `CommandBarItem`, `useCommandBarFilter` collapsed into a pure reducer + `interpretKey` in `components/command-bar/commandBarState.ts` and one `CommandBar.tsx`.
- `commandBarController` lives in `lib/shortcuts/` so `lib/` never imports from `components/`.
- Alt+W opens the bar at `w` when a bar is mounted and no modal is open; otherwise the legacy armed leader runs (keeps `useWindowShortcuts` standalone and its tests valid).
- Reply options 1..N use the Reply card's own buttons (click `reply-option-N`), so the audited send path is unchanged. Known coupling: a test id as control flow.
- Idle auto-close defaults to 3 s (`ssq.leaderIdleMs`, never lower).
- Not done: macOS Safari/Chrome and headed Linux Chrome chord probe (`docs/reference/leader-key-chord-probe.md`); no `"command-bar"` ShortcutContext (not needed).
- Accepted deviations: j/k are text in fuzzy mode (arrows navigate); modified chords (Ctrl+V etc.) are swallowed while the bar is open; the pane picker's window-level key capture is not suppressed by an open bar.
