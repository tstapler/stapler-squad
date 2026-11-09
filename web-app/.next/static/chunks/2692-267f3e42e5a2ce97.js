"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[2692],{

/***/ 42692:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  r: () => (/* binding */ LogViewer)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
;// ./src/lib/hooks/useLiveTail.ts
/* __next_internal_client_entry_do_not_use__ useLiveTail,default auto */ 
/**
 * Hook for live tailing logs with auto-refresh
 *
 * @param fetchLogs - Function to fetch new logs
 * @param options - Live tail configuration options
 * @returns Live tail state and controls
 */ function useLiveTail(fetchLogs, options) {
    const { interval = 2000, enabled, onNewLogs } = options;
    const [isActive, setIsActive] = (0,react.useState)(false);
    const [isPaused, setIsPaused] = (0,react.useState)(false);
    const [newLogCount, setNewLogCount] = (0,react.useState)(0);
    const [lastFetch, setLastFetch] = (0,react.useState)(null);
    const [error, setError] = (0,react.useState)(null);
    const intervalRef = (0,react.useRef)(null);
    const fetchLogsRef = (0,react.useRef)(fetchLogs);
    const onNewLogsRef = (0,react.useRef)(onNewLogs);
    // Update refs when callbacks change
    (0,react.useEffect)(()=>{
        fetchLogsRef.current = fetchLogs;
    }, [
        fetchLogs
    ]);
    (0,react.useEffect)(()=>{
        onNewLogsRef.current = onNewLogs;
    }, [
        onNewLogs
    ]);
    // Fetch function that tracks state
    const doFetch = (0,react.useCallback)(async ()=>{
        try {
            setError(null);
            await fetchLogsRef.current();
            setLastFetch(new Date());
        } catch (err) {
            setError(err instanceof Error ? err.message : 'Failed to fetch logs');
        }
    }, []);
    // Start live tailing
    const start = (0,react.useCallback)(()=>{
        if (intervalRef.current) {
            clearInterval(intervalRef.current);
        }
        setIsActive(true);
        setIsPaused(false);
        setError(null);
        // Immediate fetch
        doFetch();
        // Start interval
        intervalRef.current = setInterval(()=>{
            if (!isPaused) {
                doFetch();
            }
        }, interval);
    }, [
        interval,
        isPaused,
        doFetch
    ]);
    // Stop live tailing
    const stop = (0,react.useCallback)(()=>{
        if (intervalRef.current) {
            clearInterval(intervalRef.current);
            intervalRef.current = null;
        }
        setIsActive(false);
        setIsPaused(false);
    }, []);
    // Pause live tailing
    const pause = (0,react.useCallback)(()=>{
        setIsPaused(true);
    }, []);
    // Resume live tailing
    const resume = (0,react.useCallback)(()=>{
        setIsPaused(false);
        // Immediate fetch on resume
        doFetch();
    }, [
        doFetch
    ]);
    // Toggle pause/resume
    const toggle = (0,react.useCallback)(()=>{
        if (isPaused) {
            resume();
        } else {
            pause();
        }
    }, [
        isPaused,
        pause,
        resume
    ]);
    // Clear new log count
    const clearNewLogCount = (0,react.useCallback)(()=>{
        setNewLogCount(0);
    }, []);
    // Handle enabled state changes
    (0,react.useEffect)(()=>{
        if (enabled && !isActive) {
            start();
        } else if (!enabled && isActive) {
            stop();
        }
    }, [
        enabled,
        isActive,
        start,
        stop
    ]);
    // Cleanup on unmount
    (0,react.useEffect)(()=>{
        return ()=>{
            if (intervalRef.current) {
                clearInterval(intervalRef.current);
            }
        };
    }, []);
    // Update interval when it changes
    (0,react.useEffect)(()=>{
        if (isActive && !isPaused && intervalRef.current) {
            clearInterval(intervalRef.current);
            intervalRef.current = setInterval(doFetch, interval);
        }
    }, [
        interval,
        isActive,
        isPaused,
        doFetch
    ]);
    const state = {
        isActive,
        isPaused,
        newLogCount,
        lastFetch,
        error
    };
    const controls = {
        start,
        stop,
        pause,
        resume,
        toggle,
        clearNewLogCount
    };
    return [
        state,
        controls
    ];
}
/* harmony default export */ const hooks_useLiveTail = ((/* unused pure expression or super */ null && (useLiveTail)));

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/ansi-to-html@0.7.2/node_modules/ansi-to-html/lib/ansi_to_html.js
var ansi_to_html = __webpack_require__(64957);
var ansi_to_html_default = /*#__PURE__*/__webpack_require__.n(ansi_to_html);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dompurify@3.4.3/node_modules/dompurify/dist/purify.es.mjs
var purify_es = __webpack_require__(40260);
;// ./src/lib/logs/logParser.ts
// logParser.ts — Epic 3 implementation: level detection, search segmentation,
// ANSI pipeline, and JSON detection.


// ---------------------------------------------------------------------------
// T1: Level detection
// ---------------------------------------------------------------------------
// Precompile at module level for performance (not inside the function)
const LEVEL_PATTERNS = [
    [
        "ERROR",
        /\b(ERROR|ERR)\b/i
    ],
    [
        "WARN",
        /\bWARN(?:ING)?\b/i
    ],
    [
        "INFO",
        /\bINFO\b/i
    ],
    [
        "DEBUG",
        /\bDEBUG\b/i
    ],
    [
        "TRACE",
        /\bTRACE\b/i
    ]
];
function detectLevel(line) {
    for (const [level, pattern] of LEVEL_PATTERNS){
        if (pattern.test(line)) return level;
    }
    return "UNKNOWN";
}
// ---------------------------------------------------------------------------
// T2: Search highlight segmentation
// ---------------------------------------------------------------------------
function segmentText(text, query) {
    if (!query) return [
        {
            text,
            highlight: false
        }
    ];
    const lowerText = text.toLowerCase();
    const lowerQuery = query.toLowerCase();
    const segments = [];
    let pos = 0;
    let idx;
    while((idx = lowerText.indexOf(lowerQuery, pos)) !== -1){
        if (idx > pos) segments.push({
            text: text.slice(pos, idx),
            highlight: false
        });
        segments.push({
            text: text.slice(idx, idx + query.length),
            highlight: true
        });
        pos = idx + query.length;
    }
    if (pos < text.length) segments.push({
        text: text.slice(pos),
        highlight: false
    });
    return segments.length > 0 ? segments : [
        {
            text,
            highlight: false
        }
    ];
}
// ---------------------------------------------------------------------------
// T3: ANSI pipeline
// ---------------------------------------------------------------------------
const converter = new (ansi_to_html_default())({
    escapeXML: true
});
// Strip OSC sequences (OSC 8 hyperlinks etc.) before processing
const OSC_REGEX = /\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;
function renderAnsi(raw) {
    const stripped = raw.replace(OSC_REGEX, "");
    const html = converter.toHtml(stripped);
    // DOMPurify only runs in browser; in SSR/test context, return raw html
    if (false) {}
    return DOMPurify.sanitize(html, {
        ALLOWED_TAGS: [
            "span"
        ],
        ALLOWED_ATTR: [
            "style"
        ]
    });
}
// ---------------------------------------------------------------------------
// T4: JSON detection with bounded LRU-style cache
// ---------------------------------------------------------------------------
const jsonCache = new Map();
function tryParseJson(text) {
    const trimmed = text.trim();
    if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return null;
    if (jsonCache.has(trimmed)) return jsonCache.get(trimmed);
    try {
        const parsed = JSON.parse(trimmed);
        if (jsonCache.size >= 500) {
            const firstKey = jsonCache.keys().next().value;
            if (firstKey !== undefined) jsonCache.delete(firstKey);
        }
        jsonCache.set(trimmed, typeof parsed === "object" ? parsed : null);
        return typeof parsed === "object" ? parsed : null;
    } catch (e) {
        jsonCache.set(trimmed, null);
        return null;
    }
}

;// ./src/lib/hooks/useLogViewer.ts
/* __next_internal_client_entry_do_not_use__ useLogViewer auto */ 






