"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export type SuccessKind = "saved" | "merge_saved" | "reloaded" | "draft_restored" | "overwrote" | "reopened";

export interface FeedbackMessage {
  id: number;
  kind: "success" | "error" | "progress";
  text: string;
  /** Offered with a success (eviction undone): keeps the toast up longer. */
  undo?: () => void;
}

/** ux.md 8.6: successes clear after 8 s, an Undo toast after 12 s; errors persist until dismissed. */
export const SUCCESS_MS = 8000;
export const UNDO_MS = 12000;

export function successText(kind: SuccessKind, name: string): string {
  switch (kind) {
    case "saved": return `Saved ${name}`;
    case "merge_saved": return `Merged and saved ${name}`;
    case "reloaded": return `Reloaded ${name} from disk`;
    case "draft_restored": return `Restored your unsaved draft of ${name}`;
    case "overwrote": return `Overwrote ${name} on disk with your version`;
    case "reopened": return `Reopened ${name}`;
  }
}

/**
 * Success and error messages for the status-bar region and the mobile toast. The status region holds
 * one message (an error is never replaced by a success); the toast follows the latest success.
 */
export function useFilesFeedback() {
  const [message, setMessage] = useState<FeedbackMessage | null>(null);
  const [toast, setToast] = useState<FeedbackMessage | null>(null);
  const seq = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lifetime = useRef(SUCCESS_MS);

  const stop = useCallback(() => {
    if (timer.current !== null) clearTimeout(timer.current);
    timer.current = null;
  }, []);
  const clearSuccess = useCallback(() => {
    setToast(null);
    setMessage((m) => (m?.kind === "success" ? null : m));
  }, []);
  const start = useCallback((ms: number) => {
    stop();
    lifetime.current = ms;
    timer.current = setTimeout(clearSuccess, ms);
  }, [stop, clearSuccess]);
  useEffect(() => stop, [stop]);

  const announceSuccess = useCallback((kind: SuccessKind, name: string, opts: { undo?: () => void } = {}) => {
    const next: FeedbackMessage = { id: ++seq.current, kind: "success", text: successText(kind, name), undo: opts.undo };
    setToast(next);
    setMessage((m) => (m?.kind === "error" ? m : next));
    start(opts.undo ? UNDO_MS : SUCCESS_MS);
  }, [start]);

  /** A polite status-bar note with no toast (e.g. "Moved to line 15"); clears like a success. */
  const announceInfo = useCallback((text: string) => {
    setMessage((m) => (m?.kind === "error" ? m : { id: ++seq.current, kind: "success", text }));
    start(SUCCESS_MS);
  }, [start]);

  /** A wait in flight ("Checking the file on disk..."); the next success or error replaces it. */
  const announceProgress = useCallback((text: string) => {
    setMessage((m) => (m?.kind === "error" ? m : { id: ++seq.current, kind: "progress", text }));
  }, []);

  const announceError = useCallback((text: string) => {
    setMessage({ id: ++seq.current, kind: "error", text });
  }, []);

  const dismiss = useCallback(() => {
    stop();
    setToast(null);
    setMessage(null);
  }, [stop]);

  const pause = useCallback(() => stop(), [stop]);
  /** Hover or focus left: the full window starts again. */
  const resume = useCallback(() => {
    if (toast || message?.kind === "success") start(lifetime.current);
  }, [toast, message, start]);

  return { message, toast, announceSuccess, announceInfo, announceProgress, announceError, dismiss, pause, resume };
}

export type FilesFeedback = ReturnType<typeof useFilesFeedback>;
