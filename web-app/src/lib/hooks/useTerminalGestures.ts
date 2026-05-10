"use client";

/**
 * useTerminalGestures — Unified mobile gesture state machine for xterm.js terminals.
 *
 * Implements a 5-state machine:
 *   IDLE → PENDING → SCROLLING | SELECTING | TAPPING → IDLE
 *
 * Replaces the conflicting useTouchScroll + useMobileTerminalGestures hooks (R4.3):
 * having both hooks register touchmove handlers on the same element caused double-scroll
 * and prevented selection during scroll.
 *
 * Architecture decision (ADR-012): TouchEvent preferred over PointerEvent because
 * PointerEvent fires pointercancel on iOS when a scroll gesture is detected, complicating
 * the long-press state machine. The existing codebase already uses TouchEvent exclusively.
 */

import { useEffect, useRef, RefObject } from "react";
import type { Terminal } from "@xterm/xterm";
import { getCellDimensions } from "@/lib/terminal/cellDimensions";

// Re-export for consumers that import from this module
export { getCellDimensions };

// ---- Gesture state machine types ----
type GestureState = 'IDLE' | 'PENDING' | 'SCROLLING' | 'SELECTING' | 'TAPPING';

interface UseTerminalGesturesOptions {
  containerRef: RefObject<HTMLElement | null>;
  /** Pass the RefObject itself (not .current) so event handlers always see the live terminal instance. */
  terminalRef: RefObject<Terminal | null>;
  onSendData: (data: string) => void;
  /** Milliseconds of hold before a touch becomes a long-press selection. Default: 400ms. */
  longPressMs?: number;
}

/**
 * Attaches a unified touch gesture recognizer to an xterm.js container.
 *
 * Returns a cleanup function (for use in useEffect return or manually).
 */
