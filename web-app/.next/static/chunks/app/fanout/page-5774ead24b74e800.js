(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[1764],{

/***/ 2663:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ useAbortableRequest)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ useAbortableRequest auto */ 
/**
 * Returns a `start()` function that hands back an AbortSignal for the next
 * request, automatically aborting whatever request this hook last started
 * (on the next call) and on unmount.
 *
 * A hook/component tied to a fast-changing key (session id, file path, ...)
 * that fires a request per render/effect without cancellation leaves every
 * prior request's promise — and everything it closes over — alive until it
 * resolves or times out. That's what happened in useSessionVcs: switching
 * sessions rapidly with no cancellation took the tab from ~44MB to ~154MB of
 * JS heap over 138 switches (measured 2026-09-04), each abandoned request
 * living until its own `deadline_exceeded`.
 *
 * Usage — one instance per independent request stream (a hook fetching two
 * unrelated things, like status and diff, uses two):
 *
 *   const startStatus = useAbortableRequest();
 *   const fetchStatus = useCallback(async () => {
 *     const signal = startStatus();
 *     try {
 *       const res = await client.getVCSStatus({ id }, { signal });
 *       if (signal.aborted) return;
 *       setStatus(res.vcsStatus);
 *     } catch (err) {
 *       if (signal.aborted) return;
 *       setError(err);
 *     } finally {
 *       if (!signal.aborted) setLoading(false);
 *     }
 *   }, [id, startStatus]);
 *
 * Every state update after the await must still be guarded by
 * `signal.aborted` — this hook only owns the controller's lifecycle, not
 * your component's state. The require-abort-signal ESLint rule
 * (web-app/eslint-plugin-rpc-lifecycle) enforces that every RPC client call
 * inside lib/hooks/** and lib/contexts/** passes a `{ signal }` at all.
 */ function useAbortableRequest() {
    const controllerRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const start = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        var _controllerRef_current;
        (_controllerRef_current = controllerRef.current) === null || _controllerRef_current === void 0 ? void 0 : _controllerRef_current.abort();
        const controller = new AbortController();
        controllerRef.current = controller;
        return controller.signal;
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            var _controllerRef_current;
            return (_controllerRef_current = controllerRef.current) === null || _controllerRef_current === void 0 ? void 0 : _controllerRef_current.abort();
        };
    }, []);
    return start;
}


/***/ }),

/***/ 3335:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
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

/***/ 3358:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   cf: () => (/* binding */ pickFanoutWinner),
/* harmony export */   mB: () => (/* binding */ fanOutSession),
/* harmony export */   tI: () => (/* binding */ listFanoutMembers),
/* harmony export */   w3: () => (/* binding */ fanoutComparisonPath),
/* harmony export */   wD: () => (/* binding */ MAX_FANOUT_VARIANTS)
/* harmony export */ });
/* unused harmony exports FANOUT_GROUP_TAG_PREFIX, FANOUT_VARIANT_TAG_PREFIX, fanoutGroupIdOf, fanoutVariantOf */
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);



// Fan-out group membership is carried as session tags (see server/services/session_service_fanout.go).
const FANOUT_GROUP_TAG_PREFIX = "fanout:";
const FANOUT_VARIANT_TAG_PREFIX = "fanout-variant:";
const MAX_FANOUT_VARIANTS = 5;
function fanoutGroupIdOf(session) {
    const tag = session.tags.find((t)=>t.startsWith(FANOUT_GROUP_TAG_PREFIX));
    return tag === null || tag === void 0 ? void 0 : tag.slice(FANOUT_GROUP_TAG_PREFIX.length);
}
function fanoutVariantOf(session) {
    const tag = session.tags.find((t)=>t.startsWith(FANOUT_VARIANT_TAG_PREFIX));
    return tag ? Number.parseInt(tag.slice(FANOUT_VARIANT_TAG_PREFIX.length), 10) || 0 : 0;
}
function fanoutComparisonPath(groupId) {
    // Query param rather than /fanout/:groupId — the app is a static export, which cannot
    // pre-render dynamic path segments (same constraint as /sessions/summary?sessionId=).
    return "/fanout?groupId=".concat(encodeURIComponent(groupId));
}
function client() {
    return (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
}
async function fanOutSession(base, count) {
    let programOverrides = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : [];
    const res = await client().fanOutSession({
        base,
        count,
        programOverrides
    });
    return {
        groupId: res.groupId,
        sessions: res.sessions
    };
}
async function listFanoutMembers(groupId) {
    const res = await client().listSessions({
        includeArchived: true
    });
    return res.sessions.filter((s)=>fanoutGroupIdOf(s) === groupId).sort((a, b)=>fanoutVariantOf(a) - fanoutVariantOf(b));
}
async function pickFanoutWinner(groupId, winnerSessionId) {
    return client().pickFanoutWinner({
        groupId,
        winnerSessionId
    });
}


/***/ }),

/***/ 8854:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   u: () => (/* binding */ getErrorMessage)
/* harmony export */ });
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);

/**
 * Converts a thrown value from a ConnectRPC client call into a human-readable
 * message, using `ConnectError.rawMessage` (no `[code]` prefix) instead of the
 * `[code] message`-formatted `.message`. Non-ConnectError Errors fall back to
 * `.message`; anything else (including an empty `rawMessage`) falls back to
 * the caller-supplied fallback string.
 */ function getErrorMessage(e, fallback) {
    if (e instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T) return e.rawMessage || fallback;
    if (e instanceof Error) return e.message || fallback;
    return fallback;
}


/***/ }),

/***/ 21694:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ getKeepaliveConnectTransport),
/* harmony export */   VZ: () => (/* binding */ getWatchTransport),
/* harmony export */   qW: () => (/* binding */ getConnectTransport)
/* harmony export */ });
/* unused harmony export _resetTransportForTesting */
/* harmony import */ var _connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(46564);
/* harmony import */ var _lib_config__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(51202);
/* harmony import */ var _lib_transport_watch_ws_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(90062);



let _transport = null;
let _watchTransport = null;
let _keepaliveTransport = null;
/**
 * Every unary RPC through this transport sends a Connect-Timeout-Ms header
 * with this value unless a call site passes its own timeoutMs — connect-go
 * reads that header and derives the handler's ctx deadline automatically, so
 * this is what makes "the backend can tell a caller disconnected" true for
 * ordinary RPCs (streamViaHub's WebSocket path uses connection-scoped
 * cancellation instead, since a live terminal stream has no fixed deadline).
 */ const DEFAULT_RPC_TIMEOUT_MS = 30000;
/**
 * Returns the shared ConnectRPC HTTP transport singleton.
 *
 * Every non-streaming hook should call this instead of constructing its own
 * transport. This ensures a single transport instance for all unary/streaming
 * non-watch RPCs, carrying the auth interceptor (redirects to /login on a
 * 401) in one place instead of every call site wiring it up separately.
 *
 * The streaming watch transport (createWatchTransport) is separate and should
 * NOT use this singleton — it has its own reconnect logic. Use
 * getWatchTransport() for that.
 */ function getConnectTransport() {
    if (!_transport) {
        _transport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .getApiBaseUrl */ .h)(),
            defaultTimeoutMs: DEFAULT_RPC_TIMEOUT_MS,
            interceptors: [
                (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .createAuthInterceptor */ ._)()
            ]
        });
    }
    return _transport;
}
/**
 * A unary transport whose requests use `fetch(..., { keepalive: true })`, so a
 * request started as the page hides or unloads still reaches the server. Bodies
 * must stay under the browser's 64KB keepalive limit; only use it for small
 * "complete this action now" RPCs (the tray's deferred clear).
 */ function getKeepaliveConnectTransport() {
    if (!_keepaliveTransport) {
        _keepaliveTransport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .getApiBaseUrl */ .h)(),
            defaultTimeoutMs: DEFAULT_RPC_TIMEOUT_MS,
            interceptors: [
                (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .createAuthInterceptor */ ._)()
            ],
            // Cast: Bun's global fetch type also requires `preconnect`, which a wrapper cannot provide (tsconfig.bun.json).
            fetch: (input, init)=>fetch(input, {
                    ...init,
                    keepalive: true
                })
        });
    }
    return _keepaliveTransport;
}
/**
 * Returns the shared transport singleton for server-streaming Watch* RPCs.
 *
 * Selects WebSocket-bridge vs. native ConnectRPC streaming per
 * createSessionWatchTransport's rules (see watch-ws-transport.ts) — every
 * Watch* hook should call this instead of constructing its own transport, so
 * that choice and the auth interceptor live in one place.
 */ function getWatchTransport() {
    if (!_watchTransport) {
        _watchTransport = (0,_lib_transport_watch_ws_transport__WEBPACK_IMPORTED_MODULE_1__/* .createSessionWatchTransport */ .ZX)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .getApiBaseUrl */ .h)(),
            interceptors: [
                (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .createAuthInterceptor */ ._)()
            ]
        });
    }
    return _watchTransport;
}
/** Reset the transport singletons (for testing only). */ function _resetTransportForTesting() {
    _transport = null;
    _watchTransport = null;
    _keepaliveTransport = null;
}


/***/ }),

/***/ 22906:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   F: () => (/* binding */ parseDiff),
/* harmony export */   g: () => (/* binding */ buildGutterMarks)
/* harmony export */ });
/** Parse unified diff format from git into structured DiffFile objects. */ function parseDiff(diffContent) {
    if (!(diffContent === null || diffContent === void 0 ? void 0 : diffContent.trim())) return [];
    const files = [];
    const lineList = diffContent.split("\n");
    let currentFile = null;
    let currentHunk = null;
    let oldLineNum = 0;
    let newLineNum = 0;
    for (const l of lineList){
        if (l.startsWith("diff --git")) {
            if (currentFile && currentHunk) currentFile.changes.push(currentHunk);
            if (currentFile) files.push(currentFile);
            currentFile = {
                filename: "",
                additions: 0,
                deletions: 0,
                changes: []
            };
            currentHunk = null;
        } else if (l.startsWith("+++")) {
            const match = l.match(/\+\+\+ b\/(.*)/);
            if (match && currentFile) currentFile.filename = match[1];
        } else if (l.startsWith("@@")) {
            if (currentFile && currentHunk) currentFile.changes.push(currentHunk);
            const match = l.match(/@@ -(\d+),(\d+) \+(\d+),(\d+) @@/);
            if (match) {
                currentHunk = {
                    oldStart: parseInt(match[1]),
                    oldLines: parseInt(match[2]),
                    newStart: parseInt(match[3]),
                    newLines: parseInt(match[4]),
                    lines: []
                };
                oldLineNum = parseInt(match[1]);
                newLineNum = parseInt(match[3]);
            }
        } else if (currentHunk && (l.startsWith("+") || l.startsWith("-") || l.startsWith(" "))) {
            if (l.startsWith("+")) {
                currentHunk.lines.push({
                    type: "add",
                    content: l,
                    newLineNumber: newLineNum++
                });
                if (currentFile) currentFile.additions++;
            } else if (l.startsWith("-")) {
                currentHunk.lines.push({
                    type: "delete",
                    content: l,
                    oldLineNumber: oldLineNum++
                });
                if (currentFile) currentFile.deletions++;
            } else {
                currentHunk.lines.push({
                    type: "context",
                    content: l,
                    oldLineNumber: oldLineNum++,
                    newLineNumber: newLineNum++
                });
            }
        }
    }
    if (currentFile && currentHunk) currentFile.changes.push(currentHunk);
    if (currentFile) files.push(currentFile);
    return files;
}
/**
 * Build a map of new-file line number → change type, for gutter decorations
 * on an open file. Pure deletions (no matching added line in the hunk) are
 * marked on the line immediately following the deletion point (hunk.newStart),
 * matching common diff-gutter UX (e.g. VS Code's "removed above" indicator).
 */ function buildGutterMarks(diffContent, filePath) {
    const marks = new Map();
    const file = parseDiff(diffContent).find((f)=>f.filename === filePath);
    if (!file) return marks;
    for (const hunk of file.changes){
        const hasAdd = hunk.lines.some((l)=>l.type === "add");
        const hasDelete = hunk.lines.some((l)=>l.type === "delete");
        for (const l of hunk.lines){
            if (l.type === "add" && l.newLineNumber !== undefined) {
                marks.set(l.newLineNumber, hasDelete ? "modify" : "add");
            }
        }
        if (hasDelete && !hasAdd) {
            marks.set(Math.max(1, hunk.newStart), "delete");
        }
    }
    return marks;
}


/***/ }),

/***/ 24031:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ connectApi)
/* harmony export */ });
/* harmony import */ var _reduxjs_toolkit_query_react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(90453);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);


// Generic base query for ConnectRPC unary calls.
// arg shape: { call: () => Promise<unknown> }
const connectBaseQuery = async (param)=>{
    let { call } = param;
    try {
        const result = await call();
        return {
            data: result
        };
    } catch (err) {
        if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T) {
            // Store rawMessage, not message — .message is ConnectRPC's
            // `[code] message`-prefixed string, and callers reading this error
            // slice (e.g. BacklogItemDetail.tsx's lastError?.message reads)
            // display it verbatim to users.
            return {
                error: {
                    status: err.code,
                    error: err.rawMessage || err.message
                }
            };
        }
        const msg = err instanceof Error ? err.message : "Unknown error";
        return {
            error: {
                status: -1,
                error: msg
            }
        };
    }
};
const connectApi = (0,_reduxjs_toolkit_query_react__WEBPACK_IMPORTED_MODULE_1__/* .createApi */ .xP)({
    reducerPath: "connectApi",
    baseQuery: connectBaseQuery,
    tagTypes: [
        "Approvals",
        "GuidanceRequests"
    ],
    endpoints: ()=>({})
});


