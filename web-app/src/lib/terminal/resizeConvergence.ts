/**
 * resizeConvergence — pure decision functions for the terminal resize/fit convergence loop.
 *
 * Extracted so AC2 (gate `fit()` on live cols/rows), AC3 (dedup `TerminalResize` RPC sends),
 * and AC4 (detect resize oscillation) are independently unit-testable without mounting React
 * components or mocking xterm.js internals. See
 * `project_plans/terminal-resize-fit-loop/implementation/plan.md` Phase 1.
 *
 * Terminology note: `shouldAbandonWebgl`'s name and this module's naming avoid the word
 * "canvas" on purpose. When the WebGL addon is disposed in response to an oscillation, xterm.js
 * does NOT fall back to a canvas renderer — `@xterm/addon-canvas` is deprecated and incompatible
 * with the pinned `@xterm/xterm ^6.0.0`. It falls back to xterm.js's default (DOM) renderer. See
 * `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md` for the full rationale.
 */

export interface TerminalSize {
  cols: number;
  rows: number;
}

/**
 * AC2 gate: true iff `FitAddon.proposeDimensions()`'s integer output genuinely differs from the
 * terminal's live `cols`/`rows`. Returns `false` (not `true`) when either proposed value is
 * `undefined` (pre-layout / addon not ready yet).
 *
 * `current` must be the live `terminal.cols`/`terminal.rows`, never a fit()-only ref — see
 * plan.md's Pattern Decisions ("AC2 comparison baseline").
 */
export function shouldFit(
  proposed: Partial<TerminalSize> | undefined,
  current: TerminalSize,
): boolean {
  if (proposed?.cols === undefined || proposed?.rows === undefined) return false;
  return proposed.cols !== current.cols || proposed.rows !== current.rows;
}

/**
 * AC3 gate: true iff `next` differs from `lastSent` (or `lastSent` is `null`, i.e. nothing has
 * been sent yet). Independent of, and applied in addition to, the existing 200ms time-throttle.
 */
export function shouldSendResize(next: TerminalSize, lastSent: TerminalSize | null): boolean {
  return lastSent === null || lastSent.cols !== next.cols || lastSent.rows !== next.rows;
}

/** One entry in the oscillation/burst history — a resize that `terminal.onResize` reported. */
export interface ResizeEvent extends TerminalSize {
  at: number;
}

/** Default oscillation-detection window, in ms — single source of truth (also used by callers). */
export const OSCILLATION_WINDOW_MS = 2000;

/** Default oscillation-detection repeat threshold — single source of truth (also used by callers). */
export const OSCILLATION_THRESHOLD = 3;

/**
 * AC4 oscillation/burst detector: true iff the most recent `{cols, rows}` entry in `history`
 * recurs `>= threshold` times within `windowMs` of `now`. Entries older than `windowMs` are
 * pruned before counting; the window boundary is inclusive (`now - e.at <= windowMs`).
 */
export function shouldAbandonWebgl(
  history: ResizeEvent[],
  now: number,
  windowMs = OSCILLATION_WINDOW_MS,
  threshold = OSCILLATION_THRESHOLD,
): boolean {
  const recent = history.filter((e) => now - e.at <= windowMs);
  if (recent.length === 0) return false;
  const last = recent[recent.length - 1];
  const matches = recent.filter((e) => e.cols === last.cols && e.rows === last.rows);
  return matches.length >= threshold;
}
