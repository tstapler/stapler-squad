"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[2275],{

/***/ 2113:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   j: () => (/* binding */ GESTURE_MOVEMENT_THRESHOLD_PX)
/* harmony export */ });
// Shared movement threshold for touch-gesture disambiguation on the window
// tab strip: WindowTabStrip's long-press-to-rename check (Story 3.1.2) and
// useWindowSwipe's swipe-vs-scroll check (Epic 3.2, a later task) both need
// "did this touch move enough to not be a stationary hold" to agree on one
// number rather than two independently-tuned ones.
const GESTURE_MOVEMENT_THRESHOLD_PX = 10;


/***/ }),

/***/ 5647:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   FR: () => (/* binding */ AnnouncerContext),
/* harmony export */   XE: () => (/* binding */ useAnnounce),
/* harmony export */   _h: () => (/* binding */ AnnouncerEngine)
/* harmony export */ });
/* unused harmony exports NOOP_ANNOUNCE, COALESCE_WINDOW_MS, ANNOUNCE_HOLD_MS, POLITE_QUEUE_MAX */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ NOOP_ANNOUNCE,AnnouncerContext,useAnnounce,COALESCE_WINDOW_MS,ANNOUNCE_HOLD_MS,POLITE_QUEUE_MAX,AnnouncerEngine auto */ 
const NOOP_ANNOUNCE = {
    announce: ()=>{},
    announceArrival: ()=>{}
};
const AnnouncerContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.createContext)(NOOP_ANNOUNCE);
function useAnnounce() {
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useContext)(AnnouncerContext);
}
const COALESCE_WINDOW_MS = 500;
/** How long a message stays in its region before the next is spoken. */ const ANNOUNCE_HOLD_MS = 1000;
const POLITE_QUEUE_MAX = 5;
/**
 * Framework-free scheduling core of the Announcer, so the queue rules are
 * unit-testable. Each channel has its own region and speaks one message at a time;
 * an assertive message never waits behind a polite one.
 */ class AnnouncerEngine {
    announce(message) {
        let politeness = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : "polite", key = arguments.length > 2 ? arguments[2] : void 0;
        const queue = this.queues[politeness];
        const existing = key === undefined ? -1 : queue.findIndex((q)=>q.key === key);
        if (existing >= 0) queue[existing] = {
            text: message,
            key
        };
        else queue.push({
            text: message,
            key
        });
        if (politeness === "polite" && queue.length > POLITE_QUEUE_MAX) queue.shift();
        this.drain(politeness);
    }
    announceArrival(arrival) {
        this.arrivals.push(arrival);
        if (this.arrivalTimer !== null) return;
        this.arrivalTimer = setTimeout(()=>this.flushArrivals(), COALESCE_WINDOW_MS);
    }
    /** Stops every timer. Queued messages are kept so `resume()` can speak them (StrictMode runs cleanup, then setup, on one instance). */ dispose() {
        if (this.arrivalTimer !== null) clearTimeout(this.arrivalTimer);
        this.arrivalTimer = null;
        for (const channel of [
            "polite",
            "assertive"
        ]){
            const timer = this.holdTimers[channel];
            if (timer !== null) clearTimeout(timer);
            this.holdTimers[channel] = null;
            // A cleared hold timer would never fire, leaving the channel stuck "speaking" and silent.
            this.speaking[channel] = false;
        }
    }
    /** Restarts scheduling after `dispose()`: pending arrivals flush and queued messages drain. */ resume() {
        if (this.arrivals.length > 0 && this.arrivalTimer === null) {
            this.arrivalTimer = setTimeout(()=>this.flushArrivals(), COALESCE_WINDOW_MS);
        }
        this.drain("polite");
        this.drain("assertive");
    }
    flushArrivals() {
        this.arrivalTimer = null;
        const batch = this.arrivals;
        this.arrivals = [];
        if (batch.length === 0) return;
        const pinned = batch.filter((a)=>a.pinned);
        const newest = batch[batch.length - 1];
        if (batch.length === 1) {
            if (newest.pinned) this.announce(newest.title, "assertive");
            else this.announce("1 new notification");
        } else if (pinned.length === 0) {
            this.announce("".concat(batch.length, " new notifications"));
        } else if (pinned.length === batch.length) {
            this.announce("".concat(batch.length, " notifications need attention, newest: ").concat(newest.title), "assertive");
        } else {
            const needs = pinned.length === 1 ? "1 needs attention" : "".concat(pinned.length, " need attention");
            this.announce("".concat(batch.length, " new notifications, ").concat(needs), "assertive");
        }
    }
    drain(channel) {
        if (this.speaking[channel]) return;
        const next = this.queues[channel].shift();
        if (!next) return;
        const write = channel === "assertive" ? this.out.setAssertive : this.out.setPolite;
        this.speaking[channel] = true;
        write(next.text);
        this.holdTimers[channel] = setTimeout(()=>{
            this.holdTimers[channel] = null;
            // Clearing lets an identical later message change the region, so it is announced again.
            write("");
            this.speaking[channel] = false;
            this.drain(channel);
        }, ANNOUNCE_HOLD_MS);
    }
    constructor(out){
        this.out = out;
        this.queues = {
            polite: [],
            assertive: []
        };
        this.speaking = {
            polite: false,
            assertive: false
        };
        this.holdTimers = {
            polite: null,
            assertive: null
        };
        this.arrivals = [];
        this.arrivalTimer = null;
    }
}


/***/ }),

/***/ 13867:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   r: () => (/* binding */ useAuditLog)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(4504);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(95444);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(21694);
/* __next_internal_client_entry_do_not_use__ useAuditLog auto */ 






function useAuditLog() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { enabled = true } = options;
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    // Initialize gRPC client on first use
    const getClient = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        if (!clientRef.current) {
            clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_4__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)());
        }
        return clientRef.current;
    }, []);
    // Log user interaction to server
    const logInteraction = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (entry)=>{
        if (!enabled) {
            return false; // Silently skip if disabled
        }
        try {
            const client = getClient();
            const request = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_5__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .LogUserInteractionRequestSchema */ ._4C, {
                sessionId: entry.sessionId,
                interactionType: entry.interactionType,
                context: entry.context,
                notificationId: entry.notificationId,
                metadata: entry.metadata
            });
            const response = await client.logUserInteraction(request);
            return response.success;
        } catch (error) {
            console.error("Failed to log user interaction:", error);
            return false;
        }
    }, [
        enabled,
        getClient
    ]);
    // Convenience methods for common notification interactions
    const logNotificationPanelOpened = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        return logInteraction({
            interactionType: _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__/* .UserInteractionEvent_InteractionType */ .fv.NOTIFICATION_PANEL_OPENED,
            context: "User opened notification panel"
        });
    }, [
        logInteraction
    ]);
    const logNotificationPanelClosed = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        return logInteraction({
            interactionType: _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__/* .UserInteractionEvent_InteractionType */ .fv.NOTIFICATION_PANEL_CLOSED,
            context: "User closed notification panel"
        });
    }, [
        logInteraction
    ]);
    const logNotificationViewed = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((notificationId, sessionId)=>{
        return logInteraction({
            sessionId,
            interactionType: _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__/* .UserInteractionEvent_InteractionType */ .fv.NOTIFICATION_VIEWED,
            context: "User viewed notification",
            notificationId
        });
    }, [
        logInteraction
    ]);
    const logNotificationDismissed = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((notificationId, sessionId)=>{
        return logInteraction({
            sessionId,
            interactionType: _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__/* .UserInteractionEvent_InteractionType */ .fv.NOTIFICATION_DISMISSED,
            context: "User dismissed notification toast",
            notificationId
        });
    }, [
        logInteraction
    ]);
    const logNotificationMarkedRead = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((notificationId, sessionId)=>{
        return logInteraction({
            sessionId,
            interactionType: _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__/* .UserInteractionEvent_InteractionType */ .fv.NOTIFICATION_MARKED_READ,
            context: "User marked notification as read",
            notificationId
        });
    }, [
        logInteraction
    ]);
    const logNotificationMarkedAllRead = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((count)=>{
        return logInteraction({
            interactionType: _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__/* .UserInteractionEvent_InteractionType */ .fv.NOTIFICATION_MARKED_ALL_READ,
            context: "User marked all notifications as read",
            metadata: {
                count: count.toString()
            }
        });
    }, [
        logInteraction
    ]);
    const logNotificationRemoved = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((notificationId, sessionId)=>{
        return logInteraction({
            sessionId,
            interactionType: _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__/* .UserInteractionEvent_InteractionType */ .fv.NOTIFICATION_REMOVED,
            context: "User removed notification from history",
            notificationId
        });
    }, [
        logInteraction
    ]);
    const logNotificationHistoryCleared = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((count)=>{
        return logInteraction({
            interactionType: _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__/* .UserInteractionEvent_InteractionType */ .fv.NOTIFICATION_HISTORY_CLEARED,
            context: "User cleared notification history",
            metadata: {
                count: count.toString()
            }
        });
    }, [
        logInteraction
    ]);
    const logNotificationSessionViewed = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((notificationId, sessionId)=>{
        return logInteraction({
            sessionId,
            interactionType: _gen_session_v1_events_pb__WEBPACK_IMPORTED_MODULE_2__/* .UserInteractionEvent_InteractionType */ .fv.NOTIFICATION_SESSION_VIEWED,
            context: "User clicked notification to view session",
            notificationId
        });
    }, [
        logInteraction
    ]);
    return {
        // Generic method
        logInteraction,
        // Convenience methods for notifications
        logNotificationPanelOpened,
        logNotificationPanelClosed,
        logNotificationViewed,
        logNotificationDismissed,
        logNotificationMarkedRead,
        logNotificationMarkedAllRead,
        logNotificationRemoved,
        logNotificationHistoryCleared,
        logNotificationSessionViewed
    };
}


/***/ }),

/***/ 15203:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   DS: () => (/* binding */ MAX_REPLY_OPTIONS),
/* harmony export */   MN: () => (/* binding */ unavailableCopy),
/* harmony export */   rb: () => (/* binding */ truncatePrompt),
/* harmony export */   wR: () => (/* binding */ replyAvailability),
/* harmony export */   xb: () => (/* binding */ pendingQuestionFor)
/* harmony export */ });
/* unused harmony exports PROMPT_TRUNCATE_CHARS, isHookProofCause */
// Pure helpers for the Reply card (ADR-010, design/ux.md Surface 12b). The server stamps
// question_id, question_shape, question_options and question_pending_count (server const
// metaQuestionPending, server/services/approval_question.go) on a replyable question's
// notification; everything else is "Answer in the terminal".
const MAX_REPLY_OPTIONS = 7;
const PROMPT_TRUNCATE_CHARS = 280;
/** The first 280 characters of a question, and whether anything was cut. */ function truncatePrompt(prompt) {
    const chars = Array.from(prompt);
    if (chars.length <= PROMPT_TRUNCATE_CHARS) return {
        shown: prompt,
        truncated: false
    };
    return {
        shown: chars.slice(0, PROMPT_TRUNCATE_CHARS).join(""),
        truncated: true
    };
}
function parseOptions(raw) {
    if (!raw) return null;
    try {
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed) || parsed.length < 1 || parsed.length > MAX_REPLY_OPTIONS) return null;
        if (!parsed.every((o)=>typeof o === "string" && o.trim() !== "")) return null;
        return parsed;
    } catch (e) {
        return null;
    }
}
/**
 * Notifications dedupe per (session, type), so metadata carries only the newest question_id and
 * a question other than the one on screen may be the one offered. The server stamps how many
 * questions were live when this one registered; that is authoritative. occurrenceCount counts
 * every question the record ever merged, answered or not, so it only stands in for servers
 * that do not stamp the pending count.
 */ function hasParallelQuestions(pendingCount, occurrenceCount) {
    const pending = Number(pendingCount);
    if (Number.isInteger(pending) && pending >= 1) return pending > 1;
    return (occurrenceCount !== null && occurrenceCount !== void 0 ? occurrenceCount : 1) > 1;
}
/**
 * null when the notification is not a question at all; otherwise whether Reply can
 * answer it. A question the server did not register (no question_id) is unavailable.
 */ function replyAvailability(n) {
    if (n.notificationType !== "question") return null;
    var _n_metadata;
    const md = (_n_metadata = n.metadata) !== null && _n_metadata !== void 0 ? _n_metadata : {};
    const options = parseOptions(md["question_options"]);
    if (md["question_id"] && md["question_shape"] === "single" && options) {
        if (hasParallelQuestions(md["question_pending_count"], n.occurrenceCount)) {
            return {
                kind: "unavailable",
                cause: "parallel_question"
            };
        }
        return {
            kind: "replyable",
            questionId: md["question_id"],
            options,
            prompt: n.message
        };
    }
    const cause = md["reply_unavailable"];
    if (cause === "no_proof" || cause === "bad_proof" || cause === "path_only") {
        return {
            kind: "unavailable",
            cause
        };
    }
    return {
        kind: "unavailable",
        cause: "shape"
    };
}
/** True for the two causes that say the session's hook has no valid proof (RP-17). */ function isHookProofCause(cause) {
    return cause === "no_proof" || cause === "bad_proof";
}
/** The copy of an unavailable question's card. */ function unavailableCopy(cause) {
    return isHookProofCause(cause) ? "Reply unavailable for this session. Answer in the terminal." : "Answer in the terminal";
}
/** The newest unread question of sessionId, preferring the one a deep link named. */ function pendingQuestionFor(history, sessionId, preferId) {
    const mine = history.filter((n)=>n.sessionId === sessionId && n.notificationType === "question");
    if (preferId) {
        const named = mine.find((n)=>n.id === preferId && !n.isRead);
        if (named) return named;
    }
    return mine.filter((n)=>!n.isRead).sort((a, b)=>b.timestamp - a.timestamp)[0];
}


/***/ }),

/***/ 15254:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   DC: () => (/* binding */ writeQuietMode),
/* harmony export */   EJ: () => (/* binding */ readQuietMode),
/* harmony export */   FU: () => (/* binding */ writeBellPos),
/* harmony export */   FX: () => (/* binding */ readTrayPinned),
/* harmony export */   Gf: () => (/* binding */ readFloatingBell),
/* harmony export */   MG: () => (/* binding */ readWhatChangedSeen),
/* harmony export */   S1: () => (/* binding */ writeTrayPinned),
/* harmony export */   TD: () => (/* binding */ PINNED_COLLAPSE_CHOICES_MS),
/* harmony export */   TW: () => (/* binding */ writeFloatingBell),
/* harmony export */   Vn: () => (/* binding */ readUndoWindowMs),
/* harmony export */   Y_: () => (/* binding */ readBellPos),
/* harmony export */   ZU: () => (/* binding */ resetBellPos),
/* harmony export */   aW: () => (/* binding */ UNDO_WINDOW_CHOICES_MS),
/* harmony export */   bx: () => (/* binding */ writeWhatChangedSeen),
/* harmony export */   gA: () => (/* binding */ writeUndoWindowMs),
/* harmony export */   h5: () => (/* binding */ BELL_SETTINGS_EVENT),
/* harmony export */   jH: () => (/* binding */ writePinnedCollapseMs),
/* harmony export */   q0: () => (/* binding */ readPinnedCollapseMs)
/* harmony export */ });
/* unused harmony exports PINNED_COLLAPSE_MS, PINNED_COLLAPSE_NEVER, UNDO_WINDOW_DEFAULT_MS, PINNED_COLLAPSE_STORAGE_KEY, UNDO_WINDOW_STORAGE_KEY, WHAT_CHANGED_STORAGE_KEY, QUIET_MODE_STORAGE_KEY, TRAY_PINNED_STORAGE_KEY, BELL_POS_STORAGE_KEY, FLOATING_BELL_STORAGE_KEY */
/* harmony import */ var _bellPosition__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(41364);
/**
 * Per-device deck timing settings. The choices live in localStorage (they are
 * about this device's screen and thumb, not the account), and every read falls
 * back to the default when storage is missing, blocked or holds junk.
 */ 
const PINNED_COLLAPSE_MS = 8000;
const PINNED_COLLAPSE_CHOICES_MS = [
    8000,
    15000,
    30000
];
const PINNED_COLLAPSE_NEVER = "never";
const UNDO_WINDOW_DEFAULT_MS = 8000;
const UNDO_WINDOW_CHOICES_MS = [
    5000,
    8000,
    15000,
    30000
];
const PINNED_COLLAPSE_STORAGE_KEY = "ssq.notifications.pinnedCollapse";
const UNDO_WINDOW_STORAGE_KEY = "ssq.notifications.undoWindow";
function readRaw(key) {
    try {
        return  false ? 0 : window.localStorage.getItem(key);
    } catch (e) {
        return null;
    }
}
/** Milliseconds before a pinned card collapses on a phone, or null for "Never". */ function readPinnedCollapseMs() {
    const raw = readRaw(PINNED_COLLAPSE_STORAGE_KEY);
    if (raw === PINNED_COLLAPSE_NEVER) return null;
    const value = Number(raw);
    return PINNED_COLLAPSE_CHOICES_MS.includes(value) ? value : PINNED_COLLAPSE_MS;
}
function readUndoWindowMs() {
    const value = Number(readRaw(UNDO_WINDOW_STORAGE_KEY));
    return UNDO_WINDOW_CHOICES_MS.includes(value) ? value : UNDO_WINDOW_DEFAULT_MS;
}
const WHAT_CHANGED_STORAGE_KEY = "ssq.notifications.whatChangedSeen";
function writeRaw(key, value) {
    try {
        if (true) window.localStorage.setItem(key, value);
    } catch (e) {
    // Storage blocked: the setting simply does not persist; reads keep returning the default.
    }
}
function writeUndoWindowMs(ms) {
    writeRaw(UNDO_WINDOW_STORAGE_KEY, String(ms));
}
/** Pass null for "Never". */ function writePinnedCollapseMs(ms) {
    writeRaw(PINNED_COLLAPSE_STORAGE_KEY, ms === null ? PINNED_COLLAPSE_NEVER : String(ms));
}
function readWhatChangedSeen() {
    return readRaw(WHAT_CHANGED_STORAGE_KEY) === "true";
}
function writeWhatChangedSeen() {
    writeRaw(WHAT_CHANGED_STORAGE_KEY, "true");
}
const QUIET_MODE_STORAGE_KEY = "ssq.notifications.quietMode";
/** Quiet mode: non-pinned toasts go straight to the tray on this device. Off when storage is unavailable. */ function readQuietMode() {
    return readRaw(QUIET_MODE_STORAGE_KEY) === "true";
}
function writeQuietMode(on) {
    writeRaw(QUIET_MODE_STORAGE_KEY, on ? "true" : "false");
}
const TRAY_PINNED_STORAGE_KEY = "ssq.notifications.trayPinned";
/** Pin tray: dock the open tray as a layout column. Off when storage is unavailable. */ function readTrayPinned() {
    return readRaw(TRAY_PINNED_STORAGE_KEY) === "true";
}
function writeTrayPinned(on) {
    writeRaw(TRAY_PINNED_STORAGE_KEY, on ? "true" : "false");
}
const BELL_POS_STORAGE_KEY = "ssq.notifications.bellPos";
const FLOATING_BELL_STORAGE_KEY = "ssq.notifications.floatingBell";
/** Fired on this window after a bell setting changes so a mounted handle re-reads it. */ const BELL_SETTINGS_EVENT = "ssq:bell-settings-changed";
function notifyBellSettings() {
    if (true) window.dispatchEvent(new Event(BELL_SETTINGS_EVENT));
}
function readBellPos() {
    return (0,_bellPosition__WEBPACK_IMPORTED_MODULE_0__/* .parseBellPos */ .jN)(readRaw(BELL_POS_STORAGE_KEY));
}
function writeBellPos(pos) {
    writeRaw(BELL_POS_STORAGE_KEY, JSON.stringify(pos));
    notifyBellSettings();
}
function resetBellPos() {
    try {
        if (true) window.localStorage.removeItem(BELL_POS_STORAGE_KEY);
    } catch (e) {
    // Storage blocked: nothing persisted, so there is nothing to reset.
    }
    notifyBellSettings();
}
/** Opt-in floating bell on a phone (the bottom nav's Alerts tab is the default entry). Off by default. */ function readFloatingBell() {
    return readRaw(FLOATING_BELL_STORAGE_KEY) === "true";
}
function writeFloatingBell(on) {
    writeRaw(FLOATING_BELL_STORAGE_KEY, on ? "true" : "false");
    notifyBellSettings();
}


/***/ }),

