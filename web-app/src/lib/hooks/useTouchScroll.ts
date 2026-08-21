import { RefObject, useEffect } from 'react';

// Terminal type - use a minimal interface to avoid tight coupling to xterm version
interface TerminalLike {
  scrollLines: (count: number) => void;
  options: { fontSize?: number };
}

export function useTouchScroll(
  containerRef: RefObject<HTMLElement | null>,
  getTerminal: () => TerminalLike | null
): void {
  useEffect(() => {
    // Skip on non-touch devices
    if (typeof window === 'undefined' || !('ontouchstart' in window)) {
      return;
    }

    const container = containerRef.current;
    if (!container) return;

    let touchStartY = 0;
    let touchStartX = 0;

    const onTouchStart = (event: TouchEvent) => {
      touchStartY = event.touches[0].clientY;
      touchStartX = event.touches[0].clientX;
    };

    const onTouchMove = (event: TouchEvent) => {
      const terminal = getTerminal();
      if (!terminal) return;

      const currentY = event.touches[0].clientY;
      const currentX = event.touches[0].clientX;
      const deltaY = touchStartY - currentY;
      const deltaX = touchStartX - currentX;

      // Only intercept primarily vertical swipes (vertical delta > horizontal + 10px threshold)
      if (Math.abs(deltaY) > Math.abs(deltaX) + 10) {
        // lineHeightPx: fontSize * 1.2 line-height approximation
        const fontSize = terminal.options.fontSize ?? 14;
        const lineHeightPx = fontSize * 1.2;
        const lineDelta = Math.round(deltaY / lineHeightPx);

        if (lineDelta !== 0) {
          terminal.scrollLines(lineDelta);
        }

        touchStartY = currentY;
        event.preventDefault();
      }
    };

    const onTouchEnd = () => {
      touchStartY = 0;
      touchStartX = 0;
    };

    container.addEventListener('touchstart', onTouchStart, { passive: true });
    container.addEventListener('touchmove', onTouchMove, { passive: false });
    container.addEventListener('touchend', onTouchEnd, { passive: true });

    return () => {
      container.removeEventListener('touchstart', onTouchStart);
      container.removeEventListener('touchmove', onTouchMove);
      container.removeEventListener('touchend', onTouchEnd);
    };
  }, [containerRef, getTerminal]);
}