/***/ }),

/***/ 24044:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Bl: () => (/* binding */ AnalyticsContextProvider),
/* harmony export */   Ig: () => (/* binding */ AnalyticsContext),
/* harmony export */   st: () => (/* binding */ useAnalytics)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ AnalyticsContext,useAnalytics,AnalyticsContextProvider auto */ 

const AnalyticsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
function useAnalytics() {
    const context = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(AnalyticsContext);
    if (!context) {
        throw new Error("useAnalytics must be used within an AnalyticsContextProvider");
    }
    return context;
}
function AnalyticsContextProvider(param) {
    let { provider, children } = param;
    // Stable ref to the provider so identity doesn't change across parent re-renders
    const providerRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(provider);
    providerRef.current = provider;
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        var _p_initialize;
        const p = provider;
        providerRef.current = p;
        void ((_p_initialize = p.initialize) === null || _p_initialize === void 0 ? void 0 : _p_initialize.call(p));
        return ()=>{
            var _p_onClose;
            void ((_p_onClose = p.onClose) === null || _p_onClose === void 0 ? void 0 : _p_onClose.call(p));
        };
    }, [
        provider
    ]);
    const contextValue = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>({
            provider: providerRef.current,
            track: (event)=>providerRef.current.track(event)
        }), // track delegates through the ref so consumers don't re-render on provider swap
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(AnalyticsContext.Provider, {
        value: contextValue,
        children: children
    });
}


/***/ }),

/***/ 24621:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   N0: () => (/* binding */ toggleSelection),
/* harmony export */   Ub: () => (/* binding */ selectAll),
/* harmony export */   wr: () => (/* binding */ clearSelection),
/* harmony export */   xc: () => (/* binding */ selectionAfterBulk)
/* harmony export */ });
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(73559);

const initialState = {
    selectedIds: [],
    selectMode: false
};
// Separate from bulkSelectionSlice (sessions) so selecting backlog rows never
// touches session selection.
const backlogBulkSelectionSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_0__/* .createSlice */ .Z0)({
    name: "backlogBulkSelection",
    initialState,
    reducers: {
        toggleSelection (state, action) {
            const idx = state.selectedIds.indexOf(action.payload);
            if (idx >= 0) {
                state.selectedIds.splice(idx, 1);
            } else {
                state.selectedIds.push(action.payload);
            }
            state.selectMode = true;
        },
        selectAll (state, action) {
            state.selectedIds = action.payload;
            state.selectMode = action.payload.length > 0;
        },
        clearSelection (state) {
            state.selectedIds = [];
            state.selectMode = false;
        }
    }
});
const { toggleSelection, selectAll, clearSelection } = backlogBulkSelectionSlice.actions;
/** Action to apply after a bulk fan-out: clear on full success, otherwise keep only the failed ids selected. */ const selectionAfterBulk = (failedIds)=>failedIds.length === 0 ? clearSelection() : selectAll(failedIds);
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (backlogBulkSelectionSlice.reducer);


/***/ }),

/***/ 25337:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  M_: () => (/* binding */ store),
  jL: () => (/* binding */ useAppDispatch),
  GV: () => (/* binding */ useAppSelector)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@reduxjs+toolkit@2.11.2+71d2413d80c012e4/node_modules/@reduxjs/toolkit/dist/redux-toolkit.modern.mjs + 1 modules
var redux_toolkit_modern = __webpack_require__(73559);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-redux@9.2.0+5436edeee5f9f735/node_modules/react-redux/dist/react-redux.mjs
var react_redux = __webpack_require__(55324);
// EXTERNAL MODULE: ./src/lib/store/backlogItemsSlice.ts
var backlogItemsSlice = __webpack_require__(63939);
// EXTERNAL MODULE: ./src/lib/store/backlogBulkSelectionSlice.ts
var backlogBulkSelectionSlice = __webpack_require__(24621);
;// ./src/lib/store/bulkSelectionSlice.ts

const initialState = {
    selectedIds: []
};
const bulkSelectionSlice = (0,redux_toolkit_modern/* createSlice */.Z0)({
    name: 'bulkSelection',
    initialState,
    reducers: {
        toggleSelection (state, action) {
            const id = action.payload;
            const idx = state.selectedIds.indexOf(id);
            if (idx >= 0) {
                state.selectedIds.splice(idx, 1);
            } else {
                state.selectedIds.push(id);
            }
        },
        selectAll (state, action) {
            state.selectedIds = action.payload;
        },
        clearSelection (state) {
            state.selectedIds = [];
        }
    }
});
const { toggleSelection, selectAll, clearSelection } = bulkSelectionSlice.actions;
/* harmony default export */ const store_bulkSelectionSlice = (bulkSelectionSlice.reducer);

// EXTERNAL MODULE: ./src/lib/store/remotesSlice.ts
var remotesSlice = __webpack_require__(81921);
// EXTERNAL MODULE: ./src/lib/store/reviewQueueSlice.ts
var reviewQueueSlice = __webpack_require__(68453);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ./src/lib/api/connectApi.ts
var connectApi = __webpack_require__(24031);
;// ./src/lib/store/store.ts









const store = (0,redux_toolkit_modern/* configureStore */.U1)({
    reducer: {
        backlogItems: backlogItemsSlice/* default */.Ay,
        backlogBulkSelection: backlogBulkSelectionSlice/* default */.Ay,
        bulkSelection: store_bulkSelectionSlice,
        remotes: remotesSlice/* default */.Ay,
        reviewQueue: reviewQueueSlice/* default */.Ay,
        sessions: sessionsSlice/* default */.Ay,
        [connectApi/* connectApi */.X.reducerPath]: connectApi/* connectApi */.X.reducer
    },
    middleware: (getDefaultMiddleware)=>getDefaultMiddleware().concat(connectApi/* connectApi */.X.middleware)
});
// Typed hooks for use throughout the app
const useAppDispatch = react_redux/* useDispatch */.wA.withTypes();
const useAppSelector = react_redux/* useSelector */.d4.withTypes();


/***/ }),

/***/ 26254:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 38590:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   l: () => (/* binding */ useSessionVcsContext),
/* harmony export */   m: () => (/* binding */ SessionVcsProvider)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_hooks_useSessionVcs__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(60976);
/* __next_internal_client_entry_do_not_use__ SessionVcsProvider,useSessionVcsContext auto */ 


const SessionVcsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
/**
 * Provides a single shared VCS state (status + diff) for a SessionDetail and
 * all of its tabs (VcsPanel, FilesTab, DiffViewer). Without this provider all
 * three tabs fetched independently with no shared cache.
 *
 * Place this around the tab content in SessionDetail, not at the app root —
 * the data is scoped to a single session and should start/stop with it.
 */ function SessionVcsProvider(param) {
    let { sessionId, baseUrl, isActive, children } = param;
    const value = (0,_lib_hooks_useSessionVcs__WEBPACK_IMPORTED_MODULE_2__/* .useSessionVcs */ .u)(sessionId, baseUrl, isActive);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(SessionVcsContext.Provider, {
        value: value,
        children: children
    });
}
function useSessionVcsContext() {
    const ctx = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(SessionVcsContext);
    if (!ctx) throw new Error("useSessionVcsContext must be used within SessionVcsProvider");
    return ctx;
}


/***/ }),

/***/ 41078:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   KA: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useRouter),
/* harmony export */   N_: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.Link),
/* harmony export */   a8: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.usePathname),
/* harmony export */   nQ: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.replaceHash),
/* harmony export */   ok: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useSearchParams),
/* harmony export */   tk: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.pushHistoryMarker),
/* harmony export */   wV: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useReplaceSearch),
/* harmony export */   x7: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.pushTrayEntry)
/* harmony export */ });
/* harmony import */ var _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(99863);
// `@/lib/nav/impl` is aliased per build (next.config.ts, tsconfig paths, jest moduleNameMapper); everything outside this directory imports from here.




/***/ }),

/***/ 56751:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ FanoutRoute)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ./src/components/sessions/DiffViewer.tsx + 2 modules
var DiffViewer = __webpack_require__(81124);
// EXTERNAL MODULE: ./src/lib/contexts/SessionVcsContext.tsx
var SessionVcsContext = __webpack_require__(38590);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/utils/connectError.ts
var connectError = __webpack_require__(8854);
// EXTERNAL MODULE: ./src/lib/hooks/useFanoutService.ts
var useFanoutService = __webpack_require__(3358);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Ffanout%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xYmRmMWkxMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMTJweDsKICBwYWRkaW5nOiAxNnB4OwogIGhlaWdodDogMTAwJTsKICBtaW4taGVpZ2h0OiAwOwp9Ci5fMWJkZjFpMTEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDEycHg7CiAganVzdGlmeS1jb250ZW50OiBzcGFjZS1iZXR3ZWVuOwp9Ci5fMWJkZjFpMTIgewogIGRpc3BsYXk6IGdyaWQ7CiAgZ3JpZC1hdXRvLWZsb3c6IGNvbHVtbjsKICBncmlkLWF1dG8tY29sdW1uczogbWlubWF4KDMyMHB4LCAxZnIpOwogIGdhcDogMTJweDsKICBmbGV4OiAxOwogIG1pbi1oZWlnaHQ6IDA7CiAgb3ZlcmZsb3cteDogYXV0bzsKfQouXzFiZGYxaTEzIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiA4cHg7CiAgbWluLWhlaWdodDogMDsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IDhweDsKICBwYWRkaW5nOiA4cHg7Cn0KLl8xYmRmMWkxNCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogc3BhY2UtYmV0d2VlbjsKICBnYXA6IDhweDsKfQouXzFiZGYxaTE1IHsKICBmbGV4OiAxOwogIG1pbi1oZWlnaHQ6IDA7CiAgb3ZlcmZsb3c6IGF1dG87Cn0KLl8xYmRmMWkxNiB7CiAgZm9udC1zaXplOiAxMnB4OwogIHBhZGRpbmc6IDJweCA4cHg7CiAgYm9yZGVyLXJhZGl1czogOTk5cHg7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKfQouXzFiZGYxaTE3IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMCk7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Ffanout_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xYmRmMWkxMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMTJweDsKICBwYWRkaW5nOiAxNnB4OwogIGhlaWdodDogMTAwJTsKICBtaW4taGVpZ2h0OiAwOwp9Ci5fMWJkZjFpMTEgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDEycHg7CiAganVzdGlmeS1jb250ZW50OiBzcGFjZS1iZXR3ZWVuOwp9Ci5fMWJkZjFpMTIgewogIGRpc3BsYXk6IGdyaWQ7CiAgZ3JpZC1hdXRvLWZsb3c6IGNvbHVtbjsKICBncmlkLWF1dG8tY29sdW1uczogbWlubWF4KDMyMHB4LCAxZnIpOwogIGdhcDogMTJweDsKICBmbGV4OiAxOwogIG1pbi1oZWlnaHQ6IDA7CiAgb3ZlcmZsb3cteDogYXV0bzsKfQouXzFiZGYxaTEzIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiA4cHg7CiAgbWluLWhlaWdodDogMDsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IDhweDsKICBwYWRkaW5nOiA4cHg7Cn0KLl8xYmRmMWkxNCB7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogc3BhY2UtYmV0d2VlbjsKICBnYXA6IDhweDsKfQouXzFiZGYxaTE1IHsKICBmbGV4OiAxOwogIG1pbi1oZWlnaHQ6IDA7CiAgb3ZlcmZsb3c6IGF1dG87Cn0KLl8xYmRmMWkxNiB7CiAgZm9udC1zaXplOiAxMnB4OwogIHBhZGRpbmc6IDJweCA4cHg7CiAgYm9yZGVyLXJhZGl1czogOTk5cHg7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKfQouXzFiZGYxaTE3IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxMCk7Cn0_22_7D = __webpack_require__(26254);
;// ./src/app/fanout/page.css.ts


var badge = '_1bdf1i16';
var column = '_1bdf1i13';
var columnHeader = '_1bdf1i14';
var diffPane = '_1bdf1i15';
var errorText = '_1bdf1i17';
var grid = '_1bdf1i12';
var header = '_1bdf1i11';
var page = '_1bdf1i10';
;// ./src/app/fanout/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: parallel-fanout










