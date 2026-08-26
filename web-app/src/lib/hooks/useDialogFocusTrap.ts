import { useEffect, useRef, RefObject } from "react";

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

interface UseDialogFocusTrapOptions {
  /** Ref to the dialog container element that Tab-cycling is scoped to. */
  dialogRef: RefObject<HTMLElement | null>;
  /** Ref to the element that should receive focus when the dialog mounts. */
  initialFocusRef: RefObject<HTMLElement | null>;
  /** Called on Escape (typically the dialog's cancel/close handler). */
  onEscape: () => void;
}

/**
 * Shared modal focus-trap behavior: focuses `initialFocusRef` on mount,
 * restores focus to whatever was focused before the dialog opened once it
 * unmounts, and cycles Tab/Shift+Tab within `dialogRef` while routing
 * Escape to `onEscape`.
 *
 * Extracted from BackwardSyncConfirmDialog and HostKeyTrustDialog, which
 * both implemented this identically apart from which button receives
 * initial focus (Confirm vs Cancel).
 */
export function useDialogFocusTrap({ dialogRef, initialFocusRef, onEscape }: UseDialogFocusTrapOptions) {
  // Capture whatever had focus when the dialog mounted (the element that
  // triggered it) so focus can be restored on close.
  const previouslyFocusedRef = useRef<HTMLElement | null>(
    typeof document !== "undefined" ? (document.activeElement as HTMLElement | null) : null
  );

  useEffect(() => {
    initialFocusRef.current?.focus();
    return () => {
      previouslyFocusedRef.current?.focus();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once on mount/unmount only
  }, []);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onEscape();
        return;
      }
      if (e.key === "Tab" && dialogRef.current) {
        const focusable = Array.from(dialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
        if (focusable.length === 0) return;
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [dialogRef, onEscape]);
}