/***/ 18311:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $: () => (/* binding */ titleRow),
/* harmony export */   $O: () => (/* binding */ denyButton),
/* harmony export */   $S: () => (/* binding */ undoButton),
/* harmony export */   BM: () => (/* binding */ overflowTrigger),
/* harmony export */   BQ: () => (/* binding */ chipRow),
/* harmony export */   C5: () => (/* binding */ swipeCardDragging),
/* harmony export */   DZ: () => (/* binding */ undoAction),
/* harmony export */   Dd: () => (/* binding */ typeLabel),
/* harmony export */   EZ: () => (/* binding */ undoBar),
/* harmony export */   GQ: () => (/* binding */ closeButtonStacked),
/* harmony export */   Gg: () => (/* binding */ overflowMenu),
/* harmony export */   Kk: () => (/* binding */ icon),
/* harmony export */   L: () => (/* binding */ commandBlock),
/* harmony export */   Ln: () => (/* binding */ repeatBadge),
/* harmony export */   Me: () => (/* binding */ compactPreview),
/* harmony export */   NM: () => (/* binding */ trayButton),
/* harmony export */   Np: () => (/* binding */ focusButton),
/* harmony export */   Oh: () => (/* binding */ deckBesideTray),
/* harmony export */   PQ: () => (/* binding */ swipeRow),
/* harmony export */   Pg: () => (/* binding */ actionError),
/* harmony export */   Pi: () => (/* binding */ sourceApp),
/* harmony export */   RP: () => (/* binding */ exiting),
/* harmony export */   Sn: () => (/* binding */ compactTrailing),
/* harmony export */   T0: () => (/* binding */ compactApprovalCard),
/* harmony export */   Tv: () => (/* binding */ toastStacked),
/* harmony export */   Wz: () => (/* binding */ approveButton),
/* harmony export */   XY: () => (/* binding */ deckPlacement),
/* harmony export */   Xe: () => (/* binding */ toastApproval),
/* harmony export */   Zb: () => (/* binding */ decisionPair),
/* harmony export */   _c: () => (/* binding */ reveal),
/* harmony export */   a7: () => (/* binding */ inlineAction),
/* harmony export */   ak: () => (/* binding */ commandTextExpanded),
/* harmony export */   as: () => (/* binding */ compactCard),
/* harmony export */   b: () => (/* binding */ closeButton),
/* harmony export */   dg: () => (/* binding */ deckHeader),
/* harmony export */   h$: () => (/* binding */ swipeCard),
/* harmony export */   iI: () => (/* binding */ disclosureButton),
/* harmony export */   iU: () => (/* binding */ message),
/* harmony export */   jt: () => (/* binding */ commandText),
/* harmony export */   kR: () => (/* binding */ deckBehindTray),
/* harmony export */   o1: () => (/* binding */ actions),
/* harmony export */   oB: () => (/* binding */ titleWrapper),
/* harmony export */   oR: () => (/* binding */ toast),
/* harmony export */   ou: () => (/* binding */ deckAction),
/* harmony export */   pd: () => (/* binding */ minimized),
/* harmony export */   qM: () => (/* binding */ viewButton),
/* harmony export */   r: () => (/* binding */ dismissButton),
/* harmony export */   rO: () => (/* binding */ trayEntry),
/* harmony export */   rf: () => (/* binding */ body),
/* harmony export */   t: () => (/* binding */ offlineHint),
/* harmony export */   tG: () => (/* binding */ overflowChip),
/* harmony export */   uI: () => (/* binding */ riskLabel),
/* harmony export */   ue: () => (/* binding */ collapsedChip),
/* harmony export */   vE: () => (/* binding */ timestamp),
/* harmony export */   vm: () => (/* binding */ workingDir),
/* harmony export */   wx: () => (/* binding */ header),
/* harmony export */   xv: () => (/* binding */ subtitleRow),
/* harmony export */   z5: () => (/* binding */ revealEnd)
/* harmony export */ });
/* unused harmony export minimizeHint */
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FNotificationToast_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1bSY_bOBa_51cQBTRgJ2aN5K1cChLkNLe_zFxmMGg0KIm2GUukQlFealD_fcBFMkUtlpxKpxsYpOEuWeTjWz6_jfSXA75sOUpxDn73v0bnU7rwwH_fAeD9MgO_5_2iHgAQHNF8y3gaAM4EEnjixXg3_fgOgNd3APhy_EJ_rOTHk_x47pkM_dr0uRy_lB9r_bHpmWlPfH33pcm_r6ZuOUubNNSfCRL4X5PlysvOig4ALEMREZcAeCVDgvVO9tyJfsnQY8nGXBHIWE4EYTQAW3LGsRwVMiFYGoD5MjvLZ052e3F9PJFY7AOw2Hj6OUVnuMd6TISSaHJEfALhkeBTxrgw75St4uN_CiBYboxgIYoOO84KGgdAz_rdf8Yvc_7yrN8zHmMegHl2BjlLSGxGZZwwTsQFRixhfObMTafWZMhRTIo8AP5csxuyM8z3KGanAHhgk53BQpLnuxBNvBkw_z0upzPgqX9_dnYWmK_UAi_Q0BifA_B7G2UXdsR8m0jCexLHmMrv_sxTNw6iJEXaFFekeI_LHERFSCIY4heC_cR7XBoW5zPgT6s1jBVRkgyZZOFgoXBwC4M2Aq3JSzXZQGK_boGENHaXcqKC54wHIGOECsxbrFaCrsSkv5bPFgMrm4GlxqT1eq1exyTPEnQJwDbBihxKyI5CInCaByDC5eI7lF2BkqE4JnSn1zQfc_OXC96EUIw43Em2MRUTwQzHs6GInWm1Z4hjKpzNAXPyggPt7Jbu4pDjDCMRAMrMn5YaK7U5G6hcGE8dazqKu_rAM_K3EKeMYlch_juL9JN2eowKI01pWam6CixqI0gjwXzPCT0Yh9eyNTzgPa5ygFGOIaGQFaJLksbKpQEtqg12N3pSorb3xzYIKS5jwnGkSUQsKVJaochIlxIKDTjrG_d5PDA3DrafQS44o7uGfCu9tvrqZDS79pQmFfxcZ6u90WlPBIZ5hiIsFXLiKOt0avgs4PUFThKS5ST_WFdZp_yWcTplWDjCosYIr1tKxZ_l0YoswzxCuYJpgoXAXAmqoO09rpwNL3f5ugb10h01dl9z67uBZDWdNYLLSu73EaHMWE1ZqMdU7sZpVzmqI68B_fBOZNatY1xFK97mnV4nvMFbdEVBafJVD7CdSBc3MDSCS6wmN33cNUspn20nt_lycp1LtkbFCpqelYGVxEu65fNQ030tckG2EnlUYCrsV_2wb_YZvnHBN9CHg730Fg0N1qXfTDsV43UiBjdsuqmlqXPPUZKbIWydQGeH_NLFwUsAUCGYm9ssPJeaxdn2Bpa1z0sR3xFqDGuL0RoeH1djnHjGMSx9w4nxGIYco0MA1P_g_KbG0L7GkMyMVQI8dmeP9B1lVfEG0ceShbQ7MdtbNU1_bD2gGDZPfkTuWH9r26CIaNllTMvXRa9BmOu_6k267rpjJctobL0m01n07fNLQkOje38x4dCmx3Lx2xJIqSGQlW6UpznE_RqpWcOuffE_iXFWmtOlxmZ7UPXKad0JDccGxP_Y7tatGBMEtGuFMDjiERLm0QtEqSTgnSQRLoURru0qtob2ovPkimtFuTvC4MvUGp6KaUO5Qszn4EetgNTn2vm1XhkPrBrGY3nNq3NzPft5_3m78FWxYVOTySnISJzi9YIaRza3ai6pKZYZBttzkWanSNNO9quTVaSWPLzTKmaS8O8RFTkdsh4ixjgYolVfVe5y3Xtmvv_Vl9PcHRBe4RjRNZeOdYyJZodp6CD0YlAvOUUJTAPOIsSULEdRU4A3LY1G4htvWLRrUQLf6FRr2M1us_OfPPa5f54sq8IpGisyGTspAkGMY4OshUxECrDI7mbS6Q81pxOwOYHic52mKIOEZaYjnMvAcfQNmJTfBWBFKUicwy2ifKMZYejHb753AtezVprLLeq1Zsey5u6e_o9KeFKNMkw981a9LItlbVX0GKjtW6EjpXNBj1ZohiKFAIc8FJVo01wFHp4welhzYNlH1AOfrHK_LkeBiOEyTIUTkBBa_yjDHaKR9L7ZTPWpXlk4G8bEC6vFdFb6Np5_qiBq_n2zl6rXDIBcci2reWrmp5TOM7Wbno0qcvBfeqNNptUc_HJdzrzoR7SPp3_7CkN79046DfduaxsJI0v6q0lA0ChYi6zgZnodakl0a57reo6kmr6lr_dDbkyi7gXcm3Pzj57u1u_N7ovlKJ7Bwn22tFbNP0R3XhevdDX4JQ7vA_8ebfsTlaOoN_3b1RU8o96PcX_czlZ_5G9uqSQRDLE4YXzPg6tjuDoQvO81inWG0WWreB_pGn_beumiggW2JVnX3ESYsOrRF8jFIbJdkSI_Am44AbjPovNXZVK_G1j2Z09CMQuZYRbSHyBRpGaLwUl_m6Vqcm4WqGlaWouZE0z3O9DeNaY3i2392JEBhzpJCnxipXNTwfs_RjgWKsqPedb_iRiQe5oS_F0a2XvSBV0_ItAeHY1zBTzoN6oixPQ4ufOMDGD8ao6Y3OJCyjwosLG5u2_FGsGtV5UD0LUZF3z77RHdlE3j8Se36NlZi8LnzGkjZQltat0FsN9F6M6ROfPA5tI_BZxAWQjBqnZVcm8etkbjvCoEfg_ESCCYJ0x8etBOOn_4rS6ipuUIpErnNmlaCQY4zcTllnjb_vO_eZuvWPfg03_zWxw2l7sfiYbdCDTsaroOWXxxLWez0daiuIpqWrt9C5wYP8SEmzUG89UBqk3z9pQFqY4t26Epf19naceJLvM4iaHAaSab4FD3h2Vc3vJqw4xc5jPQDX9FWRMMgA_BqBfLcilPuf9VKxNXCM1r1Pxh9TFpGPXbFFKkksACgJTRpnaADPwz7_yiiD_8C7IkF8Bn7FNGEzUI3o8OjOofuysbHsk_WqHye_CACil9Mec3wjjlTmgyccHojQ_XhQPSoWogSlMu7ZL2SNxThR8fGIuSARStp3eJdJiAYsPmeIxjj_9LBFSY4NZNvWX3y0LlCoZqVUFHgvVQPeA7c0_tpRe7SRLmRe2rE5nXuk3nH_8Vb70z_MyUtsrOmIKBAXTmQZker1mFv52hhHjJsbhwWNMZea_L6kordD5yftCYKSVaO1BLFy284Nb10k1unVNvNANZuGovzXuH3jp_ffNmtK4VS3LWptj5xudjtKvNH9gHvbLzevEYzuErCeirHj_GRlnq8nlq0Zfvdx5dA7_fk7Xf28rK3qvneuNboKCI6blD438b01u_7IjvklqXPf_K5Tpspa_zp_GfW_PSQMo5lBvLw2wx0jzrgS8gQj2G0J1nr0ILG7NOD4EUZS6uTlL7VHbpNKDZc5i0K3T66QUof_quISxL1845redB_l8CerY_c7VP2lNDJUl6KlD_WOp7U8Wh52P0lxTFBII84xhQgGgN5zF1ur83zszxOlQSdXyi557oN11T5_dL8f7rD3ldbqlNNqivPzols80y2Ed9er1qdZBxvMc8hx3ER4RimrGxjyucOxTZ_AVH9oIzUv60J8NQ9uzbOLwe2U7yFiKf1poGI1Xcjohxi685adLFqwtCkUX1B7fXd6_8AQ2w1Uqg4AAA_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(2816);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FNotificationToast_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1bSY_bOBa_51cQBTRgJ2aN5K1cChLkNLe_zFxmMGg0KIm2GUukQlFealD_fcBFMkUtlpxKpxsYpOEuWeTjWz6_jfSXA75sOUpxDn73v0bnU7rwwH_fAeD9MgO_5_2iHgAQHNF8y3gaAM4EEnjixXg3_fgOgNd3APhy_EJ_rOTHk_x47pkM_dr0uRy_lB9r_bHpmWlPfH33pcm_r6ZuOUubNNSfCRL4X5PlysvOig4ALEMREZcAeCVDgvVO9tyJfsnQY8nGXBHIWE4EYTQAW3LGsRwVMiFYGoD5MjvLZ052e3F9PJFY7AOw2Hj6OUVnuMd6TISSaHJEfALhkeBTxrgw75St4uN_CiBYboxgIYoOO84KGgdAz_rdf8Yvc_7yrN8zHmMegHl2BjlLSGxGZZwwTsQFRixhfObMTafWZMhRTIo8AP5csxuyM8z3KGanAHhgk53BQpLnuxBNvBkw_z0upzPgqX9_dnYWmK_UAi_Q0BifA_B7G2UXdsR8m0jCexLHmMrv_sxTNw6iJEXaFFekeI_LHERFSCIY4heC_cR7XBoW5zPgT6s1jBVRkgyZZOFgoXBwC4M2Aq3JSzXZQGK_boGENHaXcqKC54wHIGOECsxbrFaCrsSkv5bPFgMrm4GlxqT1eq1exyTPEnQJwDbBihxKyI5CInCaByDC5eI7lF2BkqE4JnSn1zQfc_OXC96EUIw43Em2MRUTwQzHs6GInWm1Z4hjKpzNAXPyggPt7Jbu4pDjDCMRAMrMn5YaK7U5G6hcGE8dazqKu_rAM_K3EKeMYlch_juL9JN2eowKI01pWam6CixqI0gjwXzPCT0Yh9eyNTzgPa5ygFGOIaGQFaJLksbKpQEtqg12N3pSorb3xzYIKS5jwnGkSUQsKVJaochIlxIKDTjrG_d5PDA3DrafQS44o7uGfCu9tvrqZDS79pQmFfxcZ6u90WlPBIZ5hiIsFXLiKOt0avgs4PUFThKS5ST_WFdZp_yWcTplWDjCosYIr1tKxZ_l0YoswzxCuYJpgoXAXAmqoO09rpwNL3f5ugb10h01dl9z67uBZDWdNYLLSu73EaHMWE1ZqMdU7sZpVzmqI68B_fBOZNatY1xFK97mnV4nvMFbdEVBafJVD7CdSBc3MDSCS6wmN33cNUspn20nt_lycp1LtkbFCpqelYGVxEu65fNQ030tckG2EnlUYCrsV_2wb_YZvnHBN9CHg730Fg0N1qXfTDsV43UiBjdsuqmlqXPPUZKbIWydQGeH_NLFwUsAUCGYm9ssPJeaxdn2Bpa1z0sR3xFqDGuL0RoeH1djnHjGMSx9w4nxGIYco0MA1P_g_KbG0L7GkMyMVQI8dmeP9B1lVfEG0ceShbQ7MdtbNU1_bD2gGDZPfkTuWH9r26CIaNllTMvXRa9BmOu_6k267rpjJctobL0m01n07fNLQkOje38x4dCmx3Lx2xJIqSGQlW6UpznE_RqpWcOuffE_iXFWmtOlxmZ7UPXKad0JDccGxP_Y7tatGBMEtGuFMDjiERLm0QtEqSTgnSQRLoURru0qtob2ovPkimtFuTvC4MvUGp6KaUO5Qszn4EetgNTn2vm1XhkPrBrGY3nNq3NzPft5_3m78FWxYVOTySnISJzi9YIaRza3ai6pKZYZBttzkWanSNNO9quTVaSWPLzTKmaS8O8RFTkdsh4ixjgYolVfVe5y3Xtmvv_Vl9PcHRBe4RjRNZeOdYyJZodp6CD0YlAvOUUJTAPOIsSULEdRU4A3LY1G4htvWLRrUQLf6FRr2M1us_OfPPa5f54sq8IpGisyGTspAkGMY4OshUxECrDI7mbS6Q81pxOwOYHic52mKIOEZaYjnMvAcfQNmJTfBWBFKUicwy2ifKMZYejHb753AtezVprLLeq1Zsey5u6e_o9KeFKNMkw981a9LItlbVX0GKjtW6EjpXNBj1ZohiKFAIc8FJVo01wFHp4welhzYNlH1AOfrHK_LkeBiOEyTIUTkBBa_yjDHaKR9L7ZTPWpXlk4G8bEC6vFdFb6Np5_qiBq_n2zl6rXDIBcci2reWrmp5TOM7Wbno0qcvBfeqNNptUc_HJdzrzoR7SPp3_7CkN79046DfduaxsJI0v6q0lA0ChYi6zgZnodakl0a57reo6kmr6lr_dDbkyi7gXcm3Pzj57u1u_N7ovlKJ7Bwn22tFbNP0R3XhevdDX4JQ7vA_8ebfsTlaOoN_3b1RU8o96PcX_czlZ_5G9uqSQRDLE4YXzPg6tjuDoQvO81inWG0WWreB_pGn_beumiggW2JVnX3ESYsOrRF8jFIbJdkSI_Am44AbjPovNXZVK_G1j2Z09CMQuZYRbSHyBRpGaLwUl_m6Vqcm4WqGlaWouZE0z3O9DeNaY3i2392JEBhzpJCnxipXNTwfs_RjgWKsqPedb_iRiQe5oS_F0a2XvSBV0_ItAeHY1zBTzoN6oixPQ4ufOMDGD8ao6Y3OJCyjwosLG5u2_FGsGtV5UD0LUZF3z77RHdlE3j8Se36NlZi8LnzGkjZQltat0FsN9F6M6ROfPA5tI_BZxAWQjBqnZVcm8etkbjvCoEfg_ESCCYJ0x8etBOOn_4rS6ipuUIpErnNmlaCQY4zcTllnjb_vO_eZuvWPfg03_zWxw2l7sfiYbdCDTsaroOWXxxLWez0daiuIpqWrt9C5wYP8SEmzUG89UBqk3z9pQFqY4t26Epf19naceJLvM4iaHAaSab4FD3h2Vc3vJqw4xc5jPQDX9FWRMMgA_BqBfLcilPuf9VKxNXCM1r1Pxh9TFpGPXbFFKkksACgJTRpnaADPwz7_yiiD_8C7IkF8Bn7FNGEzUI3o8OjOofuysbHsk_WqHye_CACil9Mec3wjjlTmgyccHojQ_XhQPSoWogSlMu7ZL2SNxThR8fGIuSARStp3eJdJiAYsPmeIxjj_9LBFSY4NZNvWX3y0LlCoZqVUFHgvVQPeA7c0_tpRe7SRLmRe2rE5nXuk3nH_8Vb70z_MyUtsrOmIKBAXTmQZker1mFv52hhHjJsbhwWNMZea_L6kordD5yftCYKSVaO1BLFy284Nb10k1unVNvNANZuGovzXuH3jp_ffNmtK4VS3LWptj5xudjtKvNH9gHvbLzevEYzuErCeirHj_GRlnq8nlq0Zfvdx5dA7_fk7Xf28rK3qvneuNboKCI6blD438b01u_7IjvklqXPf_K5Tpspa_zp_GfW_PSQMo5lBvLw2wx0jzrgS8gQj2G0J1nr0ILG7NOD4EUZS6uTlL7VHbpNKDZc5i0K3T66QUof_quISxL1845redB_l8CerY_c7VP2lNDJUl6KlD_WOp7U8Wh52P0lxTFBII84xhQgGgN5zF1ur83zszxOlQSdXyi557oN11T5_dL8f7rD3ldbqlNNqivPzols80y2Ed9er1qdZBxvMc8hx3ER4RimrGxjyucOxTZ_AVH9oIzUv60J8NQ9uzbOLwe2U7yFiKf1poGI1Xcjohxi685adLFqwtCkUX1B7fXd6_8AQ2w1Uqg4AAA_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FNotificationToast_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1bSY_bOBa_51cQBTRgJ2aN5K1cChLkNLe_zFxmMGg0KIm2GUukQlFealD_fcBFMkUtlpxKpxsYpOEuWeTjWz6_jfSXA75sOUpxDn73v0bnU7rwwH_fAeD9MgO_5_2iHgAQHNF8y3gaAM4EEnjixXg3_fgOgNd3APhy_EJ_rOTHk_x47pkM_dr0uRy_lB9r_bHpmWlPfH33pcm_r6ZuOUubNNSfCRL4X5PlysvOig4ALEMREZcAeCVDgvVO9tyJfsnQY8nGXBHIWE4EYTQAW3LGsRwVMiFYGoD5MjvLZ052e3F9PJFY7AOw2Hj6OUVnuMd6TISSaHJEfALhkeBTxrgw75St4uN_CiBYboxgIYoOO84KGgdAz_rdf8Yvc_7yrN8zHmMegHl2BjlLSGxGZZwwTsQFRixhfObMTafWZMhRTIo8AP5csxuyM8z3KGanAHhgk53BQpLnuxBNvBkw_z0upzPgqX9_dnYWmK_UAi_Q0BifA_B7G2UXdsR8m0jCexLHmMrv_sxTNw6iJEXaFFekeI_LHERFSCIY4heC_cR7XBoW5zPgT6s1jBVRkgyZZOFgoXBwC4M2Aq3JSzXZQGK_boGENHaXcqKC54wHIGOECsxbrFaCrsSkv5bPFgMrm4GlxqT1eq1exyTPEnQJwDbBihxKyI5CInCaByDC5eI7lF2BkqE4JnSn1zQfc_OXC96EUIw43Em2MRUTwQzHs6GInWm1Z4hjKpzNAXPyggPt7Jbu4pDjDCMRAMrMn5YaK7U5G6hcGE8dazqKu_rAM_K3EKeMYlch_juL9JN2eowKI01pWam6CixqI0gjwXzPCT0Yh9eyNTzgPa5ygFGOIaGQFaJLksbKpQEtqg12N3pSorb3xzYIKS5jwnGkSUQsKVJaochIlxIKDTjrG_d5PDA3DrafQS44o7uGfCu9tvrqZDS79pQmFfxcZ6u90WlPBIZ5hiIsFXLiKOt0avgs4PUFThKS5ST_WFdZp_yWcTplWDjCosYIr1tKxZ_l0YoswzxCuYJpgoXAXAmqoO09rpwNL3f5ugb10h01dl9z67uBZDWdNYLLSu73EaHMWE1ZqMdU7sZpVzmqI68B_fBOZNatY1xFK97mnV4nvMFbdEVBafJVD7CdSBc3MDSCS6wmN33cNUspn20nt_lycp1LtkbFCpqelYGVxEu65fNQ030tckG2EnlUYCrsV_2wb_YZvnHBN9CHg730Fg0N1qXfTDsV43UiBjdsuqmlqXPPUZKbIWydQGeH_NLFwUsAUCGYm9ssPJeaxdn2Bpa1z0sR3xFqDGuL0RoeH1djnHjGMSx9w4nxGIYco0MA1P_g_KbG0L7GkMyMVQI8dmeP9B1lVfEG0ceShbQ7MdtbNU1_bD2gGDZPfkTuWH9r26CIaNllTMvXRa9BmOu_6k267rpjJctobL0m01n07fNLQkOje38x4dCmx3Lx2xJIqSGQlW6UpznE_RqpWcOuffE_iXFWmtOlxmZ7UPXKad0JDccGxP_Y7tatGBMEtGuFMDjiERLm0QtEqSTgnSQRLoURru0qtob2ovPkimtFuTvC4MvUGp6KaUO5Qszn4EetgNTn2vm1XhkPrBrGY3nNq3NzPft5_3m78FWxYVOTySnISJzi9YIaRza3ai6pKZYZBttzkWanSNNO9quTVaSWPLzTKmaS8O8RFTkdsh4ixjgYolVfVe5y3Xtmvv_Vl9PcHRBe4RjRNZeOdYyJZodp6CD0YlAvOUUJTAPOIsSULEdRU4A3LY1G4htvWLRrUQLf6FRr2M1us_OfPPa5f54sq8IpGisyGTspAkGMY4OshUxECrDI7mbS6Q81pxOwOYHic52mKIOEZaYjnMvAcfQNmJTfBWBFKUicwy2ifKMZYejHb753AtezVprLLeq1Zsey5u6e_o9KeFKNMkw981a9LItlbVX0GKjtW6EjpXNBj1ZohiKFAIc8FJVo01wFHp4welhzYNlH1AOfrHK_LkeBiOEyTIUTkBBa_yjDHaKR9L7ZTPWpXlk4G8bEC6vFdFb6Np5_qiBq_n2zl6rXDIBcci2reWrmp5TOM7Wbno0qcvBfeqNNptUc_HJdzrzoR7SPp3_7CkN79046DfduaxsJI0v6q0lA0ChYi6zgZnodakl0a57reo6kmr6lr_dDbkyi7gXcm3Pzj57u1u_N7ovlKJ7Bwn22tFbNP0R3XhevdDX4JQ7vA_8ebfsTlaOoN_3b1RU8o96PcX_czlZ_5G9uqSQRDLE4YXzPg6tjuDoQvO81inWG0WWreB_pGn_beumiggW2JVnX3ESYsOrRF8jFIbJdkSI_Am44AbjPovNXZVK_G1j2Z09CMQuZYRbSHyBRpGaLwUl_m6Vqcm4WqGlaWouZE0z3O9DeNaY3i2392JEBhzpJCnxipXNTwfs_RjgWKsqPedb_iRiQe5oS_F0a2XvSBV0_ItAeHY1zBTzoN6oixPQ4ufOMDGD8ao6Y3OJCyjwosLG5u2_FGsGtV5UD0LUZF3z77RHdlE3j8Se36NlZi8LnzGkjZQltat0FsN9F6M6ROfPA5tI_BZxAWQjBqnZVcm8etkbjvCoEfg_ESCCYJ0x8etBOOn_4rS6ipuUIpErnNmlaCQY4zcTllnjb_vO_eZuvWPfg03_zWxw2l7sfiYbdCDTsaroOWXxxLWez0daiuIpqWrt9C5wYP8SEmzUG89UBqk3z9pQFqY4t26Epf19naceJLvM4iaHAaSab4FD3h2Vc3vJqw4xc5jPQDX9FWRMMgA_BqBfLcilPuf9VKxNXCM1r1Pxh9TFpGPXbFFKkksACgJTRpnaADPwz7_yiiD_8C7IkF8Bn7FNGEzUI3o8OjOofuysbHsk_WqHye_CACil9Mec3wjjlTmgyccHojQ_XhQPSoWogSlMu7ZL2SNxThR8fGIuSARStp3eJdJiAYsPmeIxjj_9LBFSY4NZNvWX3y0LlCoZqVUFHgvVQPeA7c0_tpRe7SRLmRe2rE5nXuk3nH_8Vb70z_MyUtsrOmIKBAXTmQZker1mFv52hhHjJsbhwWNMZea_L6kordD5yftCYKSVaO1BLFy284Nb10k1unVNvNANZuGovzXuH3jp_ffNmtK4VS3LWptj5xudjtKvNH9gHvbLzevEYzuErCeirHj_GRlnq8nlq0Zfvdx5dA7_fk7Xf28rK3qvneuNboKCI6blD438b01u_7IjvklqXPf_K5Tpspa_zp_GfW_PSQMo5lBvLw2wx0jzrgS8gQj2G0J1nr0ILG7NOD4EUZS6uTlL7VHbpNKDZc5i0K3T66QUof_quISxL1845redB_l8CerY_c7VP2lNDJUl6KlD_WOp7U8Wh52P0lxTFBII84xhQgGgN5zF1ur83zszxOlQSdXyi557oN11T5_dL8f7rD3ldbqlNNqivPzols80y2Ed9er1qdZBxvMc8hx3ER4RimrGxjyucOxTZ_AVH9oIzUv60J8NQ9uzbOLwe2U7yFiKf1poGI1Xcjohxi685adLFqwtCkUX1B7fXd6_8AQ2w1Uqg4AAA_22_7D__WEBPACK_IMPORTED_MODULE_1__);


var actionError = '_1jcxwm31m';
var actions = '_1jcxwm3i';
var approveButton = '_1jcxwm3n _1jcxwm3j';
var body = '_1jcxwm3f';
var chipRow = '_1jcxwm3x';
var closeButton = '_1jcxwm3e';
var closeButtonStacked = '_1jcxwm31b';
var collapsedChip = '_1jcxwm315';
var commandBlock = '_1jcxwm31h';
var commandText = '_1jcxwm31i';
var commandTextExpanded = '_1jcxwm31j';
var compactApprovalCard = '_1jcxwm31g';
var compactCard = '_1jcxwm31d';
var compactPreview = '_1jcxwm31f';
var compactTrailing = '_1jcxwm31e';
var decisionPair = '_1jcxwm31l';
var deckAction = '_1jcxwm312';
var deckBehindTray = '_1jcxwm31s';
var deckBesideTray = '_1jcxwm31t';
var deckHeader = '_1jcxwm311';
var deckPlacement = {desktop:'_1jcxwm3s _1jcxwm3r',desktopTopRight:'_1jcxwm3t _1jcxwm3r',mobileTop:'_1jcxwm3u _1jcxwm3r',mobileBottom:'_1jcxwm3v _1jcxwm3r'};
var denyButton = '_1jcxwm3o _1jcxwm3j';
var disclosureButton = '_1jcxwm31k';
var dismissButton = '_1jcxwm3l _1jcxwm3j';
var exiting = '_1jcxwm33';
var focusButton = '_1jcxwm3m _1jcxwm3j';
var header = '_1jcxwm36';
var icon = '_1jcxwm37';
var inlineAction = '_1jcxwm31n';
var message = '_1jcxwm3g';
var minimizeHint = '_1jcxwm3p';
var minimized = '_1jcxwm34';
var offlineHint = '_1jcxwm310';
var overflowChip = '_1jcxwm3y';
var overflowMenu = '_1jcxwm31o';
var overflowTrigger = '_1jcxwm31p';
var repeatBadge = '_1jcxwm3z';
var reveal = '_1jcxwm319';
var revealEnd = '_1jcxwm31a';
var riskLabel = '_1jcxwm31q';
var sourceApp = '_1jcxwm3c';
var subtitleRow = '_1jcxwm3b';
var swipeCard = '_1jcxwm317';
var swipeCardDragging = '_1jcxwm318';
var swipeRow = '_1jcxwm316';
var timestamp = '_1jcxwm3d';
var titleRow = '_1jcxwm39';
var titleWrapper = '_1jcxwm38';
var toast = '_1jcxwm32';
var toastApproval = '_1jcxwm35';
var toastStacked = '_1jcxwm3w';
var trayButton = '_1jcxwm31c';
var trayEntry = '_1jcxwm31r';
var typeLabel = '_1jcxwm3a';
var undoAction = '_1jcxwm314';
var undoBar = '_1jcxwm313';
var undoButton = '_1jcxwm3q _1jcxwm3j';
var viewButton = '_1jcxwm3k _1jcxwm3j';
var workingDir = '_1jcxwm3h';

/***/ }),

/***/ 18969:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   k: () => (/* binding */ riskLevelRank),
/* harmony export */   p: () => (/* binding */ RISK_LEVEL_LABELS)
/* harmony export */ });
// Mirrors classifier.RiskLevel (pkg/classifier/classifier.go). Keep in sync by hand — there is
// no codegen bridge for this Go string-constant set today, matching escalationCategory.ts's
// existing convention.
// riskLevelRank orders RiskLevel strings by severity, highest first. Unrecorded ("") ranks
// alongside "high" — fail-safe, since an unclassified request must never sort as if it were
// safe. Mirrors session/review_queue_poller.go's riskLevelRank (Go side).
const RISK_LEVEL_RANK = {
    critical: 4,
    high: 3,
    "": 3,
    medium: 2,
    low: 1
};
function riskLevelRank(riskLevel) {
    var _RISK_LEVEL_RANK_riskLevel;
    return (_RISK_LEVEL_RANK_riskLevel = RISK_LEVEL_RANK[riskLevel]) !== null && _RISK_LEVEL_RANK_riskLevel !== void 0 ? _RISK_LEVEL_RANK_riskLevel : RISK_LEVEL_RANK[""];
}
// Single source for the 4 base human-readable labels — consumed by both SeverityBadge.tsx
// (which layers its own "Severity not recorded" fallback on top for "") and
// ApprovalAnalyticsPanel.tsx (which falls back to the raw string for forward-compat with an
// unrecognized future level). Keep the fallback logic at each call site, not here.
const RISK_LEVEL_LABELS = {
    critical: "Critical",
    high: "High",
    medium: "Medium",
    low: "Low"
};


/***/ }),

/***/ 20802:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   _I: () => (/* binding */ markNotified),
/* harmony export */   aO: () => (/* binding */ markAcknowledged),
/* harmony export */   lx: () => (/* binding */ shouldNotify),
/* harmony export */   o3: () => (/* binding */ markNotifiedBatch),
/* harmony export */   wt: () => (/* binding */ cleanupExpired)
/* harmony export */ });
/* unused harmony exports getNotifiedSessions, getAcknowledgedSessions, clearAll */
/**
 * notificationStorage.ts
 *
 * Manages localStorage persistence for notification deduplication and acknowledgment tracking.
 * Prevents duplicate notifications on WebSocket reconnection and page refresh.
 *
 * Key features:
 * - Track notified sessions to prevent re-notification on snapshot events
 * - Track acknowledged sessions with grace period to prevent immediate re-notification
 * - Automatic cleanup of expired records to prevent localStorage quota issues
 */ /** Record of a notification shown to the user */ const STORAGE_KEY = "stapler-squad-notifications";
/** Time-to-live for notification records (1 hour) */ const NOTIFICATION_TTL_MS = 60 * 60 * 1000;
/** Grace period after acknowledgment before re-notifying (5 minutes) */ const GRACE_PERIOD_MS = 5 * 60 * 1000;
/**
 * Safely retrieve notification records from localStorage.
 * Returns empty map on parse errors or missing data.
 */ function getNotifiedSessions() {
    try {
        const data = localStorage.getItem(STORAGE_KEY);
        if (!data) return new Map();
        const records = JSON.parse(data);
        return new Map(records.map((r)=>[
                r.sessionId,
                r
            ]));
    } catch (e) {
        // Parse error or localStorage unavailable
        return new Map();
    }
}
/**
 * Save notification records to localStorage.
 * Automatically cleans up expired records before saving.
 */ function saveRecords(records) {
    try {
        // Clean up expired records before saving
        const now = Date.now();
        for (const [sessionId, record] of records){
            if (now - record.notifiedAt > NOTIFICATION_TTL_MS) {
                records.delete(sessionId);
            }
        }
        const data = Array.from(records.values());
        localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
    } catch (e) {
        // localStorage unavailable or quota exceeded
        console.warn("[notificationStorage] Failed to save notification records");
    }
}
/**
 * Mark a session as notified.
 * Called when a notification is shown to the user.
 */ function markNotified(sessionId) {
    const records = getNotifiedSessions();
    const existing = records.get(sessionId);
    records.set(sessionId, {
        sessionId,
        notifiedAt: Date.now(),
        // Preserve acknowledgment if it exists
        acknowledgedAt: existing === null || existing === void 0 ? void 0 : existing.acknowledgedAt
    });
    saveRecords(records);
}
/**
 * Mark a session as acknowledged (dismissed/snoozed).
 * The grace period prevents re-notification for GRACE_PERIOD_MS.
 */ function markAcknowledged(sessionId) {
    const records = getNotifiedSessions();
    const existing = records.get(sessionId);
    var _existing_notifiedAt;
    records.set(sessionId, {
        sessionId,
        notifiedAt: (_existing_notifiedAt = existing === null || existing === void 0 ? void 0 : existing.notifiedAt) !== null && _existing_notifiedAt !== void 0 ? _existing_notifiedAt : Date.now(),
        acknowledgedAt: Date.now()
    });
    saveRecords(records);
}
/**
 * Check if a notification should be shown for a session.
 *
 * Returns false if:
 * - Session was already notified and is still in TTL window
 * - Session was acknowledged within grace period
 *
 * Returns true if:
 * - Never notified before
 * - Previous notification expired (> TTL)
 * - Acknowledgment expired (> grace period)
 */ function shouldNotify(sessionId) {
    const records = getNotifiedSessions();
    const record = records.get(sessionId);
    if (!record) {
        // Never notified - should notify
        return true;
    }
    const now = Date.now();
    // Check acknowledgment grace period first
    if (record.acknowledgedAt) {
        const timeSinceAck = now - record.acknowledgedAt;
        if (timeSinceAck < GRACE_PERIOD_MS) {
            // Still within grace period - don't notify
            return false;
        }
    }
    // Check if previous notification has expired
    const timeSinceNotified = now - record.notifiedAt;
    if (timeSinceNotified > NOTIFICATION_TTL_MS) {
        // Notification expired - can notify again
        return true;
    }
    // Already notified within TTL - don't duplicate
    return false;
}
/**
 * Get all sessions that are currently acknowledged (within grace period).
 */ function getAcknowledgedSessions() {
    const records = getNotifiedSessions();
    const acknowledged = new Set();
    const now = Date.now();
    for (const [sessionId, record] of records){
        if (record.acknowledgedAt) {
            const timeSinceAck = now - record.acknowledgedAt;
            if (timeSinceAck < GRACE_PERIOD_MS) {
                acknowledged.add(sessionId);
            }
        }
    }
    return acknowledged;
}
/**
 * Remove all expired notification records.
 * Called automatically by saveRecords, but can be called manually.
 */ function cleanupExpired() {
    const records = getNotifiedSessions();
    const now = Date.now();
    for (const [sessionId, record] of records){
        if (now - record.notifiedAt > NOTIFICATION_TTL_MS) {
            records.delete(sessionId);
        }
    }
    saveRecords(records);
}
/**
 * Clear all notification records.
 * Useful for testing or resetting state.
 */ function clearAll() {
    try {
        localStorage.removeItem(STORAGE_KEY);
    } catch (e) {
    // localStorage unavailable
    }
}
/**
 * Mark multiple sessions as notified (batch operation).
 * More efficient than calling markNotified multiple times.
 */ function markNotifiedBatch(sessionIds) {
    if (sessionIds.length === 0) return;
    const records = getNotifiedSessions();
    const now = Date.now();
    for (const sessionId of sessionIds){
        const existing = records.get(sessionId);
        records.set(sessionId, {
            sessionId,
            notifiedAt: now,
            acknowledgedAt: existing === null || existing === void 0 ? void 0 : existing.acknowledgedAt
        });
    }
    saveRecords(records);
}


/***/ }),

/***/ 23925:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   RI: () => (/* binding */ useReviewedMarks),
/* harmony export */   "if": () => (/* binding */ markKeyOf)
/* harmony export */ });
/* unused harmony export resetReviewedMarksForTests */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(24044);
/* __next_internal_client_entry_do_not_use__ markKeyOf,resetReviewedMarksForTests,useReviewedMarks auto */ 

const MARKS_KEY = "ssq.background.reviewed";
const FIRST_RUN_KEY = "ssq.background.firstRunSeen";
const MAX_MARKS = 1000;
let store = null;
const listeners = new Set();
function read() {
    try {
        const raw = window.localStorage.getItem(MARKS_KEY);
        const marks = new Set(raw ? JSON.parse(raw) : []);
        return {
            marks,
            firstRunSeen: window.localStorage.getItem(FIRST_RUN_KEY) === "1",
            storage: "local"
        };
    } catch (e) {
        return {
            marks: new Set(),
            firstRunSeen: false,
            storage: "session"
        };
    }
}
function current() {
    store !== null && store !== void 0 ? store : store = read();
    return store;
}
function commit(next) {
    let storage = "local";
    try {
        window.localStorage.setItem(MARKS_KEY, JSON.stringify([
            ...next.marks
        ].slice(-MAX_MARKS)));
        if (next.firstRunSeen) window.localStorage.setItem(FIRST_RUN_KEY, "1");
    } catch (e) {
        storage = "session";
    }
    store = {
        ...next,
        storage
    };
    listeners.forEach((l)=>l());
}
/** A mark is per (row, last activity): a newer failure on the same session is unreviewed again. */ const markKeyOf = (row)=>"".concat(row.key, "@").concat(row.timestampMs);
/** Test seam: forget the in-memory copy so the next read goes back to storage, as a reload does. */ function resetReviewedMarksForTests() {
    store = null;
    listeners.forEach((l)=>l());
}
/**
 * Reviewed marks live in this browser only (no server write, ADR-001); when storage throws they
 * last for the page session. State is shared by every caller so the tab badge and the list agree.
 */ function useReviewedMarks() {
    const snap = (0,react__WEBPACK_IMPORTED_MODULE_0__.useSyncExternalStore)((l)=>{
        listeners.add(l);
        return ()=>listeners.delete(l);
    }, current, current);
    const analytics = (0,react__WEBPACK_IMPORTED_MODULE_0__.useContext)(_lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_1__/* .AnalyticsContext */ .Ig);
    const mark = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(function(keys) {
        let options = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
        const before = current();
        commit({
            ...before,
            marks: new Set([
                ...before.marks,
                ...keys
            ])
        });
        if (options.track === false) return;
        analytics === null || analytics === void 0 ? void 0 : analytics.track({
            name: "background_reviewed_mark_total",
            category: "user_action",
            component: "BackgroundActivity",
            labels: {
                storage: current().storage
            }
        });
    }, [
        analytics
    ]);
    const dismissFirstRun = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>commit({
            ...current(),
            firstRunSeen: true
        }), []);
    return {
        isReviewed: (key)=>snap.marks.has(key),
        mark,
        firstRunSeen: snap.firstRunSeen,
        dismissFirstRun,
        storage: snap.storage
    };
}


/***/ }),

/***/ 26657:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  J: () => (/* binding */ useSwipeToDismiss)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/window/windowGestureConstants.ts
var windowGestureConstants = __webpack_require__(2113);
;// ./src/lib/hooks/swipeToDismissMachine.ts

/**
 * Pure state machine for swipe-to-dismiss on a toast or tray row. No React and no
 * DOM: it reduces touch samples to a drag offset and, on release, an outcome.
 *
 *  - A horizontal drag locks after GESTURE_MOVEMENT_THRESHOLD_PX of travel.
 *  - Vertical drift of the same threshold cancels at any time, so the page or
 *    list scrolls instead.
 *  - Release dismisses when the drag passed DISMISS_FRACTION of the row's width,
 *    or was a fling: at least FLING_MIN_PX at FLING_MIN_VELOCITY px/ms.
 */ const VERTICAL_CANCEL_PX = windowGestureConstants/* GESTURE_MOVEMENT_THRESHOLD_PX */.j;
const DISMISS_FRACTION = 0.4;
const FLING_MIN_PX = 60;
const FLING_MIN_VELOCITY = 0.3;
/** A touch starting this close to a screen edge is the OS back gesture, never a dismiss. */ const EDGE_GUARD_PX = 30;
/** Past this the reveal layer ("Dismiss" or "Move to tray") shows behind the row. */ const REVEAL_PX = 24;
function startsInEdgeGuard(x, viewportWidth) {
    return x < EDGE_GUARD_PX || x > viewportWidth - EDGE_GUARD_PX;
}
const IDLE_SWIPE = {
    phase: "idle",
    startX: 0,
    startY: 0,
    startT: 0,
    width: 0
};
function finish(outcome) {
    return {
        state: IDLE_SWIPE,
        offset: 0,
        outcome
    };
}
function swipeStep(state, sample) {
    if (sample.type === "start") {
        return {
            state: {
                phase: "tracking",
                startX: sample.x,
                startY: sample.y,
                startT: sample.t,
                width: sample.width
            },
            offset: 0
        };
    }
    if (sample.type === "cancel") {
        return finish(state.phase === "dragging" ? "cancel" : undefined);
    }
    if (state.phase === "idle" || state.phase === "done") return {
        state,
        offset: 0
    };
    const dx = sample.x - state.startX;
    const dy = sample.y - state.startY;
    if (Math.abs(dy) >= VERTICAL_CANCEL_PX) return {
        state: {
            ...state,
            phase: "done"
        },
        offset: 0,
        outcome: "cancel"
    };
    if (sample.type === "move") {
        if (state.phase === "tracking" && Math.abs(dx) < windowGestureConstants/* GESTURE_MOVEMENT_THRESHOLD_PX */.j) return {
            state,
            offset: 0
        };
        return {
            state: {
                ...state,
                phase: "dragging"
            },
            offset: dx
        };
    }
    // end
    if (state.phase !== "dragging") return finish();
    const elapsed = Math.max(1, sample.t - state.startT);
    const farEnough = Math.abs(dx) >= state.width * DISMISS_FRACTION;
    const fling = Math.abs(dx) >= FLING_MIN_PX && Math.abs(dx) / elapsed >= FLING_MIN_VELOCITY;
    return finish(farEnough || fling ? "dismiss" : "cancel");
}

;// ./src/lib/hooks/useSwipeToDismiss.ts
/* __next_internal_client_entry_do_not_use__ useSwipeToDismiss auto */ 

