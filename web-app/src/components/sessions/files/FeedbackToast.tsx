"use client";

import type { FeedbackMessage } from "@/lib/hooks/useFilesFeedback";
import { toastBox, toastButton } from "./StatusBar.css";

interface FeedbackToastProps {
  toast: FeedbackMessage | null;
  onDismiss: () => void;
  onPause: () => void;
  onResume: () => void;
}

const reducedMotion = () => typeof window !== "undefined" && typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/** Mobile cue for a success; the status-bar region is the announced copy, so this one is not live. */
export function FeedbackToast({ toast, onDismiss, onPause, onResume }: FeedbackToastProps) {
  if (!toast) return null;
  return (
    <div
      className={toastBox}
      role="group"
      aria-label="Notification"
      data-testid="files-toast"
      data-motion={reducedMotion() ? "none" : "fade"}
      onMouseEnter={onPause}
      onMouseLeave={onResume}
      onFocus={onPause}
      onBlur={onResume}
    >
      <span>{toast.text}</span>
      {toast.undo && <button type="button" className={toastButton} onClick={() => { toast.undo?.(); onDismiss(); }}>Undo</button>}
      <button type="button" className={toastButton} aria-label="Dismiss" onClick={onDismiss}>×</button>
    </div>
  );
}
