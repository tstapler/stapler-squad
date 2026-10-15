/**
 * Repaint seam and zero-size guard for terminal fits (Story 2.1.1).
 * Pure: no React, no xterm imports, so the sampler in XtermTerminal can be
 * tested against fakes.
 */
import { mobileDebug } from "./mobileDebug";

export type RendererKind = "webgl" | "canvas" | "dom";

export type RefitReason =
  | "viewport-settle"
  | "visibility"
  | "manual-resize"
  | "font-change"
  | "context-loss";

export interface RequestFitOptions {
  forceRepaint: boolean;
  reason: RefitReason;
  onFitted?: RefitOptions["onFitted"];
}

/** Options for the XtermTerminal handle's `refit()`. */
export interface RefitOptions {
  reason?: RefitReason;
  /**
   * Called once when the fit run ends, whichever way it ends (fit applied, dims already at rest,
   * sampler gave up, or zero-size retry exhausted), with the terminal's dims at that moment.
   * Not called if the terminal is disposed first or the container is gone when refit() is requested.
   * `stale` is true when the zero-size retry gave up: the dims are the pre-hide ones, not a fresh fit.
   */
  onFitted?: (dims: { cols: number; rows: number; stale?: boolean }) => void;
}

/** Zero-size retry bounds: whichever limit is hit first ends the retry. */
export const FIT_RETRY_MAX_ATTEMPTS = 20;
export const FIT_RETRY_TIMEOUT_MS = 1000;

export interface RepaintTerminal {
  rows: number;
  refresh(start: number, end: number): void;
  clearTextureAtlas?(): void;
}

export interface FitContainer {
  clientWidth: number;
  clientHeight: number;
}

/**
 * Repaints every row. `refresh` is renderer-independent and must suffice on its
 * own (the target Android phone likely runs the canvas renderer); the texture
 * atlas only exists under WebGL.
 */
export function postFitRepaint(terminal: RepaintTerminal, renderer: RendererKind, reason: RefitReason): void {
  const rows = terminal.rows;
  mobileDebug.log("forced-refresh", { event: "forced-refresh", rows, renderer, reason });
  if (renderer === "webgl") {
    // addon-webgl shares one atlas texture across terminals with the same font config, but
    // clearTextureAtlas() only resets the caller's glyph model; peers would draw stale glyph
    // indices (garbled text) until repainted. Clear and repaint every live webgl terminal,
    // once per frame: later callers in the same frame (every pane fires visibilitychange)
    // only refresh, since a second wipe would garble peers that already repainted.
    const now = performance.now();
    // Timestamp, not rAF: rAF is paused in hidden tabs, which would stick the guard shut
    // exactly when visibilitychange needs the clear. now < last covers a reset clock.
    if (now < lastAtlasClearAt || now - lastAtlasClearAt >= ATLAS_CLEAR_WINDOW_MS) {
      lastAtlasClearAt = now;
      for (const t of new Set([terminal, ...webglTerminals])) {
        t.clearTextureAtlas?.();
        t.refresh(0, Math.max(0, t.rows - 1));
      }
      return;
    }
  }
  terminal.refresh(0, Math.max(0, rows - 1));
}

const ATLAS_CLEAR_WINDOW_MS = 16;
let lastAtlasClearAt = -Infinity;

const webglTerminals = new Set<RepaintTerminal>();

/** Drop a terminal from the shared-atlas repaint set (dispose, or fallback off webgl). */
export function forgetWebglTerminal(terminal: RepaintTerminal): void {
  webglTerminals.delete(terminal);
}

/** Track a terminal that loaded the webgl addon so peers repaint when it clears the shared atlas. */
export function trackWebglTerminal(terminal: RepaintTerminal): void {
  webglTerminals.add(terminal);
}

/** FitGuard: false when the container has no layout box, so `fit()` would size to 0. */
export function canFit(container: FitContainer | null | undefined): boolean {
  return !!container && container.clientWidth > 0 && container.clientHeight > 0;
}
