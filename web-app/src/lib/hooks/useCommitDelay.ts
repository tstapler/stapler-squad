"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { getTiming } from "./interactionTiming";
import { useEngagement } from "./useEngagedCard";

export type CancelReason = "undo" | "leave";

interface Options<T> {
  /** Runs once, when the hold ran its full length with the operator still here. */
  onCommit: (held: T) => void;
  /** Runs when the hold ended without sending: Undo, or the operator left. */
  onCancel: (held: T, reason: CancelReason) => void;
  /** Clicks outside this element's closest match cancel the hold (the desktop popover). */
  insideSelector?: string;
}

export interface CommitDelay<T> {
  held: T | null;
  secondsLeft: number;
  /** Starts the hold, or commits at once when the delay is 0. */
  start: (held: T) => void;
  undo: () => void;
}

const SECOND_MS = 1_000;

/**
 * The 3 s hold behind Approve, Send back and Mark done (design/ux.md 2.4). Leaving cancels and
 * never commits: panel close, an outside click, route change, Back, `visibilitychange` to hidden,
 * `pagehide`, going offline or unmounting mid-hold all send nothing. The countdown is plain text.
 */
export function useCommitDelay<T>({ onCommit, onCancel, insideSelector = "[data-notification-tray]" }: Options<T>): CommitDelay<T> {
  const [held, setHeld] = useState<T | null>(null);
  const [secondsLeft, setSecondsLeft] = useState(0);
  const heldRef = useRef<T | null>(null);
  const timers = useRef<{ commit?: ReturnType<typeof setTimeout>; tick?: ReturnType<typeof setInterval> }>({});
  const handlers = useRef({ onCommit, onCancel });
  handlers.current = { onCommit, onCancel };
  const tracker = useEngagement();
  const pathname = usePathname();

  const stop = useCallback(() => {
    clearTimeout(timers.current.commit);
    clearInterval(timers.current.tick);
    timers.current = {};
    heldRef.current = null;
    setHeld(null);
  }, []);

  const cancel = useCallback(
    (reason: CancelReason) => {
      const current = heldRef.current;
      if (current === null) return;
      stop();
      handlers.current.onCancel(current, reason);
    },
    [stop],
  );

  const start = useCallback(
    (next: T) => {
      const delay = getTiming().commitDelayMs;
      if (delay <= 0) {
        handlers.current.onCommit(next);
        return;
      }
      stop();
      heldRef.current = next;
      setHeld(next);
      setSecondsLeft(Math.ceil(delay / SECOND_MS));
      const startedAt = Date.now();
      timers.current.tick = setInterval(() => setSecondsLeft(Math.max(1, Math.ceil((delay - (Date.now() - startedAt)) / SECOND_MS))), SECOND_MS);
      timers.current.commit = setTimeout(() => {
        const committing = heldRef.current;
        stop();
        if (committing !== null) handlers.current.onCommit(committing);
      }, delay);
    },
    [stop],
  );

  const undo = useCallback(() => cancel("undo"), [cancel]);
  const leave = useCallback(() => cancel("leave"), [cancel]);

  useEffect(() => {
    if (held === null) return;
    const onVisibility = () => document.visibilityState === "hidden" && leave();
    const onPointerDown = (e: PointerEvent) => {
      if (!(e.target as Element | null)?.closest?.(insideSelector)) leave();
    };
    document.addEventListener("visibilitychange", onVisibility);
    document.addEventListener("pointerdown", onPointerDown, true);
    window.addEventListener("pagehide", leave);
    window.addEventListener("offline", leave);
    window.addEventListener("popstate", leave);
    const offClose = tracker?.onClose(leave);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      document.removeEventListener("pointerdown", onPointerDown, true);
      window.removeEventListener("pagehide", leave);
      window.removeEventListener("offline", leave);
      window.removeEventListener("popstate", leave);
      offClose?.();
    };
  }, [held, leave, tracker, insideSelector]);

  // A route change mid-hold is leaving too.
  const firstPath = useRef(pathname);
  useEffect(() => {
    if (pathname !== firstPath.current) {
      firstPath.current = pathname;
      leave();
    }
  }, [pathname, leave]);

  // Unmounting mid-hold (the row left the list, the page closed) also cancels.
  useEffect(() => leave, [leave]);

  return { held, secondsLeft, start, undo };
}
