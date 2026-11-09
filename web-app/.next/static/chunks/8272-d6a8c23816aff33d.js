"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[8272],{

/***/ 34313:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  z: () => (/* binding */ ReviewQueueProvider),
  O: () => (/* binding */ useReviewQueueContext)
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
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/reviewQueueSlice.ts
var reviewQueueSlice = __webpack_require__(68453);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
;// ./src/lib/utils/reviewQueueVisibility.ts

// Idle-reason items are no longer review-queue-worthy (Epic 3.2.2, ADR-002): an idle
// "ready for next task" session gets the Sessions-list SubStatusChip instead. Applied once,
// centrally, in useReviewQueue.ts so every consumer (ReviewQueuePanel, ReviewQueueNavBadge,
// BottomNav, DrawerNav) sees the same filtered view by construction — avoiding the
// count-vs-list mismatch class of bug StuckItemsSection's own comment warns about.
function isReviewQueueVisible(item) {
    return item.reason !== types_pb/* AttentionReason */.X9.IDLE;
}

// EXTERNAL MODULE: ./src/lib/hooks/useWatchStream.ts
var useWatchStream = __webpack_require__(36095);
;// ./src/lib/hooks/useReviewQueue.ts
/* __next_internal_client_entry_do_not_use__ useReviewQueue auto */ 









