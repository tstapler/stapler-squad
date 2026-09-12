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
import { isMouseTracking as isMouseTrackingUtil } from "@/lib/terminal/mouseTracking";
import { pointToCell, rafThrottlePoint, type CellGeometry } from "@/lib/terminal/touchDrag";

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

    // Double-tap detection
    let lastTapTime = 0;
    let lastTapX = 0;
    let lastTapY = 0;
    const DOUBLE_TAP_MS = 300;
    const DOUBLE_TAP_RADIUS_PX = 20;

    // Geometry + per-frame throttling for the current gesture. `getBoundingClientRect()`
    // and `getCellDimensions()` both force a layout read — measuring them on every raw
    // touchmove (Android fires these far more densely than iOS Safari coalesces them) is
    // what made scroll/select feel jumpy. Cache once per gesture instead, and coalesce
    // multiple touchmove events into a single update per animation frame.
    let cellGeometry: CellGeometry | null = null;
    let cachedCellH = 0;
    let scrollThrottled: ((clientX: number, clientY: number) => void) | null = null;
    let cancelScrollThrottle: (() => void) | null = null;
    let selectThrottled: ((clientX: number, clientY: number) => void) | null = null;
    let cancelSelectThrottle: (() => void) | null = null;

    const cancelPendingFrames = () => {
      cancelScrollThrottle?.();
      cancelSelectThrottle?.();
      scrollThrottled = null;
      cancelScrollThrottle = null;
      selectThrottled = null;
      cancelSelectThrottle = null;
    };

    const clearLongPressTimer = () => {
      if (longPressTimer !== null) {
        clearTimeout(longPressTimer);
        longPressTimer = null;
      }
    };

    // Mouse-tracking-aware mode check — delegates to shared utility.
    const isMouseTracking = (): boolean => {
      const t = terminalRef.current;
      if (!t) return false;
      return isMouseTrackingUtil(t);
    };

    const getScreenEl = (): HTMLElement | null =>
      containerEl.querySelector('.xterm-screen') as HTMLElement | null;

    // ---- Transition helpers ----
    const transitionToIdle = () => {
      clearLongPressTimer();
      cancelPendingFrames();
      state = 'IDLE';
    };

    // Non-mouse-tracking drag: xterm owns selection via real DOM mouse events. Coalesce
    // touchmove-driven mousemove dispatches to one per frame — xterm's own
    // SelectionService re-renders on every dispatched mousemove, which otherwise stacks
    // on top of our own per-event work.
    const beginSyntheticMouseSelection = () => {
      [selectThrottled, cancelSelectThrottle] = rafThrottlePoint((clientX, clientY) => {
        getScreenEl()?.dispatchEvent(new MouseEvent('mousemove', {
          clientX, clientY, bubbles: true, cancelable: true, button: 0, buttons: 1,
        }));
      });
      getScreenEl()?.dispatchEvent(new MouseEvent('mousedown', {
        clientX: startX, clientY: startY, bubbles: true, cancelable: true, button: 0, buttons: 1,
      }));
    };

    // Mouse-tracking-mode drag: bypass xterm's mouse handling and call the public
    // select() API directly. Cache rect/cell geometry once for the whole drag —
    // re-measuring on every touchmove is the main source of the jump-during-drag feel
    // on Android.
    const beginDirectSelectDrag = (t: Terminal, el: HTMLElement) => {
      const { cellH, cellW } = getCellDimensions(t);
      cellGeometry = { rect: el.getBoundingClientRect(), cellW, cellH, maxCol: t.cols - 1, maxRow: t.rows - 1 };
      [selectThrottled, cancelSelectThrottle] = rafThrottlePoint((clientX, clientY) => {
        if (!cellGeometry) return;
        const { col: currentCol, row: currentRow } = pointToCell(clientX, clientY, cellGeometry);
        const length = Math.max(1, (currentRow - startRow) * t.cols + (currentCol - startCol) + 1);
        t.select(startCol, startRow, length);
      });
      t.select(startCol, startRow, 1);
    };

    const enterSelecting = () => {
      const t = terminalRef.current;
      if (!t) { transitionToIdle(); return; }

      state = 'SELECTING';
      clearLongPressTimer();

      // Haptic feedback if available (R4.3)
      navigator.vibrate?.(10);

      if (!isMouseTracking()) {
        beginSyntheticMouseSelection();
      } else if (t.element) {
        beginDirectSelectDrag(t, t.element);
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
        if (absDy > 15) {
          // Moved enough to be a scroll — cancel long-press.
          // 15px threshold (was 8px): gives long-press timer room to fire even with
          // minor finger drift, preventing accidental scroll-instead-of-select.
          clearLongPressTimer();
          state = 'SCROLLING';
          lastY = touch.clientY;

          // Cache cell height once for the drag and coalesce touchmove into one
          // scrollLines() per frame — same rationale as the SELECTING geometry cache.
          const t = terminalRef.current;
          cachedCellH = t ? getCellDimensions(t).cellH : 0;
          [scrollThrottled, cancelScrollThrottle] = rafThrottlePoint((_clientX, clientY) => {
            const terminal = terminalRef.current;
            if (!terminal || cachedCellH <= 0) return;
            const moveDy = clientY - lastY;
            lastY = clientY;
            const lines = Math.round(-moveDy / cachedCellH);
            if (lines !== 0) terminal.scrollLines(lines);
          });
        }
        // Stay in PENDING if movement is small
        return;
      }

      if (state === 'SCROLLING') {
        scrollThrottled?.(0, touch.clientY);
        e.preventDefault();
        return;
      }

      if (state === 'SELECTING') {
        // Task 3.1.5 — extend selection (both tracking modes), coalesced to one
        // update per animation frame via the throttled callback set up in
        // beginSyntheticMouseSelection/beginDirectSelectDrag.
        selectThrottled?.(touch.clientX, touch.clientY);
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

        const now = Date.now();
        const isDoubleTap =
          (now - lastTapTime) < DOUBLE_TAP_MS &&
          Math.abs(tapX - lastTapX) < DOUBLE_TAP_RADIUS_PX &&
          Math.abs(tapY - lastTapY) < DOUBLE_TAP_RADIUS_PX;

        const t = terminalRef.current;
        if (isDoubleTap && !isMouseTracking()) {
          // Dispatch synthetic dblclick to trigger xterm's native word selection
          getScreenEl()?.dispatchEvent(new MouseEvent('dblclick', {
            clientX: tapX,
            clientY: tapY,
            bubbles: true,
            cancelable: true,
            button: 0,
            buttons: 1,
            detail: 2,
          }));
        } else if (t) {
          // Normal tap handling
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

        lastTapTime = now;
        lastTapX = tapX;
        lastTapY = tapY;

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
          // xterm.js's platform check for "Linux" matches Android's user agent too, so
          // completing a real mouse-event-driven selection makes it focus+select the
          // hidden input textarea (to populate the X11 primary-selection clipboard for a
          // desktop middle-click paste) — that focus() call is what pops the Android soft
          // keyboard mid text-selection. Desktop Linux wants that; touch doesn't. The
          // focus already happened synchronously inside dispatchEvent above, so blur
          // it back immediately.
          t?.textarea?.blur();
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
      cancelPendingFrames();
      containerEl.removeEventListener('touchstart', onTouchStart);
      document.removeEventListener('touchmove', onTouchMove);
      document.removeEventListener('touchend', onTouchEnd);
      document.removeEventListener('touchcancel', onTouchCancel);
    };
  }, [containerRef]); // Re-run only if containerRef changes (terminal/onSendData accessed via refs)
}