const POLL_MS = 5000;
// Mirrors server fanoutWinnerEligibility (session_service_fanout.go); the server is authoritative
// and also rejects an Active session that is still producing output.
const MAYBE_PICKABLE = [
    types_pb/* SessionStatus */.Ph.ACTIVE,
    types_pb/* SessionStatus */.Ph.PAUSED,
    types_pb/* SessionStatus */.Ph.STOPPED,
    types_pb/* SessionStatus */.Ph.HIBERNATED
];
function statusLabel(s) {
    var _SessionStatus_s_status;
    if (s.archivedAt) return "Archived";
    var _SessionStatus_s_status_toString_toLowerCase;
    return (_SessionStatus_s_status_toString_toLowerCase = (_SessionStatus_s_status = types_pb/* SessionStatus */.Ph[s.status]) === null || _SessionStatus_s_status === void 0 ? void 0 : _SessionStatus_s_status.toString().toLowerCase()) !== null && _SessionStatus_s_status_toString_toLowerCase !== void 0 ? _SessionStatus_s_status_toString_toLowerCase : "unknown";
}
function FanoutComparison(param) {
    let { groupId } = param;
    (0,usePageView/* usePageView */.O)();
    const [members, setMembers] = (0,react.useState)([]);
    const [error, setError] = (0,react.useState)(null);
    const [confirming, setConfirming] = (0,react.useState)(null);
    const [picked, setPicked] = (0,react.useState)(null);
    const [pending, setPending] = (0,react.useState)(false);
    const requestSeq = (0,react.useRef)(0);
    // Only the latest request may update state, so a slow poll issued before a pick can't
    // overwrite the post-pick refresh with stale members.
    const refresh = (0,react.useCallback)(async ()=>{
        const seq = ++requestSeq.current;
        try {
            const next = await (0,useFanoutService/* listFanoutMembers */.tI)(groupId);
            if (seq !== requestSeq.current) return;
            setMembers(next);
            setError(null);
        } catch (err) {
            if (seq !== requestSeq.current) return;
            setError((0,connectError/* getErrorMessage */.u)(err, "Fan-out request failed"));
        }
    }, [
        groupId
    ]);
    (0,react.useEffect)(()=>{
        if (!groupId) return;
        void refresh();
        const t = setInterval(()=>void refresh(), POLL_MS);
        return ()=>{
            clearInterval(t);
            requestSeq.current++; // invalidate in-flight requests after unmount/groupId change
        };
    }, [
        groupId,
        refresh
    ]);
    const pick = async (sessionId)=>{
        if (pending) return;
        setPending(true);
        setError(null);
        try {
            const res = await (0,useFanoutService/* pickFanoutWinner */.cf)(groupId, sessionId);
            if (res.failedArchiveSessionIds.length > 0) {
                // Leave picking enabled: repeating the same pick is idempotent and retries the archive.
                await refresh();
                setError("Winner merged, but ".concat(res.failedArchiveSessionIds.length, " other variant(s) could not be archived. Pick the same winner again to retry."));
            } else {
                setPicked(sessionId);
                await refresh();
            }
        } catch (err) {
            // Includes the structured merge-conflict error (Aborted) — surfaced, base branch unchanged.
            setError((0,connectError/* getErrorMessage */.u)(err, "Fan-out request failed"));
        } finally{
            setConfirming(null);
            setPending(false);
        }
    };
    if (!groupId) return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: page,
        children: "Missing groupId."
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: page,
        "data-testid": "fanout-comparison",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("h1", {
                        children: [
                            "Compare variants (",
                            members.length,
                            ")"
                        ]
                    }),
                    picked && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "data-testid": "fanout-picked",
                        children: "Winner merged; other variants archived."
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "alert",
                className: errorText,
                "data-testid": "fanout-error",
                children: error
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: grid,
                children: members.map((s)=>{
                    const archived = Boolean(s.archivedAt);
                    const canPick = !picked && !archived && MAYBE_PICKABLE.includes(s.status);
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                        className: column,
                        "data-testid": "fanout-column-".concat(s.id),
                        "aria-label": s.title,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: columnHeader,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                        children: s.title
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: badge,
                                        "data-testid": "fanout-status-".concat(s.id),
                                        children: statusLabel(s)
                                    })
                                ]
                            }),
                            confirming === s.id ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                "data-testid": "fanout-confirm-".concat(s.id),
                                disabled: pending,
                                onClick: ()=>void pick(s.id),
                                children: "Confirm: merge this variant and archive the others"
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                "data-testid": "fanout-pick-".concat(s.id),
                                disabled: !canPick || pending,
                                onClick: ()=>setConfirming(s.id),
                                children: "Pick as winner"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: diffPane,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionVcsContext/* SessionVcsProvider */.m, {
                                    sessionId: s.id,
                                    baseUrl: (0,config/* getApiBaseUrl */.h)(),
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(DiffViewer/* DiffViewer */.M, {
                                        session: s
                                    })
                                })
                            })
                        ]
                    }, s.id);
                })
            })
        ]
    });
}
function FanoutRouteInner() {
    var _useSearchParams_get;
    const groupId = (_useSearchParams_get = (0,nav/* useSearchParams */.ok)().get("groupId")) !== null && _useSearchParams_get !== void 0 ? _useSearchParams_get : "";
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(FanoutComparison, {
        groupId: groupId
    });
}
function FanoutRoute() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FanoutRouteInner, {})
    });
}


/***/ }),

/***/ 58252:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 60795:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AY: () => (/* binding */ selectSessionsError),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   Hx: () => (/* binding */ selectDetectedStatusMap),
/* harmony export */   Nh: () => (/* binding */ setError),
/* harmony export */   OS: () => (/* binding */ selectSessionsLoading),
/* harmony export */   QA: () => (/* binding */ setErrorCode),
/* harmony export */   Qr: () => (/* binding */ selectSessionsHasLoadedOnce),
/* harmony export */   YI: () => (/* binding */ setSessions),
/* harmony export */   aA: () => (/* binding */ setConnectionState),
/* harmony export */   dF: () => (/* binding */ removeSession),
/* harmony export */   ih: () => (/* binding */ upsertSession),
/* harmony export */   mN: () => (/* binding */ selectConnectionState),
/* harmony export */   od: () => (/* binding */ selectActiveSessionsSortedByUpdatedAt),
/* harmony export */   pu: () => (/* binding */ removeDetectedStatus),
/* harmony export */   r1: () => (/* binding */ setLoading),
/* harmony export */   rt: () => (/* binding */ selectAllSessions)
/* harmony export */ });
/* unused harmony exports selectSessionById, selectSessionIds, selectSessionsTotal, selectSessionsErrorCode */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(13009);
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);


const sessionsAdapter = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createEntityAdapter */ .pU)({
    selectId: (session)=>session.id
});
const initialState = sessionsAdapter.getInitialState({
    loading: false,
    hasLoadedOnce: false,
    error: null,
    detectedStatusMap: {},
    connectionState: "disconnected",
    deletedIds: {}
});
const sessionsSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "sessions",
    initialState,
    reducers: {
        setSessions (state, action) {
            state.hasLoadedOnce = true;
            const filtered = action.payload.filter((s)=>!state.deletedIds[s.id]);
            // Preserve existing entity references when a session's data is unchanged
            // (same updatedAt) so React.memo on SessionRowWrapper can skip re-rendering
            // rows untouched by this snapshot/poll — mirrors the guard in upsertSession.
            const merged = filtered.map((incoming)=>{
                const existing = state.entities[incoming.id];
                if (existing && existing.updatedAt !== undefined && incoming.updatedAt !== undefined && existing.updatedAt.seconds === incoming.updatedAt.seconds && existing.updatedAt.nanos === incoming.updatedAt.nanos) {
                    return existing;
                }
                return incoming;
            });
            sessionsAdapter.setAll(state, merged);
        },
        upsertSession (state, action) {
            // Don't resurrect a deleted session via an in-flight update event
            if (!state.deletedIds[action.payload.id]) {
                // Skip no-op upserts: if updatedAt matches the existing record the data
                // hasn't changed and we'd just cause a spurious re-render. This prevents
                // the N-render storm from WatchSessions initial-snapshot events that
                // duplicate sessions already loaded by the preceding listSessions() call.
                const existing = state.entities[action.payload.id];
                const incoming = action.payload;
                if (existing && existing.updatedAt !== undefined && incoming.updatedAt !== undefined && existing.updatedAt.seconds === incoming.updatedAt.seconds && existing.updatedAt.nanos === incoming.updatedAt.nanos) {
                    return;
                }
                sessionsAdapter.upsertOne(state, action.payload);
                // Sync detectedStatusMap from the session's proto fields
                const session = action.payload;
                if (session.status !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.ACTIVE) {
                    // Non-active: clear badge unconditionally
                    delete state.detectedStatusMap[session.id];
                } else if (session.detectedStatus !== undefined && session.detectedStatus !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNSPECIFIED) {
                    var _session_detectedContext;
                    // Active with typed detection: update map from proto field
                    state.detectedStatusMap[session.id] = {
                        detectedStatus: session.detectedStatus,
                        detectedContext: (_session_detectedContext = session.detectedContext) !== null && _session_detectedContext !== void 0 ? _session_detectedContext : ""
                    };
                } else {
                    // Active + UNSPECIFIED: clear
                    delete state.detectedStatusMap[session.id];
                }
            }
        },
        removeSession (state, action) {
            sessionsAdapter.removeOne(state, action.payload);
            state.deletedIds[action.payload] = true;
        },
        setLoading (state, action) {
            state.loading = action.payload;
        },
        setError (state, action) {
            state.error = action.payload;
            // A fresh error/clear always invalidates the previously-classified code — callers that
            // need a code alongside the message dispatch setErrorCode immediately after setError.
            state.errorCode = undefined;
        },
        setErrorCode (state, action) {
            state.errorCode = action.payload;
        },
        setConnectionState (state, action) {
            state.connectionState = action.payload;
        },
        removeDetectedStatus (state, action) {
            delete state.detectedStatusMap[action.payload];
        }
    }
});
const { setSessions, upsertSession, removeSession, setLoading, setError, setErrorCode, setConnectionState, removeDetectedStatus } = sessionsSlice.actions;
// Use the adapter's built-in selectors scoped to the sessions slice
const adapterSelectors = sessionsAdapter.getSelectors((state)=>state.sessions);
const selectAllSessions = adapterSelectors.selectAll;
const selectSessionById = adapterSelectors.selectById;
// Memoized selector: active sessions pre-sorted by updatedAt descending.
// Avoids repeated O(n log n) sort inside render functions that show recent sessions.
const selectActiveSessionsSortedByUpdatedAt = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectAllSessions, (sessions)=>sessions.filter((s)=>s.status !== _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SessionStatus */ .Ph.UNSPECIFIED).sort((a, b)=>{
        var _b_updatedAt, _a_updatedAt, _b_createdAt, _a_createdAt;
        var _b_updatedAt_seconds, _a_updatedAt_seconds;
        const byUpdated = Number((_b_updatedAt_seconds = (_b_updatedAt = b.updatedAt) === null || _b_updatedAt === void 0 ? void 0 : _b_updatedAt.seconds) !== null && _b_updatedAt_seconds !== void 0 ? _b_updatedAt_seconds : 0) - Number((_a_updatedAt_seconds = (_a_updatedAt = a.updatedAt) === null || _a_updatedAt === void 0 ? void 0 : _a_updatedAt.seconds) !== null && _a_updatedAt_seconds !== void 0 ? _a_updatedAt_seconds : 0);
        if (byUpdated !== 0) return byUpdated;
        var _b_createdAt_seconds, _a_createdAt_seconds;
        const byCreated = Number((_b_createdAt_seconds = (_b_createdAt = b.createdAt) === null || _b_createdAt === void 0 ? void 0 : _b_createdAt.seconds) !== null && _b_createdAt_seconds !== void 0 ? _b_createdAt_seconds : 0) - Number((_a_createdAt_seconds = (_a_createdAt = a.createdAt) === null || _a_createdAt === void 0 ? void 0 : _a_createdAt.seconds) !== null && _a_createdAt_seconds !== void 0 ? _a_createdAt_seconds : 0);
        if (byCreated !== 0) return byCreated;
        return a.id.localeCompare(b.id);
    }));
const selectSessionIds = adapterSelectors.selectIds;
const selectSessionsTotal = adapterSelectors.selectTotal;
const selectSessionsLoading = (state)=>state.sessions.loading;
const selectSessionsHasLoadedOnce = (state)=>state.sessions.hasLoadedOnce;
const selectSessionsError = (state)=>state.sessions.error;
const selectSessionsErrorCode = (state)=>state.sessions.errorCode;
const selectDetectedStatusMap = (state)=>state.sessions.detectedStatusMap;
const selectConnectionState = (state)=>state.sessions.connectionState;
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (sessionsSlice.reducer);


/***/ }),

/***/ 60976:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   u: () => (/* binding */ useSessionVcs)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(66144);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(16294);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* harmony import */ var _lib_store__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(93602);
/* harmony import */ var _lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(60795);
/* harmony import */ var _lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(2663);
/* __next_internal_client_entry_do_not_use__ useSessionVcs auto */ 






/** Fallback poll interval for the currently-focused/visible session. */ const ACTIVE_STATUS_POLL_MS = 60000;
/**
 * Fallback poll interval for a session whose pane is open but not focused
 * (e.g. a background split-pane, or a peeked session behind the active
 * one). Event-driven refetches (Redux `sessionUpdatedAt`) still fire
 * regardless of focus, so this is just the backstop for state changes the
 * push stream might miss — 10x looser than the active interval is enough
 * for a pane the user isn't looking at, and cuts N-1 background panes'
 * contribution to GetVCSStatus/server load by the same factor.
 */ const INACTIVE_STATUS_POLL_MS = 600000;
