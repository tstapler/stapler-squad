(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[5469],{

/***/ 6112:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 8462:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   c: () => (/* binding */ createRuntimeFn)
/* harmony export */ });
/* unused harmony export m */
function toPrimitive(t, r) {
  if ("object" != typeof t || !t) return t;
  var e = t[Symbol.toPrimitive];
  if (void 0 !== e) {
    var i = e.call(t, r || "default");
    if ("object" != typeof i) return i;
    throw new TypeError("@@toPrimitive must return a primitive value.");
  }
  return ("string" === r ? String : Number)(t);
}

function toPropertyKey(t) {
  var i = toPrimitive(t, "string");
  return "symbol" == typeof i ? i : String(i);
}

function _defineProperty(obj, key, value) {
  key = toPropertyKey(key);
  if (key in obj) {
    Object.defineProperty(obj, key, {
      value: value,
      enumerable: true,
      configurable: true,
      writable: true
    });
  } else {
    obj[key] = value;
  }
  return obj;
}

function ownKeys(e, r) {
  var t = Object.keys(e);
  if (Object.getOwnPropertySymbols) {
    var o = Object.getOwnPropertySymbols(e);
    r && (o = o.filter(function (r) {
      return Object.getOwnPropertyDescriptor(e, r).enumerable;
    })), t.push.apply(t, o);
  }
  return t;
}
function _objectSpread2(e) {
  for (var r = 1; r < arguments.length; r++) {
    var t = null != arguments[r] ? arguments[r] : {};
    r % 2 ? ownKeys(Object(t), !0).forEach(function (r) {
      _defineProperty(e, r, t[r]);
    }) : Object.getOwnPropertyDescriptors ? Object.defineProperties(e, Object.getOwnPropertyDescriptors(t)) : ownKeys(Object(t)).forEach(function (r) {
      Object.defineProperty(e, r, Object.getOwnPropertyDescriptor(t, r));
    });
  }
  return e;
}

function mapValues(input, fn) {
  var result = {};
  for (var _key in input) {
    result[_key] = fn(input[_key], _key);
  }
  return result;
}

var shouldApplyCompound = (compoundCheck, selections, defaultVariants) => {
  for (var key of Object.keys(compoundCheck)) {
    var _selections$key;
    if (compoundCheck[key] !== ((_selections$key = selections[key]) !== null && _selections$key !== void 0 ? _selections$key : defaultVariants[key])) {
      return false;
    }
  }
  return true;
};
var createRuntimeFn = config => {
  var runtimeFn = options => {
    var className = config.defaultClassName;
    var selections = _objectSpread2(_objectSpread2({}, config.defaultVariants), options);
    for (var variantName in selections) {
      var _selections$variantNa;
      var variantSelection = (_selections$variantNa = selections[variantName]) !== null && _selections$variantNa !== void 0 ? _selections$variantNa : config.defaultVariants[variantName];
      if (variantSelection != null) {
        var selection = variantSelection;
        if (typeof selection === 'boolean') {
          // @ts-expect-error
          selection = selection === true ? 'true' : 'false';
        }
        var selectionClassName =
        // @ts-expect-error
        config.variantClassNames[variantName][selection];
        if (selectionClassName) {
          className += ' ' + selectionClassName;
        }
      }
    }
    for (var [compoundCheck, compoundClassName] of config.compoundVariants) {
      if (shouldApplyCompound(compoundCheck, selections, config.defaultVariants)) {
        className += ' ' + compoundClassName;
      }
    }
    return className;
  };
  runtimeFn.variants = () => Object.keys(config.variantClassNames);
  runtimeFn.classNames = {
    get base() {
      return config.defaultClassName.split(' ')[0];
    },
    get variants() {
      return mapValues(config.variantClassNames, classNames => mapValues(classNames, className => className.split(' ')[0]));
    }
  };
  return runtimeFn;
};




/***/ }),

