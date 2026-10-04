"use client";
// +feature: terminal-jump-to-latest

import { useCallback, useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import { srOnly } from "@/components/ui/LiveRegion.css";
import {
  MIN_ROWS_FOR_OVERLAYS,
  NET_PAGES_UP_LIMIT,
  isAwayFromLive,
  type NetPagesUpTracker,
} from "@/lib/terminal/scrollPosition";
import { PAGE_RATE_LIMIT_MS, encodePageKeys, type ScrollTarget } from "@/lib/terminal/scrollRouting";
import * as styles from "./JumpToLatestButton.css";

/** The slice of xterm's Terminal this component reads. */
export interface JumpTerminal {
  rows: number;
  buffer: { active: { viewportY: number; baseY: number; length: number } };
  scrollToBottom(): void;
  onScroll?(cb: () => void): { dispose(): void };
  onWriteParsed?(cb: () => void): { dispose(): void };
}

export interface ViewRect {
  top: number;
  bottom: number;
  left: number;
  right: number;
}

export interface JumpToLatestButtonProps {
  terminal: JumpTerminal | null;
  route: ScrollTarget;
  /** TUI page-key estimate shared with the drag hook and toolbar keys. */
  netPagesUp: NetPagesUpTracker;
  /** A change invalidates the TUI estimate (reconnect or full snapshot). */
  connectionEpoch: number;
  sendData: (data: string) => void;
  /** True while a chunked paste is in flight; TUI page keys are dropped (not queued) meanwhile. */
  isInputBusy?: () => boolean;
  /** True while a finger is down or momentum runs; placement never changes meanwhile. */
  gestureActive?: boolean;
  /** Bumped on each output write; drives the "new output" dot and announcement. */
  outputTick?: number;
  /** Viewport rectangle of the cursor row, for the top-right fallback placement. */
  getCursorRowRect?: () => ViewRect | null;
}

type Corner = "bottom-right" | "top-right";

const NEW_OUTPUT_ANNOUNCE_DELAY_MS = 2000;
const LOCAL_LABEL = "Jump to latest";
const TUI_LABEL = "Page down to latest";

interface BufferSnapshot {
  rows: number;
  away: boolean;
}

function readSnapshot(terminal: JumpTerminal | null): BufferSnapshot {
  if (!terminal) return { rows: 0, away: false };
  const { viewportY, baseY, length } = terminal.buffer.active;
  return { rows: terminal.rows, away: length > 0 && isAwayFromLive({ viewportY, baseY }) };
}

function overlaps(a: ViewRect, b: ViewRect): boolean {
  return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
}

/**
 * Affordance back to live output (design/ux.md S7). Local buffer: shown while the
 * viewport is above the bottom. TUI: shown only while the page-key estimate is valid
 * and positive. Never focuses the terminal, so the keyboard stays as it was.
 */
export function JumpToLatestButton({
  terminal,
  route,
  netPagesUp,
  connectionEpoch,
  sendData,
  isInputBusy,
  gestureActive = false,
  outputTick = 0,
  getCursorRowRect,
}: JumpToLatestButtonProps) {
  const [snapshot, setSnapshot] = useState(() => readSnapshot(terminal));
  const estimate = useSyncExternalStore(netPagesUp.subscribe, netPagesUp.getState, netPagesUp.getState);
  const isLocal = route === "xterm-local";

  const refresh = useCallback(() => {
    setSnapshot((prev) => {
      const next = readSnapshot(terminal);
      return next.rows === prev.rows && next.away === prev.away ? prev : next;
    });
  }, [terminal]);

  useEffect(() => {
    refresh();
    const subscriptions = [terminal?.onScroll?.(refresh), terminal?.onWriteParsed?.(refresh)];
    return () => subscriptions.forEach((s) => s?.dispose());
  }, [terminal, refresh]);

  const visible =
    snapshot.rows >= MIN_ROWS_FOR_OVERLAYS && (isLocal ? snapshot.away : estimate.valid && estimate.pages > 0);

  // ---- Reconnect: the estimate no longer describes the app's position ----
  const lastEpochRef = useRef(connectionEpoch);
  useEffect(() => {
    if (lastEpochRef.current === connectionEpoch) return;
    lastEpochRef.current = connectionEpoch;
    netPagesUp.invalidate("reconnect");
  }, [connectionEpoch, netPagesUp]);

  // ---- Placement: evaluated when shown and when a gesture ends, never mid-gesture ----
  const buttonRef = useRef<HTMLButtonElement>(null);
  const [corner, setCorner] = useState<Corner>("bottom-right");
  const evaluatePlacement = useCallback(() => {
    const cursor = getCursorRowRect?.();
    const own = buttonRef.current?.getBoundingClientRect();
    if (!cursor || !own || !overlaps(own, cursor)) return;
    setCorner((c) => (c === "bottom-right" ? "top-right" : "bottom-right"));
  }, [getCursorRowRect]);

  useLayoutEffect(() => {
    if (visible) evaluatePlacement();
    else setCorner("bottom-right");
    // Deliberately keyed on `visible` only: the corner is kept until the button hides.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible]);

  const wasGestureRef = useRef(gestureActive);
  useEffect(() => {
    if (wasGestureRef.current && !gestureActive && visible) evaluatePlacement();
    wasGestureRef.current = gestureActive;
  }, [gestureActive, visible, evaluatePlacement]);

  // ---- Output while scrolled: dot plus one debounced announcement per episode ----
  const [hasNewOutput, setHasNewOutput] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const announcedRef = useRef(false);
  const announceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastTickRef = useRef(outputTick);

  const clearAnnounceTimer = useCallback(() => {
    if (announceTimerRef.current) clearTimeout(announceTimerRef.current);
    announceTimerRef.current = null;
  }, []);

  useEffect(() => {
    if (lastTickRef.current === outputTick) return;
    lastTickRef.current = outputTick;
    if (!visible) return;
    setHasNewOutput(true);
    if (announcedRef.current || announceTimerRef.current) return;
    announceTimerRef.current = setTimeout(() => {
      announceTimerRef.current = null;
      announcedRef.current = true;
      setAnnouncement("New output below");
    }, NEW_OUTPUT_ANNOUNCE_DELAY_MS);
  }, [outputTick, visible]);

  useEffect(() => {
    if (visible) return;
    clearAnnounceTimer();
    announcedRef.current = false;
    setHasNewOutput(false);
    setAnnouncement("");
  }, [visible, clearAnnounceTimer]);

  // ---- Tap ----
  const pageKeyTimersRef = useRef(new Set<ReturnType<typeof setTimeout>>());
  const cancelPageKeyTimers = useCallback(() => {
    pageKeyTimersRef.current.forEach(clearTimeout);
    pageKeyTimersRef.current.clear();
  }, []);

  useEffect(() => () => {
    cancelPageKeyTimers();
    clearAnnounceTimer();
  }, [cancelPageKeyTimers, clearAnnounceTimer]);

  // Queued extra PgDn keys target a position that no longer holds once the estimate is invalidated
  // (keystroke, resize, mode change, reconnect), the epoch bumps, or the route flips.
  useEffect(() => {
    if (!estimate.valid) cancelPageKeyTimers();
  }, [estimate.valid, cancelPageKeyTimers]);
  useEffect(() => cancelPageKeyTimers, [route, connectionEpoch, cancelPageKeyTimers]);

  const handleClick = () => {
    if (isLocal) {
      terminal?.scrollToBottom();
      return;
    }
    if (isInputBusy?.()) return;
    const count = Math.min(estimate.pages, NET_PAGES_UP_LIMIT);
    netPagesUp.markLive();
    for (let i = 0; i < count; i++) {
      const send = () => {
        if (!isInputBusy?.()) sendData(encodePageKeys("newer"));
      };
      if (i === 0) {
        send();
        continue;
      }
      const timers = pageKeyTimersRef.current;
      const id = setTimeout(() => {
        timers.delete(id);
        send();
      }, PAGE_RATE_LIMIT_MS * i);
      timers.add(id);
    }
  };

  return (
    <>
      <div role="status" aria-live="polite" className={srOnly}>
        {announcement}
      </div>
      {visible && (
        <button
          ref={buttonRef}
          type="button"
          className={styles.button}
          data-corner={corner}
          onMouseDown={(e) => e.preventDefault()}
          onClick={handleClick}
        >
          <span aria-hidden="true">↓</span>
          <span>{isLocal ? LOCAL_LABEL : TUI_LABEL}</span>
          {hasNewOutput && (
            <>
              <span className={styles.newOutputDot} data-testid="jump-new-output-dot" aria-hidden="true" />
              <span className={srOnly}>new output</span>
            </>
          )}
        </button>
      )}
    </>
  );
}