function mapLevel(raw) {
    const upper = raw.toUpperCase();
    if (upper === "ERROR" || upper === "ERR") return "ERROR";
    if (upper === "WARN" || upper === "WARNING") return "WARN";
    if (upper === "INFO") return "INFO";
    if (upper === "DEBUG") return "DEBUG";
    if (upper === "TRACE") return "TRACE";
    return detectLevel(raw);
}
let entryCounter = 0;
function makeId() {
    return "log-".concat(++entryCounter);
}
function useLogViewer(source, sessionId, options) {
    var _options_limit;
    const fetchLimit = (_options_limit = options === null || options === void 0 ? void 0 : options.limit) !== null && _options_limit !== void 0 ? _options_limit : 200;
    const timeRange = options === null || options === void 0 ? void 0 : options.timeRange;
    // Time range as primitives so it can sit in a dependency array without a
    // new-object-every-render identity problem (callers often build the Date
    // objects inline).
    const timeRangeStartMs = timeRange === null || timeRange === void 0 ? void 0 : timeRange.start.getTime();
    const timeRangeEndMs = timeRange === null || timeRange === void 0 ? void 0 : timeRange.end.getTime();
    // --- Core log storage: mutable ref + version counter for O(1) appending ---
    // Each display entry is paired with its raw proto counterpart in one
    // record, so filtering can never desync the two the way two independently
    // maintained parallel arrays could.
    const entriesRef = (0,react.useRef)([]);
    const [version, setVersion] = (0,react.useState)(0);
    const [serverTotalCount, setServerTotalCount] = (0,react.useState)(0);
    const [lastRefresh, setLastRefresh] = (0,react.useState)(null);
    const [error, setError] = (0,react.useState)(null);
    // --- Search and filter ---
    const [searchQuery, setSearchQuery] = (0,react.useState)("");
    const [levelFilters, setLevelFilters] = (0,react.useState)([]);
    // --- Expansion ---
    const [expandedRowIndex, setExpandedRowIndex] = (0,react.useState)(null);
    // --- Selection (keyboard row navigation) ---
    const [selectedRowIndex, setSelectedRowIndex] = (0,react.useState)(null);
    // --- Live-tail pause state machine ---
    const [isFollowing, setIsFollowing] = (0,react.useState)(true);
    const [queuedNewLineCount, setQueuedNewLineCount] = (0,react.useState)(0);
    // --- Live-tail toggle ---
    const [liveTailEnabled, setLiveTailEnabled] = (0,react.useState)(true);
    // --- Virtuoso ref for programmatic scroll ---
    const virtuosoRef = (0,react.useRef)(null);
    // --- ConnectRPC client ---
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    // --- Live-tail cursor: tracks server-side total_count so polling can detect new entries ---
    // GetLogs returns entries newest-first (DESC). Using offset:0 always yields the most-recent
    // entries. We compare total_count to knownTotalRef to find how many new entries arrived.
    const knownTotalRef = (0,react.useRef)(0);
    // --- Initial fetch ---
    const fetchInitialLogs = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        try {
            const response = await clientRef.current.getLogs({
                limit: fetchLimit,
                offset: 0,
                sessionId: source === "session" ? sessionId : undefined,
                startTime: timeRangeStartMs !== undefined ? (0,timestamp/* timestampFromDate */.LI)(new Date(timeRangeStartMs)) : undefined,
                endTime: timeRangeEndMs !== undefined ? (0,timestamp/* timestampFromDate */.LI)(new Date(timeRangeEndMs)) : undefined
            });
            var _response_entries;
            const protoEntries = (_response_entries = response.entries) !== null && _response_entries !== void 0 ? _response_entries : [];
            const entries = protoEntries.map((e)=>({
                    display: {
                        id: makeId(),
                        timestamp: e.timestamp ? new Date(Number(e.timestamp.seconds) * 1000).toISOString() : new Date().toISOString(),
                        level: mapLevel(e.level),
                        message: e.message,
                        raw: e.message
                    },
                    raw: e
                }));
            var _response_totalCount;
            // Seed knownTotalRef from the initial fetch so live-tail polling knows
            // how many entries the server already had.
            knownTotalRef.current = (_response_totalCount = response.totalCount) !== null && _response_totalCount !== void 0 ? _response_totalCount : entries.length;
            entriesRef.current = entries;
            var _response_totalCount1;
            setServerTotalCount((_response_totalCount1 = response.totalCount) !== null && _response_totalCount1 !== void 0 ? _response_totalCount1 : entries.length);
            setLastRefresh(new Date());
            setError(null);
            setVersion((v)=>v + 1);
        } catch (err) {
            setError(err instanceof Error ? err.message : "Failed to load logs");
        }
    }, [
        source,
        sessionId,
        fetchLimit,
        timeRangeStartMs,
        timeRangeEndMs
    ]);
    (0,react.useEffect)(()=>{
        void fetchInitialLogs();
    }, [
        fetchInitialLogs
    ]);
    // --- Live-tail polling: fetch incremental logs ---
    const isFollowingRef = (0,react.useRef)(isFollowing);
    (0,react.useEffect)(()=>{
        isFollowingRef.current = isFollowing;
    }, [
        isFollowing
    ]);
    const fetchNewLogs = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        // A caller-supplied time range means "browse this historical window",
        // not "follow the tail" — polling for entries newer than knownTotalRef
        // would pull in current-time entries outside that window. The live-tail
        // toggle stays available but is a no-op until the range is cleared.
        if (timeRangeStartMs !== undefined || timeRangeEndMs !== undefined) return;
        try {
            const knownTotal = knownTotalRef.current;
            // Pass multi-level filter via repeated levels field; fall back to no filter for "ALL"
            const activeLevels = levelFilters.filter((l)=>l !== "ALL");
            // Request from offset:0 (newest-first) with a limit that covers any newly
            // arrived entries since the last poll. Cap at 200 to bound payload size.
            const limit = Math.min(Math.max(100, 200), 200);
            const response = await clientRef.current.getLogs({
                limit,
                offset: 0,
                sessionId: source === "session" ? sessionId : undefined,
                levels: activeLevels.length > 0 ? activeLevels : undefined
            });
            var _response_totalCount;
            const serverTotal = (_response_totalCount = response.totalCount) !== null && _response_totalCount !== void 0 ? _response_totalCount : 0;
            const newCount = serverTotal - knownTotal;
            setServerTotalCount(serverTotal);
            setLastRefresh(new Date());
            setError(null);
            if (newCount <= 0) return;
            var _response_entries;
            // The first `newCount` entries in the newest-first response are the ones
            // we haven't seen yet. Slice them off and reverse so they're oldest-first
            // for appending at the end of entriesRef.current.
            const freshProtoSlice = ((_response_entries = response.entries) !== null && _response_entries !== void 0 ? _response_entries : []).slice(0, newCount).reverse();
            const newEntries = freshProtoSlice.map((e)=>({
                    display: {
                        id: makeId(),
                        timestamp: e.timestamp ? new Date(Number(e.timestamp.seconds) * 1000).toISOString() : new Date().toISOString(),
                        level: mapLevel(e.level),
                        message: e.message,
                        raw: e.message
                    },
                    raw: e
                }));
            if (newEntries.length === 0) return;
            knownTotalRef.current = serverTotal;
            entriesRef.current = [
                ...entriesRef.current,
                ...newEntries
            ];
            (0,react.startTransition)(()=>{
                setVersion((v)=>v + 1);
                if (!isFollowingRef.current) {
                    setQueuedNewLineCount((prev)=>prev + newEntries.length);
                }
            });
        } catch (err) {
            setError(err instanceof Error ? err.message : "Failed to refresh logs");
        }
    }, [
        source,
        sessionId,
        levelFilters,
        timeRangeStartMs,
        timeRangeEndMs
    ]);
    const [_liveTailState, _liveTailControls] = useLiveTail(fetchNewLogs, {
        enabled: liveTailEnabled,
        interval: 2000
    });
    // --- At-bottom state change: drive follow/pause state machine ---
    const onAtBottomStateChange = (0,react.useCallback)((atBottom)=>{
        if (atBottom) {
            setIsFollowing(true);
            setQueuedNewLineCount(0);
        } else {
            setIsFollowing(false);
        }
    }, []);
    // --- Jump to latest ---
    const jumpToLatest = (0,react.useCallback)(()=>{
        var _virtuosoRef_current;
        setIsFollowing(true);
        setQueuedNewLineCount(0);
        (_virtuosoRef_current = virtuosoRef.current) === null || _virtuosoRef_current === void 0 ? void 0 : _virtuosoRef_current.scrollToIndex({
            index: "LAST",
            behavior: "smooth"
        });
    }, []);
    // --- Row toggle (accordion) ---
    const toggleRow = (0,react.useCallback)((i)=>{
        setExpandedRowIndex((prev)=>prev === i ? null : i);
    }, []);
    // --- Derived: filtered entries (display + raw stay paired since they're
    // filtered together as one record, not as two independently-indexed arrays) ---
    const matchesFilter = (0,react.useCallback)((e)=>{
        if (searchQuery && !e.message.toLowerCase().includes(searchQuery.toLowerCase())) return false;
        if (levelFilters.length > 0 && !levelFilters.includes("ALL") && !levelFilters.includes(e.level)) return false;
        return true;
    }, [
        searchQuery,
        levelFilters
    ]);
    const filteredEntries = (0,react.useMemo)(()=>{
        // version is the cache key — reading entriesRef.current here is intentional
        // eslint-disable-next-line @typescript-eslint/no-unused-expressions
        version;
        return entriesRef.current.filter((e)=>matchesFilter(e.display));
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        version,
        matchesFilter
    ]);
    const filteredLogs = (0,react.useMemo)(()=>filteredEntries.map((e)=>e.display), [
        filteredEntries
    ]);
    const filteredRawEntries = (0,react.useMemo)(()=>filteredEntries.map((e)=>e.raw), [
        filteredEntries
    ]);
    const refresh = (0,react.useCallback)(()=>{
        void fetchInitialLogs();
    }, [
        fetchInitialLogs
    ]);
    return {
        logs: filteredLogs,
        isFollowing,
        searchQuery,
        setSearchQuery,
        levelFilters,
        setLevelFilters,
        matchCount: searchQuery ? filteredLogs.length : 0,
        totalCount: entriesRef.current.length,
        toggleRow,
        expandedRowIndex,
        selectedRowIndex,
        setSelectedRowIndex,
        jumpToLatest,
        queuedNewLineCount,
        onAtBottomStateChange,
        virtuosoRef,
        liveTailEnabled,
        setLiveTailEnabled,
        rawEntries: filteredRawEntries,
        serverTotalCount,
        lastRefresh,
        error,
        refresh
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-virtuoso@4.18.7+498059a1009c1789/node_modules/react-virtuoso/dist/index.mjs
var dist = __webpack_require__(58362);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FLogRow.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61W226jMBB9z1f4sZFwhCEkxH3ZP6kMOODG2KwxDc2q_76yDawJbS7SSkkU7JkztzMzbN5QzmKm9iH4swKgYG3DyScGTHAmKMy4zE-vKwAaUhRMlBigpge7pjdnmVQFVVCRgnUtBrE7PUqhYcsuFAOEvKMzZWWlMdiH4XR2JDXjnxjUUsi2ITk1N5xqTRU0z9ZiuAkTWpubmgl4ZoWuMNhuHbSmvYaEs1JgkFOhqXpdfa02Y1TIRpWR_FQq2YkCgw-iXiB8Qwd6idQFibVBySWXanHXrmdY0T0seQNLz7Hie1jN435t72H9fhwruYelHsfaLbC0IqJtiKJCeyhMVFQxPdPdW91GtkwzKTBQlBPNPiw_Jo4eObUcsOWHTNO6dYew1URpj6KZ1FrWjryt5KxY3MHBGVVm5CVKkmD8hptw52LuVGskGskczRwfq4HWUToQUnZ5BUnu_K6JYE1nfJdiFmB6lZyZfZQmQZSaT7gJ03lWD7cUI7QPEDoEu9FtT5H8oDgriiefPSmfPylfPClPrbzstBlMXild5MkhQHEYRFsTemILNohCeTy2VGMA7TTyEMsrkpGslbzTwxA6agxCV9Fm-DfSyD4Mgygd6n6BTBS0xwA50UVc82ZxnLpN5XGeAVCSBoNh4k2TOPo3ibvWDEzKaa4xEFLYEFwnVIqJ0xTJQ9ysbF6MOgbIZvKDqiOXZ9hjQDotPTegS9SYhXPFNLWjmxpHzoo0t4e9vy0ih2GLNrYV2iRLzxsiYG9_P80lPNPsxDSc3GxzJTm3SbKKs-iYjW7WbskuQLs4QHvT7vv1tV-h88vbPlOze9tHGYdnlt7vWHI0rYkqmYDKxZv-sEQ92NN8U4_l_u92-M1Jk8RBFG2DPQrCzdZfCtM4X7wgRM7Ar5oWjICXmvRjPuM0bPq1tXfVnVOfjQv_y5epBpk5Gb8VZYPoIm9f37u0jZcuvf-M4UmdrqXcu9Rg6y8rdAv3cQkAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FLogRow_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61W226jMBB9z1f4sZFwhCEkxH3ZP6kMOODG2KwxDc2q_76yDawJbS7SSkkU7JkztzMzbN5QzmKm9iH4swKgYG3DyScGTHAmKMy4zE_vKwAaUhRMlBigpge7pjdnmVQFVVCRgnUtBrE7PUqhYcsuFAOEvKMzZWWlMdiH4XR2JDXjnxjUUsi2ITk1N5xqTRU0z9ZiuAkTWpubmgl4ZoWuMNhuHbSmvYaEs1JgkFOhqXpdfa02Y1TIRpWR_FQq2YkCgw_iXiB8Qwd6idQFibVBySWXanHXrmdY0T0seQNLz7Hie1jN435t72H9fhwruYelHsfaLbC0IqJtiKJCeyhMVFQxPdPdW91GtkwzKTBQlBPNPiw_Jo4eObUcsOWHTNO6dYew1URpj6KZ1FrWjryt5KxY3MHBGVVm5CVKkmD8hptw52LuVGskGskczRwfq4HWUToQUnZ5BUnu_K6JYE1nfJdiFmB6lZyZfZQmQZSaT7gJ03lWD7cUI7QPEDoEu9FtT5H8oDgriiefPSmfPylfPClPrbzstBlMXild5MkhQHEYRFsTemILNohCeTy2VGMA7TTyEMsrkpGslbzTwxA6agxCV9Fm_DfSyD4Mgygd6n6BTBS0xwA50UVc82ZxnLpN5XGeAVCSBoNh4k2TOPo3ibvWDEzKaa4xEFLYEFwnVIqJ0xTJQ9ysbF6MOgbIZvKDqiOXZ9hjQDotPTegS9SYhXPFNLWjmxpHzoo0t4e9vy0ih2GLNrYV2iRLzxsiYG9_P80lPNPsxDSc3GxzJTm3SbKKs_iYjW7WbskuQLs4QHvT7vv1tV_h88vbPlOze9tHGYdnlt7vWHI0rYkqmYDKxZv_sEQ92NN8U4_l_u92_M1Jk8RBFG2DPQrCzdZfCtM4X7wgRM7Ar5oWjICXmvRjPuM0bPq1tXfVnVOfjQv_y5epBpk5Gb8VZYPoIm9f37u0jZcuvf_M4UmdrqXcu9Rg6y8rdAv3cQkAAA_22_7D = __webpack_require__(15017);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+recipes@0.5.7+6974e6bd53375d7a/node_modules/@vanilla-extract/recipes/dist/createRuntimeFn-62c9670f.esm.js
var createRuntimeFn_62c9670f_esm = __webpack_require__(8462);
;// ./src/components/shared/LogRow.css.ts


var GUTTER_WIDTH_NARROW_PX = 44;
var GUTTER_WIDTH_PX = 88;
var bodyCol = '_1ci3ir7h';
var bodyScrollable = '_1ci3ir7h';
var gutterAbsolute = '_1ci3ir7g';
var gutterCol = '_1ci3ir7g';
var levelBadge = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_1ci3ir70',variantClassNames:{level:{ERROR:'_1ci3ir71',WARN:'_1ci3ir72',INFO:'_1ci3ir73',DEBUG:'_1ci3ir74',TRACE:'_1ci3ir75',UNKNOWN:'_1ci3ir76'}},defaultVariants:{level:'UNKNOWN'},compoundVariants:[]});
var lineNumber = '_1ci3ir7i';
var mark = '_1ci3ir7l';
var row = '_1ci3ir77 _1ci3ir7d _1ci3ir7f';
var rowTint = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_1ci3ir77',variantClassNames:{level:{ERROR:'_1ci3ir78',WARN:'_1ci3ir79',INFO:'_1ci3ir7a',DEBUG:'_1ci3ir7b',TRACE:'_1ci3ir7c',UNKNOWN:'_1ci3ir7d'},isSelected:{true:'_1ci3ir7e',false:'_1ci3ir7f'}},defaultVariants:{level:'UNKNOWN',isSelected:false},compoundVariants:[]});
var timestampFull = '_1ci3ir7j';
var timestampShort = '_1ci3ir7k';
;// ./src/components/shared/LogRow.tsx




/**
 * Extract HH:mm:ss from an ISO-like timestamp string.
 * Returns the original string unchanged if parsing fails.
 */ function abbreviateTimestamp(ts) {
    // Handles "2026-01-02T15:04:05.000Z", "2026-01-02 15:04:05", etc.
    const match = ts.match(/T?(\d{2}:\d{2}:\d{2})/);
    return match ? match[1] : ts;
}
const LogRow = /*#__PURE__*/ react.memo(function LogRow(param) {
    let { entry, index, isExpanded, isSelected, onToggle, searchQuery } = param;
    const level = entry.level;
    const segments = segmentText(entry.message, searchQuery);
    const shortTs = abbreviateTimestamp(entry.timestamp);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: rowTint({
            level,
            isSelected
        }),
        role: "row",
        "aria-expanded": isExpanded,
        "aria-selected": isSelected,
        tabIndex: isSelected ? 0 : -1,
        "data-testid": "log-row-".concat(index),
        onClick: onToggle,
        onKeyDown: (e)=>{
            if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onToggle();
            }
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: gutterAbsolute,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: lineNumber,
                        children: index + 1
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: levelBadge({
                            level
                        }),
                        children: level === "UNKNOWN" ? "" : level
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: bodyScrollable,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: timestampFull,
                        children: entry.timestamp
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: timestampShort,
                        children: shortTs
                    }),
                    segments.map((seg, i)=>seg.highlight ? /*#__PURE__*/ (0,jsx_runtime.jsx)("mark", {
                            className: mark,
                            children: seg.text
                        }, i) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            children: seg.text
                        }, i))
                ]
            })
        ]
    });
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FExpandedLogDetail.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-VS0Y7aMBB85yv2sUg4igPhkPmYk5NsgnuOba03DbTqv5_sAAJOrfpeRU7kmY1mZ2eLd7nlej7sS_i1Agi664wbFBzCGWR1fR1XAI2nDkk0ntmPChIVvTUd0NDob1Vdb26nLOQ6_6Hbj4H85DrReutJLaXlJj9FtT6ufq-Km77M-p2JweqLgt5iltXWDE4YxjEqaNExUoK_T5FNfxGtd4yOFcSgWxQN8ozoUsWoaTDu3u8-uXiQq7Jc7x2LaH6iAlkuPjM0oxlOrGBflgmzyIwkkkYeTlmUBxwTw3hmwaRd7D2NCqYQkFodMZGPrmW938j9diPf0oDenr1vvzQjl2buceQkrh2OxonTtcHd7ikd0p2ZooInVIH8W1jVS1gKsp-gCR0_2DDuhGQWZKKYoODNLRD2U3sSumXjnYJROxMmq9PtyekuO12yUVAeX1bu-DKG6-5Z4_BuWRb1vazXo7EXBaN3Pm9AYuaTYcxhoYJAKGbSIROeOtEQ6g8F-SMSkhj_A6m3fhZnBXpin7AppsjRYssq5_wPOy3r9Z_SeBhC_f8O4RMd7obobwQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FExpandedLogDetail_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_VS0Y7aMBB85yv2sUg4igPhkPmYk5NsgnuOba03DbTqv5_sAAJOrfpeRU7kmY1mZ2eLd7nlej7sS_i1Agi664wbFBzCGWR1fR1XAI2nDkk0ntmPChIVvTUd0NDob1Vdb26nLOQ6_6Hbj4H85DrReutJLaXlJj9FtT6ufq_Km77M_p2JweqLgt5iltXWDE4YxjEqaNExUoK_T5FNfxGtd4yOFcSgWxQN8ozoUsWoaTDu3u8_uXiQq7Jc7x2LaH6iAlkuPjM0oxlOrGBflgmzyIwkkkYeTlmUBxwTw3hmwaRd7D2NCqYQkFodMZGPrmW938j9diPf0oDenr1vvzQjl2buceQkrh2OxonTtcHd7ikd0p2ZooInVIH8W1jVS1gKsp_gCR0_2DDuhGQWZKKYoODNLRD2U3sSumXjnYJROxMmq9PtyekuO12yUVAeX1bu_DKG6_5Z4_BuWRb1vazXo7EXBaN3Pm9AYuaTYcxhoYJAKGbSIROeOtEQ6g8F_SMSkhj_A6m3fhZnBXpin7AppsjRYssq5_wPOy3r9Z_SeBhC_f8O4RMd7obobwQAAA_22_7D = __webpack_require__(57084);
;// ./src/components/shared/ExpandedLogDetail.css.ts

