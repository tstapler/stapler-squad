/**
 * Shared terminal value types.
 *
 * Hoisted out of XtermTerminal.tsx so useTerminalFlowControl.ts can import
 * ResizeDimensions / isFiniteResizeDimensions without depending on a leaf
 * component (see project_plans/terminal-resize-fit-loop/implementation/plan.md
 * §2 Domain Glossary, architecture-review.md Concern 1).
 */

/**
 * Value type replacing raw positional (cols, rows) pairs wherever a size is
 * held as state (applied / proposed / pending / last-sent dimensions).
 */
export interface ResizeDimensions {
  cols: number;
  rows: number;
}

/**
 * Canonical Number.isFinite guard on a ResizeDimensions value. Returns false
 * for undefined or for a value with a non-finite cols/rows (e.g. Infinity or
 * NaN), so callers can trust a `true` result before using the dimensions.
 */
export function isFiniteResizeDimensions(
  d: ResizeDimensions | undefined
): d is ResizeDimensions {
  return d !== undefined && Number.isFinite(d.cols) && Number.isFinite(d.rows);
}

/**
 * Structural cols/rows equality. Replaces the hand-rolled `a.cols === b.cols
 * && a.rows === b.rows` comparisons duplicated across the resize pipeline
 * (shouldScheduleFit's two checks, useTerminalFlowControl's resize()
 * value-dedup guard).
 */
export function dimensionsEqual(a: ResizeDimensions, b: ResizeDimensions): boolean {
  return a.cols === b.cols && a.rows === b.rows;
}
