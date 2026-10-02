"use client";

/**
 * useTerminalGestures — Unified mobile gesture state machine for xterm.js terminals.
 *
 * Implements a gesture state machine (pure transition table in
 * lib/terminal/gestureMachine.ts; this hook wires listeners, timers, rAF and effects):
 *   IDLE → PENDING → SCROLLING | SELECTING | TAPPING → IDLE   (CANCELLED on interrupts)
 *
 * Scroll drags run through ScrollAccumulator and a per-frame routing decision
 * (xterm-local scrollLines, or PgUp/PgDn / wheel bytes to a TUI).
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
import { isMouseTracking as isMouseTrackingUtil, readScrollMode } from "@/lib/terminal/mouseTracking";
import { pointToCell, rafThrottlePoint, type CellGeometry } from "@/lib/terminal/touchDrag";
import { reduce, type GestureEffect, type GestureEvent, type GestureState } from "@/lib/terminal/gestureMachine";
import { clampLinesPerFrame, ScrollAccumulator, SLOP_PX } from "@/lib/terminal/scrollKinematics";
import {
  decideAndLogScrollTarget,
  encodeWheel,
  PageAccumulator,
  ROUTING_VERIFIED,
  type RouteDecisionLog,
  type ScrollMode,
  type ScrollOverride,
  type ScrollRoutingPolicy,
  type ScrollTarget,
  type TuiScrollPolicy,
} from "@/lib/terminal/scrollRouting";
import { mobileDebug } from "@/lib/terminal/mobileDebug";

// Re-export for consumers that import from this module
export { getCellDimensions };

/** Touches that begin on these (xterm's own scrollbar, anything opted out) never start a gesture. */
export const GESTURE_IGNORE_SELECTOR = '[data-gesture-ignore], .xterm-scrollable-element .scrollbar';

export interface GestureOptions {
  containerRef: RefObject<HTMLElement | null>;
  /** Pass the RefObject itself (not .current) so event handlers always see the live terminal instance. */
  terminalRef: RefObject<Terminal | null>;
  onSendData: (data: string) => void;
  /** Milliseconds of hold before a touch becomes a long-press selection. Default: 400ms. */
  longPressMs?: number;
  /** Scroll-target override (S6). Changing it mid-gesture cancels the gesture. Default 'auto'. */
  override?: ScrollOverride;
  /** When false the hook registers no touch listeners at all (TalkBack fallback). Default true. */
  gestureScrollEnabled?: boolean;
  /** How a TUI target is driven; 'wheel' is the only way to reach `tui-wheel`. Default: TUI_SCROLL_POLICY. */
  tuiScrollPolicy?: TuiScrollPolicy;
  /** Injected routing table (tests, spike results). Default: DEFAULT_ROUTING_POLICY. */
  routingPolicy?: ScrollRoutingPolicy;
  /** Bumped on reconnect / full-snapshot write; a change cancels the gesture. Default 0. */
  connectionEpoch?: number;
}

/**
 * Attaches a unified touch gesture recognizer to an xterm.js container.
 *
 * Returns a cleanup function (for use in useEffect return or manually).
 */
