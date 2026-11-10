"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[4298],{

/***/ 146:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   d$: () => (/* binding */ writeDraft),
/* harmony export */   kF: () => (/* binding */ readDraft),
/* harmony export */   nr: () => (/* binding */ clearDraft)
/* harmony export */ });
const PREFIX = "ssq:draft:";
const memory = new Map();
/**
 * Unsent text (an answer, a Deny reason) kept per item id for the life of the tab.
 * `sessionStorage` first; a memory map backs it when storage throws (private mode, quota).
 */ function readDraft(id) {
    try {
        const stored = window.sessionStorage.getItem(PREFIX + id);
        if (stored !== null) return stored;
    } catch (e) {
    // fall through to memory
    }
    var _memory_get;
    return (_memory_get = memory.get(id)) !== null && _memory_get !== void 0 ? _memory_get : "";
}
function writeDraft(id, text) {
    if (text === "") {
        clearDraft(id);
        return;
    }
    memory.set(id, text);
    try {
        window.sessionStorage.setItem(PREFIX + id, text);
    } catch (e) {
    // memory copy above is the fallback
    }
}
/** Call on send or Dismiss. */ function clearDraft(id) {
    memory.delete(id);
    try {
        window.sessionStorage.removeItem(PREFIX + id);
    } catch (e) {
    // nothing stored to remove
    }
}


/***/ }),

/***/ 3335:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   CC: () => (/* binding */ formatTimestampShort),
/* harmony export */   Ey: () => (/* binding */ formatTimestamp),
/* harmony export */   dB: () => (/* binding */ getUserTimezone),
/* harmony export */   fw: () => (/* binding */ formatRelativeTime),
/* harmony export */   jX: () => (/* binding */ TIME_RANGE_PRESETS)
/* harmony export */ });
/* unused harmony export formatTime */
/**
 * Utility functions for date and time formatting with timezone support
 */ /**
 * Format a timestamp for display with full date, time, and timezone
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM PST")
 */ function formatTimestamp(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleString(undefined, {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit',
        timeZoneName: 'short'
    });
}
/**
 * Format a timestamp for display with date and time (no timezone)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM")
 */ function formatTimestampShort(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleString(undefined, {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit'
    });
}
/**
 * Format a timestamp for display with just time (no date)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "3:45:30 PM")
 */ function formatTime(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleTimeString(undefined, {
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit'
    });
}
/**
 * Format a timestamp as relative time (e.g., "5m ago", "2h ago")
 * @param timestamp - Unix timestamp in milliseconds
 * @returns Relative time string
 */ function formatRelativeTime(timestamp) {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    if (diffMins < 1) return "Just now";
    if (diffMins < 60) return "".concat(diffMins, "m ago");
    if (diffHours < 24) return "".concat(diffHours, "h ago");
    if (diffDays < 7) return "".concat(diffDays, "d ago");
    return date.toLocaleDateString();
}
/**
 * Get the user's timezone abbreviation (e.g., "PST", "EST", "UTC")
 */ function getUserTimezone() {
    const date = new Date();
    const timeZoneName = date.toLocaleString(undefined, {
        timeZoneName: 'short'
    }).split(' ').pop();
    return timeZoneName || Intl.DateTimeFormat().resolvedOptions().timeZone;
}
/**
 * Standard time range presets for log filtering
 */ const TIME_RANGE_PRESETS = [
    {
        label: 'Last 5 minutes',
        value: '5m',
        getRange: ()=>({
                start: new Date(Date.now() - 5 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 15 minutes',
        value: '15m',
        getRange: ()=>({
                start: new Date(Date.now() - 15 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 1 hour',
        value: '1h',
        getRange: ()=>({
                start: new Date(Date.now() - 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 4 hours',
        value: '4h',
        getRange: ()=>({
                start: new Date(Date.now() - 4 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 24 hours',
        value: '24h',
        getRange: ()=>({
                start: new Date(Date.now() - 24 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 7 days',
        value: '7d',
        getRange: ()=>({
                start: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'All time',
        value: 'all',
        getRange: ()=>({
                start: new Date(0),
                end: new Date()
            })
    }
];


/***/ }),

/***/ 7202:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   l: () => (/* binding */ copyToClipboard)
/* harmony export */ });
// execCommand fallback covers plain-HTTP LAN access, where navigator.clipboard is undefined.
async function copyToClipboard(text) {
    var _navigator_clipboard;
    if ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText) {
        try {
            await navigator.clipboard.writeText(text);
            return true;
        } catch (e) {
        // fall through to execCommand fallback
        }
    }
    const el = document.createElement("textarea");
    el.value = text;
    el.style.position = "fixed";
    el.style.opacity = "0";
    document.body.appendChild(el);
    el.select();
    let ok = false;
    try {
        ok = document.execCommand("copy");
    } catch (e) {
        ok = false;
    }
    document.body.removeChild(el);
    return ok;
}


/***/ }),

/***/ 26198:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   V: () => (/* binding */ GuidanceDeliveryLine)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(77448);
/* harmony import */ var _components_ui_notification_actions_ActionButton__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(62590);
/* harmony import */ var _components_ui_notification_actions_SubSteps__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(43943);
/* harmony import */ var _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(28453);
/* __next_internal_client_entry_do_not_use__ GuidanceDeliveryLine auto */ 





const CONFIRM_COPY = {
    retryAnyway: {
        prompt: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .COPY */ .sF.retryAnywayPrompt,
        confirmLabel: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .COPY */ .sF.retryAnywayConfirm
    },
    markHandled: {
        prompt: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .DELIVERY_COPY */ .ZP.markHandledPrompt,
        confirmLabel: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .DELIVERY_COPY */ .ZP.markHandledConfirm
    },
    dismissConfirm: {
        prompt: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .DELIVERY_COPY */ .ZP.dismissUnfinishedPrompt,
        confirmLabel: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .DELIVERY_COPY */ .ZP.dismissUnfinishedConfirm
    }
};
const LABEL = {
    retry: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .COPY */ .sF.retry,
    retryAnyway: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .COPY */ .sF.retryAnyway,
    markHandled: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .DELIVERY_COPY */ .ZP.markHandled,
    dismiss: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .COPY */ .sF.dismiss,
    dismissConfirm: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .COPY */ .sF.dismiss
};
/**
 * The one delivery-state line of a question card: plain text (never a live region; the
 * single global polite region announces the short result), an Unconfirmed chip on rows 3 and
 * 6, and the row's actions. Retry anyway, Mark as handled and Dismiss (aborted) confirm inline.
 */ function GuidanceDeliveryLine(param) {
    let { view, onAction, openControl, minHeight, busy = false } = param;
    const [confirming, setConfirming] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const confirmRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(null);
    const press = (a)=>{
        if (a === "retryAnyway" || a === "markHandled" || a === "dismissConfirm") setConfirming(a);
        else if (a !== "open") onAction(a);
    };
    if (confirming) {
        const copy = CONFIRM_COPY[confirming];
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
            className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_5__/* .deliveryLine */ .dJ,
            "data-testid": "delivery-line",
            "data-delivery-row": view.row,
            children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_components_ui_notification_actions_SubSteps__WEBPACK_IMPORTED_MODULE_4__/* .ConfirmStep */ .Hb, {
                ...copy,
                backLabel: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .COPY */ .sF.back,
                busy: busy,
                minHeight: minHeight,
                confirmRef: confirmRef,
                testId: "delivery-confirm",
                onBack: ()=>setConfirming(null),
                onConfirm: ()=>{
                    setConfirming(null);
                    onAction(confirming);
                }
            })
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_5__/* .deliveryLine */ .dJ,
        "data-testid": "delivery-line",
        "data-delivery-row": view.row,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_5__/* .deliveryText */ ._P,
                "data-testid": "delivery-text",
                children: view.text
            }),
            view.chip && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_5__/* .chip */ .jw,
                "data-testid": "delivery-chip",
                children: view.chip
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_5__/* .optionList */ .UX,
                children: view.actions.map((a)=>a === "open" ? /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                        children: openControl
                    }, "open") : /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_components_ui_notification_actions_ActionButton__WEBPACK_IMPORTED_MODULE_3__/* .ActionButton */ .r, {
                        id: "delivery-".concat(a),
                        label: LABEL[a],
                        minHeight: minHeight,
                        disabled: busy,
                        onClick: ()=>press(a)
                    }, a))
            })
        ]
    });
}


/***/ }),

/***/ 26946:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Q: () => (/* binding */ visuallyHidden)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(47439);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fa11y_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll85amk4YnAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlcjogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var visuallyHidden = '_9ji8bp0';

/***/ }),

/***/ 27924:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  XC: () => (/* binding */ CardEngagementContext),
  jJ: () => (/* binding */ EngagementContext),
  j9: () => (/* binding */ useCardEngagement),
  ZB: () => (/* binding */ useEngagedCard),
  Tq: () => (/* binding */ useEngagement),
  PR: () => (/* binding */ useEngagementTracker)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/interactionTiming.ts
var interactionTiming = __webpack_require__(29901);
;// ./src/lib/hooks/engagementTracker.ts

const blank = ()=>({
        focus: false,
        pointer: false,
        touchUntil: 0,
        subStep: false,
        hold: false,
        resolved: false,
        expansion: "none",
        armed: false,
        touched: false
    });