/**
 * Single source of truth for a session's VCS status and diff data.
 *
 * Intended to be instantiated once per SessionDetail via SessionVcsProvider
 * and consumed by VcsPanel, FilesTab, and DiffViewer through
 * useSessionVcsContext(). This eliminates the 3 independent, uncached fetches
 * those components previously made independently.
 *
 * @param isActive - Whether this session's pane currently has focus.
 * Defaults to true (matches prior always-poll-every-60s behavior) so every
 * caller except a multi-pane layout is unaffected. A multi-pane layout
 * should pass the focused pane's own focus state so unfocused panes fall
 * back to INACTIVE_STATUS_POLL_MS instead of polling as if the user were
 * watching them.
 */ function useSessionVcs(sessionId, baseUrl) {
    let isActive = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : true;
    const [status, setStatus] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [diff, setDiff] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [statusLoading, setStatusLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    const [diffLoading, setDiffLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    // Set once GetVCSStatus reports the session no longer exists, so the 60s
    // fallback interval below stops calling a deleted session forever.
    const stoppedRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    // Stable client reference — initialized once from the shared transport singleton.
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const getClient = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        if (!clientRef.current) {
            clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
        }
        return clientRef.current;
    }, []);
    // Cancel the in-flight status/diff request on the next call or on unmount —
    // this hook is instantiated per-SessionDetail, so switching sessions rapidly
    // without cancellation left every prior request's promise (and its closure
    // over sessionId/setState) alive until it resolved or hit its deadline,
    // growing heap under fast session switching (measured: ~44MB -> ~154MB
    // used JS heap over 138 switches in a profiling run).
    const startStatus = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_5__/* .useAbortableRequest */ .X)();
    const startDiff = (0,_lib_hooks_useAbortableRequest__WEBPACK_IMPORTED_MODULE_5__/* .useAbortableRequest */ .X)();
    const fetchStatus = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (stoppedRef.current) {
            setStatusLoading(false);
            return;
        }
        const signal = startStatus();
        try {
            const response = await getClient().getVCSStatus({
                id: sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            if (response.error) {
                setError(new Error(response.error));
                setStatus(null);
            } else {
                var _response_vcsStatus;
                setStatus((_response_vcsStatus = response.vcsStatus) !== null && _response_vcsStatus !== void 0 ? _response_vcsStatus : null);
                setError(null);
            }
        } catch (err) {
            if (signal.aborted) return;
            if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__/* .ConnectError */ .T && err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_8__/* .Code */ .C.NotFound) {
                stoppedRef.current = true;
                setError(err);
                setStatus(null);
                return;
            }
            setError(err instanceof Error ? err : new Error("Failed to load VCS status"));
        } finally{
            if (!signal.aborted) setStatusLoading(false);
        }
    }, [
        sessionId,
        getClient,
        startStatus
    ]);
    const fetchDiff = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (stoppedRef.current) {
            setDiffLoading(false);
            return;
        }
        const signal = startDiff();
        setDiffLoading(true);
        try {
            const response = await getClient().getSessionDiff({
                id: sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            if (response.diffStats) {
                setDiff({
                    content: response.diffStats.content,
                    added: response.diffStats.added,
                    removed: response.diffStats.removed
                });
            } else {
                setDiff(null);
            }
        } catch (err) {
            if (signal.aborted) return;
            if (err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_7__/* .ConnectError */ .T && err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_8__/* .Code */ .C.NotFound) {
                stoppedRef.current = true;
            }
            // Diff errors are non-fatal — status error is the primary signal.
            console.error("useSessionVcs: failed to load diff:", err);
        } finally{
            if (!signal.aborted) setDiffLoading(false);
        }
    }, [
        sessionId,
        getClient,
        startDiff
    ]);
    const refresh = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        fetchStatus();
        fetchDiff();
    }, [
        fetchStatus,
        fetchDiff
    ]);
    // Watch session state from the Redux store so VCS data re-fetches whenever
    // the session is updated by the server-pushed watchSessions stream, instead
    // of polling on a fixed interval.
    const sessionStatus = (0,_lib_store__WEBPACK_IMPORTED_MODULE_3__/* .useAppSelector */ .GV)((state)=>{
        var _selectAllSessions_find;
        return (_selectAllSessions_find = (0,_lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_4__/* .selectAllSessions */ .rt)(state).find((s)=>s.id === sessionId)) === null || _selectAllSessions_find === void 0 ? void 0 : _selectAllSessions_find.status;
    });
    const sessionUpdatedAt = (0,_lib_store__WEBPACK_IMPORTED_MODULE_3__/* .useAppSelector */ .GV)((state)=>{
        var _s_updatedAt;
        const s = (0,_lib_store_sessionsSlice__WEBPACK_IMPORTED_MODULE_4__/* .selectAllSessions */ .rt)(state).find((s)=>s.id === sessionId);
        var _s_updatedAt_seconds;
        return (_s_updatedAt_seconds = s === null || s === void 0 ? void 0 : (_s_updatedAt = s.updatedAt) === null || _s_updatedAt === void 0 ? void 0 : _s_updatedAt.seconds) !== null && _s_updatedAt_seconds !== void 0 ? _s_updatedAt_seconds : null;
    });
    // VCS status: re-fetch on mount and whenever the session changes in Redux.
    // Event-driven is the primary trigger; the fallback interval catches git
    // commits that happen mid-session without changing the session's status
    // field. isActive controls the fallback's cadence, not whether it fires —
    // a backgrounded pane still needs a backstop, just not one tuned for a
    // user actively watching it.
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        stoppedRef.current = false;
        setStatusLoading(true);
        fetchStatus();
        const pollMs = isActive ? ACTIVE_STATUS_POLL_MS : INACTIVE_STATUS_POLL_MS;
        const fallback = setInterval(()=>{
            if (stoppedRef.current) {
                clearInterval(fallback);
                return;
            }
            if (!document.hidden) fetchStatus();
        }, pollMs);
        return ()=>clearInterval(fallback);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        fetchStatus,
        sessionStatus,
        sessionUpdatedAt,
        isActive
    ]);
    // Diff: fetch once on mount; consumers call refreshDiff() when needed.
    // Cancellation on unmount/re-run is handled by startDiff's useAbortableRequest.
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        fetchDiff();
    }, [
        fetchDiff
    ]);
    return {
        status,
        diff,
        statusLoading,
        diffLoading,
        loading: statusLoading,
        error,
        refreshStatus: fetchStatus,
        refreshDiff: fetchDiff,
        refresh
    };
}


/***/ }),

/***/ 63939:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ removeItem),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   Bi: () => (/* binding */ selectAllBacklogItems),
/* harmony export */   Ky: () => (/* binding */ selectBacklogItemsLiveVersionMap),
/* harmony export */   Se: () => (/* binding */ appendActivityNote),
/* harmony export */   vH: () => (/* binding */ upsertItem),
/* harmony export */   zc: () => (/* binding */ selectBacklogItemById)
/* harmony export */ });
/* unused harmony exports selectBacklogItemsMap, selectBacklogItemsTotal, makeSelectBacklogItemsByStatus */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(13009);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(28765);


const initialState = {
    items: {},
    liveVersion: {}
};
/** Returns the epoch-ms value of a proto Timestamp, or 0 if unset (oldest possible). */ function timestampMs(ts) {
    return ts ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_0__/* .timestampDate */ .FP)(ts).getTime() : 0;
}
const backlogItemsSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "backlogItems",
    initialState,
    reducers: {
        /**
     * Upserts a backlog item, guarded by a real `updatedAt`-based staleness
     * check (not sessionsSlice.upsertSession's equal-only check): an incoming
     * item strictly older than the currently-stored item for the same id is
     * dropped, so out-of-order event delivery (e.g. concurrent publishers,
     * stream replay) can never regress the store to older data.
     *
     * `isSnapshot` (default `true`) drives `liveVersion` bookkeeping only —
     * pass `false` exclusively for a genuine live `BacklogItemEvent` (i.e.
     * `event.isSnapshot === false`) so Epic 6.1's flash treatment can tell a
     * real-time change apart from a snapshot/replay/poll refresh.
     */ upsertItem: {
            reducer (state, action) {
                const { item: incoming, isSnapshot } = action.payload;
                const existing = state.items[incoming.id];
                if (existing && timestampMs(incoming.updatedAt) < timestampMs(existing.updatedAt)) {
                    return;
                }
                // Backstop for a partially-loaded event push (item_sessions/activity_notes/
                // allowed_transitions dropped by a publishItemChanged call site or a
                // non-eager-loaded snapshot query) racing a fully-loaded one: never let an
                // empty itemSessions, activityNotes, or allowedTransitions clobber data we
                // already have for this item. A genuine all-removed update would still
                // arrive with a newer updatedAt and populated (even if empty-by-intent)
                // data from a call site that *did* eager-load, so this only masks the
                // known-bad partial-load case, not real removals.
                //
                // activityNotes specifically: every BacklogChangeKind other than
                // ChangeActivityNoteAdded publishes via publishItemChanged ->
                // attachItemSessionsForPublish (session/ent_repository_backlog.go), which
                // re-populates only ItemSessions, never ActivityNotes — so their wire
                // events embed a full protoItem whose activity_notes is empty. Without
                // this guard, the very next status transition/verdict/session-attach would
                // wipe out any notes accumulated via the dedicated appendActivityNote
                // reducer below (ADR-002, Blocker 1 fix).
                //
                // allowedTransitions specifically: a pure function of status, so it's
                // never legitimately empty for a real item — an empty value is always a
                // sparse-DTO/resync artifact (e.g. a stale ListBacklogItems shape).
                //
                // planArtifactsPath specifically: unlike the three fields above, it CAN
                // legitimately go from set back to empty (TransitionBacklogItemStatus
                // resets it when an item moves back to idea/refining, e.g. re-triage) —
                // so this is gated on the item's status being unchanged. Every real
                // reset path changes status in the same write (server-side
                // backlog_service_lifecycle.go), so "same status, plan vanished" is
                // never a legitimate transition and is always a sparse-DTO artifact
                // (ListBacklogItems' backlogItemSummaryToProto omitted this field
                // entirely until the fix alongside this guard — getPrimaryCardAction/
                // itemActions.ts derives a ready item's card action directly from it,
                // so a sparse resync could flip "Approve Plan" to "Trigger Triage").
                let nextItem = incoming;
                if (existing) {
                    var _existing_itemSessions, _incoming_itemSessions, _existing_activityNotes, _incoming_activityNotes, _existing_allowedTransitions, _incoming_allowedTransitions, _existing_blocking, _incoming_blocking;
                    const patch = {};
                    var _existing_itemSessions_length, _incoming_itemSessions_length;
                    if (((_existing_itemSessions_length = (_existing_itemSessions = existing.itemSessions) === null || _existing_itemSessions === void 0 ? void 0 : _existing_itemSessions.length) !== null && _existing_itemSessions_length !== void 0 ? _existing_itemSessions_length : 0) > 0 && ((_incoming_itemSessions_length = (_incoming_itemSessions = incoming.itemSessions) === null || _incoming_itemSessions === void 0 ? void 0 : _incoming_itemSessions.length) !== null && _incoming_itemSessions_length !== void 0 ? _incoming_itemSessions_length : 0) === 0) {
                        patch.itemSessions = existing.itemSessions;
                    }
                    var _existing_activityNotes_length, _incoming_activityNotes_length;
                    if (((_existing_activityNotes_length = (_existing_activityNotes = existing.activityNotes) === null || _existing_activityNotes === void 0 ? void 0 : _existing_activityNotes.length) !== null && _existing_activityNotes_length !== void 0 ? _existing_activityNotes_length : 0) > 0 && ((_incoming_activityNotes_length = (_incoming_activityNotes = incoming.activityNotes) === null || _incoming_activityNotes === void 0 ? void 0 : _incoming_activityNotes.length) !== null && _incoming_activityNotes_length !== void 0 ? _incoming_activityNotes_length : 0) === 0) {
                        patch.activityNotes = existing.activityNotes;
                    }
                    var _existing_allowedTransitions_length, _incoming_allowedTransitions_length;
                    if (((_existing_allowedTransitions_length = (_existing_allowedTransitions = existing.allowedTransitions) === null || _existing_allowedTransitions === void 0 ? void 0 : _existing_allowedTransitions.length) !== null && _existing_allowedTransitions_length !== void 0 ? _existing_allowedTransitions_length : 0) > 0 && ((_incoming_allowedTransitions_length = (_incoming_allowedTransitions = incoming.allowedTransitions) === null || _incoming_allowedTransitions === void 0 ? void 0 : _incoming_allowedTransitions.length) !== null && _incoming_allowedTransitions_length !== void 0 ? _incoming_allowedTransitions_length : 0) === 0) {
                        patch.allowedTransitions = existing.allowedTransitions;
                    }
                    var _existing_blocking_length, _incoming_blocking_length;
                    // blocking is only populated by GetBacklogItem/Add/Remove; Watch
                    // payloads never carry it, so a live event must not erase it.
                    if (((_existing_blocking_length = (_existing_blocking = existing.blocking) === null || _existing_blocking === void 0 ? void 0 : _existing_blocking.length) !== null && _existing_blocking_length !== void 0 ? _existing_blocking_length : 0) > 0 && ((_incoming_blocking_length = (_incoming_blocking = incoming.blocking) === null || _incoming_blocking === void 0 ? void 0 : _incoming_blocking.length) !== null && _incoming_blocking_length !== void 0 ? _incoming_blocking_length : 0) === 0) {
                        patch.blocking = existing.blocking;
                    }
                    if (existing.status === incoming.status && existing.planArtifactsPath && !incoming.planArtifactsPath) {
                        patch.planArtifactsPath = existing.planArtifactsPath;
                    }
                    if (Object.keys(patch).length > 0) {
                        nextItem = {
                            ...incoming,
                            ...patch
                        };
                    }
                }
                state.items[incoming.id] = nextItem;
                if (!isSnapshot) {
                    var _state_liveVersion_incoming_id;
                    state.liveVersion[incoming.id] = ((_state_liveVersion_incoming_id = state.liveVersion[incoming.id]) !== null && _state_liveVersion_incoming_id !== void 0 ? _state_liveVersion_incoming_id : 0) + 1;
                }
            },
            prepare (item) {
                let isSnapshot = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : true;
                return {
                    payload: {
                        item,
                        isSnapshot
                    }
                };
            }
        },
        /** Deletes an item from the map entirely (BacklogItemRemovedEvent — permanent delete, not upsert). */ removeItem (state, action) {
            delete state.items[action.payload];
        },
        /**
     * Targeted single-note append for the dedicated activityNoteAdded live
     * event (ADR-002, backlog-item-activity-log) — appends `note` to
     * `state.items[itemId].activityNotes` without touching any other field,
     * never a call to upsertItem/a wholesale item replace. No-op if the item
     * isn't in the store yet (matches removeItem's "item not present"
     * tolerance) — the next GetBacklogItem/ListBacklogItems refresh will
     * include it.
     */ appendActivityNote (state, action) {
            const { itemId, note } = action.payload;
            const existing = state.items[itemId];
            if (!existing) return;
            var _existing_activityNotes;
            state.items[itemId] = {
                ...existing,
                activityNotes: [
                    ...(_existing_activityNotes = existing.activityNotes) !== null && _existing_activityNotes !== void 0 ? _existing_activityNotes : [],
                    note
                ]
            };
        }
    }
});
const { upsertItem, removeItem, appendActivityNote } = backlogItemsSlice.actions;
// Base selectors
const selectBacklogItemsMap = (state)=>state.backlogItems.items;
const selectBacklogItemById = (state, id)=>state.backlogItems.items[id];
/**
 * Raw `liveVersion` map — returned directly (like `selectBacklogItemsMap`)
 * rather than via `createSelector`: Immer already gives it the same
 * structural-sharing guarantee (unrelated keys keep their prior value
 * across an unrelated item's update), so no extra memoization is needed.
 */ const selectBacklogItemsLiveVersionMap = (state)=>state.backlogItems.liveVersion;
