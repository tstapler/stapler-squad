# UX Design: multi-window

Terminology used below matches `implementation/plan.md`'s Domain Glossary exactly: `NamedWindow`
(id + name + `PaneState`), `WindowsState` (the shared, persisted list — no active pointer),
`currentWindowId` (per-tab, resolved from `?window=` → `lastFocusedWindowId` → `windows[0].id`),
`switchToWindow(id)` (URL navigation, from `useWindowUrlSync`), `WindowTabStrip` (forked from
`MobilePaneTabStrip.tsx`), `useWindowSwipe`, `useWindowShortcuts`, `leaderArmed`. Do not introduce
new names for these concepts.

---

## 1. Interactive surfaces (full treatment)

1. Window Tab Strip — desktop (top bar)
2. Window Tab Strip — mobile (bottom-docked)
3. Create window
4. Switch window (click / leader-key / swipe — one flow, three triggers)
5. Rename window (double-click desktop / long-press mobile / `F2` keyboard)
6. Close window (incl. last-window auto-recreate / `Delete` keyboard)
7. Leader-key sequence (`Alt+W` then digit/`n`/`p`/`,`/`Escape`)
8. Swipe gesture (mobile)
9. Per-tab URL binding (`?window=<id>`) — what copying/opening a URL does
10. tmux-vocabulary onboarding hint

## 2. Non-interactive surfaces (condensed)

11. `cockpit.windowLayout` v2 persisted schema
12. Migration/error console logging
13. `?` shortcut overlay entry for window shortcuts

---

## Surface 1 & 2: Window Tab Strip (desktop top / mobile bottom)

### Wireframe — desktop (above the existing app header, below browser chrome)

```
┌─────────────────────────────────────────────────────────────────────┐
│ role="tablist" aria-label="Window switcher"                          │
│  ┌───────────────┐┌────────────────┐┌───────────┐  ┌───┐             │
│  │ Window 1    ●│││ Reviewing PR#x ││ Debug y   │  │ + │             │
│  └───────────────┘└────────────────┘└───────────┘  └───┘             │
│   ▲ active tab      ▲ inactive         ▲ inactive    ▲ new-window    │
│   (bold, underline)  (dimmed text)      (dimmed)      button          │
└─────────────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────────────┐
│  existing app header / mobile pane tab row (role="tablist"           │
│  aria-label="Pane switcher") — unchanged, sits BELOW this strip       │
└─────────────────────────────────────────────────────────────────────┘
```

- Each tab: `role="tab"`, `aria-selected`, `aria-controls="window-panel-<id>"`, roving `tabindex`
  (active tab `0`, others `-1`).
- Active tab: bold label + underline/accent bar (not color alone — contrast ≥ 4.5:1, no
  color-only state per WCAG).
- Tab width: fixed max-width, `text-overflow: ellipsis`, `title={name}` (verbatim
  `MobilePaneTabStrip` convention).
- `+` control is the last item in the tablist's DOM order but is **not itself** a `role="tab"` —
  it is a plain button (`aria-label="New window"`) after the tablist, so arrow-key roving
  navigation among tabs doesn't land on it; Tab key reaches it as the strip's second focus stop.

### Wireframe — mobile (bottom-docked, above the existing pane tab row per the density rabbit hole)

```
┌───────────────────────────────────────────┐
│         (pane content area)                │
├───────────────────────────────────────────┤
│ existing mobile pane tab row (unchanged)   │  ← "Pane switcher" tablist
│  [ Session A ] [ Session B ] [ + ]         │
├───────────────────────────────────────────┤
│ ← swipe →   Window 1 ● Reviewing PR#x  +   │  ← NEW "Window switcher" tablist
└───────────────────────────────────────────┘
```

- Window strip sits **below** the pane strip, closest to the thumb, and uses a visually distinct
  background tone (not just label text) so the two rows are distinguishable at a glance without
  reading — this directly answers the "mobile tab-strip density" rabbit hole.
- Tap targets: match `mobileTabButton`'s existing size in `mobilePaneTabStrip.css.ts`, verified
  ≥ 44×44px (practical floor) even with two stacked rows present.

### Interaction flow