/**
 * Framework-free core of `design/ux.md` 7.1: which cards are engaged, whether the Needs-you
 * list is frozen, and whether a card is digit-armed. `frozen` is true while any card is engaged
 * and for `graceMs` after the last stops being so.
 */ class EngagementTracker {
    card(id) {
        let c = this.cards.get(id);
        if (!c) {
            c = blank();
            this.cards.set(id, c);
        }
        return c;
    }
    patch(id, p) {
        Object.assign(this.card(id), p);
        this.recompute();
    }
    /** Engaged by the 7.1 definition: a resolved card is held only by pointer and touch. */ isEngaged(id) {
        let now = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : Date.now();
        const c = this.cards.get(id);
        if (!c) return false;
        if (c.pointer || c.touchUntil > now) return true;
        if (c.resolved) return false;
        return c.focus || c.subStep || c.hold || c.expansion === "operator";
    }
    anyEngaged() {
        const now = Date.now();
        for (const id of this.cards.keys())if (this.isEngaged(id, now)) return true;
        return false;
    }
    recompute() {
        if (this.anyEngaged()) {
            if (this.releaseTimer) clearTimeout(this.releaseTimer);
            this.releaseTimer = null;
            if (!this.frozen) this.setFrozen(true);
            return;
        }
        if (this.frozen && !this.releaseTimer) {
            this.releaseTimer = setTimeout(()=>{
                this.releaseTimer = null;
                if (this.anyEngaged()) return;
                this.setFrozen(false);
            }, (0,interactionTiming/* getTiming */.Hl)().graceMs);
        }
    }
    setFrozen(v) {
        this.frozen = v;
        this.listeners.forEach((fn)=>fn());
    }
    /** The operator's focus landed in the card. Program-placed focus never engages. */ focusIn(id) {
        if (this.programmaticDepth > 0) return;
        const c = this.card(id);
        c.focus = true;
        // A digit's own focus move is the operator's keystroke: it engages but arms nothing new.
        if (this.digitDepth === 0) c.armed = true;
        this.recompute();
    }
    focusOut(id) {
        if (this.cards.has(id)) this.patch(id, {
            focus: false
        });
    }
    pointer(id, over) {
        this.patch(id, {
            pointer: over
        });
    }
    touch(id) {
        const hold = (0,interactionTiming/* getTiming */.Hl)().touchHoldMs;
        this.card(id).touchUntil = Date.now() + hold;
        const prev = this.touchTimers.get(id);
        if (prev) clearTimeout(prev);
        this.touchTimers.set(id, setTimeout(()=>{
            this.touchTimers.delete(id);
            this.recompute();
        }, hold));
        this.recompute();
    }
    expansion(id) {
        var _this_cards_get;
        var _this_cards_get_expansion;
        return (_this_cards_get_expansion = (_this_cards_get = this.cards.get(id)) === null || _this_cards_get === void 0 ? void 0 : _this_cards_get.expansion) !== null && _this_cards_get_expansion !== void 0 ? _this_cards_get_expansion : "none";
    }
    /** The program's default: expand the first item on open. Ignored once the operator touched it. */ preExpand(id) {
        const c = this.card(id);
        if (!c.touched && c.expansion === "none") this.patch(id, {
            expansion: "pre"
        });
    }
    /** Enter, Space or a tap on the header. Pre-expanded collapses; the next toggle opens and arms it. */ toggleExpanded(id) {
        const c = this.card(id);
        c.touched = true;
        if (c.expansion === "none") {
            c.expansion = "operator";
            c.armed = true;
        } else {
            c.expansion = "none";
        }
        this.recompute();
        return c.expansion;
    }
    collapse(id) {
        if (this.cards.has(id)) this.patch(id, {
            expansion: "none",
            touched: true
        });
    }
    /** A row was inserted, removed or reordered, or focus moved by the program. */ noteListChange() {
        this.lastProgrammaticAt = Date.now();
        this.lastLayoutChangeAt = this.lastProgrammaticAt;
    }
    /** Moves focus without engaging: the program's focus never counts. */ focusProgrammatically(el) {
        if (!el) return;
        this.lastProgrammaticAt = Date.now();
        this.programmaticDepth += 1;
        try {
            el.focus({
                preventScroll: true
            });
        } finally{
            this.programmaticDepth -= 1;
        }
    }
    /** A digit key's focus move: engages the card, restarts no clock, disarms nothing. */ focusByDigit(el) {
        if (!el) return;
        this.digitDepth += 1;
        try {
            el.focus({
                preventScroll: true
            });
        } finally{
            this.digitDepth -= 1;
        }
    }
    isDigitArmed(id) {
        let now = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : Date.now();
        const c = this.cards.get(id);
        if (!c || c.resolved || !c.armed) return false;
        const held = c.focus || c.touchUntil > now || c.expansion === "operator";
        return held && now - this.lastProgrammaticAt >= (0,interactionTiming/* getTiming */.Hl)().graceMs;
    }
    /** A control mounted or moved at `since` ignores activation until the layout has settled. */ isLayoutSettled(since) {
        let now = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : Date.now();
        return now - Math.max(since, this.lastLayoutChangeAt) >= (0,interactionTiming/* getTiming */.Hl)().settleMs;
    }
    /** The card's row left the list. */ forget(id) {
        if (!this.cards.delete(id)) return;
        const t = this.touchTimers.get(id);
        if (t) clearTimeout(t);
        this.touchTimers.delete(id);
        this.recompute();
    }
    /** The panel closed: nothing is engaged any more and the list unfreezes at once. */ reset() {
        this.closeListeners.forEach((fn)=>fn());
        this.cards.clear();
        this.touchTimers.forEach((t)=>clearTimeout(t));
        this.touchTimers.clear();
        if (this.releaseTimer) clearTimeout(this.releaseTimer);
        this.releaseTimer = null;
        if (this.frozen) this.setFrozen(false);
    }
    constructor(){
        this.cards = new Map();
        this.listeners = new Set();
        this.closeListeners = new Set();
        this.frozen = false;
        this.releaseTimer = null;
        this.touchTimers = new Map();
        this.lastProgrammaticAt = Number.NEGATIVE_INFINITY;
        this.lastLayoutChangeAt = Number.NEGATIVE_INFINITY;
        this.programmaticDepth = 0;
        this.digitDepth = 0;
        this.subscribe = (fn)=>{
            this.listeners.add(fn);
            return ()=>this.listeners.delete(fn);
        };
        this.isFrozen = ()=>this.frozen;
        this.setSubStep = (id, v)=>this.patch(id, {
                subStep: v
            });
        this.setHold = (id, v)=>this.patch(id, {
                hold: v
            });
        this.setResolved = (id, v)=>this.patch(id, {
                resolved: v
            });
        /** Runs `fn` when the panel closes (holds cancel then). */ this.onClose = (fn)=>{
            this.closeListeners.add(fn);
            return ()=>this.closeListeners.delete(fn);
        };
    }
}

;// ./src/lib/hooks/useEngagedCard.ts
/* __next_internal_client_entry_do_not_use__ EngagementContext,useEngagementTracker,useEngagement,CardEngagementContext,useCardEngagement,useEngagedCard auto */ 

/** Provided by the panel; without one (page, stories, unit tests) every card is simply never engaged. */ const EngagementContext = (0,react.createContext)(null);
/** The panel owns one tracker; `frozen` is true while a card is engaged and for 750 ms after. */ function useEngagementTracker() {
    const ref = (0,react.useRef)(null);
    if (!ref.current) ref.current = new EngagementTracker();
    const tracker = ref.current;
    const frozen = (0,react.useSyncExternalStore)(tracker.subscribe, tracker.isFrozen, ()=>false);
    return {
        tracker,
        frozen
    };
}
const useEngagement = ()=>(0,react.useContext)(EngagementContext);
const NOOP_CARD = {
    setSubStep: ()=>{},
    setHold: ()=>{},
    setResolved: ()=>{}
};
const CardEngagementContext = (0,react.createContext)(NOOP_CARD);
const useCardEngagement = ()=>(0,react.useContext)(CardEngagementContext);
/** Element handlers that feed one card's engagement conditions (a), (b) and (c) into the tracker. */ function useEngagedCard(cardId) {
    const tracker = useEngagement();
    const onFocusCapture = (0,react.useCallback)(()=>tracker === null || tracker === void 0 ? void 0 : tracker.focusIn(cardId), [
        tracker,
        cardId
    ]);
    const onBlurCapture = (0,react.useCallback)((e)=>{
        if (!e.currentTarget.contains(e.relatedTarget)) tracker === null || tracker === void 0 ? void 0 : tracker.focusOut(cardId);
    }, [
        tracker,
        cardId
    ]);
    const onPointerEnter = (0,react.useCallback)((e)=>e.pointerType !== "touch" && (tracker === null || tracker === void 0 ? void 0 : tracker.pointer(cardId, true)), [
        tracker,
        cardId
    ]);
    const onPointerLeave = (0,react.useCallback)((e)=>e.pointerType !== "touch" && (tracker === null || tracker === void 0 ? void 0 : tracker.pointer(cardId, false)), [
        tracker,
        cardId
    ]);
    const onTouchStart = (0,react.useCallback)((_e)=>tracker === null || tracker === void 0 ? void 0 : tracker.touch(cardId), [
        tracker,
        cardId
    ]);
    const card = (0,react.useMemo)(()=>tracker ? {
            setSubStep: (v)=>tracker.setSubStep(cardId, v),
            setHold: (v)=>tracker.setHold(cardId, v),
            setResolved: (v)=>tracker.setResolved(cardId, v)
        } : NOOP_CARD, [
        tracker,
        cardId
    ]);
    const handlers = {
        onFocusCapture,
        onBlurCapture,
        onPointerEnter,
        onPointerLeave,
        onTouchStart
    };
    return {
        tracker,
        handlers,
        card
    };
}


/***/ }),

/***/ 28453:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $i: () => (/* binding */ fieldError),
/* harmony export */   AM: () => (/* binding */ groupLabel),
/* harmony export */   Bd: () => (/* binding */ selectField),
/* harmony export */   J1: () => (/* binding */ submitRow),
/* harmony export */   Os: () => (/* binding */ group),
/* harmony export */   Rm: () => (/* binding */ toggleButton),
/* harmony export */   UX: () => (/* binding */ optionList),
/* harmony export */   Xo: () => (/* binding */ otherField),
/* harmony export */   Xy: () => (/* binding */ otherRow),
/* harmony export */   Zd: () => (/* binding */ form),
/* harmony export */   _P: () => (/* binding */ deliveryText),
/* harmony export */   dJ: () => (/* binding */ deliveryLine),
/* harmony export */   dw: () => (/* binding */ helper),
/* harmony export */   gC: () => (/* binding */ textList),
/* harmony export */   h_: () => (/* binding */ description),
/* harmony export */   ij: () => (/* binding */ choiceInput),
/* harmony export */   jw: () => (/* binding */ chip),
/* harmony export */   vU: () => (/* binding */ textOption),
/* harmony export */   we: () => (/* binding */ choiceRow)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fguidance_2FGuidanceQuestionForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VWy46bMBTd5yuskSolCyJgZvJgVKlSN_2A7qqqcuwLWAEb2U5CqPLvFTYQIIFEk2y6SWTwPT7n3IeZR7EnC_GivxOEKFNZgo8BChPIPybI_DuUSSCaCR4gIpJdyss3Ec4CtMdy6jh_vDUUvixe3dnH5DSZW0TvCYh_MSvfbISkIAPEBYdynWIZMR4gt1xkmFLGo2qVMu4cGNWxWTdkfENmJC4UXDuKFXAhisya9wdgUawDtHBNDBGJkP39rtku9iDDRBycgyxVYX48xCChxeh1xB4bVP6O_dJAvV2HwgmLuMM0pCpABLgGOW5z6V1cSXx7y_JxXzazhz14N8SrdPmuPbFmUK_NIyqWjG_7OV2Y6GGCeJig17ZveVkbn1Lt_W3U1bNQ26DrvmRvYU26lrpzgc_fJaTnRnIkpmynLlgsO83mZTlSImG0tw3sLky2kRQ7Tvswy1t1keK8blHPdb_05OHrhTxcsVdLvIHbWLcSyAPkXQ6H_9zIRiZ5eJLcMpI_b5S3GqKBhye2yj1jJzTnWckKkjCoZozGUo8fjgfvgqZG_Cwfu8SGSmZxoxiOw7KLu6otb5dM9CTH7x308eD16yQQ6osjbSYfLLhjWzH7hSXDTiZBKaBfX7TcwctvQ2vY9HRY_aKd06s7UnP8txQow0gRCcAR5hRNW_NvtV5n_cyw6HwWjEntdDYXdW_fzhikwsiEYjZaaUa2Rxu9EVqLtMpCq3BXWV4_uzWTTmdh01BIAtRaoAKEiWZ76EgKKzpds75jvsfqJ_S6x34kU11mP1gUJ2UTWtIVbvO0hu4f3Ak7TU7_AJbpNuABCwAA_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(46085);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fguidance_2FGuidanceQuestionForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VWy46bMBTd5yuskSolCyJgZvJgVKlSN_2A7qqqcuwLWAEb2U5CqPLvFTYQIIFEk2y6SWTwPT7n3IeZR7EnC_GivxOEKFNZgo8BChPIPybI_DuUSSCaCR4gIpJdyss3Ec4CtMdy6jh_vDUUvixe3dnH5DSZW0TvCYh_MSvfbISkIAPEBYdynWIZMR4gt1xkmFLGo2qVMu4cGNWxWTdkfENmJC4UXDuKFXAhisya9wdgUawDtHBNDBGJkP39rtku9iDDRBycgyxVYX48xCChxeh1xB4bVP6O_dJAvV2HwgmLuMM0pCpABLgGOW5z6V1cSXx7y_JxXzazhz14N8SrdPmuPbFmUK_NIyqWjG_7OV2Y6GGCeJig17ZveVkbn1Lt_W3U1bNQ26DrvmRvYU26lrpzgc_fJaTnRnIkpmynLlgsO83mZTlSImG0tw3sLky2kRQ7Tvswy1t1keK8blHPdb_05OHrhTxcsVdLvIHbWLcSyAPkXQ6H_9zIRiZ5eJLcMpI_b5S3GqKBhye2yj1jJzTnWckKkjCoZozGUo8fjgfvgqZG_Cwfu8SGSmZxoxiOw7KLu6otb5dM9CTH7x308eD16yQQ6osjbSYfLLhjWzH7hSXDTiZBKaBfX7TcwctvQ2vY9HRY_aKd06s7UnP8txQow0gRCcAR5hRNW_NvtV5n_cyw6HwWjEntdDYXdW_fzhikwsiEYjZaaUa2Rxu9EVqLtMpCq3BXWV4_uzWTTmdh01BIAtRaoAKEiWZ76EgKKzpds75jvsfqJ_S6x34kU11mP1gUJ2UTWtIVbvO0hu4f3Ak7TU7_AJbpNuABCwAA_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fguidance_2FGuidanceQuestionForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VWy46bMBTd5yuskSolCyJgZvJgVKlSN_2A7qqqcuwLWAEb2U5CqPLvFTYQIIFEk2y6SWTwPT7n3IeZR7EnC_GivxOEKFNZgo8BChPIPybI_DuUSSCaCR4gIpJdyss3Ec4CtMdy6jh_vDUUvixe3dnH5DSZW0TvCYh_MSvfbISkIAPEBYdynWIZMR4gt1xkmFLGo2qVMu4cGNWxWTdkfENmJC4UXDuKFXAhisya9wdgUawDtHBNDBGJkP39rtku9iDDRBycgyxVYX48xCChxeh1xB4bVP6O_dJAvV2HwgmLuMM0pCpABLgGOW5z6V1cSXx7y_JxXzazhz14N8SrdPmuPbFmUK_NIyqWjG_7OV2Y6GGCeJig17ZveVkbn1Lt_W3U1bNQ26DrvmRvYU26lrpzgc_fJaTnRnIkpmynLlgsO83mZTlSImG0tw3sLky2kRQ7Tvswy1t1keK8blHPdb_05OHrhTxcsVdLvIHbWLcSyAPkXQ6H_9zIRiZ5eJLcMpI_b5S3GqKBhye2yj1jJzTnWckKkjCoZozGUo8fjgfvgqZG_Cwfu8SGSmZxoxiOw7KLu6otb5dM9CTH7x308eD16yQQ6osjbSYfLLhjWzH7hSXDTiZBKaBfX7TcwctvQ2vY9HRY_aKd06s7UnP8txQow0gRCcAR5hRNW_NvtV5n_cyw6HwWjEntdDYXdW_fzhikwsiEYjZaaUa2Rxu9EVqLtMpCq3BXWV4_uzWTTmdh01BIAtRaoAKEiWZ76EgKKzpds75jvsfqJ_S6x34kU11mP1gUJ2UTWtIVbvO0hu4f3Ak7TU7_AJbpNuABCwAA_22_7D__WEBPACK_IMPORTED_MODULE_1__);


