"use client";

import { useEffect, type RefObject } from "react";
import { replaceHash } from "@/lib/nav";

export const PULL_REQUESTS_ANCHOR = "pull-requests";
const OPEN_EVENT = "ssq:pull-requests-anchor";
// Survives a route update that rewrites the URL (and drops the hash) before the section mounts.
let revealPending = false;

/**
 * Points the URL at the detail "Pull requests" section (replace, so Back returns to the list) and
 * tells an already-mounted section to scroll and focus. A section mounting later reads the hash.
 */
export function openPullRequestsAnchor(): void {
  revealPending = true;
  replaceHash(`#${PULL_REQUESTS_ANCHOR}`);
  window.dispatchEvent(new Event(OPEN_EVENT));
}

/** Scrolls to and focuses the section heading when the anchor is requested or present at mount. */
export function usePullRequestsAnchor(headingRef: RefObject<HTMLElement | null>, ready = true): void {
  useEffect(() => {
    if (!ready) return;
    const reveal = () => {
      const el = headingRef.current;
      if (!el) return;
      revealPending = false;
      el.scrollIntoView?.({ block: "start" });
      el.focus({ preventScroll: true });
    };
    if (revealPending || window.location.hash === `#${PULL_REQUESTS_ANCHOR}`) reveal();
    window.addEventListener(OPEN_EVENT, reveal);
    return () => window.removeEventListener(OPEN_EVENT, reveal);
  }, [headingRef, ready]);
}
