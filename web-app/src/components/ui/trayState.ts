/**
 * The tray's eight display states as one pure selector, so connectivity and a
 * failed fetch are never merged into one flag and every state names its exit.
 */

export type TrayStateKind =
  | "loading"
  | "offline"
  | "load-error"
  | "empty-filtered"
  | "all-caught-up"
  | "needs-attention-empty"
  | "load-more"
  | "list";

export interface TrayStateInput {
  /** True for anything but "connected" (the one connectivity source). */
  isOffline: boolean;
  /** A history fetch is in flight. */
  loading: boolean;
  /** The most recent fetch failed (a load error, not offline). */
  hasLoadError: boolean;
  /** A fetch has succeeded at least once. */
  hasLoadedOnce: boolean;
  rowCount: number;
  /** Search text or a type filter is narrowing the list. */
  filtered: boolean;
  needsAttentionCount: number;
  hasMore: boolean;
}

export interface TrayState {
  kind: TrayStateKind;
  /** Banner or empty-state copy; null when the list simply renders. */
  text: string | null;
  /** The visible control that leaves the state; null when the state exits on its own. */
  exit: "retry" | "clear-filters" | "load-more" | null;
}

export function selectTrayState(input: TrayStateInput): TrayState {
  if (input.loading && input.rowCount === 0 && !input.hasLoadedOnce) {
    return { kind: "loading", text: null, exit: null };
  }
  if (input.rowCount === 0) {
    // Offline and load errors are failures, never "All caught up" (TE-1).
    if (input.isOffline) return { kind: "offline", text: "Offline - showing cached", exit: null };
    if (input.hasLoadError) return { kind: "load-error", text: "Could not load notifications.", exit: "retry" };
    if (input.filtered) return { kind: "empty-filtered", text: "No matching notifications", exit: "clear-filters" };
    return { kind: "all-caught-up", text: "All caught up", exit: null };
  }
  if (input.hasMore) return { kind: "load-more", text: null, exit: "load-more" };
  if (input.needsAttentionCount === 0) return { kind: "needs-attention-empty", text: "Nothing needs attention", exit: null };
  return { kind: "list", text: null, exit: null };
}

/** Banner shown above rows while the cached list is on screen (TE-2, TE-3). */
export function selectTrayBanner(input: Pick<TrayStateInput, "isOffline" | "hasLoadError" | "rowCount">): "offline" | "load-error" | null {
  if (input.isOffline) return "offline";
  if (input.hasLoadError && input.rowCount > 0) return "load-error";
  return null;
}
