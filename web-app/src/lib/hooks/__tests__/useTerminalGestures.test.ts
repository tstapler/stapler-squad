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
    addEventListener: jest.fn((type: string, listener: EventListenerOrEventListenerObject) => {
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
): TouchEvent {
  const touch = { clientX, clientY } as Touch;
  const event: Partial<TouchEvent> = {
    type,
    touches: touchListKey === 'touches' ? [touch] as unknown as TouchList : [] as unknown as TouchList,
    changedTouches: [touch] as unknown as TouchList,
    preventDefault: jest.fn(),
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

    it('touchend_should_Tap_When_MovedUnderSlopAndReleasedBefore400ms (current tolerance: dy < 8px)', () => {
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      move(100, 107); // 7px: stays PENDING (below the 15px scroll threshold)
      jest.advanceTimersByTime(399);
      fireTouchEnd(100, 107);
      expect(terminalRef.current!.focus).toHaveBeenCalledTimes(1);
    });

    it('touchend_should_NotTap_When_TotalDyIs8OrMore (current: 8px boundary, not 12/14px)', () => {
      const { terminalRef } = mount('none');
      fireTouchStart(100, 100);
      move(100, 108);
      jest.advanceTimersByTime(100);
      fireTouchEnd(100, 108);
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

    it('touchmove_should_NotPreventDefault_When_FirstMovePastSlop (current: only subsequent moves preventDefault)', () => {
      mount();
      fireTouchStart(100, 100);
      expect(move(100, 70).preventDefault).not.toHaveBeenCalled(); // PENDING -> SCROLLING
      expect(move(100, 40).preventDefault).toHaveBeenCalled(); // SCROLLING
    });

    it('scroll_should_ScrollLinesByRoundedCellDelta_And_NotFocusOnTouchEnd (current: scroll is lastY-relative)', () => {
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