/***/ 9833:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  EscapeAnalyticsPage: () => (/* binding */ EscapeAnalyticsPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
;// ./src/lib/hooks/useEscapeAnalytics.ts
/* __next_internal_client_entry_do_not_use__ useEscapeAnalyticsSummary,useEscapeAnalyticsGlobalSummary,useEscapeEvents auto */ // +feature: escape-analytics







function useEscapeAnalyticsSummary(sessionId) {
    let enabled = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : true;
    const [histogram, setHistogram] = (0,react.useState)([]);
    const [totalSequences, setTotalSequences] = (0,react.useState)(0n);
    const [totalMangled, setTotalMangled] = (0,react.useState)(0n);
    const [mangleRate, setMangleRate] = (0,react.useState)(0);
    const [correlationOutcomes, setCorrelationOutcomes] = (0,react.useState)(0n);
    const [correlationCoverage, setCorrelationCoverage] = (0,react.useState)(0);
    const [captureHealthy, setCaptureHealthy] = (0,react.useState)(true);
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const fetchSummary = (0,react.useCallback)(async ()=>{
        if (!clientRef.current || !sessionId || !enabled) return;
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(session_pb/* GetEscapeAnalyticsSummaryRequestSchema */.mdm, {
                sessionId
            });
            const resp = await clientRef.current.getEscapeAnalyticsSummary(req);
            var _resp_histogram;
            setHistogram((_resp_histogram = resp.histogram) !== null && _resp_histogram !== void 0 ? _resp_histogram : []);
            var _resp_totalSequences;
            setTotalSequences((_resp_totalSequences = resp.totalSequences) !== null && _resp_totalSequences !== void 0 ? _resp_totalSequences : 0n);
            var _resp_totalMangled;
            setTotalMangled((_resp_totalMangled = resp.totalMangled) !== null && _resp_totalMangled !== void 0 ? _resp_totalMangled : 0n);
            var _resp_mangleRate;
            setMangleRate((_resp_mangleRate = resp.mangleRate) !== null && _resp_mangleRate !== void 0 ? _resp_mangleRate : 0);
            var _resp_correlationOutcomes;
            setCorrelationOutcomes((_resp_correlationOutcomes = resp.correlationOutcomes) !== null && _resp_correlationOutcomes !== void 0 ? _resp_correlationOutcomes : 0n);
            var _resp_correlationCoverage;
            setCorrelationCoverage((_resp_correlationCoverage = resp.correlationCoverage) !== null && _resp_correlationCoverage !== void 0 ? _resp_correlationCoverage : 0);
            var _resp_captureHealthy;
            setCaptureHealthy((_resp_captureHealthy = resp.captureHealthy) !== null && _resp_captureHealthy !== void 0 ? _resp_captureHealthy : false);
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Failed to fetch escape analytics summary");
            setError(e);
            console.error("Failed to fetch escape analytics summary:", e);
        } finally{
            setLoading(false);
        }
    }, [
        sessionId,
        enabled
    ]);
    (0,react.useEffect)(()=>{
        if (!clientRef.current || !sessionId || !enabled) return;
        let cancelled = false;
        setLoading(true);
        setError(null);
        const fetchData = async ()=>{
            try {
                const req = (0,create/* create */.v)(session_pb/* GetEscapeAnalyticsSummaryRequestSchema */.mdm, {
                    sessionId
                });
                const resp = await clientRef.current.getEscapeAnalyticsSummary(req);
                if (cancelled) return;
                var _resp_histogram;
                setHistogram((_resp_histogram = resp.histogram) !== null && _resp_histogram !== void 0 ? _resp_histogram : []);
                var _resp_totalSequences;
                setTotalSequences((_resp_totalSequences = resp.totalSequences) !== null && _resp_totalSequences !== void 0 ? _resp_totalSequences : 0n);
                var _resp_totalMangled;
                setTotalMangled((_resp_totalMangled = resp.totalMangled) !== null && _resp_totalMangled !== void 0 ? _resp_totalMangled : 0n);
                var _resp_mangleRate;
                setMangleRate((_resp_mangleRate = resp.mangleRate) !== null && _resp_mangleRate !== void 0 ? _resp_mangleRate : 0);
                var _resp_correlationOutcomes;
                setCorrelationOutcomes((_resp_correlationOutcomes = resp.correlationOutcomes) !== null && _resp_correlationOutcomes !== void 0 ? _resp_correlationOutcomes : 0n);
                var _resp_correlationCoverage;
                setCorrelationCoverage((_resp_correlationCoverage = resp.correlationCoverage) !== null && _resp_correlationCoverage !== void 0 ? _resp_correlationCoverage : 0);
                var _resp_captureHealthy;
                setCaptureHealthy((_resp_captureHealthy = resp.captureHealthy) !== null && _resp_captureHealthy !== void 0 ? _resp_captureHealthy : false);
            } catch (err) {
                if (cancelled) return;
                const e = err instanceof Error ? err : new Error("Failed to fetch escape analytics summary");
                setError(e);
                console.error("Failed to fetch escape analytics summary:", e);
            } finally{
                if (!cancelled) setLoading(false);
            }
        };
        fetchData();
        return ()=>{
            cancelled = true;
        };
    }, [
        sessionId,
        enabled
    ]);
    return {
        histogram,
        totalSequences,
        totalMangled,
        mangleRate,
        correlationOutcomes,
        correlationCoverage,
        captureHealthy,
        loading,
        error,
        refresh: fetchSummary
    };
}
function useEscapeAnalyticsGlobalSummary() {
    let enabled = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : true, filters = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    const { startTime, endTime } = filters;
    const [histogram, setHistogram] = (0,react.useState)([]);
    const [totalSequences, setTotalSequences] = (0,react.useState)(0n);
    const [totalMangled, setTotalMangled] = (0,react.useState)(0n);
    const [mangleRate, setMangleRate] = (0,react.useState)(0);
    const [perSession, setPerSession] = (0,react.useState)([]);
    const [perProject, setPerProject] = (0,react.useState)([]);
    const [correlationOutcomes, setCorrelationOutcomes] = (0,react.useState)(0n);
    const [correlationCoverage, setCorrelationCoverage] = (0,react.useState)(0);
    const [captureHealthy, setCaptureHealthy] = (0,react.useState)(true);
    const [droppedEvents, setDroppedEvents] = (0,react.useState)(0n);
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const buildRequest = (0,react.useCallback)(()=>(0,create/* create */.v)(session_pb/* GetEscapeAnalyticsGlobalSummaryRequestSchema */.Tje, {
            startTime: startTime ? (0,timestamp/* timestampFromDate */.LI)(startTime) : undefined,
            endTime: endTime ? (0,timestamp/* timestampFromDate */.LI)(endTime) : undefined
        }), [
        startTime,
        endTime
    ]);
    const fetchGlobalSummary = (0,react.useCallback)(async ()=>{
        if (!clientRef.current || !enabled) return;
        setLoading(true);
        setError(null);
        try {
            const req = buildRequest();
            const resp = await clientRef.current.getEscapeAnalyticsGlobalSummary(req);
            var _resp_histogram;
            setHistogram((_resp_histogram = resp.histogram) !== null && _resp_histogram !== void 0 ? _resp_histogram : []);
            var _resp_totalSequences;
            setTotalSequences((_resp_totalSequences = resp.totalSequences) !== null && _resp_totalSequences !== void 0 ? _resp_totalSequences : 0n);
            var _resp_totalMangled;
            setTotalMangled((_resp_totalMangled = resp.totalMangled) !== null && _resp_totalMangled !== void 0 ? _resp_totalMangled : 0n);
            var _resp_mangleRate;
            setMangleRate((_resp_mangleRate = resp.mangleRate) !== null && _resp_mangleRate !== void 0 ? _resp_mangleRate : 0);
            var _resp_perSession;
            setPerSession((_resp_perSession = resp.perSession) !== null && _resp_perSession !== void 0 ? _resp_perSession : []);
            var _resp_perProject;
            setPerProject((_resp_perProject = resp.perProject) !== null && _resp_perProject !== void 0 ? _resp_perProject : []);
            var _resp_correlationOutcomes;
            setCorrelationOutcomes((_resp_correlationOutcomes = resp.correlationOutcomes) !== null && _resp_correlationOutcomes !== void 0 ? _resp_correlationOutcomes : 0n);
            var _resp_correlationCoverage;
            setCorrelationCoverage((_resp_correlationCoverage = resp.correlationCoverage) !== null && _resp_correlationCoverage !== void 0 ? _resp_correlationCoverage : 0);
            var _resp_captureHealthy;
            setCaptureHealthy((_resp_captureHealthy = resp.captureHealthy) !== null && _resp_captureHealthy !== void 0 ? _resp_captureHealthy : false);
            var _resp_droppedEvents;
            setDroppedEvents((_resp_droppedEvents = resp.droppedEvents) !== null && _resp_droppedEvents !== void 0 ? _resp_droppedEvents : 0n);
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Failed to fetch escape analytics global summary");
            setError(e);
            console.error("Failed to fetch escape analytics global summary:", e);
        } finally{
            setLoading(false);
        }
    }, [
        enabled,
        buildRequest
    ]);
    (0,react.useEffect)(()=>{
        if (!clientRef.current || !enabled) return;
        let cancelled = false;
        setLoading(true);
        setError(null);
        const fetchData = async ()=>{
            try {
                const req = buildRequest();
                const resp = await clientRef.current.getEscapeAnalyticsGlobalSummary(req);
                if (cancelled) return;
                var _resp_histogram;
                setHistogram((_resp_histogram = resp.histogram) !== null && _resp_histogram !== void 0 ? _resp_histogram : []);
                var _resp_totalSequences;
                setTotalSequences((_resp_totalSequences = resp.totalSequences) !== null && _resp_totalSequences !== void 0 ? _resp_totalSequences : 0n);
                var _resp_totalMangled;
                setTotalMangled((_resp_totalMangled = resp.totalMangled) !== null && _resp_totalMangled !== void 0 ? _resp_totalMangled : 0n);
                var _resp_mangleRate;
                setMangleRate((_resp_mangleRate = resp.mangleRate) !== null && _resp_mangleRate !== void 0 ? _resp_mangleRate : 0);
                var _resp_perSession;
                setPerSession((_resp_perSession = resp.perSession) !== null && _resp_perSession !== void 0 ? _resp_perSession : []);
                var _resp_perProject;
                setPerProject((_resp_perProject = resp.perProject) !== null && _resp_perProject !== void 0 ? _resp_perProject : []);
                var _resp_correlationOutcomes;
                setCorrelationOutcomes((_resp_correlationOutcomes = resp.correlationOutcomes) !== null && _resp_correlationOutcomes !== void 0 ? _resp_correlationOutcomes : 0n);
                var _resp_correlationCoverage;
                setCorrelationCoverage((_resp_correlationCoverage = resp.correlationCoverage) !== null && _resp_correlationCoverage !== void 0 ? _resp_correlationCoverage : 0);
                var _resp_captureHealthy;
                setCaptureHealthy((_resp_captureHealthy = resp.captureHealthy) !== null && _resp_captureHealthy !== void 0 ? _resp_captureHealthy : false);
                var _resp_droppedEvents;
                setDroppedEvents((_resp_droppedEvents = resp.droppedEvents) !== null && _resp_droppedEvents !== void 0 ? _resp_droppedEvents : 0n);
            } catch (err) {
                if (cancelled) return;
                const e = err instanceof Error ? err : new Error("Failed to fetch escape analytics global summary");
                setError(e);
                console.error("Failed to fetch escape analytics global summary:", e);
            } finally{
                if (!cancelled) setLoading(false);
            }
        };
        fetchData();
        return ()=>{
            cancelled = true;
        };
    }, [
        enabled,
        buildRequest
    ]);
    return {
        histogram,
        totalSequences,
        totalMangled,
        mangleRate,
        correlationOutcomes,
        correlationCoverage,
        captureHealthy,
        droppedEvents,
        perSession,
        perProject,
        loading,
        error,
        refresh: fetchGlobalSummary
    };
}
function useEscapeEvents(sessionId) {
    let filters = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    const { stage = "", sequenceType = "", mangledOnly = false, pageSize = 50 } = filters;
    const [events, setEvents] = (0,react.useState)([]);
    const [nextPageToken, setNextPageToken] = (0,react.useState)("");
    const [totalCount, setTotalCount] = (0,react.useState)(0);
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    const pageTokenRef = (0,react.useRef)("");
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    // Reset and re-fetch when session or filters change
    (0,react.useEffect)(()=>{
        pageTokenRef.current = "";
        setEvents([]);
        setNextPageToken("");
        setTotalCount(0);
        setError(null);
        if (!clientRef.current || !sessionId) return;
        let cancelled = false;
        setLoading(true);
        const fetchPage = async ()=>{
            try {
                const req = (0,create/* create */.v)(session_pb/* QueryEscapeAnalyticsRequestSchema */.w67, {
                    sessionId,
                    stage,
                    sequenceType,
                    mangledOnly,
                    pageSize,
                    pageToken: ""
                });
                const resp = await clientRef.current.queryEscapeAnalytics(req);
                if (cancelled) return;
                var _resp_events;
                setEvents((_resp_events = resp.events) !== null && _resp_events !== void 0 ? _resp_events : []);
                var _resp_nextPageToken;
                setNextPageToken((_resp_nextPageToken = resp.nextPageToken) !== null && _resp_nextPageToken !== void 0 ? _resp_nextPageToken : "");
                var _resp_totalCount;
                setTotalCount((_resp_totalCount = resp.totalCount) !== null && _resp_totalCount !== void 0 ? _resp_totalCount : 0);
                var _resp_nextPageToken1;
                pageTokenRef.current = (_resp_nextPageToken1 = resp.nextPageToken) !== null && _resp_nextPageToken1 !== void 0 ? _resp_nextPageToken1 : "";
            } catch (err) {
                if (cancelled) return;
                const e = err instanceof Error ? err : new Error("Failed to fetch escape events");
                setError(e);
                console.error("Failed to fetch escape events:", e);
            } finally{
                if (!cancelled) setLoading(false);
            }
        };
        fetchPage();
        return ()=>{
            cancelled = true;
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionId,
        stage,
        sequenceType,
        mangledOnly,
        pageSize
    ]);
    const fetchNextPage = (0,react.useCallback)(async ()=>{
        if (!clientRef.current || !sessionId || !pageTokenRef.current) return;
        setLoading(true);
        setError(null);
        try {
            const req = (0,create/* create */.v)(session_pb/* QueryEscapeAnalyticsRequestSchema */.w67, {
                sessionId,
                stage,
                sequenceType,
                mangledOnly,
                pageSize,
                pageToken: pageTokenRef.current
            });
            const resp = await clientRef.current.queryEscapeAnalytics(req);
            setEvents((prev)=>{
                var _resp_events;
                return [
                    ...prev,
                    ...(_resp_events = resp.events) !== null && _resp_events !== void 0 ? _resp_events : []
                ];
            });
            var _resp_nextPageToken;
            setNextPageToken((_resp_nextPageToken = resp.nextPageToken) !== null && _resp_nextPageToken !== void 0 ? _resp_nextPageToken : "");
            var _resp_totalCount;
            setTotalCount((_resp_totalCount = resp.totalCount) !== null && _resp_totalCount !== void 0 ? _resp_totalCount : 0);
            var _resp_nextPageToken1;
            pageTokenRef.current = (_resp_nextPageToken1 = resp.nextPageToken) !== null && _resp_nextPageToken1 !== void 0 ? _resp_nextPageToken1 : "";
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Failed to fetch more escape events");
            setError(e);
            console.error("Failed to fetch more escape events:", e);
        } finally{
            setLoading(false);
        }
    }, [
        sessionId,
        stage,
        sequenceType,
        mangledOnly,
        pageSize
    ]);
    const reset = (0,react.useCallback)(()=>{
        pageTokenRef.current = "";
        setEvents([]);
        setNextPageToken("");
        setTotalCount(0);
        setError(null);
    }, []);
    return {
        events,
        nextPageToken,
        totalCount,
        loading,
        error,
        fetchNextPage,
        reset
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fanalytics%2FSequenceHistogram.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA72U3W6jMBCF7_MUvllp98IRkG42dR-mGvAEZmtsyzb5q_ruKxwSSCEKaqW9QvKMme-Mz8zy9akKMoeEvS8Yk-StgqNgW4WHlwWLXy7JYRHIaMEKo5pat5ESrGA7cD85f02f8ZS5U3b61Ub2JEMlWJokP14WH4tlVyGNFQqjjBtdjPe2Rgfu6YSfw6s8xi1ISbocRdMYDXgIHBSVLSXqgG5YPJuWF_M5Bax9f2tS2yqJVWrSvEIqqyBYtrGHYY1VrHFRv05s30FfOdJvgiVzdE62KO1btIWa1HHUwyZmmB26rTJ7wSqSEvW1NX0AlSLryce3qigg9xYKFEybvQM7lPQUJbUSBEvb_Kv2Tl0OxVvpTKMln8SWESo3TqLjDiQ1fqR6fRd89F7WeDob0aGCQDsc0v6OtBfEs_8eI9azEefa52_jA22PvDA6oA7nfI5aXhzUeSSzd0UxFhzoy3nMZ8ky8wzB32hef0lzmswWPeadSfZnOA6bL0wD3J-GbDTyrm3A9X87cAQ6cN3U6KgQLEDeKHDtgR9Cbqb3wtQCSB8sqYe4NbiSNA9menH2UM_fXVbZ8eZ_cLOYupfoLdNN8gwvzBulz2_cg-T_C6Tz94jkH-CUN4LuBgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fanalytics_2FSequenceHistogram_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA72U3W6jMBCF7_MUvllp98IRkG42dR_mGvAEZmtsyzb5q_ruKxwSSCEKaqW9QvKMme_Mz8zy9akKMoeEvS8Yk_StgqNgW4WHlwWLXy7JYRHIaMEKo5pat5ESrGA7cD85f02f8ZS5U3b61Ub2JEMlWJokP14WH4tlVyGNFQqjjBtdjPe2Rgfu6YSfw6s8xi1ISbocRdMYDXgIHBSVLSXqgG5YPJuWF_M5Bax9f2tS2yqJVWrSvEIqqyBYtrGHYY1VrHFRv05s30FfOdJvgiVzdE62KO1btIWa1HHUwyZmmB26rTJ7wSqSEvW1NX0AlSLryce3qigg9xYKFEybvQM7lPQUJbUSBEvb_Kv2Tl0OxVvpTKMln8SWESo3TqLjDiQ1fqR6fRd89F7WeDob0aGCQDsc0v6OtBfEs_8eI9azEefa52_jA22PvDA6oA7nfI5aXhzUeSSzd0UxFhzoy3nMZ8ky8wzB32hef0lzmswWPeadSfZnOA6bL0wD3J_GbDTyrm3A9X87cAQ6cN3U6KgQLEDeKHDtgR9Cbqb3wtQCSB8sqYe4NbiSNA9menH2UM_fXVbZ8eZ_cLOYupfoLdNN8gwvzBulz2_cg_T_C6Tz94jkH_CUN4LuBgAA_22_7D = __webpack_require__(53972);
;// ./src/components/analytics/SequenceHistogram.css.ts


var barFill = '_4htdba5';
var barMangledFill = '_4htdba6';
var barTrack = '_4htdba4';
var container = '_4htdba0';
var countLabel = '_4htdba7';
var emptyState = '_4htdba1';
var label = '_4htdba3';
var legend = '_4htdba8';
var legendDot = '_4htdbaa';
var legendDotMangled = '_4htdbab';
var legendItem = '_4htdba9';
var row = '_4htdba2';
;// ./src/components/analytics/SequenceHistogram.tsx
// +feature: escape-analytics


function SequenceHistogram(param) {
    let { histogram } = param;
    if (histogram.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: container,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: emptyState,
                children: "No escape sequences recorded."
            })
        });
    }
    // Find the max count to compute relative bar widths
    const maxCount = histogram.reduce((max, entry)=>entry.count > max ? entry.count : max, 0n);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        role: "list",
        "aria-label": "Escape sequence histogram",
        children: [
            histogram.map((entry)=>{
                const totalWidth = maxCount > 0n ? Number(entry.count * 100n / maxCount) : 0;
                const mangledWidth = entry.count > 0n ? Number(entry.mangledCount * 100n / entry.count) : 0;
                const cleanWidth = totalWidth - totalWidth * mangledWidth / 100;
                const mangledFillWidth = totalWidth * mangledWidth / 100;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: row,
                    role: "listitem",
                    "aria-label": "".concat(entry.sequenceType, ": ").concat(entry.count, " total, ").concat(entry.mangledCount, " mangled"),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: label,
                            title: entry.sequenceType,
                            children: entry.sequenceType || "(unknown)"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: barTrack,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: barFill,
                                    style: {
                                        width: "".concat(cleanWidth, "%")
                                    },
                                    "aria-hidden": "true"
                                }),
                                entry.mangledCount > 0n && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: barMangledFill,
                                    style: {
                                        width: "".concat(mangledFillWidth, "%")
                                    },
                                    "aria-hidden": "true"
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: countLabel,
                            children: [
                                entry.count.toString(),
                                entry.mangledCount > 0n && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        " / ",
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            style: {
                                                color: "var(--error-inline)"
                                            },
                                            children: entry.mangledCount.toString()
                                        })
                                    ]
                                })
                            ]
                        })
                    ]
                }, entry.sequenceType);
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: legend,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: legendItem,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: legendDot,
                                "aria-hidden": "true"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: "Total"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: legendItem,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: legendDotMangled,
                                "aria-hidden": "true"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: "Mangled"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fanalytics%2FMangleRateIndicator.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62Uz3KCMBDG7z5FjvUQx2DVig_jLMmCaSFhkkCFju_eIbZjgyAcespM9ptf9ts_WZ1YvWv3abImXwtChLRlDk1M0hwvxwXxJxXSIHdSq5hwnVeF6iIZlDGpwbxQemIHbCPTRu2yi5QghFRZP7phPppoI9BQA0JW9kGz_6OJCSsvxOpcip4MbyrgH5nRlRKU61ybPqxjXRerX4ds2CHkMlNUOixsTDgqh2bE3mYdAiMPTLVy1MoWH-S3LH38E2V2dg8KeVfUYCQoR1VVoJE8Jg6SKgfTXdhOlUuF9PzDYUEiG5_IYA2qMOXXceUlVG7HlaxXh11YWKl8qhP1fa-sk2lDuVYOlfsbGpmfqFkOz9vziTrcazzYJZjs0jm0u_d2p4av9tg5PXmbxWvGeW3IO8ziMTYOZFFIhIlBT8ZRPVIyvINDy8Ym-vbkURY-yv_ja2tCppgoiZg1VXN3f9BlbwtxIiN43iRCHF4cdQaUTbUpYlKVJRoOFv3vg86hobYE7ldzvVpvsTgurt_db4RHQQYAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fanalytics_2FMangleRateIndicator_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62Uz3KCMBDG7z5FjvUQx2DVig_jLMmCaSFhkkCFju_eIbZjgyAcespM9ptf9ts_WZ1YvWv3abImXwtChLRlDk1M0hwvxwXxJxXSIHdSq5hwnVeF6iIZlDGpwbxQemIHbCPTRu2yi5QghFRZP7phPppoI9BQA0JW9kGz_6OJCSsvxOpcip4MbyrgH5nRlRKU61ybPqxjXRerX4ds2CHkMlNUOixsTDgqh2bE3mYdAiMPTLVy1MoWH_S3LH38E2V2dg8KeVfUYCQoR1VVoJE8Jg6SKgfTXdhOlUuF9PzDYUEiG5_IYA2qMOXXceUlVG7HlaxXh11YWKl8qhP1fa_sk2lDuVYOlfsbGpmfqFkOz9vziTrcazzYJZjs0jm0u_d2p4av9tg5PXmbxWvGeW3IO8ziMTYOZFFIhIlBT8ZRPVIyvINDy8Ym_vbkURY_yv_ja2tCppgoiZg1VXN3f9BlbwtxIiN43iRCHF4cdQaUTbUpYlKVJRoOFv3vg86hobYE7ldzvVpvsTgurt_db4RHQQYAAA_22_7D = __webpack_require__(96465);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+recipes@0.5.7+6974e6bd53375d7a/node_modules/@vanilla-extract/recipes/dist/createRuntimeFn-62c9670f.esm.js
