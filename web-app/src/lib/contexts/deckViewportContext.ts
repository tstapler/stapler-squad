"use client";

import { createContext, useContext } from "react";

/**
 * The slice of viewport state the toast deck needs. ViewportProvider owns the
 * measurements; components/providers/DeckViewportBridge feeds them in here, so
 * the deck (a ui component) never imports a provider. Without a bridge (tests,
 * stories) it reports a desktop viewport with no keyboard.
 */
export interface DeckViewport {
  isInnerScreen: boolean;
  isVirtualKeyboardOpen: boolean;
  /** A phone on its side (narrower than 900px and wider than tall); the tray becomes a side panel. */
  isLandscape?: boolean;
}

export const DeckViewportContext = createContext<DeckViewport>({
  isInnerScreen: true,
  isVirtualKeyboardOpen: false,
});

export function useDeckViewport(): DeckViewport {
  return useContext(DeckViewportContext);
}
