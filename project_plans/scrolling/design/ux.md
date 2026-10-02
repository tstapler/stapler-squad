# UX Design: mobile terminal touch scrolling and keyboard redraw

No new screens. This changes the behavior of one existing surface (the terminal view on touch devices) and its interaction with the on-screen toolbar and soft keyboard. Inputs: `requirements.md`, `research/ux.md`. Research constants (slop, friction, velocity cap) are starting values to tune on device, not requirements.

## Surfaces

| # | Surface | Type | Treatment |
|---|---|---|---|
| S1 | Terminal touch-drag scroll (with momentum) | Interactive gesture | Full |
| S2 | Gesture disambiguation (scroll / tap / long-press / double-tap) | Interactive gesture | Full (table) |
| S3 | Keyboard open/close/rotate redraw | Automatic recovery, no direct input | Full (flow + error table) |
| S4 | Toolbar PgUp/PgDn keys | Existing keys, made route-aware: the WCAG 2.5.1 single-pointer equivalent of the drag | Full (equivalence rule) |
| S5 | Debug log on forced refresh | Non-interactive | Condensed |
| S6 | Scrolling settings panel: scroll-mode override (`auto` / `local` / `tui`), Gesture scrolling on/off, mode chip | Settings, persisted | Full |
| S7 | Jump-to-latest affordance and new-output-while-scrolled behavior | Visible control plus rules | Full |
| S8 | Manual "Redraw" action | Existing always-visible toolbar button, relabelled | Condensed |
| S9 | Gesture state-transition table | Reference | Full (table) |

## S1. Touch-drag scroll

```
 Terminal surface (touch-action: none)
 +--------------------------+
 |  earlier output          |   finger drags DOWN  -> reveals EARLIER output
 |  ...                     |   finger drags UP    -> reveals LATER output
 |  ...            (finger) |   local xterm buffer: content follows the finger 1:1
 |  latest output           |   TUI modes: page-stepped, capped, rate-limited
 +--------------------------+
 [ Esc Tab Ctrl ... PgUp PgDn ]   toolbar, usable with keyboard closed
```

Success statement (decision 2026-10-01): **1:1 in the local xterm buffer; page-stepped, capped and rate-limited in TUI modes (Claude Code etc.).** The TUI exception is accepted, not a defect.

Routing is decided by `decideScrollTarget(mode, policy, tuiPolicy, override)` over the `ScrollRoutingPolicy` decision table in plan.md (rows keyed on `bufferType` and `mouseTrackingMode`, first match wins, user override short-circuits it). `bufferType` alone is not the verdict: Claude Code may render in the normal buffer and a plain shell inside tmux may report the alternate buffer. **Default rows until the device spike verifies them (decision 2026-10-01, reverses the earlier safe default):** `bufferType === 'alternate'` or `mouseTrackingMode !== 'none'` routes to `tui-pgkeys` (PgUp/PgDn bytes); `bufferType === 'normal'` with tracking `none` routes to `xterm-local`. `ROUTING_VERIFIED=false` marks these as unverified. Accepted misroute risk: a plain shell inside tmux reports the alternate screen, so its drag sends PgUp/PgDn (tmux copy-mode scrolls tmux history, bash ignores the key); a Claude Code pane in the normal buffer with tracking `none` routes local and does nothing. The S6 override (`local` / `tui`) fixes either case, and the spike then confirms or adjusts the rows post-hoc. The tmux copy-mode note in plan.md Story 0.1.3 still applies: PgUp in tmux enters copy-mode, PgDn at the bottom exits it, and a stale copy-mode can swallow keystrokes meant for the app.

```
 touchmove --> accumulate dy --> once per rAF --> ScrollRoutingPolicy decides target
                                                   |
        xterm-local ------> whole lines (carry fractional remainder) -> terminal.scrollLines(n)   [1:1]
        tui-pgkeys -------> step accumulator (step = half a page)    -> \x1b[5~ / \x1b[6~          [page-stepped]
        tui-wheel --------> SGR wheel reports (only if spike Q1 proves it works)                  [capped]
```

Flow:
1. Finger lands: any running momentum stops immediately; no focus, no keyboard.
2. Finger moves past the slop (`SLOP_PX`, **default 15 px**; 10 px is a tuning candidate adopted only if D9 shows 15 px fails criterion (c) below; **tap tolerance equals the slop**, so there is no dead zone: a release under the slop before 400 ms is a tap, see S2/S9): scroll mode begins, long-press timer cancelled. The accumulator is seeded with the overshoot beyond the slop only (travel minus `SLOP_PX`), so content starts moving from the crossing point with no initial jump; the fixed finger-to-content offset that results (`SLOP_PX`) is accepted unless device tuning (below) says otherwise.
3. While down: in `xterm-local` content tracks the finger 1:1; in TUI modes one page key is sent each time cumulative post-slop travel reaches a **half page** (`TUI_PAGE_STEP_LINES = max(1, floor((rows-1)/2))`; a full-page threshold made a half-screen drag do nothing and read as broken; tuned on device, D9). So a half-screen drag moves the app one page (2x gain, accepted). One dispatch per animation frame. The first TUI-routed drag shows the one-time hint described in S6.
4. Release with velocity above the canonical momentum constants (below): momentum with exponential decay. TUI modes: momentum is bounded by the per-fling page cap (5) and the 100 ms page rate limit, then cancelled.
5. `xterm-local` momentum ends at the decay threshold or the top/bottom boundary (`viewportY` at 0 or `baseY`; no bounce). TUI modes cannot detect a boundary (the app owns its scrollback): momentum ends at the page cap or decay threshold.
6. Release below the min fling velocity: stops, no momentum.