const NO_SWIPE = {
    offset: 0,
    dragging: false,
    revealed: false
};
/**
 * Wires touch events on `ref` to the swipe machine.
 *
 * Touch events that start on the row stop propagating at the row, so the terminal
 * gesture machine's document-level touchmove/touchend listeners never see them
 * (AWAITING OPERATOR: Spike 1.2 has not recorded whether that listener double
 * handles such a swipe on a real device; stopping here is the safe default).
 * Vertical scrolling stays with the browser through `touch-action: pan-y` on the row.
 * Callbacks live in refs, so changing them never re-binds the listeners.
 */ function useSwipeToDismiss(ref, param) {
    let { onDismiss, disabled = false } = param;
    const [view, setView] = (0,react.useState)(NO_SWIPE);
    const onDismissRef = (0,react.useRef)(onDismiss);
    onDismissRef.current = onDismiss;
    (0,react.useEffect)(()=>{
        const el = ref.current;
        if (!el || disabled) return;
        let state = IDLE_SWIPE;
        let touching = false;
        const apply = (sample)=>{
            const step = swipeStep(state, sample);
            state = step.state;
            const dragging = state.phase === "dragging";
            setView({
                offset: step.offset,
                dragging,
                revealed: dragging && Math.abs(step.offset) >= REVEAL_PX
            });
            if (step.outcome === "dismiss") onDismissRef.current();
        };
        const point = (e)=>{
            var _e_touches_;
            return (_e_touches_ = e.touches[0]) !== null && _e_touches_ !== void 0 ? _e_touches_ : e.changedTouches[0];
        };
        const onStart = (e)=>{
            const p = point(e);
            if (!p || startsInEdgeGuard(p.clientX, window.innerWidth)) return;
            e.stopPropagation();
            touching = true;
            apply({
                type: "start",
                x: p.clientX,
                y: p.clientY,
                t: e.timeStamp,
                width: el.getBoundingClientRect().width
            });
        };
        const onMove = (e)=>{
            if (!touching) return;
            e.stopPropagation();
            const p = point(e);
            if (p) apply({
                type: "move",
                x: p.clientX,
                y: p.clientY,
                t: e.timeStamp
            });
        };
        const onEnd = (e)=>{
            if (!touching) return;
            e.stopPropagation();
            touching = false;
            const p = point(e);
            if (p) apply({
                type: "end",
                x: p.clientX,
                y: p.clientY,
                t: e.timeStamp
            });
        };
        const onCancel = (e)=>{
            if (!touching) return;
            e.stopPropagation();
            touching = false;
            apply({
                type: "cancel"
            });
        };
        el.addEventListener("touchstart", onStart, {
            passive: true
        });
        el.addEventListener("touchmove", onMove, {
            passive: true
        });
        el.addEventListener("touchend", onEnd, {
            passive: true
        });
        el.addEventListener("touchcancel", onCancel, {
            passive: true
        });
        return ()=>{
            el.removeEventListener("touchstart", onStart);
            el.removeEventListener("touchmove", onMove);
            el.removeEventListener("touchend", onEnd);
            el.removeEventListener("touchcancel", onCancel);
            setView(NO_SWIPE);
        };
    }, [
        ref,
        disabled
    ]);
    return view;
}


/***/ }),

/***/ 35172:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $g: () => (/* binding */ TOAST_STALE_MS),
/* harmony export */   B2: () => (/* binding */ TOAST_DEDUP_WINDOW_MS),
/* harmony export */   Lx: () => (/* binding */ partitionToasts),
/* harmony export */   T4: () => (/* binding */ isPinned),
/* harmony export */   TM: () => (/* binding */ hasLongToastLifetime),
/* harmony export */   o$: () => (/* binding */ NATIVE_MEDIUM_TTL_MS),
/* harmony export */   tx: () => (/* binding */ DEFAULT_TOAST_MS),
/* harmony export */   vW: () => (/* binding */ toastCapFor),
/* harmony export */   v_: () => (/* binding */ NATIVE_ACTIONABLE_TTL_MS),
/* harmony export */   vk: () => (/* binding */ nativeAutoCloseMs),
/* harmony export */   wu: () => (/* binding */ ACTIONABLE_TOAST_STALE_MS),
/* harmony export */   zZ: () => (/* binding */ toastAutoCloseMs)
/* harmony export */ });
/* unused harmony exports NATIVE_HIGH_TTL_MS, DESKTOP_TOAST_CAP, MOBILE_TOAST_CAP */
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
/**
 * Notification lifecycle policy.
 *
 * Centralises all rules about when toasts appear, minimize, and expire.
 * Components and the context import from here — business rules live in
 * exactly one place.
 */ 
/** Non-actionable toasts are swept from the active list after 5 minutes. */ const TOAST_STALE_MS = 5 * 60 * 1000;
/**
 * Deduplication window for toasts: suppress a second toast for the same
 * (sessionId, notificationType) if one was shown within this period.
 * Actionable types (approval_needed, question) are never suppressed.
 */ const TOAST_DEDUP_WINDOW_MS = 10000;
/**
 * Actionable toasts (approval_needed, question) are swept after 6 minutes.
 * They stay in the notification history panel regardless.
 */ const ACTIONABLE_TOAST_STALE_MS = 6 * 60 * 1000;
/**
 * Toast staleness only: approval_needed and question toasts outlive the
 * 5-minute sweep. Whether a notification is a pending *decision* is the
 * server-sent `isPendingDecision`, never this type check.
 */ function hasLongToastLifetime(type) {
    return type === "approval_needed" || type === "question";
}
/**
 * Default (non-actionable, non-error) toast auto-close duration in ms.
 * Named separately from `toastAutoCloseMs`'s default branch so callers that
 * have no `NotificationData["notificationType"]` to pass — e.g.
 * `InputDropBadge` (Story 2.3), which is a terminal-local transient UI
 * signal, not a session `NotificationData` member — can reference this
 * value directly instead of forcing an unrelated/misleading type argument
 * just to extract a millisecond constant. `toastAutoCloseMs`'s default case
 * references this constant so the two values can't silently drift.
 */ const DEFAULT_TOAST_MS = 8000;
/**
 * How long (ms) before a toast auto-closes via the component timer.
 * Actionable types use the full ACTIONABLE_TOAST_STALE_MS so they remain
 * visible until resolved, or until the 6-minute fallback fires.
 */ function toastAutoCloseMs(type) {
    if (hasLongToastLifetime(type)) return ACTIONABLE_TOAST_STALE_MS;
    if (type === "error" || type === "task_failed") return 12000;
    return DEFAULT_TOAST_MS;
}
/** Auto-close delay for high/urgent native (OS) notifications (FR-3). */ const NATIVE_HIGH_TTL_MS = 30000;
/** Auto-close delay for medium/low native (OS) notifications (FR-3). */ const NATIVE_MEDIUM_TTL_MS = 15000;
/** Auto-close delay for actionable native (OS) notifications (approval_needed, input_required) (FR-3). */ const NATIVE_ACTIONABLE_TTL_MS = 5 * 60 * 1000;
/** Maps a NotificationPriority to the native notification auto-close TTL. */ function nativeAutoCloseMs(priority) {
    switch(priority){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationPriority */ .AE.URGENT:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationPriority */ .AE.HIGH:
            return NATIVE_HIGH_TTL_MS;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationPriority */ .AE.MEDIUM:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationPriority */ .AE.LOW:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationPriority */ .AE.UNSPECIFIED:
        default:
            return NATIVE_MEDIUM_TTL_MS;
    }
}
/**
 * A toast is pinned (it never auto-closes and is never cleared in bulk) exactly
 * when the server marked it a pending decision. Presentational policy only: the
 * server's `IsPendingDecision` is the one definition, so this reads the field
 * and never inspects types.
 */ function isPinned(toast) {
    return toast.isPendingDecision === true;
}
/**
 * Splits a deck (oldest first) into what the cap shows and what collapses to the
 * chip. Pinned toasts claim the slots first, in arrival order so a new pinned
 * arrival never shuffles a card under the user's thumb; any remaining slots go
 * to the newest non-pinned toasts.
 */ function partitionToasts(toasts, cap) {
    const pinned = toasts.filter(isPinned);
    const rest = toasts.filter((t)=>!isPinned(t));
    const shownPinned = pinned.slice(0, cap);
    const slotsLeft = Math.max(0, cap - shownPinned.length);
    const shownRest = slotsLeft > 0 ? rest.slice(-slotsLeft) : [];
    const visible = [
        ...shownPinned,
        ...shownRest
    ];
    return {
        visible,
        overflow: toasts.length - visible.length,
        pinnedCount: pinned.length
    };
}
const DESKTOP_TOAST_CAP = 3;
const MOBILE_TOAST_CAP = 1;
/**
 * How many toast cards the deck shows: 3 on desktop, 1 on a phone (portrait or
 * landscape), 0 with the soft keyboard open so only the chip is left. The rest
 * collapse into the "+N more" chip (ADR-009).
 */ function toastCapFor(viewport) {
    if (viewport.isVirtualKeyboardOpen) return 0;
    return viewport.isInnerScreen ? DESKTOP_TOAST_CAP : MOBILE_TOAST_CAP;
}


/***/ }),

/***/ 36475:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  l: () => (/* binding */ BackgroundChip)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FBackgroundChip.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xdWZ5MzF5MCB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBwYWRkaW5nOiAwIHZhcigtLV8xOWV6MnJ6MnopOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM5KTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNnKTsKICBsaW5lLWhlaWdodDogMS41OwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FBackgroundChip_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xdWZ5MzF5MCB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBwYWRkaW5nOiAwIHZhcigtLV8xOWV6MnJ6MnopOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM5KTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNnKTsKICBsaW5lLWhlaWdodDogMS41OwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0_22_7D = __webpack_require__(66485);
;// ./src/components/ui/BackgroundChip.css.ts


var chip = '_1ufy31y0';
;// ./src/components/ui/BackgroundChip.tsx


/** Text chip marking a notification whose session is hidden (Story 5.3, UX C6): text, not color alone. */ function BackgroundChip() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
        className: chip,
        "data-testid": "notification-background-chip",
        children: "Background"
    });
}


/***/ }),

/***/ 39823:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   V: () => (/* binding */ useHandedness)
/* harmony export */ });
/* unused harmony export HANDEDNESS_KEY */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ HANDEDNESS_KEY,useHandedness auto */ 
const HANDEDNESS_KEY = "stapler-squad:left-handed";
/**
 * Reads and persists the user's handedness preference.
 *
 * On mount (and on every change), sets `data-left-handed` on <html> so that
 * CSS can flip thumb-sensitive layouts with a single selector:
 *
 *   selectors: { ':root[data-left-handed] &': { flexDirection: 'row' } }
 *
 * Components that need to respond in JS can read `leftHanded` directly.
 */ function useHandedness() {
    const [leftHanded, setLeftHanded] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    // Read preference on mount (client-side only)
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        try {
            const stored = localStorage.getItem(HANDEDNESS_KEY) === "true";
            setLeftHanded(stored);
            applyToRoot(stored);
        } catch (e) {}
    }, []);
    const toggleHandedness = ()=>{
        const next = !leftHanded;
        setLeftHanded(next);
        try {
            localStorage.setItem(HANDEDNESS_KEY, String(next));
        } catch (e) {}
        applyToRoot(next);
    };
    return {
        leftHanded,
        toggleHandedness
    };
}
function applyToRoot(leftHanded) {
    if (typeof document === "undefined") return;
    if (leftHanded) {
        document.documentElement.dataset.leftHanded = "true";
    } else {
        delete document.documentElement.dataset.leftHanded;
    }
}


/***/ }),

/***/ 41364:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $Z: () => (/* binding */ PHONE_DEFAULT_Y),
/* harmony export */   $k: () => (/* binding */ DESKTOP_DEFAULT_Y_FRACTION_OF_VIEWPORT),
/* harmony export */   PG: () => (/* binding */ describeY),
/* harmony export */   Qc: () => (/* binding */ stepY),
/* harmony export */   Vf: () => (/* binding */ clampTop),
/* harmony export */   aY: () => (/* binding */ yToTop),
/* harmony export */   bW: () => (/* binding */ computeBounds),
/* harmony export */   jN: () => (/* binding */ parseBellPos),
/* harmony export */   p1: () => (/* binding */ movedPastThreshold),
/* harmony export */   qY: () => (/* binding */ topToY)
/* harmony export */ });
/* unused harmony exports BELL_EDGE_MARGIN_PX, KEY_STEP, KEY_STEP_LARGE, dragThreshold */
/** Pure geometry and parsing for the draggable notification bell (no DOM, no storage). */ const BELL_EDGE_MARGIN_PX = 8;
const DESKTOP_DEFAULT_Y_FRACTION_OF_VIEWPORT = 0.35;
const PHONE_DEFAULT_Y = 0.8;
const KEY_STEP = 0.05;
const KEY_STEP_LARGE = 0.2;
const MOUSE_THRESHOLD_PX = 6;
const TOUCH_THRESHOLD_PX = 10;
const clamp01 = (n)=>Math.min(1, Math.max(0, n));
/** Returns null for anything that is not a well-formed v1 record. */ function parseBellPos(raw) {
    if (!raw) return null;
    try {
        const o = JSON.parse(raw);
        if ((o === null || o === void 0 ? void 0 : o.v) !== 1 || o.edge !== "left" && o.edge !== "right") return null;
        if (typeof o.y !== "number" || !Number.isFinite(o.y)) return null;
        return {
            v: 1,
            edge: o.edge,
            y: clamp01(o.y)
        };
    } catch (e) {
        return null;
    }
}
function computeBounds(p) {
    const minTop = p.safeTop + BELL_EDGE_MARGIN_PX;
    const maxTop = p.viewportHeight - p.safeBottom - p.bottomInset - BELL_EDGE_MARGIN_PX - p.handleHeight;
    return {
        minTop,
        maxTop: Math.max(minTop, maxTop)
    };
}
function yToTop(y, b) {
    return b.minTop + clamp01(y) * (b.maxTop - b.minTop);
}
function topToY(top, b) {
    const range = b.maxTop - b.minTop;
    return range <= 0 ? 0 : clamp01((top - b.minTop) / range);
}
function clampTop(top, b) {
    return Math.min(b.maxTop, Math.max(b.minTop, top));
}
function dragThreshold(pointerType) {
    return pointerType === "mouse" ? MOUSE_THRESHOLD_PX : TOUCH_THRESHOLD_PX;
}
function movedPastThreshold(dx, dy, pointerType) {
    return Math.hypot(dx, dy) >= dragThreshold(pointerType);
}
function stepY(y, direction, large) {
    const step = large ? KEY_STEP_LARGE : KEY_STEP;
    return clamp01(y + (direction === "down" ? step : -step));
}
function describeY(y) {
    return y < 1 / 3 ? "top" : y < 2 / 3 ? "middle" : "bottom";
}


/***/ }),

/***/ 44865:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $: () => (/* binding */ DeckViewportContext),
/* harmony export */   T: () => (/* binding */ useDeckViewport)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ DeckViewportContext,useDeckViewport auto */ 
const DeckViewportContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.createContext)({
    isInnerScreen: true,
    isVirtualKeyboardOpen: false
});
function useDeckViewport() {
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useContext)(DeckViewportContext);
}


/***/ }),

/***/ 46765:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Hz: () => (/* binding */ knownHiddenSessions),
/* harmony export */   iB: () => (/* binding */ rememberHiddenSession)
/* harmony export */ });
/* unused harmony export resetHiddenSessionRegistry */
// Notification history records carry no hidden flag, so once a hidden session is deleted
// nothing server-side says it was hidden. The ids seen hidden in this tab are what keeps a
// failure row alive as "Session no longer available" (C7).
const known = new Map();
function rememberHiddenSession(ref) {
    known.set(ref.id, {
        id: ref.id,
        title: ref.title
    });
}
function knownHiddenSessions() {
    return Array.from(known.values());
}
/** Test seam. */ function resetHiddenSessionRegistry() {
    known.clear();
}


/***/ }),

/***/ 48469:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   H4: () => (/* binding */ readTerminalCursor),
/* harmony export */   Xn: () => (/* binding */ cursorCellRect),
/* harmony export */   p5: () => (/* binding */ registerTerminalCursorSource)
/* harmony export */ });
/**
 * Where the xterm cursor cell is on screen, for UI that must not cover the
 * line being typed on (the desktop toast deck). Terminals register a getter while
 * mounted; the reader picks the first one that reports a visible rectangle.
 */ const sources = new Set();
function registerTerminalCursorSource(source) {
    sources.add(source);
    return ()=>sources.delete(source);
}
/** "no-terminal": nothing to cover. "unreadable": a terminal is mounted but gave no rectangle. */ function readTerminalCursor() {
    if (sources.size === 0) return {
        kind: "no-terminal"
    };
    for (const source of sources){
        const rect = source();
        if (rect && rect.width > 0 && rect.height > 0) return {
            kind: "rect",
            rect
        };
    }
    return {
        kind: "unreadable"
    };
}
/** Cursor cell in viewport coordinates, from the `.xterm-screen` box and the cell grid. */ function cursorCellRect(terminal, screen) {
    if (!screen || terminal.cols <= 0 || terminal.rows <= 0) return null;
    // The pool keeps off-screen terminals mounted with visibility: hidden.
    if (screen instanceof Element && getComputedStyle(screen).visibility === "hidden") return null;
    const box = screen.getBoundingClientRect();
    if (box.width === 0 || box.height === 0) return null; // hidden (pooled terminal not on screen)
    const cellW = box.width / terminal.cols;
    const cellH = box.height / terminal.rows;
    const { cursorX, cursorY } = terminal.buffer.active;
    return {
        left: box.left + cursorX * cellW,
        top: box.top + cursorY * cellH,
        width: cellW,
        height: cellH
    };
}


/***/ }),

/***/ 62275:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  ph: () => (/* binding */ NotificationProvider),
  rN: () => (/* binding */ getFailureReasonToastMessage),
  E$: () => (/* reexport */ notificationContexts/* useNotifications */.E$)
});

// UNUSED EXPORTS: useNotificationCommands, useNotificationState

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
// EXTERNAL MODULE: ./src/lib/reply/replyQuestion.ts
var replyQuestion = __webpack_require__(15203);
// EXTERNAL MODULE: ./src/lib/hooks/useAuditLog.ts
var useAuditLog = __webpack_require__(13867);
// EXTERNAL MODULE: ./src/lib/hooks/useCoarsePointer.ts
var useCoarsePointer = __webpack_require__(69250);
// EXTERNAL MODULE: ./src/lib/hooks/useNowTicker.ts
var useNowTicker = __webpack_require__(70562);
// EXTERNAL MODULE: ./src/lib/hooks/useSwipeToDismiss.ts + 1 modules
var useSwipeToDismiss = __webpack_require__(26657);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionHidden.ts
var useSessionHidden = __webpack_require__(76468);
// EXTERNAL MODULE: ./src/components/ui/BackgroundChip.tsx + 1 modules
var BackgroundChip = __webpack_require__(36475);
;// ./src/lib/hooks/useDecisionFlow.ts
/* __next_internal_client_entry_do_not_use__ needsApproveConfirm,useDecisionFlow auto */ 
/** low and medium approve in one tap; high, critical and an unrecorded level ask first (fail safe). */ function needsApproveConfirm(riskLevel) {
    return riskLevel !== "low" && riskLevel !== "medium";
}
/**
 * The stacked card's Approve and Deny flow: both buttons disable together while a
 * request is in flight (a double tap sends one), a failure keeps the toast with
 * "Could not <verb> - Retry", and on a phone a risky approve asks first.
 */ function useDecisionFlow(args) {
    const { notification, onClose, confirmApprove, onBusyChange, onActionFailed } = args;
    const [phase, setPhase] = (0,react.useState)("idle");
    const [verb, setVerb] = (0,react.useState)("approve");
    const inFlight = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        onBusyChange === null || onBusyChange === void 0 ? void 0 : onBusyChange(phase === "sending" || phase === "confirming");
    }, [
        phase,
        onBusyChange
    ]);
    const run = (0,react.useCallback)(async (next)=>{
        if (inFlight.current) return;
        inFlight.current = true;
        setVerb(next);
        setPhase("sending");
        try {
            var _notification_onApprove, _notification_onDeny;
            await (next === "approve" ? (_notification_onApprove = notification.onApprove) === null || _notification_onApprove === void 0 ? void 0 : _notification_onApprove.call(notification) : (_notification_onDeny = notification.onDeny) === null || _notification_onDeny === void 0 ? void 0 : _notification_onDeny.call(notification));
            onClose({
                acknowledge: true
            });
        } catch (e) {
            setPhase("failed");
            onActionFailed === null || onActionFailed === void 0 ? void 0 : onActionFailed();
        } finally{
            inFlight.current = false;
        }
    }, [
        notification,
        onClose,
        onActionFailed
    ]);
    const approve = (0,react.useCallback)(()=>{
        if (confirmApprove && phase !== "confirming") setPhase("confirming");
        else void run("approve");
    }, [
        confirmApprove,
        phase,
        run
    ]);
    return {
        phase,
        verb,
        run,
        approve,
        cancelConfirm: ()=>setPhase("idle")
    };
}

// EXTERNAL MODULE: ./src/lib/notification-policy.ts
var notification_policy = __webpack_require__(35172);
// EXTERNAL MODULE: ./src/lib/utils/notificationMapping.ts
var notificationMapping = __webpack_require__(71249);
// EXTERNAL MODULE: ./src/components/ui/NotificationToast.css.ts
var NotificationToast_css = __webpack_require__(18311);
// EXTERNAL MODULE: ./src/lib/sessions/riskLevel.ts
var sessions_riskLevel = __webpack_require__(18969);
;// ./src/components/ui/toast/ToastParts.tsx
/* __next_internal_client_entry_do_not_use__ ToastCloseControl,ToastCommand,SwipeReveal auto */ 


/**
 * The card's close control. Legacy cards keep the x. In the deck an informational
 * toast shows an x ("Dismiss notification") and a pinned one a tray icon plus the
 * visible word "Tray" ("Move to tray"), so a touch user can see it is not a
 * dismissal. Both are at least 44x44.
 */ function ToastCloseControl(param) {
    let { stacked, pinned, onClose } = param;
    if (!stacked) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
            className: NotificationToast_css/* closeButton */.b,
            onClick: onClose,
            "aria-label": "Close notification",
            children: "\xd7"
        });
    }
    if (pinned) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
            type: "button",
            className: NotificationToast_css/* trayButton */.NM,
            "aria-label": "Move to tray",
            "data-testid": "toast-move-to-tray",
            onClick: onClose,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    "aria-hidden": "true",
                    children: "\uD83D\uDCE5"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    children: "Tray"
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        type: "button",
        className: NotificationToast_css/* closeButtonStacked */.GQ,
        "aria-label": "Dismiss notification",
        "data-testid": "toast-dismiss",
        onClick: onClose,
        children: "\xd7"
    });
}
/** The command an approval is for: wrapped, clamped to four lines (three in the phone deck) until "Show full command". */ function ToastCommand(param) {
    let { command, riskLevel, expanded, onToggle } = param;
    const risk = riskLevel && riskLevel in sessions_riskLevel/* RISK_LEVEL_LABELS */.p ? "Risk: ".concat(sessions_riskLevel/* RISK_LEVEL_LABELS */.p[riskLevel]) : "Risk not recorded";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: NotificationToast_css/* commandBlock */.L,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: NotificationToast_css/* riskLabel */.uI,
                "data-testid": "toast-risk",
                children: risk
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: "".concat(NotificationToast_css/* commandText */.jt, " ").concat(expanded ? NotificationToast_css/* commandTextExpanded */.ak : ""),
                "data-testid": "toast-command",
                "data-expanded": expanded ? "true" : "false",
                children: command
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: NotificationToast_css/* disclosureButton */.iI,
                "data-testid": "toast-show-command",
                "aria-expanded": expanded,
                onClick: onToggle,
                children: expanded ? "Hide full command" : "Show full command"
            })
        ]
    });
}
/** Layer behind a dragged card: what releasing will do. */ function SwipeReveal(param) {
    let { pinned, toTheLeft } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(NotificationToast_css/* reveal */._c, " ").concat(toTheLeft ? NotificationToast_css/* revealEnd */.z5 : ""),
        "data-testid": "toast-swipe-reveal",
        "aria-hidden": "true",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: pinned ? "📥" : "✕"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: pinned ? "Move to tray" : "Dismiss"
            })
        ]
    });
}

;// ./src/components/ui/NotificationToast.tsx
/* __next_internal_client_entry_do_not_use__ NotificationToast auto */ 