var chip = 'gh1rzof';
var choiceInput = 'gh1rzo5';
var choiceRow = 'gh1rzo4';
var deliveryLine = 'gh1rzod';
var deliveryText = 'gh1rzoe';
var description = 'gh1rzo6';
var fieldError = 'gh1rzo7';
var form = 'gh1rzo0';
var group = 'gh1rzo1';
var groupLabel = 'gh1rzo2';
var helper = 'gh1rzo8';
var optionList = 'gh1rzo3';
var otherField = 'gh1rzob';
var otherRow = 'gh1rzoa';
var selectField = 'gh1rzo9';
var submitRow = 'gh1rzoc';
var textList = 'gh1rzoh';
var textOption = 'gh1rzog';
var toggleButton = 'gh1rzoi';

/***/ }),

/***/ 29901:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Hl: () => (/* binding */ getTiming)
/* harmony export */ });
/* unused harmony exports DEFAULT_TIMING, setTimingForTests */
/**
 * Interaction-safety windows (design/ux.md 7.1 and 2.4), in one place so tests can pin them.
 * `jest.setup.js` zeroes the settle window and the commit delay for suites that click right
 * after render; the interaction-safety suites restore the real values.
 */ const DEFAULT_TIMING = {
    graceMs: 750,
    touchHoldMs: 5000,
    settleMs: 300,
    flingGuardMs: 120,
    commitDelayMs: 3000
};
let timing = DEFAULT_TIMING;
const getTiming = ()=>timing;
/** Test seam: pass nothing to restore the defaults. */ function setTimingForTests() {
    let over = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    timing = {
        ...DEFAULT_TIMING,
        ...over
    };
}


/***/ }),

/***/ 33378:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   BM: () => (/* binding */ formatAnswers),
/* harmony export */   hI: () => (/* binding */ GuidanceQuestionForm)
/* harmony export */ });
/* unused harmony export SELECT_THRESHOLD */
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(77448);
/* harmony import */ var _lib_utils_sessionDrafts__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(146);
/* harmony import */ var _components_ui_notification_actions_ActionButton__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(62590);
/* harmony import */ var _components_ui_notification_actions_ActionRow_css__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(48241);
/* harmony import */ var _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(28453);
/* __next_internal_client_entry_do_not_use__ SELECT_THRESHOLD,formatAnswers,GuidanceQuestionForm auto */ 
// +feature: session:guidance-structured






/** Past this many options a single question becomes a native select (design/ux.md 3.1). */ const SELECT_THRESHOLD = 8;
/**
 * The answer text a durable-record route stores: the labels of a single question, or one
 * "question: labels" line per question for several.
 */ function formatAnswers(answers) {
    if (answers.length === 1) return answers[0].selected.join(", ");
    return answers.map((a)=>"".concat(a.header || a.question, ": ").concat(a.selected.join(", "))).join("\n");
}
function loadPicks(id, count) {
    try {
        var _parsed_picks;
        const parsed = JSON.parse((0,_lib_utils_sessionDrafts__WEBPACK_IMPORTED_MODULE_3__/* .readDraft */ .kF)("guidance:".concat(id)) || "null");
        if ((parsed === null || parsed === void 0 ? void 0 : (_parsed_picks = parsed.picks) === null || _parsed_picks === void 0 ? void 0 : _parsed_picks.length) === count) return parsed.picks;
    } catch (e) {
    // a corrupt draft is no draft
    }
    return Array.from({
        length: count
    }, ()=>[]);
}
const isSingleCard = (qs)=>qs.length === 1 && !qs[0].multiSelect && qs[0].options.length > 0 && qs[0].options.length <= SELECT_THRESHOLD;
const optionText = (o)=>o.description ? "".concat(o.label, " - ").concat(o.description) : o.label;
const answerOf = (q, selected)=>({
        question: q.text,
        header: q.header,
        selected: selected.filter(Boolean)
    });
