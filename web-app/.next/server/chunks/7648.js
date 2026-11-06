"use strict";
exports.id = 7648;
exports.ids = [7648];
exports.modules = {

/***/ 67648:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   HH: () => (/* binding */ useDismissFinding),
/* harmony export */   Ih: () => (/* binding */ useSessionTurnTimeline),
/* harmony export */   VT: () => (/* binding */ useInsightsSummary),
/* harmony export */   xk: () => (/* binding */ useSessionDetail)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(4435);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(73396);
/* harmony import */ var _gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(87948);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(7094);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(42865);
// +feature: insights-dashboard
/* __next_internal_client_entry_do_not_use__ useInsightsSummary,useSessionDetail,useSessionTurnTimeline,useDismissFinding auto */ 






/** Hook that fetches the InsightsSummary and subscribes to live updates via WatchInsights. */ function useInsightsSummary(filters = {}) {
    const [summary, setSummary] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    const [isLiveUpdating, setIsLiveUpdating] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const abortWatchRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const fetchCountRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(0);
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__/* .createClient */ .UU)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__/* .InsightsService */ .VJ, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getWatchTransport */ .VZ)()), []);
    const fetchSummary = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        const fetchId = ++fetchCountRef.current;
        setLoading(true);
        setError(null);
        try {
            const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__/* .GetInsightsSummaryRequestSchema */ .s7, {
                includeOrphans: filters.includeOrphans ?? true,
                ...filters.modelFilter && {
                    modelFilter: filters.modelFilter
                },
                ...filters.sessionIdFilter && {
                    sessionIdFilter: filters.sessionIdFilter
                },
                ...filters.from && {
                    from: (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(filters.from)
                },
                ...filters.to && {
                    to: (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(filters.to)
                }
            });
            const res = await client.getInsightsSummary(req);
            if (fetchId === fetchCountRef.current) {
                setSummary(res);
            }
        } catch (err) {
            if (fetchId === fetchCountRef.current) {
                setError(err instanceof Error ? err.message : "Failed to load insights");
            }
        } finally{
            if (fetchId === fetchCountRef.current) {
                setLoading(false);
            }
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        filters.includeOrphans,
        filters.modelFilter,
        filters.sessionIdFilter,
        filters.from,
        filters.to
    ]);
    const INITIAL_RETRY_DELAY_MS = 1000;
    const MAX_RETRY_DELAY_MS = 30000;
    /** Subscribe to WatchInsights with exponential-backoff reconnect. */ const startWatch = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        if (abortWatchRef.current) {
            abortWatchRef.current.abort();
        }
        const abort = new AbortController();
        abortWatchRef.current = abort;
        (async ()=>{
            let retryDelay = INITIAL_RETRY_DELAY_MS;
            while(!abort.signal.aborted){
                try {
                    const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__/* .WatchInsightsRequestSchema */ .dQ, {
                        ...filters.from && {
                            from: (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(filters.from)
                        },
                        ...filters.to && {
                            to: (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_5__/* .timestampFromDate */ .LI)(filters.to)
                        }
                    });
                    const stream = client.watchInsights(req, {
                        signal: abort.signal
                    });
                    retryDelay = INITIAL_RETRY_DELAY_MS;
                    for await (const event of stream){
                        if (abort.signal.aborted) break;
                        if (event.eventType === "update" && event.session) {
                            setIsLiveUpdating(true);
                            setSummary((prev)=>{
                                if (!prev) return prev;
                                // If time filter is active, skip sessions outside the filter window
                                if (filters.from || filters.to) {
                                    const lastTs = event.session.lastMessageAt;
                                    if (lastTs) {
                                        const ts = Number(lastTs.seconds) * 1000;
                                        if (filters.from && ts < filters.from.getTime()) return prev;
                                        if (filters.to && ts > filters.to.getTime()) return prev;
                                    }
                                }
                                const key = event.session.conversationId || event.session.sessionId;
                                const idx = prev.sessions.findIndex((s)=>(s.conversationId || s.sessionId) === key);
                                if (idx === -1) {
                                    return {
                                        ...prev,
                                        sessions: [
                                            ...prev.sessions,
                                            event.session
                                        ]
                                    };
                                }
                                const sessions = [
                                    ...prev.sessions
                                ];
                                sessions[idx] = event.session;
                                return {
                                    ...prev,
                                    sessions
                                };
                            });
                            setIsLiveUpdating(false);
                        } else if (event.eventType === "parse_complete") {
                            setIsLiveUpdating(true);
                            await fetchSummary();
                            setIsLiveUpdating(false);
                        }
                    }
                } catch (err) {
                    if (!abort.signal.aborted) {
                        console.error("[WatchInsights] stream error, reconnecting:", err);
                    }
                }
                if (!abort.signal.aborted) {
                    await new Promise((r)=>setTimeout(r, retryDelay));
                    retryDelay = Math.min(retryDelay * 2, MAX_RETRY_DELAY_MS);
                }
            }
        })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        fetchSummary,
        filters.from,
        filters.to
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        fetchSummary();
        startWatch();
        return ()=>{
            abortWatchRef.current?.abort();
        };
    }, [
        fetchSummary,
        startWatch
    ]);
    return {
        summary,
        loading,
        isLiveUpdating,
        error,
        refetch: fetchSummary
    };
}
/**
 * Hook that fetches a single session's SessionTokenSummary by ID, for the
 * deep-linkable /insights/session-detail?sessionId= route (Epic 1.4, Story
 * 1.4.3).
 * Sets both sessionIdFilter and conversationIdFilter to the same value so an
 * orphan session (empty sessionId) is still reachable, and includeOrphans so
 * it isn't filtered out. Deliberately omits from/to — the route must resolve
 * regardless of the dashboard's global date range at the time the link was
 * created (bookmarkability, per research/features.md §5).
 */ function useSessionDetail(sessionId) {
    const [summary, setSummary] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__/* .createClient */ .UU)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__/* .InsightsService */ .VJ, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)()), []);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        let cancelled = false;
        if (!sessionId) {
            // An empty sessionId (e.g. "?sessionId=" absent) is not "no filter" to
            // this hook, unlike the server's GetInsightsSummary RPC, which treats an
            // empty sessionIdFilter as unset and would return every session,
            // making res.sessions[0] an arbitrary session's data. Short-circuit to
            // a not-found state instead of issuing the request.
            setSummary(null);
            setLoading(false);
            setError(null);
            return;
        }
        setLoading(true);
        setError(null);
        (async ()=>{
            try {
                const req = (0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__/* .GetInsightsSummaryRequestSchema */ .s7, {
                    sessionIdFilter: sessionId,
                    conversationIdFilter: sessionId,
                    includeOrphans: true
                });
                const res = await client.getInsightsSummary(req);
                if (!cancelled) setSummary(res.sessions[0] ?? null);
            } catch (err) {
                if (!cancelled) {
                    setError(err instanceof Error ? err.message : "Failed to load session");
                }
            } finally{
                if (!cancelled) setLoading(false);
            }
        })();
        return ()=>{
            cancelled = true;
        };
    }, [
        sessionId,
        client
    ]);
    return {
        summary,
        loading,
        error
    };
}
/**
 * Hook that lazily fetches per-turn token stats for a single session, keyed on
 * conversationId. No-ops (empty turns, not loading) when conversationId is
 * falsy — used by SessionDetailDrawer, fetched only when the drawer is open
 * for a given session, never batched with the list RPCs.
 */ function useSessionTurnTimeline(conversationId) {
    const [turns, setTurns] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)([]);
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__/* .createClient */ .UU)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__/* .InsightsService */ .VJ, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)()), []);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!conversationId) {
            setTurns([]);
            setLoading(false);
            return;
        }
        let cancelled = false;
        setLoading(true);
        setError(null);
        (async ()=>{
            try {
                const res = await client.getSessionTurnTimeline((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__/* .GetSessionTurnTimelineRequestSchema */ .H5, {
                    conversationId
                }));
                if (!cancelled) setTurns(res.turns);
            } catch (err) {
                if (!cancelled) {
                    console.error("[GetSessionTurnTimeline] fetch error:", err);
                    setError(err instanceof Error ? err.message : "Failed to load turn timeline");
                }
            } finally{
                if (!cancelled) setLoading(false);
            }
        })();
        return ()=>{
            cancelled = true;
        };
    }, [
        conversationId,
        client
    ]);
    return {
        turns,
        loading,
        error
    };
}
/**
 * Hook that dismisses a WasteFinding by its stable finding_id (see
 * ComputeFindingID, session/tokens/findings.go), so it's excluded from future
 * GetInsightsSummary responses. The caller (FindingsPanel) is responsible for
 * removing the card from its own rendered list — this hook only performs the
 * RPC call.
 */ function useDismissFinding() {
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_2__/* .createClient */ .UU)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__/* .InsightsService */ .VJ, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_3__/* .getConnectTransport */ .qW)()), []);
    const dismissFinding = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (findingId, sessionId, conversationId, findingType)=>{
        await client.dismissFinding((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_insights_pb__WEBPACK_IMPORTED_MODULE_1__/* .DismissFindingRequestSchema */ .O5, {
            findingId,
            sessionId,
            conversationId,
            findingType
        }));
    }, [
        client
    ]);
    return {
        dismissFinding
    };
}