function getRelativeTime(timestampMs, now) {
    const seconds = Math.floor((now - timestampMs) / 1000);
    if (seconds < 5) return "just now";
    if (seconds < 60) return "".concat(seconds, "s ago");
    const minutes = Math.floor(seconds / 60);
    if (minutes === 1) return "1 min ago";
    if (minutes < 60) return "".concat(minutes, " mins ago");
    const hours = Math.floor(minutes / 60);
    if (hours === 1) return "1 hr ago";
    return "".concat(hours, " hrs ago");
}
/**
 * One toast card. Presentational: it owns no timers. Auto-close, minimize,
 * collapse and exit timing live in the ToastStack through the timer registry, and
 * the timing policy in lib/notification-policy.ts.
 */ function NotificationToast(param) {
    let { notification, onClose, stacked = false, compact = false, offlineReason, collapsed = false, onExpandCollapsed, onBusyChange, onActionFailed, holdHandlers, exiting = false, minimized = false, onExpand, onPresent } = param;
    var _notification_metadata, _replyAvailability, _notification_metadata1, _notification_metadata2;
    const auditLog = (0,useAuditLog/* useAuditLog */.r)();
    const coarse = (0,useCoarsePointer/* useCoarsePointer */.O)();
    const undoButtonRef = (0,react.useRef)(null);
    const rowRef = (0,react.useRef)(null);
    const [menuOpen, setMenuOpen] = (0,react.useState)(false);
    const [commandExpanded, setCommandExpanded] = (0,react.useState)(false);
    const now = (0,useNowTicker/* useNowTicker */.w)(1000);
    const relativeTime = getRelativeTime(notification.timestamp, now);
    const pinned = (0,notification_policy/* isPinned */.T4)(notification);
    const isApproval = Boolean(notification.onApprove || notification.onDeny);
    const riskLevel = (_notification_metadata = notification.metadata) === null || _notification_metadata === void 0 ? void 0 : _notification_metadata.risk_level;
    const decision = useDecisionFlow({
        notification,
        onClose,
        confirmApprove: coarse && needsApproveConfirm(riskLevel),
        onBusyChange,
        onActionFailed
    });
    const sending = decision.phase === "sending";
    const swipe = (0,useSwipeToDismiss/* useSwipeToDismiss */.J)(rowRef, {
        onDismiss: ()=>onClose(),
        disabled: !stacked || collapsed || sending
    });
    (0,react.useEffect)(()=>onPresent === null || onPresent === void 0 ? void 0 : onPresent(), [
        onPresent
    ]);
    // WCAG 2.4.3: move focus to Undo button when undo toast mounts
    (0,react.useEffect)(()=>{
        if (notification.notificationType === "undo") {
            var _undoButtonRef_current;
            (_undoButtonRef_current = undoButtonRef.current) === null || _undoButtonRef_current === void 0 ? void 0 : _undoButtonRef_current.focus();
        }
    }, []);
    // A hidden session opens read-only: label the action for that expectation (Story 5.3).
    const sessionHidden = (0,useSessionHidden/* useSessionHidden */.o)(notification.sessionId) === true;
    const viewLabel = sessionHidden ? "View output" : "View Session";
    const viewTestId = sessionHidden ? "notification-view-output" : "notification-view-session";
    // Reply opens the same read-only view, where the Reply card is (Surface 12b).
    const replyEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)("hidden_session_reply");
    const showReply = sessionHidden && replyEnabled && ((_replyAvailability = (0,replyQuestion/* replyAvailability */.wR)(notification)) === null || _replyAvailability === void 0 ? void 0 : _replyAvailability.kind) === "replyable";
    const handleView = ()=>{
        var _notification_onView;
        auditLog.logNotificationSessionViewed(notification.id, notification.sessionId);
        (_notification_onView = notification.onView) === null || _notification_onView === void 0 ? void 0 : _notification_onView.call(notification);
        onClose();
    };
    const displayTitle = notification.title || notification.sessionName;
    const hasSourceApp = notification.sourceApp || notification.sourceBundleId;
    const projectName = notification.sourceProject;
    const workingDirName = notification.sourceWorkingDir ? notification.sourceWorkingDir.split('/').pop() : null;
    const contextName = projectName || workingDirName || notification.sessionName;
    const subtitleParts = [];
    if (contextName && contextName !== displayTitle) subtitleParts.push(contextName);
    if (hasSourceApp && notification.sourceApp) subtitleParts.push("via ".concat(notification.sourceApp));
    const subtitleText = subtitleParts.join(' ');
    if (collapsed) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: "".concat(NotificationToast_css/* toast */.oR, " ").concat(NotificationToast_css/* toastStacked */.Tv),
            style: {
                "--priority-color": (0,notificationMapping/* priorityColor */.yA)(notification.priority)
            },
            "data-testid": "toast",
            "data-collapsed": "true",
            ...holdHandlers,
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                type: "button",
                className: NotificationToast_css/* collapsedChip */.ue,
                "data-testid": "toast-collapsed-chip",
                onClick: onExpandCollapsed,
                children: [
                    notification.sessionName || displayTitle,
                    " needs you"
                ]
            })
        });
    }
    var _notification_metadata_tool_input_command, _ref;
    const commandSource = (_ref = (_notification_metadata_tool_input_command = (_notification_metadata1 = notification.metadata) === null || _notification_metadata1 === void 0 ? void 0 : _notification_metadata1.tool_input_command) !== null && _notification_metadata_tool_input_command !== void 0 ? _notification_metadata_tool_input_command : (_notification_metadata2 = notification.metadata) === null || _notification_metadata2 === void 0 ? void 0 : _notification_metadata2.tool_input_file) !== null && _ref !== void 0 ? _ref : notification.message;
    const focusWindowAvailable = Boolean(hasSourceApp && notification.onFocusWindow) && !sessionHidden;
    // Focus Window is a desktop-terminal action: absent on touch, and tucked into "..." when crowded.
    const focusInOverflow = stacked && !coarse && focusWindowAvailable && isApproval;
    const showFocusInline = focusWindowAvailable && !coarse && !(stacked && isApproval);
    const offline = Boolean(offlineReason);
    const decisionBlocked = offline || sending;
    const cardClass = [
        NotificationToast_css/* toast */.oR,
        notification.notificationType === "approval_needed" && !stacked ? NotificationToast_css/* toastApproval */.Xe : "",
        exiting ? NotificationToast_css/* exiting */.RP : "",
        minimized ? NotificationToast_css/* minimized */.pd : "",
        stacked ? NotificationToast_css/* toastStacked */.Tv : "",
        stacked ? NotificationToast_css/* swipeCard */.h$ : "",
        stacked && swipe.dragging ? NotificationToast_css/* swipeCardDragging */.C5 : "",
        stacked && compact && !isApproval ? NotificationToast_css/* compactCard */.as : "",
        stacked && compact && isApproval ? NotificationToast_css/* compactApprovalCard */.T0 : ""
    ].filter(Boolean).join(" ");
    const verbLabel = (v)=>v === "approve" ? "✓ Approve" : "✗ Deny";
    const decisionLabel = (v)=>{
        if (sending && decision.verb === v) return v === "approve" ? "Approving..." : "Denying...";
        return verbLabel(v);
    };
    const legacyDecide = (v)=>{
        var _notification_onApprove, _notification_onDeny;
        if (offline) return;
        // Fire-and-forget, as before the deck: the toast closes at once.
        void Promise.resolve(v === "approve" ? (_notification_onApprove = notification.onApprove) === null || _notification_onApprove === void 0 ? void 0 : _notification_onApprove.call(notification) : (_notification_onDeny = notification.onDeny) === null || _notification_onDeny === void 0 ? void 0 : _notification_onDeny.call(notification)).catch(()=>{});
        onClose({
            acknowledge: true
        });
    };
    const approveButtonEl = notification.onApprove && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        className: NotificationToast_css/* approveButton */.Wz,
        "aria-disabled": decisionBlocked ? true : undefined,
        onClick: ()=>{
            if (decisionBlocked) return;
            if (stacked) decision.approve();
            else legacyDecide("approve");
        },
        title: offlineReason !== null && offlineReason !== void 0 ? offlineReason : "Allow this tool use",
        children: stacked ? decisionLabel("approve") : verbLabel("approve")
    }, "approve");
    const denyButtonEl = notification.onDeny && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        className: NotificationToast_css/* denyButton */.$O,
        "aria-disabled": decisionBlocked ? true : undefined,
        onClick: ()=>{
            if (decisionBlocked) return;
            if (stacked) void decision.run("deny");
            else legacyDecide("deny");
        },
        title: offlineReason !== null && offlineReason !== void 0 ? offlineReason : "Deny this tool use",
        children: stacked ? decisionLabel("deny") : verbLabel("deny")
    }, "deny");
    const closeControl = /*#__PURE__*/ (0,jsx_runtime.jsx)(ToastCloseControl, {
        stacked: stacked,
        pinned: pinned,
        onClose: ()=>onClose()
    });
    const phoneApproval = stacked && isApproval;
    // The phone's one-row card carries View/Reply beside the Tray control, not in an actions row.
    const compactRow = stacked && compact && !isApproval;
    const showViewInline = !(phoneApproval && coarse) && !compactRow;
    const showDismissButton = !isApproval ? !(stacked && compact) : !stacked;
    var _notification_repeatCount;
    const card = /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: cardClass,
        style: {
            "--priority-color": (0,notificationMapping/* priorityColor */.yA)(notification.priority),
            ...stacked && swipe.offset !== 0 ? {
                transform: "translateX(".concat(swipe.offset, "px)")
            } : {}
        },
        "data-testid": "toast",
        "data-pinned": pinned ? "true" : undefined,
        tabIndex: stacked ? 0 : undefined,
        onKeyDown: stacked ? (e)=>{
            if ((e.key === "Delete" || e.key === "Backspace") && e.target === e.currentTarget) {
                e.preventDefault();
                onClose();
            }
        } : undefined,
        onClick: minimized ? onExpand : undefined,
        title: minimized ? "Click to expand" : undefined,
        ...stacked ? {} : holdHandlers,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationToast_css/* header */.wx,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: NotificationToast_css/* icon */.Kk,
                        "aria-hidden": "true",
                        children: (0,notificationMapping/* notificationTypeIcon */.Ds)(notification.notificationType)
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationToast_css/* titleWrapper */.oB,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: NotificationToast_css/* titleRow */.$,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                        children: displayTitle
                                    }),
                                    ((_notification_repeatCount = notification.repeatCount) !== null && _notification_repeatCount !== void 0 ? _notification_repeatCount : 1) > 1 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: NotificationToast_css/* repeatBadge */.Ln,
                                        "data-testid": "toast-repeat-count",
                                        children: [
                                            "x",
                                            notification.repeatCount
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: NotificationToast_css/* typeLabel */.Dd,
                                        children: (0,notificationMapping/* notificationTypeLabel */.MD)(notification.notificationType)
                                    }),
                                    sessionHidden && /*#__PURE__*/ (0,jsx_runtime.jsx)(BackgroundChip/* BackgroundChip */.l, {})
                                ]
                            }),
                            compactRow ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: NotificationToast_css/* compactPreview */.Me,
                                "data-testid": "toast-preview",
                                children: notification.message
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: NotificationToast_css/* subtitleRow */.xv,
                                children: [
                                    subtitleText && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: NotificationToast_css/* sourceApp */.Pi,
                                        children: subtitleText
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: NotificationToast_css/* timestamp */.vE,
                                        title: new Date(notification.timestamp).toLocaleTimeString(),
                                        children: relativeTime
                                    })
                                ]
                            })
                        ]
                    }),
                    compactRow ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationToast_css/* compactTrailing */.Sn,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: NotificationToast_css/* viewButton */.qM,
                                onClick: handleView,
                                "data-testid": viewTestId,
                                children: viewLabel
                            }),
                            showReply && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: NotificationToast_css/* viewButton */.qM,
                                onClick: handleView,
                                "data-testid": "notification-reply",
                                children: "Reply"
                            }),
                            closeControl
                        ]
                    }) : closeControl
                ]
            }),
            !compactRow && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationToast_css/* body */.rf,
                "data-slot": "body",
                children: [
                    phoneApproval ? /*#__PURE__*/ (0,jsx_runtime.jsx)(ToastCommand, {
                        command: commandSource,
                        riskLevel: riskLevel,
                        expanded: commandExpanded,
                        onToggle: ()=>setCommandExpanded((v)=>!v)
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: NotificationToast_css/* message */.iU,
                        children: notification.message
                    }),
                    notification.sourceWorkingDir && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                        className: NotificationToast_css/* workingDir */.vm,
                        "data-slot": "workdir",
                        title: notification.sourceWorkingDir,
                        children: [
                            "\uD83D\uDCC1 ",
                            notification.sourceWorkingDir.split('/').slice(-2).join('/')
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationToast_css/* actions */.o1,
                "data-slot": "actions",
                children: [
                    phoneApproval ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            decision.phase === "confirming" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: NotificationToast_css/* actionError */.Pg,
                                "data-testid": "toast-approve-confirm",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                        children: "Approve this command?"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: NotificationToast_css/* inlineAction */.a7,
                                        onClick: decision.cancelConfirm,
                                        children: "Back"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: NotificationToast_css/* approveButton */.Wz,
                                        "data-testid": "toast-confirm-approve",
                                        "aria-disabled": decisionBlocked ? true : undefined,
                                        onClick: ()=>!decisionBlocked && void decision.run("approve"),
                                        children: "Confirm approve"
                                    })
                                ]
                            }),
                            decision.phase === "failed" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: NotificationToast_css/* actionError */.Pg,
                                "data-testid": "toast-action-error",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        children: [
                                            "Could not ",
                                            decision.verb,
                                            " - Retry"
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: NotificationToast_css/* inlineAction */.a7,
                                        "aria-disabled": offline ? true : undefined,
                                        onClick: ()=>!offline && void decision.run(decision.verb),
                                        children: "Retry"
                                    })
                                ]
                            }),
                            decision.phase !== "confirming" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: NotificationToast_css/* decisionPair */.Zb,
                                children: [
                                    denyButtonEl,
                                    approveButtonEl
                                ]
                            })
                        ]
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            showFocusInline && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: NotificationToast_css/* focusButton */.Np,
                                onClick: notification.onFocusWindow,
                                title: "Focus the source application window",
                                children: "\uD83D\uDD17 Focus Window"
                            }),
                            approveButtonEl,
                            denyButtonEl
                        ]
                    }),
                    offline && isApproval && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: NotificationToast_css/* offlineHint */.t,
                        "data-testid": "toast-offline-hint",
                        children: offlineReason
                    }),
                    notification.notificationType === "undo" && notification.onUndo && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        ref: undoButtonRef,
                        className: NotificationToast_css/* undoButton */.$S,
                        "data-testid": "undo-toast-button",
                        "aria-label": "Undo the last bulk delete",
                        onClick: ()=>{
                            var _notification_onUndo;
                            (_notification_onUndo = notification.onUndo) === null || _notification_onUndo === void 0 ? void 0 : _notification_onUndo.call(notification);
                            onClose();
                        },
                        children: "Undo"
                    }),
                    showViewInline && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: NotificationToast_css/* viewButton */.qM,
                        onClick: handleView,
                        "data-testid": viewTestId,
                        children: viewLabel
                    }),
                    showViewInline && showReply && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: NotificationToast_css/* viewButton */.qM,
                        onClick: handleView,
                        "data-testid": "notification-reply",
                        children: "Reply"
                    }),
                    showDismissButton && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: NotificationToast_css/* dismissButton */.r,
                        onClick: ()=>onClose({
                                acknowledge: true
                            }),
                        children: "Dismiss"
                    }),
                    phoneApproval && (focusInOverflow || coarse) && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: NotificationToast_css/* overflowTrigger */.BM,
                        "aria-label": "More actions",
                        "aria-expanded": menuOpen,
                        "data-testid": "toast-overflow-menu",
                        onClick: ()=>setMenuOpen((v)=>!v),
                        children: "..."
                    }),
                    phoneApproval && menuOpen && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: NotificationToast_css/* overflowMenu */.Gg,
                        children: [
                            coarse && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: NotificationToast_css/* inlineAction */.a7,
                                onClick: handleView,
                                "data-testid": viewTestId,
                                children: viewLabel
                            }),
                            focusInOverflow && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: NotificationToast_css/* inlineAction */.a7,
                                onClick: notification.onFocusWindow,
                                children: "\uD83D\uDD17 Focus Window"
                            })
                        ]
                    })
                ]
            })
        ]
    });
    if (!stacked) return card;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: rowRef,
        className: NotificationToast_css/* swipeRow */.PQ,
        "data-testid": "toast-row",
        ...holdHandlers,
        children: [
            swipe.revealed && /*#__PURE__*/ (0,jsx_runtime.jsx)(SwipeReveal, {
                pinned: pinned,
                toTheLeft: swipe.offset < 0
            }),
            card
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/contexts/deckViewportContext.ts
var deckViewportContext = __webpack_require__(44865);
// EXTERNAL MODULE: ./src/lib/contexts/notificationContexts.ts
var notificationContexts = __webpack_require__(82514);
// EXTERNAL MODULE: ./src/lib/hooks/useNotificationConnectivity.ts + 1 modules
var useNotificationConnectivity = __webpack_require__(66876);
// EXTERNAL MODULE: ./src/lib/hooks/useAnnounce.ts
var useAnnounce = __webpack_require__(5647);
// EXTERNAL MODULE: ./src/lib/utils/deckSettings.ts
var deckSettings = __webpack_require__(15254);
;// ./src/lib/contexts/toastTray.ts

/**
 * "Move all to tray": a demote-only deck action. It takes toasts off the screen
 * and nothing else; it must stay free of history and RPC imports, which a test
 * enforces by reading this file. Marking read or deleting history is a different
 * command, in a different place.
 */ const MOVE_UNDO_TIMER_ID = "move-all-to-tray";
function createToastTrayCommands(deps) {
    const { queueRef, dispatch, timers, movedRef, setMoved, announce } = deps;
    const finalize = ()=>{
        timers.cancel(MOVE_UNDO_TIMER_ID);
        setMoved(null);
    };
    return {
        /** Returns the ids that left the deck. */ moveAllToTray () {
            var _deps_notifyOtherTabs;
            const toasts = queueRef.current;
            if (toasts.length === 0) return [];
            const ids = toasts.map((t)=>t.id);
            ids.forEach((id)=>timers.cancel(id));
            dispatch({
                type: "remove",
                ids: new Set(ids)
            });
            setMoved({
                toasts: [
                    ...toasts
                ]
            });
            timers.register(MOVE_UNDO_TIMER_ID, "undo-window", (0,deckSettings/* readUndoWindowMs */.Vn)(), ()=>setMoved(null));
            (_deps_notifyOtherTabs = deps.notifyOtherTabs) === null || _deps_notifyOtherTabs === void 0 ? void 0 : _deps_notifyOtherTabs.call(deps, ids);
            announce("".concat(toasts.length, " moved to tray"));
            return ids;
        },
        undoMoveToTray () {
            const batch = movedRef.current;
            if (!batch) return;
            finalize();
            dispatch({
                type: "restore",
                toasts: batch.toasts
            });
            announce("Restored ".concat(batch.toasts.length, " notifications"));
        },
        /** The undo window ends early, e.g. because the tray opened. */ finalizeMove: finalize
    };
}

// EXTERNAL MODULE: ./src/lib/utils/toastDock.ts
var toastDock = __webpack_require__(86130);
// EXTERNAL MODULE: ./src/lib/terminal/cursorRect.ts
var cursorRect = __webpack_require__(48469);
;// ./src/components/ui/toastDeckAnchor.ts
const DECK_GAP_PX = 8;
/**
 * Where the desktop deck sits so it never covers the line being typed on.
 * Bottom-right is the default; the deck moves to the top-right anchor when its
 * footprint would cover the cursor cell, and also when a terminal is mounted but
 * its cursor cannot be read, since covering the input line is the costlier error.
 * With no terminal on the page there is nothing to cover.
 */ function deckAnchor(cursor, bottomRightFootprint) {
    if (cursor.kind === "no-terminal") return "bottom-right";
    if (cursor.kind === "unreadable") return "top-right";
    return intersects(cursor.rect, bottomRightFootprint) ? "top-right" : "bottom-right";
}
function intersects(cell, deck) {
    return cell.left < deck.right + DECK_GAP_PX && cell.left + cell.width > deck.left - DECK_GAP_PX && cell.top < deck.bottom + DECK_GAP_PX && cell.top + cell.height > deck.top - DECK_GAP_PX;
}
/** The rectangle a bottom-right deck would occupy in a viewport. */ function bottomRightFootprint(viewport, deck, inset) {
    const right = viewport.width - inset.right;
    const bottom = viewport.height - inset.bottom;
    return {
        left: right - deck.width,
        right,
        bottom,
        top: bottom - deck.height
    };
}

// EXTERNAL MODULE: ./src/components/ui/TrayHandle.tsx + 1 modules
var TrayHandle = __webpack_require__(85771);
// EXTERNAL MODULE: ./src/components/ui/trayVariant.ts
var trayVariant = __webpack_require__(83501);
;// ./src/components/ui/ToastStack.tsx
// +feature: notification-toast-stack
/* __next_internal_client_entry_do_not_use__ ToastStack auto */ 
















const EXIT_ANIMATION_MS = 300;
/**
 * One toast and its timers. Every timer goes through the registry so the stack,
 * and not the card, decides when a toast closes, minimizes or exits.
 */ function ToastSlot(param) {
    let { notification, timers, onRemove, collapsible = false, offlineReason } = param;
    const { id } = notification;
    // The audit-log object is new every render; a ref keeps timer effects from restarting.
    const auditLog = (0,useAuditLog/* useAuditLog */.r)();
    const auditLogRef = (0,react.useRef)(auditLog);
    auditLogRef.current = auditLog;
    const [presented, setPresented] = (0,react.useState)(false);
    const [exiting, setExiting] = (0,react.useState)(false);
    const [collapsed, setCollapsed] = (0,react.useState)(false);
    // Once the user expands a collapsed card it stays open: no collapse loop.
    const expandedByUser = (0,react.useRef)(false);
    // True while an Approve or Deny is in flight or its confirm step is open (set by the card).
    const [busy, setBusy] = (0,react.useState)(false);
    // An Approve or Deny that failed keeps the toast up until it is resolved.
    const [actionFailed, setActionFailed] = (0,react.useState)(false);
    const closingRef = (0,react.useRef)(false);
    const pinned = (0,notification_policy/* isPinned */.T4)(notification) || actionFailed;
    const requestClose = (0,react.useCallback)(function() {
        let { acknowledge = false } = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
        if (closingRef.current) return;
        closingRef.current = true;
        setExiting(true);
        auditLogRef.current.logNotificationDismissed(id, notification.sessionId);
        timers.register(id, "exit", EXIT_ANIMATION_MS, ()=>{
            var _notification_onDismiss, _notification_onAcknowledge;
            (_notification_onDismiss = notification.onDismiss) === null || _notification_onDismiss === void 0 ? void 0 : _notification_onDismiss.call(notification);
            if (acknowledge) (_notification_onAcknowledge = notification.onAcknowledge) === null || _notification_onAcknowledge === void 0 ? void 0 : _notification_onAcknowledge.call(notification);
            onRemove(id);
        });
    }, [
        id,
        notification,
        onRemove,
        timers
    ]);
    const onPresent = (0,react.useCallback)(()=>{
        setPresented(true);
        return ()=>setPresented(false);
    }, []);
    (0,react.useEffect)(()=>{
        if (!presented) return;
        // A pinned decision is never timed out: it leaves only by an explicit action
        // (the tray keeps it either way).
        if (pinned) return;
        const autoClose = (0,notification_policy/* toastAutoCloseMs */.zZ)(notification.notificationType);
        if (autoClose > 0) timers.register(id, "close", autoClose, ()=>requestClose());
        return ()=>timers.cancel(id, "close");
    }, [
        presented,
        pinned,
        id,
        notification.notificationType,
        requestClose,
        timers
    ]);
    (0,react.useEffect)(()=>{
        if (!presented || !collapsible || !pinned || collapsed || busy || expandedByUser.current) return;
        const collapseMs = (0,deckSettings/* readPinnedCollapseMs */.q0)();
        if (collapseMs === null) return;
        timers.register(id, "collapse", collapseMs, ()=>setCollapsed(true));
        return ()=>timers.cancel(id, "collapse");
    }, [
        presented,
        collapsible,
        pinned,
        collapsed,
        busy,
        id,
        timers
    ]);
    (0,react.useEffect)(()=>()=>timers.cancel(id), [
        id,
        timers
    ]);
    // Hover and keyboard focus hold this toast's timers; the remaining time is kept.
    // A touch tap is neither: it leaves the card "hovered" and focused until the next tap
    // elsewhere, which would hold the pinned-card collapse indefinitely on a phone.
    const touchFocus = (0,react.useRef)(false);
    const holdHandlers = (0,react.useMemo)(()=>({
            onPointerDown: (e)=>{
                touchFocus.current = e.pointerType === "touch";
            },
            onPointerEnter: (e)=>e.pointerType !== "touch" && timers.pause(id, "hover"),
            onPointerLeave: ()=>timers.resume(id, "hover"),
            onFocus: ()=>!touchFocus.current && timers.pause(id, "focus"),
            onBlur: ()=>{
                touchFocus.current = false;
                timers.resume(id, "focus");
            }
        }), [
        id,
        timers
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(NotificationToast, {
        notification: notification,
        stacked: true,
        compact: collapsible,
        offlineReason: offlineReason,
        collapsed: collapsed,
        onExpandCollapsed: ()=>{
            expandedByUser.current = true;
            setCollapsed(false);
        },
        onBusyChange: setBusy,
        onActionFailed: ()=>setActionFailed(true),
        holdHandlers: holdHandlers,
        exiting: exiting,
        onClose: requestClose,
        onPresent: onPresent
    });
}
/** Speaks each newly arrived toast once; the Announcer coalesces bursts and owns the live regions. */ function useAnnounceArrivals(toasts) {
    const { announce, announceArrival } = (0,useAnnounce/* useAnnounce */.XE)();
    const seen = (0,react.useRef)(new Set());
    (0,react.useEffect)(()=>{
        const current = new Set(toasts.map((n)=>n.id));
        for (const n of toasts){
            var _n_metadata;
            if (seen.current.has(n.id)) continue;
            seen.current.add(n.id);
            const isReceipt = n.notificationType === "undo" || ((_n_metadata = n.metadata) === null || _n_metadata === void 0 ? void 0 : _n_metadata.actionToastKey) !== undefined;
            if (isReceipt) {
                announce(n.message, n.notificationType === "error" ? "assertive" : "polite", n.id);
            } else {
                announceArrival({
                    title: n.title || n.sessionName || n.message,
                    pinned: (0,notification_policy/* isPinned */.T4)(n)
                });
            }
        }
        seen.current.forEach((id)=>current.has(id) || seen.current.delete(id));
    }, [
        toasts,
        announce,
        announceArrival
    ]);
}
const MOVE_ALL_LABEL = "Move all notifications to tray";
const ANCHOR_POLL_MS = 250;
// The right inset leaves room for the 44px tray handle on the edge.
const DESKTOP_DECK = {
    width: 360,
    inset: {
        right: 60,
        bottom: 24
    },
    fallbackHeight: 120
};
/**
 * Deck controls must not pull focus off the terminal: a mousedown on a button
 * would blur xterm's textarea (and, on a phone, collapse the soft keyboard).
 */ const keepTerminalFocus = {
    onMouseDown: (e)=>e.preventDefault()
};
/**
 * Desktop overlap guard (TD-15): re-reads the xterm cursor a few times a second
 * while a deck is showing and moves the deck to the top-right anchor whenever
 * its bottom-right footprint would cover the cursor cell.
 */ function useDesktopDeckAnchor(enabled, deckRef) {
    const [anchor, setAnchor] = (0,react.useState)("bottom-right");
    (0,react.useEffect)(()=>{
        if (!enabled) return;
        const check = ()=>{
            const deck = deckRef.current;
            const footprint = bottomRightFootprint({
                width: window.innerWidth,
                height: window.innerHeight
            }, {
                width: DESKTOP_DECK.width,
                height: (deck === null || deck === void 0 ? void 0 : deck.offsetHeight) || DESKTOP_DECK.fallbackHeight
            }, DESKTOP_DECK.inset);
            setAnchor(deckAnchor((0,cursorRect/* readTerminalCursor */.H4)(), footprint));
        };
        check();
        const interval = setInterval(check, ANCHOR_POLL_MS);
        return ()=>clearInterval(interval);
    }, [
        enabled,
        deckRef
    ]);
    return enabled ? anchor : "bottom-right";
}
/** "Moved N to tray - Undo", paused while hovered or focused so it cannot vanish under a tap. */ function UndoBar(param) {
    let { count, timers, onUndo } = param;
    const pause = (reason)=>timers.pause(MOVE_UNDO_TIMER_ID, reason);
    const resume = (reason)=>timers.resume(MOVE_UNDO_TIMER_ID, reason);
    // Release any hold when the bar goes away, so the next window cannot start paused.
    (0,react.useEffect)(()=>()=>{
            timers.resume(MOVE_UNDO_TIMER_ID, "hover");
            timers.resume(MOVE_UNDO_TIMER_ID, "focus");
        }, [
        timers
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: NotificationToast_css/* undoBar */.EZ,
        "data-testid": "toast-undo-bar",
        onPointerEnter: ()=>pause("hover"),
        onPointerLeave: ()=>resume("hover"),
        onFocus: ()=>pause("focus"),
        onBlur: ()=>resume("focus"),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                children: [
                    "Moved ",
                    count,
                    " to tray"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: NotificationToast_css/* undoAction */.DZ,
                "data-testid": "toast-undo-move",
                onClick: onUndo,
                ...keepTerminalFocus,
                children: "Undo"
            })
        ]
    });
}
/** The capped deck: at most `cap` cards and a "+N more" chip for the rest. */ function Deck(param) {
    let { toasts, timers, onRemove, onOpenTray } = param;
    const viewport = (0,deckViewportContext/* useDeckViewport */.T)();
    const { isOffline } = (0,useNotificationConnectivity/* useNotificationConnectivity */.yO)();
    const { movedToTray, isPanelOpen } = (0,notificationContexts/* useNotificationState */.IB)();
    const { moveAllToTray, undoMoveToTray } = (0,notificationContexts/* useNotificationCommands */.RK)();
    const cap = (0,notification_policy/* toastCapFor */.vW)(viewport);
    const { visible, overflow, pinnedCount } = (0,notification_policy/* partitionToasts */.Lx)(toasts, cap);
    const onPhone = !viewport.isInnerScreen;
    const topOffset = (0,react.useSyncExternalStore)(toastDock/* subscribeStackTopOffset */.GR, toastDock/* getStackTopOffset */.vI, ()=>null);
    const deckRef = (0,react.useRef)(null);
    const hasTerminal = topOffset !== null;
    const hasContent = visible.length > 0 || overflow > 0 || movedToTray !== null;
    const desktopAnchor = useDesktopDeckAnchor(!onPhone && hasContent, deckRef);
    if (!hasContent) return null;
    // Phone: top dock under the session tab row when one is published, else above the bottom nav.
    let placement;
    if (!onPhone) placement = desktopAnchor === "top-right" ? "desktopTopRight" : "desktop";
    else placement = topOffset !== null ? "mobileTop" : "mobileBottom";
    // With the keyboard open the cap is 0: one 40px strip with a verb, "Open tray - M need you".
    const needYou = "".concat(pinnedCount, " ").concat(pinnedCount === 1 ? "needs" : "need", " you");
    const chipText = cap === 0 ? "Open tray - ".concat(pinnedCount > 0 ? needYou : "".concat(toasts.length, " notification").concat(toasts.length === 1 ? "" : "s")) : "+".concat(overflow, " more");
    const chipLabel = cap === 0 ? chipText : "".concat(overflow, " more notifications, open tray");
    const moveAll = /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
        type: "button",
        className: NotificationToast_css/* deckAction */.ou,
        "data-testid": "toast-move-all-to-tray",
        "aria-label": MOVE_ALL_LABEL,
        onClick: ()=>moveAllToTray(),
        ...keepTerminalFocus,
        children: [
            "Move all to tray (",
            toasts.length,
            ")"
        ]
    });
    // Not with the keyboard open: the strip is the only deck control then.
    const showMoveAll = toasts.length >= 2 && cap > 0;
    const undo = movedToTray ? /*#__PURE__*/ (0,jsx_runtime.jsx)(UndoBar, {
        count: movedToTray.count,
        timers: timers,
        onUndo: undoMoveToTray
    }) : null;
    const affordance = (0,trayVariant/* trayAffordanceVariant */.B_)(overflow, toasts.length, viewport.isVirtualKeyboardOpen, hasTerminal);
    // A page with no terminal floats the entry above the bottom nav (the dock's own placement), but
    // the entry still says the same thing: overflow, keyboard chip or nothing.
    const entryContent = affordance === "floating-bottom" ? (0,trayVariant/* trayAffordanceVariant */.B_)(overflow, toasts.length, viewport.isVirtualKeyboardOpen, true) : affordance;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: deckRef,
        className: "".concat(NotificationToast_css/* deckPlacement */.XY[placement], " ").concat(isPanelOpen ? onPhone ? NotificationToast_css/* deckBehindTray */.kR : NotificationToast_css/* deckBesideTray */.Oh : ""),
        "data-testid": "toast-stack",
        "data-placement": placement,
        "data-gesture-ignore": true,
        children: [
            !onPhone && (undo !== null && undo !== void 0 ? undo : showMoveAll ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: NotificationToast_css/* deckHeader */.dg,
                children: moveAll
            }) : null),
            onPhone && (!undo && !showMoveAll && entryContent === "none" ? null : /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: NotificationToast_css/* chipRow */.BQ,
                children: [
                    !undo && showMoveAll && moveAll,
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(TrayHandle/* TrayEntryChip */.HR, {
                        content: entryContent,
                        chipText: chipText,
                        chipLabel: chipLabel,
                        undo: undo,
                        onOpen: onOpenTray
                    })
                ]
            })),
            visible.map((notification)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(ToastSlot, {
                    notification: notification,
                    timers: timers,
                    onRemove: onRemove,
                    collapsible: onPhone,
                    offlineReason: isOffline ? "Offline" : undefined
                }, notification.id)),
            !onPhone && overflow > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: NotificationToast_css/* chipRow */.BQ,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    className: NotificationToast_css/* overflowChip */.tG,
                    "data-testid": "toast-overflow-chip",
                    "aria-label": chipLabel,
                    onClick: onOpenTray,
                    ...keepTerminalFocus,
                    children: chipText
                })
            })
        ]
    });
}
/**
 * Renders the active toasts. The provider owns the queue and the timer registry;
 * this owns presentation, per-toast timers and the cap.
 */ function ToastStack(param) {
    let { timers } = param;
    const { notifications, isPanelOpen } = (0,notificationContexts/* useNotificationState */.IB)();
    const { removeNotification, togglePanel } = (0,notificationContexts/* useNotificationCommands */.RK)();
    useAnnounceArrivals(notifications);
    // An open tray holds every toast timer; closing it resumes them with time preserved.
    (0,react.useEffect)(()=>{
        if (isPanelOpen) timers.hold("tray-open");
        else timers.release("tray-open");
        return ()=>timers.release("tray-open");
    }, [
        isPanelOpen,
        timers
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(Deck, {
        toasts: notifications,
        timers: timers,
        onRemove: removeNotification,
        onOpenTray: togglePanel
    });
}

// EXTERNAL MODULE: ./src/components/ui/LiveRegion.css.ts
var LiveRegion_css = __webpack_require__(76863);
;// ./src/components/ui/Announcer.tsx
/* __next_internal_client_entry_do_not_use__ AnnouncerProvider auto */ 



/**
 * The one owner of the notification system's live regions: an always-mounted
 * polite `role="status"` and an always-mounted assertive `role="alert"`. They
 * carry only the role (its implicit aria-live), never an extra aria-live, so no
 * reader double-announces. Surfaces call `useAnnounce()`; the API is documented
 * in lib/hooks/useAnnounce.ts.
 *
 * Reuses LiveRegion's visually-hidden style rather than the LiveRegion component:
 * that component retains the last message and sets aria-live beside the role,
 * which would defeat clear-then-repeat announcements.
 */ function AnnouncerProvider(param) {
    let { children } = param;
    const [polite, setPolite] = (0,react.useState)("");
    const [assertive, setAssertive] = (0,react.useState)("");
    const engineRef = (0,react.useRef)(null);
    if (engineRef.current === null) {
        engineRef.current = new useAnnounce/* AnnouncerEngine */._h({
            setPolite,
            setAssertive
        });
    }
    (0,react.useEffect)(()=>{
        const engine = engineRef.current;
        engine === null || engine === void 0 ? void 0 : engine.resume();
        return ()=>engine === null || engine === void 0 ? void 0 : engine.dispose();
    }, []);
    const api = (0,react.useMemo)(()=>({
            announce: (message, politeness, key)=>{
                var _engineRef_current;
                return (_engineRef_current = engineRef.current) === null || _engineRef_current === void 0 ? void 0 : _engineRef_current.announce(message, politeness, key);
            },
            announceArrival: (arrival)=>{
                var _engineRef_current;
                return (_engineRef_current = engineRef.current) === null || _engineRef_current === void 0 ? void 0 : _engineRef_current.announceArrival(arrival);
            }
        }), []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(useAnnounce/* AnnouncerContext */.FR.Provider, {
        value: api,
        children: [
            children,
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "aria-atomic": "true",
                "data-testid": "announcer-polite",
                className: LiveRegion_css/* srOnly */.s,
                children: polite
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "alert",
                "aria-atomic": "true",
                "data-testid": "announcer-assertive",
                className: LiveRegion_css/* srOnly */.s,
                children: assertive
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
;// ./src/lib/hooks/useNotificationHistory.ts
/* __next_internal_client_entry_do_not_use__ useNotificationHistory auto */ 





const DEFAULT_PAGE_SIZE = 50;
/** Minimum gap between automatic refreshes, so a burst of focus/online events fetches once. */ const AUTO_REFRESH_MIN_GAP_MS = 5000;
/** Poll cadence while the live stream is down (new notifications otherwise stop arriving). */ const FALLBACK_POLL_MS = 30000;
/**
 * React hook for managing persisted notification history.
 *
 * Fetches notification history from the GetNotificationHistory RPC on mount
 * and exposes methods to mark as read, clear, and paginate.
 */ function useNotificationHistory() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { streamLive = true } = options;
    const [notifications, setNotifications] = (0,react.useState)([]);
    const [unreadCount, setUnreadCount] = (0,react.useState)(0);
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [hasMore, setHasMore] = (0,react.useState)(false);
    const [offset, setOffset] = (0,react.useState)(0);
    const [lastUpdatedAt, setLastUpdatedAt] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    // Initialize ConnectRPC client
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    // Fetch notification history from the server
    const fetchHistory = (0,react.useCallback)(async function() {
        let resetOffset = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : true;
        if (!clientRef.current) return;
        setLoading(true);
        setError(null);
        try {
            const currentOffset = resetOffset ? 0 : offset;
            const request = (0,create/* create */.v)(session_pb/* GetNotificationHistoryRequestSchema */.G5n, {
                limit: DEFAULT_PAGE_SIZE,
                offset: currentOffset
            });
            const response = await clientRef.current.getNotificationHistory(request);
            if (resetOffset) {
                var _response_notifications;
                setNotifications((_response_notifications = response.notifications) !== null && _response_notifications !== void 0 ? _response_notifications : []);
                setOffset(DEFAULT_PAGE_SIZE);
            } else {
                // Append for loadMore -- deduplicate by ID
                setNotifications((prev)=>{
                    const existingIds = new Set(prev.map((n)=>n.id));
                    var _response_notifications;
                    const newRecords = ((_response_notifications = response.notifications) !== null && _response_notifications !== void 0 ? _response_notifications : []).filter((n)=>!existingIds.has(n.id));
                    return [
                        ...prev,
                        ...newRecords
                    ];
                });
                setOffset(currentOffset + DEFAULT_PAGE_SIZE);
            }
            setUnreadCount(response.unreadCount);
            setHasMore(response.hasMore);
            setLastUpdatedAt(Date.now());
        } catch (err) {
            const fetchError = err instanceof Error ? err : new Error("Failed to fetch notification history");
            setError(fetchError);
            console.error("Failed to fetch notification history:", fetchError);
        } finally{
            setLoading(false);
        }
    }, [
        offset
    ]);
    // Fetch history once on mount. New notifications arrive via the watchSessions
    // stream and are added to local state by NotificationContext.addNotification,
    // so periodic polling is not needed.
    (0,react.useEffect)(()=>{
        fetchHistory(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    // Refresh: reload from the beginning
    const refresh = (0,react.useCallback)(async ()=>{
        await fetchHistory(true);
    }, [
        fetchHistory
    ]);
    // The stream delivers new notifications, so history is not polled while it is live. Any
    // other time (tab refocus, network back, stream down) the list is re-fetched by RPC so a
    // cached list cannot sit stale and the tray can tell "unreachable" from "stream paused".
    const lastAutoRefreshRef = (0,react.useRef)(0);
    const refreshRef = (0,react.useRef)(refresh);
    refreshRef.current = refresh;
    (0,react.useEffect)(()=>{
        const autoRefresh = ()=>{
            if (document.visibilityState !== "visible") return;
            const now = Date.now();
            if (now - lastAutoRefreshRef.current < AUTO_REFRESH_MIN_GAP_MS) return;
            lastAutoRefreshRef.current = now;
            void refreshRef.current();
        };
        document.addEventListener("visibilitychange", autoRefresh);
        window.addEventListener("online", autoRefresh);
        const poll = streamLive ? null : setInterval(autoRefresh, FALLBACK_POLL_MS);
        return ()=>{
            document.removeEventListener("visibilitychange", autoRefresh);
            window.removeEventListener("online", autoRefresh);
            if (poll) clearInterval(poll);
        };
    }, [
        streamLive
    ]);
    // Load more: append next page
    const loadMore = (0,react.useCallback)(async ()=>{
        await fetchHistory(false);
    }, [
        fetchHistory
    ]);
    // Mark specific notifications as read
    const markAsRead = (0,react.useCallback)(async (ids)=>{
        if (!clientRef.current) return false;
        // Optimistic update; reading a row also resolves it as a pending decision, so the
        // merge into the context history does not resurrect the server's stale flag.
        setNotifications((prev)=>prev.map((n)=>ids.includes(n.id) ? {
                    ...n,
                    isRead: true,
                    isPendingDecision: false
                } : n));
        setUnreadCount((prev)=>Math.max(0, prev - ids.length));
        try {
            const request = (0,create/* create */.v)(session_pb/* MarkNotificationReadRequestSchema */.bMy, {
                notificationIds: ids
            });
            await clientRef.current.markNotificationRead(request);
            return true;
        } catch (err) {
            console.error("Failed to mark notifications as read:", err);
            // Rollback on failure
            await fetchHistory(true);
            return false;
        }
    }, [
        fetchHistory
    ]);
    // Clear notification history
    const clearHistory = (0,react.useCallback)(async (beforeTimestamp)=>{
        if (!clientRef.current) return;
        // Optimistic update
        const previousNotifications = notifications;
        setNotifications([]);
        setUnreadCount(0);
        setHasMore(false);
        try {
            const request = (0,create/* create */.v)(session_pb/* ClearNotificationHistoryRequestSchema */.b2z, {
                beforeTimestamp
            });
            await clientRef.current.clearNotificationHistory(request);
        } catch (err) {
            console.error("Failed to clear notification history:", err);
            // Rollback on failure
            setNotifications(previousNotifications);
            await fetchHistory(true);
        }
    }, [
        notifications,
        fetchHistory
    ]);
    const clearByIds = (0,react.useCallback)(async (ids, options)=>{
        const client = (options === null || options === void 0 ? void 0 : options.keepalive) ? (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getKeepaliveConnectTransport */.O)()) : clientRef.current;
        if (!client) throw new Error("notification client not ready");
        const response = await client.clearNotificationHistory((0,create/* create */.v)(session_pb/* ClearNotificationHistoryRequestSchema */.b2z, {
            notificationIds: ids
        }));
        var _response_keptIds;
        const kept = (_response_keptIds = response.keptIds) !== null && _response_keptIds !== void 0 ? _response_keptIds : [];
        const gone = new Set(ids.filter((id)=>!kept.includes(id)));
        setNotifications((prev)=>prev.filter((n)=>!gone.has(n.id)));
        return {
            deleted: response.clearedCount,
            kept
        };
    }, []);
    return {
        notifications,
        unreadCount,
        loading,
        error,
        hasMore,
        lastUpdatedAt,
        markAsRead,
        clearHistory,
        clearByIds,
        loadMore,
        refresh
    };
}

;// ./src/lib/hooks/useToastQueue.ts
/* __next_internal_client_entry_do_not_use__ toastQueueReducer,useToastQueue auto */ 

function isApprovalToast(n) {
    return Boolean(n.onApprove || n.onDeny);
}
function toastQueueReducer(queue, action) {
    switch(action.type){
        case "add":
            {
                const { notification } = action;
                // One toast per session: the latest replaces the earlier (history keeps both).
                // An approval toast is never displaced by a non-approval one — it needs resolving.
                const existing = queue.find((n)=>n.sessionId === notification.sessionId);
                if (existing && isApprovalToast(existing) && !isApprovalToast(notification)) return queue;
                var _existing_repeatCount;
                const repeated = existing && existing.notificationType === notification.notificationType ? {
                    ...notification,
                    repeatCount: ((_existing_repeatCount = existing.repeatCount) !== null && _existing_repeatCount !== void 0 ? _existing_repeatCount : 1) + 1
                } : notification;
                return [
                    ...queue.filter((n)=>n.sessionId !== notification.sessionId),
                    repeated
                ];
            }
        case "append":
            {
                const { notification, replaceKey } = action;
                const kept = replaceKey === undefined ? queue : queue.filter((n)=>{
                    var _n_metadata;
                    return ((_n_metadata = n.metadata) === null || _n_metadata === void 0 ? void 0 : _n_metadata.actionToastKey) !== replaceKey;
                });
                return [
                    ...kept,
                    notification
                ];
            }
        case "remove":
            return queue.some((n)=>action.ids.has(n.id)) ? queue.filter((n)=>!action.ids.has(n.id)) : queue;
        case "removeByApprovalId":
            return queue.filter((n)=>{
                var _n_metadata;
                return ((_n_metadata = n.metadata) === null || _n_metadata === void 0 ? void 0 : _n_metadata.approval_id) !== action.approvalId;
            });
        case "removeBySessionIds":
            return queue.filter((n)=>{
                var _n_sessionId;
                return !action.sessionIds.has((_n_sessionId = n.sessionId) !== null && _n_sessionId !== void 0 ? _n_sessionId : "");
            });
        case "clearUnpinned":
            {
                const next = queue.filter(notification_policy/* isPinned */.T4);
                return next.length === queue.length ? queue : next;
            }
        case "restore":
            {
                const present = new Set(queue.map((n)=>n.id));
                const sessions = new Set(queue.map((n)=>n.sessionId).filter(Boolean));
                const back = action.toasts.filter((n)=>!present.has(n.id) && !(n.sessionId && sessions.has(n.sessionId)));
                return back.length === 0 ? queue : [
                    ...back,
                    ...queue
                ];
            }
        case "prune":
            {
                const next = queue.filter((n)=>{
                    var _action_keep;
                    if ((_action_keep = action.keep) === null || _action_keep === void 0 ? void 0 : _action_keep.call(action, n)) return true;
                    const window = (0,notification_policy/* hasLongToastLifetime */.TM)(n.notificationType) ? notification_policy/* ACTIONABLE_TOAST_STALE_MS */.wu : notification_policy/* TOAST_STALE_MS */.$g;
                    return action.now - n.timestamp < window;
                });
                return next.length === queue.length ? queue : next;
            }
    }
}
/**
 * Toast queue as a synchronously-readable reducer: `queueRef.current` is always the
 * latest state, so a command can compute from it without a state updater (which
 * must stay pure) and without waiting for a render.
 */ function useToastQueue() {
    const queueRef = (0,react.useRef)([]);
    const [queue, setQueue] = (0,react.useState)([]);
    const dispatch = (0,react.useCallback)((action)=>{
        const next = toastQueueReducer(queueRef.current, action);
        if (next === queueRef.current) return;
        queueRef.current = next;
        setQueue(next);
    }, []);
    return {
        queue,
        queueRef,
        dispatch
    };
}

;// ./src/lib/hooks/useToastTimers.ts
/* __next_internal_client_entry_do_not_use__ createToastTimerRegistry,useToastTimers auto */ 
function createToastTimerRegistry() {
    const entries = new Map();
    const idReasons = new Map();
    const holds = new Set();
    const keyOf = (id, kind)=>"".concat(id, "\0").concat(kind);
    const isHeld = (id)=>{
        var _idReasons_get;
        var _idReasons_get_size;
        return holds.size > 0 || ((_idReasons_get_size = (_idReasons_get = idReasons.get(id)) === null || _idReasons_get === void 0 ? void 0 : _idReasons_get.size) !== null && _idReasons_get_size !== void 0 ? _idReasons_get_size : 0) > 0;
    };
    const stop = (entry)=>{
        if (entry.handle === null) return;
        clearTimeout(entry.handle);
        entry.handle = null;
        entry.remaining = Math.max(0, entry.remaining - (Date.now() - entry.startedAt));
    };
    const start = (entry)=>{
        if (entry.handle !== null) return;
        entry.startedAt = Date.now();
        entry.handle = setTimeout(()=>{
            entry.handle = null;
            entries.delete(keyOf(entry.id, entry.kind));
            entry.fire();
        }, entry.remaining);
    };
    const sync = (entry)=>isHeld(entry.id) ? stop(entry) : start(entry);
    const syncId = (id)=>entries.forEach((e)=>e.id === id && sync(e));
    return {
        register (id, kind, ms, fire) {
            const key = keyOf(id, kind);
            const previous = entries.get(key);
            if (previous) stop(previous);
            const entry = {
                id,
                kind,
                remaining: ms,
                startedAt: 0,
                handle: null,
                fire
            };
            entries.set(key, entry);
            sync(entry);
        },
        cancel (id, kind) {
            entries.forEach((entry, key)=>{
                if (entry.id !== id || kind !== undefined && entry.kind !== kind) return;
                stop(entry);
                entries.delete(key);
            });
            if (kind === undefined) idReasons.delete(id);
        },
        cancelAll () {
            entries.forEach(stop);
            entries.clear();
            idReasons.clear();
        },
        pause (id, reason) {
            var _idReasons_get;
            const reasons = (_idReasons_get = idReasons.get(id)) !== null && _idReasons_get !== void 0 ? _idReasons_get : new Set();
            reasons.add(reason);
            idReasons.set(id, reasons);
            syncId(id);
        },
        resume (id, reason) {
            const reasons = idReasons.get(id);
            if (!reasons) return;
            reasons.delete(reason);
            if (reasons.size === 0) idReasons.delete(id);
            syncId(id);
        },
        hold (source) {
            holds.add(source);
            entries.forEach(sync);
        },
        release (source) {
            holds.delete(source);
            entries.forEach(sync);
        },
        has: (id, kind)=>entries.has(keyOf(id, kind)),
        pendingCount: ()=>entries.size
    };
}
/** One stable registry per provider; everything it owns is cancelled on unmount. */ function useToastTimers() {
    const ref = (0,react.useRef)(null);
    if (ref.current === null) ref.current = createToastTimerRegistry();
    (0,react.useEffect)(()=>{
        const registry = ref.current;
        return ()=>registry === null || registry === void 0 ? void 0 : registry.cancelAll();
    }, []);
    return ref.current;
}

// EXTERNAL MODULE: ./src/lib/hooks/useReviewedMarks.ts
var useReviewedMarks = __webpack_require__(23925);
// EXTERNAL MODULE: ./src/lib/utils/notificationGrouping.ts
var notificationGrouping = __webpack_require__(65232);
// EXTERNAL MODULE: ./src/lib/utils/viewedSessions.ts
var viewedSessions = __webpack_require__(76991);
;// ./src/lib/utils/notificationHistoryMerge.ts

function recordToHistoryItem(record) {
    return {
        id: record.id,
        sessionId: record.sessionId,
        sessionName: record.sessionName,
        title: record.title,
        message: record.message,
        timestamp: record.createdAt ? Number(record.createdAt.seconds) * 1000 : Date.now(),
        priority: (0,notificationMapping/* mapPriority */.vV)(record.priority),
        notificationType: (0,notificationMapping/* mapNotificationType */.QE)(record.notificationType),
        metadata: record.metadata ? Object.fromEntries(Object.entries(record.metadata)) : undefined,
        isRead: record.isRead,
        isPendingDecision: record.isPendingDecision,
        occurrenceCount: record.occurrenceCount,
        actions: record.actions
    };
}
const dedupKeyOf = (n)=>{
    var _n_sessionId, _n_notificationType;
    return "".concat((_n_sessionId = n.sessionId) !== null && _n_sessionId !== void 0 ? _n_sessionId : "", ":").concat((_n_notificationType = n.notificationType) !== null && _n_notificationType !== void 0 ? _n_notificationType : "");
};
/**
 * Merges the server's history into the local list. The backend is authoritative:
 * a local item is replaced by its server version (matched by id, or by
 * session+type for stream-added items that still carry a client-generated id) so
 * isRead and metadata always reflect server truth. Local-only callbacks (onView,
 * onApprove, ...) are carried over because they are never persisted.
 */ function mergeBackendHistory(prev, backendItems) {
    const backendById = new Map(backendItems.map((n)=>[
            n.id,
            n
        ]));
    const backendByDedupKey = new Map(backendItems.map((n)=>[
            dedupKeyOf(n),
            n
        ]));
    const updated = [];
    const consumedDedupKeys = new Set();
    for (const n of prev){
        const dk = dedupKeyOf(n);
        if (consumedDedupKeys.has(dk)) continue;
        var _backendById_get;
        const serverVersion = (_backendById_get = backendById.get(n.id)) !== null && _backendById_get !== void 0 ? _backendById_get : backendByDedupKey.get(dk);
        updated.push(serverVersion ? {
            ...serverVersion,
            onView: n.onView,
            onApprove: n.onApprove,
            onDeny: n.onDeny,
            onFocusWindow: n.onFocusWindow
        } : n);
        consumedDedupKeys.add(dk);
    }
    // Mutating the key set as we go keeps duplicate-type backend records (several
    // auto_approved entries for one session) from all slipping through.
    const existingIds = new Set(updated.map((n)=>n.id));
    const existingDedupKeys = new Set(updated.map(dedupKeyOf));
    const fromBackend = [];
    for (const n of backendItems){
        if (existingIds.has(n.id)) continue;
        const dk = dedupKeyOf(n);
        if (existingDedupKeys.has(dk)) continue;
        fromBackend.push(n);
        existingDedupKeys.add(dk);
    }
    return [
        ...fromBackend,
        ...updated
    ];
}

;// ./src/lib/utils/broadcastChannel.ts
const CHANNEL_NAME = "stapler-squad:notification-sync";
const STORAGE_KEY = "stapler-squad:notification-sync";
/**
 * Fallback for browsers without BroadcastChannel: a localStorage write, which the
 * `storage` event delivers to every other tab (never the writer).
 */ function createStorageChannel() {
    return {
        broadcast: (message)=>{
            try {
                // A nonce makes a repeat of an identical message a change, so it fires again.
                window.localStorage.setItem(STORAGE_KEY, JSON.stringify({
                    nonce: Math.random(),
                    message
                }));
                window.localStorage.removeItem(STORAGE_KEY);
            } catch (e) {
            // Storage blocked: cross-tab sync is best effort; history refresh still converges.
            }
        },
        subscribe: (handler)=>{
            const listener = (event)=>{
                if (event.key !== STORAGE_KEY || !event.newValue) return;
                try {
                    handler(JSON.parse(event.newValue).message);
                } catch (e) {
                // Ignore a malformed payload.
                }
            };
            window.addEventListener("storage", listener);
            return ()=>window.removeEventListener("storage", listener);
        }
    };
}
function createNotificationSyncChannel() {
    if (false) {}
    if (typeof BroadcastChannel === "undefined") return createStorageChannel();
    const channel = new BroadcastChannel(CHANNEL_NAME);
    return {
        broadcast: (message)=>channel.postMessage(message),
        subscribe: (handler)=>{
            const listener = (event)=>{
                handler(event.data);
            };
            channel.addEventListener("message", listener);
            return ()=>{
                channel.removeEventListener("message", listener);
                channel.close();
            };
        }
    };
}

// EXTERNAL MODULE: ./src/lib/utils/notificationStorage.ts
var notificationStorage = __webpack_require__(20802);
;// ./src/lib/contexts/NotificationContext.tsx
/* __next_internal_client_entry_do_not_use__ useNotificationState,useNotificationCommands,useNotifications,getFailureReasonToastMessage,NotificationProvider auto */ 





















// One sender per tab; a channel per broadcast was opened and never closed.
let broadcastSender = null;
function sendToOtherTabs(message) {
    if (!broadcastSender) broadcastSender = createNotificationSyncChannel();
    broadcastSender.broadcast(message);
}

/**
 * Reason-specific toast copy for a session that transitioned to
 * SESSION_STATUS_FAILED (async-session-creation Epic 5.3, Surface 4). Mirrors
 * the FailureReason taxonomy set server-side by the Background Resolution
 * Pipeline (session.Instance.FailureReason) — "GitHubResolutionError",
 * "StartupError", "Stale", "Cancelled", "WorktreeResolutionFailed", or
 * "DirectoryCollision". A reasonable default per
 * plan.md's Unresolved Questions (exact copy is a pending product decision).
 *
 * Deliberately different wording than SessionCard.tsx/SessionRow.tsx's
 * persistent card/row copy (lib/utils/sessionFailure.ts's getFailureMessage)
 * — toast copy is transient and can afford to be more conversational, while
 * the card/row copy is a durable record. The `FailureReason` type import
 * (rather than a bare `string` param) only guarantees the *set* of reasons
 * this switch handles stays in sync with the card/row's; it does not force
 * the wording to match.
 */ function getFailureReasonToastMessage(failureReason) {
    switch(failureReason){
        case "GitHubResolutionError":
            return "Couldn't resolve the GitHub repository. Check the URL and try again.";
        case "StartupError":
            return "The session failed to start. See the session card for details.";
        case "Stale":
            return "Session creation timed out and was marked as failed.";
        case "Cancelled":
            return "Session creation was cancelled.";
        case "WorktreeResolutionFailed":
            return "Couldn't set up an isolated workspace for this session. It was not started.";
        case "DirectoryCollision":
            return "Another session is already running in that directory, so this one wasn't started.";
        default:
            return "Session creation failed.";
    }
}
function reviewItemToNotificationType(reason) {
    switch(reason){
        case types_pb/* AttentionReason */.X9.APPROVAL_PENDING:
        case types_pb/* AttentionReason */.X9.WAITING_FOR_USER:
            return "approval_needed";
        case types_pb/* AttentionReason */.X9.INPUT_REQUIRED:
            return "question";
        case types_pb/* AttentionReason */.X9.ERROR_STATE:
        case types_pb/* AttentionReason */.X9.TESTS_FAILING:
            return "error";
        case types_pb/* AttentionReason */.X9.STALE:
            return "warning";
        case types_pb/* AttentionReason */.X9.TASK_COMPLETE:
            return "task_complete";
        default:
            return "info";
    }
}
const handledKeyOf = (n)=>"answer:".concat(n.id, "@").concat(n.timestamp);
const reviewedKeyOf = (n)=>"bgrecord:".concat(n.id, "@").concat(n.timestamp);
const newNotificationId = ()=>"notification-".concat(Date.now(), "-").concat(Math.random());
const toIdSet = (id)=>new Set(Array.isArray(id) ? id : [
        id
    ]);
const SWEEP_INTERVAL_MS = 60000;
const UNDO_TOAST_DEFAULT_MS = 5000;
const ACTION_TOAST_SUCCESS_MS = 5000;
const ACTION_TOAST_ERROR_MS = 10000;
function NotificationProvider(param) {
    let { children } = param;
    // The Announcer wraps the provider's own logic so commands can announce receipts.
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(AnnouncerProvider, {
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(NotificationProviderInner, {
            children: children
        })
    });
}
function NotificationProviderInner(param) {
    let { children } = param;
    const { queue: notifications, queueRef, dispatch } = useToastQueue();
    const timers = useToastTimers();
    const [notificationHistory, setNotificationHistory] = (0,react.useState)([]);
    const [isPanelOpen, setIsPanelOpen] = (0,react.useState)(false);
    // Commands read these for side effects (audit, RPC) so React updater functions stay pure.
    const historyStateRef = (0,react.useRef)([]);
    historyStateRef.current = notificationHistory;
    const isPanelOpenRef = (0,react.useRef)(false);
    isPanelOpenRef.current = isPanelOpen;
    const [moved, setMovedState] = (0,react.useState)(null);
    const movedRef = (0,react.useRef)(null);
    const { announce } = (0,useAnnounce/* useAnnounce */.XE)();
    const announceRef = (0,react.useRef)(announce);
    announceRef.current = announce;
    const handledMarks = (0,useReviewedMarks/* useReviewedMarks */.RI)();
    const handledMarksRef = (0,react.useRef)(handledMarks);
    handledMarksRef.current = handledMarks;
    const auditLog = (0,useAuditLog/* useAuditLog */.r)();
    const streamLive = (0,useNotificationConnectivity/* useStreamConnectionState */.Ru)() === "connected";
    const history = useNotificationHistory({
        streamLive
    });
    // Commands read these through refs so their identities stay stable across renders.
    const auditLogRef = (0,react.useRef)(auditLog);
    auditLogRef.current = auditLog;
    const historyRef = (0,react.useRef)(history);
    historyRef.current = history;
    const [quietMode, setQuietModeState] = (0,react.useState)(false);
    const [answeredElsewhere, setAnsweredElsewhere] = (0,react.useState)({});
    const quietRef = (0,react.useRef)(false);
    // Read after mount so server and client render the same first frame.
    (0,react.useEffect)(()=>{
        quietRef.current = (0,deckSettings/* readQuietMode */.EJ)();
        setQuietModeState(quietRef.current);
    }, []);
    // Backend data is authoritative: runs on initial load and whenever refreshHistory()
    // is called (reconnect, approval_response), updating local items with the server version.
    (0,react.useEffect)(()=>{
        if (history.notifications.length === 0) return;
        const backendItems = history.notifications.map(recordToHistoryItem);
        setNotificationHistory((prev)=>mergeBackendHistory(prev, backendItems));
    }, [
        history.notifications
    ]);
    const commands = (0,react.useMemo)(()=>{
        const removeToast = (id)=>{
            timers.cancel(id);
            dispatch({
                type: "remove",
                ids: new Set([
                    id
                ])
            });
        };
        const tray = createToastTrayCommands({
            queueRef,
            dispatch,
            timers,
            movedRef,
            setMoved: (batch)=>{
                movedRef.current = batch;
                setMovedState(batch);
            },
            announce: (message)=>announceRef.current(message),
            notifyOtherTabs: (ids)=>sendToOtherTabs({
                    type: "NOTIFICATIONS_BULK_DISMISSED",
                    kind: "moved",
                    ids
                })
        });
        const appendEphemeral = (notification, lifetimeMs, replaceKey)=>{
            dispatch({
                type: "append",
                notification,
                replaceKey
            });
            timers.register(notification.id, "expire", lifetimeMs, ()=>removeToast(notification.id));
            return notification.id;
        };
        // Pure transform applied to the state and, eagerly, to the ref a second command in the same tick reads.
        const mutateHistory = (fn)=>{
            historyStateRef.current = fn(historyStateRef.current);
            setNotificationHistory(fn);
        };
        const addToHistory = (notification)=>setNotificationHistory((prev)=>prev.some((n)=>n.id === notification.id) ? prev : [
                    {
                        ...notification,
                        isRead: false
                    },
                    ...prev
                ]);
        const addNotification = (notification)=>{
            var _notification_id;
            const next = {
                ...notification,
                id: (_notification_id = notification.id) !== null && _notification_id !== void 0 ? _notification_id : newNotificationId(),
                timestamp: Date.now()
            };
            // Under the capped deck a non-pinned toast for the session already on screen adds
            // nothing, and Quiet mode demotes every non-pinned toast; a pending decision always
            // toasts. The history row is recorded either way.
            const suppressed = !(0,notification_policy/* isPinned */.T4)(next) && (quietRef.current || (0,viewedSessions/* isSessionViewed */.l)(next.sessionId));
            if (!suppressed) dispatch({
                type: "add",
                notification: next
            });
            addToHistory(next);
        };
        const addToHistoryOnly = (notification)=>{
            var _notification_id;
            return addToHistory({
                ...notification,
                id: (_notification_id = notification.id) !== null && _notification_id !== void 0 ? _notification_id : newNotificationId(),
                timestamp: Date.now()
            });
        };
        const markAnswerHandled = (ids)=>{
            const set = new Set(ids);
            const keys = historyStateRef.current.filter((n)=>set.has(n.id)).map(handledKeyOf);
            handledMarksRef.current.mark(keys, {
                track: false
            });
        };
        const markBackgroundReviewed = (ids)=>{
            const set = new Set(ids);
            const keys = historyStateRef.current.filter((n)=>set.has(n.id)).map(reviewedKeyOf);
            handledMarksRef.current.mark(keys, {
                track: false
            });
        };
        const markAsRead = (id)=>{
            const ids = Array.isArray(id) ? id : [
                id
            ];
            const idSet = new Set(ids);
            for (const n of historyStateRef.current){
                if (idSet.has(n.id)) auditLogRef.current.logNotificationMarkedRead(n.id, n.sessionId);
            }
            mutateHistory((prev)=>prev.map((n)=>idSet.has(n.id) ? {
                        ...n,
                        isRead: true,
                        isPendingDecision: false
                    } : n));
            return historyRef.current.markAsRead(ids);
        };
        return {
            addNotification,
            markAnswerHandled,
            markBackgroundReviewed,
            addToHistoryOnly,
            removeNotification: removeToast,
            dismissToast: removeToast,
            moveAllToTray: tray.moveAllToTray,
            undoMoveToTray: tray.undoMoveToTray,
            removeToastByApprovalId: (approvalId)=>dispatch({
                    type: "removeByApprovalId",
                    approvalId
                }),
            clearAll: ()=>{
                queueRef.current.filter((n)=>!(0,notification_policy/* isPinned */.T4)(n)).forEach((n)=>timers.cancel(n.id));
                dispatch({
                    type: "clearUnpinned"
                });
            },
            showSessionNotification: (item, onView, onAcknowledge)=>addNotification({
                    sessionId: item.sessionId,
                    sessionName: item.sessionName || "Unnamed Session",
                    message: item.context || "This session is waiting for your input",
                    priority: (0,notificationMapping/* mapPriority */.vV)(item.priority),
                    notificationType: reviewItemToNotificationType(item.reason),
                    onView,
                    onAcknowledge
                }),
            showUndoToast: function(message, onUndo) {
                let durationMs = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : UNDO_TOAST_DEFAULT_MS;
                return appendEphemeral({
                    id: newNotificationId(),
                    sessionId: "",
                    sessionName: "",
                    message,
                    timestamp: Date.now(),
                    notificationType: "undo",
                    onUndo
                }, durationMs);
            },
            showActionToast: (message, type, key)=>appendEphemeral({
                    id: newNotificationId(),
                    sessionId: "",
                    sessionName: "",
                    message,
                    timestamp: Date.now(),
                    notificationType: type === "success" ? "task_complete" : "error",
                    metadata: {
                        actionToastKey: key
                    }
                }, type === "success" ? ACTION_TOAST_SUCCESS_MS : ACTION_TOAST_ERROR_MS, key),
            setQuietMode: (on)=>{
                quietRef.current = on;
                (0,deckSettings/* writeQuietMode */.DC)(on);
                setQuietModeState(on);
            },
            togglePanel: ()=>{
                const opening = !isPanelOpenRef.current;
                isPanelOpenRef.current = opening;
                if (opening) auditLogRef.current.logNotificationPanelOpened();
                else auditLogRef.current.logNotificationPanelClosed();
                setIsPanelOpen(opening);
            },
            markAsRead,
            // Removes the toast(s) AND marks them read in one operation; prefer it to
            // calling removeNotification + markAsRead separately.
            acknowledgeNotification: (id)=>{
                const ids = Array.isArray(id) ? id : [
                    id
                ];
                const idSet = new Set(ids);
                for (const n of queueRef.current){
                    if (!idSet.has(n.id)) continue;
                    sendToOtherTabs({
                        type: "NOTIFICATION_DISMISSED",
                        notificationId: n.id
                    });
                    if (n.sessionId) (0,notificationStorage/* markAcknowledged */.aO)(n.sessionId);
                    timers.cancel(n.id);
                }
                dispatch({
                    type: "remove",
                    ids: idSet
                });
                markAsRead(ids);
            },
            broadcastActionResolved: (ids, summary)=>sendToOtherTabs({
                    type: "NOTIFICATION_ACTION_RESOLVED",
                    ids,
                    summary
                }),
            markAsReadBySessionId: (sessionId)=>{
                const sessionIds = toIdSet(sessionId);
                const idsToMark = historyStateRef.current.filter((n)=>!n.isRead && n.sessionId != null && sessionIds.has(n.sessionId)).map((n)=>n.id);
                if (idsToMark.length === 0) return;
                const marked = new Set(idsToMark);
                mutateHistory((prev)=>prev.map((n)=>marked.has(n.id) ? {
                            ...n,
                            isRead: true,
                            isPendingDecision: false
                        } : n));
                historyRef.current.markAsRead(idsToMark);
            },
            removeToastBySessionId: (sessionId)=>{
                const sessionIds = toIdSet(sessionId);
                sessionIds.delete(""); // never match notifications without a sessionId
                if (sessionIds.size > 0) dispatch({
                    type: "removeBySessionIds",
                    sessionIds
                });
            },
            removeFromHistory: (id)=>{
                const notification = historyStateRef.current.find((n)=>n.id === id);
                if (notification) auditLogRef.current.logNotificationRemoved(notification.id, notification.sessionId);
                mutateHistory((prev)=>prev.filter((n)=>n.id !== id));
            },
            clearHistory: ()=>{
                const count = historyStateRef.current.length;
                if (count > 0) auditLogRef.current.logNotificationHistoryCleared(count);
                mutateHistory(()=>[]);
                historyRef.current.clearHistory();
            },
            clearHistoryByIds: async (ids, options)=>{
                const result = await historyRef.current.clearByIds(ids, options);
                const gone = ids.filter((id)=>!result.kept.includes(id));
                if (gone.length > 0) {
                    const removedCount = historyStateRef.current.filter((n)=>gone.includes(n.id)).length;
                    if (removedCount > 0) auditLogRef.current.logNotificationHistoryCleared(removedCount);
                    mutateHistory((prev)=>prev.filter((n)=>!gone.includes(n.id)));
                    sendToOtherTabs({
                        type: "NOTIFICATIONS_BULK_DISMISSED",
                        kind: "dismissed",
                        ids: gone
                    });
                }
                return result;
            },
            loadMoreHistory: ()=>historyRef.current.loadMore(),
            refreshHistory: ()=>historyRef.current.refresh()
        };
    }, [
        dispatch,
        queueRef,
        timers
    ]);
    // Opening the tray ends the undo window: the moved toasts are now simply tray rows.
    (0,react.useEffect)(()=>{
        if (isPanelOpen && movedRef.current) {
            timers.cancel(MOVE_UNDO_TIMER_ID);
            movedRef.current = null;
            setMovedState(null);
        }
    }, [
        isPanelOpen,
        timers
    ]);
    // Remove stale toasts every minute (history keeps them): plain toasts after
    // TOAST_STALE_MS, approval/question toasts after ACTIONABLE_TOAST_STALE_MS.
    (0,react.useEffect)(()=>{
        const interval = setInterval(()=>dispatch({
                type: "prune",
                now: Date.now(),
                keep: notification_policy/* isPinned */.T4
            }), SWEEP_INTERVAL_MS);
        return ()=>clearInterval(interval);
    }, [
        dispatch
    ]);
    // Cross-tab sync: when another tab dismisses a notification, reflect it locally.
    (0,react.useEffect)(()=>{
        const syncChannel = createNotificationSyncChannel();
        const unsubscribe = syncChannel.subscribe((message)=>{
            if (message.type === "NOTIFICATION_DISMISSED") {
                const { notificationId } = message;
                timers.cancel(notificationId);
                dispatch({
                    type: "remove",
                    ids: new Set([
                        notificationId
                    ])
                });
                setNotificationHistory((prev)=>prev.map((n)=>n.id === notificationId ? {
                            ...n,
                            isRead: true,
                            isPendingDecision: false
                        } : n));
            }
            if (message.type === "NOTIFICATION_ACTION_RESOLVED") {
                const ids = new Set(message.ids);
                ids.forEach((id)=>timers.cancel(id));
                dispatch({
                    type: "remove",
                    ids
                });
                setAnsweredElsewhere((prev)=>({
                        ...prev,
                        ...Object.fromEntries(message.ids.map((id)=>[
                                id,
                                message.summary
                            ]))
                    }));
                setNotificationHistory((prev)=>prev.map((n)=>ids.has(n.id) ? {
                            ...n,
                            isRead: true,
                            isPendingDecision: false
                        } : n));
            }
            if (message.type === "NOTIFICATIONS_BULK_DISMISSED") {
                const ids = new Set(message.ids);
                ids.forEach((id)=>timers.cancel(id));
                dispatch({
                    type: "remove",
                    ids
                });
                // "moved" only leaves the deck; "dismissed" also drops the rows the other tab cleared.
                if (message.kind === "dismissed") {
                    setNotificationHistory((prev)=>prev.filter((n)=>!ids.has(n.id)));
                }
            }
        // NOTIFICATION_ACKNOWLEDGED is intentionally not handled here.
        // Cross-tab session acknowledgement is driven by the sessionAcknowledged
        // event from the server stream (useSessionService), not BroadcastChannel.
        });
        return ()=>{
            unsubscribe();
            broadcastSender = null; // the provider is gone; the next one opens its own sender
        };
    }, [
        dispatch,
        timers
    ]);
    var _history_unreadCount;
    // Only one page of history is loaded, so the server's unread total is the floor: a badge
    // for 120 unread must not read 49 just because that is what has been paged in.
    const serverUnread = (_history_unreadCount = history.unreadCount) !== null && _history_unreadCount !== void 0 ? _history_unreadCount : 0;
    // Records the operator marked handled in this browser leave Needs you and the badge here only.
    const isHandled = handledMarks.isReviewed;
    const visibleHistory = (0,react.useMemo)(()=>notificationHistory.map((n)=>!n.isRead && isHandled(handledKeyOf(n)) ? {
                ...n,
                isRead: true,
                isPendingDecision: false
            } : n), [
        notificationHistory,
        isHandled
    ]);
    const handledUnread = visibleHistory.filter((n, i)=>n !== notificationHistory[i]).length;
    // Reviewed Background records stay unread in the list (their row must keep its exception state) but not in the bell.
    const unreviewedUnread = visibleHistory.filter((n)=>!n.isRead && !isHandled(reviewedKeyOf(n)));
    const reviewedUnread = visibleHistory.filter((n)=>!n.isRead).length - unreviewedUnread.length;
    const unreadCount = Math.max((0,notificationGrouping/* groupNotifications */._Q)(unreviewedUnread).length, serverUnread - handledUnread - reviewedUnread);
    const state = (0,react.useMemo)(()=>({
            notifications,
            notificationHistory: visibleHistory,
            isPanelOpen,
            historyLoading: history.loading,
            historyHasMore: history.hasMore,
            historyError: history.error,
            historyLastUpdatedAt: history.lastUpdatedAt,
            unreadCount,
            movedToTray: moved ? {
                count: moved.toasts.length
            } : null,
            answeredElsewhere,
            quietMode
        }), [
        notifications,
        visibleHistory,
        isPanelOpen,
        moved,
        quietMode,
        answeredElsewhere,
        history.loading,
        history.hasMore,
        history.error,
        history.lastUpdatedAt,
        unreadCount
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(notificationContexts/* NotificationCommandsContext */.jo.Provider, {
        value: commands,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(notificationContexts/* NotificationStateContext */._J.Provider, {
            value: state,
            children: [
                children,
                /*#__PURE__*/ (0,jsx_runtime.jsx)(ToastStack, {
                    timers: timers
                })
            ]
        })
    });
}


/***/ }),

/***/ 63367:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   RequireFeatureFlag: () => (/* binding */ RequireFeatureFlag),
/* harmony export */   h: () => (/* binding */ useFeatureFlags),
/* harmony export */   j$: () => (/* binding */ FeatureFlagsProvider),
/* harmony export */   u: () => (/* binding */ useFeatureFlag)
/* harmony export */ });
/* unused harmony export readbackMismatch */
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_nav__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(41078);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(21694);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(23575);
/* harmony import */ var _featureFlagScopes__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(80070);
/* __next_internal_client_entry_do_not_use__ readbackMismatch,FeatureFlagsProvider,useFeatureFlags,useFeatureFlag,RequireFeatureFlag auto */ 






function scopesOf(flag) {
    return Object.fromEntries(flag.scopes.map((s)=>[
            s.scope,
            s.enabled
        ]));
}
function metaOf(f) {
    return {
        name: f.name,
        enabled: f.enabled,
        description: f.description,
        statusDetail: f.statusDetail,
        scopes: scopesOf(f)
    };
}
/** What the server must have persisted for `change`, or null when the read-back agrees. */ function readbackMismatch(change, flag) {
    if (change.mutation === "reset") return null;
    if (change.scope === _featureFlagScopes__WEBPACK_IMPORTED_MODULE_5__/* .GLOBAL_SCOPE */ .N) {
        return change.mutation === "set" && flag.enabled !== change.enabled ? "global value" : null;
    }
    const entry = flag.scopes.find((s)=>s.scope === change.scope);
    if (change.mutation === "clear") return entry ? change.scope : null;
    return (entry === null || entry === void 0 ? void 0 : entry.enabled) === change.enabled ? null : change.scope;
}
const FeatureFlagsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)({
    flags: {},
    flagList: [],
    isLoading: true,
    error: null,
    setFlag: async ()=>{},
    refresh: async ()=>{}
});
function FeatureFlagsProvider(param) {
    let { children } = param;
    const [flags, setFlags] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({});
    const [flagList, setFlagList] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)([]);
    const [isLoading, setIsLoading] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(true);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(null);
    const client = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)()), []);
    const fetchFlags = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async ()=>{
        try {
            const res = await client.getFeatureFlags({});
            const map = {};
            const list = [];
            for (const f of res.flags){
                map[f.name] = f.enabled;
                list.push(metaOf(f));
            }
            setFlags(map);
            setFlagList(list);
            setError(null);
        } catch (e) {
            setError("Failed to load feature flags");
        } finally{
            setIsLoading(false);
        }
    }, [
        client
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        fetchFlags();
    }, [
        fetchFlags
    ]);
    // The displayed state is always the server's read-back, never an optimistic
    // value: a failed or ignored change leaves the previous state and re-reads it.
    const setFlag = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async (name, change)=>{
        const req = typeof change === "boolean" ? {
            name,
            enabled: change
        } : change.mutation === "set" ? {
            name,
            scope: change.scope,
            mutation: change.enabled ? _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.SET_ENABLED : _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.SET_DISABLED
        } : change.mutation === "clear" ? {
            name,
            scope: change.scope,
            mutation: _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.CLEAR_SCOPE
        } : {
            name,
            mutation: _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_4__/* .FlagMutation */ .s34.RESET_GLOBAL
        };
        try {
            const res = await client.updateFeatureFlag(req);
            const flag = res.flag;
            if (!flag) throw new Error("response carried no flag");
            if (typeof change !== "boolean") {
                const bad = readbackMismatch(change, flag);
                if (bad) {
                    // An older server that ignores `scope` answers with success and no entry.
                    throw new Error("server read-back does not show the change for ".concat(bad));
                }
            }
            setFlags((prev)=>({
                    ...prev,
                    [flag.name]: flag.enabled
                }));
            setFlagList((prev)=>prev.map((f)=>f.name === flag.name ? {
                        ...f,
                        ...metaOf(flag)
                    } : f));
        } catch (err) {
            console.error("Failed to update feature flag", name, err);
            // Re-read first: a successful fetch clears the error, and this one must outlive it.
            await fetchFlags();
            setError("Failed to update feature flag");
        }
    }, [
        client,
        fetchFlags
    ]);
    const value = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>({
            flags,
            flagList,
            isLoading,
            error,
            setFlag,
            refresh: fetchFlags
        }), [
        flags,
        flagList,
        isLoading,
        error,
        setFlag,
        fetchFlags
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(FeatureFlagsContext.Provider, {
        value: value,
        children: children
    });
}
function useFeatureFlags() {
    return (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(FeatureFlagsContext);
}
function useFeatureFlag(name) {
    const { flags } = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(FeatureFlagsContext);
    var _flags_name;
    return (_flags_name = flags[name]) !== null && _flags_name !== void 0 ? _flags_name : false;
}
/**
 * Gates `children` behind a feature flag: renders nothing (and redirects to
 * `redirectTo`) while the flag is disabled or still loading; renders
 * `children` once the flag is confirmed enabled.
 *
 * Intentionally usable from more than one place in a route's render tree
 * (e.g. a segment's `layout.tsx` AND its `page.tsx`) — that's not
 * duplication, it's defense in depth: the layout keeps a disabled flag from
 * ever mounting the page's client bundle, and the same check inside the
 * page guarantees the page's own render output can never show gated content
 * even if it's ever rendered outside that layout (unit tests, future route
 * restructuring). Using a Server Component `page.tsx` to render this Client
 * Component also lets the page keep a static `metadata` export, which a
 * fully-client page cannot do.
 */ function RequireFeatureFlag(param) {
    let { flag, redirectTo = "/", children } = param;
    const { flags, isLoading } = useFeatureFlags();
    const router = (0,_lib_nav__WEBPACK_IMPORTED_MODULE_2__/* .useRouter */ .KA)();
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        if (!isLoading && !flags[flag]) {
            router.replace(redirectTo);
        }
    }, [
        isLoading,
        flags,
        flag,
        redirectTo,
        router
    ]);
    if (isLoading) return null;
    if (!flags[flag]) return null;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: children
    });
}