// Memoized selectors: list-shaped reads off backlogItemsSlice must not allocate
// a new array/object on every call, or every consumer (e.g. BacklogItemCard)
// re-renders whenever ANY item in the store changes, not just its own.
const selectAllBacklogItems = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectBacklogItemsMap, (items)=>Object.values(items));
const selectBacklogItemsTotal = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSelector */ .Mz)(selectBacklogItemsMap, (items)=>Object.keys(items).length);
/** Shallow array equality: same length and same element references, in order. */ function shallowArrayEqual(a, b) {
    if (a === b) return true;
    if (a.length !== b.length) return false;
    for(let i = 0; i < a.length; i++){
        if (a[i] !== b[i]) return false;
    }
    return true;
}
/**
 * Selector factory for a status-filtered view. Use one instance per
 * component (e.g. via `useMemo(makeSelectBacklogItemsByStatus, [])`) rather
 * than a single shared selector — a shared selector's single-entry cache
 * would thrash (and stop memoizing) if multiple components filter by
 * different statuses simultaneously.
 *
 * `resultEqualityCheck: shallowArrayEqual` is what actually delivers on the
 * "unrelated item update shouldn't re-render this list" guarantee: without
 * it, the filtered array is a brand-new reference every time ANY item in the
 * store changes (because `selectAllBacklogItems` itself must return a new
 * array whenever the underlying map changes), even if none of the matching
 * items actually differ. With it, the selector still recomputes, but returns
 * the previous array reference when the filtered contents are unchanged.
 */ const makeSelectBacklogItemsByStatus = ()=>createSelector([
        selectAllBacklogItems,
        (_state, status)=>status
    ], (items, status)=>items.filter((item)=>item.status === status), {
        memoizeOptions: {
            resultEqualityCheck: shallowArrayEqual
        }
    });
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (backlogItemsSlice.reducer);


/***/ }),

/***/ 68453:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ removeItem),
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   B5: () => (/* binding */ addItem),
/* harmony export */   K6: () => (/* binding */ selectReviewQueueLoading),
/* harmony export */   KS: () => (/* binding */ selectReviewQueue),
/* harmony export */   Nh: () => (/* binding */ setError),
/* harmony export */   _W: () => (/* binding */ selectReviewQueueItemsWithLiveStatus),
/* harmony export */   fk: () => (/* binding */ updateItem),
/* harmony export */   r1: () => (/* binding */ setLoading),
/* harmony export */   s6: () => (/* binding */ setReviewQueue),
/* harmony export */   wk: () => (/* binding */ selectReviewQueueLastUpdatedAt),
/* harmony export */   wq: () => (/* binding */ selectReviewQueueError)
/* harmony export */ });
/* unused harmony exports setReviewQueueStats, selectReviewQueueItems, selectReviewQueueStats, selectWaitingItems, selectQueueCountsByWorkingState */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(73559);
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(13009);
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
/* harmony import */ var _lib_utils_deriveWorkingState__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(81801);



const initialState = {
    reviewQueue: null,
    loading: false,
    error: null,
    lastUpdatedAt: null,
    stats: {
        totalItems: 0,
        byPriority: {},
        byReason: {},
        averageAgeSeconds: "0",
        oldestItemId: "",
        oldestAgeSeconds: "0"
    }
};
const reviewQueueSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_2__/* .createSlice */ .Z0)({
    name: "reviewQueue",
    initialState,
    reducers: {
        setReviewQueue (state, action) {
            state.reviewQueue = action.payload;
            if (action.payload) {
                state.stats.totalItems = action.payload.totalItems;
            }
            state.lastUpdatedAt = Date.now();
        },
        setReviewQueueStats (state, action) {
            state.stats = action.payload;
        },
        setLoading (state, action) {
            state.loading = action.payload;
        },
        setError (state, action) {
            state.error = action.payload;
        },
        removeItem (state, action) {
            if (!state.reviewQueue) return;
            const before = state.reviewQueue.items.length;
            state.reviewQueue.items = state.reviewQueue.items.filter((item)=>item.sessionId !== action.payload);
            const removed = before - state.reviewQueue.items.length;
            state.reviewQueue.totalItems = Math.max(0, state.reviewQueue.totalItems - removed);
            state.stats.totalItems = Math.max(0, state.stats.totalItems - removed);
        },
        addItem (state, action) {
            if (!state.reviewQueue) return;
            const exists = state.reviewQueue.items.some((i)=>i.sessionId === action.payload.sessionId);
            if (!exists) {
                state.reviewQueue.items.push(action.payload);
                state.reviewQueue.totalItems += 1;
                state.stats.totalItems += 1;
            }
        },
        updateItem (state, action) {
            if (!state.reviewQueue) return;
            const item = state.reviewQueue.items.find((i)=>i.sessionId === action.payload.sessionId);
            if (item) {
                Object.assign(item, action.payload.updates);
            }
        }
    }
});
const { setReviewQueue, setReviewQueueStats, setLoading, setError, removeItem, addItem, updateItem } = reviewQueueSlice.actions;
// Selectors
const selectReviewQueue = (state)=>state.reviewQueue.reviewQueue;
const selectReviewQueueItems = (state)=>{
    var _state_reviewQueue_reviewQueue;
    var _state_reviewQueue_reviewQueue_items;
    return (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
};
const selectReviewQueueStats = (state)=>state.reviewQueue.stats;
const selectReviewQueueLoading = (state)=>state.reviewQueue.loading;
const selectReviewQueueError = (state)=>state.reviewQueue.error;
const selectReviewQueueLastUpdatedAt = (state)=>state.reviewQueue.lastUpdatedAt;
// selectReviewQueueItemsWithLiveStatus joins review queue items with live session state from
// sessionsSlice. It overrides each item's workingState using the live detectedStatus from
// the WatchSessions stream, so the review queue panel always reflects the current session
// state even when the WatchReviewQueue stream hasn't pushed a fresh snapshot yet.
const selectReviewQueueItemsWithLiveStatus = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_3__/* .createSelector */ .Mz)([
    (state)=>{
        var _state_reviewQueue_reviewQueue;
        var _state_reviewQueue_reviewQueue_items;
        return (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
    },
    (state)=>state.sessions.detectedStatusMap
], (items, detectedStatusMap)=>items.map((item)=>{
        const liveStatus = detectedStatusMap[item.sessionId];
        if (!liveStatus) return item;
        const liveWorkingState = (0,_lib_utils_deriveWorkingState__WEBPACK_IMPORTED_MODULE_1__/* .deriveWorkingState */ .m)({
            subStatus: item.subStatus,
            detectedStatus: liveStatus.detectedStatus
        });
        return {
            ...item,
            workingState: liveWorkingState
        };
    }));
// selectWaitingItems returns only items that are NOT actively working,
// so the queue shows sessions that genuinely need user attention.
const selectWaitingItems = (state)=>{
    var _state_reviewQueue_reviewQueue;
    var _state_reviewQueue_reviewQueue_items;
    const items = (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
    return items.filter((item)=>{
        const ws = deriveWorkingState(item);
        return ws !== WorkingState.ACTIVE && ws !== WorkingState.PROCESSING;
    });
};
// selectQueueCountsByWorkingState returns per-category counts for the header badge.
const selectQueueCountsByWorkingState = (state)=>{
    var _state_reviewQueue_reviewQueue;
    var _state_reviewQueue_reviewQueue_items;
    const items = (_state_reviewQueue_reviewQueue_items = (_state_reviewQueue_reviewQueue = state.reviewQueue.reviewQueue) === null || _state_reviewQueue_reviewQueue === void 0 ? void 0 : _state_reviewQueue_reviewQueue.items) !== null && _state_reviewQueue_reviewQueue_items !== void 0 ? _state_reviewQueue_reviewQueue_items : [];
    let waiting = 0;
    let working = 0;
    let stuck = 0;
    for (const item of items){
        const ws = deriveWorkingState(item);
        if (ws === WorkingState.ACTIVE || ws === WorkingState.PROCESSING) {
            working++;
        } else if (ws === WorkingState.WAITING) {
            stuck++;
        } else {
            waiting++;
        }
    }
    return {
        waiting,
        working,
        stuck
    };
};
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (reviewQueueSlice.reducer);


/***/ }),

