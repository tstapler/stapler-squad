"use client";

import { useMemo, type ReactNode } from "react";
import { DeckViewportContext } from "@/lib/contexts/deckViewportContext";
import { useViewport } from "./ViewportProvider";

/** Publishes the viewport facts the notification deck needs; mount inside ViewportProvider. */
export function DeckViewportBridge({ children }: { children: ReactNode }) {
  const { isInnerScreen, isVirtualKeyboardOpen } = useViewport();
  const value = useMemo(() => ({ isInnerScreen, isVirtualKeyboardOpen }), [isInnerScreen, isVirtualKeyboardOpen]);
  return <DeckViewportContext.Provider value={value}>{children}</DeckViewportContext.Provider>;
}
