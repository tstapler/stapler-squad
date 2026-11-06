import type { MouseEvent } from "react";

const INTERACTIVE = "a, button, input, textarea, select, [role=button], [role=menuitem]";

/**
 * UX-AC71, shared by every list: a click on row text toggles its details, a control inside the
 * row never does (the row's own title button opts back in with `data-row-toggle`), and ending
 * a text selection inside the row is not a click on it.
 */
export function isRowTextClick(e: MouseEvent<HTMLElement>): boolean {
  const control = (e.target as HTMLElement).closest(INTERACTIVE);
  if (control && !control.hasAttribute("data-row-toggle")) return false;
  return !window.getSelection()?.toString();
}
