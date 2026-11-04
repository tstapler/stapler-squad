"use strict";
exports.id = 673;
exports.ids = [673];
exports.modules = {

/***/ 38880:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O5: () => (/* binding */ getDateGroup),
/* harmony export */   Yq: () => (/* binding */ formatDate),
/* harmony export */   ak: () => (/* binding */ formatTimeAgo),
/* harmony export */   uj: () => (/* binding */ protoTimestampToDate),
/* harmony export */   wZ: () => (/* binding */ isWithinDateFilter)
/* harmony export */ });
/* unused harmony export truncateMiddle */
/**
 * protoTimestampToDate converts a protobuf Timestamp to a Date, including the
 * sub-second nanos component (unlike this file's other helpers, which only use
 * `seconds`). Shared by TriggersPanel.tsx and TriggerExecutionHistory.tsx, which
 * previously carried two divergent inline copies of this arithmetic — one returning a
 * Date, the other returning an ISO string. Returns Date so callers format as needed
 * (toLocaleString, relative-time, etc.) rather than baking in one representation.
 */ const protoTimestampToDate = (timestamp)=>{
    if (!timestamp) return null;
    const ms = Number(timestamp.seconds) * 1000 + Math.floor(timestamp.nanos / 1e6);
    return new Date(ms);
};
const formatTimeAgo = (timestamp)=>{
    if (!timestamp) return "N/A";
    const now = Date.now();
    const date = new Date(Number(timestamp.seconds) * 1000);
    const seconds = Math.floor((now - date.getTime()) / 1000);
    if (seconds < 60) return "just now";
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
    if (seconds < 604800) return `${Math.floor(seconds / 86400)}d ago`;
    return date.toLocaleDateString();
};
const formatDate = (timestamp)=>{
    if (!timestamp) return "N/A";
    const date = new Date(Number(timestamp.seconds) * 1000);
    return date.toLocaleString();
};
const truncateMiddle = (str, maxLength)=>{
    if (str.length <= maxLength) return str;
    const ellipsis = "...";
    const charsToShow = maxLength - ellipsis.length;
    const frontChars = Math.ceil(charsToShow / 2);
    const backChars = Math.floor(charsToShow / 2);
    return str.substring(0, frontChars) + ellipsis + str.substring(str.length - backChars);
};
const getDateGroup = (timestamp)=>{
    if (!timestamp) return "Unknown";
    const date = new Date(Number(timestamp.seconds) * 1000);
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const yesterday = new Date(today.getTime() - 86400000);
    const weekAgo = new Date(today.getTime() - 7 * 86400000);
    const monthAgo = new Date(today.getTime() - 30 * 86400000);
    if (date >= today) return "Today";
    if (date >= yesterday) return "Yesterday";
    if (date >= weekAgo) return "This Week";
    if (date >= monthAgo) return "This Month";
    return "Older";
};
const isWithinDateFilter = (timestamp, filter)=>{
    if (filter === "all" || !timestamp) return true;
    const date = new Date(Number(timestamp.seconds) * 1000);
    const now = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    switch(filter){
        case "today":
            return date >= today;
        case "week":
            return date >= new Date(today.getTime() - 7 * 86400000);
        case "month":
            return date >= new Date(today.getTime() - 30 * 86400000);
        default:
            return true;
    }
};


/***/ }),

/***/ 40673:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  L: () => (/* binding */ SessionSummaryPanel)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-markdown@10.1.0+5bba37e96b694e48/node_modules/react-markdown/lib/index.js + 112 modules
var lib = __webpack_require__(57286);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/remark-gfm@4.0.1+759ce506b1ed1a42/node_modules/remark-gfm/lib/index.js + 56 modules
var remark_gfm_lib = __webpack_require__(76877);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(4435);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/file.js
var file = __webpack_require__(89974);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/service.js
var service = __webpack_require__(90477);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/gen/google/protobuf/timestamp_pb.js
var timestamp_pb = __webpack_require__(21657);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(29238);
;// ./src/gen/session/v1/session_summary_pb.ts
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/session_summary.proto (package session.v1, syntax proto3)
/* eslint-disable */ 


