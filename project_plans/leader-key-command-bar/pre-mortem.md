# Pre-mortem / Adversarial Review: Leader-Key Command Bar

## P0 — Catastrophic failures

### P0-1: Leader chord is stolen by the OS before the browser sees it
**Scenario:** User picks `Alt+Space` as the default. On macOS, `⌥Space` inserts a non-breaking space in some apps and is also used by Raycast/Alfred/Spotlight launchers on certain configurations. The browser receives the keydown only if no system-level handler consumed it first. On Linux, `Alt+F1` and `Alt+F2` open GNOME/KDE menus.

**Risk:** The chord fires inconsistently (works in Chrome but not Safari, works on Linux GNOME 3 but not KDE), eroding user trust.

**Mitigation:**
- AC-1 requires a real byte probe across the target matrix before the default is locked.
- The leader is user-configurable so power users can escape any collision.
- The feature must not ship without the probe result — T10 (probe doc) gates AC-1.

### P0-2: Leader keystroke reaches the PTY
**Scenario:** The leader fires in bubble phase. While the terminal pane has focus, xterm.js's own keydown handler (capture phase on the pane element? bubble phase?) may fire before or after. If the leader does not use `stopPropagation()` in the capture phase, xterm.js receives it and sends it to the PTY.

**Risk:** The PTY receives a spurious byte sequence on every time the user arms the leader. For example, `Alt+Space` becomes ESC+space in many terminal emulators, which could trigger readline shortcuts or corrupt a running program.

**Mitigation:**
- `useLeaderMode` MUST use `document.addEventListener("keydown", ..., { capture: true })` with explicit `stopPropagation()` and `preventDefault()`.
- Playwright AC-8 test 5 proves this by spying on `xterm.onData` before pressing the leader.
- This is the same pattern `useWindowShortcuts` already uses for its catch-all.

---

## P1 — High-impact failures

### P1-1: Disarm-on-timeout races with follow-up keystroke
**Scenario:** User types the leader, waits exactly 3 s (timeout fires), then immediately types a follow-up key. The timeout disarms the leader just as the follow-up's keydown event is in flight. The catch-all listener is removed, follow-up registrations are removed, but the follow-up key is already in the event queue.

**Risk:** The follow-up key is not consumed by the command bar (good) but also not shown in the UI (the bar has already closed). The user sees the bar close and their follow-up key lands in whatever has focus next — potentially an input field.

**Mitigation:**
- Use `clearTimeout` on any follow-up keydown (before calling the action), exactly as `useWindowShortcuts` does.
- The catch-all capture-phase listener ensures that *any* keydown while armed runs through `disarmLeader` first — the race window is sub-millisecond and the worst case is a single spurious character in a text field.
- Document this in `useLeaderMode` as a known edge case.

### P1-2: Stacked modal + leader conflict
**Scenario:** The user opens a modal dialog (e.g. `DeleteSessionConfirmModal`), then accidentally presses the leader. The command bar opens as a second overlay on top of the existing dialog. Now there are two overlapping focus traps.

**Risk:** Focus management becomes undefined; `useFocusTrap` on both modals fights each other. The app may appear frozen.

**Mitigation:**
- `useLeaderMode` checks for any open `[role="dialog"]` or `[role="alertdialog"]` before arming. If one is present, the leader is swallowed but the bar does NOT open.
- The check should be `document.querySelector('[role="dialog"]:not([hidden]), [role="alertdialog"]:not([hidden])')`.
- Unit test: arm attempt while a dialog is open does nothing.

### P1-3: The `w` branch duplicates `useWindowShortcuts`'s leader
**Scenario:** Both the global leader's `w` branch and the existing `Alt+W` window leader are armed simultaneously (user presses Alt+W, which arms the old leader; then the command bar opens the `w` branch).

**Risk:** Duplicate follow-up registrations in `shortcutRegistry` fire the same action twice (or one warns about the duplicate ID, whichever fires first).