/***/ }),

/***/ 87948:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AI: () => (/* binding */ Severity),
/* harmony export */   H5: () => (/* binding */ GetSessionTurnTimelineRequestSchema),
/* harmony export */   Mr: () => (/* binding */ ActivityType),
/* harmony export */   O5: () => (/* binding */ DismissFindingRequestSchema),
/* harmony export */   VJ: () => (/* binding */ InsightsService),
/* harmony export */   dQ: () => (/* binding */ WatchInsightsRequestSchema),
/* harmony export */   s7: () => (/* binding */ GetInsightsSummaryRequestSchema)
/* harmony export */ });
/* unused harmony exports file_session_v1_insights, WasteFindingSchema, SessionTokenSummarySchema, TopToolEntrySchema, DailyTokenBucketSchema, ModelBreakdownSchema, ActivityCostBreakdownSchema, ItemRoleCostSchema, RoleCostBreakdownSchema, TopEntrySchema, GetInsightsSummaryResponseSchema, ListSessionTokensRequestSchema, ListSessionTokensResponseSchema, InsightsEventSchema, TurnTokenStatSchema, GetSessionTurnTimelineResponseSchema, DismissFindingResponseSchema, ContextSampleSchema, CompactionEventSchema, ContextCeilingSummarySchema, GetContextHistoryRequestSchema, GetContextHistoryResponseSchema, ListSessionsByCeilingTimeRequestSchema, ListSessionsByCeilingTimeResponseSchema, GetCompactionStatsRequestSchema, GetCompactionStatsResponseSchema, SeveritySchema, FindingType, FindingTypeSchema, ActivityTypeSchema */
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(89974);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(67793);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(90477);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21657);
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/insights.proto (package session.v1, syntax proto3)
/* eslint-disable */ 