/**
 * Describes the file session/v1/session_summary.proto.
 */ const file_session_v1_session_summary = /*@__PURE__*/ (0,file/* fileDesc */.w)("CiBzZXNzaW9uL3YxL3Nlc3Npb25fc3VtbWFyeS5wcm90bxIKc2Vzc2lvbi52MSKPBwoTU2Vzc2lvblN1bW1hcnlQcm90bxISCgpzZXNzaW9uX2lkGAEgASgJEhUKDXNlc3Npb25fdGl0bGUYAiABKAkSMAoGc3RhdHVzGAMgASgOMiAuc2Vzc2lvbi52MS5TZXNzaW9uU3VtbWFyeVN0YXR1cxIRCgluYXJyYXRpdmUYBCABKAkSHwoXbmFycmF0aXZlX2ZhbGxiYWNrX3VzZWQYBSABKAgSMgoEZGlmZhgGIAEoCzIkLnNlc3Npb24udjEuU2Vzc2lvblN1bW1hcnlQcm90by5EaWZmEjwKCWRlY2lzaW9ucxgHIAEoCzIpLnNlc3Npb24udjEuU2Vzc2lvblN1bW1hcnlQcm90by5EZWNpc2lvbnMSOgoIdGltZWxpbmUYCCABKAsyKC5zZXNzaW9uLnYxLlNlc3Npb25TdW1tYXJ5UHJvdG8uVGltZWxpbmUSMgoEY29zdBgJIAEoCzIkLnNlc3Npb24udjEuU2Vzc2lvblN1bW1hcnlQcm90by5Db3N0EhAKCG1hcmtkb3duGAogASgJEhUKDWVycm9yX21lc3NhZ2UYCyABKAkSEwoLZXJyb3Jfc3RhZ2UYDCABKAkSMAoMZ2VuZXJhdGVkX2F0GA0gASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcBo9CgREaWZmEhUKDWZpbGVzX2NoYW5nZWQYASABKAUSDQoFYWRkZWQYAiABKAUSDwoHcmVtb3ZlZBgDIAEoBRqAAQoJRGVjaXNpb25zEhUKDWF1dG9fYXBwcm92ZWQYASABKAUSGQoRbWFudWFsbHlfYXBwcm92ZWQYAiABKAUSDgoGZGVuaWVkGAMgASgFEh0KFXJldmlld19xdWV1ZV9yZXNvbHZlZBgEIAEoBRISCgpzdGlsbF9vcGVuGAUgASgFGn8KCFRpbWVsaW5lEi4KCnN0YXJ0ZWRfYXQYASABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEi4KCnN0b3BwZWRfYXQYAiABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEhMKC2R1cmF0aW9uX21zGAMgASgDGlIKBENvc3QSFAoMdG90YWxfdG9rZW5zGAEgASgDEhoKEmVzdGltYXRlZF9jb3N0X3VzZBgCIAEoARIYChBkYXRhX3VuYXZhaWxhYmxlGAMgASgIIi4KGEdldFNlc3Npb25TdW1tYXJ5UmVxdWVzdBISCgpzZXNzaW9uX2lkGAEgASgJIl4KGUdldFNlc3Npb25TdW1tYXJ5UmVzcG9uc2USNQoHc3VtbWFyeRgBIAEoCzIfLnNlc3Npb24udjEuU2Vzc2lvblN1bW1hcnlQcm90b0gAiAEBQgoKCF9zdW1tYXJ5IjUKH1JlZ2VuZXJhdGVTZXNzaW9uU3VtbWFyeVJlcXVlc3QSEgoKc2Vzc2lvbl9pZBgBIAEoCSJUCiBSZWdlbmVyYXRlU2Vzc2lvblN1bW1hcnlSZXNwb25zZRIwCgdzdW1tYXJ5GAEgASgLMh8uc2Vzc2lvbi52MS5TZXNzaW9uU3VtbWFyeVByb3RvMvQBChVTZXNzaW9uU3VtbWFyeVNlcnZpY2USYgoRR2V0U2Vzc2lvblN1bW1hcnkSJC5zZXNzaW9uLnYxLkdldFNlc3Npb25TdW1tYXJ5UmVxdWVzdBolLnNlc3Npb24udjEuR2V0U2Vzc2lvblN1bW1hcnlSZXNwb25zZSIAEncKGFJlZ2VuZXJhdGVTZXNzaW9uU3VtbWFyeRIrLnNlc3Npb24udjEuUmVnZW5lcmF0ZVNlc3Npb25TdW1tYXJ5UmVxdWVzdBosLnNlc3Npb24udjEuUmVnZW5lcmF0ZVNlc3Npb25TdW1tYXJ5UmVzcG9uc2UiAEKzAQoOY29tLnNlc3Npb24udjFCE1Nlc3Npb25TdW1tYXJ5UHJvdG9QAVpDZ2l0aHViLmNvbS90c3RhcGxlci9zdGFwbGVyLXNxdWFkL2dlbi9wcm90by9nby9zZXNzaW9uL3YxO3Nlc3Npb252MaICA1NYWKoCClNlc3Npb24uVjHKAgpTZXNzaW9uXFYx4gIWU2Vzc2lvblxWMVxHUEJNZXRhZGF0YeoCC1Nlc3Npb246OlYxYgZwcm90bzM", [
    timestamp_pb/* file_google_protobuf_timestamp */.i,
    types_pb/* file_session_v1_types */.m$
]);
/**
 * Describes the message session.v1.SessionSummaryProto.
 * Use `create(SessionSummaryProtoSchema)` to create a new message.
 */ const SessionSummaryProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_session_summary, 0)));
/**
 * Describes the message session.v1.SessionSummaryProto.Diff.
 * Use `create(SessionSummaryProto_DiffSchema)` to create a new message.
 */ const SessionSummaryProto_DiffSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_session_summary, 0, 0)));
/**
 * Describes the message session.v1.SessionSummaryProto.Decisions.
 * Use `create(SessionSummaryProto_DecisionsSchema)` to create a new message.
 */ const SessionSummaryProto_DecisionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_session_summary, 0, 1)));
/**
 * Describes the message session.v1.SessionSummaryProto.Timeline.
 * Use `create(SessionSummaryProto_TimelineSchema)` to create a new message.
 */ const SessionSummaryProto_TimelineSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_session_summary, 0, 2)));
/**
 * Describes the message session.v1.SessionSummaryProto.Cost.
 * Use `create(SessionSummaryProto_CostSchema)` to create a new message.
 */ const SessionSummaryProto_CostSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_session_summary, 0, 3)));
/**
 * Describes the message session.v1.GetSessionSummaryRequest.
 * Use `create(GetSessionSummaryRequestSchema)` to create a new message.
 */ const GetSessionSummaryRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_session_summary, 1)));
/**
 * Describes the message session.v1.GetSessionSummaryResponse.
 * Use `create(GetSessionSummaryResponseSchema)` to create a new message.
 */ const GetSessionSummaryResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_session_summary, 2)));
/**
 * Describes the message session.v1.RegenerateSessionSummaryRequest.
 * Use `create(RegenerateSessionSummaryRequestSchema)` to create a new message.
 */ const RegenerateSessionSummaryRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_session_summary, 3)));
/**
 * Describes the message session.v1.RegenerateSessionSummaryResponse.
 * Use `create(RegenerateSessionSummaryResponseSchema)` to create a new message.
 */ const RegenerateSessionSummaryResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_session_summary, 4)));
/**
 * SessionSummaryService provides read and regenerate access to session
 * completion summaries, sourced from the SessionSummary table rather than
 * SessionService's live-instance machinery.
 *
 * @generated from service session.v1.SessionSummaryService
 */ const SessionSummaryService = /*@__PURE__*/ (0,service/* serviceDesc */.Z)(file_session_v1_session_summary, 0);

// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(73396);
// EXTERNAL MODULE: ./src/lib/clipboard.ts
var clipboard = __webpack_require__(82832);
// EXTERNAL MODULE: ./src/lib/hooks/useAbortableRequest.ts
var useAbortableRequest = __webpack_require__(10937);
;// ./src/lib/hooks/useSessionSummary.ts
/* __next_internal_client_entry_do_not_use__ isGenerating,useSessionSummary auto */ 






const POLL_INTERVAL_MS = 2000;
/**
 * Cap on consecutive `nil` GetSessionSummary reads before giving up
 * (~10 * 2s = 20s). Closes the "will never resolve" dead end flagged in
 * design/ux.md (surface (g)) for a sessionId that will never produce a row —
 * a typo'd URL, or a session excluded via reconcile-session-missing.
 */ const MAX_NULL_POLL_ATTEMPTS = 10;
/**
 * Single canonical definition of "still generating," shared by the hook's own
 * polling logic and by SessionSummaryPanel's UI branching — a prior copy
 * diverged between the two (panel treated UNSPECIFIED as generating, hook
 * didn't), which could make the hook stop polling on a row the panel was
 * still rendering as a loading skeleton, so it would never resolve. proto3's
 * zero value (`UNSPECIFIED`) is not a state the backend intentionally sets;
 * it only appears when a row is read before its terminal status has ever
 * been assigned, which is not a terminal state either — treat it as
 * generating so both layers keep polling until a real READY/ERROR arrives.
 */ function isGenerating(status) {
    return status === types_pb/* SessionSummaryStatus */.z.UNSPECIFIED || status === types_pb/* SessionSummaryStatus */.z.PENDING || status === types_pb/* SessionSummaryStatus */.z.GENERATING;
}
/**
 * Fetches a session's completion summary, polling every 2s while generation
 * is in flight. Per design/ux.md surface (b)'s edge case, a `nil` response
 * (no row exists yet) polls identically to PENDING/GENERATING — there is no
 * user-visible distinction between "not started yet" and "in progress."
 * Polling stops once `status` reaches READY/ERROR, or once
 * `MAX_NULL_POLL_ATTEMPTS` consecutive `nil` reads have occurred, at which
 * point `neverResolved` is set so callers can render a terminal empty state
 * instead of polling forever.
 *
 * Shared data layer for both the Summary tab (Epic 3.2) and the standalone
 * post-deletion route (Epic 3.3) — see plan.md Story 3.1.1.
 */ function useSessionSummary(sessionId) {
    const [data, setData] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(!!sessionId);
    const [error, setError] = (0,react.useState)(null);
    const [neverResolved, setNeverResolved] = (0,react.useState)(false);
    // Consecutive `nil` GetSessionSummary reads. Only counts consecutive
    // nulls — reset to 0 the moment a non-null summary is observed (even if
    // still GENERATING), per Task 3.1.1a.
    const nullAttemptsRef = (0,react.useRef)(0);
    const intervalRef = (0,react.useRef)(null);
    // Holds the latest fetchSummary closure so the interval tick (set up once
    // per sessionId) always calls the current version rather than a stale one.
    const fetchSummaryRef = (0,react.useRef)(async ()=>{});
    // The sessionId this hook instance is *currently* rendering for. Updated
    // synchronously every render (not via an effect) so it reflects the latest
    // prop before any async callback scheduled during this render can resolve.
    // SessionDetailView's Next/Previous queue navigation keeps the same
    // SessionSummaryPanel (and this hook instance) mounted across a session
    // switch, so a fetch/regenerate call started for the OLD sessionId can
    // still be in flight when `sessionId` changes — compare against this ref
    // before applying a response to state so a late-resolving stale request
    // can't clobber freshly-fetched state for the new session.
    const activeSessionIdRef = (0,react.useRef)(sessionId);
    activeSessionIdRef.current = sessionId;
    // True while a getSessionSummary request (initial fetch, refetch, or poll
    // tick) is in flight. Guards against overlapping poll requests: if a
    // single call takes longer than POLL_INTERVAL_MS, the next interval tick
    // skips issuing a new request rather than firing a second one whose
    // out-of-order response could leave stale data displayed.
    const pollInFlightRef = (0,react.useRef)(false);
    // Shared between fetchSummary and regenerate (safe: pollInFlightRef
    // already keeps them from firing concurrently) so either cancels the
    // other's in-flight request, and both are cancelled on sessionId
    // change/unmount.
    const startRequest = (0,useAbortableRequest/* useAbortableRequest */.X)();
    const stopPolling = (0,react.useCallback)(()=>{
        if (intervalRef.current !== null) {
            clearInterval(intervalRef.current);
            intervalRef.current = null;
        }
    }, []);
    const startPolling = (0,react.useCallback)(()=>{
        if (intervalRef.current !== null) return;
        intervalRef.current = setInterval(()=>{
            if (pollInFlightRef.current) return;
            fetchSummaryRef.current();
        }, POLL_INTERVAL_MS);
    }, []);
    const fetchSummary = (0,react.useCallback)(async ()=>{
        if (!sessionId) {
            setLoading(false);
            return;
        }
        pollInFlightRef.current = true;
        const signal = startRequest();
        try {
            const client = (0,promise_client/* createClient */.UU)(SessionSummaryService, (0,transport/* getConnectTransport */.qW)());
            const response = await client.getSessionSummary({
                sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            // A newer sessionId may have become active while this request was in
            // flight — discard this response rather than overwriting the fresher
            // session's state.
            if (activeSessionIdRef.current !== sessionId) return;
            const summary = response.summary ?? null;
            setData(summary);
            setError(null);
            if (summary === null) {
                nullAttemptsRef.current += 1;
            } else {
                nullAttemptsRef.current = 0;
            }
            const exceededNullAttempts = summary === null && nullAttemptsRef.current >= MAX_NULL_POLL_ATTEMPTS;
            if (exceededNullAttempts) {
                setNeverResolved(true);
            }
            const shouldPoll = !exceededNullAttempts && (summary === null || isGenerating(summary.status));
            if (shouldPoll) {
                startPolling();
            } else {
                stopPolling();
            }
        } catch (err) {
            if (signal.aborted || activeSessionIdRef.current !== sessionId) return;
            setError(err instanceof Error ? err : new Error("Failed to load session summary"));
        } finally{
            if (!signal.aborted && activeSessionIdRef.current === sessionId) {
                setLoading(false);
            }
            pollInFlightRef.current = false;
        }
    }, [
        sessionId,
        startPolling,
        stopPolling,
        startRequest
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        fetchSummaryRef.current = fetchSummary;
    }, [
        fetchSummary
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        nullAttemptsRef.current = 0;
        setNeverResolved(false);
        stopPolling();
        fetchSummary();
        return ()=>stopPolling();
    // fetchSummary/stopPolling are recreated only when sessionId changes, so
    // this intentionally keys off sessionId alone to avoid re-running on
    // every fetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionId
    ]);
    const regenerate = (0,react.useCallback)(async ()=>{
        if (!sessionId) return;
        pollInFlightRef.current = true;
        const signal = startRequest();
        try {
            const client = (0,promise_client/* createClient */.UU)(SessionSummaryService, (0,transport/* getConnectTransport */.qW)());
            const response = await client.regenerateSessionSummary({
                sessionId
            }, {
                signal
            });
            if (signal.aborted) return;
            // See fetchSummary's identical guard — the sessionId this call was
            // issued for may no longer be the active one by the time it resolves.
            if (activeSessionIdRef.current !== sessionId) return;
            const summary = response.summary ?? null;
            setData(summary);
            setError(null);
            nullAttemptsRef.current = 0;
            setNeverResolved(false);
            const shouldPoll = summary === null || isGenerating(summary.status);
            if (shouldPoll) {
                startPolling();
            } else {
                stopPolling();
            }
        } catch (err) {
            if (signal.aborted) return;
            const e = err instanceof Error ? err : new Error("Failed to regenerate session summary");
            if (activeSessionIdRef.current === sessionId) {
                setError(e);
            }
            // Rethrow so callers awaiting regenerate() can distinguish success
            // from failure via the promise itself — see SessionSummaryPanel's
            // handleRegenerate, which relies on this to reset regeneratingRef
            // (a piece of state the phase-transition effect does NOT reset here,
            // since data/phase never change on a failed regenerate).
            throw e;
        } finally{
            pollInFlightRef.current = false;
        }
    }, [
        sessionId,
        startPolling,
        stopPolling,
        startRequest
    ]);
    const copy = (0,react.useCallback)(async ()=>{
        return (0,clipboard/* copyToClipboard */.l)(data?.markdown ?? "");
    }, [
        data
    ]);
    return {
        data,
        loading,
        error,
        neverResolved,
        regenerate,
        refetch: fetchSummary,
        copy
    };
}

// EXTERNAL MODULE: ./src/lib/utils/timestamp.ts
var timestamp = __webpack_require__(38880);
// EXTERNAL MODULE: ./src/components/backlog/markdownBody.css.ts
var markdownBody_css = __webpack_require__(49623);
// EXTERNAL MODULE: ./src/components/ui/LiveRegion.css.ts
var LiveRegion_css = __webpack_require__(40855);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionSummaryPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VY226jMBB9z1f4JVIjrSMglzbsy_5JZfBA3IBNbVNCVv33FSbdYgiXJtlK-xIUZnzO-MzFFr8OUEaSpKDQs5sHqqTrV_R7hpAzNw-EAhIeYilyTnEmFNNMcB95jjNHzs8ZQu8zhFxn2Bs33d9nyw8mxyyiTGUJKX0UJXCsfKonpkxCWC8PRZKnvLLEJPPRG5EPGD-7Ozh58rRyF5UlI5QyHl-0Nhjdy4wkYTHHTEOqfBQC1yCr1y-50iwqcSi4Bq59pDISAg5AFwC9ATmLv5soZGWvfq0ovC9FcYHDO41yrAxHJLjGip2gEyWtESp7ASze644HMx6hSIRs2xxb1fUIV9CP5NlIm29QZmtzMJ4wDvgKqnKo8rzT4nJhBEJSkFgSynLVkerxMy0DUg6lLa7FzqWq1M4E-9hCTewjNzsiJRJGWyvrhZ8N7CMtCVcZkcB1bwJbDbb19-INpJG4CWUvemoqcRF2P7n4tj5ligQJUEMqMhIyXfrIWW6aQnChMUkSUQC1lj-NhJr2B_I6uovUDvVpkjh8FJYPx9Rg3F3fmW5LZ3KHYX3uzr5hPb1Bxmo56tSybX-09xZMkmmo6wbqtT6gNBw1Ng0VCZn6KM8ykCFRUBkT0Bokro4Xo4uzdDaQWiGGA_I3B12_7g0w-j0jsGyPwHP-hzO8GxmBZOII7M8-nTxc4LaT7WOpLhPwEdMkYaGFH_37pnLv1lTuakRXt3UYxPc6zXv1D29qzNblY3_DsGxBsREoMvlG9HI9UisbB4OUEhkzjrX4-n1mvI62kxrPbCYiKUvKDktuPIo902DGIfgok4A_ZlshJMWBBHLwkXng6o21zeR_OqeOI4KVdgrT6yu0dQLwu7RmA1DcQ_bBClzVE7UnRHO-Glvz7ZQOy0ZEvW3G2FzyXsXZwFT1ffKmxoxsSN25olYXBCJxXOED1w87h0L8o4OCvM28_TZeoI3TeRst0ONm3grrnADzuaD6qGDyzVlKalk-v1C4y41CjEeMMw1W6PnI7Tq2d_pm3AtG9d5H7trJTD725zy72-p_w71oum9b3t66_j9-vWkgHq0Aznv-y79u8Zc2_7Dzqem8vuj8B47YYaP_EQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionSummaryPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VY226jMBB9z1f4JVIjrSMglzbsy_5JZfBA3IBNbVNCVv33FSbdYgiXJtlK_xIUZnzO_MzFFr8OUEaSpKDQs5sHqqTrV_R7hpAzNw_EAhIeYilyTnEmFNNMcB95jjNHzs8ZQu8zhFxn2Bs33d9nyw8mxyyiTGUJKX0UJXCsfKonpkxCWC8PRZKnvLLEJPPRG5EPGD_7Ozh58rRyF5UlI5QyHl_0Nhjdy4wkYTHHTEOqfBQC1yCr1y_50iwqcSi4Bq59pDISAg5AFwC9ATmLv5soZGWvfq0ovC9FcYHDO41yrAxHJLjGip2gEyWtESp7ASze644HMx6hSIRs2xxb1fUIV9CP5NlIm29QZmtzMJ4wDvgKqnKo8rzT4nJhBEJSkFgSynLVkerxMy0DUg6lLa7FzqWq1M4E_9hCTewjNzsiJRJGWyvrhZ8N7CMtCVcZkcB1bwJbDbb19_INpJG4CWUvemoqcRF2P7n4tj5ligQJUEMqMhIyXfrIWW6aQnChMUkSUQC1lj_NhJr2B_I6uovUDvVpkjh8FJYPx9Rg3F3fmW5LZ3KHYX3uzr5hPb1Bxmo56tSybX_09xZMkmmo6wbqtT6gNBw1Ng0VCZn6KM8ykCFRUBkT0Bokro4Xo4uzdDaQWiGGA_I3B12_7g0w_j0jsGyPwHP_hzO8GxmBZOII7M8_nTxc4LaT7WOpLhPwEdMkYaGFH_37pnLv1lTuakRXt3UYxPc6zXv1D29qzNblY3_DsGxBsREoMvlG9HI9UisbB4OUEhkzjrX4_n1mvI62kxrPbCYiKUvKDktuPIo902DGIfgok4A_ZlshJMWBBHLwkXng6o21zeR_OqeOI4KVdgrT6yu0dQLwu7RmA1DcQ_bBClzVE7UnRHO_Glvz7ZQOy0ZEvW3G2FzyXsXZwFT1ffKmxoxsSN25olYXBCJxXOED1w87h0L8o4OCvM28_TZeoI3TeRst0ONm3grrnADzuaD6qGDyzVlKalk_v1C4y41CjEeMMw1W6PnI7Tq2d_pm3AtG9d5H7trJTD725zy72_p_w71oum9b3t66_j9_vWkgHq0Aznv_y79u8Zc2_7Dzqem8vuj8B47YYaP_EQAA_22_7D = __webpack_require__(87303);
;// ./src/components/sessions/SessionSummaryPanel.css.ts


var container = '_1ubsyd40';
var copyButton = '_1ubsyd47 _1ubsyd46';
var copyFailureText = '_1ubsyd49';
var emptyState = '_1ubsyd4o';
var emptyStateHeading = '_1ubsyd4p';
var errorCard = '_1ubsyd4f';
var errorDetails = '_1ubsyd4j';
var errorDetailsBody = '_1ubsyd4k';
var errorLead = '_1ubsyd4g';
var errorStageText = '_1ubsyd4h';
var errorTimestamp = '_1ubsyd4i';
var glanceCard = '_1ubsyd4a';
var glanceEmptyText = '_1ubsyd4e';
var glancePill = '_1ubsyd4d';
var glancePills = '_1ubsyd4c';
var glanceTitle = '_1ubsyd4b';
var header = '_1ubsyd41';
var primaryButton = '_1ubsyd48 _1ubsyd46';
var skeletonBlock = '_1ubsyd4t _1ubsyd4s';
var skeletonBlockReducedMotion = '_1ubsyd4u _1ubsyd4s';
var skeletonHeading = '_1ubsyd4v';
var skeletonLine = '_1ubsyd4x';
var skeletonLineWidth40 = '_1ubsyd4z';
var skeletonLineWidth60 = '_1ubsyd4y';
var skeletonPill = '_1ubsyd4w';
var skeletonSection = '_1ubsyd4r';
var staleBanner = '_1ubsyd4l';
var staleBannerActions = '_1ubsyd4n';
var staleBannerText = '_1ubsyd4m';
var statusText = '_1ubsyd44';
var title = '_1ubsyd43';
var titleGroup = '_1ubsyd42';
var toolbar = '_1ubsyd45';
;// ./src/components/sessions/SessionSummaryPanel.tsx
/* __next_internal_client_entry_do_not_use__ SessionSummaryPanel auto */ 
// +feature: session-summary-tab









// Plain-language lead sentence per `error_stage`, per design/ux.md surface (d) —
// never the raw enum string or a stack trace in the primary error text.
const ERROR_STAGE_COPY = {
    decisions: "Failed while computing approval decisions.",
    diff: "Failed while computing the diff summary.",
    persist: "Failed while saving the generated summary.",
    "restart-interrupted": "Generation was interrupted, possibly by a server restart."
};
function stageSentence(stage) {
    return ERROR_STAGE_COPY[stage] ?? "Something went wrong while generating this summary.";
}
// `error !== null` only reflects a failed *initial* fetch (data is still
// null) — a poll-tick failure after data has already loaded once leaves the
// prior data in place and is not surfaced as this phase, since polling
// continues to retry on the next tick.
function computePhase(data, neverResolved, error) {
    if (neverResolved) return "empty";
    if (data === null && error !== null) return "transport-error";
    if (data === null) return "loading";
    if (isGenerating(data.status)) return "loading";
    if (data.status === types_pb/* SessionSummaryStatus */.z.ERROR) {
        return data.markdown ? "error-stale" : "error";
    }
    return "ready";
}
const SKELETON_SECTIONS = [
    {
        title: "Decisions",
        blocks: [
            {
                kind: "heading"
            },
            {
                kind: "pill"
            },
            {
                kind: "pill"
            },
            {
                kind: "pill"
            },
            {
                kind: "pill"
            },
            {
                kind: "pill"
            }
        ]
    },
    {
        title: "What Was Done",
        blocks: [
            {
                kind: "heading"
            },
            {
                kind: "line"
            },
            {
                kind: "line"
            },
            {
                kind: "lineWidth60"
            }
        ]
    },
    {
        title: "Changes",
        blocks: [
            {
                kind: "heading"
            },
            {
                kind: "line"
            },
            {
                kind: "lineWidth40"
            }
        ]
    },
    {
        title: "Timeline",
        blocks: [
            {
                kind: "heading"
            },
            {
                kind: "line"
            }
        ]
    },
    {
        title: "Token Usage",
        blocks: [
            {
                kind: "heading"
            },
            {
                kind: "line"
            }
        ]
    }
];
function skeletonShapeClass(kind) {
    switch(kind){
        case "heading":
            return skeletonHeading;
        case "pill":
            return skeletonPill;
        case "lineWidth60":
            return skeletonLineWidth60;
        case "lineWidth40":
            return skeletonLineWidth40;
        case "line":
        default:
            return skeletonLine;
    }
}
/**
 * Reads `prefers-reduced-motion` at the JS layer (not just via a CSS media
 * query) so the shimmer-vs-static class choice is directly assertable in
 * tests via `window.matchMedia` mocking (UX-AC-14, Task 3.1.2c).
 */ function usePrefersReducedMotion() {
    const [reduced, setReduced] = (0,react.useState)(()=>{
        if (true) return false;
        return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    });
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (true) return;
        const mql = window.matchMedia("(prefers-reduced-motion: reduce)");
        const handler = ()=>setReduced(mql.matches);
        handler();
        mql.addEventListener?.("change", handler);
        return ()=>mql.removeEventListener?.("change", handler);
    }, []);
    return reduced;
}
function SummarySkeleton() {
    const reducedMotion = usePrefersReducedMotion();
    const blockClass = reducedMotion ? skeletonBlockReducedMotion : skeletonBlock;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(react_jsx_runtime.Fragment, {
        children: SKELETON_SECTIONS.map((section)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: skeletonSection,
                children: section.blocks.map((block, i)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        "data-testid": "summary-skeleton-block",
                        className: `${blockClass} ${skeletonShapeClass(block.kind)}`
                    }, `${section.title}-${i}`))
            }, section.title))
    });
}
// ---------------------------------------------------------------------------
// Decisions-at-a-glance card — built from structured `data.decisions` proto
// fields, not parsed from markdown, so it's unmissable near the top rather
// than buried in the flowing document (research/ux.md §5).
// ---------------------------------------------------------------------------
function DecisionsGlanceCard({ decisions }) {
    const autoApproved = decisions?.autoApproved ?? 0;
    const manuallyApproved = decisions?.manuallyApproved ?? 0;
    const denied = decisions?.denied ?? 0;
    const reviewQueueResolved = decisions?.reviewQueueResolved ?? 0;
    const stillOpen = decisions?.stillOpen ?? 0;
    const total = autoApproved + manuallyApproved + denied + reviewQueueResolved + stillOpen;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: glanceCard,
        "data-testid": "summary-decisions-glance",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: glanceTitle,
                children: "Decisions at a glance"
            }),
            total === 0 ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: glanceEmptyText,
                children: "No approval requests occurred during this session."
            }) : /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: glancePills,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        className: glancePill,
                        children: [
                            "✓ ",
                            autoApproved,
                            " auto-approved"
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        className: glancePill,
                        children: [
                            "✓ ",
                            manuallyApproved,
                            " manually approved"
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        className: glancePill,
                        children: [
                            "✕ ",
                            denied,
                            " denied"
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        className: glancePill,
                        children: [
                            "◔ ",
                            reviewQueueResolved,
                            " review-resolved"
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        className: glancePill,
                        children: [
                            "● ",
                            stillOpen,
                            " still open"
                        ]
                    })
                ]
            })
        ]
    });
}
// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------
function SessionSummaryPanel({ sessionId }) {
    const { data, error, neverResolved, regenerate, refetch, copy } = useSessionSummary(sessionId);
    const phase = computePhase(data, neverResolved, error);
    const [liveMessage, setLiveMessage] = (0,react.useState)("");
    const [regenerating, setRegenerating] = (0,react.useState)(false);
    const regeneratingRef = (0,react.useRef)(false);
    const [retrying, setRetrying] = (0,react.useState)(false);
    const [copyState, setCopyState] = (0,react.useState)("idle");
    const copyRevertTimeoutRef = (0,react.useRef)(null);
    const prevPhaseRef = (0,react.useRef)(null);
    const setRegeneratingBoth = (0,react.useCallback)((value)=>{
        regeneratingRef.current = value;
        setRegenerating(value);
    }, []);
    // Announce state transitions through the single shared aria-live region,
    // per design/ux.md's exact-strings table (surface (e)).
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (prevPhaseRef.current === phase) return;
        prevPhaseRef.current = phase;
        switch(phase){
            case "loading":
                setLiveMessage("Generating summary…");
                break;
            case "ready":
                setLiveMessage(regeneratingRef.current ? "Summary regenerated." : "Summary ready.");
                setRegeneratingBoth(false);
                break;
            case "error":
                setLiveMessage(`Summary generation failed: ${stageSentence(data?.errorStage ?? "")}`);
                setRegeneratingBoth(false);
                break;
            case "error-stale":
                setLiveMessage("Showing the previous summary. Regeneration failed — see the banner for details.");
                setRegeneratingBoth(false);
                break;
            case "empty":
                setLiveMessage("No summary available for this session.");
                break;
            case "transport-error":
                setLiveMessage(`Couldn't load this summary: ${error?.message ?? "unknown error"}`);
                break;
        }
    // data/error are read for their current-render value only within the
    // specific phase-transition branches above — intentionally not
    // dependencies so we don't re-run this effect on every poll tick.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        phase,
        setRegeneratingBoth
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            if (copyRevertTimeoutRef.current) clearTimeout(copyRevertTimeoutRef.current);
        };
    }, []);
    const handleCopy = (0,react.useCallback)(async ()=>{
        const ok = await copy();
        if (ok) {
            setCopyState("success");
            setLiveMessage("Summary copied to clipboard.");
            if (copyRevertTimeoutRef.current) clearTimeout(copyRevertTimeoutRef.current);
            copyRevertTimeoutRef.current = setTimeout(()=>setCopyState("idle"), 1500);
        } else {
            setCopyState("failure");
            setLiveMessage("Copy failed. Select the text and copy manually.");
        }
    }, [
        copy
    ]);
    const handleRegenerate = (0,react.useCallback)(async ()=>{
        setRegeneratingBoth(true);
        setLiveMessage("Regenerating summary…");
        try {
            await regenerate();
        } catch (err) {
            // regenerate() rethrows after a failed RPC. data/phase don't change in
            // that case (nothing new was fetched), so the phase-transition effect
            // above never re-fires — its early-return means regeneratingRef would
            // otherwise stay stuck `true` forever, causing a later *unrelated*
            // transition to "ready" (e.g. via ordinary polling) to wrongly
            // announce "Summary regenerated." instead of "Summary ready." Reset it
            // here, and announce the failure explicitly since the phase-transition
            // effect's error branches won't fire for this either.
            regeneratingRef.current = false;
            const message = err instanceof Error ? err.message : "unknown error";
            setLiveMessage(`Regeneration failed: ${message}`);
        } finally{
            // Only clears the button's in-flight state here — regeneratingRef
            // itself stays true until the phase-transition effect above observes
            // a real terminal phase, so "Summary regenerated." vs "Summary
            // ready." still resolves correctly once polling reaches READY. This
            // finally is what prevents the button getting stuck disabled forever
            // when regenerate() fails (data/phase never change in that case, so
            // the phase-transition effect's early-return never fires).
            setRegenerating(false);
        }
    }, [
        regenerate,
        setRegeneratingBoth
    ]);
    const handleRetry = (0,react.useCallback)(async ()=>{
        setRetrying(true);
        try {
            await refetch();
        } finally{
            setRetrying(false);
        }
    }, [
        refetch
    ]);
    let body;
    if (phase === "empty") {
        body = /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: emptyState,
            "data-testid": "summary-empty-state",
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: emptyStateHeading,
                children: "No summary available for this session."
            })
        });
    } else if (phase === "transport-error") {
        body = /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: errorCard,
            "data-testid": "summary-transport-error",
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: errorLead,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            "aria-hidden": "true",
                            children: "⚠"
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            children: "Couldn't load this summary."
                        })
                    ]
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: errorStageText,
                    children: error?.message ?? "Something went wrong while loading this summary."
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    type: "button",
                    className: primaryButton,
                    onClick: handleRetry,
                    disabled: retrying,
                    "data-testid": "summary-transport-error-retry",
                    children: retrying ? "⟳ Retrying…" : "↻ Retry"
                })
            ]
        });
    } else if (phase === "loading") {
        body = /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: header,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: titleGroup,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                    className: title,
                                    children: "Session Summary"
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                    className: statusText,
                                    "aria-hidden": "true",
                                    children: "⟳ Generating…"
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                            className: toolbar,
                            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                type: "button",
                                className: copyButton,
                                "aria-label": "Copy summary as Markdown",
                                disabled: true,
                                children: "\uD83D\uDCCB Copy as Markdown"
                            })
                        })
                    ]
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)(SummarySkeleton, {})
            ]
        });
    } else {
        // phase is "ready" | "error" | "error-stale" here — data is guaranteed non-null.
        const summary = data;
        const isStale = phase === "error-stale";
        const isBareError = phase === "error";
        body = /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: header,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: titleGroup,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                    className: title,
                                    children: [
                                        "Session Summary",
                                        summary.sessionTitle ? `: ${summary.sessionTitle}` : ""
                                    ]
                                }),
                                !isBareError && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                    className: statusText,
                                    children: isStale ? "⚠ Regeneration failed" : `✓ Ready · generated ${(0,timestamp/* formatDate */.Yq)(summary.generatedAt)}`
                                })
                            ]
                        }),
                        !isBareError && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                            className: toolbar,
                            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                type: "button",
                                className: copyButton,
                                "aria-label": "Copy summary as Markdown",
                                onClick: handleCopy,
                                children: copyState === "success" ? "✓ Copied" : "📋 Copy as Markdown"
                            })
                        })
                    ]
                }),
                copyState === "failure" && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: copyFailureText,
                    "data-testid": "summary-copy-failure",
                    children: "Copy failed — select the text below and copy manually."
                }),
                isBareError && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: errorCard,
                    "data-testid": "summary-error-card",
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: errorLead,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                    "aria-hidden": "true",
                                    children: "⚠"
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                    children: "Couldn't finish generating this summary."
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                            className: errorStageText,
                            children: stageSentence(summary.errorStage)
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: errorTimestamp,
                            children: [
                                "Last attempt: ",
                                (0,timestamp/* formatDate */.Yq)(summary.generatedAt)
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            type: "button",
                            className: primaryButton,
                            onClick: handleRegenerate,
                            disabled: regenerating,
                            children: regenerating ? "⟳ Regenerating…" : "↻ Regenerate"
                        }),
                        summary.errorMessage && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("details", {
                            className: errorDetails,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("summary", {
                                    children: "Details"
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                    className: errorDetailsBody,
                                    children: summary.errorMessage
                                })
                            ]
                        })
                    ]
                }),
                isStale && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: staleBanner,
                    "data-testid": "summary-stale-banner",
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: staleBannerText,
                            children: [
                                "Showing the summary from the last successful generation, dated",
                                " ",
                                (0,timestamp/* formatDate */.Yq)(summary.generatedAt),
                                " — regeneration failed, see error above."
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: staleBannerActions,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: primaryButton,
                                    onClick: handleRegenerate,
                                    disabled: regenerating,
                                    "data-testid": "summary-try-again",
                                    children: regenerating ? "⟳ Regenerating…" : "↻ Try again"
                                }),
                                summary.errorMessage && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("details", {
                                    className: errorDetails,
                                    children: [
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("summary", {
                                            children: "Details"
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                            className: errorDetailsBody,
                                            children: summary.errorMessage
                                        })
                                    ]
                                })
                            ]
                        })
                    ]
                }),
                (phase === "ready" || isStale) && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)(DecisionsGlanceCard, {
                            decisions: summary.decisions
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                            className: markdownBody_css/* markdownBody */.e,
                            "data-testid": "summary-markdown-body",
                            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(lib/* Markdown */.oz, {
                                remarkPlugins: [
                                    remark_gfm_lib/* default */.A
                                ],
                                children: summary.markdown
                            })
                        })
                    ]
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: container,
        "data-testid": "session-summary-panel",
        role: phase === "empty" ? undefined : "region",
        "aria-label": phase === "empty" ? undefined : "Session summary",
        "aria-busy": phase === "loading" ? "true" : undefined,
        children: [
            body,
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                "aria-atomic": "true",
                className: LiveRegion_css/* srOnly */.s,
                children: liveMessage
            })
        ]
    });
}