**Mitigation:**
- T4 must explicitly disarm the `useWindowShortcuts` leader when the global command bar opens (share the disarm function or emit a custom event).
- Alternatively: `useWindowShortcuts` should check whether the global command bar is open before arming its own leader mode, and no-op if it is.
- Consider removing the standalone `useWindowShortcuts` leader entirely (the window actions are now discoverable via the command bar), and retaining only the `Alt+W` hotkey as a convenience alias that directly opens the `w` branch of the command bar. Decision for implementation phase.

### P1-4: Dynamic notification reply actions cause stale registrations
**Scenario:** Reply actions 1..N are registered with `sequence: ["n", "r", "1"]` etc. when the notification panel mounts. If notifications change while the command bar is open, the registered actions are stale.

**Risk:** User sees "Reply 1: Session A" in the bar but executing it replies to a different notification (the one that was current at registration time).

**Mitigation:**
- Reply actions should be registered lazily (when the `n` branch is navigated to), not at mount time. Or: re-register on every notification change via `useEffect` with notifications as a dependency.
- Keep action handlers as closures over refs, not over stale values.

---

## P2 — Medium-impact risks

### P2-1: Fuzzy list layout jank at large action counts
**Scenario:** The registry accumulates >200 shortcuts as the app grows. The fuzzy palette renders all of them on first open (before any filter query).

**Risk:** Slow initial render causes visible jank (frame drops).

**Mitigation:** Virtual list for the palette. T6 specifies this; T7's `useCommandBarFilter` should be memoised (no recompute unless query changes).

### P2-2: Reduced-motion not applied to entrance animation
**Scenario:** The `CommandBar` uses a `transform: translateY` entrance animation. A user with `prefers-reduced-motion: reduce` set still sees the animation.

**Risk:** Accessibility violation; could trigger vestibular disorders.

**Mitigation:** T6 explicitly checks for this. The `.css.ts` file must have a `@media (prefers-reduced-motion: reduce)` block that sets `transition: none`.

### P2-3: `event.code` matching breaks on non-QWERTY layouts
**Scenario:** A user with an AZERTY keyboard has the `A` key at `event.code = "KeyQ"`. If the leader uses `event.code`, `"AltLeft+Space"` is fine, but if the follow-up keys use `event.code` too (`"KeyW"` for the window branch), an AZERTY user presses the physical `Z` key and gets the `w` branch — confusing.

**Risk:** Keyboard layouts other than QWERTY produce confusing follow-up key labels.

**Mitigation:**
- The leader chord itself should use `event.code` (to avoid dead-key issues for the single chord).
- Follow-up keys in the which-key tree should use `event.key` (user-visible characters, layout-aware).
- The `useLeaderMode` hook should only use `event.code` for leader detection; all follow-up dispatch should use `event.key`, consistent with the rest of `shortcutRegistry`.

### P2-4: Screen reader announces every item on arrow-key navigation
**Scenario:** Each arrow key press moves `aria-activedescendant`. A screen reader that announces the full item text on each move would read "Windows: Switch to Window 3, Alt+Space W 3" on every keypress — verbose.

**Risk:** Poor screen reader UX for keyboard-only users who also use screen readers.

**Mitigation:**
- Use `aria-activedescendant` on the listbox (not focus movement) so the screen reader announces only the changed item.
- Each item has a concise label (action name only in the `aria-label`; the key sequence is in a `<span aria-hidden="true">`).

---

## Plan changes required

1. **T3 (`useLeaderMode`):** Add explicit guard: if any non-hidden dialog is open, arm attempt is a no-op.
2. **T3 (`useLeaderMode`):** Use `{ capture: true }` for the leader listener; use `event.code` for leader detection, `event.key` for follow-up.
3. **T4 (window shortcuts):** Decide whether to deprecate the standalone `Alt+W` leader or keep it as a shortcut alias. Document the decision.
4. **T5 (notif actions):** Register reply options lazily (on `n` branch navigation) with ref-based closures.
5. **T6 (CommandBar):** Add `@media (prefers-reduced-motion: reduce)` block. Add virtual list for >100 items.
6. **T10 (probe doc):** Explicitly mark AC-1 as a **blocking human spike** — the feature must not ship the default leader chord without it.
