/**
 * Tests for useTerminalGestures — 5-state gesture machine.
 *
 * Covers: IDLE→PENDING, PENDING→TAPPING, PENDING→SCROLLING,
 * PENDING→SELECTING (long-press), X10 encoding, and mode guard.
 */

import { renderHook } from '@testing-library/react';
import { RefObject } from 'react';

// Mock cellDimensions before importing the hook so the module resolves cleanly.
jest.mock('@/lib/terminal/cellDimensions', () => ({
  getCellDimensions: jest.fn().mockReturnValue({ cellH: 20, cellW: 10 }),
}));

import { readFileSync, readdirSync, statSync } from 'fs';
import { join, relative } from 'path';
import { getCellDimensions } from '@/lib/terminal/cellDimensions';
import { SLOP_PX } from '@/lib/terminal/scrollKinematics';
import { useTerminalGestures } from '../useTerminalGestures';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a fake container element whose addEventListener/removeEventListener
 *  are jest spies. The spies delegate to a real event-handler map so that
 *  tests can fire events by calling the stored handler directly. */
function makeFakeContainer() {
  const handlers: Record<string, EventListenerOrEventListenerObject> = {};

  const el = {
    addEventListener: jest.fn((type: string, listener: EventListenerOrEventListenerObject, _options?: unknown) => {
      handlers[type] = listener;
    }),
    removeEventListener: jest.fn((type: string) => {
      delete handlers[type];
    }),
    querySelector: jest.fn(() => null), // .xterm-screen not needed for most tests
  } as unknown as HTMLElement;

  const fire = (type: string, event: Event) => {
    const h = handlers[type];
    if (h) {
      if (typeof h === 'function') h(event);
      else h.handleEvent(event);
    }
  };

  return { el, handlers, fire };
}

/** Build a TouchEvent-like object with one touch point. */
function makeTouchEvent(
  type: string,
  clientX: number,
  clientY: number,
  touchListKey: 'touches' | 'changedTouches' = 'touches',
  opts: { cancelable?: boolean; target?: unknown } = {},
): TouchEvent {
  const touch = { clientX, clientY } as Touch;
  const event: Partial<TouchEvent> = {
    type,
    touches: touchListKey === 'touches' ? [touch] as unknown as TouchList : [] as unknown as TouchList,
    changedTouches: [touch] as unknown as TouchList,
    preventDefault: jest.fn(),
    cancelable: opts.cancelable ?? true,
    target: (opts.target ?? null) as EventTarget | null,
  };
  return event as TouchEvent;
}

/** Build a fake terminal ref with controllable mouseTrackingMode. */
function makeTerminalRef(mouseTrackingMode: string = 'none') {
  const terminal = {
    modes: { mouseTrackingMode },
    focus: jest.fn(),
    select: jest.fn(),
    getSelection: jest.fn().mockReturnValue(''),
    scrollLines: jest.fn(),
    element: {
      getBoundingClientRect: jest.fn().mockReturnValue({ left: 0, top: 0 }),
      clientHeight: 480,
      clientWidth: 800,
      clientLeft: 0,
      clientTop: 0,
    } as unknown as HTMLElement,
    rows: 24,
    cols: 80,
    options: { fontSize: 14, lineHeight: 1 },
  };
  return { current: terminal } as RefObject<typeof terminal>;
}

// ---------------------------------------------------------------------------
// Document-level touch spy setup
// ---------------------------------------------------------------------------

// Store document handlers so tests can fire touchmove / touchend.
const docHandlers: Record<string, ((e: TouchEvent) => void)> = {};
const origDocAdd = document.addEventListener.bind(document);
const origDocRemove = document.removeEventListener.bind(document);

beforeAll(() => {
  jest.spyOn(document, 'addEventListener').mockImplementation((type, listener) => {
    if (type === 'touchmove' || type === 'touchend' || type === 'touchcancel') {
      docHandlers[type] = listener as (e: TouchEvent) => void;
    } else {
      origDocAdd(type, listener);
    }
  });
  jest.spyOn(document, 'removeEventListener').mockImplementation((type, listener) => {
    if (type === 'touchmove' || type === 'touchend' || type === 'touchcancel') {
      delete docHandlers[type];
    } else {
      origDocRemove(type, listener);
    }
  });
});