/***/ }),

/***/ 87303:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes _1ubsyd4q {\n  0% {\n    background-position: 200% 0;\n  }\n  100% {\n    background-position: -200% 0;\n  }\n}\n._1ubsyd40 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz31);\n  padding: var(--_19ez2rz31);\n}\n._1ubsyd41 {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  gap: var(--_19ez2rz30);\n  flex-wrap: wrap;\n}\n._1ubsyd42 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n  flex-wrap: wrap;\n}\n._1ubsyd43 {\n  font-size: var(--_19ez2rz3d);\n  font-weight: var(--_19ez2rz3i);\n  color: var(--_19ez2rz0);\n}\n._1ubsyd44 {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz2);\n}\n._1ubsyd45 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n  flex-wrap: wrap;\n}\n._1ubsyd46 {\n  display: inline-flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  cursor: pointer;\n  border: 1px solid var(--_19ez2rzg);\n  background: transparent;\n  color: var(--_19ez2rz1);\n}\n._1ubsyd46:hover {\n  background: var(--_19ez2rz8);\n  border-color: var(--_19ez2rzh);\n  color: var(--_19ez2rz0);\n}\n._1ubsyd46:disabled {\n  opacity: 0.5;\n  cursor: not-allowed;\n}\n._1ubsyd48 {\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rzq);\n  border-color: var(--_19ez2rzm);\n}\n._1ubsyd48:hover {\n  background: var(--_19ez2rzn);\n  border-color: var(--_19ez2rzn);\n  color: var(--_19ez2rzq);\n}\n._1ubsyd49 {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz10);\n}\n._1ubsyd4a {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz30);\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rzf);\n  background: var(--_19ez2rz7);\n}\n._1ubsyd4b {\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3h);\n  color: var(--_19ez2rz1);\n  text-transform: uppercase;\n  letter-spacing: 0.05em;\n}\n._1ubsyd4c {\n  display: flex;\n  flex-wrap: wrap;\n  gap: var(--_19ez2rz2z);\n}\n._1ubsyd4d {\n  display: inline-flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  border-radius: var(--_19ez2rz39);\n  font-size: var(--_19ez2rz3a);\n  font-weight: var(--_19ez2rz3g);\n  background: var(--_19ez2rzd);\n  color: var(--_19ez2rz0);\n}\n._1ubsyd4e {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz2);\n  font-style: italic;\n}\n._1ubsyd4f {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz31);\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rz13);\n  background: var(--_19ez2rz11);\n}\n._1ubsyd4g {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n  font-size: var(--_19ez2rz3c);\n  font-weight: var(--_19ez2rz3h);\n  color: var(--_19ez2rz12);\n}\n._1ubsyd4h {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz12);\n}\n._1ubsyd4i {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz2);\n}\n._1ubsyd4j {\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz1);\n}\n._1ubsyd4k {\n  margin-top: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2z);\n  border-radius: var(--_19ez2rz36);\n  background: var(--_19ez2rzd);\n  font-family: var(--_19ez2rz2u);\n  white-space: pre-wrap;\n  word-break: break-word;\n}\n._1ubsyd4l {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz30);\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rzx);\n  background: var(--_19ez2rzy);\n}\n._1ubsyd4m {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rzz);\n}\n._1ubsyd4n {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n}\n._1ubsyd4o {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz32);\n  align-items: center;\n  text-align: center;\n  color: var(--_19ez2rz2);\n}\n._1ubsyd4p {\n  font-size: var(--_19ez2rz3c);\n  font-weight: var(--_19ez2rz3h);\n  color: var(--_19ez2rz1);\n}\n._1ubsyd4r {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n}\n._1ubsyd4s {\n  border-radius: var(--_19ez2rz36);\n  background: var(--_19ez2rzf);\n}\n._1ubsyd4t {\n  background: linear-gradient(90deg, var(--_19ez2rzf) 25%, var(--_19ez2rzg) 50%, var(--_19ez2rzf) 75%);\n  background-size: 200% 100%;\n  animation: _1ubsyd4q 1.5s infinite;\n}\n._1ubsyd4u {\n  background: var(--_19ez2rzg);\n}\n._1ubsyd4v {\n  width: 140px;\n  height: 16px;\n}\n._1ubsyd4w {\n  width: 60px;\n  height: 24px;\n  border-radius: var(--_19ez2rz39);\n}\n._1ubsyd4x {\n  width: 100%;\n  height: 14px;\n}\n._1ubsyd4y {\n  width: 60%;\n  height: 14px;\n}\n._1ubsyd4z {\n  width: 40%;\n  height: 14px;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ })

};
;