function Footer(param) {
    let { s } = param;
    var _s_delivery;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: [
            s.disabledReason && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                id: s.reasonId,
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .helper */ .dw,
                children: s.disabledReason
            }),
            (_s_delivery = s.delivery) !== null && _s_delivery !== void 0 ? _s_delivery : /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .helper */ .dw,
                children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.noDeliveryYet
            })
        ]
    });
}
/** Read-only shape: text list, no control (unmeasured harness, archived, canary, S1c fail). */ function ReadOnlyForm(param) {
    let { id, questions, unrenderable, openControl, delivery } = param;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .form */ .Zd,
        "data-testid": "guidance-form-".concat(id),
        children: [
            questions.map((q, qi)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                    className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .group */ .Os,
                    "data-testid": "question-text",
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                            className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .groupLabel */ .AM,
                            children: q.text
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("ul", {
                            className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .textList */ .gC,
                            children: q.options.map((o)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("li", {
                                    className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .textOption */ .vU,
                                    children: optionText(o)
                                }, o.label))
                        })
                    ]
                }, qi)),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .helper */ .dw,
                children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.answerInSession
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .optionList */ .UX,
                children: [
                    unrenderable && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                        className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .textOption */ .vU,
                        children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.viewFullPrompt
                    }),
                    !delivery && openControl
                ]
            }),
            delivery
        ]
    });
}
/** One question of up to eight options: option buttons in a labelled group; one activation sends. */ function SingleQuestionCard(param) {
    let { s } = param;
    const uid = (0,react__WEBPACK_IMPORTED_MODULE_1__.useId)();
    const q = s.questions[0];
    const [other, setOther] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)("");
    const labelId = "".concat(uid, "-q0");
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("form", {
        className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .form */ .Zd,
        "aria-label": _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.answerFor(q.text),
        "data-testid": "guidance-form-".concat(s.id),
        onSubmit: (e)=>e.preventDefault(),
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                id: labelId,
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .groupLabel */ .AM,
                children: q.text
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
                role: "group",
                "aria-labelledby": labelId,
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .optionList */ .UX,
                children: q.options.map((o, oi)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_components_ui_notification_actions_ActionButton__WEBPACK_IMPORTED_MODULE_4__/* .ActionButton */ .r, {
                        id: "option-".concat(oi),
                        label: optionText(o),
                        variant: _components_ui_notification_actions_ActionRow_css__WEBPACK_IMPORTED_MODULE_5__/* .secondaryVariant */ .OL,
                        disabled: s.blocked,
                        reasonId: s.reasonId,
                        minHeight: s.minHeight,
                        option: true,
                        onClick: ()=>s.send([
                                answerOf(q, [
                                    o.label
                                ])
                            ])
                    }, o.label))
            }),
            s.canTypeText && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .otherRow */ .Xy,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("label", {
                        htmlFor: "".concat(uid, "-other"),
                        children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.otherLabel
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("input", {
                        id: "".concat(uid, "-other"),
                        type: "text",
                        className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .otherField */ .Xo,
                        value: other,
                        "data-testid": "guidance-other",
                        enterKeyHint: "send",
                        autoComplete: "off",
                        onFocus: (e)=>{
                            var _e_currentTarget_scrollIntoView, _e_currentTarget;
                            return (_e_currentTarget_scrollIntoView = (_e_currentTarget = e.currentTarget).scrollIntoView) === null || _e_currentTarget_scrollIntoView === void 0 ? void 0 : _e_currentTarget_scrollIntoView.call(_e_currentTarget, {
                                block: "nearest"
                            });
                        },
                        onChange: (e)=>setOther(e.target.value),
                        onKeyDown: (e)=>{
                            if (e.key === "Enter" && !e.nativeEvent.isComposing && other.trim()) {
                                e.preventDefault();
                                s.send([
                                    answerOf(q, [
                                        other.trim()
                                    ])
                                ]);
                            }
                        }
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_components_ui_notification_actions_ActionButton__WEBPACK_IMPORTED_MODULE_4__/* .ActionButton */ .r, {
                        id: "other-send",
                        label: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.send,
                        minHeight: s.minHeight,
                        disabled: s.blocked || other.trim() === "",
                        reasonId: s.reasonId,
                        onClick: ()=>s.send([
                                answerOf(q, [
                                    other.trim()
                                ])
                            ])
                    })
                ]
            }),
            !s.delivery && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .optionList */ .UX,
                children: s.openControl
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(Footer, {
                s: s
            })
        ]
    });
}
function Choices(param) {
    let { q, picked, s, onToggle, name } = param;
    if (!q.multiSelect && q.options.length > SELECT_THRESHOLD) {
        var _picked_;
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("select", {
            className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .selectField */ .Bd,
            "aria-label": q.text,
            value: (_picked_ = picked[0]) !== null && _picked_ !== void 0 ? _picked_ : "",
            "aria-disabled": s.blocked || undefined,
            "aria-describedby": s.reasonId,
            onChange: (e)=>onToggle(e.target.value),
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("option", {
                    value: ""
                }),
                q.options.map((o)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("option", {
                        value: o.label,
                        children: o.label
                    }, o.label))
            ]
        });
    }
    if (!q.multiSelect) {
        // Never a radiogroup: arrow keys on radios would change the selection while someone browses (design/ux.md 3.1).
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
            className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .optionList */ .UX,
            children: q.options.map((o)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                    type: "button",
                    className: "".concat(_components_ui_notification_actions_ActionRow_css__WEBPACK_IMPORTED_MODULE_5__/* .actionButton */ .hP, " ").concat(_components_ui_notification_actions_ActionRow_css__WEBPACK_IMPORTED_MODULE_5__/* .secondaryVariant */ .OL, " ").concat(_GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .toggleButton */ .Rm),
                    "aria-pressed": picked.includes(o.label),
                    "aria-disabled": s.blocked || undefined,
                    "aria-describedby": s.reasonId,
                    style: {
                        minHeight: s.minHeight
                    },
                    onClick: ()=>onToggle(o.label),
                    children: optionText(o)
                }, o.label))
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: q.options.map((o)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("label", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .choiceRow */ .we,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("input", {
                        className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .choiceInput */ .ij,
                        type: "checkbox",
                        name: name,
                        checked: picked.includes(o.label),
                        "aria-disabled": s.blocked || undefined,
                        "aria-describedby": s.reasonId,
                        onChange: ()=>onToggle(o.label)
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("span", {
                        children: [
                            o.label,
                            o.description && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("span", {
                                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .description */ .h_,
                                children: [
                                    " - ",
                                    o.description
                                ]
                            })
                        ]
                    })
                ]
            }, o.label))
    });
}
function QuestionGroup(param) {
    let { q, qi, total, picked, other, error, s, groupRef, onToggle, onOther } = param;
    const uid = (0,react__WEBPACK_IMPORTED_MODULE_1__.useId)();
    const errId = "".concat(uid, "-err");
    const legend = total > 1 ? "".concat(_lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.questionOf(qi + 1, total), ": ").concat(q.text) : q.text;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("fieldset", {
        ref: groupRef,
        className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .group */ .Os,
        "data-testid": "question-group-".concat(qi),
        "aria-invalid": error ? true : undefined,
        "aria-describedby": error ? errId : undefined,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("legend", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .groupLabel */ .AM,
                children: [
                    legend,
                    " ",
                    q.multiSelect && _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.chooseAny
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(Choices, {
                q: q,
                picked: picked,
                s: s,
                onToggle: onToggle,
                name: "".concat(uid, "-choice")
            }),
            s.canTypeText && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .otherRow */ .Xy,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("label", {
                        htmlFor: "".concat(uid, "-other"),
                        children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.otherLabel
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("input", {
                        id: "".concat(uid, "-other"),
                        type: "text",
                        className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .otherField */ .Xo,
                        value: other,
                        autoComplete: "off",
                        onChange: (e)=>onOther(e.target.value),
                        onKeyDown: (e)=>{
                            // Cmd/Ctrl+Enter sends (UX-AC21); browsers differ on implicit submission with a modifier held.
                            if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && !e.nativeEvent.isComposing) {
                                var _e_currentTarget_form;
                                e.preventDefault();
                                (_e_currentTarget_form = e.currentTarget.form) === null || _e_currentTarget_form === void 0 ? void 0 : _e_currentTarget_form.requestSubmit();
                            }
                        }
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                id: errId,
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .fieldError */ .$i,
                "data-testid": "question-error-".concat(qi),
                children: error
            })
        ]
    });
}
/** Multi-select, multi-question and long-list shapes: stacked groups and one explicit Submit. */ function SelectionForm(param) {
    let { s } = param;
    const { id, questions } = s;
    const [picks, setPicks] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(()=>loadPicks(id, questions.length));
    const [others, setOthers] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(()=>questions.map(()=>""));
    const [errors, setErrors] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({});
    const groupRefs = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)([]);
    const persist = (next)=>{
        setPicks(next);
        (0,_lib_utils_sessionDrafts__WEBPACK_IMPORTED_MODULE_3__/* .writeDraft */ .d$)("guidance:".concat(id), next.some((p)=>p.length > 0) ? JSON.stringify({
            picks: next
        }) : "");
    };
    const collect = ()=>questions.map((q, qi)=>answerOf(q, [
                ...picks[qi],
                ...s.canTypeText ? [
                    others[qi].trim()
                ] : []
            ]));
    const firstEmpty = collect().findIndex((a)=>a.selected.length === 0);
    const toggle = (qi, label)=>{
        if (s.blocked) return;
        const cur = picks[qi];
        const next = questions[qi].multiSelect ? cur.includes(label) ? cur.filter((l)=>l !== label) : [
            ...cur,
            label
        ] : [
            label
        ];
        persist(picks.map((p, i)=>i === qi ? next : p));
        setErrors((param)=>{
            let { [qi]: _cleared, ...rest } = param;
            return rest;
        });
    };
    const submit = (e)=>{
        e.preventDefault();
        const answers = collect();
        const flagged = {};
        answers.forEach((a, i)=>{
            if (a.selected.length === 0) flagged[i] = _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.chooseAtLeastOne;
        });
        setErrors(flagged);
        const first = answers.findIndex((a)=>a.selected.length === 0);
        if (first >= 0) {
            var _this;
            const group = groupRefs.current[first];
            var _group_querySelector;
            (_this = (_group_querySelector = group === null || group === void 0 ? void 0 : group.querySelector("input, select, button")) !== null && _group_querySelector !== void 0 ? _group_querySelector : group) === null || _this === void 0 ? void 0 : _this.focus();
            return;
        }
        s.send(answers);
    };
    const clear = ()=>{
        if (s.blocked) return;
        persist(questions.map(()=>[]));
        setOthers(questions.map(()=>""));
        setErrors({});
    };
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("form", {
        className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .form */ .Zd,
        "aria-label": _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.answerFor(questions[0].text),
        "data-testid": "guidance-form-".concat(id),
        onSubmit: submit,
        children: [
            questions.map((q, qi)=>/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(QuestionGroup, {
                    q: q,
                    qi: qi,
                    total: questions.length,
                    picked: picks[qi],
                    other: others[qi],
                    error: errors[qi],
                    s: s,
                    groupRef: (el)=>{
                        groupRefs.current[qi] = el;
                    },
                    onToggle: (label)=>toggle(qi, label),
                    onOther: (text)=>setOthers(others.map((v, i)=>i === qi ? text : v))
                }, qi)),
            questions.some((q)=>q.multiSelect) && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .helper */ .dw,
                "data-testid": "selected-count",
                children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.selectedCount(picks.reduce((n, p, i)=>n + (questions[i].multiSelect ? p.length : 0), 0))
            }),
            firstEmpty >= 0 && questions.length > 1 && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("p", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .helper */ .dw,
                children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.answerToContinue(firstEmpty + 1)
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                className: _GuidanceQuestionForm_css__WEBPACK_IMPORTED_MODULE_6__/* .submitRow */ .J1,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                        type: "submit",
                        className: "".concat(_components_ui_notification_actions_ActionRow_css__WEBPACK_IMPORTED_MODULE_5__/* .actionButton */ .hP, " ").concat(_components_ui_notification_actions_ActionRow_css__WEBPACK_IMPORTED_MODULE_5__/* .secondaryVariant */ .OL),
                        "data-testid": "submit-answers",
                        "aria-disabled": s.blocked || firstEmpty >= 0 ? true : undefined,
                        "aria-describedby": s.reasonId,
                        style: {
                            minHeight: s.minHeight
                        },
                        children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.submitAnswers
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
                        type: "button",
                        className: _components_ui_notification_actions_ActionRow_css__WEBPACK_IMPORTED_MODULE_5__/* .actionButton */ .hP,
                        "data-testid": "clear-answers",
                        "aria-disabled": s.blocked || undefined,
                        style: {
                            minHeight: s.minHeight
                        },
                        onClick: clear,
                        children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_2__/* .QUESTION_FORM_COPY */ .Uv.clear
                    }),
                    !s.delivery && s.openControl
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(Footer, {
                s: s
            })
        ]
    });
}
/**
 * The question UI shared by GuidanceRequestPanel and ActionRow (design/ux.md 3). A single
 * select question is a group of option buttons where one activation sends; multi-select and
 * multi-question are stacked groups with one explicit Submit; more than eight options is a
 * native select. "Other" free text exists only on routes that can carry text.
 */ function GuidanceQuestionForm(props) {
    const { id, questions, mode, canTypeText = false, unrenderable = false, locked = false, disabledReason, onSubmit, openControl, delivery, minHeight } = props;
    const uid = (0,react__WEBPACK_IMPORTED_MODULE_1__.useId)();
    const blocked = locked || !!disabledReason;
    // One activation sends: a second tap before the parent locks the card must not send again.
    const sentRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(false);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        if (!locked) sentRef.current = false;
    }, [
        locked
    ]);
    if (mode === "readonly" || questions.length === 0) {
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(ReadOnlyForm, {
            id: id,
            questions: questions,
            unrenderable: unrenderable,
            openControl: openControl,
            delivery: delivery
        });
    }
    const shared = {
        id,
        questions,
        canTypeText,
        blocked,
        disabledReason,
        reasonId: disabledReason ? "".concat(uid, "-reason") : undefined,
        minHeight,
        openControl,
        delivery,
        send: (answers)=>{
            if (blocked || sentRef.current) return;
            sentRef.current = true;
            (0,_lib_utils_sessionDrafts__WEBPACK_IMPORTED_MODULE_3__/* .clearDraft */ .nr)("guidance:".concat(id));
            onSubmit(answers);
        }
    };
    return isSingleCard(questions) ? /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(SingleQuestionCard, {
        s: shared
    }) : /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(SelectionForm, {
        s: shared
    });
}


/***/ }),

/***/ 35610:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   eY: () => (/* binding */ snapshotOf),
/* harmony export */   ho: () => (/* binding */ announceTransition),
/* harmony export */   sH: () => (/* binding */ deliveryView)
/* harmony export */ });
/* unused harmony export displayAnswer */
/* harmony import */ var _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(77448);

const view = function(row, text, actions) {
    let extra = arguments.length > 3 && arguments[3] !== void 0 ? arguments[3] : {};
    return {
        row,
        text,
        actions,
        needsYou: false,
        locked: true,
        ...extra
    };
};
const OPEN = [
    "open"
];
function unconfirmed(indeterminate) {
    return indeterminate ? view(6, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.indeterminate, [
        "open",
        "retryAnyway"
    ], {
        chip: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.unconfirmedChip,
        needsYou: true
    }) : view(3, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.unconfirmed, [
        "open",
        "markHandled"
    ], {
        chip: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.unconfirmedChip,
        needsYou: true
    });
}
function cancelled(snap) {
    if (snap.cancelReason === "dialog_replaced") return view(12, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.staleReplaced, OPEN);
    if (snap.cancelReason === "answered_in_terminal" || snap.cancelReason === "dialog_gone") return view(11, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.staleGone, OPEN);
    return view(10, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.sessionEnded, [
        "open",
        "dismiss"
    ]);
}
/** A multi-question answer is stored as a JSON list of label lists; show it as "Redis; A, B". */ function displayAnswer(answer) {
    if (!answer.startsWith("[[")) return answer;
    try {
        const picks = JSON.parse(answer);
        if (Array.isArray(picks) && picks.every((p)=>Array.isArray(p) && p.every((l)=>typeof l === "string"))) {
            return picks.map((p)=>p.join(", ")).join("; ");
        }
    } catch (e) {
    // not a sequence: show it as typed
    }
    return answer;
}
function answeredRow(snap, client) {
    switch(snap.deliveryState){
        case "acknowledged":
            return view(4, (0,_lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .answered */ .oQ)(displayAnswer(snap.answer)), []);
        case "sent":
            if (snap.sessionEnded) return view(17, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.sentSessionEnded, OPEN);
            return snap.unacknowledged || client.indeterminate ? unconfirmed(!!client.indeterminate) : view(2, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.sentWaiting, []);
        case "delivery_failed":
            return snap.sessionEnded ? view(17, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.sentSessionEnded, OPEN) : view(16, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.stillShowing, [
                "open",
                "retryAnyway"
            ], {
                needsYou: true
            });
        case "saved_for_later":
            return snap.deliveryRoute === "read_only" ? view(9, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.savedArchived, OPEN) : view(8, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.savedStopped, OPEN);
        default:
            return view(4, (0,_lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .answered */ .oQ)(displayAnswer(snap.answer)), []);
    }
}
const abortedPartial = ()=>view(14, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.aborted, [
        "open",
        "dismissConfirm"
    ], {
        needsYou: true
    });
