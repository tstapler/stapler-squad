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
   * Not called if the terminal is disposed first.
   */
  onFitted?: (dims: { cols: number; rows: number }) => void;
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
  if (renderer === "webgl") terminal.clearTextureAtlas?.();
  terminal.refresh(0, Math.max(0, rows - 1));
}

/** FitGuard: false when the container has no layout box, so `fit()` would size to 0. */
export function canFit(container: FitContainer | null | undefined): boolean {
  return !!container && container.clientWidth > 0 && container.clientHeight > 0;
}