/***/ }),

/***/ 65232:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $F: () => (/* binding */ countNeedsAttention),
/* harmony export */   B: () => (/* binding */ flattenGroups),
/* harmony export */   _Q: () => (/* binding */ groupNotifications)
/* harmony export */ });
/* unused harmony exports NEEDS_ATTENTION_KEY, selectPinnedDecisions, groupBySession */
/**
 * Groups notifications by (sessionId, notificationType).
 *
 * - Uses server-provided occurrenceCount when a record has one (>= 1)
 * - Falls back to client-side grouping for backward compatibility
 * - Groups ordered by most recent timestamp
 * - Representative notification is the most recent one (latest metadata/approval_id)
 *
 * Count precedence (see ADR in notification-deduplication.md):
 *   count = max(representative.occurrenceCount ?? 0, group.length)
 * This ensures the badge is correct both for server-deduplicated single records
 * (occurrenceCount = N, group.length = 1) and for pre-dedup stale data where
 * multiple records exist for the same key (occurrenceCount = 0, group.length = N).
 */ function groupNotifications(notifications) {
    // 1. Build a Map keyed by "sessionId:notificationType"
    const groups = new Map();
    for (const notification of notifications){
        var _notification_sessionId, _notification_notificationType;
        const key = "".concat((_notification_sessionId = notification.sessionId) !== null && _notification_sessionId !== void 0 ? _notification_sessionId : "", ":").concat((_notification_notificationType = notification.notificationType) !== null && _notification_notificationType !== void 0 ? _notification_notificationType : "");
        const existing = groups.get(key);
        if (existing) {
            existing.push(notification);
        } else {
            groups.set(key, [
                notification
            ]);
        }
    }
    // 2. For each group, sort by timestamp descending and build the result
    const result = [];
    for (const members of groups.values()){
        // Sort members by timestamp descending (most recent first)
        members.sort((a, b)=>b.timestamp - a.timestamp);
        const representative = members[0];
        var _representative_occurrenceCount;
        // The server's occurrenceCount field (proto field 12) is carried through
        // from NotificationHistoryRecord during hydration in NotificationContext.
        // When the record was server-deduplicated it will be >= 1.
        // For old records without the field it defaults to 0.
        const serverCount = (_representative_occurrenceCount = representative.occurrenceCount) !== null && _representative_occurrenceCount !== void 0 ? _representative_occurrenceCount : 0;
        // Prefer server count when available, fall back to client-side group size
        const count = Math.max(serverCount, members.length);
        const allIds = members.map((m)=>m.id);
        result.push({
            notification: representative,
            count,
            allIds
        });
    }
    // 3. Sort groups by representative's timestamp descending (most recent first)
    result.sort((a, b)=>b.notification.timestamp - a.notification.timestamp);
    return result;
}
// ---------------------------------------------------------------------------
// Tray model: per-session groups, the pinned "Needs attention" group and the
// flattened row list the virtualized tray renders.
// ---------------------------------------------------------------------------
const NEEDS_ATTENTION_KEY = "__needs_attention__";
/** Groups whose representative row is a pending decision (the server-sent field, never a type list). */ function selectPinnedDecisions(groups) {
    return groups.filter((g)=>g.notification.isPendingDecision === true);
}
/** Count behind "N need attention" and the handle's dot: one per pinned group. */ function countNeedsAttention(notifications) {
    return selectPinnedDecisions(groupNotifications(notifications)).length;
}
/** Splits non-pinned groups by session, newest session first. */ function groupBySession(groups) {
    const bySession = new Map();
    for (const group of groups){
        const key = group.notification.sessionId || "__no_session__";
        let bucket = bySession.get(key);
        if (!bucket) {
            bucket = {
                key,
                sessionName: group.notification.sessionName || group.notification.sessionId || "No session",
                groups: [],
                rowCount: 0
            };
            bySession.set(key, bucket);
        }
        bucket.groups.push(group);
        bucket.rowCount += group.allIds.length;
    }
    // `groups` arrive newest first, so insertion order is already newest-session first.
    return [
        ...bySession.values()
    ];
}
/**
 * Flattens the tray into the single ordered list the virtualizer windows over:
 * the pinned group first (never collapsible), then one header and its rows per
 * session, skipping the rows of a collapsed session.
 */ function flattenGroups(notifications, collapsed) {
    const all = groupNotifications(notifications);
    const pinned = selectPinnedDecisions(all);
    const pinnedSet = new Set(pinned);
    const rest = all.filter((g)=>!pinnedSet.has(g));
    const rows = [];
    let pos = 0;
    if (pinned.length > 0) {
        rows.push({
            kind: "header",
            key: NEEDS_ATTENTION_KEY,
            label: "NEEDS ATTENTION",
            count: pinned.length,
            collapsed: false,
            pinned: true
        });
        for (const group of pinned){
            rows.push({
                kind: "group",
                key: group.notification.id,
                group,
                pinned: true,
                posInSet: ++pos
            });
        }
    } else if (rest.length > 0) {
        rows.push({
            kind: "note",
            key: "nothing-needs-attention",
            text: "Nothing needs attention"
        });
    }
    for (const session of groupBySession(rest)){
        const isCollapsed = collapsed.has(session.key);
        rows.push({
            kind: "header",
            key: session.key,
            label: session.sessionName,
            count: session.rowCount,
            collapsed: isCollapsed,
            pinned: false
        });
        if (isCollapsed) continue;
        for (const group of session.groups){
            rows.push({
                kind: "group",
                key: group.notification.id,
                group,
                pinned: false,
                posInSet: ++pos
            });
        }
    }
    return {
        rows,
        needsAttention: pinned.length,
        setSize: pos
    };
}