var copyButton = '_13t5w863';
var detailHeader = '_13t5w861';
var detailLabel = '_13t5w862';
var detailPanel = '_13t5w860';
var jsonBlock = '_13t5w864';
var rawBlock = '_13t5w865';
;// ./src/components/shared/ExpandedLogDetail.tsx
/* __next_internal_client_entry_do_not_use__ ExpandedLogDetail auto */ 


/**
 * ExpandedLogDetail — accordion detail panel for a log entry.
 * Epic 4: JSON pretty-print detection and one-tap copy (with mobile fallback).
 */ function ExpandedLogDetail(param) {
    let { entry } = param;
    const text = entry.raw || entry.message;
    const parsed = tryParseJson(text);
    const handleCopy = ()=>{
        navigator.clipboard.writeText(text).catch(()=>{
            // Fallback for browsers without clipboard API (some mobile WebViews)
            const el = document.createElement("textarea");
            el.value = text;
            document.body.appendChild(el);
            el.select();
            document.execCommand("copy");
            document.body.removeChild(el);
        });
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: detailPanel,
        role: "cell",
        "aria-label": "Log entry detail",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: detailHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: detailLabel,
                        children: "Full entry"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: copyButton,
                        onClick: handleCopy,
                        "aria-label": "Copy log entry",
                        type: "button",
                        children: "Copy"
                    })
                ]
            }),
            parsed ? /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                className: jsonBlock,
                children: JSON.stringify(parsed, null, 2)
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                className: rawBlock,
                children: text
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FVirtualLogList.css.ts.vanilla.css%22%2C%22source%22%3A%22Lmx2cWF2NTAgewogIGhlaWdodDogMTAwJTsKICBvdmVyZmxvdzogaGlkZGVuOwogIG92ZXJzY3JvbGwtYmVoYXZpb3IteTogY29udGFpbjsKICBvdmVyZmxvdy1hbmNob3I6IG5vbmU7Cn0KLmx2cWF2NTEgewogIHBvc2l0aW9uOiBhYnNvbHV0ZTsKICB3aWR0aDogMXB4OwogIGhlaWdodDogMXB4OwogIG92ZXJmbG93OiBoaWRkZW47CiAgY2xpcDogcmVjdCgwLCAwLCAwLCAwKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIHBvaW50ZXItZXZlbnRzOiBub25lOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FVirtualLogList_css_ts_vanilla_css_22_2C_22source_22_3A_22Lmx2cWF2NTAgewogIGhlaWdodDogMTAwJTsKICBvdmVyZmxvdzogaGlkZGVuOwogIG92ZXJzY3JvbGwtYmVoYXZpb3IteTogY29udGFpbjsKICBvdmVyZmxvdy1hbmNob3I6IG5vbmU7Cn0KLmx2cWF2NTEgewogIHBvc2l0aW9uOiBhYnNvbHV0ZTsKICB3aWR0aDogMXB4OwogIGhlaWdodDogMXB4OwogIG92ZXJmbG93OiBoaWRkZW47CiAgY2xpcDogcmVjdCgwLCAwLCAwLCAwKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIHBvaW50ZXItZXZlbnRzOiBub25lOwp9_22_7D = __webpack_require__(2947);
;// ./src/components/shared/VirtualLogList.css.ts