Canonical momentum constants (one set, exported as `MOMENTUM_CONSTANTS` in `lib/terminal/scrollKinematics.ts`; starting values, tune on device): velocity window 100 ms; min fling velocity 0.3 px/ms; decay `v *= 0.95` per 16 ms frame; stop below 0.02 px/ms; velocity cap 8 px/ms; max 120 frames.

**Scroll axis**: only vertical travel scrolls. The gesture locks to vertical when the first move past slop has |dy| >= |dx|; a drag that starts more horizontal than vertical (|dx| > |dy|) is ignored by the scroll path (no scroll, no tap, no selection) until touchend. Diagonal drags after a vertical lock scroll by dy only. Horizontal pan of wide output is not provided by this project: long lines wrap at the terminal width, and a horizontal drag near the left screen edge is left for Android's back-swipe (the handler does not `preventDefault` before the vertical lock). Wide-output reachability is an accepted gap, recorded in "Flows lacking an exit path".

**Pinch-zoom and `touch-action: none`**: `touch-action: none` on the terminal surface disables browser pinch-zoom there (WCAG 1.4.4 resize text, 1.4.10 reflow). Alternative that meets the criterion: the terminal font-size setting (XtermTerminal font-size prop) already scales text to 200% without browser zoom; the plan verifies it is reachable on mobile and offers at least a 200% step (device check D8). The **Gesture scrolling: Off** setting (S6) also restores `touch-action` and therefore browser pinch-zoom over the terminal. Pinch is not intercepted by the handler (multi-touch ignored), so pinch-zoom outside the terminal and the page-level zoom of the toolbar still work.