/**
 * Describes the file session/v1/insights.proto.
 */ const file_session_v1_insights = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__/* .fileDesc */ .w)("ChlzZXNzaW9uL3YxL2luc2lnaHRzLnByb3RvEgpzZXNzaW9uLnYxItIBCgxXYXN0ZUZpbmRpbmcSLQoMZmluZGluZ190eXBlGAEgASgOMhcuc2Vzc2lvbi52MS5GaW5kaW5nVHlwZRImCghzZXZlcml0eRgCIAEoDjIULnNlc3Npb24udjEuU2V2ZXJpdHkSGQoRZG9sbGFyX2ltcGFjdF91c2QYAyABKAESEgoKc2Vzc2lvbl9pZBgEIAEoCRIXCg9jb252ZXJzYXRpb25faWQYBSABKAkSDwoHbWVzc2FnZRgGIAEoCRISCgpmaW5kaW5nX2lkGAcgASgJIpEGChNTZXNzaW9uVG9rZW5TdW1tYXJ5EhIKCnNlc3Npb25faWQYASABKAkSFwoPY29udmVyc2F0aW9uX2lkGAIgASgJEhQKDHByb2plY3RfcGF0aBgDIAEoCRIVCg1wcmltYXJ5X21vZGVsGAQgASgJEhoKEnRvdGFsX2lucHV0X3Rva2VucxgFIAEoAxIbChN0b3RhbF9vdXRwdXRfdG9rZW5zGAYgASgDEh0KFWNhY2hlX2NyZWF0aW9uX3Rva2VucxgHIAEoAxIZChFjYWNoZV9yZWFkX3Rva2VucxgIIAEoAxIaChJlc3RpbWF0ZWRfY29zdF91c2QYCSABKAESFgoOY2FjaGVfaGl0X3JhdGUYCiABKAESFQoNbWVzc2FnZV9jb3VudBgLIAEoBRI0ChBmaXJzdF9tZXNzYWdlX2F0GAwgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBIzCg9sYXN0X21lc3NhZ2VfYXQYDSABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEhEKCWlzX29ycGhhbhgOIAEoCBIZChFza2lsbF9hY3RpdmF0aW9ucxgPIAMoCRIrCgl0b3BfdG9vbHMYECADKAsyGC5zZXNzaW9uLnYxLlRvcFRvb2xFbnRyeRIXCg91bnByaWNlZF9tb2RlbHMYESADKAkSFQoNY2FjaGVfcm9pX3VzZBgSIAEoARIYCgt3YXN0ZV9zY29yZRgTIAEoAUgAiAEBEi8KDWFjdGl2aXR5X3R5cGUYFCABKA4yGC5zZXNzaW9uLnYxLkFjdGl2aXR5VHlwZRIMCgR0YWdzGBUgAygJEhQKDHNlc3Npb25fcm9sZRgWIAEoCRIWCg5pbnB1dF9jb3N0X3VzZBgXIAEoARIXCg9vdXRwdXRfY29zdF91c2QYGCABKAESHwoXY2FjaGVfY3JlYXRpb25fY29zdF91c2QYGSABKAESGwoTY2FjaGVfcmVhZF9jb3N0X3VzZBgaIAEoAUIOCgxfd2FzdGVfc2NvcmUikQEKDFRvcFRvb2xFbnRyeRIRCgl0b29sX25hbWUYASABKAkSEgoKY2FsbF9jb3VudBgCIAEoBRISCgptY3Bfc2VydmVyGAMgASgJEhAKCGNvc3RfdXNkGAQgASgBEh0KFWNvc3RfbWF5X2RvdWJsZV9jb3VudBgFIAEoCBIVCg1jb3N0X3VucHJpY2VkGAYgASgIItYDChBEYWlseVRva2VuQnVja2V0EigKBGRhdGUYASABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEhoKEnRvdGFsX2lucHV0X3Rva2VucxgCIAEoAxIbChN0b3RhbF9vdXRwdXRfdG9rZW5zGAMgASgDEhkKEWNhY2hlX3JlYWRfdG9rZW5zGAQgASgDEhoKEmVzdGltYXRlZF9jb3N0X3VzZBgFIAEoARIVCg1zZXNzaW9uX2NvdW50GAYgASgFEkQKDWNvc3RfYnlfbW9kZWwYByADKAsyLS5zZXNzaW9uLnYxLkRhaWx5VG9rZW5CdWNrZXQuQ29zdEJ5TW9kZWxFbnRyeRJICg90b2tlbnNfYnlfbW9kZWwYCCADKAsyLy5zZXNzaW9uLnYxLkRhaWx5VG9rZW5CdWNrZXQuVG9rZW5zQnlNb2RlbEVudHJ5EhcKD3VucHJpY2VkX21vZGVscxgJIAMoCRoyChBDb3N0QnlNb2RlbEVudHJ5EgsKA2tleRgBIAEoCRINCgV2YWx1ZRgCIAEoAToCOAEaNAoSVG9rZW5zQnlNb2RlbEVudHJ5EgsKA2tleRgBIAEoCRINCgV2YWx1ZRgCIAEoAzoCOAEiygEKDk1vZGVsQnJlYWtkb3duEhQKDG1vZGVsX2ZhbWlseRgBIAEoCRIaChJ0b3RhbF9pbnB1dF90b2tlbnMYAiABKAMSGwoTdG90YWxfb3V0cHV0X3Rva2VucxgDIAEoAxIZChFjYWNoZV9yZWFkX3Rva2VucxgEIAEoAxIaChJlc3RpbWF0ZWRfY29zdF91c2QYBSABKAESFQoNc2Vzc2lvbl9jb3VudBgGIAEoBRIbChNwcmljaW5nX3VuYXZhaWxhYmxlGAcgASgIInsKFUFjdGl2aXR5Q29zdEJyZWFrZG93bhIvCg1hY3Rpdml0eV90eXBlGAEgASgOMhguc2Vzc2lvbi52MS5BY3Rpdml0eVR5cGUSGgoSZXN0aW1hdGVkX2Nvc3RfdXNkGAIgASgBEhUKDXNlc3Npb25fY291bnQYAyABKAUihgEKDEl0ZW1Sb2xlQ29zdBIPCgdpdGVtX2lkGAEgASgJEhIKCml0ZW1fdGl0bGUYAiABKAkSGgoSZXN0aW1hdGVkX2Nvc3RfdXNkGAMgASgBEhUKDXNlc3Npb25fY291bnQYBCABKAUSHgoWdW5wcmljZWRfc2Vzc2lvbl9jb3VudBgFIAEoBSKlAQoRUm9sZUNvc3RCcmVha2Rvd24SFAoMc2Vzc2lvbl9yb2xlGAEgASgJEhoKEmVzdGltYXRlZF9jb3N0X3VzZBgCIAEoARIVCg1zZXNzaW9uX2NvdW50GAMgASgFEicKBWl0ZW1zGAQgAygLMhguc2Vzc2lvbi52MS5JdGVtUm9sZUNvc3QSHgoWdW5wcmljZWRfc2Vzc2lvbl9jb3VudBgFIAEoBSJXCghUb3BFbnRyeRIMCgRuYW1lGAEgASgJEhMKC3Rva2VuX2NvdW50GAIgASgDEhgKEGFjdGl2YXRpb25fY291bnQYAyABKAVKBAgEEAVSCGNvc3RfdXNkIqgCChlHZXRJbnNpZ2h0c1N1bW1hcnlSZXF1ZXN0EigKBGZyb20YASABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEiYKAnRvGAIgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBIZCgxtb2RlbF9maWx0ZXIYAyABKAlIAIgBARIeChFzZXNzaW9uX2lkX2ZpbHRlchgEIAEoCUgBiAEBEhcKD2luY2x1ZGVfb3JwaGFucxgFIAEoCBIjChZjb252ZXJzYXRpb25faWRfZmlsdGVyGAYgASgJSAKIAQFCDwoNX21vZGVsX2ZpbHRlckIUChJfc2Vzc2lvbl9pZF9maWx0ZXJCGQoXX2NvbnZlcnNhdGlvbl9pZF9maWx0ZXIijwUKGkdldEluc2lnaHRzU3VtbWFyeVJlc3BvbnNlEjEKCHNlc3Npb25zGAEgAygLMh8uc2Vzc2lvbi52MS5TZXNzaW9uVG9rZW5TdW1tYXJ5EhYKDnRvdGFsX2Nvc3RfdXNkGAIgASgBEhoKEnRvdGFsX2lucHV0X3Rva2VucxgDIAEoAxIbChN0b3RhbF9vdXRwdXRfdG9rZW5zGAQgASgDEh8KF3RvdGFsX2NhY2hlX3JlYWRfdG9rZW5zGAUgASgDEh4KFm92ZXJhbGxfY2FjaGVfaGl0X3JhdGUYBiABKAESKwoFZGFpbHkYByADKAsyHC5zZXNzaW9uLnYxLkRhaWx5VG9rZW5CdWNrZXQSKgoGbW9kZWxzGAggAygLMhouc2Vzc2lvbi52MS5Nb2RlbEJyZWFrZG93bhIoCgp0b3Bfc2tpbGxzGAkgAygLMhQuc2Vzc2lvbi52MS5Ub3BFbnRyeRInCgl0b3BfdG9vbHMYCiADKAsyFC5zZXNzaW9uLnYxLlRvcEVudHJ5EhIKCmlzX2xvYWRpbmcYCyABKAgSMQoNcHJpY2luZ19hc19vZhgMIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASFwoPdW5wcmljZWRfbW9kZWxzGA0gAygJEioKCGZpbmRpbmdzGA4gAygLMhguc2Vzc2lvbi52MS5XYXN0ZUZpbmRpbmcSPQoSYWN0aXZpdHlfYnJlYWtkb3duGA8gAygLMiEuc2Vzc2lvbi52MS5BY3Rpdml0eUNvc3RCcmVha2Rvd24SNQoOcm9sZV9icmVha2Rvd24YECADKAsyHS5zZXNzaW9uLnYxLlJvbGVDb3N0QnJlYWtkb3duIrcBChhMaXN0U2Vzc2lvblRva2Vuc1JlcXVlc3QSKAoEZnJvbRgBIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASJgoCdG8YAiABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEg8KB3NvcnRfYnkYAyABKAkSEQoJc29ydF9kZXNjGAQgASgIEhEKCXBhZ2Vfc2l6ZRgFIAEoBRISCgpwYWdlX3Rva2VuGAYgASgJInwKGUxpc3RTZXNzaW9uVG9rZW5zUmVzcG9uc2USMQoIc2Vzc2lvbnMYASADKAsyHy5zZXNzaW9uLnYxLlNlc3Npb25Ub2tlblN1bW1hcnkSFwoPbmV4dF9wYWdlX3Rva2VuGAIgASgJEhMKC3RvdGFsX2NvdW50GAMgASgFImgKFFdhdGNoSW5zaWdodHNSZXF1ZXN0EigKBGZyb20YASABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEiYKAnRvGAIgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcCJ6Cg1JbnNpZ2h0c0V2ZW50EhIKCmV2ZW50X3R5cGUYASABKAkSNQoHc2Vzc2lvbhgCIAEoCzIfLnNlc3Npb24udjEuU2Vzc2lvblRva2VuU3VtbWFyeUgAiAEBEhIKCmFsbF9wYXJzZWQYAyABKAhCCgoIX3Nlc3Npb24iyAEKDVR1cm5Ub2tlblN0YXQSLQoJdGltZXN0YW1wGAEgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBINCgVtb2RlbBgCIAEoCRIUCgxpbnB1dF90b2tlbnMYAyABKAMSFQoNb3V0cHV0X3Rva2VucxgEIAEoAxIdChVjYWNoZV9jcmVhdGlvbl90b2tlbnMYBSABKAMSGQoRY2FjaGVfcmVhZF90b2tlbnMYBiABKAMSEgoKdG9vbF9uYW1lcxgHIAMoCSI4Ch1HZXRTZXNzaW9uVHVyblRpbWVsaW5lUmVxdWVzdBIXCg9jb252ZXJzYXRpb25faWQYASABKAkiSgoeR2V0U2Vzc2lvblR1cm5UaW1lbGluZVJlc3BvbnNlEigKBXR1cm5zGAEgAygLMhkuc2Vzc2lvbi52MS5UdXJuVG9rZW5TdGF0IocBChVEaXNtaXNzRmluZGluZ1JlcXVlc3QSEgoKZmluZGluZ19pZBgBIAEoCRISCgpzZXNzaW9uX2lkGAIgASgJEhcKD2NvbnZlcnNhdGlvbl9pZBgDIAEoCRItCgxmaW5kaW5nX3R5cGUYBCABKA4yFy5zZXNzaW9uLnYxLkZpbmRpbmdUeXBlIikKFkRpc21pc3NGaW5kaW5nUmVzcG9uc2USDwoHc3VjY2VzcxgBIAEoCCKPAQoNQ29udGV4dFNhbXBsZRISCgp0dXJuX2luZGV4GAEgASgFEi4KCnNhbXBsZWRfYXQYAiABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEg0KBW1vZGVsGAMgASgJEhYKDmNvbnRleHRfdG9rZW5zGAQgASgDEhMKC2NvbnRleHRfbWF4GAUgASgDIsMBCg9Db21wYWN0aW9uRXZlbnQSFwoPY29udmVyc2F0aW9uX2lkGAEgASgJEhIKCnR1cm5faW5kZXgYAiABKAUSLwoLb2NjdXJyZWRfYXQYAyABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEg8KB3RyaWdnZXIYBCABKAkSFQoNdG9rZW5zX2JlZm9yZRgFIAEoAxIUCgx0b2tlbnNfYWZ0ZXIYBiABKAMSFAoMdG9rZW5zX2ZyZWVkGAcgASgDIpYBChVDb250ZXh0Q2VpbGluZ1N1bW1hcnkSFwoPY29udmVyc2F0aW9uX2lkGAEgASgJEg0KBXR1cm5zGAIgASgFEhMKC3BlYWtfdG9rZW5zGAMgASgDEh8KF2ZyYWN0aW9uX2F0X29yX2Fib3ZlXzc1GAQgASgBEh8KF2ZyYWN0aW9uX2F0X29yX2Fib3ZlXzkwGAUgASgBIjMKGEdldENvbnRleHRIaXN0b3J5UmVxdWVzdBIXCg9jb252ZXJzYXRpb25faWQYASABKAkirQEKGUdldENvbnRleHRIaXN0b3J5UmVzcG9uc2USKgoHc2FtcGxlcxgBIAMoCzIZLnNlc3Npb24udjEuQ29udGV4dFNhbXBsZRIyCgdzdW1tYXJ5GAIgASgLMiEuc2Vzc2lvbi52MS5Db250ZXh0Q2VpbGluZ1N1bW1hcnkSMAoLY29tcGFjdGlvbnMYAyADKAsyGy5zZXNzaW9uLnYxLkNvbXBhY3Rpb25FdmVudCIxCiBMaXN0U2Vzc2lvbnNCeUNlaWxpbmdUaW1lUmVxdWVzdBINCgVsaW1pdBgBIAEoBSJYCiFMaXN0U2Vzc2lvbnNCeUNlaWxpbmdUaW1lUmVzcG9uc2USMwoIc2Vzc2lvbnMYASADKAsyIS5zZXNzaW9uLnYxLkNvbnRleHRDZWlsaW5nU3VtbWFyeSJGChlHZXRDb21wYWN0aW9uU3RhdHNSZXF1ZXN0EikKBXNpbmNlGAEgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcCKxAQoaR2V0Q29tcGFjdGlvblN0YXRzUmVzcG9uc2USDQoFY291bnQYASABKAUSFAoMbWFudWFsX2NvdW50GAIgASgFEhIKCmF1dG9fY291bnQYAyABKAUSGgoSdG90YWxfdG9rZW5zX2ZyZWVkGAQgASgDEhwKFGF2ZXJhZ2VfdG9rZW5zX2ZyZWVkGAUgASgDEiAKGHNlc3Npb25zX3dpdGhfY29tcGFjdGlvbhgGIAEoBSphCghTZXZlcml0eRIYChRTRVZFUklUWV9VTlNQRUNJRklFRBAAEhEKDVNFVkVSSVRZX0lORk8QARIRCg1TRVZFUklUWV9XQVJOEAISFQoRU0VWRVJJVFlfQ1JJVElDQUwQAyqfAgoLRmluZGluZ1R5cGUSHAoYRklORElOR19UWVBFX1VOU1BFQ0lGSUVEEAASJwojRklORElOR19UWVBFX0NBQ0hFX0hJVF9GTE9PUl9CUkVBQ0gQARImCiJGSU5ESU5HX1RZUEVfU0VTU0lPTl9UT0tFTl9DRUlMSU5HEAISKAokRklORElOR19UWVBFX01PREVMX1NXSVRDSF9DQUNIRV9CVVNUEAMSKAokRklORElOR19UWVBFX09WRVJTSVpFRF9TVEFSVF9DT05URVhUEAQSHgoaRklORElOR19UWVBFX0xPV19DQUNIRV9ST0kQBRItCilGSU5ESU5HX1RZUEVfQ09OVEVYVF9HUk9XVEhfTk9fQ09NUEFDVElPThAGKsABCgxBY3Rpdml0eVR5cGUSHQoZQUNUSVZJVFlfVFlQRV9VTlNQRUNJRklFRBAAEhsKF0FDVElWSVRZX1RZUEVfREVCVUdHSU5HEAESHQoZQUNUSVZJVFlfVFlQRV9SRUZBQ1RPUklORxACEh0KGUFDVElWSVRZX1RZUEVfRkVBVFVSRV9ERVYQAxIdChlBQ1RJVklUWV9UWVBFX0VYUExPUkFUT1JZEAQSFwoTQUNUSVZJVFlfVFlQRV9PVEhFUhAFMsMGCg9JbnNpZ2h0c1NlcnZpY2USZQoSR2V0SW5zaWdodHNTdW1tYXJ5EiUuc2Vzc2lvbi52MS5HZXRJbnNpZ2h0c1N1bW1hcnlSZXF1ZXN0GiYuc2Vzc2lvbi52MS5HZXRJbnNpZ2h0c1N1bW1hcnlSZXNwb25zZSIAEmIKEUxpc3RTZXNzaW9uVG9rZW5zEiQuc2Vzc2lvbi52MS5MaXN0U2Vzc2lvblRva2Vuc1JlcXVlc3QaJS5zZXNzaW9uLnYxLkxpc3RTZXNzaW9uVG9rZW5zUmVzcG9uc2UiABJQCg1XYXRjaEluc2lnaHRzEiAuc2Vzc2lvbi52MS5XYXRjaEluc2lnaHRzUmVxdWVzdBoZLnNlc3Npb24udjEuSW5zaWdodHNFdmVudCIAMAEScQoWR2V0U2Vzc2lvblR1cm5UaW1lbGluZRIpLnNlc3Npb24udjEuR2V0U2Vzc2lvblR1cm5UaW1lbGluZVJlcXVlc3QaKi5zZXNzaW9uLnYxLkdldFNlc3Npb25UdXJuVGltZWxpbmVSZXNwb25zZSIAElkKDkRpc21pc3NGaW5kaW5nEiEuc2Vzc2lvbi52MS5EaXNtaXNzRmluZGluZ1JlcXVlc3QaIi5zZXNzaW9uLnYxLkRpc21pc3NGaW5kaW5nUmVzcG9uc2UiABJiChFHZXRDb250ZXh0SGlzdG9yeRIkLnNlc3Npb24udjEuR2V0Q29udGV4dEhpc3RvcnlSZXF1ZXN0GiUuc2Vzc2lvbi52MS5HZXRDb250ZXh0SGlzdG9yeVJlc3BvbnNlIgASegoZTGlzdFNlc3Npb25zQnlDZWlsaW5nVGltZRIsLnNlc3Npb24udjEuTGlzdFNlc3Npb25zQnlDZWlsaW5nVGltZVJlcXVlc3QaLS5zZXNzaW9uLnYxLkxpc3RTZXNzaW9uc0J5Q2VpbGluZ1RpbWVSZXNwb25zZSIAEmUKEkdldENvbXBhY3Rpb25TdGF0cxIlLnNlc3Npb24udjEuR2V0Q29tcGFjdGlvblN0YXRzUmVxdWVzdBomLnNlc3Npb24udjEuR2V0Q29tcGFjdGlvblN0YXRzUmVzcG9uc2UiAEKtAQoOY29tLnNlc3Npb24udjFCDUluc2lnaHRzUHJvdG9QAVpDZ2l0aHViLmNvbS90c3RhcGxlci9zdGFwbGVyLXNxdWFkL2dlbi9wcm90by9nby9zZXNzaW9uL3YxO3Nlc3Npb252MaICA1NYWKoCClNlc3Npb24uVjHKAgpTZXNzaW9uXFYx4gIWU2Vzc2lvblxWMVxHUEJNZXRhZGF0YeoCC1Nlc3Npb246OlYxYgZwcm90bzM", [
    _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_1__/* .file_google_protobuf_timestamp */ .i
]);
/**
 * Describes the message session.v1.WasteFinding.
 * Use `create(WasteFindingSchema)` to create a new message.
 */ const WasteFindingSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 0)));