var scrollContainer = 'lvqav50';
var srOnly = 'lvqav51';
;// ./src/components/shared/VirtualLogList.tsx
/* __next_internal_client_entry_do_not_use__ VirtualLogList auto */ 





/**
 * VirtualLogList — react-virtuoso backed virtual list with live-tail follow mode.
 * Implements Epic 2: virtual scroll, followOutput, atBottomStateChange, and aria-live throttling.
 * Epic 4: accordion row expansion via ExpandedLogDetail, selectedRowIndex highlight.
 */ function VirtualLogList(param) {
    let { entries, isFollowing, expandedRowIndex, selectedRowIndex, searchQuery, onAtBottomStateChange, onToggleRow, virtuosoRef } = param;
    // Internal ref used when caller doesn't provide one
    const internalRef = (0,react.useRef)(null);
    const ref = virtuosoRef !== null && virtuosoRef !== void 0 ? virtuosoRef : internalRef;
    // Throttled aria-live announcement: announce new log count at most once per 3s
    const [announcement, setAnnouncement] = (0,react.useState)("");
    const announcementTimerRef = (0,react.useRef)(null);
    const pendingCountRef = (0,react.useRef)(0);
    const scheduleAnnouncement = (0,react.useCallback)((count)=>{
        pendingCountRef.current = count;
        if (!announcementTimerRef.current) {
            announcementTimerRef.current = setTimeout(()=>{
                announcementTimerRef.current = null;
                if (pendingCountRef.current > 0) {
                    setAnnouncement("".concat(pendingCountRef.current, " new log entries"));
                    pendingCountRef.current = 0;
                }
            }, 3000);
        }
    }, []);
    // Announce when entries grow during live tail
    const prevLengthRef = (0,react.useRef)(entries.length);
    (0,react.useEffect)(()=>{
        const added = entries.length - prevLengthRef.current;
        prevLengthRef.current = entries.length;
        if (added > 0 && isFollowing) {
            scheduleAnnouncement(added);
        }
    }, [
        entries.length,
        isFollowing,
        scheduleAnnouncement
    ]);
    // Cleanup timer on unmount
    (0,react.useEffect)(()=>{
        return ()=>{
            if (announcementTimerRef.current) {
                clearTimeout(announcementTimerRef.current);
            }
        };
    }, []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: scrollContainer,
        style: {
            height: "100%"
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                "aria-atomic": "true",
                className: srOnly,
                children: announcement
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Virtuoso */.aY, {
                ref: ref,
                data: entries,
                followOutput: isFollowing ? "smooth" : false,
                overscan: 300,
                atBottomStateChange: onAtBottomStateChange,
                itemContent: (index, entry)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)(react.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(LogRow, {
                                entry: entry,
                                index: index,
                                isExpanded: expandedRowIndex === index,
                                isSelected: selectedRowIndex === index,
                                onToggle: ()=>onToggleRow(index),
                                searchQuery: searchQuery
                            }),
                            expandedRowIndex === index && /*#__PURE__*/ (0,jsx_runtime.jsx)(ExpandedLogDetail, {
                                entry: entry
                            })
                        ]
                    }, entry.id),
                role: "log",
                "aria-live": "polite",
                "aria-label": "Log output"
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FJumpToLatestButton.css.ts.vanilla.css%22%2C%22source%22%3A%22LnE3M2l3aTAgewogIHBvc2l0aW9uOiBmaXhlZDsKICBib3R0b206IGNhbGMoZW52KHNhZmUtYXJlYS1pbnNldC1ib3R0b20sIDBweCkgKyAxNnB4KTsKICByaWdodDogdmFyKC0tXzE5ZXoycnozMSk7CiAgei1pbmRleDogMTA7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeikgdmFyKC0tXzE5ZXoycnozMSk7CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycnptKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnpxKTsKICBib3JkZXI6IG5vbmU7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozOSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBmb250LXdlaWdodDogdmFyKC0tXzE5ZXoycnozaCk7CiAgY3Vyc29yOiBwb2ludGVyOwogIGJveC1zaGFkb3c6IHZhcigtLV8xOWV6MnJ6M2wpOwogIHRyYW5zaXRpb246IG9wYWNpdHkgMC4xNXMsIHRyYW5zZm9ybSAwLjE1czsKICB0b3VjaC1hY3Rpb246IG1hbmlwdWxhdGlvbjsKfQoucTczaXdpMDpob3ZlciB7CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycnpuKTsKfQoucTczaXdpMDphY3RpdmUgewogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6byk7CiAgdHJhbnNmb3JtOiBzY2FsZSgwLjk3KTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FJumpToLatestButton_css_ts_vanilla_css_22_2C_22source_22_3A_22LnE3M2l3aTAgewogIHBvc2l0aW9uOiBmaXhlZDsKICBib3R0b206IGNhbGMoZW52KHNhZmUtYXJlYS1pbnNldC1ib3R0b20sIDBweCkgKyAxNnB4KTsKICByaWdodDogdmFyKC0tXzE5ZXoycnozMSk7CiAgei1pbmRleDogMTA7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeikgdmFyKC0tXzE5ZXoycnozMSk7CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycnptKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnpxKTsKICBib3JkZXI6IG5vbmU7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozOSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBmb250LXdlaWdodDogdmFyKC0tXzE5ZXoycnozaCk7CiAgY3Vyc29yOiBwb2ludGVyOwogIGJveC1zaGFkb3c6IHZhcigtLV8xOWV6MnJ6M2wpOwogIHRyYW5zaXRpb246IG9wYWNpdHkgMC4xNXMsIHRyYW5zZm9ybSAwLjE1czsKICB0b3VjaC1hY3Rpb246IG1hbmlwdWxhdGlvbjsKfQoucTczaXdpMDpob3ZlciB7CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycnpuKTsKfQoucTczaXdpMDphY3RpdmUgewogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6byk7CiAgdHJhbnNmb3JtOiBzY2FsZSgwLjk3KTsKfQ_22_7D = __webpack_require__(83467);
;// ./src/components/shared/JumpToLatestButton.css.ts


var pill = 'q73iwi0';
;// ./src/components/shared/JumpToLatestButton.tsx


/**
 * JumpToLatestButton — fixed-position pill shown when the user has scrolled
 * away from the bottom during live tail. Shows queued new-line count.
 */ function JumpToLatestButton(param) {
    let { newLineCount, onClick } = param;
    if (newLineCount === 0) return null;
    const label = "Jump to latest log entry, ".concat(newLineCount, " new line").concat(newLineCount !== 1 ? "s" : "");
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
        className: pill,
        onClick: onClick,
        "aria-label": label,
        "data-testid": "jump-to-latest",
        children: [
            "↓ ",
            newLineCount,
            " new line",
            newLineCount !== 1 ? "s" : ""
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FLevelFilterChips.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6WT0U7DMAxF3_cVfgSJTM1gSGQfg9LGa81Ckjkp3Yb4d5R2Kx0g-sBDler62se5VZfP8ljr0tgC3hcAhmKw-qhga_GwWQDUOih4CP27f0PeWt-JgwLdJp-17BMdZ5fz-cxirNhbW2oWHZnU5JLDXBAdljtKYpw0OMnVCpJvq2az-Fgsx5WUujSME6-3HOZOWmRfLz0bZAUyHCB6Swa4LvXNar2-uzzFUq5vN6NXsDbURgVyNdw1aGP6tVbhALIYxFdyokGqm6Tg4RzK1rskIp1QgZQTqTv7Hosia1XL0bOC4Mkl5J6sq13NvnVGQWLtYtCMLvVub7OZXINMvdI1lFDEoCucJt3HHxsmt1PQg8ZsyFlyKC4fUluqnaCEr1FBhZclXtqYaHsUlXcJXfoqTUJ9GkKdrPum-UaIZ_mEpxWfpLudLP2tFqcp_27J7ROcnsP5P3BpHuevceUcLvzvduEaV83h9v_D7a9x5gdu9lc4z_3Fd59nfwL6u6XIMgQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FLevelFilterChips_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6WT0U7DMAxF3_cVfgSJTM1gSGQfg9LGa81Ckjkp3Yb4d5R2Kx0g_sBDler62se5VZfP8ljr0tgC3hcAhmKw_qhga_GwWQDUOih4CP27f0PeWt_JgwLdJp_17BMdZ5fz_cxirNhbW2oWHZnU5JLDXBAdljtKYpw0OMnVCpJvq2az_Fgsx5WUujSME6_3HOZOWmRfLz0bZAUyHCB6Swa4LvXNar2_uzzFUq5vN6NXsDbURgVyNdw1aGP6tVbhALIYxFdyokGqm6Tg4RzK1rskIp1QgZQTqTv7Hosia1XL0bOC4Mkl5J6sq13NvnVGQWLtYtCMLvVub7OZXINMvdI1lFDEoCucJt3HHxsmt1PQg8ZsyFlyKC4fUluqnaCEr1FBhZclXtqYaHsUlXcJXfoqTUJ9GkKdrPum_UaIZ_mEpxWfpLudLP2tFqcp_27J7ROcnsP5P3BpHuevceUcLvzvduEaV83h9v_D7a9x5gdu9lc4z_3Fd59nfwL6u6XIMgQAAA_22_7D = __webpack_require__(67272);
;// ./src/components/shared/LevelFilterChips.css.ts


var chip = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_1ygabdl1',variantClassNames:{level:{ALL:'_1ygabdl2',ERROR:'_1ygabdl3',WARN:'_1ygabdl4',INFO:'_1ygabdl5',DEBUG:'_1ygabdl6'},isActive:{true:'_1ygabdl7',false:'_1ygabdl8'}},defaultVariants:{level:'ALL',isActive:false},compoundVariants:[[{level:'ERROR',isActive:true},'_1ygabdl9'],[{level:'WARN',isActive:true},'_1ygabdla'],[{level:'INFO',isActive:true},'_1ygabdlb'],[{level:'DEBUG',isActive:true},'_1ygabdlc'],[{level:'ALL',isActive:true},'_1ygabdld']]});
var chipRow = '_1ygabdl0';
;// ./src/components/shared/LevelFilterChips.tsx


const LEVELS = [
    "ALL",
    "ERROR",
    "WARN",
    "INFO",
    "DEBUG"
];
function LevelFilterChips(param) {
    let { active, onChange } = param;
    const handleClick = (level)=>{
        if (level === "ALL") {
            onChange([
                "ALL"
            ]);
        } else {
            const next = active.includes("ALL") ? [
                level
            ] : active.includes(level) ? active.filter((l)=>l !== level) : [
                ...active,
                level
            ];
            onChange(next.length === 0 ? [
                "ALL"
            ] : next);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: chipRow,
        role: "group",
        "aria-label": "Filter by log level",
        children: LEVELS.map((level)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: chip({
                    level,
                    isActive: active.includes(level)
                }),
                "aria-pressed": active.includes(level),
                onClick: ()=>handleClick(level),
                children: level
            }, level))
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FLogViewerToolbar.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7WWbW-bMBDH3-dT-E3VRIojHgJNXE3a15imqXLgCF7B9myT0E757hPGICCkUTdNURRkzue7393_nM2Lf9ZAf5Ue-r1AKGVaFvSNoKyA-nmB7C9OmYLEMMEJSkRRlbx5c6SSoK20VpKmKeNHgmJZo127dhAqBYUPwhhREuTLGmlRsBSp44Eugyhad19v4-1W_WE6V4y_EuQ9Ly6LTReePx8eLdiRY2ag1AQlwA2oPra4ieOyIEoI8z2lhuICMoNzylNIf6CJ62miSpxHAQTWSgrN3HsoqGEn6OImyB_Zh9b-zFKTE-R73kPLpMaavVtUPR-bSMk4zoEdc0NQGE-obmWNwkDW9sH3RnwVTVmlXbbd6h3cgaV9oMnrUYmKp2S2JpG1SkQhFEGM56CYsdkKbposgCA_bE-1SxktWfFGUCm40JImFo2oTME4EMQFhzEgkomk0haTy8WdZYOJ9ms_9NbBNl57m3jV08tpKs4EefbTMLm2btIbHLSdVI4etCgqY8NTLXHXskZIgqK2VEZRrjOhStI-FtTAtyWOvIfVFIIryDB6P4rXfhyu_acG5ZPdIgVr-hPDCbjRHRGEzjkzgC2xZvGsqBzFH9n4mwZx3bR1qhv2TLfWK6TzfkMhPyttWPaGE8ENcDN81bVQ52HYJxaFpAq4bYWkUrrJ2aU23y9GVEmOqdNVSTmTVaMdwacgXRk-GAPx7Sk1FeDu8xMjus2116I3EeAdqfnR6gOx_iPZIbtgoMSzCz72vDme90pyryF3rXLxiWl2KMBy7nUejGnMyNiZYpFlGozdMXK_H05O1xJdOXazs89J9mqgjVUY3bhj3N3ATvDl0agKHgd3QxvL0PGJqiXGL_4e3gP17lezk6mZ0zOGgyTpX4n6ThP_b1FfV3S_-pzCw8906UBz27ZHvpaQMoqWJa17bqEn65WlObnRZ-90rOAESlsGl-GeyO25Yj2yiqdWHc-R1dMtX1d_CC6Lyx8YBObqggkAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FLogViewerToolbar_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7WWbW_bMBDH3_dT_E3VRIojHgJNXE3a15imqXLgCF7B9myT0E757hPGICCkUTdNURRkzue7393_nM2Lf9ZAf5Ue_r1AKGVaFvSNoKyA_nmB7C9OmYLEMMEJSkRRlbx5c6SSoK20VpKmKeNHgmJZo127dhAqBYUPwhhREuTLGmlRsBSp44Eugyhad19v4_1W_WE6V4y_EuQ9Ly6LTReePx8eLdiRY2ag1AQlwA2oPra4ieOyIEoI8z2lhuICMoNzylNIf6CJ62miSpxHAQTWSgrN3HsoqGEn6OImyB_Zh9b_zFKTE_R73kPLpMaavVtUPR_bSMk4zoEdc0NQGE_obmWNwkDW9sH3RnwVTVmlXbbd6h3cgaV9oMnrUYmKp2S2JpG1SkQhFEGM56CYsdkKbposgCA_bE_1SxktWfFGUCm40JImFo2oTME4EMQFhzEgkomk0haTy8WdZYOJ9ms_9NbBNl57m3jV08tpKs4EefbTMLm2btIbHLSdVI4etCgqY8NTLXHXskZIgqK2VEZRrjOhStI_FtTAtyWOvIfVFIIryDB6P4rXfhyu_acG5ZPdIgVr_hPDCbjRHRGEzjkzgC2xZvGsqBzFH9n4mwZx3bR1qhv2TLfWK6TzfkMhPyttWPaGE8ENcDN81bVQ52HYJxaFpAq4bYWkUrrJ2aU23y9GVEmOqdNVSTmTVaMdwacgXRk_GAPx7Sk1FeDu8xMjus2116I3EeAdqfnR6gOx_iPZIbtgoMSzCz72vDme90pyryF3rXLxiWl2KMBy7nUejGnMyNiZYpFlGozdMXK_H05O1xJdOXazs89J9mqgjVUY3bhj3N3ATvDl0agKHgd3QxvL0PGJqiXGL_4e3gP17lezk6mZ0zOGgyTpX4n6ThP_b1FfV3S_pzCw8906UBz27ZHvpaQMoqWJa17bqEn65WlObnRZ_90rOAESlsGl_GeyO25Yj2yiqdWHc_R1dMtX1d_CC6Lyx8YBObqggkAAA_22_7D = __webpack_require__(71822);
;// ./src/components/shared/LogViewerToolbar.css.ts

var liveTailButton = '_1wseaqm8';
var liveTailDot = '_1wseaqm9';
var matchCounter = '_1wseaqm4';
var searchDoneButton = '_1wseaqma';
var searchExpandableRow = '_1wseaqm6';
var searchExpandableRowOpen = '_1wseaqm7';
var searchIconButton = '_1wseaqm5';
var searchInput = '_1wseaqm3';
var searchWrapper = '_1wseaqm2';
var toolbar = '_1wseaqm0';
var toolbarRow = '_1wseaqm1';
;// ./src/components/shared/LogViewerToolbar.tsx




/**
 * LogViewerToolbar — search input with match counter, level filter chips,
 * and optional live-tail toggle.
 *
 * Epic 3: searchInputRef wired for '/' keyboard shortcut; match counter display.
 * Epic 5 / T3: Collapsible search bar on narrow screens (< 430px).
 *   - Wide screens (≥ 431px): search input always visible in the chips row.
 *   - Narrow screens (< 430px): shows a 🔍 icon button; tapping it expands a
 *     full-width search input row below the chips. A "Done" button collapses it
 *     (does NOT clear the search query).
 */ function LogViewerToolbar(param) {
    let { searchQuery, onSearchChange, matchCount, totalCount, levelFilters, onLevelFiltersChange, searchInputRef, liveTailEnabled = false, isFollowing = false, onLiveTailChange } = param;
    // T3: narrow-screen expanded state
    const [isSearchExpanded, setIsSearchExpanded] = (0,react.useState)(false);
    const handleSearchIconClick = (0,react.useCallback)(()=>{
        setIsSearchExpanded(true);
        // autoFocus via setTimeout to ensure the row is visible first
        setTimeout(()=>{
            var _searchInputRef_current;
            return searchInputRef === null || searchInputRef === void 0 ? void 0 : (_searchInputRef_current = searchInputRef.current) === null || _searchInputRef_current === void 0 ? void 0 : _searchInputRef_current.focus();
        }, 50);
    }, [
        searchInputRef
    ]);
    const handleDone = (0,react.useCallback)(()=>{
        var _searchInputRef_current;
        setIsSearchExpanded(false);
        searchInputRef === null || searchInputRef === void 0 ? void 0 : (_searchInputRef_current = searchInputRef.current) === null || _searchInputRef_current === void 0 ? void 0 : _searchInputRef_current.blur();
    }, [
        searchInputRef
    ]);
    // Combined class for the expandable search row: always open on wide screens,
    // open on narrow only when isSearchExpanded is true.
    const expandableRowClass = isSearchExpanded ? "".concat(searchExpandableRow, " ").concat(searchExpandableRowOpen) : searchExpandableRow;
    const searchInputEl = /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: searchWrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                ref: searchInputRef,
                type: "search",
                inputMode: "search",
                autoCapitalize: "none",
                autoCorrect: "off",
                autoComplete: "off",
                value: searchQuery,
                onChange: (e)=>onSearchChange(e.target.value),
                placeholder: "Search logs... (/ to focus)",
                "aria-label": "Search logs",
                className: searchInput,
                style: searchQuery ? {
                    paddingRight: 80
                } : undefined
            }),
            searchQuery && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                "aria-live": "polite",
                className: matchCounter,
                children: [
                    matchCount,
                    " / ",
                    totalCount
                ]
            })
        ]
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: toolbar,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: toolbarRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        type: "button",
                        className: liveTailButton,
                        "aria-label": "Toggle live tail",
                        "aria-pressed": liveTailEnabled,
                        "data-testid": "live-tail-toggle",
                        "data-live": liveTailEnabled && isFollowing ? "true" : "false",
                        onClick: ()=>onLiveTailChange === null || onLiveTailChange === void 0 ? void 0 : onLiveTailChange(!liveTailEnabled),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: liveTailDot
                            }),
                            liveTailEnabled && isFollowing ? "Live" : "Paused"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: searchIconButton,
                        "aria-label": "Expand search",
                        onClick: handleSearchIconClick,
                        children: "\uD83D\uDD0D"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(LevelFilterChips, {
                        active: levelFilters,
                        onChange: onLevelFiltersChange
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: searchExpandableRow,
                        style: {
                            flex: 1
                        },
                        children: searchInputEl
                    })
                ]
            }),
            isSearchExpanded && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: "".concat(toolbarRow, " ").concat(expandableRowClass),
                children: [
                    searchInputEl,
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: searchDoneButton,
                        "aria-label": "Collapse search",
                        onClick: handleDone,
                        children: "Done"
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FShortcutHelpOverlay.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA41UwXLjIAy95yu4dCY50LGdNk3ox-wQI9tqMVDAsZ2d_fcdQ9K4cVLnBpKQeO9Jeu6hfPtKE_J3QYjRDj1qxUiBHYj3BSGoHHhGkuF8pKgEdIykSRIMAp2RvGekkNANBi6xVBQ91I6RHJQHO5g_Guex6GmulQflx649zz9LqxslaK6ltowcuF1S-ifdwTGzR746RwmrDS1QerCM7GVjl5npVu-Lf4vniCENGOYS7mJCbcWQJzUdcVqiuIr6GEVRywU27jrRehtiDBcCVTnxZsFbo6ItCl8xkm0TE0iqeXe2vWQn2-m-Sw5tLNxRV3Gh20neOlb9VsqC5B4PMCIiC0TU3JaoTtLFC91r73U9yZmGnIVWnjo8wsQvLv4WsKz8JKIKETcJT8YarcPXTmjTJHka0ZxrKblxwMj5NHr4wpT2Sya58zSvUIpVVDs-PeO6J2cx_sNr7PXbumX96hpbcm0ZYgKrB7Aecy5p6HtGahRCwkjP9evTqPBmrvBM0pv0zmm3H0N_Cz_4HltUEhXQvdT5549eHnjcxM6cm6e3h-apfGCeNhckBa9R9hOKmhms_PcuJKSt0AN1hufAiNKt5WbEzvZqCfK907LxgXuvzaRczGlvDkSy-kkeI95y5Qy3oPyYMaVVFLexbvi20XjejfeBwn2gce8EXavTrKa_7Kmsf1CZC0us0gewDy3b7cxa-A9D8hzufgYAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FShortcutHelpOverlay_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA41UwXLjIAy95yu4dCY50LGdNk3ox_wQI9tqMVDAsZ2d_fcdQ9K4cVLnBpKQeO9Jeu6hfPtKE_J3QYjRDj1qxUiBHYj3BSGoHHhGkuF8pKgEdIykSRIMAp2RvGekkNANBi6xVBQ91I6RHJQHO5g_Guex6GmulQflx649zz9LqxslaK6ltowcuF1S_ifdwTGzR746RwmrDS1QerCM7GVjl5npVu_Lf4vniCENGOYS7mJCbcWQJzUdcVqiuIr6GEVRywU27jrRehtiDBcCVTnxZsFbo6ItCl8xkm0TE0iqeXe2vWQn2_m_Sw5tLNxRV3Gh20neOlb9VsqC5B4PMCIiC0TU3JaoTtLFC91r73U9yZmGnIVWnjo8wsQvLv4WsKz8JKIKETcJT8YarcPXTmjTJHka0ZxrKblxwMj5NHr4wpT2Sya58zSvUIpVVDs_PeO6J2cx_sNr7PXbumX96hpbcm0ZYgKrB7Aecy5p6HtGahRCwkjP9evTqPBmrvBM0pv0zmm3H0N_Cz_4HltUEhXQvdT5549eHnjcxM6cm6e3h_apfGCeNhckBa9R9hOKmhms_PcuJKSt0AN1hufAiNKt5WbEzvZqCfK907LxgXuvzaRczGlvDkSy_kkeI95y5Qy3oPyYMaVVFLexbvi20XjejfeBwn2gce8EXavTrKa_7Kmsf1CZC0us0gewDy3b7cxa_A9D8hzufgYAAA_22_7D = __webpack_require__(73340);
;// ./src/components/shared/ShortcutHelpOverlay.css.ts


var closeButton = 'yeg7q18';
var descCell = 'yeg7q16';
var kbd = 'yeg7q17';
var keyCell = 'yeg7q15';
var overlay = 'yeg7q10';
var panel = 'yeg7q11';
var ShortcutHelpOverlay_css_row = 'yeg7q14';
var table = 'yeg7q13';
var title = 'yeg7q12';
;// ./src/components/shared/ShortcutHelpOverlay.tsx


const SHORTCUTS = [
    {
        key: "/",
        desc: "Focus search"
    },
    {
        key: "Esc",
        desc: "Clear search"
    },
    {
        key: "g",
        desc: "Scroll to top"
    },
    {
        key: "G",
        desc: "Scroll to bottom"
    },
    {
        key: "=",
        desc: "Toggle live tail"
    },
    {
        key: "?",
        desc: "Toggle this help"
    }
];
function ShortcutHelpOverlay(param) {
    let { isOpen, onClose } = param;
    if (!isOpen) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        role: "dialog",
        "aria-modal": "true",
        "aria-label": "Keyboard shortcuts",
        className: overlay,
        onClick: (e)=>{
            // Close when clicking the backdrop
            if (e.target === e.currentTarget) onClose();
        },
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: panel,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: title,
                    children: "Keyboard Shortcuts"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("table", {
                    className: table,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                        children: SHORTCUTS.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                className: ShortcutHelpOverlay_css_row,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: keyCell,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                            className: kbd,
                                            children: s.key
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: descCell,
                                        children: s.desc
                                    })
                                ]
                            }, s.key))
                    })
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    onClick: onClose,
                    "aria-label": "Close shortcuts help",
                    className: closeButton,
                    children: "\xd7"
                })
            ]
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fshared%2FLogViewer.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xMTNmdXNpMCB7CiAgd2lkdGg6IDEwMCU7CiAgaGVpZ2h0OiAxMDAlOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBvdmVyZmxvdzogaGlkZGVuOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6Nik7Cn0KLl8xMTNmdXNpMSB7CiAgd2lkdGg6IDEwMCU7CiAgaGVpZ2h0OiAxMDAlOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBvdmVyZmxvdzogaGlkZGVuOwogIG1pbi1oZWlnaHQ6IDA7CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycno2KTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FLogViewer_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xMTNmdXNpMCB7CiAgd2lkdGg6IDEwMCU7CiAgaGVpZ2h0OiAxMDAlOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBvdmVyZmxvdzogaGlkZGVuOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6Nik7Cn0KLl8xMTNmdXNpMSB7CiAgd2lkdGg6IDEwMCU7CiAgaGVpZ2h0OiAxMDAlOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBvdmVyZmxvdzogaGlkZGVuOwogIG1pbi1oZWlnaHQ6IDA7CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycno2KTsKfQ_22_7D = __webpack_require__(61671);
;// ./src/components/shared/LogViewer.css.ts