function pendingRow(snap, client) {
    if (snap.deliveryState === "aborted_partial") return abortedPartial(); // survives a reload: the durable row says so
    // UX-AC27: the archived copy shows before the operator acts, and the card stays answerable.
    if (snap.sessionArchived && !snap.deliveryState) return view(9, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.savedArchived, OPEN, {
        locked: false
    });
    if (snap.deliveryState === "pending") return view(1, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.sending, [], {
        locked: false
    });
    if (snap.deliveryState === "delivery_failed") return notSent(client);
    if (snap.deliveryState === "stale") return client.staleReason === "dialog_replaced" ? view(12, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.staleReplaced, OPEN) : view(11, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.staleGone, OPEN);
    return null;
}
const notSent = (client)=>view(5, client.notSent === "human_active" ? _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.notSentHumanActive : _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.notSent, [
        "retry",
        "open"
    ], {
        needsYou: true,
        locked: false
    });
function clientRow(client) {
    if (client.sending) return view(1, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.sending, [], {
        locked: false
    });
    if (client.clientFailure) return view(7, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.clientFailure, [
        "retry",
        "dismiss"
    ], {
        locked: false
    });
    if (client.notSent) return notSent(client);
    if (client.indeterminate) return unconfirmed(true);
    if (client.staleReason === "session_ended") return view(10, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.sessionEnded, [
        "open",
        "dismiss"
    ]);
    if (client.staleReason === "dialog_replaced") return view(12, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.staleReplaced, OPEN);
    if (client.staleReason === "dialog_gone") return view(11, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.staleGone, OPEN);
    if (client.applied) return view(2, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.sentWaiting, []);
    return null;
}
/**
 * The line for a card, or null when nothing was sent and nothing is wrong ("Delivery: (none
 * until sent)"). Precedence: facts about another tab or the listener, then the durable row,
 * then this browser's in-flight state.
 */ function deliveryView(snap) {
    let client = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    // Row 13; with no value from the other tab the card can only say it was answered (design/ux.md 3.3).
    if (client.elsewhere !== undefined) return view(13, client.elsewhere ? (0,_lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .answeredElsewhere */ .ul)(client.elsewhere) : _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .DELIVERY_COPY */ .ZP.alreadyAnsweredReload, []);
    if (client.abortedPartial) return abortedPartial();
    if (client.desktopOnly) return view(15, _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .COPY */ .sF.desktopOnly, OPEN, {
        locked: false
    });
    if (client.sending) return clientRow(client);
    if (!snap) return clientRow(client);
    if (snap.status === "cancelled") return cancelled(snap);
    if (snap.status === "answered") return answeredRow(snap, client);
    var _pendingRow;
    return (_pendingRow = pendingRow(snap, client)) !== null && _pendingRow !== void 0 ? _pendingRow : clientRow(client);
}
/**
 * The one polite announcement for a card moving from one row to another, or undefined
 * (design/ux.md 11.1). `prevRow` is undefined on a card's first render (nothing to announce) and
 * null while it showed no line. A late acknowledgement and a session ending on a sent answer are silent.
 */ function announceTransition(prevRow, next) {
    if (prevRow === undefined || !next || prevRow === next.row) return undefined;
    switch(next.row){
        case 2:
            return _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .ANNOUNCE */ .eV.answerSent;
        case 5:
            return _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .ANNOUNCE */ .eV.notDelivered;
        case 16:
            return _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .ANNOUNCE */ .eV.stillShowing;
        case 3:
        case 6:
            return _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .ANNOUNCE */ .eV.notConfirmed;
        case 14:
            return _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .ANNOUNCE */ .eV.partlySent;
        case 11:
            return _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_0__/* .ANNOUNCE */ .eV.handledInSession; // UX-AC26: flipped by a terminal answer or a vanished dialog
        default:
            return undefined;
    }
}
/** A snapshot from the plain wire object, tolerating fields an older server omits. */ function snapshotOf(row) {
    if (!row) return null;
    var _row_status, _row_deliveryRoute, _row_deliveryState, _row_unacknowledged, _row_sessionEnded, _row_sessionArchived, _row_cancelReason, _row_answer;
    return {
        status: (_row_status = row.status) !== null && _row_status !== void 0 ? _row_status : "pending",
        deliveryRoute: (_row_deliveryRoute = row.deliveryRoute) !== null && _row_deliveryRoute !== void 0 ? _row_deliveryRoute : "",
        deliveryState: (_row_deliveryState = row.deliveryState) !== null && _row_deliveryState !== void 0 ? _row_deliveryState : "",
        unacknowledged: (_row_unacknowledged = row.unacknowledged) !== null && _row_unacknowledged !== void 0 ? _row_unacknowledged : false,
        sessionEnded: (_row_sessionEnded = row.sessionEnded) !== null && _row_sessionEnded !== void 0 ? _row_sessionEnded : false,
        sessionArchived: (_row_sessionArchived = row.sessionArchived) !== null && _row_sessionArchived !== void 0 ? _row_sessionArchived : false,
        cancelReason: (_row_cancelReason = row.cancelReason) !== null && _row_cancelReason !== void 0 ? _row_cancelReason : "",
        answer: (_row_answer = row.answer) !== null && _row_answer !== void 0 ? _row_answer : ""
    };
}


/***/ }),

/***/ 43661:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Rw: () => (/* binding */ useFeatureFlagValue),
/* harmony export */   jG: () => (/* binding */ flagOffMessage)
/* harmony export */ });
/* unused harmony exports resetFeatureFlagValueCache, resolveFlagValue */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(23575);
/* __next_internal_client_entry_do_not_use__ flagOffMessage,resetFeatureFlagValueCache,resolveFlagValue,useFeatureFlagValue auto */ 



/** Operator copy for the FLAG_OFF action outcome (UX-AC55, UX-AC57); Story 2.5's ActionRow maps the outcome to it. */ const flagOffMessage = "This action is turned off. Open session.";
const CACHE_TTL_MS = 30000;
const FETCH_TIMEOUT_MS = 10000;
let cached = null;
let inFlight = null;
/** Clears the module cache (tests, and after a flag flip a caller knows about). */ function resetFeatureFlagValueCache() {
    cached = null;
    inFlight = null;
}
async function loadFlags(now) {
    if (cached && now() - cached.at < CACHE_TTL_MS) return cached.flags;
    if (!inFlight) {
        const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_2__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
        inFlight = client// Shared across hook instances, so a per-component abort would cancel others' read; bound it by time instead.
        .getFeatureFlags({}, {
            signal: AbortSignal.timeout(FETCH_TIMEOUT_MS)
        }).then((res)=>{
            cached = {
                at: now(),
                flags: res.flags
            };
            return res.flags;
        }).finally(()=>{
            inFlight = null;
        });
    }
    return inFlight;
}
/**
 * Advisory read of one decision point: the scope's explicit value, else the
 * global value. Dependencies (render before answer, ...) are applied by the
 * server on every write; this only chooses which row to draw.
 */ function resolveFlagValue(flags, name, scope) {
    const flag = flags.find((f)=>f.name === name);
    if (!flag) return undefined;
    const override = scope ? flag.scopes.find((s)=>s.scope === scope) : undefined;
    return override ? override.enabled : flag.enabled;
}
/**
 * Reads a feature flag (optionally at a scope) through GetFeatureFlags, with a
 * short shared cache. Until loaded, and when the first read fails, it returns
 * `fallback` (default OFF), so a failed read never enables a new path. It re-reads
 * every cache TTL so a kill switch reaches an open page; a failed re-read keeps the
 * last known value rather than reverting to `fallback`.
 * `enabled=false` skips the read (the row has nothing the flag could change).
 */ function useFeatureFlagValue(name, scope) {
    let fallback = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false, enabled = arguments.length > 3 && arguments[3] !== void 0 ? arguments[3] : true;
    const [value, setValue] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(fallback);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!enabled) return;
        let live = true;
        const read = ()=>loadFlags(Date.now).then((flags)=>{
                var _resolveFlagValue;
                if (live) setValue((_resolveFlagValue = resolveFlagValue(flags, name, scope)) !== null && _resolveFlagValue !== void 0 ? _resolveFlagValue : fallback);
            }).catch(()=>{});
        void read();
        const timer = setInterval(()=>void read(), CACHE_TTL_MS);
        return ()=>{
            live = false;
            clearInterval(timer);
        };
    }, [
        name,
        scope,
        fallback,
        enabled
    ]);
    return value;
}


/***/ }),

/***/ 43943:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Hb: () => (/* binding */ ConfirmStep),
/* harmony export */   JQ: () => (/* binding */ DenyStep),
/* harmony export */   py: () => (/* binding */ createRuleStepProps)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(77448);
/* harmony import */ var _ActionButton__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(62590);
/* harmony import */ var _ActionRow_css__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(48241);
/* __next_internal_client_entry_do_not_use__ ConfirmStep,DenyStep,createRuleStepProps auto */ 