/**
 * Describes the message session.v1.SessionTokenSummary.
 * Use `create(SessionTokenSummarySchema)` to create a new message.
 */ const SessionTokenSummarySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 1)));
/**
 * Describes the message session.v1.TopToolEntry.
 * Use `create(TopToolEntrySchema)` to create a new message.
 */ const TopToolEntrySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 2)));
/**
 * Describes the message session.v1.DailyTokenBucket.
 * Use `create(DailyTokenBucketSchema)` to create a new message.
 */ const DailyTokenBucketSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 3)));
/**
 * Describes the message session.v1.ModelBreakdown.
 * Use `create(ModelBreakdownSchema)` to create a new message.
 */ const ModelBreakdownSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 4)));
/**
 * Describes the message session.v1.ActivityCostBreakdown.
 * Use `create(ActivityCostBreakdownSchema)` to create a new message.
 */ const ActivityCostBreakdownSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 5)));
/**
 * Describes the message session.v1.ItemRoleCost.
 * Use `create(ItemRoleCostSchema)` to create a new message.
 */ const ItemRoleCostSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 6)));
/**
 * Describes the message session.v1.RoleCostBreakdown.
 * Use `create(RoleCostBreakdownSchema)` to create a new message.
 */ const RoleCostBreakdownSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 7)));