var container = '_113fusi0';
var containerSession = '_113fusi1';
;// ./src/components/shared/LogViewer.tsx
/* __next_internal_client_entry_do_not_use__ LogViewer auto */ 







/**
 * LogViewer — shared log display component for both the app logs page and session logs tab.
 * Epic 2: wired with react-virtuoso virtual list, live-tail follow/pause state machine,
 * and Jump to Latest pill.
 * Epic 3: keyboard shortcuts (/, Esc, g, G, =, ?, Cmd+F) and shortcut help overlay.
 */ const LogViewer = /*#__PURE__*/ (0,react.forwardRef)(function LogViewer(param, ref) {
    let { source, sessionId, timeRange, limit, onStateChange } = param;
    const { logs, isFollowing, searchQuery, setSearchQuery, levelFilters, setLevelFilters, matchCount, totalCount, toggleRow, expandedRowIndex, selectedRowIndex, setSelectedRowIndex, jumpToLatest, queuedNewLineCount, onAtBottomStateChange, virtuosoRef, liveTailEnabled, setLiveTailEnabled, rawEntries, serverTotalCount, lastRefresh, error, refresh } = useLogViewer(source, sessionId, {
        timeRange,
        limit
    });
    (0,react.useImperativeHandle)(ref, ()=>({
            refresh
        }), [
        refresh
    ]);
    (0,react.useEffect)(()=>{
        onStateChange === null || onStateChange === void 0 ? void 0 : onStateChange({
            logs,
            rawEntries,
            totalCount: serverTotalCount,
            lastRefresh,
            error
        });
    }, [
        logs,
        rawEntries,
        serverTotalCount,
        lastRefresh,
        error,
        onStateChange
    ]);
    const containerRef = (0,react.useRef)(null);
    const searchInputRef = (0,react.useRef)(null);
    const [showShortcutHelp, setShowShortcutHelp] = (0,react.useState)(false);
    // T9: Keyboard shortcuts — liveTailEnabled in a ref to avoid stale closure
    const liveTailEnabledRef = (0,react.useRef)(liveTailEnabled);
    (0,react.useEffect)(()=>{
        liveTailEnabledRef.current = liveTailEnabled;
    }, [
        liveTailEnabled
    ]);
    // selectedRowIndex in a ref to avoid stale closure in keyboard handler
    const selectedRowIndexRef = (0,react.useRef)(selectedRowIndex);
    (0,react.useEffect)(()=>{
        selectedRowIndexRef.current = selectedRowIndex;
    }, [
        selectedRowIndex
    ]);
    // logs length in a ref to avoid stale closure
    const logsLengthRef = (0,react.useRef)(logs.length);
    (0,react.useEffect)(()=>{
        logsLengthRef.current = logs.length;
    }, [
        logs.length
    ]);
    const handleKeyDown = (0,react.useCallback)((e)=>{
        // Do not fire shortcuts when focus is inside an input/textarea/select
        const target = e.target;
        if ([
            "INPUT",
            "TEXTAREA",
            "SELECT"
        ].includes(target.tagName)) return;
        // Cmd+F / Ctrl+F — intercept browser find and focus search
        if (e.key === "f" && (e.metaKey || e.ctrlKey)) {
            var _searchInputRef_current;
            e.preventDefault();
            (_searchInputRef_current = searchInputRef.current) === null || _searchInputRef_current === void 0 ? void 0 : _searchInputRef_current.focus();
            return;
        }
        switch(e.key){
            case "/":
                var _searchInputRef_current1;
                e.preventDefault();
                (_searchInputRef_current1 = searchInputRef.current) === null || _searchInputRef_current1 === void 0 ? void 0 : _searchInputRef_current1.focus();
                break;
            case "Escape":
                var _searchInputRef_current2;
                (_searchInputRef_current2 = searchInputRef.current) === null || _searchInputRef_current2 === void 0 ? void 0 : _searchInputRef_current2.blur();
                setSearchQuery("");
                break;
            case "g":
                var _virtuosoRef_current;
                (_virtuosoRef_current = virtuosoRef.current) === null || _virtuosoRef_current === void 0 ? void 0 : _virtuosoRef_current.scrollToIndex(0);
                break;
            case "G":
                var _virtuosoRef_current1;
                (_virtuosoRef_current1 = virtuosoRef.current) === null || _virtuosoRef_current1 === void 0 ? void 0 : _virtuosoRef_current1.scrollToIndex({
                    index: "LAST",
                    behavior: "smooth"
                });
                break;
            case "=":
                setLiveTailEnabled(!liveTailEnabledRef.current);
                break;
            case "?":
                setShowShortcutHelp((prev)=>!prev);
                break;
            case "j":
            case "ArrowDown":
                {
                    var _virtuosoRef_current2;
                    e.preventDefault();
                    const len = logsLengthRef.current;
                    if (len === 0) break;
                    const cur = selectedRowIndexRef.current;
                    const next = cur === null ? 0 : Math.min(cur + 1, len - 1);
                    setSelectedRowIndex(next);
                    (_virtuosoRef_current2 = virtuosoRef.current) === null || _virtuosoRef_current2 === void 0 ? void 0 : _virtuosoRef_current2.scrollToIndex({
                        index: next,
                        behavior: "smooth"
                    });
                    break;
                }
            case "k":
            case "ArrowUp":
                {
                    var _virtuosoRef_current3;
                    e.preventDefault();
                    const len = logsLengthRef.current;
                    if (len === 0) break;
                    const cur = selectedRowIndexRef.current;
                    const prev = cur === null ? 0 : Math.max(cur - 1, 0);
                    setSelectedRowIndex(prev);
                    (_virtuosoRef_current3 = virtuosoRef.current) === null || _virtuosoRef_current3 === void 0 ? void 0 : _virtuosoRef_current3.scrollToIndex({
                        index: prev,
                        behavior: "smooth"
                    });
                    break;
                }
            default:
                break;
        }
    }, [
        setSearchQuery,
        virtuosoRef,
        setLiveTailEnabled,
        setSelectedRowIndex
    ]);
    // T6: VisualViewport resize handler — recalculates scroll container height
    // when the iOS software keyboard appears or disappears. Uses
    // window.visualViewport (not window.resize) to get the actual visible height.
    // Debounced 400ms to avoid thrashing during keyboard animation.
    (0,react.useEffect)(()=>{
        if (!window.visualViewport) return;
        let debounceTimer;
        const handler = ()=>{
            clearTimeout(debounceTimer);
            debounceTimer = setTimeout(()=>{
                var _containerRef_current;
                const height = window.visualViewport.height;
                (_containerRef_current = containerRef.current) === null || _containerRef_current === void 0 ? void 0 : _containerRef_current.style.setProperty("--log-container-height", "".concat(height, "px"));
            }, 400);
        };
        window.visualViewport.addEventListener("resize", handler);
        return ()=>{
            window.visualViewport.removeEventListener("resize", handler);
            clearTimeout(debounceTimer);
        };
    }, []);
    (0,react.useEffect)(()=>{
        const container = containerRef.current;
        if (!container) return;
        const onKeyDown = (e)=>{
            // Cmd+F: intercept at window level when our container is active
            if (e.key === "f" && (e.metaKey || e.ctrlKey)) {
                const active = document.activeElement;
                if (active && !container.contains(active) && active !== document.body) return;
                handleKeyDown(e);
                return;
            }
            // Other shortcuts: only fire when focus is inside our container or on body
            const active = document.activeElement;
            if (container.contains(active) || active === document.body) {
                handleKeyDown(e);
            }
        };
        window.addEventListener("keydown", onKeyDown);
        return ()=>window.removeEventListener("keydown", onKeyDown);
    }, [
        handleKeyDown
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: source === "session" ? containerSession : container,
        ref: containerRef,
        tabIndex: -1,
        "data-testid": "log-viewer",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(LogViewerToolbar, {
                searchQuery: searchQuery,
                onSearchChange: setSearchQuery,
                matchCount: matchCount,
                totalCount: totalCount,
                levelFilters: levelFilters,
                onLevelFiltersChange: setLevelFilters,
                searchInputRef: searchInputRef,
                liveTailEnabled: liveTailEnabled,
                isFollowing: isFollowing,
                onLiveTailChange: setLiveTailEnabled
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                style: {
                    flex: 1,
                    minHeight: 0,
                    position: "relative"
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(VirtualLogList, {
                        entries: logs,
                        isFollowing: isFollowing,
                        expandedRowIndex: expandedRowIndex,
                        selectedRowIndex: selectedRowIndex,
                        searchQuery: searchQuery,
                        onAtBottomStateChange: onAtBottomStateChange,
                        onToggleRow: toggleRow,
                        virtuosoRef: virtuosoRef
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(JumpToLatestButton, {
                        newLineCount: queuedNewLineCount,
                        onClick: jumpToLatest
                    })
                ]
            }),
            source === "app" && /*#__PURE__*/ (0,jsx_runtime.jsx)(ShortcutHelpOverlay, {
                isOpen: showShortcutHelp,
                onClose: ()=>setShowShortcutHelp(false)
            })
        ]
    });
});


/***/ })

}]);
//# sourceMappingURL=2692-267f3e42e5a2ce97.js.map