// How long a reconciled row stays visible-but-disabled (ux.md Surface 9) before the
// deferred removeItem dispatch actually removes it from the store.
const AUTO_RESOLVED_DISPLAY_WINDOW_MS = 5000;
/**
 * React hook for managing review queue data with real-time push updates and optimistic UI.
 *
 * The review queue tracks sessions that need user attention, ordered by priority.
 * Supports optional filtering by priority level or attention reason.
 *
 * **Update Strategy:**
 * - **Push (WebSocket)**: Direct WatchReviewQueue stream for <100ms queue updates
 * - **Optimistic Updates**: UI updates immediately on user actions (acknowledge)
 * - **Pull (Polling)**: Fallback polling (30s) for eventual consistency
 * - **Initial Snapshot**: Immediate queue state on connection
 *
 * **Performance:**
 * - <100ms latency for queue updates (vs 7-32 seconds with polling)
 * - Zero flickering - direct queue event stream
 * - Reduced server load - targeted updates only
 *
 * @example
 * ```tsx
 * // Default: WebSocket push + optimistic updates
 * const { items, totalItems, acknowledgeSession } = useReviewQueue();
 *
 * // Acknowledge with immediate UI feedback
 * await acknowledgeSession('session-id');
 *
 * // Legacy polling-only mode
 * const { items } = useReviewQueue({
 *   useWebSocketPush: false,
 *   autoRefresh: true,
 *   refreshInterval: 5000,
 * });
 * ```
 */ function useReviewQueue() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { autoRefresh = false, refreshInterval = 5000, priorityFilter, reasonFilter, useWebSocketPush = true, fallbackPollInterval = 30000, onAutoResolved } = options;
    // Ref'd so the itemRemoved handler (set once, see handleReviewQueueEventRef below)
    // always calls the latest callback without needing it in that effect's deps.
    const onAutoResolvedRef = (0,react.useRef)(onAutoResolved);
    (0,react.useEffect)(()=>{
        onAutoResolvedRef.current = onAutoResolved;
    }, [
        onAutoResolved
    ]);
    // sessionId -> rule name for items in their post-reconciliation display window
    // (ux.md Surface 9's "disable, don't hide"). See autoResolvedRules on the return type.
    const [autoResolvedRules, setAutoResolvedRules] = (0,react.useState)({});
    // Pending deferred-removal timers, keyed by sessionId, so they can be cleared on unmount.
    const autoResolveTimeoutsRef = (0,react.useRef)({});
    (0,react.useEffect)(()=>{
        const timeouts = autoResolveTimeoutsRef.current;
        return ()=>{
            for (const id of Object.values(timeouts))clearTimeout(id);
        };
    }, []);
    const dispatch = (0,store/* useAppDispatch */.jL)();
    const reviewQueue = (0,store/* useAppSelector */.GV)(reviewQueueSlice/* selectReviewQueue */.KS);
    const liveItems = (0,store/* useAppSelector */.GV)(reviewQueueSlice/* selectReviewQueueItemsWithLiveStatus */._W);
    const loading = (0,store/* useAppSelector */.GV)(reviewQueueSlice/* selectReviewQueueLoading */.K6);
    const errorStr = (0,store/* useAppSelector */.GV)(reviewQueueSlice/* selectReviewQueueError */.wq);
    const lastUpdatedAt = (0,store/* useAppSelector */.GV)(reviewQueueSlice/* selectReviewQueueLastUpdatedAt */.wk);
    // Always-fresh refs for fetchReviewQueue (declared with a stable [dispatch]
    // dep array below) to read current queue/window state without a stale closure.
    const reviewQueueRef = (0,react.useRef)(reviewQueue);
    reviewQueueRef.current = reviewQueue;
    const autoResolvedRulesRef = (0,react.useRef)(autoResolvedRules);
    autoResolvedRulesRef.current = autoResolvedRules;
    // Idle-reason items no longer occupy review-queue slots (Epic 3.2.2, ADR-002) — filtered
    // once here so every consumer (ReviewQueuePanel, ReviewQueueNavBadge, BottomNav, DrawerNav)
    // sees the same list, and totalItems (below) is derived from this same filtered array
    // rather than the raw backend stat, avoiding a count-vs-list mismatch.
    const filteredItems = (0,react.useMemo)(()=>liveItems.filter(isReviewQueueVisible), [
        liveItems
    ]);
    const clientRef = (0,react.useRef)(null);
    const intervalRef = (0,react.useRef)(null);
    const lastUpdateRef = (0,react.useRef)(Date.now());
    // Initialize ConnectRPC client — uses HTTP for unary, WebSocket for streaming Watch* RPCs
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getWatchTransport */.VZ)());
    }, []);
    // Fetch review queue with optional filters
    const fetchReviewQueue = (0,react.useCallback)(async (filters)=>{
        if (!clientRef.current) return;
        dispatch((0,reviewQueueSlice/* setLoading */.r1)(true));
        dispatch((0,reviewQueueSlice/* setError */.Nh)(null));
        try {
            const request = (0,create/* create */.v)(session_pb/* GetReviewQueueRequestSchema */.ysi, {});
            // Apply filters if provided
            if ((filters === null || filters === void 0 ? void 0 : filters.priorityFilter) !== undefined) {
                request.priorityFilter = filters.priorityFilter;
            }
            if ((filters === null || filters === void 0 ? void 0 : filters.reasonFilter) !== undefined) {
                request.reasonFilter = filters.reasonFilter;
            }
            const response = await clientRef.current.getReviewQueue(request);
            var _response_reviewQueue;
            // A REST fetch (including the fallback poll) fully replaces the queue, but
            // an item in its ~5s post-reconciliation display window (autoResolvedRules)
            // has already been deleted backend-side — a poll landing mid-window would
            // otherwise evict the row early while its banner lingers. Re-splice any such
            // item back in from the current store state before replacing (finding #2).
            let reviewQueueToStore = (_response_reviewQueue = response.reviewQueue) !== null && _response_reviewQueue !== void 0 ? _response_reviewQueue : null;
            const preserveIds = Object.keys(autoResolvedRulesRef.current);
            if (preserveIds.length > 0 && reviewQueueToStore) {
                var _reviewQueueRef_current;
                var _reviewQueueRef_current_items;
                const existingItems = (_reviewQueueRef_current_items = (_reviewQueueRef_current = reviewQueueRef.current) === null || _reviewQueueRef_current === void 0 ? void 0 : _reviewQueueRef_current.items) !== null && _reviewQueueRef_current_items !== void 0 ? _reviewQueueRef_current_items : [];
                const incomingIds = new Set(reviewQueueToStore.items.map((item)=>item.sessionId));
                const toPreserve = existingItems.filter((item)=>preserveIds.includes(item.sessionId) && !incomingIds.has(item.sessionId));
                if (toPreserve.length > 0) {
                    reviewQueueToStore = {
                        ...reviewQueueToStore,
                        items: [
                            ...reviewQueueToStore.items,
                            ...toPreserve
                        ],
                        totalItems: reviewQueueToStore.totalItems + toPreserve.length
                    };
                }
            }
            dispatch((0,reviewQueueSlice/* setReviewQueue */.s6)(reviewQueueToStore));
            dispatch((0,reviewQueueSlice/* setError */.Nh)(null));
        } catch (err) {
            const error = err instanceof Error ? err : new Error("Failed to fetch review queue");
            dispatch((0,reviewQueueSlice/* setError */.Nh)(error.message));
            console.error("Failed to fetch review queue:", error);
        } finally{
            dispatch((0,reviewQueueSlice/* setLoading */.r1)(false));
        }
    }, [
        dispatch
    ]);
    // Refresh with current filters
    const refresh = (0,react.useCallback)(async ()=>{
        await fetchReviewQueue({
            priorityFilter,
            reasonFilter
        });
    }, [
        fetchReviewQueue,
        priorityFilter,
        reasonFilter
    ]);
    // Get review queue filtered by priority
    const getByPriority = (0,react.useCallback)(async (priority)=>{
        if (!clientRef.current) return null;
        try {
            const response = await clientRef.current.getReviewQueue({
                priorityFilter: priority
            });
            var _response_reviewQueue;
            return (_response_reviewQueue = response.reviewQueue) !== null && _response_reviewQueue !== void 0 ? _response_reviewQueue : null;
        } catch (err) {
            dispatch((0,reviewQueueSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to fetch by priority"));
            return null;
        }
    }, [
        dispatch
    ]);
    // Get review queue filtered by reason
    const getByReason = (0,react.useCallback)(async (reason)=>{
        if (!clientRef.current) return null;
        try {
            const response = await clientRef.current.getReviewQueue({
                reasonFilter: reason
            });
            var _response_reviewQueue;
            return (_response_reviewQueue = response.reviewQueue) !== null && _response_reviewQueue !== void 0 ? _response_reviewQueue : null;
        } catch (err) {
            dispatch((0,reviewQueueSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to fetch by reason"));
            return null;
        }
    }, [
        dispatch
    ]);
    // Handle review queue events from dedicated WatchReviewQueue stream
    // Use ref callback to avoid dependency issues that would cause stream reconnects.
    // The handler only uses dispatch with functional updates (no stale closures),
    // so it only needs to be set once on mount.
    const handleReviewQueueEventRef = (0,react.useRef)(undefined);
    (0,react.useEffect)(()=>{
        handleReviewQueueEventRef.current = (event)=>{
            switch(event.event.case){
                case "itemAdded":
                    if (event.event.value.item) {
                        dispatch((0,reviewQueueSlice/* addItem */.B5)(event.event.value.item));
                    }
                    break;
                case "itemRemoved":
                    {
                        const sessionId = event.event.value.sessionId;
                        if (!sessionId) break;
                        const ruleName = event.event.value.autoResolvedByRule;
                        if (ruleName) {
                            var _onAutoResolvedRef_current;
                            // Reconciliation-driven removal (Epic 2.3.1): disable the row in place
                            // rather than removing it immediately (ux.md Surface 9) — surface the
                            // rule name now, defer the actual removeItem dispatch for ~5s so the
                            // disabled state + banner is visible first.
                            setAutoResolvedRules((prev)=>({
                                    ...prev,
                                    [sessionId]: ruleName
                                }));
                            (_onAutoResolvedRef_current = onAutoResolvedRef.current) === null || _onAutoResolvedRef_current === void 0 ? void 0 : _onAutoResolvedRef_current.call(onAutoResolvedRef, sessionId, ruleName);
                            const timeoutId = setTimeout(()=>{
                                dispatch((0,reviewQueueSlice/* removeItem */.Ai)(sessionId));
                                setAutoResolvedRules((prev)=>{
                                    const next = {
                                        ...prev
                                    };
                                    delete next[sessionId];
                                    return next;
                                });
                                delete autoResolveTimeoutsRef.current[sessionId];
                            }, AUTO_RESOLVED_DISPLAY_WINDOW_MS);
                            autoResolveTimeoutsRef.current[sessionId] = timeoutId;
                        } else {
                            dispatch((0,reviewQueueSlice/* removeItem */.Ai)(sessionId));
                        }
                        break;
                    }
                case "itemUpdated":
                    if (event.event.value.item && event.event.value.sessionId) {
                        dispatch((0,reviewQueueSlice/* updateItem */.fk)({
                            sessionId: event.event.value.sessionId,
                            updates: event.event.value.item
                        }));
                    }
                    break;
                case "statistics":
                    break;
                default:
                    break;
            }
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        dispatch
    ]);
    // Setup WebSocket push updates with dedicated WatchReviewQueue stream.
    // Connection mechanics (backoff reconnect, idle-staleness watchdog) live in
    // the shared useWatchStream.ts -- this only supplies the request shape and
    // event dispatch. ReviewQueueEvent has no seq/afterSeq replay buffer (the
    // review queue manager isn't built on pkg/events.EventBus like the other
    // three Watch* streams), so getSeq is omitted; a stall or filter change
    // just reconnects to a fresh initialSnapshot.
    const subscribeReviewQueue = (0,react.useCallback)((_afterSeq, signal)=>{
        const request = (0,create/* create */.v)(session_pb/* WatchReviewQueueRequestSchema */.VXY, {
            priorityFilter: priorityFilter !== undefined ? [
                priorityFilter
            ] : [],
            reasonFilter: reasonFilter !== undefined ? [
                reasonFilter
            ] : [],
            initialSnapshot: true,
            includeStatistics: true
        });
        return clientRef.current.watchReviewQueue(request, {
            signal
        });
    }, [
        priorityFilter,
        reasonFilter
    ]);
    const onReviewQueueEvent = (0,react.useCallback)((event)=>{
        var _handleReviewQueueEventRef_current;
        (_handleReviewQueueEventRef_current = handleReviewQueueEventRef.current) === null || _handleReviewQueueEventRef_current === void 0 ? void 0 : _handleReviewQueueEventRef_current.call(handleReviewQueueEventRef, event);
    }, []);
    (0,useWatchStream/* useWatchStream */.t)({
        subscribe: subscribeReviewQueue,
        onEvent: onReviewQueueEvent,
        isHeartbeat: (event)=>event.heartbeat,
        enabled: useWebSocketPush,
        restartKey: "".concat(priorityFilter !== null && priorityFilter !== void 0 ? priorityFilter : "", ":").concat(reasonFilter !== null && reasonFilter !== void 0 ? reasonFilter : "")
    });
    // Keep a ref to the latest refresh so interval callbacks are always current
    // without needing refresh in the interval-setup effect's dep array.
    const refreshRef = (0,react.useRef)(refresh);
    (0,react.useEffect)(()=>{
        refreshRef.current = refresh;
    }, [
        refresh
    ]);
    // Always do an initial REST fetch on mount for immediate fresh data.
    // Even in WebSocket push mode the stream may take a moment to connect and
    // deliver its initialSnapshot, leaving the page empty in the meantime.
    // The REST response fills the UI instantly; the stream then overlays updates.
    (0,react.useEffect)(()=>{
        refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []); // intentionally empty -- run once on mount
    // Setup fallback polling or legacy auto-refresh.
    // Intentionally excludes `refresh` from deps; uses refreshRef.current instead
    // so that filter changes (which change `refresh` identity) don't cause an
    // immediate duplicate fetch -- the WatchReviewQueue stream re-connects on
    // filter changes and delivers a fresh initialSnapshot. useWatchStream's own
    // backoff + idle-staleness watchdog self-heal the stream independently, so
    // this poll no longer needs to coordinate a reconnect -- it's just a REST
    // consistency backstop (hybrid mode) or the sole refresh source (legacy).
    (0,react.useEffect)(()=>{
        let interval = null;
        if (useWebSocketPush) {
            interval = setInterval(()=>{
                void refreshRef.current();
            }, fallbackPollInterval);
        } else if (autoRefresh) {
            // Legacy mode: Use original refresh interval
            interval = setInterval(()=>{
                refreshRef.current();
            }, refreshInterval);
        }
        return ()=>{
            if (interval) {
                clearInterval(interval);
            }
        };
    }, [
        useWebSocketPush,
        autoRefresh,
        refreshInterval,
        fallbackPollInterval
    ]);
    // Acknowledge session with optimistic update
    const acknowledgeSession = (0,react.useCallback)(async (sessionId)=>{
        if (!clientRef.current) return;
        // Optimistic update via slice action — no closure dependency on reviewQueue.
        // The removeItem reducer handles the filter + totalItems decrement internally.
        dispatch((0,reviewQueueSlice/* removeItem */.Ai)(sessionId));
        try {
            const request = (0,create/* create */.v)(session_pb/* AcknowledgeSessionRequestSchema */.Zki, {
                id: sessionId
            });
            await clientRef.current.acknowledgeSession(request);
        // Success - optimistic update was correct
        } catch (err) {
            console.error("Failed to acknowledge session:", err);
            dispatch((0,reviewQueueSlice/* setError */.Nh)(err instanceof Error ? err.message : "Failed to acknowledge session"));
            // Rollback - refetch to get correct state
            await refresh();
        }
    }, [
        refresh,
        dispatch
    ]);
    // Acknowledges multiple sessions in parallel. Unlike acknowledgeSession, a per-item
    // RPC failure does not dispatch the global queue error (which would blank the whole
    // panel) — failures are collected and returned so the caller can show a scoped result
    // instead.
    const acknowledgeSessions = (0,react.useCallback)(async (sessionIds)=>{
        if (!clientRef.current) return {
            failed: sessionIds
        };
        sessionIds.forEach((id)=>dispatch((0,reviewQueueSlice/* removeItem */.Ai)(id)));
        const results = await Promise.allSettled(sessionIds.map((id)=>{
            const request = (0,create/* create */.v)(session_pb/* AcknowledgeSessionRequestSchema */.Zki, {
                id
            });
            return clientRef.current.acknowledgeSession(request);
        }));
        const failed = sessionIds.filter((_, i)=>results[i].status === "rejected");
        if (failed.length > 0) {
            console.error("Failed to acknowledge ".concat(failed.length, " of ").concat(sessionIds.length, " sessions"));
            // Reconcile optimistic removals against real server state for the failed items.
            await refresh();
        }
        return {
            failed
        };
    }, [
        refresh,
        dispatch
    ]);
    var _reviewQueue_byPriority, _reviewQueue_byReason, _reviewQueue_averageAgeSeconds, _reviewQueue_oldestItemId, _reviewQueue_oldestAgeSeconds;
    // Extract statistics from review queue
    const statistics = {
        // Filtered items.length, not the raw backend stat (reviewQueue?.totalItems counts ALL
        // items including idle ones — session/queue/queue.go's TotalItems is deliberately
        // unchanged per ADR-002's scope boundary). Every ReviewQueuePanel headline-count read
        // sources totalItems from this hook, so fixing it here fixes every read site at once
        // (adversarial-review.md Blocker 2).
        totalItems: filteredItems.length,
        byPriority: new Map(Object.entries((_reviewQueue_byPriority = reviewQueue === null || reviewQueue === void 0 ? void 0 : reviewQueue.byPriority) !== null && _reviewQueue_byPriority !== void 0 ? _reviewQueue_byPriority : {}).map((param)=>{
            let [key, value] = param;
            return [
                parseInt(key),
                value
            ];
        })),
        byReason: new Map(Object.entries((_reviewQueue_byReason = reviewQueue === null || reviewQueue === void 0 ? void 0 : reviewQueue.byReason) !== null && _reviewQueue_byReason !== void 0 ? _reviewQueue_byReason : {}).map((param)=>{
            let [key, value] = param;
            return [
                parseInt(key),
                value
            ];
        })),
        averageAgeSeconds: (_reviewQueue_averageAgeSeconds = reviewQueue === null || reviewQueue === void 0 ? void 0 : reviewQueue.averageAgeSeconds) !== null && _reviewQueue_averageAgeSeconds !== void 0 ? _reviewQueue_averageAgeSeconds : BigInt(0),
        oldestItemId: (_reviewQueue_oldestItemId = reviewQueue === null || reviewQueue === void 0 ? void 0 : reviewQueue.oldestItemId) !== null && _reviewQueue_oldestItemId !== void 0 ? _reviewQueue_oldestItemId : "",
        oldestAgeSeconds: (_reviewQueue_oldestAgeSeconds = reviewQueue === null || reviewQueue === void 0 ? void 0 : reviewQueue.oldestAgeSeconds) !== null && _reviewQueue_oldestAgeSeconds !== void 0 ? _reviewQueue_oldestAgeSeconds : BigInt(0)
    };
    // Convert error string back to Error object for backward compatibility.
    // Memoised so the Error identity stays stable across renders.
    const error = (0,react.useMemo)(()=>errorStr ? new Error(errorStr) : null, [
        errorStr
    ]);
    return {
        reviewQueue,
        items: filteredItems,
        lastUpdatedAt,
        autoResolvedRules,
        loading,
        error,
        ...statistics,
        refresh,
        getByPriority,
        getByReason,
        acknowledgeSession,
        acknowledgeSessions
    };
}

;// ./src/lib/contexts/ReviewQueueContext.tsx
/* __next_internal_client_entry_do_not_use__ ReviewQueueProvider,useReviewQueueContext auto */ 


const ReviewQueueContext = /*#__PURE__*/ (0,react.createContext)(null);
function ReviewQueueProvider(param) {
    let { children } = param;
    const value = useReviewQueue({
        useWebSocketPush: true,
        autoRefresh: true
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(ReviewQueueContext.Provider, {
        value: value,
        children: children
    });
}
function useReviewQueueContext() {
    const ctx = (0,react.useContext)(ReviewQueueContext);
    if (!ctx) throw new Error("useReviewQueueContext must be used within ReviewQueueProvider");
    return ctx;
}


/***/ }),

/***/ 38808:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  s: () => (/* binding */ ApprovalsProvider),
  B: () => (/* binding */ useApprovalsContext)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/api/connectApi.ts
var connectApi = __webpack_require__(24031);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/lib/api/serialization.ts
var serialization = __webpack_require__(48443);
;// ./src/lib/api/approvalsApi.ts







function getClient() {
    return (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
}
const approvalsApi = connectApi/* connectApi */.X.injectEndpoints({
    endpoints: (builder)=>({
            getApprovals: builder.query({
                queryFn: async ()=>{
                    try {
                        const client = getClient();
                        const req = (0,create/* create */.v)(session_pb/* ListPendingApprovalsRequestSchema */.WLA, {});
                        const response = await client.listPendingApprovals(req);
                        // Serialize to plain objects (boundary rule: no protobuf instances in Redux).
                        const approvals = response.approvals.map((a)=>(0,serialization/* toPlainObject */.V)(a));
                        return {
                            data: {
                                approvals
                            }
                        };
                    } catch (err) {
                        const msg = err instanceof Error ? err.message : "Failed to fetch approvals";
                        return {
                            error: {
                                status: -1,
                                error: msg
                            }
                        };
                    }
                },
                providesTags: [
                    "Approvals"
                ]
            }),
            resolveApproval: builder.mutation({
                queryFn: async (param)=>{
                    let { approvalId, decision, message } = param;
                    try {
                        const client = getClient();
                        const req = (0,create/* create */.v)(session_pb/* ResolveApprovalRequestSchema */.I6_, {
                            approvalId,
                            decision,
                            message
                        });
                        await client.resolveApproval(req);
                        return {
                            data: undefined
                        };
                    } catch (err) {
                        const msg = err instanceof Error ? err.message : "Failed to resolve approval";
                        return {
                            error: {
                                status: -1,
                                error: msg
                            }
                        };
                    }
                },
                invalidatesTags: [
                    "Approvals"
                ]
            })
        })
});
const { useGetApprovalsQuery, useResolveApprovalMutation } = approvalsApi;

;// ./src/lib/utils/rtkQueryError.ts
/**
 * Converts an RTK Query error value to a standard `Error | null`.
 *
 * RTK Query surfaces errors as an opaque `unknown` value. This helper
 * normalises it into a typed `Error` so callers always receive a consistent
 * `Error | null` shape rather than reimplementing the conversion inline.
 */ function toErrorOrNull(queryError) {
    if (!queryError) return null;
    const msg = typeof queryError === "object" && "error" in queryError ? String(queryError.error) : "Unknown error";
    return new Error(msg);
}

;// ./src/lib/contexts/ApprovalsContext.tsx
/* __next_internal_client_entry_do_not_use__ ApprovalsProvider,useApprovalsContext auto */ 
// +feature: approval-enter-detection



const ApprovalsContext = /*#__PURE__*/ (0,react.createContext)(null);
/** Fallback returned by useApprovalsContext when used outside ApprovalsProvider. */ const noopAsync = async ()=>{};
const FALLBACK_CONTEXT = {
    approvals: [],
    pendingCount: 0,
    loading: false,
    error: null,
    approve: noopAsync,
    deny: noopAsync,
    refresh: noopAsync,
    clearForSession: ()=>{},
    clearedSessions: new Set()
};
function ApprovalsProvider(param) {
    let { children } = param;
    // RTK Query with 5s polling — single authoritative source for the entire app
    const { data, isLoading, error: queryError, refetch } = useGetApprovalsQuery(undefined, {
        pollingInterval: 5000
    });
    const [resolveApproval] = useResolveApprovalMutation();
    const [clearedSessions, setClearedSessions] = (0,react.useState)(new Set());
    // In-flight count per session prevents premature removal when Enter is pressed twice rapidly
    const clearCountRef = (0,react.useRef)({});
    const approve = (0,react.useCallback)(async (approvalId)=>{
        // .unwrap() is required for the returned promise to actually reject on failure --
        // an RTK Query mutation trigger resolves to a {data}/{error} result object by default,
        // it never rejects on its own, which silently broke every try/catch caller expecting
        // this Promise<void> contract to reject (e.g. SessionBoard.tsx's approveNeedsReview).
        await resolveApproval({
            approvalId,
            decision: "allow"
        }).unwrap();
    }, [
        resolveApproval
    ]);
    const deny = (0,react.useCallback)(async (approvalId, message)=>{
        await resolveApproval({
            approvalId,
            decision: "deny",
            message
        }).unwrap();
    }, [
        resolveApproval
    ]);
    const refresh = (0,react.useCallback)(async ()=>{
        await refetch();
    }, [
        refetch
    ]);
    const clearForSession = (0,react.useCallback)((sessionId)=>{
        var _clearCountRef_current_sessionId;
        clearCountRef.current[sessionId] = ((_clearCountRef_current_sessionId = clearCountRef.current[sessionId]) !== null && _clearCountRef_current_sessionId !== void 0 ? _clearCountRef_current_sessionId : 0) + 1;
        setClearedSessions((prev)=>new Set(prev).add(sessionId));
        void (async ()=>{
            try {
                await refetch();
            } catch (err) {
                console.error("[ApprovalsContext] refetch failed during optimistic clear:", err);
            } finally{
                clearCountRef.current[sessionId]--;
                var _clearCountRef_current_sessionId;
                if (((_clearCountRef_current_sessionId = clearCountRef.current[sessionId]) !== null && _clearCountRef_current_sessionId !== void 0 ? _clearCountRef_current_sessionId : 0) <= 0) {
                    delete clearCountRef.current[sessionId];
                    setClearedSessions((prev)=>{
                        const next = new Set(prev);
                        next.delete(sessionId);
                        return next;
                    });
                }
            }
        })();
    }, [
        refetch
    ]);
    const filteredApprovals = (0,react.useMemo)(()=>{
        var _data_approvals;
        return ((_data_approvals = data === null || data === void 0 ? void 0 : data.approvals) !== null && _data_approvals !== void 0 ? _data_approvals : []).filter((a)=>!clearedSessions.has(a.sessionId));
    }, [
        data,
        clearedSessions
    ]);
    const error = toErrorOrNull(queryError);
    const pendingCount = filteredApprovals.length;
    const value = (0,react.useMemo)(()=>({
            approvals: filteredApprovals,
            pendingCount,
            loading: isLoading,
            error,
            approve,
            deny,
            refresh,
            clearForSession,
            clearedSessions
        }), [
        filteredApprovals,
        pendingCount,
        isLoading,
        error,
        approve,
        deny,
        refresh,
        clearForSession,
        clearedSessions
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(ApprovalsContext.Provider, {
        value: value,
        children: children
    });
}
/**
 * Returns the approvals context value.
 * Safe to call outside ApprovalsProvider — returns a no-op fallback instead of throwing.
 */ function useApprovalsContext() {
    const ctx = (0,react.useContext)(ApprovalsContext);
    return ctx !== null && ctx !== void 0 ? ctx : FALLBACK_CONTEXT;
}


/***/ }),

/***/ 48443:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   V: () => (/* binding */ toPlainObject)
/* harmony export */ });
// Converts a protobuf Message to a plain JSON-serializable object.
// Used as the serialization boundary so Redux never stores protobuf class instances.
// In @bufbuild/protobuf v2, Message is a plain TypeScript type (not a class),
// so we use JSON round-trip to strip any non-serializable values.
function toPlainObject(msg) {
    // BigInt replacer: protobuf v2 represents int64/uint64 as bigint, which JSON.stringify rejects.
    // Timestamp seconds fit safely in Number (epoch seconds won't exceed 2^53 for millennia).
    return JSON.parse(JSON.stringify(msg, (_, v)=>typeof v === "bigint" ? Number(v) : v));
}


/***/ }),

/***/ 60446:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $E: () => (/* binding */ playNotificationSound),
/* harmony export */   UM: () => (/* binding */ notificationTag),
/* harmony export */   dL: () => (/* binding */ showBrowserNotification),
/* harmony export */   fe: () => (/* binding */ closeNativeNotification),
/* harmony export */   tI: () => (/* binding */ NotificationSound),
/* harmony export */   tL: () => (/* binding */ playPriorityNotificationSound)
/* harmony export */ });
/* unused harmony exports getNotificationPreference, setNotificationPreference, requestNotificationPermission */
/* harmony import */ var _lib_notification_policy__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(35172);
/* harmony import */ var _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(81432);
/* __next_internal_client_entry_do_not_use__ notificationTag,NotificationSound,playNotificationSound,playPriorityNotificationSound,closeNativeNotification,showBrowserNotification,getNotificationPreference,setNotificationPreference,requestNotificationPermission auto */ /**
 * Notification utilities for audio alerts and browser notifications
 */ 