/**
 * Describes the message session.v1.TopEntry.
 * Use `create(TopEntrySchema)` to create a new message.
 */ const TopEntrySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 8)));
/**
 * Describes the message session.v1.GetInsightsSummaryRequest.
 * Use `create(GetInsightsSummaryRequestSchema)` to create a new message.
 */ const GetInsightsSummaryRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_insights, 9);
/**
 * Describes the message session.v1.GetInsightsSummaryResponse.
 * Use `create(GetInsightsSummaryResponseSchema)` to create a new message.
 */ const GetInsightsSummaryResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 10)));
/**
 * Describes the message session.v1.ListSessionTokensRequest.
 * Use `create(ListSessionTokensRequestSchema)` to create a new message.
 */ const ListSessionTokensRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 11)));
/**
 * Describes the message session.v1.ListSessionTokensResponse.
 * Use `create(ListSessionTokensResponseSchema)` to create a new message.
 */ const ListSessionTokensResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 12)));
/**
 * Describes the message session.v1.WatchInsightsRequest.
 * Use `create(WatchInsightsRequestSchema)` to create a new message.
 */ const WatchInsightsRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_insights, 13);
/**
 * Describes the message session.v1.InsightsEvent.
 * Use `create(InsightsEventSchema)` to create a new message.
 */ const InsightsEventSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 14)));
