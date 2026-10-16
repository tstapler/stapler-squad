/**
 * Pure rules for where the notification tray sits and which single entry point
 * is shown. Kept free of React and the DOM so they are table-testable.
 */

export type TrayVariant = "side-overlay" | "bottom-sheet" | "landscape-panel";

export interface TrayViewport {
  /** Viewport is >= 900px wide. */
  isInnerScreen: boolean;
  isVirtualKeyboardOpen: boolean;
  /** Narrower than 900px and wider than tall. */
  isLandscape?: boolean;
}

/**
 * Desktop gets a right-edge overlay; a phone on its side a right panel; every other
 * phone the bottom sheet. The soft keyboard does not change the variant: the sheet
 * stays put and rides the keyboard's top edge (CSS, `data-keyboard`), so opening the
 * keyboard never teleports it to another edge.
 */
export function selectTrayVariant(viewport: TrayViewport): TrayVariant {
  if (viewport.isInnerScreen) return "side-overlay";
  if (viewport.isLandscape) return "landscape-panel";
  return "bottom-sheet";
}

/**
 * What the phone entry shows; `floating-bottom` is the no-terminal page only. `none` means
 * there is nothing to say: the bottom nav's Alerts tab is the always-present way into the tray.
 */
export type TrayAffordance = "none" | "more-row" | "keyboard-chip" | "floating-bottom";

/** Content of the phone tray entry: overflow, the keyboard chip, or nothing. Never a bell. */
export function trayAffordanceVariant(
  overflowCount: number,
  toastCount: number,
  keyboardOpen: boolean,
  hasTerminal: boolean,
): TrayAffordance {
  if (!hasTerminal) return "floating-bottom";
  if (keyboardOpen && toastCount > 0) return "keyboard-chip";
  if (overflowCount > 0) return "more-row";
  return "none";
}

/** Peek and expanded sheet heights as a fraction of `--viewport-height` (TS-1: 22-28% and 84-86%). */
export const SHEET_PEEK_FRACTION = 0.25;
export const SHEET_EXPANDED_FRACTION = 0.85;
