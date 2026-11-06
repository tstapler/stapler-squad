"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[7455],{

/***/ 7455:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

__webpack_require__.r(__webpack_exports__);
/* harmony import */ var _opentelemetry_instrumentation__WEBPACK_IMPORTED_MODULE_9__ = __webpack_require__(34916);
/* harmony import */ var _opentelemetry_instrumentation_document_load__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(92178);
/* harmony import */ var _opentelemetry_instrumentation_fetch__WEBPACK_IMPORTED_MODULE_10__ = __webpack_require__(66757);
/* harmony import */ var _opentelemetry_exporter_trace_otlp_http__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(28247);
/* harmony import */ var _opentelemetry_resources__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(51468);
/* harmony import */ var _opentelemetry_semantic_conventions__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(39003);
/* harmony import */ var _opentelemetry_sdk_trace_web__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(75888);
/* harmony import */ var _opentelemetry_sdk_trace_web__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(66691);
/* harmony import */ var _opentelemetry_context_zone__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(45845);
/* harmony import */ var _active_session__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(73483);
/* harmony import */ var _span_buffer__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(17073);
/* provided dependency */ var process = __webpack_require__(36843);










var _process_env_NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT;
// Same-origin by default: the Go server proxies this to the real collector
// (server/handlers/otel_proxy_handler.go) so the browser never needs CORS
// access to the collector itself.
const tracesUrl = (_process_env_NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT = process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT) !== null && _process_env_NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT !== void 0 ? _process_env_NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT : "/api/otel/v1/traces";
// Writes `session.uuid` onto every span started while a session is focused.
// State lives in active-session.ts (no OTel imports) so components can read
// it without triggering this file's module-level SDK initialisation.
class SessionSpanProcessor {
    onStart(span) {
        const uuid = (0,_active_session__WEBPACK_IMPORTED_MODULE_2__/* .getActiveSessionUUID */ .E)();
        if (uuid !== null) {
            span.setAttribute("session.uuid", uuid);
        }
    }
    onEnd(span) {
        var _span_parentSpanContext;
        if (span.attributes["session.uuid"] === undefined) return;
        (0,_span_buffer__WEBPACK_IMPORTED_MODULE_3__/* .recordSpan */ .o)({
            name: span.name,
            traceId: span.spanContext().traceId,
            spanId: span.spanContext().spanId,
            parentSpanId: (_span_parentSpanContext = span.parentSpanContext) === null || _span_parentSpanContext === void 0 ? void 0 : _span_parentSpanContext.spanId,
            startTimeMs: span.startTime[0] * 1000 + span.startTime[1] / 1e6,
            durationMs: span.duration[0] * 1000 + span.duration[1] / 1e6,
            status: span.status.code,
            attributes: {
                ...span.attributes
            }
        });
    }
    shutdown() {
        return Promise.resolve();
    }
    forceFlush() {
        return Promise.resolve();
    }
}
const provider = new _opentelemetry_sdk_trace_web__WEBPACK_IMPORTED_MODULE_4__/* .WebTracerProvider */ .Z({
    resource: (0,_opentelemetry_resources__WEBPACK_IMPORTED_MODULE_5__/* .resourceFromAttributes */ .QZ)({
        [_opentelemetry_semantic_conventions__WEBPACK_IMPORTED_MODULE_6__/* .ATTR_SERVICE_NAME */ .UX4]: "stapler-squad-web"
    }),
    spanProcessors: [
        new _opentelemetry_sdk_trace_web__WEBPACK_IMPORTED_MODULE_7__/* .BatchSpanProcessor */ .J(new _opentelemetry_exporter_trace_otlp_http__WEBPACK_IMPORTED_MODULE_8__/* .OTLPTraceExporter */ .Q({
            url: tracesUrl
        })),
        new SessionSpanProcessor()
    ]
});
provider.register({
    contextManager: new _opentelemetry_context_zone__WEBPACK_IMPORTED_MODULE_1__/* .ZoneContextManager */ .T()
});
(0,_opentelemetry_instrumentation__WEBPACK_IMPORTED_MODULE_9__/* .registerInstrumentations */ .R)({
    instrumentations: [
        new _opentelemetry_instrumentation_document_load__WEBPACK_IMPORTED_MODULE_0__/* .DocumentLoadInstrumentation */ .g(),
        new _opentelemetry_instrumentation_fetch__WEBPACK_IMPORTED_MODULE_10__/* .FetchInstrumentation */ .X({
            // Don't trace the exporter's own export calls — every export would
            // otherwise emit a span that needs exporting, spamming the trace with
            // noise about itself.
            ignoreUrls: [
                new RegExp(tracesUrl.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))
            ]
        })
    ]
});


/***/ }),

/***/ 17073:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   o: () => (/* binding */ recordSpan),
/* harmony export */   y: () => (/* binding */ getSessionSpansJson)
/* harmony export */ });
// Bounded in-memory record of ended browser spans, read by the diagnostic
// bundle export. No OTel imports so components can use it without triggering
// otel-init.ts's module-level SDK setup.
const MAX_SPANS = 2000;
const spans = [];
function recordSpan(span) {
    spans.push(span);
    if (spans.length > MAX_SPANS) spans.shift();
}
/** Spans tagged with sessionUuid, JSON-encoded for the export request. */ function getSessionSpansJson(sessionUuid) {
    const matching = spans.filter((s)=>s.attributes["session.uuid"] === sessionUuid);
    return new TextEncoder().encode(JSON.stringify(matching));
}


/***/ })

}]);
//# sourceMappingURL=7455.587417b9da0ae01a.js.map