/**
 * Describes the message session.v1.TurnTokenStat.
 * Use `create(TurnTokenStatSchema)` to create a new message.
 */ const TurnTokenStatSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 15)));
/**
 * Describes the message session.v1.GetSessionTurnTimelineRequest.
 * Use `create(GetSessionTurnTimelineRequestSchema)` to create a new message.
 */ const GetSessionTurnTimelineRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_insights, 16);
/**
 * Describes the message session.v1.GetSessionTurnTimelineResponse.
 * Use `create(GetSessionTurnTimelineResponseSchema)` to create a new message.
 */ const GetSessionTurnTimelineResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 17)));
/**
 * Describes the message session.v1.DismissFindingRequest.
 * Use `create(DismissFindingRequestSchema)` to create a new message.
 */ const DismissFindingRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .messageDesc */ .Z)(file_session_v1_insights, 18);
/**
 * Describes the message session.v1.DismissFindingResponse.
 * Use `create(DismissFindingResponseSchema)` to create a new message.
 */ const DismissFindingResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 19)));
/**
 * Describes the message session.v1.ContextSample.
 * Use `create(ContextSampleSchema)` to create a new message.
 */ const ContextSampleSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 20)));
/**
 * Describes the message session.v1.CompactionEvent.
 * Use `create(CompactionEventSchema)` to create a new message.
 */ const CompactionEventSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 21)));