/***/ }),

/***/ 66876:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  yO: () => (/* binding */ useNotificationConnectivity),
  Ru: () => (/* binding */ useStreamConnectionState)
});

// UNUSED EXPORTS: STREAM_DOWN_GRACE_MS

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-redux@9.2.0+5436edeee5f9f735/node_modules/react-redux/dist/react-redux.mjs
var react_redux = __webpack_require__(55324);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
;// ./src/lib/hooks/useOnlineStatus.ts
/* __next_internal_client_entry_do_not_use__ useOnlineStatus auto */ 
function subscribe(onChange) {
    window.addEventListener("online", onChange);
    window.addEventListener("offline", onChange);
    return ()=>{
        window.removeEventListener("online", onChange);
        window.removeEventListener("offline", onChange);
    };
}
/** `navigator.onLine`, live; the browser's own signal, not proof the server is reachable. */ function useOnlineStatus() {
    return (0,react.useSyncExternalStore)(subscribe, ()=>navigator.onLine, ()=>true);
}

// EXTERNAL MODULE: ./src/lib/contexts/notificationContexts.ts
var notificationContexts = __webpack_require__(82514);
;// ./src/lib/hooks/useNotificationConnectivity.ts
/* __next_internal_client_entry_do_not_use__ STREAM_DOWN_GRACE_MS,useStreamConnectionState,useNotificationConnectivity auto */ 




/** How long the stream may be non-live before we start to believe it is a real outage (reconnect cycles are routine). */ const STREAM_DOWN_GRACE_MS = 10000;
const neverNotifies = ()=>()=>{};
const CONNECTED = "connected";
/** The raw WatchSessions stream state; "connected" outside a Redux provider so components render in isolation. */ function useStreamConnectionState() {
    const redux = (0,react.useContext)(react_redux/* ReactReduxContext */.ty);
    const store = redux === null || redux === void 0 ? void 0 : redux.store;
    const subscribe = (0,react.useCallback)((onChange)=>store ? store.subscribe(onChange) : neverNotifies(), [
        store
    ]);
    const getSnapshot = (0,react.useCallback)(()=>store ? (0,sessionsSlice/* selectConnectionState */.mN)(store.getState()) : CONNECTED, [
        store
    ]);
    return (0,react.useSyncExternalStore)(subscribe, getSnapshot, ()=>CONNECTED);
}
function useSessionsLoadedOnce() {
    const redux = (0,react.useContext)(react_redux/* ReactReduxContext */.ty);
    const store = redux === null || redux === void 0 ? void 0 : redux.store;
    const subscribe = (0,react.useCallback)((onChange)=>store ? store.subscribe(onChange) : neverNotifies(), [
        store
    ]);
    const getSnapshot = (0,react.useCallback)(()=>store ? (0,sessionsSlice/* selectSessionsHasLoadedOnce */.Qr)(store.getState()) : true, [
        store
    ]);
    return (0,react.useSyncExternalStore)(subscribe, getSnapshot, ()=>true);
}
/**
 * Offline means "notification data cannot be reached", and is claimed only on evidence:
 *  - the browser reports no network, or
 *  - the live stream has been down past a grace period AND the history fetch is failing too.
 * A non-live stream alone (connecting, reconnecting, stale after a tab refocus) is not offline:
 * history refreshes by RPC on its own (useNotificationHistory), so the list is still current.
 */ function useNotificationConnectivity() {
    const state = useStreamConnectionState();
    const browserOnline = useOnlineStatus();
    const { historyError } = (0,notificationContexts/* useNotificationState */.IB)();
    const sessionsLoadedOnce = useSessionsLoadedOnce();
    // Until the first session snapshot lands the stream is "connecting", not "dropped": a cold deep link must
    // not read as an outage. Known limit: a first load that fails outright keeps this false, so only the
    // browser-offline signal can claim offline in that case.
    const streamLive = state === CONNECTED || !sessionsLoadedOnce;
    const [streamDownLong, setStreamDownLong] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        if (streamLive) {
            setStreamDownLong(false);
            return;
        }
        const timer = setTimeout(()=>setStreamDownLong(true), STREAM_DOWN_GRACE_MS);
        return ()=>clearTimeout(timer);
    }, [
        streamLive
    ]);
    const isOffline = !browserOnline || streamDownLong && historyError !== null;
    return {
        state,
        isOffline
    };
}


/***/ }),

/***/ 69250:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ useCoarsePointer)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ useCoarsePointer auto */ 
const QUERY = "(pointer: coarse)";
/** True while the primary input is a finger. False where matchMedia is unavailable (SSR, tests). */ function useCoarsePointer() {
    const [coarse, setCoarse] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(()=> true && typeof window.matchMedia === "function" ? window.matchMedia(QUERY).matches : false);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (typeof window.matchMedia !== "function") return;
        const mq = window.matchMedia(QUERY);
        const update = ()=>setCoarse(mq.matches);
        update();
        mq.addEventListener("change", update);
        return ()=>mq.removeEventListener("change", update);
    }, []);
    return coarse;
}


/***/ }),

/***/ 70562:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   w: () => (/* binding */ useNowTicker)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ useNowTicker auto */ 
/** Returns `Date.now()`, re-read every `intervalMs`. One call per panel = one shared ticker. */ function useNowTicker() {
    let intervalMs = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : 30000;
    const [now, setNow] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(()=>Date.now());
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        setNow(Date.now());
        const id = setInterval(()=>setNow(Date.now()), intervalMs);
        return ()=>clearInterval(id);
    }, [
        intervalMs
    ]);
    return now;
}


/***/ }),

/***/ 71249:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   BK: () => (/* binding */ capBadgeCount),
/* harmony export */   Ds: () => (/* binding */ notificationTypeIcon),
/* harmony export */   MD: () => (/* binding */ notificationTypeLabel),
/* harmony export */   QE: () => (/* binding */ mapNotificationType),
/* harmony export */   f: () => (/* binding */ computeScopedMarkReadIds),
/* harmony export */   rK: () => (/* binding */ splitCIBlockMessage),
/* harmony export */   uI: () => (/* binding */ notificationTypeFilter),
/* harmony export */   vV: () => (/* binding */ mapPriority),
/* harmony export */   yA: () => (/* binding */ priorityColor),
/* harmony export */   yL: () => (/* binding */ isReconciledNotification)
/* harmony export */ });
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
/**
 * Canonical notification mappings.
 *
 * - Proto → UI type: mapNotificationType, mapPriority
 * - UI type → display: notificationTypeIcon, notificationTypeLabel, priorityColor,
 *   notificationTypeFilter
 *
 * Components import from here. Never duplicate these switch statements in UI files.
 */ 
function mapNotificationType(protoType) {
    switch(protoType){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.APPROVAL_NEEDED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.CONFIRMATION_NEEDED:
            return "approval_needed";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.INPUT_REQUIRED:
            return "question";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.ERROR:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.FAILURE:
            return "error";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.WARNING:
            return "warning";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.TASK_COMPLETE:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.PROCESS_FINISHED:
            return "task_complete";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.PROCESS_STARTED:
            return "progress";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.AUTO_APPROVED:
            return "auto_approved";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.UNSPECIFIED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.INFO:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.DEBUG:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.STATUS_CHANGE:
            return "info";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationType */ ._n.CUSTOM:
            return "custom";
        default:
            // Fail-safe: an unmapped backend NotificationType must not silently
            // land in the informational bucket — that's the exact bug Task 3.1.1a
            // fixes one stage downstream. Warn for triage and default to an
            // actionable type so it surfaces in NeedsDecisionSection instead of
            // disappearing into collapsed informational activity. "warning" is
            // used (not "approval_needed") since it carries no approve/deny
            // button assumptions elsewhere in the rendering pipeline.
            console.warn("[notificationMapping] Unmapped NotificationType ".concat(protoType, '; defaulting to "warning" (actionable fail-safe)'));
            return "warning";
    }
}
function mapPriority(protoPriority) {
    switch(protoPriority){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationPriority */ .AE.URGENT:
            return "urgent";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationPriority */ .AE.HIGH:
            return "high";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationPriority */ .AE.MEDIUM:
            return "medium";
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .NotificationPriority */ .AE.LOW:
            return "low";
        default:
            return "medium";
    }
}
// ---------------------------------------------------------------------------
// UI display helpers — used by both NotificationToast and NotificationPanel
// ---------------------------------------------------------------------------
function notificationTypeIcon(type) {
    switch(type){
        case "approval_needed":
            return "⚠️";
        case "auto_approved":
            return "✅";
        case "error":
            return "❌";
        case "warning":
            return "⚠️";
        case "task_complete":
            return "✅";
        case "task_failed":
            return "💥";
        case "progress":
            return "⏳";
        case "question":
            return "❓";
        case "reminder":
            return "⏰";
        case "system":
            return "⚙️";
        default:
            return "🔔";
    }
}
function notificationTypeLabel(type) {
    switch(type){
        case "approval_needed":
            return "Approval Needed";
        case "auto_approved":
            return "Auto-handled";
        case "error":
            return "Error";
        case "warning":
            return "Warning";
        case "task_complete":
            return "Task Complete";
        case "task_failed":
            return "Task Failed";
        case "progress":
            return "Progress";
        case "question":
            return "Question";
        case "reminder":
            return "Reminder";
        case "system":
            return "System";
        case "custom":
            return "Custom";
        default:
            return "Info";
    }
}
function priorityColor(priority) {
    switch(priority){
        case "urgent":
            return "var(--color-error, #f44336)";
        case "high":
            return "var(--color-warning, #ff9800)";
        case "medium":
            return "var(--color-info, #2196f3)";
        case "low":
            return "var(--color-success, #4caf50)";
        default:
            return "var(--color-primary, #0070f3)";
    }
}
/**
 * Returns the unread notification IDs that are safe for a bulk "mark read"
 * action to touch: everything except a pending decision, which must only leave
 * "needs a decision" by being resolved. "Pending decision" is the server-sent
 * `isPendingDecision`, the same field the toast stack pins on. Shared by
 * NotificationsPage's "Mark activity read" and NotificationPanel's bulk-read
 * button so the scoping rule is defined once (Task 3.1.2e / 3.1.5a).
 */ function computeScopedMarkReadIds(notifications) {
    return notifications.filter((n)=>!n.isRead && !n.isPendingDecision).map((n)=>n.id);
}
/**
 * Caps a raw badge count at "99+", matching NavBadge.tsx's convention.
 */ function capBadgeCount(n) {
    return n > 99 ? "99+" : String(n);
}
function isApprovalCategory(t) {
    return t === "approval_needed" || t === "question";
}
function isErrorCategory(t) {
    return t === "error" || t === "task_failed" || t === "warning";
}
/**
 * Returns the set of UI notification types that belong to a given filter category.
 * The "error" pill covers task_failed and warning; "info" is the complement of the
 * approval and error pills so a new UI type always lands somewhere. These are
 * display filters only; whether something is a pending decision is the server field.
 */ function notificationTypeFilter(category, types) {
    switch(category){
        case "approval_needed":
            return types.filter(isApprovalCategory);
        case "error":
            return types.filter(isErrorCategory);
        case "task_complete":
            return types.filter((t)=>t === "task_complete");
        case "info":
            return types.filter((t)=>!isApprovalCategory(t) && !isErrorCategory(t));
        default:
            return types;
    }
}
/**
 * ApprovalService appends a trailing " https://.../checks" URL to the CI-block error
 * message (AC5/AC6) — split it out so approval UIs can render it as a "View CI run"
 * link instead of a raw URL in the warning text.
 */ function splitCIBlockMessage(message) {
    const match = message.match(/\s(https?:\/\/\S+)$/);
    if (!match) {
        return {
            text: message
        };
    }
    return {
        text: message.slice(0, match.index),
        checksUrl: match[1]
    };
}
/**
 * Whether a notification's resolution came from rule-reconciliation rather than a
 * live human decision (Epic 2.1's `IsReconciled()` on the Go side — this is the
 * single shared TS accessor for the same `metadata.reconciled` check, used by every
 * UI branch that needs to distinguish the two instead of each inlining its own
 * `metadata?.reconciled === "true"` comparison.
 */ function isReconciledNotification(n) {
    var _n_metadata;
    return ((_n_metadata = n.metadata) === null || _n_metadata === void 0 ? void 0 : _n_metadata.reconciled) === "true";
}


/***/ }),

/***/ 76468:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   o: () => (/* binding */ useSessionHidden)
/* harmony export */ });
/* unused harmony export resetSessionHiddenCache */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var react_redux__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(55324);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* harmony import */ var _lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(60795);
/* harmony import */ var _lib_utils_hiddenSessionRegistry__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(46765);
/* __next_internal_client_entry_do_not_use__ resetSessionHiddenCache,useSessionHidden auto */ 






// A session never becomes visible after it was created hidden, so both answers are cached.
const hiddenBySessionId = new Map();
const inFlight = new Map();
// A deleted session stays deleted; only NotFound is remembered, so a network error retries.
const missingSessionIds = new Set();
/** Test seam: forget cached answers. */ function resetSessionHiddenCache() {
    hiddenBySessionId.clear();
    inFlight.clear();
    missingSessionIds.clear();
}
function lookupViaRpc(sessionId) {
    const pending = inFlight.get(sessionId);
    if (pending) return pending;
    const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
    // One lookup is shared by every toast of the session, so no single caller owns it.
    // abort-signal-exempt
    const lookup = client.getSession({
        id: sessionId
    });
    const request = lookup.then((res)=>{
        if (!res.session) return undefined;
        hiddenBySessionId.set(sessionId, res.session.hidden);
        if (res.session.hidden) (0,_lib_utils_hiddenSessionRegistry__WEBPACK_IMPORTED_MODULE_4__/* .rememberHiddenSession */ .iB)({
            id: sessionId,
            title: res.session.title
        });
        return res.session.hidden;
    }).catch((err)=>{
        if ((err === null || err === void 0 ? void 0 : err.code) === 5 || (err === null || err === void 0 ? void 0 : err.code) === "not_found") missingSessionIds.add(sessionId);
        return undefined;
    }).finally(()=>inFlight.delete(sessionId));
    inFlight.set(sessionId, request);
    return request;
}
/**
 * Whether the session behind a notification is hidden (a Background session). The live
 * list never carries hidden sessions, so an id absent from it is asked of the server once.
 * `undefined` while unknown, for a deleted session, and outside a Redux Provider. Toasts
 * render above the session-service context, so this reads the store directly.
 * With `lookup` false the server is never asked (a closed tray keeps its rows mounted);
 * cached and live-list answers still apply.
 */ function useSessionHidden(sessionId) {
    let lookup = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : true;
    const redux = (0,react__WEBPACK_IMPORTED_MODULE_0__.useContext)(react_redux__WEBPACK_IMPORTED_MODULE_6__/* .ReactReduxContext */ .ty);
    const [hidden, setHidden] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(()=>sessionId ? hiddenBySessionId.get(sessionId) : undefined);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        // Reused rows and toasts must not keep the previous session's answer.
        setHidden(sessionId ? hiddenBySessionId.get(sessionId) : undefined);
        if (!sessionId || !redux) return;
        const cached = hiddenBySessionId.get(sessionId);
        if (cached !== undefined) {
            setHidden(cached);
            return;
        }
        const live = (0,_lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_3__/* .selectAllSessions */ .rt)(redux.store.getState()).find((s)=>s.id === sessionId);
        if (live) {
            hiddenBySessionId.set(sessionId, live.hidden);
            if (live.hidden) (0,_lib_utils_hiddenSessionRegistry__WEBPACK_IMPORTED_MODULE_4__/* .rememberHiddenSession */ .iB)({
                id: sessionId,
                title: live.title
            });
            setHidden(live.hidden);
            return;
        }
        if (!lookup || missingSessionIds.has(sessionId)) return;
        let cancelled = false;
        void lookupViaRpc(sessionId).then((answer)=>{
            if (!cancelled) setHidden(answer);
        });
        return ()=>{
            cancelled = true;
        };
    }, [
        sessionId,
        redux,
        lookup
    ]);
    return hidden;
}


/***/ }),

/***/ 76863:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   s: () => (/* binding */ srOnly)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FLiveRegion_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll81M2V5NjAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlci13aWR0aDogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(6112);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FLiveRegion_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll81M2V5NjAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlci13aWR0aDogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FLiveRegion_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll81M2V5NjAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlci13aWR0aDogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var srOnly = '_53ey600';

/***/ }),

/***/ 76991:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ markSessionViewed),
/* harmony export */   l: () => (/* binding */ isSessionViewed)
/* harmony export */ });
/**
 * Which sessions the operator is looking at right now. SessionDetailView reports
 * its session here while mounted and the tab is visible; the notification
 * context uses it to keep a non-pinned toast for that session off the deck (the
 * history row is still recorded). A multiset, because two panes can show one session.
 */ const counts = new Map();
function markSessionViewed(sessionId) {
    if (!sessionId) return ()=>{};
    var _counts_get;
    counts.set(sessionId, ((_counts_get = counts.get(sessionId)) !== null && _counts_get !== void 0 ? _counts_get : 0) + 1);
    let released = false;
    return ()=>{
        if (released) return;
        released = true;
        var _counts_get;
        const next = ((_counts_get = counts.get(sessionId)) !== null && _counts_get !== void 0 ? _counts_get : 1) - 1;
        if (next <= 0) counts.delete(sessionId);
        else counts.set(sessionId, next);
    };
}
function isSessionViewed(sessionId) {
    if (!sessionId || !counts.has(sessionId)) return false;
    return typeof document === "undefined" || document.visibilityState !== "hidden";
}


/***/ }),

/***/ 80070:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   N: () => (/* binding */ GLOBAL_SCOPE)
/* harmony export */ });
/** The scope literal a client sends for a flag's global value (mirrors the server's globalScopeLiteral). */ const GLOBAL_SCOPE = "global";


/***/ }),