var createRuntimeFn_62c9670f_esm = __webpack_require__(8462);
;// ./src/components/analytics/MangleRateIndicator.css.ts



var MangleRateIndicator_css_container = '_1v6z7fb0';
var countItem = '_1v6z7fbc';
var MangleRateIndicator_css_countLabel = '_1v6z7fbe';
var countValue = '_1v6z7fbd';
var countsRow = '_1v6z7fbb';
var rateBadge = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_1v6z7fb6',variantClassNames:{severity:{good:'_1v6z7fb7',warning:'_1v6z7fb8',error:'_1v6z7fb9'}},defaultVariants:{severity:'good'},compoundVariants:[]});
var rateRow = '_1v6z7fb1';
var rateValue = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_1v6z7fb2',variantClassNames:{severity:{good:'_1v6z7fb3',warning:'_1v6z7fb4',error:'_1v6z7fb5'}},defaultVariants:{severity:'good'},compoundVariants:[]});
var subtitle = '_1v6z7fba';
;// ./src/components/analytics/MangleRateIndicator.tsx
// +feature: escape-analytics


function getSeverity(mangleRate) {
    if (mangleRate < 0.01) return "good";
    if (mangleRate <= 0.05) return "warning";
    return "error";
}
function getSeverityLabel(severity) {
    switch(severity){
        case "good":
            return "Healthy";
        case "warning":
            return "Elevated";
        case "error":
            return "High";
    }
}
function MangleRateIndicator(param) {
    let { mangleRate, totalSequences, totalMangled } = param;
    const severity = getSeverity(mangleRate);
    const percentage = (mangleRate * 100).toFixed(2);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: MangleRateIndicator_css_container,
        "data-testid": "mangle-rate-indicator",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: rateRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: rateValue({
                            severity
                        }),
                        "aria-label": "Mangle rate: ".concat(percentage, " percent"),
                        "data-testid": "mangle-rate-value",
                        children: [
                            percentage,
                            "%"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: rateBadge({
                            severity
                        }),
                        "data-testid": "mangle-rate-badge",
                        "data-severity": severity,
                        children: getSeverityLabel(severity)
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: subtitle,
                children: "Mangle rate — proportion of correlated transport outcomes altered during processing"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: countsRow,
                "data-testid": "mangle-counts",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: countItem,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: countValue,
                                children: totalSequences.toString()
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: MangleRateIndicator_css_countLabel,
                                children: "Correlated outcomes"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: countItem,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: countValue,
                                children: totalMangled.toString()
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: MangleRateIndicator_css_countLabel,
                                children: "Mangled"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fanalytics%2FEscapeEventTable.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7VV0W6jMBB8z1f45aTLgysgTdM4H1MZvIRNjW3ZJpCc7t9POG0DgSOkUp-wtN4Z73g8PL0lzh0OaUT-LAgR6IzkJ0ZyCc1uQcKXCrSQedSKkUzLqlRtZc8NI0duf1P6Fm_hnNjzKlq2FX0Em0td04YRXnm9W_xdPH3QxIGmRuELRuIo-tU2pNoKsDTTUnLjILCEVTiBVp46PMOALQ1smZba3tbag1xJk0BqtMPLEM5j9n5qm702jETt6kxRCWgYicOJePa-t7pSgo7ib3r4qws-FwLV_nZrcl6Oy-Sh8ZRL3CtGJOR-eli-_KrXgPvCD3YU_5cjuRJ6y5XLtS0ZqYwBm_GLzBK8B0ud4VkYInqK1lB2bifV3uuSkdg0xGmJ4oYDAkddoIeAAowoXVtuuko9B6VmIuY9kZ9Z0RrrAnDnel57netHSe8TxHGf4YGzhVZCdCu0P7VCb7tQL9_y0hGsx4zLTz-VKISELu4m4Ab_5LxEeRpgV8s5DhyfKZRK3tCPl51EkWm6WcBIgUKA-vLhtQBSonHoZtjntR9SqCQqoJ9ZFWan6KF0jGSgPNhdV8vENEM9Ow63XGDlBnO_zNLl_sucaYzxWtLz23Y8rA-V85ifaKaVB-VHRbg5Wt_H_EHzxTPk23T23EuPObE7kfgzfhaVdW2v0XhRpjP7t_OFM4GOpxJEaO687HWXU-k27qWuQXS70ynNV6vBr-J6pxNRP63DlTybJI9_hPwfhycp03IIAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fanalytics_2FEscapeEventTable_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7VV0W6jMBB8z1f45aTLgysgTdM4H1MZvIRNjW3ZJpCc7t9POG0DgSOkUp_wtN4Z73g8PL0lzh0OaUT_LAgR6IzkJ0ZyCc1uQcKXCrSQedSKkUzLqlRtZc8NI0duf1P6Fm_hnNjzKlq2FX0Em0td04YRXnm9W_xdPH3QxIGmRuELRuIo_tU2pNoKsDTTUnLjILCEVTiBVp46PMOALQ1smZba3tbag1xJk0BqtMPLEM5j9n5qm702jETt6kxRCWgYicOJePa_t7pSgo7ib3r4qws_FwLV_nZrcl6Oy_Sh8ZRL3CtGJOR_eli_KrXgPvCD3YU_5cjuRJ6y5XLtS0ZqYwBm_GLzBK8B0ud4VkYInqK1lB2bifV3uuSkdg0xGmJ4oYDAkddoIeAAowoXVtuuko9B6VmIuY9kZ9Z0RrrAnDnel57netHSe8TxHGf4YGzhVZCdCu0P7VCb7tQL9_y0hGsx4zLTz_VKISELu4m4Ab_5LxEeRpgV8s5DhyfKZRK3tCPl51EkWm6WcBIgUKA_vLhtQBSonHoZtjntR9SqCQqoJ9ZFWan6KF0jGSgPNhdV8vENEM9Ow63XGDlBnO_zNLl_sucaYzxWtLz23Y8rA_V85ifaKaVB_VHRbg5Wt_H_EHzxTPk23T23EuPObE7kfgzfhaVdW2v0XhRpjP7t_OFM4GOpxJEaO687HWXU_k27qWuQXS70ynNV6vBr_J6pxNRP63DlTybJI9_hPwfhycp03IIAAA_22_7D = __webpack_require__(73337);
;// ./src/components/analytics/EscapeEventTable.css.ts


var codeCell = '_2ssjjb7';
var EscapeEventTable_css_container = '_2ssjjb0';
var EscapeEventTable_css_emptyState = '_2ssjjbb';
var loadMoreButton = '_2ssjjba';
var loadMoreRow = '_2ssjjb9';
var loadingState = '_2ssjjbc';
var mangledBadge = '_2ssjjb8';
var table = '_2ssjjb1';
var td = '_2ssjjb6';
var th = '_2ssjjb3';
var thead = '_2ssjjb2';
var tr = '_2ssjjb4';
var trMangled = '_2ssjjb5';
;// ./src/components/analytics/EscapeEventTable.tsx
// +feature: escape-analytics


/**
 * Convert a Uint8Array to a hex string for safe display.
 * IMPORTANT: raw_bytes MUST NEVER be passed to a terminal renderer — display as hex only.
 */ function toHexString(bytes) {
    return Array.from(bytes).map((b)=>b.toString(16).padStart(2, "0")).join(" ");
}
function formatTimestamp(wallTime) {
    if (!wallTime) return "—";
    const ms = Number(wallTime.seconds) * 1000 + Math.floor(wallTime.nanos / 1000000);
    const d = new Date(ms);
    return d.toISOString().replace("T", " ").replace("Z", "");
}
function EscapeEventTable(param) {
    let { events, loading, onLoadMore, hasMore } = param;
    if (!loading && events.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: EscapeEventTable_css_container,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: EscapeEventTable_css_emptyState,
                children: "No escape events found."
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: EscapeEventTable_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                className: table,
                "data-testid": "escape-event-table",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                        className: thead,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Time"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Stage"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Sequence Type"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Subtype"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Signature"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Project"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Byte Length"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Mangled"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Mangle Type"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: th,
                                    scope: "col",
                                    children: "Raw Bytes (hex)"
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                        children: events.map((event)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                className: event.mangled ? trMangled : tr,
                                "data-testid": "escape-event-row",
                                "data-mangled": event.mangled ? "true" : "false",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: codeCell,
                                            children: formatTimestamp(event.wallTime)
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: codeCell,
                                            children: event.stage || "—"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: codeCell,
                                            children: event.sequenceType || "—"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: codeCell,
                                            children: event.sequenceSubtype || "—"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: codeCell,
                                            children: event.sequenceSignature || "—"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: codeCell,
                                            children: event.projectPath || "—"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: event.byteLength
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: event.mangled ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: mangledBadge,
                                            "aria-label": "Mangled",
                                            children: "Yes"
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: "No"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: codeCell,
                                            children: event.mangleType || "—"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: td,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: codeCell,
                                            title: "Raw bytes (hex)",
                                            "aria-label": "Raw bytes as hex",
                                            children: event.rawBytes.length > 0 ? toHexString(event.rawBytes) : "—"
                                        })
                                    })
                                ]
                            }, event.id))
                    })
                ]
            }),
            loading && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: loadingState,
                role: "status",
                "aria-live": "polite",
                children: "Loading…"
            }),
            !loading && hasMore && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: loadMoreRow,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: loadMoreButton,
                    onClick: onLoadMore,
                    "data-testid": "load-more-button",
                    children: "Load more"
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/ui/LiveRegion.css.ts
var LiveRegion_css = __webpack_require__(76863);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fanalytics%2FSessionEscapeBreakdownTable.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7WV227jIBCG7_MU3Ky0uaCynTZN6MNUBMb2KBgQ4By82ndfgZvGdk5tpb2yHWC-mX9-Jk_vucpXbaMz8mdGiERvFT8yUio4vM1IelKJDkRAoxkRRrWNjisVt4zsuPtN6Xu-hq5w3SKbxxWzA1cqs6cHRngbzNvs7-zpxMkTZ48y1IzkWfYrntgYJ8FRYZTi1kPCpLeUgtGBeuzgArdJOGGUcdO1mMmAWiSqNR77MnxAsT3G08FYRrL41lHUEg6M5CklLraVM62W9CrgdQxY9AAuJepqurfo5teVCnAIlCusNCMKynC_XD7_XN8DVnW42FHfFqQ4A4Pj2pfGNYy01oITvBdaQQjgqLdcpCKyp-wFmkF_NiYE0zCS2wPxRqGcMCAx9jUGSFGAEW32jtuRVM9jp6FWqIGeDJfUoBig8YwI0AHcDbcVxx53zUoRrGHcR0ZS4ZY70Enpz26l9jfcVag_PqLIMbUaHIaBpoNfplIOlqZCTk9NOi5a52Nwa_BU7ofgjkts_UWbl2PvPbPSiNbTHXrcKEjymjZEWRkpbvRK9Ve130ZNWXoIafckdB1vcwr5lXv2knY-svAdg6Z542uHeptaMYi9TLG_aMVynNdyUMejm70aH339LvYx4TghfCO53vMmOisc4xVdj0Ktbneqm59dTt218VFM8lr_aKbtwAUUXJ1c3qCUCkaB-b3Ai8XFaDyPgXvOufs3MaBv7tLz_0L_B-7Gkp1nBwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fanalytics_2FSessionEscapeBreakdownTable_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7WV227jIBCG7_MU3Ky0uaCynTZN6MNUBMb2KBgQ4By82ndfgZvGdk5tpb2yHWC_mX9_Jk_vucpXbaMz8mdGiERvFT8yUio4vM1IelKJDkRAoxkRRrWNjisVt4zsuPtN6Xu_hq5w3SKbxxWzA1cqs6cHRngbzNvs7_zpxMkTZ48y1IzkWfYrntgYJ8FRYZTi1kPCpLeUgtGBeuzgArdJOGGUcdO1mMmAWiSqNR77MnxAsT3G08FYRrL41lHUEg6M5CklLraVM62W9CrgdQxY9AAuJepqurfo5teVCnAIlCusNCMKynC_XD7_XN8DVnW42FHfFqQ4A4Pj2pfGNYy01oITvBdaQQjgqLdcpCKyp_wFmkF_NiYE0zCS2wPxRqGcMCAx9jUGSFGAEW32jtuRVM9jp6FWqIGeDJfUoBig8YwI0AHcDbcVxx53zUoRrGHcR0ZS4ZY70Enpz26l9jfcVag_PqLIMbUaHIaBpoNfplIOlqZCTk9NOi5a52Nwa_BU7ofgjkts_UWbl2PvPbPSiNbTHXrcKEjymjZEWRkpbvRK9Ve130ZNWXoIafckdB1vcwr5lXv2knY_svAdg6Z542uHeptaMYi9TLG_aMVynNdyUMejm70aH339LvYx4TghfCO53vMmOisc4xVdj0Ktbneqm59dTt218VFM8lr_aKbtwAUUXJ1c3qCUCkaB_b3Ai8XFaDyPgXvOufs3MaBv7tLz_0L_B_7Gkp1nBwAA_22_7D = __webpack_require__(52897);
;// ./src/components/analytics/SessionEscapeBreakdownTable.css.ts


