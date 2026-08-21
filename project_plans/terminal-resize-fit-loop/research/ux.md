# UX Research: Terminal Resize/Fit Loop Fix

Agent 5 (UX), Phase 2 research. Scope check confirmed up front: this is a bug fix
with a narrow user-facing surface. No new UI, no new user flow. Findings below
are deliberately short — padding this would misrepresent how much UX surface
actually exists.

## Bottom line

There is no meaningful user-facing UX surface here beyond "the terminal
resizes smoothly, the UI never freezes/pegs CPU, and the Fit button still
works." The four sub-questions below confirm that, with two small,
low-risk observations (not requirements) worth carrying into the plan.

## 1. Is there a user-visible "resizing" / renderer-fallback signal today?

Checked both files directly:

- **`XtermTerminal.tsx`** (`web-app/src/components/sessions/XtermTerminal.tsx`):
  WebGL vs canvas renderer selection happens at terminal-creation time
  (lines ~148-155) via a try/catch around `WebglAddon` load. The only
  feedback is `console.log`/`console.warn` — there is no DOM/visual
  indicator of which renderer is active, and none is exposed to the
  component's props/state for a parent to show one. There is also no
  "mid-fit" visual state — `fitAddon.fit()` runs synchronously inside the
  `ResizeObserver` callback (debounced 10ms/250ms, lines 257-293) with no
  loading/spinner wrapper around it.
- **`TerminalOutput.tsx`**: there IS an existing loading overlay
  (`styles.loadingOverlay` / `styles.loadingSpinner`, ~line 628) and a
  `statusIndicator` with a `stabilizing` CSS state driven by
  `isWaitingForStableSize` (~line 527), text "Initializing..." — but these
  are for **initial connection / first-paint size stabilization**, not for
  steady-state resize-refit or renderer fallback. There's no code path that
  would show either overlay during an ordinary window-resize or tab
  background/resume event.

**Recommendation**: Do not add a new visual state for either concern.
- A resize/refit signal would be over-engineering: real terminal emulators
  (see Q2) resize in perceived real-time with no loading state, and adding
  one for a hidden-detail internal recovery mechanism (the 2-tick
  confirmation) would call attention to something that should be invisible
  when working correctly.
- A renderer-fallback signal is a rendering-internals concern, not a user
  job. Console-only (already present) is the right altitude — matches the
  existing pattern of `console.warn("[XtermTerminal] WebGL not available,
  using canvas fallback")`. No action needed.

## 2. Does a ~2-tick confirmation delay before fit() feel laggy?

Current behavior is already debounced 10ms (first 3 resizes) / 250ms
(steady state) per `resizeCount`/`debounceDelay` in `XtermTerminal.tsx`
lines 279-293. A "confirm on two consecutive ResizeObserver ticks" scheme
(per requirements.md AC #2) adds one extra RO tick before the existing
debounce timer starts, not two full debounce cycles — ResizeObserver ticks
fire once per animation frame during a continuous drag (~16ms at 60fps), so
worst case this adds roughly one frame (~16-33ms) of extra delay before the
250ms debounce timer even starts.

Industry reference points:
- VS Code's integrated terminal (xterm.js-based, same library this app
  uses) debounces its own resize-observer-driven fit similarly and is
  broadly perceived as instantaneous; users do not perceive terminal
  reflow lag until well past ~100ms.
- General UI latency budgets (Nielsen Norman Group's classic thresholds):
  ~100ms feels instantaneous, ~1s keeps flow uninterrupted. A resize
  confirm-delay of a few tens of ms is far under the 100ms "instantaneous"
  threshold and will be imperceptible, especially since visual content
  (the pane border, the browser chrome resizing) is already giving the
  user continuous feedback independent of when the terminal grid re-flows.
- Because the 200ms RPC-throttle (AC #3) and the 250ms fit debounce already
  exist and are unremarked-upon in current usage, an additional ~16-33ms
  gate is well within existing, already-accepted latency.

**Verdict**: not laggy. No latency budget change needed; the existing
debounce values (10ms/250ms) already set the applicable budget, and a
one-tick confirmation before that debounce fires is not perceptible.

## 3. Accessibility impact

Confirmed minimal/none, as expected:
- No focus is moved and no ARIA live region exists for resize/fit today,
  and the fix doesn't need to add one — resize/refit is a
  layout-mechanical operation, not a state announcement (unlike, say,
  connect/disconnect, which already gets `aria-label`s on the toolbar
  buttons: `"Reconnect to terminal"`, `"Resize terminal"` at lines
  548/596 in `TerminalOutput.tsx`).
- The Fit button (`handleManualResize`, `onClick={handleManualResize}`,
  `aria-label="Resize terminal"`, line 594-596) already has a correct,
  stable `aria-label` and keyboard reachability as an ordinary `<button>`.
  A `force:true` change to its resize-RPC call (AC #4) is behaviorally
  invisible to the label/keyboard path — no ARIA changes required.
- Terminal focus (`terminalRef.current?.focus()` in `XtermTerminal.tsx`
  imperative handle) is untouched by any of the proposed fixes; none of
  the four requirements-doc code changes (ResizeObserver confirm-gate,
  resize() dedupe, force:true call sites, WebGL/canvas tolerance) touch
  focus management.

**Verdict**: no accessibility work required.

## 4. Error/fallback visibility for WebGL→Canvas

As in Q1: console-only is correct. The user's job here is "have a terminal
that works," not "understand rendering internals." Two supporting points:

- The existing WebGL-unavailable-at-startup fallback (lines 148-155) is
  already console-only and has apparently not generated user complaints —
  precedent for treating renderer choice as an implementation detail.
  Repurposing the AC #5 "runtime px/col discrepancy" fallback (a *new*
  code path, triggered mid-session rather than at startup) the same way is
  consistent.
- A one-directional runtime fallback (WebGL → canvas, never back) is by
  definition a rare, one-time-per-session event correcting a rendering
  bug — surfacing it to the user would raise more questions ("why did my
  terminal just change?") than it answers, when in fact nothing
  user-actionable happened. If anything, a text glyph or cursor
  micro-flicker at the moment of renderer swap is possible (canvas repaint
  vs WebGL context swap) — that's a rendering/engineering concern for
  Agent handling implementation to keep sub-perceptible (e.g. swap between
  frames), not a UX signal to add.

**Verdict**: console-only logging is sufficient; no visible indicator
needed. Worth a one-line implementation note (not a UX requirement): keep
the renderer swap itself visually atomic (no blank frame) so it reads as
"nothing happened" rather than "something happened," but this is a QA/perf
detail, not new UX surface.

## Non-findings worth stating explicitly

- No new UI component, copy, or interaction is warranted by any of the
  seven acceptance criteria in requirements.md.
- The Fit button's existing behavior, label, and placement are unaffected
  by AC #4's `force:true` change — it is a wire-protocol-level fix, not a
  UI fix.
- The existing `stabilizing` connection-status affordance
  (`isWaitingForStableSize`, "Initializing...") is unrelated to this bug
  and should not be touched or reused for the resize-loop fix.