/***/ 82254:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $5: () => (/* binding */ autoHandledMeta),
/* harmony export */   $M: () => (/* binding */ groupHeaderPinned),
/* harmony export */   $O: () => (/* binding */ denyButton),
/* harmony export */   $T: () => (/* binding */ itemFooter),
/* harmony export */   $s: () => (/* binding */ headerActions),
/* harmony export */   A1: () => (/* binding */ typeIcon),
/* harmony export */   AS: () => (/* binding */ item),
/* harmony export */   BI: () => (/* binding */ emptyText),
/* harmony export */   C2: () => (/* binding */ traySettings),
/* harmony export */   C3: () => (/* binding */ needsDecisionEmptySubtext),
/* harmony export */   DC: () => (/* binding */ sheetGrabber),
/* harmony export */   DD: () => (/* binding */ title),
/* harmony export */   DT: () => (/* binding */ removeButton),
/* harmony export */   Dd: () => (/* binding */ typeLabel),
/* harmony export */   Dw: () => (/* binding */ emptyIcon),
/* harmony export */   E0: () => (/* binding */ autoHandledDecision),
/* harmony export */   E1: () => (/* binding */ traySettingsRow),
/* harmony export */   EC: () => (/* binding */ itemSubtitle),
/* harmony export */   Eh: () => (/* binding */ groupNote),
/* harmony export */   F_: () => (/* binding */ incompleteSearchNoticeButton),
/* harmony export */   HN: () => (/* binding */ unreadBadge),
/* harmony export */   Ie: () => (/* binding */ empty),
/* harmony export */   Jt: () => (/* binding */ trayBanner),
/* harmony export */   L: () => (/* binding */ autoHandledBadge),
/* harmony export */   LF: () => (/* binding */ read),
/* harmony export */   Li: () => (/* binding */ itemHeader),
/* harmony export */   Lj: () => (/* binding */ stalenessRetry),
/* harmony export */   Ly: () => (/* binding */ trayMenuDivider),
/* harmony export */   M2: () => (/* binding */ stalenessIndicator),
/* harmony export */   MT: () => (/* binding */ approvalDetails),
/* harmony export */   Mi: () => (/* binding */ trayMenu),
/* harmony export */   NJ: () => (/* binding */ statusSlot),
/* harmony export */   NM: () => (/* binding */ trayButton),
/* harmony export */   Nc: () => (/* binding */ itemContext),
/* harmony export */   Np: () => (/* binding */ focusButton),
/* harmony export */   O3: () => (/* binding */ trayHandleDot),
/* harmony export */   Oj: () => (/* binding */ trayConfirmActions),
/* harmony export */   Oo: () => (/* binding */ autoHandledHeaderLeft),
/* harmony export */   P0: () => (/* binding */ autoHandledChevronOpen),
/* harmony export */   PQ: () => (/* binding */ swipeRow),
/* harmony export */   PY: () => (/* binding */ autoHandledChevron),
/* harmony export */   Pc: () => (/* binding */ searchRow),
/* harmony export */   QD: () => (/* binding */ trayUndoBar),
/* harmony export */   Qf: () => (/* binding */ loadMoreButton),
/* harmony export */   Qo: () => (/* binding */ trayFooter),
/* harmony export */   Qs: () => (/* binding */ content),
/* harmony export */   RM: () => (/* binding */ traySelect),
/* harmony export */   RV: () => (/* binding */ trayConfirm),
/* harmony export */   Rt: () => (/* binding */ searchInput),
/* harmony export */   TE: () => (/* binding */ trayHandle),
/* harmony export */   TN: () => (/* binding */ itemActions),
/* harmony export */   TZ: () => (/* binding */ ciBlockedRow),
/* harmony export */   UX: () => (/* binding */ loadMore),
/* harmony export */   VS: () => (/* binding */ filterPills),
/* harmony export */   Vt: () => (/* binding */ markAllButton),
/* harmony export */   Wz: () => (/* binding */ approveButton),
/* harmony export */   XT: () => (/* binding */ sheetGrabberRow),
/* harmony export */   Xi: () => (/* binding */ ciBlockedLink),
/* harmony export */   Xn: () => (/* binding */ incompleteSearchNotice),
/* harmony export */   ZS: () => (/* binding */ autoHandledContent),
/* harmony export */   Zm: () => (/* binding */ unreadDot),
/* harmony export */   _J: () => (/* binding */ needsDecisionEmptyText),
/* harmony export */   _O: () => (/* binding */ autoHandledTimestamp),
/* harmony export */   _y: () => (/* binding */ filterPill),
/* harmony export */   aF: () => (/* binding */ searchClearButton),
/* harmony export */   b: () => (/* binding */ closeButton),
/* harmony export */   cG: () => (/* binding */ sheetScrim),
/* harmony export */   cH: () => (/* binding */ groupToggle),
/* harmony export */   cK: () => (/* binding */ trayQuietBadge),
/* harmony export */   e: () => (/* binding */ itemWorkingDir),
/* harmony export */   gM: () => (/* binding */ needsDecisionHeading),
/* harmony export */   gb: () => (/* binding */ autoHandledList),
/* harmony export */   gy: () => (/* binding */ unread),
/* harmony export */   hm: () => (/* binding */ itemMessage),
/* harmony export */   i6: () => (/* binding */ needsDecisionEmptyIcon),
/* harmony export */   ie: () => (/* binding */ approvalTool),
/* harmony export */   ig: () => (/* binding */ approvalCwd),
/* harmony export */   j8: () => (/* binding */ autoHandledSection),
/* harmony export */   jY: () => (/* binding */ approvalCommand),
/* harmony export */   kT: () => (/* binding */ filterBar),
/* harmony export */   k_: () => (/* binding */ autoHandledTitle),
/* harmony export */   kq: () => (/* binding */ trayVariant),
/* harmony export */   lP: () => (/* binding */ emptySubtext),
/* harmony export */   mU: () => (/* binding */ ciBlockedText),
/* harmony export */   mt: () => (/* binding */ clearButton),
/* harmony export */   nc: () => (/* binding */ autoHandledItem),
/* harmony export */   oU: () => (/* binding */ virtualViewport),
/* harmony export */   p9: () => (/* binding */ needsDecisionClearFilterButton),
/* harmony export */   p_: () => (/* binding */ list),
/* harmony export */   qM: () => (/* binding */ viewButton),
/* harmony export */   rb: () => (/* binding */ trayErrorLink),
/* harmony export */   rh: () => (/* binding */ trayMenuItem),
/* harmony export */   ro: () => (/* binding */ needsDecisionSection),
/* harmony export */   rt: () => (/* binding */ needsDecisionEmpty),
/* harmony export */   s8: () => (/* binding */ filterPillActive),
/* harmony export */   tU: () => (/* binding */ trayCounts),
/* harmony export */   u: () => (/* binding */ trayMenuCaption),
/* harmony export */   uk: () => (/* binding */ filterPillExcludeActive),
/* harmony export */   ur: () => (/* binding */ newPill),
/* harmony export */   uw: () => (/* binding */ resolvedBadge),
/* harmony export */   v9: () => (/* binding */ trayAttention),
/* harmony export */   vE: () => (/* binding */ timestamp),
/* harmony export */   w4: () => (/* binding */ needsDecisionHeadingRow),
/* harmony export */   wp: () => (/* binding */ itemTitle),
/* harmony export */   wt: () => (/* binding */ virtualRow),
/* harmony export */   wx: () => (/* binding */ header),
/* harmony export */   x_: () => (/* binding */ groupHeader),
/* harmony export */   z2: () => (/* binding */ countBadge),
/* harmony export */   zt: () => (/* binding */ autoHandledHeader)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FNotificationPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1cW4_buBV_z68gplhkvLUCSb6OghQFCvSp_7J96SIIFrRE28zIokLRM55Z5L8XEimJpEjqYk_KBQpks7HE6_G5fOdCffh9ebgkceaDP94BkOAiT_FLBPYpunx8BwBM8SHzMEOnIgIxyhii5eOv54Lh_YsXk4yhjEWgyGGMvB1izwhlZYtyAO_ZwjwC5d_lo0P54wnSe8_7PXhAryF9DV9n5ZscJgnODhEIKDqB4EO4ouhUvtgRmiDq7Qhj5BSBIL_AgqQ40YZB1Sg7GD8eKDlniReTlFB9ss2sWVhxpDh7jID_8d33d58TyKBXHBFin_5yhB7vvoAPClma9S3zC9jml2Y3Pv_1_V3dPqja70nGvAK_okjeTPX0GeHDkUVg7fvlsxOkB5xVCxlOfT71h3pc42b9Wd_Wukutxmv3EqosgbMUZ8gbyxntqxPOvGecsGNJlXrxR0GP9klDbF_aY9_ZBr7OAvgEDygC5Zoh9Q4UJhhl7J4edvDenwPx50O4ms2B4eFMouzzETMksWM51rmIQBByVpBo6H_Y2I_7Je1ilLTZZKkmj_uctbnEwRT93f4Gfq66VpuOQCBvYKnKBV_JvPvugTEKsyKHFGWspWWvTKsUX3OCx2dalEPmBDc0ks9gu_mVDcDXgxkmWQRgmpbHXgAEC6TsMzqSJ0Sr3fbx4HYm91z9mSkUjCDRajKJ1tfZnPaV0CqhplPCsZTOSGaS85X_Uz9NA7c_DjoE1Vdlpu56NHX7hFqlvti5ZIy8FO1ZBOCZEXkhG24uUnSpVAEA5bL2KXn2YBYfy8XwHtKLF8OzSwSOOEk4SGilQ55pa2aLSgsmmKKY0y8m6fmUTWOYxur4_GibhdQsw9CFedW4cjfrybaLf9Ct6lKMKIjbQBnxmOQwxuyllMmFPA7sAonAjCRWGpJo7CY_VAc6qOfa6XPJ_kFBKAMIEI87vbZjoirM66Agb8w5eaE1zSkmFLMXLkSDoON6JjOxzMISqPGnSXgyWX8iS58azgRBOAfhclFCGn_jdN1z4W_ZbyO_PdzYGdB4v8Ut9YTHURNq_He86TcAKkmxqUcqL7JglAjiNAqND7OjCD56z4QmlT0iNPGqR_obPo0PgkYz2mbDZnhucmDqLl_7crx2eh_Vlqs4dk_oKQLnPEc0rvgTgBQxhqhXHqZAMAuOLVplmV9Kj8hgMUXLDn4eDc_XBnS_5OC8GrVaHiott3yQZvI8dsnTg4JmDuY1aeF2rvTGc40UjebUJ7Fpu43TdLlsB8l0JLzYCCw8GAobSXaSVL3dIJyuQMlmX04YXAc4tjCnRJIROv80s1KAKjqdGP31XUriRwkki_BFjYDEz2GLMOHidv78VnBeUolLoV_KR_WS62c_Bta7hDbs85JkE_tWTjUKUInZ9Fac73wkYhgEAr9Vw6koctPCSIuRrVisjeEsB05GbwwwdESg2_d25mKsYg6VhTOrLnP49aPkuZdtb627hqFUNoLnMoVkZ_NZO_Rf8QHkoZ6MMZVguB35cwSdnjito4yw_yjBBdylKJlNcBCemt463F_JO8pI6eum5Bkpxv_Z4cmp4HkYTnfEwrrOa5e8_ejJ7gnV0NFfdt3D0MILFxMv_FUKoPxP0d4XnTFDvsG1aSdOyZzEoR2DOAZAdfha2paDEfkKrOPKg1zDhC_G2ISDfGOwdR1PaSd7HRt50uF2w8eNXTAzfuuW_WieNdJSyl_xhJeDjC6vJ5BySnt4wulLBN7_5_kIx4v6LDOYX0_Rz8g2QFSWExB_hXeIwpKqoGz0fg5_QVlK5uBEMlJZ_a70OBWBngbqCIICzHOKvFqHdZ0hmKYfrUHKILQTKpzZZF7qv_iojA5vmPdtcuT7tMb5hlrjbNYavaAiWBVzoPwQqoD_VIkz3QKe7fCqmGmT2BTTcohmCjo5sLc8wTrBeZMjFIO97Rkup59hvT7TOzbTZrnuEFdXJRu4P8cgZV1rIE2yHut0vKqb3Iztzy17Fd5LUEy4io3AOUsQLV02ZfStMW7Q45N1sY6Z513hRwszdyTBpcK7sZXg4S3rFhrRbp3esAtwGwAwxCEM7My_Ngl94BvKDtZbJ6mV1J6JReFE6GMA95vV_6KKp9nJzrwTbb1GNpBG4Wms_tj1NGHXg7G4gtc4nMOSUg4YRM6sZP02kqBodwnAc_1d_aFzsCMlZWFCnqXHfJWXcocVAZojVauf4ijKUxijI0kTESwYEFQJ4mhP4nMxyLEA0gKFw1jB4aqDd8KXe5yBgh52805nEPo_zWVtoq4iaTLcMmeZoz02E39z167DCwMtfJ9rFyhpx16aD_DzAuQUvFYfOqJzwX4spro5weuirhtR3BTykYWqz5jtb35MhxFZh4kzqnmJ4DgG9LmmHIELsUwxRvI3NztfbxzStpseLgqKgR2bCemNZbprTVwVTlLhTIk2hjkVnRBo8HVyVUTwOCVpueh66On_caS1gDU4uWoNBibLmsoDNWXmEDKeSpYqFihhkKH7YOsn6KCyARmLaI3lPfDS5h39p_NHY32bNOuQJKjRa7RFyafiaJs8O1hHLgdaqbuKUlgwLz7iNJEVa70UrleUDmNTlOZUZ_DN4fqaHJ0ufBOuT2UD_A_uuv7UL200VaTE0jZwyEq0VfKxibWYARsSsqy3wMy5HFUUnXj48F52lKmHMqTic9cLC_1tWSuFCHcwQLxUEh_gb3dvTX60hc1lT4QtG44I3XLrkRcxFmsNaAwM_ikUtTWi53dfoT2iitpgldN3PUUdvtbjqk7Q1h9qzWpABMGHxQJCy2XoG5ZedzmtjvqfWTNsT3eGBpuIXXgTRh2ZflhMSCI5Do0tcojXBiSd0E3WTaQgcOlJFe16jCAJkNNxhhw_yPd9N4MbLgc4f9J3cTtE1JDrT2_oMrevHo4S0pDJ9TNlfjIFhO6QfRKD_OUjrnpplbZ9gkXeIfTCj_0i7NiTeFweOTM5lJf4BeiIJU3Uikq7lWUePPTHclRdvelIrI8d_XvVIu1eQmqKOyrA_KkAGfjsgPlXCh4UCSpJU_whLP7pe_nl3mJRJ9bNM3NQfXPFDL0n_vSZ_zmjIfZznBt26d5Lk3ieaZChJltuzrByz1fwyN62RFIE4Gx58DPL7M6cMebexl8Ul7PZK8phmksxnrC6DknlDWNA99Pno4z8HN5mpuZDufdvWzU_a1D3SHBhaZldRbdWFP7vqJXp4FQbp5MwYoTqv2j7Om_gHvkQYqgh7MCMdGSUwx4PRR1e2aKtPB_rcPGn_KB9acDJWszhuN_63Ccct8JXXKYJSgRI4xlk_3KOHjNpsrqrh17O0H_F_vq1Fe1_Nd8IfqaGIG25yz3qOY1tWck77au19PHaW_kk7ZTdNKRnVLej1H44uU4y1Dy6Y7RM7r7Av5ygjirAZqMKwQpu9pWWs_DzbpWFBHHpltaKzaQBrZk4a67ha84XhVm3GoXOTrX2MOdySXWC7BvUOrNyDk_elBgDT08eaBw1wOrVM9C3kEU7dCeUCSyUGLquztJpJa_WgxvvsxiSAi4slkhTx0OLGKXzqBW9reLOFaH3Q9NJyHerXLFx3jLylId60C7sVQz8hlSDL26uuPT_1Jw33_ZUgYbxuaxOvWIQypG6sFyiopCG2tYpGsEjBeJUU_gSr7gOqtrKwI8yclfj_z3lU4KNQlPnL6Z0a82umga8yPnsAOSwaGWAtzmFxCsDZFqaVi7aG5kig8LnDo4_sYXFoQI2ePd4a0vYlbTaqCSf9UkMKgJlQQP40lpyKU3NrNgOH58_ShBIGnfR83Awl1B0jO_RFg1r0BXiSnAX8vlz3QAJWnfcO139zt8q9PUouq8ljwcLg3u62KmIIVgnEsuEQw3NsdqYNblKYdSLFHL0g02stYLEtebglHxCKvhk_nyNgYF304ve7pixrc0VoYrwf33UKX_j4qrEwgcIwJ3TUm5XZI0IJ8qkXFZtbdxUcUnnih9PfZgbJlbv6K2XJSVe3fYtRoOZeqBZf0U2kqCfA2Vbk8FS8r1NlbSpEKkufN_Dej388eAqOTU6LC01m_aeaMohQw_oY963YfUh_aZRJE9aiJvtoGKNzwkzZsp_7cecoH3BrBtDJKd8t0DiYJMjsZzii9dZftS1_Mt_WqZ8ldadMHxODsiillNN_WBTjTplU4y6ZXL2zu_oRXVropW3NjnRriuFk7JQ4fPDjlXIx85zLwXLrVp6sVHmJVfx2iorYx6ubEAy0LrktbB4e5xn1jU99OJ5LxYNfubZmo737eqZcVhAF_HGKHG1dLCTJpJb8C2VUs6KjC_Dc_Ot8tf_La4ZnXoi9VPupvlCHBthWK6IuE4MeClU3zbD1Mfbmv2OQUs3lK1HAmLSJ5iqcUqxbe1pzqn3fvfdO_90zqRI_V3VG7gUluG25uw_R8tA7gi5At_cvnmwn_zmNeiUxdh_UBSXYyjfN7CjA78UCrfMIMKaQmhTcQEVVvlICzNukeMhgWJB4aef5iEqJ6IzSAPK9tZhG_HMQv1GpCaLqh_m78rY6dloFLMtsIH6XOOGWF4j_OKDFU_69PdU3j3BUS4uN_dGSPZHBQoRTGbA5zlZ_aZveTo012BII2PdyLKYeKCTp6ib0r4_UjR_suU4uiB8_9hBIMQRFThDIAswTcl4l70Wqz3pY57XL6_pi43TJV6QlG_i41DkRjy7cV5aZJd9ytfeBctJYrWbWpF30LNdNIGWN5gzFiMUZvw8TR0JjjUnrvnb0dt_m_t1xwn1O0R7TwKErOMUq8E6nBdPlbZQRR96QautoOy0sThSr9DRe_s_H3_wLLA_HmQF0AAA_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(52162);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FNotificationPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1cW4_buBV_z68gplhkvLUCSb6OghQFCvSp_7J96SIIFrRE28zIokLRM55Z5L8XEimJpEjqYk_KBQpks7HE6_G5fOdCffh9ebgkceaDP94BkOAiT_FLBPYpunx8BwBM8SHzMEOnIgIxyhii5eOv54Lh_YsXk4yhjEWgyGGMvB1izwhlZYtyAO_ZwjwC5d_lo0P54wnSe8_7PXhAryF9DV9n5ZscJgnODhEIKDqB4EO4ouhUvtgRmiDq7Qhj5BSBIL_AgqQ40YZB1Sg7GD8eKDlniReTlFB9ss2sWVhxpDh7jID_8d33d58TyKBXHBFin_5yhB7vvoAPClma9S3zC9jml2Y3Pv_1_V3dPqja70nGvAK_okjeTPX0GeHDkUVg7fvlsxOkB5xVCxlOfT71h3pc42b9Wd_Wukutxmv3EqosgbMUZ8gbyxntqxPOvGecsGNJlXrxR0GP9klDbF_aY9_ZBr7OAvgEDygC5Zoh9Q4UJhhl7J4edvDenwPx50O4ms2B4eFMouzzETMksWM51rmIQBByVpBo6H_Y2I_7Je1ilLTZZKkmj_uctbnEwRT93f4Gfq66VpuOQCBvYKnKBV_JvPvugTEKsyKHFGWspWWvTKsUX3OCx2dalEPmBDc0ks9gu_mVDcDXgxkmWQRgmpbHXgAEC6TsMzqSJ0Sr3fbx4HYm91z9mSkUjCDRajKJ1tfZnPaV0CqhplPCsZTOSGaS85X_Uz9NA7c_DjoE1Vdlpu56NHX7hFqlvti5ZIy8FO1ZBOCZEXkhG24uUnSpVAEA5bL2KXn2YBYfy8XwHtKLF8OzSwSOOEk4SGilQ55pa2aLSgsmmKKY0y8m6fmUTWOYxur4_GibhdQsw9CFedW4cjfrybaLf9Ct6lKMKIjbQBnxmOQwxuyllMmFPA7sAonAjCRWGpJo7CY_VAc6qOfa6XPJ_kFBKAMIEI87vbZjoirM66Agb8w5eaE1zSkmFLMXLkSDoON6JjOxzMISqPGnSXgyWX8iS58azgRBOAfhclFCGn_jdN1z4W_ZbyO_PdzYGdB4v8Ut9YTHURNq_He86TcAKkmxqUcqL7JglAjiNAqND7OjCD56z4QmlT0iNPGqR_obPo0PgkYz2mbDZnhucmDqLl_7crx2eh_Vlqs4dk_oKQLnPEc0rvgTgBQxhqhXHqZAMAuOLVplmV9Kj8hgMUXLDn4eDc_XBnS_5OC8GrVaHiott3yQZvI8dsnTg4JmDuY1aeF2rvTGc40UjebUJ7Fpu43TdLlsB8l0JLzYCCw8GAobSXaSVL3dIJyuQMlmX04YXAc4tjCnRJIROv80s1KAKjqdGP31XUriRwkki_BFjYDEz2GLMOHidv78VnBeUolLoV_KR_WS62c_Bta7hDbs85JkE_tWTjUKUInZ9Fac73wkYhgEAr9Vw6koctPCSIuRrVisjeEsB05GbwwwdESg2_d25mKsYg6VhTOrLnP49aPkuZdtb627hqFUNoLnMoVkZ_NZO_Rf8QHkoZ6MMZVguB35cwSdnjito4yw_yjBBdylKJlNcBCemt463F_JO8pI6eum5Bkpxv_Z4cmp4HkYTnfEwrrOa5e8_ejJ7gnV0NFfdt3D0MILFxMv_FUKoPxP0d4XnTFDvsG1aSdOyZzEoR2DOAZAdfha2paDEfkKrOPKg1zDhC_G2ISDfGOwdR1PaSd7HRt50uF2w8eNXTAzfuuW_WieNdJSyl_xhJeDjC6vJ5BySnt4wulLBN7_5_kIx4v6LDOYX0_Rz8g2QFSWExB_hXeIwpKqoGz0fg5_QVlK5uBEMlJZ_a70OBWBngbqCIICzHOKvFqHdZ0hmKYfrUHKILQTKpzZZF7qv_iojA5vmPdtcuT7tMb5hlrjbNYavaAiWBVzoPwQqoD_VIkz3QKe7fCqmGmT2BTTcohmCjo5sLc8wTrBeZMjFIO97Rkup59hvT7TOzbTZrnuEFdXJRu4P8cgZV1rIE2yHut0vKqb3Iztzy17Fd5LUEy4io3AOUsQLV02ZfStMW7Q45N1sY6Z513hRwszdyTBpcK7sZXg4S3rFhrRbp3esAtwGwAwxCEM7My_Ngl94BvKDtZbJ6mV1J6JReFE6GMA95vV_6KKp9nJzrwTbb1GNpBG4Wms_tj1NGHXg7G4gtc4nMOSUg4YRM6sZP02kqBodwnAc_1d_aFzsCMlZWFCnqXHfJWXcocVAZojVauf4ijKUxijI0kTESwYEFQJ4mhP4nMxyLEA0gKFw1jB4aqDd8KXe5yBgh52805nEPo_zWVtoq4iaTLcMmeZoz02E39z167DCwMtfJ9rFyhpx16aD_DzAuQUvFYfOqJzwX4spro5weuirhtR3BTykYWqz5jtb35MhxFZh4kzqnmJ4DgG9LmmHIELsUwxRvI3NztfbxzStpseLgqKgR2bCemNZbprTVwVTlLhTIk2hjkVnRBo8HVyVUTwOCVpueh66On_caS1gDU4uWoNBibLmsoDNWXmEDKeSpYqFihhkKH7YOsn6KCyARmLaI3lPfDS5h39p_NHY32bNOuQJKjRa7RFyafiaJs8O1hHLgdaqbuKUlgwLz7iNJEVa70UrleUDmNTlOZUZ_DN4fqaHJ0ufBOuT2UD_A_uuv7UL200VaTE0jZwyEq0VfKxibWYARsSsqy3wMy5HFUUnXj48F52lKmHMqTic9cLC_1tWSuFCHcwQLxUEh_gb3dvTX60hc1lT4QtG44I3XLrkRcxFmsNaAwM_ikUtTWi53dfoT2iitpgldN3PUUdvtbjqk7Q1h9qzWpABMGHxQJCy2XoG5ZedzmtjvqfWTNsT3eGBpuIXXgTRh2ZflhMSCI5Do0tcojXBiSd0E3WTaQgcOlJFe16jCAJkNNxhhw_yPd9N4MbLgc4f9J3cTtE1JDrT2_oMrevHo4S0pDJ9TNlfjIFhO6QfRKD_OUjrnpplbZ9gkXeIfTCj_0i7NiTeFweOTM5lJf4BeiIJU3Uikq7lWUePPTHclRdvelIrI8d_XvVIu1eQmqKOyrA_KkAGfjsgPlXCh4UCSpJU_whLP7pe_nl3mJRJ9bNM3NQfXPFDL0n_vSZ_zmjIfZznBt26d5Lk3ieaZChJltuzrByz1fwyN62RFIE4Gx58DPL7M6cMebexl8Ul7PZK8phmksxnrC6DknlDWNA99Pno4z8HN5mpuZDufdvWzU_a1D3SHBhaZldRbdWFP7vqJXp4FQbp5MwYoTqv2j7Om_gHvkQYqgh7MCMdGSUwx4PRR1e2aKtPB_rcPGn_KB9acDJWszhuN_63Ccct8JXXKYJSgRI4xlk_3KOHjNpsrqrh17O0H_F_vq1Fe1_Nd8IfqaGIG25yz3qOY1tWck77au19PHaW_kk7ZTdNKRnVLej1H44uU4y1Dy6Y7RM7r7Av5ygjirAZqMKwQpu9pWWs_DzbpWFBHHpltaKzaQBrZk4a67ha84XhVm3GoXOTrX2MOdySXWC7BvUOrNyDk_elBgDT08eaBw1wOrVM9C3kEU7dCeUCSyUGLquztJpJa_WgxvvsxiSAi4slkhTx0OLGKXzqBW9reLOFaH3Q9NJyHerXLFx3jLylId60C7sVQz8hlSDL26uuPT_1Jw33_ZUgYbxuaxOvWIQypG6sFyiopCG2tYpGsEjBeJUU_gSr7gOqtrKwI8yclfj_z3lU4KNQlPnL6Z0a82umga8yPnsAOSwaGWAtzmFxCsDZFqaVi7aG5kig8LnDo4_sYXFoQI2ePd4a0vYlbTaqCSf9UkMKgJlQQP40lpyKU3NrNgOH58_ShBIGnfR83Awl1B0jO_RFg1r0BXiSnAX8vlz3QAJWnfcO139zt8q9PUouq8ljwcLg3u62KmIIVgnEsuEQw3NsdqYNblKYdSLFHL0g02stYLEtebglHxCKvhk_nyNgYF304ve7pixrc0VoYrwf33UKX_j4qrEwgcIwJ3TUm5XZI0IJ8qkXFZtbdxUcUnnih9PfZgbJlbv6K2XJSVe3fYtRoOZeqBZf0U2kqCfA2Vbk8FS8r1NlbSpEKkufN_Dej388eAqOTU6LC01m_aeaMohQw_oY963YfUh_aZRJE9aiJvtoGKNzwkzZsp_7cecoH3BrBtDJKd8t0DiYJMjsZzii9dZftS1_Mt_WqZ8ldadMHxODsiillNN_WBTjTplU4y6ZXL2zu_oRXVropW3NjnRriuFk7JQ4fPDjlXIx85zLwXLrVp6sVHmJVfx2iorYx6ubEAy0LrktbB4e5xn1jU99OJ5LxYNfubZmo737eqZcVhAF_HGKHG1dLCTJpJb8C2VUs6KjC_Dc_Ot8tf_La4ZnXoi9VPupvlCHBthWK6IuE4MeClU3zbD1Mfbmv2OQUs3lK1HAmLSJ5iqcUqxbe1pzqn3fvfdO_90zqRI_V3VG7gUluG25uw_R8tA7gi5At_cvnmwn_zmNeiUxdh_UBSXYyjfN7CjA78UCrfMIMKaQmhTcQEVVvlICzNukeMhgWJB4aef5iEqJ6IzSAPK9tZhG_HMQv1GpCaLqh_m78rY6dloFLMtsIH6XOOGWF4j_OKDFU_69PdU3j3BUS4uN_dGSPZHBQoRTGbA5zlZ_aZveTo012BII2PdyLKYeKCTp6ib0r4_UjR_suU4uiB8_9hBIMQRFThDIAswTcl4l70Wqz3pY57XL6_pi43TJV6QlG_i41DkRjy7cV5aZJd9ytfeBctJYrWbWpF30LNdNIGWN5gzFiMUZvw8TR0JjjUnrvnb0dt_m_t1xwn1O0R7TwKErOMUq8E6nBdPlbZQRR96QautoOy0sThSr9DRe_s_H3_wLLA_HmQF0AAA_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FNotificationPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1cW4_buBV_z68gplhkvLUCSb6OghQFCvSp_7J96SIIFrRE28zIokLRM55Z5L8XEimJpEjqYk_KBQpks7HE6_G5fOdCffh9ebgkceaDP94BkOAiT_FLBPYpunx8BwBM8SHzMEOnIgIxyhii5eOv54Lh_YsXk4yhjEWgyGGMvB1izwhlZYtyAO_ZwjwC5d_lo0P54wnSe8_7PXhAryF9DV9n5ZscJgnODhEIKDqB4EO4ouhUvtgRmiDq7Qhj5BSBIL_AgqQ40YZB1Sg7GD8eKDlniReTlFB9ss2sWVhxpDh7jID_8d33d58TyKBXHBFin_5yhB7vvoAPClma9S3zC9jml2Y3Pv_1_V3dPqja70nGvAK_okjeTPX0GeHDkUVg7fvlsxOkB5xVCxlOfT71h3pc42b9Wd_Wukutxmv3EqosgbMUZ8gbyxntqxPOvGecsGNJlXrxR0GP9klDbF_aY9_ZBr7OAvgEDygC5Zoh9Q4UJhhl7J4edvDenwPx50O4ms2B4eFMouzzETMksWM51rmIQBByVpBo6H_Y2I_7Je1ilLTZZKkmj_uctbnEwRT93f4Gfq66VpuOQCBvYKnKBV_JvPvugTEKsyKHFGWspWWvTKsUX3OCx2dalEPmBDc0ks9gu_mVDcDXgxkmWQRgmpbHXgAEC6TsMzqSJ0Sr3fbx4HYm91z9mSkUjCDRajKJ1tfZnPaV0CqhplPCsZTOSGaS85X_Uz9NA7c_DjoE1Vdlpu56NHX7hFqlvti5ZIy8FO1ZBOCZEXkhG24uUnSpVAEA5bL2KXn2YBYfy8XwHtKLF8OzSwSOOEk4SGilQ55pa2aLSgsmmKKY0y8m6fmUTWOYxur4_GibhdQsw9CFedW4cjfrybaLf9Ct6lKMKIjbQBnxmOQwxuyllMmFPA7sAonAjCRWGpJo7CY_VAc6qOfa6XPJ_kFBKAMIEI87vbZjoirM66Agb8w5eaE1zSkmFLMXLkSDoON6JjOxzMISqPGnSXgyWX8iS58azgRBOAfhclFCGn_jdN1z4W_ZbyO_PdzYGdB4v8Ut9YTHURNq_He86TcAKkmxqUcqL7JglAjiNAqND7OjCD56z4QmlT0iNPGqR_obPo0PgkYz2mbDZnhucmDqLl_7crx2eh_Vlqs4dk_oKQLnPEc0rvgTgBQxhqhXHqZAMAuOLVplmV9Kj8hgMUXLDn4eDc_XBnS_5OC8GrVaHiott3yQZvI8dsnTg4JmDuY1aeF2rvTGc40UjebUJ7Fpu43TdLlsB8l0JLzYCCw8GAobSXaSVL3dIJyuQMlmX04YXAc4tjCnRJIROv80s1KAKjqdGP31XUriRwkki_BFjYDEz2GLMOHidv78VnBeUolLoV_KR_WS62c_Bta7hDbs85JkE_tWTjUKUInZ9Fac73wkYhgEAr9Vw6koctPCSIuRrVisjeEsB05GbwwwdESg2_d25mKsYg6VhTOrLnP49aPkuZdtb627hqFUNoLnMoVkZ_NZO_Rf8QHkoZ6MMZVguB35cwSdnjito4yw_yjBBdylKJlNcBCemt463F_JO8pI6eum5Bkpxv_Z4cmp4HkYTnfEwrrOa5e8_ejJ7gnV0NFfdt3D0MILFxMv_FUKoPxP0d4XnTFDvsG1aSdOyZzEoR2DOAZAdfha2paDEfkKrOPKg1zDhC_G2ISDfGOwdR1PaSd7HRt50uF2w8eNXTAzfuuW_WieNdJSyl_xhJeDjC6vJ5BySnt4wulLBN7_5_kIx4v6LDOYX0_Rz8g2QFSWExB_hXeIwpKqoGz0fg5_QVlK5uBEMlJZ_a70OBWBngbqCIICzHOKvFqHdZ0hmKYfrUHKILQTKpzZZF7qv_iojA5vmPdtcuT7tMb5hlrjbNYavaAiWBVzoPwQqoD_VIkz3QKe7fCqmGmT2BTTcohmCjo5sLc8wTrBeZMjFIO97Rkup59hvT7TOzbTZrnuEFdXJRu4P8cgZV1rIE2yHut0vKqb3Iztzy17Fd5LUEy4io3AOUsQLV02ZfStMW7Q45N1sY6Z513hRwszdyTBpcK7sZXg4S3rFhrRbp3esAtwGwAwxCEM7My_Ngl94BvKDtZbJ6mV1J6JReFE6GMA95vV_6KKp9nJzrwTbb1GNpBG4Wms_tj1NGHXg7G4gtc4nMOSUg4YRM6sZP02kqBodwnAc_1d_aFzsCMlZWFCnqXHfJWXcocVAZojVauf4ijKUxijI0kTESwYEFQJ4mhP4nMxyLEA0gKFw1jB4aqDd8KXe5yBgh52805nEPo_zWVtoq4iaTLcMmeZoz02E39z167DCwMtfJ9rFyhpx16aD_DzAuQUvFYfOqJzwX4spro5weuirhtR3BTykYWqz5jtb35MhxFZh4kzqnmJ4DgG9LmmHIELsUwxRvI3NztfbxzStpseLgqKgR2bCemNZbprTVwVTlLhTIk2hjkVnRBo8HVyVUTwOCVpueh66On_caS1gDU4uWoNBibLmsoDNWXmEDKeSpYqFihhkKH7YOsn6KCyARmLaI3lPfDS5h39p_NHY32bNOuQJKjRa7RFyafiaJs8O1hHLgdaqbuKUlgwLz7iNJEVa70UrleUDmNTlOZUZ_DN4fqaHJ0ufBOuT2UD_A_uuv7UL200VaTE0jZwyEq0VfKxibWYARsSsqy3wMy5HFUUnXj48F52lKmHMqTic9cLC_1tWSuFCHcwQLxUEh_gb3dvTX60hc1lT4QtG44I3XLrkRcxFmsNaAwM_ikUtTWi53dfoT2iitpgldN3PUUdvtbjqk7Q1h9qzWpABMGHxQJCy2XoG5ZedzmtjvqfWTNsT3eGBpuIXXgTRh2ZflhMSCI5Do0tcojXBiSd0E3WTaQgcOlJFe16jCAJkNNxhhw_yPd9N4MbLgc4f9J3cTtE1JDrT2_oMrevHo4S0pDJ9TNlfjIFhO6QfRKD_OUjrnpplbZ9gkXeIfTCj_0i7NiTeFweOTM5lJf4BeiIJU3Uikq7lWUePPTHclRdvelIrI8d_XvVIu1eQmqKOyrA_KkAGfjsgPlXCh4UCSpJU_whLP7pe_nl3mJRJ9bNM3NQfXPFDL0n_vSZ_zmjIfZznBt26d5Lk3ieaZChJltuzrByz1fwyN62RFIE4Gx58DPL7M6cMebexl8Ul7PZK8phmksxnrC6DknlDWNA99Pno4z8HN5mpuZDufdvWzU_a1D3SHBhaZldRbdWFP7vqJXp4FQbp5MwYoTqv2j7Om_gHvkQYqgh7MCMdGSUwx4PRR1e2aKtPB_rcPGn_KB9acDJWszhuN_63Ccct8JXXKYJSgRI4xlk_3KOHjNpsrqrh17O0H_F_vq1Fe1_Nd8IfqaGIG25yz3qOY1tWck77au19PHaW_kk7ZTdNKRnVLej1H44uU4y1Dy6Y7RM7r7Av5ygjirAZqMKwQpu9pWWs_DzbpWFBHHpltaKzaQBrZk4a67ha84XhVm3GoXOTrX2MOdySXWC7BvUOrNyDk_elBgDT08eaBw1wOrVM9C3kEU7dCeUCSyUGLquztJpJa_WgxvvsxiSAi4slkhTx0OLGKXzqBW9reLOFaH3Q9NJyHerXLFx3jLylId60C7sVQz8hlSDL26uuPT_1Jw33_ZUgYbxuaxOvWIQypG6sFyiopCG2tYpGsEjBeJUU_gSr7gOqtrKwI8yclfj_z3lU4KNQlPnL6Z0a82umga8yPnsAOSwaGWAtzmFxCsDZFqaVi7aG5kig8LnDo4_sYXFoQI2ePd4a0vYlbTaqCSf9UkMKgJlQQP40lpyKU3NrNgOH58_ShBIGnfR83Awl1B0jO_RFg1r0BXiSnAX8vlz3QAJWnfcO139zt8q9PUouq8ljwcLg3u62KmIIVgnEsuEQw3NsdqYNblKYdSLFHL0g02stYLEtebglHxCKvhk_nyNgYF304ve7pixrc0VoYrwf33UKX_j4qrEwgcIwJ3TUm5XZI0IJ8qkXFZtbdxUcUnnih9PfZgbJlbv6K2XJSVe3fYtRoOZeqBZf0U2kqCfA2Vbk8FS8r1NlbSpEKkufN_Dej388eAqOTU6LC01m_aeaMohQw_oY963YfUh_aZRJE9aiJvtoGKNzwkzZsp_7cecoH3BrBtDJKd8t0DiYJMjsZzii9dZftS1_Mt_WqZ8ldadMHxODsiillNN_WBTjTplU4y6ZXL2zu_oRXVropW3NjnRriuFk7JQ4fPDjlXIx85zLwXLrVp6sVHmJVfx2iorYx6ubEAy0LrktbB4e5xn1jU99OJ5LxYNfubZmo737eqZcVhAF_HGKHG1dLCTJpJb8C2VUs6KjC_Dc_Ot8tf_La4ZnXoi9VPupvlCHBthWK6IuE4MeClU3zbD1Mfbmv2OQUs3lK1HAmLSJ5iqcUqxbe1pzqn3fvfdO_90zqRI_V3VG7gUluG25uw_R8tA7gi5At_cvnmwn_zmNeiUxdh_UBSXYyjfN7CjA78UCrfMIMKaQmhTcQEVVvlICzNukeMhgWJB4aef5iEqJ6IzSAPK9tZhG_HMQv1GpCaLqh_m78rY6dloFLMtsIH6XOOGWF4j_OKDFU_69PdU3j3BUS4uN_dGSPZHBQoRTGbA5zlZ_aZveTo012BII2PdyLKYeKCTp6ib0r4_UjR_suU4uiB8_9hBIMQRFThDIAswTcl4l70Wqz3pY57XL6_pi43TJV6QlG_i41DkRjy7cV5aZJd9ytfeBctJYrWbWpF30LNdNIGWN5gzFiMUZvw8TR0JjjUnrvnb0dt_m_t1xwn1O0R7TwKErOMUq8E6nBdPlbZQRR96QautoOy0sThSr9DRe_s_H3_wLLA_HmQF0AAA_22_7D__WEBPACK_IMPORTED_MODULE_1__);