var SessionEscapeBreakdownTable_css_container = '_1l18umn0';
var SessionEscapeBreakdownTable_css_emptyState = '_1l18umna';
var SessionEscapeBreakdownTable_css_loadingState = '_1l18umnb';
var outlierIcon = '_1l18umn8';
var outlierRow = '_1l18umn7';
var sortButton = '_1l18umn4';
var sortIcon = '_1l18umn5';
var SessionEscapeBreakdownTable_css_table = '_1l18umn1';
var SessionEscapeBreakdownTable_css_td = '_1l18umn9';
var SessionEscapeBreakdownTable_css_th = '_1l18umn3';
var SessionEscapeBreakdownTable_css_thead = '_1l18umn2';
var SessionEscapeBreakdownTable_css_tr = '_1l18umn6';
;// ./src/components/analytics/SessionEscapeBreakdownTable.tsx
// +feature: escape-analytics





const COLUMNS = [
    {
        key: "sessionId",
        label: "Session"
    },
    {
        key: "totalSequences",
        label: "Total Sequences"
    },
    {
        key: "totalMangled",
        label: "Total Mangled"
    },
    {
        key: "mangleRate",
        label: "Mangle Rate"
    }
];
// Outlier threshold per ux.md §3: a session is an outlier when its mangle
// rate exceeds 2x the fleet-wide rate, or a flat 5% floor when the
// fleet-wide rate is ~0 (2x of ~0 would otherwise flag nearly every row).
const OUTLIER_FLOOR = 0.05;
const FLEET_RATE_EPSILON = 0.001;
function isOutlier(rowMangleRate, fleetMangleRate) {
    const threshold = fleetMangleRate > FLEET_RATE_EPSILON ? fleetMangleRate * 2 : OUTLIER_FLOOR;
    return rowMangleRate > threshold;
}
function compareRows(a, b, column) {
    switch(column){
        case "sessionId":
            return a.sessionId.localeCompare(b.sessionId);
        case "totalSequences":
            return a.totalSequences === b.totalSequences ? 0 : a.totalSequences > b.totalSequences ? 1 : -1;
        case "totalMangled":
            return a.totalMangled === b.totalMangled ? 0 : a.totalMangled > b.totalMangled ? 1 : -1;
        case "mangleRate":
            return a.mangleRate - b.mangleRate;
    }
}
function SessionEscapeBreakdownTable(param) {
    let { rows, fleetMangleRate, loading = false } = param;
    // Default sort: mangleRate descending (ux.md §3 / plan.md Task 2.4.4).
    const [sortColumn, setSortColumn] = (0,react.useState)("mangleRate");
    const [sortDirection, setSortDirection] = (0,react.useState)("descending");
    const sortedRows = (0,react.useMemo)(()=>{
        const sorted = [
            ...rows
        ].sort((a, b)=>compareRows(a, b, sortColumn));
        if (sortDirection === "descending") sorted.reverse();
        return sorted;
    }, [
        rows,
        sortColumn,
        sortDirection
    ]);
    const handleSort = (column)=>{
        if (column === sortColumn) {
            setSortDirection((prev)=>prev === "ascending" ? "descending" : "ascending");
        } else {
            setSortColumn(column);
            setSortDirection("descending");
        }
    };
    if (!loading && rows.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: SessionEscapeBreakdownTable_css_container,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: SessionEscapeBreakdownTable_css_emptyState,
                children: "No per-session breakdown available."
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: SessionEscapeBreakdownTable_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                className: SessionEscapeBreakdownTable_css_table,
                "data-testid": "session-escape-breakdown-table",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                        className: SessionEscapeBreakdownTable_css_thead,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                            children: COLUMNS.map((column)=>{
                                const active = column.key === sortColumn;
                                const ariaSort = active ? sortDirection : "none";
                                return /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    className: SessionEscapeBreakdownTable_css_th,
                                    scope: "col",
                                    "aria-sort": ariaSort,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        type: "button",
                                        className: sortButton,
                                        onClick: ()=>handleSort(column.key),
                                        "data-testid": "sort-button-".concat(column.key),
                                        children: [
                                            column.label,
                                            active && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: sortIcon,
                                                "aria-hidden": "true",
                                                children: sortDirection === "ascending" ? "▲" : "▼"
                                            })
                                        ]
                                    })
                                }, column.key);
                            })
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                        children: sortedRows.map((row)=>{
                            const outlier = isOutlier(row.mangleRate, fleetMangleRate);
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                className: outlier ? outlierRow : SessionEscapeBreakdownTable_css_tr,
                                "data-testid": "session-escape-breakdown-row",
                                "data-outlier": outlier ? "true" : "false",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                        className: SessionEscapeBreakdownTable_css_td,
                                        children: [
                                            outlier && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: outlierIcon,
                                                        "aria-hidden": "true",
                                                        children: "⚠"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: LiveRegion_css/* srOnly */.s,
                                                        children: "Outlier: "
                                                    })
                                                ]
                                            }),
                                            row.sessionId
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: SessionEscapeBreakdownTable_css_td,
                                        children: row.totalSequences.toString()
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: SessionEscapeBreakdownTable_css_td,
                                        children: row.totalMangled.toString()
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        className: SessionEscapeBreakdownTable_css_td,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(MangleRateIndicator, {
                                            mangleRate: row.mangleRate,
                                            totalSequences: row.correlationOutcomes,
                                            totalMangled: row.totalMangled
                                        })
                                    })
                                ]
                            }, row.sessionId);
                        })
                    })
                ]
            }),
            loading && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: SessionEscapeBreakdownTable_css_loadingState,
                role: "status",
                "aria-live": "polite",
                children: "Loading…"
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fanalytics%2FEscapeAnalyticsPage.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91XbW-bMBD-nl9hVZrUSHOEadckVJP2P6apcvABt4KNbJOyTP3vE07SYhJe2mZf-iFE4OOeu-eeO5vFA5Oi5mUQkL8zQgSaMud_IpLkUN_PiPunAjXEFpWMSKzyqpDNSsrLiGy5vqb0ga1hF-rdTThvVkouBMq0Z7XgNX1CYbOIsDAIynr_UKcoIxIQXlnVPDmaBMGX-9nzbHGMk10gznA393yGzmeipKUGd3ASOLjA3foTYJrZEwt0FrHKle6uBXMvQQ_4ZgR40-82HHB7e56jPiYI2SgtQNONslYVEWFlTYzKUXSswaft23kYnmMqKVooTERikBb0gCrC3bybNWvFFBGpJLh7Hj-mWlVSRMRqLk3JNUg7QtAYt0NFTffsV9o03kuFx1Q6dIUvdHXicrf4IkilSbAIzdfj-6-PfFqjRMWVoVs0uMnBkawqm6OENpYfbO5iPZhRlSQGrLPuuM7UFrRz2SvWtv1PrpFTAznEFsT3K6sruPrV_35xKic6YDjEfuaHcneJ-cR8n8s3Cficw31zuyCedLPeXD2M1aQWnyDDcyyyV3iTaZSP3UmwdujTWy9o109zgZU5iWfptWffrHic-017XgaMDyS3mdTBBcrjfhLeBh3Br_e95PfQy0RptWHXee4rhftKSTUKJwmNglooypxboHvtmYiwRDe_iSrcXELZweDOyyaUdTWprDCprEs_w3ikB-JJw-Ad-6v49NTC5K3-kEfPcBsaYclnHyLtXC80MNL3n-z2ivrArtQ93mZD9XtLtcbqwAaSmnYcGxME6xxQsP8oMg1wuK0t1JY64l8pb6H__l_oNwPoPwoQyImJNYAkXApy3fqkWt6tynru4lr4n3ZjufbvVYQ8j-Kumy-5Di4_4Pbvkgff_wBWtPawhQ4AAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fanalytics_2FEscapeAnalyticsPage_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91XbW_bMBD_nl9hVZrUSHOEadckVJP2P6apcvABt4KNbJOyTP3vE07SYhJe2mZf_iFE4OOeu_eeO5vFA5Oi5mUQkL8zQgSaMud_IpLkUN_PiPunAjXEFpWMSKzyqpDNSsrLiGy5vqb0ga1hF_rdTThvVkouBMq0Z7XgNX1CYbOIsDAIynr_UKcoIxIQXlnVPDmaBMGX_9nzbHGMk10gznA393yGzmeipKUGd3ASOLjA3foTYJrZEwt0FrHKle6uBXMvQQ_4ZgR40_82HHB7e56jPiYI2SgtQNONslYVEWFlTYzKUXSswaft23kYnmMqKVooTERikBb0gCrC3bybNWvFFBGpJLh7Hj_mWlVSRMRqLk3JNUg7QtAYt0NFTffsV9o03kuFx1Q6dIUvdHXicrf4IkilSbAIzdfj_6_PfFqjRMWVoVs0uMnBkawqm6OENpYfbO5iPZhRlSQGrLPuuM7UFrRz2SvWtv1PrpFTAznEFsT3K6sruPrV_35xKic6YDjEfuaHcneJ_cR8n8s3Cficw31zuyCedLPeXD2M1aQWnyDDcyyyV3iTaZSP3UmwdujTWy9o109zgZU5iWfptWffrHic_017XgaMDyS3mdTBBcrjfhLeBh3Br_e95PfQy0RptWHXee4rhftKSTUKJwmNglooypxboHvtmYiwRDe_iSrcXELZweDOyyaUdTWprDCprEs_w3ikB_JJw_Ad_6v49NTC5K3_kEfPcBsaYclnHyLtXC80MNL3n_z2ivrArtQ93mZD9XtLtcbqwAaSmnYcGxME6xxQsP8oMg1wuK0t1JY64l8pb6H_l_oNwPoPwoQyImJNYAkXApy3fqkWt6tynru4lr4n3ZjufbvVYQ8j_Kumy_5Di4_4Pbvkgff_wBWtPawhQ4AAA_22_7D = __webpack_require__(34308);
;// ./src/components/analytics/EscapeAnalyticsPage.css.ts


