"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";

export type PaneState = "files" | "editor" | "search";
export type PaneVia = "bottom_bar" | "back_arrow" | "system_back" | "tap_hit";

const KEY = "ssqFilesPane";

/** How many editor entries we pushed, as carried by a history state; 0 for any state that is not ours. */
const depthOf = (state: unknown): number => {
  const v = (state as Record<string, unknown> | null)?.[KEY];
  return typeof v === "number" ? v : 0;
};

/**
 * The mobile nav-stack (ux.md 7.4). Opening the editor pushes one history entry over the pane it came
 * from, so browser Back pops it exactly like the top-bar arrow and never leaves the session. The entry
 * carries its depth, which tells a pop from a forward and from a sheet's own entry (same depth). If
 * `pushState` throws (router race) the pane falls back to in-memory state. A keyboard resize is not a
 * navigation: nothing here listens to the viewport.
 */
export function useMobilePane(active: boolean) {
  const analytics = useFilesAnalytics({ component: "MobileFilesLayout" });
  const [pane, setPaneState] = useState<PaneState>("files");
  const paneRef = useRef<PaneState>("files");
  const origin = useRef<PaneState>("files");
  const depth = useRef(0);
  const viaOnPop = useRef<PaneVia | null>(null);

  const show = useCallback((to: PaneState, via: PaneVia) => {
    const from = paneRef.current;
    if (from === to) return;
    analytics.emit("files.mobile.pane_switch", { from, to, via });
    paneRef.current = to;
    setPaneState(to);
  }, [analytics]);

  const go = useCallback((to: PaneState, via: PaneVia) => {
    if (!active || to === paneRef.current) return;
    if (to !== "editor") {
      show(to, via);
      return;
    }
    origin.current = paneRef.current;
    try {
      window.history.pushState({ ...(window.history.state ?? {}), [KEY]: depth.current + 1 }, "");
      depth.current += 1;
    } catch {
      // router race: stay on in-memory state
    }
    show("editor", via);
  }, [active, show]);

  /** The top-bar arrow: the editor pops its entry and returns to its origin; Search returns to Files. */
  const back = useCallback((via: PaneVia = "back_arrow") => {
    const here = paneRef.current;
    if (here === "search") show("files", via);
    else if (here === "editor" && depth.current > 0) {
      viaOnPop.current = via;
      window.history.back();
    } else if (here === "editor") show(origin.current, via);
  }, [show]);

  useEffect(() => {
    if (!active) return;
    const onPopState = (e: PopStateEvent) => {
      const next = depthOf(e.state);
      const before = depth.current;
      if (next === before) return;
      depth.current = next;
      const via = viaOnPop.current ?? "system_back";
      viaOnPop.current = null;
      if (next > before) {
        show("editor", via);
        return;
      }
      const here = paneRef.current;
      if (here === "editor") show(origin.current, via);
      else if (here === "search") show("files", via);
    };
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, [active, show]);

  return { pane, origin: origin.current, go, back };
}