/** An inline two-step ("Merge PR #901?"); focus lands on the confirm button. */ function ConfirmStep(param) {
    let { prompt, confirmLabel, backLabel, onConfirm, onBack, confirmRef, busy, minHeight, testId = "confirm-step", confirmId = "confirm" } = param;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        className: _ActionRow_css__WEBPACK_IMPORTED_MODULE_3__/* .subStep */ .b6,
        "data-testid": testId,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                children: prompt
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                className: _ActionRow_css__WEBPACK_IMPORTED_MODULE_3__/* .stepActions */ .k6,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_ActionButton__WEBPACK_IMPORTED_MODULE_2__/* .ActionButton */ .r, {
                        id: confirmId,
                        label: confirmLabel,
                        onClick: onConfirm,
                        buttonRef: confirmRef,
                        variant: _ActionRow_css__WEBPACK_IMPORTED_MODULE_3__/* .approveVariant */ .TE,
                        disabled: busy,
                        minHeight: minHeight
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_ActionButton__WEBPACK_IMPORTED_MODULE_2__/* .ActionButton */ .r, {
                        id: "back",
                        label: backLabel,
                        onClick: onBack,
                        minHeight: minHeight
                    })
                ]
            })
        ]
    });
}
/** Optional one-line reason; Back keeps the draft. */ function DenyStep(param) {
    let { fieldId, reason, onReason, onSend, onBack, fieldRef, busy, minHeight } = param;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
        className: _ActionRow_css__WEBPACK_IMPORTED_MODULE_3__/* .subStep */ .b6,
        "data-testid": "deny-step",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("label", {
                htmlFor: fieldId,
                children: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_1__/* .COPY */ .sF.denyReasonLabel
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("input", {
                id: fieldId,
                ref: fieldRef,
                type: "text",
                className: _ActionRow_css__WEBPACK_IMPORTED_MODULE_3__/* .field */ .ZZ,
                value: reason,
                "data-testid": "deny-reason",
                // Mobile keyboard matrix (ux.md 11.4): Enter sends a single-line reason.
                enterKeyHint: "send",
                autoCapitalize: "sentences",
                autoComplete: "off",
                spellCheck: true,
                onFocus: (e)=>{
                    var _e_currentTarget_scrollIntoView, _e_currentTarget;
                    return (_e_currentTarget_scrollIntoView = (_e_currentTarget = e.currentTarget).scrollIntoView) === null || _e_currentTarget_scrollIntoView === void 0 ? void 0 : _e_currentTarget_scrollIntoView.call(_e_currentTarget, {
                        block: "nearest"
                    });
                },
                onKeyDown: (e)=>{
                    if (e.key === "Enter" && !e.nativeEvent.isComposing && !busy) {
                        e.preventDefault();
                        onSend();
                    }
                },
                onChange: (e)=>onReason(e.target.value)
            }),
            /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("div", {
                className: "".concat(_ActionRow_css__WEBPACK_IMPORTED_MODULE_3__/* .stepActions */ .k6, " ").concat(_ActionRow_css__WEBPACK_IMPORTED_MODULE_3__/* .stickyBar */ .LI),
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_ActionButton__WEBPACK_IMPORTED_MODULE_2__/* .ActionButton */ .r, {
                        id: "send-denial",
                        label: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_1__/* .COPY */ .sF.sendDenial,
                        onClick: onSend,
                        variant: _ActionRow_css__WEBPACK_IMPORTED_MODULE_3__/* .denyVariant */ .Av,
                        disabled: busy,
                        minHeight: minHeight
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(_ActionButton__WEBPACK_IMPORTED_MODULE_2__/* .ActionButton */ .r, {
                        id: "back",
                        label: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_1__/* .COPY */ .sF.back,
                        onClick: onBack,
                        minHeight: minHeight
                    })
                ]
            })
        ]
    });
}
const createRuleStepProps = (scope)=>({
        prompt: (0,_lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_1__/* .createRulePrompt */ .XN)(scope),
        confirmLabel: _lib_copy_notificationCopy__WEBPACK_IMPORTED_MODULE_1__/* .COPY */ .sF.createRule,
        testId: "create-rule-step",
        confirmId: "create-rule-confirm"
    });


/***/ }),

/***/ 48241:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Av: () => (/* binding */ denyVariant),
/* harmony export */   LI: () => (/* binding */ stickyBar),
/* harmony export */   Lu: () => (/* binding */ commandMeta),
/* harmony export */   MK: () => (/* binding */ menu),
/* harmony export */   OL: () => (/* binding */ secondaryVariant),
/* harmony export */   TE: () => (/* binding */ approveVariant),
/* harmony export */   Uo: () => (/* binding */ buttons),
/* harmony export */   ZZ: () => (/* binding */ field),
/* harmony export */   b6: () => (/* binding */ subStep),
/* harmony export */   h3: () => (/* binding */ commandScroll),
/* harmony export */   h5: () => (/* binding */ status),
/* harmony export */   hP: () => (/* binding */ actionButton),
/* harmony export */   jt: () => (/* binding */ commandText),
/* harmony export */   k6: () => (/* binding */ stepActions),
/* harmony export */   s8: () => (/* binding */ commandRegion),
/* harmony export */   zr: () => (/* binding */ root)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2Fnotification_actions_2FActionRow_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA51V227bMAx9z1cIBQY0DwpsN1cVA_a2H9jbMBSyRCdCbcmQ5NjO0H8ffEslL3bavtiARB0ekofk6iXcm4sMtwH6u0CIC5OntCYoSaF6XqD2j7nQwKxQkiCm0iKTzc2R5gTt89aqFNyeCAqD4Nvz4m2xGkDDGdBSNwDNtzmiqThKLCxkhiAG0oL2nTioUYuaCYlPII4nS9BT1PHIKedCHgkKVtFGQ4aC1a75t06VtNiICzS3_zDyzsseaBsEzVmsNAeNNeWiMARtO3RWaKM0QbkSA7_YstejVoXkBFlNpcmpBmlba5U2xmeqHzF_CQ9wifQlWL7DExTmFTIqFXxkBUs_3t9UC4q5MDROgX9_sLqAhz9tFlROmbB1E9Nm45KUymKapqoE7oE9tc9ukisccnjSwsFaT2OFwV2w1sRB20yjZXfBMh9r_3U1Oyi7FsUTd8OiamTUyqxnFKtqpLCwl4wr0vX6P5EOGrytt4_IxJegf79bzurQCXTfdRTVRyEJCuba5XYpW0_qDDpJVdm3NpV1eQINnqfDlwqzHhWGTvBNaCbSeswu6oQ9FVF5EhawySkDgnINeJhJk_HMJZ0vP1FjJ6S4D6m66iWMuodXGjVBtLDKf0YSxQqDz8KIOIVuJhQ2FRIIiiaE07VTb4ZVkhiwrbUHzW6X6u6kHmV6d1c5jk_dXk4SV9_qn9u995sVzmM4cMrzh0xPzLggiLDNIBEVHL02JS_HzX7wyGvli3waJXOpWJUGmM1WHZ6fn_V0ZFqIPTm4kc9_mja3R5h_qzr6_vBJT1crozoiBor2GvdvY6VtSrrO9cp3D6vhrN7Y_3tPYuPidIMeLcYDEGUWXGGUf7mluhVmz81rX9BZQeW7r7x7657N_zJ_ANlndPMSQkAAA_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21411);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2Fnotification_actions_2FActionRow_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA51V227bMAx9z1cIBQY0DwpsN1cVA_a2H9jbMBSyRCdCbcmQ5NjO0H8ffEslL3bavtiARB0ekofk6iXcm4sMtwH6u0CIC5OntCYoSaF6XqD2j7nQwKxQkiCm0iKTzc2R5gTt89aqFNyeCAqD4Nvz4m2xGkDDGdBSNwDNtzmiqThKLCxkhiAG0oL2nTioUYuaCYlPII4nS9BT1PHIKedCHgkKVtFGQ4aC1a75t06VtNiICzS3_zDyzsseaBsEzVmsNAeNNeWiMARtO3RWaKM0QbkSA7_YstejVoXkBFlNpcmpBmlba5U2xmeqHzF_CQ9wifQlWL7DExTmFTIqFXxkBUs_3t9UC4q5MDROgX9_sLqAhz9tFlROmbB1E9Nm45KUymKapqoE7oE9tc9ukisccnjSwsFaT2OFwV2w1sRB20yjZXfBMh9r_3U1Oyi7FsUTd8OiamTUyqxnFKtqpLCwl4wr0vX6P5EOGrytt4_IxJegf79bzurQCXTfdRTVRyEJCuba5XYpW0_qDDpJVdm3NpV1eQINnqfDlwqzHhWGTvBNaCbSeswu6oQ9FVF5EhawySkDgnINeJhJk_HMJZ0vP1FjJ6S4D6m66iWMuodXGjVBtLDKf0YSxQqDz8KIOIVuJhQ2FRIIiiaE07VTb4ZVkhiwrbUHzW6X6u6kHmV6d1c5jk_dXk4SV9_qn9u995sVzmM4cMrzh0xPzLggiLDNIBEVHL02JS_HzX7wyGvli3waJXOpWJUGmM1WHZ6fn_V0ZFqIPTm4kc9_mja3R5h_qzr6_vBJT1crozoiBor2GvdvY6VtSrrO9cp3D6vhrN7Y_3tPYuPidIMeLcYDEGUWXGGUf7mluhVmz81rX9BZQeW7r7x7657N_zJ_ANlndPMSQkAAA_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2Fnotification_actions_2FActionRow_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA51V227bMAx9z1cIBQY0DwpsN1cVA_a2H9jbMBSyRCdCbcmQ5NjO0H8ffEslL3bavtiARB0ekofk6iXcm4sMtwH6u0CIC5OntCYoSaF6XqD2j7nQwKxQkiCm0iKTzc2R5gTt89aqFNyeCAqD4Nvz4m2xGkDDGdBSNwDNtzmiqThKLCxkhiAG0oL2nTioUYuaCYlPII4nS9BT1PHIKedCHgkKVtFGQ4aC1a75t06VtNiICzS3_zDyzsseaBsEzVmsNAeNNeWiMARtO3RWaKM0QbkSA7_YstejVoXkBFlNpcmpBmlba5U2xmeqHzF_CQ9wifQlWL7DExTmFTIqFXxkBUs_3t9UC4q5MDROgX9_sLqAhz9tFlROmbB1E9Nm45KUymKapqoE7oE9tc9ukisccnjSwsFaT2OFwV2w1sRB20yjZXfBMh9r_3U1Oyi7FsUTd8OiamTUyqxnFKtqpLCwl4wr0vX6P5EOGrytt4_IxJegf79bzurQCXTfdRTVRyEJCuba5XYpW0_qDDpJVdm3NpV1eQINnqfDlwqzHhWGTvBNaCbSeswu6oQ9FVF5EhawySkDgnINeJhJk_HMJZ0vP1FjJ6S4D6m66iWMuodXGjVBtLDKf0YSxQqDz8KIOIVuJhQ2FRIIiiaE07VTb4ZVkhiwrbUHzW6X6u6kHmV6d1c5jk_dXk4SV9_qn9u995sVzmM4cMrzh0xPzLggiLDNIBEVHL02JS_HzX7wyGvli3waJXOpWJUGmM1WHZ6fn_V0ZFqIPTm4kc9_mja3R5h_qzr6_vBJT1crozoiBor2GvdvY6VtSrrO9cp3D6vhrN7Y_3tPYuPidIMeLcYDEGUWXGGUf7mluhVmz81rX9BZQeW7r7x7657N_zJ_ANlndPMSQkAAA_22_7D__WEBPACK_IMPORTED_MODULE_1__);


var actionButton = '_18szn162';
var approveVariant = '_18szn163';
var buttons = '_18szn161';
var commandMeta = '_18szn16c';
var commandRegion = '_18szn169';
var commandScroll = '_18szn16b';
var commandText = '_18szn16a';
var denyVariant = '_18szn164';
var field = '_18szn167';
var menu = '_18szn16d';
var root = '_18szn160';
var secondaryVariant = '_18szn165';
var status = '_18szn168';
var stepActions = '_18szn16e';
var stickyBar = '_18szn16f';
var subStep = '_18szn166';

/***/ }),

/***/ 50207:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ko: () => (/* binding */ AnswerGuidanceRequestRequestSchema),
/* harmony export */   g3: () => (/* binding */ GetGuidanceRequestRequestSchema),
/* harmony export */   h$: () => (/* binding */ GuidanceRequestService),
/* harmony export */   vJ: () => (/* binding */ ListGuidanceRequestsRequestSchema),
/* harmony export */   zR: () => (/* binding */ ListSavedGuidanceAnswersRequestSchema)
/* harmony export */ });
/* unused harmony exports file_session_v1_guidance_request, GuidanceRequestSchema, GuidanceQuestionSchema, GuidanceOptionSchema, CreateGuidanceRequestRequestSchema, CreateGuidanceRequestResponseSchema, AnswerGuidanceRequestResponseSchema, GetGuidanceRequestResponseSchema, ListGuidanceRequestsResponseSchema, ListAllPendingGuidanceRequestsRequestSchema, ListAllPendingGuidanceRequestsResponseSchema, ListSavedGuidanceAnswersResponseSchema */
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(1844);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(15033);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(95765);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(43963);
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/guidance_request.proto (package session.v1, syntax proto3)
/* eslint-disable */ 