var card = '_1ndxap0b';
var cardTitle = '_1ndxap0c';
var errorBanner = '_1ndxap0h';
var filterInput = '_1ndxap0f';
var filterLabel = '_1ndxap0g';
var filterRow = '_1ndxap0e';
var fullWidthCard = '_1ndxap0d';
var grid = '_1ndxap0a';
var header = '_1ndxap01';
var loadingText = '_1ndxap0i';
var noSessionMessage = '_1ndxap0j';
var page = '_1ndxap00';
var selectorLabel = '_1ndxap08';
var sessionSelect = '_1ndxap09';
var sessionSelectorRow = '_1ndxap07';
var EscapeAnalyticsPage_css_subtitle = '_1ndxap03';
var EscapeAnalyticsPage_css_tab = '_1ndxap05';
var tablist = '_1ndxap04';
var tabpanel = '_1ndxap06';
var title = '_1ndxap02';
;// ./src/components/analytics/EscapeAnalyticsPage.tsx
/* __next_internal_client_entry_do_not_use__ EscapeAnalyticsPage auto */ 
// +feature: escape-analytics









const TABS = [
    {
        id: "per_session",
        label: "Per-Session"
    },
    {
        id: "all_sessions",
        label: "All Sessions"
    }
];
// Dominant-contributor threshold (ux.md §3): informational only, shown when a
// single session accounts for more than half of all mangled sequences fleet-wide.
const DOMINANT_CONTRIBUTOR_SHARE_THRESHOLD = 0.5;
function EscapeAnalyticsPage() {
    const sessions = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectAllSessions */.rt);
    const [selectedSessionId, setSelectedSessionId] = (0,react.useState)("");
    const [stageFilter, setStageFilter] = (0,react.useState)("");
    const [sequenceTypeFilter, setSequenceTypeFilter] = (0,react.useState)("");
    const [mangledOnly, setMangledOnly] = (0,react.useState)(false);
    const [viewMode, setViewMode] = (0,react.useState)("per_session");
    const tabRefs = (0,react.useRef)({
        per_session: null,
        all_sessions: null
    });
    const perSessionActive = viewMode === "per_session";
    const allSessionsActive = viewMode === "all_sessions";
    const { histogram, totalSequences, totalMangled, mangleRate, correlationOutcomes, correlationCoverage, captureHealthy, loading: summaryLoading, error: summaryError } = useEscapeAnalyticsSummary(selectedSessionId, perSessionActive);
    const { events, nextPageToken, loading: eventsLoading, error: eventsError, fetchNextPage } = useEscapeEvents(perSessionActive ? selectedSessionId : "", {
        stage: stageFilter,
        sequenceType: sequenceTypeFilter,
        mangledOnly,
        pageSize: 50
    });
    const { histogram: globalHistogram, totalSequences: globalTotalSequences, totalMangled: globalTotalMangled, mangleRate: globalMangleRate, correlationOutcomes: globalCorrelationOutcomes, correlationCoverage: globalCorrelationCoverage, captureHealthy: globalCaptureHealthy, droppedEvents, perSession, perProject, loading: globalLoading, error: globalError } = useEscapeAnalyticsGlobalSummary(allSessionsActive);
    const dominantContributor = perSession.reduce((max, row)=>!max || row.totalMangled > max.totalMangled ? row : max, null);
    const dominantContributorShare = dominantContributor && globalTotalMangled > 0n ? Number(dominantContributor.totalMangled) / Number(globalTotalMangled) : 0;
    const showDominantContributor = !!dominantContributor && dominantContributorShare > DOMINANT_CONTRIBUTOR_SHARE_THRESHOLD;
    const handleTabKeyDown = (event)=>{
        var _tabRefs_current_nextTab_id;
        if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
        event.preventDefault();
        const currentIndex = TABS.findIndex((tab)=>tab.id === viewMode);
        const delta = event.key === "ArrowRight" ? 1 : -1;
        const nextIndex = (currentIndex + delta + TABS.length) % TABS.length;
        const nextTab = TABS[nextIndex];
        setViewMode(nextTab.id);
        (_tabRefs_current_nextTab_id = tabRefs.current[nextTab.id]) === null || _tabRefs_current_nextTab_id === void 0 ? void 0 : _tabRefs_current_nextTab_id.focus();
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: page,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                        className: title,
                        children: "Escape Analytics"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: EscapeAnalyticsPage_css_subtitle,
                        children: "Inspect terminal escape sequence statistics and mangle events per session, or across the whole fleet."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: tablist,
                role: "tablist",
                "aria-label": "Escape analytics view",
                children: TABS.map((tab)=>{
                    const selected = tab.id === viewMode;
                    return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        ref: (el)=>{
                            tabRefs.current[tab.id] = el;
                        },
                        id: "tab-".concat(tab.id),
                        role: "tab",
                        type: "button",
                        className: EscapeAnalyticsPage_css_tab,
                        "aria-selected": selected,
                        "aria-controls": "tabpanel-".concat(tab.id),
                        tabIndex: selected ? 0 : -1,
                        onClick: ()=>setViewMode(tab.id),
                        onKeyDown: handleTabKeyDown,
                        "data-testid": "tab-".concat(tab.id),
                        children: tab.label
                    }, tab.id);
                })
            }),
            perSessionActive && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                id: "tabpanel-per_session",
                role: "tabpanel",
                "aria-labelledby": "tab-per_session",
                className: tabpanel,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: sessionSelectorRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: selectorLabel,
                                htmlFor: "session-selector",
                                children: "Session:"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                id: "session-selector",
                                className: sessionSelect,
                                value: selectedSessionId,
                                onChange: (e)=>setSelectedSessionId(e.target.value),
                                "aria-label": "Select session for escape analytics",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: "",
                                        children: "— select a session —"
                                    }),
                                    sessions.map((session)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                            value: session.id,
                                            children: session.title || session.id
                                        }, session.id))
                                ]
                            })
                        ]
                    }),
                    !selectedSessionId && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: noSessionMessage,
                        children: "Select a session above to view escape analytics."
                    }),
                    selectedSessionId && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            summaryError && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: errorBanner,
                                role: "alert",
                                children: [
                                    "Failed to load summary: ",
                                    summaryError.message
                                ]
                            }),
                            !captureHealthy && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: errorBanner,
                                role: "alert",
                                children: [
                                    "Capture is incomplete (",
                                    (correlationCoverage * 100).toFixed(1),
                                    "% of source sequences matched). Mangle rates may be unreliable."
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: grid,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: card,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                                className: cardTitle,
                                                children: "Mangle Rate"
                                            }),
                                            summaryLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                                className: loadingText,
                                                children: "Loading…"
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(MangleRateIndicator, {
                                                mangleRate: mangleRate,
                                                totalSequences: correlationOutcomes,
                                                totalMangled: totalMangled
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: card,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                                className: cardTitle,
                                                children: "Sequence Histogram"
                                            }),
                                            summaryLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                                className: loadingText,
                                                children: "Loading…"
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(SequenceHistogram, {
                                                histogram: histogram
                                            })
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: fullWidthCard,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                        className: cardTitle,
                                        children: "Escape Events"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: filterRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                className: filterInput,
                                                type: "text",
                                                placeholder: "Filter by stage…",
                                                value: stageFilter,
                                                onChange: (e)=>setStageFilter(e.target.value),
                                                "aria-label": "Filter by stage"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                className: filterInput,
                                                type: "text",
                                                placeholder: "Filter by sequence type…",
                                                value: sequenceTypeFilter,
                                                onChange: (e)=>setSequenceTypeFilter(e.target.value),
                                                "aria-label": "Filter by sequence type"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                className: filterLabel,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                        type: "checkbox",
                                                        checked: mangledOnly,
                                                        onChange: (e)=>setMangledOnly(e.target.checked),
                                                        "aria-label": "Show mangled events only"
                                                    }),
                                                    "Mangled only"
                                                ]
                                            })
                                        ]
                                    }),
                                    eventsError && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: errorBanner,
                                        role: "alert",
                                        children: [
                                            "Failed to load events: ",
                                            eventsError.message
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(EscapeEventTable, {
                                        events: events,
                                        loading: eventsLoading,
                                        onLoadMore: fetchNextPage,
                                        hasMore: !!nextPageToken
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            allSessionsActive && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                id: "tabpanel-all_sessions",
                role: "tabpanel",
                "aria-labelledby": "tab-all_sessions",
                className: tabpanel,
                children: [
                    globalError && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: errorBanner,
                        role: "alert",
                        children: [
                            "Failed to load global summary: ",
                            globalError.message
                        ]
                    }),
                    !globalError && globalLoading && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: loadingText,
                        children: "Loading…"
                    }),
                    !globalError && !globalLoading && globalTotalSequences === 0n && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: noSessionMessage,
                        children: "No escape sequence events recorded across any session yet."
                    }),
                    !globalError && !globalLoading && globalTotalSequences > 0n && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            (!globalCaptureHealthy || droppedEvents > 0n) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: errorBanner,
                                role: "alert",
                                children: [
                                    "Capture health warning: ",
                                    (globalCorrelationCoverage * 100).toFixed(1),
                                    "% correlation coverage; ",
                                    droppedEvents.toString(),
                                    " events dropped. Rates use correlated outcomes only."
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: grid,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: card,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                                className: cardTitle,
                                                children: "Fleet-Wide Mangle Rate"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)(MangleRateIndicator, {
                                                mangleRate: globalMangleRate,
                                                totalSequences: globalCorrelationOutcomes,
                                                totalMangled: globalTotalMangled
                                            }),
                                            showDominantContributor && dominantContributor && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                                className: EscapeAnalyticsPage_css_subtitle,
                                                children: [
                                                    "Session ",
                                                    dominantContributor.sessionId,
                                                    " accounts for the majority of mangled sequences fleet-wide."
                                                ]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: card,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                                className: cardTitle,
                                                children: "Sequence Histogram"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)(SequenceHistogram, {
                                                histogram: globalHistogram
                                            })
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: fullWidthCard,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                        className: cardTitle,
                                        children: "Per-Project Breakdown"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                            children: "Project"
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                            children: "Source"
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                            children: "Outcomes"
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                            children: "Mangled"
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                            children: "Rate"
                                                        })
                                                    ]
                                                })
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                                children: perProject.map((row)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                                        children: [
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                                children: row.projectPath || "Unknown"
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                                children: row.totalSequences.toString()
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                                children: row.correlationOutcomes.toString()
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                                                children: row.totalMangled.toString()
                                                            }),
                                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                                                children: [
                                                                    (row.mangleRate * 100).toFixed(2),
                                                                    "%"
                                                                ]
                                                            })
                                                        ]
                                                    }, row.projectPath || "unknown"))
                                            })
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: fullWidthCard,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                        className: cardTitle,
                                        children: "Per-Session Breakdown"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionEscapeBreakdownTable, {
                                        rows: perSession,
                                        fleetMangleRate: globalMangleRate,
                                        loading: globalLoading
                                    })
                                ]
                            })
                        ]
                    })
                ]
            })
        ]
    });
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

/***/ 28934:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 9833));


/***/ }),

/***/ 34308:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 52897:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 53972:
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

/***/ 73337:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 76863:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   s: () => (/* binding */ srOnly)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FLiveRegion_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll81M2V5NjAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlci13aWR0aDogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(6112);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FLiveRegion_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll81M2V5NjAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlci13aWR0aDogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FLiveRegion_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll81M2V5NjAwIHsKICBwb3NpdGlvbjogYWJzb2x1dGU7CiAgd2lkdGg6IDFweDsKICBoZWlnaHQ6IDFweDsKICBwYWRkaW5nOiAwOwogIG1hcmdpbjogLTFweDsKICBvdmVyZmxvdzogaGlkZGVuOwogIGNsaXAtcGF0aDogaW5zZXQoNTAlKTsKICB3aGl0ZS1zcGFjZTogbm93cmFwOwogIGJvcmRlci13aWR0aDogMDsKfQ_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var srOnly = '_53ey600';

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

/***/ 96465:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,4202,9353,8182,7924,8165,3575,9691,3516,7358], () => (__webpack_exec__(28934)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-c3ffa3ababe792a2.js.map