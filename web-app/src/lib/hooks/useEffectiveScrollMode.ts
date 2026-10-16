import { useMemo, useSyncExternalStore } from "react";
import {
  decideScrollTarget,
  type ScrollMode,
  type ScrollOverride,
  type ScrollTarget,
  type TuiScrollPolicy,
} from "@/lib/terminal/scrollRouting";
import {
  DEFAULT_GESTURE_SCROLL,
  DEFAULT_SCROLL_OVERRIDE,
  scrollSettings,
  type ScrollSettingsStore,
} from "@/lib/terminal/scrollOverride";

export interface EffectiveScroll {
  target: ScrollTarget;
  source: "auto" | "override";
  gesturesOn: boolean;
}

/**
 * Single source for the route the toolbar PgUp/PgDn, chip and jump button follow.
 * The drag hook does not use it: it re-reads the terminal every frame.
 * Turning gestures off never changes the route, only `gesturesOn`.
 */
export function useEffectiveScrollMode(
  scrollMode: ScrollMode,
  override: ScrollOverride,
  gestureOn: boolean,
  tuiPolicy?: TuiScrollPolicy,
): EffectiveScroll {
  const { bufferType, mouseTrackingMode } = scrollMode;
  return useMemo(
    () => ({
      target: decideScrollTarget({ bufferType, mouseTrackingMode }, undefined, tuiPolicy, override),
      source: override === "auto" ? "auto" : "override",
      gesturesOn: gestureOn,
    }),
    [bufferType, mouseTrackingMode, override, gestureOn, tuiPolicy],
  );
}

export interface ScrollSettingsValues {
  override: ScrollOverride;
  gestureScrollEnabled: boolean;
}

/** Live view of the persisted override and Gesture scrolling switch. */
export function useScrollSettings(store: ScrollSettingsStore = scrollSettings): ScrollSettingsValues {
  const override = useSyncExternalStore(store.subscribe, store.getOverride, () => DEFAULT_SCROLL_OVERRIDE);
  const gestureScrollEnabled = useSyncExternalStore(store.subscribe, store.getGestureScroll, () => DEFAULT_GESTURE_SCROLL);
  return useMemo(() => ({ override, gestureScrollEnabled }), [override, gestureScrollEnabled]);
}
