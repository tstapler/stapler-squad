"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[3517],{

/***/ 28990:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 40230:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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
/** First matching row wins; `tui-wheel` is only reachable when tuiPolicy is 'wheel'. */ function decideScrollTarget(mode) {
    let policy = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : DEFAULT_ROUTING_POLICY, tuiPolicy = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : TUI_SCROLL_POLICY, override = arguments.length > 3 && arguments[3] !== void 0 ? arguments[3] : "auto";
    const tuiTarget = tuiPolicy === "wheel" ? "tui-wheel" : "tui-pgkeys";
    if (override === "local") return "xterm-local";
    if (override === "tui") return tuiTarget;
    const row = policy.rules.find((r)=>ruleMatches(r.when, mode));
    const target = row ? row.target : policy.default;
    return target === "xterm-local" ? target : tuiTarget;
}
/** decideScrollTarget plus a log entry carrying `unverified:true` while routing is unverified. */ function decideAndLogScrollTarget(mode) {
    let opts = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    var _opts_log;
    var _opts_override;
    const override = (_opts_override = opts.override) !== null && _opts_override !== void 0 ? _opts_override : "auto";
    const target = decideScrollTarget(mode, opts.policy, opts.tuiPolicy, override);
    var _opts_routingVerified;
    const verified = (_opts_routingVerified = opts.routingVerified) !== null && _opts_routingVerified !== void 0 ? _opts_routingVerified : ROUTING_VERIFIED;
    const entry = {
        target,
        source: override === "auto" ? "auto" : "override",
        mode,
        ...verified ? {} : {
            unverified: true
        }
    };
    (_opts_log = opts.log) === null || _opts_log === void 0 ? void 0 : _opts_log.call(opts, entry);
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
    return "\x1b[<".concat(button, ";").concat(col, ";").concat(row, "M").repeat(count);
}
/** X10 fallback; retained for the spike but deliberately not selectable via decideScrollTarget. */ function encodeWheelX10(lineDelta, cell) {
    const count = Math.min(Math.abs(Math.trunc(lineDelta)), MAX_WHEEL_REPORTS_PER_FRAME);
    if (count === 0) return "";
    const col = clamp(Math.trunc(cell.col), 1, Math.max(1, cell.cols));
    const row = clamp(Math.trunc(cell.row), 1, Math.max(1, cell.rows));
    const button = lineDelta < 0 ? 64 : 65;
    // X10 encodes each value as a single byte offset by 32 (limited to 223 cells).
    const ch = (n)=>String.fromCharCode(clamp(n, 0, 223) + 32);
    return "\x1b[M".concat(ch(button)).concat(ch(col)).concat(ch(row)).repeat(count);
}
const PAGE_UP_BYTES = "\x1b[5~";
const PAGE_DOWN_BYTES = "\x1b[6~";
/** direction 'older' = PgUp, 'newer' = PgDn (same bytes as the toolbar keys). */ function encodePageKeys(direction) {
    let count = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : 1;
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
    constructor(opts){
        this.carry = 0;
        this.emitted = 0;
        this.lastEmitAt = Number.NEGATIVE_INFINITY;
        this.rows = opts.rows;
        this.now = opts.now;
        var _opts_rateLimitMs;
        this.rateLimitMs = (_opts_rateLimitMs = opts.rateLimitMs) !== null && _opts_rateLimitMs !== void 0 ? _opts_rateLimitMs : PAGE_RATE_LIMIT_MS;
        var _opts_flingCap;
        this.flingCap = (_opts_flingCap = opts.flingCap) !== null && _opts_flingCap !== void 0 ? _opts_flingCap : PAGE_FLING_CAP;
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

/***/ 81073:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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
    if (false) {}
    try {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (!stored) {
            return DEFAULT_TERMINAL_CONFIG;
        }
        const config = JSON.parse(stored);
        var _config_scrollbackLines, _config_fontSize;
        // Validate and merge with defaults
        return {
            ...DEFAULT_TERMINAL_CONFIG,
            ...config,
            // Ensure scrollbackLines is within reasonable bounds (0 = disabled, tmux handles it)
            scrollbackLines: Math.max(0, Math.min((_config_scrollbackLines = config.scrollbackLines) !== null && _config_scrollbackLines !== void 0 ? _config_scrollbackLines : DEFAULT_TERMINAL_CONFIG.scrollbackLines, 100000)),
            // Ensure fontSize is within reasonable bounds
            fontSize: Math.max(8, Math.min((_config_fontSize = config.fontSize) !== null && _config_fontSize !== void 0 ? _config_fontSize : DEFAULT_TERMINAL_CONFIG.fontSize, 32))
        };
    } catch (err) {
        console.error("[terminalConfig] Failed to load config:", err);
        return DEFAULT_TERMINAL_CONFIG;
    }
}
/**
 * Save terminal configuration to localStorage
 */ function saveTerminalConfig(config) {
    if (false) {}
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

/***/ 83517:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
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
    var _terminal_options_fontSize;
    // Fallback: use font metrics (less accurate but never undefined)
    const fontSize = (_terminal_options_fontSize = terminal.options.fontSize) !== null && _terminal_options_fontSize !== void 0 ? _terminal_options_fontSize : 14;
    var _terminal_options_lineHeight;
    const lineHeight = (_terminal_options_lineHeight = terminal.options.lineHeight) !== null && _terminal_options_lineHeight !== void 0 ? _terminal_options_lineHeight : 1.0;
    return {
        cellH: fontSize * lineHeight,
        cellW: fontSize * 0.6
    };
}

;// ./src/lib/terminal/mouseTracking.ts
function readMouseTrackingMode(terminal) {
    var _terminal_modes;
    return (_terminal_modes = terminal.modes) === null || _terminal_modes === void 0 ? void 0 : _terminal_modes.mouseTrackingMode;
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
    var _terminal_buffer_active, _terminal_buffer;
    const raw = readMouseTrackingMode(terminal);
    const mouseTrackingMode = raw !== undefined && KNOWN_TRACKING_MODES.includes(raw) ? raw : raw === undefined ? "none" : "any";
    const bufferType = ((_terminal_buffer = terminal.buffer) === null || _terminal_buffer === void 0 ? void 0 : (_terminal_buffer_active = _terminal_buffer.active) === null || _terminal_buffer_active === void 0 ? void 0 : _terminal_buffer_active.type) === "alternate" ? "alternate" : "normal";
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
 */ function getTerminalRowText(terminal, viewportRow, activeCol) {
    let windowSize = arguments.length > 3 && arguments[3] !== void 0 ? arguments[3] : 20;
    const buf = terminal.buffer.active;
    const line = buf.getLine(viewportRow + buf.viewportY);
    const startCol = Math.max(0, activeCol - Math.floor(windowSize / 2));
    const endCol = Math.min(terminal.cols, startCol + windowSize);
    var _line_translateToString;
    const text = ((_line_translateToString = line === null || line === void 0 ? void 0 : line.translateToString(false, startCol, endCol)) !== null && _line_translateToString !== void 0 ? _line_translateToString : '').padEnd(endCol - startCol, ' ');
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
    var _e_totalDx;
    // Inclusive: PENDING persists while travel <= slop, so a release at exactly the slop is still a tap.
    if (state === "PENDING" && Math.max(e.totalDy, (_e_totalDx = e.totalDx) !== null && _e_totalDx !== void 0 ? _e_totalDx : 0) <= e.tapTolerancePx && e.elapsedMs < e.longPressMs) {
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
var scrollRouting = __webpack_require__(40230);
// EXTERNAL MODULE: ./src/lib/terminal/mobileDebug.ts
var mobileDebug = __webpack_require__(99050);
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
    (0,react.useEffect)(()=>{
        optionsRef.current = options;
    });
    const cancelGestureRef = (0,react.useRef)(null);
    // Override / connection changes cancel an in-flight gesture (S9: reset remainders, ignore the touch).
    const previousRef = (0,react.useRef)({
        override,
        connectionEpoch
    });
    (0,react.useEffect)(()=>{
        const prev = previousRef.current;
        if (prev.override !== override || prev.connectionEpoch !== connectionEpoch) {
            var _cancelGestureRef_current;
            (_cancelGestureRef_current = cancelGestureRef.current) === null || _cancelGestureRef_current === void 0 ? void 0 : _cancelGestureRef_current.call(cancelGestureRef);
        }
        previousRef.current = {
            override,
            connectionEpoch
        };
    }, [
        override,
        connectionEpoch
    ]);
    (0,react.useEffect)(()=>{
        if (!gestureScrollEnabled) return;
        const containerEl = containerRef.current;
        if (!containerEl) return;
        const onSendData = (data)=>optionsRef.current.onSendData(data);
        const longPressMs = ()=>{
            var _optionsRef_current_longPressMs;
            return (_optionsRef_current_longPressMs = optionsRef.current.longPressMs) !== null && _optionsRef_current_longPressMs !== void 0 ? _optionsRef_current_longPressMs : 400;
        };
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
            var _optionsRef_current_onGestureActiveChange, _optionsRef_current;
            const active = state === 'PENDING' || state === 'SCROLLING' || state === 'SELECTING' || state === 'COASTING';
            if (active === gestureActive) return;
            gestureActive = active;
            (_optionsRef_current_onGestureActiveChange = (_optionsRef_current = optionsRef.current).onGestureActiveChange) === null || _optionsRef_current_onGestureActiveChange === void 0 ? void 0 : _optionsRef_current_onGestureActiveChange.call(_optionsRef_current, active);
        };
        const sampleViewportY = ()=>{
            var _terminalRef_current_buffer_active, _terminalRef_current_buffer, _terminalRef_current;
            const y = (_terminalRef_current = terminalRef.current) === null || _terminalRef_current === void 0 ? void 0 : (_terminalRef_current_buffer = _terminalRef_current.buffer) === null || _terminalRef_current_buffer === void 0 ? void 0 : (_terminalRef_current_buffer_active = _terminalRef_current_buffer.active) === null || _terminalRef_current_buffer_active === void 0 ? void 0 : _terminalRef_current_buffer_active.viewportY;
            if (y !== undefined && y !== dragStartViewportY) dragViewportYChanged = true;
        };
        const reportScrollGesture = ()=>{
            var _optionsRef_current_onScrollGesture, _optionsRef_current;
            if (!dragActive || state !== 'SCROLLING') return;
            dragActive = false;
            sampleViewportY();
            (_optionsRef_current_onScrollGesture = (_optionsRef_current = optionsRef.current).onScrollGesture) === null || _optionsRef_current_onScrollGesture === void 0 ? void 0 : _optionsRef_current_onScrollGesture.call(_optionsRef_current, {
                route: dragRoute,
                postSlopLines: dragPostSlopLines,
                viewportYChanged: dragViewportYChanged
            });
        };
        // Release applies the rAF-coalesced last point (up to a frame of travel) before the gesture is summarized and torn down.
        const flushAndReportScroll = ()=>{
            if (state === 'SCROLLING') flushScrollThrottle === null || flushScrollThrottle === void 0 ? void 0 : flushScrollThrottle();
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
            cancelScrollThrottle === null || cancelScrollThrottle === void 0 ? void 0 : cancelScrollThrottle();
            cancelSelectThrottle === null || cancelSelectThrottle === void 0 ? void 0 : cancelSelectThrottle();
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
            bufferChangeSub === null || bufferChangeSub === void 0 ? void 0 : bufferChangeSub.dispose();
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
            var _getScreenEl;
            [selectThrottled, cancelSelectThrottle, flushSelectThrottle] = rafThrottlePoint((clientX, clientY)=>{
                var _getScreenEl;
                (_getScreenEl = getScreenEl()) === null || _getScreenEl === void 0 ? void 0 : _getScreenEl.dispatchEvent(new MouseEvent('mousemove', {
                    clientX,
                    clientY,
                    bubbles: true,
                    cancelable: true,
                    button: 0,
                    buttons: 1
                }));
            });
            (_getScreenEl = getScreenEl()) === null || _getScreenEl === void 0 ? void 0 : _getScreenEl.dispatchEvent(new MouseEvent('mousedown', {
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
            var // Haptic feedback if available (R4.3)
            _navigator_vibrate, _navigator;
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
            (_navigator_vibrate = (_navigator = navigator).vibrate) === null || _navigator_vibrate === void 0 ? void 0 : _navigator_vibrate.call(_navigator, 10);
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
                        var _touch_clientY;
                        scrollThrottled === null || scrollThrottled === void 0 ? void 0 : scrollThrottled(0, (_touch_clientY = touch === null || touch === void 0 ? void 0 : touch.clientY) !== null && _touch_clientY !== void 0 ? _touch_clientY : lastY);
                        break;
                    case 'continueSelect':
                        if (touch) {
                            lastSelectX = touch.clientX;
                            lastSelectY = touch.clientY;
                            selectThrottled === null || selectThrottled === void 0 ? void 0 : selectThrottled(touch.clientX, touch.clientY);
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
                var _logged_entry;
                if (lastTarget !== null) {
                    lineAcc.reset();
                    pageAcc.reset();
                }
                lastTarget = target;
                var _logged_entry_source;
                mobileDebug/* mobileDebug */.Uo.routeDecision({
                    target,
                    source: (_logged_entry_source = (_logged_entry = logged.entry) === null || _logged_entry === void 0 ? void 0 : _logged_entry.source) !== null && _logged_entry_source !== void 0 ? _logged_entry_source : 'auto',
                    unverified: !scrollRouting/* ROUTING_VERIFIED */.YA
                });
            }
            return target;
        };
        const dispatchScroll = function(terminal, target, lines) {
            let pages = arguments.length > 3 && arguments[3] !== void 0 ? arguments[3] : pageAcc;
            switch(target){
                case 'xterm-local':
                    {
                        var _terminal_buffer_active, _terminal_buffer;
                        if (((_terminal_buffer = terminal.buffer) === null || _terminal_buffer === void 0 ? void 0 : (_terminal_buffer_active = _terminal_buffer.active) === null || _terminal_buffer_active === void 0 ? void 0 : _terminal_buffer_active.length) === 0) return; // nothing to scroll; mirrors the jump button's empty-buffer rule
                        const clamped = clampLinesPerFrame(lines, terminal.rows);
                        if (clamped !== 0) terminal.scrollLines(clamped);
                        return;
                    }
                case 'tui-pgkeys':
                    {
                        var _optionsRef_current_isInputBusy, _optionsRef_current;
                        if ((_optionsRef_current_isInputBusy = (_optionsRef_current = optionsRef.current).isInputBusy) === null || _optionsRef_current_isInputBusy === void 0 ? void 0 : _optionsRef_current_isInputBusy.call(_optionsRef_current)) {
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
                            var _optionsRef_current_onPageKeysSent, _optionsRef_current1;
                            onSendData(keys);
                            const count = keys.split(scrollRouting/* PAGE_UP_BYTES */.UU).length - 1 + keys.split(scrollRouting/* PAGE_DOWN_BYTES */.Nl).length - 1;
                            const direction = keys.startsWith(scrollRouting/* PAGE_UP_BYTES */.UU) ? 'up' : 'down';
                            (_optionsRef_current_onPageKeysSent = (_optionsRef_current1 = optionsRef.current).onPageKeysSent) === null || _optionsRef_current_onPageKeysSent === void 0 ? void 0 : _optionsRef_current_onPageKeysSent.call(_optionsRef_current1, direction, count);
                        }
                        return;
                    }
                case 'tui-wheel':
                    {
                        var _optionsRef_current_isInputBusy1, _optionsRef_current2;
                        if ((_optionsRef_current_isInputBusy1 = (_optionsRef_current2 = optionsRef.current).isInputBusy) === null || _optionsRef_current_isInputBusy1 === void 0 ? void 0 : _optionsRef_current_isInputBusy1.call(_optionsRef_current2)) {
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
            var _o_isAltScreenActive, _terminal_buffer;
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
            if (target === 'xterm-local' && lines < 0 && ((_o_isAltScreenActive = o.isAltScreenActive) === null || _o_isAltScreenActive === void 0 ? void 0 : _o_isAltScreenActive.call(o)) && o.onAltScreenScrollUp) {
                o.onAltScreenScrollUp(-lines);
            } else {
                dispatchScroll(terminal, target, lines);
            }
            sampleViewportY();
            const active = (_terminal_buffer = terminal.buffer) === null || _terminal_buffer === void 0 ? void 0 : _terminal_buffer.active;
            if (debugOn) {
                mobileDebug/* mobileDebug */.Uo.log('scroll', {
                    ...mode,
                    viewportY: active === null || active === void 0 ? void 0 : active.viewportY,
                    baseY: active === null || active === void 0 ? void 0 : active.baseY,
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
            var _terminal_buffer;
            const active = (_terminal_buffer = terminal.buffer) === null || _terminal_buffer === void 0 ? void 0 : _terminal_buffer.active;
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
            var _terminal_buffer_onBufferChange, _terminal_buffer;
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
            var _terminal_buffer_onBufferChange1;
            bufferChangeSub = (_terminal_buffer_onBufferChange1 = (_terminal_buffer = terminal.buffer) === null || _terminal_buffer === void 0 ? void 0 : (_terminal_buffer_onBufferChange = _terminal_buffer.onBufferChange) === null || _terminal_buffer_onBufferChange === void 0 ? void 0 : _terminal_buffer_onBufferChange.call(_terminal_buffer, ()=>{
                if (state === 'COASTING') dispatch({
                    type: 'interrupt'
                }, null, undefined);
            })) !== null && _terminal_buffer_onBufferChange1 !== void 0 ? _terminal_buffer_onBufferChange1 : null;
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
            var _t_buffer_active, _t_buffer;
            var _touch_clientY;
            const y = (_touch_clientY = touch === null || touch === void 0 ? void 0 : touch.clientY) !== null && _touch_clientY !== void 0 ? _touch_clientY : lastY;
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
            var _t_buffer_active_viewportY;
            dragStartViewportY = (_t_buffer_active_viewportY = t === null || t === void 0 ? void 0 : (_t_buffer = t.buffer) === null || _t_buffer === void 0 ? void 0 : (_t_buffer_active = _t_buffer.active) === null || _t_buffer_active === void 0 ? void 0 : _t_buffer_active.viewportY) !== null && _t_buffer_active_viewportY !== void 0 ? _t_buffer_active_viewportY : 0;
            if (t && cachedCellH > 0) {
                var _optionsRef_current_onScrollStart, _optionsRef_current;
                dragRoute = resolveTarget(readScrollMode(t));
                (_optionsRef_current_onScrollStart = (_optionsRef_current = optionsRef.current).onScrollStart) === null || _optionsRef_current_onScrollStart === void 0 ? void 0 : _optionsRef_current_onScrollStart.call(_optionsRef_current, dragRoute);
            }
        };
        const hasSelection = (t)=>{
            var _t_getSelection;
            return typeof t.hasSelection === 'function' ? t.hasSelection() : Boolean((_t_getSelection = t.getSelection) === null || _t_getSelection === void 0 ? void 0 : _t_getSelection.call(t));
        };
        const clearSelection = ()=>{
            var _terminalRef_current_clearSelection, _terminalRef_current;
            (_terminalRef_current = terminalRef.current) === null || _terminalRef_current === void 0 ? void 0 : (_terminalRef_current_clearSelection = _terminalRef_current.clearSelection) === null || _terminalRef_current_clearSelection === void 0 ? void 0 : _terminalRef_current_clearSelection.call(_terminalRef_current);
        };
        const handleTap = ()=>{
            const now = Date.now();
            const isDoubleTap = now - lastTapTime < DOUBLE_TAP_MS && Math.abs(tapX - lastTapX) < DOUBLE_TAP_RADIUS_PX && Math.abs(tapY - lastTapY) < DOUBLE_TAP_RADIUS_PX;
            const t = terminalRef.current;
            if (isDoubleTap && !isMouseTracking()) {
                var // Dispatch synthetic dblclick to trigger xterm's native word selection
                _getScreenEl;
                (_getScreenEl = getScreenEl()) === null || _getScreenEl === void 0 ? void 0 : _getScreenEl.dispatchEvent(new MouseEvent('dblclick', {
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
                    var _optionsRef_current_isSgrMouseEncoding, _optionsRef_current;
                    const { cellH, cellW } = getCellDimensions(t);
                    const canvasRect = t.element.getBoundingClientRect();
                    const col = Math.floor((tapX - canvasRect.left) / cellW) + 1; // 1-based
                    const row = Math.floor((tapY - canvasRect.top) / cellH) + 1; // 1-based
                    if ((_optionsRef_current_isSgrMouseEncoding = (_optionsRef_current = optionsRef.current).isSgrMouseEncoding) === null || _optionsRef_current_isSgrMouseEncoding === void 0 ? void 0 : _optionsRef_current_isSgrMouseEncoding.call(_optionsRef_current)) {
                        // SGR (?1006h): \x1b[<0;col;rowM press, ...m release; no coordinate limit.
                        const c = Math.max(1, col);
                        const r = Math.max(1, row);
                        onSendData("\x1b[<0;".concat(c, ";").concat(r, "M\x1b[<0;").concat(c, ";").concat(r, "m"));
                    } else {
                        // X10 mouse encoding: \x1b[M + button(32=left-press) + col+32 + row+32
                        // Clamp col/row to 1-223 so charCode stays in 33-255 (valid X10 range)
                        const clampedCol = Math.max(1, Math.min(col, 223));
                        const clampedRow = Math.max(1, Math.min(row, 223));
                        const press = "\x1b[M".concat(String.fromCharCode(32, clampedCol + 32, clampedRow + 32));
                        const release = "\x1b[M".concat(String.fromCharCode(35, clampedCol + 32, clampedRow + 32)); // 35 = release
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
            flushSelectThrottle === null || flushSelectThrottle === void 0 ? void 0 : flushSelectThrottle(); // apply the last coalesced point before the button releases
            if (!isMouseTracking()) {
                var _getScreenEl, // xterm.js's platform check for "Linux" matches Android's user agent too, so
                // completing a real mouse-event-driven selection makes it focus+select the
                // hidden input textarea (to populate the X11 primary-selection clipboard for a
                // desktop middle-click paste) — that focus() call is what pops the Android soft
                // keyboard mid text-selection. Desktop Linux wants that; touch doesn't. The
                // focus already happened synchronously inside dispatchEvent above, so blur
                // it back immediately.
                _t_textarea;
                var _touch_clientX, _touch_clientY;
                (_getScreenEl = getScreenEl()) === null || _getScreenEl === void 0 ? void 0 : _getScreenEl.dispatchEvent(new MouseEvent('mouseup', {
                    clientX: (_touch_clientX = touch === null || touch === void 0 ? void 0 : touch.clientX) !== null && _touch_clientX !== void 0 ? _touch_clientX : lastSelectX,
                    clientY: (_touch_clientY = touch === null || touch === void 0 ? void 0 : touch.clientY) !== null && _touch_clientY !== void 0 ? _touch_clientY : lastSelectY,
                    bubbles: true,
                    cancelable: true,
                    button: 0,
                    buttons: 0
                }));
                t === null || t === void 0 ? void 0 : (_t_textarea = t.textarea) === null || _t_textarea === void 0 ? void 0 : _t_textarea.blur();
            }
        // Selection preserved in xterm's buffer — just transition back
        };
        // ---- touchstart (registered on containerEl, passive: false) ----
        const startedOnIgnoredSurface = (e)=>{
            var _target_closest;
            const target = e.target;
            return Boolean(target === null || target === void 0 ? void 0 : (_target_closest = target.closest) === null || _target_closest === void 0 ? void 0 : _target_closest.call(target, GESTURE_IGNORE_SELECTOR));
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
                var _window_matchMedia, _window;
                var _window_matchMedia_matches;
                reducedMotion = (_window_matchMedia_matches = (_window_matchMedia = (_window = window).matchMedia) === null || _window_matchMedia === void 0 ? void 0 : _window_matchMedia.call(_window, '(prefers-reduced-motion: reduce)').matches) !== null && _window_matchMedia_matches !== void 0 ? _window_matchMedia_matches : false;
                const touch = e.touches[0];
                startX = touch.clientX;
                startY = touch.clientY;
                lastY = touch.clientY;
                startTime = Date.now();
                tapX = touch.clientX;
                tapY = touch.clientY;
                // Calculate starting cell coordinates for selection/tap
                const t = terminalRef.current;
                if (t === null || t === void 0 ? void 0 : t.element) {
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
            var _touch_clientY, _touch_clientX;
            dispatch({
                type: 'touchend',
                elapsedMs: Date.now() - startTime,
                totalDy: Math.abs(((_touch_clientY = touch === null || touch === void 0 ? void 0 : touch.clientY) !== null && _touch_clientY !== void 0 ? _touch_clientY : startY) - startY),
                totalDx: Math.abs(((_touch_clientX = touch === null || touch === void 0 ? void 0 : touch.clientX) !== null && _touch_clientX !== void 0 ? _touch_clientX : startX) - startX),
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
        visualViewport === null || visualViewport === void 0 ? void 0 : visualViewport.addEventListener('resize', onInterrupt);
        window.addEventListener('orientationchange', onInterrupt);
        cancelGestureRef.current = onInterrupt;
        return ()=>{
            cancelGestureRef.current = null;
            if (gestureActive) {
                var _optionsRef_current_onGestureActiveChange, _optionsRef_current;
                gestureActive = false;
                (_optionsRef_current_onGestureActiveChange = (_optionsRef_current = optionsRef.current).onGestureActiveChange) === null || _optionsRef_current_onGestureActiveChange === void 0 ? void 0 : _optionsRef_current_onGestureActiveChange.call(_optionsRef_current, false);
            }
            clearLongPressTimer();
            cancelPendingFrames();
            cancelMomentum();
            containerEl.removeEventListener('touchstart', onTouchStart);
            document.removeEventListener('touchmove', onTouchMove);
            document.removeEventListener('touchend', onTouchEnd);
            document.removeEventListener('touchcancel', onTouchCancel);
            visualViewport === null || visualViewport === void 0 ? void 0 : visualViewport.removeEventListener('resize', onInterrupt);
            window.removeEventListener('orientationchange', onInterrupt);
        };
    }, [
        containerRef,
        terminalRef,
        gestureScrollEnabled
    ]); // options other than these are read via optionsRef
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+xterm@6.0.0/node_modules/@xterm/xterm/lib/xterm.mjs
var xterm = __webpack_require__(9296);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-fit@0.11.0/node_modules/@xterm/addon-fit/lib/addon-fit.mjs
var addon_fit = __webpack_require__(70568);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-web-links@0.12.0/node_modules/@xterm/addon-web-links/lib/addon-web-links.mjs
var addon_web_links = __webpack_require__(79758);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-canvas@0.7.0+48a5bd62dd64c178/node_modules/@xterm/addon-canvas/lib/addon-canvas.js
var addon_canvas = __webpack_require__(11009);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-search@0.16.0/node_modules/@xterm/addon-search/lib/addon-search.mjs
var addon_search = __webpack_require__(10800);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+addon-serialize@0.14.0/node_modules/@xterm/addon-serialize/lib/addon-serialize.mjs
var addon_serialize = __webpack_require__(76880);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@xterm+xterm@6.0.0/node_modules/@xterm/xterm/css/xterm.css
var css_xterm = __webpack_require__(3310);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FXtermTerminal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VY7WvzNhD_3r9CCzzQQBRkJ24TlcFgn5993hhjyPY5FrUlT5YTt6P_-7Dk99h5aZ-RktS60-nud6_yL6_wFimWQo44iQuxRf8-IES-mR-EZMYCrt8oIi_mWSsm8kiqlNp_E6bh90fskW_LbuGPx21WLqsNHw8IOd6ZMOcuYaQVtftxohzyVRtxZ-THw9qAR4zEEw91TM0JFTkGfoh19xzyPEvYG0VRAmW1UP3ikCsINJeCokAmRSoqSiZzbtcUJEzzI1SrPgteD0oWIqToyNQjxn87e3h31fuz0ceXKgSFFQt5kVO0zcwp8ggqSuSJopiHIQjLWeKcv3NxoM0uX5YvrT2OsafTgvm5TApttNAyGx_vvpvzlbV3kvaOuQihpMj1jIEsDM3pI-a35fT2vunq4LNHskL133rrGZZAJlLVVNfzVqj7Iutnr4cQRU5WolwmPJxhd7wpQIeabSzokRS6whLO6Kyjn2AKms3BKl6ovNI8k1xoUBbjIogxq-MiZYJnRRUH0nivyEHhHBIINEVCCuMWfAL_lWs8SUy4ANzGY8_PNK7Cw3j7EsRPN0C8H2B2kXWz7KtQmXkEo0OXketdx-IaWpUtddpfSbSUi9ZYMpcCMynWS4xACg1C28zohawRmTJ14KInPw-UTBLsQ8yOvDK92s646JnxZ8g0wwfIdaEA2w0_L6RY_GUMHDrduq6DYF1qUKlh7NuLfuJpJpVmQo-QGVE69UeE1pLR-gwUA66xepVRAOLzWt5y6N22nGmp5Cm3Mf8ZG48cThXVRmwdWvitH1zWtz5TuDZVx3xEuJggbt1wMqbgog6UNnnfSu63ol1W3rcZa8WC17N68GllsI6L1B_Ju2b7XC-7_-DJ8nZ7berC2tRTLsWXRdFIBoUNPlnoqi5T5A670WazQo5HVsjdbioxtq7W3FhGUQ6aIuz2IdmMGnbESwgHfdchu_s674acdd4hQ9rvCUPSP4Nu23Shr7VT_2o7jefaqUnzmIVVByAG7l1WNk3OfNbb5Y_vu61z5lqccUhv6swDlsAjWe-fe5HjzbnWl1rLlKIdsYNeApGmyLM98MIs-z-HRTEfFuSGqWr_5TA4LG_0-WZpBwETJxiOIHQ-9t2TQZ8JnjILf31ZctZejoDlgLnAstAokurEVJh3W58vDNHWV6Sdp0nfoeRlVMHPx-eRzqzQ8qZRcmjc7qqGjj2-07FW68muj1y5qVfHk-R5jd9NQt-YMaVzZ9xBMf8r6bjrp-N1XT1vOT7d5-LQydvPJWiNletaWJqBqHkegVcn7rV62xRVd-21beMUcw3TQf_UBX13YZqodQ1-vcKwJ327Q4hYkejP3T-mqpEpRitEeqVuT6kPkVTWNfVARtFi8TIbpg3GQ4id7XkkzoPZusBpHGPzsLlP3FNY7QSKWxH9Js2uBQohQzM27ij7HbJ35mvWZzwz9yZg5r3BWbZsqzFlhdyduQJu77wm3nAT3918E7_3nYLpIRFLefJG0XcQiVyh71KwQK7Q4ldZKA4K_QanxQqlUsg8YwHccuMfXrbXW-PhKkOxEUFRpiaSdZuVyJnKVlN_mhCyo7SGUuMQAqnqplSIEFR17ss5FU-2Yhv_Y1Yd8-BVQJ63GWU4WvHt9FlTZ0V__Ae5y5JxXxQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FXtermTerminal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VY7WvzNhD_3r9CCzzQQBRkJ24TlcFgn5993hhjyPY5FrUlT5YTt6P_7Dk99h5aZ_RktS60_nud6_yL6_wFimWQo44iQuxRf8_IES_mR_EZMYCrt8oIi_mWSsm8kiqlNp_E6bh90fskW_LbuGPx21WLqsNHw8IOd6ZMOcuYaQVtftxohzyVRtxZ_THw9qAR4zEEw91TM0JFTkGfoh19xzyPEvYG0VRAmW1UP3ikCsINJeCokAmRSoqSiZzbtcUJEzzI1SrPgteD0oWIqToyNQjxn87e3h31fuz0ceXKgSFFQt5kVO0zcwp8ggqSuSJopiHIQjLWeKcv3NxoM0uX5YvrT2OsafTgvm5TApttNAyGx_vvpvzlbV3kvaOuQihpMj1jIEsDM3pI_a35fT2vunq4LNHskL133rrGZZAJlLVVNfzVqj7Iutnr4cQRU5WolwmPJxhd7wpQIeabSzokRS6whLO6Kyjn2AKms3BKl6ovNI8k1xoUBbjIogxq_MiZYJnRRUH0nivyEHhHBIINEVCCuMWfAL_lWs8SUy4ANzGY8_PNK7Cw3j7EsRPN0C8H2B2kXWz7KtQmXkEo0OXketdx_IaWpUtddpfSbSUi9ZYMpcCMynWS4xACg1C28zohawRmTJ14KInPw_UTBLsQ8yOvDK92s646JnxZ8g0wwfIdaEA2w0_L6RY_GUMHDrduq6DYF1qUKlh7NuLfuJpJpVmQo_QGVE69UeE1pLR_gwUA66xepVRAOLzWt5y6N22nGmp5Cm3Mf8ZG48cThXVRmwdWvitH1zWtz5TuDZVx3xEuJggbt1wMqbgog6UNnnfSu63ol1W3rcZa8WC17N68GllsI6L1B_Ju2b7XC_7_DJ8nZ7berC2tRTLsWXRdFIBoUNPlnoqi5T5A670WazQo5HVsjdbioxtq7W3FhGUQ6aIuz2IdmMGnbESwgHfdchu_s674acdd4hQ9rvCUPSP4Nu23Shr7VT_2o7jefaqUnzmIVVByAG7l1WNk3OfNbb5Y_vu61z5lqccUhv6swDlsAjWe_fe5HjzbnWl1rLlKIdsYNeApGmyLM98MIs_z_HRTEfFuSGqWr_5TA4LG_0_WZpBwETJxiOIHQ_9t2TQZ8JnjILf31ZctZejoDlgLnAstAokurEVJh3W58vDNHWV6Sdp0nfoeRlVMHPx_eRzqzQ8qZRcmjc7qqGjj2_07FW68muj1y5qVfHk_R5jd9NQt_YMaVzZ9xBMf8r6bjrp_N1XT1vOT7d5_LQydvPJWiNletaWJqBqHkegVcn7rV62xRVd_21beMUcw3TQf_UBX13YZqodQ1_vcKwJ327Q4hYkejP3T_mqpEpRitEeqVuT6kPkVTWNfVARtFi8TIbpg3GQ4id7XkkzoPZusBpHGPzsLlP3FNY7QSKWxH9Js2uBQohQzM27ij7HbJ35mvWZzwz9yZg5r3BWbZsqzFlhdyduQJu77wm3nAT3918E7_3nYLpIRFLefJG0XcQiVyh71KwQK7Q4ldZKA4K_QanxQqlUsg8YwHccuMfXrbXW_PhKkOxEUFRpiaSdZuVyJnKVlN_mhCyo7SGUuMQAqnqplSIEFR17ss5FU_2Yhv_Y1Yd8_BVQJ63GWU4WvHt9FlTZ0V_Ae5y5JxXxQAAA_22_7D = __webpack_require__(6349);
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
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTerminalContextMenu_css_ts_vanilla_css_22_2C_22source_22_3A_22LnFmYm5qMTAgewogIHBvc2l0aW9uOiBmaXhlZDsKICB6LWluZGV4OiAxMDg1OwogIG1pbi13aWR0aDogMTQwcHg7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzcpOwogIGJveC1zaGFkb3c6IHZhcigtLV8xOWV6MnJ6M20pOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnkpIDA7CiAgbGlzdC1zdHlsZTogbm9uZTsKICBtYXJnaW46IDA7CiAgb3V0bGluZTogbm9uZTsKfQoucWZibmoxMSB7CiAgZGlzcGxheTogYmxvY2s7CiAgd2lkdGg6IDEwMCU7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeikgdmFyKC0tXzE5ZXoycnozMCk7CiAgYmFja2dyb3VuZDogbm9uZTsKICBib3JkZXI6IG5vbmU7CiAgdGV4dC1hbGlnbjogbGVmdDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2IpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIGN1cnNvcjogcG9pbnRlcjsKICB1c2VyLXNlbGVjdDogbm9uZTsKfQoucWZibmoxMTpob3Zlcjpub3QoOmRpc2FibGVkKSB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno4KTsKfQoucWZibmoxMTpkaXNhYmxlZCB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Myk7CiAgY3Vyc29yOiBkZWZhdWx0Owp9_22_7D = __webpack_require__(60520);
;// ./src/components/sessions/TerminalContextMenu.css.ts


var menu = 'qfbnj10';
var menuItem = 'qfbnj11';
;// ./src/components/sessions/TerminalContextMenu.tsx
/* __next_internal_client_entry_do_not_use__ TerminalContextMenu auto */ 



function TerminalContextMenu(param) {
    let { x, y, hasSelection, onCopy, onSelectAll, onPaste, onDismiss } = param;
    var _navigator_clipboard;
    const menuRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
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
        onPaste === null || onPaste === void 0 ? void 0 : onPaste();
        onDismiss();
    };
    const hasPasteSupport = typeof navigator !== "undefined" && typeof ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.readText) === "function";
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)("ul", {
        ref: menuRef,
        className: menu,
        style: {
            left: x,
            top: y
        },
        "data-testid": "terminal-context-menu",
        role: "menu",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: menuItem,
                    role: "menuitem",
                    disabled: !hasSelection,
                    onPointerDown: (e)=>e.preventDefault(),
                    onClick: handleCopy,
                    children: "Copy"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: menuItem,
                    role: "menuitem",
                    onClick: handleSelectAll,
                    children: "Select All"
                })
            }),
            onPaste && hasPasteSupport && /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
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
function TerminalLinkMenu(param) {
    let { x, y, uri, onOpen, onCopy, onDismiss } = param;
    const menuRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
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
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)("ul", {
        ref: menuRef,
        className: menu,
        style: {
            left: x,
            top: y
        },
        "data-testid": "terminal-link-menu",
        role: "menu",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: menuItem,
                    role: "menuitem",
                    onClick: handleCopy,
                    children: "Copy link"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                role: "none",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
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
var terminalConfig = __webpack_require__(81073);
// EXTERNAL MODULE: ./src/lib/terminal/types.ts
var types = __webpack_require__(28990);
// EXTERNAL MODULE: ./src/lib/terminal/cursorRect.ts
var cursorRect = __webpack_require__(48469);
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
                var _t_clearTextureAtlas;
                (_t_clearTextureAtlas = t.clearTextureAtlas) === null || _t_clearTextureAtlas === void 0 ? void 0 : _t_clearTextureAtlas.call(t);
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
/** Registers `slot`, then evicts others until at most `max` remain. Prefers hidden victims. */ function claimWebglSlot(slot) {
    let max = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : MAX_WEBGL_CONTEXTS;
    slots.delete(slot);
    slots.add(slot);
    while(slots.size > max){
        const others = [
            ...slots
        ].filter((s)=>s !== slot);
        var _others_find;
        const victim = (_others_find = others.find((s)=>!s.isVisible())) !== null && _others_find !== void 0 ? _others_find : others[0];
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
    var _terminal__core__renderService, _terminal__core, _dims_css_cell, _dims_css;
    const dims = (_terminal__core = terminal._core) === null || _terminal__core === void 0 ? void 0 : (_terminal__core__renderService = _terminal__core._renderService) === null || _terminal__core__renderService === void 0 ? void 0 : _terminal__core__renderService.dimensions;
    if (!(dims === null || dims === void 0 ? void 0 : (_dims_css = dims.css) === null || _dims_css === void 0 ? void 0 : (_dims_css_cell = _dims_css.cell) === null || _dims_css_cell === void 0 ? void 0 : _dims_css_cell.width)) return null;
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
 */ const XtermTerminal = /*#__PURE__*/ (0,react.forwardRef)((param, ref)=>{
    let { onData, onResize, theme: themeProp, fontSize: fontSizeProp, scrollback: scrollbackProp, useConfig = false, scrollGesture, onScrollModeChange, isAltScreenActive, onAltScreenScrollUp } = param;
    var _terminalRef_current_getSelection, _terminalRef_current;
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
    var _ref;
    // Use props or config values
    const theme = (_ref = themeProp !== null && themeProp !== void 0 ? themeProp : config === null || config === void 0 ? void 0 : config.theme) !== null && _ref !== void 0 ? _ref : "dark";
    var _ref1;
    const fontSize = (_ref1 = fontSizeProp !== null && fontSizeProp !== void 0 ? fontSizeProp : config === null || config === void 0 ? void 0 : config.fontSize) !== null && _ref1 !== void 0 ? _ref1 : 14;
    var _ref2;
    const scrollback = (_ref2 = scrollbackProp !== null && scrollbackProp !== void 0 ? scrollbackProp : config === null || config === void 0 ? void 0 : config.scrollbackLines) !== null && _ref2 !== void 0 ? _ref2 : 0;
    var _config_fontFamily;
    // Mouse tracking mode is set at runtime by PTY escape sequences and read via terminal.modes.mouseTrackingMode.
    // It is not configurable via prop — the 'mouseTracking' ITerminalOptions field does not exist in xterm.js 6.
    const fontFamily = (_config_fontFamily = config === null || config === void 0 ? void 0 : config.fontFamily) !== null && _config_fontFamily !== void 0 ? _config_fontFamily : 'Menlo, Monaco, "Courier New", monospace';
    var _config_cursorStyle;
    const cursorStyle = (_config_cursorStyle = config === null || config === void 0 ? void 0 : config.cursorStyle) !== null && _config_cursorStyle !== void 0 ? _config_cursorStyle : "block";
    var _config_cursorBlink;
    const cursorBlink = (_config_cursorBlink = config === null || config === void 0 ? void 0 : config.cursorBlink) !== null && _config_cursorBlink !== void 0 ? _config_cursorBlink : true;
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
    const mouseModeOverrideRef = (0,react.useRef)(scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.mouseModeOverride);
    mouseModeOverrideRef.current = scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.mouseModeOverride;
    const onProgrammaticDataRef = (0,react.useRef)(scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.onProgrammaticData);
    const onProgrammaticData = scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.onProgrammaticData;
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
    var _scrollGesture_gestureScrollEnabled;
    const gestureScrollEnabled = (_scrollGesture_gestureScrollEnabled = scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.gestureScrollEnabled) !== null && _scrollGesture_gestureScrollEnabled !== void 0 ? _scrollGesture_gestureScrollEnabled : true;
    // Unified mobile gesture state machine (R4.3).
    // Replaces the conflicting useTouchScroll + useMobileTerminalGestures hooks:
    // having both register touchmove caused double-scroll and prevented selection.
    // Pass terminalRef (the RefObject itself, not .current) so gesture handlers always
    // read the live terminal instance — at render time terminalRef.current is null since
    // the terminal is created inside an effect (Bug 1 fix).
    useTerminalGestures({
        containerRef,
        terminalRef,
        onSendData: (0,react.useCallback)((data)=>{
            var _this;
            var _onProgrammaticDataRef_current;
            return (_this = (_onProgrammaticDataRef_current = onProgrammaticDataRef.current) !== null && _onProgrammaticDataRef_current !== void 0 ? _onProgrammaticDataRef_current : onDataRef.current) === null || _this === void 0 ? void 0 : _this(data);
        }, []),
        isAltScreenActive,
        onAltScreenScrollUp,
        override: scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.scrollOverride,
        gestureScrollEnabled,
        tuiScrollPolicy: scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.tuiScrollPolicy,
        connectionEpoch: scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.connectionEpoch,
        onScrollStart: scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.onScrollStart,
        onScrollGesture: scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.onScrollGesture,
        onPageKeysSent: scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.onPageKeysSent,
        onGestureActiveChange: scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.onGestureActiveChange,
        isInputBusy: scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.isInputBusy,
        mouseModeOverride: scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.mouseModeOverride,
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
        var _navigator_clipboard;
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
        if ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText) {
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
        var _navigator_clipboard;
        const terminal = terminalRef.current;
        if (!terminal) return;
        const text = terminal.getSelection();
        if (!text) return;
        if (copyButtonRef.current) {
            copyButtonRef.current.style.display = 'none';
        }
        if ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText) {
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
        var _navigator_clipboard;
        const terminal = terminalRef.current;
        if (!terminal) return;
        const text = terminal.getSelection();
        if (!text) return;
        if ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText) {
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
        var _terminalRef_current;
        (_terminalRef_current = terminalRef.current) === null || _terminalRef_current === void 0 ? void 0 : _terminalRef_current.selectAll();
        setContextMenuState(null);
    }, []);
    const handleMenuPaste = (0,react.useCallback)(()=>{
        navigator.clipboard.readText().then((text)=>{
            var _onDataRef_current;
            (_onDataRef_current = onDataRef.current) === null || _onDataRef_current === void 0 ? void 0 : _onDataRef_current.call(onDataRef, text);
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
        var _navigator_clipboard;
        if ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText) {
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
            } catch (e) {
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
        thumb.style.height = "".concat(thumbHeight, "px");
        thumb.style.transform = "translateY(".concat(thumbTop, "px)");
    }, []);
    // Initialize terminal on mount
    (0,react.useEffect)(()=>{
        // SSR guard
        if (false) {}
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
        const logFit = function(event) {
            let extra = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
            const el = containerRef.current;
            const vv = window.visualViewport;
            mobileDebug/* mobileDebug */.Uo.log(event, {
                vvHeight: vv === null || vv === void 0 ? void 0 : vv.height,
                vvOffsetTop: vv === null || vv === void 0 ? void 0 : vv.offsetTop,
                clientWidth: el === null || el === void 0 ? void 0 : el.clientWidth,
                clientHeight: el === null || el === void 0 ? void 0 : el.clientHeight,
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
            isVisible: ()=>{
                var _containerRef_current;
                var _containerRef_current_clientWidth;
                return document.visibilityState === "visible" && ((_containerRef_current_clientWidth = (_containerRef_current = containerRef.current) === null || _containerRef_current === void 0 ? void 0 : _containerRef_current.clientWidth) !== null && _containerRef_current_clientWidth !== void 0 ? _containerRef_current_clientWidth : 0) > 0;
            },
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
            var _webglAddonRef_current;
            webglFallbackTriggered = true; // eviction must also arm the latch, or a later mismatch loads a second CanvasAddon
            releaseWebglSlot(webglSlot);
            forgetWebglTerminal(terminal);
            (_webglAddonRef_current = webglAddonRef.current) === null || _webglAddonRef_current === void 0 ? void 0 : _webglAddonRef_current.dispose();
            webglAddonRef.current = null;
            try {
                terminal.loadAddon(new addon_canvas.CanvasAddon());
                rendererRef.current = "canvas";
                // Wait one RAF frame after the addon swap before fitting, per the
                // historical xterm.js #1416 crash precedent (measuring against a
                // not-yet-initialized renderer).
                postFallbackRafId = requestAnimationFrame(()=>{
                    var _fitAddonRef_current;
                    postFallbackRafId = null;
                    const proposed = (_fitAddonRef_current = fitAddonRef.current) === null || _fitAddonRef_current === void 0 ? void 0 : _fitAddonRef_current.proposeDimensions();
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
                console.error("[XtermTerminal] ⚠️ SIZING MISMATCH! Container width doesn't match cell width calculation");
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
        if ( true && localStorage.getItem("debug-terminal") === "true" && containerRef.current) {
            containerRef.current.__staplerSquadForceCanvasFallback = ()=>triggerCanvasFallback();
        }
        // true once on mount — pointer:coarse means a touch-primary device. Hoisted here
        // (rather than only at its other use-site below) so the WebLinksAddon handler can
        // read it too.
        const isTouchPrimary =  true && window.matchMedia('(pointer: coarse)').matches;
        // Create and load addons
        const fitAddon = new addon_fit/* FitAddon */.Y();
        // Default web-links behavior opens the link immediately on any click/tap, which on
        // touch devices (Android Chrome) means a bare tap navigates away before there's any
        // chance to copy the URL — the only way to get at a link with no public host name.
        // Gate that default behind a menu on touch devices; leave mouse click-to-open as-is.
        const webLinksAddon = new addon_web_links/* WebLinksAddon */.c(isTouchPrimary ? (event, uri)=>{
            event.preventDefault();
            setLinkMenuState({
                x: event.clientX,
                y: event.clientY,
                uri
            });
        } : undefined);
        const searchAddon = new addon_search/* SearchAddon */.n();
        const serializeAddon = new addon_serialize/* SerializeAddon */.V();
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
                    const { WebglAddon } = await __webpack_require__.e(/* import() */ 4926).then(__webpack_require__.bind(__webpack_require__, 84926));
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
            var // Custom key handler — a single handler for all shortcuts (attaching multiple replaces).
            // All key intercepts must be here. No cleanup needed; tied to terminal instance lifecycle.
            // iOS note: navigator.clipboard.writeText may fail in keydown because keydown events may
            // not carry the same user-gesture trust as pointer events. iOS users don't have Ctrl+C
            // hardware — they use the floating Copy button (onPointerDown path) instead.
            _terminal_attachCustomKeyEventHandler, _terminal_onScroll, _terminal_onWriteParsed, _terminal_buffer_onBufferChange, _terminal_buffer;
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
            (_terminal_attachCustomKeyEventHandler = terminal.attachCustomKeyEventHandler) === null || _terminal_attachCustomKeyEventHandler === void 0 ? void 0 : _terminal_attachCustomKeyEventHandler.call(terminal, (event)=>{
                if (event.type !== 'keydown') return true;
                const isCopyShortcut = (event.ctrlKey || event.metaKey) && event.key === 'c';
                const isSelectAllShortcut = (event.ctrlKey || event.metaKey) && event.key === 'a';
                if (isCopyShortcut && terminal.getSelection().length > 0) {
                    var _navigator_clipboard;
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
                    if ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText) {
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
                    var _terminal__core__renderService, _terminal__core, _dims_css;
                    // Double RAF ensures layout is stable before FitAddon measures dimensions
                    const containerEl = containerRef.current;
                    if (containerEl) {
                        const rect = containerEl.getBoundingClientRect();
                        console.log("[XtermTerminal] Container size before fit: ".concat(rect.width, "px \xd7 ").concat(rect.height, "px"));
                    }
                    // Log what FitAddon will see
                    const proposedDims = fitAddon.proposeDimensions();
                    console.log("[XtermTerminal] Proposed dimensions:", proposedDims);
                    // Check if cell dimensions are available (via private API for debugging)
                    const dims = (_terminal__core = terminal._core) === null || _terminal__core === void 0 ? void 0 : (_terminal__core__renderService = _terminal__core._renderService) === null || _terminal__core__renderService === void 0 ? void 0 : _terminal__core__renderService.dimensions;
                    if (dims === null || dims === void 0 ? void 0 : (_dims_css = dims.css) === null || _dims_css === void 0 ? void 0 : _dims_css.cell) {
                        console.log("[XtermTerminal] Cell dimensions: ".concat(dims.css.cell.width, "px \xd7 ").concat(dims.css.cell.height, "px"));
                    } else {
                        console.warn("[XtermTerminal] Cell dimensions not available yet!");
                    }
                    fitAddon.fit();
                    updateScrollbar(terminal);
                    console.log("[XtermTerminal] Initial fit complete: ".concat(terminal.cols, " cols \xd7 ").concat(terminal.rows, " rows"));
                    // Calculate actual pixels per column for verification (AC5 mismatch tracker)
                    if (containerEl) {
                        const mismatchInputs = extractCellMismatchInputs(terminal, containerEl);
                        if (mismatchInputs) {
                            console.log("[XtermTerminal] Actual pixels per column: ".concat(mismatchInputs.actualPxPerCol.toFixed(2), "px"));
                            console.log("[XtermTerminal] Expected pixels per column: ".concat(mismatchInputs.expectedPxPerCol.toFixed(2), "px"));
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
                var _onDataRef_current;
                if (mouseModeOverrideRef.current === 'none' && isMouseReport(data)) return;
                (_onDataRef_current = onDataRef.current) === null || _onDataRef_current === void 0 ? void 0 : _onDataRef_current.call(onDataRef, data);
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
                        btn.style.left = "".concat(rect.left + pos.end.x * cellW, "px");
                        btn.style.top = "".concat(rect.top + pos.end.y * cellH - 40, "px");
                        btn.style.display = 'block';
                        // Draggable selection handles — mobile only
                        if (isTouchPrimary) {
                            const sh = startHandleRef.current;
                            const eh = endHandleRef.current;
                            if (sh && eh) {
                                // Start handle: left edge of first selected character, below the row
                                sh.style.left = "".concat(rect.left + pos.start.x * cellW, "px");
                                sh.style.top = "".concat(rect.top + (pos.start.y + 1) * cellH, "px");
                                sh.style.display = 'block';
                                // End handle: right edge of last selected character, below the row
                                eh.style.left = "".concat(rect.left + (pos.end.x + 1) * cellW, "px");
                                eh.style.top = "".concat(rect.top + (pos.end.y + 1) * cellH, "px");
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
            const resizeDisposable = terminal.onResize((param)=>{
                let { cols, rows } = param;
                // Only trigger callback if size actually changed
                const lastSize = lastSizeRef.current;
                if (!lastSize || lastSize.cols !== cols || lastSize.rows !== rows) {
                    var _onResizeRef_current;
                    lastSizeRef.current = {
                        cols,
                        rows
                    };
                    (_onResizeRef_current = onResizeRef.current) === null || _onResizeRef_current === void 0 ? void 0 : _onResizeRef_current.call(onResizeRef, cols, rows);
                }
                updateScrollbar(terminal);
            });
            const scrollDisposable = (_terminal_onScroll = terminal.onScroll) === null || _terminal_onScroll === void 0 ? void 0 : _terminal_onScroll.call(terminal, ()=>updateScrollbar(terminal));
            // onWriteParsed fires after each chunk of data is rendered. When the user has
            // scrolled up into history and new output arrives, onScroll doesn't fire (the
            // viewport position didn't change) but the buffer grew, so thumb proportions
            // go stale. Updating here keeps the thumb correctly sized while streaming.
            // The same write hook re-reads the scroll mode: mouse-tracking changes only arrive via writes.
            let lastScrollMode = INITIAL_SCROLL_MODE;
            const reportScrollMode = ()=>{
                var _onScrollModeChangeRef_current;
                const next = readScrollMode(terminal);
                if (next.bufferType === lastScrollMode.bufferType && next.mouseTrackingMode === lastScrollMode.mouseTrackingMode) return;
                lastScrollMode = next;
                (_onScrollModeChangeRef_current = onScrollModeChangeRef.current) === null || _onScrollModeChangeRef_current === void 0 ? void 0 : _onScrollModeChangeRef_current.call(onScrollModeChangeRef, next);
            };
            const writeParsedDisposable = (_terminal_onWriteParsed = terminal.onWriteParsed) === null || _terminal_onWriteParsed === void 0 ? void 0 : _terminal_onWriteParsed.call(terminal, ()=>{
                updateScrollbar(terminal);
                reportScrollMode();
            });
            const bufferChangeDisposable = (_terminal_buffer = terminal.buffer) === null || _terminal_buffer === void 0 ? void 0 : (_terminal_buffer_onBufferChange = _terminal_buffer.onBufferChange) === null || _terminal_buffer_onBufferChange === void 0 ? void 0 : _terminal_buffer_onBufferChange.call(_terminal_buffer, reportScrollMode);
            // CRITICAL: Store refs BEFORE triggering callbacks
            // This ensures terminalRef is available when parent component calls getTerminal()
            terminalRef.current = terminal;
            // Lets the desktop toast deck keep clear of the cursor line.
            const unregisterCursorSource = (0,cursorRect/* registerTerminalCursorSource */.p5)(()=>{
                var _terminal_element;
                var _terminal_element_querySelector;
                return (0,cursorRect/* cursorCellRect */.Xn)(terminal, (_terminal_element_querySelector = (_terminal_element = terminal.element) === null || _terminal_element === void 0 ? void 0 : _terminal_element.querySelector(".xterm-screen")) !== null && _terminal_element_querySelector !== void 0 ? _terminal_element_querySelector : null);
            });
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
                        mag.style.left = "".concat(left, "px");
                        mag.style.top = "".concat(top, "px");
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
                        selectThrottled === null || selectThrottled === void 0 ? void 0 : selectThrottled(touch.clientX, touch.clientY);
                    };
                    const onTouchEnd = ()=>{
                        anchor = null;
                        geometry = null;
                        cancelSelectThrottle === null || cancelSelectThrottle === void 0 ? void 0 : cancelSelectThrottle();
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
                        cancelSelectThrottle === null || cancelSelectThrottle === void 0 ? void 0 : cancelSelectThrottle();
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
            const flushFitted = function() {
                let stale = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false;
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
                    var _terminalRef_current_scrollToBottom, _terminalRef_current;
                    fitAddonRef.current.fit();
                    console.log("[XtermTerminal] Sampler confirmed resize, fit applied: ".concat(terminalRef.current.cols, " cols \xd7 ").concat(terminalRef.current.rows, " rows"));
                    logFit("sampler-fit", {
                        forced: repaintRequested
                    });
                    if (anchorBottom) (_terminalRef_current_scrollToBottom = (_terminalRef_current = terminalRef.current).scrollToBottom) === null || _terminalRef_current_scrollToBottom === void 0 ? void 0 : _terminalRef_current_scrollToBottom.call(_terminalRef_current);
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
                    var _terminal_buffer;
                    repaintRequested = true;
                    repaintReason = opts.reason;
                    const buf = (_terminal_buffer = terminal.buffer) === null || _terminal_buffer === void 0 ? void 0 : _terminal_buffer.active;
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
                    console.warn("[XtermTerminal] refit: container (bottom ".concat(rect.bottom, "px) does not track visualViewport (bottom ").concat(vv.offsetTop + vv.height, "px)"));
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
                    onFitted === null || onFitted === void 0 ? void 0 : onFitted({
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
                    console.log("[XtermTerminal] Container resized to ".concat(width, "px \xd7 ").concat(height, "px (before fit)"));
                    console.log("[XtermTerminal] Terminal dimensions BEFORE fit: ".concat(terminalRef.current.cols, " cols \xd7 ").concat(terminalRef.current.rows, " rows"));
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
                    console.log("[XtermTerminal] Skipping fit: container collapsed to zero-size (".concat(width, "px \xd7 ").concat(height, "px)"));
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
            focusContainer === null || focusContainer === void 0 ? void 0 : focusContainer.addEventListener("focusin", onFocusIn);
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
                dataDisposable === null || dataDisposable === void 0 ? void 0 : dataDisposable.dispose();
                sgrDisposables.forEach((d)=>d.dispose());
                selectionDisposable === null || selectionDisposable === void 0 ? void 0 : selectionDisposable.dispose();
                resizeDisposable === null || resizeDisposable === void 0 ? void 0 : resizeDisposable.dispose();
                scrollDisposable === null || scrollDisposable === void 0 ? void 0 : scrollDisposable.dispose();
                writeParsedDisposable === null || writeParsedDisposable === void 0 ? void 0 : writeParsedDisposable.dispose();
                bufferChangeDisposable === null || bufferChangeDisposable === void 0 ? void 0 : bufferChangeDisposable.dispose();
                handleCleanupFns.forEach((fn)=>fn());
                forgetWebglTerminal(terminal);
                releaseWebglSlot(webglSlot);
                focusContainer === null || focusContainer === void 0 ? void 0 : focusContainer.removeEventListener("focusin", onFocusIn);
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
        if ( false || themeProp !== undefined) return;
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
                var _terminalRef_current;
                (_terminalRef_current = terminalRef.current) === null || _terminalRef_current === void 0 ? void 0 : _terminalRef_current.write(data);
            },
            writeln: (data)=>{
                var _terminalRef_current;
                (_terminalRef_current = terminalRef.current) === null || _terminalRef_current === void 0 ? void 0 : _terminalRef_current.writeln(data);
            },
            clear: ()=>{
                var _terminalRef_current;
                (_terminalRef_current = terminalRef.current) === null || _terminalRef_current === void 0 ? void 0 : _terminalRef_current.clear();
            },
            focus: ()=>{
                var _terminalRef_current;
                (_terminalRef_current = terminalRef.current) === null || _terminalRef_current === void 0 ? void 0 : _terminalRef_current.focus();
            },
            fit: ()=>{
                requestFitRef.current({
                    forceRepaint: true,
                    reason: "manual-resize"
                });
            },
            resize: (cols, rows)=>{
                var _terminalRef_current;
                (_terminalRef_current = terminalRef.current) === null || _terminalRef_current === void 0 ? void 0 : _terminalRef_current.resize(cols, rows);
            },
            refit: (opts)=>{
                var _opts_reason;
                requestFitRef.current({
                    forceRepaint: true,
                    reason: (_opts_reason = opts === null || opts === void 0 ? void 0 : opts.reason) !== null && _opts_reason !== void 0 ? _opts_reason : "manual-resize",
                    onFitted: opts === null || opts === void 0 ? void 0 : opts.onFitted
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
    var _terminalRef_current_getSelection_length;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        "data-context": "terminal",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                ref: containerRef,
                className: terminal,
                "data-gesture-scroll": gestureScrollEnabled ? "on" : "off"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                ref: scrollTrackRef,
                className: scrollTrack,
                style: {
                    display: 'none'
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    ref: scrollThumbRef,
                    className: scrollThumb
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                "aria-label": "Copy terminal scrollback to clipboard",
                className: scrollbackCopyButton,
                onPointerDown: handleCopyScrollbackPointerDown,
                children: "\uD83D\uDCCB Copy all"
            }),
            typeof document !== 'undefined' && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        ref: copyButtonRef,
                        "aria-label": "Copy selected text",
                        className: floatingCopyButton,
                        style: {
                            display: 'none'
                        },
                        onPointerDown: handleCopyButtonPointerDown,
                        children: "Copy"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        ref: toastRef,
                        className: copiedToast,
                        "aria-live": "polite",
                        style: {
                            display: 'none'
                        }
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        ref: startHandleRef,
                        className: selectionHandle,
                        "aria-hidden": "true",
                        style: {
                            display: 'none'
                        }
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        ref: endHandleRef,
                        className: selectionHandle,
                        "aria-hidden": "true",
                        style: {
                            display: 'none'
                        }
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
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
            contextMenuState && /*#__PURE__*/ (0,jsx_runtime.jsx)(TerminalContextMenu, {
                x: contextMenuState.x,
                y: contextMenuState.y,
                hasSelection: ((_terminalRef_current_getSelection_length = (_terminalRef_current = terminalRef.current) === null || _terminalRef_current === void 0 ? void 0 : (_terminalRef_current_getSelection = _terminalRef_current.getSelection()) === null || _terminalRef_current_getSelection === void 0 ? void 0 : _terminalRef_current_getSelection.length) !== null && _terminalRef_current_getSelection_length !== void 0 ? _terminalRef_current_getSelection_length : 0) > 0,
                onCopy: handleMenuCopy,
                onSelectAll: handleMenuSelectAll,
                onPaste: handleMenuPaste,
                onDismiss: handleContextMenuDismiss
            }),
            linkMenuState && /*#__PURE__*/ (0,jsx_runtime.jsx)(TerminalLinkMenu, {
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


/***/ })

}]);
//# sourceMappingURL=3517.e5af2d2fc15f1366.js.map