import { useEffect } from "react";

interface ReviewNavHandlers {
  isFullscreen: boolean;
  setIsFullscreen: (v: boolean) => void;
  showNavigation: boolean;
  onNext?: () => void;
  onPrevious?: () => void;
  /** Escape-to-exit-fullscreen stays live for a fullscreen pane even when `enabled` is false (e.g. unfocused). */
  escapeEnabled?: boolean;
}

/**
 * Window keydown: Escape exits fullscreen, Shift+Arrow walks the review queue.
 * `enabled` gates both; `escapeEnabled` additionally keeps Escape alone live.
 */
export function useReviewNavKeys(
  enabled: boolean,
  { isFullscreen, setIsFullscreen, showNavigation, onNext, onPrevious, escapeEnabled = false }: ReviewNavHandlers,
): void {
  useEffect(() => {
    if (!enabled && !escapeEnabled) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isFullscreen) {
        setIsFullscreen(false);
      }

      if (enabled && showNavigation && e.shiftKey && !e.ctrlKey && !e.altKey && !e.metaKey) {
        if (e.key === "ArrowRight" && onNext) {
          e.preventDefault();
          onNext();
        } else if (e.key === "ArrowLeft" && onPrevious) {
          e.preventDefault();
          onPrevious();
        }
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [enabled, escapeEnabled, isFullscreen, setIsFullscreen, showNavigation, onNext, onPrevious]);
}
