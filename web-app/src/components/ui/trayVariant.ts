/**
 * Pure rules for where the notification tray sits and which single entry point
 * is shown. Kept free of React and the DOM so they are table-testable.
 */

export type TrayVariant = "side-overlay" | "bottom-sheet";

export interface TrayViewport {
  /** Viewport is >= 900px wide. */
  isInnerScreen: boolean;
  isVirtualKeyboardOpen: boolean;
}

/** Desktop gets a right-edge overlay; everything narrower gets the capped bottom sheet. */
export function selectTrayVariant(viewport: TrayViewport): TrayVariant {
  return viewport.isInnerScreen ? "side-overlay" : "bottom-sheet";
}

/** What the single phone entry shows; `floating-bottom` is the no-terminal page only. */
export type TrayAffordance = "bell" | "more-row" | "keyboard-chip" | "floating-bottom";

/**
 * Content of the one tray entry on a phone page. It changes what the entry says,
 * never where it is or whether it exists, so no frame shows two entries or none.
 */
export function trayAffordanceVariant(
  overflowCount: number,
  toastCount: number,
  keyboardOpen: boolean,
  hasTerminal: boolean,
): TrayAffordance {
  if (!hasTerminal) return "floating-bottom";
  if (keyboardOpen && toastCount > 0) return "keyboard-chip";
  if (overflowCount > 0) return "more-row";
  return "bell";
}

/** Peek and expanded sheet heights as a fraction of `--viewport-height` (TS-1: 22-28% and 84-86%). */
export const SHEET_PEEK_FRACTION = 0.25;
export const SHEET_EXPANDED_FRACTION = 0.85;