var approvalCommand = '_4gxdcn11';
var approvalCwd = '_4gxdcn12';
var approvalDetails = '_4gxdcnz';
var approvalTool = '_4gxdcn10';
var approveButton = '_4gxdcn13';
var autoHandledBadge = '_4gxdcn1l';
var autoHandledChevron = '_4gxdcn1m';
var autoHandledChevronOpen = '_4gxdcn1n';
var autoHandledContent = '_4gxdcn1r';
var autoHandledDecision = '_4gxdcn1q';
var autoHandledHeader = '_4gxdcn1j';
var autoHandledHeaderLeft = '_4gxdcn1k';
var autoHandledItem = '_4gxdcn1p';
var autoHandledList = '_4gxdcn1o';
var autoHandledMeta = '_4gxdcn1t';
var autoHandledSection = '_4gxdcn1i';
var autoHandledTimestamp = '_4gxdcn1u';
var autoHandledTitle = '_4gxdcn1s';
var ciBlockedLink = '_4gxdcn17';
var ciBlockedRow = '_4gxdcn15';
var ciBlockedText = '_4gxdcn16';
var clearButton = '_4gxdcn5';
var closeButton = '_4gxdcn6';
var content = '_4gxdcn7';
var countBadge = '_4gxdcn19';
var denyButton = '_4gxdcn14';
var empty = '_4gxdcn8';
var emptyIcon = '_4gxdcn9';
var emptySubtext = '_4gxdcnb';
var emptyText = '_4gxdcna';
var filterBar = '_4gxdcn1a';
var filterPill = '_4gxdcn1f';
var filterPillActive = '_4gxdcn1g';
var filterPillExcludeActive = '_4gxdcn1h';
var filterPills = '_4gxdcn1e';
var focusButton = '_4gxdcnn';
var groupHeader = '_4gxdcn2s';
var groupHeaderPinned = '_4gxdcn2t';
var groupNote = '_4gxdcn2v';
var groupToggle = '_4gxdcn2u';
var header = '_4gxdcn0';
var headerActions = '_4gxdcn3';
var incompleteSearchNotice = '_4gxdcnw';
var incompleteSearchNoticeButton = '_4gxdcnx';
var item = '_4gxdcnd';
var itemActions = '_4gxdcnm';
var itemContext = '_4gxdcnk';
var itemFooter = '_4gxdcnr';
var itemHeader = '_4gxdcng';
var itemMessage = '_4gxdcnq';
var itemSubtitle = '_4gxdcny';
var itemTitle = '_4gxdcnh';
var itemWorkingDir = '_4gxdcnl';
var list = '_4gxdcnc';
var loadMore = '_4gxdcnu';
var loadMoreButton = '_4gxdcnv';
var markAllButton = '_4gxdcn4';
var needsDecisionClearFilterButton = '_4gxdcn24';
var needsDecisionEmpty = '_4gxdcn20';
var needsDecisionEmptyIcon = '_4gxdcn21';
var needsDecisionEmptySubtext = '_4gxdcn23';
var needsDecisionEmptyText = '_4gxdcn22';
var needsDecisionHeading = '_4gxdcn1x';
var needsDecisionHeadingRow = '_4gxdcn1w';
var needsDecisionSection = '_4gxdcn1v';
var newPill = '_4gxdcn2z';
var read = '_4gxdcnf';
var removeButton = '_4gxdcnp';
var resolvedBadge = '_4gxdcn18';
var searchClearButton = '_4gxdcn1d';
var searchInput = '_4gxdcn1c';
var searchRow = '_4gxdcn1b';
var sheetGrabber = '_4gxdcn2b';
var sheetGrabberRow = '_4gxdcn2a';
var sheetScrim = '_4gxdcn29';
var stalenessIndicator = '_4gxdcn1y';
var stalenessRetry = '_4gxdcn1z';
var statusSlot = '_4gxdcn2y';
var swipeRow = '_4gxdcn2w';
var timestamp = '_4gxdcns';
var title = '_4gxdcn1';
var trayAttention = '_4gxdcn2d';
var trayBanner = '_4gxdcn2f';
var trayButton = '_4gxdcn2c';
var trayConfirm = '_4gxdcn2l';
var trayConfirmActions = '_4gxdcn2m';
var trayCounts = '_4gxdcn2e';
var trayErrorLink = '_4gxdcn32';
var trayFooter = '_4gxdcn2x';
var trayHandle = '_4gxdcn30';
var trayHandleDot = '_4gxdcn33';
var trayMenu = '_4gxdcn2h';
var trayMenuCaption = '_4gxdcn2j';
var trayMenuDivider = '_4gxdcn2k';
var trayMenuItem = '_4gxdcn2i';
var trayQuietBadge = '_4gxdcn31';
var traySelect = '_4gxdcn2p';
var traySettings = '_4gxdcn2n';
var traySettingsRow = '_4gxdcn2o';
var trayUndoBar = '_4gxdcn2g';
var trayVariant = {sideOverlay:'_4gxdcn26 _4gxdcn25',bottomSheet:'_4gxdcn27 _4gxdcn25',landscapePanel:'_4gxdcn28 _4gxdcn25'};
var typeIcon = '_4gxdcni';
var typeLabel = '_4gxdcnj';
var unread = '_4gxdcne';
var unreadBadge = '_4gxdcn2';
var unreadDot = '_4gxdcno';
var viewButton = '_4gxdcnt';
var virtualRow = '_4gxdcn2r';
var virtualViewport = '_4gxdcn2q';

/***/ }),

/***/ 82514:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   E$: () => (/* binding */ useNotifications),
/* harmony export */   IB: () => (/* binding */ useNotificationState),
/* harmony export */   RK: () => (/* binding */ useNotificationCommands),
/* harmony export */   _J: () => (/* binding */ NotificationStateContext),
/* harmony export */   jo: () => (/* binding */ NotificationCommandsContext)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ NotificationStateContext,NotificationCommandsContext,useNotificationState,useNotificationCommands,useNotifications auto */ 
const NotificationStateContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.createContext)(null);
const NotificationCommandsContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.createContext)(null);
const noop = ()=>{};
const OUTSIDE_PROVIDER_STATE = {
    notifications: [],
    notificationHistory: [],
    isPanelOpen: false,
    historyLoading: false,
    historyHasMore: false,
    historyError: null,
    historyLastUpdatedAt: null,
    unreadCount: 0,
    movedToTray: null,
    answeredElsewhere: {},
    quietMode: false
};
/**
 * Outside a NotificationProvider these return no-ops, so components render in
 * tests or embedded views without a full provider tree.
 */ const OUTSIDE_PROVIDER_COMMANDS = {
    addNotification: noop,
    addToHistoryOnly: noop,
    removeNotification: noop,
    removeToastByApprovalId: noop,
    acknowledgeNotification: noop,
    broadcastActionResolved: noop,
    markAnswerHandled: noop,
    markBackgroundReviewed: noop,
    clearAll: noop,
    dismissToast: noop,
    moveAllToTray: ()=>[],
    undoMoveToTray: noop,
    showSessionNotification: noop,
    togglePanel: noop,
    markAsRead: noop,
    showActionToast: ()=>""
};
/** Toast and history state, without the commands. */ function useNotificationState() {
    var _useContext;
    return (_useContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.useContext)(NotificationStateContext)) !== null && _useContext !== void 0 ? _useContext : OUTSIDE_PROVIDER_STATE;
}
/** Stable commands only; consumers using just these never re-render on notification state. */ function useNotificationCommands() {
    var _useContext;
    return (_useContext = (0,react__WEBPACK_IMPORTED_MODULE_0__.useContext)(NotificationCommandsContext)) !== null && _useContext !== void 0 ? _useContext : OUTSIDE_PROVIDER_COMMANDS;
}
/** Both halves merged: the original `useNotifications()` surface. */ function useNotifications() {
    const state = useNotificationState();
    const commands = useNotificationCommands();
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>({
            ...state,
            ...commands,
            getUnreadCount: ()=>state.unreadCount
        }), [
        state,
        commands
    ]);
}


/***/ }),

/***/ 83501:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B_: () => (/* binding */ trayAffordanceVariant),
/* harmony export */   ix: () => (/* binding */ selectTrayVariant)
/* harmony export */ });
/* unused harmony exports SHEET_PEEK_FRACTION, SHEET_EXPANDED_FRACTION */
/**
 * Pure rules for where the notification tray sits and which single entry point
 * is shown. Kept free of React and the DOM so they are table-testable.
 */ /**
 * Desktop gets a right-edge overlay; a phone on its side a right panel; every other
 * phone the bottom sheet. The soft keyboard does not change the variant: the sheet
 * stays put and rides the keyboard's top edge (CSS, `data-keyboard`), so opening the
 * keyboard never teleports it to another edge.
 */ function selectTrayVariant(viewport) {
    if (viewport.isInnerScreen) return "side-overlay";
    if (viewport.isLandscape) return "landscape-panel";
    return "bottom-sheet";
}
/** Content of the phone tray entry: overflow, the keyboard chip, or nothing. Never a bell. */ function trayAffordanceVariant(overflowCount, toastCount, keyboardOpen, hasTerminal) {
    if (!hasTerminal) return "floating-bottom";
    if (keyboardOpen && toastCount > 0) return "keyboard-chip";
    if (overflowCount > 0) return "more-row";
    return "none";
}
/** Peek and expanded sheet heights as a fraction of `--viewport-height` (TS-1: 22-28% and 84-86%). */ const SHEET_PEEK_FRACTION = 0.25;
const SHEET_EXPANDED_FRACTION = 0.85;


/***/ }),

/***/ 85771:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  HR: () => (/* binding */ TrayEntryChip),
  hx: () => (/* binding */ TrayHandle)
});

// UNUSED EXPORTS: keepTerminalFocus, trayEntryLabel, useTrayCounts

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/useAnnounce.ts
var useAnnounce = __webpack_require__(5647);
// EXTERNAL MODULE: ./src/lib/hooks/useHandedness.ts
var useHandedness = __webpack_require__(39823);
// EXTERNAL MODULE: ./src/lib/utils/deckSettings.ts
var deckSettings = __webpack_require__(15254);
// EXTERNAL MODULE: ./src/lib/utils/bellPosition.ts
var bellPosition = __webpack_require__(41364);
;// ./src/lib/hooks/useDraggableBell.ts
/* __next_internal_client_entry_do_not_use__ useDraggableBell,resetBellPos auto */ 




const FALLBACK_HANDLE_HEIGHT = 88;
/** Special-keys bar sits above the bottom nav on a phone terminal. */ const PHONE_KEYS_BAR_RESERVE = 48;
function cssPx(name) {
    if (typeof document === "undefined") return 0;
    const n = parseFloat(getComputedStyle(document.documentElement).getPropertyValue(name));
    return Number.isFinite(n) ? n : 0;
}
/** env() cannot be read from JS directly; a probe element resolves it. */ function safeInset(side) {
    if (typeof document === "undefined") return 0;
    const probe = document.createElement("div");
    probe.style.cssText = "position:fixed;visibility:hidden;padding-".concat(side, ":env(safe-area-inset-").concat(side, ", 0px)");
    document.body.appendChild(probe);
    const n = parseFloat(getComputedStyle(probe).getPropertyValue("padding-".concat(side)));
    probe.remove();
    return Number.isFinite(n) ? n : 0;
}
function measureSafeInsets() {
    return {
        top: safeInset("top"),
        bottom: safeInset("bottom")
    };
}
function measureBounds(phone, handleHeight, insets) {
    var _window_visualViewport;
    // Reached during render, so it must survive the server prerender (no window).
    const viewportHeight = cssPx("--viewport-height") || ( false ? 0 : ((_window_visualViewport = window.visualViewport) === null || _window_visualViewport === void 0 ? void 0 : _window_visualViewport.height) || window.innerHeight);
    return (0,bellPosition/* computeBounds */.bW)({
        viewportHeight,
        safeTop: insets.top,
        safeBottom: insets.bottom,
        bottomInset: phone ? cssPx("--bottom-nav-height") + PHONE_KEYS_BAR_RESERVE : 0,
        handleHeight
    });
}
/**
 * Click-and-drag repositioning for the tray handle. Plain pointer events: a press
 * only becomes a drag past a small threshold, so a tap still toggles the tray.
 * Desktop is right-edge, vertical only; a phone (opt-in) snaps to the thumb side.
 */ function useDraggableBell(ref, phone) {
    const { announce } = (0,useAnnounce/* useAnnounce */.XE)();
    const { leftHanded } = (0,useHandedness/* useHandedness */.V)();
    const [stored, setStored] = (0,react.useState)(()=> false ? 0 : (0,deckSettings/* readBellPos */.Y_)());
    const [floatingBellEnabled, setFloatingBellEnabled] = (0,react.useState)(()=> true && (0,deckSettings/* readFloatingBell */.Gf)());
    const [dragTop, setDragTop] = (0,react.useState)(null);
    const [, setLayoutTick] = (0,react.useState)(0);
    const drag = (0,react.useRef)(null);
    const suppressClick = (0,react.useRef)(false);
    // Probed in effects only: render must not touch the DOM.
    const insets = (0,react.useRef)({
        top: 0,
        bottom: 0
    });
    (0,react.useEffect)(()=>{
        var _window_visualViewport;
        const reread = ()=>{
            setStored((0,deckSettings/* readBellPos */.Y_)());
            setFloatingBellEnabled((0,deckSettings/* readFloatingBell */.Gf)());
        };
        const relayout = ()=>{
            insets.current = measureSafeInsets();
            setLayoutTick((n)=>n + 1);
        };
        relayout();
        window.addEventListener(deckSettings/* BELL_SETTINGS_EVENT */.h5, reread);
        window.addEventListener("resize", relayout);
        window.addEventListener("orientationchange", relayout);
        (_window_visualViewport = window.visualViewport) === null || _window_visualViewport === void 0 ? void 0 : _window_visualViewport.addEventListener("resize", relayout);
        return ()=>{
            var _window_visualViewport;
            window.removeEventListener(deckSettings/* BELL_SETTINGS_EVENT */.h5, reread);
            window.removeEventListener("resize", relayout);
            window.removeEventListener("orientationchange", relayout);
            (_window_visualViewport = window.visualViewport) === null || _window_visualViewport === void 0 ? void 0 : _window_visualViewport.removeEventListener("resize", relayout);
        };
    }, []);
    const handleHeight = ()=>{
        var _ref_current;
        return ((_ref_current = ref.current) === null || _ref_current === void 0 ? void 0 : _ref_current.offsetHeight) || FALLBACK_HANDLE_HEIGHT;
    };
    var _stored_edge;
    const edge = phone ? (_stored_edge = stored === null || stored === void 0 ? void 0 : stored.edge) !== null && _stored_edge !== void 0 ? _stored_edge : leftHanded ? "left" : "right" : "right";
    const currentY = (0,react.useCallback)((b)=>{
        var _ref_current;
        if (stored) return stored.y;
        if (phone) return bellPosition/* PHONE_DEFAULT_Y */.$Z;
        var _ref_current_getBoundingClientRect_top;
        const top = (_ref_current_getBoundingClientRect_top = (_ref_current = ref.current) === null || _ref_current === void 0 ? void 0 : _ref_current.getBoundingClientRect().top) !== null && _ref_current_getBoundingClientRect_top !== void 0 ? _ref_current_getBoundingClientRect_top : window.innerHeight * bellPosition/* DESKTOP_DEFAULT_Y_FRACTION_OF_VIEWPORT */.$k;
        return (0,bellPosition/* topToY */.qY)(top, b);
    }, [
        stored,
        phone,
        ref
    ]);
    const commit = (y, announceMove)=>{
        const pos = {
            v: 1,
            edge,
            y
        };
        setStored(pos);
        (0,deckSettings/* writeBellPos */.FU)(pos);
        if (announceMove) announce("Notification button moved to ".concat((0,bellPosition/* describeY */.PG)(y)), "polite", "bell-moved");
    };
    const onPointerDown = (e)=>{
        if (e.button !== undefined && e.button !== 0) return;
        const rect = e.currentTarget.getBoundingClientRect();
        drag.current = {
            pointerId: e.pointerId,
            startX: e.clientX,
            startY: e.clientY,
            startTop: rect.top,
            pointerType: e.pointerType || "mouse",
            active: false
        };
        try {
            var _e_currentTarget_setPointerCapture, _e_currentTarget;
            (_e_currentTarget_setPointerCapture = (_e_currentTarget = e.currentTarget).setPointerCapture) === null || _e_currentTarget_setPointerCapture === void 0 ? void 0 : _e_currentTarget_setPointerCapture.call(_e_currentTarget, e.pointerId);
        } catch (e) {
        // Capture is best effort; move/up still arrive while the pointer stays over the handle.
        }
    };
    const onPointerMove = (e)=>{
        const d = drag.current;
        if (!d || d.pointerId !== e.pointerId) return;
        const dx = e.clientX - d.startX;
        const dy = e.clientY - d.startY;
        if (!d.active) {
            if (!(0,bellPosition/* movedPastThreshold */.p1)(dx, dy, d.pointerType)) return;
            d.active = true;
        }
        setDragTop((0,bellPosition/* clampTop */.Vf)(d.startTop + dy, measureBounds(phone, handleHeight(), insets.current)));
    };
    const finish = (e)=>{
        const d = drag.current;
        if (!d || d.pointerId !== e.pointerId) return;
        drag.current = null;
        try {
            var _e_currentTarget_releasePointerCapture, _e_currentTarget;
            (_e_currentTarget_releasePointerCapture = (_e_currentTarget = e.currentTarget).releasePointerCapture) === null || _e_currentTarget_releasePointerCapture === void 0 ? void 0 : _e_currentTarget_releasePointerCapture.call(_e_currentTarget, e.pointerId);
        } catch (e) {
        // Already released.
        }
        if (!d.active) return;
        const b = measureBounds(phone, handleHeight(), insets.current);
        const top = (0,bellPosition/* clampTop */.Vf)(d.startTop + (e.clientY - d.startY), b);
        suppressClick.current = true;
        setTimeout(()=>suppressClick.current = false, 0);
        setDragTop(null);
        commit((0,bellPosition/* topToY */.qY)(top, b), false);
    };
    const onKeyDown = (e)=>{
        if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
        e.preventDefault();
        const b = measureBounds(phone, handleHeight(), insets.current);
        commit((0,bellPosition/* stepY */.Qc)(currentY(b), e.key === "ArrowUp" ? "up" : "down", e.shiftKey), true);
    };
    const consumeClickSuppression = ()=>{
        const was = suppressClick.current;
        suppressClick.current = false;
        return was;
    };
    const style = {};
    if (dragTop !== null || stored || phone) {
        const b = measureBounds(phone, handleHeight(), insets.current);
        style.top = dragTop !== null && dragTop !== void 0 ? dragTop : (0,bellPosition/* yToTop */.aY)(stored ? stored.y : bellPosition/* PHONE_DEFAULT_Y */.$Z, b);
        if (dragTop !== null) style.transition = "none";
    }
    if (phone && edge === "left") {
        Object.assign(style, {
            left: 0,
            right: "auto",
            borderLeft: "none",
            borderRight: "1px solid var(--border-color, currentColor)",
            borderRadius: "0 8px 8px 0"
        });
    }
    return {
        style,
        edge,
        dragging: dragTop !== null,
        handlers: {
            onPointerDown,
            onPointerMove,
            onPointerUp: finish,
            onPointerCancel: finish,
            onKeyDown
        },
        consumeClickSuppression,
        floatingBellEnabled
    };
}


// EXTERNAL MODULE: ./src/lib/contexts/deckViewportContext.ts
var deckViewportContext = __webpack_require__(44865);
// EXTERNAL MODULE: ./src/lib/contexts/notificationContexts.ts
var notificationContexts = __webpack_require__(82514);
// EXTERNAL MODULE: ./src/lib/utils/notificationGrouping.ts
var notificationGrouping = __webpack_require__(65232);
// EXTERNAL MODULE: ./src/lib/utils/notificationMapping.ts
var notificationMapping = __webpack_require__(71249);
// EXTERNAL MODULE: ./src/components/ui/NotificationPanel.css.ts
var NotificationPanel_css = __webpack_require__(82254);
// EXTERNAL MODULE: ./src/components/ui/NotificationToast.css.ts
var NotificationToast_css = __webpack_require__(18311);
;// ./src/components/ui/TrayHandle.tsx
// +feature: notification-tray
/* __next_internal_client_entry_do_not_use__ keepTerminalFocus,trayEntryLabel,useTrayCounts,TrayHandle,TrayEntryChip auto */ 








/** A mousedown on a button would blur xterm's textarea (and collapse a soft keyboard). */ const keepTerminalFocus = {
    onMouseDown: (e)=>e.preventDefault()
};
const TRAY_ID = "notification-tray";
function trayEntryLabel(unread, needsAttention) {
    let quiet = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false;
    const decisions = needsAttention > 0 ? ", ".concat(needsAttention, " need attention") : "";
    return "Notifications, ".concat(unread, " unread").concat(decisions).concat(quiet ? ", quiet mode on" : "");
}
function BellIcon() {
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("svg", {
        "aria-hidden": "true",
        xmlns: "http://www.w3.org/2000/svg",
        width: "18",
        height: "18",
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        strokeWidth: "2",
        strokeLinecap: "round",
        strokeLinejoin: "round",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                d: "M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                d: "M13.73 21a2 2 0 0 1-3.46 0"
            })
        ]
    });
}
/** Unread count and the "needs attention" count, derived once for every entry point. */ function useTrayCounts() {
    const { notificationHistory, unreadCount } = (0,notificationContexts/* useNotificationState */.IB)();
    const needsAttention = (0,react.useMemo)(()=>(0,notificationGrouping/* countNeedsAttention */.$F)(notificationHistory), [
        notificationHistory
    ]);
    return {
        unreadCount,
        needsAttention
    };
}
/**
 * The desktop edge handle: a fixed 44px-wide band at 35% from the top (O6),
 * capped "99+" visually with the exact count in its accessible name. It overlays
 * the terminal; nothing reflows. Draggable (see useDraggableBell). On a phone it renders only when the opt-in floating bell is on.
 */ function TrayHandle() {
    const { isInnerScreen } = (0,deckViewportContext/* useDeckViewport */.T)();
    const { isPanelOpen, quietMode } = (0,notificationContexts/* useNotificationState */.IB)();
    const { togglePanel } = (0,notificationContexts/* useNotificationCommands */.RK)();
    const { unreadCount, needsAttention } = useTrayCounts();
    const { isVirtualKeyboardOpen } = (0,deckViewportContext/* useDeckViewport */.T)();
    const ref = (0,react.useRef)(null);
    const bell = useDraggableBell(ref, !isInnerScreen);
    // The phone's standing entry is the bottom nav; the floating bell is opt-in and yields to the keyboard.
    if (!isInnerScreen && (!bell.floatingBellEnabled || isVirtualKeyboardOpen)) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
        ref: ref,
        type: "button",
        className: NotificationPanel_css/* trayHandle */.TE,
        "data-testid": "tray-handle",
        "data-open": isPanelOpen,
        "aria-expanded": isPanelOpen,
        "aria-controls": TRAY_ID,
        "aria-keyshortcuts": "ArrowUp ArrowDown",
        "aria-label": trayEntryLabel(unreadCount, needsAttention, quietMode),
        "data-gesture-ignore": true,
        "data-dragging": bell.dragging,
        style: {
            touchAction: "none",
            userSelect: "none",
            WebkitTouchCallout: "none",
            ...bell.style
        },
        onClick: ()=>{
            if (bell.consumeClickSuppression()) return;
            togglePanel();
        },
        ...bell.handlers,
        ...keepTerminalFocus,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(BellIcon, {}),
            quietMode && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: NotificationPanel_css/* trayQuietBadge */.cK,
                "data-testid": "tray-handle-quiet",
                "aria-hidden": "true",
                children: "Quiet"
            }),
            unreadCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                "data-testid": "tray-handle-count",
                children: (0,notificationMapping/* capBadgeCount */.BK)(unreadCount)
            }),
            needsAttention > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: NotificationPanel_css/* trayHandleDot */.O3,
                "data-testid": "tray-handle-dot",
                "aria-hidden": "true"
            })
        ]
    });
}
/**
 * The phone deck's tray entry: the "+N more" row, the keyboard chip, or the undo bar.
 * There is no bell here; the bottom nav's Alerts tab is the standing entry to the tray.
 * Renders nothing when there is nothing to say. Rendered by the deck's dock, never by a page.
 */ function TrayEntryChip(param) {
    let { content, chipText, chipLabel, undo, onOpen } = param;
    const { isPanelOpen } = (0,notificationContexts/* useNotificationState */.IB)();
    const wide = content === "more-row" || content === "keyboard-chip";
    if (!undo && !wide) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: NotificationToast_css/* trayEntry */.rO,
        "data-testid": "tray-entry",
        "data-content": content,
        "data-undo": undo ? "true" : "false",
        children: undo !== null && undo !== void 0 ? undo : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
            type: "button",
            className: NotificationToast_css/* overflowChip */.tG,
            "data-testid": "toast-overflow-chip",
            "aria-expanded": isPanelOpen,
            "aria-controls": TRAY_ID,
            "aria-label": chipLabel,
            onClick: onOpen,
            ...keepTerminalFocus,
            children: chipText
        })
    });
}


/***/ }),

/***/ 86130:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   GR: () => (/* binding */ subscribeStackTopOffset),
/* harmony export */   Gp: () => (/* binding */ setDeckTopOffset),
/* harmony export */   nu: () => (/* binding */ setStackTopOffset),
/* harmony export */   vI: () => (/* binding */ getStackTopOffset)
/* harmony export */ });
/**
 * The y coordinate (px) of the bottom edge of the session tab row, or null when
 * no session page is mounted. The session layout publishes it; the mobile toast
 * deck docks directly below it (ADR-009) and falls back to the bottom dock
 * when it is null. Mirrored to `--mobile-stack-top-offset` for CSS.
 */ let topOffsetPx = null;
const listeners = new Set();
function setStackTopOffset(px) {
    if (px === topOffsetPx) return;
    topOffsetPx = px;
    if (typeof document !== "undefined") {
        document.documentElement.style.setProperty("--mobile-stack-top-offset", "".concat(px !== null && px !== void 0 ? px : 0, "px"));
    }
    listeners.forEach((l)=>l());
}
function getStackTopOffset() {
    return topOffsetPx;
}
function subscribeStackTopOffset(listener) {
    listeners.add(listener);
    return ()=>listeners.delete(listener);
}
/**
 * Bottom edge (px) of the terminal's own toolbar, or null when none is visible.
 * The phone deck docks here, over the terminal output, so it never covers the
 * toolbar controls. Only the CSS variable is needed: the deck's presence is
 * still decided by `getStackTopOffset`.
 *
 * Several terminals can be mounted at once, so the last non-null publisher owns
 * the value and only the owner may clear it: a hidden instance unmounting or
 * measuring 0 must not blank the visible one's anchor.
 */ let deckOwner = null;
function setDeckTopOffset(px, owner) {
    if (typeof document === "undefined") return;
    if (px === null) {
        if (owner !== undefined && owner !== deckOwner) return;
        deckOwner = null;
        document.documentElement.style.removeProperty("--mobile-deck-top-offset");
        return;
    }
    if (owner !== undefined) deckOwner = owner;
    document.documentElement.style.setProperty("--mobile-deck-top-offset", "".concat(px, "px"));
}


/***/ })

}]);
//# sourceMappingURL=2275-ca08904c5c9ae67c.js.map