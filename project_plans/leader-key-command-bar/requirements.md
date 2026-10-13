# Requirements: Leader-Key Command Bar (which-key + fuzzy palette)

**Item:** feat(web): leader-key command bar (which-key + fuzzy palette) for keyboard-first actions
**Project name:** leader-key-command-bar
**Follow-up to:** #974 (tray hotkey unshipped; no safe single chord found, see Task 4.2f)

---

## Problem Statement

The app has multiple keyboard shortcuts spread across contexts (global, session-list, terminal, cockpit, omnibar). Finding and remembering them requires the `?` overlay or reading the Settings > Keyboard Shortcuts tab. The `Alt+W` window leader (PR #974) shipped but the notification tray hotkey was explicitly left out because no safe single chord existed. As the app grows more actions, chord collisions multiply.

A tmux-style leader key solves this: one configurable prefix arms a brief modal state, then a single character picks the action. The bar can also surface a fuzzy-search palette (VS Code / Sublime style), making every action discoverable without memorising individual chords.

---

## Goals

1. Ship a single configurable **leader key** that captures one chord per session without flooding the chord namespace.
2. Deliver two discoverable modes from the same registry: **which-key** (hierarchical menu) and **fuzzy palette** (type-to-filter all actions).
3. Fold the existing `Alt+W` window leader into the `w` branch; add notification-tray actions under `n`.
4. Guarantee the leader chord is **never forwarded to the PTY** while a terminal pane is focused.
5. Meet WCAG 2.2 AA accessibility requirements (focus trap, `aria-activedescendant`, reduced-motion, 44px touch targets).

---

## Non-Goals

- Replacing the existing `?` shortcut overlay in Settings.
- Remapping every existing shortcut to sequences (shortcuts remain usable both ways).
- Mobile-native app (touch-first designs handled separately under `mobile-ux-improvements`).
- Backend API changes — this is a pure frontend feature.

---

## Acceptance Criteria

### AC-1: Leader chord selection and configuration
- Leader key defaults must be determined by running a real browser/OS byte probe (macOS Safari, macOS Chrome, Linux Chrome) matching `event.code`, not `event.key`, to avoid dead-key problems on Option+letter on macOS.
- A "no safe chord" fallback path exists and is user-configurable via Settings > Keyboard Shortcuts (or config.json for headless use).
- The chosen default and the probe methodology are documented in a reference doc under `docs/reference/`.

### AC-2: Leader mode arms and disarms safely
- Leader fires in the **capture phase** (`addEventListener("keydown", ..., true)`) so the PTY never sees the chord.
- While the bar is closed, pressing the leader key opens it; pressing it again while open closes it (toggle).
- `Esc` backs out one level; `Esc Esc` (or a second `Esc` at root level) closes fully.
- An unrecognised key while the bar is open is **swallowed and not forwarded** to the PTY (same guarantee as `useWindowShortcuts`'s catch-all listener).
- Leader is disarmed while an IME composition event (`event.isComposing`) is active.
- Leader is disarmed while any modal dialog (`[role="dialog"]`, `[role="alertdialog"]`) is open.
- A timeout of ≥ 3 s (configurable) disarms the leader automatically with no action.

### AC-3: Which-key mode
- After the leader, show a grouped menu of next-key bindings (e.g. `w` → windows, `n` → notifications, `s` → sessions).
- As the user types a sequence, the current path is shown (breadcrumb or inline label).
- `j`/`k` (or arrow keys) navigate the list; `Enter` executes the highlighted action.
- `?` at any level shows a help overlay for the current level's keys.
- The menu is rendered as a `role="listbox"` with `aria-activedescendant` tracking.

### AC-4: Fuzzy palette mode
- Typing any printable character while the bar is open (not a registered single-key binding) switches to fuzzy-filter mode.
- All registered shortcut-registry actions appear in the list with their full key sequence displayed beside each entry.
- Fuzzy matching is case-insensitive and matches substrings (not just prefix).
- `↑`/`↓` or `j`/`k` navigate; `Enter` executes; `Esc` closes.
- The list is virtualised for >100 items to avoid layout jank.

### AC-5: Registry integration
- `shortcutRegistry.ts` is extended with: `sequence?: string[]` on `Shortcut`, `group?: string`, `hidden?: boolean`.
- A `getAll()` overload (or method) returns a tree structure usable by both modes.
- The existing `Alt+W` window leader (`useWindowShortcuts.ts`) becomes the `w` branch of the new leader.
- Notification tray toggle, mark-all-read, Quiet mode, Move-all-to-tray, and Reply options 1..N are registered under the `n` branch.
- No existing shortcut's ID, label, or behaviour changes.

### AC-6: Terminal safety
- `Ctrl+C`, `Ctrl+A`, and every other xterm-standard chord continue to work in terminal panes.
- A Playwright test confirms the leader keystroke never arrives at `xterm.js`'s `onData` callback when a terminal pane is focused.
- The capture-phase listener is attached on the **document** (not a pane element), consistent with `useWindowShortcuts`.

### AC-7: Accessibility
- The bar renders in a `<dialog>` element (native focus trap, `aria-modal="true"`), or uses `useFocusTrap` consistently with the rest of the codebase.
- Every list item has a minimum 44×44 CSS px tap target.
- `prefers-reduced-motion` suppresses entrance/exit animations.
- Colour contrast ≥ 4.5:1 for text, ≥ 3:1 for UI components (WCAG 2.2 AA).
- Axe Core CI check passes (already wired via `make e2e-lighthouse`).

### AC-8: Playwright coverage
Five test scenarios required:
1. **Arm**: pressing the leader key opens the bar.
2. **Navigate**: `j`/`k` moves the selection; `Enter` executes the highlighted action.
3. **Execute**: a specific action from the menu actually fires (e.g. toggling a window).
4. **Cancel**: `Esc` closes the bar; focus returns to the element that had it before.
5. **PTY safety**: with a terminal pane focused, the leader keystroke does NOT appear in `xterm.js` `onData`.

### AC-9: No regression on existing shortcuts
- All existing `useShortcut` registrations continue to fire as before (unit test coverage on `shortcutRegistry.ts`).
- `KeyboardShortcutsTab.tsx` renders the new `sequence` field when present.

---

## Constraints and Context

- CSS architecture: vanilla-extract (`.css.ts`). No inline styles or Tailwind.
- State management: React hooks + context; no Redux.
- Build: Next.js (`web-app/`), `pnpm` only (never npm/yarn).
- Tests: Jest + React Testing Library for unit; Playwright for E2E (conventions in `e2e-test-conventions` skill).
- No `waitForTimeout` in Playwright tests; `data-testid` or ARIA role locators only.
- E2E test file must begin with `// @feature` annotation.
- The `shortcutRegistry.ts` changes must not break existing `getAll()` consumers (`KeyboardShortcutsTab.tsx`).
- The feature's Omnibar registry entry (if it opens via Omnibar) must follow the 7-touchpoint session-creation pattern from `docs/reference/session-creation-registry.md` only if it creates sessions; otherwise just the standard OmnibarAction union update.
