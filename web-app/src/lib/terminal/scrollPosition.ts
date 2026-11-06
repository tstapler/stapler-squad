/**
 * Away-from-live detection and the TUI `netPagesUp` estimate (design/ux.md S7).
 * Pure: no DOM, no React. The estimate counts page keys sent up minus down
 * (drag and toolbar); the app owns its real position, so the estimate is only
 * trusted while nothing has happened that could have moved it.
 */

/** Overlays (chip, jump button) are hidden when fewer terminal rows are visible. */
export const MIN_ROWS_FOR_OVERLAYS = 5;
/** Beyond this the estimate is too uncertain to send keys from. */
export const NET_PAGES_UP_LIMIT = 5;

export interface ViewportPosition {
  viewportY: number;
  baseY: number;
}

/** Local buffer only: true when the viewport sits above the live bottom. */
export function isAwayFromLive({ viewportY, baseY }: ViewportPosition): boolean {
  return viewportY < baseY;
}

export type InvalidationReason = "keystroke" | "resize" | "mode-change" | "reconnect" | "overflow";

export interface NetPagesUpState {
  pages: number;
  valid: boolean;
  /** Why the estimate is invalid; undefined while valid. */
  reason?: InvalidationReason;
}

export interface NetPagesUpTracker {
  /** Identity changes only when the state does (safe for useSyncExternalStore). */
  getState(): NetPagesUpState;
  pageUp(): void;
  pageDown(): void;
  /**
   * Invalidation zeroes the count and hides the jump button. A later page-up
   * restarts the estimate from live (typing, resize and reconnect leave the app
   * at its live position); after "overflow" only markLive() restores it.
   */
  invalidate(reason: InvalidationReason): void;
  /** The view is known to be live again (jump tapped, fresh snapshot): valid, zero pages. */
  markLive(): void;
  subscribe(listener: () => void): () => void;
}

const LIVE: NetPagesUpState = { pages: 0, valid: true };

export function createNetPagesUpTracker(limit: number = NET_PAGES_UP_LIMIT): NetPagesUpTracker {
  let state: NetPagesUpState = LIVE;
  const listeners = new Set<() => void>();

  const set = (next: NetPagesUpState) => {
    if (next.pages === state.pages && next.valid === state.valid && next.reason === state.reason) return;
    state = next;
    listeners.forEach((l) => l());
  };
  const invalidate = (reason: InvalidationReason) => set({ pages: 0, valid: false, reason });

  return {
    getState: () => state,
    pageUp() {
      if (!state.valid && state.reason === "overflow") return;
      const pages = (state.valid ? state.pages : 0) + 1;
      if (pages > limit) invalidate("overflow");
      else set({ pages, valid: true });
    },
    pageDown() {
      if (!state.valid) return;
      set({ pages: Math.max(0, state.pages - 1), valid: true });
    },
    invalidate,
    markLive: () => set(LIVE),
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}