/**
 * Shared tag format helpers for native OS notifications.
 * Using helpers avoids duplicated format strings across callers
 * and ensures close/dedup calls use the same tag as show calls.
 */ const notificationTag = {
    approval: (approvalId)=>"approval:".concat(approvalId),
    tier1Review: (sessionId)=>"review-queue-tier1-".concat(sessionId)
};
/**
 * Notification sound types
 */ var NotificationSound = /*#__PURE__*/ function(NotificationSound) {
    NotificationSound["DING"] = "ding";
    NotificationSound["CHIME"] = "chime";
    NotificationSound["ALERT"] = "alert";
    return NotificationSound;
}({});
/**
 * Returns the shared AudioContext for the page lifetime, creating it on first use.
 * A singleton avoids accumulating OS-level audio handles — each `new AudioContext()`
 * allocates a native context that is never released unless explicitly `.close()`d.
 */ let _sharedAudioCtx = null;
function getAudioContext() {
    if (false) {}
    try {
        if (!_sharedAudioCtx || _sharedAudioCtx.state === "closed") {
            _sharedAudioCtx = new (window.AudioContext || window.webkitAudioContext)();
        }
        return _sharedAudioCtx;
    } catch (e) {
        return null;
    }
}
/**
 * Plays a notification sound using the Web Audio API
 * Falls back to a simple beep if sound synthesis fails
 */ function playNotificationSound() {
    let soundType = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : "ding";
    try {
        // Check if user has enabled notifications (localStorage preference)
        const notificationsEnabled = localStorage.getItem("notifications-enabled");
        if (notificationsEnabled === "false") {
            return;
        }
        const audioContext = getAudioContext();
        if (!audioContext) return;
        // Create oscillator for tone generation
        const oscillator = audioContext.createOscillator();
        const gainNode = audioContext.createGain();
        oscillator.connect(gainNode);
        gainNode.connect(audioContext.destination);
        // Configure sound based on type
        switch(soundType){
            case "ding":
                // Pleasant ding sound (E note)
                oscillator.frequency.value = 659.25; // E5
                oscillator.type = "sine";
                gainNode.gain.setValueAtTime(0.3, audioContext.currentTime);
                gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.5);
                oscillator.start(audioContext.currentTime);
                oscillator.stop(audioContext.currentTime + 0.5);
                break;
            case "chime":
                // Two-tone chime (C -> E)
                oscillator.frequency.value = 523.25; // C5
                oscillator.type = "sine";
                gainNode.gain.setValueAtTime(0.3, audioContext.currentTime);
                oscillator.frequency.setValueAtTime(659.25, audioContext.currentTime + 0.15); // E5
                gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.6);
                oscillator.start(audioContext.currentTime);
                oscillator.stop(audioContext.currentTime + 0.6);
                break;
            case "alert":
                // Attention-grabbing alert sound
                oscillator.frequency.value = 800;
                oscillator.type = "square";
                gainNode.gain.setValueAtTime(0.2, audioContext.currentTime);
                gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.3);
                oscillator.start(audioContext.currentTime);
                oscillator.stop(audioContext.currentTime + 0.3);
                break;
        }
    } catch (error) {
        console.warn("Failed to play notification sound:", error);
    }
}
/**
 * Plays a priority-mapped notification chime using the Web Audio API.
 * URGENT → 3 rapid beeps, HIGH → 2 beeps, MEDIUM → single chime, LOW → soft tone.
 */ function playPriorityNotificationSound(priority) {
    if ( false || !window.AudioContext) return;
    try {
        const audioCtx = getAudioContext();
        if (!audioCtx) return;
        const oscillator = audioCtx.createOscillator();
        const gainNode = audioCtx.createGain();
        oscillator.connect(gainNode);
        gainNode.connect(audioCtx.destination);
        switch(priority){
            case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_1__/* .NotificationPriority */ .AE.URGENT:
                // Rapid high-pitched alert (3 quick beeps)
                oscillator.type = "sine";
                oscillator.frequency.setValueAtTime(880, audioCtx.currentTime); // A5
                gainNode.gain.setValueAtTime(0.3, audioCtx.currentTime);
                gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.1);
                oscillator.start(audioCtx.currentTime);
                oscillator.stop(audioCtx.currentTime + 0.15);
                setTimeout(()=>{
                    const ctx = getAudioContext();
                    if (!ctx) return;
                    const osc2 = ctx.createOscillator();
                    const gain2 = ctx.createGain();
                    osc2.connect(gain2);
                    gain2.connect(ctx.destination);
                    osc2.type = "sine";
                    osc2.frequency.setValueAtTime(880, ctx.currentTime);
                    gain2.gain.setValueAtTime(0.3, ctx.currentTime);
                    gain2.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.1);
                    osc2.start();
                    osc2.stop(ctx.currentTime + 0.15);
                }, 150);
                setTimeout(()=>{
                    const ctx = getAudioContext();
                    if (!ctx) return;
                    const osc3 = ctx.createOscillator();
                    const gain3 = ctx.createGain();
                    osc3.connect(gain3);
                    gain3.connect(ctx.destination);
                    osc3.type = "sine";
                    osc3.frequency.setValueAtTime(880, ctx.currentTime);
                    gain3.gain.setValueAtTime(0.3, ctx.currentTime);
                    gain3.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.1);
                    osc3.start();
                    osc3.stop(ctx.currentTime + 0.15);
                }, 300);
                break;
            case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_1__/* .NotificationPriority */ .AE.HIGH:
                // Double beep
                oscillator.type = "sine";
                oscillator.frequency.setValueAtTime(659, audioCtx.currentTime); // E5
                gainNode.gain.setValueAtTime(0.2, audioCtx.currentTime);
                gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.15);
                oscillator.start(audioCtx.currentTime);
                oscillator.stop(audioCtx.currentTime + 0.2);
                setTimeout(()=>{
                    const ctx = getAudioContext();
                    if (!ctx) return;
                    const osc2 = ctx.createOscillator();
                    const gain2 = ctx.createGain();
                    osc2.connect(gain2);
                    gain2.connect(ctx.destination);
                    osc2.type = "sine";
                    osc2.frequency.setValueAtTime(784, ctx.currentTime); // G5
                    gain2.gain.setValueAtTime(0.2, ctx.currentTime);
                    gain2.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.15);
                    osc2.start();
                    osc2.stop(ctx.currentTime + 0.2);
                }, 200);
                break;
            case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_1__/* .NotificationPriority */ .AE.MEDIUM:
                // Single soft chime
                oscillator.type = "sine";
                oscillator.frequency.setValueAtTime(523, audioCtx.currentTime); // C5
                gainNode.gain.setValueAtTime(0.15, audioCtx.currentTime);
                gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.3);
                oscillator.start(audioCtx.currentTime);
                oscillator.stop(audioCtx.currentTime + 0.4);
                break;
            case _gen_session_v1_types_pb__WEBPACK_IMPORTED_MODULE_1__/* .NotificationPriority */ .AE.LOW:
                // Very soft, low tone
                oscillator.type = "sine";
                oscillator.frequency.setValueAtTime(392, audioCtx.currentTime); // G4
                gainNode.gain.setValueAtTime(0.08, audioCtx.currentTime);
                gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.2);
                oscillator.start(audioCtx.currentTime);
                oscillator.stop(audioCtx.currentTime + 0.25);
                break;
            default:
                // Default medium chime
                oscillator.type = "sine";
                oscillator.frequency.setValueAtTime(523, audioCtx.currentTime);
                gainNode.gain.setValueAtTime(0.1, audioCtx.currentTime);
                gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.3);
                oscillator.start(audioCtx.currentTime);
                oscillator.stop(audioCtx.currentTime + 0.4);
        }
    } catch (e) {
        console.warn("Failed to play notification sound:", e);
    }
}
/**
 * Tracks open native Notification handles by tag for dedup and auto-close (FR-3, FR-4).
 *
 * macOS Notification Center limitation: notification.close() dismisses the banner
 * but cannot remove NC entries that the user has already swiped into the NC tray.
 * This is a known browser/OS limitation; no workaround is attempted here.
 */ const activeNativeNotifications = new Map();
