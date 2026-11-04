"use client";

import { useSyncExternalStore } from "react";

/** True while the tab is in the background; polls should not run then. SSR counts as visible. */
export function isPageHidden(): boolean {
  return typeof document !== "undefined" && document.hidden;
}

/** Calls `listener` whenever the tab is shown or hidden. Returns the unsubscribe function. */
export function onPageVisibilityChange(listener: () => void): () => void {
  document.addEventListener("visibilitychange", listener);
  return () => document.removeEventListener("visibilitychange", listener);
}

/** Re-renders when the tab is shown or hidden. */
export function usePageVisible(): boolean {
  return !useSyncExternalStore(onPageVisibilityChange, isPageHidden, () => false);
}