/***/ 74512:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B2: () => (/* binding */ prBadgeBlocking),
/* harmony export */   BS: () => (/* binding */ priorityLabel),
/* harmony export */   Eg: () => (/* binding */ prBadge),
/* harmony export */   Kk: () => (/* binding */ icon),
/* harmony export */   L7: () => (/* binding */ prBadgeComplete),
/* harmony export */   Qq: () => (/* binding */ text),
/* harmony export */   VG: () => (/* binding */ prBadgeReady),
/* harmony export */   hC: () => (/* binding */ prBadgeDraft),
/* harmony export */   ho: () => (/* binding */ prBadgePending),
/* harmony export */   m2: () => (/* binding */ repoBadge),
/* harmony export */   o5: () => (/* binding */ prBadgeUnknown),
/* harmony export */   oE: () => (/* binding */ compact),
/* harmony export */   qS: () => (/* binding */ badge),
/* harmony export */   sv: () => (/* binding */ prBadgeError)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FGitHubBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W227bMAx9z1fwsQFiQ3Lu6st_Y0_FYtGxUEfSJDlxO_TfB8tOYnvJ3C3FgCCIKfLwHJKxGL9QR8V65Qn8nAAI6UzB3xhIVUiFUVZg9TwB4IXcq0h6PDgGKSqPtjbvuWGwMMHFcCGk2odnoKQx7rQVaCPLhSwdA5o05kwrHzn5jgPTCeU_9wxWhNQ2j5WPBKbaci_1YqC0wnBguXKysfGiABInDpC7cJiW1mnLwGh55tnQYEBNBU4XUjQIhltU_nnyMYkvZWC5PqINxQg_mbYH1vwsuMfvTxE11bQBrSKXc6FPDAgkpgrK7X7Hn8gM2k9MpwN8nnp5xD8kIMOITKelCwG69HVbWMjWCDly_xRFL3SL74l9PwRmrVuks8yhD949SBrAeg3b_N4YOohKQtSOp697q0sl2M3cqS7q4vePfkw7w3DTw_RFJ5023M9o7vRhde4DnZMZbMgMkmReN2PezzIfwU_vKyKjinCQ61OKNqOwckzz_dlbjCSn5JEO0sHcLjqCteGp9G8MSLz9PP2kD7gcoV8_wr4c5Ppq8qsR8tUjo1YNcn01_fUI_eSRyg9zPUyeLvuIm7Gxf2jqmxfuleimn_uWGtpz2f7Xf_X2q2eD_s7lP7tO5Q_Pht9wF2A6l565MY2sm62kQK9Rxu5OnF9bZKYzPFw2VM6l3lpDNq0XUeGRNtrNyowO1_NlyI0tuuWEugny_UMrl8kXoS6HbjdS9WF6QhLg7CTFD5nQNsNLW_1nJ_r5S5yuZXqlQHpxYsmPpceg2Csd6_T5SYoOqLNirrQuRQC1aUE1wMsCmmcdA3RKmqZJIQ0RL8dUEgOLrWICrgS8NRxW682ppoGCnF_P72xodTGyyozv64yH93otI0_VyQ5R15qktyIEm1UhxpdkrPjxy_hBehOPAsAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(78554);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FGitHubBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W227bMAx9z1fwsQFiQ3Lu6st_Y0_FYtGxUEfSJDlxO_TfB8tOYnvJ3C3FgCCIKfLwHJKxGL9QR8V65Qn8nAAI6UzB3xhIVUiFUVZg9TwB4IXcq0h6PDgGKSqPtjbvuWGwMMHFcCGk2odnoKQx7rQVaCPLhSwdA5o05kwrHzn5jgPTCeU_9wxWhNQ2j5WPBKbaci_1YqC0wnBguXKysfGiABInDpC7cJiW1mnLwGh55tnQYEBNBU4XUjQIhltU_nnyMYkvZWC5PqINxQg_mbYH1vwsuMfvTxE11bQBrSKXc6FPDAgkpgrK7X7Hn8gM2k9MpwN8nnp5xD8kIMOITKelCwG69HVbWMjWCDly_xRFL3SL74l9PwRmrVuks8yhD949SBrAeg3b_N4YOohKQtSOp697q0sl2M3cqS7q4vePfkw7w3DTw_RFJ5023M9o7vRhde4DnZMZbMgMkmReN2PezzIfwU_vKyKjinCQ61OKNqOwckzz_dlbjCSn5JEO0sHcLjqCteGp9G8MSLz9PP2kD7gcoV8_wr4c5Ppq8qsR8tUjo1YNcn01_fUI_eSRyg9zPUyeLvuIm7Gxf2jqmxfuleimn_uWGtpz2f7Xf_X2q2eD_s7lP7tO5Q_Pht9wF2A6l565MY2sm62kQK9Rxu5OnF9bZKYzPFw2VM6l3lpDNq0XUeGRNtrNyowO1_NlyI0tuuWEugny_UMrl8kXoS6HbjdS9WF6QhLg7CTFD5nQNsNLW_1nJ_r5S5yuZXqlQHpxYsmPpceg2Csd6_T5SYoOqLNirrQuRQC1aUE1wMsCmmcdA3RKmqZJIQ0RL8dUEgOLrWICrgS8NRxW682ppoGCnF_P72xodTGyyozv64yH93otI0_VyQ5R15qktyIEm1UhxpdkrPjxy_hBehOPAsAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FGitHubBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W227bMAx9z1fwsQFiQ3Lu6st_Y0_FYtGxUEfSJDlxO_TfB8tOYnvJ3C3FgCCIKfLwHJKxGL9QR8V65Qn8nAAI6UzB3xhIVUiFUVZg9TwB4IXcq0h6PDgGKSqPtjbvuWGwMMHFcCGk2odnoKQx7rQVaCPLhSwdA5o05kwrHzn5jgPTCeU_9wxWhNQ2j5WPBKbaci_1YqC0wnBguXKysfGiABInDpC7cJiW1mnLwGh55tnQYEBNBU4XUjQIhltU_nnyMYkvZWC5PqINxQg_mbYH1vwsuMfvTxE11bQBrSKXc6FPDAgkpgrK7X7Hn8gM2k9MpwN8nnp5xD8kIMOITKelCwG69HVbWMjWCDly_xRFL3SL74l9PwRmrVuks8yhD949SBrAeg3b_N4YOohKQtSOp697q0sl2M3cqS7q4vePfkw7w3DTw_RFJ5023M9o7vRhde4DnZMZbMgMkmReN2PezzIfwU_vKyKjinCQ61OKNqOwckzz_dlbjCSn5JEO0sHcLjqCteGp9G8MSLz9PP2kD7gcoV8_wr4c5Ppq8qsR8tUjo1YNcn01_fUI_eSRyg9zPUyeLvuIm7Gxf2jqmxfuleimn_uWGtpz2f7Xf_X2q2eD_s7lP7tO5Q_Pht9wF2A6l565MY2sm62kQK9Rxu5OnF9bZKYzPFw2VM6l3lpDNq0XUeGRNtrNyowO1_NlyI0tuuWEugny_UMrl8kXoS6HbjdS9WF6QhLg7CTFD5nQNsNLW_1nJ_r5S5yuZXqlQHpxYsmPpceg2Csd6_T5SYoOqLNirrQuRQC1aUE1wMsCmmcdA3RKmqZJIQ0RL8dUEgOLrWICrgS8NRxW682ppoGCnF_P72xodTGyyozv64yH93otI0_VyQ5R15qktyIEm1UhxpdkrPjxy_hBehOPAsAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var badge = '_1s1d76t0';
var compact = '_1s1d76t1';
var icon = '_1s1d76tc';
var prBadge = '_1s1d76t2';
var prBadgeBlocking = '_1s1d76t4';
var prBadgeComplete = '_1s1d76t8';
var prBadgeDraft = '_1s1d76t7';
var prBadgeError = '_1s1d76t9';
var prBadgePending = '_1s1d76t6';
var prBadgeReady = '_1s1d76t5';
var prBadgeUnknown = '_1s1d76ta';
var priorityLabel = '_1s1d76tb';
var repoBadge = '_1s1d76t3';
var text = '_1s1d76td';

/***/ }),

/***/ 78516:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  F: () => (/* binding */ DiffRenderer)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/utils/parseDiff.ts
var parseDiff = __webpack_require__(22906);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FDiffRenderer.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VYS2_bOBC-51fwskgDlIbk2k7CXgr0nOOeFouCksYWY4pUScp2ssh_X4h6kbIsyUmx2J4smcPhfDPfPKjFj5C_7ON0GaB_bhBKmM45fSFoy-H09QbZX5wwBbFhUhAUS15kolxJge1SQ1AYBH-U7xGN9zslC5EQdKDqE8Y_wkd4XarXMLkrBWLJpTpbA7u2lcLgLc0YfyHo9kkKGsvbz-j2CQS3D39GhTAFepLCvn6XQktOdfmsZaFiwLFMAOeqXM6kkDqnMXy9ebtZNAjDYYSUs53AzECmCYpBGFDl38-FNmz7gmMpDAhDkFWIIzBHAOuAnCYJEzuCgsX9WkGGQgXZhCd2Fm0kVQIKR9IYmREU5iekJWdJX3p714ZAp4qJPUFBCYkoKc1fCTUUc9ganFKRQPI36kHtx07JI1ZwAKV9xyyvcsyO5qSFauOm2SuUXniwbvBUf7GqL0feEV1dFi06jhxr1q2DwNu9HjkomN6-GXaBxRosluvpyO7dyCqasEITtMpPPaYsz1x0b492RFYll2pO9Q81igqdUwXCdMcRJKSAgeO_VMdPJl4_gAjFhdLlnlyyJu72aFYxqTOphKQ_V0fYZx8bSeUBlEU44Ttn18OEfHa5nPz0NT1eLmoEheVjxgRuKplPCdrlkJN8CB1ZYlKClpug8m4JcMvlEb8QRAsj3TjUFXI8vyfKhWNS9IsrWE3vJug1sqagO5Rcf5iSZZ2yhKw90ds1n6K1BQZOBlvgBJW6B0nruu5dTIznyHsY8SCSzFebWLUNcQhKWZJUEbGwugXgnOWaaRudlBnANoald4-K5vZvqRIcKaB7guwPppx7p8FobVs1lfyM6J7fz0v7ts2POpWGEqHlUHi2nRB8hGjPDNaxkpxHtIpOw8JllV7tmGHfJxRgo2i8nwpacjdDUVpk0VXRb4ruprK7yYPlpeSfbcU85m59dTsrn1G1Y6IbNNpUb6ybKk2D0M6J6xyc_pZTlgOAVbx2p4VNEIwXKWf782i2tRV0anCqSVxBMDKfbz_vzxNzvbYfmdKfx4YFb3hnIgXFjGdSdmVGiqs45FgV1hMPZwLath4uNnMbkWODnLD5MK_yFz6yfMoT4Ty1dqZ19P680sPK9zAT1mMRl_HemQTWQX9-dSnstmA76UzRp9CgsAYOsenmg_PbTWuk7jG5u3V0TcfrirkCrxGdmkbkKv1o3xlS8K6-M6zov-47l614V98x135GmNsYuqX-h4eWIMsBYjbbHBOL_6mJY8njmH9wO7vtDN0Ef-k2N0fv8TdzS68Cns7c0lSL_tV6qBVeuMRc7LnByG1_8iPER6_bPu5rLjbfMkgYRZ8yesJ1vbvfPOSnO7u99_loLPa96GujwMRptdIfdUZvkm_uucv63BEKefKbK-UfJ3F54rQW792LS5aduumi-Q7Qv_U3De7az32eCdvahP4UfCaYXhuyqu8aqowXtUHlynfEqkXsDQaDW_UZAEfSn93qxvt28_YvVa-cE5UWAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FDiffRenderer_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VYS2_bOBC_51fwskgDlIbk2k7CXgr0nOOeFouCksYWY4pUScp2ssh_X4h6kbIsyUmx2J4smcPhfDPfPKjFj5C_7ON0GaB_bhBKmM45fSFoy_H09QbZX5wwBbFhUhAUS15kolxJge1SQ1AYBH_U7xGN9zslC5EQdKDqE8Y_wkd4XarXMLkrBWLJpTpbA7u2lcLgLc0YfyHo9kkKGsvbz_j2CQS3D39GhTAFepLCvn6XQktOdfmsZaFiwLFMAOeqXM6kkDqnMXy9ebtZNAjDYYSUs53AzECmCYpBGFDl38_FNmz7gmMpDAhDkFWIIzBHAOuAnCYJEzuCgsX9WkGGQgXZhCd2Fm0kVQIKR9IYmREU5iekJWdJX3p714ZAp4qJPUFBCYkoKc1fCTUUc9ganFKRQPI36kHtx07JI1ZwAKV9xyyvcsyO5qSFauOm2SuUXniwbvBUf7GqL0feEV1dFi06jhxr1q2DwNu9HjkomN6_GXaBxRosluvpyO7dyCqasEITtMpPPaYsz1x0b492RFYll2pO9Q81igqdUwXCdMcRJKSAgeO_VMdPJl4_gAjFhdLlnlyyJu72aFYxqTOphKQ_V0fYZx8bSeUBlEU44Ttn18OEfHa5nPz0NT1eLmoEheVjxgRuKplPCdrlkJN8CB1ZYlKClpug8m4JcMvlEb8QRAsj3TjUFXI8vyfKhWNS9IsrWE3vJug1sqagO5Rcf5iSZZ2yhKw90ds1n6K1BQZOBlvgBJW6B0nruu5dTIznyHsY8SCSzFebWLUNcQhKWZJUEbGwugXgnOWaaRudlBnANoald4_K5vZvqRIcKaB7guwPppx7p8FobVs1lfyM6J7fz0v7ts2POpWGEqHlUHi2nRB8hGjPDNaxkpxHtIpOw8JllV7tmGHfJxRgo2i8nwpacjdDUVpk0VXRb4ruprK7yYPlpeSfbcU85m59dTsrn1G1Y6IbNNpUb6ybKk2D0M6J6xyc_pZTlgOAVbx2p4VNEIwXKWf782i2tRV0anCqSVxBMDKfbz_vzxNzvbYfmdKfx4YFb3hnIgXFjGdSdmVGiqs45FgV1hMPZwLath4uNnMbkWODnLD5MK_yFz6yfMoT4Ty1dqZ19P680sPK9zAT1mMRl_HemQTWQX9_dSnstmA76UzRp9CgsAYOsenmg_PbTWuk7jG5u3V0TcfrirkCrxGdmkbkKv1o3xlS8K6_M6zov_47l614V98x135GmNsYuqX_h4eWIMsBYjbbHBOL_6mJY8njmH9wO7vtDN0Ef_k2N0fv8TdzS68Cns7c0lSL_tV6qBVeuMRc7LnByG1_8iPER6_bPu5rLjbfMkgYRZ8yesJ1vbvfPOSnO7u99_loLPa96GujwMRptdIfdUZvkm_uucv63BEKefKbK_UfJ3F54rQW792LS5aduumi_Q7Qv_U3De7az32eCdvahP4UfCaYXhuyqu8aqowXtUHlynfEqkXsDQaDW_UZAEfSn93qxvt28_YvVa_cE5UWAAA_22_7D = __webpack_require__(99339);
;// ./src/components/shared/DiffRenderer.css.ts


var additions = '_1lykch24';
var body = '_1lykch29';
var container = '_1lykch20';
var deletions = '_1lykch25';
var diffContent = '_1lykch2f';
var empty = '_1lykch2u';
var emptyHint = '_1lykch2v';
var errorRetryButton = '_1lykch2x';
var errorState = '_1lykch2w';
var file = '_1lykch2g';
var fileHeader = '_1lykch2h';
var fileStats = '_1lykch2j';
var fileTree = '_1lykch2a';
var fileTreeFilename = '_1lykch2d';
var fileTreeItem = '_1lykch2b';
var fileTreeItemActive = '_1lykch2c';
var fileTreeStats = '_1lykch2e';
var filename = '_1lykch2i';
var filesChanged = '_1lykch23';
var hunk = '_1lykch2k';
var hunkHeader = '_1lykch2l';
var line = '_1lykch2n';
var lineAdd = '_1lykch2o';
var lineContent = '_1lykch2s';
var lineContext = '_1lykch2q';
var lineDelete = '_1lykch2p';
var lineNumber = '_1lykch2r';
var lines = '_1lykch2m';
var DiffRenderer_css_loading = '_1lykch2t';
var stats = '_1lykch22';
var toolbar = '_1lykch21';
var viewModeButton = '_1lykch27';
var viewModeButtonActive = '_1lykch28';
var viewModeToggle = '_1lykch26';
;// ./src/components/shared/DiffRenderer.tsx
/* __next_internal_client_entry_do_not_use__ DiffRenderer auto */ 



