import type { CellRect, CursorRead } from "@/lib/terminal/cursorRect";

export type DeckAnchor = "bottom-right" | "top-right";

export interface DeckFootprint {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

const DECK_GAP_PX = 8;

/**
 * Where the desktop deck sits so it never covers the line being typed on.
 * Bottom-right is the default; the deck moves to the top-right anchor when its
 * footprint would cover the cursor cell, and also when a terminal is mounted but
 * its cursor cannot be read, since covering the input line is the costlier error.
 * With no terminal on the page there is nothing to cover.
 */
export function deckAnchor(cursor: CursorRead, bottomRightFootprint: DeckFootprint): DeckAnchor {
  if (cursor.kind === "no-terminal") return "bottom-right";
  if (cursor.kind === "unreadable") return "top-right";
  return intersects(cursor.rect, bottomRightFootprint) ? "top-right" : "bottom-right";
}

function intersects(cell: CellRect, deck: DeckFootprint): boolean {
  return (
    cell.left < deck.right + DECK_GAP_PX &&
    cell.left + cell.width > deck.left - DECK_GAP_PX &&
    cell.top < deck.bottom + DECK_GAP_PX &&
    cell.top + cell.height > deck.top - DECK_GAP_PX
  );
}

/** The rectangle a bottom-right deck would occupy in a viewport. */
export function bottomRightFootprint(
  viewport: { width: number; height: number },
  deck: { width: number; height: number },
  inset: { right: number; bottom: number },
): DeckFootprint {
  const right = viewport.width - inset.right;
  const bottom = viewport.height - inset.bottom;
  return { left: right - deck.width, right, bottom, top: bottom - deck.height };
}