/**
 * Programmatically closes the native OS notification with the given tag.
 * Removes the entry from activeNativeNotifications so the auto-close timer
 * won't fire on a stale reference.
 *
 * Note: on macOS, notification.close() dismisses the banner but cannot
 * remove entries already swiped into the Notification Center tray.
 * This is a known OS/browser limitation.
 */ function closeNativeNotification(tag) {
    const notif = activeNativeNotifications.get(tag);
    if (!notif) return;
    try {
        notif.close();
    } catch (e) {
    // Expected on macOS NC tray entries
    }
    activeNativeNotifications.delete(tag);
}
/**
 * Shows a browser notification if permission is granted.
 * Falls back to audio-only if notifications are not supported or denied.
 *
 * Implements:
 * - FR-3: Auto-close via setTimeout after autoCloseMs
 * - FR-4: Close-before-open dedup via activeNativeNotifications Map
 */ async function showBrowserNotification(title, options) {
    // Check if notifications are enabled
    const notificationsEnabled = localStorage.getItem("notifications-enabled");
    if (notificationsEnabled === "false") {
        return;
    }
    // Check if browser supports notifications
    if (!("Notification" in window)) {
        console.warn("Browser does not support notifications");
        return;
    }
    // Request permission if needed
    if (Notification.permission === "default") {
        await Notification.requestPermission();
    }
    // Show notification if permission granted
    if (Notification.permission === "granted") {
        var _options_tag;
        const tag = (_options_tag = options === null || options === void 0 ? void 0 : options.tag) !== null && _options_tag !== void 0 ? _options_tag : "__untagged__";
        // Close previous notification for this tag (FR-4)
        try {
            var _activeNativeNotifications_get;
            (_activeNativeNotifications_get = activeNativeNotifications.get(tag)) === null || _activeNativeNotifications_get === void 0 ? void 0 : _activeNativeNotifications_get.close();
        } catch (e) {
            if (false) {}
        }
        // Strip autoCloseMs from native NotificationOptions before passing to constructor
        const { autoCloseMs: _autoCloseMs, ...notifOptions } = options !== null && options !== void 0 ? options : {};
        const autoCloseMs = _autoCloseMs !== null && _autoCloseMs !== void 0 ? _autoCloseMs : (options === null || options === void 0 ? void 0 : options.requireInteraction) ? _lib_notification_policy__WEBPACK_IMPORTED_MODULE_0__/* .NATIVE_ACTIONABLE_TTL_MS */ .v_ : _lib_notification_policy__WEBPACK_IMPORTED_MODULE_0__/* .NATIVE_MEDIUM_TTL_MS */ .o$;
        const notif = new Notification(title, {
            icon: "/favicon.ico",
            badge: "/favicon.ico",
            ...notifOptions
        });
        activeNativeNotifications.set(tag, notif);
        // Auto-close (FR-3)
        const timerId = setTimeout(()=>{
            try {
                notif.close();
            } catch (e) {
                if (false) {}
            }
            if (activeNativeNotifications.get(tag) === notif) {
                activeNativeNotifications.delete(tag);
            }
        }, autoCloseMs);
        // Clean up map entry when the OS closes the notification
        notif.onclose = ()=>{
            clearTimeout(timerId);
            if (activeNativeNotifications.get(tag) === notif) {
                activeNativeNotifications.delete(tag);
            }
        };
    }
}
/**
 * Gets the current notification preference from localStorage
 */ function getNotificationPreference() {
    const stored = localStorage.getItem("notifications-enabled");
    // Default to enabled if not set
    return stored !== "false";
}
/**
 * Sets the notification preference in localStorage
 */ function setNotificationPreference(enabled) {
    localStorage.setItem("notifications-enabled", enabled.toString());
}
/**
 * Requests notification permission from the browser
 * Returns true if permission was granted
 */ async function requestNotificationPermission() {
    if (!("Notification" in window)) {
        return false;
    }
    if (Notification.permission === "granted") {
        return true;
    }
    const permission = await Notification.requestPermission();
    return permission === "granted";
}