| User action | System response |
|---|---|
| Click/tap an inactive tab | `switchToWindow(id)` navigates (`router.replace` with new `?window=`) → that window's `PaneState` renders in place, focus moves to the newly active tab, `aria-selected` flips. Perceived latency target: <100ms (NFR). |
| Arrow Left/Right while strip has focus | Moves focus to adjacent tab and immediately activates it (automatic activation, per `research/ux.md` §3 — no separate confirm step, matches the instant-switch NFR). Home/End jump to first/last tab. |
| Click `+` | See Surface 3 (Create window). |
| Double-click (desktop) / long-press (mobile) a tab label | See Surface 5 (Rename). |
| Click the close affordance on a tab (small `×`, visible on hover/focus, always visible on the active tab) | See Surface 6 (Close). |
| `F2` while a tab has keyboard focus | Enters rename mode on that tab, same as double-click/long-press. See Surface 5 (Rename). |
| `Delete` while a tab has keyboard focus | Closes that tab, same as clicking `×` (including last-window auto-recreate). See Surface 6 (Close). |

### Error / edge cases

| Case | Behavior |
|---|---|
| Only one window exists | Strip still renders (single tab + `+`); the single tab has no close `×` shown at all — closing the only window is only reachable via the explicit Close flow's own last-window handling (see Surface 6), not hidden behind a disabled control. |
| Window name is very long | Ellipsis-truncated in the fixed-width tab; full name via `title` attribute and on focus/hover tooltip. |
| Many windows overflow the strip width | Strip becomes horizontally scrollable (native overflow-x, drag-to-scroll on desktop, swipe-to-scroll on mobile — distinct from the swipe-to-*switch* gesture, see Surface 8's disambiguation) rather than wrapping to a second row, keeping the strip's height fixed on mobile. |

---

## Surface 3: Create window

### Flow

1. User clicks `+` at the end of the strip (or invokes a future keyboard shortcut — out of scope
   for v1 per requirements; `+` click is the only trigger this version ships).
2. `useWindowManager.createWindow()` generates a new `WindowId`, dispatches `CREATE_WINDOW` with
   a default name `"Window <N>"` (N = count of existing windows + 1, not reused from closed
   windows — e.g. after closing Window 2, the next new window is still "Window 3", not a
   recycled "Window 2", so names never silently collide with a just-closed window a user might
   still remember).
3. `switchToWindow(newId)` navigates the current tab to `?window=<newId>` immediately — the user
   lands directly in the new, empty window (matches "id generation happens synchronously at the
   hook layer" from the plan's Pattern Decisions).
4. New tab appears in the strip, already active, with the default name in an **inline-editable**
   state is NOT auto-entered — the tab shows the default name normally; renaming is a deliberate
   separate action (Surface 5), not forced on every creation. This avoids interrupting a user who
   just wants a blank window right now.

### Wireframe (before → after)

```
Before:  [ Window 1 ● ][ Reviewing PR#x ]              [+]
                     click ↓
After:   [ Window 1 ][ Reviewing PR#x ][ Window 3 ● ]  [+]
                                        ▲ new, active, empty pane tree
```

### Error / edge cases

- `createWindow()` cannot fail under normal operation (no validation, no async call) — there is
  no error state for this action itself. The only failure mode is the *persistence* layer's
  cross-tab revision conflict on save, which is covered under Surface 9/11, not here.

---

## Surface 4: Switch window (click / leader-key / swipe)

All three triggers converge on the same `switchToWindow(id)` call — this surface documents the
shared response; Surfaces 7 and 8 document trigger-specific UX only.

### Interaction flow

1. Trigger fires (click, leader-key digit/`n`/`p`, or swipe past threshold).
2. `switchToWindow(id)` → URL updates (`router.replace`, no full navigation/reload) →
   `currentWindowId` recomputes → `PaneTilingContainer` receives the new window's `paneState` as
   props → re-render.
3. Visual: active-tab indicator moves instantly (no transition longer than ~150ms — a brief
   crossfade on the pane content area is acceptable polish, but must not be the *only* signal of
   which window is active; the tab strip's `aria-selected`/underline is the authoritative
   indicator).
4. Focus: after a leader-key or swipe switch, focus moves to the newly active window's pane
   content (not to the tab strip itself) so the user can immediately keep working — this differs
   from arrow-key navigation *within* the strip (Surface 1/2), which intentionally keeps focus on
   the strip for continued arrow navigation.

### Error / edge cases

| Case | Behavior |
|---|---|
| Target window no longer exists (e.g. closed from another tab between action and execution) | `useWindowUrlSync`'s self-heal path fires: falls back to `lastFocusedWindowId` if still valid, else `windows[0].id`, and calls `router.replace` to correct the URL silently — no error toast, no dead end. Same mechanism as an invalid `?window=` param (Surface 9). |
| Switch attempted while `windows` array is mid-reconciliation from a cross-tab `storage` event | Switch is queued against the *resolved* post-reconciliation list, not the stale one — never switches to a window id from a state snapshot the tab no longer holds. |

---

## Surface 5: Rename window (double-click desktop / long-press mobile / `F2` keyboard)

### Wireframe

```
Idle:        [ Reviewing PR#x ]
Desktop dbl-click / mobile long-press (500ms, stationary) / F2 while tab has focus ↓
Editing:     [ [Reviewing PR#x_____] ]   ← inline <input>, auto-focused, text pre-selected
Enter / blur with non-empty text  → commits new name, exits edit mode
Escape, OR blur with empty text   → reverts to previous name (cancel), exits edit mode
```

### Interaction flow

1. Desktop: double-click the tab label → tab label swaps for an inline text `<input>`,
   auto-focused, existing name pre-selected (so typing immediately replaces it).
2. Mobile: long-press (500ms stationary hold, see Surface 8's disambiguation) → same inline
   input, virtual keyboard opens.
3. Keyboard: with a tab focused via Tab/arrow-key navigation (Surface 1/2), pressing `F2` opens
   the same inline input — chosen over `Enter` because arrow-key movement in the strip already
   auto-activates (switches) the focused tab (Surface 1/2's interaction flow), so `Enter` has no
   free meaning left to claim here without colliding with that behavior.
4. Commit: `Enter` or blur with non-empty trimmed text → `RENAME_WINDOW` dispatched, input closes,
   tab shows new name.
5. Cancel: `Escape`, or blur with an empty/whitespace-only value → **no-op**, reverts to the
   previous name silently, no validation error message shown (per `research/ux.md` §4 — treat
   empty as cancel, not error).
6. Duplicate names across windows are allowed with no warning (windows are addressed by id/order
   internally, never by name — per `research/ux.md` §4).

### Error / edge cases

| Case | Behavior |
|---|---|
| Rename committed from Tab A while Tab B is currently displaying that same window | Tab B's tab strip label updates live via the `storage`-event cross-tab reconciliation (no manual reload) — see Surface 9. |
| Long-press triggers while the touch then moves (user was actually trying to scroll/swipe) | Long-press is cancelled, event falls through to swipe handling (Surface 8) — movement-based disambiguation, not timing alone. |

---

## Surface 6: Close window (including last-window auto-recreate)

### Wireframe

```
[ Window 1 ][ Reviewing PR#x ✕ (hover/focus only) ][ Debug y ]
                              ▲ click ✕, or Delete while the tab has keyboard focus
                                (plain Delete, not Ctrl+W — Ctrl+W is already claimed
                                at the pane level per requirements.md; the leader-key
                                sequence still handles switch only, not close)
```

### Interaction flow

1. User clicks the `×` on a tab (visible on hover/focus for inactive tabs, always visible on the
   active tab so it's discoverable without hovering), **or** presses `Delete` while that tab has
   keyboard focus (via Tab/arrow-key navigation, Surface 1/2) — both trigger the identical
   `closeWindow(id)` path below.
2. **No confirmation dialog** — per requirements.md's Rabbit Holes resolution and `ux.md` §4:
   there is no "unsaved data" concept (everything is already persisted local state), so closing
   is a single, immediately-reversible-in-spirit action (the window's last-known layout is gone,
   but nothing is destroyed that couldn't be rebuilt as easily as it was built).
3. `closeWindow(id)` dispatches `CLOSE_WINDOW`. If other windows remain, the strip re-renders
   without that tab, and if the closed window was active, the tab immediately to its left becomes
   active (or the new first tab, if it was leftmost) — never lands on a blank/no-selection state.
4. **Last window closed**: `windowReducer`'s `CLOSE_WINDOW` case auto-recreates a fresh blank
   window (via the `replacement` payload) rather than blocking — the strip never shows zero tabs.
   The user sees the tab strip go from `[ Only Window ✕ ]` directly to `[ Window 1 ]` (a new blank
   window, same default-naming rule as Surface 3) with no dialog, no flash of an empty state.

### Error / edge cases

| Case | Behavior |
|---|---|
| User closes the window another browser tab is currently displaying | The other tab's `useWindowUrlSync` self-heal fires (its `?window=` param now points at a nonexistent id) — same fallback chain as Surface 4's stale-target case: falls back to `lastFocusedWindowId`, else `windows[0].id`, silently corrects the URL. The affected tab does **not** show an error; it simply lands on a different (valid) window, and a subtle one-line inline notice ("This window was closed in another tab — showing Window 1") is shown for ~4s so the switch isn't confusing, then auto-dismisses. |
| Rapid repeated close clicks (double-click the `×`) | `closeWindow` is idempotent against an already-removed id (no-op if id not found) — no crash, no duplicate-close error. |
| `Delete` pressed on the focused tab, repeatedly | Keyboard focus moves with the roving `tabindex` to whichever tab becomes active per step 3/4 above (left-neighbor, new-first-tab, or the auto-recreated blank window), so a second `Delete` keypress immediately closes the next tab without an extra Tab keystroke to re-focus the strip. |

---

## Surface 7: Leader-key sequence (`Alt+W` then digit/`n`/`p`/`,`/`Escape`)

### Flow diagram

```
Idle ──Alt+W──▶ Armed (leaderArmed = true, 3000ms timeout starts)
                    │
                    ├─ digit 1-9 ──▶ switchToWindow(windows[digit-1].id) ──▶ Idle
                    ├─ "n"        ──▶ switchToWindow(next window, wraps) ──▶ Idle
                    ├─ "p"        ──▶ switchToWindow(prev window, wraps) ──▶ Idle
                    ├─ ","        ──▶ enters rename mode on the ACTIVE tab (Surface 5) ──▶ Idle
                    ├─ "Escape"   ──▶ Idle (explicit cancel)
                    ├─ any other key ──▶ Idle (disarm, key falls through unconsumed —
                    │                     e.g. typed into a terminal pane, never swallowed)
                    └─ 3000ms elapse, no follow-up ──▶ Idle (auto-disarm)
```

### Interaction flow

1. User presses `Alt+W` anywhere the app has focus (registered via `ShortcutRegistry`, matching
   the existing single-shortcut convention — no core registry change).
2. A small, transient visual indicator appears (e.g. a pill near the window tab strip reading
   "Leader: waiting for window key…") — required so the user isn't guessing whether the chord
   registered; disappears on disarm (any exit path above).
3. Follow-up key resolves per the diagram. Digit-out-of-range (e.g. `5` when only 3 windows exist)
   is a no-op disarm — not an error, since tmux itself silently no-ops on a nonexistent window
   number.

### Error / edge cases

| Case | Behavior |
|---|---|
| `Alt+W` collides with a browser/OS binding (e.g. an `Alt`-menu-accelerator) on some platform | Per plan's Unresolved Questions — spike-verified at implementation time; documented escape hatch is `Alt+Shift+W` if a collision is found, same pattern as `Ctrl+-`/`Ctrl+Shift+H` elsewhere in `usePaneShortcuts.ts`. UX-wise, whichever leader key ships must appear correctly in the `?` shortcut overlay (Surface 13) — the wireframe above is leader-key-agnostic. |
| User presses `Alt+W` while an `<input>`/`<textarea>` (e.g. a rename field, Surface 5) has focus | Shortcut is suppressed inside text-entry contexts (existing `ShortcutRegistry` convention for other shortcuts) — typing literal Alt+W in a text field is never hijacked. |
| User sends a real tmux `Ctrl+b` prefix into a terminal pane (e.g. to run their own tmux window/pane commands inside the session) | Does not arm this app's leader mode at all — `Alt+W` shares no keys with tmux's own default prefix (`Ctrl+B`) or its common `screen`/remap alternative (`Ctrl+A`), eliminating the collision class described in `pre-mortem.md` P1 #1/Failure Mode #1 rather than requiring a terminal-focus exclusion. |
| Leader armed, then focus moves to a different pane/tab entirely (e.g. via mouse click) before a follow-up key | Auto-disarms on blur, not just timeout — prevents a stale "armed" state surviving a context switch the user didn't intend as part of the sequence. |

---

## Surface 8: Swipe gesture (mobile)

### Flow

```
Touch down on WindowTabStrip container
        │
        ├─ moves > threshold (few px) horizontally before 500ms ─▶ SWIPE mode
        │        │                                                   │
        │        ├─ net horizontal distance > swipe-commit threshold  │
        │        │        on release ──▶ switchToWindow(next/prev)   │
        │        └─ released before threshold ──▶ no-op (snap back)  │
        │
        └─ stays within threshold for 500ms ─▶ LONG-PRESS mode (Surface 5's rename)
                 any subsequent movement in this mode is suppressed (no swipe fallback
                 once long-press has already committed to rename mode)
```

- Swipe left → next window; swipe right → previous window (matches horizontal tab-order
  convention, consistent with `n`/`p` leader-key direction).
- Visual feedback during drag: tab strip content shifts slightly with the finger (rubber-band),
  snapping fully to the target window on commit or snapping back on cancel — never leaves the
  strip in a visually "half-switched" resting state.
- This gesture is scoped to the `WindowTabStrip` container only — it must not fire from a swipe
  that starts inside the pane content area or the existing pane tab row, so it can't be confused
  with any pane-level touch interaction.

### Error / edge cases

| Case | Behavior |
|---|---|
| Swipe past the last/first window | No wrap-around for swipe specifically (unlike `n`/`p` leader-key, which does wrap) — swiping right past Window 1 simply snaps back with no window change, matching the common "can't scroll past the edge" mobile convention for a bounded list. This is an intentional asymmetry from the leader-key `n`/`p` wrap behavior, called out here so it isn't "fixed" later as a bug. |
| Swipe starts on the tab strip's own horizontal-scroll region (Surface 1/2's overflow case) when there are more tabs than fit | Native scroll (dragging the strip to reveal more tabs) takes priority over the switch-gesture when the strip itself is scrollable and not yet at a scroll boundary; the switch-commit gesture only engages once the strip is at a scroll edge (start/end), mirroring how horizontal-swipe-to-navigate works in browsers/OSes that also scroll horizontally lists. |

---

## Surface 9: Per-tab URL binding (`?window=<id>`)

### What the user experiences

1. **Copying the current URL and opening it in a new tab**: the new tab loads showing the exact
   same window as the tab it was copied from (same `?window=<id>` param resolves to the same
   `NamedWindow`). If that tab later switches windows, the original tab is unaffected — each
   `?window=` param is independent per tab, per requirements.md's explicit success metric.
2. **Bookmarking a `?window=<id>` URL and returning later**: if that window still exists, it loads
   directly into it. If the window was closed in the meantime (by any tab), the self-heal path
   (Surface 4/6) silently redirects to a valid window and briefly explains why (the same
   inline notice pattern as Surface 6's cross-tab close case).
3. **Opening the app fresh with no `?window=` param at all** (e.g. typing the bare root URL): the
   app falls back to `lastFocusedWindowId`, or `windows[0].id` on a true first load, and then
   `router.replace`s the URL to include the resolved `?window=` param — so after the very first
   render, the URL always reflects the tab's actual window, ready to be copied correctly.
4. **Manually editing the URL to an id that never existed** (typo, stale bookmark from a deleted
   window): same self-heal fallback as case 2 — never a blank page or a thrown error.

### Wireframe (address bar, illustrative)

```
Tab A:  https://cockpit.example/…?window=win-a3f91c2b   →  shows "Reviewing PR#x"
Tab B:  https://cockpit.example/…?window=win-7e02d914   →  shows "Debug y"
        (switching Tab A's window does not change Tab B's URL or content)
```

### Error / edge cases

| Case | Behavior |
|---|---|
| Two tabs both editing the shared `windows` list concurrently (e.g. Tab A renames Window X while Tab B splits a pane in Window Y) | Optimistic-Offline-Lock revision guard (ADR-002) resolves the write race at the persistence layer; from the UX side, each tab's own in-progress edit is never silently lost — the losing writer's save retries against the latest revision rather than overwriting the other tab's change, and both tabs converge via the `storage`-event listener within one debounce cycle (no manual reload, no visible conflict dialog). |
| `?window=` param present but malformed (not matching any generated id shape) | Treated identically to "window no longer exists" — self-heal fallback, `router.replace` to a valid id. |

---

## Surface 10: tmux-vocabulary onboarding hint

Per `research/ux.md` §2's recommendation: a lightweight, dismissible hint for users who know tmux
and might expect "window" to nest *inside* "session" (backwards from this app's actual hierarchy).

### Wireframe

```
┌─────────────────────────────────────────────────────────────────────┐
│  ⓘ Windows group your panes into separate workspaces — like a tmux   │
│    window, but the "sessions" here are your agent panes, not tmux    │
│    sessions.                                              [Got it ✕] │
└─────────────────────────────────────────────────────────────────────┘
   (appears once, anchored below the Window Tab Strip, first time a
    second window is ever created — not on first load with only Window 1,
    since the concept is meaningless until there's more than one window
    to compare)
```

### Interaction flow

1. Trigger: the moment `windows.length` transitions from 1 → 2 for the first time on a given
   browser profile (tracked via a small localStorage flag, e.g. `cockpit.windowHintDismissed`,
   distinct from the main persisted schema so it never participates in the revision guard).
2. Dismiss: `[Got it ✕]` sets the flag permanently; hint never reappears. No auto-timeout — this
   is informational, not transient, so it should not vanish before being read, but it also never
   blocks any interaction underneath it (non-modal).

### Error / edge cases

- If localStorage is unavailable/full when trying to set the dismissed flag, the hint may
  reappear on a later session — acceptable degraded behavior (informational only, never a
  functional error), no special handling needed.

---

## Surface 11: `cockpit.windowLayout` v2 schema (non-interactive)

```json
{
  "version": 2,
  "revision": 3,
  "windows": [
    {
      "id": "win-a3f91c2b",
      "name": "Reviewing PR#x",
      "paneState": { "root": { "...": "..." }, "focusedPaneId": "p1", "zoomedPaneId": null }
    }
  ]
}
```

Acceptance criteria:
- Loading a well-formed v2 blob never mutates it before rendering (round-trips exactly).
- A v1 `cockpit.paneLayout` blob with no v2 key present migrates into a one-window v2 blob and is
  written through to `cockpit.windowLayout` exactly once; `cockpit.paneLayout` itself is never
  deleted or rewritten (rollback safety, per ADR-003).
- An unrecognized `version` value (neither 1 nor absent-implying-legacy, nor 2) fails closed:
  logs a client-side error (Surface 12) and falls back to a fresh blank window rather than
  crashing the app or silently discarding data.
- `revision` strictly increases on every successful save from any tab.

## Surface 12: migration/error console logging (non-interactive)

- Every fail-closed branch (unknown version, corrupt JSON, cross-tab revision conflict) logs via
  `console.error` with enough context to diagnose (window count, revision numbers) but never pane
  content (per requirements.md's Observability Requirements — no session content in logs).
- No user-facing error toast for these — they degrade to a working blank/fallback state per
  Surface 11, so the log is for developer diagnosis, not user notification.

## Surface 13: `?` shortcut overlay entry (non-interactive from a design standpoint — no new UI shell, only a new row)

Acceptance criteria:
- The existing `?`-triggered shortcut overlay (`KeyboardShortcutOverlay.tsx` /
  `ShortcutHelpOverlay.tsx`) gains one new row/section for window shortcuts, listing the resolved
  leader key (`Alt+W` or its escape-hatch alternative) and its follow-ups (`1`-`9`, `n`, `p`,
  `,`) — those are registered through `useWindowShortcuts` exactly like every other row is
  registered through `ShortcutRegistry`, no bespoke overlay markup for this feature — plus a
  static entry for the two tab-focus shortcuts `F2` (rename focused tab, Surface 5) and `Delete`
  (close focused tab, Surface 6). Those two are per-tab `onKeyDown` handlers on the focused tab
  element itself (`implementation/plan.md` Task 3.1.1d/3.1.2a), not `ShortcutRegistry` entries —
  they're listed in the overlay for discoverability alongside the leader-key rows, but don't ride
  the same single-source-of-truth mechanism the leader key's escape-hatch note below describes.
- If the leader key ships as the escape-hatch alternative (`Alt+Shift+W`) due to a platform
  collision, the overlay reflects that automatically (single source of truth: whatever
  `useWindowShortcuts` actually registered), not a hardcoded `Alt+W` string in the overlay
  component.

---

## UX Acceptance Criteria

**Task completion:**
1. A user can create a new window and land in it in 1 click (the `+` button) — no dialog, no
   naming prompt required.
2. A user can switch to any of up to 9 open windows in 2 keystrokes (`Alt+W` then a digit) with
   no mouse involvement.
3. A user can rename a window in 1 interaction (double-click/long-press) + typing + Enter — no
   separate "confirm rename" step.
4. A user can close a window in 1 click, with no confirmation dialog, and never ends up looking
   at zero windows.
5. A user can copy the current tab's URL into a new browser tab and see byte-for-byte the same
   window, in 0 additional steps (no re-navigation required).

**Error states:**
6. Every self-heal path (stale/missing/malformed `?window=` param, window closed by another tab)
   silently resolves to a valid window and shows a transient (~4s), non-blocking inline notice
   when the cause was a cross-tab action — never a full-page error, never a blank screen, never a
   modal the user must dismiss to keep working.
7. Rename-to-empty and rename-to-duplicate are both handled with zero validation-error UI (revert
   silently / allow silently, respectively) — no error state exists for either case because
   neither is actually invalid.
8. No dead ends: every state reachable through this feature (armed leader-key with no valid
   follow-up, swipe past the first/last window, closing the last window, a stale URL) has a
   defined next state that leaves the user able to keep working immediately, with no dialog to
   dismiss and no reload required.

**Accessibility:**
9. Both the Window Tab Strip and the existing Pane Tab Strip are independent, correctly labelled
   `role="tablist"` regions (`aria-label="Window switcher"` vs. `"Pane switcher"`) — verifiable
   via `axe-core`/Lighthouse in the existing PR-triggered UX-analysis CI (per this repo's
   `CLAUDE.md`, "UX analysis CI ... runs on PRs touching `web-app/src/`").
10. The Window Tab Strip is fully keyboard-operable without a mouse or touch: Tab key reaches it
    as a single stop (roving tabindex), Left/Right/Home/End move and immediately activate, the `+`
    button is reachable as the next Tab stop, `F2` on a focused tab enters rename mode (Surface 5,
    equivalent to double-click/long-press), and `Delete` on a focused tab closes it including the
    last-window auto-recreate case (Surface 6, equivalent to clicking `×`) — every mouse/touch-only
    interaction documented for the strip has a keyboard equivalent.
11. Active-window indication is never color-only — it must also be encoded via `aria-selected`,
    bold weight, and an underline/accent bar, so it remains legible under a color-vision
    deficiency simulation and meets ≥ 4.5:1 contrast for the text itself against its background
    in both the active and inactive state.
12. All interactive elements in the strip (tabs, `+`, per-tab `×`, inline rename input) have an
    accessible name distinct from their sibling elements (verifiable by `aria-label`/`title`/
    visible text — no two controls share an identical accessible name that would be ambiguous to
    a screen reader user tabbing through the strip).
13. Touch targets in the mobile strip are ≥ 44×44 CSS px (practical floor, exceeding WCAG 2.2 SC
    2.5.8's 24×24px legal minimum), verified against whatever size `mobileTabButton` already uses
    in `mobilePaneTabStrip.css.ts` so the two stacked mobile tab rows are consistent with each
    other, not just individually compliant.

**Performance:**
14. Window switch (any trigger) completes with no visible re-render jank and meets the < 100ms
    perceived-switch-time NFR from requirements.md — testable by a human as "the new window's
    content appears to replace the old one with no flash of blank/loading state."