export function useTerminalGestures({
  containerRef,
  terminalRef,
  onSendData,
  longPressMs = 400,
}: UseTerminalGesturesOptions): void {
  // Keep stable refs so event handlers don't form stale closures
  const onSendDataRef = useRef(onSendData);
  const longPressMsRef = useRef(longPressMs);

  useEffect(() => {
    onSendDataRef.current = onSendData;
  }, [onSendData]);

  useEffect(() => {
    longPressMsRef.current = longPressMs;
  }, [longPressMs]);

  useEffect(() => {
    const containerEl = containerRef.current;
    if (!containerEl) return;

    // ---- State machine ----
    let state: GestureState = 'IDLE';

    // Touch tracking
    let startX = 0;
    let startY = 0;
    let lastY = 0;
    let startTime = 0;
    let startCol = 0;
    let startRow = 0;
    let tapX = 0;
    let tapY = 0;
    let longPressTimer: ReturnType<typeof setTimeout> | null = null;

    const clearLongPressTimer = () => {
      if (longPressTimer !== null) {
        clearTimeout(longPressTimer);
        longPressTimer = null;
      }
    };

    // Task 3.1.3 — Mouse-tracking-aware mode check.
    // Read runtime PTY-driven mode, not a prop/config value.
    const isMouseTracking = (): boolean => {
      const t = terminalRef.current;
      if (!t) return false;
      return (t.modes as any)?.mouseTrackingMode !== 'none' &&
             (t.modes as any)?.mouseTrackingMode !== undefined;
    };

    const getScreenEl = (): HTMLElement | null =>
      containerEl.querySelector('.xterm-screen') as HTMLElement | null;

    // ---- Transition helpers ----
    const transitionToIdle = () => {
      clearLongPressTimer();
      state = 'IDLE';
    };

    const enterSelecting = () => {
      const t = terminalRef.current;
      if (!t) { transitionToIdle(); return; }

      state = 'SELECTING';
      clearLongPressTimer();

      // Haptic feedback if available (R4.3)
      navigator.vibrate?.(10);

      if (!isMouseTracking()) {
        // Dispatch synthetic mousedown to .xterm-screen for native xterm.js selection
        getScreenEl()?.dispatchEvent(new MouseEvent('mousedown', {
          clientX: startX,
          clientY: startY,
          bubbles: true,
          cancelable: true,
          button: 0,
          buttons: 1,
        }));
      } else {
        // Use public terminal.select() API — bypasses mouse tracking mode
        t.select(startCol, startRow, 1);
      }
    };

    // ---- touchstart (registered on containerEl, passive: false) ----
    const onTouchStart = (e: TouchEvent) => {
      if (e.touches.length !== 1) {
        // Multi-touch: cancel any in-progress gesture
        transitionToIdle();
        return;
      }

      const touch = e.touches[0];
      startX = touch.clientX;
      startY = touch.clientY;
      lastY = touch.clientY;
      startTime = Date.now();
      tapX = touch.clientX;
      tapY = touch.clientY;

      // Calculate starting cell coordinates for selection/tap
      const t = terminalRef.current;
      if (t?.element) {
        const { cellH, cellW } = getCellDimensions(t);
        const rect = t.element.getBoundingClientRect();
        startCol = Math.max(0, Math.floor((startX - rect.left) / cellW));
        startRow = Math.max(0, Math.floor((startY - rect.top) / cellH));
      }

      state = 'PENDING';

      // Start long-press timer → SELECTING
      longPressTimer = setTimeout(() => {
        longPressTimer = null;
        if (state === 'PENDING') {
          enterSelecting();
        }
      }, longPressMsRef.current);
    };

    // ---- touchmove (registered on document, passive: false to allow preventDefault) ----
    const onTouchMove = (e: TouchEvent) => {
      if (e.touches.length !== 1) {
        transitionToIdle();
        return;
      }

      const touch = e.touches[0];
      const dy = touch.clientY - startY;
      const absDy = Math.abs(dy);

      if (state === 'PENDING') {
        if (absDy > 8) {
          // Moved enough to be a scroll — cancel long-press
          clearLongPressTimer();
          state = 'SCROLLING';
          lastY = touch.clientY;
        }
        // Stay in PENDING if movement is small
        return;
      }

      if (state === 'SCROLLING') {
        // Task 3.1.4 — per-event delta scroll with public cell height
        const t = terminalRef.current;
        if (t) {
          const moveDy = touch.clientY - lastY;
          lastY = touch.clientY;
          const { cellH } = getCellDimensions(t);
          const lines = Math.round(-moveDy / cellH);
          if (lines !== 0) t.scrollLines(lines);
        }
        e.preventDefault();
        return;
      }

      if (state === 'SELECTING') {
        // Task 3.1.5 — extend selection (both tracking modes)
        const t = terminalRef.current;
        if (!isMouseTracking()) {
          getScreenEl()?.dispatchEvent(new MouseEvent('mousemove', {
            clientX: touch.clientX,
            clientY: touch.clientY,
            bubbles: true,
            cancelable: true,
            button: 0,
            buttons: 1,
          }));
        } else if (t?.element) {
          const { cellH, cellW } = getCellDimensions(t);
          const rect = t.element.getBoundingClientRect();
          const currentCol = Math.max(0, Math.floor((touch.clientX - rect.left) / cellW));
          const currentRow = Math.max(0, Math.floor((touch.clientY - rect.top) / cellH));
          const rowDiff = currentRow - startRow;
          const colDiff = currentCol - startCol;
          const length = Math.max(1, rowDiff * t.cols + colDiff + 1);
          t.select(startCol, startRow, length);
        }
        e.preventDefault();
        return;
      }
    };

    // ---- touchend (registered on document) ----
    const onTouchEnd = (e: TouchEvent) => {
      const touch = e.changedTouches[0];
      const elapsed = Date.now() - startTime;
      const totalDy = Math.abs((touch?.clientY ?? startY) - startY);

      if (state === 'PENDING' && totalDy < 8 && elapsed < longPressMsRef.current) {
        // Short tap — transition to TAPPING and handle
        clearLongPressTimer();
        state = 'TAPPING';

        const t = terminalRef.current;
        if (t) {
          // Task 3.1.6 — TAPPING action
          if (!isMouseTracking()) {
            t.focus();
          } else if (t.element) {
            const { cellH, cellW } = getCellDimensions(t);
            const canvasRect = t.element.getBoundingClientRect();
            const col = Math.floor((tapX - canvasRect.left) / cellW) + 1; // 1-based
            const row = Math.floor((tapY - canvasRect.top) / cellH) + 1;   // 1-based
            // X10 mouse encoding: \x1b[M + button(32=left-press) + col+32 + row+32
            // Clamp col/row to 1-223 so charCode stays in 33-255 (valid X10 range)
            const clampedCol = Math.max(1, Math.min(col, 223));
            const clampedRow = Math.max(1, Math.min(row, 223));
            const press   = `\x1b[M${String.fromCharCode(32, clampedCol + 32, clampedRow + 32)}`;
            const release = `\x1b[M${String.fromCharCode(35, clampedCol + 32, clampedRow + 32)}`; // 35 = release
            onSendDataRef.current(press + release);
            t.focus();
          }
        }

        state = 'IDLE';
        return;
      }

      if (state === 'SELECTING') {
        const t = terminalRef.current;
        if (!isMouseTracking() && touch) {
          getScreenEl()?.dispatchEvent(new MouseEvent('mouseup', {
            clientX: touch.clientX,
            clientY: touch.clientY,
            bubbles: true,
            cancelable: true,
            button: 0,
            buttons: 0,
          }));
        }
        // Selection preserved in xterm's buffer — just transition back
      }

      transitionToIdle();
    };

    // ---- touchcancel ----
    const onTouchCancel = () => {
      transitionToIdle();
    };

    // Register listeners:
    // - touchstart on containerEl (catches gesture origin)
    // - touchmove + touchend on document (handles drags outside container)
    containerEl.addEventListener('touchstart', onTouchStart, { passive: false });
    document.addEventListener('touchmove', onTouchMove, { passive: false });
    document.addEventListener('touchend', onTouchEnd, { passive: true });
    document.addEventListener('touchcancel', onTouchCancel, { passive: true });

    return () => {
      clearLongPressTimer();
      containerEl.removeEventListener('touchstart', onTouchStart);
      document.removeEventListener('touchmove', onTouchMove);
      document.removeEventListener('touchend', onTouchEnd);
      document.removeEventListener('touchcancel', onTouchCancel);
    };
  }, [containerRef]); // Re-run only if containerRef changes (terminal/onSendData accessed via refs)
}