/***/ }),

/***/ 63991:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   r: () => (/* binding */ useFocusTrap)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);

const FOCUSABLE_SELECTORS = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
function useFocusTrap(ref, isActive, triggerRef) {
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        var // Move focus into the container
        _getFocusable_;
        if (!isActive || !ref.current) return;
        const container = ref.current;
        // Ensure the "no focusable descendants" fallback below can actually
        // move focus onto the container itself — a plain <div> without a
        // tabindex silently no-ops .focus().
        if (!container.hasAttribute("tabindex")) {
            container.setAttribute("tabindex", "-1");
        }
        // Computed fresh on every Tab press (not cached once at activation) so a
        // control that becomes disabled/enabled after the trap activates (e.g. a
        // dialog's input+Send disabled while its own async action is in flight)
        // doesn't leave a stale "first"/"last" pointing at an element that's no
        // longer in the tab order — chasing a stale pointer would fail to
        // preventDefault, letting Tab/Shift+Tab escape the container to whatever
        // real browser tab order finds next (confirmed via a real-browser
        // regression: SessionActionsOverflow's "Give Direction" dialog disables
        // its input/Send while steering, and a stale snapshot let Shift+Tab from
        // the still-enabled Cancel button escape the dialog entirely).
        const getFocusable = ()=>Array.from(container.querySelectorAll(FOCUSABLE_SELECTORS)).filter((el)=>!el.closest("[aria-hidden='true']"));
        (_getFocusable_ = getFocusable()[0]) === null || _getFocusable_ === void 0 ? void 0 : _getFocusable_.focus();
        const handleKeyDown = (e)=>{
            if (e.key !== "Tab") return;
            const focusable = getFocusable();
            if (focusable.length === 0) {
                e.preventDefault();
                return;
            }
            const first = focusable[0];
            const last = focusable[focusable.length - 1];
            if (e.shiftKey) {
                if (document.activeElement === first) {
                    e.preventDefault();
                    last === null || last === void 0 ? void 0 : last.focus();
                }
            } else {
                if (document.activeElement === last) {
                    e.preventDefault();
                    first === null || first === void 0 ? void 0 : first.focus();
                }
            }
        };
        // Safety net for widgets that rewrite their own tabindex on focus (e.g.
        // react-arborist's FileTree), which can let native Tab slip past
        // handleKeyDown's first/last check entirely — backlog item
        // 4a1f73c4-5558-41f8-9860-8508fb874fcc.
        const handleFocusIn = (e)=>{
            var _targetEl_closest;
            const target = e.target;
            if (!target || container.contains(target)) return;
            // A deliberate close-restore (e.g. a sibling trap sharing this trigger,
            // like SessionActionsOverflow's Program Picker over its overflow menu),
            // not an escape.
            if (target === (triggerRef === null || triggerRef === void 0 ? void 0 : triggerRef.current)) return;
            // Focus moved into another legitimate dialog — e.g. React's autoFocus
            // lands here before that dialog's own trap effect has registered.
            const targetEl = target;
            if ((_targetEl_closest = targetEl.closest) === null || _targetEl_closest === void 0 ? void 0 : _targetEl_closest.call(targetEl, '[role="dialog"], [role="alertdialog"]')) return;
            var _getFocusable_;
            ((_getFocusable_ = getFocusable()[0]) !== null && _getFocusable_ !== void 0 ? _getFocusable_ : container).focus();
        };
        document.addEventListener("keydown", handleKeyDown);
        document.addEventListener("focusin", handleFocusIn);
        const triggerEl = triggerRef === null || triggerRef === void 0 ? void 0 : triggerRef.current;
        return ()=>{
            document.removeEventListener("keydown", handleKeyDown);
            document.removeEventListener("focusin", handleFocusIn);
            // Return focus to the trigger element when the trap is deactivated
            triggerEl === null || triggerEl === void 0 ? void 0 : triggerEl.focus();
        };
    }, [
        isActive,
        ref,
        triggerRef
    ]);
}


