# Architecture Research: Terminal Resize/Fit Feedback Loop Convergence

## 0. Building on prior work: why the R1.2/R1.3 debounce fix is insufficient

`project_plans/terminal-robustness/research/architecture.md:96` documents the fix already
shipped and present in the current tree:

> "Frontend debounce fix (R1.2/R1.3): The `ResizeObserver` callback in `XtermTerminal.tsx`
> (lines ~311-332) uses an adaptive debounce: 10ms for first 3 resizes, then 250ms. Replace
> with a flat 150ms debounce. Remove the secondary `setTimeout(..., 100)` fit ... the double-RAF
> inside the ResizeObserver callback is already sufficient."

That fix is confirmed live in `web-app/src/components/sessions/XtermTerminal.tsx:472-502`: a flat
`debounceDelay = 150` (line 476), a double-`requestAnimationFrame` before `fitAddonRef.current?.fit()`
(lines 488-489), and a post-fit re-sync of `lastContainerSize` to the DOM's post-fit
`getBoundingClientRect()` (lines 494-497) specifically to "break the scrollbar-appearance
oscillation loop" (comment at line 493).

**Why it's still not enough:** R1.2/R1.3 debounces by *time* (150ms) and gates re-entry by *raw
pixel delta* (`XtermTerminal.tsx:463-464`, `Math.abs(width - lastContainerSize.width) > 1`). Both
of those are proxies for "did the terminal's integer grid actually need to change" — not the
thing itself. A WebGL sub-cell glyph-metric mismatch (the `Actual pixels per column: 8.45px` vs
`Expected pixels per column: 8.33px` case logged at `XtermTerminal.tsx:387-393`) can produce a
container pixel delta > 1px on every settle-and-refit cycle without ever changing
`FitAddon.proposeDimensions()`'s integer cols/rows. R1.2/R1.3's own mitigation (line 494-497,
re-syncing `lastContainerSize` post-fit) only prevents *fit() itself* from re-triggering the
observer — it does nothing about layout perturbations that arise from other causes (scrollbar
appearing/disappearing, WebGL canvas re-render, parent flex layout reflow) that are pixel-different
but grid-identical. Each such fit() still calls `terminal.onResize` if xterm.js's own
`resize(cols, rows)` no-ops when unchanged (it does — xterm.js's own `Terminal.resize()` is a
no-op if cols/rows match), so in principle a grid-stable refit is harmless *downstream* of
`fitAddon.fit()` — but it is not harmless *upstream*: every 150ms-debounced fit() cycle still (a)
burns a `requestAnimationFrame` double-tick and DOM measurement, (b) can retrigger the
ResizeObserver via any layout side effect `fit()` itself causes beyond container resize (e.g.
xterm's internal `<canvas>` resize triggering a scrollbar), and ties up the debounce timer so a
*genuine* resize is delayed. R1.2/R1.3 raised the bar for entering the loop; it did not add a
convergence *test* — nothing today asks "would fitting actually change the applied grid?" before
committing to the 150ms-debounce-then-fit cycle. That is the gap AC2 (integer cols/rows gate) closes.

Symmetrically, on the `useTerminalFlowControl.ts` side, the existing mitigation is the 200ms
`THROTTLE_MS` time-throttle in `resize()` (`useTerminalFlowControl.ts:409-416`). That throttle
only rejects calls that arrive *within* 200ms of the last one — it does nothing once 200ms has
elapsed, so a value that recurs every 150-200ms (which is exactly the cadence R1.2/R1.3's own
debounce produces) will still resend the RPC and the follow-up `currentPaneRequest` every time
the throttle window reopens. That is the gap AC3 (value dedup, independent of and in addition to
the time throttle) closes.

## 1. Current control flow (traced, with file:line)

### 1a. ResizeObserver → fit() (`XtermTerminal.tsx`)

- `resizeObserver` created and attached at component-mount effect, `XtermTerminal.tsx:453-508`.
- Gate: `widthChanged || heightChanged` from raw pixel delta > 1px (`:463-464`), plus
  `width > 0 && height > 0` (`:466`) to skip collapsed containers.
- On pass: clears any pending `resizeTimeout`, schedules a new `setTimeout(150ms)` → double-rAF →
  `fitAddonRef.current?.fit()` (`:479-502`).
- `fit()` internally computes `FitAddon.proposeDimensions()` and, if different from current
  `terminal.cols/rows`, calls `terminal.resize(cols, rows)`, which fires `terminal.onResize` only
  if the values actually changed (xterm.js's own no-op guard).
- **No cols/rows check happens before scheduling the debounce+fit cycle** — only the raw pixel
  gate at `:463-464`. This is the AC2 gap: `proposeDimensions()` is never consulted before
  committing to the debounce timer.

### 1b. terminal.onResize → onResizeRef (`XtermTerminal.tsx:433-440`)

- Already has an integer-value dedup: `lastSizeRef` compares incoming `{cols, rows}` against the
  last-seen value and only calls `onResizeRef.current?.(cols, rows)` if different (`:435-439`).
  This is a **different, already-correct gate** — it protects the callback-to-parent boundary, not
  the ResizeObserver-to-fit() boundary. It does not eliminate wasted `fit()` calls (that
  `fit()`/rAF/DOM-measurement work already happened before `onResize` even fires, or fires-and-noops
  if xterm didn't change size), so AC1 ("zero fit() calls") requires gating fit() itself, not just
  gating the callback after the fact.

### 1c. onResize callback chain into `TerminalOutput.tsx`

`XtermTerminal`'s `onResize` prop is wired to `handleTerminalResize` in
`TerminalOutput.tsx:541-641` (verify prop name via the JSX usage; logic traced below regardless):

- `handleTerminalResize(cols, rows)` (`:541-641`) computes its own
  `sizeChanged = !lastResize || lastResize.cols !== cols || lastResize.rows !== rows`
  (`:544-545`) against `lastResizeRef` (component-level ref, persists across reconnects).
- If `sizeChanged`, updates `lastResizeRef`, persists cell-pixel-dimension cache (`:550-572`),
  and (on first connection) drives the "size stability" connect-gating state machine
  (`:582-627`).
- After that large conditional block, **regardless of whether it was entered**, execution falls
  through to (`:629-640`):
  ```
  if (!isConnected) { ...; return; }
  if (!sizeChanged) { ...; return; }
  resize(cols, rows);   // <-- the useTerminalFlowControl.resize() call
  ```
  So call site 1 (`TerminalOutput.tsx:640`) is **already gated by a caller-side value-dedup**
  (`sizeChanged`, backed by `lastResizeRef`). This means two of the three planned dedup layers
  (xterm's own `lastSizeRef` at `XtermTerminal.tsx:435`, and `TerminalOutput`'s `sizeChanged` at
  `:544-545`) already partially cover the "same value called twice" case for the *normal* resize
  path. **What's missing is a dedup inside `useTerminalFlowControl.resize()` itself**, because (a)
  AC3 explicitly requires it there, independent of caller behavior, and (b) — critically — it is
  NOT actually redundant, because of the other two call sites below, which intentionally bypass
  `TerminalOutput`'s own `sizeChanged` gate.

### 1d. All callers of `useTerminalFlowControl.resize()` — traced

Found by grepping `resize(` across `web-app/src` excluding xterm's own `terminal.resize()` calls
(those are unrelated: `DeltaApplicator.ts:126,171` and `StateApplicator.ts:562` call
`this.terminal.resize()`, xterm.js's own method, not the hook's). Exactly three callers of the
hook's `resize`, all in `TerminalOutput.tsx`:

1. **`TerminalOutput.tsx:640`** — normal resize path via `handleTerminalResize`, described above.
   Already gated by `sizeChanged` at the call site.
2. **`TerminalOutput.tsx:664`** — inside the `isConnected` transition effect (`:644-683`), on
   `!wasConnected && isConnected` (fresh connect or reconnect): unconditionally calls
   `resize(currentSize.cols, currentSize.rows)` using the *cached* `lastResizeRef.current` value —
   **with no `sizeChanged` check**, because the point is to (re-)sync the server to a value the
   client already has, which by definition equals the last value in `lastResizeRef`. This call is
   deliberately NOT deduped by `TerminalOutput`.
3. **`TerminalOutput.tsx:1160`** — inside `handleManualResize` (`:1146-1164`), a manual/debug
   "force resize" action (calls `xtermRef.current.fit()` then unconditionally re-sends `resize()`
   with whatever `terminal.cols/rows` currently are, even if unchanged) — also deliberately NOT
   deduped.

**Implication for the AC3 value-dedup design:** if `useTerminalFlowControl.resize()` adds a naive
"skip if `(cols,rows) === lastSent`" check with no escape hatch, call sites 2 and 3 above break:
- Call site 2 (post-reconnect resync) is the mechanism that tells a *freshly connected* server
  what size the client is already at. If the server-side session was resized while disconnected,
  or if this is a brand new WebSocket to an existing session at a stale server-side size, this
  RPC must go through even though it matches the client's own last-sent value from the *previous*
  connection.
- Call site 3 (manual force-resize debug action) exists specifically to bypass all the normal
  gating for debugging/recovery — it must always send.

  So the `lastSentSizeRef` used for AC3 dedup must be **reset on reconnect** (cleared in the same
  effect that fires call site 2, e.g. in the `!wasConnected && isConnected` branch, before or via
  the resize call itself) and the `resize()` function should accept an optional `force?: boolean`
  parameter (or equivalent) that call sites 2 and 3 pass, bypassing only the value-dedup (not the
  200ms time-throttle, which is separate and orthogonal — AC3 says "both apply" for the normal
  path, but a forced resize is explicitly the caller overriding intent). This is a **required
  design finding**, not an optional embellishment: without it, requirement 6 ("real cols/rows
  change still triggers exactly one fit() + one resize RPC — no regression") is satisfied, but the
  reconnect-resync and manual-force paths silently stop working, which is a functional regression
  not covered by the stated acceptance criteria but discoverable only by tracing callers as this
  research task required.

### 1e. `handleStateMessage`'s dimension-mismatch resync — does NOT call `resize()`

Traced `setupDimensionMismatchHandler` (`useTerminalFlowControl.ts:154-170`) and
`handleStateMessage` (`:208-246`): a dimension mismatch between server-reported state and the
client's actual terminal size triggers `requestFullResync(true)` (`:165`), which only sends a
`currentPaneRequest` (`:130-146`) — **it never calls `resize()`**. So this path is irrelevant to
the resize-RPC convergence problem; it's a content-resync path, not a size-RPC path. Confirmed no
resize-loop interaction here. `requestFullResync` has its own independent 2000ms throttle
(`:107-113`) unrelated to `resize()`'s 200ms throttle.

**Conclusion on 1d/1e:** `useTerminalFlowControl.resize()` has exactly 3 callers, all in
`TerminalOutput.tsx`; 1 is already value-deduped by the caller, 2 deliberately are not and must
stay that way via an explicit force/bypass mechanism paired with resetting `lastSentSizeRef` on
reconnect.

## 2. WebGL addon lifecycle (`XtermTerminal.tsx:263-281`)

- WebGL addon is loaded in a fire-and-forget async IIFE (`:264-281`), **not stored in any ref** —
  `webglAddon` is a local `const` inside the IIFE closure, referenced only by its own
  `onContextLoss` handler (`:269-272`) which disposes itself.
- Nothing else in the component can currently reach the WebGL addon instance. For AC4 (oscillation
  detector disposes WebGL addon and falls back to canvas), a `webglAddonRef` (parallel to
  `fitAddonRef`, `searchAddonRef`, `serializeAddonRef` at `:445-447`) must be added, set inside the
  IIFE after `terminal.loadAddon(webglAddon)` succeeds, and cleared to `null` in the effect
  cleanup (`:510-525`) alongside the other addon refs.
- `console.error` backstop (AC4's "when no WebGL addon exists to dispose"): the disposal call site
  must handle `webglAddonRef.current === null` (WebGL never loaded — mobile/Android per `:278-280`,
  or load failed per `:275-277`, or context already lost and self-disposed per `:271`) by logging
  via `console.error` instead of throwing, since canvas is presumably already active in those
  cases and "falling back" is a no-op that should still be visible/debuggable.

## 3. Design: where the pure decision functions live

Established pattern in `web-app/src/lib/terminal/`: small, single-purpose pure-function files
imported directly into `XtermTerminal.tsx` — e.g. `getCellDimensions`
(`web-app/src/lib/terminal/cellDimensions.ts`, imported at `XtermTerminal.tsx:15`) and
`isMouseTracking` (`web-app/src/lib/terminal/mouseTracking.ts`, imported at `:16`). Both are pure
functions taking a `Terminal` (or primitives) and returning a value, with no React/DOM
dependencies beyond what's passed in — exactly the shape needed here. Test convention is mixed:
some pure helpers have a colocated `<name>.test.ts` (e.g. `CircularBuffer.test.ts` sits next to
`CircularBuffer.ts`), others live under `__tests__/` (e.g. `mouseTracking.test.ts` in
`web-app/src/lib/terminal/__tests__/`). Follow whichever convention is already used for direct
neighbors — since `cellDimensions.ts` and `mouseTracking.ts` are the closest analogues and
`mouseTracking.test.ts` uses `__tests__/`, prefer `__tests__/resizeConvergence.test.ts` for
consistency with the most recently added sibling.

**Recommendation: new file `web-app/src/lib/terminal/resizeConvergence.ts`** containing three pure
functions, all synchronous, all free of refs/DOM/xterm-instance state (state is passed in, not
captured):

```ts
// (a) AC2 — gates fit() scheduling in the ResizeObserver handler
export function shouldFit(
  proposedCols: number | undefined,
  proposedRows: number | undefined,
  currentCols: number,
  currentRows: number,
): boolean {
  if (proposedCols === undefined || proposedRows === undefined) return false; // proposeDimensions() can return undefined pre-layout
  return proposedCols !== currentCols || proposedRows !== currentRows;
}

// (b) AC3 — value-dedup gate for useTerminalFlowControl.resize()
export function shouldSendResize(
  cols: number,
  rows: number,
  lastSent: { cols: number; rows: number } | null,
): boolean {
  return lastSent === null || lastSent.cols !== cols || lastSent.rows !== rows;
}

// (c) AC4 — oscillation/refit-burst detector
export interface ResizeEvent { cols: number; rows: number; at: number }
export function shouldFallbackToCanvas(
  history: ResizeEvent[],
  now: number,
  windowMs = 2000,
  threshold = 3,
): boolean {
  // count entries within the rolling window that match the most recent (cols,rows) pair
  const recent = history.filter((e) => now - e.at <= windowMs);
  if (recent.length === 0) return false;
  const last = recent[recent.length - 1];
  const matches = recent.filter((e) => e.cols === last.cols && e.rows === last.rows);
  return matches.length >= threshold;
}
```

Why one file, not three: they're small (a handful of lines each), share the same domain
(resize-convergence decisions), and will be imported together into `XtermTerminal.tsx` — matching
the existing grain of `cellDimensions.ts`/`mouseTracking.ts` (single-concept files) rather than
`TerminalDimensionCache.ts` (which is a class with more surface area — resize convergence's pure
functions don't need class encapsulation, so a class would be over-structuring). `shouldSendResize`
technically belongs conceptually to `useTerminalFlowControl.ts`'s domain rather than
`XtermTerminal`'s, but colocating all three in `resizeConvergence.ts` keeps the "resize
convergence" concept in one place for AC5's unit tests and avoids splitting one logical feature
across two lib directories; `useTerminalFlowControl.ts` already imports pure helpers from
`@/lib/terminal/` (e.g. `StateApplicator`, `EchoOverlay` at `:6-7`), so importing
`shouldSendResize` from `@/lib/terminal/resizeConvergence` is consistent with that hook's existing
import style.

## 4. Data flow: where state lives

- **Oscillation history (b→c input)**: must be a `useRef<ResizeEvent[]>([])` **inside
  `XtermTerminal.tsx`**, not module state — it's genuinely per-terminal-instance (multi-terminal
  usage is explicitly the trigger scenario in the problem statement), and refs are the established
  pattern for this component's mutable, non-rendering state (`lastSizeRef`, `fitAddonRef`, etc. are
  all instance-scoped refs declared alongside the terminal instance). Push a `{cols, rows, at:
  Date.now()}` entry each time `fitAddon.fit()` actually changes `terminal.cols/rows` (i.e., each
  time the AC2 `shouldFit` gate passes and a real resize happens) — prune entries older than the
  rolling window on each push to bound memory.
- **`lastSentSizeRef` (AC3 input)**: lives **inside `useTerminalFlowControl`**, alongside the
  existing `lastResizeTimeRef` (`useTerminalFlowControl.ts:70`) — same lifecycle, same hook
  instance, same reset-on-reconnect concern (see §1d). Do not lift it into `TerminalOutput.tsx`;
  the hook already owns the RPC-send boundary and `TerminalOutput` should not need to know about
  "last sent to server" as opposed to its own "last resize event observed" (`lastResizeRef`) —
  those are different concepts (client-observed vs. server-informed) that happen to often be equal.
- **WebGL fallback execution**: needs `webglAddonRef` (see §2) in `XtermTerminal.tsx`. The
  oscillation check happens in the ResizeObserver's post-fit callback (after
  `fitAddonRef.current?.fit()` at `:490`, inside the same double-rAF): compute `shouldFit` first
  (skip fit entirely if false — AC2), and if a fit *did* happen, push to history and call
  `shouldFallbackToCanvas(history, now)`; if true, call `webglAddonRef.current?.dispose()` (or
  `console.error(...)` per the AC4 backstop if the ref is null), set a
  `webglFallbackActiveRef.current = true` **session-scoped flag**, and — critically — do NOT set
  `webglAddonRef.current = null` silently without also guarding the mount-time WebGL-load IIFE:
  since the IIFE only runs once at mount (inside the same effect that sets up the ResizeObserver),
  there's no "reload WebGL" call to prevent mid-session — the fallback is inherently one-shot
  per-mount already, because the WebGL-load code path never re-executes without a full remount.
  So "session-scoped, not persisted" is actually the *default* behavior for free, given the current
  effect structure — no additional guard is needed to keep it non-global, but this must be
  explicitly stated as a design decision (and captured in the ADR) since it would be easy to
  "improve" this later by, e.g., moving the WebGL load into a separate effect keyed on a dep that
  re-runs, or by remembering the fallback in `localStorage`/module state, both of which would
  contradict AC4's implicit scope and should be called out as explicitly NOT done, with rationale
  (WebGL sub-pixel glyph metrics are font/zoom/monitor-DPI dependent per session, not a stable
  global property of the browser — see Out of Scope: "Root-causing exact WebGL sub-pixel glyph
  metric math upstream").

## 5. Integration point summary (answers to research questions)

| Question | Answer |
|---|---|
| Does `onResize`'s `lastSizeRef` dedup (`XtermTerminal.tsx:435-439`) make `resize()` calls already deduped? | Partially, and only for call site 1 of 3. `TerminalOutput.tsx`'s `handleTerminalResize` adds a second caller-side dedup (`sizeChanged`, `:544-545`) on top, which fully covers the *normal* resize path. It does NOT cover the other 2 callers (reconnect-resync at `:664`, manual force at `:1160`), which is exactly why AC3 requires a dedup *inside* `resize()` — but that internal dedup must be bypassable, or those 2 legitimate callers break. |
| Can `resize()` still be called with unchanged values via other paths? | Yes — reconnect-resync (`TerminalOutput.tsx:664`) and manual force-resize (`:1160`), both deliberately, both need a `force`/bypass escape hatch from any new value-dedup, and the dedup ref needs to reset on reconnect. |
| Does `handleStateMessage`'s dimension-mismatch resync call `resize()`? | No — it calls `requestFullResync(true)` → `currentPaneRequest` only (`useTerminalFlowControl.ts:154-170`, `:94-150`). Not part of this bug's loop. |
| Where should `shouldFit`, `shouldSendResize`, `shouldFallbackToCanvas` live? | One new file: `web-app/src/lib/terminal/resizeConvergence.ts`, following the `cellDimensions.ts`/`mouseTracking.ts` pattern (pure fn, no DOM/ref capture), tests in `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts`. |
| Where does oscillation history live? | `useRef` inside `XtermTerminal.tsx` (per-instance, matches multi-terminal scenario). |
| Where does `lastSentSizeRef` live? | Inside `useTerminalFlowControl.ts`, alongside `lastResizeTimeRef` (`:70`); reset on reconnect. |
| How does WebGL→canvas fallback execute? | New `webglAddonRef` in `XtermTerminal.tsx` (parallel to `fitAddonRef` et al.), set in the mount-time WebGL-load IIFE (`:264-281`), disposed from the ResizeObserver's oscillation-check branch; `console.error` backstop if ref is null when disposal is attempted. |
| Is the fallback session-scoped or persisted? | Session-scoped by construction — WebGL only loads once per mount, in the same effect; no re-load path exists to guard against, so nothing needs to be written to `localStorage`/module state, and the ADR should explicitly record that this is a deliberate choice, not an oversight, given the Out-of-Scope note on not root-causing the sub-pixel glyph math (a per-session mitigation, not a permanent global disablement, is the correct level of commitment). |

## 6. Required non-regression checks for the fix (maps to AC5/AC6)

- `shouldFit` must return `true` on genuine cols/rows changes with the same signature xterm's
  `FitAddon.proposeDimensions()` produces (including its `undefined` case pre-layout) — unit test
  with concrete before/after pairs.
- `shouldSendResize`/the `lastSentSizeRef` reset-on-reconnect logic must be exercised by a test
  that (1) sends a resize, (2) simulates reconnect, (3) sends the *same* cols/rows again, and
  asserts the RPC fires (regression guard for the force/bypass design in §1d — without this test,
  a future contributor could "simplify" by removing the reset and silently break reconnect resync).
- New `XtermTerminal` component test (AC5) should live alongside the existing
  `web-app/src/components/sessions/__tests__/XtermTerminal.test.tsx` /
  `XtermTerminalBug.test.tsx` split — likely a new `XtermTerminalResize.test.tsx` given the other
  two are already topic-scoped (`Bug`, `Selection`) — simulating a `ResizeObserver` firing
  repeatedly with sub-cell pixel deltas that don't change `proposeDimensions()`'s integer output,
  asserting `fit()`/resize-callback invoked at most once, per AC5's literal wording.

## 7. ADR

AC4 requires an ADR documenting the WebGL→canvas fallback decision. Next available number in
`docs/adr/` is **018** (existing range 003–017, no gaps below 018 as of this research). Suggested
filename: `docs/adr/018-webgl-oscillation-fallback-to-canvas.md`. Content should cover: the
sub-cell glyph metric mismatch problem (§0), why a per-session runtime fallback (not a permanent
disablement, not a localStorage-persisted flag) is the chosen scope, the `shouldFallbackToCanvas`
threshold/window rationale (>=3 recurrences within 2000ms), and the explicit non-goal of
root-causing the WebGL metric math (already stated as Out of Scope in requirements.md).