/**
 * Describes the message session.v1.ContextCeilingSummary.
 * Use `create(ContextCeilingSummarySchema)` to create a new message.
 */ const ContextCeilingSummarySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 22)));
/**
 * Describes the message session.v1.GetContextHistoryRequest.
 * Use `create(GetContextHistoryRequestSchema)` to create a new message.
 */ const GetContextHistoryRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 23)));
/**
 * Describes the message session.v1.GetContextHistoryResponse.
 * Use `create(GetContextHistoryResponseSchema)` to create a new message.
 */ const GetContextHistoryResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 24)));
/**
 * Describes the message session.v1.ListSessionsByCeilingTimeRequest.
 * Use `create(ListSessionsByCeilingTimeRequestSchema)` to create a new message.
 */ const ListSessionsByCeilingTimeRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 25)));
/**
 * Describes the message session.v1.ListSessionsByCeilingTimeResponse.
 * Use `create(ListSessionsByCeilingTimeResponseSchema)` to create a new message.
 */ const ListSessionsByCeilingTimeResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 26)));
/**
 * Describes the message session.v1.GetCompactionStatsRequest.
 * Use `create(GetCompactionStatsRequestSchema)` to create a new message.
 */ const GetCompactionStatsRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 27)));
/**
 * Describes the message session.v1.GetCompactionStatsResponse.
 * Use `create(GetCompactionStatsResponseSchema)` to create a new message.
 */ const GetCompactionStatsResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_insights, 28)));
