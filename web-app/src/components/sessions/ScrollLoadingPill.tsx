"use client";
// +feature: scroll-forward-loading-pill

import * as styles from "./TerminalOutput.css";

export interface ScrollLoadingPillProps {
  /** True once the 150ms show-delay has elapsed with the request still in flight. */
  visible: boolean;
  /** True once the request has been pending 8s total (design/ux.md Surface 1, UX-AC-5). */
  stalled: boolean;
  /** Program name from the last AppScrollbackResponse this session has seen, if any. */
  program?: string;
  /** Cancel handler — resets local fetch-in-flight state without cancelling the in-flight request. */
  onCancel: () => void;
}

/**
 * Loading affordance for a scroll-up-triggered scrollback fetch (Story 1.4.5,
 * design/ux.md Surface 1) — shared by both the pre-existing tmux-native
 * paged-scrollback path and Story 1.4.0's app-forwarded (alt-screen) path.
 * Reuses `reconnectingBanner`'s exact visual shape. TerminalOutput.tsx owns
 * the 150ms show-delay and 8s stalled timer; this component only renders
 * the state it's given.
 */
export function ScrollLoadingPill({ visible, stalled, program, onCancel }: ScrollLoadingPillProps) {
  if (!visible) return null;

  const label = stalled
    ? program
      ? `Still trying to load ${program}'s history…`
      : "Still trying to load more…"
    : program
      ? `Loading ${program}'s history…`
      : "Loading more…";

  return (
    <div
      className={styles.scrollLoadingPill}
      role="status"
      aria-live="polite"
      data-testid="scroll-loading-pill"
    >
      <span>{label}</span>
      {stalled && (
        // A transient loading-state escape hatch, not a user-initiated
        // feature action — mirrors the untracked reconnect/dismiss controls
        // elsewhere in this file.
        // analytics-exempt
        <button
          type="button"
          className={styles.scrollLoadingPillCancel}
          onClick={onCancel}
          data-testid="scroll-loading-pill-cancel"
        >
          Cancel
        </button>
      )}
    </div>
  );
}
