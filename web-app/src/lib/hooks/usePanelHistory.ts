"use client";

import { useEffect, useRef } from "react";

interface Options {
  /** The panel is open and presented as a page/sheet that owns a history entry. */
  active: boolean;
  /** Called when the browser's Back closes the panel. */
  onBack: () => void;
  /** Opened by a deep link: reuse the current entry instead of adding one. */
  replace?: boolean;
}

/**
 * One history entry per open panel (ux.md 11.3): Back closes it, closing it any other way
 * removes the entry, and a deep link replaces the current entry so Back leaves the page.
 * `releaseHistoryEntry` lets a navigation that closes the panel keep its own new entry.
 */
export function usePanelHistory({ active, onBack, replace = false }: Options): { releaseHistoryEntry: () => void } {
  const backRef = useRef(onBack);
  backRef.current = onBack;
  const ownsEntry = useRef(false);
  // A navigation that closes the panel must not also history.back(): that pops the page it just opened.
  const releaseHistoryEntry = useRef(() => {
    ownsEntry.current = false;
  }).current;

  useEffect(() => {
    if (!active) return;
    const state = { ...(window.history.state ?? {}), ssqTray: true };
    if (replace) window.history.replaceState(state, "");
    else window.history.pushState(state, "");
    ownsEntry.current = !replace;
    const onPopState = () => {
      ownsEntry.current = false;
      backRef.current();
    };
    window.addEventListener("popstate", onPopState);
    return () => {
      window.removeEventListener("popstate", onPopState);
      if (ownsEntry.current) {
        ownsEntry.current = false;
        window.history.back();
      }
    };
  }, [active, replace]);

  return { releaseHistoryEntry };
}
