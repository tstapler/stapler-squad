# UX Design: client-reconnect

**Date**: 2026-06-23
**Feature**: Soft reconnect for session-watch and terminal streams
**Status**: Design — ready for implementation

---

## Surfaces Covered

1. [ConnectionIndicator — "Live" state (header)](#surface-1-connectionindicator--live-state)
2. [ConnectionIndicator — "Reconnecting…" state (header)](#surface-2-connectionindicator--reconnecting-state)
3. [ConnectionIndicator — Tooltip (hover/focus on Reconnecting button)](#surface-3-connectionindicator--tooltip)
4. [Terminal pane — reconnecting banner overlay (2 s delay)](#surface-4-terminal-pane--reconnecting-banner)
5. [Terminal pane — hard failure state (5+ attempts)](#surface-5-terminal-pane--hard-failure-state)
6. [Terminal pane — recovery separator line](#surface-6-terminal-pane--recovery-separator)
7. [Screen reader / aria-live announcements](#surface-7-screen-reader-aria-live-region)

---

## Surface 1: ConnectionIndicator — "Live" State

### Layout

```
┌─────────────────────────────────────────────────────────────────┐
│  [Logo]  Session Name         ●  Live          [Other controls]  │
│                               └── green dot, no button affordance│
└─────────────────────────────────────────────────────────────────┘
```

The indicator is a `<button>` in the header, rendered as:

```
┌──────────────┐
│  ● Live      │   ← green dot (8 px), "Live" label
└──────────────┘
   disabled, cursor: default, no hover state
```

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Page loads; stream connects | Button appears with green dot and "Live" label |
| 2 | User hovers or tabs to indicator | No tooltip; no cursor change (disabled state) |
| 3 | User presses Enter/Space | No-op (button is disabled) |

### Error / Edge Cases

- If `connectionState` arrives as `undefined` before first stream event: treat as `reconnecting` to avoid a flash of "Live" before the stream confirms health.
- Do not render the indicator at all during initial server-side render (no `connectionState` available); render only on mount.

---

## Surface 2: ConnectionIndicator — "Reconnecting…" State

### Layout

```
┌─────────────────────────────────────────────────────────────────┐
│  [Logo]  Session Name      ⟳  Reconnecting…   [Other controls]  │
│                            └── amber, CSS spinner, clickable     │
└─────────────────────────────────────────────────────────────────┘
```

The button in the "Reconnecting…" state:

```
┌──────────────────────┐
│  ⟳  Reconnecting…   │   ← CSS spinner (no dot), amber text + border
└──────────────────────┘
   active, cursor: pointer, hover: slightly brighter amber border
```

The spinner is a pure CSS `@keyframes` rotation on a 12 px ring — no image, no emoji, no dependency. It replaces the coloured dot entirely in this state.

### State Mapping

| Redux `connectionState` | Visible state |
|---|---|
| `"connected"` | Surface 1 (Live, disabled) |
| `"stale"` | Surface 2 (Reconnecting…, active) |
| `"disconnected"` | Surface 2 (Reconnecting…, active) |

The `stale` and `disconnected` states collapse to a single visual state. The distinction remains in Redux as an internal signal to the reconnect hook.

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Stream drops (stale or disconnected) | Button transitions from "Live" to "Reconnecting…" + spinner. Automatic backoff reconnect begins in background. |
| 2 | User hovers indicator | Tooltip appears (see Surface 3). |
| 3 | User clicks "Reconnecting…" button | `triggerSoftReconnect()` is called immediately (calls `watchSessions()`, does NOT call `window.location.reload()`). Attempt counter resets to 1. Spinner continues. |
| 4 | Stream reconnects | Button transitions back to Surface 1 ("Live"). `aria-live` region announces "Connection restored." |
| 5 | User ignores button | Automatic backoff continues (1 s → 2 s → 4 s … up to 30 s). No further visual change during retries — spinner keeps spinning. Attempt count increments silently (visible only in tooltip). |

### Error / Edge Cases

- If `triggerSoftReconnect()` is unavailable (context not wired): log a warning and fall back to `window.location.reload()` as a last resort (but do not expose this to the user as the primary action).
- If reconnect succeeds then fails again within 5 s: immediately re-enter "Reconnecting…" state without passing through "Live".
- On very slow connections where a reconnect attempt takes >10 s: the spinner continues; no "Timed out" state is shown (the backoff handles retry scheduling).

---

## Surface 3: ConnectionIndicator — Tooltip

### Layout

The tooltip appears on hover and on keyboard focus of the "Reconnecting…" button:

```
┌──────────────────────┐
│  ⟳  Reconnecting…   │  ← button
└──────────────────────┘
         │
         ▼
 ┌─────────────────────────────────┐
 │ Reconnecting… attempt 3         │  ← primary line (dynamic)
 │ ─────────────────────────────── │
 │ Reload page (resets state)  [→] │  ← secondary escape hatch link
 └─────────────────────────────────┘
```

- Primary line: `"Reconnecting… attempt {n}"` — updates on each new attempt, but the tooltip only re-renders if it is open.
- Secondary line: plain text link or small button that calls `window.location.reload()`. Label is always "Reload page (resets state)" — the "(resets state)" suffix is mandatory so users understand the cost.
- The separator (horizontal rule or equivalent) visually separates the informational primary line from the destructive secondary action.
- Tooltip role: `role="tooltip"` with `id` referenced by `aria-describedby` on the button.

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | User hovers "Reconnecting…" button | Tooltip appears after 300 ms delay |
| 2 | User reads attempt count | No interaction required |
| 3 | User clicks "Reload page (resets state)" | `window.location.reload()` — page reloads, all React state lost |
| 4 | User moves mouse away | Tooltip dismisses after 200 ms |
| 5 | User focuses button via Tab | Tooltip appears immediately (no delay for keyboard users) |
| 6 | User presses Escape | Tooltip dismisses; focus stays on button |

### Error / Edge Cases

- If attempt count is 0 (first attempt has not yet returned an error): show "Reconnecting…" with no attempt count.
- Do not show the tooltip in the "Live" (disabled) state.
- On touch devices (no hover): tooltip content is not shown; the button label alone must communicate state. The "Reload page" escape hatch is therefore not accessible on mobile via tooltip — it is acceptable since mobile users can always manually refresh.

---

## Surface 4: Terminal Pane — Reconnecting Banner

The terminal pane gains a thin overlay banner that appears 2 seconds after the stream disconnects. The 2-second delay prevents a flash for fast reconnects (network hiccup that resolves in under 2 s).

### Layout

```
┌───────────────────────────────────────────────────────────────┐
│  [toolbar] Connected  ●  Copy  Paste  Bottom  Clear  Mouse    │
├───────────────────────────────────────────────────────────────┤
│                                                               │
│  $ git commit -m "fix: session auth"                          │
│  [1] 1 file changed, 3 insertions(+)                          │
│  $                                                            │
│ ╔═══════════════════════════════════════════════════════════╗ │  ← overlay banner,
│ ║  ⟳  Reconnecting terminal…                               ║ │    appears after 2 s
│ ╚═══════════════════════════════════════════════════════════╝ │    amber background
│  $ █                                                          │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

The banner is:
- `position: absolute`, centered horizontally, near the top of the terminal content area (not covering the toolbar above it).
- `z-index` above the terminal canvas.
- Amber/warning background (`--warning-bg`), amber border (`--warning`), dark text.
- Height: approximately 36 px (single line with padding).
- CSS spinner on the left, "Reconnecting terminal…" text, no button in this initial state.
- `pointer-events: none` on the banner itself so users can still scroll and select text in the terminal beneath it.
- The terminal container must have `position: relative` to establish the stacking context.

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Terminal stream drops | No immediate change (banner is hidden). |
| 2 | 2 seconds elapse without reconnect | Banner fades in (200 ms ease-in). Toolbar status text also changes from "Connected" to "Disconnected • Reconnecting (attempt 1/5)…". |
| 3 | User reads terminal history | Terminal buffer is intact; user can scroll, select, and copy freely. Banner does not block this. |
| 4 | Stream reconnects | Banner fades out (200 ms ease-out). Dim `--- reconnected ---` separator appended to terminal buffer (see Surface 6). |
| 5 | User ignores banner | Auto-reconnect backoff continues silently. Banner remains until reconnect succeeds or fails permanently. |

### Error / Edge Cases

- If the terminal was already disconnected on initial mount (no previous connection): do not show the banner; the initial loading overlay (`loadingOverlay`) handles that state.
- If the component unmounts while the 2 s timer is pending: clear the timer to prevent a state update on an unmounted component.
- On mobile: banner width is constrained to the terminal container; text truncates with ellipsis if needed.
- If `isVisible` is false (terminal pane is not the active tab): suppress the banner entirely — showing it on an invisible pane would cause an erroneous `aria-live` announcement for off-screen content.

---

## Surface 5: Terminal Pane — Hard Failure State

After 5 consecutive failed reconnect attempts, the banner upgrades to a hard-failure state.

### Layout

```
┌───────────────────────────────────────────────────────────────┐
│  [toolbar] Disconnected ●  Copy  Paste  ...  🔄 Reconnect     │
├───────────────────────────────────────────────────────────────┤
│                                                               │
│  $ git commit -m "fix: session auth"                          │
│  [1] 1 file changed, 3 insertions(+)                          │
│  $                                                            │
│ ╔═══════════════════════════════════════════════════════════╗ │
│ ║  ⚠  Connection lost                     [ Retry ]        ║ │  ← primary action
│ ╚═══════════════════════════════════════════════════════════╝ │    is Retry, not reload
│  $ █                                                          │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

Changes from Surface 4:
- Spinner replaced by warning icon (⚠ or equivalent SVG).
- Text changes from "Reconnecting terminal…" to "Connection lost".
- A "Retry" button appears inline on the right side of the banner.
- Banner background shifts from amber (`--warning-bg`) to a slightly stronger warning tone (or remains amber; it must not become red/error since the connection is recoverable).
- `pointer-events: auto` on the banner — the Retry button must be clickable.

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | 5th reconnect attempt fails | Banner upgrades from spinner to warning + Retry button. Toolbar status shows "Terminal unavailable". |
| 2 | User clicks "Retry" | `handleManualReconnect()` is called. Attempt counter resets to 0. Banner reverts to Surface 4 state (spinner, "Reconnecting terminal…"). |
| 3 | Retry succeeds | Banner disappears; `--- reconnected ---` separator appended (Surface 6). |
| 4 | Retry fails again | After 5 more attempts, banner returns to hard-failure state. |
| 5 | User does nothing | Banner stays; terminal history remains accessible. No automatic reload. |

### Error / Edge Cases

- The "Retry" button in the banner is the primary retry affordance. The "🔄 Reconnect" button that already exists in the toolbar also triggers `handleManualReconnect()` — both are valid entry points.
- Do not show both the banner's Retry and an interstitial overlay (`unavailableOverlay`) simultaneously. When the banner is shown, the `unavailableOverlay` that currently renders at `connectionAttempts >= 5` must be suppressed if the banner is active.

---

## Surface 6: Terminal Pane — Recovery Separator

After a successful reconnect following a disconnection, a dim separator line is appended to the terminal buffer.

### Visual Appearance

```
  $ git commit -m "fix: session auth"
  [1] 1 file changed, 3 insertions(+)
  $
  ──────── reconnected ────────          ← dim, non-interactive, separator line
  $ █
```

- Text: `\x1b[2m--- reconnected ---\x1b[0m\r\n` — the `\x1b[2m` ANSI code applies "dim" intensity.
- The line is written to the terminal via the stream manager's `write()` method after the reconnect handshake completes.
- The separator is appended only when reconnecting after a drop, not on initial connect or session switch.
- `pointer-events: none` — this is terminal output, not a UI element.

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Stream reconnects successfully | `--- reconnected ---` dim line appears at bottom of current terminal buffer. |
| 2 | User scrolls up | History before disconnect is intact above the separator. |
| 3 | New terminal output arrives | Output appears below the separator as normal. |

### Error / Edge Cases

- If reconnect occurs and the terminal buffer is empty (initial connect): do not append the separator — it would appear as the first visible content and is meaningless.
- If multiple reconnects occur in rapid succession (network flapping): append a separator for each successful reconnect. Duplicate separators are acceptable; they communicate the connection instability.
- The separator should survive terminal clear operations only if the user explicitly cleared; it should not be re-injected after a programmatic clear.

---

## Surface 7: Screen Reader / aria-live Region

A visually-hidden `<div>` with `aria-live="polite" aria-atomic="true"` provides connection state announcements to screen readers. This is separate from the visible button.

### Placement in DOM

```tsx
{/* Persistent live region — always in DOM, text changes trigger announcements */}
<div
  aria-live="polite"
  aria-atomic="true"
  className={visuallyHidden}   // position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0,0,0,0)
>
  {liveRegionText}
</div>

{/* Visible button — no aria-live */}
<button aria-label={buttonLabel} aria-describedby="connection-tooltip" ...>
  ...
</button>
```

### Announcement Text and Timing

| Transition | Announced text | Timing |
|---|---|---|
| `connected` → `stale` or `disconnected` | "Reconnecting…" | Immediately on state change |
| `stale`/`disconnected` → `connected` | "Connection restored" | Immediately on state change |
| Any state → same state (retry attempt) | _(nothing)_ | Suppressed — do not announce on each retry |
| Hard failure (attempts exhausted) | "Connection lost" | Immediately when retry limit reached |
| Hard failure → `connected` after manual retry | "Connection restored" | Immediately on reconnect |

### Interaction Flow

| Step | User action | System response |
|---|---|---|
| 1 | Screen reader user is navigating the app | Live region is in DOM but empty; no announcement. |
| 2 | Stream drops | Live region text changes to "Reconnecting…"; screen reader announces it when user is not mid-sentence. |
| 3 | User navigates to "Reconnecting…" button | `aria-label` reads full label: "Reconnecting — click to retry connection". `aria-describedby` references tooltip for attempt count when tooltip is open. |
| 4 | Stream reconnects | Live region text changes to "Connection restored"; announced once. |

### Error / Edge Cases

- The live region `<div>` must be present in the DOM on initial render, before any state changes. A newly-inserted element with `aria-live` will not trigger announcements for changes that happen immediately after insertion.
- Do not use `aria-live="assertive"` — this would interrupt the user mid-sentence. "polite" queues the announcement.
- The terminal banner (Surface 4/5) does not need its own `aria-live` region since the ConnectionIndicator already announces the global reconnect state. Adding a second announcement would be redundant.

---

## Complete Interaction Flows

### Flow A: Fast Reconnect (under 2 s)

```
Stream drops → [< 2 s] → Stream reconnects
     │                          │
     │   No UI change           │   No banner shown
     │   during this period     │   "--- reconnected ---" appended
     │                          │   "Connection restored" announced
     ▼                          ▼
  (invisible to user except for separator in terminal)
```

### Flow B: Slow Reconnect (2–30 s)

```
Stream drops
     │
     ├── t=0: ConnectionIndicator: "Live" → "Reconnecting…" + spinner
     │         aria-live: announces "Reconnecting…"
     │
     ├── t=2s: Terminal banner fades in: "Reconnecting terminal…" + spinner
     │
     ├── t=4s: Attempt 2, backoff. Tooltip shows "attempt 2" (if open).
     │
     ├── t=8s: Attempt 3...
     │
     ├── tNs: Stream reconnects
     │         ConnectionIndicator: "Reconnecting…" → "Live"
     │         aria-live: announces "Connection restored"
     │         Terminal banner fades out
     │         Terminal: "--- reconnected ---" appended
     ▼
  (user sees session list and terminal updating again)
```

### Flow C: Permanent Failure (5 failed attempts)

```
Stream drops
     │
     ├── [Reconnecting, attempts 1-4] (see Flow B)
     │
     ├── Attempt 5 fails:
     │     Terminal banner upgrades: spinner → warning icon + "Connection lost" + [Retry]
     │     Toolbar status: "Terminal unavailable"
     │     aria-live: announces "Connection lost"
     │     ConnectionIndicator: still shows "Reconnecting…" (session watch may still retry)
     │
     ├── User clicks [Retry]:
     │     Attempt counter resets
     │     Terminal banner reverts to spinner state
     │     Reconnect begins
     │
     ├── User opens tooltip on ConnectionIndicator:
     │     Sees "Reconnecting… attempt 6" (or current count)
     │     Sees "Reload page (resets state)" escape hatch
     │
     └── User clicks "Reload page (resets state)":
           window.location.reload() — all state lost, full page reload
```

### Flow D: User Initiates Manual Reconnect from ConnectionIndicator

```
User sees "Reconnecting…" button
     │
     ├── User clicks button
     │     triggerSoftReconnect() called
     │     watchSessions() re-invoked
     │     Attempt counter resets to 1
     │     Spinner continues (no visual change; button stays in same state)
     │
     ├── Reconnect succeeds:
     │     Button → "Live"
     │     aria-live: "Connection restored"
     │
     └── Reconnect fails:
           Stays in "Reconnecting…" state
           Backoff continues
```

---

## UX Acceptance Criteria

### AC-1: ConnectionIndicator — Live State

**Given** the session-watch stream is connected  
**When** a human looks at the header  
**Then** the ConnectionIndicator shows a green dot and the label "Live"  
AND the button appears disabled (no pointer cursor, no hover effect)  
AND clicking or pressing Enter/Space on the indicator does nothing

### AC-2: ConnectionIndicator — Reconnecting Label

**Given** the session-watch stream has dropped (stale or disconnected state)  
**When** a human looks at the header  
**Then** the ConnectionIndicator shows an animated spinner (not a dot) and the label "Reconnecting…"  
AND the button appears active (pointer cursor)  
AND the colour is amber (not red, not green)

### AC-3: No Page Reload on Click

**Given** the ConnectionIndicator is in the "Reconnecting…" state  
**When** a human clicks the "Reconnecting…" button  
**Then** the page does NOT reload (React state, selected session, filters are preserved)  
AND a soft reconnect attempt begins  
AND the spinner continues spinning

### AC-4: Stale Collapses to Reconnecting

**Given** the Redux store transitions to `connectionState: "stale"`  
**When** a human reads the ConnectionIndicator  
**Then** it shows "Reconnecting…" — not "Stale" and not "Offline"

### AC-5: Tooltip Shows Attempt Count

**Given** the ConnectionIndicator is in the "Reconnecting…" state and has made at least one attempt  
**When** a human hovers over or focuses the button  
**Then** a tooltip appears showing "Reconnecting… attempt N" where N is the current attempt number  
AND the tooltip contains a secondary item labelled "Reload page (resets state)"

### AC-6: Hard Reload Only via Tooltip Escape Hatch

**Given** the app is in any reconnect state  
**When** a human checks all interactive elements in the app  
**Then** no button or link triggers `window.location.reload()` automatically  
AND the only reload trigger is the "Reload page (resets state)" item inside the tooltip  
AND that item is never activated without an explicit human click

### AC-7: aria-live Announcement on State Change

**Given** a screen reader is active  
**When** the session-watch stream drops  
**Then** the screen reader announces "Reconnecting…" exactly once (not repeated on each retry attempt)  
AND when the stream reconnects, the screen reader announces "Connection restored" exactly once

### AC-8: aria-live Not on Button

**Given** any connection state  
**When** a developer inspects the DOM  
**Then** the `<button>` element for ConnectionIndicator does NOT have an `aria-live` attribute  
AND there is a separate visually-hidden `<div>` with `aria-live="polite" aria-atomic="true"`

### AC-9: Terminal Banner Delayed 2 Seconds

**Given** the terminal stream drops  
**When** less than 2 seconds have elapsed since the drop  
**Then** no reconnecting banner is visible over the terminal content area

### AC-10: Terminal Banner Appears After 2 Seconds

**Given** the terminal stream has been disconnected for more than 2 seconds  
**When** a human looks at the terminal pane  
**Then** a banner reading "Reconnecting terminal…" with a spinner is visible overlaid on the terminal content area  
AND the terminal's existing output remains visible and scrollable beneath the banner  
AND the human can still select text in the terminal

### AC-11: Terminal Buffer Preserved on Reconnect

**Given** the terminal stream has dropped and then reconnected  
**When** a human scrolls up in the terminal  
**Then** all output that was visible before the disconnect is still present in the scroll buffer  
AND no output has been cleared or lost

### AC-12: Reconnected Separator Appended

**Given** the terminal stream reconnects after a previous disconnection  
**When** a human looks at the terminal  
**Then** a dim line reading `--- reconnected ---` appears at the point in the output where the stream resumed  
AND the line is visually dimmer than normal terminal output (dim ANSI style or equivalent)  
AND the line is not interactable (cannot be clicked)

### AC-13: Terminal Hard Failure State After 5 Attempts

**Given** the terminal stream has failed to reconnect after 5 consecutive attempts  
**When** a human looks at the terminal pane  
**Then** the banner changes from a spinner to a warning icon and the text "Connection lost"  
AND a "Retry" button appears inside the banner  
AND clicking "Retry" begins a new reconnect attempt without reloading the page

### AC-14: Terminal Banner Not Shown on Initial Load

**Given** the terminal component has just mounted for the first time (no prior connection)  
**When** a human looks at the terminal pane  
**Then** no reconnecting banner is visible  
AND the existing loading overlay (`loadingOverlay` / "Loading terminal content…") is shown instead

### AC-15: Terminal Banner Not Shown on Hidden Pane

**Given** the terminal pane is not the currently active tab (isVisible is false)  
**When** a reconnect event occurs  
**Then** no reconnecting banner appears or is announced on the hidden pane

### AC-16: ConnectionIndicator Focus Preserved After Click

**Given** a keyboard user has focused the "Reconnecting…" button and pressed Enter to trigger reconnect  
**When** the reconnect action completes (success or failure)  
**Then** focus remains on the ConnectionIndicator button  
AND focus has not jumped to the terminal or session list

### AC-17: Reconnect Separator Not Shown on First Connect

**Given** the terminal component has just mounted and the stream connects for the first time  
**When** a human looks at the terminal  
**Then** no `--- reconnected ---` separator appears  
AND only the normal terminal output or scrollback is visible

### AC-18: Tooltip Accessible to Keyboard Users

**Given** a keyboard user has tabbed to the "Reconnecting…" button  
**When** the button has focus  
**Then** the tooltip content (attempt count and reload escape hatch) is accessible via `aria-describedby` or revealed without a mouse hover  
AND the "Reload page (resets state)" link is reachable via Tab without closing the tooltip

---

## Component Ownership Summary

| Surface | Component | File |
|---|---|---|
| ConnectionIndicator states 1–3 | `ConnectionIndicator` | `web-app/src/components/layout/ConnectionIndicator.tsx` |
| ConnectionIndicator tooltip | New `ConnectionTooltip` or inline | `ConnectionIndicator.tsx` or new colocated file |
| aria-live region | `ConnectionIndicator` | `ConnectionIndicator.tsx` |
| Terminal banner (surfaces 4–5) | `TerminalOutput` | `web-app/src/components/sessions/TerminalOutput.tsx` |
| Terminal separator (surface 6) | `TerminalStreamManager` or `TerminalOutput` | `web-app/src/lib/terminal/TerminalStreamManager.ts` |

The `BrowserTab` reconnecting banner (`BrowserTab.css.ts` + inline JSX in `BrowserTab.tsx`) is the reference implementation for the terminal banner pattern. Adopt its CSS class names and absolute-positioning approach rather than inventing a new pattern.
