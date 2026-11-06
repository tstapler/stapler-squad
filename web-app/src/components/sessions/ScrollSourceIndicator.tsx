"use client";
// +feature: scroll-source-indicator

import * as styles from "./TerminalOutput.css";

export interface ScrollSourceIndicatorProps {
  /** Program name from AppScrollbackResponse.program, e.g. "Claude Code". */
  program: string;
  /** True while a scroll lease showing forwarded history is held by this client. */
  visible: boolean;
}

/**
 * Persistent banner shown while the viewport is displaying an app-forwarded
 * page of a program's own history instead of the live tail (Story 1.4.2,
 * design/ux.md Surface 2). Unlike ScrollLoadingPill this does not auto-dismiss
 * on a timer — TerminalOutput.tsx unmounts it once a non-app-scrollback frame
 * arrives (return to live) or the session disconnects.
 */
export function ScrollSourceIndicator({ program, visible }: ScrollSourceIndicatorProps) {
  if (!visible) return null;

  return (
    <div
      className={styles.scrollSourceIndicator}
      role="status"
      aria-live="polite"
      data-testid="scroll-source-indicator"
    >
      <span aria-hidden="true">📜</span>
      <span>Viewing {program}&apos;s own history</span>
    </div>
  );
}
