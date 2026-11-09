import { isFiniteResizeDimensions, type ResizeDimensions } from "@/lib/terminal/types";

/** The slice of xterm's FitAddon that safeFit needs. */
export interface FitLike {
  proposeDimensions(): { cols: number; rows: number } | undefined;
  fit(): void;
}

/**
 * FitAddon.proposeDimensions() reads `terminal._core._renderService.dimensions`, and
 * `_renderService` is undefined until the renderer exists and again after a renderer swap or
 * dispose: it throws "Cannot read properties of undefined (reading 'dimensions')". fit() calls
 * it internally. On a reload with several terminals mounting at once that is dozens of
 * uncaught errors, so an unmeasurable terminal is reported as "no proposal" instead.
 */
export function safeProposeDimensions(fit: FitLike): ResizeDimensions | undefined {
  try {
    return fit.proposeDimensions();
  } catch {
    return undefined;
  }
}

/** Fits only when the terminal can be measured; returns whether a fit was applied. */
export function safeFit(fit: FitLike): boolean {
  if (!isFiniteResizeDimensions(safeProposeDimensions(fit))) return false;
  try {
    fit.fit();
    return true;
  } catch {
    return false; // the renderer went away between the proposal and the fit
  }
}
