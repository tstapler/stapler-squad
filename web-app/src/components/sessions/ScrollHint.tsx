"use client";
// +feature: terminal-scroll-settings

import { useCallback, useRef, useState } from "react";
import type { ScrollTarget } from "@/lib/terminal/scrollRouting";
import * as styles from "./ScrollingPanel.css";

export const SCROLL_HINT_SEEN_KEY = "terminal-scroll-hint-seen";

export const HINT_TEXT_LOCAL = "Dragging scrolls the terminal's history. Tap the chip to change.";
export const HINT_TEXT_TUI = "Dragging sends Page Up/Down to the app. Tap the chip to change.";

type StorageLike = Pick<Storage, "getItem" | "setItem">;

export interface UseScrollHintOptions {
  /** Resolved on every access; a throwing or missing storage falls back to memory. */
  storage?: () => StorageLike | null;
}

function defaultStorage(): StorageLike | null {
  return typeof localStorage === "undefined" ? null : localStorage;
}

/**
 * First-use hint state. Shown on the first scroll start while the seen flag is unset; stays
 * (no timeout) until dismissed or the picker/panel opens. The flag is written then, not on display.
 */
export function useScrollHint(options: UseScrollHintOptions = {}) {
  const getStorage = options.storage ?? defaultStorage;
  const memorySeen = useRef(false);
  const [visible, setVisible] = useState(false);
  const [route, setRoute] = useState<ScrollTarget>("xterm-local");

  const isSeen = useCallback((): boolean => {
    if (memorySeen.current) return true;
    try {
      return getStorage()?.getItem(SCROLL_HINT_SEEN_KEY) === "1";
    } catch {
      return false;
    }
  }, [getStorage]);

  const markSeen = useCallback(() => {
    memorySeen.current = true;
    try {
      getStorage()?.setItem(SCROLL_HINT_SEEN_KEY, "1");
    } catch {
      // in-memory flag still applies for this page load
    }
  }, [getStorage]);

  const notifyScrollStart = useCallback(
    (next: ScrollTarget) => {
      if (isSeen()) return;
      setRoute(next);
      setVisible(true);
    },
    [isSeen],
  );

  const dismiss = useCallback(() => {
    markSeen();
    setVisible(false);
  }, [markSeen]);

  return { visible, route, notifyScrollStart, dismiss, markSeenOnPanelOpen: dismiss };
}

export interface ScrollHintProps {
  visible: boolean;
  route: ScrollTarget;
  onDismiss: () => void;
}

/** The status region stays mounted so the hint's appearance is announced once. */
export function ScrollHint({ visible, route, onDismiss }: ScrollHintProps) {
  return (
    <div role="status" aria-live="polite">
      {visible && (
        <div className={styles.hint} data-testid="scroll-hint">
          <span>{route === "xterm-local" ? HINT_TEXT_LOCAL : HINT_TEXT_TUI}</span>
          <button type="button" className={styles.button} onClick={onDismiss}>
            Got it
          </button>
        </div>
      )}
    </div>
  );
}
