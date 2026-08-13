"use client";

import { useEffect, useRef } from "react";
import type { Terminal } from "@xterm/xterm";

interface UseMobileTerminalGesturesOptions {
  /** Ref to the terminal container element. */
  containerRef: React.RefObject<HTMLElement | null>;
  /** Returns the current xterm Terminal instance (may be null during init). */
  getTerminal: () => Terminal | null;
  /** Returns the current mouseTracking value without needing the terminal to re-init. */
  getMouseTracking: () => string;
  /** Font size in px — used as fallback cell height when private API is unavailable. */
  fontSize: number;
  /**
   * Milliseconds of hold before a touch turns into a selection drag.
   * 400ms is tight but consistent with Termux.
   */
  longPressMs?: number;
}

/**
 * Attaches mobile touch gesture handlers to an xterm.js container:
 *   - Short swipe  → scroll the terminal (translates dy into scrollLines)
 *   - Long press + drag → text selection (dispatches synthetic mouse events to .xterm-screen)
 *
 * Selection only works when mouseTracking is "none"; when mouse reporting is on the terminal
 * processes events itself via VT sequences.
 *
 * Extracted from XtermTerminal so the gesture logic can be tested and reused independently.
 */
export function useMobileTerminalGestures({
  containerRef,
  getTerminal,
  getMouseTracking,
  fontSize,
  longPressMs = 400,
}: UseMobileTerminalGesturesOptions): void {
  const longPressMsRef = useRef(longPressMs);
  const fontSizeRef = useRef(fontSize);

  useEffect(() => {
    longPressMsRef.current = longPressMs;
    fontSizeRef.current = fontSize;
  }, [longPressMs, fontSize]);

  useEffect(() => {
    const containerEl = containerRef.current;
    if (!containerEl) return;

    const touchState = {
      startX: 0,
      startY: 0,
      lastY: 0,
      isSelecting: false,
      selectionTimer: null as ReturnType<typeof setTimeout> | null,
    };

    const clearSelectionTimer = () => {
      if (touchState.selectionTimer) {
        clearTimeout(touchState.selectionTimer);
        touchState.selectionTimer = null;
      }
    };

    const getScreenEl = () =>
      containerEl.querySelector('.xterm-screen') as HTMLElement | null;

    const onTouchStart = (e: TouchEvent) => {
      if (e.touches.length !== 1) return;
      const t = e.touches[0];
      // Capture primitives immediately — Touch objects are event-scoped and
      // may be recycled/invalid after the handler returns (especially in Safari).
      const startX = t.clientX;
      const startY = t.clientY;
      touchState.startX = startX;
      touchState.startY = startY;
      touchState.lastY = startY;
      touchState.isSelecting = false;
      clearSelectionTimer();

      touchState.selectionTimer = setTimeout(() => {
        touchState.selectionTimer = null;
        if (getMouseTracking() !== 'none') return;
        touchState.isSelecting = true;
        getScreenEl()?.dispatchEvent(new MouseEvent('mousedown', {
          clientX: startX, clientY: startY,
          bubbles: true, cancelable: true, button: 0, buttons: 1,
        }));
      }, longPressMsRef.current);
    };

    const onTouchMove = (e: TouchEvent) => {
      if (e.touches.length !== 1) return;
      const t = e.touches[0];
      const dy = t.clientY - touchState.lastY;
      const totalDy = Math.abs(t.clientY - touchState.startY);
      touchState.lastY = t.clientY;

      if (touchState.isSelecting) {
        getScreenEl()?.dispatchEvent(new MouseEvent('mousemove', {
          clientX: t.clientX, clientY: t.clientY,
          bubbles: true, cancelable: true, button: 0, buttons: 1,
        }));
        e.preventDefault();
        return;
      }

      if (totalDy > 8) {
        clearSelectionTimer();
        const terminal = getTerminal();
        if (!terminal) return;
        const cellH =
          (terminal as any)._core?._renderService?.dimensions?.css?.cell?.height ??
          fontSizeRef.current;
        const lines = Math.round(-dy / cellH);
        if (lines !== 0) terminal.scrollLines(lines);
        e.preventDefault();
      }
    };

    const onTouchEnd = (e: TouchEvent) => {
      clearSelectionTimer();
      if (touchState.isSelecting) {
        const t = e.changedTouches[0];
        getScreenEl()?.dispatchEvent(new MouseEvent('mouseup', {
          clientX: t.clientX, clientY: t.clientY,
          bubbles: true, cancelable: true, button: 0, buttons: 0,
        }));
        touchState.isSelecting = false;
      }
    };

    containerEl.addEventListener('touchstart', onTouchStart, { passive: true });
    containerEl.addEventListener('touchmove', onTouchMove, { passive: false });
    containerEl.addEventListener('touchend', onTouchEnd, { passive: true });

    return () => {
      clearSelectionTimer();
      containerEl.removeEventListener('touchstart', onTouchStart);
      containerEl.removeEventListener('touchmove', onTouchMove);
      containerEl.removeEventListener('touchend', onTouchEnd);
    };
  }, [containerRef, getTerminal, getMouseTracking]);
}