/** Pure diff display component — no context coupling, no data fetching. */ function DiffRenderer(param) {
    let { content, added, removed, loading = false, error, onRefresh } = param;
    const diff = (0,parseDiff/* parseDiff */.F)(content);
    const fileRefs = (0,react.useRef)([]);
    const [activeFileIndex, setActiveFileIndex] = (0,react.useState)(null);
    const getLineClass = (type)=>{
        if (type === "add") return lineAdd;
        if (type === "delete") return lineDelete;
        return lineContext;
    };
    const jumpToFile = (fi)=>{
        var _fileRefs_current_fi;
        setActiveFileIndex(fi);
        (_fileRefs_current_fi = fileRefs.current[fi]) === null || _fileRefs_current_fi === void 0 ? void 0 : _fileRefs_current_fi.scrollIntoView({
            behavior: "smooth",
            block: "start"
        });
    };
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: DiffRenderer_css_loading,
                children: "Loading diff…"
            })
        });
    }
    if (error) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: errorState,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        children: "Failed to load changes"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: emptyHint,
                        children: error
                    }),
                    onRefresh && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: errorRetryButton,
                        onClick: onRefresh,
                        children: "Retry"
                    })
                ]
            })
        });
    }
    if (diff.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: empty,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        children: "No changes to display"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: emptyHint,
                        children: "Diff will show here when there are uncommitted changes."
                    })
                ]
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: toolbar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: stats,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: filesChanged,
                                children: [
                                    diff.length,
                                    " ",
                                    diff.length === 1 ? "file" : "files",
                                    " changed"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: additions,
                                children: [
                                    "+",
                                    added
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: deletions,
                                children: [
                                    "-",
                                    removed
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        style: {
                            display: "flex",
                            gap: "0.5rem",
                            alignItems: "center"
                        },
                        children: [
                            onRefresh && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: viewModeButton,
                                onClick: onRefresh,
                                title: "Refresh diff",
                                children: "↺"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: viewModeToggle,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: "".concat(viewModeButton, " ").concat(viewModeButtonActive),
                                        children: "Unified"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: viewModeButton,
                                        disabled: true,
                                        title: "Split view coming soon",
                                        children: "Split"
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: body,
                children: [
                    diff.length > 1 && /*#__PURE__*/ (0,jsx_runtime.jsx)("nav", {
                        className: fileTree,
                        "aria-label": "Changed files",
                        children: diff.map((diffFile, fi)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                type: "button",
                                className: "".concat(fileTreeItem, " ").concat(activeFileIndex === fi ? fileTreeItemActive : ""),
                                onClick: ()=>jumpToFile(fi),
                                title: diffFile.filename,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: fileTreeFilename,
                                        children: diffFile.filename
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: fileTreeStats,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: additions,
                                                children: [
                                                    "+",
                                                    diffFile.additions
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: deletions,
                                                children: [
                                                    "-",
                                                    diffFile.deletions
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            }, fi))
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: diffContent,
                        children: diff.map((diffFile, fi)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                ref: (el)=>{
                                    fileRefs.current[fi] = el;
                                },
                                className: file,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: fileHeader,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: filename,
                                                children: diffFile.filename
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: fileStats,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: additions,
                                                        children: [
                                                            "+",
                                                            diffFile.additions
                                                        ]
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: deletions,
                                                        children: [
                                                            "-",
                                                            diffFile.deletions
                                                        ]
                                                    })
                                                ]
                                            })
                                        ]
                                    }),
                                    diffFile.changes.map((h, hi)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: hunk,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                    className: hunkHeader,
                                                    children: [
                                                        "@@ -",
                                                        h.oldStart,
                                                        ",",
                                                        h.oldLines,
                                                        " +",
                                                        h.newStart,
                                                        ",",
                                                        h.newLines,
                                                        " @@"
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: lines,
                                                    children: h.lines.map((l, li)=>{
                                                        var _l_oldLineNumber, _l_newLineNumber;
                                                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                            className: "".concat(line, " ").concat(getLineClass(l.type)),
                                                            children: [
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                    className: lineNumber,
                                                                    children: (_l_oldLineNumber = l.oldLineNumber) !== null && _l_oldLineNumber !== void 0 ? _l_oldLineNumber : " "
                                                                }),
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                    className: lineNumber,
                                                                    children: (_l_newLineNumber = l.newLineNumber) !== null && _l_newLineNumber !== void 0 ? _l_newLineNumber : " "
                                                                }),
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                    className: lineContent,
                                                                    children: l.content
                                                                })
                                                            ]
                                                        }, li);
                                                    })
                                                })
                                            ]
                                        }, hi))
                                ]
                            }, fi))
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 78554:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 81124:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  M: () => (/* binding */ DiffViewer)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ./src/lib/contexts/SessionVcsContext.tsx
var SessionVcsContext = __webpack_require__(38590);
// EXTERNAL MODULE: ./src/components/shared/DiffRenderer.tsx + 1 modules
var DiffRenderer = __webpack_require__(78516);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
// EXTERNAL MODULE: ./src/lib/utils/datetime.ts
var datetime = __webpack_require__(3335);
// EXTERNAL MODULE: ./src/components/shared/GitHubBadge.css.ts
var GitHubBadge_css = __webpack_require__(74512);
;// ./src/components/sessions/CIStatusBadge.tsx
// +feature: session:ci-status-badge
/* __next_internal_client_entry_do_not_use__ CIStatusBadge auto */ 



/** Map a GitHub CI check conclusion to the badge's display state (AC1). */ function statusInfo(checkConclusion) {
    switch(checkConclusion){
        case "success":
            return {
                label: "Passing",
                icon: "✅",
                variant: GitHubBadge_css/* prBadgeReady */.VG
            };
        case "failure":
            return {
                label: "Failing",
                icon: "❌",
                variant: GitHubBadge_css/* prBadgeBlocking */.B2
            };
        case "pending":
            return {
                label: "Pending",
                icon: "⏳",
                variant: GitHubBadge_css/* prBadgePending */.ho
            };
        default:
            return {
                label: "No checks",
                icon: "⬤",
                variant: GitHubBadge_css/* prBadgeUnknown */.o5
            };
    }
}
/**
 * Read-only CI status badge for the diff viewer header. Purely presentational over
 * already-delivered Session data (AC3) — renders nothing when the session has no
 * associated PR (AC7).
 */ function CIStatusBadge(param) {
    let { checkConclusion, prUrl, prNumber, lastChecked } = param;
    if (!prNumber) {
        return null;
    }
    const info = statusInfo(checkConclusion);
    const href = prUrl ? "".concat(prUrl, "/checks") : undefined;
    const tooltipParts = [
        "CI status: ".concat(info.label)
    ];
    if (lastChecked) {
        tooltipParts.push("checked ".concat((0,datetime/* formatRelativeTime */.fw)((0,timestamp/* timestampDate */.FP)(lastChecked).getTime())));
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
        href: href,
        target: "_blank",
        rel: "noopener noreferrer",
        className: "".concat(GitHubBadge_css/* badge */.qS, " ").concat(GitHubBadge_css/* prBadge */.Eg, " ").concat(info.variant),
        role: "status",
        "aria-label": "CI status: ".concat(info.label),
        title: tooltipParts.join(" · "),
        "data-testid": "ci-status-badge",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: "".concat(GitHubBadge_css/* icon */.Kk),
                "aria-hidden": "true",
                children: info.icon
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: "".concat(GitHubBadge_css/* text */.Qq),
                children: info.label
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FDiffViewer.css.ts.vanilla.css%22%2C%22source%22%3A%22LmU0MzE5dDAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FDiffViewer_css_ts_vanilla_css_22_2C_22source_22_3A_22LmU0MzE5dDAgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ6KSB2YXIoLS1fMTllejJyejMwKTsKfQ_22_7D = __webpack_require__(58252);
;// ./src/components/sessions/DiffViewer.css.ts


var diffHeader = 'e4319t0';
;// ./src/components/sessions/DiffViewer.tsx
/* __next_internal_client_entry_do_not_use__ DiffViewer auto */ 




/** Session-aware diff viewer — reads from SessionVcsContext. */ function DiffViewer(param) {
    let { session } = param;
    const { diff: rawDiff, diffLoading: loading, refreshDiff } = (0,SessionVcsContext/* useSessionVcsContext */.l)();
    var _rawDiff_content, _rawDiff_added, _rawDiff_removed;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            (session === null || session === void 0 ? void 0 : session.githubPrNumber) ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: diffHeader,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(CIStatusBadge, {
                    checkConclusion: session.githubCheckConclusion,
                    prUrl: session.githubPrUrl,
                    prNumber: session.githubPrNumber,
                    lastChecked: session.lastPrStatusCheck
                })
            }) : null,
            /*#__PURE__*/ (0,jsx_runtime.jsx)(DiffRenderer/* DiffRenderer */.F, {
                content: (_rawDiff_content = rawDiff === null || rawDiff === void 0 ? void 0 : rawDiff.content) !== null && _rawDiff_content !== void 0 ? _rawDiff_content : "",
                added: (_rawDiff_added = rawDiff === null || rawDiff === void 0 ? void 0 : rawDiff.added) !== null && _rawDiff_added !== void 0 ? _rawDiff_added : 0,
                removed: (_rawDiff_removed = rawDiff === null || rawDiff === void 0 ? void 0 : rawDiff.removed) !== null && _rawDiff_removed !== void 0 ? _rawDiff_removed : 0,
                loading: loading,
                onRefresh: refreshDiff
            })
        ]
    });
}


/***/ }),

/***/ 81801:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   m: () => (/* binding */ deriveWorkingState)
/* harmony export */ });
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(81432);
/* harmony import */ var _lib_utils_assertNever__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(92418);


/**
 * Derives the effective WorkingState for a session from its SubStatus and DetectedStatus.
 *
 * This is the frontend equivalent of the removed server-side MapDetectedStatusToWorkingState.
 * SubStatus is the primary signal; DetectedStatus is a fallback for cases where SubStatus
 * is UNSPECIFIED (e.g. non-Active sessions or legacy items).
 *
 * Mapping:
 *   SubStatus.PROCESSING, NEEDS_APPROVAL, INPUT_REQUIRED, ERROR, TESTS_FAILING, RATE_LIMITED, WAITING_FOR_AGENT, COMPACTING
 *     → WorkingState.PROCESSING  (actively doing something or blocked on user)
 *   SubStatus.IDLE, READY
 *     → WorkingState.IDLE        (at prompt, ready for next instruction)
 *   SubStatus.SUCCESS
 *     → WorkingState.IDLE        (task finished, prompt available)
 *   SubStatus.UNSPECIFIED → fall through to detectedStatus
 *     DetectedStatus.EXECUTING, WAITING_FOR_AGENT, COMPACTING → WorkingState.ACTIVE
 *     DetectedStatus.PROCESSING                                → WorkingState.PROCESSING
 *     default                                                  → WorkingState.UNSPECIFIED
 */ function deriveWorkingState(session) {
    switch(session.subStatus){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.PROCESSING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.NEEDS_APPROVAL:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.INPUT_REQUIRED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.ERROR:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.TESTS_FAILING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.RATE_LIMITED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.WAITING_FOR_AGENT:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.COMPACTING:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.PROCESSING;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.IDLE:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.READY:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.SUCCESS:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.IDLE;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .SubStatus */ .BJ.UNSPECIFIED:
            break;
    }
    // detectedStatus-based fallback (used when subStatus is UNSPECIFIED)
    switch(session.detectedStatus){
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.EXECUTING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.WAITING_FOR_AGENT:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.COMPACTING:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.ACTIVE;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.PROCESSING:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.NEEDS_APPROVAL:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.INPUT_REQUIRED:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.ERROR:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.TESTS_FAILING:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.PROCESSING;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.IDLE:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.READY:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.IDLE;
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.SUCCESS:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNKNOWN:
        case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .DetectedStatus */ .nB.UNSPECIFIED:
        case undefined:
            return _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_0__/* .WorkingState */ .RA.UNSPECIFIED;
        default:
            return (0,_lib_utils_assertNever__WEBPACK_IMPORTED_MODULE_1__/* .assertNever */ .x)(session.detectedStatus);
    }
}


/***/ }),

/***/ 81921:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ay: () => (__WEBPACK_DEFAULT_EXPORT__),
/* harmony export */   lj: () => (/* binding */ selectRemoteHealthEntry),
/* harmony export */   ng: () => (/* binding */ remoteHealthChanged)
/* harmony export */ });
/* unused harmony export selectRemoteConnectionState */
/* harmony import */ var _reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(73559);
/* harmony import */ var _gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88149);


const initialState = {
    byName: {}
};
const remotesSlice = (0,_reduxjs_toolkit__WEBPACK_IMPORTED_MODULE_1__/* .createSlice */ .Z0)({
    name: "remotes",
    initialState,
    reducers: {
        remoteHealthChanged (state, action) {
            const { remoteName, state: connState, previousState } = action.payload;
            if (!remoteName) return;
            state.byName[remoteName] = {
                state: connState,
                previousState,
                updatedAt: Date.now()
            };
        }
    }
});
const { remoteHealthChanged } = remotesSlice.actions;
// selectRemoteConnectionState returns a selector for a single remote's
// current connection state, defaulting to UNSPECIFIED (never RemoteConnectionState's
// CONNECTED) when no health-change event has been observed yet -- consumers
// (RemoteConnectionIndicator) must treat UNSPECIFIED as "unknown," not as a
// synonym for disconnected or connected.
const selectRemoteConnectionState = (remoteName)=>(state)=>{
        var _state_remotes_byName_remoteName;
        var _state_remotes_byName_remoteName_state;
        return (_state_remotes_byName_remoteName_state = (_state_remotes_byName_remoteName = state.remotes.byName[remoteName]) === null || _state_remotes_byName_remoteName === void 0 ? void 0 : _state_remotes_byName_remoteName.state) !== null && _state_remotes_byName_remoteName_state !== void 0 ? _state_remotes_byName_remoteName_state : RemoteConnectionState.UNSPECIFIED;
    };