/***/ }),

/***/ 87963:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  ol: () => (/* binding */ GlobalSessionServiceProvider),
  MS: () => (/* binding */ useSessionServiceContext)
});

// UNUSED EXPORTS: SessionServiceContext

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionService.ts + 1 modules
var useSessionService = __webpack_require__(73117);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ./src/lib/utils/notificationMapping.ts
var notificationMapping = __webpack_require__(71249);
// EXTERNAL MODULE: ./src/lib/notification-policy.ts
var notification_policy = __webpack_require__(35172);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/lib/utils/notifications.ts
var notifications = __webpack_require__(60446);
;// ./src/lib/hooks/useSessionNotifications.ts
/* __next_internal_client_entry_do_not_use__ useSessionNotifications auto */ 








/**
 * Notification types that should only appear in history — no toast, no sound.
 * These are informational events where interrupting the user adds no value.
 */ const HISTORY_ONLY_TYPES = new Set([
    types_pb/* NotificationType */._n.TASK_COMPLETE,
    types_pb/* NotificationType */._n.PROCESS_FINISHED,
    types_pb/* NotificationType */._n.PROCESS_STARTED,
    types_pb/* NotificationType */._n.STATUS_CHANGE,
    types_pb/* NotificationType */._n.INFO,
    types_pb/* NotificationType */._n.DEBUG
]);
/**
 * Calls resolveApproval RPC to allow or deny a pending tool use. A failure is
 * logged and rethrown so the toast can show "Could not approve - Retry".
 */ async function resolveApproval(approvalId, decision) {
    try {
        const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
        // One-shot mutation triggered by an explicit user action (Approve/Deny
        // click on a toast), not tied to a mount/effect.
        // abort-signal-exempt
        await client.resolveApproval({
            approvalId,
            decision
        });
    } catch (error) {
        console.error("[resolveApproval] Failed to resolve approval ".concat(approvalId, ":"), error);
        throw error;
    }
}
/**
 * Calls the FocusWindow API to bring an application window to front
 */ async function focusWindow(bundleId, appName) {
    if (!bundleId && !appName) return;
    try {
        const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
        // One-shot action triggered by an explicit user click ("focus window"
        // on a toast), not tied to a mount/effect.
        // abort-signal-exempt
        const response = await client.focusWindow({
            bundleId: bundleId,
            appName: appName
        });
        // Check if the server reported a permissions issue
        if (!response.success && response.message) {
            console.warn("Focus window failed:", response.message);
        }
    } catch (error) {
        console.warn("Failed to focus window:", error);
    }
}
/**
 * Hook that handles session notification events from the server.
 * Creates notification toasts and plays audio chimes based on priority.
 *
 * @returns A callback to handle NotificationEvent from useSessionService
 */ function useSessionNotifications() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { enableAudio = true, onViewSession } = options;
    const { addNotification, addToHistoryOnly } = (0,NotificationContext/* useNotifications */.E$)();
    // Use refs to avoid recreating callback when dependencies change
    const enableAudioRef = (0,react.useRef)(enableAudio);
    const onViewSessionRef = (0,react.useRef)(onViewSession);
    // Dedup cache: maps "sessionId:notificationType" -> timestamp of last shown toast
    const recentToastKeys = (0,react.useRef)(new Map());
    (0,react.useEffect)(()=>{
        enableAudioRef.current = enableAudio;
    }, [
        enableAudio
    ]);
    (0,react.useEffect)(()=>{
        onViewSessionRef.current = onViewSession;
    }, [
        onViewSession
    ]);
    const handleNotification = (0,react.useCallback)((event)=>{
        var _event_metadata, _event_metadata1, _event_metadata2, _event_metadata3, _event_metadata4, _event_metadata5;
        // --- Toast deduplication ---
        // Within a 10-second window, suppress audio and native notifications for
        // repeat (sessionId, notificationType) events. However, visible toasts are
        // still refreshed with the latest content so that rapidly-updating system
        // alerts (e.g. fork-pressure) replace their stale toast rather than going
        // silent until the window expires.
        const dedupKey = "".concat(event.sessionId, ":").concat(event.notificationType);
        const now = Date.now();
        // Prune stale entries before reading — ensures lastShown reflects the
        // post-prune state, so a boundary-case entry doesn't linger in the map.
        for (const [key, ts] of recentToastKeys.current){
            if (now - ts >= notification_policy/* TOAST_DEDUP_WINDOW_MS */.B2) {
                recentToastKeys.current.delete(key);
            }
        }
        const lastShown = recentToastKeys.current.get(dedupKey);
        // Never suppress approval_needed or question notifications — each one blocks Claude and requires a response.
        const isApproval = event.notificationType === types_pb/* NotificationType */._n.APPROVAL_NEEDED || event.notificationType === types_pb/* NotificationType */._n.INPUT_REQUIRED;
        const isDuplicate = !isApproval && !!lastShown && now - lastShown < notification_policy/* TOAST_DEDUP_WINDOW_MS */.B2;
        // Backlog-item notifications carry metadata.item_id and, as of the coalescing-key fix,
        // also carry the backlog item's ID as event.sessionId (not a real live session). "View
        // Session" must not navigate using that ID — it isn't a session and /?session=<itemId>
        // would 404. Route these through the item_id Link in NotificationPanel/NotificationsPage
        // instead by leaving onView unset here.
        const isBacklogItemNotification = !!((_event_metadata = event.metadata) === null || _event_metadata === void 0 ? void 0 : _event_metadata["item_id"]);
        // History-only types: no toast, no sound — just record in the history panel.
        // Duplicates are fully suppressed (no visible toast to refresh).
        if (HISTORY_ONLY_TYPES.has(event.notificationType)) {
            if (isDuplicate) return;
            addToHistoryOnly({
                id: event.notificationId || undefined,
                sessionId: event.sessionId,
                // Empty, not "Unknown Session": a placeholder here would win the
                // NotificationPanel fallback chain over the event's real title for
                // sessionless events (e.g. notifyReworkCapHit).
                sessionName: event.sessionName || "",
                title: event.title,
                message: event.message,
                priority: (0,notificationMapping/* mapPriority */.vV)(event.priority),
                notificationType: (0,notificationMapping/* mapNotificationType */.QE)(event.notificationType),
                metadata: event.metadata,
                isPendingDecision: event.isPendingDecision,
                onView: onViewSessionRef.current && !isBacklogItemNotification ? ()=>{
                    var _onViewSessionRef_current;
                    return (_onViewSessionRef_current = onViewSessionRef.current) === null || _onViewSessionRef_current === void 0 ? void 0 : _onViewSessionRef_current.call(onViewSessionRef, event.sessionId, event.notificationId || undefined);
                } : undefined
            });
            return;
        }
        // Extract source app metadata from the event
        const sourceApp = (_event_metadata1 = event.metadata) === null || _event_metadata1 === void 0 ? void 0 : _event_metadata1["source_app"];
        const sourceBundleId = (_event_metadata2 = event.metadata) === null || _event_metadata2 === void 0 ? void 0 : _event_metadata2["source_bundle"];
        const sourceWorkingDir = (_event_metadata3 = event.metadata) === null || _event_metadata3 === void 0 ? void 0 : _event_metadata3["cwd"];
        const sourceProject = (_event_metadata4 = event.metadata) === null || _event_metadata4 === void 0 ? void 0 : _event_metadata4["source_project"];
        const approvalId = (_event_metadata5 = event.metadata) === null || _event_metadata5 === void 0 ? void 0 : _event_metadata5["approval_id"];
        // Build the notification data with all available fields
        const notificationData = {
            // The server's id names the same toast in every open tab (cross-tab bulk sync).
            id: event.notificationId || undefined,
            sessionId: event.sessionId,
            // See the comment on the history-only branch above.
            sessionName: event.sessionName || "",
            title: event.title,
            message: event.message,
            priority: (0,notificationMapping/* mapPriority */.vV)(event.priority),
            notificationType: (0,notificationMapping/* mapNotificationType */.QE)(event.notificationType),
            sourceApp: sourceApp,
            sourceBundleId: sourceBundleId,
            sourceWorkingDir: sourceWorkingDir,
            sourceProject: sourceProject,
            metadata: event.metadata,
            isPendingDecision: event.isPendingDecision,
            onView: onViewSessionRef.current && !isBacklogItemNotification ? ()=>{
                var _onViewSessionRef_current;
                return (_onViewSessionRef_current = onViewSessionRef.current) === null || _onViewSessionRef_current === void 0 ? void 0 : _onViewSessionRef_current.call(onViewSessionRef, event.sessionId, event.notificationId || undefined);
            } : undefined,
            // Add focus window handler if we have source app info
            onFocusWindow: sourceApp || sourceBundleId ? ()=>focusWindow(sourceBundleId, sourceApp) : undefined,
            // Attach approve/deny callbacks for tool-use approval requests
            onApprove: approvalId ? ()=>resolveApproval(approvalId, "allow") : undefined,
            onDeny: approvalId ? ()=>resolveApproval(approvalId, "deny") : undefined
        };
        // Duplicate visible-toast event: refresh the existing toast with updated
        // content (addNotification replaces by sessionId, not stacks) but skip
        // audio and native notification to avoid spamming the user.
        if (isDuplicate) {
            addNotification(notificationData);
            return;
        }
        recentToastKeys.current.set(dedupKey, now);
        // Play audio chime based on priority
        if (enableAudioRef.current) {
            (0,notifications/* playPriorityNotificationSound */.tL)(event.priority);
        }
        // Add visual notification
        addNotification(notificationData);
        // Native notification (FR-6): fire alongside toast for non-history types.
        // Guard: only when already granted — we do NOT call requestPermission() here.
        // Requesting permission mid-session triggers a disruptive OS prompt; permission
        // is requested proactively via requestNotificationPermission() in the settings flow.
        if ( true && "Notification" in window && Notification.permission === "granted") {
            var _event_metadata6;
            const nativeTag = isApproval && ((_event_metadata6 = event.metadata) === null || _event_metadata6 === void 0 ? void 0 : _event_metadata6["approval_id"]) ? "approval:".concat(event.metadata["approval_id"]) : "".concat(event.sessionId, ":").concat((0,notificationMapping/* mapNotificationType */.QE)(event.notificationType));
            var _event_message;
            // Approval body is redacted in the OS tray: shell commands and file paths from
            // tool inputs must not appear in notification center, screen recordings, or
            // lock-screen previews. The in-app toast shows the full content.
            const nativeBody = isApproval ? "Open Stapler Squad to review and approve" : (_event_message = event.message) !== null && _event_message !== void 0 ? _event_message : undefined;
            void (0,notifications/* showBrowserNotification */.dL)(event.title, {
                body: nativeBody,
                tag: nativeTag,
                autoCloseMs: isApproval ? notification_policy/* NATIVE_ACTIONABLE_TTL_MS */.v_ : (0,notification_policy/* nativeAutoCloseMs */.vk)(event.priority),
                requireInteraction: isApproval
            });
        }
    }, [
        addNotification,
        addToHistoryOnly
    ]);
    return handleNotification;
}

