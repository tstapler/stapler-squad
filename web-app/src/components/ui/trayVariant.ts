/**
 * Pure rules for where the notification tray sits and which single entry point
 * is shown. Kept free of React and the DOM so they are table-testable.
 */

export type TrayVariant = "side-overlay" | "bottom-sheet" | "top-sheet" | "landscape-panel";

export interface TrayViewport {
  /** Viewport is >= 900px wide. */
  isInnerScreen: boolean;
  isVirtualKeyboardOpen: boolean;
  /** Narrower than 900px and wider than tall. */
  isLandscape?: boolean;
}

/**
 * Desktop gets a right-edge overlay; a phone on its side a right panel; a phone with
 * the soft keyboard open a top-anchored sheet (so the keyboard never covers it);
 * everything else the bottom sheet.
 */
export function selectTrayVariant(viewport: TrayViewport): TrayVariant {
  if (viewport.isInnerScreen) return "side-overlay";
  if (viewport.isLandscape) return "landscape-panel";
  if (viewport.isVirtualKeyboardOpen) return "top-sheet";
  return "bottom-sheet";
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
