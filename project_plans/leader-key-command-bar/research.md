# Research: Leader-Key Command Bar

## Codebase Findings

### Existing keyboard infrastructure

**`web-app/src/lib/shortcuts/shortcutRegistry.ts`** (singleton `registry`)
- Single `document.addEventListener("keydown", ...)` dispatch loop.
- `Shortcut` type: `{ key, modifiers?, label, context, action }`.
- `getAll()` returns `Record<ShortcutContext, Shortcut[]>` — already consumed by `KeyboardShortcutsTab`.
- `isInputElement()` guard skips modifier-less keys in `<input>`/`<textarea>`/`contenteditable`.
- No sequence support yet; no `group` or `hidden` fields.
- The listener runs in the **bubble** phase, not capture. The window leader (`useWindowShortcuts`) works around this by attaching a separate `capture: true` catch-all while armed.

**`web-app/src/lib/shortcuts/useShortcut.ts`**
- `useEffect` wrapper that calls `registry.register(id, shortcut)` and returns cleanup.
- Used broadly across the codebase to register per-component shortcuts.

**`web-app/src/lib/window/useWindowShortcuts.ts`** (Alt+W leader)
- Arms `Alt+W` via `useShortcut`. When triggered: registers follow-up keys in the registry (`window.leader.1`…`window.leader.9`, `window.leader.next`, etc.), attaches a capture-phase catch-all that disarms on unrecognised keys, sets a 3 s timeout.
- `disarmLeader()` cleans up all follow-up registrations, clears timeout, removes catch-all.
- The catch-all pattern can be reused for the global leader.

**`web-app/src/lib/hooks/useFocusTrap.ts`**
- Focus trap hook used by modals throughout the app.
- Handles Tab/Shift+Tab cycling, `focusin` escape detection, returns focus on cleanup.
- Already handles the `[role="dialog"]`/`[role="alertdialog"]` escape path.

**`web-app/src/app/settings/KeyboardShortcutsTab.tsx`**
- Renders `registry.getAll()` grouped by `ShortcutContext`.
- Uses `formatKey()` to stringify modifiers. Extending `Shortcut` with `sequence` would require updating `formatKey` here too.

### Existing modal/overlay patterns

The app already has:
- `DeleteSessionConfirmModal.tsx` — uses `useFocusTrap`.
- `useDialogFocusTrap.ts` — specialized variant.
- `lib/omnibar/` — full omnibar implementation (actions + detectors + modes).

### Notification tray

`components/ui/NotificationPanel.tsx` and related `TrayHandle.tsx`, `TrayList.tsx` exist. No keyboard shortcuts for tray toggle, mark-all-read, or Quiet mode are currently in the registry.

### omnibar actions registry

`web-app/src/lib/omnibar/actions/` — the OmnibarAction union. The command bar is a distinct UI surface from the Omnibar (the Omnibar is a text-first creation tool; the command bar is a keyboard-only action picker). They don't need to share an action union, but the command bar can surface the same actions.

### CSS architecture

vanilla-extract (`.css.ts`) throughout. Animations use CSS transitions; `@media (prefers-reduced-motion: reduce)` patterns already exist in the codebase.

### Fuzzy matching

No shared fuzzy-search utility currently in `web-app/src/lib/`. The Omnibar has its own matching logic (`lib/omnibar/detectors/`). A lightweight fuzzy-filter function (substring match, score by consecutive chars) should be written in `lib/shortcuts/` rather than pulling in a library. If a library is needed, `fuse.js` is already a common choice but adds ~10 KB; consider inline first.

### Terminal safety analysis

The PTY keyboard path in xterm.js starts with `xterm.attachCustomKeyEventHandler`. The `useWindowShortcuts` pattern — capturing in the document's capture phase before the event reaches xterm — is the proven safe model. The new leader listener must use `{ capture: true }`.

### Chord safety probe findings (design sketch context)

The item says to match `event.code` (not `event.key`) to avoid macOS dead-key issues (Option+letter produces special chars on macOS). The proposed default is configurable. Common safe choices on macOS/Linux:
- `AltLeft+Space` — safe on macOS Chrome/Firefox, collides with Spotlight on macOS (but can be disabled).
- `AltLeft+KeyP` — safe in most contexts.
- `F1`/`F2` — safe but requires a function key.
- The item calls for a real byte probe (manual spike, AC-1); the implementation should leave the default injectable from a config value, not hardcoded.

## Prior art / patterns referenced

- tmux prefix key: armed in the outer mux, follow-up consumed by mux, unrecognised key forwarded to pane.
- VS Code command palette (`Ctrl+Shift+P`): fuzzy search over all commands.
- Emacs `M-x`: fuzzy completion over all interactive functions.
- which-key (Neovim plugin): shows key hints after a partial sequence with a timeout.
- Helix / Kakoune: modal, each mode has a distinct key map shown in a status bar.

## Risk assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Leader chord collides with browser/OS shortcut | Medium | High | Byte probe (AC-1); user-configurable fallback |
| Capture-phase listener intercepts unintended keys | Low | High | Strict arm/disarm lifecycle; catch-all only while armed |
| PTY sees leader key | Low | Critical | Capture phase + `stopPropagation`; Playwright AC-8 test proves it |
| Registry extension breaks `KeyboardShortcutsTab` | Low | Medium | Backward-compatible optional fields; existing `getAll()` signature unchanged |
| Fuzzy list causes layout jank at >100 items | Low | Medium | Virtual list (react-virtual or a simple windowed render) |
| Focus not returned after close | Low | Medium | `useFocusTrap` pattern already handles this |

## Open questions

1. Should the leader default be `Alt+Space` or `Alt+P`? Needs the byte probe.
2. Should `n` (notifications) branch be visible even when the notification panel has zero items?
3. Does the fuzzy palette need to surface actions from the Omnibar action registry, or only from `shortcutRegistry`?
4. What's the maximum depth of the which-key tree? (Currently `w`, `n`, `s` — 2 levels seems sufficient.)
5. Should the command bar overlay the terminal or be positioned outside it?