export function useTerminalGestures(options: GestureOptions): void {
  const { containerRef, terminalRef, gestureScrollEnabled = true, override = 'auto', connectionEpoch = 0 } = options;

  // Latest options for event handlers, so they never form stale closures.
  const optionsRef = useRef(options);
  useEffect(() => {
    optionsRef.current = options;
  });
  const cancelGestureRef = useRef<(() => void) | null>(null);

  // Override / connection changes cancel an in-flight gesture (S9: reset remainders, ignore the touch).
  const previousRef = useRef({ override, connectionEpoch });
  useEffect(() => {
    const prev = previousRef.current;
    if (prev.override !== override || prev.connectionEpoch !== connectionEpoch) {
      cancelGestureRef.current?.();
    }
    previousRef.current = { override, connectionEpoch };
  }, [override, connectionEpoch]);

  useEffect(() => {
    if (!gestureScrollEnabled) return;
    const containerEl = containerRef.current;
    if (!containerEl) return;

    const onSendData = (data: string) => optionsRef.current.onSendData(data);
    const longPressMs = () => optionsRef.current.longPressMs ?? 400;

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
    // Quantization + routing state for the current scroll gesture.
    const lineAcc = new ScrollAccumulator();
    const pageAcc = new PageAccumulator({
      rows: 24,
      now: () => Date.now(),
      // The per-fling cap guards momentum, not a finger that is still down (Story 1.2.2 owns it).
      flingCap: Number.POSITIVE_INFINITY,
    });
    let lastTarget: ScrollTarget | null = null;
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
    // Stops timers, frames and remainders; the machine decides the resulting state.
    const stopGesture = () => {
      clearLongPressTimer();
      cancelPendingFrames();
      lineAcc.reset();
      pageAcc.reset();
      lastTarget = null;
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
      if (!t) { state = 'IDLE'; stopGesture(); return; }

      clearLongPressTimer();

      // Haptic feedback if available (R4.3)
      navigator.vibrate?.(10);

      if (!isMouseTracking()) {
        beginSyntheticMouseSelection();
      } else if (t.element) {
        beginDirectSelectDrag(t, t.element);
      }
    };

    // ---- Effect executor: runs the machine's effects in order ----
    const runEffects = (effects: readonly GestureEffect[], e: TouchEvent | null, touch: Touch | undefined) => {
      for (const effect of effects) {
        switch (effect) {
          case 'abort':
            stopGesture();
            break;
          case 'startLongPressTimer':
            longPressTimer = setTimeout(() => {
              longPressTimer = null;
              dispatch({ type: 'longPress' }, null, undefined);
            }, longPressMs());
            break;
          case 'clearLongPressTimer':
            clearLongPressTimer();
            break;
          case 'beginScroll':
            beginScroll(touch);
            break;
          case 'continueScroll':
            scrollThrottled?.(0, touch?.clientY ?? lastY);
            break;
          case 'continueSelect':
            if (touch) selectThrottled?.(touch.clientX, touch.clientY);
            break;
          case 'preventDefault':
            preventDefaultIfCancelable(e);
            break;
          case 'enterSelecting':
            enterSelecting();
            break;
          case 'endSelecting':
            endSelecting(touch);
            break;
          case 'tap':
            handleTap();
            break;
        }
      }
    };

    const dispatch = (event: GestureEvent, e: TouchEvent | null, touch: Touch | undefined) => {
      const next = reduce(state, event);
      state = next.state;
      runEffects(next.effects, e, touch);
    };

    const preventDefaultIfCancelable = (e: TouchEvent | null) => {
      if (!e) return;
      if (e.cancelable) e.preventDefault();
      else mobileDebug.log('not-cancelable', { event: e.type });
    };

    // Re-evaluated every frame; a target change resets the remainders and logs the decision.
    const resolveTarget = (mode: ScrollMode): ScrollTarget => {
      const o = optionsRef.current;
      const logged: { entry?: RouteDecisionLog } = {};
      const target = decideAndLogScrollTarget(mode, {
        policy: o.routingPolicy,
        tuiPolicy: o.tuiScrollPolicy,
        override: o.override,
        log: (entry) => { logged.entry = entry; },
      });
      if (target !== lastTarget) {
        if (lastTarget !== null) {
          lineAcc.reset();
          pageAcc.reset();
        }
        lastTarget = target;
        mobileDebug.routeDecision({
          target,
          source: logged.entry?.source ?? 'auto',
          unverified: !ROUTING_VERIFIED,
        });
      }
      return target;
    };

    const dispatchScroll = (terminal: Terminal, target: ScrollTarget, lines: number) => {
      switch (target) {
        case 'xterm-local': {
          const clamped = clampLinesPerFrame(lines, terminal.rows);
          if (clamped !== 0) terminal.scrollLines(clamped);
          return;
        }
        case 'tui-pgkeys': {
          pageAcc.setRows(terminal.rows);
          const keys = pageAcc.push(lines);
          if (keys) onSendData(keys);
          return;
        }
        case 'tui-wheel': {
          const reports = encodeWheel(lines, {
            col: startCol + 1,
            row: startRow + 1,
            cols: terminal.cols,
            rows: terminal.rows,
          });
          if (reports) onSendData(reports);
          return;
        }
      }
    };

    const onScrollFrame = (_clientX: number, clientY: number) => {
      const terminal = terminalRef.current;
      if (!terminal || cachedCellH <= 0) return;
      const moveDy = clientY - lastY;
      lastY = clientY;
      const mode = readScrollMode(terminal);
      const target = resolveTarget(mode);
      const lines = lineAcc.push(-moveDy, cachedCellH);
      dispatchScroll(terminal, target, lines);
      const active = terminal.buffer?.active;
      mobileDebug.log('scroll', {
        ...mode,
        viewportY: active?.viewportY,
        baseY: active?.baseY,
        rows: terminal.rows,
        cellH: cachedCellH,
        moveDy,
        lines,
        target,
      });
    };

    const beginScroll = (touch: Touch | undefined) => {
      const y = touch?.clientY ?? lastY;
      lastY = y;
      lineAcc.reset();
      pageAcc.reset();
      lastTarget = null;
      // Seed with the overshoot past the slop only, so crossing it emits no jump.
      const travel = y - startY;
      const overshoot = Math.sign(travel) * (Math.abs(travel) - SLOP_PX);
      lineAcc.seed(-overshoot);
      mobileDebug.log('scroll-start', { startY, y, slopPx: SLOP_PX, overshoot });

      // Cache cell height once for the drag and coalesce touchmove into one
      // dispatch per frame — same rationale as the SELECTING geometry cache.
      const t = terminalRef.current;
      cachedCellH = t ? getCellDimensions(t).cellH : 0;
      [scrollThrottled, cancelScrollThrottle] = rafThrottlePoint(onScrollFrame);
      scrollThrottled(0, y); // one frame carries the seeded overshoot
    };

    const handleTap = () => {
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
          onSendData(press + release);
          t.focus();
        }
      }

      lastTapTime = now;
      lastTapX = tapX;
      lastTapY = tapY;
    };

    const endSelecting = (touch: Touch | undefined) => {
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
    };

    // ---- touchstart (registered on containerEl, passive: false) ----
    const startedOnIgnoredSurface = (e: TouchEvent): boolean => {
      const target = e.target as { closest?: (selector: string) => unknown } | null;
      return Boolean(target?.closest?.(GESTURE_IGNORE_SELECTOR));
    };

    const onTouchStart = (e: TouchEvent) => {
      if (startedOnIgnoredSurface(e)) {
        dispatch({ type: 'touchcancel' }, null, undefined);
        return;
      }
      if (e.touches.length === 1) {
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
      }
      dispatch({ type: 'touchstart', touchCount: e.touches.length }, e, undefined);
    };

    // ---- touchmove (registered on document, passive: false to allow preventDefault) ----
    const onTouchMove = (e: TouchEvent) => {
      const touch = e.touches[0];
      const absDy = touch ? Math.abs(touch.clientY - startY) : 0;
      dispatch({ type: 'touchmove', touchCount: e.touches.length, absDy, slopPx: SLOP_PX }, e, touch);
    };

    // ---- touchend (registered on document) ----
    const onTouchEnd = (e: TouchEvent) => {
      const touch = e.changedTouches[0];
      dispatch(
        {
          type: 'touchend',
          elapsedMs: Date.now() - startTime,
          totalDy: Math.abs((touch?.clientY ?? startY) - startY),
          longPressMs: longPressMs(),
          tapTolerancePx: SLOP_PX,
        },
        e,
        touch,
      );
    };

    // ---- touchcancel ----
    const onTouchCancel = () => {
      mobileDebug.log('touchcancel', { state });
      dispatch({ type: 'touchcancel' }, null, undefined);
    };

    // Keyboard toggle / rotation mid-gesture: drop remainders and ignore the rest of the touch.
    const onInterrupt = () => dispatch({ type: 'interrupt' }, null, undefined);

    // Register listeners:
    // - touchstart on containerEl (catches gesture origin)
    // - touchmove + touchend on document (handles drags outside container)
    // touchend is non-passive so it can preventDefault the synthesized click after a scroll.
    const visualViewport = window.visualViewport;
    containerEl.addEventListener('touchstart', onTouchStart, { passive: false });
    document.addEventListener('touchmove', onTouchMove, { passive: false });
    document.addEventListener('touchend', onTouchEnd, { passive: false });
    document.addEventListener('touchcancel', onTouchCancel, { passive: true });
    visualViewport?.addEventListener('resize', onInterrupt);
    window.addEventListener('orientationchange', onInterrupt);
    cancelGestureRef.current = onInterrupt;

    return () => {
      cancelGestureRef.current = null;
      clearLongPressTimer();
      cancelPendingFrames();
      containerEl.removeEventListener('touchstart', onTouchStart);
      document.removeEventListener('touchmove', onTouchMove);
      document.removeEventListener('touchend', onTouchEnd);
      document.removeEventListener('touchcancel', onTouchCancel);
      visualViewport?.removeEventListener('resize', onInterrupt);
      window.removeEventListener('orientationchange', onInterrupt);
    };
  }, [containerRef, terminalRef, gestureScrollEnabled]); // options other than these are read via optionsRef
}