// EXTERNAL MODULE: ./src/lib/contexts/AuthContext.tsx
var AuthContext = __webpack_require__(64792);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
;// ./src/lib/contexts/SessionServiceContext.tsx
/* __next_internal_client_entry_do_not_use__ SessionServiceContext,GlobalSessionServiceProvider,useSessionServiceContext auto */ 








const SessionServiceContext = /*#__PURE__*/ (0,react.createContext)(null);
/**
 * GlobalSessionServiceProvider mounts a single persistent watchSessions connection
 * that lives at the layout level. This ensures:
 *  - Notification toasts appear on every page, not just the main session list.
 *  - The streaming connection is not torn down on page navigation.
 *
 * page.tsx reads session state from Redux (which this provider keeps up-to-date)
 * and calls CRUD methods via useSessionServiceContext().
 */ function GlobalSessionServiceProvider(param) {
    let { children } = param;
    const { authEnabled, authenticated, loading: authLoading } = (0,AuthContext/* useAuth */.A)();
    const { refreshHistory, markAsReadBySessionId, removeToastByApprovalId, removeToastBySessionId } = (0,NotificationContext/* useNotifications */.E$)();
    const router = (0,nav/* useRouter */.KA)();
    // Navigate to the session detail when user clicks "View" on a toast.
    // Works from any page — redirects to /?session=<id> if not already on home.
    const onViewSession = (0,react.useCallback)((sessionId, notificationId)=>{
        // notification=<id> lets a deleted-session card show the notification's own text (Story 5.3).
        const notification = notificationId ? "&notification=".concat(encodeURIComponent(notificationId)) : "";
        router.push("/?session=".concat(encodeURIComponent(sessionId), "&tab=terminal").concat(notification));
    }, [
        router
    ]);
    const handleNotification = useSessionNotifications({
        enableAudio: true,
        onViewSession
    });
    const service = (0,useSessionService/* useSessionService */.D)({
        baseUrl: (0,config/* getApiBaseUrl */.h)(),
        autoWatch: true,
        enabled: !authLoading && (!authEnabled || authenticated),
        onNotification: handleNotification,
        onReconnect: refreshHistory,
        onApprovalResponse: (approvalId, sessionId)=>{
            removeToastByApprovalId(approvalId);
            // Close native OS notification for this approval
            (0,notifications/* closeNativeNotification */.fe)(notifications/* notificationTag */.UM.approval(approvalId));
            // Also close any review-queue tier-1 notification for this session
            (0,notifications/* closeNativeNotification */.fe)(notifications/* notificationTag */.UM.tier1Review(sessionId));
            void refreshHistory();
        },
        onSessionDeleted: (sessionId)=>{
            // Mark notification history as read for the deleted session.
            markAsReadBySessionId(sessionId);
            // Remove any active approval toasts for the deleted session so stale
            // Approve/Deny buttons are not shown after the session is gone.
            removeToastBySessionId(sessionId);
        }
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionServiceContext.Provider, {
        value: service,
        children: children
    });
}
function useSessionServiceContext() {
    const ctx = (0,react.useContext)(SessionServiceContext);
    if (!ctx) {
        throw new Error("useSessionServiceContext must be used within GlobalSessionServiceProvider");
    }
    return ctx;
}


/***/ })

}]);
//# sourceMappingURL=8272-d6a8c23816aff33d.js.map