**TalkBack and screen readers (concrete fallback)**: single-finger explore-by-touch and double-tap activation use touch events that `preventDefault` over the canvas can swallow. Page JavaScript cannot detect TalkBack or any accessibility service (no web API exposes it), so the project does not try to auto-detect it. Fallback: a persisted **Gesture scrolling** on/off setting (S6; localStorage `terminal-gesture-scroll`, default on). **Off** means the gesture hook registers no touch listeners on the terminal surface, never calls `preventDefault`, and the surface returns to the browser's default `touch-action`, so native accessibility gestures are untouched; the toolbar PgUp/PgDn keys (S4, route-aware) remain the way to scroll. Costs of Off, stated in the panel: no drag-scroll, no long-press selection, no double-tap word select (tap focus falls back to xterm's native click handling plus the always-visible keyboard toggle; unverified, D8). The setting is a manual switch: a screen-reader user finds it through the toolbar, which is native buttons, and the panel text says "Turn off if you use a screen reader". D8 verifies on a physical phone with TalkBack that, with the setting Off, the terminal region and toolbar are explorable and operable and **that the user can focus the terminal and type** (must-pass: tap-to-focus then falls back to unverified xterm click handling, so a failure here means a TalkBack user cannot open the keyboard and the fix is a focus affordance, not a record-only note), and records whether the default (On) already works (if it does, the setting stays as an escape hatch, not a requirement). **Out of scope**: making terminal *content* readable by a screen reader (xterm's `screenReaderMode`/accessibility tree). Reason: the content is a continuously rewritten TUI screen, the feature has its own performance and verbosity design questions, and this project is about touch scrolling and redraw; recorded as a follow-up.

Interruptions: new touch cancels momentum (and is not a tap-to-focus). A keyboard open/close, `visualViewport` resize, or orientation change during an active PENDING/SCROLLING drag or COASTING cancels the gesture, resets the line/page remainder, and ignores the rest of that touch until a new `touchstart` (no stale deltas, no jump). Multi-touch (`touches.length > 1`) is ignored by the handler so TalkBack and system two-finger gestures are untouched. `prefers-reduced-motion: reduce` (read at gesture start) disables momentum while the drag still works.

| Situation | Behavior | Exit path |
|---|---|---|
| TUI/alt-screen session, no xterm scrollback | Routed per `ScrollRoutingPolicy` (default until spike: alternate screen or mouse tracking forwards page keys to the app; unverified, may misroute a plain tmux shell) | S6 override (`local` or `tui`); PgUp/PgDn keys (S4) |
| App ignores forwarded wheel/PgUp | No feedback possible; drag simply has no effect | PgUp/PgDn keys |
| At top/bottom boundary (`xterm-local`) | Stop momentum, no overscroll bounce, no pull-to-refresh | Drag the other way |
| Fling in TUI mode | Stops at page cap or decay; no boundary knowledge | Drag again, PgUp/PgDn keys |
| Keyboard opens or device rotates mid-gesture | Gesture and momentum cancelled; remainder reset; no stale deltas applied | Start a new drag |
| Selection mode active | Drag extends selection, never scrolls | Tap anywhere to clear the selection (that tap does not open the keyboard) |
| Scrolled away from live output | S7: "Jump to latest" button visible; position held while output arrives | Tap the button, or drag/PgDn to the end |
| Horizontal or edge-swipe drag | Ignored by the scroll path; Android back-swipe left to the OS | Drag vertically |

**On-device tuning criteria** (`SLOP_PX` 15 px default with 10 px as a fallback candidate, TUI step half a page, decay 0.95, min fling 0.3 px/ms, velocity cap 8 px/ms, 120 frames are starting values, tuned in the Phase 0 spike and again after Story 1.2.2): (a) a 1.2 px/ms fling in a local-buffer session coasts between 0.5 and 1.5 screens of content, comparable to a native Chrome list; (b) a slow deliberate drag never triggers a fling; (c) the finger-to-content offset after the slop crossing does not make the operator re-drag to reach a line: **the default is 15 px; try 10 px in D9 only if 15 px fails this criterion, and adopt it only if long-press and tap (tolerance moves with the slop) still pass**; (d) a flick that overshoots in TUI mode stops within the 5-page cap with no more than one page of surprise; (e) in TUI mode a half-screen drag moves the app by one page and a shorter deliberate drag is not silent for long (if half-page steps still feel dead or jumpy, try a quarter page or wheel ticks if Q1 allows). Pass is the operator's judgment on the phone against these five statements; record the final constants in the plan.

## S2. Gesture disambiguation

| Input | Result |
|---|---|
| Move > `SLOP_PX` (15 px default) before 400 ms | Scroll (cancels long-press timer) |
| Hold 400 ms with movement < slop | Selection mode; scroll disabled for the rest of that touch |
| Touch with total movement < `SLOP_PX` (15 px; **tap tolerance = slop**, widened from 8 px so a drifting thumb tap still taps; one intended anchor change) and < 400 ms | Tap: focus terminal, soft keyboard opens (a tap while a selection is active only clears it, see S9) |
| Two taps within 300 ms / 20 px | Double-tap word select (unchanged) |
| Touch that scrolled | Never tap-to-focus; `touchend` calls `preventDefault` when cancelable so no click/focus is synthesized (verify on device) |
| Touch that stopped momentum | Consumed; not a tap |
| First move past slop with |dx| > |dy| | Horizontal: ignored for scrolling (see S1); no tap on release |

## S9. Gesture state-transition table

States: `IDLE`, `PENDING` (touch down, under slop), `SCROLLING`, `COASTING` (momentum after release), `SELECTING` (long-press selection), `CANCELLED` (touch ignored until the next `touchstart`).

| From | Event | To | Effect |
|---|---|---|---|
| IDLE | touchstart (1 finger) | PENDING | Start long-press timer (400 ms); record start cell |
| IDLE | touchstart (2+ fingers) | CANCELLED | Ignored; system/TalkBack gestures untouched |
| PENDING | move > `SLOP_PX`, vertical | SCROLLING | Cancel long-press timer; seed accumulator with overshoot; `preventDefault` if cancelable |
| PENDING | move > `SLOP_PX`, horizontal | CANCELLED | No scroll, no tap |
| PENDING | move < `SLOP_PX` (any drift under the slop, no dead zone) then touchend < 400 ms | IDLE | Tap: focus and open keyboard, unless a selection is active (then clear the selection only, no focus); second tap within 300 ms/20 px: word select |
| PENDING | hold 400 ms, movement < slop | SELECTING | Scroll disabled for this touch |
| SCROLLING | touchend, velocity >= min fling, no reduced motion | COASTING | Start momentum; `touchend` `preventDefault` if cancelable |
| SCROLLING | touchend, slow | IDLE | Stop; `touchend` `preventDefault` if cancelable |
| SCROLLING | touchcancel (system gesture, edge swipe, call) | IDLE | Reset remainder; no momentum from a cancelled touch |
| SCROLLING / COASTING / PENDING | keyboard toggle, `visualViewport` resize, orientation change, S6 override change, reconnect or full-snapshot write (`connectionEpoch` change) | CANCELLED | Reset line/page remainder and `netPagesUp`; ignore rest of touch |
| COASTING | touchstart | PENDING | Cancel momentum; set `consumedByCoast` (the matching touchend is not a tap) |
| COASTING | momentum ends (decay, edge, page cap) | IDLE | |
| COASTING | touchcancel | IDLE | Reset state and `consumedByCoast` |
| SELECTING | move | SELECTING | Extend selection, never scroll |
| SELECTING | touchend | IDLE | Selection stays until the next tap |
| IDLE/PENDING | tap (touchend under slop, < 400 ms) while a selection is active | IDLE | **Clears the selection only; does not focus or open the keyboard.** The next tap focuses as usual. Rationale: the user's intent after selecting is to dismiss the highlight (often after copying), and an unwanted keyboard shifts the viewport |
| CANCELLED | touchend / touchcancel | IDLE | |

Scroll mode and selection mode are mutually exclusive per touch: the first of slop-crossing (SCROLLING) or 400 ms hold (SELECTING) wins and the other is disabled until the touch ends. Streaming output arriving in any state never changes gesture state (see S7).

## S3. Keyboard open/close redraw

```
 keyboard toggles
   -> visualViewport resize events (rAF-batched)
   -> cancel scroll gesture/momentum
   -> ViewportSettled (3 frames with stable `visualViewport` height and offsetTop, or 600 ms max wait; armed by vv resize and scroll; replaces the fixed 400 ms wait)
   -> refit()                            <- fits if dims changed, always repaints
   -> terminal.refresh(0, rows-1)        <- forced repaint (all renderers), guarantees content visible
   -> keep cursor line visible
```

Silent recovery: the user never sees an error or is asked to act. Target (outcome): content is visible promptly once the keyboard stops moving, no later than the 400 ms path it replaces, with the exact budget in plan.md (repaint timing budget); 0 blank occurrences in N open/close cycles on Android Chrome, with N chosen from the measured baseline (requirements.md). The target phone may run the canvas renderer (the app logs "WebGL2 unavailable (Android?), using canvas renderer"), so the repaint must not depend on WebGL-only calls.

| Failure | User sees | Recovery |
|---|---|---|
| Missed fit/refresh, zero-size canvas, or lost WebGL context | Black terminal, content off-screen (today) | Automatic `refit()` + refresh after `ViewportSettled`; a second refresh on next viewport event; manual fallback of close/reopen keyboard still works |
| Rotation | Same as keyboard resize | Cancel gesture and momentum, `refit()`, refresh, reset line remainder |
| Viewport never reports settled | Terminal stays at last good size | Bounded fallback timeout (600 ms) forces `refit()` + refresh |
| Still blank after the automatic recovery | Black terminal | S8 "Redraw" button in the always-visible toolbar row (one tap, keyboard closed); then close/reopen keyboard |

## S4. Toolbar PgUp/PgDn (route-aware; WCAG 2.5.1 equivalent)

WCAG 2.5.1 requires a single-pointer alternative that does the *same thing* as the path-based drag. The earlier design left the keys sending only bytes, which in an `xterm-local` session do not move xterm's history, so the alternative was not equivalent. Decision (2026-10-01): **the toolbar PgUp/PgDn follow the same effective route as the drag.**

| Effective route at tap time (`decideScrollTarget`, including the S6 override) | PgUp / PgDn do |
|---|---|
| `xterm-local` | `terminal.scrollPages(-1)` / `scrollPages(+1)`; no bytes sent |
| `tui-pgkeys` or `tui-wheel` | send `\x1b[5~` / `\x1b[6~` exactly as today (byte parity pinned by test) |
| any route with a Ctrl/Alt/Shift modifier armed | send the modified byte sequence as today (modifier maps in `TerminalOutput.tsx`), never `scrollPages` |

Rules:
- In `auto`, if the route is `xterm-local` but `scrollPages` cannot move (already at that edge), the key falls through to sending the bytes, which preserves today's proven behavior for a TUI that renders in the normal buffer with tracking `none` (the route misclassifies it as local). With an explicit `local` override there is no fall-through.
- Toolbar presses in a TUI route count toward the `netPagesUp` estimate (S7) exactly like drag-sent keys.
- Residual risk, stated plainly: in `auto`, a normal-buffer TUI that has xterm scrollback to move will be treated as local, so toolbar PgUp scrolls xterm history rather than the app until the spike corrects the rows or the user sets "Page keys" (UNVERIFIED; spike Q2, device D5).
- Acceptance bullets: visible and operable with keyboard closed; **touch targets are 44x44 CSS px (target) with 24x24 CSS px as the WCAG 2.5.8 AA floor; if the toolbar cannot fit 44x44 the keys may shrink to no less than 24x24 with at least 8 px spacing, and the shortfall is recorded in the PR**; measured on device (D5, D8); in a TUI route behavior is identical to before this change (regression anchor); in a local route the keys now scroll the same history the drag scrolls (AC27). The D5 check records which history PgUp reaches in a plain tmux shell and in Claude Code.

## S7. Jump to latest and output arriving while scrolled

- **Affordance**: a button with a fixed-width text label and a down-arrow icon (44x44 minimum), bottom-right of the terminal surface above the toolbar, appears whenever the view is away from live output and disappears at the bottom. The label is **fixed per route and never changes while the button is visible** (no layout shift on narrow screens): "Jump to latest" in the local buffer, "Page down to latest" in TUI mode (it does not promise "latest" because the app owns its position). The two strings are the only variants; a route change hides and re-evaluates the button. New output while scrolled is shown by a small dot on the button plus visually hidden text, not by a longer label.
- **Placement**: the button must not cover the cursor line or the last line of live output. If the cursor row's rectangle intersects the button's rectangle, the button moves to the top-right of the surface. **Relocation rule (to avoid a moving control): placement is evaluated only when the button becomes visible and when a gesture or fling ends, never mid-drag or mid-fling, and the chosen corner is kept until the button hides.** The button is hidden when fewer than `MIN_ROWS_FOR_OVERLAYS` (5) terminal rows are visible (landscape with the keyboard open); the toolbar keys still scroll.
- **Tap** (local buffer): `terminal.scrollToBottom()`, never focuses the terminal or opens the keyboard.
- **Detection (local buffer)**: `viewportY < baseY` (read once per frame during scroll and on output).
- **TUI mode (gated)**: the app's position cannot be read, so the hook keeps an estimate `netPagesUp` (page keys sent up minus down, counting drag and toolbar keys). The estimate is treated as **valid only while** (a) no user keystroke or paste has been sent since the last page key, (b) no resize, mode/override change, reconnect or session change has happened since, and (c) `netPagesUp <= 5`. While invalid the button is **hidden, not clamped** (the user falls back to PgDn). When valid and `netPagesUp > 0` the button shows and sends `min(netPagesUp, 5)` PgDn at the 100 ms page rate limit, then resets the estimate. The earlier cap of 20 is withdrawn: a wrong estimate injected up to 20 stray keys into the app. Whether PgDn x `netPagesUp` really returns Claude Code to live is INFERRED; D2f verifies it.
- **New output while scrolled up (local buffer)**: the viewport holds its position (no auto-follow); the button's dot appears. New output while the finger is down or momentum is running never programmatically scrolls (no jump under the finger). At the bottom, output follows as today.
- **New output while scrolled (TUI)**: the app owns its redraw and position; the project does not intervene; the button stays only while the estimate is valid.
- **Accessibility**: a real `<button>` with an accessible name. Live region: one polite announcement "New output below" **once per scrolled-away episode**, debounced 2 s, re-armed only after the view returns to the bottom; appearing/disappearing of the button itself and scrolling are never announced.
- **Reduced motion**: with `prefers-reduced-motion: reduce` the button appears and disappears without fade or slide, and the jump is instant (it always is: no smooth scrolling).
- **Reconnect, empty buffer**: a reconnect or full-snapshot write cancels any gesture, resets `netPagesUp` and recomputes the button from the buffer (usually hidden, as a snapshot returns the view to live). An empty or one-screen buffer has nothing to scroll: the drag is a no-op (no feedback needed) and the button stays hidden.

## S8. Manual "Redraw" action (condensed)

The terminal toolbar already has an always-visible "Resize" button outside the collapsible group (`TerminalOutput.tsx`, the button with `aria-label="Resize terminal to fit container"`, calling `handleManualResize`; VERIFIED present, its visibility on a narrow phone viewport is checked in D10). Decision: that button is the manual redraw, so it is discoverable without opening any menu. It is relabelled "Redraw" with `aria-label="Redraw terminal (fixes a blank screen)"` and a title of the same text, and `handleManualResize` calls `refit({ reason: 'manual-resize' })`: the same forced refit plus `refresh(0, rows-1)` path as the automatic recovery, one tap, no confirmation, no state change. It is not placed in the Scrolling panel. A user looking at a black terminal sees "Redraw" in the always-visible toolbar row; the keyboard toggle beside it is the other recovery they already know.

## S5. Debug log on forced refresh (condensed)

Format: a `MobileDebugLog` structured entry (flag `localStorage['debug-terminal-mobile']`, 500-entry ring buffer, dumped via `window.__termDebug.dump()`), e.g. `log('fit', { event: 'forced-refresh', rows: 30, renderer: 'canvas', reason: 'viewport-settle' })`. `reason` is one of `viewport-settle | visibility | manual-resize | font-change | context-loss` (the settle signal cannot tell keyboard from rotation, so those are not separate reasons).
- Debug level only (off by default); no user-visible output.
- Emitted once per forced refresh, not per frame.
- Includes the trigger reason and active renderer to help diagnose recurrence.

## S6. Scrolling settings panel (override, Gesture scrolling, mode chip)

Two persisted per-device settings plus a chip, in one small panel:

- `terminal-scroll-override` (`auto | local | tui`): sets the `override` argument of `decideScrollTarget`. The escape hatch when auto-routing misclassifies a session (normal-buffer TUI, plain shell inside tmux) and the only way to reach TUI routing reliably before the device spike has verified the table rows.
- `terminal-gesture-scroll` (`on | off`, default `on`): the TalkBack/screen-reader fallback from S1. Off = no touch capture over the canvas; toolbar keys (S4) remain.

**Placement (decided)**: the host is `TerminalOutput.tsx`'s toolbar. The `secondaryActions` array (rendered inline on desktop and in the mobile "More" overflow row) gets a "Scrolling" entry that expands an inline panel beneath the toolbar, built like the existing `mobileOverflowRow` (no new popover library). The mode chip sits in the always-visible actions row next to "Redraw" (reachable with the toolbar collapsed and the keyboard closed). Not on the drag surface.

**Chip tap opens a compact picker, not the full panel** (misroute recovery must be one tap to open and one to choose, not 3-4 taps): the picker shows only the three route radios (native radios, same labels and descriptions as below), **closes on select** and returns focus to the chip, and has a "More scrolling settings" button that opens the full panel (Gesture scrolling switch, help text, tmux note). The "Scrolling" toolbar entry opens the full panel directly. One `ScrollingPanel` component with a `variant: 'picker' | 'full'`.

**Panel and picker must not shrink the terminal below 5 rows**: if opening inline under the toolbar would leave fewer than `MIN_ROWS_FOR_OVERLAYS` (5) terminal rows (keyboard open on a small screen), the panel/picker renders as a scrollable overlay anchored under the toolbar (`max-height` = visible viewport minus toolbar minus 5 rows) instead of resizing the terminal, so opening it never triggers a refit. Under `prefers-reduced-motion: reduce` the panel, picker and hint open and close with no animation.

**Mode chip**: visible text is the effective route in plain words, "Page keys" (route `tui-*`) or "Terminal history" (route `xterm-local`), **always shown, including when Gesture scrolling is Off**, where a suffix is added: "Terminal history, gestures off". The accessible name carries the gesture state and the action: `aria-label="Scroll mode: Terminal history. Gesture scrolling on. Opens scroll settings"` (and `... Gesture scrolling off ...`). At least 44x44 CSS px target (24x24 floor). **Touch-detection rule**: shown when `matchMedia('(any-pointer: coarse)').matches` is true or a `touchstart` has been seen on the terminal in this page load; hidden otherwise (a mouse-only desktop never shows it; a touch laptop does, which is acceptable). Hidden when fewer than 5 terminal rows are visible (S7).

**Lightweight misroute cue**: when a drag that crossed the slop ends on the `xterm-local` route with at least one line of post-slop travel and `viewportY` unchanged for the whole gesture (nothing visibly moved: the usual symptom of a normal-buffer TUI routed local), the chip is highlighted (heavier border plus a leading "!" in its text, not colour alone) for 5 s and one polite announcement is made: "The drag did not scroll. Scroll mode is Terminal history. Tap the chip to change." At most once per 30 s. It can false-positive at a true buffer edge; that is accepted (the cue is a nudge, not an error). The TUI route has no such signal (the app's position is unreadable).

**Panel content** (plain labels, each with a one-line description; native `<input type="radio">` inputs inside a `<fieldset>` with a `<legend>`, not custom ARIA radios, so keyboard and screen-reader behavior is the platform's):

| Option | Label | One-line description | Value |
|---|---|---|---|
| Auto | "Auto (recommended)" + secondary "now: Page keys" or "now: Terminal history" | "Picks for you based on what is running." | `auto` |
| Terminal history | "Terminal history" | "Dragging moves the terminal's own scrollback, following your finger. Use in a plain shell." | `local` |
| Page keys | "Page keys" | "Dragging sends Page Up/Down to the app, one page per half-screen. Use for Claude Code or tmux history." | `tui` |

Each radio also carries the description as `aria-describedby`, so the plain label is short and the explanation is still announced. Labels avoid jargon ("App pages" / "Screen history" were replaced because "App pages" did not say it sends Page Up/Down).

Below the radios: a switch "Gesture scrolling" (On/Off) with the description "Turn off if you use a screen reader (TalkBack). Dragging no longer scrolls; use the PgUp/PgDn keys." and a fixed note "Long lines wrap; wide output cannot be panned sideways." Each control and the chip: at least 44x44 CSS px (24x24 floor, WCAG 2.5.8), visible focus ring, state in text not color; changes announced via one `aria-live="polite"` region ("Scroll mode: Terminal history"; "Gesture scrolling: off") owned by the panel/picker host; **while the panel or picker is open the chip does not announce**, so a change is never announced twice; closing the panel returns focus to the opener. The panel also carries the permanent help text: the full tmux note ("In tmux, Page keys scroll tmux history. If typing seems ignored, tap PgDn until you reach the bottom. Esc is not offered because it interrupts a running Claude Code turn.") and the wide-output note.

**Scope**: both settings are **per device**, not per session. A user who alternates between a plain shell and Claude Code must flip the radio by hand; known limitation (per-session memory is a follow-up). Changing either setting during an active gesture cancels it like any mode change.

**First-use and tmux hints (no copy-mode detection)**: the client has no signal for tmux copy-mode (it is tmux server state, not visible in the byte stream or in xterm modes), so a "you are in copy-mode" indicator is **not feasible** and is not promised. Instead: the first time a drag scrolls, a **short, dismissible** `role="status"` hint names the effective route and points to the chip; it **stays until the user dismisses it** (a "Got it" button, 44x44, no auto-timeout, which would fail WCAG 2.2.1 for slow readers) or opens the picker/panel. `terminal-scroll-hint-seen` is set **on dismissal**, not on display, so a hint nobody read reappears next time. Page keys: "Dragging sends Page Up/Down to the app. Tap the chip to change." Terminal history: "Dragging scrolls the terminal history. Tap the chip to change." The full tmux exit text lives in the Scrolling panel's permanent help (above), not in the hint. Neither has an Esc/`q` button: Esc interrupts a running Claude Code turn and `q` is typed text there, so a one-tap exit key would do harm. The chip is the quick access to the toggle. This replaces the earlier undocumented stale-copy-mode behavior with: documented risk plus a hint plus the always-visible chip.

| Misroute symptom | Cue the user has | Exit |
|---|---|---|
| Plain tmux shell, drag enters tmux copy-mode and typing is swallowed | Chip reads "Page keys"; the panel's permanent tmux note | PgDn to the bottom (exits copy-mode), or tap chip and choose Terminal history |
| Normal-buffer TUI routed local, drag does nothing | Chip reads "Terminal history" and is highlighted with "!" after the zero-movement drag (lightweight misroute cue) | Tap chip, choose Page keys (picker closes on select) |

**Local-only misroute metrics**: see plan.md Observability ("route-decision", "override-change", "misroute-proxy" entries in `MobileDebugLog`; nothing leaves the device).

Exit path: switching back to `auto` restores default routing; PgUp/PgDn (S4) always work regardless of setting.

## UX acceptance criteria

Each is testable by a human on a physical Android Chrome device unless noted.

Scrolling
1. Dragging a finger down reveals earlier output and dragging up reveals later output, in both local-buffer and TUI (Claude Code) sessions. With default (unverified) routing, a session in the alternate screen or with mouse tracking on routes to the TUI automatically; a plain tmux shell may misroute until the spike corrects the rows, and the S6 `local` override fixes it. Drag down sends PgUp (`\x1b[5~`, earlier output) and drag up sends PgDn, tested explicitly on device (D2e), since the original bug was direction.
2. While the finger is down in the local xterm buffer, content moves 1:1 with the finger with no initial jump at slop crossover (accumulator seeded with travel minus `SLOP_PX`; the fixed offset is accepted). In TUI modes, movement is page-stepped (one page key per half page of cumulative post-slop travel), capped and rate-limited; that is the accepted exception, not 1:1.
3. Drag-scroll reaches the same history the toolbar PgUp/PgDn keys reach in a Claude Code session.
4. A scroll gesture never opens the soft keyboard or focuses the terminal.
5. Releasing a fast flick continues scrolling with decaying momentum; releasing a slow drag does not.
6. Touching the terminal during momentum stops it immediately and does not focus the terminal.
7. In the local xterm buffer, momentum stops at the top/bottom boundary with no bounce and no Chrome pull-to-refresh. In TUI modes there is no boundary detection: momentum stops at the per-fling page cap (5) or decay threshold, and Chrome pull-to-refresh is still suppressed.
8. With `prefers-reduced-motion: reduce`, momentum is disabled; the drag still works.
9. Multi-touch input is ignored by the handler (two-finger system/TalkBack gestures unaffected).
10. A slow drag of less than one line of cumulative movement per frame still scrolls over time in the local buffer (no stall from line quantization). In TUI modes, a slow drag produces a page key each time cumulative post-slop travel reaches half a page; a shorter drag sends nothing (the first-use hint says so; the PgUp/PgDn keys cover finer needs).

Regression
11. Tap (movement under `SLOP_PX`, < 400 ms) focuses the terminal and opens the keyboard; a tap that drifts 8-14 px still taps. A tap while a selection is active clears the selection and does not open the keyboard.
12. Long-press (400 ms, no movement) enters selection; dragging in selection mode extends selection and does not scroll.
13. Double-tap selects a word.
14. PgUp/PgDn toolbar keys send the same bytes as before in a TUI route and are visible and operable with the keyboard closed (their local-route behavior is AC27).

Redraw
15. After keyboard open and after keyboard close, terminal content is visible within the plan's repaint timing budget (plan.md glossary) after the viewport settles, and no later than the 400 ms path it replaces; 0 blank occurrences in N consecutive open/close cycles, N chosen by the baseline rule in requirements.md.
16. After rotation, the terminal refits and repaints with content visible, with no stale scroll momentum.
17. A keyboard toggle or rotation during an active SCROLLING drag, or during a fling, cancels the gesture without a jump, resets the line/page remainder, and the rest of that touch produces no scroll.
18. No user-facing error message appears for any redraw recovery; no reload or keyboard toggle is required.

Accessibility and non-functional
19. Scroll dispatches are at most one per animation frame (verifiable with a unit test and a frame trace).
20. Unit tests cover both drag signs, the remainder accumulator, mode routing, and momentum cancellation on touchstart/resize.

Scroll-mode override (S6)
21. The scroll-mode control is reachable with the keyboard closed (via the mode chip or the "Scrolling" toolbar entry), offers Auto / Terminal history / Page keys with one-line descriptions (also `aria-describedby`), shows the effective mode under Auto and in the chip (kept visible, with ", gestures off", when Gesture scrolling is Off), persists across reload (per device), takes effect on the next frame, is operable by touch and keyboard with targets >= 44x44 CSS px (24x24 floor), uses native radio inputs in a labelled fieldset, and announces changes via a polite live region. Setting it to Page keys routes a drag to page keys; setting it to Terminal history never sends page keys. The chip tap opens a compact picker that closes on select (misroute recovery in two taps); the chip has an accessible name that names the route and the gesture state; a drag that visibly moved nothing on the local route highlights the chip once. The panel never leaves fewer than 5 terminal rows (overlay when needed). The chip appears only under the touch-detection rule in S6.

Navigation and state
22. When the view is away from live output, a fixed-width "Jump to latest" button (44x44 minimum; "Page down to latest" in TUI mode) is visible and does not cover the cursor or last line; tapping it returns toward live output without focusing the terminal. In the local buffer, output arriving while scrolled up holds position and shows a dot on the button; output never moves content under an active finger or momentum. The new-output announcement is made once per scrolled-away episode.
23. Gesture state transitions follow S9: a touchcancel resets state with no momentum; a horizontal-first drag does not scroll; a release under the slop (including 8-14 px of drift) is a tap; a reconnect or snapshot cancels a gesture; streaming output does not change gesture state.
24. A visible "Redraw" button in the always-visible toolbar row forces a refit and repaint in one tap with the keyboard closed.
25. Pinch-zoom and TalkBack: font-size setting reaches 200% (WCAG 1.4.4 alternative to browser pinch-zoom); with Gesture scrolling Off, a TalkBack pass confirms single-finger explore-by-touch on the terminal region and toolbar operability **and, as a must-pass, that the user can focus the terminal and type (open the keyboard and enter text) with Gesture scrolling Off**; the On default is also recorded (D8). D8 also verifies at 200% font size that the toolbar, chip, picker and panel remain usable (no clipped or overlapping controls, targets still >= 24x24, Redraw and chip still visible with the toolbar collapsed). Failure of the focus-and-type check blocks closing the story (Off would otherwise strand a TalkBack user); other failures are recorded as follow-ups, not silently accepted.
26. PgUp/PgDn toolbar targets measure at least 44x44 CSS px (24x24 floor); the chip and panel controls meet the same floor; the PR records the measured sizes.
27. (WCAG 2.5.1 equivalence) With the effective route `xterm-local`, toolbar PgUp scrolls the xterm history up one page and PgDn down one page (the history the drag moves); with a TUI route they send `\x1b[5~`/`\x1b[6~`; with a modifier armed they send the modified bytes. In `auto` a local route that cannot move falls through to the bytes.
28. Gesture scrolling Off: no touch listeners or `preventDefault` over the canvas, default `touch-action` restored, setting persisted and reachable, toolbar keys still scroll, and the panel states the costs. An unavailable or throwing `localStorage` leaves the default (On).
29. The first drag-scroll on a device shows a short hint for the effective route that points to the mode chip and stays until dismissed with a "Got it" button (no auto-timeout; the seen flag is set on dismissal). The tmux exit text (PgDn to the bottom) is permanent help in the Scrolling panel; no hint or panel offers an Esc or `q` key.
30. In TUI mode the jump button is hidden unless the `netPagesUp` estimate is valid (S7), never sends more than 5 PgDn, and is hidden after any keystroke, paste, resize, mode change or reconnect.
31. With fewer than 5 visible terminal rows (landscape with the keyboard open) the chip and jump button are hidden and the toolbar keys still work.
32. A reconnect while scrolled, and an empty buffer, behave as described in S7: no stale gesture, no stale button, no error.

## Flows lacking an exit path or error state

None. Every interactive flow has an exit: ignored forwarded input falls back to the route-aware PgUp/PgDn keys; a misrouted session is fixed with the S6 override via the always-visible chip; gestures can always be restarted; redraw failure self-recovers with a bounded timeout fallback and the always-visible Redraw button; a screen-reader user has Gesture scrolling Off. Residual gaps, not missing exits: if a TUI ignores forwarded wheel/PgUp there is no feedback, only the toolbar keys; tmux copy-mode cannot be detected, so only a one-time hint and the chip cover it (S6); a selection longer than one screen cannot be made by a single drag (selection mode has no edge auto-scroll; extend it screen by screen, scrolling with the route-aware toolbar keys, which keep the selection). **Recorded as a known WCAG 2.5.1 limitation for drag-selection beyond one screen (borderline; the screen-by-screen path is the only alternative), not fixed here; listed in plan.md "Plan risks"**; wide output cannot be panned horizontally (long lines wrap; horizontal drag is ignored; the panel says so). Also unverified: whether Claude Code responds to wheel events or only PgUp/PgDn keys (requirements open question), which `bufferType`/`mouseTrackingMode` Claude Code and a tmux shell report (decides the verified routing rows), whether `touchend` `preventDefault` fully suppresses synthesized focus, and whether TalkBack works with the default On; all need device checks.
