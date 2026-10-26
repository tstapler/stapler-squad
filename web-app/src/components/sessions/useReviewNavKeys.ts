import { useEffect } from "react";

interface ReviewNavHandlers {
  isFullscreen: boolean;
  setIsFullscreen: (v: boolean) => void;
  showNavigation: boolean;
  onNext?: () => void;
  onPrevious?: () => void;
}

/** Window keydown: Escape exits fullscreen, Shift+Arrow walks the review queue. No-op when `enabled` is false. */
export function useReviewNavKeys(
  enabled: boolean,
  { isFullscreen, setIsFullscreen, showNavigation, onNext, onPrevious }: ReviewNavHandlers,
): void {
  useEffect(() => {
    if (!enabled) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isFullscreen) {
        setIsFullscreen(false);
      }

      if (showNavigation && e.shiftKey && !e.ctrlKey && !e.altKey && !e.metaKey) {
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
  }, [enabled, isFullscreen, setIsFullscreen, showNavigation, onNext, onPrevious]);
}