/**
 * Severity classifies how urgently a WasteFinding should be acted on.
 *
 * @generated from enum session.v1.Severity
 */ var Severity = /*#__PURE__*/ function(Severity) {
    /**
   * @generated from enum value: SEVERITY_UNSPECIFIED = 0;
   */ Severity[Severity["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: SEVERITY_INFO = 1;
   */ Severity[Severity["INFO"] = 1] = "INFO";
    /**
   * @generated from enum value: SEVERITY_WARN = 2;
   */ Severity[Severity["WARN"] = 2] = "WARN";
    /**
   * @generated from enum value: SEVERITY_CRITICAL = 3;
   */ Severity[Severity["CRITICAL"] = 3] = "CRITICAL";
    return Severity;
}({});
/**
 * Describes the enum session.v1.Severity.
 */ const SeveritySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_insights, 0)));
/**
 * FindingType identifies which waste-pattern heuristic produced a WasteFinding.
 * 4 non-zero values shipped in v1 — see plan.md's "Detector scope cut" note.
 * FINDING_TYPE_LOW_CACHE_ROI (5) shipped later, promoting ComputeCacheROI from
 * a tooltip-only metric to a first-class finding — see tstapler/stapler-squad#879.
 *
 * @generated from enum session.v1.FindingType
 */ var FindingType = /*#__PURE__*/ function(FindingType) {
    /**
   * @generated from enum value: FINDING_TYPE_UNSPECIFIED = 0;
   */ FindingType[FindingType["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: FINDING_TYPE_CACHE_HIT_FLOOR_BREACH = 1;
   */ FindingType[FindingType["CACHE_HIT_FLOOR_BREACH"] = 1] = "CACHE_HIT_FLOOR_BREACH";
    /**
   * @generated from enum value: FINDING_TYPE_SESSION_TOKEN_CEILING = 2;
   */ FindingType[FindingType["SESSION_TOKEN_CEILING"] = 2] = "SESSION_TOKEN_CEILING";
    /**
   * @generated from enum value: FINDING_TYPE_MODEL_SWITCH_CACHE_BUST = 3;
   */ FindingType[FindingType["MODEL_SWITCH_CACHE_BUST"] = 3] = "MODEL_SWITCH_CACHE_BUST";
    /**
   * @generated from enum value: FINDING_TYPE_OVERSIZED_START_CONTEXT = 4;
   */ FindingType[FindingType["OVERSIZED_START_CONTEXT"] = 4] = "OVERSIZED_START_CONTEXT";
    /**
   * @generated from enum value: FINDING_TYPE_LOW_CACHE_ROI = 5;
   */ FindingType[FindingType["LOW_CACHE_ROI"] = 5] = "LOW_CACHE_ROI";
    /**
   * @generated from enum value: FINDING_TYPE_CONTEXT_GROWTH_NO_COMPACTION = 6;
   */ FindingType[FindingType["CONTEXT_GROWTH_NO_COMPACTION"] = 6] = "CONTEXT_GROWTH_NO_COMPACTION";
    return FindingType;
}({});
/**
 * Describes the enum session.v1.FindingType.
 */ const FindingTypeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_insights, 1)));
/**
 * ActivityType classifies the kind of work a session performed. Defined now
 * (Epic 1.1) but not populated until Epic 1.2's ClassifyActivity lands.
 *
 * @generated from enum session.v1.ActivityType
 */ var ActivityType = /*#__PURE__*/ function(ActivityType) {
    /**
   * @generated from enum value: ACTIVITY_TYPE_UNSPECIFIED = 0;
   */ ActivityType[ActivityType["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: ACTIVITY_TYPE_DEBUGGING = 1;
   */ ActivityType[ActivityType["DEBUGGING"] = 1] = "DEBUGGING";
    /**
   * @generated from enum value: ACTIVITY_TYPE_REFACTORING = 2;
   */ ActivityType[ActivityType["REFACTORING"] = 2] = "REFACTORING";
    /**
   * @generated from enum value: ACTIVITY_TYPE_FEATURE_DEV = 3;
   */ ActivityType[ActivityType["FEATURE_DEV"] = 3] = "FEATURE_DEV";
    /**
   * @generated from enum value: ACTIVITY_TYPE_EXPLORATORY = 4;
   */ ActivityType[ActivityType["EXPLORATORY"] = 4] = "EXPLORATORY";
    /**
   * @generated from enum value: ACTIVITY_TYPE_OTHER = 5;
   */ ActivityType[ActivityType["OTHER"] = 5] = "OTHER";
    return ActivityType;
}({});
/**
 * Describes the enum session.v1.ActivityType.
 */ const ActivityTypeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_insights, 2)));
/**
 * InsightsService provides token usage analytics derived from JSONL transcripts.
 *
 * @generated from service session.v1.InsightsService
 */ const InsightsService = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_3__/* .serviceDesc */ .Z)(file_session_v1_insights, 0);


/***/ })

};
;