exports.id = 2145;
exports.ids = [2145];
exports.modules = {

/***/ 26550:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   H$: () => (/* binding */ decideScrollTarget),
/* harmony export */   Md: () => (/* binding */ PAGE_FLING_CAP),
/* harmony export */   Mu: () => (/* binding */ PAGE_RATE_LIMIT_MS),
/* harmony export */   Nl: () => (/* binding */ PAGE_DOWN_BYTES),
/* harmony export */   Ql: () => (/* binding */ decideAndLogScrollTarget),
/* harmony export */   UU: () => (/* binding */ PAGE_UP_BYTES),
/* harmony export */   YA: () => (/* binding */ ROUTING_VERIFIED),
/* harmony export */   d4: () => (/* binding */ toolbarPageAction),
/* harmony export */   gU: () => (/* binding */ encodeWheel),
/* harmony export */   qC: () => (/* binding */ encodePageKeys),
/* harmony export */   yC: () => (/* binding */ PageAccumulator)
/* harmony export */ });
/* unused harmony exports TUI_SCROLL_POLICY, DEFAULT_ROUTING_POLICY, MAX_WHEEL_REPORTS_PER_FRAME, encodeWheelX10, tuiPageStepLines */
/**
 * Scroll routing for touch drags: decides where a scroll step is delivered
 * (xterm local scrollback vs. TUI via wheel reports or PgUp/PgDn) and encodes
 * the bytes. Pure and deterministic; clocks and loggers are injected.
 */ /** Flips to true once the Phase 0 spike confirms the default rows. */ const ROUTING_VERIFIED = false;
const TUI_SCROLL_POLICY = "pgkeys";
const TRACKING_ON = [
    "x10",
    "vt200",
    "drag",
    "any"
];
const DEFAULT_ROUTING_POLICY = {
    rules: [
        {
            when: {
                bufferType: "alternate"
            },
            target: "tui-pgkeys"
        },
        ...TRACKING_ON.map((mouseTrackingMode)=>({
                when: {
                    mouseTrackingMode
                },
                target: "tui-pgkeys"
            }))
    ],
    default: "xterm-local"
};
function ruleMatches(when, mode) {
    return (when.bufferType === undefined || when.bufferType === mode.bufferType) && (when.mouseTrackingMode === undefined || when.mouseTrackingMode === mode.mouseTrackingMode);
}
/** First matching row wins; `tui-wheel` is only reachable when tuiPolicy is 'wheel'. */ function decideScrollTarget(mode, policy = DEFAULT_ROUTING_POLICY, tuiPolicy = TUI_SCROLL_POLICY, override = "auto") {
    const tuiTarget = tuiPolicy === "wheel" ? "tui-wheel" : "tui-pgkeys";
    if (override === "local") return "xterm-local";
    if (override === "tui") return tuiTarget;
    const row = policy.rules.find((r)=>ruleMatches(r.when, mode));
    const target = row ? row.target : policy.default;
    return target === "xterm-local" ? target : tuiTarget;
}
/** decideScrollTarget plus a log entry carrying `unverified:true` while routing is unverified. */ function decideAndLogScrollTarget(mode, opts = {}) {
    const override = opts.override ?? "auto";
    const target = decideScrollTarget(mode, opts.policy, opts.tuiPolicy, override);
    const verified = opts.routingVerified ?? ROUTING_VERIFIED;
    const entry = {
        target,
        source: override === "auto" ? "auto" : "override",
        mode,
        ...verified ? {} : {
            unverified: true
        }
    };
    opts.log?.(entry);
    return target;
}
// ---- Encoders ----
const MAX_WHEEL_REPORTS_PER_FRAME = 3;
function clamp(v, lo, hi) {
    return Math.min(Math.max(v, lo), hi);
}
/**
 * SGR wheel reports. lineDelta < 0 = toward older output (wheel up, button 64),
 * > 0 = toward newer (button 65). Emits at most MAX_WHEEL_REPORTS_PER_FRAME reports.
 */ function encodeWheel(lineDelta, cell) {
    const count = Math.min(Math.abs(Math.trunc(lineDelta)), MAX_WHEEL_REPORTS_PER_FRAME);
    if (count === 0) return "";
    const col = clamp(Math.trunc(cell.col), 1, Math.max(1, cell.cols));
    const row = clamp(Math.trunc(cell.row), 1, Math.max(1, cell.rows));
    const button = lineDelta < 0 ? 64 : 65;
    return `\x1b[<${button};${col};${row}M`.repeat(count);
}
/** X10 fallback; retained for the spike but deliberately not selectable via decideScrollTarget. */ function encodeWheelX10(lineDelta, cell) {
    const count = Math.min(Math.abs(Math.trunc(lineDelta)), MAX_WHEEL_REPORTS_PER_FRAME);
    if (count === 0) return "";
    const col = clamp(Math.trunc(cell.col), 1, Math.max(1, cell.cols));
    const row = clamp(Math.trunc(cell.row), 1, Math.max(1, cell.rows));
    const button = lineDelta < 0 ? 64 : 65;
    // X10 encodes each value as a single byte offset by 32 (limited to 223 cells).
    const ch = (n)=>String.fromCharCode(clamp(n, 0, 223) + 32);
    return `\x1b[M${ch(button)}${ch(col)}${ch(row)}`.repeat(count);
}
const PAGE_UP_BYTES = "\x1b[5~";
const PAGE_DOWN_BYTES = "\x1b[6~";
/** direction 'older' = PgUp, 'newer' = PgDn (same bytes as the toolbar keys). */ function encodePageKeys(direction, count = 1) {
    return (direction === "older" ? PAGE_UP_BYTES : PAGE_DOWN_BYTES).repeat(Math.max(0, count));
}
// ---- Page accumulator ----
const PAGE_RATE_LIMIT_MS = 100;
const PAGE_FLING_CAP = 5;
/** Post-slop finger travel (lines) that emits one page key: half a page. */ function tuiPageStepLines(rows) {
    return Math.max(1, Math.floor((rows - 1) / 2));
}
/**
 * Accumulates line travel into page keys. Positive lines = newer (PgDn),
 * negative = older (PgUp). Emits at most one key per call (frame), carries the
 * remainder, rate-limits to one per rateLimitMs and caps keys per fling.
 */ class PageAccumulator {
    constructor(opts){
        this.carry = 0;
        this.emitted = 0;
        this.lastEmitAt = Number.NEGATIVE_INFINITY;
        this.rows = opts.rows;
        this.now = opts.now;
        this.rateLimitMs = opts.rateLimitMs ?? PAGE_RATE_LIMIT_MS;
        this.flingCap = opts.flingCap ?? PAGE_FLING_CAP;
    }
    setRows(rows) {
        this.rows = rows;
    }
    /** True once the per-fling cap is hit; callers cancel momentum. */ get capped() {
        return this.emitted >= this.flingCap;
    }
    /** Returns the page-key bytes for this frame ("" when none). */ push(lines) {
        this.carry += lines;
        const step = tuiPageStepLines(this.rows);
        if (Math.abs(this.carry) < step) return "";
        if (this.capped) {
            this.carry = 0;
            return "";
        }
        const t = this.now();
        if (t - this.lastEmitAt < this.rateLimitMs) return "";
        const older = this.carry < 0;
        this.carry += older ? step : -step;
        this.emitted += 1;
        this.lastEmitAt = t;
        return encodePageKeys(older ? "older" : "newer");
    }
    /** Reset at gesture start / mode flip. */ reset() {
        this.carry = 0;
        this.emitted = 0;
        this.lastEmitAt = Number.NEGATIVE_INFINITY;
    }
}
/**
 * Same route as the drag, so the toolbar is an equivalent single-pointer
 * alternative. A modifier always sends bytes; in `auto`, a local route at its
 * edge falls through to bytes (today's behaviour for a normal-buffer TUI the
 * table misclassifies as local); an explicit `local` override never falls through.
 */ function toolbarPageAction(input) {
    const { route, direction, modifiers, canScroll, override } = input;
    const bytes = direction === "up" ? PAGE_UP_BYTES : PAGE_DOWN_BYTES;
    const modified = modifiers.ctrl || modifiers.alt || modifiers.shift;
    const isTui = route !== "xterm-local";
    if (modified) return {
        type: "send-keys",
        bytes,
        countsTowardNetPages: false
    };
    if (isTui) return {
        type: "send-keys",
        bytes,
        countsTowardNetPages: true
    };
    if (!canScroll && override !== "local") return {
        type: "send-keys",
        bytes,
        countsTowardNetPages: false
    };
    return {
        type: "scroll-pages",
        pages: direction === "up" ? -1 : 1
    };
}


/***/ }),

/***/ 32145:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  MAX_SAMPLES: () => (/* binding */ MAX_SAMPLES),
  SAMPLE_INTERVAL_MS: () => (/* binding */ SAMPLE_INTERVAL_MS),
  XtermTerminal: () => (/* binding */ XtermTerminal),
  extractCellMismatchInputs: () => (/* binding */ extractCellMismatchInputs),
  isMouseReport: () => (/* binding */ isMouseReport),
  isSustainedMismatch: () => (/* binding */ isSustainedMismatch),
  shouldScheduleFit: () => (/* binding */ shouldScheduleFit),
  trackSgrMouseEncoding: () => (/* binding */ trackSgrMouseEncoding)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-dom.js
var react_dom = __webpack_require__(68051);
;// ./src/lib/terminal/cellDimensions.ts
/**
 * getCellDimensions — compute terminal cell dimensions from public xterm.js API.
 *
 * Uses element.clientHeight / rows (public) instead of the private
 * _core._renderService.dimensions.css.cell.height, which may be undefined until
 * after the first render frame in xterm.js 6.x (R3.4 / R4.4).
 *
 * Shared between useTerminalGestures and XtermTerminal to avoid duplication.
 */ function getCellDimensions(terminal) {
    const el = terminal.element;
    if (el && terminal.rows > 0 && terminal.cols > 0) {
        return {
            cellH: el.clientHeight / terminal.rows,
            cellW: el.clientWidth / terminal.cols
        };
    }
    // Fallback: use font metrics (less accurate but never undefined)
    const fontSize = terminal.options.fontSize ?? 14;
    const lineHeight = terminal.options.lineHeight ?? 1.0;
    return {
        cellH: fontSize * lineHeight,
        cellW: fontSize * 0.6
    };
}

;// ./src/lib/terminal/mouseTracking.ts
function readMouseTrackingMode(terminal) {
    return terminal.modes?.mouseTrackingMode;
}
function mouseTracking_isMouseTracking(terminal) {
    const mode = readMouseTrackingMode(terminal);
    return mode !== "none" && mode !== undefined;
}
const KNOWN_TRACKING_MODES = [
    "none",
    "x10",
    "vt200",
    "drag",
    "any"
];
/** Snapshot of the terminal's routing signals; re-read every frame. */ function readScrollMode(terminal) {
    const raw = readMouseTrackingMode(terminal);
    const mouseTrackingMode = raw !== undefined && KNOWN_TRACKING_MODES.includes(raw) ? raw : raw === undefined ? "none" : "any";
    const bufferType = terminal.buffer?.active?.type === "alternate" ? "alternate" : "normal";
    return {
        bufferType,
        mouseTrackingMode
    };
}

;// ./src/lib/terminal/touchDrag.ts
/**
 * touchDrag — shared helpers for touch-driven terminal drag gestures
 * (long-press text selection in useTerminalGestures, and the draggable
 * selection handles in XtermTerminal). Kept here instead of duplicated in
 * both call sites so the cell-coordinate math and per-frame throttling stay
 * consistent between the two gestures.
 */ /**
 * Converts a viewport point to clamped terminal cell coordinates.
 * `rect` should be captured once per drag (at touchstart), not
 * re-measured on every touchmove — the DOM read is what makes raw,
 * unthrottled touchmove handlers janky on Android.
 */ function pointToCell(clientX, clientY, geometry) {
    const { rect, cellW, cellH, maxCol, maxRow } = geometry;
    return {
        col: Math.max(0, Math.min(maxCol, Math.floor((clientX - rect.left) / cellW))),
        row: Math.max(0, Math.min(maxRow, Math.floor((clientY - rect.top) / cellH)))
    };
}
/**
 * Wraps a per-point handler so it runs at most once per animation frame,
 * always with the most recent point — intermediate touchmove events are
 * coalesced instead of each one forcing an immediate xterm redraw.
 * Returns [throttled handler, cancel, flush] — call cancel() on gesture end/cleanup
 * to drop any frame still pending from the last point before teardown, or
 * flush() to apply that point immediately first.
 */ function rafThrottlePoint(handler) {
    let rafId = null;
    let latestX = 0;
    let latestY = 0;
    const throttled = (clientX, clientY)=>{
        latestX = clientX;
        latestY = clientY;
        if (rafId !== null) return;
        rafId = requestAnimationFrame(()=>{
            rafId = null;
            handler(latestX, latestY);
        });
    };
    const cancel = ()=>{
        if (rafId !== null) {
            cancelAnimationFrame(rafId);
            rafId = null;
        }
    };
    /** Runs the handler now with the latest point if a frame is pending, so release does not drop the last travel. */ const flush = ()=>{
        if (rafId === null) return;
        cancelAnimationFrame(rafId);
        rafId = null;
        handler(latestX, latestY);
    };
    return [
        throttled,
        cancel,
        flush
    ];
}
/**
 * Text window of `windowSize` columns around `activeCol` on a viewport row, for the
 * selection magnifier. `cursorOffset` is the index of `activeCol` within `text`.
 * Display only — never feeds back into selection math.
 */ function getTerminalRowText(terminal, viewportRow, activeCol, windowSize = 20) {
    const buf = terminal.buffer.active;
    const line = buf.getLine(viewportRow + buf.viewportY);
    const startCol = Math.max(0, activeCol - Math.floor(windowSize / 2));
    const endCol = Math.min(terminal.cols, startCol + windowSize);
    const text = (line?.translateToString(false, startCol, endCol) ?? '').padEnd(endCol - startCol, ' ');
    return {
        text,
        cursorOffset: activeCol - startCol
    };
}
/** Fixed bubble size so positioning needs no layout read inside the RAF callback. */ const MAGNIFIER_WIDTH = 200;
const MAGNIFIER_HEIGHT = 32;
const MAGNIFIER_GAP = 56; // clears the finger and fingertip occlusion
const MAGNIFIER_EDGE = 8;
/** Top-left of the magnifier bubble: centered above the touch point, kept inside the viewport width. */ function computeMagnifierPosition(clientX, clientY, viewportWidth) {
    const maxLeft = Math.max(MAGNIFIER_EDGE, viewportWidth - MAGNIFIER_WIDTH - MAGNIFIER_EDGE);
    return {
        left: Math.max(MAGNIFIER_EDGE, Math.min(maxLeft, clientX - MAGNIFIER_WIDTH / 2)),
        top: Math.max(MAGNIFIER_EDGE, clientY - MAGNIFIER_HEIGHT - MAGNIFIER_GAP)
    };
}

;// ./src/lib/terminal/gestureMachine.ts
/**
 * Pure touch-gesture state machine (design/ux.md S9). No DOM, React or timers:
 * the hook feeds it events and executes the returned effects in order.
 */ /**
 * COASTING: momentum running after a fling release (no finger down).
 * CANCELLED: the touch is ignored until the next touchstart (design/ux.md S9).
 */ const NONE = [];
const stay = (state)=>({
        state,
        effects: NONE
    });
// A SELECTING abort must close the synthetic mouse selection first, or xterm keeps tracking a button that never releases.
const abort = (from)=>({
        state: "IDLE",
        effects: from === "SELECTING" ? [
            "endSelecting",
            "abort"
        ] : [
            "abort"
        ]
    });
function onTouchStart(state, touchCount) {
    // Multi-touch parks the gesture in CANCELLED so the remaining fingers' moves are ignored until a lone touchstart.
    if (touchCount !== 1) return {
        state: "CANCELLED",
        effects: state === "SELECTING" ? [
            "endSelecting",
            "abort"
        ] : [
            "abort"
        ]
    };
    // abort cancels momentum; no long-press timer, a touch-to-stop must not become a selection.
    if (state === "COASTING") return {
        state: "PENDING",
        effects: [
            "abort",
            "setConsumedByCoast"
        ]
    };
    return {
        state: "PENDING",
        effects: [
            "startLongPressTimer"
        ]
    };
}
function onTouchMove(state, e) {
    if (e.touchCount !== 1) return state === "CANCELLED" ? stay(state) : abort(state);
    switch(state){
        case "PENDING":
            if (e.absDx <= e.slopPx && e.absDy <= e.slopPx) return stay(state);
            // Horizontal-first: no scroll and no preventDefault (Android back-swipe stays with the browser). The
            // long-press timer keeps running so a held touch with sideways drift still selects; a release is not a tap
            // because the tap check below counts horizontal travel.
            if (e.absDx > e.absDy) return stay(state);
            return {
                state: "SCROLLING",
                effects: [
                    "clearLongPressTimer",
                    "beginScroll",
                    "preventDefault"
                ]
            };
        case "SCROLLING":
            return {
                state,
                effects: [
                    "continueScroll",
                    "preventDefault"
                ]
            };
        case "SELECTING":
            return {
                state,
                effects: [
                    "continueSelect",
                    "preventDefault"
                ]
            };
        default:
            return stay(state);
    }
}
function onTouchEnd(state, e) {
    if (state === "COASTING") return stay(state);
    if (state === "PENDING" && e.consumedByCoast) return {
        state: "IDLE",
        effects: [
            "abort",
            "preventDefault"
        ]
    };
    // Inclusive: PENDING persists while travel <= slop, so a release at exactly the slop is still a tap.
    if (state === "PENDING" && Math.max(e.totalDy, e.totalDx ?? 0) <= e.tapTolerancePx && e.elapsedMs < e.longPressMs) {
        return {
            state: "IDLE",
            effects: [
                "clearLongPressTimer",
                e.selectionActive ? "clearSelection" : "tap"
            ]
        };
    }
    if (state === "SELECTING") return {
        state: "IDLE",
        effects: [
            "endSelecting",
            "abort"
        ]
    };
    // preventDefault stops Chrome synthesizing a click (and focus) after a scroll.
    if (state === "SCROLLING") {
        return e.flinging ? {
            state: "COASTING",
            effects: [
                "startMomentum",
                "preventDefault"
            ]
        } : {
            state: "IDLE",
            effects: [
                "abort",
                "preventDefault"
            ]
        };
    }
    return abort();
}
function reduce(state, event) {
    switch(event.type){
        case "touchstart":
            return onTouchStart(state, event.touchCount);
        case "touchmove":
            return onTouchMove(state, event);
        case "touchend":
            return onTouchEnd(state, event);
        case "touchcancel":
            // Intentionally IDLE (not CANCELLED as in ux.md S9 for SCROLLING): plan AC 1.2.10 specifies IDLE, and
            // the next touchstart begins a fresh PENDING from either state, so the two are observably identical.
            return abort(state);
        case "longPress":
            return state === "PENDING" ? {
                state: "SELECTING",
                effects: [
                    "enterSelecting"
                ]
            } : stay(state);
        case "momentumEnd":
            return state === "COASTING" ? abort() : stay(state);
        case "interrupt":
            return state === "PENDING" || state === "SCROLLING" || state === "COASTING" ? {
                state: "CANCELLED",
                effects: [
                    "abort"
                ]
            } : stay(state);
    }
}

;// ./src/lib/terminal/scrollKinematics.ts
/**
 * Pure, DOM-free touch-scroll kinematics: fractional-line accumulation,
 * fling momentum, and per-frame output clamping. Time is injected by callers.
 */ /** Signed whole-line count; positive = toward newer output (finger moved up). */ /** Pixels of finger travel before a touch becomes a scroll; also the tap tolerance. */ const SLOP_PX = 15;
/** Canonical momentum constants (shared with design/ux.md; tune on device). */ const MOMENTUM_CONSTANTS = {
    windowMs: 100,
    minFlingPxPerMs: 0.3,
    decayPerFrame: 0.95,
    frameMs: 16,
    stopPxPerMs: 0.02,
    maxVelocityPxPerMs: 8,
    maxFrames: 120
};
/** Converts pixel deltas to whole lines, carrying the fractional remainder (px). */ class ScrollAccumulator {
    get remainder() {
        return this.acc;
    }
    /** Seed with the slop overshoot (travel minus SLOP_PX) so crossing emits no jump. */ seed(overshootPx) {
        this.acc = Number.isFinite(overshootPx) ? overshootPx : 0;
    }
    push(dyPx, cellH) {
        if (!(cellH > 0) || !Number.isFinite(dyPx)) return 0;
        this.acc += dyPx;
        const lines = Math.trunc(this.acc / cellH);
        this.acc -= lines * cellH;
        return lines;
    }
    reset() {
        this.acc = 0;
    }
    constructor(){
        this.acc = 0;
    }
}
/**
 * Tracks recent samples, then after release yields decaying per-frame pixel
 * deltas. Output keeps the sign of the sampled y movement.
 */ class MomentumTracker {
    get active() {
        return this.running;
    }
    addSample(t, y) {
        this.samples.push({
            t,
            y
        });
        const cutoff = t - MOMENTUM_CONSTANTS.windowMs;
        while(this.samples.length > 2 && this.samples[0].t < cutoff)this.samples.shift();
    }
    /** Returns true if a fling started; false for reduced motion or a slow release. */ release(reducedMotion) {
        const samples = this.samples;
        this.samples = [];
        this.running = false;
        this.velocity = 0;
        this.frames = 0;
        if (reducedMotion || samples.length < 2) return false;
        const last = samples[samples.length - 1];
        const inWindow = samples.filter((s)=>last.t - s.t <= MOMENTUM_CONSTANTS.windowMs);
        const first = inWindow[0];
        const dt = last.t - first.t;
        if (inWindow.length < 2 || dt <= 0) return false;
        const v = (last.y - first.y) / dt;
        if (Math.abs(v) < MOMENTUM_CONSTANTS.minFlingPxPerMs) return false;
        const cap = MOMENTUM_CONSTANTS.maxVelocityPxPerMs;
        this.velocity = Math.max(-cap, Math.min(cap, v));
        this.running = true;
        return true;
    }
    /** Advances by dtMs and returns the pixel delta; 0 once momentum has ended. */ step(dtMs) {
        if (!this.running) return 0;
        const dy = this.velocity * dtMs;
        this.frames++;
        this.velocity *= Math.pow(MOMENTUM_CONSTANTS.decayPerFrame, dtMs / MOMENTUM_CONSTANTS.frameMs);
        if (Math.abs(this.velocity) < MOMENTUM_CONSTANTS.stopPxPerMs || this.frames >= MOMENTUM_CONSTANTS.maxFrames) {
            this.running = false;
        }
        return dy;
    }
    cancel() {
        this.samples = [];
        this.velocity = 0;
        this.frames = 0;
        this.running = false;
    }
    constructor(){
        this.samples = [];
        this.velocity = 0 // px/ms
        ;
        this.frames = 0;
        this.running = false;
    }
}
/** Clamps one frame's line output to +-rows; the excess is discarded, not carried. */ function clampLinesPerFrame(lines, rows) {
    return Math.max(-rows, Math.min(rows, lines));
}

// EXTERNAL MODULE: ./src/lib/terminal/scrollRouting.ts
var scrollRouting = __webpack_require__(26550);
// EXTERNAL MODULE: ./src/lib/terminal/mobileDebug.ts
var mobileDebug = __webpack_require__(28582);
;// ./src/lib/hooks/useTerminalGestures.ts
/* __next_internal_client_entry_do_not_use__ getCellDimensions,GESTURE_IGNORE_SELECTOR,useTerminalGestures auto */ /**
 * useTerminalGestures — Unified mobile gesture state machine for xterm.js terminals.
 *
 * Implements a gesture state machine (pure transition table in
 * lib/terminal/gestureMachine.ts; this hook wires listeners, timers, rAF and effects):
 *   IDLE → PENDING → SCROLLING (→ COASTING) | SELECTING | TAPPING → IDLE   (CANCELLED on interrupts)
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







// Re-export for consumers that import from this module

/** Touches that begin on these (xterm's own scrollbar, anything opted out) never start a gesture. */ const GESTURE_IGNORE_SELECTOR = '[data-gesture-ignore], .xterm-scrollable-element .scrollbar';
/**
 * Attaches a unified touch gesture recognizer to an xterm.js container.
 *
 * Returns a cleanup function (for use in useEffect return or manually).
 */ function useTerminalGestures(options) {
    const { containerRef, terminalRef, gestureScrollEnabled = true, override = 'auto', connectionEpoch = 0 } = options;
    // Latest options for event handlers, so they never form stale closures.
    const optionsRef = (0,react.useRef)(options);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        optionsRef.current = options;
    });
    const cancelGestureRef = (0,react.useRef)(null);
    // Override / connection changes cancel an in-flight gesture (S9: reset remainders, ignore the touch).
    const previousRef = (0,react.useRef)({
        override,
        connectionEpoch
    });
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const prev = previousRef.current;
        if (prev.override !== override || prev.connectionEpoch !== connectionEpoch) {
            cancelGestureRef.current?.();
        }
        previousRef.current = {
            override,
            connectionEpoch
        };
    }, [
        override,
        connectionEpoch
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!gestureScrollEnabled) return;
        const containerEl = containerRef.current;
        if (!containerEl) return;
        const onSendData = (data)=>optionsRef.current.onSendData(data);
        const longPressMs = ()=>optionsRef.current.longPressMs ?? 400;
        // ---- State machine ----
        let state = 'IDLE';
        // Touch tracking
        let startX = 0;
        let startY = 0;
        let lastY = 0;
        let startTime = 0;
        let startCol = 0;
        let startRow = 0;
        let tapX = 0;
        let tapY = 0;
        let longPressTimer = null;
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
        let cellGeometry = null;
        let cachedCellH = 0;
        // Quantization + routing state for the current scroll gesture.
        const lineAcc = new ScrollAccumulator();
        const pageAcc = new scrollRouting/* PageAccumulator */.yC({
            rows: 24,
            now: ()=>Date.now(),
            // The per-fling cap guards momentum, not a finger that is still down (Story 1.2.2 owns it).
            flingCap: Number.POSITIVE_INFINITY
        });
        let lastTarget = null;
        // Per-drag summary for onScrollGesture.
        let dragActive = false;
        let dragStartViewportY = 0;
        let dragViewportYChanged = false;
        let dragPostSlopLines = 0;
        let dragRoute = 'xterm-local';
        let gestureActive = false;
        // Emits only when activity flips; called after every state change.
        const syncActive = ()=>{
            const active = state === 'PENDING' || state === 'SCROLLING' || state === 'SELECTING' || state === 'COASTING';
            if (active === gestureActive) return;
            gestureActive = active;
            optionsRef.current.onGestureActiveChange?.(active);
        };
        const sampleViewportY = ()=>{
            const y = terminalRef.current?.buffer?.active?.viewportY;
            if (y !== undefined && y !== dragStartViewportY) dragViewportYChanged = true;
        };
        const reportScrollGesture = ()=>{
            if (!dragActive || state !== 'SCROLLING') return;
            dragActive = false;
            sampleViewportY();
            optionsRef.current.onScrollGesture?.({
                route: dragRoute,
                postSlopLines: dragPostSlopLines,
                viewportYChanged: dragViewportYChanged
            });
        };
        // Release applies the rAF-coalesced last point (up to a frame of travel) before the gesture is summarized and torn down.
        const flushAndReportScroll = ()=>{
            if (state === 'SCROLLING') flushScrollThrottle?.();
            reportScrollGesture();
        };
        // Momentum (COASTING). The page accumulator is separate so only the fling is capped.
        const tracker = new MomentumTracker();
        let lastSampleAt = 0;
        let reducedMotion = false;
        let consumedByCoast = false;
        let flingPageAcc = null;
        let momentumTarget = null;
        let momentumRaf = null;
        let momentumLastFrameAt = 0;
        let bufferChangeSub = null;
        const MAX_FRAME_DT_MS = 50;
        let scrollThrottled = null;
        let cancelScrollThrottle = null;
        let flushScrollThrottle = null;
        let selectThrottled = null;
        let cancelSelectThrottle = null;
        let flushSelectThrottle = null;
        let lastSelectX = 0;
        let lastSelectY = 0;
        // mobileDebug.isEnabled reads localStorage; sample it once per touch so per-frame logging allocates nothing when off.
        let debugOn = false;
        const cancelPendingFrames = ()=>{
            cancelScrollThrottle?.();
            cancelSelectThrottle?.();
            scrollThrottled = null;
            cancelScrollThrottle = null;
            flushScrollThrottle = null;
            selectThrottled = null;
            cancelSelectThrottle = null;
            flushSelectThrottle = null;
        };
        const clearLongPressTimer = ()=>{
            if (longPressTimer !== null) {
                clearTimeout(longPressTimer);
                longPressTimer = null;
            }
        };
        // Mouse-tracking-aware mode check — delegates to shared utility.
        const isMouseTracking = ()=>{
            const t = terminalRef.current;
            if (!t) return false;
            return mouseTracking_isMouseTracking(t);
        };
        const getScreenEl = ()=>containerEl.querySelector('.xterm-screen');
        // ---- Momentum helpers ----
        const cancelMomentum = ()=>{
            if (momentumRaf !== null) {
                cancelAnimationFrame(momentumRaf);
                momentumRaf = null;
            }
            bufferChangeSub?.dispose();
            bufferChangeSub = null;
            tracker.cancel();
            flingPageAcc = null;
            momentumTarget = null;
        };
        // ---- Transition helpers ----
        // Stops timers, frames, momentum and remainders; the machine decides the resulting state.
        const stopGesture = ()=>{
            clearLongPressTimer();
            cancelPendingFrames();
            cancelMomentum();
            lineAcc.reset();
            pageAcc.reset();
            lastTarget = null;
        };
        // Non-mouse-tracking drag: xterm owns selection via real DOM mouse events. Coalesce
        // touchmove-driven mousemove dispatches to one per frame — xterm's own
        // SelectionService re-renders on every dispatched mousemove, which otherwise stacks
        // on top of our own per-event work.
        const beginSyntheticMouseSelection = ()=>{
            [selectThrottled, cancelSelectThrottle, flushSelectThrottle] = rafThrottlePoint((clientX, clientY)=>{
                getScreenEl()?.dispatchEvent(new MouseEvent('mousemove', {
                    clientX,
                    clientY,
                    bubbles: true,
                    cancelable: true,
                    button: 0,
                    buttons: 1
                }));
            });
            getScreenEl()?.dispatchEvent(new MouseEvent('mousedown', {
                clientX: startX,
                clientY: startY,
                bubbles: true,
                cancelable: true,
                button: 0,
                buttons: 1
            }));
        };
        // Mouse-tracking-mode drag: bypass xterm's mouse handling and call the public
        // select() API directly. Cache rect/cell geometry once for the whole drag —
        // re-measuring on every touchmove is the main source of the jump-during-drag feel
        // on Android.
        const beginDirectSelectDrag = (t, el)=>{
            const { cellH, cellW } = getCellDimensions(t);
            cellGeometry = {
                rect: el.getBoundingClientRect(),
                cellW,
                cellH,
                maxCol: t.cols - 1,
                maxRow: t.rows - 1
            };
            [selectThrottled, cancelSelectThrottle, flushSelectThrottle] = rafThrottlePoint((clientX, clientY)=>{
                if (!cellGeometry) return;
                const { col: currentCol, row: currentRow } = pointToCell(clientX, clientY, cellGeometry);
                const length = Math.max(1, (currentRow - startRow) * t.cols + (currentCol - startCol) + 1);
                t.select(startCol, startRow, length);
            });
            t.select(startCol, startRow, 1);
        };
        const enterSelecting = ()=>{
            const t = terminalRef.current;
            if (!t) {
                state = 'IDLE';
                stopGesture();
                syncActive();
                return;
            }
            clearLongPressTimer();
            // Release fallback for a cancel before any drag point arrives.
            lastSelectX = startX;
            lastSelectY = startY;
            // Haptic feedback if available (R4.3)
            navigator.vibrate?.(10);
            if (!isMouseTracking()) {
                beginSyntheticMouseSelection();
            } else if (t.element) {
                beginDirectSelectDrag(t, t.element);
            }
        };
        // ---- Effect executor: runs the machine's effects in order ----
        const runEffects = (effects, e, touch)=>{
            for (const effect of effects){
                switch(effect){
                    case 'abort':
                        stopGesture();
                        break;
                    case 'startLongPressTimer':
                        longPressTimer = setTimeout(()=>{
                            longPressTimer = null;
                            dispatch({
                                type: 'longPress'
                            }, null, undefined);
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
                        if (touch) {
                            lastSelectX = touch.clientX;
                            lastSelectY = touch.clientY;
                            selectThrottled?.(touch.clientX, touch.clientY);
                        }
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
                    case 'clearSelection':
                        clearSelection();
                        break;
                    case 'startMomentum':
                        startMomentum();
                        break;
                    case 'setConsumedByCoast':
                        consumedByCoast = true;
                        break;
                }
            }
        };
        const dispatch = (event, e, touch)=>{
            const next = reduce(state, event);
            state = next.state;
            runEffects(next.effects, e, touch);
            syncActive();
        };
        const preventDefaultIfCancelable = (e)=>{
            if (!e) return;
            if (e.cancelable) e.preventDefault();
            else mobileDebug/* mobileDebug */.Uo.log('not-cancelable', {
                event: e.type
            });
        };
        // Re-evaluated every frame; a target change resets the remainders and logs the decision.
        const resolveTarget = (mode)=>{
            const o = optionsRef.current;
            const logged = {};
            const target = (0,scrollRouting/* decideAndLogScrollTarget */.Ql)(mode, {
                policy: o.routingPolicy,
                tuiPolicy: o.tuiScrollPolicy,
                override: o.override,
                log: (entry)=>{
                    logged.entry = entry;
                }
            });
            if (target !== lastTarget) {
                if (lastTarget !== null) {
                    lineAcc.reset();
                    pageAcc.reset();
                }
                lastTarget = target;
                mobileDebug/* mobileDebug */.Uo.routeDecision({
                    target,
                    source: logged.entry?.source ?? 'auto',
                    unverified: !scrollRouting/* ROUTING_VERIFIED */.YA
                });
            }
            return target;
        };
        const dispatchScroll = (terminal, target, lines, pages = pageAcc)=>{
            switch(target){
                case 'xterm-local':
                    {
                        if (terminal.buffer?.active?.length === 0) return; // nothing to scroll; mirrors the jump button's empty-buffer rule
                        const clamped = clampLinesPerFrame(lines, terminal.rows);
                        if (clamped !== 0) terminal.scrollLines(clamped);
                        return;
                    }
                case 'tui-pgkeys':
                    {
                        if (optionsRef.current.isInputBusy?.()) {
                            // Not pushed to the accumulator, so a dropped step neither counts toward the fling cap nor carries over.
                            mobileDebug/* mobileDebug */.Uo.log('input-busy-drop', {
                                target,
                                lines
                            });
                            return;
                        }
                        pages.setRows(terminal.rows);
                        const keys = pages.push(lines);
                        if (keys) {
                            onSendData(keys);
                            const count = keys.split(scrollRouting/* PAGE_UP_BYTES */.UU).length - 1 + keys.split(scrollRouting/* PAGE_DOWN_BYTES */.Nl).length - 1;
                            const direction = keys.startsWith(scrollRouting/* PAGE_UP_BYTES */.UU) ? 'up' : 'down';
                            optionsRef.current.onPageKeysSent?.(direction, count);
                        }
                        return;
                    }
                case 'tui-wheel':
                    {
                        if (optionsRef.current.isInputBusy?.()) {
                            mobileDebug/* mobileDebug */.Uo.log('input-busy-drop', {
                                target,
                                lines
                            });
                            return;
                        }
                        const reports = (0,scrollRouting/* encodeWheel */.gU)(lines, {
                            col: startCol + 1,
                            row: startRow + 1,
                            cols: terminal.cols,
                            rows: terminal.rows
                        });
                        if (reports) onSendData(reports);
                        return;
                    }
            }
        };
        const onScrollFrame = (_clientX, clientY)=>{
            const terminal = terminalRef.current;
            if (!terminal || cachedCellH <= 0) return;
            const moveDy = clientY - lastY;
            lastY = clientY;
            const mode = readScrollMode(terminal);
            const target = resolveTarget(mode);
            const lines = lineAcc.push(-moveDy, cachedCellH);
            dragRoute = target;
            dragPostSlopLines += Math.abs(lines);
            const o = optionsRef.current;
            if (target === 'xterm-local' && lines < 0 && o.isAltScreenActive?.() && o.onAltScreenScrollUp) {
                o.onAltScreenScrollUp(-lines);
            } else {
                dispatchScroll(terminal, target, lines);
            }
            sampleViewportY();
            const active = terminal.buffer?.active;
            if (debugOn) {
                mobileDebug/* mobileDebug */.Uo.log('scroll', {
                    ...mode,
                    viewportY: active?.viewportY,
                    baseY: active?.baseY,
                    rows: terminal.rows,
                    cellH: cachedCellH,
                    moveDy,
                    lines,
                    target
                });
            }
        };
        // ---- Momentum loop: one rAF per frame, one dispatch per frame ----
        const endMomentumOnItsOwn = ()=>dispatch({
                type: 'momentumEnd'
            }, null, undefined);
        const atLocalEdge = (terminal, lines)=>{
            const active = terminal.buffer?.active;
            if (!active || lines === 0) return false;
            return lines < 0 ? active.viewportY <= 0 : active.viewportY >= active.baseY;
        };
        const momentumFrame = ()=>{
            momentumRaf = null;
            const terminal = terminalRef.current;
            if (state !== 'COASTING' || !terminal || cachedCellH <= 0 || !flingPageAcc) {
                endMomentumOnItsOwn();
                return;
            }
            const target = resolveTarget(readScrollMode(terminal));
            if (target !== momentumTarget) {
                endMomentumOnItsOwn(); // routing flipped without a buffer switch (e.g. mouse tracking toggled)
                return;
            }
            const now = Date.now();
            const dt = Math.min(Math.max(now - momentumLastFrameAt, 1), MAX_FRAME_DT_MS);
            momentumLastFrameAt = now;
            // step() keeps the sign of the sampled y movement; line deltas are positive toward newer (finger up).
            const lines = lineAcc.push(-tracker.step(dt), cachedCellH);
            // A TUI owns its scrollback, so only the local buffer has a detectable edge.
            if (target === 'xterm-local' && atLocalEdge(terminal, lines)) {
                endMomentumOnItsOwn();
                return;
            }
            dispatchScroll(terminal, target, lines, flingPageAcc);
            if (debugOn) mobileDebug/* mobileDebug */.Uo.log('momentum', {
                target,
                lines,
                active: tracker.active
            });
            if (!tracker.active || target === 'tui-pgkeys' && flingPageAcc.capped) {
                endMomentumOnItsOwn();
                return;
            }
            momentumRaf = requestAnimationFrame(momentumFrame);
        };
        const startMomentum = ()=>{
            const terminal = terminalRef.current;
            cancelPendingFrames(); // the finger-down frame; remainders stay for the first momentum frame
            if (!terminal || cachedCellH <= 0) {
                endMomentumOnItsOwn();
                return;
            }
            momentumTarget = resolveTarget(readScrollMode(terminal));
            flingPageAcc = new scrollRouting/* PageAccumulator */.yC({
                rows: terminal.rows,
                now: ()=>Date.now(),
                flingCap: scrollRouting/* PAGE_FLING_CAP */.Md
            });
            momentumLastFrameAt = Date.now();
            bufferChangeSub = terminal.buffer?.onBufferChange?.(()=>{
                if (state === 'COASTING') dispatch({
                    type: 'interrupt'
                }, null, undefined);
            }) ?? null;
            momentumRaf = requestAnimationFrame(momentumFrame);
        };
        // Release speed from recent samples; a finger that paused before lifting does not fling.
        const releaseStartsFling = ()=>{
            if (cachedCellH <= 0 || Date.now() - lastSampleAt > MOMENTUM_CONSTANTS.windowMs) {
                tracker.cancel();
                return false;
            }
            return tracker.release(reducedMotion);
        };
        const beginScroll = (touch)=>{
            const y = touch?.clientY ?? lastY;
            lastY = y;
            consumedByCoast = false;
            tracker.cancel();
            lineAcc.reset();
            pageAcc.reset();
            lastTarget = null;
            // Seed with the overshoot past the slop only, so crossing it emits no jump.
            const travel = y - startY;
            const overshoot = Math.sign(travel) * (Math.abs(travel) - SLOP_PX);
            lineAcc.seed(-overshoot);
            mobileDebug/* mobileDebug */.Uo.log('scroll-start', {
                startY,
                y,
                slopPx: SLOP_PX,
                overshoot
            });
            // Cache cell height once for the drag and coalesce touchmove into one
            // dispatch per frame — same rationale as the SELECTING geometry cache.
            const t = terminalRef.current;
            cachedCellH = t ? getCellDimensions(t).cellH : 0;
            [scrollThrottled, cancelScrollThrottle, flushScrollThrottle] = rafThrottlePoint(onScrollFrame);
            scrollThrottled(0, y); // one frame carries the seeded overshoot
            dragActive = true;
            dragPostSlopLines = 0;
            dragViewportYChanged = false;
            dragStartViewportY = t?.buffer?.active?.viewportY ?? 0;
            if (t && cachedCellH > 0) {
                dragRoute = resolveTarget(readScrollMode(t));
                optionsRef.current.onScrollStart?.(dragRoute);
            }
        };
        const hasSelection = (t)=>typeof t.hasSelection === 'function' ? t.hasSelection() : Boolean(t.getSelection?.());
        const clearSelection = ()=>{
            terminalRef.current?.clearSelection?.();
        };
        const handleTap = ()=>{
            const now = Date.now();
            const isDoubleTap = now - lastTapTime < DOUBLE_TAP_MS && Math.abs(tapX - lastTapX) < DOUBLE_TAP_RADIUS_PX && Math.abs(tapY - lastTapY) < DOUBLE_TAP_RADIUS_PX;
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
                    detail: 2
                }));
            } else if (t) {
                // Normal tap handling
                // Read via optionsRef: the gesture effect doesn't re-run when the toggle changes.
                const forwardMouseClicks = isMouseTracking() && optionsRef.current.mouseModeOverride !== 'none';
                if (!forwardMouseClicks) {
                    t.focus();
                } else if (t.element) {
                    const { cellH, cellW } = getCellDimensions(t);
                    const canvasRect = t.element.getBoundingClientRect();
                    const col = Math.floor((tapX - canvasRect.left) / cellW) + 1; // 1-based
                    const row = Math.floor((tapY - canvasRect.top) / cellH) + 1; // 1-based
                    if (optionsRef.current.isSgrMouseEncoding?.()) {
                        // SGR (?1006h): \x1b[<0;col;rowM press, ...m release; no coordinate limit.
                        const c = Math.max(1, col);
                        const r = Math.max(1, row);
                        onSendData(`\x1b[<0;${c};${r}M\x1b[<0;${c};${r}m`);
                    } else {
                        // X10 mouse encoding: \x1b[M + button(32=left-press) + col+32 + row+32
                        // Clamp col/row to 1-223 so charCode stays in 33-255 (valid X10 range)
                        const clampedCol = Math.max(1, Math.min(col, 223));
                        const clampedRow = Math.max(1, Math.min(row, 223));
                        const press = `\x1b[M${String.fromCharCode(32, clampedCol + 32, clampedRow + 32)}`;
                        const release = `\x1b[M${String.fromCharCode(35, clampedCol + 32, clampedRow + 32)}`; // 35 = release
                        onSendData(press + release);
                    }
                    t.focus();
                }
            }
            lastTapTime = now;
            lastTapX = tapX;
            lastTapY = tapY;
        };
        // `touch` is absent on touchcancel and timer-driven aborts; fall back to the last point the drag reported.
        const endSelecting = (touch)=>{
            const t = terminalRef.current;
            flushSelectThrottle?.(); // apply the last coalesced point before the button releases
            if (!isMouseTracking()) {
                getScreenEl()?.dispatchEvent(new MouseEvent('mouseup', {
                    clientX: touch?.clientX ?? lastSelectX,
                    clientY: touch?.clientY ?? lastSelectY,
                    bubbles: true,
                    cancelable: true,
                    button: 0,
                    buttons: 0
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
        const startedOnIgnoredSurface = (e)=>{
            const target = e.target;
            return Boolean(target?.closest?.(GESTURE_IGNORE_SELECTOR));
        };
        const onTouchStart = (e)=>{
            if (startedOnIgnoredSurface(e)) {
                dispatch({
                    type: 'touchcancel'
                }, null, undefined);
                return;
            }
            // A stale coast-stop flag never survives a touchstart that is not itself a touch-to-stop.
            if (state !== 'COASTING') consumedByCoast = false;
            debugOn = mobileDebug/* mobileDebug */.Uo.enabled();
            if (e.touches.length !== 1) reportScrollGesture();
            if (e.touches.length === 1) {
                reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
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
            dispatch({
                type: 'touchstart',
                touchCount: e.touches.length
            }, e, undefined);
        };
        // ---- touchmove (registered on document, passive: false to allow preventDefault) ----
        const onTouchMove = (e)=>{
            const touch = e.touches[0];
            const absDy = touch ? Math.abs(touch.clientY - startY) : 0;
            const absDx = touch ? Math.abs(touch.clientX - startX) : 0;
            // A second finger aborts the drag; report it while the state is still SCROLLING.
            if (e.touches.length !== 1) reportScrollGesture();
            dispatch({
                type: 'touchmove',
                touchCount: e.touches.length,
                absDx,
                absDy,
                slopPx: SLOP_PX
            }, e, touch);
            if (state === 'SCROLLING' && touch) {
                lastSampleAt = Date.now();
                tracker.addSample(lastSampleAt, touch.clientY);
            }
        };
        // ---- touchend (registered on document) ----
        const onTouchEnd = (e)=>{
            const touch = e.changedTouches[0];
            const t = terminalRef.current;
            const wasConsumedByCoast = consumedByCoast;
            consumedByCoast = false;
            flushAndReportScroll();
            dispatch({
                type: 'touchend',
                elapsedMs: Date.now() - startTime,
                totalDy: Math.abs((touch?.clientY ?? startY) - startY),
                totalDx: Math.abs((touch?.clientX ?? startX) - startX),
                longPressMs: longPressMs(),
                tapTolerancePx: SLOP_PX,
                flinging: state === 'SCROLLING' && releaseStartsFling(),
                consumedByCoast: wasConsumedByCoast,
                selectionActive: state === 'PENDING' && t !== null && hasSelection(t)
            }, e, touch);
        };
        // ---- touchcancel ----
        const onTouchCancel = ()=>{
            mobileDebug/* mobileDebug */.Uo.log('touchcancel', {
                state
            });
            consumedByCoast = false;
            reportScrollGesture();
            dispatch({
                type: 'touchcancel'
            }, null, undefined);
        };
        // Keyboard toggle / rotation mid-gesture: drop remainders and ignore the rest of the touch.
        const onInterrupt = ()=>dispatch({
                type: 'interrupt'
            }, null, undefined);
        // Register listeners:
        // - touchstart on containerEl (catches gesture origin)
        // - touchmove + touchend on document (handles drags outside container)
        // touchend is non-passive so it can preventDefault the synthesized click after a scroll.
        const visualViewport = window.visualViewport;
        containerEl.addEventListener('touchstart', onTouchStart, {
            passive: false
        });
        document.addEventListener('touchmove', onTouchMove, {
            passive: false
        });
        document.addEventListener('touchend', onTouchEnd, {
            passive: false
        });
        document.addEventListener('touchcancel', onTouchCancel, {
            passive: true
        });
        visualViewport?.addEventListener('resize', onInterrupt);
        window.addEventListener('orientationchange', onInterrupt);
        cancelGestureRef.current = onInterrupt;
        return ()=>{
            cancelGestureRef.current = null;
            if (gestureActive) {
                gestureActive = false;
                optionsRef.current.onGestureActiveChange?.(false);
            }
            clearLongPressTimer();
            cancelPendingFrames();
            cancelMomentum();
            containerEl.removeEventListener('touchstart', onTouchStart);
            document.removeEventListener('touchmove', onTouchMove);
            document.removeEventListener('touchend', onTouchEnd);
            document.removeEventListener('touchcancel', onTouchCancel);
            visualViewport?.removeEventListener('resize', onInterrupt);
            window.removeEventListener('orientationchange', onInterrupt);
        };
    }, [
        containerRef,
        terminalRef,
        gestureScrollEnabled
    ]); // options other than these are read via optionsRef
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+xterm@6.0.0/node_modules/@xterm/xterm/lib/xterm.mjs
var xterm = __webpack_require__(35986);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-fit@0.11.0/node_modules/@xterm/addon-fit/lib/addon-fit.mjs
var addon_fit = __webpack_require__(79212);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-web-links@0.12.0/node_modules/@xterm/addon-web-links/lib/addon-web-links.mjs
/**
 * Copyright (c) 2014-2024 The xterm.js authors. All rights reserved.
 * @license MIT
 *
 * Copyright (c) 2012-2013, Christopher Jeffrey (MIT License)
 * @license MIT
 *
 * Originally forked from (with the author's permission):
 *   Fabrice Bellard's javascript vt100 for jslinux:
 *   http://bellard.org/jslinux/
 *   Copyright (c) 2011 Fabrice Bellard
 */
/*---------------------------------------------------------------------------------------------
 *  Copyright (c) Microsoft Corporation. All rights reserved.
 *  Licensed under the MIT License. See License.txt in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
var v=class{constructor(e,t,n,o={}){this._terminal=e;this._regex=t;this._handler=n;this._options=o}provideLinks(e,t){let n=g.computeLink(e,this._regex,this._terminal,this._handler);t(this._addCallbacks(n))}_addCallbacks(e){return e.map(t=>(t.leave=this._options.leave,t.hover=(n,o)=>{if(this._options.hover){let{range:p}=t;this._options.hover(n,o,p)}},t))}};function k(l){try{let e=new URL(l),t=e.password&&e.username?`${e.protocol}//${e.username}:${e.password}@${e.host}`:e.username?`${e.protocol}//${e.username}@${e.host}`:`${e.protocol}//${e.host}`;return l.toLocaleLowerCase().startsWith(t.toLocaleLowerCase())}catch{return!1}}var g=class l{static computeLink(e,t,n,o){let p=new RegExp(t.source,(t.flags||"")+"g"),[i,r]=l._getWindowedLineStrings(e-1,n),s=i.join(""),a,d=[];for(;a=p.exec(s);){let u=a[0];if(!k(u))continue;let[c,h]=l._mapStrIdx(n,r,0,a.index),[m,f]=l._mapStrIdx(n,c,h,u.length);if(c===-1||h===-1||m===-1||f===-1)continue;let b={start:{x:h+1,y:c+1},end:{x:f,y:m+1}};d.push({range:b,text:u,activate:o})}return d}static _getWindowedLineStrings(e,t){let n,o=e,p=e,i=0,r="",s=[];if(n=t.buffer.active.getLine(e)){let a=n.translateToString(!0);if(n.isWrapped&&a[0]!==" "){for(i=0;(n=t.buffer.active.getLine(--o))&&i<2048&&(r=n.translateToString(!0),i+=r.length,s.push(r),!(!n.isWrapped||r.indexOf(" ")!==-1)););s.reverse()}for(s.push(a),i=0;(n=t.buffer.active.getLine(++p))&&n.isWrapped&&i<2048&&(r=n.translateToString(!0),i+=r.length,s.push(r),r.indexOf(" ")===-1););}return[s,o]}static _mapStrIdx(e,t,n,o){let p=e.buffer.active,i=p.getNullCell(),r=n;for(;o;){let s=p.getLine(t);if(!s)return[-1,-1];for(let a=r;a<s.length;++a){s.getCell(a,i);let d=i.getChars();if(i.getWidth()&&(o-=d.length||1,a===s.length-1&&d==="")){let c=p.getLine(t+1);c&&c.isWrapped&&(c.getCell(0,i),i.getWidth()===2&&(o+=1))}if(o<0)return[t,a]}t++,r=0}return[t,r]}};var _=/(https?|HTTPS?):[/]{2}[^\s"'!*(){}|\\\^<>`]*[^\s"':,.!?{}|\\\^~\[\]`()<>]/;function w(l,e){let t=window.open();if(t){try{t.opener=null}catch{}t.location.href=e}else console.warn("Opening link blocked as opener could not be cleared")}var L=class{constructor(e=w,t={}){this._handler=e;this._options=t}activate(e){this._terminal=e;let t=this._options,n=t.urlRegex||_;this._linkProvider=this._terminal.registerLinkProvider(new v(this._terminal,n,this._handler,t))}dispose(){this._linkProvider?.dispose()}};
//# sourceMappingURL=addon-web-links.mjs.map

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-canvas@0.7.0+48a5bd62dd64c178/node_modules/@xterm/addon-canvas/lib/addon-canvas.js
var addon_canvas = __webpack_require__(76029);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-search@0.16.0/node_modules/@xterm/addon-search/lib/addon-search.mjs
/**
 * Copyright (c) 2014-2024 The xterm.js authors. All rights reserved.
 * @license MIT
 *
 * Copyright (c) 2012-2013, Christopher Jeffrey (MIT License)
 * @license MIT
 *
 * Originally forked from (with the author's permission):
 *   Fabrice Bellard's javascript vt100 for jslinux:
 *   http://bellard.org/jslinux/
 *   Copyright (c) 2011 Fabrice Bellard
 */
/*---------------------------------------------------------------------------------------------
 *  Copyright (c) Microsoft Corporation. All rights reserved.
 *  Licensed under the MIT License. See License.txt in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
var Re=class{constructor(){this.listeners=[],this.unexpectedErrorHandler=function(e){setTimeout(()=>{throw e.stack?ae.isErrorNoTelemetry(e)?new ae(e.message+`

`+e.stack):new Error(e.message+`

`+e.stack):e},0)}}addListener(e){return this.listeners.push(e),()=>{this._removeListener(e)}}emit(e){this.listeners.forEach(t=>{t(e)})}_removeListener(e){this.listeners.splice(this.listeners.indexOf(e),1)}setUnexpectedErrorHandler(e){this.unexpectedErrorHandler=e}getUnexpectedErrorHandler(){return this.unexpectedErrorHandler}onUnexpectedError(e){this.unexpectedErrorHandler(e),this.emit(e)}onUnexpectedExternalError(e){this.unexpectedErrorHandler(e)}},pt=new Re;function le(r){ft(r)||pt.onUnexpectedError(r)}var Ce="Canceled";function ft(r){return r instanceof ee?!0:r instanceof Error&&r.name===Ce&&r.message===Ce}var ee=class extends Error{constructor(){super(Ce),this.name=this.message}};var ae=class r extends Error{constructor(e){super(e),this.name="CodeExpectedError"}static fromError(e){if(e instanceof r)return e;let t=new r;return t.message=e.message,t.stack=e.stack,t}static isErrorNoTelemetry(e){return e.name==="CodeExpectedError"}};function mt(r,e,t=0,n=r.length){let i=t,s=n;for(;i<s;){let a=Math.floor((i+s)/2);e(r[a])?i=a+1:s=a}return i-1}var ue=class ue{constructor(e){this._array=e;this._findLastMonotonousLastIdx=0}findLastMonotonous(e){if(ue.assertInvariants){if(this._prevFindLastPredicate){for(let n of this._array)if(this._prevFindLastPredicate(n)&&!e(n))throw new Error("MonotonousArray: current predicate must be weaker than (or equal to) the previous predicate.")}this._prevFindLastPredicate=e}let t=mt(this._array,e,this._findLastMonotonousLastIdx);return this._findLastMonotonousLastIdx=t+1,t===-1?void 0:this._array[t]}};ue.assertInvariants=!1;var Ke=(/* unused pure expression or super */ null && (ue));var Qe;(h=>{function r(u){return u<0}h.isLessThan=r;function e(u){return u<=0}h.isLessThanOrEqual=e;function t(u){return u>0}h.isGreaterThan=t;function n(u){return u===0}h.isNeitherLessOrGreaterThan=n,h.greaterThan=1,h.lessThan=-1,h.neitherLessOrGreaterThan=0})(Qe||={});function $e(r,e){return(t,n)=>e(r(t),r(n))}var Be=(r,e)=>r-e;var V=class V{constructor(e){this.iterate=e}forEach(e){this.iterate(t=>(e(t),!0))}toArray(){let e=[];return this.iterate(t=>(e.push(t),!0)),e}filter(e){return new V(t=>this.iterate(n=>e(n)?t(n):!0))}map(e){return new V(t=>this.iterate(n=>t(e(n))))}some(e){let t=!1;return this.iterate(n=>(t=e(n),!t)),t}findFirst(e){let t;return this.iterate(n=>e(n)?(t=n,!1):!0),t}findLast(e){let t;return this.iterate(n=>(e(n)&&(t=n),!0)),t}findLastMaxBy(e){let t,n=!0;return this.iterate(i=>((n||Qe.isGreaterThan(e(i,t)))&&(n=!1,t=i),!0)),t}};V.empty=new V(e=>{});var He=(/* unused pure expression or super */ null && (V));function Xe(r,e){let t=Object.create(null);for(let n of r){let i=e(n),s=t[i];s||(s=t[i]=[]),s.push(n)}return t}var Ye,Je,Ge=class{constructor(e,t){this.toKey=t;this._map=new Map;this[Ye]="SetWithKey";for(let n of e)this.add(n)}get size(){return this._map.size}add(e){let t=this.toKey(e);return this._map.set(t,e),this}delete(e){return this._map.delete(this.toKey(e))}has(e){return this._map.has(this.toKey(e))}*entries(){for(let e of this._map.values())yield[e,e]}keys(){return this.values()}*values(){for(let e of this._map.values())yield e}clear(){this._map.clear()}forEach(e,t){this._map.forEach(n=>e.call(t,n,n,this))}[(Je=Symbol.iterator,Ye=Symbol.toStringTag,Je)](){return this.values()}};var ce=class{constructor(){this.map=new Map}add(e,t){let n=this.map.get(e);n||(n=new Set,this.map.set(e,n)),n.add(t)}delete(e,t){let n=this.map.get(e);n&&(n.delete(t),n.size===0&&this.map.delete(e))}forEach(e,t){let n=this.map.get(e);n&&n.forEach(t)}get(e){let t=this.map.get(e);return t||new Set}};function Pe(r,e){let t=this,n=!1,i;return function(){if(n)return i;if(n=!0,e)try{i=r.apply(t,arguments)}finally{e()}else i=r.apply(t,arguments);return i}}var Le;(z=>{function r(m){return m&&typeof m=="object"&&typeof m[Symbol.iterator]=="function"}z.is=r;let e=Object.freeze([]);function t(){return e}z.empty=t;function*n(m){yield m}z.single=n;function i(m){return r(m)?m:n(m)}z.wrap=i;function s(m){return m||e}z.from=s;function*a(m){for(let _=m.length-1;_>=0;_--)yield m[_]}z.reverse=a;function h(m){return!m||m[Symbol.iterator]().next().done===!0}z.isEmpty=h;function u(m){return m[Symbol.iterator]().next().value}z.first=u;function p(m,_){let y=0;for(let L of m)if(_(L,y++))return!0;return!1}z.some=p;function T(m,_){for(let y of m)if(_(y))return y}z.find=T;function*v(m,_){for(let y of m)_(y)&&(yield y)}z.filter=v;function*I(m,_){let y=0;for(let L of m)yield _(L,y++)}z.map=I;function*E(m,_){let y=0;for(let L of m)yield*_(L,y++)}z.flatMap=E;function*S(...m){for(let _ of m)yield*_}z.concat=S;function D(m,_,y){let L=y;for(let X of m)L=_(L,X);return L}z.reduce=D;function*x(m,_,y=m.length){for(_<0&&(_+=m.length),y<0?y+=m.length:y>m.length&&(y=m.length);_<y;_++)yield m[_]}z.slice=x;function J(m,_=Number.POSITIVE_INFINITY){let y=[];if(_===0)return[y,m];let L=m[Symbol.iterator]();for(let X=0;X<_;X++){let Se=L.next();if(Se.done)return[y,z.empty()];y.push(Se.value)}return[y,{[Symbol.iterator](){return L}}]}z.consume=J;async function q(m){let _=[];for await(let y of m)_.push(y);return Promise.resolve(_)}z.asyncToArray=q})(Le||={});var Tt=!1,K=null,de=class de{constructor(){this.livingDisposables=new Map}getDisposableData(e){let t=this.livingDisposables.get(e);return t||(t={parent:null,source:null,isSingleton:!1,value:e,idx:de.idx++},this.livingDisposables.set(e,t)),t}trackDisposable(e){let t=this.getDisposableData(e);t.source||(t.source=new Error().stack)}setParent(e,t){let n=this.getDisposableData(e);n.parent=t}markAsDisposed(e){this.livingDisposables.delete(e)}markAsSingleton(e){this.getDisposableData(e).isSingleton=!0}getRootParent(e,t){let n=t.get(e);if(n)return n;let i=e.parent?this.getRootParent(this.getDisposableData(e.parent),t):e;return t.set(e,i),i}getTrackedDisposables(){let e=new Map;return[...this.livingDisposables.entries()].filter(([,n])=>n.source!==null&&!this.getRootParent(n,e).isSingleton).flatMap(([n])=>n)}computeLeakingDisposables(e=10,t){let n;if(t)n=t;else{let u=new Map,p=[...this.livingDisposables.values()].filter(v=>v.source!==null&&!this.getRootParent(v,u).isSingleton);if(p.length===0)return;let T=new Set(p.map(v=>v.value));if(n=p.filter(v=>!(v.parent&&T.has(v.parent))),n.length===0)throw new Error("There are cyclic diposable chains!")}if(!n)return;function i(u){function p(v,I){for(;v.length>0&&I.some(E=>typeof E=="string"?E===v[0]:v[0].match(E));)v.shift()}let T=u.source.split(`
`).map(v=>v.trim().replace("at ","")).filter(v=>v!=="");return p(T,["Error",/^trackDisposable \(.*\)$/,/^DisposableTracker.trackDisposable \(.*\)$/]),T.reverse()}let s=new ce;for(let u of n){let p=i(u);for(let T=0;T<=p.length;T++)s.add(p.slice(0,T).join(`
`),u)}n.sort($e(u=>u.idx,Be));let a="",h=0;for(let u of n.slice(0,e)){h++;let p=i(u),T=[];for(let v=0;v<p.length;v++){let I=p[v];I=`(shared with ${s.get(p.slice(0,v+1).join(`
`)).size}/${n.length} leaks) at ${I}`;let S=s.get(p.slice(0,v).join(`
`)),D=Xe([...S].map(x=>i(x)[v]),x=>x);delete D[p[v]];for(let[x,J]of Object.entries(D))T.unshift(`    - stacktraces of ${J.length} other leaks continue with ${x}`);T.unshift(I)}a+=`


==================== Leaking disposable ${h}/${n.length}: ${u.value.constructor.name} ====================
${T.join(`
`)}
============================================================

`}return n.length>e&&(a+=`


... and ${n.length-e} more leaking disposables

`),{leaks:n,details:a}}};de.idx=0;var Ze=(/* unused pure expression or super */ null && (de));function vt(r){K=r}if(Tt){let r="__is_disposable_tracked__";vt(new class{trackDisposable(e){let t=new Error("Potentially leaked disposable").stack;setTimeout(()=>{e[r]||console.log(t)},3e3)}setParent(e,t){if(e&&e!==addon_search_k.None)try{e[r]=!0}catch{}}markAsDisposed(e){if(e&&e!==addon_search_k.None)try{e[r]=!0}catch{}}markAsSingleton(e){}})}function pe(r){return K?.trackDisposable(r),r}function fe(r){K?.markAsDisposed(r)}function te(r,e){K?.setParent(r,e)}function bt(r,e){if(K)for(let t of r)K.setParent(t,e)}function Q(r){if(Le.is(r)){let e=[];for(let t of r)if(t)try{t.dispose()}catch(n){e.push(n)}if(e.length===1)throw e[0];if(e.length>1)throw new AggregateError(e,"Encountered errors while disposing of store");return Array.isArray(r)?[]:r}else if(r)return r.dispose(),r}function me(...r){let e=A(()=>Q(r));return bt(r,e),e}function A(r){let e=pe({dispose:Pe(()=>{fe(e),r()})});return e}var he=class he{constructor(){this._toDispose=new Set;this._isDisposed=!1;pe(this)}dispose(){this._isDisposed||(fe(this),this._isDisposed=!0,this.clear())}get isDisposed(){return this._isDisposed}clear(){if(this._toDispose.size!==0)try{Q(this._toDispose)}finally{this._toDispose.clear()}}add(e){if(!e)return e;if(e===this)throw new Error("Cannot register a disposable on itself!");return te(e,this),this._isDisposed?he.DISABLE_DISPOSED_WARNING||console.warn(new Error("Trying to add a disposable to a DisposableStore that has already been disposed of. The added object will be leaked!").stack):this._toDispose.add(e),e}delete(e){if(e){if(e===this)throw new Error("Cannot dispose a disposable on itself!");this._toDispose.delete(e),e.dispose()}}deleteAndLeak(e){e&&this._toDispose.has(e)&&(this._toDispose.delete(e),te(e,null))}};he.DISABLE_DISPOSED_WARNING=!1;var H=he,addon_search_k=class{constructor(){this._store=new H;pe(this),te(this._store,this)}dispose(){fe(this),this._store.dispose()}_register(e){if(e===this)throw new Error("Cannot register a disposable on itself!");return this._store.add(e)}};addon_search_k.None=Object.freeze({dispose(){}});var F=class{constructor(){this._isDisposed=!1;pe(this)}get value(){return this._isDisposed?void 0:this._value}set value(e){this._isDisposed||e===this._value||(this._value?.dispose(),e&&te(e,this),this._value=e)}clear(){this.value=void 0}dispose(){this._isDisposed=!0,fe(this),this._value?.dispose(),this._value=void 0}clearAndLeak(){let e=this._value;return this._value=void 0,e&&te(e,null),e}};var $=class ${constructor(e){this.element=e,this.next=$.Undefined,this.prev=$.Undefined}};$.Undefined=new $(void 0);var et=(/* unused pure expression or super */ null && ($));var _t=globalThis.performance&&typeof globalThis.performance.now=="function",Te=class r{static create(e){return new r(e)}constructor(e){this._now=_t&&e===!1?Date.now:globalThis.performance.now.bind(globalThis.performance),this._startTime=this._now(),this._stopTime=-1}stop(){this._stopTime=this._now()}reset(){this._startTime=this._now(),this._stopTime=-1}elapsed(){return this._stopTime!==-1?this._stopTime-this._startTime:this._now()-this._startTime}};var gt=!1,tt=!1,yt=!1,ie;(re=>{re.None=()=>addon_search_k.None;function e(d){if(yt){let{onDidAddListener:o}=d,c=ne.create(),l=0;d.onDidAddListener=()=>{++l===2&&(console.warn("snapshotted emitter LIKELY used public and SHOULD HAVE BEEN created with DisposableStore. snapshotted here"),c.print()),o?.()}}}function t(d,o){return I(d,()=>{},0,void 0,!0,void 0,o)}re.defer=t;function n(d){return(o,c=null,l)=>{let f=!1,b;return b=d(g=>{if(!f)return b?b.dispose():f=!0,o.call(c,g)},null,l),f&&b.dispose(),b}}re.once=n;function i(d,o,c){return T((l,f=null,b)=>d(g=>l.call(f,o(g)),null,b),c)}re.map=i;function s(d,o,c){return T((l,f=null,b)=>d(g=>{o(g),l.call(f,g)},null,b),c)}re.forEach=s;function a(d,o,c){return T((l,f=null,b)=>d(g=>o(g)&&l.call(f,g),null,b),c)}re.filter=a;function h(d){return d}re.signal=h;function u(...d){return(o,c=null,l)=>{let f=me(...d.map(b=>b(g=>o.call(c,g))));return v(f,l)}}re.any=u;function p(d,o,c,l){let f=c;return i(d,b=>(f=o(f,b),f),l)}re.reduce=p;function T(d,o){let c,l={onWillAddFirstListener(){c=d(f.fire,f)},onDidRemoveLastListener(){c?.dispose()}};o||e(l);let f=new C(l);return o?.add(f),f.event}function v(d,o){return o instanceof Array?o.push(d):o&&o.add(d),d}function I(d,o,c=100,l=!1,f=!1,b,g){let w,R,U,se=0,Z,Ve={leakWarningThreshold:b,onWillAddFirstListener(){w=d(dt=>{se++,R=o(R,dt),l&&!U&&(oe.fire(R),R=void 0),Z=()=>{let ht=R;R=void 0,U=void 0,(!l||se>1)&&oe.fire(ht),se=0},typeof c=="number"?(clearTimeout(U),U=setTimeout(Z,c)):U===void 0&&(U=0,queueMicrotask(Z))})},onWillRemoveListener(){f&&se>0&&Z?.()},onDidRemoveLastListener(){Z=void 0,w.dispose()}};g||e(Ve);let oe=new C(Ve);return g?.add(oe),oe.event}re.debounce=I;function E(d,o=0,c){return re.debounce(d,(l,f)=>l?(l.push(f),l):[f],o,void 0,!0,void 0,c)}re.accumulate=E;function S(d,o=(l,f)=>l===f,c){let l=!0,f;return a(d,b=>{let g=l||!o(b,f);return l=!1,f=b,g},c)}re.latch=S;function D(d,o,c){return[re.filter(d,o,c),re.filter(d,l=>!o(l),c)]}re.split=D;function x(d,o=!1,c=[],l){let f=c.slice(),b=d(R=>{f?f.push(R):w.fire(R)});l&&l.add(b);let g=()=>{f?.forEach(R=>w.fire(R)),f=null},w=new C({onWillAddFirstListener(){b||(b=d(R=>w.fire(R)),l&&l.add(b))},onDidAddFirstListener(){f&&(o?setTimeout(g):g())},onDidRemoveLastListener(){b&&b.dispose(),b=null}});return l&&l.add(w),w.event}re.buffer=x;function J(d,o){return(l,f,b)=>{let g=o(new z);return d(function(w){let R=g.evaluate(w);R!==q&&l.call(f,R)},void 0,b)}}re.chain=J;let q=Symbol("HaltChainable");class z{constructor(){this.steps=[]}map(o){return this.steps.push(o),this}forEach(o){return this.steps.push(c=>(o(c),c)),this}filter(o){return this.steps.push(c=>o(c)?c:q),this}reduce(o,c){let l=c;return this.steps.push(f=>(l=o(l,f),l)),this}latch(o=(c,l)=>c===l){let c=!0,l;return this.steps.push(f=>{let b=c||!o(f,l);return c=!1,l=f,b?f:q}),this}evaluate(o){for(let c of this.steps)if(o=c(o),o===q)break;return o}}function m(d,o,c=l=>l){let l=(...w)=>g.fire(c(...w)),f=()=>d.on(o,l),b=()=>d.removeListener(o,l),g=new C({onWillAddFirstListener:f,onDidRemoveLastListener:b});return g.event}re.fromNodeEventEmitter=m;function _(d,o,c=l=>l){let l=(...w)=>g.fire(c(...w)),f=()=>d.addEventListener(o,l),b=()=>d.removeEventListener(o,l),g=new C({onWillAddFirstListener:f,onDidRemoveLastListener:b});return g.event}re.fromDOMEventEmitter=_;function y(d){return new Promise(o=>n(d)(o))}re.toPromise=y;function L(d){let o=new C;return d.then(c=>{o.fire(c)},()=>{o.fire(void 0)}).finally(()=>{o.dispose()}),o.event}re.fromPromise=L;function X(d,o){return d(c=>o.fire(c))}re.forward=X;function Se(d,o,c){return o(c),d(l=>o(l))}re.runAndSubscribe=Se;class ct{constructor(o,c){this._observable=o;this._counter=0;this._hasChanged=!1;let l={onWillAddFirstListener:()=>{o.addObserver(this)},onDidRemoveLastListener:()=>{o.removeObserver(this)}};c||e(l),this.emitter=new C(l),c&&c.add(this.emitter)}beginUpdate(o){this._counter++}handlePossibleChange(o){}handleChange(o,c){this._hasChanged=!0}endUpdate(o){this._counter--,this._counter===0&&(this._observable.reportChanges(),this._hasChanged&&(this._hasChanged=!1,this.emitter.fire(this._observable.get())))}}function zt(d,o){return new ct(d,o).emitter.event}re.fromObservable=zt;function Ut(d){return(o,c,l)=>{let f=0,b=!1,g={beginUpdate(){f++},endUpdate(){f--,f===0&&(d.reportChanges(),b&&(b=!1,o.call(c)))},handlePossibleChange(){},handleChange(){b=!0}};d.addObserver(g),d.reportChanges();let w={dispose(){d.removeObserver(g)}};return l instanceof H?l.add(w):Array.isArray(l)&&l.push(w),w}}re.fromObservableLight=Ut})(ie||={});var B=class B{constructor(e){this.listenerCount=0;this.invocationCount=0;this.elapsedOverall=0;this.durations=[];this.name=`${e}_${B._idPool++}`,B.all.add(this)}start(e){this._stopWatch=new Te,this.listenerCount=e}stop(){if(this._stopWatch){let e=this._stopWatch.elapsed();this.durations.push(e),this.elapsedOverall+=e,this.invocationCount+=1,this._stopWatch=void 0}}};B.all=new Set,B._idPool=0;var Oe=B,nt=-1;var be=class be{constructor(e,t,n=(be._idPool++).toString(16).padStart(3,"0")){this._errorHandler=e;this.threshold=t;this.name=n;this._warnCountdown=0}dispose(){this._stacks?.clear()}check(e,t){let n=this.threshold;if(n<=0||t<n)return;this._stacks||(this._stacks=new Map);let i=this._stacks.get(e.value)||0;if(this._stacks.set(e.value,i+1),this._warnCountdown-=1,this._warnCountdown<=0){this._warnCountdown=n*.5;let[s,a]=this.getMostFrequentStack(),h=`[${this.name}] potential listener LEAK detected, having ${t} listeners already. MOST frequent listener (${a}):`;console.warn(h),console.warn(s);let u=new Me(h,s);this._errorHandler(u)}return()=>{let s=this._stacks.get(e.value)||0;this._stacks.set(e.value,s-1)}}getMostFrequentStack(){if(!this._stacks)return;let e,t=0;for(let[n,i]of this._stacks)(!e||t<i)&&(e=[n,i],t=i);return e}};be._idPool=1;var Ae=be,ne=class r{constructor(e){this.value=e}static create(){let e=new Error;return new r(e.stack??"")}print(){console.warn(this.value.split(`
`).slice(2).join(`
`))}},Me=class extends Error{constructor(e,t){super(e),this.name="ListenerLeakError",this.stack=t}},Fe=class extends Error{constructor(e,t){super(e),this.name="ListenerRefusalError",this.stack=t}},xt=0,G=class{constructor(e){this.value=e;this.id=xt++}},It=2,Dt=(r,e)=>{if(r instanceof G)e(r);else for(let t=0;t<r.length;t++){let n=r[t];n&&e(n)}},ve;if(gt){let r=[];setInterval(()=>{r.length!==0&&(console.warn("[LEAKING LISTENERS] GC'ed these listeners that were NOT yet disposed:"),console.warn(r.join(`
`)),r.length=0)},3e3),ve=new FinalizationRegistry(e=>{typeof e=="string"&&r.push(e)})}var C=class{constructor(e){this._size=0;this._options=e,this._leakageMon=nt>0||this._options?.leakWarningThreshold?new Ae(e?.onListenerError??le,this._options?.leakWarningThreshold??nt):void 0,this._perfMon=this._options?._profName?new Oe(this._options._profName):void 0,this._deliveryQueue=this._options?.deliveryQueue}dispose(){if(!this._disposed){if(this._disposed=!0,this._deliveryQueue?.current===this&&this._deliveryQueue.reset(),this._listeners){if(tt){let e=this._listeners;queueMicrotask(()=>{Dt(e,t=>t.stack?.print())})}this._listeners=void 0,this._size=0}this._options?.onDidRemoveLastListener?.(),this._leakageMon?.dispose()}}get event(){return this._event??=(e,t,n)=>{if(this._leakageMon&&this._size>this._leakageMon.threshold**2){let u=`[${this._leakageMon.name}] REFUSES to accept new listeners because it exceeded its threshold by far (${this._size} vs ${this._leakageMon.threshold})`;console.warn(u);let p=this._leakageMon.getMostFrequentStack()??["UNKNOWN stack",-1],T=new Fe(`${u}. HINT: Stack shows most frequent listener (${p[1]}-times)`,p[0]);return(this._options?.onListenerError||le)(T),addon_search_k.None}if(this._disposed)return addon_search_k.None;t&&(e=e.bind(t));let i=new G(e),s,a;this._leakageMon&&this._size>=Math.ceil(this._leakageMon.threshold*.2)&&(i.stack=ne.create(),s=this._leakageMon.check(i.stack,this._size+1)),tt&&(i.stack=a??ne.create()),this._listeners?this._listeners instanceof G?(this._deliveryQueue??=new Ne,this._listeners=[this._listeners,i]):this._listeners.push(i):(this._options?.onWillAddFirstListener?.(this),this._listeners=i,this._options?.onDidAddFirstListener?.(this)),this._size++;let h=A(()=>{ve?.unregister(h),s?.(),this._removeListener(i)});if(n instanceof H?n.add(h):Array.isArray(n)&&n.push(h),ve){let u=new Error().stack.split(`
`).slice(2,3).join(`
`).trim(),p=/(file:|vscode-file:\/\/vscode-app)?(\/[^:]*:\d+:\d+)/.exec(u);ve.register(h,p?.[2]??u,h)}return h},this._event}_removeListener(e){if(this._options?.onWillRemoveListener?.(this),!this._listeners)return;if(this._size===1){this._listeners=void 0,this._options?.onDidRemoveLastListener?.(this),this._size=0;return}let t=this._listeners,n=t.indexOf(e);if(n===-1)throw console.log("disposed?",this._disposed),console.log("size?",this._size),console.log("arr?",JSON.stringify(this._listeners)),new Error("Attempted to dispose unknown listener");this._size--,t[n]=void 0;let i=this._deliveryQueue.current===this;if(this._size*It<=t.length){let s=0;for(let a=0;a<t.length;a++)t[a]?t[s++]=t[a]:i&&(this._deliveryQueue.end--,s<this._deliveryQueue.i&&this._deliveryQueue.i--);t.length=s}}_deliver(e,t){if(!e)return;let n=this._options?.onListenerError||le;if(!n){e.value(t);return}try{e.value(t)}catch(i){n(i)}}_deliverQueue(e){let t=e.current._listeners;for(;e.i<e.end;)this._deliver(t[e.i++],e.value);e.reset()}fire(e){if(this._deliveryQueue?.current&&(this._deliverQueue(this._deliveryQueue),this._perfMon?.stop()),this._perfMon?.start(this._size),this._listeners)if(this._listeners instanceof G)this._deliver(this._listeners,e);else{let t=this._deliveryQueue;t.enqueue(this,e,this._listeners.length),this._deliverQueue(t)}this._perfMon?.stop()}hasListeners(){return this._size>0}};var Ne=class{constructor(){this.i=-1;this.end=0}enqueue(e,t,n){this.i=0,this.end=n,this.current=e,this.value=t}reset(){this.i=this.end,this.current=void 0,this.value=void 0}};var it=Object.freeze(function(r,e){let t=setTimeout(r.bind(e),0);return{dispose(){clearTimeout(t)}}}),Et;(n=>{function r(i){return i===n.None||i===n.Cancelled||i instanceof We?!0:!i||typeof i!="object"?!1:typeof i.isCancellationRequested=="boolean"&&typeof i.onCancellationRequested=="function"}n.isCancellationToken=r,n.None=Object.freeze({isCancellationRequested:!1,onCancellationRequested:ie.None}),n.Cancelled=Object.freeze({isCancellationRequested:!0,onCancellationRequested:it})})(Et||={});var We=class{constructor(){this._isCancelled=!1;this._emitter=null}cancel(){this._isCancelled||(this._isCancelled=!0,this._emitter&&(this._emitter.fire(void 0),this.dispose()))}get isCancellationRequested(){return this._isCancelled}get onCancellationRequested(){return this._isCancelled?it:(this._emitter||(this._emitter=new C),this._emitter.event)}dispose(){this._emitter&&(this._emitter.dispose(),this._emitter=null)}};var Y="en",qe=!1,ze=!1,ge=!1,wt=(/* unused pure expression or super */ null && (!1)),kt=(/* unused pure expression or super */ null && (!1)),st=!1,St=(/* unused pure expression or super */ null && (!1)),Rt=(/* unused pure expression or super */ null && (!1)),Ct=(/* unused pure expression or super */ null && (!1)),Pt=(/* unused pure expression or super */ null && (!1)),_e,ye=Y,rt=(/* unused pure expression or super */ null && (Y)),Lt,N,W=globalThis,O;typeof W.vscode<"u"&&typeof W.vscode.process<"u"?O=W.vscode.process:typeof process<"u"&&typeof process?.versions?.node=="string"&&(O=process);var ot=typeof O?.versions?.electron=="string",Ot=ot&&O?.type==="renderer";if(typeof O=="object"){qe=O.platform==="win32",ze=O.platform==="darwin",ge=O.platform==="linux",wt=ge&&!!O.env.SNAP&&!!O.env.SNAP_REVISION,St=ot,Ct=!!O.env.CI||!!O.env.BUILD_ARTIFACTSTAGINGDIRECTORY,_e=Y,ye=Y;let r=O.env.VSCODE_NLS_CONFIG;if(r)try{let e=JSON.parse(r);_e=e.userLocale,rt=e.osLocale,ye=e.resolvedLanguage||Y,Lt=e.languagePack?.translationsConfigFile}catch{}kt=!0}else typeof navigator=="object"&&!Ot?(N=navigator.userAgent,qe=N.indexOf("Windows")>=0,ze=N.indexOf("Macintosh")>=0,Rt=(N.indexOf("Macintosh")>=0||N.indexOf("iPad")>=0||N.indexOf("iPhone")>=0)&&!!navigator.maxTouchPoints&&navigator.maxTouchPoints>0,ge=N.indexOf("Linux")>=0,Pt=N?.indexOf("Mobi")>=0,st=!0,ye=globalThis._VSCODE_NLS_LANGUAGE||Y,_e=navigator.language.toLowerCase(),rt=_e):console.error("Unable to resolve platform.");var je=0;ze?je=1:qe?je=3:ge&&(je=2);var At=st&&typeof W.importScripts=="function",gn=At?W.origin:void 0;var M=N,j=ye,Mt;(n=>{function r(){return j}n.value=r;function e(){return j.length===2?j==="en":j.length>=3?j[0]==="e"&&j[1]==="n"&&j[2]==="-":!1}n.isDefaultVariant=e;function t(){return j==="en"}n.isDefault=t})(Mt||={});var Ft=typeof W.postMessage=="function"&&!W.importScripts,at=(()=>{if(Ft){let r=[];W.addEventListener("message",t=>{if(t.data&&t.data.vscodeScheduleAsyncWork)for(let n=0,i=r.length;n<i;n++){let s=r[n];if(s.id===t.data.vscodeScheduleAsyncWork){r.splice(n,1),s.callback();return}}});let e=0;return t=>{let n=++e;r.push({id:n,callback:t}),W.postMessage({vscodeScheduleAsyncWork:n},"*")}}return r=>setTimeout(r)})();var Nt=!!(M&&M.indexOf("Chrome")>=0),yn=!!(M&&M.indexOf("Firefox")>=0),xn=!!(!Nt&&M&&M.indexOf("Safari")>=0),In=!!(M&&M.indexOf("Edg/")>=0),Dn=!!(M&&M.indexOf("Android")>=0);var Wt=Symbol("MicrotaskDelay");function xe(r,e=0,t){let n=setTimeout(()=>{r(),t&&i.dispose()},e),i=A(()=>{clearTimeout(n),t?.deleteAndLeak(i)});return t?.add(i),i}var jt,Ue;(function(){typeof globalThis.requestIdleCallback!="function"||typeof globalThis.cancelIdleCallback!="function"?Ue=(r,e)=>{at(()=>{if(t)return;let n=Date.now()+15;e(Object.freeze({didTimeout:!0,timeRemaining(){return Math.max(0,n-Date.now())}}))});let t=!1;return{dispose(){t||(t=!0)}}}:Ue=(r,e,t)=>{let n=r.requestIdleCallback(e,typeof t=="number"?{timeout:t}:void 0),i=!1;return{dispose(){i||(i=!0,r.cancelIdleCallback(n))}}},jt=r=>Ue(globalThis,r)})();var qt;(t=>{async function r(n){let i,s=await Promise.all(n.map(a=>a.then(h=>h,h=>{i||(i=h)})));if(typeof i<"u")throw i;return s}t.settled=r;function e(n){return new Promise(async(i,s)=>{try{await n(i,s)}catch(a){s(a)}})}t.withAsyncBody=e})(qt||={});var P=class P{static fromArray(e){return new P(t=>{t.emitMany(e)})}static fromPromise(e){return new P(async t=>{t.emitMany(await e)})}static fromPromises(e){return new P(async t=>{await Promise.all(e.map(async n=>t.emitOne(await n)))})}static merge(e){return new P(async t=>{await Promise.all(e.map(async n=>{for await(let i of n)t.emitOne(i)}))})}constructor(e,t){this._state=0,this._results=[],this._error=null,this._onReturn=t,this._onStateChanged=new C,queueMicrotask(async()=>{let n={emitOne:i=>this.emitOne(i),emitMany:i=>this.emitMany(i),reject:i=>this.reject(i)};try{await Promise.resolve(e(n)),this.resolve()}catch(i){this.reject(i)}finally{n.emitOne=void 0,n.emitMany=void 0,n.reject=void 0}})}[Symbol.asyncIterator](){let e=0;return{next:async()=>{do{if(this._state===2)throw this._error;if(e<this._results.length)return{done:!1,value:this._results[e++]};if(this._state===1)return{done:!0,value:void 0};await ie.toPromise(this._onStateChanged.event)}while(!0)},return:async()=>(this._onReturn?.(),{done:!0,value:void 0})}}static map(e,t){return new P(async n=>{for await(let i of e)n.emitOne(t(i))})}map(e){return P.map(this,e)}static filter(e,t){return new P(async n=>{for await(let i of e)t(i)&&n.emitOne(i)})}filter(e){return P.filter(this,e)}static coalesce(e){return P.filter(e,t=>!!t)}coalesce(){return P.coalesce(this)}static async toPromise(e){let t=[];for await(let n of e)t.push(n);return t}toPromise(){return P.toPromise(this)}emitOne(e){this._state===0&&(this._results.push(e),this._onStateChanged.fire())}emitMany(e){this._state===0&&(this._results=this._results.concat(e),this._onStateChanged.fire())}resolve(){this._state===0&&(this._state=1,this._onStateChanged.fire())}reject(e){this._state===0&&(this._state=2,this._error=e,this._onStateChanged.fire())}};P.EMPTY=P.fromArray([]);var lt=(/* unused pure expression or super */ null && (P));var Ie=class extends addon_search_k{constructor(t){super();this._terminal=t;this._linesCacheTimeout=this._register(new F);this._linesCacheDisposables=this._register(new F);this._register(A(()=>this._destroyLinesCache()))}initLinesCache(){this._linesCache||(this._linesCache=new Array(this._terminal.buffer.active.length),this._linesCacheDisposables.value=me(this._terminal.onLineFeed(()=>this._destroyLinesCache()),this._terminal.onCursorMove(()=>this._destroyLinesCache()),this._terminal.onResize(()=>this._destroyLinesCache()))),this._linesCacheTimeout.value=xe(()=>this._destroyLinesCache(),15e3)}_destroyLinesCache(){this._linesCache=void 0,this._linesCacheDisposables.clear(),this._linesCacheTimeout.clear()}getLineFromCache(t){return this._linesCache?.[t]}setLineInCache(t,n){this._linesCache&&(this._linesCache[t]=n)}translateBufferLineToStringWithWrap(t,n){let i=[],s=[0],a=this._terminal.buffer.active.getLine(t);for(;a;){let h=this._terminal.buffer.active.getLine(t+1),u=h?h.isWrapped:!1,p=a.translateToString(!u&&n);if(u&&h){let T=a.getCell(a.length-1);T&&T.getCode()===0&&T.getWidth()===1&&h.getCell(0)?.getWidth()===2&&(p=p.slice(0,-1))}if(i.push(p),u)s.push(s[s.length-1]+p.length);else break;t++,a=h}return[i.join(""),s]}};var De=class{get cachedSearchTerm(){return this._cachedSearchTerm}set cachedSearchTerm(e){this._cachedSearchTerm=e}get lastSearchOptions(){return this._lastSearchOptions}set lastSearchOptions(e){this._lastSearchOptions=e}isValidSearchTerm(e){return!!(e&&e.length>0)}didOptionsChange(e){return this._lastSearchOptions?e?this._lastSearchOptions.caseSensitive!==e.caseSensitive||this._lastSearchOptions.regex!==e.regex||this._lastSearchOptions.wholeWord!==e.wholeWord:!1:!0}shouldUpdateHighlighting(e,t){return t?.decorations?this._cachedSearchTerm===void 0||e!==this._cachedSearchTerm||this.didOptionsChange(t):!1}clearCachedTerm(){this._cachedSearchTerm=void 0}reset(){this._cachedSearchTerm=void 0,this._lastSearchOptions=void 0}};var Ee=class{constructor(e,t){this._terminal=e;this._lineCache=t}find(e,t,n,i){if(!e||e.length===0){this._terminal.clearSelection();return}if(n>this._terminal.cols)throw new Error(`Invalid col: ${n} to search in terminal of ${this._terminal.cols} cols`);this._lineCache.initLinesCache();let s={startRow:t,startCol:n},a=this._findInLine(e,s,i);if(!a)for(let h=t+1;h<this._terminal.buffer.active.baseY+this._terminal.rows&&(s.startRow=h,s.startCol=0,a=this._findInLine(e,s,i),!a);h++);return a}findNextWithSelection(e,t,n){if(!e||e.length===0){this._terminal.clearSelection();return}let i=this._terminal.getSelectionPosition();this._terminal.clearSelection();let s=0,a=0;i&&(n===e?(s=i.end.x,a=i.end.y):(s=i.start.x,a=i.start.y)),this._lineCache.initLinesCache();let h={startRow:a,startCol:s},u=this._findInLine(e,h,t);if(!u)for(let p=a+1;p<this._terminal.buffer.active.baseY+this._terminal.rows&&(h.startRow=p,h.startCol=0,u=this._findInLine(e,h,t),!u);p++);if(!u&&a!==0)for(let p=0;p<a&&(h.startRow=p,h.startCol=0,u=this._findInLine(e,h,t),!u);p++);return!u&&i&&(h.startRow=i.start.y,h.startCol=0,u=this._findInLine(e,h,t)),u}findPreviousWithSelection(e,t,n){if(!e||e.length===0){this._terminal.clearSelection();return}let i=this._terminal.getSelectionPosition();this._terminal.clearSelection();let s=this._terminal.buffer.active.baseY+this._terminal.rows-1,a=this._terminal.cols,h=!0;this._lineCache.initLinesCache();let u={startRow:s,startCol:a},p;if(i&&(u.startRow=s=i.start.y,u.startCol=a=i.start.x,n!==e&&(p=this._findInLine(e,u,t,!1),p||(u.startRow=s=i.end.y,u.startCol=a=i.end.x))),p||(p=this._findInLine(e,u,t,h)),!p){u.startCol=Math.max(u.startCol,this._terminal.cols);for(let T=s-1;T>=0&&(u.startRow=T,p=this._findInLine(e,u,t,h),!p);T--);}if(!p&&s!==this._terminal.buffer.active.baseY+this._terminal.rows-1)for(let T=this._terminal.buffer.active.baseY+this._terminal.rows-1;T>=s&&(u.startRow=T,p=this._findInLine(e,u,t,h),!p);T--);return p}_isWholeWord(e,t,n){return(e===0||" ~!@#$%^&*()+`-=[]{}|\\;:\"',./<>?".includes(t[e-1]))&&(e+n.length===t.length||" ~!@#$%^&*()+`-=[]{}|\\;:\"',./<>?".includes(t[e+n.length]))}_findInLine(e,t,n={},i=!1){let s=t.startRow,a=t.startCol;if(this._terminal.buffer.active.getLine(s)?.isWrapped){if(i){t.startCol+=this._terminal.cols;return}return t.startRow--,t.startCol+=this._terminal.cols,this._findInLine(e,t,n)}let u=this._lineCache.getLineFromCache(s);u||(u=this._lineCache.translateBufferLineToStringWithWrap(s,!0),this._lineCache.setLineInCache(s,u));let[p,T]=u,v=this._bufferColsToStringOffset(s,a),I=e,E=p;n.regex||(I=n.caseSensitive?e:e.toLowerCase(),E=n.caseSensitive?p:p.toLowerCase());let S=-1;if(n.regex){let D=RegExp(I,n.caseSensitive?"g":"gi"),x;if(i)for(;x=D.exec(E.slice(0,v));)S=D.lastIndex-x[0].length,e=x[0],D.lastIndex-=e.length-1;else x=D.exec(E.slice(v)),x&&x[0].length>0&&(S=v+(D.lastIndex-x[0].length),e=x[0])}else i?v-I.length>=0&&(S=E.lastIndexOf(I,v-I.length)):S=E.indexOf(I,v);if(S>=0){if(n.wholeWord&&!this._isWholeWord(S,E,e))return;let D=0;for(;D<T.length-1&&S>=T[D+1];)D++;let x=D;for(;x<T.length-1&&S+e.length>=T[x+1];)x++;let J=S-T[D],q=S+e.length-T[x],z=this._stringLengthToBufferSize(s+D,J),_=this._stringLengthToBufferSize(s+x,q)-z+this._terminal.cols*(x-D);return{term:e,col:z,row:s+D,size:_}}}_stringLengthToBufferSize(e,t){let n=this._terminal.buffer.active.getLine(e);if(!n)return 0;for(let i=0;i<t;i++){let s=n.getCell(i);if(!s)break;let a=s.getChars();a.length>1&&(t-=a.length-1);let h=n.getCell(i+1);h&&h.getWidth()===0&&t++}return t}_bufferColsToStringOffset(e,t){let n=e,i=0,s=this._terminal.buffer.active.getLine(n);for(;t>0&&s;){for(let a=0;a<t&&a<this._terminal.cols;a++){let h=s.getCell(a);if(!h)break;h.getWidth()&&(i+=h.getCode()===0?1:h.getChars().length)}if(n++,s=this._terminal.buffer.active.getLine(n),s&&!s.isWrapped)break;t-=this._terminal.cols}return i}};var we=class extends addon_search_k{constructor(t){super();this._terminal=t;this._highlightDecorations=[];this._highlightedLines=new Set;this._register(A(()=>this.clearHighlightDecorations()))}createHighlightDecorations(t,n){this.clearHighlightDecorations();for(let i of t){let s=this._createResultDecorations(i,n,!1);if(s)for(let a of s)this._storeDecoration(a,i)}}createActiveDecoration(t,n){let i=this._createResultDecorations(t,n,!0);if(i)return{decorations:i,match:t,dispose(){Q(i)}}}clearHighlightDecorations(){Q(this._highlightDecorations),this._highlightDecorations=[],this._highlightedLines.clear()}_storeDecoration(t,n){this._highlightedLines.add(t.marker.line),this._highlightDecorations.push({decoration:t,match:n,dispose(){t.dispose()}})}_applyStyles(t,n,i){t.classList.contains("xterm-find-result-decoration")||(t.classList.add("xterm-find-result-decoration"),n&&(t.style.outline=`1px solid ${n}`)),i&&t.classList.add("xterm-find-active-result-decoration")}_createResultDecorations(t,n,i){let s=[],a=t.col,h=t.size,u=-this._terminal.buffer.active.baseY-this._terminal.buffer.active.cursorY+t.row;for(;h>0;){let T=Math.min(this._terminal.cols-a,h);s.push([u,a,T]),a=0,h-=T,u++}let p=[];for(let T of s){let v=this._terminal.registerMarker(T[0]),I=this._terminal.registerDecoration({marker:v,x:T[1],width:T[2],backgroundColor:i?n.activeMatchBackground:n.matchBackground,overviewRulerOptions:this._highlightedLines.has(v.line)?void 0:{color:i?n.activeMatchColorOverviewRuler:n.matchOverviewRuler,position:"center"}});if(I){let E=[];E.push(v),E.push(I.onRender(S=>this._applyStyles(S,i?n.activeMatchBorder:n.matchBorder,!1))),E.push(I.onDispose(()=>Q(E))),p.push(I)}}return p.length===0?void 0:p}};var ke=class extends addon_search_k{constructor(){super(...arguments);this._searchResults=[];this._onDidChangeResults=this._register(new C)}get onDidChangeResults(){return this._onDidChangeResults.event}get searchResults(){return this._searchResults}get selectedDecoration(){return this._selectedDecoration}set selectedDecoration(t){this._selectedDecoration=t}updateResults(t,n){this._searchResults=t.slice(0,n)}clearResults(){this._searchResults=[]}clearSelectedDecoration(){this._selectedDecoration&&(this._selectedDecoration.dispose(),this._selectedDecoration=void 0)}findResultIndex(t){for(let n=0;n<this._searchResults.length;n++){let i=this._searchResults[n];if(i.row===t.row&&i.col===t.col&&i.size===t.size)return n}return-1}fireResultsChanged(t){if(!t)return;let n=-1;this._selectedDecoration&&(n=this.findResultIndex(this._selectedDecoration.match)),this._onDidChangeResults.fire({resultIndex:n,resultCount:this._searchResults.length})}reset(){this.clearSelectedDecoration(),this.clearResults()}};var ut=class extends addon_search_k{constructor(t){super();this._highlightTimeout=this._register(new F);this._lineCache=this._register(new F);this._state=new De;this._resultTracker=this._register(new ke);this._highlightLimit=t?.highlightLimit??1e3}get onDidChangeResults(){return this._resultTracker.onDidChangeResults}activate(t){this._terminal=t,this._lineCache.value=new Ie(t),this._engine=new Ee(t,this._lineCache.value),this._decorationManager=new we(t),this._register(this._terminal.onWriteParsed(()=>this._updateMatches())),this._register(this._terminal.onResize(()=>this._updateMatches())),this._register(A(()=>this.clearDecorations()))}_updateMatches(){this._highlightTimeout.clear(),this._state.cachedSearchTerm&&this._state.lastSearchOptions?.decorations&&(this._highlightTimeout.value=xe(()=>{let t=this._state.cachedSearchTerm;this._state.clearCachedTerm(),this.findPrevious(t,{...this._state.lastSearchOptions,incremental:!0},{noScroll:!0})},200))}clearDecorations(t){this._resultTracker.clearSelectedDecoration(),this._decorationManager?.clearHighlightDecorations(),this._resultTracker.clearResults(),t||this._state.clearCachedTerm()}clearActiveDecoration(){this._resultTracker.clearSelectedDecoration()}findNext(t,n,i){if(!this._terminal||!this._engine)throw new Error("Cannot use addon until it has been loaded");this._state.lastSearchOptions=n,this._state.shouldUpdateHighlighting(t,n)&&this._highlightAllMatches(t,n);let s=this._findNextAndSelect(t,n,i);return this._fireResults(n),this._state.cachedSearchTerm=t,s}_highlightAllMatches(t,n){if(!this._terminal||!this._engine||!this._decorationManager)throw new Error("Cannot use addon until it has been loaded");if(!this._state.isValidSearchTerm(t)){this.clearDecorations();return}this.clearDecorations(!0);let i=[],s,a=this._engine.find(t,0,0,n);for(;a&&(s?.row!==a.row||s?.col!==a.col)&&!(i.length>=this._highlightLimit);)s=a,i.push(s),a=this._engine.find(t,s.col+s.term.length>=this._terminal.cols?s.row+1:s.row,s.col+s.term.length>=this._terminal.cols?0:s.col+1,n);this._resultTracker.updateResults(i,this._highlightLimit),n.decorations&&this._decorationManager.createHighlightDecorations(i,n.decorations)}_findNextAndSelect(t,n,i){if(!this._terminal||!this._engine)return!1;if(!this._state.isValidSearchTerm(t))return this._terminal.clearSelection(),this.clearDecorations(),!1;let s=this._engine.findNextWithSelection(t,n,this._state.cachedSearchTerm);return this._selectResult(s,n?.decorations,i?.noScroll)}findPrevious(t,n,i){if(!this._terminal||!this._engine)throw new Error("Cannot use addon until it has been loaded");this._state.lastSearchOptions=n,this._state.shouldUpdateHighlighting(t,n)&&this._highlightAllMatches(t,n);let s=this._findPreviousAndSelect(t,n,i);return this._fireResults(n),this._state.cachedSearchTerm=t,s}_fireResults(t){this._resultTracker.fireResultsChanged(!!t?.decorations)}_findPreviousAndSelect(t,n,i){if(!this._terminal||!this._engine)return!1;if(!this._state.isValidSearchTerm(t))return this._terminal.clearSelection(),this.clearDecorations(),!1;let s=this._engine.findPreviousWithSelection(t,n,this._state.cachedSearchTerm);return this._selectResult(s,n?.decorations,i?.noScroll)}_selectResult(t,n,i){if(!this._terminal||!this._decorationManager)return!1;if(this._resultTracker.clearSelectedDecoration(),!t)return this._terminal.clearSelection(),!1;if(this._terminal.select(t.col,t.row,t.size),n){let s=this._decorationManager.createActiveDecoration(t,n);s&&(this._resultTracker.selectedDecoration=s)}if(!i&&(t.row>=this._terminal.buffer.active.viewportY+this._terminal.rows||t.row<this._terminal.buffer.active.viewportY)){let s=t.row-this._terminal.buffer.active.viewportY;s-=Math.floor(this._terminal.rows/2),this._terminal.scrollLines(s)}return!0}};
//# sourceMappingURL=addon-search.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-serialize@0.14.0/node_modules/@xterm/addon-serialize/lib/addon-serialize.mjs
/**
 * Copyright (c) 2014-2024 The xterm.js authors. All rights reserved.
 * @license MIT
 *
 * Copyright (c) 2012-2013, Christopher Jeffrey (MIT License)
 * @license MIT
 *
 * Originally forked from (with the author's permission):
 *   Fabrice Bellard's javascript vt100 for jslinux:
 *   http://bellard.org/jslinux/
 *   Copyright (c) 2011 Fabrice Bellard
 */
/*---------------------------------------------------------------------------------------------
 *  Copyright (c) Microsoft Corporation. All rights reserved.
 *  Licensed under the MIT License. See License.txt in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
var m=0,p=0,addon_serialize_g=0,b=0;var I;(r=>{function u(t,a,s,i){return i!==void 0?`#${addon_serialize_B(t)}${addon_serialize_B(a)}${addon_serialize_B(s)}${addon_serialize_B(i)}`:`#${addon_serialize_B(t)}${addon_serialize_B(a)}${addon_serialize_B(s)}`}r.toCss=u;function o(t,a,s,i=255){return(t<<24|a<<16|s<<8|i)>>>0}r.toRgba=o;function e(t,a,s,i){return{css:r.toCss(t,a,s,i),rgba:r.toRgba(t,a,s,i)}}r.toColor=e})(I||={});var addon_serialize_$;(i=>{function u(n,l){if(b=(l.rgba&255)/255,b===1)return{css:l.css,rgba:l.rgba};let f=l.rgba>>24&255,d=l.rgba>>16&255,c=l.rgba>>8&255,C=n.rgba>>24&255,h=n.rgba>>16&255,x=n.rgba>>8&255;m=C+Math.round((f-C)*b),p=h+Math.round((d-h)*b),addon_serialize_g=x+Math.round((c-x)*b);let R=I.toCss(m,p,addon_serialize_g),L=I.toRgba(m,p,addon_serialize_g);return{css:R,rgba:L}}i.blend=u;function o(n){return(n.rgba&255)===255}i.isOpaque=o;function e(n,l,f){let d=y.ensureContrastRatio(n.rgba,l.rgba,f);if(d)return I.toColor(d>>24&255,d>>16&255,d>>8&255)}i.ensureContrastRatio=e;function r(n){let l=(n.rgba|255)>>>0;return[m,p,addon_serialize_g]=y.toChannels(l),{css:I.toCss(m,p,addon_serialize_g),rgba:l}}i.opaque=r;function t(n,l){return b=Math.round(l*255),[m,p,addon_serialize_g]=y.toChannels(n.rgba),{css:I.toCss(m,p,addon_serialize_g,b),rgba:I.toRgba(m,p,addon_serialize_g,b)}}i.opacity=t;function a(n,l){return b=n.rgba&255,t(n,b*l/255)}i.multiplyOpacity=a;function s(n){return[n.rgba>>24&255,n.rgba>>16&255,n.rgba>>8&255]}i.toColorRGB=s})(addon_serialize_$||={});var addon_serialize_;(r=>{let u,o;try{let t=document.createElement("canvas");t.width=1,t.height=1;let a=t.getContext("2d",{willReadFrequently:!0});a&&(u=a,u.globalCompositeOperation="copy",o=u.createLinearGradient(0,0,1,1))}catch{}function e(t){if(t.match(/#[\da-f]{3,8}/i))switch(t.length){case 4:return m=parseInt(t.slice(1,2).repeat(2),16),p=parseInt(t.slice(2,3).repeat(2),16),addon_serialize_g=parseInt(t.slice(3,4).repeat(2),16),I.toColor(m,p,addon_serialize_g);case 5:return m=parseInt(t.slice(1,2).repeat(2),16),p=parseInt(t.slice(2,3).repeat(2),16),addon_serialize_g=parseInt(t.slice(3,4).repeat(2),16),b=parseInt(t.slice(4,5).repeat(2),16),I.toColor(m,p,addon_serialize_g,b);case 7:return{css:t,rgba:(parseInt(t.slice(1),16)<<8|255)>>>0};case 9:return{css:t,rgba:parseInt(t.slice(1),16)>>>0}}let a=t.match(/rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*(,\s*(0|1|\d?\.(\d+))\s*)?\)/);if(a)return m=parseInt(a[1]),p=parseInt(a[2]),addon_serialize_g=parseInt(a[3]),b=Math.round((a[5]===void 0?1:parseFloat(a[5]))*255),I.toColor(m,p,addon_serialize_g,b);if(!u||!o)throw new Error("css.toColor: Unsupported css format");if(u.fillStyle=o,u.fillStyle=t,typeof u.fillStyle!="string")throw new Error("css.toColor: Unsupported css format");if(u.fillRect(0,0,1,1),[m,p,addon_serialize_g,b]=u.getImageData(0,0,1,1).data,b!==255)throw new Error("css.toColor: Unsupported css format");return{rgba:I.toRgba(m,p,addon_serialize_g,b),css:t}}r.toColor=e})(addon_serialize_||={});var addon_serialize_v;(e=>{function u(r){return o(r>>16&255,r>>8&255,r&255)}e.relativeLuminance=u;function o(r,t,a){let s=r/255,i=t/255,n=a/255,l=s<=.03928?s/12.92:Math.pow((s+.055)/1.055,2.4),f=i<=.03928?i/12.92:Math.pow((i+.055)/1.055,2.4),d=n<=.03928?n/12.92:Math.pow((n+.055)/1.055,2.4);return l*.2126+f*.7152+d*.0722}e.relativeLuminance2=o})(addon_serialize_v||={});var y;(a=>{function u(s,i){if(b=(i&255)/255,b===1)return i;let n=i>>24&255,l=i>>16&255,f=i>>8&255,d=s>>24&255,c=s>>16&255,C=s>>8&255;return m=d+Math.round((n-d)*b),p=c+Math.round((l-c)*b),addon_serialize_g=C+Math.round((f-C)*b),I.toRgba(m,p,addon_serialize_g)}a.blend=u;function o(s,i,n){let l=addon_serialize_v.relativeLuminance(s>>8),f=addon_serialize_v.relativeLuminance(i>>8);if(addon_serialize_F(l,f)<n){if(f<l){let h=e(s,i,n),x=addon_serialize_F(l,addon_serialize_v.relativeLuminance(h>>8));if(x<n){let R=r(s,i,n),L=addon_serialize_F(l,addon_serialize_v.relativeLuminance(R>>8));return x>L?h:R}return h}let c=r(s,i,n),C=addon_serialize_F(l,addon_serialize_v.relativeLuminance(c>>8));if(C<n){let h=e(s,i,n),x=addon_serialize_F(l,addon_serialize_v.relativeLuminance(h>>8));return C>x?c:h}return c}}a.ensureContrastRatio=o;function e(s,i,n){let l=s>>24&255,f=s>>16&255,d=s>>8&255,c=i>>24&255,C=i>>16&255,h=i>>8&255,x=addon_serialize_F(addon_serialize_v.relativeLuminance2(c,C,h),addon_serialize_v.relativeLuminance2(l,f,d));for(;x<n&&(c>0||C>0||h>0);)c-=Math.max(0,Math.ceil(c*.1)),C-=Math.max(0,Math.ceil(C*.1)),h-=Math.max(0,Math.ceil(h*.1)),x=addon_serialize_F(addon_serialize_v.relativeLuminance2(c,C,h),addon_serialize_v.relativeLuminance2(l,f,d));return(c<<24|C<<16|h<<8|255)>>>0}a.reduceLuminance=e;function r(s,i,n){let l=s>>24&255,f=s>>16&255,d=s>>8&255,c=i>>24&255,C=i>>16&255,h=i>>8&255,x=addon_serialize_F(addon_serialize_v.relativeLuminance2(c,C,h),addon_serialize_v.relativeLuminance2(l,f,d));for(;x<n&&(c<255||C<255||h<255);)c=Math.min(255,c+Math.ceil((255-c)*.1)),C=Math.min(255,C+Math.ceil((255-C)*.1)),h=Math.min(255,h+Math.ceil((255-h)*.1)),x=addon_serialize_F(addon_serialize_v.relativeLuminance2(c,C,h),addon_serialize_v.relativeLuminance2(l,f,d));return(c<<24|C<<16|h<<8|255)>>>0}a.increaseLuminance=r;function t(s){return[s>>24&255,s>>16&255,s>>8&255,s&255]}a.toChannels=t})(y||={});function addon_serialize_B(u){let o=u.toString(16);return o.length<2?"0"+o:o}function addon_serialize_F(u,o){return u<o?(o+.05)/(u+.05):(u+.05)/(o+.05)}var E=Object.freeze((()=>{let u=[addon_serialize_.toColor("#2e3436"),addon_serialize_.toColor("#cc0000"),addon_serialize_.toColor("#4e9a06"),addon_serialize_.toColor("#c4a000"),addon_serialize_.toColor("#3465a4"),addon_serialize_.toColor("#75507b"),addon_serialize_.toColor("#06989a"),addon_serialize_.toColor("#d3d7cf"),addon_serialize_.toColor("#555753"),addon_serialize_.toColor("#ef2929"),addon_serialize_.toColor("#8ae234"),addon_serialize_.toColor("#fce94f"),addon_serialize_.toColor("#729fcf"),addon_serialize_.toColor("#ad7fa8"),addon_serialize_.toColor("#34e2e2"),addon_serialize_.toColor("#eeeeec")],o=[0,95,135,175,215,255];for(let e=0;e<216;e++){let r=o[e/36%6|0],t=o[e/6%6|0],a=o[e%6];u.push({css:I.toCss(r,t,a),rgba:I.toRgba(r,t,a)})}for(let e=0;e<24;e++){let r=8+e*10;u.push({css:I.toCss(r,r,r),rgba:I.toRgba(r,r,r)})}return u})());function addon_serialize_A(u,o,e){return Math.max(o,Math.min(u,e))}function addon_serialize_O(u){switch(u){case"&":return"&amp;";case"<":return"&lt;"}return u}var S=class{constructor(o){this._buffer=o}serialize(o,e){let r=this._buffer.getNullCell(),t=this._buffer.getNullCell(),a=r,s=o.start.y,i=o.end.y,n=o.start.x,l=o.end.x;this._beforeSerialize(i-s,s,i);for(let f=s;f<=i;f++){let d=this._buffer.getLine(f);if(d){let c=f===o.start.y?n:0,C=f===o.end.y?l:d.length;for(let h=c;h<C;h++){let x=d.getCell(h,a===r?t:r);if(!x){console.warn(`Can't get cell at row=${f}, col=${h}`);continue}this._nextCell(x,a,f,h),a=x}}this._rowEnd(f,f===i)}return this._afterSerialize(),this._serializeString(e)}_nextCell(o,e,r,t){}_rowEnd(o,e){}_beforeSerialize(o,e,r){}_afterSerialize(){}_serializeString(o){return""}};function T(u,o){return u.getFgColorMode()===o.getFgColorMode()&&u.getFgColor()===o.getFgColor()}function addon_serialize_w(u,o){return u.getBgColorMode()===o.getBgColorMode()&&u.getBgColor()===o.getBgColor()}function z(u,o){return u.isInverse()===o.isInverse()&&u.isBold()===o.isBold()&&u.isUnderline()===o.isUnderline()&&u.isOverline()===o.isOverline()&&u.isBlink()===o.isBlink()&&u.isInvisible()===o.isInvisible()&&u.isItalic()===o.isItalic()&&u.isDim()===o.isDim()&&u.isStrikethrough()===o.isStrikethrough()}var addon_serialize_k=class extends S{constructor(e,r){super(e);this._terminal=r;this._rowIndex=0;this._allRows=new Array;this._allRowSeparators=new Array;this._currentRow="";this._nullCellCount=0;this._cursorStyle=this._buffer.getNullCell();this._cursorStyleRow=0;this._cursorStyleCol=0;this._backgroundCell=this._buffer.getNullCell();this._firstRow=0;this._lastCursorRow=0;this._lastCursorCol=0;this._lastContentCursorRow=0;this._lastContentCursorCol=0;this._thisRowLastChar=this._buffer.getNullCell();this._thisRowLastSecondChar=this._buffer.getNullCell();this._nextRowFirstChar=this._buffer.getNullCell()}_beforeSerialize(e,r,t){this._allRows=new Array(e),this._lastContentCursorRow=r,this._lastCursorRow=r,this._firstRow=r}_rowEnd(e,r){this._nullCellCount>0&&!addon_serialize_w(this._cursorStyle,this._backgroundCell)&&(this._currentRow+=`\x1B[${this._nullCellCount}X`);let t="";if(!r){e-this._firstRow>=this._terminal.rows&&this._buffer.getLine(this._cursorStyleRow)?.getCell(this._cursorStyleCol,this._backgroundCell);let a=this._buffer.getLine(e),s=this._buffer.getLine(e+1);if(!s.isWrapped)t=`\r
`,this._lastCursorRow=e+1,this._lastCursorCol=0;else{t="";let i=a.getCell(a.length-1,this._thisRowLastChar),n=a.getCell(a.length-2,this._thisRowLastSecondChar),l=s.getCell(0,this._nextRowFirstChar),f=l.getWidth()>1,d=!1;(l.getChars()&&f?this._nullCellCount<=1:this._nullCellCount<=0)&&((i.getChars()||i.getWidth()===0)&&addon_serialize_w(i,l)&&(d=!0),f&&(n.getChars()||n.getWidth()===0)&&addon_serialize_w(i,l)&&addon_serialize_w(n,l)&&(d=!0)),d||(t="-".repeat(this._nullCellCount+1),t+="\x1B[1D\x1B[1X",this._nullCellCount>0&&(t+="\x1B[A",t+=`\x1B[${a.length-this._nullCellCount}C`,t+=`\x1B[${this._nullCellCount}X`,t+=`\x1B[${a.length-this._nullCellCount}D`,t+="\x1B[B"),this._lastContentCursorRow=e+1,this._lastContentCursorCol=0,this._lastCursorRow=e+1,this._lastCursorCol=0)}}this._allRows[this._rowIndex]=this._currentRow,this._allRowSeparators[this._rowIndex++]=t,this._currentRow="",this._nullCellCount=0}_diffStyle(e,r){let t=[],a=!T(e,r),s=!addon_serialize_w(e,r),i=!z(e,r);if(a||s||i)if(e.isAttributeDefault())r.isAttributeDefault()||t.push(0);else{if(a){let n=e.getFgColor();e.isFgRGB()?t.push(38,2,n>>>16&255,n>>>8&255,n&255):e.isFgPalette()?n>=16?t.push(38,5,n):t.push(n&8?90+(n&7):30+(n&7)):t.push(39)}if(s){let n=e.getBgColor();e.isBgRGB()?t.push(48,2,n>>>16&255,n>>>8&255,n&255):e.isBgPalette()?n>=16?t.push(48,5,n):t.push(n&8?100+(n&7):40+(n&7)):t.push(49)}i&&(e.isInverse()!==r.isInverse()&&t.push(e.isInverse()?7:27),e.isBold()!==r.isBold()&&t.push(e.isBold()?1:22),e.isUnderline()!==r.isUnderline()&&t.push(e.isUnderline()?4:24),e.isOverline()!==r.isOverline()&&t.push(e.isOverline()?53:55),e.isBlink()!==r.isBlink()&&t.push(e.isBlink()?5:25),e.isInvisible()!==r.isInvisible()&&t.push(e.isInvisible()?8:28),e.isItalic()!==r.isItalic()&&t.push(e.isItalic()?3:23),e.isDim()!==r.isDim()&&t.push(e.isDim()?2:22),e.isStrikethrough()!==r.isStrikethrough()&&t.push(e.isStrikethrough()?9:29))}return t}_nextCell(e,r,t,a){if(e.getWidth()===0)return;let i=e.getChars()==="",n=this._diffStyle(e,this._cursorStyle);if(i?!addon_serialize_w(this._cursorStyle,e):n.length>0){this._nullCellCount>0&&(addon_serialize_w(this._cursorStyle,this._backgroundCell)||(this._currentRow+=`\x1B[${this._nullCellCount}X`),this._currentRow+=`\x1B[${this._nullCellCount}C`,this._nullCellCount=0),this._lastContentCursorRow=this._lastCursorRow=t,this._lastContentCursorCol=this._lastCursorCol=a,this._currentRow+=`\x1B[${n.join(";")}m`;let f=this._buffer.getLine(t);f!==void 0&&(f.getCell(a,this._cursorStyle),this._cursorStyleRow=t,this._cursorStyleCol=a)}i?this._nullCellCount+=e.getWidth():(this._nullCellCount>0&&(addon_serialize_w(this._cursorStyle,this._backgroundCell)?this._currentRow+=`\x1B[${this._nullCellCount}C`:(this._currentRow+=`\x1B[${this._nullCellCount}X`,this._currentRow+=`\x1B[${this._nullCellCount}C`),this._nullCellCount=0),this._currentRow+=e.getChars(),this._lastContentCursorRow=this._lastCursorRow=t,this._lastContentCursorCol=this._lastCursorCol=a+e.getWidth())}_serializeString(e){let r=this._allRows.length;this._buffer.length-this._firstRow<=this._terminal.rows&&(r=this._lastContentCursorRow+1-this._firstRow,this._lastCursorCol=this._lastContentCursorCol,this._lastCursorRow=this._lastContentCursorRow);let t="";for(let i=0;i<r;i++)t+=this._allRows[i],i+1<r&&(t+=this._allRowSeparators[i]);if(!e){let i=this._buffer.baseY+this._buffer.cursorY,n=this._buffer.cursorX,l=i!==this._lastCursorRow||n!==this._lastCursorCol,f=c=>{c>0?t+=`\x1B[${c}C`:c<0&&(t+=`\x1B[${-c}D`)};l&&((c=>{c>0?t+=`\x1B[${c}B`:c<0&&(t+=`\x1B[${-c}A`)})(i-this._lastCursorRow),f(n-this._lastCursorCol))}let a=this._terminal._core._inputHandler._curAttrData,s=this._diffStyle(a,this._cursorStyle);return s.length>0&&(t+=`\x1B[${s.join(";")}m`),t}},D=class{activate(o){this._terminal=o}_serializeBufferByScrollback(o,e,r){let t=e.length,a=r===void 0?t:addon_serialize_A(r+o.rows,0,t);return this._serializeBufferByRange(o,e,{start:t-a,end:t-1},!1)}_serializeBufferByRange(o,e,r,t){return new addon_serialize_k(e,o).serialize({start:{x:0,y:typeof r.start=="number"?r.start:r.start.line},end:{x:o.cols,y:typeof r.end=="number"?r.end:r.end.line}},t)}_serializeBufferAsHTML(o,e){let r=o.buffer.active,t=new addon_serialize_M(r,o,e),a=e.onlySelection??!1,s=e.range;if(s)return t.serialize({start:{x:s.startCol,y:(typeof s.startLine=="number",s.startLine)},end:{x:o.cols,y:(typeof s.endLine=="number",s.endLine)}});if(!a){let n=r.length,l=e.scrollback,f=l===void 0?n:addon_serialize_A(l+o.rows,0,n);return t.serialize({start:{x:0,y:n-f},end:{x:o.cols,y:n-1}})}let i=this._terminal?.getSelectionPosition();return i!==void 0?t.serialize({start:{x:i.start.x,y:i.start.y},end:{x:i.end.x,y:i.end.y}}):""}_serializeModes(o){let e="",r=o.modes;if(r.applicationCursorKeysMode&&(e+="\x1B[?1h"),r.applicationKeypadMode&&(e+="\x1B[?66h"),r.bracketedPasteMode&&(e+="\x1B[?2004h"),r.insertMode&&(e+="\x1B[4h"),r.originMode&&(e+="\x1B[?6h"),r.reverseWraparoundMode&&(e+="\x1B[?45h"),r.sendFocusMode&&(e+="\x1B[?1004h"),r.wraparoundMode===!1&&(e+="\x1B[?7l"),r.mouseTrackingMode!=="none")switch(r.mouseTrackingMode){case"x10":e+="\x1B[?9h";break;case"vt200":e+="\x1B[?1000h";break;case"drag":e+="\x1B[?1002h";break;case"any":e+="\x1B[?1003h";break}return e}serialize(o){if(!this._terminal)throw new Error("Cannot use addon until it has been loaded");let e=o?.range?this._serializeBufferByRange(this._terminal,this._terminal.buffer.normal,o.range,!0):this._serializeBufferByScrollback(this._terminal,this._terminal.buffer.normal,o?.scrollback);if(!o?.excludeAltBuffer&&this._terminal.buffer.active.type==="alternate"){let r=this._serializeBufferByScrollback(this._terminal,this._terminal.buffer.alternate,void 0);e+=`\x1B[?1049h\x1B[H${r}`}return o?.excludeModes||(e+=this._serializeModes(this._terminal)),e}serializeAsHTML(o){if(!this._terminal)throw new Error("Cannot use addon until it has been loaded");return this._serializeBufferAsHTML(this._terminal,o||{})}dispose(){}},addon_serialize_M=class extends S{constructor(e,r,t){super(e);this._terminal=r;this._options=t;this._currentRow="";this._htmlContent="";r._core._themeService?this._ansiColors=r._core._themeService.colors.ansi:this._ansiColors=E}_padStart(e,r,t){return r=r>>0,t=t??" ",e.length>r?e:(r-=e.length,r>t.length&&(t+=t.repeat(r/t.length)),t.slice(0,r)+e)}_beforeSerialize(e,r,t){this._htmlContent+="<html><body><!--StartFragment--><pre>";let a="#000000",s="#ffffff";(this._options.includeGlobalBackground??!1)&&(a=this._terminal.options.theme?.foreground??"#ffffff",s=this._terminal.options.theme?.background??"#000000");let i=[];i.push("color: "+a+";"),i.push("background-color: "+s+";"),i.push("font-family: "+this._terminal.options.fontFamily+";"),i.push("font-size: "+this._terminal.options.fontSize+"px;"),this._htmlContent+="<div style='"+i.join(" ")+"'>"}_afterSerialize(){this._htmlContent+="</div>",this._htmlContent+="</pre><!--EndFragment--></body></html>"}_rowEnd(e,r){this._htmlContent+="<div><span>"+this._currentRow+"</span></div>",this._currentRow=""}_getHexColor(e,r){let t=r?e.getFgColor():e.getBgColor();if(r?e.isFgRGB():e.isBgRGB())return"#"+[t>>16&255,t>>8&255,t&255].map(s=>this._padStart(s.toString(16),2,"0")).join("");if(r?e.isFgPalette():e.isBgPalette())return this._ansiColors[t].css}_diffStyle(e,r){let t=[],a=!T(e,r),s=!addon_serialize_w(e,r),i=!z(e,r);if(a||s||i){let n=this._getHexColor(e,!0);n&&t.push("color: "+n+";");let l=this._getHexColor(e,!1);return l&&t.push("background-color: "+l+";"),e.isInverse()&&t.push("color: #000000; background-color: #BFBFBF;"),e.isBold()&&t.push("font-weight: bold;"),e.isUnderline()&&e.isOverline()?t.push("text-decoration: overline underline;"):e.isUnderline()?t.push("text-decoration: underline;"):e.isOverline()&&t.push("text-decoration: overline;"),e.isBlink()&&t.push("text-decoration: blink;"),e.isInvisible()&&t.push("visibility: hidden;"),e.isItalic()&&t.push("font-style: italic;"),e.isDim()&&t.push("opacity: 0.5;"),e.isStrikethrough()&&t.push("text-decoration: line-through;"),t}}_nextCell(e,r,t,a){if(e.getWidth()===0)return;let i=e.getChars()==="",n=this._diffStyle(e,r);n&&(this._currentRow+=n.length===0?"</span><span>":"</span><span style='"+n.join(" ")+"'>"),i?this._currentRow+=" ":this._currentRow+=addon_serialize_O(e.getChars())}_serializeString(){return this._htmlContent}};
//# sourceMappingURL=addon-serialize.mjs.map

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+xterm@6.0.0/node_modules/@xterm/xterm/css/xterm.css
var css_xterm = __webpack_require__(15577);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FXtermTerminal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VY7WvzNhD_3r9CCzzQQBRkJ24TlcFgn5993hhjyPY5FrUlT5YTt6P_-7Dk99h5aZ-RktS60-nud6_yL6_wFimWQo44iQuxRf8-IES-mR-EZMYCrt8oIi_mWSsm8kiqlNp_E6bh90fskW_LbuGPx21WLqsNHw8IOd6ZMOcuYaQVtftxohzyVRtxZ-THw9qAR4zEEw91TM0JFTkGfoh19xzyPEvYG0VRAmW1UP3ikCsINJeCokAmRSoqSiZzbtcUJEzzI1SrPgteD0oWIqToyNQjxn87e3h31fuz0ceXKgSFFQt5kVO0zcwp8ggqSuSJopiHIQjLWeKcv3NxoM0uX5YvrT2OsafTgvm5TApttNAyGx_vvpvzlbV3kvaOuQihpMj1jIEsDM3pI-a35fT2vunq4LNHskL133rrGZZAJlLVVNfzVqj7Iutnr4cQRU5WolwmPJxhd7wpQIeabSzokRS6whLO6Kyjn2AKms3BKl6ovNI8k1xoUBbjIogxq-MiZYJnRRUH0nivyEHhHBIINEVCCuMWfAL_lWs8SUy4ANzGY8_PNK7Cw3j7EsRPN0C8H2B2kXWz7KtQmXkEo0OXketdx-IaWpUtddpfSbSUi9ZYMpcCMynWS4xACg1C28zohawRmTJ14KInPw-UTBLsQ8yOvDK92s646JnxZ8g0wwfIdaEA2w0_L6RY_GUMHDrduq6DYF1qUKlh7NuLfuJpJpVmQo-QGVE69UeE1pLR-gwUA66xepVRAOLzWt5y6N22nGmp5Cm3Mf8ZG48cThXVRmwdWvitH1zWtz5TuDZVx3xEuJggbt1wMqbgog6UNnnfSu63ol1W3rcZa8WC17N68GllsI6L1B_Ju2b7XC-7_-DJ8nZ7berC2tRTLsWXRdFIBoUNPlnoqi5T5A670WazQo5HVsjdbioxtq7W3FhGUQ6aIuz2IdmMGnbESwgHfdchu_s674acdd4hQ9rvCUPSP4Nu23Shr7VT_2o7jefaqUnzmIVVByAG7l1WNk3OfNbb5Y_vu61z5lqccUhv6swDlsAjWe-fe5HjzbnWl1rLlKIdsYNeApGmyLM98MIs-z-HRTEfFuSGqWr_5TA4LG_0-WZpBwETJxiOIHQ-9t2TQZ8JnjILf31ZctZejoDlgLnAstAokurEVJh3W58vDNHWV6Sdp0nfoeRlVMHPx-eRzqzQ8qZRcmjc7qqGjj2-07FW68muj1y5qVfHk-R5jd9NQt-YMaVzZ9xBMf8r6bjrp-N1XT1vOT7d5-LQydvPJWiNletaWJqBqHkegVcn7rV62xRVd-21beMUcw3TQf_UBX13YZqodQ1-vcKwJ327Q4hYkejP3T-mqpEpRitEeqVuT6kPkVTWNfVARtFi8TIbpg3GQ4id7XkkzoPZusBpHGPzsLlP3FNY7QSKWxH9Js2uBQohQzM27ij7HbJ35mvWZzwz9yZg5r3BWbZsqzFlhdyduQJu77wm3nAT3918E7_3nYLpIRFLefJG0XcQiVyh71KwQK7Q4ldZKA4K_QanxQqlUsg8YwHccuMfXrbXW-PhKkOxEUFRpiaSdZuVyJnKVlN_mhCyo7SGUuMQAqnqplSIEFR17ss5FU-2Yhv_Y1Yd8-BVQJ63GWU4WvHt9FlTZ0V__Ae5y5JxXxQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FXtermTerminal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VY7WvzNhD_3r9CCzzQQBRkJ24TlcFgn5993hhjyPY5FrUlT5YTt6P_7Dk99h5aZ_RktS60_nud6_yL6_wFimWQo44iQuxRf8_IES_mR_EZMYCrt8oIi_mWSsm8kiqlNp_E6bh90fskW_LbuGPx21WLqsNHw8IOd6ZMOcuYaQVtftxohzyVRtxZ_THw9qAR4zEEw91TM0JFTkGfoh19xzyPEvYG0VRAmW1UP3ikCsINJeCokAmRSoqSiZzbtcUJEzzI1SrPgteD0oWIqToyNQjxn87e3h31fuz0ceXKgSFFQt5kVO0zcwp8ggqSuSJopiHIQjLWeKcv3NxoM0uX5YvrT2OsafTgvm5TApttNAyGx_vvpvzlbV3kvaOuQihpMj1jIEsDM3pI_a35fT2vunq4LNHskL133rrGZZAJlLVVNfzVqj7Iutnr4cQRU5WolwmPJxhd7wpQIeabSzokRS6whLO6Kyjn2AKms3BKl6ovNI8k1xoUBbjIogxq_MiZYJnRRUH0nivyEHhHBIINEVCCuMWfAL_lWs8SUy4ANzGY8_PNK7Cw3j7EsRPN0C8H2B2kXWz7KtQmXkEo0OXketdx_IaWpUtddpfSbSUi9ZYMpcCMynWS4xACg1C28zohawRmTJ14KInPw_UTBLsQ8yOvDK92s646JnxZ8g0wwfIdaEA2w0_L6RY_GUMHDrduq6DYF1qUKlh7NuLfuJpJpVmQo_QGVE69UeE1pLR_gwUA66xepVRAOLzWt5y6N22nGmp5Cm3Mf8ZG48cThXVRmwdWvitH1zWtz5TuDZVx3xEuJggbt1wMqbgog6UNnnfSu63ol1W3rcZa8WC17N68GllsI6L1B_Ju2b7XC_7_DJ8nZ7berC2tRTLsWXRdFIBoUNPlnoqi5T5A670WazQo5HVsjdbioxtq7W3FhGUQ6aIuz2IdmMGnbESwgHfdchu_s674acdd4hQ9rvCUPSP4Nu23Shr7VT_2o7jefaqUnzmIVVByAG7l1WNk3OfNbb5Y_vu61z5lqccUhv6swDlsAjWe_fe5HjzbnWl1rLlKIdsYNeApGmyLM98MIs_z_HRTEfFuSGqWr_5TA4LG_0_WZpBwETJxiOIHQ_9t2TQZ8JnjILf31ZctZejoDlgLnAstAokurEVJh3W58vDNHWV6Sdp0nfoeRlVMHPx_eRzqzQ8qZRcmjc7qqGjj2_07FW68muj1y5qVfHk_R5jd9NQt_YMaVzZ9xBMf8r6bjrp_N1XT1vOT7d5_LQydvPJWiNletaWJqBqHkegVcn7rV62xRVd_21beMUcw3TQf_UBX13YZqodQ1_vcKwJ327Q4hYkejP3T_mqpEpRitEeqVuT6kPkVTWNfVARtFi8TIbpg3GQ4id7XkkzoPZusBpHGPzsLlP3FNY7QSKWxH9Js2uBQohQzM27ij7HbJ35mvWZzwz9yZg5r3BWbZsqzFlhdyduQJu77wm3nAT3918E7_3nYLpIRFLefJG0XcQiVyh71KwQK7Q4ldZKA4K_QanxQqlUsg8YwHccuMfXrbXW_PhKkOxEUFRpiaSdZuVyJnKVlN_mhCyo7SGUuMQAqnqplSIEFR17ss5FU_2Yhv_Y1Yd8_BVQJ63GWU4WvHt9FlTZ0V_Ae5y5JxXxQAAA_22_7D = __webpack_require__(57338);
;// ./src/components/sessions/XtermTerminal.css.ts


var container = 'i0hun0';
var copiedToast = 'i0hun5';
var copiedToastVisible = 'i0hun6';
var floatingCopyButton = 'i0hun3';
var scrollThumb = 'i0hun8';
var scrollTrack = 'i0hun7';
var scrollbackCopyButton = 'i0hun1';
var selectionHandle = 'i0hun9';
var selectionMagnifier = 'i0huna';
var selectionMagnifierCursor = 'i0hunb';
var terminal = 'i0hun2';
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FTerminalContextMenu.css.ts.vanilla.css%22%2C%22source%22%3A%22LnFmYm5qMTAgewogIHBvc2l0aW9uOiBmaXhlZDsKICB6LWluZGV4OiAxMDg1OwogIG1pbi13aWR0aDogMTQwcHg7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzcpOwogIGJveC1zaGFkb3c6IHZhcigtLV8xOWV6MnJ6M20pOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnkpIDA7CiAgbGlzdC1zdHlsZTogbm9uZTsKICBtYXJnaW46IDA7CiAgb3V0bGluZTogbm9uZTsKfQoucWZibmoxMSB7CiAgZGlzcGxheTogYmxvY2s7CiAgd2lkdGg6IDEwMCU7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeikgdmFyKC0tXzE5ZXoycnozMCk7CiAgYmFja2dyb3VuZDogbm9uZTsKICBib3JkZXI6IG5vbmU7CiAgdGV4dC1hbGlnbjogbGVmdDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIGN1cnNvcjogcG9pbnRlcjsKICB1c2VyLXNlbGVjdDogbm9uZTsKfQoucWZibmoxMTpob3Zlcjpub3QoOmRpc2FibGVkKSB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno4KTsKfQoucWZibmoxMTpkaXNhYmxlZCB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Myk7CiAgY3Vyc29yOiBkZWZhdWx0Owp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTerminalContextMenu_css_ts_vanilla_css_22_2C_22source_22_3A_22LnFmYm5qMTAgewogIHBvc2l0aW9uOiBmaXhlZDsKICB6LWluZGV4OiAxMDg1OwogIG1pbi13aWR0aDogMTQwcHg7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzcpOwogIGJveC1zaGFkb3c6IHZhcigtLV8xOWV6MnJ6M20pOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnkpIDA7CiAgbGlzdC1zdHlsZTogbm9uZTsKICBtYXJnaW46IDA7CiAgb3V0bGluZTogbm9uZTsKfQoucWZibmoxMSB7CiAgZGlzcGxheTogYmxvY2s7CiAgd2lkdGg6IDEwMCU7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeikgdmFyKC0tXzE5ZXoycnozMCk7CiAgYmFja2dyb3VuZDogbm9uZTsKICBib3JkZXI6IG5vbmU7CiAgdGV4dC1hbGlnbjogbGVmdDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIGN1cnNvcjogcG9pbnRlcjsKICB1c2VyLXNlbGVjdDogbm9uZTsKfQoucWZibmoxMTpob3Zlcjpub3QoOmRpc2FibGVkKSB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno4KTsKfQoucWZibmoxMTpkaXNhYmxlZCB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Myk7CiAgY3Vyc29yOiBkZWZhdWx0Owp9_22_7D = __webpack_require__(70683);
;// ./src/components/sessions/TerminalContextMenu.css.ts


var menu = 'qfbnj10';
var menuItem = 'qfbnj11';
;// ./src/components/sessions/TerminalContextMenu.tsx
/* __next_internal_client_entry_do_not_use__ TerminalContextMenu auto */ 



function TerminalContextMenu({ x, y, hasSelection, onCopy, onSelectAll, onPaste, onDismiss }) {
    const menuRef = (0,react.useRef)(null);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const onKeyDown = (e)=>{
            if (e.key === "Escape") {
                onDismiss();
            }
        };
        const onMouseDown = (e)=>{
            if (menuRef.current && !menuRef.current.contains(e.target)) {
                onDismiss();
            }
        };
        const onScroll = ()=>{
            onDismiss();
        };
        document.addEventListener("keydown", onKeyDown);
        document.addEventListener("mousedown", onMouseDown);
        window.addEventListener("scroll", onScroll, {
            passive: true,
            once: true
        });
        return ()=>{
            document.removeEventListener("keydown", onKeyDown);
            document.removeEventListener("mousedown", onMouseDown);
            window.removeEventListener("scroll", onScroll);
        };
    }, [
        onDismiss
    ]);
    const handleCopy = ()=>{
        onCopy();
        onDismiss();
    };
    const handleSelectAll = ()=>{
        onSelectAll();
        onDismiss();
    };
    const handlePaste = ()=>{
        onPaste?.();
        onDismiss();
    };
    const hasPasteSupport = typeof navigator !== "undefined" && typeof navigator.clipboard?.readText === "function";
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("ul", {
        ref: menuRef,
        className: menu,
        style: {
            left: x,
            top: y
        },
        "data-testid": "terminal-context-menu",
        role: "menu",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    className: menuItem,
                    role: "menuitem",
                    disabled: !hasSelection,
                    onPointerDown: (e)=>e.preventDefault(),
                    onClick: handleCopy,
                    children: "Copy"
                })
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    className: menuItem,
                    role: "menuitem",
                    onClick: handleSelectAll,
                    children: "Select All"
                })
            }),
            onPaste && hasPasteSupport && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    className: menuItem,
                    role: "menuitem",
                    onClick: handlePaste,
                    children: "Paste"
                })
            })
        ]
    }), document.body);
}

;// ./src/components/sessions/TerminalLinkMenu.tsx
/* __next_internal_client_entry_do_not_use__ TerminalLinkMenu auto */ 



// Shown on touch devices instead of xterm's default tap-to-open behavior — a bare tap on a
// terminal link (e.g. an internal-only hostname with no public DNS) opens it immediately,
// leaving no way to copy the URL before Chrome navigates away from it.
function TerminalLinkMenu({ x, y, uri, onOpen, onCopy, onDismiss }) {
    const menuRef = (0,react.useRef)(null);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const onKeyDown = (e)=>{
            if (e.key === "Escape") onDismiss();
        };
        const onMouseDown = (e)=>{
            if (menuRef.current && !menuRef.current.contains(e.target)) onDismiss();
        };
        const onScroll = ()=>onDismiss();
        document.addEventListener("keydown", onKeyDown);
        document.addEventListener("mousedown", onMouseDown);
        window.addEventListener("scroll", onScroll, {
            passive: true,
            once: true
        });
        return ()=>{
            document.removeEventListener("keydown", onKeyDown);
            document.removeEventListener("mousedown", onMouseDown);
            window.removeEventListener("scroll", onScroll);
        };
    }, [
        onDismiss
    ]);
    const handleOpen = ()=>{
        onOpen(uri);
        onDismiss();
    };
    const handleCopy = ()=>{
        onCopy(uri);
        onDismiss();
    };
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("ul", {
        ref: menuRef,
        className: menu,
        style: {
            left: x,
            top: y
        },
        "data-testid": "terminal-link-menu",
        role: "menu",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    className: menuItem,
                    role: "menuitem",
                    onClick: handleCopy,
                    children: "Copy link"
                })
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    className: menuItem,
                    role: "menuitem",
                    onClick: handleOpen,
                    children: "Open link"
                })
            })
        ]
    }), document.body);
}

// EXTERNAL MODULE: ./src/lib/config/terminalConfig.ts
var terminalConfig = __webpack_require__(40731);
// EXTERNAL MODULE: ./src/lib/terminal/types.ts
var types = __webpack_require__(47462);
// EXTERNAL MODULE: ./src/lib/terminal/cursorRect.ts
var cursorRect = __webpack_require__(77155);
;// ./src/lib/terminal/postFitRepaint.ts
/**
 * Repaint seam and zero-size guard for terminal fits (Story 2.1.1).
 * Pure: no React, no xterm imports, so the sampler in XtermTerminal can be
 * tested against fakes.
 */ 
/** Zero-size retry bounds: whichever limit is hit first ends the retry. */ const FIT_RETRY_MAX_ATTEMPTS = 20;
const FIT_RETRY_TIMEOUT_MS = 1000;
/**
 * Repaints every row. `refresh` is renderer-independent and must suffice on its
 * own (the target Android phone likely runs the canvas renderer); the texture
 * atlas only exists under WebGL.
 */ function postFitRepaint(terminal, renderer, reason) {
    const rows = terminal.rows;
    mobileDebug/* mobileDebug */.Uo.log("forced-refresh", {
        event: "forced-refresh",
        rows,
        renderer,
        reason
    });
    if (renderer === "webgl") {
        // addon-webgl shares one atlas texture across terminals with the same font config, but
        // clearTextureAtlas() only resets the caller's glyph model; peers would draw stale glyph
        // indices (garbled text) until repainted. Clear and repaint every live webgl terminal,
        // once per frame: later callers in the same frame (every pane fires visibilitychange)
        // only refresh, since a second wipe would garble peers that already repainted.
        const now = performance.now();
        // Timestamp, not rAF: rAF is paused in hidden tabs, which would stick the guard shut
        // exactly when visibilitychange needs the clear. now < last covers a reset clock.
        if (now < lastAtlasClearAt || now - lastAtlasClearAt >= ATLAS_CLEAR_WINDOW_MS) {
            lastAtlasClearAt = now;
            for (const t of new Set([
                terminal,
                ...webglTerminals
            ])){
                t.clearTextureAtlas?.();
                t.refresh(0, Math.max(0, t.rows - 1));
            }
            return;
        }
    }
    terminal.refresh(0, Math.max(0, rows - 1));
}
const ATLAS_CLEAR_WINDOW_MS = 16;
let lastAtlasClearAt = -Infinity;
const webglTerminals = new Set();
/** Drop a terminal from the shared-atlas repaint set (dispose, or fallback off webgl). */ function forgetWebglTerminal(terminal) {
    webglTerminals.delete(terminal);
}
/** Track a terminal that loaded the webgl addon so peers repaint when it clears the shared atlas. */ function trackWebglTerminal(terminal) {
    webglTerminals.add(terminal);
}
/** FitGuard: false when the container has no layout box, so `fit()` would size to 0. */ function canFit(container) {
    return !!container && container.clientWidth > 0 && container.clientHeight > 0;
}

;// ./src/lib/terminal/webglBudget.ts
/**
 * Caps live WebGL contexts across terminals. Browsers drop the oldest context past a per-page
 * limit (commonly cited as ~16, lower on some GPUs; not verified here), which can blank those
 * panes; evicting deliberately lets us choose the victim (hidden, then least recently used) and
 * swap it to another renderer first. An evicted pane stays on that renderer for its lifetime.
 */ const MAX_WEBGL_CONTEXTS = 8;
// Insertion order doubles as recency: touch() re-inserts, so the first entry is the oldest.
const slots = new Set();
/** Registers `slot`, then evicts others until at most `max` remain. Prefers hidden victims. */ function claimWebglSlot(slot, max = MAX_WEBGL_CONTEXTS) {
    slots.delete(slot);
    slots.add(slot);
    while(slots.size > max){
        const others = [
            ...slots
        ].filter((s)=>s !== slot);
        const victim = others.find((s)=>!s.isVisible()) ?? others[0];
        if (!victim) return;
        slots.delete(victim);
        try {
            victim.release();
        } catch (err) {
            // A victim failing to swap must not abort the newcomer's own setup.
            console.error("[webglBudget] failed to release evicted WebGL slot", err);
        }
    }
}
/** Marks `slot` most recently used (call on focus) so it is evicted last. */ function touchWebglSlot(slot) {
    if (!slots.delete(slot)) return;
    slots.add(slot);
}
function releaseWebglSlot(slot) {
    slots.delete(slot);
}

;// ./src/components/sessions/XtermTerminal.tsx
/* __next_internal_client_entry_do_not_use__ SAMPLE_INTERVAL_MS,MAX_SAMPLES,shouldScheduleFit,extractCellMismatchInputs,isSustainedMismatch,isMouseReport,trackSgrMouseEncoding,XtermTerminal auto */ 







// @xterm/addon-canvas@0.7.0's peer dependency is @xterm/xterm: ^5.0.0 and will never be
// bumped -- upstream removed the canvas renderer from the xterm.js monorepo entirely as of
// 6.0.0 (xtermjs/xterm.js#5105). Despite the stale peer range (and the resulting pnpm install
// warning against our @xterm/xterm@^6.0.0), this addon is verified to still activate and
// function correctly against xterm 6 -- see the "Addendum (2026-07-27)" in
// project_plans/terminal-resize-fit-loop/decisions/ADR-001-add-xterm-addon-canvas-dependency.md
// and the runnable proof in
// __tests__/XtermTerminal.canvasAddonXterm6Compat.test.ts. Re-verify before any future
// @xterm/xterm major bump.
















const DEFAULT_SCROLLBACK_SIZE = 5000;
/**
 * Fixed cadence (ms) of the decoupled resize sampler. See
 * project_plans/terminal-resize-fit-loop/decisions/ADR-002-decoupled-sampler-tick-semantics.md
 *
 * Exported so tests import the real cadence instead of redeclaring a local
 * copy that could silently desync from the implementation.
 */ const SAMPLE_INTERVAL_MS = 50;
/**
 * Bounded give-up threshold (~1s of sampling) for sustained oscillation. See
 * project_plans/terminal-resize-fit-loop/decisions/ADR-002-decoupled-sampler-tick-semantics.md
 *
 * Exported so tests import the real threshold instead of redeclaring a local
 * copy that could silently desync from the implementation.
 */ const MAX_SAMPLES = 20;
/**
 * Tolerance (px) and consecutive-sample threshold for the WebGL actual-vs-
 * expected pixels-per-column mismatch tracker (AC5). Provisional values, not
 * yet validated against a real fractionally-scaled display — jsdom cannot
 * reproduce real WebGL glyph-width mismatch magnitude, especially under
 * fractional OS display scaling (Windows 125%/150%, macOS non-integer zoom),
 * which is exactly the condition that produces this mismatch in the first
 * place. Chosen as a reasonable starting point from requirements.md's own
 * "warns above a 1px tolerance" precedent, not measured data. See Task 5.2
 * step 7 in project_plans/terminal-resize-fit-loop/implementation/plan.md
 * for the real-device validation/tuning step.
 */ const MISMATCH_TOLERANCE_PX = 1;
const MISMATCH_THRESHOLD = 3;
/**
 * Pure Reading-A dead-band decision: a fit() should only be scheduled once a
 * proposed candidate matches the immediately preceding sampled candidate
 * exactly (not merely "differs from applied"). See ADR-002 §2 for the full
 * derivation of why Reading A is correct and Reading B is not.
 */ function shouldScheduleFit(proposed, applied, pending) {
    if (!proposed) return {
        schedule: false,
        nextPending: null
    };
    if ((0,types/* dimensionsEqual */.k)(proposed, applied)) {
        return {
            schedule: false,
            nextPending: null
        };
    }
    if (pending && (0,types/* dimensionsEqual */.k)(pending, proposed)) {
        return {
            schedule: true,
            nextPending: null
        };
    }
    return {
        schedule: false,
        nextPending: proposed
    };
}
/**
 * Impure extraction of the raw actual-vs-expected pixels-per-column inputs
 * from xterm.js internals and DOM measurement (AC5). Returns null when the
 * renderer hasn't measured cell dimensions yet. Deliberately does no
 * Number.isFinite guarding here — `terminal.cols === 0` simply produces
 * `Infinity` and is passed through; `isSustainedMismatch()` is the sole
 * guard boundary (architecture-review.md Concern 2).
 */ function extractCellMismatchInputs(terminal, containerEl) {
    const dims = terminal._core?._renderService?.dimensions;
    if (!dims?.css?.cell?.width) return null;
    return {
        actualPxPerCol: containerEl.getBoundingClientRect().width / terminal.cols,
        expectedPxPerCol: dims.css.cell.width
    };
}
/**
 * Pure, Number.isFinite-guarded mismatch decision (AC5). Returns false
 * unless both inputs are finite — guards against the `terminal.cols === 0`
 * / hidden-tab `Infinity` case (pitfalls §4) using `Number.isFinite`, not
 * `Number.isNaN` (per AC5's explicit wording; `Number.isNaN(Infinity)` is
 * `false`, which would incorrectly admit the sample).
 */ function isSustainedMismatch(actualPxPerCol, expectedPxPerCol, tolerance) {
    if (!Number.isFinite(actualPxPerCol) || !Number.isFinite(expectedPxPerCol)) {
        return false;
    }
    return Math.abs(actualPxPerCol - expectedPxPerCol) > tolerance;
}
// A whole onData chunk made of one or more SGR (\x1b[<b;x;yM/m) or X10 (\x1b[M + 3 bytes) mouse report.
const MOUSE_REPORT_RE = /^(?:\x1b\[(?:<\d+;\d+;\d+[Mm]|M[\s\S]{3}))+$/;
const isMouseReport = (data)=>MOUSE_REPORT_RE.test(data);
/** Tracks DECSET/DECRST ?1006 (SGR mouse encoding), which xterm's public `modes` doesn't expose. */ function trackSgrMouseEncoding(terminal, flag) {
    const track = (enabled)=>(params)=>{
            if (params.includes(1006)) flag.current = enabled;
            return false; // let xterm's own handler run
        };
    const reset = ()=>{
        flag.current = false;
        return false;
    };
    const parser = terminal.parser;
    if (!parser) return []; // partial terminal mocks in tests
    return [
        parser.registerCsiHandler({
            prefix: "?",
            final: "h"
        }, track(true)),
        parser.registerCsiHandler({
            prefix: "?",
            final: "l"
        }, track(false)),
        parser.registerEscHandler({
            final: "c"
        }, reset),
        parser.registerCsiHandler({
            intermediates: "!",
            final: "p"
        }, reset)
    ];
}
const INITIAL_SCROLL_MODE = {
    bufferType: "normal",
    mouseTrackingMode: "none"
};
/**
 * XtermTerminal - React wrapper for xterm.js terminal emulator
 *
 * Features:
 * - Canvas-based rendering (10-100x faster than DOM)
 * - WebGL acceleration (2x faster than canvas)
 * - Automatic resizing with FitAddon
 * - Clickable web links
 * - Search functionality
 * - Mouse event reporting (drag-to-select, clicks, etc.)
 * - Professional terminal UX
 */ const XtermTerminal = /*#__PURE__*/ (0,react.forwardRef)(({ onData, onResize, theme: themeProp, fontSize: fontSizeProp, scrollback: scrollbackProp, useConfig = false, scrollGesture, onScrollModeChange, isAltScreenActive, onAltScreenScrollUp }, ref)=>{
    // Re-render on Settings > Appearance changes so the font/cursor effects below pick them up.
    const [, setConfigVersion] = (0,react.useState)(0);
    (0,react.useEffect)(()=>{
        if (!useConfig) return;
        const onChange = ()=>setConfigVersion((v)=>v + 1);
        window.addEventListener("terminal-config-changed", onChange);
        return ()=>window.removeEventListener("terminal-config-changed", onChange);
    }, [
        useConfig
    ]);
    // Load configuration
    const config = useConfig ? (0,terminalConfig/* loadTerminalConfig */.X6)() : null;
    // Use props or config values
    const theme = themeProp ?? config?.theme ?? "dark";
    const fontSize = fontSizeProp ?? config?.fontSize ?? 14;
    const scrollback = scrollbackProp ?? config?.scrollbackLines ?? 0;
    // Mouse tracking mode is set at runtime by PTY escape sequences and read via terminal.modes.mouseTrackingMode.
    // It is not configurable via prop — the 'mouseTracking' ITerminalOptions field does not exist in xterm.js 6.
    const fontFamily = config?.fontFamily ?? 'Menlo, Monaco, "Courier New", monospace';
    const cursorStyle = config?.cursorStyle ?? "block";
    const cursorBlink = config?.cursorBlink ?? true;
    const terminalRef = (0,react.useRef)(null);
    const containerRef = (0,react.useRef)(null);
    const fitAddonRef = (0,react.useRef)(null);
    const searchAddonRef = (0,react.useRef)(null);
    const webglAddonRef = (0,react.useRef)(null);
    const serializeAddonRef = (0,react.useRef)(null);
    const lastSizeRef = (0,react.useRef)(null);
    // Active renderer, for postFitRepaint's WebGL-only atlas clear. Starts as 'dom'
    // (xterm's default) until an addon load succeeds.
    const rendererRef = (0,react.useRef)("dom");
    // Bridges the effect-local sampler to useImperativeHandle's refit().
    const requestFitRef = (0,react.useRef)(()=>{});
    // Refs for floating Copy button and toast — avoid React re-renders on 60fps selection changes
    const copyButtonRef = (0,react.useRef)(null);
    const toastRef = (0,react.useRef)(null);
    // Draggable selection handles for mobile — DOM-mutated at 60fps alongside the copy button
    const startHandleRef = (0,react.useRef)(null);
    const endHandleRef = (0,react.useRef)(null);
    // Magnifier bubble shown above the finger during a handle drag (display aid only)
    const magnifierRef = (0,react.useRef)(null);
    // Custom left-side scrollbar track and thumb
    const scrollTrackRef = (0,react.useRef)(null);
    const scrollThumbRef = (0,react.useRef)(null);
    // Context menu uses useState (shown at most once per right-click — not a hot path)
    const [contextMenuState, setContextMenuState] = (0,react.useState)(null);
    // Link tap menu (touch devices only) — same rationale as contextMenuState
    const [linkMenuState, setLinkMenuState] = (0,react.useState)(null);
    // Store callbacks in refs to avoid recreating terminal on callback changes
    const onDataRef = (0,react.useRef)(onData);
    const onResizeRef = (0,react.useRef)(onResize);
    const onScrollModeChangeRef = (0,react.useRef)(onScrollModeChange);
    const sgrMouseEncodingRef = (0,react.useRef)(false);
    const mouseModeOverrideRef = (0,react.useRef)(scrollGesture?.mouseModeOverride);
    mouseModeOverrideRef.current = scrollGesture?.mouseModeOverride;
    const onProgrammaticDataRef = (0,react.useRef)(scrollGesture?.onProgrammaticData);
    const onProgrammaticData = scrollGesture?.onProgrammaticData;
    (0,react.useEffect)(()=>{
        onDataRef.current = onData;
        onResizeRef.current = onResize;
        onScrollModeChangeRef.current = onScrollModeChange;
        onProgrammaticDataRef.current = onProgrammaticData;
    }, [
        onData,
        onResize,
        onScrollModeChange,
        onProgrammaticData
    ]);
    const gestureScrollEnabled = scrollGesture?.gestureScrollEnabled ?? true;
    // Unified mobile gesture state machine (R4.3).
    // Replaces the conflicting useTouchScroll + useMobileTerminalGestures hooks:
    // having both register touchmove caused double-scroll and prevented selection.
    // Pass terminalRef (the RefObject itself, not .current) so gesture handlers always
    // read the live terminal instance — at render time terminalRef.current is null since
    // the terminal is created inside an effect (Bug 1 fix).
    useTerminalGestures({
        containerRef,
        terminalRef,
        onSendData: (0,react.useCallback)((data)=>(onProgrammaticDataRef.current ?? onDataRef.current)?.(data), []),
        isAltScreenActive,
        onAltScreenScrollUp,
        override: scrollGesture?.scrollOverride,
        gestureScrollEnabled,
        tuiScrollPolicy: scrollGesture?.tuiScrollPolicy,
        connectionEpoch: scrollGesture?.connectionEpoch,
        onScrollStart: scrollGesture?.onScrollStart,
        onScrollGesture: scrollGesture?.onScrollGesture,
        onPageKeysSent: scrollGesture?.onPageKeysSent,
        onGestureActiveChange: scrollGesture?.onGestureActiveChange,
        isInputBusy: scrollGesture?.isInputBusy,
        mouseModeOverride: scrollGesture?.mouseModeOverride,
        isSgrMouseEncoding: ()=>sgrMouseEncodingRef.current
    });
    // Show the "Copied/Copy failed" toast via DOM mutation (no re-render).
    // Defined as useCallback with empty deps — only reads stable refs.
    const showToast = (0,react.useCallback)((status)=>{
        const toast = toastRef.current;
        if (!toast) return;
        toast.textContent = status === 'copied' ? 'Copied' : 'Copy failed';
        // Remove animation class first to allow restart; force reflow before re-adding.
        toast.classList.remove(copiedToastVisible);
        toast.style.display = 'block';
        void toast.offsetHeight;
        toast.classList.add(copiedToastVisible);
        setTimeout(()=>{
            if (toastRef.current) {
                toastRef.current.style.display = 'none';
                toastRef.current.classList.remove(copiedToastVisible);
            }
        }, 1500);
    }, []);
    // execCommand fallback for browsers that deny clipboard API
    const execCommandCopy = (0,react.useCallback)((text)=>{
        const el = document.createElement('textarea');
        el.value = text;
        document.body.appendChild(el);
        el.select();
        const ok = document.execCommand('copy');
        document.body.removeChild(el);
        return ok;
    }, []);
    // "Copy all" scrollback handler — extracts plain text from xterm buffer (no ANSI codes).
    // Uses onPointerDown (not onClick) to stay in a synchronous user-gesture context for
    // iOS Safari, which only allows clipboard writes inside unbroken gesture handlers.
    const handleCopyScrollbackPointerDown = (0,react.useCallback)((e)=>{
        e.preventDefault();
        const terminal = terminalRef.current;
        if (!terminal) return;
        const buffer = terminal.buffer.active;
        const lineStrings = [];
        for(let i = 0; i < buffer.length; i++){
            const line = buffer.getLine(i);
            if (line) {
                // translateToString(true) trims trailing whitespace — gives clean plain text
                lineStrings.push(line.translateToString(true));
            }
        }
        const text = lineStrings.join('\n').trimEnd();
        if (!text) return;
        if (navigator.clipboard?.writeText) {
            navigator.clipboard.writeText(text).then(()=>showToast('copied')).catch(()=>showToast(execCommandCopy(text) ? 'copied' : 'failed'));
        } else {
            showToast(execCommandCopy(text) ? 'copied' : 'failed');
        }
    }, [
        showToast,
        execCommandCopy
    ]);
    // Floating Copy button pointer-down handler — synchronous user gesture path (iOS safe)
    const handleCopyButtonPointerDown = (0,react.useCallback)((e)=>{
        const terminal = terminalRef.current;
        if (!terminal) return;
        const text = terminal.getSelection();
        if (!text) return;
        if (copyButtonRef.current) {
            copyButtonRef.current.style.display = 'none';
        }
        if (navigator.clipboard?.writeText) {
            navigator.clipboard.writeText(text).then(()=>{
                showToast('copied');
            }).catch(()=>{
                const ok = execCommandCopy(text);
                showToast(ok ? 'copied' : 'failed');
            });
        } else {
            const ok = execCommandCopy(text);
            showToast(ok ? 'copied' : 'failed');
        }
        e.preventDefault();
    }, [
        showToast,
        execCommandCopy
    ]);
    // Context menu action handlers
    const handleMenuCopy = (0,react.useCallback)(()=>{
        const terminal = terminalRef.current;
        if (!terminal) return;
        const text = terminal.getSelection();
        if (!text) return;
        if (navigator.clipboard?.writeText) {
            navigator.clipboard.writeText(text).then(()=>showToast('copied')).catch(()=>{
                const ok = execCommandCopy(text);
                showToast(ok ? 'copied' : 'failed');
            });
        } else {
            const ok = execCommandCopy(text);
            showToast(ok ? 'copied' : 'failed');
        }
        setContextMenuState(null);
    }, [
        showToast,
        execCommandCopy
    ]);
    const handleMenuSelectAll = (0,react.useCallback)(()=>{
        terminalRef.current?.selectAll();
        setContextMenuState(null);
    }, []);
    const handleMenuPaste = (0,react.useCallback)(()=>{
        navigator.clipboard.readText().then((text)=>{
            onDataRef.current?.(text);
        }).catch(()=>{
        // Clipboard read permission denied — silently ignore
        });
        setContextMenuState(null);
    }, []);
    const handleContextMenuDismiss = (0,react.useCallback)(()=>{
        setContextMenuState(null);
    }, []);
    // Link tap menu action handlers (touch devices only)
    const handleLinkMenuCopy = (0,react.useCallback)((uri)=>{
        if (navigator.clipboard?.writeText) {
            navigator.clipboard.writeText(uri).then(()=>showToast('copied')).catch(()=>showToast(execCommandCopy(uri) ? 'copied' : 'failed'));
        } else {
            showToast(execCommandCopy(uri) ? 'copied' : 'failed');
        }
    }, [
        showToast,
        execCommandCopy
    ]);
    const handleLinkMenuOpen = (0,react.useCallback)((uri)=>{
        // Mirrors @xterm/addon-web-links' own default handler (window.open + null opener)
        // so touch and mouse land on identical navigation behavior, just gated differently.
        const newWindow = window.open();
        if (newWindow) {
            try {
                newWindow.opener = null;
            } catch  {
            // Some browsers throw when reassigning opener — navigation still proceeds.
            }
            newWindow.location.href = uri;
        }
    }, []);
    const handleLinkMenuDismiss = (0,react.useCallback)(()=>{
        setLinkMenuState(null);
    }, []);
    // Sync the custom left-side scrollbar with the terminal's current viewport.
    // Called from onScroll, onResize, and after the initial fit — all via direct
    // DOM mutation so there's no React re-render overhead on every scroll event.
    const updateScrollbar = (0,react.useCallback)((term)=>{
        const track = scrollTrackRef.current;
        const thumb = scrollThumbRef.current;
        if (!track || !thumb) return;
        const buf = term.buffer.active;
        const totalLines = buf.length;
        const visibleLines = term.rows;
        if (totalLines <= visibleLines) {
            track.style.display = 'none';
            return;
        }
        track.style.display = 'block';
        const trackHeight = track.clientHeight;
        if (trackHeight === 0) return;
        const thumbHeight = Math.max(20, trackHeight * (visibleLines / totalLines));
        const maxScrollLines = totalLines - visibleLines;
        const progress = maxScrollLines > 0 ? buf.viewportY / maxScrollLines : 0;
        const thumbTop = progress * (trackHeight - thumbHeight);
        thumb.style.height = `${thumbHeight}px`;
        thumb.style.transform = `translateY(${thumbTop}px)`;
    }, []);
    // Initialize terminal on mount
    (0,react.useEffect)(()=>{
        // SSR guard
        if (true) {
            console.warn('[XtermTerminal] SSR detected, terminal requires client-side rendering');
            return;
        }
        if (!containerRef.current || terminalRef.current) return;
        // Create terminal instance with configuration
        // Note: mouseTracking is NOT set here — it is not a valid ITerminalOptions field in xterm.js 6.
        // Mouse tracking mode is set at runtime by PTY escape sequences and read via terminal.modes.mouseTrackingMode.
        const terminal = new xterm/* Terminal */.B({
            cursorBlink,
            cursorStyle,
            fontSize,
            fontFamily,
            theme: getTheme(theme),
            scrollback: scrollback && scrollback > 0 ? scrollback : DEFAULT_SCROLLBACK_SIZE,
            allowProposedApi: true,
            rightClickSelectsWord: true
        });
        // Repaint seam: every confirmed fit and every refit() request repaints through
        // here. Loop guard: a repeat repaint at unchanged dims inside one frame
        // (REPAINT_COALESCE_MS) is dropped, so a burst of refit() calls costs one refresh.
        const REPAINT_COALESCE_MS = 16;
        let lastRepaintAt = -Infinity;
        let lastRepaintDims = null;
        const repaint = (reason)=>{
            const dims = {
                cols: terminal.cols,
                rows: terminal.rows
            };
            const now = performance.now();
            if (lastRepaintDims && (0,types/* dimensionsEqual */.k)(lastRepaintDims, dims) && now - lastRepaintAt < REPAINT_COALESCE_MS) {
                return;
            }
            lastRepaintAt = now;
            lastRepaintDims = dims;
            postFitRepaint(terminal, rendererRef.current, reason);
        };
        // Per-fit diagnostics for the mobile debug log (Task 0.1.1c2); zero cost when the flag is off.
        const logFit = (event, extra = {})=>{
            const el = containerRef.current;
            const vv = window.visualViewport;
            mobileDebug/* mobileDebug */.Uo.log(event, {
                vvHeight: vv?.height,
                vvOffsetTop: vv?.offsetTop,
                clientWidth: el?.clientWidth,
                clientHeight: el?.clientHeight,
                cols: terminal.cols,
                rows: terminal.rows,
                renderer: rendererRef.current,
                ...extra
            });
        };
        // WebGL mismatch tracker + one-directional Canvas fallback (AC5). See
        // project_plans/terminal-resize-fit-loop/decisions/ADR-001-add-xterm-addon-canvas-dependency.md
        let webglMismatchCount = 0;
        let webglFallbackTriggered = false;
        // RAF handle for the post-fallback fit() scheduled inside
        // triggerCanvasFallback()'s success path, so it can be cancelled on
        // unmount if the component tears down before the RAF fires.
        let postFallbackRafId = null;
        const webglSlot = {
            isVisible: ()=>document.visibilityState === "visible" && (containerRef.current?.clientWidth ?? 0) > 0,
            release: ()=>{
                if (cancelled || rendererRef.current !== "webgl") return;
                console.log("[XtermTerminal] WebGL budget exceeded, moving this pane to the canvas renderer");
                swapWebglForCanvas();
            }
        };
        const triggerCanvasFallback = ()=>{
            if (webglFallbackTriggered || cancelled) return; // one-directional latch, never re-arms (pitfalls §4)
            webglFallbackTriggered = true;
            console.warn('[XtermTerminal] WebGL cell-measurement mismatch exceeded threshold, falling back to canvas renderer');
            swapWebglForCanvas();
        };
        // Shared by the fallback latch and by WebGL-budget eviction (webglBudget.ts).
        const swapWebglForCanvas = ()=>{
            webglFallbackTriggered = true; // eviction must also arm the latch, or a later mismatch loads a second CanvasAddon
            releaseWebglSlot(webglSlot);
            forgetWebglTerminal(terminal);
            webglAddonRef.current?.dispose();
            webglAddonRef.current = null;
            try {
                terminal.loadAddon(new addon_canvas.CanvasAddon());
                rendererRef.current = "canvas";
                // Wait one RAF frame after the addon swap before fitting, per the
                // historical xterm.js #1416 crash precedent (measuring against a
                // not-yet-initialized renderer).
                postFallbackRafId = requestAnimationFrame(()=>{
                    postFallbackRafId = null;
                    const proposed = fitAddonRef.current?.proposeDimensions();
                    if (fitAddonRef.current && (0,types/* isFiniteResizeDimensions */.n)(proposed)) {
                        fitAddonRef.current.fit();
                    } else {
                        console.warn('[XtermTerminal] Skipped post-fallback fit: proposed dimensions not finite');
                    }
                    repaint("context-loss");
                });
            } catch (err) {
                // adversarial-review.md Blocker: CanvasAddon construction must be
                // guarded, mirroring the WebglAddon try/catch above. If this also
                // throws, the latch stays tripped (no retry) and xterm.js's built-in
                // DOM renderer is left active automatically — no explicit fallback
                // code path is needed (confirmed by build-vs-buy.md research).
                console.error("[XtermTerminal] Canvas renderer also failed to load; falling back to xterm's built-in DOM renderer", err);
                rendererRef.current = "dom";
            }
        };
        // Single check-and-maybe-trigger-fallback sequence, shared by both the
        // startup double-RAF check and the sampler's per-confirmed-resize check
        // (previously duplicated with inconsistent guard placement). `inputs` is
        // null when the renderer hasn't measured cell dimensions yet -- that's
        // "no sample", not a clean sample, so it neither increments nor resets
        // the counter. A genuinely clean (non-mismatching) sample resets the
        // counter to 0, making MISMATCH_THRESHOLD a true consecutive-sample
        // threshold rather than a cumulative one (AC5's "sustained" mismatch).
        const recordMismatchSample = (inputs)=>{
            if (webglFallbackTriggered || !inputs) return;
            if (isSustainedMismatch(inputs.actualPxPerCol, inputs.expectedPxPerCol, MISMATCH_TOLERANCE_PX)) {
                console.error(`[XtermTerminal] ⚠️ SIZING MISMATCH! Container width doesn't match cell width calculation`);
                webglMismatchCount++;
                if (webglMismatchCount >= MISMATCH_THRESHOLD) {
                    triggerCanvasFallback();
                }
            } else {
                webglMismatchCount = 0;
            }
        };
        // Dev-only manual trigger (Task 3.2.1a): lets a human visually confirm
        // the Canvas tier renders correctly without waiting for the mismatch
        // heuristic or a real WebGL context loss (jsdom cannot exercise either
        // path for real, so this is otherwise unverifiable pre-ship).
        //
        // Scoped to this instance's container DOM node (not `window`) so that
        // multiple concurrently mounted XtermTerminal instances -- the normal
        // case in this multi-session app -- each get their own independent
        // trigger instead of clobbering one another's global hook on mount/
        // unmount (Task 5.2 step 7 manual multi-instance verification).
        if (false) {}
        // true once on mount — pointer:coarse means a touch-primary device. Hoisted here
        // (rather than only at its other use-site below) so the WebLinksAddon handler can
        // read it too.
        const isTouchPrimary =  false && 0;
        // Create and load addons
        const fitAddon = new addon_fit/* FitAddon */.Y();
        // Default web-links behavior opens the link immediately on any click/tap, which on
        // touch devices (Android Chrome) means a bare tap navigates away before there's any
        // chance to copy the URL — the only way to get at a link with no public host name.
        // Gate that default behind a menu on touch devices; leave mouse click-to-open as-is.
        const webLinksAddon = new L(isTouchPrimary ? (event, uri)=>{
            event.preventDefault();
            setLinkMenuState({
                x: event.clientX,
                y: event.clientY,
                uri
            });
        } : undefined);
        const searchAddon = new ut();
        const serializeAddon = new D();
        terminal.loadAddon(fitAddon);
        terminal.loadAddon(webLinksAddon);
        terminal.loadAddon(searchAddon);
        terminal.loadAddon(serializeAddon);
        // xterm.js issue #2033 — guard WebGL before loading on Android/mobile.
        // Loaded dynamically (rather than the always-on static import this
        // replaced) so platforms lacking WebGL2RenderingContext never pull the
        // addon-webgl module at all. webglAddonRef is still populated (instead of
        // a locally-scoped const) so triggerCanvasFallback()'s dispose() call and
        // the unmount cleanup effect below can reach the live instance, and
        // onContextLoss routes through triggerCanvasFallback() so a real context
        // loss is treated identically to the AC5 mismatch-tracker fallback path.
        // Guards against the async import below resolving after this effect has
        // already been cleaned up (e.g. a fast session-switch unmount) — without
        // this, loadAddon() would be called on an already-disposed terminal and
        // webglAddonRef.current would be left pointing at an orphaned, undisposed
        // WebGL addon that leaks its context. Set to true in the cleanup below.
        let cancelled = false;
        (async ()=>{
            if (typeof WebGL2RenderingContext !== 'undefined') {
                try {
                    const { WebglAddon } = await __webpack_require__.e(/* import() */ 9382).then(__webpack_require__.bind(__webpack_require__, 9382));
                    if (cancelled) return;
                    webglAddonRef.current = new WebglAddon();
                    terminal.loadAddon(webglAddonRef.current);
                    rendererRef.current = "webgl";
                    mobileDebug/* mobileDebug */.Uo.log("renderer", {
                        renderer: "webgl"
                    });
                    webglAddonRef.current.onContextLoss(()=>{
                        console.warn('[XtermTerminal] WebGL context lost, falling back to canvas renderer');
                        mobileDebug/* mobileDebug */.Uo.log("context-loss", {
                            renderer: rendererRef.current
                        });
                        triggerCanvasFallback();
                    });
                    // After onContextLoss is attached: claiming may evict a peer synchronously.
                    trackWebglTerminal(terminal);
                    claimWebglSlot(webglSlot);
                    console.log("[XtermTerminal] WebGL renderer enabled");
                } catch (e) {
                    console.warn("[XtermTerminal] WebGL failed to load:", e);
                }
            } else {
                console.log("[XtermTerminal] WebGL2 unavailable (Android?), using canvas renderer");
                rendererRef.current = "canvas";
                mobileDebug/* mobileDebug */.Uo.log("renderer", {
                    renderer: "canvas"
                });
            }
        })();
        // Open terminal in container with error boundary
        try {
            terminal.open(containerRef.current);
            // Attach contextmenu listener after terminal.open() — terminal.element is non-null after open().
            // Guard against null (can occur in test environments where open() is mocked as no-op).
            const termElement = terminal.element;
            const handleContextMenu = (e)=>{
                e.preventDefault(); // always suppress browser native menu
                if (mouseTracking_isMouseTracking(terminal)) return; // let PTY handle right-click via VT sequences
                setContextMenuState({
                    x: e.clientX,
                    y: e.clientY
                });
            };
            if (termElement) {
                termElement.addEventListener('contextmenu', handleContextMenu);
            }
            // Custom key handler — a single handler for all shortcuts (attaching multiple replaces).
            // All key intercepts must be here. No cleanup needed; tied to terminal instance lifecycle.
            // iOS note: navigator.clipboard.writeText may fail in keydown because keydown events may
            // not carry the same user-gesture trust as pointer events. iOS users don't have Ctrl+C
            // hardware — they use the floating Copy button (onPointerDown path) instead.
            terminal.attachCustomKeyEventHandler?.((event)=>{
                if (event.type !== 'keydown') return true;
                const isCopyShortcut = (event.ctrlKey || event.metaKey) && event.key === 'c';
                const isSelectAllShortcut = (event.ctrlKey || event.metaKey) && event.key === 'a';
                if (isCopyShortcut && terminal.getSelection().length > 0) {
                    const text = terminal.getSelection();
                    const toast = toastRef.current;
                    const showToastInHandler = (status)=>{
                        if (!toast) return;
                        toast.textContent = status === 'copied' ? 'Copied' : 'Copy failed';
                        toast.classList.remove(copiedToastVisible);
                        toast.style.display = 'block';
                        void toast.offsetHeight;
                        toast.classList.add(copiedToastVisible);
                        setTimeout(()=>{
                            if (toastRef.current) {
                                toastRef.current.style.display = 'none';
                                toastRef.current.classList.remove(copiedToastVisible);
                            }
                        }, 1500);
                    };
                    const execFallback = (t)=>{
                        const el = document.createElement('textarea');
                        el.value = t;
                        document.body.appendChild(el);
                        el.select();
                        const ok = document.execCommand('copy');
                        document.body.removeChild(el);
                        return ok;
                    };
                    if (navigator.clipboard?.writeText) {
                        navigator.clipboard.writeText(text).then(()=>showToastInHandler('copied')).catch(()=>{
                            const ok = execFallback(text);
                            showToastInHandler(ok ? 'copied' : 'failed');
                        });
                    } else {
                        const ok = execFallback(text);
                        showToastInHandler(ok ? 'copied' : 'failed');
                    }
                    terminal.clearSelection();
                    return false;
                }
                if (isSelectAllShortcut) {
                    terminal.selectAll();
                    return false;
                }
                return true;
            });
            // CRITICAL: Wait for browser to complete layout before fitting
            // Use requestAnimationFrame to ensure DOM is rendered and measurements are accurate
            requestAnimationFrame(()=>{
                requestAnimationFrame(()=>{
                    // Double RAF ensures layout is stable before FitAddon measures dimensions
                    const containerEl = containerRef.current;
                    if (containerEl) {
                        const rect = containerEl.getBoundingClientRect();
                        console.log(`[XtermTerminal] Container size before fit: ${rect.width}px × ${rect.height}px`);
                    }
                    // Log what FitAddon will see
                    const proposedDims = fitAddon.proposeDimensions();
                    console.log(`[XtermTerminal] Proposed dimensions:`, proposedDims);
                    // Check if cell dimensions are available (via private API for debugging)
                    const dims = terminal._core?._renderService?.dimensions;
                    if (dims?.css?.cell) {
                        console.log(`[XtermTerminal] Cell dimensions: ${dims.css.cell.width}px × ${dims.css.cell.height}px`);
                    } else {
                        console.warn(`[XtermTerminal] Cell dimensions not available yet!`);
                    }
                    fitAddon.fit();
                    updateScrollbar(terminal);
                    console.log(`[XtermTerminal] Initial fit complete: ${terminal.cols} cols × ${terminal.rows} rows`);
                    // Calculate actual pixels per column for verification (AC5 mismatch tracker)
                    if (containerEl) {
                        const mismatchInputs = extractCellMismatchInputs(terminal, containerEl);
                        if (mismatchInputs) {
                            console.log(`[XtermTerminal] Actual pixels per column: ${mismatchInputs.actualPxPerCol.toFixed(2)}px`);
                            console.log(`[XtermTerminal] Expected pixels per column: ${mismatchInputs.expectedPxPerCol.toFixed(2)}px`);
                        }
                        recordMismatchSample(mismatchInputs);
                    }
                // Secondary delayed fit() removed (R1.3): the double-rAF above provides sufficient layout
                // stability; the extra setTimeout caused a second terminal.onResize, triggering a duplicate
                // server resize RPC and second capture-pane cycle (double-resize corruption on mount).
                });
            });
            // Cleanup includes contextmenu listener removal
            const cleanupContextMenu = ()=>{
                if (termElement) termElement.removeEventListener('contextmenu', handleContextMenu);
            };
            // Setup event handlers using refs to avoid recreating terminal
            sgrMouseEncodingRef.current = false; // new Terminal, no modes set yet
            const sgrDisposables = trackSgrMouseEncoding(terminal, sgrMouseEncodingRef);
            const dataDisposable = terminal.onData((data)=>{
                if (mouseModeOverrideRef.current === 'none' && isMouseReport(data)) return;
                onDataRef.current?.(data);
            });
            // Show floating Copy button on selection change via direct DOM mutation (no setState).
            // onSelectionChange fires at up to 60fps during mouse drag — setState here would cause
            // a re-render storm. Direct ref mutation costs ~0.01ms vs ~3ms for React reconcile.
            // (isTouchPrimary computed above, near the WebLinksAddon setup)
            const selectionDisposable = terminal.onSelectionChange(()=>{
                const btn = copyButtonRef.current;
                if (!btn) return;
                const text = terminal.getSelection();
                if (text && text.length > 0) {
                    const pos = terminal.getSelectionPosition();
                    if (pos && terminal.element) {
                        const rect = terminal.element.getBoundingClientRect();
                        const { cellH, cellW } = getCellDimensions(terminal);
                        btn.style.left = `${rect.left + pos.end.x * cellW}px`;
                        btn.style.top = `${rect.top + pos.end.y * cellH - 40}px`;
                        btn.style.display = 'block';
                        // Draggable selection handles — mobile only
                        if (isTouchPrimary) {
                            const sh = startHandleRef.current;
                            const eh = endHandleRef.current;
                            if (sh && eh) {
                                // Start handle: left edge of first selected character, below the row
                                sh.style.left = `${rect.left + pos.start.x * cellW}px`;
                                sh.style.top = `${rect.top + (pos.start.y + 1) * cellH}px`;
                                sh.style.display = 'block';
                                // End handle: right edge of last selected character, below the row
                                eh.style.left = `${rect.left + (pos.end.x + 1) * cellW}px`;
                                eh.style.top = `${rect.top + (pos.end.y + 1) * cellH}px`;
                                eh.style.display = 'block';
                            }
                        }
                    }
                } else {
                    btn.style.display = 'none';
                    startHandleRef.current && (startHandleRef.current.style.display = 'none');
                    endHandleRef.current && (endHandleRef.current.style.display = 'none');
                }
            });
            const resizeDisposable = terminal.onResize(({ cols, rows })=>{
                // Only trigger callback if size actually changed
                const lastSize = lastSizeRef.current;
                if (!lastSize || lastSize.cols !== cols || lastSize.rows !== rows) {
                    lastSizeRef.current = {
                        cols,
                        rows
                    };
                    onResizeRef.current?.(cols, rows);
                }
                updateScrollbar(terminal);
            });
            const scrollDisposable = terminal.onScroll?.(()=>updateScrollbar(terminal));
            // onWriteParsed fires after each chunk of data is rendered. When the user has
            // scrolled up into history and new output arrives, onScroll doesn't fire (the
            // viewport position didn't change) but the buffer grew, so thumb proportions
            // go stale. Updating here keeps the thumb correctly sized while streaming.
            // The same write hook re-reads the scroll mode: mouse-tracking changes only arrive via writes.
            let lastScrollMode = INITIAL_SCROLL_MODE;
            const reportScrollMode = ()=>{
                const next = readScrollMode(terminal);
                if (next.bufferType === lastScrollMode.bufferType && next.mouseTrackingMode === lastScrollMode.mouseTrackingMode) return;
                lastScrollMode = next;
                onScrollModeChangeRef.current?.(next);
            };
            const writeParsedDisposable = terminal.onWriteParsed?.(()=>{
                updateScrollbar(terminal);
                reportScrollMode();
            });
            const bufferChangeDisposable = terminal.buffer?.onBufferChange?.(reportScrollMode);
            // CRITICAL: Store refs BEFORE triggering callbacks
            // This ensures terminalRef is available when parent component calls getTerminal()
            terminalRef.current = terminal;
            // Lets the desktop toast deck keep clear of the cursor line.
            const unregisterCursorSource = (0,cursorRect/* registerTerminalCursorSource */.p5)(()=>(0,cursorRect/* cursorCellRect */.Xn)(terminal, terminal.element?.querySelector(".xterm-screen") ?? null));
            fitAddonRef.current = fitAddon;
            searchAddonRef.current = searchAddon;
            serializeAddonRef.current = serializeAddon;
            // Deliberately no synchronous initial onResize() call here: it used to fire
            // onResize(terminal.cols, terminal.rows) with xterm's construction-time defaults
            // (80x24) before fitAddon.fit() ever ran, corrupting TerminalOutput's dimension
            // cache with the wrong size (see XtermTerminalBug.test.tsx "Bug 1" and R-series fix
            // "prevent premature resize from corrupting dimension cache"). The double-RAF
            // initial fit() below — and the decoupled sampler / ResizeObserver after it — are
            // solely responsible for telling the parent the real size via terminal.onResize.
            // ---- Mobile selection handle drag logic ----
            // Each handle listens for touchstart, then registers a document-level
            // touchmove handler that updates the xterm selection in real time.
            // The anchor (fixed endpoint) is captured at touchstart and held throughout
            // the drag so the opposite handle stays in place.
            const handleCleanupFns = [];
            if (isTouchPrimary) {
                const attachHandleDrag = (el, handle)=>{
                    if (!el) return;
                    // { startCol, startRow, endCol, endRow } in xterm viewport coords
                    let anchor = null;
                    // Cached once per drag (not re-measured on every touchmove — see touchDrag.ts).
                    let geometry = null;
                    let selectThrottled = null;
                    let cancelSelectThrottle = null;
                    const hideMagnifier = ()=>{
                        const mag = magnifierRef.current;
                        if (mag) mag.style.display = 'none';
                    };
                    // Display only: reads the buffer, never touches selection state.
                    const updateMagnifier = (col, row, clientX, clientY)=>{
                        const mag = magnifierRef.current;
                        if (!mag) return;
                        const { text, cursorOffset } = getTerminalRowText(terminal, row, col);
                        const cursorEl = document.createElement('span');
                        cursorEl.className = selectionMagnifierCursor;
                        cursorEl.textContent = text.charAt(cursorOffset);
                        mag.replaceChildren(document.createTextNode(text.slice(0, cursorOffset)), cursorEl, document.createTextNode(text.slice(cursorOffset + 1)));
                        const { left, top } = computeMagnifierPosition(clientX, clientY, window.innerWidth);
                        mag.style.left = `${left}px`;
                        mag.style.top = `${top}px`;
                        mag.style.display = 'block';
                    };
                    const applyHandleDrag = (clientX, clientY)=>{
                        if (!anchor || !geometry) return;
                        const { col, row } = pointToCell(clientX, clientY, geometry);
                        if (handle === 'end') {
                            // Keep start fixed, extend/shrink the end
                            const len = Math.max(1, (row - anchor.sr) * terminal.cols + (col - anchor.sc));
                            terminal.select(anchor.sc, anchor.sr, len);
                        } else {
                            // Keep end fixed, extend/shrink the start
                            const len = Math.max(1, (anchor.er - row) * terminal.cols + (anchor.ec - col));
                            terminal.select(col, row, len);
                        }
                        updateMagnifier(col, row, clientX, clientY);
                    };
                    const onTouchMove = (e)=>{
                        if (!anchor) return;
                        const touch = e.touches[0];
                        if (!touch) return;
                        e.preventDefault();
                        selectThrottled?.(touch.clientX, touch.clientY);
                    };
                    const onTouchEnd = ()=>{
                        anchor = null;
                        geometry = null;
                        cancelSelectThrottle?.();
                        hideMagnifier();
                        document.removeEventListener('touchmove', onTouchMove);
                        document.removeEventListener('touchend', onTouchEnd);
                        document.removeEventListener('touchcancel', onTouchEnd);
                    };
                    const onTouchStart = (e)=>{
                        e.preventDefault();
                        e.stopPropagation();
                        const pos = terminal.getSelectionPosition();
                        if (!pos || !terminal.element) return;
                        anchor = {
                            sc: pos.start.x,
                            sr: pos.start.y,
                            ec: pos.end.x,
                            er: pos.end.y
                        };
                        const { cellH, cellW } = getCellDimensions(terminal);
                        geometry = {
                            rect: terminal.element.getBoundingClientRect(),
                            cellW,
                            cellH,
                            maxCol: terminal.cols - 1,
                            maxRow: terminal.rows - 1
                        };
                        [selectThrottled, cancelSelectThrottle] = rafThrottlePoint(applyHandleDrag);
                        document.addEventListener('touchmove', onTouchMove, {
                            passive: false
                        });
                        document.addEventListener('touchend', onTouchEnd, {
                            passive: true
                        });
                        document.addEventListener('touchcancel', onTouchEnd, {
                            passive: true
                        });
                    };
                    el.addEventListener('touchstart', onTouchStart, {
                        passive: false
                    });
                    handleCleanupFns.push(()=>{
                        el.removeEventListener('touchstart', onTouchStart);
                        cancelSelectThrottle?.();
                        hideMagnifier();
                        document.removeEventListener('touchmove', onTouchMove);
                        document.removeEventListener('touchend', onTouchEnd);
                        document.removeEventListener('touchcancel', onTouchEnd);
                    });
                };
                attachHandleDrag(startHandleRef.current, 'start');
                attachHandleDrag(endHandleRef.current, 'end');
            }
            // ---- Custom scrollbar track click (jump to position) ----
            // A tap on the track itself (not the thumb) scrolls to that proportional position.
            const trackEl = scrollTrackRef.current;
            if (trackEl) {
                const onTrackClick = (e)=>{
                    if (e.target !== trackEl) return; // thumb click handled separately
                    const trackHeight = trackEl.clientHeight;
                    if (trackHeight <= 0) return;
                    const relY = e.clientY - trackEl.getBoundingClientRect().top;
                    const buf = terminal.buffer.active;
                    const maxScrollLines = buf.length - terminal.rows;
                    if (maxScrollLines <= 0) return;
                    const targetViewportY = Math.max(0, Math.min(maxScrollLines, Math.round(relY / trackHeight * maxScrollLines)));
                    terminal.scrollLines(targetViewportY - buf.viewportY);
                };
                const onTrackTouchEnd = (e)=>{
                    if (e.target !== trackEl) return;
                    const touch = e.changedTouches[0];
                    if (!touch) return;
                    const trackHeight = trackEl.clientHeight;
                    if (trackHeight <= 0) return;
                    const relY = touch.clientY - trackEl.getBoundingClientRect().top;
                    const buf = terminal.buffer.active;
                    const maxScrollLines = buf.length - terminal.rows;
                    if (maxScrollLines <= 0) return;
                    const targetViewportY = Math.max(0, Math.min(maxScrollLines, Math.round(relY / trackHeight * maxScrollLines)));
                    terminal.scrollLines(targetViewportY - buf.viewportY);
                };
                trackEl.addEventListener('click', onTrackClick);
                trackEl.addEventListener('touchend', onTrackTouchEnd, {
                    passive: true
                });
                handleCleanupFns.push(()=>{
                    trackEl.removeEventListener('click', onTrackClick);
                    trackEl.removeEventListener('touchend', onTrackTouchEnd);
                });
            }
            // ---- Custom scrollbar thumb drag (mouse + touch, all devices) ----
            // Dragging the left-side thumb scrolls the terminal without touching the
            // right-side window scrollbar area.
            const thumbEl = scrollThumbRef.current;
            if (thumbEl) {
                let dragStartY = 0;
                let dragStartViewportY = 0;
                let isDragging = false;
                const performDrag = (clientY)=>{
                    if (!isDragging) return;
                    const track = scrollTrackRef.current;
                    if (!track) return;
                    const trackHeight = track.clientHeight;
                    const buf = terminal.buffer.active;
                    const totalLines = buf.length;
                    const visibleLines = terminal.rows;
                    const maxScrollLines = totalLines - visibleLines;
                    if (maxScrollLines <= 0 || trackHeight <= 0) return;
                    const thumbHeight = Math.max(20, trackHeight * (visibleLines / totalLines));
                    const scrollableTrack = Math.max(1, trackHeight - thumbHeight);
                    const dy = clientY - dragStartY;
                    const targetViewportY = Math.max(0, Math.min(maxScrollLines, dragStartViewportY + Math.round(dy / scrollableTrack * maxScrollLines)));
                    const delta = targetViewportY - buf.viewportY;
                    if (delta !== 0) terminal.scrollLines(delta);
                };
                const endDrag = ()=>{
                    if (!isDragging) return;
                    isDragging = false;
                    document.removeEventListener('mousemove', onMouseMove);
                    document.removeEventListener('mouseup', endDrag);
                    document.removeEventListener('touchmove', onScrollThumbTouchMove);
                    document.removeEventListener('touchend', endDrag);
                    document.removeEventListener('touchcancel', endDrag);
                };
                const onMouseMove = (e)=>performDrag(e.clientY);
                const onScrollThumbTouchMove = (e)=>{
                    e.preventDefault();
                    const t = e.touches[0];
                    if (t) performDrag(t.clientY);
                };
                const onMouseDown = (e)=>{
                    e.preventDefault();
                    isDragging = true;
                    dragStartY = e.clientY;
                    dragStartViewportY = terminal.buffer.active.viewportY;
                    document.addEventListener('mousemove', onMouseMove);
                    document.addEventListener('mouseup', endDrag);
                };
                const onScrollThumbTouchStart = (e)=>{
                    e.preventDefault();
                    e.stopPropagation();
                    const t = e.touches[0];
                    if (!t) return;
                    isDragging = true;
                    dragStartY = t.clientY;
                    dragStartViewportY = terminal.buffer.active.viewportY;
                    document.addEventListener('touchmove', onScrollThumbTouchMove, {
                        passive: false
                    });
                    document.addEventListener('touchend', endDrag, {
                        passive: true
                    });
                    document.addEventListener('touchcancel', endDrag, {
                        passive: true
                    });
                };
                thumbEl.addEventListener('mousedown', onMouseDown);
                thumbEl.addEventListener('touchstart', onScrollThumbTouchStart, {
                    passive: false
                });
                handleCleanupFns.push(()=>{
                    thumbEl.removeEventListener('mousedown', onMouseDown);
                    thumbEl.removeEventListener('touchstart', onScrollThumbTouchStart);
                    document.removeEventListener('mousemove', onMouseMove);
                    document.removeEventListener('mouseup', endDrag);
                    document.removeEventListener('touchmove', onScrollThumbTouchMove);
                    document.removeEventListener('touchend', endDrag);
                    document.removeEventListener('touchcancel', endDrag);
                });
            }
            // Setup ResizeObserver for automatic fitting
            // Track container size to avoid unnecessary fit() calls
            let lastContainerSize = {
                width: 0,
                height: 0
            };
            let resizeTimeout = null;
            // Decoupled resize sampler (ADR-002): a fixed-cadence re-sampling loop,
            // started by the debounce below but never reset by further
            // ResizeObserver deliveries once running. See ADR-002 for the full
            // algorithm and rationale.
            let samplerActive = false;
            let sampleTimeout = null;
            let sampleCount = 0;
            let pendingProposedDims = null;
            // Set by requestFitRef (refit()); consumed by whichever sampler branch ends the run.
            let repaintRequested = false;
            let repaintReason = "viewport-settle";
            let anchorBottom = false;
            // refit() found a zero-size container for the whole retry budget; the next
            // non-zero ResizeObserver delivery re-issues it.
            let pendingRefit = false;
            let wasZero = false;
            // onFitted callbacks of requests coalesced into the current run; flushed once when it ends.
            let pendingFitCallbacks = [];
            const flushFitted = (stale = false)=>{
                if (pendingFitCallbacks.length === 0) return;
                const callbacks = pendingFitCallbacks;
                pendingFitCallbacks = [];
                const dims = {
                    cols: terminal.cols,
                    rows: terminal.rows,
                    ...stale && {
                        stale
                    }
                };
                callbacks.forEach((cb)=>cb(dims));
            };
            const consumeRepaintRequest = ()=>{
                const requested = repaintRequested;
                repaintRequested = false;
                return requested;
            };
            const stopSampler = ()=>{
                samplerActive = false;
                pendingProposedDims = null;
                sampleCount = 0;
                if (sampleTimeout) {
                    clearTimeout(sampleTimeout);
                    sampleTimeout = null;
                }
            };
            const sampleTick = ()=>{
                if (!fitAddonRef.current || !terminalRef.current) {
                    consumeRepaintRequest();
                    pendingFitCallbacks = []; // disposed: nothing left to report
                    stopSampler();
                    return;
                }
                const proposed = fitAddonRef.current.proposeDimensions();
                const applied = {
                    cols: terminalRef.current.cols,
                    rows: terminalRef.current.rows
                };
                const result = shouldScheduleFit(proposed, applied, pendingProposedDims);
                logFit("sampler-tick", {
                    count: sampleCount,
                    proposed,
                    applied
                });
                if (result.schedule) {
                    fitAddonRef.current.fit();
                    console.log(`[XtermTerminal] Sampler confirmed resize, fit applied: ${terminalRef.current.cols} cols × ${terminalRef.current.rows} rows`);
                    logFit("sampler-fit", {
                        forced: repaintRequested
                    });
                    if (anchorBottom) terminalRef.current.scrollToBottom?.();
                    anchorBottom = false;
                    // Sync lastContainerSize to the post-fit DOM dimensions so the next
                    // ResizeObserver entry (triggered by fit() resizing xterm.js internals)
                    // is filtered out, breaking the scrollbar-appearance oscillation loop.
                    if (containerRef.current) {
                        const r = containerRef.current.getBoundingClientRect();
                        lastContainerSize = {
                            width: r.width,
                            height: r.height
                        };
                    }
                    // AC5: accumulate mismatch across confirmed resize events, not just
                    // a single startup check (architecture research point 2).
                    if (containerRef.current) {
                        const mismatchInputs = extractCellMismatchInputs(terminalRef.current, containerRef.current);
                        recordMismatchSample(mismatchInputs);
                    }
                    // Always repaint after a confirmed fit; reason is the pending refit's, else the RO default.
                    const reason = repaintReason;
                    consumeRepaintRequest();
                    repaint(reason);
                    stopSampler();
                    flushFitted();
                    return;
                }
                if (result.nextPending === null) {
                    // At rest (proposed equals applied) or proposeDimensions() returned undefined.
                    // Only a refit() request repaints here; RO-driven at-rest stays as ADR-002 had it.
                    if (consumeRepaintRequest()) repaint(repaintReason);
                    anchorBottom = false;
                    stopSampler();
                    flushFitted();
                    return;
                }
                pendingProposedDims = result.nextPending;
                sampleCount++;
                if (sampleCount >= MAX_SAMPLES) {
                    console.warn('[XtermTerminal] Resize did not converge after 20 samples; giving up');
                    logFit("sampler-giveup", {
                        samples: sampleCount
                    });
                    if (consumeRepaintRequest()) repaint(repaintReason);
                    anchorBottom = false;
                    // Full reset (not a partial abandon): give-up must not leave the
                    // sampler permanently inert, since startSamplerIfNeeded() is a
                    // no-op whenever samplerActive is already true. See ADR-002.
                    stopSampler();
                    flushFitted();
                    return;
                }
                sampleTimeout = setTimeout(sampleTick, SAMPLE_INTERVAL_MS);
            };
            const startSamplerIfNeeded = ()=>{
                if (samplerActive) return;
                samplerActive = true;
                sampleCount = 0;
                pendingProposedDims = null;
                logFit("sampler-start");
                sampleTick();
            };
            // Bounded zero-size retry for refit(): 20 rAF attempts or 1000 ms, whichever first.
            let retryRaf = null;
            let retryTimeout = null;
            let retryAttempts = 0;
            let retryOpts = null;
            const clearRetry = ()=>{
                if (retryRaf !== null) cancelAnimationFrame(retryRaf);
                if (retryTimeout) clearTimeout(retryTimeout);
                retryRaf = null;
                retryTimeout = null;
                retryOpts = null;
            };
            const startRequest = (opts)=>{
                if (opts.forceRepaint) {
                    repaintRequested = true;
                    repaintReason = opts.reason;
                    const buf = terminal.buffer?.active;
                    if (buf && typeof buf.baseY === "number" && buf.viewportY >= buf.baseY) anchorBottom = true;
                }
                startSamplerIfNeeded();
            };
            // Q3 signal: a visible container that extends below the visual viewport does not track it.
            const checkViewportTracking = (el)=>{
                const containerHeight = el.offsetHeight; // reads layout, forcing a reflow before the sampler measures
                const vv = window.visualViewport;
                if (!vv) return;
                const rect = el.getBoundingClientRect();
                mobileDebug/* mobileDebug */.Uo.log("refit-viewport", {
                    vvHeight: vv.height,
                    vvOffsetTop: vv.offsetTop,
                    containerHeight,
                    containerTop: rect.top
                });
                if (rect.bottom > vv.offsetTop + vv.height + 1) {
                    console.warn(`[XtermTerminal] refit: container (bottom ${rect.bottom}px) does not track visualViewport (bottom ${vv.offsetTop + vv.height}px)`);
                }
            };
            const exhaustRetry = ()=>{
                const opts = retryOpts;
                clearRetry();
                console.warn("[XtermTerminal] refit: container stayed zero-size, deferring to next ResizeObserver delivery");
                logFit("fit-skipped", {
                    reason: "zero-size-exhausted"
                });
                pendingRefit = true;
                if (opts) repaint(opts.reason); // never leave a canvas with valid dims stale
                flushFitted(true); // nothing was fitted: terminal.cols/rows are still the pre-hide dims
            };
            const retryAttempt = ()=>{
                retryRaf = null;
                if (canFit(containerRef.current)) {
                    const opts = retryOpts;
                    clearRetry();
                    if (opts) requestFitRef.current(opts);
                    return;
                }
                retryAttempts++;
                if (retryAttempts >= FIT_RETRY_MAX_ATTEMPTS) {
                    exhaustRetry();
                    return;
                }
                retryRaf = requestAnimationFrame(retryAttempt);
            };
            requestFitRef.current = (requested)=>{
                // Registered once here; the zero-size retry re-enters with the callback stripped.
                const { onFitted, ...opts } = requested;
                const el = containerRef.current;
                if (!el) {
                    onFitted?.({
                        cols: terminal.cols,
                        rows: terminal.rows,
                        stale: true
                    }); // nothing will fit; release the caller's pending state
                    return;
                }
                if (onFitted) pendingFitCallbacks.push(onFitted);
                if (!canFit(el)) {
                    logFit("fit-skipped", {
                        reason: "zero-size"
                    });
                    if (retryOpts) {
                        retryOpts = opts; // already retrying; keep the latest reason
                        return;
                    }
                    retryOpts = opts;
                    retryAttempts = 0;
                    retryTimeout = setTimeout(exhaustRetry, FIT_RETRY_TIMEOUT_MS);
                    retryRaf = requestAnimationFrame(retryAttempt);
                    return;
                }
                clearRetry();
                pendingRefit = false;
                checkViewportTracking(el);
                startRequest(opts);
            };
            const resizeObserver = new ResizeObserver((entries)=>{
                if (!fitAddonRef.current || !terminalRef.current) return;
                const entry = entries[0];
                if (!entry) return;
                // Get current container size
                const { width, height } = entry.contentRect;
                // A restore after zero-size (or an exhausted refit) must repaint even when the size
                // matches lastContainerSize, which would otherwise dedupe it away.
                if (width === 0 || height === 0) {
                    wasZero = true;
                } else if (wasZero || pendingRefit) {
                    const reason = "visibility";
                    wasZero = false;
                    pendingRefit = false;
                    requestFitRef.current({
                        forceRepaint: true,
                        reason
                    });
                }
                // Only fit if size actually changed (avoid sub-pixel changes)
                const widthChanged = Math.abs(width - lastContainerSize.width) > 1;
                const heightChanged = Math.abs(height - lastContainerSize.height) > 1;
                if ((widthChanged || heightChanged) && width > 0 && height > 0) {
                    lastContainerSize = {
                        width,
                        height
                    };
                    console.log(`[XtermTerminal] Container resized to ${width}px × ${height}px (before fit)`);
                    console.log(`[XtermTerminal] Terminal dimensions BEFORE fit: ${terminalRef.current.cols} cols × ${terminalRef.current.rows} rows`);
                    // Flat 150ms debounce (R1.2): ensures tmux has processed the previous SIGWINCH before
                    // FitAddon measures container and fires terminal.onResize. The previous adaptive debounce
                    // (10ms for first 3 resizes) fired before a single animation frame, causing the
                    // ResizeObserver to trigger fit and server resize before tmux could stabilize.
                    const debounceDelay = 150;
                    // Clear any pending resize timeout
                    if (resizeTimeout) {
                        clearTimeout(resizeTimeout);
                    }
                    // Schedule the decoupled sampler start (ADR-002) after the debounce settles.
                    // No extra rAF guard here (unlike the startup double-rAF fit): a stale first
                    // proposeDimensions() read just produces a "pending" sample that won't match
                    // the next tick SAMPLE_INTERVAL_MS later, self-correcting within one extra
                    // 50ms tick rather than committing a wrong fit(). The debounce only decides
                    // *when to start* the sampler for a burst of RO deliveries; once started, the
                    // sampler's own tick chain is never reset by further RO deliveries (ADR-002).
                    resizeTimeout = setTimeout(()=>{
                        startSamplerIfNeeded();
                        resizeTimeout = null;
                    }, debounceDelay);
                } else if ((widthChanged || heightChanged) && (width === 0 || height === 0)) {
                    console.log(`[XtermTerminal] Skipping fit: container collapsed to zero-size (${width}px × ${height}px)`);
                    logFit("fit-skipped", {
                        reason: "ro-zero-size",
                        width,
                        height
                    });
                }
            });
            resizeObserver.observe(containerRef.current);
            // A backgrounded tab can lose WebGL glyph-atlas contents without a context-loss event,
            // leaving garbled glyphs over intact buffer text; repaint (clears the atlas) on return.
            const onDocumentVisible = ()=>{
                if (document.visibilityState === "visible") {
                    requestFitRef.current({
                        forceRepaint: true,
                        reason: "visibility"
                    });
                }
            };
            document.addEventListener("visibilitychange", onDocumentVisible);
            // A focused pane is the one in use: evict it last.
            const onFocusIn = ()=>touchWebglSlot(webglSlot);
            const focusContainer = containerRef.current; // React may null the ref before cleanup runs
            focusContainer?.addEventListener("focusin", onFocusIn);
            // Cleanup
            return ()=>{
                document.removeEventListener("visibilitychange", onDocumentVisible);
                cancelled = true;
                if (resizeTimeout) {
                    clearTimeout(resizeTimeout);
                }
                stopSampler();
                clearRetry();
                requestFitRef.current = ()=>{};
                if (postFallbackRafId !== null) {
                    cancelAnimationFrame(postFallbackRafId);
                    postFallbackRafId = null;
                }
                cleanupContextMenu();
                resizeObserver.disconnect();
                dataDisposable?.dispose();
                sgrDisposables.forEach((d)=>d.dispose());
                selectionDisposable?.dispose();
                resizeDisposable?.dispose();
                scrollDisposable?.dispose();
                writeParsedDisposable?.dispose();
                bufferChangeDisposable?.dispose();
                handleCleanupFns.forEach((fn)=>fn());
                forgetWebglTerminal(terminal);
                releaseWebglSlot(webglSlot);
                focusContainer?.removeEventListener("focusin", onFocusIn);
                unregisterCursorSource();
                terminal.dispose();
                terminalRef.current = null;
                fitAddonRef.current = null;
                searchAddonRef.current = null;
                serializeAddonRef.current = null;
                webglAddonRef.current = null;
                if (containerRef.current) {
                    delete containerRef.current.__staplerSquadForceCanvasFallback;
                }
            };
        } catch (error) {
            console.error('[XtermTerminal] Terminal initialization failed:', error);
            // Notify parent via resize callback with error indicator (0x0 dimensions)
            if (onResizeRef.current) {
                // Signal error by passing 0x0 dimensions
                // Parent can detect this and show error message
                console.error('[XtermTerminal] Notifying parent of initialization failure');
            }
            return; // Stop initialization
        }
    // Only recreate terminal if scrollback changes (requires full recreation)
    // Other options can be updated dynamically below
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        scrollback
    ]);
    // Update theme dynamically (no terminal recreation needed)
    (0,react.useEffect)(()=>{
        if (terminalRef.current) {
            terminalRef.current.options.theme = getTheme(theme);
            terminalRef.current.refresh(0, terminalRef.current.rows - 1);
        }
    }, [
        theme
    ]);
    // Detect system color scheme changes and update terminal theme accordingly
    // This provides automatic theme switching when no explicit theme prop is given
    (0,react.useEffect)(()=>{
        if (true) return;
        if (typeof window.matchMedia !== "function") return;
        const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
        const handleChange = (e)=>{
            const newTheme = e.matches ? "dark" : "light";
            if (terminalRef.current) {
                terminalRef.current.options.theme = getTheme(newTheme);
                terminalRef.current.refresh(0, terminalRef.current.rows - 1);
            }
        };
        mediaQuery.addEventListener("change", handleChange);
        return ()=>mediaQuery.removeEventListener("change", handleChange);
    }, [
        themeProp
    ]);
    // Update font size dynamically (no terminal recreation needed)
    (0,react.useEffect)(()=>{
        if (terminalRef.current && terminalRef.current.options.fontSize !== fontSize) {
            terminalRef.current.options.fontSize = fontSize;
            // Defer fit to avoid synchronous resize events
            setTimeout(()=>requestFitRef.current({
                    forceRepaint: true,
                    reason: "font-change"
                }), 0);
        }
    }, [
        fontSize
    ]);
    // Update font family dynamically (no terminal recreation needed)
    (0,react.useEffect)(()=>{
        if (terminalRef.current && terminalRef.current.options.fontFamily !== fontFamily) {
            terminalRef.current.options.fontFamily = fontFamily;
            // Defer fit to avoid synchronous resize events
            setTimeout(()=>requestFitRef.current({
                    forceRepaint: true,
                    reason: "font-change"
                }), 0);
        }
    }, [
        fontFamily
    ]);
    // Update cursor options dynamically (no terminal recreation needed)
    (0,react.useEffect)(()=>{
        if (terminalRef.current) {
            terminalRef.current.options.cursorStyle = cursorStyle;
            terminalRef.current.options.cursorBlink = cursorBlink;
        }
    }, [
        cursorStyle,
        cursorBlink
    ]);
    // Expose terminal methods via ref
    // CRITICAL: Use getter for terminal property to return current ref value
    (0,react.useImperativeHandle)(ref, ()=>({
            get terminal () {
                return terminalRef.current;
            },
            get serializeAddon () {
                return serializeAddonRef.current;
            },
            write: (data)=>{
                terminalRef.current?.write(data);
            },
            writeln: (data)=>{
                terminalRef.current?.writeln(data);
            },
            clear: ()=>{
                terminalRef.current?.clear();
            },
            focus: ()=>{
                terminalRef.current?.focus();
            },
            fit: ()=>{
                requestFitRef.current({
                    forceRepaint: true,
                    reason: "manual-resize"
                });
            },
            resize: (cols, rows)=>{
                terminalRef.current?.resize(cols, rows);
            },
            refit: (opts)=>{
                requestFitRef.current({
                    forceRepaint: true,
                    reason: opts?.reason ?? "manual-resize",
                    onFitted: opts?.onFitted
                });
            },
            search: (term)=>{
                if (!searchAddonRef.current) return false;
                return searchAddonRef.current.findNext(term);
            },
            searchNext: (term)=>{
                if (!searchAddonRef.current) return false;
                return searchAddonRef.current.findNext(term);
            },
            searchPrevious: (term)=>{
                if (!searchAddonRef.current) return false;
                return searchAddonRef.current.findPrevious(term);
            }
        }), []);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: container,
        "data-context": "terminal",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                ref: containerRef,
                className: terminal,
                "data-gesture-scroll": gestureScrollEnabled ? "on" : "off"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                ref: scrollTrackRef,
                className: scrollTrack,
                style: {
                    display: 'none'
                },
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    ref: scrollThumbRef,
                    className: scrollThumb
                })
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                "aria-label": "Copy terminal scrollback to clipboard",
                className: scrollbackCopyButton,
                onPointerDown: handleCopyScrollbackPointerDown,
                children: "\uD83D\uDCCB Copy all"
            }),
            typeof document !== 'undefined' && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        ref: copyButtonRef,
                        "aria-label": "Copy selected text",
                        className: floatingCopyButton,
                        style: {
                            display: 'none'
                        },
                        onPointerDown: handleCopyButtonPointerDown,
                        children: "Copy"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        ref: toastRef,
                        className: copiedToast,
                        "aria-live": "polite",
                        style: {
                            display: 'none'
                        }
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        ref: startHandleRef,
                        className: selectionHandle,
                        "aria-hidden": "true",
                        style: {
                            display: 'none'
                        }
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        ref: endHandleRef,
                        className: selectionHandle,
                        "aria-hidden": "true",
                        style: {
                            display: 'none'
                        }
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        ref: magnifierRef,
                        className: selectionMagnifier,
                        "aria-hidden": "true",
                        role: "presentation",
                        style: {
                            display: 'none'
                        }
                    })
                ]
            }), document.body),
            contextMenuState && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(TerminalContextMenu, {
                x: contextMenuState.x,
                y: contextMenuState.y,
                hasSelection: (terminalRef.current?.getSelection()?.length ?? 0) > 0,
                onCopy: handleMenuCopy,
                onSelectAll: handleMenuSelectAll,
                onPaste: handleMenuPaste,
                onDismiss: handleContextMenuDismiss
            }),
            linkMenuState && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(TerminalLinkMenu, {
                x: linkMenuState.x,
                y: linkMenuState.y,
                uri: linkMenuState.uri,
                onOpen: handleLinkMenuOpen,
                onCopy: handleLinkMenuCopy,
                onDismiss: handleLinkMenuDismiss
            })
        ]
    });
});
XtermTerminal.displayName = "XtermTerminal";
/**
 * Get xterm.js theme configuration using named theme exports
 */ function getTheme(theme) {
    return theme === "light" ? terminalConfig/* lightTerminalTheme */.Ez : terminalConfig/* darkTerminalTheme */.Gd;
}


/***/ }),

/***/ 40731:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ DEFAULT_TERMINAL_CONFIG),
/* harmony export */   Ez: () => (/* binding */ lightTerminalTheme),
/* harmony export */   Gd: () => (/* binding */ darkTerminalTheme),
/* harmony export */   X6: () => (/* binding */ loadTerminalConfig),
/* harmony export */   wo: () => (/* binding */ saveTerminalConfig)
/* harmony export */ });
/* unused harmony export TERMINAL_CONFIG_PRESETS */
/**
 * Terminal configuration system with localStorage persistence
 *
 * Provides user-configurable settings for terminal behavior, appearance,
 * and performance. All settings persist across sessions.
 */ /**
 * Default terminal configuration
 * These values provide good performance and UX for most users
 */ const DEFAULT_TERMINAL_CONFIG = {
    scrollbackLines: 0,
    theme: "dark",
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    enableBell: false,
    cursorStyle: "block",
    cursorBlink: true,
    mouseTracking: "none"
};
/**
 * Configuration presets for different use cases
 */ const TERMINAL_CONFIG_PRESETS = {
    "default": DEFAULT_TERMINAL_CONFIG,
    "high-performance": {
        scrollbackLines: 0,
        cursorBlink: false
    },
    "accessibility": {
        fontSize: 16,
        cursorStyle: "block",
        cursorBlink: false,
        theme: "light"
    }
};
/**
 * Named dark terminal theme for xterm.js
 */ const darkTerminalTheme = {
    background: '#1e1e1e',
    foreground: '#cccccc',
    cursor: '#cccccc',
    cursorAccent: '#1e1e1e',
    selectionBackground: 'rgba(255, 255, 255, 0.3)',
    black: '#000000',
    red: '#cd3131',
    green: '#0dbc79',
    yellow: '#e5e510',
    blue: '#2472c8',
    magenta: '#bc3fbc',
    cyan: '#11a8cd',
    white: '#e5e5e5',
    brightBlack: '#666666',
    brightRed: '#f14c4c',
    brightGreen: '#23d18b',
    brightYellow: '#f5f543',
    brightBlue: '#3b8eea',
    brightMagenta: '#d670d6',
    brightCyan: '#29b8db',
    brightWhite: '#ffffff'
};
/**
 * Named light terminal theme for xterm.js
 */ const lightTerminalTheme = {
    background: '#ffffff',
    foreground: '#1a1a1a',
    cursor: '#333333',
    selectionBackground: 'rgba(0, 112, 243, 0.3)',
    black: '#000000',
    red: '#c0392b',
    green: '#27ae60',
    yellow: '#e67e22',
    blue: '#2980b9',
    magenta: '#8e44ad',
    cyan: '#16a085',
    white: '#bdc3c7',
    brightBlack: '#7f8c8d',
    brightRed: '#e74c3c',
    brightGreen: '#2ecc71',
    brightYellow: '#f39c12',
    brightBlue: '#3498db',
    brightMagenta: '#9b59b6',
    brightCyan: '#1abc9c',
    brightWhite: '#ecf0f1'
};
const STORAGE_KEY = "stapler-squad-terminal-config";
/**
 * Load terminal configuration from localStorage
 * Falls back to default config if not found or invalid
 */ function loadTerminalConfig() {
    if (true) {
        return DEFAULT_TERMINAL_CONFIG;
    }
    try {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (!stored) {
            return DEFAULT_TERMINAL_CONFIG;
        }
        const config = JSON.parse(stored);
        // Validate and merge with defaults
        return {
            ...DEFAULT_TERMINAL_CONFIG,
            ...config,
            // Ensure scrollbackLines is within reasonable bounds (0 = disabled, tmux handles it)
            scrollbackLines: Math.max(0, Math.min(config.scrollbackLines ?? DEFAULT_TERMINAL_CONFIG.scrollbackLines, 100000)),
            // Ensure fontSize is within reasonable bounds
            fontSize: Math.max(8, Math.min(config.fontSize ?? DEFAULT_TERMINAL_CONFIG.fontSize, 32))
        };
    } catch (err) {
        console.error("[terminalConfig] Failed to load config:", err);
        return DEFAULT_TERMINAL_CONFIG;
    }
}
/**
 * Save terminal configuration to localStorage
 */ function saveTerminalConfig(config) {
    if (true) {
        return;
    }
    try {
        const current = loadTerminalConfig();
        const updated = {
            ...current,
            ...config
        };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
        // Dispatch custom event for other components to react
        window.dispatchEvent(new CustomEvent("terminal-config-changed", {
            detail: updated
        }));
    } catch (err) {
        console.error("[terminalConfig] Failed to save config:", err);
    }
}


/***/ }),

/***/ 47462:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   k: () => (/* binding */ dimensionsEqual),
/* harmony export */   n: () => (/* binding */ isFiniteResizeDimensions)
/* harmony export */ });
/**
 * Shared terminal value types.
 *
 * Hoisted out of XtermTerminal.tsx so useTerminalFlowControl.ts can import
 * ResizeDimensions / isFiniteResizeDimensions without depending on a leaf
 * component (see project_plans/terminal-resize-fit-loop/implementation/plan.md
 * §2 Domain Glossary, architecture-review.md Concern 1).
 */ /**
 * Value type replacing raw positional (cols, rows) pairs wherever a size is
 * held as state (applied / proposed / pending / last-sent dimensions).
 */ /**
 * Canonical Number.isFinite guard on a ResizeDimensions value. Returns false
 * for undefined or for a value with a non-finite cols/rows (e.g. Infinity or
 * NaN), so callers can trust a `true` result before using the dimensions.
 */ function isFiniteResizeDimensions(d) {
    return d !== undefined && Number.isFinite(d.cols) && Number.isFinite(d.rows);
}
/**
 * Structural cols/rows equality. Replaces the hand-rolled `a.cols === b.cols
 * && a.rows === b.rows` comparisons duplicated across the resize pipeline
 * (shouldScheduleFit's two checks, useTerminalFlowControl's resize()
 * value-dedup guard).
 */ function dimensionsEqual(a, b) {
    return a.cols === b.cols && a.rows === b.rows;
}


/***/ }),

/***/ 57338:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes i0hun4 {\n  0% {\n    opacity: 0;\n    transform: translateX(-50%) translateY(4px);\n  }\n  15% {\n    opacity: 1;\n    transform: translateX(-50%) translateY(0);\n  }\n  85% {\n    opacity: 1;\n    transform: translateX(-50%) translateY(0);\n  }\n  100% {\n    opacity: 0;\n    transform: translateX(-50%) translateY(-4px);\n  }\n}\n.i0hun0 {\n  width: 100%;\n  height: 100%;\n  display: flex;\n  flex-direction: column;\n  position: relative;\n  background: var(--_19ez2rz7);\n  border-radius: 4px;\n  overflow: hidden;\n  box-sizing: border-box;\n}\n.i0hun1 {\n  position: absolute;\n  top: var(--_19ez2rz2z);\n  right: var(--_19ez2rz2z);\n  z-index: 25;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  background: rgba(0, 0, 0, 0.45);\n  color: rgba(255, 255, 255, 0.75);\n  border: 1px solid rgba(255, 255, 255, 0.15);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3a);\n  font-weight: var(--_19ez2rz3g);\n  cursor: pointer;\n  touch-action: manipulation;\n  -moz-user-select: none;\n       user-select: none;\n  -webkit-user-select: none;\n  line-height: 1;\n}\n.i0hun1:hover {\n  background: rgba(0, 0, 0, 0.65);\n  color: rgba(255, 255, 255, 0.95);\n  border-color: rgba(255, 255, 255, 0.3);\n}\n.i0hun1:active {\n  opacity: 0.8;\n}\n.i0hun2 {\n  flex: 1 1;\n  width: 100%;\n  height: 100%;\n  min-height: 0;\n  overflow: hidden;\n  position: relative;\n  box-sizing: content-box;\n  padding: 0;\n  margin: 0;\n  overscroll-behavior: contain;\n}\n.i0hun2[data-gesture-scroll=\"on\"] {\n  touch-action: none;\n}\n.i0hun2 .xterm {\n  height: 100% !important;\n  width: 100% !important;\n  padding: 0 !important;\n  margin: 0 !important;\n  box-sizing: content-box !important;\n}\n.i0hun2 .xterm-screen {\n  height: 100% !important;\n  width: 100% !important;\n  box-sizing: content-box !important;\n  padding: 0 !important;\n  margin: 0 !important;\n}\n.i0hun2 .xterm-rows {\n  box-sizing: content-box !important;\n}\n.i0hun2 .xterm-viewport {\n  overflow-y: hidden;\n  scrollbar-width: thin;\n  scrollbar-color: rgba(255, 255, 255, 0.2) transparent;\n}\n.i0hun2 .xterm-viewport::-webkit-scrollbar {\n  width: 8px;\n}\n.i0hun2 .xterm-viewport::-webkit-scrollbar-track {\n  background: transparent;\n}\n.i0hun2 .xterm-viewport::-webkit-scrollbar-thumb {\n  background-color: rgba(255, 255, 255, 0.2);\n  border-radius: 4px;\n}\n.i0hun2 .xterm-viewport::-webkit-scrollbar-thumb:hover {\n  background-color: rgba(255, 255, 255, 0.3);\n}\n.i0hun2 .xterm-selection {\n  background-color: rgba(255, 255, 255, 0.3);\n}\n.i0hun2 .xterm:focus {\n  outline: 2px solid rgba(33, 150, 243, 0.5);\n  outline-offset: -2px;\n}\n.i0hun3 {\n  position: fixed;\n  z-index: 1085;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz30);\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rzq);\n  border: none;\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3h);\n  cursor: pointer;\n  box-shadow: 0 2px 8px rgba(0,0,0,0.4);\n  touch-action: manipulation;\n  -moz-user-select: none;\n       user-select: none;\n  -webkit-user-select: none;\n}\n.i0hun3:active {\n  opacity: 0.85;\n  transform: scale(0.97);\n}\n.i0hun5 {\n  position: fixed;\n  bottom: 80px;\n  left: 50%;\n  transform: translateX(-50%);\n  z-index: 1085;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz30);\n  background: var(--_19ez2rzu);\n  color: var(--_19ez2rz0);\n  border-radius: var(--_19ez2rz39);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  box-shadow: 0 2px 8px rgba(0,0,0,0.3);\n  pointer-events: none;\n}\n.i0hun6 {\n  animation: i0hun4 1.5s ease-in-out forwards;\n}\n.i0hun7 {\n  position: absolute;\n  left: 0;\n  top: 0;\n  bottom: 0;\n  width: 8px;\n  z-index: 25;\n  pointer-events: auto;\n  cursor: pointer;\n  touch-action: none;\n}\n.i0hun8 {\n  position: absolute;\n  left: 1px;\n  top: 0;\n  width: 6px;\n  border-radius: 3px;\n  background: rgba(255, 255, 255, 0.28);\n  pointer-events: auto;\n  touch-action: none;\n  cursor: -webkit-grab;\n  cursor: grab;\n  -moz-user-select: none;\n       user-select: none;\n  -webkit-user-select: none;\n}\n.i0hun8:active {\n  background: rgba(255, 255, 255, 0.55);\n  cursor: -webkit-grabbing;\n  cursor: grabbing;\n}\n.i0hun9 {\n  position: fixed;\n  width: 22px;\n  height: 22px;\n  border-radius: 50%;\n  background: var(--_19ez2rzm);\n  border: 2.5px solid white;\n  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);\n  touch-action: none;\n  z-index: 1090;\n  cursor: default;\n  -moz-user-select: none;\n       user-select: none;\n  -webkit-user-select: none;\n  transform: translate(-50%, 0);\n}\n.i0hun9::before {\n  content: \"\";\n  position: absolute;\n  width: 2px;\n  height: 14px;\n  background: var(--_19ez2rzm);\n  border-radius: 1px;\n  bottom: 100%;\n  left: 50%;\n  transform: translateX(-50%);\n  margin-bottom: 1px;\n}\n.i0huna {\n  position: fixed;\n  width: 200px;\n  height: 32px;\n  z-index: 1091;\n  pointer-events: none;\n  -moz-user-select: none;\n       user-select: none;\n  -webkit-user-select: none;\n  box-sizing: border-box;\n  overflow: hidden;\n  background: rgba(24, 24, 28, 0.94);\n  color: rgba(255, 255, 255, 0.95);\n  border: 1px solid rgba(255, 255, 255, 0.18);\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  font-family: Menlo, Monaco, \"Courier New\", monospace;\n  font-size: var(--_19ez2rz3a);\n  line-height: 1.4;\n  white-space: pre;\n  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.55);\n}\n.i0hunb {\n  text-decoration: underline;\n  -webkit-text-decoration-color: var(--_19ez2rzm);\n          text-decoration-color: var(--_19ez2rzm);\n  text-decoration-thickness: 2px;\n  text-underline-offset: 2px;\n  color: var(--_19ez2rzm);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 70683:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".qfbnj10 {\n  position: fixed;\n  z-index: 1085;\n  min-width: 140px;\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  box-shadow: var(--_19ez2rz3m);\n  padding: var(--_19ez2rz2y) 0;\n  list-style: none;\n  margin: 0;\n  outline: none;\n}\n.qfbnj11 {\n  display: block;\n  width: 100%;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  background: none;\n  border: none;\n  text-align: left;\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz0);\n  cursor: pointer;\n  -webkit-user-select: none;\n     -moz-user-select: none;\n          user-select: none;\n}\n.qfbnj11:hover:not(:disabled) {\n  background: var(--_19ez2rz8);\n}\n.qfbnj11:disabled {\n  color: var(--_19ez2rz3);\n  cursor: default;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 76029:
/***/ ((module) => {

!function(e,t){ true?module.exports=t():0}(self,(()=>(()=>{"use strict";var e={903:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.BaseRenderLayer=void 0;const s=i(274),r=i(627),o=i(237),n=i(860),a=i(374),h=i(296),l=i(345),c=i(859),d=i(399),_=i(855);class u extends c.Disposable{get canvas(){return this._canvas}get cacheCanvas(){return this._charAtlas?.pages[0].canvas}constructor(e,t,i,r,o,n,a,d,_,u){super(),this._terminal=e,this._container=t,this._alpha=o,this._themeService=n,this._bufferService=a,this._optionsService=d,this._decorationService=_,this._coreBrowserService=u,this._deviceCharWidth=0,this._deviceCharHeight=0,this._deviceCellWidth=0,this._deviceCellHeight=0,this._deviceCharLeft=0,this._deviceCharTop=0,this._selectionModel=(0,h.createSelectionRenderModel)(),this._bitmapGenerator=[],this._charAtlasDisposable=this.register(new c.MutableDisposable),this._onAddTextureAtlasCanvas=this.register(new l.EventEmitter),this.onAddTextureAtlasCanvas=this._onAddTextureAtlasCanvas.event,this._cellColorResolver=new s.CellColorResolver(this._terminal,this._optionsService,this._selectionModel,this._decorationService,this._coreBrowserService,this._themeService),this._canvas=this._coreBrowserService.mainDocument.createElement("canvas"),this._canvas.classList.add(`xterm-${i}-layer`),this._canvas.style.zIndex=r.toString(),this._initCanvas(),this._container.appendChild(this._canvas),this._refreshCharAtlas(this._themeService.colors),this.register(this._themeService.onChangeColors((e=>{this._refreshCharAtlas(e),this.reset(),this.handleSelectionChanged(this._selectionModel.selectionStart,this._selectionModel.selectionEnd,this._selectionModel.columnSelectMode)}))),this.register((0,c.toDisposable)((()=>{this._canvas.remove()})))}_initCanvas(){this._ctx=(0,a.throwIfFalsy)(this._canvas.getContext("2d",{alpha:this._alpha})),this._alpha||this._clearAll()}handleBlur(){}handleFocus(){}handleCursorMove(){}handleGridChanged(e,t){}handleSelectionChanged(e,t,i=!1){this._selectionModel.update(this._terminal._core,e,t,i)}_setTransparency(e){if(e===this._alpha)return;const t=this._canvas;this._alpha=e,this._canvas=this._canvas.cloneNode(),this._initCanvas(),this._container.replaceChild(this._canvas,t),this._refreshCharAtlas(this._themeService.colors),this.handleGridChanged(0,this._bufferService.rows-1)}_refreshCharAtlas(e){if(!(this._deviceCharWidth<=0&&this._deviceCharHeight<=0)){this._charAtlas=(0,r.acquireTextureAtlas)(this._terminal,this._optionsService.rawOptions,e,this._deviceCellWidth,this._deviceCellHeight,this._deviceCharWidth,this._deviceCharHeight,this._coreBrowserService.dpr),this._charAtlasDisposable.value=(0,l.forwardEvent)(this._charAtlas.onAddTextureAtlasCanvas,this._onAddTextureAtlasCanvas),this._charAtlas.warmUp();for(let e=0;e<this._charAtlas.pages.length;e++)this._bitmapGenerator[e]=new g(this._charAtlas.pages[e].canvas)}}resize(e){this._deviceCellWidth=e.device.cell.width,this._deviceCellHeight=e.device.cell.height,this._deviceCharWidth=e.device.char.width,this._deviceCharHeight=e.device.char.height,this._deviceCharLeft=e.device.char.left,this._deviceCharTop=e.device.char.top,this._canvas.width=e.device.canvas.width,this._canvas.height=e.device.canvas.height,this._canvas.style.width=`${e.css.canvas.width}px`,this._canvas.style.height=`${e.css.canvas.height}px`,this._alpha||this._clearAll(),this._refreshCharAtlas(this._themeService.colors)}clearTextureAtlas(){this._charAtlas?.clearTexture()}_fillCells(e,t,i,s){this._ctx.fillRect(e*this._deviceCellWidth,t*this._deviceCellHeight,i*this._deviceCellWidth,s*this._deviceCellHeight)}_fillMiddleLineAtCells(e,t,i=1){const s=Math.ceil(.5*this._deviceCellHeight);this._ctx.fillRect(e*this._deviceCellWidth,(t+1)*this._deviceCellHeight-s-this._coreBrowserService.dpr,i*this._deviceCellWidth,this._coreBrowserService.dpr)}_fillBottomLineAtCells(e,t,i=1,s=0){this._ctx.fillRect(e*this._deviceCellWidth,(t+1)*this._deviceCellHeight+s-this._coreBrowserService.dpr-1,i*this._deviceCellWidth,this._coreBrowserService.dpr)}_curlyUnderlineAtCell(e,t,i=1){this._ctx.save(),this._ctx.beginPath(),this._ctx.strokeStyle=this._ctx.fillStyle;const s=this._coreBrowserService.dpr;this._ctx.lineWidth=s;for(let r=0;r<i;r++){const i=(e+r)*this._deviceCellWidth,o=(e+r+.5)*this._deviceCellWidth,n=(e+r+1)*this._deviceCellWidth,a=(t+1)*this._deviceCellHeight-s-1,h=a-s,l=a+s;this._ctx.moveTo(i,a),this._ctx.bezierCurveTo(i,h,o,h,o,a),this._ctx.bezierCurveTo(o,l,n,l,n,a)}this._ctx.stroke(),this._ctx.restore()}_dottedUnderlineAtCell(e,t,i=1){this._ctx.save(),this._ctx.beginPath(),this._ctx.strokeStyle=this._ctx.fillStyle;const s=this._coreBrowserService.dpr;this._ctx.lineWidth=s,this._ctx.setLineDash([2*s,s]);const r=e*this._deviceCellWidth,o=(t+1)*this._deviceCellHeight-s-1;this._ctx.moveTo(r,o);for(let t=0;t<i;t++){const s=(e+i+t)*this._deviceCellWidth;this._ctx.lineTo(s,o)}this._ctx.stroke(),this._ctx.closePath(),this._ctx.restore()}_dashedUnderlineAtCell(e,t,i=1){this._ctx.save(),this._ctx.beginPath(),this._ctx.strokeStyle=this._ctx.fillStyle;const s=this._coreBrowserService.dpr;this._ctx.lineWidth=s,this._ctx.setLineDash([4*s,3*s]);const r=e*this._deviceCellWidth,o=(e+i)*this._deviceCellWidth,n=(t+1)*this._deviceCellHeight-s-1;this._ctx.moveTo(r,n),this._ctx.lineTo(o,n),this._ctx.stroke(),this._ctx.closePath(),this._ctx.restore()}_fillLeftLineAtCell(e,t,i){this._ctx.fillRect(e*this._deviceCellWidth,t*this._deviceCellHeight,this._coreBrowserService.dpr*i,this._deviceCellHeight)}_strokeRectAtCell(e,t,i,s){const r=this._coreBrowserService.dpr;this._ctx.lineWidth=r,this._ctx.strokeRect(e*this._deviceCellWidth+r/2,t*this._deviceCellHeight+r/2,i*this._deviceCellWidth-r,s*this._deviceCellHeight-r)}_clearAll(){this._alpha?this._ctx.clearRect(0,0,this._canvas.width,this._canvas.height):(this._ctx.fillStyle=this._themeService.colors.background.css,this._ctx.fillRect(0,0,this._canvas.width,this._canvas.height))}_clearCells(e,t,i,s){this._alpha?this._ctx.clearRect(e*this._deviceCellWidth,t*this._deviceCellHeight,i*this._deviceCellWidth,s*this._deviceCellHeight):(this._ctx.fillStyle=this._themeService.colors.background.css,this._ctx.fillRect(e*this._deviceCellWidth,t*this._deviceCellHeight,i*this._deviceCellWidth,s*this._deviceCellHeight))}_fillCharTrueColor(e,t,i){this._ctx.font=this._getFont(!1,!1),this._ctx.textBaseline=o.TEXT_BASELINE,this._clipRow(i);let s=!1;!1!==this._optionsService.rawOptions.customGlyphs&&(s=(0,n.tryDrawCustomChar)(this._ctx,e.getChars(),t*this._deviceCellWidth,i*this._deviceCellHeight,this._deviceCellWidth,this._deviceCellHeight,this._optionsService.rawOptions.fontSize,this._coreBrowserService.dpr)),s||this._ctx.fillText(e.getChars(),t*this._deviceCellWidth+this._deviceCharLeft,i*this._deviceCellHeight+this._deviceCharTop+this._deviceCharHeight)}_drawChars(e,t,i){const s=e.getChars(),r=e.getCode(),o=e.getWidth();if(this._cellColorResolver.resolve(e,t,this._bufferService.buffer.ydisp+i,this._deviceCellWidth),!this._charAtlas)return;let n;if(n=s&&s.length>1?this._charAtlas.getRasterizedGlyphCombinedChar(s,this._cellColorResolver.result.bg,this._cellColorResolver.result.fg,this._cellColorResolver.result.ext,!0):this._charAtlas.getRasterizedGlyph(e.getCode()||_.WHITESPACE_CELL_CODE,this._cellColorResolver.result.bg,this._cellColorResolver.result.fg,this._cellColorResolver.result.ext,!0),!n.size.x||!n.size.y)return;this._ctx.save(),this._clipRow(i),this._bitmapGenerator[n.texturePage]&&this._charAtlas.pages[n.texturePage].canvas!==this._bitmapGenerator[n.texturePage].canvas&&(this._bitmapGenerator[n.texturePage]?.bitmap?.close(),delete this._bitmapGenerator[n.texturePage]),this._charAtlas.pages[n.texturePage].version!==this._bitmapGenerator[n.texturePage]?.version&&(this._bitmapGenerator[n.texturePage]||(this._bitmapGenerator[n.texturePage]=new g(this._charAtlas.pages[n.texturePage].canvas)),this._bitmapGenerator[n.texturePage].refresh(),this._bitmapGenerator[n.texturePage].version=this._charAtlas.pages[n.texturePage].version);let h=n.size.x;this._optionsService.rawOptions.rescaleOverlappingGlyphs&&(0,a.allowRescaling)(r,o,n.size.x,this._deviceCellWidth)&&(h=this._deviceCellWidth-1),this._ctx.drawImage(this._bitmapGenerator[n.texturePage]?.bitmap||this._charAtlas.pages[n.texturePage].canvas,n.texturePosition.x,n.texturePosition.y,n.size.x,n.size.y,t*this._deviceCellWidth+this._deviceCharLeft-n.offset.x,i*this._deviceCellHeight+this._deviceCharTop-n.offset.y,h,n.size.y),this._ctx.restore()}_clipRow(e){this._ctx.beginPath(),this._ctx.rect(0,e*this._deviceCellHeight,this._bufferService.cols*this._deviceCellWidth,this._deviceCellHeight),this._ctx.clip()}_getFont(e,t){return`${t?"italic":""} ${e?this._optionsService.rawOptions.fontWeightBold:this._optionsService.rawOptions.fontWeight} ${this._optionsService.rawOptions.fontSize*this._coreBrowserService.dpr}px ${this._optionsService.rawOptions.fontFamily}`}}t.BaseRenderLayer=u;class g{get bitmap(){return this._bitmap}constructor(e){this.canvas=e,this._state=0,this._commitTimeout=void 0,this._bitmap=void 0,this.version=-1}refresh(){this._bitmap?.close(),this._bitmap=void 0,d.isSafari||(void 0===this._commitTimeout&&(this._commitTimeout=window.setTimeout((()=>this._generate()),100)),1===this._state&&(this._state=2))}_generate(){0===this._state&&(this._bitmap?.close(),this._bitmap=void 0,this._state=1,window.createImageBitmap(this.canvas).then((e=>{2===this._state?this.refresh():this._bitmap=e,this._state=0})),this._commitTimeout&&(this._commitTimeout=void 0))}}},949:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.CanvasRenderer=void 0;const s=i(627),r=i(56),o=i(374),n=i(345),a=i(859),h=i(873),l=i(43),c=i(630),d=i(744);class _ extends a.Disposable{constructor(e,t,i,_,u,g,f,v,C,p,m){super(),this._terminal=e,this._screenElement=t,this._bufferService=_,this._charSizeService=u,this._optionsService=g,this._coreBrowserService=C,this._themeService=m,this._observerDisposable=this.register(new a.MutableDisposable),this._onRequestRedraw=this.register(new n.EventEmitter),this.onRequestRedraw=this._onRequestRedraw.event,this._onChangeTextureAtlas=this.register(new n.EventEmitter),this.onChangeTextureAtlas=this._onChangeTextureAtlas.event,this._onAddTextureAtlasCanvas=this.register(new n.EventEmitter),this.onAddTextureAtlasCanvas=this._onAddTextureAtlasCanvas.event;const x=this._optionsService.rawOptions.allowTransparency;this._renderLayers=[new d.TextRenderLayer(this._terminal,this._screenElement,0,x,this._bufferService,this._optionsService,f,p,this._coreBrowserService,m),new c.SelectionRenderLayer(this._terminal,this._screenElement,1,this._bufferService,this._coreBrowserService,p,this._optionsService,m),new l.LinkRenderLayer(this._terminal,this._screenElement,2,i,this._bufferService,this._optionsService,p,this._coreBrowserService,m),new h.CursorRenderLayer(this._terminal,this._screenElement,3,this._onRequestRedraw,this._bufferService,this._optionsService,v,this._coreBrowserService,p,m)];for(const e of this._renderLayers)(0,n.forwardEvent)(e.onAddTextureAtlasCanvas,this._onAddTextureAtlasCanvas);this.dimensions=(0,o.createRenderDimensions)(),this._devicePixelRatio=this._coreBrowserService.dpr,this._updateDimensions(),this._observerDisposable.value=(0,r.observeDevicePixelDimensions)(this._renderLayers[0].canvas,this._coreBrowserService.window,((e,t)=>this._setCanvasDevicePixelDimensions(e,t))),this.register(this._coreBrowserService.onWindowChange((e=>{this._observerDisposable.value=(0,r.observeDevicePixelDimensions)(this._renderLayers[0].canvas,e,((e,t)=>this._setCanvasDevicePixelDimensions(e,t)))}))),this.register((0,a.toDisposable)((()=>{for(const e of this._renderLayers)e.dispose();(0,s.removeTerminalFromCache)(this._terminal)})))}get textureAtlas(){return this._renderLayers[0].cacheCanvas}handleDevicePixelRatioChange(){this._devicePixelRatio!==this._coreBrowserService.dpr&&(this._devicePixelRatio=this._coreBrowserService.dpr,this.handleResize(this._bufferService.cols,this._bufferService.rows))}handleResize(e,t){this._updateDimensions();for(const e of this._renderLayers)e.resize(this.dimensions);this._screenElement.style.width=`${this.dimensions.css.canvas.width}px`,this._screenElement.style.height=`${this.dimensions.css.canvas.height}px`}handleCharSizeChanged(){this.handleResize(this._bufferService.cols,this._bufferService.rows)}handleBlur(){this._runOperation((e=>e.handleBlur()))}handleFocus(){this._runOperation((e=>e.handleFocus()))}handleSelectionChanged(e,t,i=!1){this._runOperation((s=>s.handleSelectionChanged(e,t,i))),this._themeService.colors.selectionForeground&&this._onRequestRedraw.fire({start:0,end:this._bufferService.rows-1})}handleCursorMove(){this._runOperation((e=>e.handleCursorMove()))}clear(){this._runOperation((e=>e.reset()))}_runOperation(e){for(const t of this._renderLayers)e(t)}renderRows(e,t){for(const i of this._renderLayers)i.handleGridChanged(e,t)}clearTextureAtlas(){for(const e of this._renderLayers)e.clearTextureAtlas()}_updateDimensions(){if(!this._charSizeService.hasValidSize)return;const e=this._coreBrowserService.dpr;this.dimensions.device.char.width=Math.floor(this._charSizeService.width*e),this.dimensions.device.char.height=Math.ceil(this._charSizeService.height*e),this.dimensions.device.cell.height=Math.floor(this.dimensions.device.char.height*this._optionsService.rawOptions.lineHeight),this.dimensions.device.char.top=1===this._optionsService.rawOptions.lineHeight?0:Math.round((this.dimensions.device.cell.height-this.dimensions.device.char.height)/2),this.dimensions.device.cell.width=this.dimensions.device.char.width+Math.round(this._optionsService.rawOptions.letterSpacing),this.dimensions.device.char.left=Math.floor(this._optionsService.rawOptions.letterSpacing/2),this.dimensions.device.canvas.height=this._bufferService.rows*this.dimensions.device.cell.height,this.dimensions.device.canvas.width=this._bufferService.cols*this.dimensions.device.cell.width,this.dimensions.css.canvas.height=Math.round(this.dimensions.device.canvas.height/e),this.dimensions.css.canvas.width=Math.round(this.dimensions.device.canvas.width/e),this.dimensions.css.cell.height=this.dimensions.css.canvas.height/this._bufferService.rows,this.dimensions.css.cell.width=this.dimensions.css.canvas.width/this._bufferService.cols}_setCanvasDevicePixelDimensions(e,t){this.dimensions.device.canvas.height=t,this.dimensions.device.canvas.width=e;for(const e of this._renderLayers)e.resize(this.dimensions);this._requestRedrawViewport()}_requestRedrawViewport(){this._onRequestRedraw.fire({start:0,end:this._bufferService.rows-1})}}t.CanvasRenderer=_},873:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.CursorRenderLayer=void 0;const s=i(457),r=i(859),o=i(399),n=i(782),a=i(903);class h extends a.BaseRenderLayer{constructor(e,t,i,s,o,a,h,l,c,d){super(e,t,"cursor",i,!0,d,o,a,c,l),this._onRequestRedraw=s,this._coreService=h,this._cursorBlinkStateManager=this.register(new r.MutableDisposable),this._cell=new n.CellData,this._state={x:0,y:0,isFocused:!1,style:"",width:0},this._cursorRenderers={bar:this._renderBarCursor.bind(this),block:this._renderBlockCursor.bind(this),underline:this._renderUnderlineCursor.bind(this),outline:this._renderOutlineCursor.bind(this)},this.register(a.onOptionChange((()=>this._handleOptionsChanged()))),this._handleOptionsChanged()}resize(e){super.resize(e),this._state={x:0,y:0,isFocused:!1,style:"",width:0}}reset(){this._clearCursor(),this._cursorBlinkStateManager.value?.restartBlinkAnimation(),this._handleOptionsChanged()}handleBlur(){this._cursorBlinkStateManager.value?.pause(),this._onRequestRedraw.fire({start:this._bufferService.buffer.y,end:this._bufferService.buffer.y})}handleFocus(){this._cursorBlinkStateManager.value?.resume(),this._onRequestRedraw.fire({start:this._bufferService.buffer.y,end:this._bufferService.buffer.y})}_handleOptionsChanged(){this._optionsService.rawOptions.cursorBlink?this._cursorBlinkStateManager.value||(this._cursorBlinkStateManager.value=new s.CursorBlinkStateManager((()=>this._render(!0)),this._coreBrowserService)):this._cursorBlinkStateManager.clear(),this._onRequestRedraw.fire({start:this._bufferService.buffer.y,end:this._bufferService.buffer.y})}handleCursorMove(){this._cursorBlinkStateManager.value?.restartBlinkAnimation()}handleGridChanged(e,t){!this._cursorBlinkStateManager.value||this._cursorBlinkStateManager.value.isPaused?this._render(!1):this._cursorBlinkStateManager.value.restartBlinkAnimation()}_render(e){if(!this._coreService.isCursorInitialized||this._coreService.isCursorHidden)return void this._clearCursor();const t=this._bufferService.buffer.ybase+this._bufferService.buffer.y,i=t-this._bufferService.buffer.ydisp;if(i<0||i>=this._bufferService.rows)return void this._clearCursor();const s=Math.min(this._bufferService.buffer.x,this._bufferService.cols-1);if(this._bufferService.buffer.lines.get(t).loadCell(s,this._cell),void 0!==this._cell.content){if(!this._coreBrowserService.isFocused){this._clearCursor(),this._ctx.save(),this._ctx.fillStyle=this._themeService.colors.cursor.css;const e=this._optionsService.rawOptions.cursorStyle,t=this._optionsService.rawOptions.cursorInactiveStyle;return t&&"none"!==t&&this._cursorRenderers[t](s,i,this._cell),this._ctx.restore(),this._state.x=s,this._state.y=i,this._state.isFocused=!1,this._state.style=e,void(this._state.width=this._cell.getWidth())}if(!this._cursorBlinkStateManager.value||this._cursorBlinkStateManager.value.isCursorVisible){if(this._state){if(this._state.x===s&&this._state.y===i&&this._state.isFocused===this._coreBrowserService.isFocused&&this._state.style===this._optionsService.rawOptions.cursorStyle&&this._state.width===this._cell.getWidth())return;this._clearCursor()}this._ctx.save(),this._cursorRenderers[this._optionsService.rawOptions.cursorStyle||"block"](s,i,this._cell),this._ctx.restore(),this._state.x=s,this._state.y=i,this._state.isFocused=!1,this._state.style=this._optionsService.rawOptions.cursorStyle,this._state.width=this._cell.getWidth()}else this._clearCursor()}}_clearCursor(){this._state&&(o.isFirefox||this._coreBrowserService.dpr<1?this._clearAll():this._clearCells(this._state.x,this._state.y,this._state.width,1),this._state={x:0,y:0,isFocused:!1,style:"",width:0})}_renderBarCursor(e,t,i){this._ctx.save(),this._ctx.fillStyle=this._themeService.colors.cursor.css,this._fillLeftLineAtCell(e,t,this._optionsService.rawOptions.cursorWidth),this._ctx.restore()}_renderBlockCursor(e,t,i){this._ctx.save(),this._ctx.fillStyle=this._themeService.colors.cursor.css,this._fillCells(e,t,i.getWidth(),1),this._ctx.fillStyle=this._themeService.colors.cursorAccent.css,this._fillCharTrueColor(i,e,t),this._ctx.restore()}_renderUnderlineCursor(e,t,i){this._ctx.save(),this._ctx.fillStyle=this._themeService.colors.cursor.css,this._fillBottomLineAtCells(e,t),this._ctx.restore()}_renderOutlineCursor(e,t,i){this._ctx.save(),this._ctx.strokeStyle=this._themeService.colors.cursor.css,this._strokeRectAtCell(e,t,i.getWidth(),1),this._ctx.restore()}}t.CursorRenderLayer=h},574:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.GridCache=void 0,t.GridCache=class{constructor(){this.cache=[]}resize(e,t){for(let i=0;i<e;i++){this.cache.length<=i&&this.cache.push([]);for(let e=this.cache[i].length;e<t;e++)this.cache[i].push(void 0);this.cache[i].length=t}this.cache.length=e}clear(){for(let e=0;e<this.cache.length;e++)for(let t=0;t<this.cache[e].length;t++)this.cache[e][t]=void 0}}},43:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.LinkRenderLayer=void 0;const s=i(197),r=i(237),o=i(903);class n extends o.BaseRenderLayer{constructor(e,t,i,s,r,o,n,a,h){super(e,t,"link",i,!0,h,r,o,n,a),this.register(s.onShowLinkUnderline((e=>this._handleShowLinkUnderline(e)))),this.register(s.onHideLinkUnderline((e=>this._handleHideLinkUnderline(e))))}resize(e){super.resize(e),this._state=void 0}reset(){this._clearCurrentLink()}_clearCurrentLink(){if(this._state){this._clearCells(this._state.x1,this._state.y1,this._state.cols-this._state.x1,1);const e=this._state.y2-this._state.y1-1;e>0&&this._clearCells(0,this._state.y1+1,this._state.cols,e),this._clearCells(0,this._state.y2,this._state.x2,1),this._state=void 0}}_handleShowLinkUnderline(e){if(e.fg===r.INVERTED_DEFAULT_COLOR?this._ctx.fillStyle=this._themeService.colors.background.css:e.fg&&(0,s.is256Color)(e.fg)?this._ctx.fillStyle=this._themeService.colors.ansi[e.fg].css:this._ctx.fillStyle=this._themeService.colors.foreground.css,e.y1===e.y2)this._fillBottomLineAtCells(e.x1,e.y1,e.x2-e.x1);else{this._fillBottomLineAtCells(e.x1,e.y1,e.cols-e.x1);for(let t=e.y1+1;t<e.y2;t++)this._fillBottomLineAtCells(0,t,e.cols);this._fillBottomLineAtCells(0,e.y2,e.x2)}this._state=e}_handleHideLinkUnderline(e){this._clearCurrentLink()}}t.LinkRenderLayer=n},630:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.SelectionRenderLayer=void 0;const s=i(903);class r extends s.BaseRenderLayer{constructor(e,t,i,s,r,o,n,a){super(e,t,"selection",i,!0,a,s,n,o,r),this._clearState()}_clearState(){this._state={start:void 0,end:void 0,columnSelectMode:void 0,ydisp:void 0}}resize(e){super.resize(e),this._selectionModel.selectionStart&&this._selectionModel.selectionEnd&&(this._clearState(),this._redrawSelection(this._selectionModel.selectionStart,this._selectionModel.selectionEnd,this._selectionModel.columnSelectMode))}reset(){this._state.start&&this._state.end&&(this._clearState(),this._clearAll())}handleBlur(){this.reset(),this._redrawSelection(this._selectionModel.selectionStart,this._selectionModel.selectionEnd,this._selectionModel.columnSelectMode)}handleFocus(){this.reset(),this._redrawSelection(this._selectionModel.selectionStart,this._selectionModel.selectionEnd,this._selectionModel.columnSelectMode)}handleSelectionChanged(e,t,i){super.handleSelectionChanged(e,t,i),this._redrawSelection(e,t,i)}_redrawSelection(e,t,i){if(!this._didStateChange(e,t,i,this._bufferService.buffer.ydisp))return;if(this._clearAll(),!e||!t)return void this._clearState();const s=e[1]-this._bufferService.buffer.ydisp,r=t[1]-this._bufferService.buffer.ydisp,o=Math.max(s,0),n=Math.min(r,this._bufferService.rows-1);if(o>=this._bufferService.rows||n<0)this._state.ydisp=this._bufferService.buffer.ydisp;else{if(this._ctx.fillStyle=(this._coreBrowserService.isFocused?this._themeService.colors.selectionBackgroundTransparent:this._themeService.colors.selectionInactiveBackgroundTransparent).css,i){const i=e[0],s=t[0]-i,r=n-o+1;this._fillCells(i,o,s,r)}else{const i=s===o?e[0]:0,a=o===r?t[0]:this._bufferService.cols;this._fillCells(i,o,a-i,1);const h=Math.max(n-o-1,0);if(this._fillCells(0,o+1,this._bufferService.cols,h),o!==n){const e=r===n?t[0]:this._bufferService.cols;this._fillCells(0,n,e,1)}}this._state.start=[e[0],e[1]],this._state.end=[t[0],t[1]],this._state.columnSelectMode=i,this._state.ydisp=this._bufferService.buffer.ydisp}}_didStateChange(e,t,i,s){return!this._areCoordinatesEqual(e,this._state.start)||!this._areCoordinatesEqual(t,this._state.end)||i!==this._state.columnSelectMode||s!==this._state.ydisp}_areCoordinatesEqual(e,t){return!(!e||!t)&&e[0]===t[0]&&e[1]===t[1]}}t.SelectionRenderLayer=r},744:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.TextRenderLayer=void 0;const s=i(577),r=i(147),o=i(782),n=i(855),a=i(903),h=i(574);class l extends a.BaseRenderLayer{constructor(e,t,i,s,r,n,a,l,c,d){super(e,t,"text",i,s,d,r,n,l,c),this._characterJoinerService=a,this._characterWidth=0,this._characterFont="",this._characterOverlapCache={},this._workCell=new o.CellData,this._state=new h.GridCache,this.register(n.onSpecificOptionChange("allowTransparency",(e=>this._setTransparency(e))))}resize(e){super.resize(e);const t=this._getFont(!1,!1);this._characterWidth===e.device.char.width&&this._characterFont===t||(this._characterWidth=e.device.char.width,this._characterFont=t,this._characterOverlapCache={}),this._state.clear(),this._state.resize(this._bufferService.cols,this._bufferService.rows)}reset(){this._state.clear(),this._clearAll()}_forEachCell(e,t,i){for(let r=e;r<=t;r++){const e=r+this._bufferService.buffer.ydisp,t=this._bufferService.buffer.lines.get(e),o=this._characterJoinerService.getJoinedCharacters(e);for(let e=0;e<this._bufferService.cols;e++){t.loadCell(e,this._workCell);let a=this._workCell,h=!1,l=e;if(0!==a.getWidth()){if(o.length>0&&e===o[0][0]){h=!0;const e=o.shift();a=new s.JoinedCellData(this._workCell,t.translateToString(!0,e[0],e[1]),e[1]-e[0]),l=e[1]-1}!h&&this._isOverlapping(a)&&l<t.length-1&&t.getCodePoint(l+1)===n.NULL_CELL_CODE&&(a.content&=-12582913,a.content|=2<<22),i(a,e,r),e=l}}}}_drawBackground(e,t){const i=this._ctx,s=this._bufferService.cols;let o=0,n=0,a=null;i.save(),this._forEachCell(e,t,((e,t,h)=>{let l=null;e.isInverse()?l=e.isFgDefault()?this._themeService.colors.foreground.css:e.isFgRGB()?`rgb(${r.AttributeData.toColorRGB(e.getFgColor()).join(",")})`:this._themeService.colors.ansi[e.getFgColor()].css:e.isBgRGB()?l=`rgb(${r.AttributeData.toColorRGB(e.getBgColor()).join(",")})`:e.isBgPalette()&&(l=this._themeService.colors.ansi[e.getBgColor()].css);let c=!1;this._decorationService.forEachDecorationAtCell(t,this._bufferService.buffer.ydisp+h,void 0,(e=>{"top"!==e.options.layer&&c||(e.backgroundColorRGB&&(l=e.backgroundColorRGB.css),c="top"===e.options.layer)})),null===a&&(o=t,n=h),h!==n?(i.fillStyle=a||"",this._fillCells(o,n,s-o,1),o=t,n=h):a!==l&&(i.fillStyle=a||"",this._fillCells(o,n,t-o,1),o=t,n=h),a=l})),null!==a&&(i.fillStyle=a,this._fillCells(o,n,s-o,1)),i.restore()}_drawForeground(e,t){this._forEachCell(e,t,((e,t,i)=>this._drawChars(e,t,i)))}handleGridChanged(e,t){0!==this._state.cache.length&&(this._charAtlas&&this._charAtlas.beginFrame(),this._clearCells(0,e,this._bufferService.cols,t-e+1),this._drawBackground(e,t),this._drawForeground(e,t))}_isOverlapping(e){if(1!==e.getWidth())return!1;if(e.getCode()<256)return!1;const t=e.getChars();if(this._characterOverlapCache.hasOwnProperty(t))return this._characterOverlapCache[t];this._ctx.save(),this._ctx.font=this._characterFont;const i=Math.floor(this._ctx.measureText(t).width)>this._characterWidth;return this._ctx.restore(),this._characterOverlapCache[t]=i,i}}t.TextRenderLayer=l},274:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.CellColorResolver=void 0;const s=i(855),r=i(160),o=i(374);let n,a=0,h=0,l=!1,c=!1,d=!1,_=0;t.CellColorResolver=class{constructor(e,t,i,s,r,o){this._terminal=e,this._optionService=t,this._selectionRenderModel=i,this._decorationService=s,this._coreBrowserService=r,this._themeService=o,this.result={fg:0,bg:0,ext:0}}resolve(e,t,i,u){if(this.result.bg=e.bg,this.result.fg=e.fg,this.result.ext=268435456&e.bg?e.extended.ext:0,h=0,a=0,c=!1,l=!1,d=!1,n=this._themeService.colors,_=0,e.getCode()!==s.NULL_CELL_CODE&&4===e.extended.underlineStyle){const e=Math.max(1,Math.floor(this._optionService.rawOptions.fontSize*this._coreBrowserService.dpr/15));_=t*u%(2*Math.round(e))}if(this._decorationService.forEachDecorationAtCell(t,i,"bottom",(e=>{e.backgroundColorRGB&&(h=e.backgroundColorRGB.rgba>>8&16777215,c=!0),e.foregroundColorRGB&&(a=e.foregroundColorRGB.rgba>>8&16777215,l=!0)})),d=this._selectionRenderModel.isCellSelected(this._terminal,t,i),d){if(67108864&this.result.fg||0!=(50331648&this.result.bg)){if(67108864&this.result.fg)switch(50331648&this.result.fg){case 16777216:case 33554432:h=this._themeService.colors.ansi[255&this.result.fg].rgba;break;case 50331648:h=(16777215&this.result.fg)<<8|255;break;default:h=this._themeService.colors.foreground.rgba}else switch(50331648&this.result.bg){case 16777216:case 33554432:h=this._themeService.colors.ansi[255&this.result.bg].rgba;break;case 50331648:h=(16777215&this.result.bg)<<8|255}h=r.rgba.blend(h,4294967040&(this._coreBrowserService.isFocused?n.selectionBackgroundOpaque:n.selectionInactiveBackgroundOpaque).rgba|128)>>8&16777215}else h=(this._coreBrowserService.isFocused?n.selectionBackgroundOpaque:n.selectionInactiveBackgroundOpaque).rgba>>8&16777215;if(c=!0,n.selectionForeground&&(a=n.selectionForeground.rgba>>8&16777215,l=!0),(0,o.treatGlyphAsBackgroundColor)(e.getCode())){if(67108864&this.result.fg&&0==(50331648&this.result.bg))a=(this._coreBrowserService.isFocused?n.selectionBackgroundOpaque:n.selectionInactiveBackgroundOpaque).rgba>>8&16777215;else{if(67108864&this.result.fg)switch(50331648&this.result.bg){case 16777216:case 33554432:a=this._themeService.colors.ansi[255&this.result.bg].rgba;break;case 50331648:a=(16777215&this.result.bg)<<8|255}else switch(50331648&this.result.fg){case 16777216:case 33554432:a=this._themeService.colors.ansi[255&this.result.fg].rgba;break;case 50331648:a=(16777215&this.result.fg)<<8|255;break;default:a=this._themeService.colors.foreground.rgba}a=r.rgba.blend(a,4294967040&(this._coreBrowserService.isFocused?n.selectionBackgroundOpaque:n.selectionInactiveBackgroundOpaque).rgba|128)>>8&16777215}l=!0}}this._decorationService.forEachDecorationAtCell(t,i,"top",(e=>{e.backgroundColorRGB&&(h=e.backgroundColorRGB.rgba>>8&16777215,c=!0),e.foregroundColorRGB&&(a=e.foregroundColorRGB.rgba>>8&16777215,l=!0)})),c&&(h=d?-16777216&e.bg&-134217729|h|50331648:-16777216&e.bg|h|50331648),l&&(a=-16777216&e.fg&-67108865|a|50331648),67108864&this.result.fg&&(c&&!l&&(a=0==(50331648&this.result.bg)?-134217728&this.result.fg|16777215&n.background.rgba>>8|50331648:-134217728&this.result.fg|67108863&this.result.bg,l=!0),!c&&l&&(h=0==(50331648&this.result.fg)?-67108864&this.result.bg|16777215&n.foreground.rgba>>8|50331648:-67108864&this.result.bg|67108863&this.result.fg,c=!0)),n=void 0,this.result.bg=c?h:this.result.bg,this.result.fg=l?a:this.result.fg,this.result.ext&=536870911,this.result.ext|=_<<29&3758096384}}},627:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.removeTerminalFromCache=t.acquireTextureAtlas=void 0;const s=i(509),r=i(197),o=[];t.acquireTextureAtlas=function(e,t,i,n,a,h,l,c){const d=(0,r.generateConfig)(n,a,h,l,t,i,c);for(let t=0;t<o.length;t++){const i=o[t],s=i.ownedBy.indexOf(e);if(s>=0){if((0,r.configEquals)(i.config,d))return i.atlas;1===i.ownedBy.length?(i.atlas.dispose(),o.splice(t,1)):i.ownedBy.splice(s,1);break}}for(let t=0;t<o.length;t++){const i=o[t];if((0,r.configEquals)(i.config,d))return i.ownedBy.push(e),i.atlas}const _=e._core,u={atlas:new s.TextureAtlas(document,d,_.unicodeService),config:d,ownedBy:[e]};return o.push(u),u.atlas},t.removeTerminalFromCache=function(e){for(let t=0;t<o.length;t++){const i=o[t].ownedBy.indexOf(e);if(-1!==i){1===o[t].ownedBy.length?(o[t].atlas.dispose(),o.splice(t,1)):o[t].ownedBy.splice(i,1);break}}}},197:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.is256Color=t.configEquals=t.generateConfig=void 0;const s=i(160);t.generateConfig=function(e,t,i,r,o,n,a){const h={foreground:n.foreground,background:n.background,cursor:s.NULL_COLOR,cursorAccent:s.NULL_COLOR,selectionForeground:s.NULL_COLOR,selectionBackgroundTransparent:s.NULL_COLOR,selectionBackgroundOpaque:s.NULL_COLOR,selectionInactiveBackgroundTransparent:s.NULL_COLOR,selectionInactiveBackgroundOpaque:s.NULL_COLOR,ansi:n.ansi.slice(),contrastCache:n.contrastCache,halfContrastCache:n.halfContrastCache};return{customGlyphs:o.customGlyphs,devicePixelRatio:a,letterSpacing:o.letterSpacing,lineHeight:o.lineHeight,deviceCellWidth:e,deviceCellHeight:t,deviceCharWidth:i,deviceCharHeight:r,fontFamily:o.fontFamily,fontSize:o.fontSize,fontWeight:o.fontWeight,fontWeightBold:o.fontWeightBold,allowTransparency:o.allowTransparency,drawBoldTextInBrightColors:o.drawBoldTextInBrightColors,minimumContrastRatio:o.minimumContrastRatio,colors:h}},t.configEquals=function(e,t){for(let i=0;i<e.colors.ansi.length;i++)if(e.colors.ansi[i].rgba!==t.colors.ansi[i].rgba)return!1;return e.devicePixelRatio===t.devicePixelRatio&&e.customGlyphs===t.customGlyphs&&e.lineHeight===t.lineHeight&&e.letterSpacing===t.letterSpacing&&e.fontFamily===t.fontFamily&&e.fontSize===t.fontSize&&e.fontWeight===t.fontWeight&&e.fontWeightBold===t.fontWeightBold&&e.allowTransparency===t.allowTransparency&&e.deviceCharWidth===t.deviceCharWidth&&e.deviceCharHeight===t.deviceCharHeight&&e.drawBoldTextInBrightColors===t.drawBoldTextInBrightColors&&e.minimumContrastRatio===t.minimumContrastRatio&&e.colors.foreground.rgba===t.colors.foreground.rgba&&e.colors.background.rgba===t.colors.background.rgba},t.is256Color=function(e){return 16777216==(50331648&e)||33554432==(50331648&e)}},237:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.TEXT_BASELINE=t.DIM_OPACITY=t.INVERTED_DEFAULT_COLOR=void 0;const s=i(399);t.INVERTED_DEFAULT_COLOR=257,t.DIM_OPACITY=.5,t.TEXT_BASELINE=s.isFirefox||s.isLegacyEdge?"bottom":"ideographic"},457:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.CursorBlinkStateManager=void 0;t.CursorBlinkStateManager=class{constructor(e,t){this._renderCallback=e,this._coreBrowserService=t,this.isCursorVisible=!0,this._coreBrowserService.isFocused&&this._restartInterval()}get isPaused(){return!(this._blinkStartTimeout||this._blinkInterval)}dispose(){this._blinkInterval&&(this._coreBrowserService.window.clearInterval(this._blinkInterval),this._blinkInterval=void 0),this._blinkStartTimeout&&(this._coreBrowserService.window.clearTimeout(this._blinkStartTimeout),this._blinkStartTimeout=void 0),this._animationFrame&&(this._coreBrowserService.window.cancelAnimationFrame(this._animationFrame),this._animationFrame=void 0)}restartBlinkAnimation(){this.isPaused||(this._animationTimeRestarted=Date.now(),this.isCursorVisible=!0,this._animationFrame||(this._animationFrame=this._coreBrowserService.window.requestAnimationFrame((()=>{this._renderCallback(),this._animationFrame=void 0}))))}_restartInterval(e=600){this._blinkInterval&&(this._coreBrowserService.window.clearInterval(this._blinkInterval),this._blinkInterval=void 0),this._blinkStartTimeout=this._coreBrowserService.window.setTimeout((()=>{if(this._animationTimeRestarted){const e=600-(Date.now()-this._animationTimeRestarted);if(this._animationTimeRestarted=void 0,e>0)return void this._restartInterval(e)}this.isCursorVisible=!1,this._animationFrame=this._coreBrowserService.window.requestAnimationFrame((()=>{this._renderCallback(),this._animationFrame=void 0})),this._blinkInterval=this._coreBrowserService.window.setInterval((()=>{if(this._animationTimeRestarted){const e=600-(Date.now()-this._animationTimeRestarted);return this._animationTimeRestarted=void 0,void this._restartInterval(e)}this.isCursorVisible=!this.isCursorVisible,this._animationFrame=this._coreBrowserService.window.requestAnimationFrame((()=>{this._renderCallback(),this._animationFrame=void 0}))}),600)}),e)}pause(){this.isCursorVisible=!0,this._blinkInterval&&(this._coreBrowserService.window.clearInterval(this._blinkInterval),this._blinkInterval=void 0),this._blinkStartTimeout&&(this._coreBrowserService.window.clearTimeout(this._blinkStartTimeout),this._blinkStartTimeout=void 0),this._animationFrame&&(this._coreBrowserService.window.cancelAnimationFrame(this._animationFrame),this._animationFrame=void 0)}resume(){this.pause(),this._animationTimeRestarted=void 0,this._restartInterval(),this.restartBlinkAnimation()}}},860:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.tryDrawCustomChar=t.powerlineDefinitions=t.boxDrawingDefinitions=t.blockElementDefinitions=void 0;const s=i(374);t.blockElementDefinitions={"▀":[{x:0,y:0,w:8,h:4}],"▁":[{x:0,y:7,w:8,h:1}],"▂":[{x:0,y:6,w:8,h:2}],"▃":[{x:0,y:5,w:8,h:3}],"▄":[{x:0,y:4,w:8,h:4}],"▅":[{x:0,y:3,w:8,h:5}],"▆":[{x:0,y:2,w:8,h:6}],"▇":[{x:0,y:1,w:8,h:7}],"█":[{x:0,y:0,w:8,h:8}],"▉":[{x:0,y:0,w:7,h:8}],"▊":[{x:0,y:0,w:6,h:8}],"▋":[{x:0,y:0,w:5,h:8}],"▌":[{x:0,y:0,w:4,h:8}],"▍":[{x:0,y:0,w:3,h:8}],"▎":[{x:0,y:0,w:2,h:8}],"▏":[{x:0,y:0,w:1,h:8}],"▐":[{x:4,y:0,w:4,h:8}],"▔":[{x:0,y:0,w:8,h:1}],"▕":[{x:7,y:0,w:1,h:8}],"▖":[{x:0,y:4,w:4,h:4}],"▗":[{x:4,y:4,w:4,h:4}],"▘":[{x:0,y:0,w:4,h:4}],"▙":[{x:0,y:0,w:4,h:8},{x:0,y:4,w:8,h:4}],"▚":[{x:0,y:0,w:4,h:4},{x:4,y:4,w:4,h:4}],"▛":[{x:0,y:0,w:4,h:8},{x:4,y:0,w:4,h:4}],"▜":[{x:0,y:0,w:8,h:4},{x:4,y:0,w:4,h:8}],"▝":[{x:4,y:0,w:4,h:4}],"▞":[{x:4,y:0,w:4,h:4},{x:0,y:4,w:4,h:4}],"▟":[{x:4,y:0,w:4,h:8},{x:0,y:4,w:8,h:4}],"🭰":[{x:1,y:0,w:1,h:8}],"🭱":[{x:2,y:0,w:1,h:8}],"🭲":[{x:3,y:0,w:1,h:8}],"🭳":[{x:4,y:0,w:1,h:8}],"🭴":[{x:5,y:0,w:1,h:8}],"🭵":[{x:6,y:0,w:1,h:8}],"🭶":[{x:0,y:1,w:8,h:1}],"🭷":[{x:0,y:2,w:8,h:1}],"🭸":[{x:0,y:3,w:8,h:1}],"🭹":[{x:0,y:4,w:8,h:1}],"🭺":[{x:0,y:5,w:8,h:1}],"🭻":[{x:0,y:6,w:8,h:1}],"🭼":[{x:0,y:0,w:1,h:8},{x:0,y:7,w:8,h:1}],"🭽":[{x:0,y:0,w:1,h:8},{x:0,y:0,w:8,h:1}],"🭾":[{x:7,y:0,w:1,h:8},{x:0,y:0,w:8,h:1}],"🭿":[{x:7,y:0,w:1,h:8},{x:0,y:7,w:8,h:1}],"🮀":[{x:0,y:0,w:8,h:1},{x:0,y:7,w:8,h:1}],"🮁":[{x:0,y:0,w:8,h:1},{x:0,y:2,w:8,h:1},{x:0,y:4,w:8,h:1},{x:0,y:7,w:8,h:1}],"🮂":[{x:0,y:0,w:8,h:2}],"🮃":[{x:0,y:0,w:8,h:3}],"🮄":[{x:0,y:0,w:8,h:5}],"🮅":[{x:0,y:0,w:8,h:6}],"🮆":[{x:0,y:0,w:8,h:7}],"🮇":[{x:6,y:0,w:2,h:8}],"🮈":[{x:5,y:0,w:3,h:8}],"🮉":[{x:3,y:0,w:5,h:8}],"🮊":[{x:2,y:0,w:6,h:8}],"🮋":[{x:1,y:0,w:7,h:8}],"🮕":[{x:0,y:0,w:2,h:2},{x:4,y:0,w:2,h:2},{x:2,y:2,w:2,h:2},{x:6,y:2,w:2,h:2},{x:0,y:4,w:2,h:2},{x:4,y:4,w:2,h:2},{x:2,y:6,w:2,h:2},{x:6,y:6,w:2,h:2}],"🮖":[{x:2,y:0,w:2,h:2},{x:6,y:0,w:2,h:2},{x:0,y:2,w:2,h:2},{x:4,y:2,w:2,h:2},{x:2,y:4,w:2,h:2},{x:6,y:4,w:2,h:2},{x:0,y:6,w:2,h:2},{x:4,y:6,w:2,h:2}],"🮗":[{x:0,y:2,w:8,h:2},{x:0,y:6,w:8,h:2}]};const r={"░":[[1,0,0,0],[0,0,0,0],[0,0,1,0],[0,0,0,0]],"▒":[[1,0],[0,0],[0,1],[0,0]],"▓":[[0,1],[1,1],[1,0],[1,1]]};t.boxDrawingDefinitions={"─":{1:"M0,.5 L1,.5"},"━":{3:"M0,.5 L1,.5"},"│":{1:"M.5,0 L.5,1"},"┃":{3:"M.5,0 L.5,1"},"┌":{1:"M0.5,1 L.5,.5 L1,.5"},"┏":{3:"M0.5,1 L.5,.5 L1,.5"},"┐":{1:"M0,.5 L.5,.5 L.5,1"},"┓":{3:"M0,.5 L.5,.5 L.5,1"},"└":{1:"M.5,0 L.5,.5 L1,.5"},"┗":{3:"M.5,0 L.5,.5 L1,.5"},"┘":{1:"M.5,0 L.5,.5 L0,.5"},"┛":{3:"M.5,0 L.5,.5 L0,.5"},"├":{1:"M.5,0 L.5,1 M.5,.5 L1,.5"},"┣":{3:"M.5,0 L.5,1 M.5,.5 L1,.5"},"┤":{1:"M.5,0 L.5,1 M.5,.5 L0,.5"},"┫":{3:"M.5,0 L.5,1 M.5,.5 L0,.5"},"┬":{1:"M0,.5 L1,.5 M.5,.5 L.5,1"},"┳":{3:"M0,.5 L1,.5 M.5,.5 L.5,1"},"┴":{1:"M0,.5 L1,.5 M.5,.5 L.5,0"},"┻":{3:"M0,.5 L1,.5 M.5,.5 L.5,0"},"┼":{1:"M0,.5 L1,.5 M.5,0 L.5,1"},"╋":{3:"M0,.5 L1,.5 M.5,0 L.5,1"},"╴":{1:"M.5,.5 L0,.5"},"╸":{3:"M.5,.5 L0,.5"},"╵":{1:"M.5,.5 L.5,0"},"╹":{3:"M.5,.5 L.5,0"},"╶":{1:"M.5,.5 L1,.5"},"╺":{3:"M.5,.5 L1,.5"},"╷":{1:"M.5,.5 L.5,1"},"╻":{3:"M.5,.5 L.5,1"},"═":{1:(e,t)=>`M0,${.5-t} L1,${.5-t} M0,${.5+t} L1,${.5+t}`},"║":{1:(e,t)=>`M${.5-e},0 L${.5-e},1 M${.5+e},0 L${.5+e},1`},"╒":{1:(e,t)=>`M.5,1 L.5,${.5-t} L1,${.5-t} M.5,${.5+t} L1,${.5+t}`},"╓":{1:(e,t)=>`M${.5-e},1 L${.5-e},.5 L1,.5 M${.5+e},.5 L${.5+e},1`},"╔":{1:(e,t)=>`M1,${.5-t} L${.5-e},${.5-t} L${.5-e},1 M1,${.5+t} L${.5+e},${.5+t} L${.5+e},1`},"╕":{1:(e,t)=>`M0,${.5-t} L.5,${.5-t} L.5,1 M0,${.5+t} L.5,${.5+t}`},"╖":{1:(e,t)=>`M${.5+e},1 L${.5+e},.5 L0,.5 M${.5-e},.5 L${.5-e},1`},"╗":{1:(e,t)=>`M0,${.5+t} L${.5-e},${.5+t} L${.5-e},1 M0,${.5-t} L${.5+e},${.5-t} L${.5+e},1`},"╘":{1:(e,t)=>`M.5,0 L.5,${.5+t} L1,${.5+t} M.5,${.5-t} L1,${.5-t}`},"╙":{1:(e,t)=>`M1,.5 L${.5-e},.5 L${.5-e},0 M${.5+e},.5 L${.5+e},0`},"╚":{1:(e,t)=>`M1,${.5-t} L${.5+e},${.5-t} L${.5+e},0 M1,${.5+t} L${.5-e},${.5+t} L${.5-e},0`},"╛":{1:(e,t)=>`M0,${.5+t} L.5,${.5+t} L.5,0 M0,${.5-t} L.5,${.5-t}`},"╜":{1:(e,t)=>`M0,.5 L${.5+e},.5 L${.5+e},0 M${.5-e},.5 L${.5-e},0`},"╝":{1:(e,t)=>`M0,${.5-t} L${.5-e},${.5-t} L${.5-e},0 M0,${.5+t} L${.5+e},${.5+t} L${.5+e},0`},"╞":{1:(e,t)=>`M.5,0 L.5,1 M.5,${.5-t} L1,${.5-t} M.5,${.5+t} L1,${.5+t}`},"╟":{1:(e,t)=>`M${.5-e},0 L${.5-e},1 M${.5+e},0 L${.5+e},1 M${.5+e},.5 L1,.5`},"╠":{1:(e,t)=>`M${.5-e},0 L${.5-e},1 M1,${.5+t} L${.5+e},${.5+t} L${.5+e},1 M1,${.5-t} L${.5+e},${.5-t} L${.5+e},0`},"╡":{1:(e,t)=>`M.5,0 L.5,1 M0,${.5-t} L.5,${.5-t} M0,${.5+t} L.5,${.5+t}`},"╢":{1:(e,t)=>`M0,.5 L${.5-e},.5 M${.5-e},0 L${.5-e},1 M${.5+e},0 L${.5+e},1`},"╣":{1:(e,t)=>`M${.5+e},0 L${.5+e},1 M0,${.5+t} L${.5-e},${.5+t} L${.5-e},1 M0,${.5-t} L${.5-e},${.5-t} L${.5-e},0`},"╤":{1:(e,t)=>`M0,${.5-t} L1,${.5-t} M0,${.5+t} L1,${.5+t} M.5,${.5+t} L.5,1`},"╥":{1:(e,t)=>`M0,.5 L1,.5 M${.5-e},.5 L${.5-e},1 M${.5+e},.5 L${.5+e},1`},"╦":{1:(e,t)=>`M0,${.5-t} L1,${.5-t} M0,${.5+t} L${.5-e},${.5+t} L${.5-e},1 M1,${.5+t} L${.5+e},${.5+t} L${.5+e},1`},"╧":{1:(e,t)=>`M.5,0 L.5,${.5-t} M0,${.5-t} L1,${.5-t} M0,${.5+t} L1,${.5+t}`},"╨":{1:(e,t)=>`M0,.5 L1,.5 M${.5-e},.5 L${.5-e},0 M${.5+e},.5 L${.5+e},0`},"╩":{1:(e,t)=>`M0,${.5+t} L1,${.5+t} M0,${.5-t} L${.5-e},${.5-t} L${.5-e},0 M1,${.5-t} L${.5+e},${.5-t} L${.5+e},0`},"╪":{1:(e,t)=>`M.5,0 L.5,1 M0,${.5-t} L1,${.5-t} M0,${.5+t} L1,${.5+t}`},"╫":{1:(e,t)=>`M0,.5 L1,.5 M${.5-e},0 L${.5-e},1 M${.5+e},0 L${.5+e},1`},"╬":{1:(e,t)=>`M0,${.5+t} L${.5-e},${.5+t} L${.5-e},1 M1,${.5+t} L${.5+e},${.5+t} L${.5+e},1 M0,${.5-t} L${.5-e},${.5-t} L${.5-e},0 M1,${.5-t} L${.5+e},${.5-t} L${.5+e},0`},"╱":{1:"M1,0 L0,1"},"╲":{1:"M0,0 L1,1"},"╳":{1:"M1,0 L0,1 M0,0 L1,1"},"╼":{1:"M.5,.5 L0,.5",3:"M.5,.5 L1,.5"},"╽":{1:"M.5,.5 L.5,0",3:"M.5,.5 L.5,1"},"╾":{1:"M.5,.5 L1,.5",3:"M.5,.5 L0,.5"},"╿":{1:"M.5,.5 L.5,1",3:"M.5,.5 L.5,0"},"┍":{1:"M.5,.5 L.5,1",3:"M.5,.5 L1,.5"},"┎":{1:"M.5,.5 L1,.5",3:"M.5,.5 L.5,1"},"┑":{1:"M.5,.5 L.5,1",3:"M.5,.5 L0,.5"},"┒":{1:"M.5,.5 L0,.5",3:"M.5,.5 L.5,1"},"┕":{1:"M.5,.5 L.5,0",3:"M.5,.5 L1,.5"},"┖":{1:"M.5,.5 L1,.5",3:"M.5,.5 L.5,0"},"┙":{1:"M.5,.5 L.5,0",3:"M.5,.5 L0,.5"},"┚":{1:"M.5,.5 L0,.5",3:"M.5,.5 L.5,0"},"┝":{1:"M.5,0 L.5,1",3:"M.5,.5 L1,.5"},"┞":{1:"M0.5,1 L.5,.5 L1,.5",3:"M.5,.5 L.5,0"},"┟":{1:"M.5,0 L.5,.5 L1,.5",3:"M.5,.5 L.5,1"},"┠":{1:"M.5,.5 L1,.5",3:"M.5,0 L.5,1"},"┡":{1:"M.5,.5 L.5,1",3:"M.5,0 L.5,.5 L1,.5"},"┢":{1:"M.5,.5 L.5,0",3:"M0.5,1 L.5,.5 L1,.5"},"┥":{1:"M.5,0 L.5,1",3:"M.5,.5 L0,.5"},"┦":{1:"M0,.5 L.5,.5 L.5,1",3:"M.5,.5 L.5,0"},"┧":{1:"M.5,0 L.5,.5 L0,.5",3:"M.5,.5 L.5,1"},"┨":{1:"M.5,.5 L0,.5",3:"M.5,0 L.5,1"},"┩":{1:"M.5,.5 L.5,1",3:"M.5,0 L.5,.5 L0,.5"},"┪":{1:"M.5,.5 L.5,0",3:"M0,.5 L.5,.5 L.5,1"},"┭":{1:"M0.5,1 L.5,.5 L1,.5",3:"M.5,.5 L0,.5"},"┮":{1:"M0,.5 L.5,.5 L.5,1",3:"M.5,.5 L1,.5"},"┯":{1:"M.5,.5 L.5,1",3:"M0,.5 L1,.5"},"┰":{1:"M0,.5 L1,.5",3:"M.5,.5 L.5,1"},"┱":{1:"M.5,.5 L1,.5",3:"M0,.5 L.5,.5 L.5,1"},"┲":{1:"M.5,.5 L0,.5",3:"M0.5,1 L.5,.5 L1,.5"},"┵":{1:"M.5,0 L.5,.5 L1,.5",3:"M.5,.5 L0,.5"},"┶":{1:"M.5,0 L.5,.5 L0,.5",3:"M.5,.5 L1,.5"},"┷":{1:"M.5,.5 L.5,0",3:"M0,.5 L1,.5"},"┸":{1:"M0,.5 L1,.5",3:"M.5,.5 L.5,0"},"┹":{1:"M.5,.5 L1,.5",3:"M.5,0 L.5,.5 L0,.5"},"┺":{1:"M.5,.5 L0,.5",3:"M.5,0 L.5,.5 L1,.5"},"┽":{1:"M.5,0 L.5,1 M.5,.5 L1,.5",3:"M.5,.5 L0,.5"},"┾":{1:"M.5,0 L.5,1 M.5,.5 L0,.5",3:"M.5,.5 L1,.5"},"┿":{1:"M.5,0 L.5,1",3:"M0,.5 L1,.5"},"╀":{1:"M0,.5 L1,.5 M.5,.5 L.5,1",3:"M.5,.5 L.5,0"},"╁":{1:"M.5,.5 L.5,0 M0,.5 L1,.5",3:"M.5,.5 L.5,1"},"╂":{1:"M0,.5 L1,.5",3:"M.5,0 L.5,1"},"╃":{1:"M0.5,1 L.5,.5 L1,.5",3:"M.5,0 L.5,.5 L0,.5"},"╄":{1:"M0,.5 L.5,.5 L.5,1",3:"M.5,0 L.5,.5 L1,.5"},"╅":{1:"M.5,0 L.5,.5 L1,.5",3:"M0,.5 L.5,.5 L.5,1"},"╆":{1:"M.5,0 L.5,.5 L0,.5",3:"M0.5,1 L.5,.5 L1,.5"},"╇":{1:"M.5,.5 L.5,1",3:"M.5,.5 L.5,0 M0,.5 L1,.5"},"╈":{1:"M.5,.5 L.5,0",3:"M0,.5 L1,.5 M.5,.5 L.5,1"},"╉":{1:"M.5,.5 L1,.5",3:"M.5,0 L.5,1 M.5,.5 L0,.5"},"╊":{1:"M.5,.5 L0,.5",3:"M.5,0 L.5,1 M.5,.5 L1,.5"},"╌":{1:"M.1,.5 L.4,.5 M.6,.5 L.9,.5"},"╍":{3:"M.1,.5 L.4,.5 M.6,.5 L.9,.5"},"┄":{1:"M.0667,.5 L.2667,.5 M.4,.5 L.6,.5 M.7333,.5 L.9333,.5"},"┅":{3:"M.0667,.5 L.2667,.5 M.4,.5 L.6,.5 M.7333,.5 L.9333,.5"},"┈":{1:"M.05,.5 L.2,.5 M.3,.5 L.45,.5 M.55,.5 L.7,.5 M.8,.5 L.95,.5"},"┉":{3:"M.05,.5 L.2,.5 M.3,.5 L.45,.5 M.55,.5 L.7,.5 M.8,.5 L.95,.5"},"╎":{1:"M.5,.1 L.5,.4 M.5,.6 L.5,.9"},"╏":{3:"M.5,.1 L.5,.4 M.5,.6 L.5,.9"},"┆":{1:"M.5,.0667 L.5,.2667 M.5,.4 L.5,.6 M.5,.7333 L.5,.9333"},"┇":{3:"M.5,.0667 L.5,.2667 M.5,.4 L.5,.6 M.5,.7333 L.5,.9333"},"┊":{1:"M.5,.05 L.5,.2 M.5,.3 L.5,.45 L.5,.55 M.5,.7 L.5,.95"},"┋":{3:"M.5,.05 L.5,.2 M.5,.3 L.5,.45 L.5,.55 M.5,.7 L.5,.95"},"╭":{1:(e,t)=>`M.5,1 L.5,${.5+t/.15*.5} C.5,${.5+t/.15*.5},.5,.5,1,.5`},"╮":{1:(e,t)=>`M.5,1 L.5,${.5+t/.15*.5} C.5,${.5+t/.15*.5},.5,.5,0,.5`},"╯":{1:(e,t)=>`M.5,0 L.5,${.5-t/.15*.5} C.5,${.5-t/.15*.5},.5,.5,0,.5`},"╰":{1:(e,t)=>`M.5,0 L.5,${.5-t/.15*.5} C.5,${.5-t/.15*.5},.5,.5,1,.5`}},t.powerlineDefinitions={"":{d:"M0,0 L1,.5 L0,1",type:0,rightPadding:2},"":{d:"M-1,-.5 L1,.5 L-1,1.5",type:1,leftPadding:1,rightPadding:1},"":{d:"M1,0 L0,.5 L1,1",type:0,leftPadding:2},"":{d:"M2,-.5 L0,.5 L2,1.5",type:1,leftPadding:1,rightPadding:1},"":{d:"M0,0 L0,1 C0.552,1,1,0.776,1,.5 C1,0.224,0.552,0,0,0",type:0,rightPadding:1},"":{d:"M.2,1 C.422,1,.8,.826,.78,.5 C.8,.174,0.422,0,.2,0",type:1,rightPadding:1},"":{d:"M1,0 L1,1 C0.448,1,0,0.776,0,.5 C0,0.224,0.448,0,1,0",type:0,leftPadding:1},"":{d:"M.8,1 C0.578,1,0.2,.826,.22,.5 C0.2,0.174,0.578,0,0.8,0",type:1,leftPadding:1},"":{d:"M-.5,-.5 L1.5,1.5 L-.5,1.5",type:0},"":{d:"M-.5,-.5 L1.5,1.5",type:1,leftPadding:1,rightPadding:1},"":{d:"M1.5,-.5 L-.5,1.5 L1.5,1.5",type:0},"":{d:"M1.5,-.5 L-.5,1.5 L-.5,-.5",type:0},"":{d:"M1.5,-.5 L-.5,1.5",type:1,leftPadding:1,rightPadding:1},"":{d:"M-.5,-.5 L1.5,1.5 L1.5,-.5",type:0}},t.powerlineDefinitions[""]=t.powerlineDefinitions[""],t.powerlineDefinitions[""]=t.powerlineDefinitions[""],t.tryDrawCustomChar=function(e,i,n,l,c,d,_,u){const g=t.blockElementDefinitions[i];if(g)return function(e,t,i,s,r,o){for(let n=0;n<t.length;n++){const a=t[n],h=r/8,l=o/8;e.fillRect(i+a.x*h,s+a.y*l,a.w*h,a.h*l)}}(e,g,n,l,c,d),!0;const f=r[i];if(f)return function(e,t,i,r,n,a){let h=o.get(t);h||(h=new Map,o.set(t,h));const l=e.fillStyle;if("string"!=typeof l)throw new Error(`Unexpected fillStyle type "${l}"`);let c=h.get(l);if(!c){const i=t[0].length,r=t.length,o=e.canvas.ownerDocument.createElement("canvas");o.width=i,o.height=r;const n=(0,s.throwIfFalsy)(o.getContext("2d")),a=new ImageData(i,r);let d,_,u,g;if(l.startsWith("#"))d=parseInt(l.slice(1,3),16),_=parseInt(l.slice(3,5),16),u=parseInt(l.slice(5,7),16),g=l.length>7&&parseInt(l.slice(7,9),16)||1;else{if(!l.startsWith("rgba"))throw new Error(`Unexpected fillStyle color format "${l}" when drawing pattern glyph`);[d,_,u,g]=l.substring(5,l.length-1).split(",").map((e=>parseFloat(e)))}for(let e=0;e<r;e++)for(let s=0;s<i;s++)a.data[4*(e*i+s)]=d,a.data[4*(e*i+s)+1]=_,a.data[4*(e*i+s)+2]=u,a.data[4*(e*i+s)+3]=t[e][s]*(255*g);n.putImageData(a,0,0),c=(0,s.throwIfFalsy)(e.createPattern(o,null)),h.set(l,c)}e.fillStyle=c,e.fillRect(i,r,n,a)}(e,f,n,l,c,d),!0;const v=t.boxDrawingDefinitions[i];if(v)return function(e,t,i,s,r,o,n){e.strokeStyle=e.fillStyle;for(const[l,c]of Object.entries(t)){let t;e.beginPath(),e.lineWidth=n*Number.parseInt(l),t="function"==typeof c?c(.15,.15/o*r):c;for(const l of t.split(" ")){const t=l[0],c=a[t];if(!c){console.error(`Could not find drawing instructions for "${t}"`);continue}const d=l.substring(1).split(",");d[0]&&d[1]&&c(e,h(d,r,o,i,s,!0,n))}e.stroke(),e.closePath()}}(e,v,n,l,c,d,u),!0;const C=t.powerlineDefinitions[i];return!!C&&(function(e,t,i,s,r,o,n,l){const c=new Path2D;c.rect(i,s,r,o),e.clip(c),e.beginPath();const d=n/12;e.lineWidth=l*d;for(const n of t.d.split(" ")){const c=n[0],_=a[c];if(!_){console.error(`Could not find drawing instructions for "${c}"`);continue}const u=n.substring(1).split(",");u[0]&&u[1]&&_(e,h(u,r,o,i,s,!1,l,(t.leftPadding??0)*(d/2),(t.rightPadding??0)*(d/2)))}1===t.type?(e.strokeStyle=e.fillStyle,e.stroke()):e.fill(),e.closePath()}(e,C,n,l,c,d,_,u),!0)};const o=new Map;function n(e,t,i=0){return Math.max(Math.min(e,t),i)}const a={C:(e,t)=>e.bezierCurveTo(t[0],t[1],t[2],t[3],t[4],t[5]),L:(e,t)=>e.lineTo(t[0],t[1]),M:(e,t)=>e.moveTo(t[0],t[1])};function h(e,t,i,s,r,o,a,h=0,l=0){const c=e.map((e=>parseFloat(e)||parseInt(e)));if(c.length<2)throw new Error("Too few arguments for instruction");for(let e=0;e<c.length;e+=2)c[e]*=t-h*a-l*a,o&&0!==c[e]&&(c[e]=n(Math.round(c[e]+.5)-.5,t,0)),c[e]+=s+h*a;for(let e=1;e<c.length;e+=2)c[e]*=i,o&&0!==c[e]&&(c[e]=n(Math.round(c[e]+.5)-.5,i,0)),c[e]+=r;return c}},56:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.observeDevicePixelDimensions=void 0;const s=i(859);t.observeDevicePixelDimensions=function(e,t,i){let r=new t.ResizeObserver((t=>{const s=t.find((t=>t.target===e));if(!s)return;if(!("devicePixelContentBoxSize"in s))return r?.disconnect(),void(r=void 0);const o=s.devicePixelContentBoxSize[0].inlineSize,n=s.devicePixelContentBoxSize[0].blockSize;o>0&&n>0&&i(o,n)}));try{r.observe(e,{box:["device-pixel-content-box"]})}catch{r.disconnect(),r=void 0}return(0,s.toDisposable)((()=>r?.disconnect()))}},374:(e,t)=>{function i(e){return 57508<=e&&e<=57558}function s(e){return e>=128512&&e<=128591||e>=127744&&e<=128511||e>=128640&&e<=128767||e>=9728&&e<=9983||e>=9984&&e<=10175||e>=65024&&e<=65039||e>=129280&&e<=129535||e>=127462&&e<=127487}Object.defineProperty(t,"__esModule",{value:!0}),t.computeNextVariantOffset=t.createRenderDimensions=t.treatGlyphAsBackgroundColor=t.allowRescaling=t.isEmoji=t.isRestrictedPowerlineGlyph=t.isPowerlineGlyph=t.throwIfFalsy=void 0,t.throwIfFalsy=function(e){if(!e)throw new Error("value must not be falsy");return e},t.isPowerlineGlyph=i,t.isRestrictedPowerlineGlyph=function(e){return 57520<=e&&e<=57527},t.isEmoji=s,t.allowRescaling=function(e,t,r,o){return 1===t&&r>Math.ceil(1.5*o)&&void 0!==e&&e>255&&!s(e)&&!i(e)&&!function(e){return 57344<=e&&e<=63743}(e)},t.treatGlyphAsBackgroundColor=function(e){return i(e)||function(e){return 9472<=e&&e<=9631}(e)},t.createRenderDimensions=function(){return{css:{canvas:{width:0,height:0},cell:{width:0,height:0}},device:{canvas:{width:0,height:0},cell:{width:0,height:0},char:{width:0,height:0,left:0,top:0}}}},t.computeNextVariantOffset=function(e,t,i=0){return(e-(2*Math.round(t)-i))%(2*Math.round(t))}},296:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.createSelectionRenderModel=void 0;class i{constructor(){this.clear()}clear(){this.hasSelection=!1,this.columnSelectMode=!1,this.viewportStartRow=0,this.viewportEndRow=0,this.viewportCappedStartRow=0,this.viewportCappedEndRow=0,this.startCol=0,this.endCol=0,this.selectionStart=void 0,this.selectionEnd=void 0}update(e,t,i,s=!1){if(this.selectionStart=t,this.selectionEnd=i,!t||!i||t[0]===i[0]&&t[1]===i[1])return void this.clear();const r=e.buffers.active.ydisp,o=t[1]-r,n=i[1]-r,a=Math.max(o,0),h=Math.min(n,e.rows-1);a>=e.rows||h<0?this.clear():(this.hasSelection=!0,this.columnSelectMode=s,this.viewportStartRow=o,this.viewportEndRow=n,this.viewportCappedStartRow=a,this.viewportCappedEndRow=h,this.startCol=t[0],this.endCol=i[0])}isCellSelected(e,t,i){return!!this.hasSelection&&(i-=e.buffer.active.viewportY,this.columnSelectMode?this.startCol<=this.endCol?t>=this.startCol&&i>=this.viewportCappedStartRow&&t<this.endCol&&i<=this.viewportCappedEndRow:t<this.startCol&&i>=this.viewportCappedStartRow&&t>=this.endCol&&i<=this.viewportCappedEndRow:i>this.viewportStartRow&&i<this.viewportEndRow||this.viewportStartRow===this.viewportEndRow&&i===this.viewportStartRow&&t>=this.startCol&&t<this.endCol||this.viewportStartRow<this.viewportEndRow&&i===this.viewportEndRow&&t<this.endCol||this.viewportStartRow<this.viewportEndRow&&i===this.viewportStartRow&&t>=this.startCol)}}t.createSelectionRenderModel=function(){return new i}},509:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.TextureAtlas=void 0;const s=i(237),r=i(860),o=i(374),n=i(160),a=i(345),h=i(485),l=i(385),c=i(147),d=i(855),_={texturePage:0,texturePosition:{x:0,y:0},texturePositionClipSpace:{x:0,y:0},offset:{x:0,y:0},size:{x:0,y:0},sizeClipSpace:{x:0,y:0}};let u;class g{get pages(){return this._pages}constructor(e,t,i){this._document=e,this._config=t,this._unicodeService=i,this._didWarmUp=!1,this._cacheMap=new h.FourKeyMap,this._cacheMapCombined=new h.FourKeyMap,this._pages=[],this._activePages=[],this._workBoundingBox={top:0,left:0,bottom:0,right:0},this._workAttributeData=new c.AttributeData,this._textureSize=512,this._onAddTextureAtlasCanvas=new a.EventEmitter,this.onAddTextureAtlasCanvas=this._onAddTextureAtlasCanvas.event,this._onRemoveTextureAtlasCanvas=new a.EventEmitter,this.onRemoveTextureAtlasCanvas=this._onRemoveTextureAtlasCanvas.event,this._requestClearModel=!1,this._createNewPage(),this._tmpCanvas=C(e,4*this._config.deviceCellWidth+4,this._config.deviceCellHeight+4),this._tmpCtx=(0,o.throwIfFalsy)(this._tmpCanvas.getContext("2d",{alpha:this._config.allowTransparency,willReadFrequently:!0}))}dispose(){for(const e of this.pages)e.canvas.remove();this._onAddTextureAtlasCanvas.dispose()}warmUp(){this._didWarmUp||(this._doWarmUp(),this._didWarmUp=!0)}_doWarmUp(){const e=new l.IdleTaskQueue;for(let t=33;t<126;t++)e.enqueue((()=>{if(!this._cacheMap.get(t,d.DEFAULT_COLOR,d.DEFAULT_COLOR,d.DEFAULT_EXT)){const e=this._drawToCache(t,d.DEFAULT_COLOR,d.DEFAULT_COLOR,d.DEFAULT_EXT);this._cacheMap.set(t,d.DEFAULT_COLOR,d.DEFAULT_COLOR,d.DEFAULT_EXT,e)}}))}beginFrame(){return this._requestClearModel}clearTexture(){if(0!==this._pages[0].currentRow.x||0!==this._pages[0].currentRow.y){for(const e of this._pages)e.clear();this._cacheMap.clear(),this._cacheMapCombined.clear(),this._didWarmUp=!1}}_createNewPage(){if(g.maxAtlasPages&&this._pages.length>=Math.max(4,g.maxAtlasPages)){const e=this._pages.filter((e=>2*e.canvas.width<=(g.maxTextureSize||4096))).sort(((e,t)=>t.canvas.width!==e.canvas.width?t.canvas.width-e.canvas.width:t.percentageUsed-e.percentageUsed));let t=-1,i=0;for(let s=0;s<e.length;s++)if(e[s].canvas.width!==i)t=s,i=e[s].canvas.width;else if(s-t==3)break;const s=e.slice(t,t+4),r=s.map((e=>e.glyphs[0].texturePage)).sort(((e,t)=>e>t?1:-1)),o=this.pages.length-s.length,n=this._mergePages(s,o);n.version++;for(let e=r.length-1;e>=0;e--)this._deletePage(r[e]);this.pages.push(n),this._requestClearModel=!0,this._onAddTextureAtlasCanvas.fire(n.canvas)}const e=new f(this._document,this._textureSize);return this._pages.push(e),this._activePages.push(e),this._onAddTextureAtlasCanvas.fire(e.canvas),e}_mergePages(e,t){const i=2*e[0].canvas.width,s=new f(this._document,i,e);for(const[r,o]of e.entries()){const e=r*o.canvas.width%i,n=Math.floor(r/2)*o.canvas.height;s.ctx.drawImage(o.canvas,e,n);for(const s of o.glyphs)s.texturePage=t,s.sizeClipSpace.x=s.size.x/i,s.sizeClipSpace.y=s.size.y/i,s.texturePosition.x+=e,s.texturePosition.y+=n,s.texturePositionClipSpace.x=s.texturePosition.x/i,s.texturePositionClipSpace.y=s.texturePosition.y/i;this._onRemoveTextureAtlasCanvas.fire(o.canvas);const a=this._activePages.indexOf(o);-1!==a&&this._activePages.splice(a,1)}return s}_deletePage(e){this._pages.splice(e,1);for(let t=e;t<this._pages.length;t++){const e=this._pages[t];for(const t of e.glyphs)t.texturePage--;e.version++}}getRasterizedGlyphCombinedChar(e,t,i,s,r){return this._getFromCacheMap(this._cacheMapCombined,e,t,i,s,r)}getRasterizedGlyph(e,t,i,s,r){return this._getFromCacheMap(this._cacheMap,e,t,i,s,r)}_getFromCacheMap(e,t,i,s,r,o=!1){return u=e.get(t,i,s,r),u||(u=this._drawToCache(t,i,s,r,o),e.set(t,i,s,r,u)),u}_getColorFromAnsiIndex(e){if(e>=this._config.colors.ansi.length)throw new Error("No color found for idx "+e);return this._config.colors.ansi[e]}_getBackgroundColor(e,t,i,s){if(this._config.allowTransparency)return n.NULL_COLOR;let r;switch(e){case 16777216:case 33554432:r=this._getColorFromAnsiIndex(t);break;case 50331648:const e=c.AttributeData.toColorRGB(t);r=n.channels.toColor(e[0],e[1],e[2]);break;default:r=i?n.color.opaque(this._config.colors.foreground):this._config.colors.background}return r}_getForegroundColor(e,t,i,r,o,a,h,l,d,_){const u=this._getMinimumContrastColor(e,t,i,r,o,a,h,d,l,_);if(u)return u;let g;switch(o){case 16777216:case 33554432:this._config.drawBoldTextInBrightColors&&d&&a<8&&(a+=8),g=this._getColorFromAnsiIndex(a);break;case 50331648:const e=c.AttributeData.toColorRGB(a);g=n.channels.toColor(e[0],e[1],e[2]);break;default:g=h?this._config.colors.background:this._config.colors.foreground}return this._config.allowTransparency&&(g=n.color.opaque(g)),l&&(g=n.color.multiplyOpacity(g,s.DIM_OPACITY)),g}_resolveBackgroundRgba(e,t,i){switch(e){case 16777216:case 33554432:return this._getColorFromAnsiIndex(t).rgba;case 50331648:return t<<8;default:return i?this._config.colors.foreground.rgba:this._config.colors.background.rgba}}_resolveForegroundRgba(e,t,i,s){switch(e){case 16777216:case 33554432:return this._config.drawBoldTextInBrightColors&&s&&t<8&&(t+=8),this._getColorFromAnsiIndex(t).rgba;case 50331648:return t<<8;default:return i?this._config.colors.background.rgba:this._config.colors.foreground.rgba}}_getMinimumContrastColor(e,t,i,s,r,o,a,h,l,c){if(1===this._config.minimumContrastRatio||c)return;const d=this._getContrastCache(l),_=d.getColor(e,s);if(void 0!==_)return _||void 0;const u=this._resolveBackgroundRgba(t,i,a),g=this._resolveForegroundRgba(r,o,a,h),f=n.rgba.ensureContrastRatio(u,g,this._config.minimumContrastRatio/(l?2:1));if(!f)return void d.setColor(e,s,null);const v=n.channels.toColor(f>>24&255,f>>16&255,f>>8&255);return d.setColor(e,s,v),v}_getContrastCache(e){return e?this._config.colors.halfContrastCache:this._config.colors.contrastCache}_drawToCache(e,t,i,n,a=!1){const h="number"==typeof e?String.fromCharCode(e):e,l=Math.min(this._config.deviceCellWidth*Math.max(h.length,2)+4,this._textureSize);this._tmpCanvas.width<l&&(this._tmpCanvas.width=l);const d=Math.min(this._config.deviceCellHeight+8,this._textureSize);if(this._tmpCanvas.height<d&&(this._tmpCanvas.height=d),this._tmpCtx.save(),this._workAttributeData.fg=i,this._workAttributeData.bg=t,this._workAttributeData.extended.ext=n,this._workAttributeData.isInvisible())return _;const u=!!this._workAttributeData.isBold(),f=!!this._workAttributeData.isInverse(),C=!!this._workAttributeData.isDim(),p=!!this._workAttributeData.isItalic(),m=!!this._workAttributeData.isUnderline(),x=!!this._workAttributeData.isStrikethrough(),w=!!this._workAttributeData.isOverline();let L=this._workAttributeData.getFgColor(),b=this._workAttributeData.getFgColorMode(),M=this._workAttributeData.getBgColor(),S=this._workAttributeData.getBgColorMode();if(f){const e=L;L=M,M=e;const t=b;b=S,S=t}const y=this._getBackgroundColor(S,M,f,C);this._tmpCtx.globalCompositeOperation="copy",this._tmpCtx.fillStyle=y.css,this._tmpCtx.fillRect(0,0,this._tmpCanvas.width,this._tmpCanvas.height),this._tmpCtx.globalCompositeOperation="source-over";const R=u?this._config.fontWeightBold:this._config.fontWeight,A=p?"italic":"";this._tmpCtx.font=`${A} ${R} ${this._config.fontSize*this._config.devicePixelRatio}px ${this._config.fontFamily}`,this._tmpCtx.textBaseline=s.TEXT_BASELINE;const D=1===h.length&&(0,o.isPowerlineGlyph)(h.charCodeAt(0)),T=1===h.length&&(0,o.isRestrictedPowerlineGlyph)(h.charCodeAt(0)),k=this._getForegroundColor(t,S,M,i,b,L,f,C,u,(0,o.treatGlyphAsBackgroundColor)(h.charCodeAt(0)));this._tmpCtx.fillStyle=k.css;const E=T?0:4;let B=!1;!1!==this._config.customGlyphs&&(B=(0,r.tryDrawCustomChar)(this._tmpCtx,h,E,E,this._config.deviceCellWidth,this._config.deviceCellHeight,this._config.fontSize,this._config.devicePixelRatio));let $,P=!D;if($="number"==typeof e?this._unicodeService.wcwidth(e):this._unicodeService.getStringCellWidth(e),m){this._tmpCtx.save();const e=Math.max(1,Math.floor(this._config.fontSize*this._config.devicePixelRatio/15)),t=e%2==1?.5:0;if(this._tmpCtx.lineWidth=e,this._workAttributeData.isUnderlineColorDefault())this._tmpCtx.strokeStyle=this._tmpCtx.fillStyle;else if(this._workAttributeData.isUnderlineColorRGB())P=!1,this._tmpCtx.strokeStyle=`rgb(${c.AttributeData.toColorRGB(this._workAttributeData.getUnderlineColor()).join(",")})`;else{P=!1;let e=this._workAttributeData.getUnderlineColor();this._config.drawBoldTextInBrightColors&&this._workAttributeData.isBold()&&e<8&&(e+=8),this._tmpCtx.strokeStyle=this._getColorFromAnsiIndex(e).css}this._tmpCtx.beginPath();const i=E,s=Math.ceil(E+this._config.deviceCharHeight)-t-(a?2*e:0),r=s+e,n=s+2*e;let l=this._workAttributeData.getUnderlineVariantOffset();for(let a=0;a<$;a++){this._tmpCtx.save();const h=i+a*this._config.deviceCellWidth,c=i+(a+1)*this._config.deviceCellWidth,d=h+this._config.deviceCellWidth/2;switch(this._workAttributeData.extended.underlineStyle){case 2:this._tmpCtx.moveTo(h,s),this._tmpCtx.lineTo(c,s),this._tmpCtx.moveTo(h,n),this._tmpCtx.lineTo(c,n);break;case 3:const i=e<=1?n:Math.ceil(E+this._config.deviceCharHeight-e/2)-t,a=e<=1?s:Math.ceil(E+this._config.deviceCharHeight+e/2)-t,_=new Path2D;_.rect(h,s,this._config.deviceCellWidth,n-s),this._tmpCtx.clip(_),this._tmpCtx.moveTo(h-this._config.deviceCellWidth/2,r),this._tmpCtx.bezierCurveTo(h-this._config.deviceCellWidth/2,a,h,a,h,r),this._tmpCtx.bezierCurveTo(h,i,d,i,d,r),this._tmpCtx.bezierCurveTo(d,a,c,a,c,r),this._tmpCtx.bezierCurveTo(c,i,c+this._config.deviceCellWidth/2,i,c+this._config.deviceCellWidth/2,r);break;case 4:const u=0===l?0:l>=e?2*e-l:e-l;!1==!(l>=e)||0===u?(this._tmpCtx.setLineDash([Math.round(e),Math.round(e)]),this._tmpCtx.moveTo(h+u,s),this._tmpCtx.lineTo(c,s)):(this._tmpCtx.setLineDash([Math.round(e),Math.round(e)]),this._tmpCtx.moveTo(h,s),this._tmpCtx.lineTo(h+u,s),this._tmpCtx.moveTo(h+u+e,s),this._tmpCtx.lineTo(c,s)),l=(0,o.computeNextVariantOffset)(c-h,e,l);break;case 5:const g=.6,f=.3,v=c-h,C=Math.floor(g*v),p=Math.floor(f*v),m=v-C-p;this._tmpCtx.setLineDash([C,p,m]),this._tmpCtx.moveTo(h,s),this._tmpCtx.lineTo(c,s);break;default:this._tmpCtx.moveTo(h,s),this._tmpCtx.lineTo(c,s)}this._tmpCtx.stroke(),this._tmpCtx.restore()}if(this._tmpCtx.restore(),!B&&this._config.fontSize>=12&&!this._config.allowTransparency&&" "!==h){this._tmpCtx.save(),this._tmpCtx.textBaseline="alphabetic";const t=this._tmpCtx.measureText(h);if(this._tmpCtx.restore(),"actualBoundingBoxDescent"in t&&t.actualBoundingBoxDescent>0){this._tmpCtx.save();const t=new Path2D;t.rect(i,s-Math.ceil(e/2),this._config.deviceCellWidth*$,n-s+Math.ceil(e/2)),this._tmpCtx.clip(t),this._tmpCtx.lineWidth=3*this._config.devicePixelRatio,this._tmpCtx.strokeStyle=y.css,this._tmpCtx.strokeText(h,E,E+this._config.deviceCharHeight),this._tmpCtx.restore()}}}if(w){const e=Math.max(1,Math.floor(this._config.fontSize*this._config.devicePixelRatio/15)),t=e%2==1?.5:0;this._tmpCtx.lineWidth=e,this._tmpCtx.strokeStyle=this._tmpCtx.fillStyle,this._tmpCtx.beginPath(),this._tmpCtx.moveTo(E,E+t),this._tmpCtx.lineTo(E+this._config.deviceCharWidth*$,E+t),this._tmpCtx.stroke()}if(B||this._tmpCtx.fillText(h,E,E+this._config.deviceCharHeight),"_"===h&&!this._config.allowTransparency){let e=v(this._tmpCtx.getImageData(E,E,this._config.deviceCellWidth,this._config.deviceCellHeight),y,k,P);if(e)for(let t=1;t<=5&&(this._tmpCtx.save(),this._tmpCtx.fillStyle=y.css,this._tmpCtx.fillRect(0,0,this._tmpCanvas.width,this._tmpCanvas.height),this._tmpCtx.restore(),this._tmpCtx.fillText(h,E,E+this._config.deviceCharHeight-t),e=v(this._tmpCtx.getImageData(E,E,this._config.deviceCellWidth,this._config.deviceCellHeight),y,k,P),e);t++);}if(x){const e=Math.max(1,Math.floor(this._config.fontSize*this._config.devicePixelRatio/10)),t=this._tmpCtx.lineWidth%2==1?.5:0;this._tmpCtx.lineWidth=e,this._tmpCtx.strokeStyle=this._tmpCtx.fillStyle,this._tmpCtx.beginPath(),this._tmpCtx.moveTo(E,E+Math.floor(this._config.deviceCharHeight/2)-t),this._tmpCtx.lineTo(E+this._config.deviceCharWidth*$,E+Math.floor(this._config.deviceCharHeight/2)-t),this._tmpCtx.stroke()}this._tmpCtx.restore();const O=this._tmpCtx.getImageData(0,0,this._tmpCanvas.width,this._tmpCanvas.height);let I;if(I=this._config.allowTransparency?function(e){for(let t=0;t<e.data.length;t+=4)if(e.data[t+3]>0)return!1;return!0}(O):v(O,y,k,P),I)return _;const F=this._findGlyphBoundingBox(O,this._workBoundingBox,l,T,B,E);let W,H;for(;;){if(0===this._activePages.length){const e=this._createNewPage();W=e,H=e.currentRow,H.height=F.size.y;break}W=this._activePages[this._activePages.length-1],H=W.currentRow;for(const e of this._activePages)F.size.y<=e.currentRow.height&&(W=e,H=e.currentRow);for(let e=this._activePages.length-1;e>=0;e--)for(const t of this._activePages[e].fixedRows)t.height<=H.height&&F.size.y<=t.height&&(W=this._activePages[e],H=t);if(H.y+F.size.y>=W.canvas.height||H.height>F.size.y+2){let e=!1;if(W.currentRow.y+W.currentRow.height+F.size.y>=W.canvas.height){let t;for(const e of this._activePages)if(e.currentRow.y+e.currentRow.height+F.size.y<e.canvas.height){t=e;break}if(t)W=t;else if(g.maxAtlasPages&&this._pages.length>=g.maxAtlasPages&&H.y+F.size.y<=W.canvas.height&&H.height>=F.size.y&&H.x+F.size.x<=W.canvas.width)e=!0;else{const t=this._createNewPage();W=t,H=t.currentRow,H.height=F.size.y,e=!0}}e||(W.currentRow.height>0&&W.fixedRows.push(W.currentRow),H={x:0,y:W.currentRow.y+W.currentRow.height,height:F.size.y},W.fixedRows.push(H),W.currentRow={x:0,y:H.y+H.height,height:0})}if(H.x+F.size.x<=W.canvas.width)break;H===W.currentRow?(H.x=0,H.y+=H.height,H.height=0):W.fixedRows.splice(W.fixedRows.indexOf(H),1)}return F.texturePage=this._pages.indexOf(W),F.texturePosition.x=H.x,F.texturePosition.y=H.y,F.texturePositionClipSpace.x=H.x/W.canvas.width,F.texturePositionClipSpace.y=H.y/W.canvas.height,F.sizeClipSpace.x/=W.canvas.width,F.sizeClipSpace.y/=W.canvas.height,H.height=Math.max(H.height,F.size.y),H.x+=F.size.x,W.ctx.putImageData(O,F.texturePosition.x-this._workBoundingBox.left,F.texturePosition.y-this._workBoundingBox.top,this._workBoundingBox.left,this._workBoundingBox.top,F.size.x,F.size.y),W.addGlyph(F),W.version++,F}_findGlyphBoundingBox(e,t,i,s,r,o){t.top=0;const n=s?this._config.deviceCellHeight:this._tmpCanvas.height,a=s?this._config.deviceCellWidth:i;let h=!1;for(let i=0;i<n;i++){for(let s=0;s<a;s++){const r=i*this._tmpCanvas.width*4+4*s+3;if(0!==e.data[r]){t.top=i,h=!0;break}}if(h)break}t.left=0,h=!1;for(let i=0;i<o+a;i++){for(let s=0;s<n;s++){const r=s*this._tmpCanvas.width*4+4*i+3;if(0!==e.data[r]){t.left=i,h=!0;break}}if(h)break}t.right=a,h=!1;for(let i=o+a-1;i>=o;i--){for(let s=0;s<n;s++){const r=s*this._tmpCanvas.width*4+4*i+3;if(0!==e.data[r]){t.right=i,h=!0;break}}if(h)break}t.bottom=n,h=!1;for(let i=n-1;i>=0;i--){for(let s=0;s<a;s++){const r=i*this._tmpCanvas.width*4+4*s+3;if(0!==e.data[r]){t.bottom=i,h=!0;break}}if(h)break}return{texturePage:0,texturePosition:{x:0,y:0},texturePositionClipSpace:{x:0,y:0},size:{x:t.right-t.left+1,y:t.bottom-t.top+1},sizeClipSpace:{x:t.right-t.left+1,y:t.bottom-t.top+1},offset:{x:-t.left+o+(s||r?Math.floor((this._config.deviceCellWidth-this._config.deviceCharWidth)/2):0),y:-t.top+o+(s||r?1===this._config.lineHeight?0:Math.round((this._config.deviceCellHeight-this._config.deviceCharHeight)/2):0)}}}}t.TextureAtlas=g;class f{get percentageUsed(){return this._usedPixels/(this.canvas.width*this.canvas.height)}get glyphs(){return this._glyphs}addGlyph(e){this._glyphs.push(e),this._usedPixels+=e.size.x*e.size.y}constructor(e,t,i){if(this._usedPixels=0,this._glyphs=[],this.version=0,this.currentRow={x:0,y:0,height:0},this.fixedRows=[],i)for(const e of i)this._glyphs.push(...e.glyphs),this._usedPixels+=e._usedPixels;this.canvas=C(e,t,t),this.ctx=(0,o.throwIfFalsy)(this.canvas.getContext("2d",{alpha:!0}))}clear(){this.ctx.clearRect(0,0,this.canvas.width,this.canvas.height),this.currentRow.x=0,this.currentRow.y=0,this.currentRow.height=0,this.fixedRows.length=0,this.version++}}function v(e,t,i,s){const r=t.rgba>>>24,o=t.rgba>>>16&255,n=t.rgba>>>8&255,a=i.rgba>>>24,h=i.rgba>>>16&255,l=i.rgba>>>8&255,c=Math.floor((Math.abs(r-a)+Math.abs(o-h)+Math.abs(n-l))/12);let d=!0;for(let t=0;t<e.data.length;t+=4)e.data[t]===r&&e.data[t+1]===o&&e.data[t+2]===n||s&&Math.abs(e.data[t]-r)+Math.abs(e.data[t+1]-o)+Math.abs(e.data[t+2]-n)<c?e.data[t+3]=0:d=!1;return d}function C(e,t,i){const s=e.createElement("canvas");return s.width=t,s.height=i,s}},577:function(e,t,i){var s=this&&this.__decorate||function(e,t,i,s){var r,o=arguments.length,n=o<3?t:null===s?s=Object.getOwnPropertyDescriptor(t,i):s;if("object"==typeof Reflect&&"function"==typeof Reflect.decorate)n=Reflect.decorate(e,t,i,s);else for(var a=e.length-1;a>=0;a--)(r=e[a])&&(n=(o<3?r(n):o>3?r(t,i,n):r(t,i))||n);return o>3&&n&&Object.defineProperty(t,i,n),n},r=this&&this.__param||function(e,t){return function(i,s){t(i,s,e)}};Object.defineProperty(t,"__esModule",{value:!0}),t.CharacterJoinerService=t.JoinedCellData=void 0;const o=i(147),n=i(855),a=i(782),h=i(97);class l extends o.AttributeData{constructor(e,t,i){super(),this.content=0,this.combinedData="",this.fg=e.fg,this.bg=e.bg,this.combinedData=t,this._width=i}isCombined(){return 2097152}getWidth(){return this._width}getChars(){return this.combinedData}getCode(){return 2097151}setFromCharData(e){throw new Error("not implemented")}getAsCharData(){return[this.fg,this.getChars(),this.getWidth(),this.getCode()]}}t.JoinedCellData=l;let c=t.CharacterJoinerService=class e{constructor(e){this._bufferService=e,this._characterJoiners=[],this._nextCharacterJoinerId=0,this._workCell=new a.CellData}register(e){const t={id:this._nextCharacterJoinerId++,handler:e};return this._characterJoiners.push(t),t.id}deregister(e){for(let t=0;t<this._characterJoiners.length;t++)if(this._characterJoiners[t].id===e)return this._characterJoiners.splice(t,1),!0;return!1}getJoinedCharacters(e){if(0===this._characterJoiners.length)return[];const t=this._bufferService.buffer.lines.get(e);if(!t||0===t.length)return[];const i=[],s=t.translateToString(!0);let r=0,o=0,a=0,h=t.getFg(0),l=t.getBg(0);for(let e=0;e<t.getTrimmedLength();e++)if(t.loadCell(e,this._workCell),0!==this._workCell.getWidth()){if(this._workCell.fg!==h||this._workCell.bg!==l){if(e-r>1){const e=this._getJoinedRanges(s,a,o,t,r);for(let t=0;t<e.length;t++)i.push(e[t])}r=e,a=o,h=this._workCell.fg,l=this._workCell.bg}o+=this._workCell.getChars().length||n.WHITESPACE_CELL_CHAR.length}if(this._bufferService.cols-r>1){const e=this._getJoinedRanges(s,a,o,t,r);for(let t=0;t<e.length;t++)i.push(e[t])}return i}_getJoinedRanges(t,i,s,r,o){const n=t.substring(i,s);let a=[];try{a=this._characterJoiners[0].handler(n)}catch(e){console.error(e)}for(let t=1;t<this._characterJoiners.length;t++)try{const i=this._characterJoiners[t].handler(n);for(let t=0;t<i.length;t++)e._mergeRanges(a,i[t])}catch(e){console.error(e)}return this._stringRangesToCellRanges(a,r,o),a}_stringRangesToCellRanges(e,t,i){let s=0,r=!1,o=0,a=e[s];if(a){for(let h=i;h<this._bufferService.cols;h++){const i=t.getWidth(h),l=t.getString(h).length||n.WHITESPACE_CELL_CHAR.length;if(0!==i){if(!r&&a[0]<=o&&(a[0]=h,r=!0),a[1]<=o){if(a[1]=h,a=e[++s],!a)break;a[0]<=o?(a[0]=h,r=!0):r=!1}o+=l}}a&&(a[1]=this._bufferService.cols)}}static _mergeRanges(e,t){let i=!1;for(let s=0;s<e.length;s++){const r=e[s];if(i){if(t[1]<=r[0])return e[s-1][1]=t[1],e;if(t[1]<=r[1])return e[s-1][1]=Math.max(t[1],r[1]),e.splice(s,1),e;e.splice(s,1),s--}else{if(t[1]<=r[0])return e.splice(s,0,t),e;if(t[1]<=r[1])return r[0]=Math.min(t[0],r[0]),e;t[0]<r[1]&&(r[0]=Math.min(t[0],r[0]),i=!0)}}return i?e[e.length-1][1]=t[1]:e.push(t),e}};t.CharacterJoinerService=c=s([r(0,h.IBufferService)],c)},160:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.contrastRatio=t.toPaddedHex=t.rgba=t.rgb=t.css=t.color=t.channels=t.NULL_COLOR=void 0;let i=0,s=0,r=0,o=0;var n,a,h,l,c;function d(e){const t=e.toString(16);return t.length<2?"0"+t:t}function _(e,t){return e<t?(t+.05)/(e+.05):(e+.05)/(t+.05)}t.NULL_COLOR={css:"#00000000",rgba:0},function(e){e.toCss=function(e,t,i,s){return void 0!==s?`#${d(e)}${d(t)}${d(i)}${d(s)}`:`#${d(e)}${d(t)}${d(i)}`},e.toRgba=function(e,t,i,s=255){return(e<<24|t<<16|i<<8|s)>>>0},e.toColor=function(t,i,s,r){return{css:e.toCss(t,i,s,r),rgba:e.toRgba(t,i,s,r)}}}(n||(t.channels=n={})),function(e){function t(e,t){return o=Math.round(255*t),[i,s,r]=c.toChannels(e.rgba),{css:n.toCss(i,s,r,o),rgba:n.toRgba(i,s,r,o)}}e.blend=function(e,t){if(o=(255&t.rgba)/255,1===o)return{css:t.css,rgba:t.rgba};const a=t.rgba>>24&255,h=t.rgba>>16&255,l=t.rgba>>8&255,c=e.rgba>>24&255,d=e.rgba>>16&255,_=e.rgba>>8&255;return i=c+Math.round((a-c)*o),s=d+Math.round((h-d)*o),r=_+Math.round((l-_)*o),{css:n.toCss(i,s,r),rgba:n.toRgba(i,s,r)}},e.isOpaque=function(e){return 255==(255&e.rgba)},e.ensureContrastRatio=function(e,t,i){const s=c.ensureContrastRatio(e.rgba,t.rgba,i);if(s)return n.toColor(s>>24&255,s>>16&255,s>>8&255)},e.opaque=function(e){const t=(255|e.rgba)>>>0;return[i,s,r]=c.toChannels(t),{css:n.toCss(i,s,r),rgba:t}},e.opacity=t,e.multiplyOpacity=function(e,i){return o=255&e.rgba,t(e,o*i/255)},e.toColorRGB=function(e){return[e.rgba>>24&255,e.rgba>>16&255,e.rgba>>8&255]}}(a||(t.color=a={})),function(e){let t,a;try{const e=document.createElement("canvas");e.width=1,e.height=1;const i=e.getContext("2d",{willReadFrequently:!0});i&&(t=i,t.globalCompositeOperation="copy",a=t.createLinearGradient(0,0,1,1))}catch{}e.toColor=function(e){if(e.match(/#[\da-f]{3,8}/i))switch(e.length){case 4:return i=parseInt(e.slice(1,2).repeat(2),16),s=parseInt(e.slice(2,3).repeat(2),16),r=parseInt(e.slice(3,4).repeat(2),16),n.toColor(i,s,r);case 5:return i=parseInt(e.slice(1,2).repeat(2),16),s=parseInt(e.slice(2,3).repeat(2),16),r=parseInt(e.slice(3,4).repeat(2),16),o=parseInt(e.slice(4,5).repeat(2),16),n.toColor(i,s,r,o);case 7:return{css:e,rgba:(parseInt(e.slice(1),16)<<8|255)>>>0};case 9:return{css:e,rgba:parseInt(e.slice(1),16)>>>0}}const h=e.match(/rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*(,\s*(0|1|\d?\.(\d+))\s*)?\)/);if(h)return i=parseInt(h[1]),s=parseInt(h[2]),r=parseInt(h[3]),o=Math.round(255*(void 0===h[5]?1:parseFloat(h[5]))),n.toColor(i,s,r,o);if(!t||!a)throw new Error("css.toColor: Unsupported css format");if(t.fillStyle=a,t.fillStyle=e,"string"!=typeof t.fillStyle)throw new Error("css.toColor: Unsupported css format");if(t.fillRect(0,0,1,1),[i,s,r,o]=t.getImageData(0,0,1,1).data,255!==o)throw new Error("css.toColor: Unsupported css format");return{rgba:n.toRgba(i,s,r,o),css:e}}}(h||(t.css=h={})),function(e){function t(e,t,i){const s=e/255,r=t/255,o=i/255;return.2126*(s<=.03928?s/12.92:Math.pow((s+.055)/1.055,2.4))+.7152*(r<=.03928?r/12.92:Math.pow((r+.055)/1.055,2.4))+.0722*(o<=.03928?o/12.92:Math.pow((o+.055)/1.055,2.4))}e.relativeLuminance=function(e){return t(e>>16&255,e>>8&255,255&e)},e.relativeLuminance2=t}(l||(t.rgb=l={})),function(e){function t(e,t,i){const s=e>>24&255,r=e>>16&255,o=e>>8&255;let n=t>>24&255,a=t>>16&255,h=t>>8&255,c=_(l.relativeLuminance2(n,a,h),l.relativeLuminance2(s,r,o));for(;c<i&&(n>0||a>0||h>0);)n-=Math.max(0,Math.ceil(.1*n)),a-=Math.max(0,Math.ceil(.1*a)),h-=Math.max(0,Math.ceil(.1*h)),c=_(l.relativeLuminance2(n,a,h),l.relativeLuminance2(s,r,o));return(n<<24|a<<16|h<<8|255)>>>0}function a(e,t,i){const s=e>>24&255,r=e>>16&255,o=e>>8&255;let n=t>>24&255,a=t>>16&255,h=t>>8&255,c=_(l.relativeLuminance2(n,a,h),l.relativeLuminance2(s,r,o));for(;c<i&&(n<255||a<255||h<255);)n=Math.min(255,n+Math.ceil(.1*(255-n))),a=Math.min(255,a+Math.ceil(.1*(255-a))),h=Math.min(255,h+Math.ceil(.1*(255-h))),c=_(l.relativeLuminance2(n,a,h),l.relativeLuminance2(s,r,o));return(n<<24|a<<16|h<<8|255)>>>0}e.blend=function(e,t){if(o=(255&t)/255,1===o)return t;const a=t>>24&255,h=t>>16&255,l=t>>8&255,c=e>>24&255,d=e>>16&255,_=e>>8&255;return i=c+Math.round((a-c)*o),s=d+Math.round((h-d)*o),r=_+Math.round((l-_)*o),n.toRgba(i,s,r)},e.ensureContrastRatio=function(e,i,s){const r=l.relativeLuminance(e>>8),o=l.relativeLuminance(i>>8);if(_(r,o)<s){if(o<r){const o=t(e,i,s),n=_(r,l.relativeLuminance(o>>8));if(n<s){const t=a(e,i,s);return n>_(r,l.relativeLuminance(t>>8))?o:t}return o}const n=a(e,i,s),h=_(r,l.relativeLuminance(n>>8));if(h<s){const o=t(e,i,s);return h>_(r,l.relativeLuminance(o>>8))?n:o}return n}},e.reduceLuminance=t,e.increaseLuminance=a,e.toChannels=function(e){return[e>>24&255,e>>16&255,e>>8&255,255&e]}}(c||(t.rgba=c={})),t.toPaddedHex=d,t.contrastRatio=_},345:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.runAndSubscribe=t.forwardEvent=t.EventEmitter=void 0,t.EventEmitter=class{constructor(){this._listeners=[],this._disposed=!1}get event(){return this._event||(this._event=e=>(this._listeners.push(e),{dispose:()=>{if(!this._disposed)for(let t=0;t<this._listeners.length;t++)if(this._listeners[t]===e)return void this._listeners.splice(t,1)}})),this._event}fire(e,t){const i=[];for(let e=0;e<this._listeners.length;e++)i.push(this._listeners[e]);for(let s=0;s<i.length;s++)i[s].call(void 0,e,t)}dispose(){this.clearListeners(),this._disposed=!0}clearListeners(){this._listeners&&(this._listeners.length=0)}},t.forwardEvent=function(e,t){return e((e=>t.fire(e)))},t.runAndSubscribe=function(e,t){return t(void 0),e((e=>t(e)))}},859:(e,t)=>{function i(e){for(const t of e)t.dispose();e.length=0}Object.defineProperty(t,"__esModule",{value:!0}),t.getDisposeArrayDisposable=t.disposeArray=t.toDisposable=t.MutableDisposable=t.Disposable=void 0,t.Disposable=class{constructor(){this._disposables=[],this._isDisposed=!1}dispose(){this._isDisposed=!0;for(const e of this._disposables)e.dispose();this._disposables.length=0}register(e){return this._disposables.push(e),e}unregister(e){const t=this._disposables.indexOf(e);-1!==t&&this._disposables.splice(t,1)}},t.MutableDisposable=class{constructor(){this._isDisposed=!1}get value(){return this._isDisposed?void 0:this._value}set value(e){this._isDisposed||e===this._value||(this._value?.dispose(),this._value=e)}clear(){this.value=void 0}dispose(){this._isDisposed=!0,this._value?.dispose(),this._value=void 0}},t.toDisposable=function(e){return{dispose:e}},t.disposeArray=i,t.getDisposeArrayDisposable=function(e){return{dispose:()=>i(e)}}},485:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.FourKeyMap=t.TwoKeyMap=void 0;class i{constructor(){this._data={}}set(e,t,i){this._data[e]||(this._data[e]={}),this._data[e][t]=i}get(e,t){return this._data[e]?this._data[e][t]:void 0}clear(){this._data={}}}t.TwoKeyMap=i,t.FourKeyMap=class{constructor(){this._data=new i}set(e,t,s,r,o){this._data.get(e,t)||this._data.set(e,t,new i),this._data.get(e,t).set(s,r,o)}get(e,t,i,s){return this._data.get(e,t)?.get(i,s)}clear(){this._data.clear()}}},399:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.isChromeOS=t.isLinux=t.isWindows=t.isIphone=t.isIpad=t.isMac=t.getSafariVersion=t.isSafari=t.isLegacyEdge=t.isFirefox=t.isNode=void 0,t.isNode="undefined"!=typeof process&&"title"in process;const i=t.isNode?"node":navigator.userAgent,s=t.isNode?"node":navigator.platform;t.isFirefox=i.includes("Firefox"),t.isLegacyEdge=i.includes("Edge"),t.isSafari=/^((?!chrome|android).)*safari/i.test(i),t.getSafariVersion=function(){if(!t.isSafari)return 0;const e=i.match(/Version\/(\d+)/);return null===e||e.length<2?0:parseInt(e[1])},t.isMac=["Macintosh","MacIntel","MacPPC","Mac68K"].includes(s),t.isIpad="iPad"===s,t.isIphone="iPhone"===s,t.isWindows=["Windows","Win16","Win32","WinCE"].includes(s),t.isLinux=s.indexOf("Linux")>=0,t.isChromeOS=/\bCrOS\b/.test(i)},385:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.DebouncedIdleTask=t.IdleTaskQueue=t.PriorityTaskQueue=void 0;const s=i(399);class r{constructor(){this._tasks=[],this._i=0}enqueue(e){this._tasks.push(e),this._start()}flush(){for(;this._i<this._tasks.length;)this._tasks[this._i]()||this._i++;this.clear()}clear(){this._idleCallback&&(this._cancelCallback(this._idleCallback),this._idleCallback=void 0),this._i=0,this._tasks.length=0}_start(){this._idleCallback||(this._idleCallback=this._requestCallback(this._process.bind(this)))}_process(e){this._idleCallback=void 0;let t=0,i=0,s=e.timeRemaining(),r=0;for(;this._i<this._tasks.length;){if(t=Date.now(),this._tasks[this._i]()||this._i++,t=Math.max(1,Date.now()-t),i=Math.max(t,i),r=e.timeRemaining(),1.5*i>r)return s-t<-20&&console.warn(`task queue exceeded allotted deadline by ${Math.abs(Math.round(s-t))}ms`),void this._start();s=r}this.clear()}}class o extends r{_requestCallback(e){return setTimeout((()=>e(this._createDeadline(16))))}_cancelCallback(e){clearTimeout(e)}_createDeadline(e){const t=Date.now()+e;return{timeRemaining:()=>Math.max(0,t-Date.now())}}}t.PriorityTaskQueue=o,t.IdleTaskQueue=!s.isNode&&"requestIdleCallback"in window?class extends r{_requestCallback(e){return requestIdleCallback(e)}_cancelCallback(e){cancelIdleCallback(e)}}:o,t.DebouncedIdleTask=class{constructor(){this._queue=new t.IdleTaskQueue}set(e){this._queue.clear(),this._queue.enqueue(e)}flush(){this._queue.flush()}}},147:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.ExtendedAttrs=t.AttributeData=void 0;class i{constructor(){this.fg=0,this.bg=0,this.extended=new s}static toColorRGB(e){return[e>>>16&255,e>>>8&255,255&e]}static fromColorRGB(e){return(255&e[0])<<16|(255&e[1])<<8|255&e[2]}clone(){const e=new i;return e.fg=this.fg,e.bg=this.bg,e.extended=this.extended.clone(),e}isInverse(){return 67108864&this.fg}isBold(){return 134217728&this.fg}isUnderline(){return this.hasExtendedAttrs()&&0!==this.extended.underlineStyle?1:268435456&this.fg}isBlink(){return 536870912&this.fg}isInvisible(){return 1073741824&this.fg}isItalic(){return 67108864&this.bg}isDim(){return 134217728&this.bg}isStrikethrough(){return 2147483648&this.fg}isProtected(){return 536870912&this.bg}isOverline(){return 1073741824&this.bg}getFgColorMode(){return 50331648&this.fg}getBgColorMode(){return 50331648&this.bg}isFgRGB(){return 50331648==(50331648&this.fg)}isBgRGB(){return 50331648==(50331648&this.bg)}isFgPalette(){return 16777216==(50331648&this.fg)||33554432==(50331648&this.fg)}isBgPalette(){return 16777216==(50331648&this.bg)||33554432==(50331648&this.bg)}isFgDefault(){return 0==(50331648&this.fg)}isBgDefault(){return 0==(50331648&this.bg)}isAttributeDefault(){return 0===this.fg&&0===this.bg}getFgColor(){switch(50331648&this.fg){case 16777216:case 33554432:return 255&this.fg;case 50331648:return 16777215&this.fg;default:return-1}}getBgColor(){switch(50331648&this.bg){case 16777216:case 33554432:return 255&this.bg;case 50331648:return 16777215&this.bg;default:return-1}}hasExtendedAttrs(){return 268435456&this.bg}updateExtended(){this.extended.isEmpty()?this.bg&=-268435457:this.bg|=268435456}getUnderlineColor(){if(268435456&this.bg&&~this.extended.underlineColor)switch(50331648&this.extended.underlineColor){case 16777216:case 33554432:return 255&this.extended.underlineColor;case 50331648:return 16777215&this.extended.underlineColor;default:return this.getFgColor()}return this.getFgColor()}getUnderlineColorMode(){return 268435456&this.bg&&~this.extended.underlineColor?50331648&this.extended.underlineColor:this.getFgColorMode()}isUnderlineColorRGB(){return 268435456&this.bg&&~this.extended.underlineColor?50331648==(50331648&this.extended.underlineColor):this.isFgRGB()}isUnderlineColorPalette(){return 268435456&this.bg&&~this.extended.underlineColor?16777216==(50331648&this.extended.underlineColor)||33554432==(50331648&this.extended.underlineColor):this.isFgPalette()}isUnderlineColorDefault(){return 268435456&this.bg&&~this.extended.underlineColor?0==(50331648&this.extended.underlineColor):this.isFgDefault()}getUnderlineStyle(){return 268435456&this.fg?268435456&this.bg?this.extended.underlineStyle:1:0}getUnderlineVariantOffset(){return this.extended.underlineVariantOffset}}t.AttributeData=i;class s{get ext(){return this._urlId?-469762049&this._ext|this.underlineStyle<<26:this._ext}set ext(e){this._ext=e}get underlineStyle(){return this._urlId?5:(469762048&this._ext)>>26}set underlineStyle(e){this._ext&=-469762049,this._ext|=e<<26&469762048}get underlineColor(){return 67108863&this._ext}set underlineColor(e){this._ext&=-67108864,this._ext|=67108863&e}get urlId(){return this._urlId}set urlId(e){this._urlId=e}get underlineVariantOffset(){const e=(3758096384&this._ext)>>29;return e<0?4294967288^e:e}set underlineVariantOffset(e){this._ext&=536870911,this._ext|=e<<29&3758096384}constructor(e=0,t=0){this._ext=0,this._urlId=0,this._ext=e,this._urlId=t}clone(){return new s(this._ext,this._urlId)}isEmpty(){return 0===this.underlineStyle&&0===this._urlId}}t.ExtendedAttrs=s},782:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.CellData=void 0;const s=i(133),r=i(855),o=i(147);class n extends o.AttributeData{constructor(){super(...arguments),this.content=0,this.fg=0,this.bg=0,this.extended=new o.ExtendedAttrs,this.combinedData=""}static fromCharData(e){const t=new n;return t.setFromCharData(e),t}isCombined(){return 2097152&this.content}getWidth(){return this.content>>22}getChars(){return 2097152&this.content?this.combinedData:2097151&this.content?(0,s.stringFromCodePoint)(2097151&this.content):""}getCode(){return this.isCombined()?this.combinedData.charCodeAt(this.combinedData.length-1):2097151&this.content}setFromCharData(e){this.fg=e[r.CHAR_DATA_ATTR_INDEX],this.bg=0;let t=!1;if(e[r.CHAR_DATA_CHAR_INDEX].length>2)t=!0;else if(2===e[r.CHAR_DATA_CHAR_INDEX].length){const i=e[r.CHAR_DATA_CHAR_INDEX].charCodeAt(0);if(55296<=i&&i<=56319){const s=e[r.CHAR_DATA_CHAR_INDEX].charCodeAt(1);56320<=s&&s<=57343?this.content=1024*(i-55296)+s-56320+65536|e[r.CHAR_DATA_WIDTH_INDEX]<<22:t=!0}else t=!0}else this.content=e[r.CHAR_DATA_CHAR_INDEX].charCodeAt(0)|e[r.CHAR_DATA_WIDTH_INDEX]<<22;t&&(this.combinedData=e[r.CHAR_DATA_CHAR_INDEX],this.content=2097152|e[r.CHAR_DATA_WIDTH_INDEX]<<22)}getAsCharData(){return[this.fg,this.getChars(),this.getWidth(),this.getCode()]}}t.CellData=n},855:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.WHITESPACE_CELL_CODE=t.WHITESPACE_CELL_WIDTH=t.WHITESPACE_CELL_CHAR=t.NULL_CELL_CODE=t.NULL_CELL_WIDTH=t.NULL_CELL_CHAR=t.CHAR_DATA_CODE_INDEX=t.CHAR_DATA_WIDTH_INDEX=t.CHAR_DATA_CHAR_INDEX=t.CHAR_DATA_ATTR_INDEX=t.DEFAULT_EXT=t.DEFAULT_ATTR=t.DEFAULT_COLOR=void 0,t.DEFAULT_COLOR=0,t.DEFAULT_ATTR=256|t.DEFAULT_COLOR<<9,t.DEFAULT_EXT=0,t.CHAR_DATA_ATTR_INDEX=0,t.CHAR_DATA_CHAR_INDEX=1,t.CHAR_DATA_WIDTH_INDEX=2,t.CHAR_DATA_CODE_INDEX=3,t.NULL_CELL_CHAR="",t.NULL_CELL_WIDTH=1,t.NULL_CELL_CODE=0,t.WHITESPACE_CELL_CHAR=" ",t.WHITESPACE_CELL_WIDTH=1,t.WHITESPACE_CELL_CODE=32},133:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.Utf8ToUtf32=t.StringToUtf32=t.utf32ToString=t.stringFromCodePoint=void 0,t.stringFromCodePoint=function(e){return e>65535?(e-=65536,String.fromCharCode(55296+(e>>10))+String.fromCharCode(e%1024+56320)):String.fromCharCode(e)},t.utf32ToString=function(e,t=0,i=e.length){let s="";for(let r=t;r<i;++r){let t=e[r];t>65535?(t-=65536,s+=String.fromCharCode(55296+(t>>10))+String.fromCharCode(t%1024+56320)):s+=String.fromCharCode(t)}return s},t.StringToUtf32=class{constructor(){this._interim=0}clear(){this._interim=0}decode(e,t){const i=e.length;if(!i)return 0;let s=0,r=0;if(this._interim){const i=e.charCodeAt(r++);56320<=i&&i<=57343?t[s++]=1024*(this._interim-55296)+i-56320+65536:(t[s++]=this._interim,t[s++]=i),this._interim=0}for(let o=r;o<i;++o){const r=e.charCodeAt(o);if(55296<=r&&r<=56319){if(++o>=i)return this._interim=r,s;const n=e.charCodeAt(o);56320<=n&&n<=57343?t[s++]=1024*(r-55296)+n-56320+65536:(t[s++]=r,t[s++]=n)}else 65279!==r&&(t[s++]=r)}return s}},t.Utf8ToUtf32=class{constructor(){this.interim=new Uint8Array(3)}clear(){this.interim.fill(0)}decode(e,t){const i=e.length;if(!i)return 0;let s,r,o,n,a=0,h=0,l=0;if(this.interim[0]){let s=!1,r=this.interim[0];r&=192==(224&r)?31:224==(240&r)?15:7;let o,n=0;for(;(o=63&this.interim[++n])&&n<4;)r<<=6,r|=o;const h=192==(224&this.interim[0])?2:224==(240&this.interim[0])?3:4,c=h-n;for(;l<c;){if(l>=i)return 0;if(o=e[l++],128!=(192&o)){l--,s=!0;break}this.interim[n++]=o,r<<=6,r|=63&o}s||(2===h?r<128?l--:t[a++]=r:3===h?r<2048||r>=55296&&r<=57343||65279===r||(t[a++]=r):r<65536||r>1114111||(t[a++]=r)),this.interim.fill(0)}const c=i-4;let d=l;for(;d<i;){for(;!(!(d<c)||128&(s=e[d])||128&(r=e[d+1])||128&(o=e[d+2])||128&(n=e[d+3]));)t[a++]=s,t[a++]=r,t[a++]=o,t[a++]=n,d+=4;if(s=e[d++],s<128)t[a++]=s;else if(192==(224&s)){if(d>=i)return this.interim[0]=s,a;if(r=e[d++],128!=(192&r)){d--;continue}if(h=(31&s)<<6|63&r,h<128){d--;continue}t[a++]=h}else if(224==(240&s)){if(d>=i)return this.interim[0]=s,a;if(r=e[d++],128!=(192&r)){d--;continue}if(d>=i)return this.interim[0]=s,this.interim[1]=r,a;if(o=e[d++],128!=(192&o)){d--;continue}if(h=(15&s)<<12|(63&r)<<6|63&o,h<2048||h>=55296&&h<=57343||65279===h)continue;t[a++]=h}else if(240==(248&s)){if(d>=i)return this.interim[0]=s,a;if(r=e[d++],128!=(192&r)){d--;continue}if(d>=i)return this.interim[0]=s,this.interim[1]=r,a;if(o=e[d++],128!=(192&o)){d--;continue}if(d>=i)return this.interim[0]=s,this.interim[1]=r,this.interim[2]=o,a;if(n=e[d++],128!=(192&n)){d--;continue}if(h=(7&s)<<18|(63&r)<<12|(63&o)<<6|63&n,h<65536||h>1114111)continue;t[a++]=h}}return a}}},776:function(e,t,i){var s=this&&this.__decorate||function(e,t,i,s){var r,o=arguments.length,n=o<3?t:null===s?s=Object.getOwnPropertyDescriptor(t,i):s;if("object"==typeof Reflect&&"function"==typeof Reflect.decorate)n=Reflect.decorate(e,t,i,s);else for(var a=e.length-1;a>=0;a--)(r=e[a])&&(n=(o<3?r(n):o>3?r(t,i,n):r(t,i))||n);return o>3&&n&&Object.defineProperty(t,i,n),n},r=this&&this.__param||function(e,t){return function(i,s){t(i,s,e)}};Object.defineProperty(t,"__esModule",{value:!0}),t.traceCall=t.setTraceLogger=t.LogService=void 0;const o=i(859),n=i(97),a={trace:n.LogLevelEnum.TRACE,debug:n.LogLevelEnum.DEBUG,info:n.LogLevelEnum.INFO,warn:n.LogLevelEnum.WARN,error:n.LogLevelEnum.ERROR,off:n.LogLevelEnum.OFF};let h,l=t.LogService=class extends o.Disposable{get logLevel(){return this._logLevel}constructor(e){super(),this._optionsService=e,this._logLevel=n.LogLevelEnum.OFF,this._updateLogLevel(),this.register(this._optionsService.onSpecificOptionChange("logLevel",(()=>this._updateLogLevel()))),h=this}_updateLogLevel(){this._logLevel=a[this._optionsService.rawOptions.logLevel]}_evalLazyOptionalParams(e){for(let t=0;t<e.length;t++)"function"==typeof e[t]&&(e[t]=e[t]())}_log(e,t,i){this._evalLazyOptionalParams(i),e.call(console,(this._optionsService.options.logger?"":"xterm.js: ")+t,...i)}trace(e,...t){this._logLevel<=n.LogLevelEnum.TRACE&&this._log(this._optionsService.options.logger?.trace.bind(this._optionsService.options.logger)??console.log,e,t)}debug(e,...t){this._logLevel<=n.LogLevelEnum.DEBUG&&this._log(this._optionsService.options.logger?.debug.bind(this._optionsService.options.logger)??console.log,e,t)}info(e,...t){this._logLevel<=n.LogLevelEnum.INFO&&this._log(this._optionsService.options.logger?.info.bind(this._optionsService.options.logger)??console.info,e,t)}warn(e,...t){this._logLevel<=n.LogLevelEnum.WARN&&this._log(this._optionsService.options.logger?.warn.bind(this._optionsService.options.logger)??console.warn,e,t)}error(e,...t){this._logLevel<=n.LogLevelEnum.ERROR&&this._log(this._optionsService.options.logger?.error.bind(this._optionsService.options.logger)??console.error,e,t)}};t.LogService=l=s([r(0,n.IOptionsService)],l),t.setTraceLogger=function(e){h=e},t.traceCall=function(e,t,i){if("function"!=typeof i.value)throw new Error("not supported");const s=i.value;i.value=function(...e){if(h.logLevel!==n.LogLevelEnum.TRACE)return s.apply(this,e);h.trace(`GlyphRenderer#${s.name}(${e.map((e=>JSON.stringify(e))).join(", ")})`);const t=s.apply(this,e);return h.trace(`GlyphRenderer#${s.name} return`,t),t}}},726:(e,t)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.createDecorator=t.getServiceDependencies=t.serviceRegistry=void 0;const i="di$target",s="di$dependencies";t.serviceRegistry=new Map,t.getServiceDependencies=function(e){return e[s]||[]},t.createDecorator=function(e){if(t.serviceRegistry.has(e))return t.serviceRegistry.get(e);const r=function(e,t,o){if(3!==arguments.length)throw new Error("@IServiceName-decorator can only be used to decorate a parameter");!function(e,t,r){t[i]===t?t[s].push({id:e,index:r}):(t[s]=[{id:e,index:r}],t[i]=t)}(r,e,o)};return r.toString=()=>e,t.serviceRegistry.set(e,r),r}},97:(e,t,i)=>{Object.defineProperty(t,"__esModule",{value:!0}),t.IDecorationService=t.IUnicodeService=t.IOscLinkService=t.IOptionsService=t.ILogService=t.LogLevelEnum=t.IInstantiationService=t.ICharsetService=t.ICoreService=t.ICoreMouseService=t.IBufferService=void 0;const s=i(726);var r;t.IBufferService=(0,s.createDecorator)("BufferService"),t.ICoreMouseService=(0,s.createDecorator)("CoreMouseService"),t.ICoreService=(0,s.createDecorator)("CoreService"),t.ICharsetService=(0,s.createDecorator)("CharsetService"),t.IInstantiationService=(0,s.createDecorator)("InstantiationService"),function(e){e[e.TRACE=0]="TRACE",e[e.DEBUG=1]="DEBUG",e[e.INFO=2]="INFO",e[e.WARN=3]="WARN",e[e.ERROR=4]="ERROR",e[e.OFF=5]="OFF"}(r||(t.LogLevelEnum=r={})),t.ILogService=(0,s.createDecorator)("LogService"),t.IOptionsService=(0,s.createDecorator)("OptionsService"),t.IOscLinkService=(0,s.createDecorator)("OscLinkService"),t.IUnicodeService=(0,s.createDecorator)("UnicodeService"),t.IDecorationService=(0,s.createDecorator)("DecorationService")}},t={};function i(s){var r=t[s];if(void 0!==r)return r.exports;var o=t[s]={exports:{}};return e[s].call(o.exports,o,o.exports,i),o.exports}var s={};return(()=>{var e=s;Object.defineProperty(e,"__esModule",{value:!0}),e.CanvasAddon=void 0;const t=i(345),r=i(859),o=i(776),n=i(949);class a extends r.Disposable{constructor(){super(...arguments),this._onChangeTextureAtlas=this.register(new t.EventEmitter),this.onChangeTextureAtlas=this._onChangeTextureAtlas.event,this._onAddTextureAtlasCanvas=this.register(new t.EventEmitter),this.onAddTextureAtlasCanvas=this._onAddTextureAtlasCanvas.event}get textureAtlas(){return this._renderer?.textureAtlas}activate(e){const i=e._core;if(!e.element)return void this.register(i.onWillOpen((()=>this.activate(e))));this._terminal=e;const s=i.coreService,a=i.optionsService,h=i.screenElement,l=i.linkifier,c=i,d=c._bufferService,_=c._renderService,u=c._characterJoinerService,g=c._charSizeService,f=c._coreBrowserService,v=c._decorationService,C=c._logService,p=c._themeService;(0,o.setTraceLogger)(C),this._renderer=new n.CanvasRenderer(e,h,l,d,g,a,u,s,f,v,p),this.register((0,t.forwardEvent)(this._renderer.onChangeTextureAtlas,this._onChangeTextureAtlas)),this.register((0,t.forwardEvent)(this._renderer.onAddTextureAtlasCanvas,this._onAddTextureAtlasCanvas)),_.setRenderer(this._renderer),_.handleResize(d.cols,d.rows),this.register((0,r.toDisposable)((()=>{_.setRenderer(this._terminal._core._createRenderer()),_.handleResize(e.cols,e.rows),this._renderer?.dispose(),this._renderer=void 0})))}clearTextureAtlas(){this._renderer?.clearTextureAtlas()}}e.CanvasAddon=a})(),s})()));
//# sourceMappingURL=addon-canvas.js.map

/***/ })

};
;