/**
 * Describes the file session/v1/guidance_request.proto.
 */ const file_session_v1_guidance_request = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__/* .fileDesc */ .w)("CiFzZXNzaW9uL3YxL2d1aWRhbmNlX3JlcXVlc3QucHJvdG8SCnNlc3Npb24udjEioQYKD0d1aWRhbmNlUmVxdWVzdBIKCgJpZBgBIAEoCRINCgVzY29wZRgCIAEoCRIUCgdpdGVtX2lkGAMgASgJSACIAQESFAoMc2Vzc2lvbl91dWlkGAQgASgJEhUKDXF1ZXN0aW9uX3RleHQYBSABKAkSFQoNcXVlc3Rpb25fdHlwZRgGIAEoCRIPCgdvcHRpb25zGAcgAygJEg4KBmFuc3dlchgIIAEoCRIuCgpjcmVhdGVkX2F0GAkgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBIvCgtub3RpZmllZF9hdBgKIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASLwoLYW5zd2VyZWRfYXQYCyABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEjAKDGNhbmNlbGxlZF9hdBgMIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASDgoGc3RhdHVzGA0gASgJEg4KBnNvdXJjZRgOIAEoCRIaChJhc2tlcl9zZXNzaW9uX3V1aWQYDyABKAkSFQoNYW5zd2VyZXJfa2luZBgQIAEoCRIdChVhbnN3ZXJlcl9zZXNzaW9uX3V1aWQYESABKAkSFAoMbXVsdGlfc2VsZWN0GBIgASgIEi8KCXF1ZXN0aW9ucxgTIAMoCzIcLnNlc3Npb24udjEuR3VpZGFuY2VRdWVzdGlvbhIWCg5kZWxpdmVyeV9yb3V0ZRgUIAEoCRIWCg5kZWxpdmVyeV9zdGF0ZRgVIAEoCRIZChFkZWxpdmVyeV9hdHRlbXB0cxgWIAEoBRIUCgxob29rX2NhbGxfaWQYFyABKAkSLgoKZXhwaXJlc19hdBgYIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASFgoOdW5hY2tub3dsZWRnZWQYGSABKAgSFQoNY2FuY2VsX3JlYXNvbhgaIAEoCRIVCg1zZXNzaW9uX2VuZGVkGBsgASgIEhgKEHNlc3Npb25fYXJjaGl2ZWQYHCABKAhCCgoIX2l0ZW1faWQidwoQR3VpZGFuY2VRdWVzdGlvbhIQCghxdWVzdGlvbhgBIAEoCRIOCgZoZWFkZXIYAiABKAkSKwoHb3B0aW9ucxgDIAMoCzIaLnNlc3Npb24udjEuR3VpZGFuY2VPcHRpb24SFAoMbXVsdGlfc2VsZWN0GAQgASgIIjQKDkd1aWRhbmNlT3B0aW9uEg0KBWxhYmVsGAEgASgJEhMKC2Rlc2NyaXB0aW9uGAIgASgJIsEBChxDcmVhdGVHdWlkYW5jZVJlcXVlc3RSZXF1ZXN0Eg0KBXNjb3BlGAEgASgJEhQKB2l0ZW1faWQYAiABKAlIAIgBARIUCgxzZXNzaW9uX3V1aWQYAyABKAkSFQoNcXVlc3Rpb25fdGV4dBgEIAEoCRIVCg1xdWVzdGlvbl90eXBlGAUgASgJEg8KB29wdGlvbnMYBiADKAkSGwoTY2FsbGVyX3Nlc3Npb25fdXVpZBgHIAEoCUIKCghfaXRlbV9pZCJNCh1DcmVhdGVHdWlkYW5jZVJlcXVlc3RSZXNwb25zZRIsCgdyZXF1ZXN0GAEgASgLMhsuc2Vzc2lvbi52MS5HdWlkYW5jZVJlcXVlc3QiOgocQW5zd2VyR3VpZGFuY2VSZXF1ZXN0UmVxdWVzdBIKCgJpZBgBIAEoCRIOCgZhbnN3ZXIYAiABKAkiXgodQW5zd2VyR3VpZGFuY2VSZXF1ZXN0UmVzcG9uc2USLAoHcmVxdWVzdBgBIAEoCzIbLnNlc3Npb24udjEuR3VpZGFuY2VSZXF1ZXN0Eg8KB2FwcGxpZWQYAiABKAgiJwoZR2V0R3VpZGFuY2VSZXF1ZXN0UmVxdWVzdBIKCgJpZBgBIAEoCSJKChpHZXRHdWlkYW5jZVJlcXVlc3RSZXNwb25zZRIsCgdyZXF1ZXN0GAEgASgLMhsuc2Vzc2lvbi52MS5HdWlkYW5jZVJlcXVlc3QiWQobTGlzdEd1aWRhbmNlUmVxdWVzdHNSZXF1ZXN0Eg0KBXNjb3BlGAEgASgJEhEKCXNjb3BlX2tleRgCIAEoCRIYChBpbmNsdWRlX2Fuc3dlcmVkGAMgASgIInEKHExpc3RHdWlkYW5jZVJlcXVlc3RzUmVzcG9uc2USLQoIcmVxdWVzdHMYASADKAsyGy5zZXNzaW9uLnYxLkd1aWRhbmNlUmVxdWVzdBIVCg1wZW5kaW5nX2NvdW50GAIgASgFEgsKA2NhcBgDIAEoBSInCiVMaXN0QWxsUGVuZGluZ0d1aWRhbmNlUmVxdWVzdHNSZXF1ZXN0IlcKJkxpc3RBbGxQZW5kaW5nR3VpZGFuY2VSZXF1ZXN0c1Jlc3BvbnNlEi0KCHJlcXVlc3RzGAEgAygLMhsuc2Vzc2lvbi52MS5HdWlkYW5jZVJlcXVlc3QiIQofTGlzdFNhdmVkR3VpZGFuY2VBbnN3ZXJzUmVxdWVzdCJRCiBMaXN0U2F2ZWRHdWlkYW5jZUFuc3dlcnNSZXNwb25zZRItCghyZXF1ZXN0cxgBIAMoCzIbLnNlc3Npb24udjEuR3VpZGFuY2VSZXF1ZXN0MtEFChZHdWlkYW5jZVJlcXVlc3RTZXJ2aWNlEm4KFUNyZWF0ZUd1aWRhbmNlUmVxdWVzdBIoLnNlc3Npb24udjEuQ3JlYXRlR3VpZGFuY2VSZXF1ZXN0UmVxdWVzdBopLnNlc3Npb24udjEuQ3JlYXRlR3VpZGFuY2VSZXF1ZXN0UmVzcG9uc2UiABJuChVBbnN3ZXJHdWlkYW5jZVJlcXVlc3QSKC5zZXNzaW9uLnYxLkFuc3dlckd1aWRhbmNlUmVxdWVzdFJlcXVlc3QaKS5zZXNzaW9uLnYxLkFuc3dlckd1aWRhbmNlUmVxdWVzdFJlc3BvbnNlIgASZQoSR2V0R3VpZGFuY2VSZXF1ZXN0EiUuc2Vzc2lvbi52MS5HZXRHdWlkYW5jZVJlcXVlc3RSZXF1ZXN0GiYuc2Vzc2lvbi52MS5HZXRHdWlkYW5jZVJlcXVlc3RSZXNwb25zZSIAEmsKFExpc3RHdWlkYW5jZVJlcXVlc3RzEicuc2Vzc2lvbi52MS5MaXN0R3VpZGFuY2VSZXF1ZXN0c1JlcXVlc3QaKC5zZXNzaW9uLnYxLkxpc3RHdWlkYW5jZVJlcXVlc3RzUmVzcG9uc2UiABKJAQoeTGlzdEFsbFBlbmRpbmdHdWlkYW5jZVJlcXVlc3RzEjEuc2Vzc2lvbi52MS5MaXN0QWxsUGVuZGluZ0d1aWRhbmNlUmVxdWVzdHNSZXF1ZXN0GjIuc2Vzc2lvbi52MS5MaXN0QWxsUGVuZGluZ0d1aWRhbmNlUmVxdWVzdHNSZXNwb25zZSIAEncKGExpc3RTYXZlZEd1aWRhbmNlQW5zd2VycxIrLnNlc3Npb24udjEuTGlzdFNhdmVkR3VpZGFuY2VBbnN3ZXJzUmVxdWVzdBosLnNlc3Npb24udjEuTGlzdFNhdmVkR3VpZGFuY2VBbnN3ZXJzUmVzcG9uc2UiAEK0AQoOY29tLnNlc3Npb24udjFCFEd1aWRhbmNlUmVxdWVzdFByb3RvUAFaQ2dpdGh1Yi5jb20vdHN0YXBsZXIvc3RhcGxlci1zcXVhZC9nZW4vcHJvdG8vZ28vc2Vzc2lvbi92MTtzZXNzaW9udjGiAgNTWFiqAgpTZXNzaW9uLlYxygIKU2Vzc2lvblxWMeICFlNlc3Npb25cVjFcR1BCTWV0YWRhdGHqAgtTZXNzaW9uOjpWMWIGcHJvdG8z", [
    _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_1__/* .file_google_protobuf_timestamp */ .i
]);
/**
 * Describes the message session.v1.GuidanceRequest.
 * Use `create(GuidanceRequestSchema)` to create a new message.
 */ const GuidanceRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 0)));
/**
 * Describes the message session.v1.GuidanceQuestion.
 * Use `create(GuidanceQuestionSchema)` to create a new message.
 */ const GuidanceQuestionSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 1)));
/**
 * Describes the message session.v1.GuidanceOption.
 * Use `create(GuidanceOptionSchema)` to create a new message.
 */ const GuidanceOptionSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 2)));
/**
 * Describes the message session.v1.CreateGuidanceRequestRequest.
 * Use `create(CreateGuidanceRequestRequestSchema)` to create a new message.
 */ const CreateGuidanceRequestRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 3)));
/**
 * Describes the message session.v1.CreateGuidanceRequestResponse.
 * Use `create(CreateGuidanceRequestResponseSchema)` to create a new message.
 */ const CreateGuidanceRequestResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 4)));
/**
 * Describes the message session.v1.AnswerGuidanceRequestRequest.
 * Use `create(AnswerGuidanceRequestRequestSchema)` to create a new message.
 */ const AnswerGuidanceRequestRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_guidance_request, 5);
/**
 * Describes the message session.v1.AnswerGuidanceRequestResponse.
 * Use `create(AnswerGuidanceRequestResponseSchema)` to create a new message.
 */ const AnswerGuidanceRequestResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 6)));
/**
 * Describes the message session.v1.GetGuidanceRequestRequest.
 * Use `create(GetGuidanceRequestRequestSchema)` to create a new message.
 */ const GetGuidanceRequestRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_guidance_request, 7);
/**
 * Describes the message session.v1.GetGuidanceRequestResponse.
 * Use `create(GetGuidanceRequestResponseSchema)` to create a new message.
 */ const GetGuidanceRequestResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 8)));
/**
 * Describes the message session.v1.ListGuidanceRequestsRequest.
 * Use `create(ListGuidanceRequestsRequestSchema)` to create a new message.
 */ const ListGuidanceRequestsRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_guidance_request, 9);
/**
 * Describes the message session.v1.ListGuidanceRequestsResponse.
 * Use `create(ListGuidanceRequestsResponseSchema)` to create a new message.
 */ const ListGuidanceRequestsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 10)));
/**
 * Describes the message session.v1.ListAllPendingGuidanceRequestsRequest.
 * Use `create(ListAllPendingGuidanceRequestsRequestSchema)` to create a new message.
 */ const ListAllPendingGuidanceRequestsRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 11)));