// selectRemoteHealthEntry exposes the full stored entry (state, previousState,
// updatedAt). RemoteConnectionIndicator currently only reads .state from this
// (it tracks its own prior-state via a local ref to decide polite vs. assertive
// announcements, not this entry's previousState) -- exposed as the full entry
// rather than just .state for any future consumer that needs updatedAt/previousState
// directly from the store instead of re-deriving it.
const selectRemoteHealthEntry = (remoteName)=>(state)=>state.remotes.byName[remoteName];
/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = (remotesSlice.reducer);


/***/ }),

/***/ 85847:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 56751));


/***/ }),

/***/ 90062:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   ZX: () => (/* binding */ createSessionWatchTransport)
/* harmony export */ });
/* unused harmony exports fromWebSocket, createWatchTransport */
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(16294);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(13399);
/* harmony import */ var _connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(46564);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(70050);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(69522);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(9103);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(48210);
/* harmony import */ var _connectrpc_connect_protocol_connect__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(60713);
/* provided dependency */ var process = __webpack_require__(36843);
/* __next_internal_client_entry_do_not_use__ fromWebSocket,createWatchTransport,createSessionWatchTransport auto */ 



function encodeEnvelope(flags, data) {
    const buf = new Uint8Array(5 + data.length);
    buf[0] = flags;
    new DataView(buf.buffer).setUint32(1, data.length, false);
    buf.set(data, 5);
    return buf;
}
async function* fromWebSocket(ws, signal) {
    const queue = [];
    let notify = null;
    const push = (item)=>{
        queue.push(item);
        notify === null || notify === void 0 ? void 0 : notify();
        notify = null;
    };
    ws.onmessage = (e)=>push(new Uint8Array(e.data));
    ws.onerror = ()=>push(new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("WebSocket error", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unavailable));
    ws.onclose = (ev)=>{
        if ((signal === null || signal === void 0 ? void 0 : signal.aborted) || ev.code === 1000) {
            push(null); // intentional close (1000=normal) or signal abort
        } else {
            // Code 1001 (Going Away / server restart) sets wasClean=true via TCP FIN but IS retriable.
            // Only 1000 and explicit signal abort are treated as non-retriable clean closes.
            push(new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("WebSocket closed", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unavailable, new Headers({
                "ws-close-code": String(ev.code)
            })));
        }
    };
    const abortHandler = ()=>{
        ws.close();
        push(null);
    };
    signal === null || signal === void 0 ? void 0 : signal.addEventListener("abort", abortHandler);
    try {
        while(true){
            while(queue.length === 0){
                await new Promise((r)=>{
                    notify = r;
                });
            }
            const item = queue.shift();
            if (item === null) return;
            if (item instanceof Error) throw item;
            yield item;
        }
    } finally{
        signal === null || signal === void 0 ? void 0 : signal.removeEventListener("abort", abortHandler);
    }
}
/**
 * Creates a ConnectRPC transport that uses standard HTTP fetch for unary calls
 * and WebSocket for server-streaming calls. Compatible with StreamingWSBridge on
 * the server (server/services/ws_stream_bridge.go).
 *
 * Protocol (streaming):
 *   Client → first binary WS message: Connect request envelope (5-byte header + proto)
 *   Server → N binary WS messages: Connect response envelopes
 *   Server → final binary WS message: Connect end-stream envelope
 *
 * Auth: cookie-based (browser sends cookies automatically on WS upgrade).
 */ function createWatchTransport(opt) {
    const httpTransport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)(opt);
    return {
        unary: httpTransport.unary.bind(httpTransport),
        async stream (method, signal, timeoutMs, header, input, contextValues) {
            var _opt_useBinaryFormat;
            const useBinaryFormat = (_opt_useBinaryFormat = opt.useBinaryFormat) !== null && _opt_useBinaryFormat !== void 0 ? _opt_useBinaryFormat : true;
            const { serialize, parse } = (0,_connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_3__/* .createClientMethodSerializers */ .Ef)(method, useBinaryFormat, opt.jsonOptions, opt.binaryOptions);
            const methodUrl = (0,_connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_4__/* .createMethodUrl */ .j)(opt.baseUrl, method);
            return (0,_connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_5__/* .runStreamingCall */ .u)({
                interceptors: opt.interceptors,
                timeoutMs,
                signal,
                req: {
                    stream: true,
                    service: method.parent,
                    method,
                    url: methodUrl,
                    requestMethod: "POST",
                    header: new Headers(header instanceof Headers ? header : undefined),
                    contextValues: contextValues !== null && contextValues !== void 0 ? contextValues : (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createContextValues */ .k)(),
                    message: input
                },
                next: async (req)=>{
                    const wsUrl = req.url.replace(/^http/, "ws");
                    const ws = new WebSocket(wsUrl);
                    ws.binaryType = "arraybuffer";
                    await new Promise((resolve, reject)=>{
                        ws.onopen = ()=>resolve();
                        ws.onerror = ()=>reject(new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("WebSocket connection failed", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unavailable));
                    });
                    // Read the first (and only) request message and send it as a Connect envelope.
                    const iter = req.message[Symbol.asyncIterator]();
                    const first = await iter.next();
                    if (first.done) {
                        ws.close();
                        throw new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("missing request message", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Internal);
                    }
                    ws.send(encodeEnvelope(0, serialize(first.value)));
                    const trailer = new Headers();
                    async function* parseResponses() {
                        let buf = new Uint8Array(0);
                        let endStreamReceived = false;
                        for await (const chunk of fromWebSocket(ws, signal)){
                            // Accumulate chunk into buffer
                            const merged = new Uint8Array(buf.length + chunk.length);
                            merged.set(buf);
                            merged.set(chunk, buf.length);
                            buf = merged;
                            // Parse all complete envelopes
                            while(buf.length >= 5){
                                const flags = buf[0];
                                const msgLen = new DataView(buf.buffer, buf.byteOffset).getUint32(1, false);
                                if (buf.length < 5 + msgLen) break;
                                const data = buf.slice(5, 5 + msgLen);
                                buf = buf.slice(5 + msgLen);
                                if (flags & _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_7__/* .compressedFlag */ .y) {
                                    throw new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("unexpected compressed response", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Internal);
                                }
                                if (flags & _connectrpc_connect_protocol_connect__WEBPACK_IMPORTED_MODULE_8__/* .endStreamFlag */ .Cu) {
                                    endStreamReceived = true;
                                    const end = (0,_connectrpc_connect_protocol_connect__WEBPACK_IMPORTED_MODULE_8__/* .endStreamFromJson */ .mo)(data);
                                    if (end.error) throw end.error;
                                    end.metadata.forEach((v, k)=>trailer.set(k, v));
                                } else {
                                    yield parse(data);
                                }
                            }
                        }
                        if (!endStreamReceived) {
                            throw new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("stream ended without end-stream message", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Internal);
                        }
                    }
                    return {
                        ...req,
                        header: new Headers({
                            "Content-Type": "application/connect+proto"
                        }),
                        trailer,
                        message: parseResponses()
                    };
                }
            });
        }
    };
}
/**
 * Selects the transport for any server-streaming Watch* RPC (SessionService,
 * ReviewQueueService, BacklogService, InsightsService,
 * UnfinishedWorkService, ...) whose path is registered with server.go's
 * StreamingWSBridge.
 *
 * Native ConnectRPC streaming only helps over a real HTTP/2 connection —
 * that's the TLS remote-access listener (:8444) only. No shipping browser
 * supports cleartext HTTP/2, so on the plain :8543 dev listener native
 * streaming would silently stay on HTTP/1.1 and hold a long-lived fetch()
 * open, reintroducing the exact browser connection-slot problem
 * StreamingWSBridge exists to avoid. Gate on baseUrl, not just the flag.
 */ function createSessionWatchTransport(opt) {
    const nativeStreamingEnabled = process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING === "true";
    if (nativeStreamingEnabled && opt.baseUrl.startsWith("https://")) {
        return (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)(opt);
    }
    return createWatchTransport(opt);
}


/***/ }),

/***/ 90814:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ usePageView)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_nav__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(41078);
/* harmony import */ var _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(24044);
/* __next_internal_client_entry_do_not_use__ usePageView auto */ 


function usePageView() {
    const pathname = (0,_lib_nav__WEBPACK_IMPORTED_MODULE_1__/* .usePathname */ .a8)();
    const { track } = (0,_lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_2__/* .useAnalytics */ .st)();
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        track({
            name: "page_view",
            category: "navigation",
            page: pathname
        });
    }, [
        pathname,
        track
    ]);
}


/***/ }),

/***/ 92418:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   x: () => (/* binding */ assertNever)
/* harmony export */ });
function assertNever(x) {
    throw new Error("Unhandled case: ".concat(String(x)));
}


/***/ }),

/***/ 93602:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   GV: () => (/* reexport safe */ _store__WEBPACK_IMPORTED_MODULE_0__.GV),
/* harmony export */   M_: () => (/* reexport safe */ _store__WEBPACK_IMPORTED_MODULE_0__.M_),
/* harmony export */   jL: () => (/* reexport safe */ _store__WEBPACK_IMPORTED_MODULE_0__.jL)
/* harmony export */ });
/* harmony import */ var _store__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(25337);



/***/ }),

/***/ 99339:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 99863:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Link: () => (/* binding */ Link),
  Redirect: () => (/* binding */ Redirect),
  pushHistoryMarker: () => (/* binding */ pushHistoryMarker),
  pushTrayEntry: () => (/* binding */ pushTrayEntry),
  replaceHash: () => (/* binding */ replaceHash),
  usePathname: () => (/* binding */ usePathname),
  useReplaceSearch: () => (/* binding */ useReplaceSearch),
  useRouter: () => (/* binding */ useRouter),
  useSearchParams: () => (/* binding */ useSearchParams)
});

// UNUSED EXPORTS: nextImpl

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/client/app-dir/link.js
var app_dir_link = __webpack_require__(6024);
var link_default = /*#__PURE__*/__webpack_require__.n(app_dir_link);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/api/navigation.js
var navigation = __webpack_require__(45853);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
;// ./src/lib/nav/history.ts
// The only place outside an impl that touches raw History; every impl shares it so foreign state survives the same way.
const TRAY_MARKER = "ssqTray";
function replaceSearchInHistory(search) {
    const query = search === "" || search.startsWith("?") ? search : "?".concat(search);
    window.history.replaceState(window.history.state, "", query === "" ? window.location.pathname : query);
}
function replaceHashInHistory(hash) {
    const url = hash ? "".concat(window.location.pathname).concat(window.location.search).concat(hash.startsWith("#") ? hash : "#".concat(hash)) : window.location.pathname + window.location.search;
    window.history.replaceState(window.history.state, "", url);
}
function pushHistoryMarkerInHistory(key, value) {
    let replace = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false;
    var _window_history_state;
    const state = {
        ...(_window_history_state = window.history.state) !== null && _window_history_state !== void 0 ? _window_history_state : {},
        [key]: value
    };
    if (replace) window.history.replaceState(state, "");
    else window.history.pushState(state, "");
}
function pushTrayEntryInHistory() {
    let replace = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false;
    pushHistoryMarkerInHistory(TRAY_MARKER, true, replace);
}

;// ./src/lib/nav/normalize.ts
/** Strips trailing slashes (Next's `trailingSlash` export reports `/x/`) but keeps the root `/`. */ function normalizePathname(pathname) {
    const trimmed = pathname.replace(/\/+$/, "");
    return trimmed === "" ? "/" : trimmed;
}

;// ./src/lib/nav/impl.next.tsx
/* __next_internal_client_entry_do_not_use__ useRouter,useSearchParams,usePathname,Link,Redirect,useReplaceSearch,replaceHash,pushTrayEntry,pushHistoryMarker,nextImpl auto */ 





const toNextOptions = (options)=>(options === null || options === void 0 ? void 0 : options.resetScroll) === false ? {
        scroll: false
    } : undefined;
function useRouter() {
    const router = (0,navigation.useRouter)();
    return (0,react.useMemo)(()=>({
            push: (href, options)=>router.push(href, toNextOptions(options)),
            replace: (href, options)=>router.replace(href, toNextOptions(options)),
            back: ()=>router.back(),
            forward: ()=>router.forward()
        }), [
        router
    ]);
}
function useSearchParams() {
    return (0,navigation.useSearchParams)();
}
function usePathname() {
    var _useNextPathname;
    return normalizePathname((_useNextPathname = (0,navigation.usePathname)()) !== null && _useNextPathname !== void 0 ? _useNextPathname : "/");
}
/** Prefetch stays off: it triggers CSS preload warnings (the reason the NavLink this replaces existed). */ const Link = /*#__PURE__*/ (0,react.forwardRef)(function Link(param, ref) {
    let { resetScroll, ...props } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)((link_default()), {
        ref: ref,
        prefetch: false,
        scroll: resetScroll === false ? false : undefined,
        ...props
    });
});
/** Next's render-time `redirect()`: the behaviour the two redirect-only pages have today. */ function Redirect(param) {
    let { to } = param;
    (0,navigation.redirect)(to);
}
function useReplaceSearch() {
    return (0,react.useCallback)((search)=>replaceSearchInHistory(search), []);
}
const replaceHash = replaceHashInHistory;
const pushTrayEntry = pushTrayEntryInHistory;
const pushHistoryMarker = pushHistoryMarkerInHistory;
const nextImpl = {
    useRouter,
    useSearchParams,
    usePathname,
    Link,
    Redirect,
    useReplaceSearch,
    replaceHash,
    pushTrayEntry,
    pushHistoryMarker
};


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,7924,8165,3575,9691,3516,7358], () => (__webpack_exec__(85847)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-5774ead24b74e800.js.map