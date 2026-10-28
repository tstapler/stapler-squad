"use client";

import { useCallback, useMemo, useRef } from "react";
import type { Dispatch, RefObject } from "react";
import { WINDOW_KEEP_ALIVE_DEFAULT, preloadFlagValue } from "@/lib/features/flagStore";
import { useFeatureFlagValue } from "@/lib/hooks/useFeatureFlagValue";
import type { PaneAction } from "@/lib/pane/paneTypes";
import type { WindowId } from "./windowTypes";

interface WindowsControllerInput {
  currentWindowId: WindowId;
  dispatchPane: (windowId: WindowId, action: PaneAction) => void;
  /** False while a different route is showing; focus effects become no-ops so they never take focus from it. */
  routeActive?: boolean;
}

export interface WindowsController {
  /** `window_keep_alive`, read through the shared flag store. */
  keepAlive: boolean;
  /** Dispatch bound to one window; identity is stable per window so a hidden container never re-renders for it. */
  paneDispatchFor: (windowId: WindowId) => Dispatch<PaneAction>;
  /** Ref for one window's cockpit container. */
  detailRefFor: (windowId: WindowId) => RefObject<HTMLDivElement | null>;
  /** Stable ref whose `current` is the active window's cockpit container (what the page's focus effects use). */
  activeDetailRef: RefObject<HTMLDivElement | null>;
  /** Focuses the active window's cockpit container; a no-op while off-route. */
  focusActiveDetail: () => void;
  /**
   * Points `activeDetailRef` at `windowId` before the next render of the owner. A layout-level provider
   * learns the current window from the page, so it must be told synchronously or the page's own effects
   * would resolve the previous window's container.
   */
  syncCurrentWindow: (windowId: WindowId) => void;
}

/**
 * Per-window state for the keep-alive host: dispatch, container refs and the flag. Provider-shaped (takes
 * only the current id and the shared dispatcher) so Story 4.1.5 can lift it into a layout-level provider.
 */
export function useWindowsController({
  currentWindowId,
  dispatchPane,
  routeActive = true,
}: WindowsControllerInput): WindowsController {
  const keepAlive = useFeatureFlagValue(
    "window_keep_alive",
    undefined,
    preloadFlagValue("window_keep_alive") ?? WINDOW_KEEP_ALIVE_DEFAULT,
  );

  const dispatchers = useRef(new Map<WindowId, Dispatch<PaneAction>>());
  const dispatchPaneRef = useRef(dispatchPane);
  dispatchPaneRef.current = dispatchPane;
  const paneDispatchFor = useCallback((windowId: WindowId) => {
    let dispatch = dispatchers.current.get(windowId);
    if (!dispatch) {
      dispatch = (action) => dispatchPaneRef.current(windowId, action);
      dispatchers.current.set(windowId, dispatch);
    }
    return dispatch;
  }, []);

  const detailRefs = useRef(new Map<WindowId, RefObject<HTMLDivElement | null>>());
  const detailRefFor = useCallback((windowId: WindowId) => {
    let ref = detailRefs.current.get(windowId);
    if (!ref) {
      ref = { current: null };
      detailRefs.current.set(windowId, ref);
    }
    return ref;
  }, []);

  const currentIdRef = useRef(currentWindowId);
  currentIdRef.current = currentWindowId;
  const activeDetailRef = useMemo(
    () =>
      ({
        get current() {
          return detailRefs.current.get(currentIdRef.current)?.current ?? null;
        },
      }) as RefObject<HTMLDivElement | null>,
    [],
  );

  const routeActiveRef = useRef(routeActive);
  routeActiveRef.current = routeActive;
  const focusActiveDetail = useCallback(() => {
    if (routeActiveRef.current) activeDetailRef.current?.focus();
  }, [activeDetailRef]);
  const syncCurrentWindow = useCallback((windowId: WindowId) => {
    currentIdRef.current = windowId;
  }, []);

  return { keepAlive, paneDispatchFor, detailRefFor, activeDetailRef, focusActiveDetail, syncCurrentWindow };
}
