"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { readUndoWindowMs } from "@/lib/utils/deckSettings";

/**
 * A configurable, pausable undo window for a deferred destructive action (Task
 * 4.4g). The action is held for the window; Undo cancels it with no RPC; the
 * window elapsing commits it. Hovering or focusing the undo control pauses the
 * countdown and resumes with the remaining time (WCAG 2.2.1). When the page is
 * hidden or closed the pending action is committed immediately with
 * `keepalive: true`, because undo can no longer be offered.
 *
 * Timers live here, never in a component: the hook owns one timeout per window.
 */

export interface UndoCommitOptions {
  /** True when committing because the page is going away. */
  keepalive: boolean;
}

export interface UndoWindowPending {
  /** Bumps every window; use as a React key. */
  id: number;
  label: string;
  windowMs: number;
}

interface Active extends UndoWindowPending {
  commit: (options: UndoCommitOptions) => void;
  remainingMs: number;
  startedAt: number;
  timer: ReturnType<typeof setTimeout> | null;
}

export interface UndoWindow {
  pending: UndoWindowPending | null;
  /** Starts a window; a window already open is committed first (never silently dropped). */
  start: (label: string, commit: (options: UndoCommitOptions) => void) => void;
  undo: () => void;
  pause: (reason: string) => void;
  resume: (reason: string) => void;
}

export function useUndoWindow(): UndoWindow {
  const [pending, setPending] = useState<UndoWindowPending | null>(null);
  const activeRef = useRef<Active | null>(null);
  const holdsRef = useRef<Set<string>>(new Set());
  const idRef = useRef(0);

  const clearTimer = () => {
    const active = activeRef.current;
    if (active?.timer) clearTimeout(active.timer);
    if (active) active.timer = null;
  };

  const finish = useCallback((keepalive: boolean, runCommit: boolean) => {
    const active = activeRef.current;
    if (!active) return;
    clearTimer();
    activeRef.current = null;
    holdsRef.current.clear();
    setPending(null);
    if (runCommit) active.commit({ keepalive });
  }, []);

  const arm = useCallback(() => {
    const active = activeRef.current;
    if (!active || holdsRef.current.size > 0) return;
    active.startedAt = Date.now();
    active.timer = setTimeout(() => finish(false, true), active.remainingMs);
  }, [finish]);

  const start = useCallback<UndoWindow["start"]>(
    (label, commit) => {
      if (activeRef.current) finish(false, true);
      const windowMs = readUndoWindowMs();
      idRef.current += 1;
      activeRef.current = { id: idRef.current, label, windowMs, commit, remainingMs: windowMs, startedAt: Date.now(), timer: null };
      setPending({ id: idRef.current, label, windowMs });
      arm();
    },
    [arm, finish],
  );

  const undo = useCallback(() => finish(false, false), [finish]);

  const pause = useCallback((reason: string) => {
    const active = activeRef.current;
    if (!active) return;
    if (holdsRef.current.size === 0 && active.timer) {
      clearTimer();
      active.remainingMs = Math.max(0, active.remainingMs - (Date.now() - active.startedAt));
    }
    holdsRef.current.add(reason);
  }, []);

  const resume = useCallback(
    (reason: string) => {
      holdsRef.current.delete(reason);
      if (holdsRef.current.size === 0) arm();
    },
    [arm],
  );

  // Page hide or close: the deferred action completes now, with keepalive.
  useEffect(() => {
    const flush = () => finish(true, true);
    const onVisibility = () => {
      if (document.visibilityState === "hidden") flush();
    };
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("pagehide", flush);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("pagehide", flush);
      clearTimer();
    };
  }, [finish]);

  return { pending, start, undo, pause, resume };
}
