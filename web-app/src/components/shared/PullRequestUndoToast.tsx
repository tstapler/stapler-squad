"use client";

import { useEffect, useRef, useState } from "react";
import * as s from "./PullRequestList.css";

/** Auto-dismiss delay; pauses while the pointer or focus is on the toast (WCAG 2.2.1). */
export const UNDO_TOAST_MS = 8000;

interface Props {
  message: string;
  /** Extra screen-reader sentence naming the untimed path ("Undo, or use Track a PR by URL."). */
  hint: string;
  onUndo: () => void;
  /** `hadFocus` tells the host to move focus to its destination instead of leaving it on a removed node. */
  onDismiss: (hadFocus: boolean) => void;
  focusUndoOnAppear: boolean;
}

export function PullRequestUndoToast({ message, hint, onUndo, onDismiss, focusUndoOnAppear }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const undoRef = useRef<HTMLButtonElement>(null);
  const [hovered, setHovered] = useState(false);
  const [focused, setFocused] = useState(false);
  const paused = hovered || focused;
  const dismissRef = useRef(onDismiss);
  dismissRef.current = onDismiss;

  useEffect(() => {
    if (focusUndoOnAppear) undoRef.current?.focus();
  }, [focusUndoOnAppear]);

  useEffect(() => {
    if (paused) return;
    const timer = setTimeout(() => dismissRef.current(false), UNDO_TOAST_MS);
    return () => clearTimeout(timer);
  }, [paused]);

  return (
    <div
      ref={ref}
      role="status"
      className={s.toast}
      data-testid="pr-undo-toast"
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
      onFocus={() => setFocused(true)}
      onBlur={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setFocused(false); }}
      onKeyDown={(e) => { if (e.key === "Escape") { e.stopPropagation(); onDismiss(focused); } }}
    >
      <span>{message}</span>
      <span className={s.visuallyHidden}>{hint}</span>
      <button ref={undoRef} type="button" className={s.toastButton} onClick={() => { onUndo(); onDismiss(focused); }}>{/* analytics-exempt */}
        Undo
      </button>
    </div>
  );
}