/**
 * Describes the message session.v1.ListAllPendingGuidanceRequestsResponse.
 * Use `create(ListAllPendingGuidanceRequestsResponseSchema)` to create a new message.
 */ const ListAllPendingGuidanceRequestsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 12)));
/**
 * Describes the message session.v1.ListSavedGuidanceAnswersRequest.
 * Use `create(ListSavedGuidanceAnswersRequestSchema)` to create a new message.
 */ const ListSavedGuidanceAnswersRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_guidance_request, 13);
/**
 * Describes the message session.v1.ListSavedGuidanceAnswersResponse.
 * Use `create(ListSavedGuidanceAnswersResponseSchema)` to create a new message.
 */ const ListSavedGuidanceAnswersResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_guidance_request, 14)));
/**
 * GuidanceRequestService lets an agent (an interactive session, headless
 * triage, or an automated workflow) ask a human a structured question
 * durably — the question survives session restarts/respawns — and lets the
 * web UI answer it. Mirrors TymuxRolloutService's registration shape
 * (proto/session/v1/tymux_rollout.proto). See
 * project_plans/durable-guidance-request/implementation/plan.md Phase 2.
 *
 * @generated from service session.v1.GuidanceRequestService
 */ const GuidanceRequestService = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .serviceDesc */ .Z)(file_session_v1_guidance_request, 0);


/***/ }),

/***/ 62590:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   r: () => (/* binding */ ActionButton)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_hooks_useEngagedCard__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(27924);
/* harmony import */ var _lib_hooks_interactionTiming__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(29901);
/* harmony import */ var _ActionRow_css__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(48241);
/* __next_internal_client_entry_do_not_use__ ActionButton auto */ 




/** The row's button: `aria-disabled` rather than `disabled`, so the reason stays reachable. */ function ActionButton(param) {
    let { id, label, onClick, variant = "", disabled, reasonId, minHeight, style, buttonRef, option } = param;
    const tracker = (0,_lib_hooks_useEngagedCard__WEBPACK_IMPORTED_MODULE_2__/* .useEngagement */ .Tq)();
    const [mountedAt] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(()=>Date.now());
    // A control that just rendered or moved ignores activation, so a tap aimed at its
    // previous occupant cannot trigger it (design/ux.md 7.1, UX-AC62).
    const settled = ()=>tracker ? tracker.isLayoutSettled(mountedAt) : Date.now() - mountedAt >= (0,_lib_hooks_interactionTiming__WEBPACK_IMPORTED_MODULE_3__/* .getTiming */ .Hl)().settleMs;
    return(// analytics-exempt
    /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("button", {
        ref: buttonRef,
        type: "button",
        className: "".concat(_ActionRow_css__WEBPACK_IMPORTED_MODULE_4__/* .actionButton */ .hP, " ").concat(variant),
        "data-testid": "action-".concat(id),
        "aria-disabled": disabled ? true : undefined,
        "aria-describedby": disabled ? reasonId : undefined,
        "data-action-option": option ? "" : undefined,
        style: {
            minHeight,
            ...style
        },
        onClick: ()=>{
            if (!disabled && settled()) onClick();
        },
        children: label
    }));
}


/***/ }),

/***/ 77448:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AE: () => (/* binding */ pendingLabel),
/* harmony export */   B3: () => (/* binding */ alreadyHandled),
/* harmony export */   CT: () => (/* binding */ UNDO_LABEL),
/* harmony export */   Dq: () => (/* binding */ characterCount),
/* harmony export */   JE: () => (/* binding */ mergePrompt),
/* harmony export */   Kx: () => (/* binding */ handledByRule),
/* harmony export */   Uv: () => (/* binding */ QUESTION_FORM_COPY),
/* harmony export */   XN: () => (/* binding */ createRulePrompt),
/* harmony export */   ZP: () => (/* binding */ DELIVERY_COPY),
/* harmony export */   a6: () => (/* binding */ HOLD_COPY),
/* harmony export */   eV: () => (/* binding */ ANNOUNCE),
/* harmony export */   gH: () => (/* binding */ PENDING_VERB),
/* harmony export */   l7: () => (/* binding */ moreWaiting),
/* harmony export */   oQ: () => (/* binding */ answered),
/* harmony export */   px: () => (/* binding */ holdCountdown),
/* harmony export */   rc: () => (/* binding */ decided),
/* harmony export */   sF: () => (/* binding */ COPY),
/* harmony export */   ul: () => (/* binding */ answeredElsewhere),
/* harmony export */   xd: () => (/* binding */ rateLimited),
/* harmony export */   zl: () => (/* binding */ cancelPrompt)
/* harmony export */ });
/* harmony import */ var _lib_hooks_useFeatureFlagValue__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(43661);

/**
 * The one vocabulary module for inline notification actions (design/ux.md section 0,
 * 2.3 and 3.1). UI strings only; internal identifiers keep their names. A test
 * (copyGuard.test.ts) scans every export for the banned terms.
 */ const COPY = {
    approve: "Approve",
    deny: "Deny",
    open: "Open",
    openSession: "Open session",
    back: "Back",
    retry: "Retry",
    refresh: "Refresh",
    copy: "Copy",
    more: "More",
    sendDenial: "Send denial",
    denyReasonLabel: "Reason (optional)",
    confirmMerge: "Confirm merge",
    createRule: "Create rule",
    createRuleMenu: "Create rule...",
    keep: "Keep",
    cancelPrompt: "Cancel prompt",
    cancelRequest: "Cancel request",
    retryAnyway: "Retry anyway",
    retryAnywayPrompt: "Retry anyway?",
    approved: "Approved",
    denied: "Denied",
    sentWaiting: "Sent - waiting for the agent",
    flagOff: _lib_hooks_useFeatureFlagValue__WEBPACK_IMPORTED_MODULE_0__/* .flagOffMessage */ .jG,
    invalid: "That action isn't valid anymore. Refresh.",
    desktopOnly: "Answer from desktop",
    resumeFromDesktop: "Resume from desktop",
    staleGone: "Already handled in the session. No action needed.",
    staleReplaced: "The session is now asking something else. Open session to answer it.",
    sessionEnded: "Session ended before this was answered.",
    notSent: "Not sent. Nothing was typed into the session. Retry",
    notSentHumanActive: "Not sent: someone is typing in that session. Retry when they are done.",
    indeterminate: "Sent, but we couldn't confirm it arrived. Check the session before retrying.",
    transient: "Couldn't send your action. Retry",
    cantShowCommand: "Can't show this command. Open session to review it.",
    commandRegionLabel: "Command, scrollable",
    forbidden: "This action isn't available here. Open session.",
    offline: "Offline",
    promptCancelled: "Prompt cancelled",
    requestCancelled: "Request cancelled",
    cancelPromptQuestion: "Cancel this prompt?",
    cancelRequestQuestion: "Cancel this request?",
    dismiss: "Dismiss",
    copied: "Copied",
    retryAnywayConfirm: "Retry"
};
/**
 * The delivery-state line of a question card (design/ux.md 3.1; UX-AC23). Rows 4 and 13
 * ("Answered: {value}", "Answered elsewhere: {value}") are the `answered` and `answeredElsewhere`
 * helpers below; the rest of the 17 settled-state strings, and the human_active variant of row 5
 * (`COPY.notSentHumanActive`), are here and in `COPY`.
 */ const DELIVERY_COPY = {
    sending: "Sending...",
    unconfirmed: "Sent, but the agent hasn't confirmed yet. Open session.",
    unconfirmedChip: "Unconfirmed",
    clientFailure: "Couldn't send your answer. Retry",
    savedStopped: "Your answer is saved. The agent can read it once you resume the session.",
    savedArchived: "This session is archived. Your answer will be saved but not delivered unless you resume it.",
    aborted: "Part of your answer was sent, then the prompt changed. Open session to finish.",
    stillShowing: "Sent, but the question is still showing. Check the session before retrying.",
    alreadyAnsweredReload: "Already answered - reload to see the answer",
    sentSessionEnded: "Sent, but the session ended before confirming it.",
    markHandled: "Mark as handled",
    markHandledPrompt: "Mark as handled in this browser?",
    markHandledConfirm: "Mark handled",
    dismissUnfinishedPrompt: "Dismiss without finishing?",
    dismissUnfinishedConfirm: "Dismiss"
};
/** Labels and helper text of the question form (design/ux.md 3.1 and 3.3). */ const QUESTION_FORM_COPY = {
    submitAnswers: "Submit answers",
    clear: "Clear",
    send: "Send",
    otherLabel: "Other",
    chooseAtLeastOne: "Choose at least one option",
    viewFullPrompt: "View full prompt in session",
    answerInSession: "Answer in the session",
    noDeliveryYet: "Delivery: (none until sent)",
    questionOf: (n, total)=>"Question ".concat(n, " of ").concat(total),
    chooseAny: "(choose any)",
    selectedCount: (n)=>"".concat(n, " selected"),
    answerToContinue: (n)=>"Answer question ".concat(n, " to continue"),
    answerFor: (question)=>"Answer: ".concat(question),
    savedForLater: (n)=>"Saved for later (".concat(n, ")"),
    savedStopped: "stopped",
    savedArchived: "archived"
};
/** Inline "Approving..." text while a request is in flight. */ const PENDING_VERB = {
    approve: "Approving",
    deny: "Denying",
    other: "Sending"
};
const decided = (choice)=>"Decided: ".concat(choice);
const answered = (value)=>"Answered: ".concat(value);
const answeredElsewhere = (summary)=>"Answered elsewhere: ".concat(summary);
const alreadyHandled = (value)=>value ? "Already handled: ".concat(value) : "Already handled";
const handledByRule = (name)=>"Handled automatically by rule ".concat(name);
const moreWaiting = (n)=>"".concat(n, " more waiting.");
const rateLimited = (seconds)=>"Too many actions. Try again in ".concat(seconds, "s.");
const pendingLabel = (verb)=>"".concat(verb, "...");
const mergePrompt = (pr)=>pr ? "Merge PR #".concat(pr, "?") : "Merge this PR?";
const createRulePrompt = (what)=>"Create a rule that auto-approves ".concat(what, "?");
const characterCount = (n)=>"".concat(n.toLocaleString("en-US"), " characters");
const cancelPrompt = (kind)=>kind === "prompt" ? COPY.cancelPrompt : COPY.cancelRequest;
/** Polite-region result for a settled outcome ("Approved", "Answer sent"). */ const ANNOUNCE = {
    applied: (text)=>text,
    answerSent: "Answer sent",
    notDelivered: "Answer not delivered",
    stillShowing: "Answer sent, question still showing",
    notConfirmed: "Answer not confirmed",
    partlySent: "Answer partly sent",
    handledInSession: "Already handled in the session. No action needed."
};
const HOLD_COPY = {
    approve: {
        button: "Approve",
        verbing: "Approving",
        pending: "Approval pending",
        undone: "Approval undone",
        cancelled: "Approval cancelled when you left. Tap Approve to try again."
    },
    rework: {
        button: "Send back",
        verbing: "Sending back",
        pending: "Send back pending",
        undone: "Send back undone",
        cancelled: "Send back cancelled when you left. Tap Send back to try again."
    },
    accept: {
        button: "Mark done",
        verbing: "Marking done",
        pending: "Mark done pending",
        undone: "Mark done undone",
        cancelled: "Mark done cancelled when you left. Tap Mark done to try again."
    }
};
const holdCountdown = (verbing, seconds)=>"".concat(verbing, " in ").concat(seconds, "s");
const UNDO_LABEL = "Undo";


/***/ })

}]);
//# sourceMappingURL=4298-46c3c86684960069.js.map