afterAll(() => {
  jest.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Suites
// ---------------------------------------------------------------------------

describe('useTerminalGestures', () => {
  let fakeContainer: ReturnType<typeof makeFakeContainer>;
  let containerRef: RefObject<HTMLElement | null>;
  let onSendData: jest.Mock;

  beforeEach(() => {
    jest.useFakeTimers();
    fakeContainer = makeFakeContainer();
    containerRef = { current: fakeContainer.el } as RefObject<HTMLElement | null>;
    onSendData = jest.fn();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
    // Clear doc handlers between tests
    Object.keys(docHandlers).forEach((k) => delete docHandlers[k]);
  });

  // Helper: mount the hook and fire touchstart
  function mount(mouseTrackingMode = 'none', longPressMs = 400) {
    const terminalRef = makeTerminalRef(mouseTrackingMode);
    renderHook(() =>
      useTerminalGestures({ containerRef, terminalRef: terminalRef as any, onSendData, longPressMs }),
    );
    return { terminalRef };
  }

  function fireTouchStart(x = 100, y = 100) {
    fakeContainer.fire('touchstart', makeTouchEvent('touchstart', x, y));
  }

  function fireTouchMove(y: number, x = 100) {
    if (docHandlers['touchmove']) {
      docHandlers['touchmove'](makeTouchEvent('touchmove', x, y));
    }
  }

  function fireTouchEnd(x = 100, y = 100) {
    if (docHandlers['touchend']) {
      docHandlers['touchend'](makeTouchEvent('touchend', x, y, 'changedTouches'));
    }
  }

  // -------------------------------------------------------------------------
  // Test 1 — IDLE → PENDING on touchstart
  // -------------------------------------------------------------------------
  describe('IDLE → PENDING on touchstart', () => {
    it('should register touchstart on container element', () => {
      mount();
      expect(fakeContainer.el.addEventListener).toHaveBeenCalledWith(
        'touchstart',
        expect.any(Function),
        expect.any(Object),
      );
    });

    it('should NOT call onSendData immediately after touchstart', () => {
      mount();
      fireTouchStart();
      expect(onSendData).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // Test 2 — PENDING → TAPPING on quick touchend (mouse tracking enabled)
  // -------------------------------------------------------------------------
  describe('PENDING → TAPPING on quick touchend with mouse tracking', () => {
    it('should call onSendData with X10 sequence on quick tap in vt200 mode', () => {
      mount('vt200');

      fireTouchStart(50, 60);
      // Advance time to well under longPressMs (400ms)
      jest.advanceTimersByTime(100);
      fireTouchEnd(50, 60);

      expect(onSendData).toHaveBeenCalledTimes(1);
      const arg: string = onSendData.mock.calls[0][0];
      // Must start with X10 escape prefix
      expect(arg).toMatch(/^\x1b\[M/);
      // Press + release = 2 sequences, each 6 chars total
      expect(arg.length).toBe(12); // 2 × "\x1b[M" + 3 chars
    });

    it('should NOT call onSendData on quick tap when tracking mode is none', () => {
      mount('none');

      fireTouchStart(50, 60);
      jest.advanceTimersByTime(100);
      fireTouchEnd(50, 60);

      // In none mode the tap just calls t.focus(), not onSendData
      expect(onSendData).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // Test 3 — PENDING → SCROLLING on large touchmove
  // -------------------------------------------------------------------------
  describe('PENDING → SCROLLING on touchmove with large delta', () => {
    it('should NOT call onSendData when dragging > 10px vertically', () => {
      mount();

      fireTouchStart(100, 100);
      // Move 50px — exceeds 8px threshold
      fireTouchMove(50, 100); // dy = -50 from startY 100
      fireTouchEnd(100, 50);

      expect(onSendData).not.toHaveBeenCalled();
    });

    it('should call terminal.scrollLines when in SCROLLING state and moved enough', () => {
      const { terminalRef } = mount();

      fireTouchStart(100, 100);
      // First move > 8px to enter SCROLLING
      fireTouchMove(80, 100);
      // Second move while SCROLLING — scrollLines is rAF-coalesced (rafThrottlePoint),
      // so it only fires once the fake animation frame is flushed.
      fireTouchMove(40, 100);
      jest.advanceTimersByTime(16);

      expect((terminalRef.current as any).scrollLines).toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // Test 4 — PENDING → SELECTING on long press
  // -------------------------------------------------------------------------
  describe('PENDING → SELECTING on long press', () => {
    it('should NOT call onSendData during long press transition', () => {
      mount();

      fireTouchStart(100, 100);
      // Advance past the long-press threshold
      jest.advanceTimersByTime(450);
      // No touchend yet — we're in SELECTING

      expect(onSendData).not.toHaveBeenCalled();
    });

    it('should fire touchend after long press without sending mouse sequence', () => {
      mount('none');

      fireTouchStart(100, 100);
      jest.advanceTimersByTime(450);
      fireTouchEnd(100, 100);

      // SELECTING → IDLE, not TAPPING, so no onSendData
      expect(onSendData).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // Test 5 — X10 encoding correctness
  // -------------------------------------------------------------------------
  describe('X10 mouse encoding', () => {
    it('should encode col/row as 1-based with +32 offset in X10 format', () => {
      // Terminal element at top-left (0,0), cell size = 10×20 (from mock)
      // Touch at (50, 60): col = floor(50/10)+1 = 6, row = floor(60/20)+1 = 4
      mount('vt200');

      fireTouchStart(50, 60);
      jest.advanceTimersByTime(50);
      fireTouchEnd(50, 60);

      expect(onSendData).toHaveBeenCalledTimes(1);
      const seq: string = onSendData.mock.calls[0][0];

      // Press sequence: \x1b[M + chr(32) + chr(col+32) + chr(row+32)
      const press = seq.slice(0, 6);
      expect(press[3]).toBe(String.fromCharCode(32));  // button = left press
      // col=6 → 6+32=38, row=4 → 4+32=36
      expect(press[4]).toBe(String.fromCharCode(38));
      expect(press[5]).toBe(String.fromCharCode(36));

      // Release sequence button = 35
      const release = seq.slice(6, 12);
      expect(release[3]).toBe(String.fromCharCode(35));
    });

    it('should clamp col/row to 1-223 range', () => {
      // Touch at a very large coordinate
      mount('vt200');

      fireTouchStart(3000, 3000);
      jest.advanceTimersByTime(50);
      fireTouchEnd(3000, 3000);

      expect(onSendData).toHaveBeenCalledTimes(1);
      const seq: string = onSendData.mock.calls[0][0];
      const col = seq.charCodeAt(4) - 32;
      const row = seq.charCodeAt(5) - 32;
      expect(col).toBeLessThanOrEqual(223);
      expect(row).toBeLessThanOrEqual(223);
    });
  });

  // -------------------------------------------------------------------------
  // Test 6 — No X10 when tracking mode is none
  // -------------------------------------------------------------------------
  describe('no X10 sequence when tracking mode is none', () => {
    it('should not emit \\x1b[M prefix when mouseTrackingMode is none', () => {
      mount('none');

      fireTouchStart(100, 100);
      jest.advanceTimersByTime(50);
      fireTouchEnd(100, 100);

      const calls = onSendData.mock.calls.filter((c: string[]) =>
        c[0]?.startsWith('\x1b[M'),
      );
      expect(calls.length).toBe(0);
    });
  });

  // -------------------------------------------------------------------------
  // Task 1.2.4a — characterization anchors (current, unmodified hook behavior)
  // -------------------------------------------------------------------------
  describe('characterization anchors (Task 1.2.4a)', () => {
    /** Give the container a fake .xterm-screen so dispatched synthetic mouse events are observable. */
    function withScreen() {
      const screen = { dispatchEvent: jest.fn() };
      (fakeContainer.el.querySelector as jest.Mock).mockReturnValue(screen);
      return screen;
    }
    const dispatchedTypes = (screen: { dispatchEvent: jest.Mock }) =>
      screen.dispatchEvent.mock.calls.map((c) => (c[0] as Event).type);

    function touchEventWith(type: string, points: Array<[number, number]>, key: 'touches' | 'changedTouches' = 'touches') {
      const list = points.map(([clientX, clientY]) => ({ clientX, clientY })) as unknown as TouchList;
      return {
        type,
        touches: key === 'touches' ? list : ([] as unknown as TouchList),
        changedTouches: list,
        preventDefault: jest.fn(),
      } as unknown as TouchEvent;
    }

    /** Fire a touchmove and return the event so preventDefault can be inspected. */
    function move(x: number, y: number) {
      const ev = makeTouchEvent('touchmove', x, y);
      docHandlers['touchmove']?.(ev);
      return ev;
    }
    const cancel = () => docHandlers['touchcancel']?.({} as TouchEvent);
    const flushFrame = () => jest.advanceTimersByTime(16);

    // ---- tap ----
    it('touchend_should_RunTapPathAndFocusOnce_When_TotalDy5And100ms', () => {
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      jest.advanceTimersByTime(100);
      fireTouchEnd(100, 105);
      expect(terminalRef.current!.focus).toHaveBeenCalledTimes(1);
      expect(onSendData).not.toHaveBeenCalled();
    });

    it('touchend_should_Tap_When_MovedUnderSlopAndReleasedBefore400ms (7px, below even the old 8px tolerance)', () => {
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      move(100, 107); // 7px: stays PENDING (below the 15px scroll threshold)
      jest.advanceTimersByTime(399);
      fireTouchEnd(100, 107);
      expect(terminalRef.current!.focus).toHaveBeenCalledTimes(1);
    });

    // Deliberate change (plan Slop rule): tap tolerance widened from 8px to SLOP_PX, so there is no dead zone.
    it.each([8, 12, 14])('touchend_should_Tap_When_MovedUnderSlop_NoDeadZone (%ipx)', (dy) => {
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      move(100, 100 + dy);
      jest.advanceTimersByTime(100);
      fireTouchEnd(100, 100 + dy);
      expect(terminalRef.current!.focus).toHaveBeenCalledTimes(1);
    });

    it('touchend_should_NotTap_When_TotalDyIsSlopOrMore (boundary is SLOP_PX, not 8px)', () => {
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      jest.advanceTimersByTime(100);
      fireTouchEnd(100, 100 + SLOP_PX);
      expect(terminalRef.current!.focus).not.toHaveBeenCalled();
    });

    it('touchend_should_Tap_When_OnlyHorizontalDrift (current: dx is not counted)', () => {
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      move(160, 100);
      jest.advanceTimersByTime(100);
      fireTouchEnd(160, 100);
      expect(terminalRef.current!.focus).toHaveBeenCalledTimes(1);
    });

    it('touchend_should_FocusAndNotClearSelection_When_TapWhileSelectionActive (current: no clear-selection branch)', () => {
      const { terminalRef } = mount('none');
      (terminalRef.current!.getSelection as jest.Mock).mockReturnValue('selected text');
      (terminalRef.current as any).clearSelection = jest.fn();
      fireTouchStart(100, 100);
      jest.advanceTimersByTime(100);
      fireTouchEnd(100, 100);
      expect(terminalRef.current!.focus).toHaveBeenCalledTimes(1);
      expect((terminalRef.current as any).clearSelection).not.toHaveBeenCalled();
    });

    // ---- long-press selecting ----
    it('touchstart_should_EnterSelecting_When_Stationary400ms', () => {
      const screen = withScreen();
      mount('none');
      fireTouchStart(100, 100);
      jest.advanceTimersByTime(399);
      expect(dispatchedTypes(screen)).toEqual([]);
      jest.advanceTimersByTime(1);
      expect(dispatchedTypes(screen)).toEqual(['mousedown']);
    });

    it('touchstart_should_EnterSelecting_When_Stationary400ms_InMouseTrackingMode (direct select)', () => {
      const { terminalRef } = mount('vt200');
      fireTouchStart(50, 60); // col 5, row 3 with 10x20 cells
      jest.advanceTimersByTime(400);
      expect(terminalRef.current!.select).toHaveBeenCalledWith(5, 3, 1);
    });

    it('selecting_should_NotScroll_When_DragInSelectionMode', () => {
      const screen = withScreen();
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      jest.advanceTimersByTime(400);
      const ev = move(100, 200);
      flushFrame();
      expect(terminalRef.current!.scrollLines).not.toHaveBeenCalled();
      expect(ev.preventDefault).toHaveBeenCalled();
      expect(dispatchedTypes(screen)).toEqual(['mousedown', 'mousemove']);
    });

    it('selecting_should_DispatchMouseUpAndReturnToIdle_When_TouchEnd', () => {
      const screen = withScreen();
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      jest.advanceTimersByTime(400);
      fireTouchEnd(100, 100);
      expect(dispatchedTypes(screen)).toEqual(['mousedown', 'mouseup']);
      expect(terminalRef.current!.focus).not.toHaveBeenCalled();
      // Back in IDLE: a further move is ignored (no preventDefault)
      expect(move(100, 300).preventDefault).not.toHaveBeenCalled();
    });

    // ---- double-tap ----
    it('doubleTap_should_SelectWord_When_TwoTapsWithin300msAnd20px', () => {
      const screen = withScreen();
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      jest.advanceTimersByTime(50);
      fireTouchEnd(100, 100);
      jest.advanceTimersByTime(100);
      fireTouchStart(110, 105);
      jest.advanceTimersByTime(50);
      fireTouchEnd(110, 105);
      expect(dispatchedTypes(screen)).toEqual(['dblclick']);
      expect(terminalRef.current!.focus).toHaveBeenCalledTimes(1); // first tap only
    });

    it('doubleTap_should_NotSelectWord_When_SecondTapAfter300ms', () => {
      const screen = withScreen();
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      fireTouchEnd(100, 100);
      jest.advanceTimersByTime(301);
      fireTouchStart(100, 100);
      fireTouchEnd(100, 100);
      expect(dispatchedTypes(screen)).toEqual([]);
      expect(terminalRef.current!.focus).toHaveBeenCalledTimes(2);
    });

    it('doubleTap_should_NotSelectWord_When_SecondTapBeyond20px', () => {
      const screen = withScreen();
      mount('none');
      fireTouchStart(100, 100);
      fireTouchEnd(100, 100);
      jest.advanceTimersByTime(100);
      fireTouchStart(125, 100);
      fireTouchEnd(125, 100);
      expect(dispatchedTypes(screen)).toEqual([]);
    });

    it('doubleTap_should_SendTwoX10Taps_When_MouseTrackingActive (no dblclick)', () => {
      const screen = withScreen();
      mount('vt200');
      fireTouchStart(100, 100);
      fireTouchEnd(100, 100);
      jest.advanceTimersByTime(100);
      fireTouchStart(100, 100);
      fireTouchEnd(100, 100);
      expect(dispatchedTypes(screen)).toEqual([]);
      expect(onSendData).toHaveBeenCalledTimes(2);
    });

    // ---- multi-touch ----
    it('touchstart_should_CancelGesture_When_TwoTouches', () => {
      const screen = withScreen();
      mount('none');
      fireTouchStart(100, 100);
      fakeContainer.fire('touchstart', touchEventWith('touchstart', [[100, 100], [200, 200]]));
      jest.advanceTimersByTime(500);
      expect(dispatchedTypes(screen)).toEqual([]); // long-press timer cleared
      fireTouchEnd(100, 100);
      expect(onSendData).not.toHaveBeenCalled();
    });

    it('touchmove_should_CancelScroll_When_SecondFingerLandsMidScroll', () => {
      const { terminalRef } = mount();
      fireTouchStart(100, 100);
      move(100, 60); // -> SCROLLING
      docHandlers['touchmove'](touchEventWith('touchmove', [[100, 40], [200, 40]]));
      flushFrame();
      expect(terminalRef.current!.scrollLines).not.toHaveBeenCalled();
      expect(move(100, 20).preventDefault).not.toHaveBeenCalled(); // IDLE now
    });

    // ---- touchcancel ----
    it('touchcancel_should_ResetStateAndNotScroll_When_ScrollingTouchCancelled', () => {
      const { terminalRef } = mount();
      fireTouchStart(100, 100);
      move(100, 60); // -> SCROLLING
      move(100, 20); // frame pending
      cancel();
      flushFrame();
      expect(terminalRef.current!.scrollLines).not.toHaveBeenCalled(); // pending frame cancelled
      expect(move(100, 0).preventDefault).not.toHaveBeenCalled();
    });

    it('touchcancel_should_PreventLongPressSelect_When_PendingTouchCancelled', () => {
      const screen = withScreen();
      mount('none');
      fireTouchStart(100, 100);
      cancel();
      jest.advanceTimersByTime(500);
      expect(dispatchedTypes(screen)).toEqual([]);
    });

    it('touchcancel_should_ExitSelecting_When_SelectingTouchCancelled', () => {
      withScreen();
      mount('none');
      fireTouchStart(100, 100);
      jest.advanceTimersByTime(400);
      cancel();
      expect(move(100, 200).preventDefault).not.toHaveBeenCalled();
    });

    // ---- scroll / move semantics ----
    it('touchmove_should_NotScrollOrPreventDefault_When_HorizontalFirstPastSlop', () => {
      const { terminalRef } = mount();
      fireTouchStart(100, 100);
      const ev = move(200, 103); // large dx, tiny dy
      flushFrame();
      expect(ev.preventDefault).not.toHaveBeenCalled();
      expect(terminalRef.current!.scrollLines).not.toHaveBeenCalled();
    });

    // Deliberate change (Task 1.2.1c): the first move past the slop now preventDefaults too.
    it('touchmove_should_PreventDefaultOnEveryMovePastSlop_When_Cancelable', () => {
      mount();
      fireTouchStart(100, 100);
      expect(move(100, 70).preventDefault).toHaveBeenCalled(); // PENDING -> SCROLLING
      expect(move(100, 40).preventDefault).toHaveBeenCalled(); // SCROLLING
    });

    it('scroll_should_ScrollLinesByWholeCellDelta_And_NotFocusOnTouchEnd (lastY-relative; 5px slop overshoot carried)', () => {
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      move(100, 80); // enters SCROLLING, lastY = 80, no scroll yet
      move(100, 40); // 40px up with 20px cells -> +2 lines
      flushFrame();
      expect(terminalRef.current!.scrollLines).toHaveBeenCalledWith(2);
      fireTouchEnd(100, 40);
      expect(terminalRef.current!.focus).not.toHaveBeenCalled();
    });
  });

  // -------------------------------------------------------------------------
  // Story 1.2.1 / Task 0.1.1b — accumulator, routing, hardening, instrumentation
  // -------------------------------------------------------------------------
  describe('scroll routing and hardening (Story 1.2.1)', () => {
    type HookOpts = Partial<Parameters<typeof useTerminalGestures>[0]>;
    const START_Y = 100;
    const PGUP = '\x1b[5~';
    const PGDN = '\x1b[6~';

    function setCellH(cellH: number) {
      (getCellDimensions as jest.Mock).mockReturnValue({ cellH, cellW: 10 });
    }

    function mountScroll(
      o: { bufferType?: 'normal' | 'alternate'; tracking?: string; cellH?: number; rows?: number } = {},
      hookOpts: HookOpts = {},
    ) {
      setCellH(o.cellH ?? 20);
      const terminalRef = makeTerminalRef(o.tracking ?? 'none') as any;
      terminalRef.current.buffer = { active: { type: o.bufferType ?? 'normal', viewportY: 3, baseY: 7 } };
      if (o.rows) terminalRef.current.rows = o.rows;
      const view = renderHook((props: HookOpts) =>
        useTerminalGestures({ containerRef, terminalRef, onSendData, ...props }), { initialProps: hookOpts });
      return { terminalRef, term: terminalRef.current, ...view };
    }

    const start = (y = START_Y, x = 100) => fireTouchStart(x, y);
    const mv = (y: number, opts: { cancelable?: boolean; x?: number } = {}) => {
      const ev = makeTouchEvent('touchmove', opts.x ?? 100, y, 'touches', { cancelable: opts.cancelable });
      docHandlers['touchmove']?.(ev);
      return ev;
    };
    const frame = () => jest.advanceTimersByTime(16);
    const end = (y: number, opts: { cancelable?: boolean } = {}) => {
      const ev = makeTouchEvent('touchend', 100, y, 'changedTouches', { cancelable: opts.cancelable });
      docHandlers['touchend']?.(ev);
      return ev;
    };

    function installFakeVisualViewport() {
      const listeners: Record<string, Set<() => void>> = {};
      const vv = {
        addEventListener: jest.fn((t: string, l: () => void) => { (listeners[t] ??= new Set()).add(l); }),
        removeEventListener: jest.fn((t: string, l: () => void) => { listeners[t]?.delete(l); }),
      };
      Object.defineProperty(window, 'visualViewport', { value: vv, configurable: true });
      return { vv, fire: (t: string) => listeners[t]?.forEach((l) => l()), count: (t: string) => listeners[t]?.size ?? 0 };
    }

    afterEach(() => {
      setCellH(20);
      localStorage.removeItem('debug-terminal-mobile');
      delete (window as any).visualViewport;
    });

    // ---- Group A: both directions, seed, local dispatch (Task 1.2.1a1) ----
    it('scrollDrag_should_CallScrollLinesMinus5ThenPlus5_When_Drag90PxDownThenUp_InNormalBuffer', () => {
      const { term } = mountScroll({ cellH: 18 });
      start();
      mv(START_Y + SLOP_PX + 90); // finger down: 90px past the slop -> older output
      frame();
      mv(START_Y + SLOP_PX); // finger back up 90px
      frame();
      expect(term.scrollLines.mock.calls).toEqual([[-5], [5]]);
    });

    it('scrollDrag_should_SeedWithOvershootOnly_When_SlopCrossed (no initial jump, fractional carry, trunc not round)', () => {
      const { term } = mountScroll({ cellH: 20 });
      start();
      mv(START_Y + SLOP_PX + 5); // overshoot 5px < one 20px line
      frame();
      expect(term.scrollLines).not.toHaveBeenCalled();
      mv(START_Y + SLOP_PX + 5 + 14); // 19px carried in total: still under one line (Math.round would give 1)
      frame();
      expect(term.scrollLines).not.toHaveBeenCalled();
      mv(START_Y + SLOP_PX + 5 + 15); // 20px: exactly one line, toward older
      frame();
      expect(term.scrollLines.mock.calls).toEqual([[-1]]);
    });

    it('scrollDrag_should_ScrollBothDirections_InNormalAndAlternate', () => {
      // Normal buffer: finger down reveals older output (-), finger up newer (+).
      const normal = mountScroll({ cellH: 20 });
      start();
      mv(START_Y + SLOP_PX + 40);
      frame();
      mv(START_Y + SLOP_PX);
      frame();
      expect(normal.term.scrollLines.mock.calls).toEqual([[-2], [2]]);
      end(START_Y);
      normal.unmount();
      onSendData.mockClear();

      // Alternate buffer: one half page (11 lines = 220px at rows 24) per key, PgUp then PgDn.
      const alt = mountScroll({ bufferType: 'alternate', cellH: 20 });
      start();
      mv(START_Y + SLOP_PX + 220);
      frame();
      expect(onSendData.mock.calls).toEqual([[PGUP]]);
      jest.advanceTimersByTime(200); // clear the 100ms page rate limit
      mv(START_Y + SLOP_PX);
      frame();
      expect(onSendData.mock.calls).toEqual([[PGUP], [PGDN]]);
      expect(alt.term.scrollLines).not.toHaveBeenCalled();
    });

    it('scrollDrag_should_DispatchAtMostOncePerRaf_When_ManyTouchmovesInOneFrame', () => {
      const { term } = mountScroll({ cellH: 20 });
      start();
      for (let i = 1; i <= 10; i++) mv(START_Y + SLOP_PX + i * 20);
      frame();
      expect(term.scrollLines).toHaveBeenCalledTimes(1);
      expect(term.scrollLines).toHaveBeenCalledWith(-10);
    });

    // Task 1.1.4a hook tests, enabled now that the accumulator is wired.
    it('scrollDrag_should_CallScrollLinesEveryFrame_When_60FramesAtOneLineEach_InLocalBuffer', () => {
      const { term } = mountScroll({ cellH: 20, rows: 24 });
      start();
      mv(START_Y + SLOP_PX + 1); // seed 1px
      frame();
      for (let i = 1; i <= 60; i++) {
        mv(START_Y + SLOP_PX + 1 + i * 20);
        frame();
      }
      expect(term.scrollLines).toHaveBeenCalledTimes(60);
      expect(term.scrollLines.mock.calls.every((c: number[]) => c[0] === -1)).toBe(true);
    });

    it('scrollDrag_should_DispatchOnceAndClamp_When_200msFrameGapWith40Touchmoves', () => {
      const { term } = mountScroll({ cellH: 20, rows: 30 });
      start();
      for (let i = 1; i <= 40; i++) mv(START_Y + i * 20); // 800px in one stalled frame
      jest.advanceTimersByTime(200);
      expect(term.scrollLines.mock.calls).toEqual([[-30]]); // clamped to +-rows, excess not replayed
    });

    // ---- Group B: routing per frame, mode flip, TUI forwarding, direction (1.2.1a2) ----
    it('scrollDrag_should_SendOnePgUpAndNeverScrollLines_When_11LinePostSlopDragInAlternateBuffer', () => {
      const { term } = mountScroll({ bufferType: 'alternate', cellH: 18, rows: 24 });
      start();
      mv(START_Y + SLOP_PX + 198); // 213px total = 15 slop + 11 lines
      frame();
      expect(onSendData.mock.calls).toEqual([[PGUP]]);
      expect(term.scrollLines).not.toHaveBeenCalled();
    });

    it('scrollDrag_should_SendNothing_When_100PxPostSlopDragInAlternateBuffer', () => {
      const { term } = mountScroll({ bufferType: 'alternate', cellH: 18, rows: 24 });
      start();
      mv(START_Y + 115);
      frame();
      expect(onSendData).not.toHaveBeenCalled();
      expect(term.scrollLines).not.toHaveBeenCalled();
    });

    it('scrollDrag_should_SwitchToLocalAndResetAccumulator_When_ModeFlipsAlternateToNormalMidDrag', () => {
      const { term } = mountScroll({ bufferType: 'alternate', cellH: 20 });
      start();
      mv(START_Y + SLOP_PX + 30); // 30px carried: 1 line, 10px remainder
      frame();
      expect(term.scrollLines).not.toHaveBeenCalled();
      expect(onSendData).not.toHaveBeenCalled();
      term.buffer.active.type = 'normal'; // TUI exits the alternate screen
      mv(START_Y + SLOP_PX + 40); // +10px; without the reset the 10px remainder would make 1 line
      frame();
      expect(term.scrollLines).not.toHaveBeenCalled();
      mv(START_Y + SLOP_PX + 50); // +10px more: now exactly one local line from the reset accumulator
      frame();
      expect(term.scrollLines.mock.calls).toEqual([[-1]]);
    });

    it('scrollDrag_should_SendPgUpBytes_When_DragDownInTuiTarget', () => {
      mountScroll({ cellH: 20 }, { override: 'tui' }); // normal buffer, override forces the TUI route
      start();
      mv(START_Y + SLOP_PX + 220);
      frame();
      expect(onSendData.mock.calls).toEqual([[PGUP]]);
    });

    it('scrollDrag_should_SendPgDnBytes_When_DragUpInTuiTarget', () => {
      mountScroll({ cellH: 20 }, { override: 'tui' });
      start();
      mv(START_Y - SLOP_PX - 220);
      frame();
      expect(onSendData.mock.calls).toEqual([[PGDN]]);
    });

    it('scrollDrag_should_UseLocalRoute_When_OverrideLocalInAlternateBuffer', () => {
      const { term } = mountScroll({ bufferType: 'alternate', cellH: 20 }, { override: 'local' });
      start();
      mv(START_Y + SLOP_PX + 40);
      frame();
      expect(term.scrollLines.mock.calls).toEqual([[-2]]);
      expect(onSendData).not.toHaveBeenCalled();
    });

    it('scrollDrag_should_RouteToTui_When_MouseTrackingOnInNormalBuffer', () => {
      const { term } = mountScroll({ tracking: 'vt200', cellH: 20 });
      start();
      mv(START_Y + SLOP_PX + 220);
      frame();
      expect(onSendData.mock.calls).toEqual([[PGUP]]);
      expect(term.scrollLines).not.toHaveBeenCalled();
    });

    it('scrollDrag_should_SendWheelReportsAtTouchStartCell_When_TuiPolicyWheel', () => {
      const { term } = mountScroll({ cellH: 20 }, { override: 'tui', tuiScrollPolicy: 'wheel' });
      start(100, 100); // cellW 10, cellH 20 -> col index 10, row index 5 -> 1-based 11;6
      mv(START_Y + SLOP_PX + 40);
      frame();
      const report = '\x1b[<64;11;6M';
      expect(onSendData.mock.calls).toEqual([[report + report]]);
      expect(term.scrollLines).not.toHaveBeenCalled();
    });

    it('scrollDrag_should_DropExtraPages_When_RateLimitedWithin100ms', () => {
      mountScroll({ bufferType: 'alternate', cellH: 20 });
      start();
      mv(START_Y + SLOP_PX + 220);
      frame();
      mv(START_Y + SLOP_PX + 440); // another half page, but only ~16ms later
      frame();
      expect(onSendData.mock.calls).toEqual([[PGUP]]);
      jest.advanceTimersByTime(100);
      mv(START_Y + SLOP_PX + 441); // carried half page is released once the limit lapses
      frame();
      expect(onSendData.mock.calls).toEqual([[PGUP], [PGUP]]);
    });

    it('scrollDrag_should_LogRouteDecisionUnverified_When_DebugFlagOn', () => {
      localStorage.setItem('debug-terminal-mobile', 'true');
      jest.spyOn(console, 'debug').mockImplementation(() => {});
      mountScroll({ bufferType: 'alternate', cellH: 20 });
      start();
      mv(START_Y + SLOP_PX + 40);
      frame();
      const entries = JSON.parse((window as any).__termDebug.dump()) as Array<{ type: string; data: any }>;
      const decisions = entries.filter((e) => e.type === 'route-decision');
      expect(decisions[decisions.length - 1].data).toMatchObject({ target: 'tui-pgkeys', source: 'auto', unverified: true });
      (console.debug as jest.Mock).mockRestore();
    });

    // ---- Task 0.1.1b instrumentation ----
    it('instrumentation_should_LogScrollFrameAndTransitionAndCancel_When_DebugFlagOn', () => {
      localStorage.setItem('debug-terminal-mobile', 'true');
      jest.spyOn(console, 'debug').mockImplementation(() => {});
      mountScroll({ cellH: 20, rows: 24 });
      start();
      mv(START_Y + SLOP_PX + 40);
      frame();
      mv(START_Y + SLOP_PX + 60, { cancelable: false });
      docHandlers['touchcancel']({} as TouchEvent);
      const entries = JSON.parse((window as any).__termDebug.dump()) as Array<{ type: string; data: any }>;
      const byType = (t: string) => entries.filter((e) => e.type === t);
      expect(byType('scroll-start').length).toBeGreaterThan(0);
      expect(byType('scroll').pop()!.data).toMatchObject({
        bufferType: 'normal', mouseTrackingMode: 'none', viewportY: 3, baseY: 7, rows: 24, cellH: 20, lines: -2,
      });
      expect(byType('scroll').pop()!.data).toHaveProperty('moveDy');
      expect(byType('not-cancelable').length).toBeGreaterThan(0);
      expect(byType('touchcancel').length).toBeGreaterThan(0);
      (console.debug as jest.Mock).mockRestore();
    });

    // ---- Group C/D: hardening (Tasks 1.2.1a3, c, d1, d2) ----
    it('touchmove_should_PreventDefault_When_FirstMovePastSlopAndCancelable', () => {
      mountScroll();
      start();
      expect(mv(START_Y + SLOP_PX).preventDefault).not.toHaveBeenCalled(); // at the slop: still PENDING
      expect(mv(START_Y + SLOP_PX + 1).preventDefault).toHaveBeenCalledTimes(1);
    });

    it('touchmove_should_NotPreventDefaultAndLog_When_NotCancelable', () => {
      localStorage.setItem('debug-terminal-mobile', 'true');
      jest.spyOn(console, 'debug').mockImplementation(() => {});
      mountScroll();
      start();
      const ev = mv(START_Y + SLOP_PX + 10, { cancelable: false });
      expect(ev.preventDefault).not.toHaveBeenCalled();
      const entries = JSON.parse((window as any).__termDebug.dump()) as Array<{ type: string; data: any }>;
      expect(entries.some((e) => e.type === 'not-cancelable' && e.data.event === 'touchmove')).toBe(true);
      (console.debug as jest.Mock).mockRestore();
    });

    it('touchstart_should_IgnoreGesture_When_TouchBeganOnScrollbarOrSelectionHandle', () => {
      const { term } = mountScroll();
      const ignoredTarget = { closest: jest.fn(() => ({})) }; // matches the scrollbar/handle ignore selector
      fakeContainer.fire('touchstart', makeTouchEvent('touchstart', 100, START_Y, 'touches', { target: ignoredTarget }));
      expect(ignoredTarget.closest).toHaveBeenCalledWith(expect.stringContaining('.scrollbar'));
      expect(mv(START_Y + 100).preventDefault).not.toHaveBeenCalled(); // still IDLE: no scroll gesture
      frame();
      jest.advanceTimersByTime(500); // and no long-press selection either
      end(START_Y + 100);
      expect(term.scrollLines).not.toHaveBeenCalled();
      expect(term.focus).not.toHaveBeenCalled();
      expect(term.select).not.toHaveBeenCalled();
    });

    it('touchstart_should_StartGesture_When_TargetIsNotIgnorable', () => {
      const { term } = mountScroll({ cellH: 20 });
      const plainTarget = { closest: jest.fn(() => null) };
      fakeContainer.fire('touchstart', makeTouchEvent('touchstart', 100, START_Y, 'touches', { target: plainTarget }));
      mv(START_Y + SLOP_PX + 40);
      frame();
      expect(term.scrollLines).toHaveBeenCalledWith(-2);
    });

    it('touchend_should_PreventDefault_When_ScrollCompletedAndCancelable', () => {
      mountScroll();
      start();
      mv(START_Y + SLOP_PX + 40);
      frame();
      expect(end(START_Y + SLOP_PX + 40).preventDefault).toHaveBeenCalledTimes(1);
    });

    it('touchend_should_NotPreventDefaultAndLog_When_NotCancelable', () => {
      localStorage.setItem('debug-terminal-mobile', 'true');
      jest.spyOn(console, 'debug').mockImplementation(() => {});
      mountScroll();
      start();
      mv(START_Y + SLOP_PX + 40);
      frame();
      const ev = end(START_Y + SLOP_PX + 40, { cancelable: false });
      expect(ev.preventDefault).not.toHaveBeenCalled();
      const entries = JSON.parse((window as any).__termDebug.dump()) as Array<{ type: string; data: any }>;
      expect(entries.some((e) => e.type === 'not-cancelable' && e.data.event === 'touchend')).toBe(true);
      (console.debug as jest.Mock).mockRestore();
    });

    it('touchend_should_NotFocusTerminal_When_ScrollCompleted', () => {
      const { term } = mountScroll();
      start();
      mv(START_Y + SLOP_PX + 40);
      frame();
      end(START_Y + SLOP_PX + 40);
      expect(term.focus).not.toHaveBeenCalled();
    });

    it('touchend_should_NotPreventDefault_When_TapCompleted', () => {
      mountScroll();
      start();
      expect(end(START_Y).preventDefault).not.toHaveBeenCalled();
    });

    it('hook_should_RegisterTouchendNonPassive_SoPreventDefaultTakesEffect', () => {
      mountScroll();
      const calls = (document.addEventListener as jest.Mock).mock.calls.filter((c) => c[0] === 'touchend');
      expect(calls[calls.length - 1][2]).toMatchObject({ passive: false });
    });

    it('scrollDrag_should_ResetAccumulatorAndCancel_When_ViewportResizeMidGesture', () => {
      const vv = installFakeVisualViewport();
      const { term } = mountScroll({ cellH: 20 });
      start();
      mv(START_Y + SLOP_PX + 30); // non-zero remainder, frame pending
      vv.fire('resize');
      frame();
      expect(term.scrollLines).not.toHaveBeenCalled(); // pending frame dropped
      expect(mv(START_Y + SLOP_PX + 100).preventDefault).not.toHaveBeenCalled(); // rest of the touch ignored
      frame();
      expect(end(START_Y + 200).preventDefault).not.toHaveBeenCalled();
      expect(term.scrollLines).not.toHaveBeenCalled();
      expect(term.focus).not.toHaveBeenCalled();
      // A new touchstart resumes normal scrolling with a clean (zero) remainder.
      start(START_Y);
      mv(START_Y + SLOP_PX + 1);
      frame();
      mv(START_Y + SLOP_PX + 1 + 19); // 20px in total -> exactly one line
      frame();
      expect(term.scrollLines.mock.calls).toEqual([[-1]]);
    });

    it('scrollDrag_should_IgnoreRemainingMoves_When_OrientationChangeMidGesture', () => {
      const { term } = mountScroll({ bufferType: 'alternate', cellH: 20 });
      start();
      mv(START_Y + SLOP_PX + 100);
      window.dispatchEvent(new Event('orientationchange'));
      frame();
      mv(START_Y + SLOP_PX + 500);
      frame();
      expect(onSendData).not.toHaveBeenCalled();
      expect(term.scrollLines).not.toHaveBeenCalled();
    });

    it('scrollDrag_should_CancelGesture_When_ConnectionEpochChanges', () => {
      const { term, rerender } = mountScroll({ cellH: 20 }, { connectionEpoch: 0 });
      start();
      mv(START_Y + SLOP_PX + 30);
      rerender({ connectionEpoch: 1 });
      frame();
      mv(START_Y + SLOP_PX + 200);
      frame();
      expect(term.scrollLines).not.toHaveBeenCalled();
    });

    it('scrollDrag_should_CancelGesture_When_OverrideChangesMidGesture', () => {
      const { term, rerender } = mountScroll({ cellH: 20 }, { override: 'auto' });
      start();
      mv(START_Y + SLOP_PX + 30);
      rerender({ override: 'tui' });
      frame();
      mv(START_Y + SLOP_PX + 400);
      frame();
      expect(term.scrollLines).not.toHaveBeenCalled();
      expect(onSendData).not.toHaveBeenCalled();
    });

    it('scrollDrag_should_NotCancel_When_UnrelatedRerender', () => {
      const { term, rerender } = mountScroll({ cellH: 20 }, { override: 'auto', connectionEpoch: 4 });
      start();
      mv(START_Y + SLOP_PX + 30);
      rerender({ override: 'auto', connectionEpoch: 4 });
      frame();
      expect(term.scrollLines).toHaveBeenCalledWith(-1);
    });

    // ---- Pins: single scrollLines caller, no double scroll ----
    it('scrollLines_should_HaveSingleProductionCaller_InUseTerminalGestures', () => {
      const root = join(__dirname, '../../..'); // web-app/src
      const callers: Record<string, number> = {};
      const walk = (dir: string) => {
        for (const name of readdirSync(dir)) {
          const full = join(dir, name);
          if (statSync(full).isDirectory()) {
            if (name === '__tests__' || name === '__mocks__' || name === 'gen' || name === 'node_modules') continue;
            walk(full);
          } else if (/\.(ts|tsx)$/.test(name) && !/\.test\.(ts|tsx)$/.test(name)) {
            const hits = readFileSync(full, 'utf8').match(/\.scrollLines\(/g)?.length ?? 0;
            if (hits > 0) callers[relative(root, full)] = hits;
          }
        }
      };
      walk(root);
      // The touch-drag path has exactly one caller. XtermTerminal.tsx's three calls belong to
      // the custom scrollbar (track click and thumb drag) which sits outside the gesture container;
      // pinned by count so a new caller fails this test.
      expect(callers).toEqual({
        'lib/hooks/useTerminalGestures.ts': 1,
        'components/sessions/XtermTerminal.tsx': 3,
      });
    });

    it('viewportTouch_should_NotDoubleScroll_When_HookOwnsDrag', () => {
      const { term } = mountScroll({ cellH: 20 });
      start();
      const events = [START_Y + SLOP_PX + 20, START_Y + SLOP_PX + 40, START_Y + SLOP_PX + 60].map((y) => mv(y));
      frame();
      // Every move past the slop (crossing included) is cancelled so the native viewport gets no default scroll,
      // and the hook is the only scroller: 40px past the slop = exactly 3 lines in total.
      events.forEach((ev) => expect(ev.preventDefault).toHaveBeenCalledTimes(1));
      expect(term.scrollLines.mock.calls).toEqual([[-3]]);
      const [, , touchmoveOptions] = (document.addEventListener as jest.Mock).mock.calls.filter((c) => c[0] === 'touchmove').pop()!;
      expect(touchmoveOptions).toMatchObject({ passive: false });
    });

    // ---- Listener lifecycle (REQ-9) ----
    it('hook_should_RegisterTouchEventListeners_AndNoPointerEventListeners_When_Mounted', () => {
      mountScroll();
      const types = [
        ...(fakeContainer.el.addEventListener as jest.Mock).mock.calls,
        ...(document.addEventListener as jest.Mock).mock.calls,
      ].map((c) => c[0] as string);
      expect(types).toEqual(expect.arrayContaining(['touchstart', 'touchmove', 'touchend', 'touchcancel']));
      expect(types.filter((t) => t.startsWith('pointer'))).toEqual([]);
    });

    it('hook_should_RemoveAllListenersAndRafs_When_Unmounted', () => {
      const vv = installFakeVisualViewport();
      const { term, unmount } = mountScroll({ cellH: 20 });
      expect(vv.count('resize')).toBe(1);
      start();
      mv(START_Y + SLOP_PX + 60); // frame pending
      unmount();
      frame();
      expect(term.scrollLines).not.toHaveBeenCalled(); // pending rAF cancelled
      expect(fakeContainer.el.removeEventListener).toHaveBeenCalledWith('touchstart', expect.any(Function));
      for (const t of ['touchmove', 'touchend', 'touchcancel']) {
        expect(document.removeEventListener).toHaveBeenCalledWith(t, expect.any(Function));
      }
      expect(vv.count('resize')).toBe(0);
      const before = onSendData.mock.calls.length;
      window.dispatchEvent(new Event('orientationchange')); // no throw, no effect after unmount
      expect(onSendData.mock.calls.length).toBe(before);
    });

    // ---- Gesture scrolling Off (hook half of Task 1.2.5c) ----
    it('hook_should_RegisterNoTouchListeners_When_GestureScrollOff', () => {
      mountScroll({}, { gestureScrollEnabled: false });
      expect(fakeContainer.el.addEventListener).not.toHaveBeenCalled();
      expect(docHandlers['touchmove']).toBeUndefined();
    });

    it('hook_should_NeverPreventDefault_When_GestureScrollOff', () => {
      mountScroll({}, { gestureScrollEnabled: false });
      fireTouchStart(100, START_Y);
      expect(mv(START_Y + 100).preventDefault).not.toHaveBeenCalled();
    });

    it('hook_should_ReRegisterListeners_When_GestureScrollTurnedOnLive', () => {
      const { rerender, term } = mountScroll({ cellH: 20 }, { gestureScrollEnabled: false });
      rerender({ gestureScrollEnabled: true });
      expect(fakeContainer.el.addEventListener).toHaveBeenCalledWith('touchstart', expect.any(Function), expect.any(Object));
      start();
      mv(START_Y + SLOP_PX + 40);
      frame();
      expect(term.scrollLines).toHaveBeenCalledWith(-2);
      rerender({ gestureScrollEnabled: false });
      expect(fakeContainer.el.removeEventListener).toHaveBeenCalledWith('touchstart', expect.any(Function));
    });
  });

  // -------------------------------------------------------------------------
  // Cleanup
  // -------------------------------------------------------------------------
  describe('cleanup', () => {
    it('should remove event listeners on unmount', () => {
      const { unmount } = renderHook(() => {
        const terminalRef = makeTerminalRef();
        useTerminalGestures({ containerRef, terminalRef: terminalRef as any, onSendData });
      });

      unmount();

      expect(fakeContainer.el.removeEventListener).toHaveBeenCalledWith(
        'touchstart',
        expect.any(Function),
      );
    });
  });
});
