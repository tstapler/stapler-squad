(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[4358],{

/***/ 3728:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   PageViewTracker: () => (/* binding */ PageViewTracker)
/* harmony export */ });
/* harmony import */ var _lib_analytics_usePageView__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(90814);
/* __next_internal_client_entry_do_not_use__ PageViewTracker auto */ 
function PageViewTracker() {
    (0,_lib_analytics_usePageView__WEBPACK_IMPORTED_MODULE_0__/* .usePageView */ .O)();
    return null;
}


/***/ }),

/***/ 6579:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 46405:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 54618:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 54763:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  ImportSessionsContainer: () => (/* binding */ ImportSessionsContainer)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/file.js
var file = __webpack_require__(1844);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/service.js
var service = __webpack_require__(95765);
;// ./src/gen/session/v1/import_pb.ts
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/import.proto (package session.v1, syntax proto3)
/* eslint-disable */ 
/**
 * Describes the file session/v1/import.proto.
 */ const file_session_v1_import = /*@__PURE__*/ (0,file/* fileDesc */.w)("ChdzZXNzaW9uL3YxL2ltcG9ydC5wcm90bxIKc2Vzc2lvbi52MSIyCgtQSURJZGVudGl0eRILCgNwaWQYASABKAUSFgoOY3JlYXRlX3RpbWVfbXMYAiABKAMipwEKG0V4dGVybmFsU2Vzc2lvbkNhbmRpZGF0ZVJlZhIxCgtzb3VyY2Vfa2luZBgBIAEoDjIcLnNlc3Npb24udjEuSW1wb3J0U291cmNlS2luZBIMCgRwYXRoGAIgASgJEg8KB3Byb2dyYW0YAyABKAkSCwoDcGlkGAQgASgFEhQKDHRtdXhfc2Vzc2lvbhgFIAEoCRITCgtzb2NrZXRfcGF0aBgGIAEoCSJhChRIaXN0b3J5RmlsZUNhbmRpZGF0ZRIZChFjb252ZXJzYXRpb25fdXVpZBgBIAEoCRIZChFoaXN0b3J5X2ZpbGVfcGF0aBgCIAEoCRITCgtwcm9qZWN0X2RpchgDIAEoCSK+AQoWQ29ycmVsYXRpb25SZXN1bHRQcm90bxIpCgRraW5kGAEgASgOMhsuc2Vzc2lvbi52MS5Db3JyZWxhdGlvbktpbmQSDAoEdXVpZBgCIAEoCRI1Cgpjb25maWRlbmNlGAMgASgOMiEuc2Vzc2lvbi52MS5Db3JyZWxhdGlvbkNvbmZpZGVuY2USNAoKY2FuZGlkYXRlcxgEIAMoCzIgLnNlc3Npb24udjEuSGlzdG9yeUZpbGVDYW5kaWRhdGUiYQojUHJldmlld0ltcG9ydEV4dGVybmFsU2Vzc2lvblJlcXVlc3QSOgoJY2FuZGlkYXRlGAEgASgLMicuc2Vzc2lvbi52MS5FeHRlcm5hbFNlc3Npb25DYW5kaWRhdGVSZWYi3wEKJFByZXZpZXdJbXBvcnRFeHRlcm5hbFNlc3Npb25SZXNwb25zZRIPCgdwcm9ncmFtGAEgASgJEgwKBHBhdGgYAiABKAkSNwoLY29ycmVsYXRpb24YAyABKAsyIi5zZXNzaW9uLnYxLkNvcnJlbGF0aW9uUmVzdWx0UHJvdG8SEgoKdHVybl9jb3VudBgEIAEoBRIcChRsYXN0X21lc3NhZ2VfZXhjZXJwdBgFIAEoCRItCgxwaWRfaWRlbnRpdHkYBiABKAsyFy5zZXNzaW9uLnYxLlBJRElkZW50aXR5IvABCiJDb21taXRJbXBvcnRFeHRlcm5hbFNlc3Npb25SZXF1ZXN0EjoKCWNhbmRpZGF0ZRgBIAEoCzInLnNlc3Npb24udjEuRXh0ZXJuYWxTZXNzaW9uQ2FuZGlkYXRlUmVmEkAKFGV4cGVjdGVkX2NvcnJlbGF0aW9uGAIgASgLMiIuc2Vzc2lvbi52MS5Db3JyZWxhdGlvblJlc3VsdFByb3RvEh0KFWRpc2FtYmlndWF0aW9uX2Nob2ljZRgDIAEoCRItCgxwaWRfaWRlbnRpdHkYBCABKAsyFy5zZXNzaW9uLnYxLlBJRElkZW50aXR5IqIBCiNDb21taXRJbXBvcnRFeHRlcm5hbFNlc3Npb25SZXNwb25zZRIoCgZzdGF0dXMYASABKA4yGC5zZXNzaW9uLnYxLkltcG9ydFN0YXR1cxITCgtpbnN0YW5jZV9pZBgCIAEoCRINCgVlcnJvchgDIAEoCRItCgxwaWRfaWRlbnRpdHkYBCABKAsyFy5zZXNzaW9uLnYxLlBJRElkZW50aXR5ImcKIUNvbmZpcm1LaWxsRXh0ZXJuYWxTZXNzaW9uUmVxdWVzdBITCgtpbnN0YW5jZV9pZBgBIAEoCRItCgxwaWRfaWRlbnRpdHkYAiABKAsyFy5zZXNzaW9uLnYxLlBJRElkZW50aXR5IlsKIkNvbmZpcm1LaWxsRXh0ZXJuYWxTZXNzaW9uUmVzcG9uc2USJgoGc3RhdHVzGAEgASgOMhYuc2Vzc2lvbi52MS5LaWxsU3RhdHVzEg0KBWVycm9yGAIgASgJIl4KGENhbmNlbFBlbmRpbmdLaWxsUmVxdWVzdBITCgtpbnN0YW5jZV9pZBgBIAEoCRItCgxwaWRfaWRlbnRpdHkYAiABKAsyFy5zZXNzaW9uLnYxLlBJRElkZW50aXR5IjsKGUNhbmNlbFBlbmRpbmdLaWxsUmVzcG9uc2USDwoHcmVzdW1lZBgBIAEoCBINCgVlcnJvchgCIAEoCSqAAQoQSW1wb3J0U291cmNlS2luZBIiCh5JTVBPUlRfU09VUkNFX0tJTkRfVU5TUEVDSUZJRUQQABIlCiFJTVBPUlRfU09VUkNFX0tJTkRfTVVYX0RJU0NPVkVSRUQQARIhCh1JTVBPUlRfU09VUkNFX0tJTkRfUExBSU5fVE1VWBACKpIBCg9Db3JyZWxhdGlvbktpbmQSIAocQ09SUkVMQVRJT05fS0lORF9VTlNQRUNJRklFRBAAEh4KGkNPUlJFTEFUSU9OX0tJTkRfTk9UX0ZPVU5EEAESHQoZQ09SUkVMQVRJT05fS0lORF9SRVNPTFZFRBACEh4KGkNPUlJFTEFUSU9OX0tJTkRfQU1CSUdVT1VTEAMqsQEKFUNvcnJlbGF0aW9uQ29uZmlkZW5jZRImCiJDT1JSRUxBVElPTl9DT05GSURFTkNFX1VOU1BFQ0lGSUVEEAASHwobQ09SUkVMQVRJT05fQ09ORklERU5DRV9OT05FEAESJAogQ09SUkVMQVRJT05fQ09ORklERU5DRV9QSURfRVhBQ1QQAhIpCiVDT1JSRUxBVElPTl9DT05GSURFTkNFX1BBVEhfSEVVUklTVElDEAMqZAoMSW1wb3J0U3RhdHVzEh0KGUlNUE9SVF9TVEFUVVNfVU5TUEVDSUZJRUQQABIbChdJTVBPUlRfU1RBVFVTX0NPTU1JVFRFRBABEhgKFElNUE9SVF9TVEFUVVNfRkFJTEVEEAIqdwoKS2lsbFN0YXR1cxIbChdLSUxMX1NUQVRVU19VTlNQRUNJRklFRBAAEhYKEktJTExfU1RBVFVTX0tJTExFRBABEhwKGEtJTExfU1RBVFVTX0FMUkVBRFlfR09ORRACEhYKEktJTExfU1RBVFVTX0ZBSUxFRBADMvsDCg1JbXBvcnRTZXJ2aWNlEoMBChxQcmV2aWV3SW1wb3J0RXh0ZXJuYWxTZXNzaW9uEi8uc2Vzc2lvbi52MS5QcmV2aWV3SW1wb3J0RXh0ZXJuYWxTZXNzaW9uUmVxdWVzdBowLnNlc3Npb24udjEuUHJldmlld0ltcG9ydEV4dGVybmFsU2Vzc2lvblJlc3BvbnNlIgASgAEKG0NvbW1pdEltcG9ydEV4dGVybmFsU2Vzc2lvbhIuLnNlc3Npb24udjEuQ29tbWl0SW1wb3J0RXh0ZXJuYWxTZXNzaW9uUmVxdWVzdBovLnNlc3Npb24udjEuQ29tbWl0SW1wb3J0RXh0ZXJuYWxTZXNzaW9uUmVzcG9uc2UiABJ9ChpDb25maXJtS2lsbEV4dGVybmFsU2Vzc2lvbhItLnNlc3Npb24udjEuQ29uZmlybUtpbGxFeHRlcm5hbFNlc3Npb25SZXF1ZXN0Gi4uc2Vzc2lvbi52MS5Db25maXJtS2lsbEV4dGVybmFsU2Vzc2lvblJlc3BvbnNlIgASYgoRQ2FuY2VsUGVuZGluZ0tpbGwSJC5zZXNzaW9uLnYxLkNhbmNlbFBlbmRpbmdLaWxsUmVxdWVzdBolLnNlc3Npb24udjEuQ2FuY2VsUGVuZGluZ0tpbGxSZXNwb25zZSIAQqsBCg5jb20uc2Vzc2lvbi52MUILSW1wb3J0UHJvdG9QAVpDZ2l0aHViLmNvbS90c3RhcGxlci9zdGFwbGVyLXNxdWFkL2dlbi9wcm90by9nby9zZXNzaW9uL3YxO3Nlc3Npb252MaICA1NYWKoCClNlc3Npb24uVjHKAgpTZXNzaW9uXFYx4gIWU2Vzc2lvblxWMVxHUEJNZXRhZGF0YeoCC1Nlc3Npb246OlYxYgZwcm90bzM");
/**
 * Describes the message session.v1.PIDIdentity.
 * Use `create(PIDIdentitySchema)` to create a new message.
 */ const PIDIdentitySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 0)));
/**
 * Describes the message session.v1.ExternalSessionCandidateRef.
 * Use `create(ExternalSessionCandidateRefSchema)` to create a new message.
 */ const ExternalSessionCandidateRefSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 1)));
/**
 * Describes the message session.v1.HistoryFileCandidate.
 * Use `create(HistoryFileCandidateSchema)` to create a new message.
 */ const HistoryFileCandidateSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 2)));
/**
 * Describes the message session.v1.CorrelationResultProto.
 * Use `create(CorrelationResultProtoSchema)` to create a new message.
 */ const CorrelationResultProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 3)));
/**
 * Describes the message session.v1.PreviewImportExternalSessionRequest.
 * Use `create(PreviewImportExternalSessionRequestSchema)` to create a new message.
 */ const PreviewImportExternalSessionRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 4)));
/**
 * Describes the message session.v1.PreviewImportExternalSessionResponse.
 * Use `create(PreviewImportExternalSessionResponseSchema)` to create a new message.
 */ const PreviewImportExternalSessionResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 5)));
/**
 * Describes the message session.v1.CommitImportExternalSessionRequest.
 * Use `create(CommitImportExternalSessionRequestSchema)` to create a new message.
 */ const CommitImportExternalSessionRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 6)));
/**
 * Describes the message session.v1.CommitImportExternalSessionResponse.
 * Use `create(CommitImportExternalSessionResponseSchema)` to create a new message.
 */ const CommitImportExternalSessionResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 7)));
/**
 * Describes the message session.v1.ConfirmKillExternalSessionRequest.
 * Use `create(ConfirmKillExternalSessionRequestSchema)` to create a new message.
 */ const ConfirmKillExternalSessionRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 8)));
/**
 * Describes the message session.v1.ConfirmKillExternalSessionResponse.
 * Use `create(ConfirmKillExternalSessionResponseSchema)` to create a new message.
 */ const ConfirmKillExternalSessionResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 9)));
/**
 * Describes the message session.v1.CancelPendingKillRequest.
 * Use `create(CancelPendingKillRequestSchema)` to create a new message.
 */ const CancelPendingKillRequestSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 10)));
/**
 * Describes the message session.v1.CancelPendingKillResponse.
 * Use `create(CancelPendingKillResponseSchema)` to create a new message.
 */ const CancelPendingKillResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_import, 11)));
/**
 * ImportSourceKind mirrors session.ImportSourceKind.
 *
 * @generated from enum session.v1.ImportSourceKind
 */ var ImportSourceKind = /*#__PURE__*/ function(ImportSourceKind) {
    /**
   * @generated from enum value: IMPORT_SOURCE_KIND_UNSPECIFIED = 0;
   */ ImportSourceKind[ImportSourceKind["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: IMPORT_SOURCE_KIND_MUX_DISCOVERED = 1;
   */ ImportSourceKind[ImportSourceKind["MUX_DISCOVERED"] = 1] = "MUX_DISCOVERED";
    /**
   * @generated from enum value: IMPORT_SOURCE_KIND_PLAIN_TMUX = 2;
   */ ImportSourceKind[ImportSourceKind["PLAIN_TMUX"] = 2] = "PLAIN_TMUX";
    return ImportSourceKind;
}({});
/**
 * Describes the enum session.v1.ImportSourceKind.
 */ const ImportSourceKindSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_import, 0)));
/**
 * CorrelationKind mirrors session.CorrelationKind. Ambiguous and NotFound
 * are valid, non-error outcomes and must never be silently collapsed.
 *
 * @generated from enum session.v1.CorrelationKind
 */ var CorrelationKind = /*#__PURE__*/ function(CorrelationKind) {
    /**
   * @generated from enum value: CORRELATION_KIND_UNSPECIFIED = 0;
   */ CorrelationKind[CorrelationKind["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: CORRELATION_KIND_NOT_FOUND = 1;
   */ CorrelationKind[CorrelationKind["NOT_FOUND"] = 1] = "NOT_FOUND";
    /**
   * @generated from enum value: CORRELATION_KIND_RESOLVED = 2;
   */ CorrelationKind[CorrelationKind["RESOLVED"] = 2] = "RESOLVED";
    /**
   * @generated from enum value: CORRELATION_KIND_AMBIGUOUS = 3;
   */ CorrelationKind[CorrelationKind["AMBIGUOUS"] = 3] = "AMBIGUOUS";
    return CorrelationKind;
}({});
/**
 * Describes the enum session.v1.CorrelationKind.
 */ const CorrelationKindSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_import, 1)));
/**
 * CorrelationConfidence mirrors session.CorrelationConfidence.
 *
 * @generated from enum session.v1.CorrelationConfidence
 */ var CorrelationConfidence = /*#__PURE__*/ function(CorrelationConfidence) {
    /**
   * @generated from enum value: CORRELATION_CONFIDENCE_UNSPECIFIED = 0;
   */ CorrelationConfidence[CorrelationConfidence["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: CORRELATION_CONFIDENCE_NONE = 1;
   */ CorrelationConfidence[CorrelationConfidence["NONE"] = 1] = "NONE";
    /**
   * @generated from enum value: CORRELATION_CONFIDENCE_PID_EXACT = 2;
   */ CorrelationConfidence[CorrelationConfidence["PID_EXACT"] = 2] = "PID_EXACT";
    /**
   * @generated from enum value: CORRELATION_CONFIDENCE_PATH_HEURISTIC = 3;
   */ CorrelationConfidence[CorrelationConfidence["PATH_HEURISTIC"] = 3] = "PATH_HEURISTIC";
    return CorrelationConfidence;
}({});
/**
 * Describes the enum session.v1.CorrelationConfidence.
 */ const CorrelationConfidenceSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_import, 2)));
/**
 * @generated from enum session.v1.ImportStatus
 */ var ImportStatus = /*#__PURE__*/ function(ImportStatus) {
    /**
   * @generated from enum value: IMPORT_STATUS_UNSPECIFIED = 0;
   */ ImportStatus[ImportStatus["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: IMPORT_STATUS_COMMITTED = 1;
   */ ImportStatus[ImportStatus["COMMITTED"] = 1] = "COMMITTED";
    /**
   * @generated from enum value: IMPORT_STATUS_FAILED = 2;
   */ ImportStatus[ImportStatus["FAILED"] = 2] = "FAILED";
    return ImportStatus;
}({});
/**
 * Describes the enum session.v1.ImportStatus.
 */ const ImportStatusSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_import, 3)));
/**
 * @generated from enum session.v1.KillStatus
 */ var KillStatus = /*#__PURE__*/ function(KillStatus) {
    /**
   * @generated from enum value: KILL_STATUS_UNSPECIFIED = 0;
   */ KillStatus[KillStatus["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: KILL_STATUS_KILLED = 1;
   */ KillStatus[KillStatus["KILLED"] = 1] = "KILLED";
    /**
   * already_gone means IsAlive re-verification failed (PID reused or
   * process already exited) -- no signal was sent.
   *
   * @generated from enum value: KILL_STATUS_ALREADY_GONE = 2;
   */ KillStatus[KillStatus["ALREADY_GONE"] = 2] = "ALREADY_GONE";
    /**
   * failed means the kill primitive (tmux kill-session) itself failed; the
   * original process is left SIGSTOP'd, never auto-resumed.
   *
   * @generated from enum value: KILL_STATUS_FAILED = 3;
   */ KillStatus[KillStatus["FAILED"] = 3] = "FAILED";
    return KillStatus;
}({});
/**
 * Describes the enum session.v1.KillStatus.
 */ const KillStatusSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_import, 4)));
/**
 * ImportService manages import of externally-created (unmanaged) agent
 * sessions -- e.g. ssq-mux-wrapped Claude processes discovered outside
 * Stapler Squad's own session tracking -- into managed Stapler Squad
 * sessions.
 *
 * Two-phase command pattern (ADR-001): Preview (side-effect-free) -> Commit
 * (persists a managed Instance, starts a resumed session, and SIGSTOPs the
 * original process to prevent two writers touching the same Claude JSONL
 * transcript) -> a separate, explicitly user-issued ConfirmKill or
 * CancelPendingKill to resolve the SIGSTOP'd original process left behind
 * by Commit. Kill confirmation is never bundled into Commit's response --
 * it is always a distinct, separately-issued action.
 *
 * All three RPCs are gated behind the STAPLER_SQUAD_ENABLE_SESSION_IMPORT
 * feature flag (Story 1.3.2) and return CodeUnimplemented when disabled.
 *
 * @generated from service session.v1.ImportService
 */ const ImportService = /*@__PURE__*/ (0,service/* serviceDesc */.Z)(file_session_v1_import, 0);

// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/telemetry/rpcTiming.ts
var rpcTiming = __webpack_require__(77665);
// EXTERNAL MODULE: ./src/lib/contexts/AnalyticsContext.tsx
var AnalyticsContext = __webpack_require__(24044);
;// ./src/lib/hooks/useImportSessionService.ts
/* __next_internal_client_entry_do_not_use__ useImportSessionService auto */ 






// ImportService is unary-only (no Watch* streaming RPCs), so a plain HTTP
// ConnectRPC transport is sufficient -- unlike useSessionService, this hook
// does not need createWatchTransport's WebSocket upgrade path.
function useImportSessionService() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { baseUrl = (0,config/* getApiBaseUrl */.h)() } = options;
    const analytics = (0,AnalyticsContext/* useAnalytics */.st)();
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl,
            interceptors: [
                (0,config/* createAuthInterceptor */._)(),
                (0,rpcTiming/* createRpcTimingInterceptor */.r)(analytics)
            ]
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(ImportService, transport);
    // analytics identity is stable (memoized in AnalyticsContextProvider); baseUrl is the only real dep
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        baseUrl
    ]);
    const previewImport = (0,react.useCallback)(async (candidate)=>{
        if (!clientRef.current) return null;
        return await clientRef.current.previewImportExternalSession({
            candidate
        });
    }, []);
    const commitImport = (0,react.useCallback)(async (param)=>{
        let { candidate, expectedCorrelation, disambiguationChoice, pidIdentity } = param;
        if (!clientRef.current) return null;
        return await clientRef.current.commitImportExternalSession({
            candidate,
            expectedCorrelation,
            disambiguationChoice: disambiguationChoice !== null && disambiguationChoice !== void 0 ? disambiguationChoice : "",
            pidIdentity
        });
    }, []);
    const confirmKill = (0,react.useCallback)(async (param)=>{
        let { instanceId, pidIdentity } = param;
        if (!clientRef.current) return null;
        return await clientRef.current.confirmKillExternalSession({
            instanceId,
            pidIdentity
        });
    }, []);
    const cancelPendingKill = (0,react.useCallback)(async (param)=>{
        let { instanceId, pidIdentity } = param;
        if (!clientRef.current) return null;
        return await clientRef.current.cancelPendingKill({
            instanceId,
            pidIdentity
        });
    }, []);
    return {
        previewImport,
        commitImport,
        confirmKill,
        cancelPendingKill
    };
}

// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FImportExternalSessionsPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81V247aMBB936-Yl0rdB6M4QAD3Y1YmNompY6e2WQJV_72Kk904FyBlXyokQI5zZs6ZMzOLt6U87tkmgt8vAHua_syMPilG4J2a7wi94R2_xua6ef1RP9eGcUMAlxVYLQUb3OLBLWQoEydLAMdlVR-XlDGhMgJx1BwwYUtJLwQOkvuD-hcxYXjqhFYEUi1PhaqfZLQkgJP6vT8vizZn7HMeoRxP1onDBaVaOa4cAVvSlKM9d2fOPRqVIlNIOF5YAilXjpsuSJutz-Vs6qP6O4wb-7gFNZlQBCJ_WyuHrLjyjp0_OnOR5Y7AJvLXUi21GUobvYbgyx74qqwgGoXAyybEJBzuwa0e1HX7fF23g7ImZQU4JN8mm7TJnoytsy21-BD8phwAzlBlRWMDKiVEi9gCp5b3yJFcv3PzgCLe9hRZ--ufWS_bejteOeSNEVripsJ9iqsGQwrFUd6WHC-SMGziw9bpHqQ-o4oAPTn9NfE79I1HPwvmcgI4ir4Fr6RaSlpa7vvJ_5u0Uwe27Su0q-s6oZLkBzcyenLH6I1utwu1D4nutXO6eKTKOReOI9_gBJQeduruNpN_CnPHqO_cOJFS-aFKIRiTPZdSIql1KM2FZOBY49Z-dKVV75V9WM7lcH4mfiiE7TZl3g4tvYsW-c89pDkkG1qhqfDEGGzd0W2CsvoYIwN_73a79vyBXWbMQN7ldqCFkBcChVbau2bYCvHsyXqY3j53d0s00H4bGLIZ-Z-W2M6j_5XJ_dxGyTo15zR9f7vlI5vEEzZZ38H79WCIFD1ZmsYaCbAe1GEV1GFiT4XLqIsM0QKvx0spn7WUVE8W8WVZxrPVJ11Sw5V72iiTOkU3dAroBBLMMMXxPzbFY7LH-fX-CyFhjnltCwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FImportExternalSessionsPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81V247aMBB936_Yl0rdB6M4QAD3Y1YmNompY6e2WQJV_72Kk904FyBlXyokQI5zZs6ZMzOLt6U87tkmgt8vAHua_syMPilG4J2a7wi94R2_xua6ef1RP9eGcUMAlxVYLQUb3OLBLWQoEydLAMdlVR_XlDGhMgJx1BwwYUtJLwQOkvuD_hcxYXjqhFYEUi1PhaqfZLQkgJP6vT8vizZn7HMeoRxP1onDBaVaOa4cAVvSlKM9d2fOPRqVIlNIOF5YAilXjpsuSJutz_Vs6qP6O4wb_7gFNZlQBCJ_WyuHrLjyjp0_OnOR5Y7AJvLXUi21GUobvYbgyx74qqwgGoXAyybEJBzuwa0e1HX7fF23g7ImZQU4JN8mm7TJnoytsy21_BD8phwAzlBlRWMDKiVEi9gCp5b3yJFcv3PzgCLe9hRZ_ufWS_bejteOeSNEVripsJ9iqsGQwrFUd6WHC_SMGziw9bpHqQ_o4oAPTn9NfE79I1HPwvmcgI4ir4Fr6RaSlpa7vvJ_5u0Uwe27Su0q_s6oZLkBzcyenLH6I1utwu1D4nutXO6eKTKOReOI9_gBJQeduruNpN_CnPHqO_cOJFS_aFKIRiTPZdSIql1KM2FZOBY49Z_dKVV75V9WM7lcH4mfiiE7TZl3g4tvYsW_c89pDkkG1qhqfDEGGzd0W2CsvoYIwN_73a79vyBXWbMQN7ldqCFkBcChVbau2bYCvHsyXqY3j53d0s00H4bGLIZ_Z_W2M6j_5XJ_dxGyTo15zR9f7vlI5vEEzZZ38H79WCIFD1ZmsYaCbAe1GEV1GFiT4XLqIsM0QKvx0spn7WUVE8W8WVZxrPVJ11Sw5V72iiTOkU3dAroBBLMMMXxPzbFY7LH_fX_CyFhjnltCwAA_22_7D = __webpack_require__(64770);
;// ./src/components/sessions/ImportExternalSessionsPanel.css.ts


var bulkActionBar = '_3ljbd7f';
var bulkActionCount = '_3ljbd7g';
var bulkClearBtn = '_3ljbd7i';
var bulkImportBtn = '_3ljbd7h';
var checkboxTd = '_3ljbd7c';
var checkboxTh = '_3ljbd7b';
var empty = '_3ljbd75';
var panel = '_3ljbd70';
var pathText = '_3ljbd7e';
var refreshButton = '_3ljbd74';
var ImportExternalSessionsPanel_css_row = '_3ljbd7a';
var rowImportBtn = '_3ljbd7j';
var sourceBadge = '_3ljbd7d';
var subtitle = '_3ljbd73';
var table = '_3ljbd77';
var tableWrapper = '_3ljbd76';
var td = '_3ljbd79';
var th = '_3ljbd78';
var title = '_3ljbd72';
var titleRow = '_3ljbd71';
;// ./src/components/sessions/ImportExternalSessionsPanel.tsx
/* __next_internal_client_entry_do_not_use__ ImportExternalSessionsPanel auto */ 
// +feature: import-external-sessions-panel






function toCandidate(session) {
    const meta = session.externalMetadata;
    const sourceKind = (meta === null || meta === void 0 ? void 0 : meta.muxEnabled) && (meta === null || meta === void 0 ? void 0 : meta.muxSocketPath) ? ImportSourceKind.MUX_DISCOVERED : ImportSourceKind.PLAIN_TMUX;
    var _meta_originalPid, _meta_tmuxSessionName, _meta_muxSocketPath;
    return {
        sourceKind,
        path: session.existingDir,
        program: session.program,
        pid: (_meta_originalPid = meta === null || meta === void 0 ? void 0 : meta.originalPid) !== null && _meta_originalPid !== void 0 ? _meta_originalPid : 0,
        tmuxSession: (_meta_tmuxSessionName = meta === null || meta === void 0 ? void 0 : meta.tmuxSessionName) !== null && _meta_tmuxSessionName !== void 0 ? _meta_tmuxSessionName : "",
        socketPath: (_meta_muxSocketPath = meta === null || meta === void 0 ? void 0 : meta.muxSocketPath) !== null && _meta_muxSocketPath !== void 0 ? _meta_muxSocketPath : ""
    };
}
function ImportExternalSessionsPanel(param) {
    let { onImport, onRefresh } = param;
    const sessions = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectAllSessions */.rt);
    const [selected, setSelected] = (0,react.useState)(new Set());
    const rows = (0,react.useMemo)(()=>{
        return sessions.filter((session)=>session.instanceType === types_pb/* InstanceType */.y9.EXTERNAL).map((session)=>{
            var _session_externalMetadata;
            var _session_externalMetadata_sourceTerminal;
            return {
                key: session.id,
                sessionId: session.id,
                title: session.title,
                path: session.existingDir,
                program: session.program,
                sourceTerminal: (_session_externalMetadata_sourceTerminal = (_session_externalMetadata = session.externalMetadata) === null || _session_externalMetadata === void 0 ? void 0 : _session_externalMetadata.sourceTerminal) !== null && _session_externalMetadata_sourceTerminal !== void 0 ? _session_externalMetadata_sourceTerminal : "",
                candidate: toCandidate(session)
            };
        });
    }, [
        sessions
    ]);
    const allChecked = rows.length > 0 && selected.size === rows.length;
    const toggleAll = (0,react.useCallback)(()=>{
        setSelected((prev)=>prev.size === rows.length ? new Set() : new Set(rows.map((r)=>r.key)));
    }, [
        rows
    ]);
    const toggleRow = (0,react.useCallback)((key)=>{
        setSelected((prev)=>{
            const next = new Set(prev);
            if (next.has(key)) {
                next.delete(key);
            } else {
                next.add(key);
            }
            return next;
        });
    }, []);
    const importRow = (0,react.useCallback)((row)=>{
        onImport === null || onImport === void 0 ? void 0 : onImport([
            row.candidate
        ]);
    }, [
        onImport
    ]);
    const importSelected = (0,react.useCallback)(()=>{
        const candidates = rows.filter((r)=>selected.has(r.key)).map((r)=>r.candidate);
        onImport === null || onImport === void 0 ? void 0 : onImport(candidates);
    }, [
        rows,
        selected,
        onImport
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: panel,
        "data-testid": "import-external-sessions-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: titleRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                                className: title,
                                children: "External Sessions"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: subtitle,
                                children: "Terminal sessions discovered outside Stapler Squad that can be imported for management."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: refreshButton,
                        onClick: onRefresh,
                        "aria-label": "Refresh discovered sessions",
                        type: "button",
                        children: "↻"
                    })
                ]
            }),
            selected.size > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: bulkActionBar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: bulkActionCount,
                        children: [
                            selected.size,
                            " selected"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: bulkImportBtn,
                        onClick: importSelected,
                        type: "button",
                        children: "Import selected →"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: bulkClearBtn,
                        onClick: ()=>setSelected(new Set()),
                        type: "button",
                        children: "Clear"
                    })
                ]
            }),
            rows.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: empty,
                "data-testid": "import-external-sessions-empty",
                children: "No external sessions detected. Sessions started outside Stapler Squad will appear here for import."
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: tableWrapper,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                    className: table,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: checkboxTh,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "checkbox",
                                            checked: allChecked,
                                            onChange: toggleAll,
                                            "aria-label": "Select all"
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Title"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Path"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Program"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th,
                                        children: "Source"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                        className: th
                                    })
                                ]
                            })
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                            children: rows.map((row)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                    className: ImportExternalSessionsPanel_css_row,
                                    "data-testid": "import-external-session-row",
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: checkboxTd,
                                            onClick: (e)=>e.stopPropagation(),
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "checkbox",
                                                checked: selected.has(row.key),
                                                onChange: ()=>toggleRow(row.key),
                                                "aria-label": "Select ".concat(row.title)
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: td,
                                            children: row.title
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: "".concat(td, " ").concat(pathText),
                                            children: row.path
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: td,
                                            children: row.program
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: td,
                                            children: row.sourceTerminal ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: sourceBadge,
                                                children: row.sourceTerminal
                                            }) : "—"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                            className: td,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: rowImportBtn,
                                                onClick: ()=>importRow(row),
                                                type: "button",
                                                "data-testid": "import-row-button",
                                                children: "Import"
                                            })
                                        })
                                    ]
                                }, row.key))
                        })
                    ]
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useFocusTrap.ts
var useFocusTrap = __webpack_require__(63991);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FImportPreviewDialog.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VY246bOhR9n6_wS6WJVCIglybOS3-jT5WDDXhibGqbhEzVf6-wIWDCZZIzR5VGGQfw2muvfTP5fiLXWKKMKPAz3K33l50Pfr8AEEuRmQUAIkcR1VcI_MMLAH9eANCifyuwt_68fL_DC_p4WiKuYiEzaJcMafLjNfTzcnGYMzi81-9vvNFZ1iRCsz8XimoqOAQxLQk-GOS8tsNIrOulpEnarI9Ca5E1X1B0SqQoOIbgjOSr5_0M9uQ9lO_IUMBU5QxdIYgZKasLiNGEe1STTEEQEa6JrC6_FUrT-OpFgmvCdffWu0c5JiUEge8bm4jTDFnWtwj5y1ABghQ5dHxcGR_HKe4X1h-JifQkwrRQEARhbojmCGPKk9rPDJXehWKdQrDZ-vaJ-vve_9I8kRKr084_p4PeV_89TCWJLP9IsCLjlkXpqRRhcYHAB1XsQWUHyOSIXv2voP5brhbDEgTVvXsJ1jbMjS_hOi_tR7CtVx0JmsgGeQmUYBT39HpbdKE3BjpDMqG84uyDCteoFQuuPUXfCTSe3C5dan22NpCRYEL2g-I7RraukR54UPMfBAocoG_3QlQ7xZnImImLd4UAFVo0Qaor5oEAJiiHINhVsK3VnWt1FTYKaVJqz9RCN9VH_RjwujWyd41UCXxTZjz5A4s7bDIcqoxtJ5I1k5XLBBkmj4q2dUGOwyDm0d0shcjsnhOyScSNTcSM8qa6vzX5WnFWqaT8ZPKutYDHLdi22-QUBCnFmPBbvNsbhDGaK6pMG0mpJp7KUUQg4OIiUX7jGaOMsisEmeDCPNH3PnS9J08FoJe0sQtyZCI6HaaqcPVgidtAGEk646vIcyIj07-q2aM1kUYV24SXG5dk4pKknFFOPHfKKMJiWEdSI6kn5k-biJ1KMn3Sd1pkUwv7_f4-FcMxHVra6cxIOo8XZeH0MzoDdB0HKh2gtxkgPNEmHKBTrw1V06uRZA7eqjs-eZJH-tFkcZo0aeZ0sNz0R3fQDKz7ydC6yj6jzLJhECdB3dy1I8bvZakr9H8QMiqkqmTLBW3KwpRnfUJEjAF_GWzuDxkZTCu1bCJZ6MEIpIvpXNg5-cRn8bKH8ETnKOGZU26veeZPRbUWvjNCnIL_ZQ_7H2_mkz3zM0ZLy0223GoGwQSDtaOmmpjS9yn6sSPJ9UO9oPwfe0Hrn-51s4HjssmhqbPygD537zkmuwjHrXi9rCwG2mo4K2U03rX9T-2464nZ_5F-MvDaVth2ArnQrxBThY6M4MXMmNp1-Y52n66RBtkAt2_YdiQ03LmojulMXKqX43b3-amgZONB-eUEhQtO_pn852fk54sb_t2PEV6Qu0eOM0SRpmcyZGL894zu_idi9xevgvhF2BEAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FImportPreviewDialog_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VY246bOhR9n6_wS6WJVCIglybOS3_jT5WDDXhibGqbhEzVf6_wIWDCZZIzR5VGGQfw2muvfTP5fiLXWKKMKPAz3K33l50Pfr8AEEuRmQUAIkcR1VcI_MMLAH9eANCifyuwt_68fL_DC_p4WiKuYiEzaJcMafLjNfTzcnGYMzi81_9vvNFZ1iRCsz8XimoqOAQxLQk_GOS8tsNIrOulpEnarI9Ca5E1X1B0SqQoOIbgjOSr5_0M9uQ9lO_IUMBU5QxdIYgZKasLiNGEe1STTEEQEa6JrC6_FUrT_OpFgmvCdffWu0c5JiUEge8bm4jTDFnWtwj5y1ABghQ5dHxcGR_HKe4X1h_JifQkwrRQEARhbojmCGPKk9rPDJXehWKdQrDZ_vaJ_vve_9I8kRKr084_p4PeV_89TCWJLP9IsCLjlkXpqRRhcYHAB1XsQWUHyOSIXv2voP5brhbDEgTVvXsJ1jbMjS_hOi_tR7CtVx0JmsgGeQmUYBT39HpbdKE3BjpDMqG84uyDCteoFQuuPUXfCTSe3C5dan22NpCRYEL2g_I7RraukR54UPMfBAocoG_3QlQ7xZnImImLd4UAFVo0Qaor5oEAJiiHINhVsK3VnWt1FTYKaVJqz9RCN9VH_RjwujWyd41UCXxTZjz5A4s7bDIcqoxtJ5I1k5XLBBkmj4q2dUGOwyDm0d0shcjsnhOyScSNTcSM8qa6vzX5WnFWqaT8ZPKutYDHLdi22_QUBCnFmPBbvNsbhDGaK6pMG0mpJp7KUUQg4OIiUX7jGaOMsisEmeDCPNH3PnS9J08FoJe0sQtyZCI6HaaqcPVgidtAGEk646vIcyIj07_q2aM1kUYV24SXG5dk4pKknFFOPHfKKMJiWEdSI6kn5k_biJ1KMn3Sd1pkUwv7_f4_FcMxHVra6cxIOo8XZeH0MzoDdB0HKh2gtxkgPNEmHKBTrw1V06uRZA7eqjs_eZJH_tFkcZo0aeZ0sNz0R3fQDKz7ydC6yj6jzLJhECdB3dy1I8bvZakr9H8QMiqkqmTLBW3KwpRnfUJEjAF_GWzuDxkZTCu1bCJZ6MEIpIvpXNg5_cRn8bKH8ETnKOGZU26veeZPRbUWvjNCnIL_ZQ_7H2_mkz3zM0ZLy0223GoGwQSDtaOmmpjS9yn6sSPJ9UO9oPwfe0Hrn_51s4HjssmhqbPygD537zkmuwjHrXi9rCwG2mo4K2U03rX9T_2464nZ_5F_MvDaVth2ArnQrxBThY6M4MXMmNp1_Y52n66RBtkAt2_YdiQ03LmojulMXKqX43b3_amgZONB_eUEhQtO_pn852fk54sb_t2PEV6Qu0eOM0SRpmcyZGL894zu_idi9xevgvhF2BEAAA_22_7D = __webpack_require__(46405);
;// ./src/components/sessions/ImportPreviewDialog.css.ts


var body = '_2849w87';
var cancelButton = '_2849w8u';
var candidateDetails = '_2849w8p';
var candidateList = '_2849w8l';
var candidateOption = '_2849w8m';
var candidateOptionSelected = '_2849w8n';
var candidatePath = '_2849w8q';
var candidateRadio = '_2849w8o';
var candidateUuid = '_2849w8r';
var confirmButton = '_2849w8v';
var contextGrid = '_2849w8a';
var contextLabel = '_2849w8c';
var contextRow = '_2849w8b';
var contextValue = '_2849w8d';
var correlationBadge = '_2849w8g';
var correlationBadgeAmbiguous = '_2849w8i';
var correlationBadgeNotFound = '_2849w8j';
var correlationBadgeResolved = '_2849w8h';
var errorState = '_2849w89';
var excerptBox = '_2849w8k';
var fieldLabel = '_2849w8f';
var footer = '_2849w8t';
var header = '_2849w84';
var loadingState = '_2849w88';
var modal = '_2849w83';
var overlay = '_2849w82';
var section = '_2849w8e';
var sigstopBanner = '_2849w8s';
var ImportPreviewDialog_css_subtitle = '_2849w86';
var ImportPreviewDialog_css_title = '_2849w85';
;// ./src/components/sessions/ImportPreviewDialog.tsx
/* __next_internal_client_entry_do_not_use__ ImportPreviewDialog auto */ 
// +feature: import-preview-dialog





const CONFIDENCE_LABEL = {
    [CorrelationConfidence.UNSPECIFIED]: "",
    [CorrelationConfidence.NONE]: "No confidence",
    [CorrelationConfidence.PID_EXACT]: "Exact PID match",
    [CorrelationConfidence.PATH_HEURISTIC]: "Path heuristic match"
};
function ImportPreviewDialog(param) {
    let { candidate, onConfirm, onCancel, triggerRef } = param;
    var _preview_correlation, _preview_correlation1, _preview_correlation2;
    const modalRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(modalRef, true, triggerRef);
    const { previewImport } = useImportSessionService();
    const [preview, setPreview] = (0,react.useState)(null);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [disambiguationChoice, setDisambiguationChoice] = (0,react.useState)("");
    const [isSubmitting, setIsSubmitting] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        let cancelled = false;
        setLoading(true);
        setError(null);
        previewImport(candidate).then((result)=>{
            var _result_correlation_candidates, _result_correlation, _result_correlation1;
            if (cancelled) return;
            if (!result) {
                setError("Failed to preview this session.");
                return;
            }
            setPreview(result);
            const firstCandidate = (_result_correlation = result.correlation) === null || _result_correlation === void 0 ? void 0 : (_result_correlation_candidates = _result_correlation.candidates) === null || _result_correlation_candidates === void 0 ? void 0 : _result_correlation_candidates[0];
            if (((_result_correlation1 = result.correlation) === null || _result_correlation1 === void 0 ? void 0 : _result_correlation1.kind) === CorrelationKind.AMBIGUOUS && firstCandidate) {
                setDisambiguationChoice(firstCandidate.conversationUuid);
            }
        }).catch(()=>{
            if (!cancelled) setError("Failed to preview this session.");
        }).finally(()=>{
            if (!cancelled) setLoading(false);
        });
        return ()=>{
            cancelled = true;
        };
    // candidate identity is stable per dialog instance
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    var _preview_correlation_kind;
    const kind = (_preview_correlation_kind = preview === null || preview === void 0 ? void 0 : (_preview_correlation = preview.correlation) === null || _preview_correlation === void 0 ? void 0 : _preview_correlation.kind) !== null && _preview_correlation_kind !== void 0 ? _preview_correlation_kind : CorrelationKind.UNSPECIFIED;
    const canConfirm = !loading && !error && !!preview && (kind !== CorrelationKind.AMBIGUOUS || !!disambiguationChoice);
    const handleSubmit = async ()=>{
        if (!preview || !canConfirm || isSubmitting) return;
        setIsSubmitting(true);
        try {
            await onConfirm({
                preview,
                disambiguationChoice: kind === CorrelationKind.AMBIGUOUS ? disambiguationChoice : undefined
            });
        } catch (e) {
            setIsSubmitting(false);
        }
    };
    const badgeClassForKind = (k)=>{
        switch(k){
            case CorrelationKind.RESOLVED:
                return "".concat(correlationBadge, " ").concat(correlationBadgeResolved);
            case CorrelationKind.AMBIGUOUS:
                return "".concat(correlationBadge, " ").concat(correlationBadgeAmbiguous);
            default:
                return "".concat(correlationBadge, " ").concat(correlationBadgeNotFound);
        }
    };
    const badgeTextForKind = (k)=>{
        switch(k){
            case CorrelationKind.RESOLVED:
                return "History matched";
            case CorrelationKind.AMBIGUOUS:
                return "Multiple histories found";
            default:
                return "No history matched";
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        onClick: onCancel,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: modal,
            onClick: (e)=>e.stopPropagation(),
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": "import-preview-title",
            ref: modalRef,
            "data-testid": "import-preview-dialog",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: header,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                            className: ImportPreviewDialog_css_title,
                            id: "import-preview-title",
                            children: "Import External Session"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: ImportPreviewDialog_css_subtitle,
                            children: "Review the discovered session before importing"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: body,
                    children: [
                        loading && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: loadingState,
                            children: "Looking up session history…"
                        }),
                        !loading && error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: errorState,
                            children: error
                        }),
                        !loading && !error && preview && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: contextGrid,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: contextRow,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextLabel,
                                                    children: "Program:"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextValue,
                                                    children: preview.program
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: contextRow,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextLabel,
                                                    children: "Path:"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: contextValue,
                                                    title: preview.path,
                                                    children: preview.path
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: section,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: fieldLabel,
                                            children: "Conversation History"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            className: badgeClassForKind(kind),
                                            children: [
                                                badgeTextForKind(kind),
                                                ((_preview_correlation1 = preview.correlation) === null || _preview_correlation1 === void 0 ? void 0 : _preview_correlation1.confidence) !== undefined && preview.correlation.confidence !== CorrelationConfidence.UNSPECIFIED && " \xb7 ".concat(CONFIDENCE_LABEL[preview.correlation.confidence])
                                            ]
                                        }),
                                        kind === CorrelationKind.RESOLVED && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                    className: contextRow,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: contextLabel,
                                                            children: "Turns:"
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: contextValue,
                                                            children: preview.turnCount
                                                        })
                                                    ]
                                                }),
                                                preview.lastMessageExcerpt && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: excerptBox,
                                                    children: preview.lastMessageExcerpt
                                                })
                                            ]
                                        }),
                                        kind === CorrelationKind.AMBIGUOUS && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: candidateList,
                                            role: "radiogroup",
                                            "aria-label": "Select conversation history",
                                            children: (_preview_correlation2 = preview.correlation) === null || _preview_correlation2 === void 0 ? void 0 : _preview_correlation2.candidates.map((c)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: disambiguationChoice === c.conversationUuid ? "".concat(candidateOption, " ").concat(candidateOptionSelected) : candidateOption,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            type: "radio",
                                                            className: candidateRadio,
                                                            name: "disambiguation-choice",
                                                            checked: disambiguationChoice === c.conversationUuid,
                                                            onChange: ()=>setDisambiguationChoice(c.conversationUuid)
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                            className: candidateDetails,
                                                            children: [
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                    className: candidatePath,
                                                                    title: c.historyFilePath,
                                                                    children: c.projectDir || c.historyFilePath
                                                                }),
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                                    className: candidateUuid,
                                                                    children: c.conversationUuid
                                                                })
                                                            ]
                                                        })
                                                    ]
                                                }, c.conversationUuid))
                                        })
                                    ]
                                }),
                                preview.pidIdentity && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: sigstopBanner,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            children: "⚠"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            children: [
                                                "Importing will suspend (SIGSTOP) the original process (PID ",
                                                preview.pidIdentity.pid,
                                                ") and resume it under Stapler Squad's management. The original terminal will freeze until you confirm or cancel the kill afterward."
                                            ]
                                        })
                                    ]
                                })
                            ]
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: footer,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: onCancel,
                            className: cancelButton,
                            disabled: isSubmitting,
                            type: "button",
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: handleSubmit,
                            className: confirmButton,
                            disabled: !canConfirm || isSubmitting,
                            type: "button",
                            "data-testid": "import-preview-confirm-button",
                            children: isSubmitting ? "Importing..." : "Import Session"
                        })
                    ]
                })
            ]
        })
    });
}

;// ./src/components/sessions/importRowStatus.ts
const TERMINAL_STATUSES = new Set([
    "imported",
    "reverted"
]);
function isTerminalImportRowStatus(status) {
    return TERMINAL_STATUSES.has(status);
}
function nextImportRowStatus(current, event) {
    if (isTerminalImportRowStatus(current)) {
        return current;
    }
    switch(event.type){
        case "commit_succeeded":
            return "committed_pending_kill";
        case "commit_failed":
            return "discovered";
        case "kill_confirmed":
            return "imported";
        case "kill_failed":
            return "kill_failed";
        case "kill_cancelled":
            return "reverted";
        case "cancel_failed":
            return "committed_pending_kill";
        default:
            return current;
    }
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FmodalChrome.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VVTW-jMBC951fMpVKQ1pGhbZqQS__GnlYONuAWbNZ2UpJV__sKG1JwgGSl3cNKETH-ePPmzfPw-s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH-UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve-FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec-UPAgaw5GoJUI_wi07R-pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3-oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd-mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO-nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U-cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9-xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FmodalChrome_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VVTW_jMBC951fMpVKQ1pGhbZqQS_GnlYONuAWbNZ2UpJV_sKG1JwgGSl3cNKETH_ePPmzfPw_s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH_UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve_FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec_UPAgaw5GoJUI_wi07R_pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3_oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd_mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO_nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U_cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9_xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA_22_7D = __webpack_require__(6579);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FConfirmKillDialog.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VTzXLbIBC--yn2aB3QgGKrLnmYDhZIJqUsAyi23Om7Z5AcR3JNMkkPvWl2l93vT2Uc6tY1FH6vAIy2ihyU7g6RAyu3j6s_q3IaYOOAE1Jq23GoNu70uALAZ-Vbg0cycBB9xFRrjTpxYOlT6uCMGPhYe-0Rqb1qokbLoUHT_7Kp0wnHgVVp7fVotTyausAul_ei-dl57K3k8Cz8mpAf7Ls6V_7MWJEGGjTo_-pVY2-PXipPvJC6DxzqaWWLNpKgz4oDe5hK7yjyMIL7LMML-gynERYH5k4Q0Gh5g77Lgr-i2oyo7jAZS8cLk5rSrES0mK3b5tbdV7f4QLP6xtDana5RutCK6PICPBV3U_XUh6jbgTRoo7JxahBlZS5Y325w0DmObLCafK5o8W_-LTXevGNZ70M671DbqHwqRS9s0FPahDFAyyqAEkHNCfND-lW5xbjmUgexN0oWowp5vrs53Lu0D8XixuvicS860eg4cKApAm_ILUYijMGjkrPHuy85wmjeku3CEotW_S_xd3nxW21igrf36ZBVIaxZyYrF4y-o-gJk5D1o1gUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FConfirmKillDialog_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VTzXLbIBC_yn2aB3QgGKrLnmYDhZIJqUsAyi23Om7Z5AcR3JNMkkPvWl2l93vT2Uc6tY1FH6vAIy2ihyU7g6RAyu3j6s_q3IaYOOAE1Jq23GoNu70uALAZ_Vbg0cycBB9xFRrjTpxYOlT6uCMGPhYe_0Rqb1qokbLoUHT_7Kp0wnHgVVp7fVotTyausAul_ei_dl57K3k8Cz8mpAf7Ls6V_7MWJEGGjTo_pVY2_PXipPvJC6DxzqaWWLNpKgz4oDe5hK7yjyMIL7LMML_gynERYH5k4Q0Gh5g77Lgr_i2oyo7jAZS8cLk5rSrES0mK3b5tbdV7f4QLP6xtDana5RutCK6PICPBV3U_XUh6jbgTRoo7JxahBlZS5Y325w0DmObLCafK5o8W_LTXevGNZ70M671DbqHwqRS9s0FPahDFAyyqAEkHNCfND_lW5xbjmUgexN0oWowp5vrs53Lu0D8XixuvicS860eg4cKApAm_ILUYijMGjkrPHuy85wmjeku3CEotW_S_xd3nxW21igrf36ZBVIaxZyYrF4y_o_gJk5D1o1gUAAA_22_7D = __webpack_require__(54618);
;// ./src/components/sessions/ConfirmKillDialog.css.ts



var ConfirmKillDialog_css_body = 'ty6fpc1';
var ConfirmKillDialog_css_cancelButton = 'ty6fpc7';
var ConfirmKillDialog_css_errorState = 'ty6fpc2';
var ConfirmKillDialog_css_footer = 'ty6fpc6';
var ConfirmKillDialog_css_header = 'x9n6md4';
var killButton = 'ty6fpc8';
var ConfirmKillDialog_css_modal = 'x9n6md3';
var optionCard = 'ty6fpc3';
var optionDescription = 'ty6fpc5';
var optionTitle = 'ty6fpc4';
var ConfirmKillDialog_css_overlay = 'x9n6md2';
var ConfirmKillDialog_css_subtitle = 'ty6fpc0 x9n6md6';
var ConfirmKillDialog_css_title = 'x9n6md5';
;// ./src/components/sessions/ConfirmKillDialog.tsx
/* __next_internal_client_entry_do_not_use__ ConfirmKillDialog auto */ 
// +feature: confirm-kill-dialog






function ConfirmKillDialog(param) {
    let { instanceId, pidIdentity, program, onStatusChange, onClose, triggerRef } = param;
    const modalRef = (0,react.useRef)(null);
    (0,useFocusTrap/* useFocusTrap */.r)(modalRef, true, triggerRef);
    const { confirmKill, cancelPendingKill } = useImportSessionService();
    const [isKilling, setIsKilling] = (0,react.useState)(false);
    const [isCancelling, setIsCancelling] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const busy = isKilling || isCancelling;
    const handleConfirmKill = async ()=>{
        if (busy) return;
        setIsKilling(true);
        setError(null);
        try {
            const result = await confirmKill({
                instanceId,
                pidIdentity
            });
            if (!result || result.status === KillStatus.FAILED) {
                setError((result === null || result === void 0 ? void 0 : result.error) || "Failed to kill the original process. It remains suspended (SIGSTOP) — you can retry or revert the import.");
                onStatusChange(nextImportRowStatus("committed_pending_kill", {
                    type: "kill_failed"
                }));
                return;
            }
            onStatusChange(nextImportRowStatus("committed_pending_kill", {
                type: "kill_confirmed"
            }));
            onClose();
        } finally{
            setIsKilling(false);
        }
    };
    const handleCancelPending = async ()=>{
        if (busy) return;
        setIsCancelling(true);
        setError(null);
        try {
            const result = await cancelPendingKill({
                instanceId,
                pidIdentity
            });
            if (!result || !result.resumed) {
                setError((result === null || result === void 0 ? void 0 : result.error) || "Failed to revert the import. The original process remains suspended (SIGSTOP) — you can retry.");
                onStatusChange(nextImportRowStatus("committed_pending_kill", {
                    type: "cancel_failed"
                }));
                return;
            }
            onStatusChange(nextImportRowStatus("committed_pending_kill", {
                type: "kill_cancelled"
            }));
            onClose();
        } finally{
            setIsCancelling(false);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: ConfirmKillDialog_css_overlay,
        onClick: busy ? undefined : onClose,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: ConfirmKillDialog_css_modal,
            onClick: (e)=>e.stopPropagation(),
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": "confirm-kill-title",
            ref: modalRef,
            "data-testid": "confirm-kill-dialog",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ConfirmKillDialog_css_header,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                            className: ConfirmKillDialog_css_title,
                            id: "confirm-kill-title",
                            children: [
                                "Finish Importing ",
                                program
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: ConfirmKillDialog_css_subtitle,
                            children: "The session has been imported and is now managed by Stapler Squad. The original process is suspended (SIGSTOP). Choose what to do with it."
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ConfirmKillDialog_css_body,
                    children: [
                        error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: ConfirmKillDialog_css_errorState,
                            children: error
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: optionCard,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: optionTitle,
                                    children: "Revert import"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: optionDescription,
                                    children: "Discard the imported session and resume (SIGCONT) the original process so it continues running as before."
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: optionCard,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: optionTitle,
                                    children: "Confirm kill"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: optionDescription,
                                    children: "Permanently terminate the original process. This cannot be undone."
                                })
                            ]
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ConfirmKillDialog_css_footer,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: handleCancelPending,
                            className: ConfirmKillDialog_css_cancelButton,
                            disabled: busy,
                            type: "button",
                            "data-testid": "confirm-kill-revert-button",
                            children: isCancelling ? "Reverting..." : "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            onClick: handleConfirmKill,
                            className: killButton,
                            disabled: busy,
                            type: "button",
                            "data-testid": "confirm-kill-kill-button",
                            children: isKilling ? "Killing..." : "Confirm Kill"
                        })
                    ]
                })
            ]
        })
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FImportSessionsContainer.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll83d2xubm0wIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiAxNnB4OwogIHBhZGRpbmc6IDI0cHg7CiAgbWF4LXdpZHRoOiA5NjBweDsKICBtYXJnaW46IDAgYXV0bzsKfQouXzd3bG5ubTEgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MTEpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEyKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyejEwKTsKICBib3JkZXItcmFkaXVzOiA4cHg7CiAgcGFkZGluZzogMTBweCAxNHB4OwogIGZvbnQtc2l6ZTogMTRweDsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FImportSessionsContainer_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll83d2xubm0wIHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiAxNnB4OwogIHBhZGRpbmc6IDI0cHg7CiAgbWF4LXdpZHRoOiA5NjBweDsKICBtYXJnaW46IDAgYXV0bzsKfQouXzd3bG5ubTEgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MTEpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEyKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyejEwKTsKICBib3JkZXItcmFkaXVzOiA4cHg7CiAgcGFkZGluZzogMTBweCAxNHB4OwogIGZvbnQtc2l6ZTogMTRweDsKfQ_22_7D = __webpack_require__(75104);
;// ./src/components/sessions/ImportSessionsContainer.css.ts


var container = '_7wlnnm0';
var errorBanner = '_7wlnnm1';
;// ./src/components/sessions/ImportSessionsContainer.tsx
/* __next_internal_client_entry_do_not_use__ ImportSessionsContainer auto */ 
// +feature: import-sessions-container







function ImportSessionsContainer() {
    const { commitImport } = useImportSessionService();
    const [previewCandidate, setPreviewCandidate] = (0,react.useState)(null);
    // importQueue holds candidates still waiting to be previewed after
    // previewCandidate, so a bulk "Import selected" (N candidates) works
    // through all of them one at a time instead of only the first.
    const [importQueue, setImportQueue] = (0,react.useState)([]);
    const [pendingKill, setPendingKill] = (0,react.useState)(null);
    const [commitError, setCommitError] = (0,react.useState)(null);
    // Import can be triggered from a per-row button or a bulk "Import
    // selected" button, so there's no single static opener element to attach
    // a ref to. Capture whichever one was actually focused at click time and
    // reuse it across the whole chained flow (preview -> kill dialog), since
    // ConfirmKillDialog has no trigger of its own.
    const importTriggerRef = (0,react.useRef)(null);
    // Persistent fallback for ConfirmKillDialog's triggerRef: the per-row
    // Import button (captured into importTriggerRef) unmounts once the row's
    // instanceType flips away from EXTERNAL as part of the same commit that
    // produces pidIdentity/pendingKill (see ImportExternalSessionsPanel's
    // EXTERNAL filter), so by the time the kill dialog opens that node can
    // already be detached. This container div is always mounted.
    const fallbackTriggerRef = (0,react.useRef)(null);
    // advanceQueue pulls the next candidate (if any) off importQueue and makes
    // it the active preview, or clears previewCandidate once the queue is
    // empty. Call this whenever the current candidate's flow (preview
    // cancelled, commit succeeded with no kill needed, or kill dialog closed)
    // has finished, so the next selected candidate is picked up automatically.
    const advanceQueue = (0,react.useCallback)((queue)=>{
        const [next, ...rest] = queue;
        setImportQueue(rest);
        setPreviewCandidate(next !== null && next !== void 0 ? next : null);
    }, []);
    const handleImport = (0,react.useCallback)((candidates)=>{
        if (candidates.length === 0) return;
        importTriggerRef.current = document.activeElement;
        setCommitError(null);
        advanceQueue(candidates);
    }, [
        advanceQueue
    ]);
    const handleConfirmPreview = (0,react.useCallback)(async (param)=>{
        let { preview, disambiguationChoice } = param;
        const candidate = previewCandidate;
        if (!candidate || !preview.correlation) return;
        const result = await commitImport({
            candidate,
            expectedCorrelation: preview.correlation,
            disambiguationChoice,
            pidIdentity: preview.pidIdentity
        });
        if (!result || result.status !== ImportStatus.COMMITTED) {
            setCommitError((result === null || result === void 0 ? void 0 : result.error) || "Failed to import session.");
            return;
        }
        if (result.pidIdentity) {
            var _importTriggerRef_current;
            if (!((_importTriggerRef_current = importTriggerRef.current) === null || _importTriggerRef_current === void 0 ? void 0 : _importTriggerRef_current.isConnected)) {
                importTriggerRef.current = fallbackTriggerRef.current;
            }
            setPreviewCandidate(null);
            setPendingKill({
                instanceId: result.instanceId,
                pidIdentity: result.pidIdentity,
                program: preview.program
            });
            return;
        }
        // No kill decision needed for this candidate -- move on to the next
        // queued candidate (if any) right away.
        advanceQueue(importQueue);
    }, [
        previewCandidate,
        commitImport,
        importQueue,
        advanceQueue
    ]);
    const handleKillStatusChange = (0,react.useCallback)((_status)=>{
    // Row status is displayed inline in ImportExternalSessionsPanel once wired
    // to live session state; this container only needs to close the dialog.
    }, []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: fallbackTriggerRef,
        tabIndex: -1,
        className: container,
        "data-testid": "import-sessions-container",
        children: [
            commitError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: errorBanner,
                role: "alert",
                "data-testid": "import-commit-error",
                children: commitError
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(ImportExternalSessionsPanel, {
                onImport: handleImport
            }),
            previewCandidate && /*#__PURE__*/ (0,jsx_runtime.jsx)(ImportPreviewDialog, {
                candidate: previewCandidate,
                onConfirm: handleConfirmPreview,
                onCancel: ()=>advanceQueue(importQueue),
                triggerRef: importTriggerRef
            }),
            pendingKill && /*#__PURE__*/ (0,jsx_runtime.jsx)(ConfirmKillDialog, {
                instanceId: pendingKill.instanceId,
                pidIdentity: pendingKill.pidIdentity,
                program: pendingKill.program,
                onStatusChange: handleKillStatusChange,
                onClose: ()=>{
                    setPendingKill(null);
                    advanceQueue(importQueue);
                },
                triggerRef: importTriggerRef
            })
        ]
    });
}


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

/***/ 63991:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
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

/***/ 64770:
/***/ (() => {

// extracted by mini-css-extract-plugin

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

/***/ 75104:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 77665:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   r: () => (/* binding */ createRpcTimingInterceptor)
/* harmony export */ });
/* harmony import */ var _opentelemetry_api__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(82099);

/**
 * Network-queueing-delay attribute name set on the active OTel span (see
 * getNetworkQueueDelayMs below). Named to sort next to the standard
 * `http.*`/`url.*` fetch-instrumentation attributes in a trace viewer.
 */ const NETWORK_QUEUE_TIME_ATTRIBUTE = "network.queue_time_ms";
/**
 * Returns a proxy for how long this call's underlying fetch spent queued or
 * negotiating a connection before the browser actually put bytes on the
 * wire — the gap between the Resource Timing API's `fetchStart` and
 * `requestStart` for the matching entry. On an HTTP/1.1 origin capped at 6
 * concurrent connections per browser (this app, prior to the WS-bridge fix
 * for the Watch-family RPCs and the terminal stream — see
 * docs/how-to/enable-opentelemetry.md's "Browser network queueing delay"
 * section), a request that has to wait for a free connection shows a large
 * gap here even though the server never saw it start late. Distinguishing
 * that from genuine server-side latency was previously only possible by
 * hand-querying Tempo and noticing an RPC had *no* server-side span at all.
 *
 * Matches by exact URL (same-origin, so ConnectRPC's per-method absolute
 * URL is unique enough in practice) and picks the most recently recorded
 * matching entry, since a prior call to the same RPC method would have left
 * an earlier, already-superseded entry in the resource timing buffer.
 * Returns undefined if the Resource Timing API is unavailable (non-browser
 * test environment) or no matching/complete entry is found.
 */ function getNetworkQueueDelayMs(url) {
    if (typeof performance === "undefined" || typeof performance.getEntriesByType !== "function") {
        return undefined;
    }
    const entries = performance.getEntriesByType("resource");
    for(let i = entries.length - 1; i >= 0; i--){
        const entry = entries[i];
        if (entry.name !== url) {
            continue;
        }
        if (entry.requestStart > 0 && entry.fetchStart > 0) {
            return entry.requestStart - entry.fetchStart;
        }
        return undefined;
    }
    return undefined;
}
/**
 * ConnectRPC interceptor that records timing for every unary RPC call.
 *
 * Each call emits a Performance API entry:
 *   performance.mark(`rpc:<MethodName>:start`)
 *   performance.measure(`rpc:<MethodName>`, start, end)
 *
 * Entries are visible in Chrome DevTools > Performance > Timings track and
 * accessible via window.performance.getEntriesByType('measure').
 *
 * Attributes recorded in the measure detail:
 *   { method, url, ok, durationMs, queueDelayMs }
 *
 * queueDelayMs (see getNetworkQueueDelayMs) is also set as the
 * `network.queue_time_ms` attribute on the currently active OTel span —
 * normally the fetch span web-app/src/lib/telemetry/otel-init.ts's
 * FetchInstrumentation created for this exact call, which stays active
 * through the fetch promise chain via ZoneContextManager — so a queued
 * request is visibly distinguishable from a slow server-side one directly
 * in Tempo/Grafana. A no-op (nothing thrown) when OTel tracing isn't
 * enabled, since trace.getActiveSpan() returns undefined in that case.
 *
 * When an optional analytics provider is supplied, each RPC call also enqueues
 * an analytics event with category "rpc" for latency tracking.
 */ function createRpcTimingInterceptor(analytics) {
    return (next)=>async (req)=>{
            const method = req.method.name;
            const startMark = "rpc:".concat(method, ":start");
            if (typeof performance !== "undefined") {
                performance.mark(startMark);
            }
            const wallStart = Date.now();
            let ok = true;
            try {
                return await next(req);
            } catch (err) {
                ok = false;
                throw err;
            } finally{
                const durationMs = Date.now() - wallStart;
                const queueDelayMs = getNetworkQueueDelayMs(req.url);
                if (typeof performance !== "undefined") {
                    try {
                        performance.measure("rpc:".concat(method), {
                            start: startMark,
                            detail: {
                                method,
                                url: req.url,
                                ok,
                                durationMs,
                                queueDelayMs
                            }
                        });
                    } catch (e) {
                    // start mark may be missing in some edge cases; ignore
                    }
                }
                if (queueDelayMs !== undefined) {
                    var _trace_getActiveSpan;
                    (_trace_getActiveSpan = _opentelemetry_api__WEBPACK_IMPORTED_MODULE_0__/* .trace */ .u.getActiveSpan()) === null || _trace_getActiveSpan === void 0 ? void 0 : _trace_getActiveSpan.setAttribute(NETWORK_QUEUE_TIME_ATTRIBUTE, queueDelayMs);
                }
                if (false) {}
                analytics === null || analytics === void 0 ? void 0 : analytics.track({
                    name: "rpc.".concat(method),
                    category: "rpc",
                    durationMs,
                    labels: {
                        method,
                        ok: String(ok),
                        ...queueDelayMs !== undefined ? {
                            networkQueueDelayMs: String(queueDelayMs)
                        } : {}
                    }
                });
            }
        };
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

/***/ 88149:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   CO: () => (/* binding */ RemoteConnectionState),
/* harmony export */   Cj: () => (/* binding */ ListRemotesRequestSchema),
/* harmony export */   Ek: () => (/* binding */ TrustRemoteHostKeyRequestSchema),
/* harmony export */   Jt: () => (/* binding */ TestRemoteConnectionRequestSchema),
/* harmony export */   TW: () => (/* binding */ CreateRemoteRequestSchema),
/* harmony export */   ag: () => (/* binding */ DraftRemoteTargetSchema),
/* harmony export */   cj: () => (/* binding */ GenerateRemoteIdentityRequestSchema),
/* harmony export */   eB: () => (/* binding */ DeleteRemoteRequestSchema),
/* harmony export */   ef: () => (/* binding */ RemoteService),
/* harmony export */   hp: () => (/* binding */ file_session_v1_remote)
/* harmony export */ });
/* unused harmony exports RemoteConfigProtoSchema, TestRemoteConnectionResponseSchema, TrustRemoteHostKeyResponseSchema, GenerateRemoteIdentityResponseSchema, ListRemotesResponseSchema, CreateRemoteResponseSchema, DeleteRemoteResponseSchema, RemoteConnectionStateSchema */
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(1844);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(15033);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(95765);
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/remote.proto (package session.v1, syntax proto3)
/* eslint-disable */ 
/**
 * Describes the file session/v1/remote.proto.
 */ const file_session_v1_remote = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__/* .fileDesc */ .w)("ChdzZXNzaW9uL3YxL3JlbW90ZS5wcm90bxIKc2Vzc2lvbi52MSJ0ChFSZW1vdGVDb25maWdQcm90bxIMCgRuYW1lGAEgASgJEgwKBGhvc3QYAiABKAkSDAoEdXNlchgDIAEoCRIMCgRwb3J0GAQgASgFEhEKCWJhc2VfcGF0aBgFIAEoCRIUCgxoYXNfaWRlbnRpdHkYBiABKAgiSwoRRHJhZnRSZW1vdGVUYXJnZXQSDAoEbmFtZRgBIAEoCRIMCgRob3N0GAIgASgJEgwKBHVzZXIYAyABKAkSDAoEcG9ydBgEIAEoBSJgChtUZXN0UmVtb3RlQ29ubmVjdGlvblJlcXVlc3QSEwoLcmVtb3RlX25hbWUYASABKAkSLAoFZHJhZnQYBSABKAsyHS5zZXNzaW9uLnYxLkRyYWZ0UmVtb3RlVGFyZ2V0InUKHFRlc3RSZW1vdGVDb25uZWN0aW9uUmVzcG9uc2USDwoHc3VjY2VzcxgBIAEoCBIYChBob3N0X2tleV91bmtub3duGAIgASgIEhMKC2ZpbmdlcnByaW50GAMgASgJEhUKDWVycm9yX21lc3NhZ2UYBCABKAkicwoZVHJ1c3RSZW1vdGVIb3N0S2V5UmVxdWVzdBITCgtyZW1vdGVfbmFtZRgBIAEoCRITCgtmaW5nZXJwcmludBgCIAEoCRIsCgVkcmFmdBgDIAEoCzIdLnNlc3Npb24udjEuRHJhZnRSZW1vdGVUYXJnZXQiRAoaVHJ1c3RSZW1vdGVIb3N0S2V5UmVzcG9uc2USDwoHc3VjY2VzcxgBIAEoCBIVCg1lcnJvcl9tZXNzYWdlGAIgASgJIi0KHUdlbmVyYXRlUmVtb3RlSWRlbnRpdHlSZXF1ZXN0EgwKBG5hbWUYASABKAkiVwoeR2VuZXJhdGVSZW1vdGVJZGVudGl0eVJlc3BvbnNlEhcKD3B1YmxpY19rZXlfdGV4dBgBIAEoCRIcChRhdXRob3JpemVkX2tleXNfbGluZRgCIAEoCSIUChJMaXN0UmVtb3Rlc1JlcXVlc3QiRQoTTGlzdFJlbW90ZXNSZXNwb25zZRIuCgdyZW1vdGVzGAEgAygLMh0uc2Vzc2lvbi52MS5SZW1vdGVDb25maWdQcm90byJgChNDcmVhdGVSZW1vdGVSZXF1ZXN0EgwKBG5hbWUYASABKAkSDAoEaG9zdBgCIAEoCRIMCgR1c2VyGAMgASgJEgwKBHBvcnQYBCABKAUSEQoJYmFzZV9wYXRoGAUgASgJIkUKFENyZWF0ZVJlbW90ZVJlc3BvbnNlEi0KBnJlbW90ZRgBIAEoCzIdLnNlc3Npb24udjEuUmVtb3RlQ29uZmlnUHJvdG8iIwoTRGVsZXRlUmVtb3RlUmVxdWVzdBIMCgRuYW1lGAEgASgJIhYKFERlbGV0ZVJlbW90ZVJlc3BvbnNlKrsBChVSZW1vdGVDb25uZWN0aW9uU3RhdGUSJwojUkVNT1RFX0NPTk5FQ1RJT05fU1RBVEVfVU5TUEVDSUZJRUQQABIlCiFSRU1PVEVfQ09OTkVDVElPTl9TVEFURV9DT05ORUNURUQQARIoCiRSRU1PVEVfQ09OTkVDVElPTl9TVEFURV9SRUNPTk5FQ1RJTkcQAhIoCiRSRU1PVEVfQ09OTkVDVElPTl9TVEFURV9ESVNDT05ORUNURUQQAzLSBAoNUmVtb3RlU2VydmljZRJrChRUZXN0UmVtb3RlQ29ubmVjdGlvbhInLnNlc3Npb24udjEuVGVzdFJlbW90ZUNvbm5lY3Rpb25SZXF1ZXN0Giguc2Vzc2lvbi52MS5UZXN0UmVtb3RlQ29ubmVjdGlvblJlc3BvbnNlIgASZQoSVHJ1c3RSZW1vdGVIb3N0S2V5EiUuc2Vzc2lvbi52MS5UcnVzdFJlbW90ZUhvc3RLZXlSZXF1ZXN0GiYuc2Vzc2lvbi52MS5UcnVzdFJlbW90ZUhvc3RLZXlSZXNwb25zZSIAEnEKFkdlbmVyYXRlUmVtb3RlSWRlbnRpdHkSKS5zZXNzaW9uLnYxLkdlbmVyYXRlUmVtb3RlSWRlbnRpdHlSZXF1ZXN0Giouc2Vzc2lvbi52MS5HZW5lcmF0ZVJlbW90ZUlkZW50aXR5UmVzcG9uc2UiABJQCgtMaXN0UmVtb3RlcxIeLnNlc3Npb24udjEuTGlzdFJlbW90ZXNSZXF1ZXN0Gh8uc2Vzc2lvbi52MS5MaXN0UmVtb3Rlc1Jlc3BvbnNlIgASUwoMQ3JlYXRlUmVtb3RlEh8uc2Vzc2lvbi52MS5DcmVhdGVSZW1vdGVSZXF1ZXN0GiAuc2Vzc2lvbi52MS5DcmVhdGVSZW1vdGVSZXNwb25zZSIAElMKDERlbGV0ZVJlbW90ZRIfLnNlc3Npb24udjEuRGVsZXRlUmVtb3RlUmVxdWVzdBogLnNlc3Npb24udjEuRGVsZXRlUmVtb3RlUmVzcG9uc2UiAEKrAQoOY29tLnNlc3Npb24udjFCC1JlbW90ZVByb3RvUAFaQ2dpdGh1Yi5jb20vdHN0YXBsZXIvc3RhcGxlci1zcXVhZC9nZW4vcHJvdG8vZ28vc2Vzc2lvbi92MTtzZXNzaW9udjGiAgNTWFiqAgpTZXNzaW9uLlYxygIKU2Vzc2lvblxWMeICFlNlc3Npb25cVjFcR1BCTWV0YWRhdGHqAgtTZXNzaW9uOjpWMWIGcHJvdG8z");
/**
 * Describes the message session.v1.RemoteConfigProto.
 * Use `create(RemoteConfigProtoSchema)` to create a new message.
 */ const RemoteConfigProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 0)));
/**
 * Describes the message session.v1.DraftRemoteTarget.
 * Use `create(DraftRemoteTargetSchema)` to create a new message.
 */ const DraftRemoteTargetSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 1);
/**
 * Describes the message session.v1.TestRemoteConnectionRequest.
 * Use `create(TestRemoteConnectionRequestSchema)` to create a new message.
 */ const TestRemoteConnectionRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 2);
/**
 * Describes the message session.v1.TestRemoteConnectionResponse.
 * Use `create(TestRemoteConnectionResponseSchema)` to create a new message.
 */ const TestRemoteConnectionResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 3)));
/**
 * Describes the message session.v1.TrustRemoteHostKeyRequest.
 * Use `create(TrustRemoteHostKeyRequestSchema)` to create a new message.
 */ const TrustRemoteHostKeyRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 4);
/**
 * Describes the message session.v1.TrustRemoteHostKeyResponse.
 * Use `create(TrustRemoteHostKeyResponseSchema)` to create a new message.
 */ const TrustRemoteHostKeyResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 5)));
/**
 * Describes the message session.v1.GenerateRemoteIdentityRequest.
 * Use `create(GenerateRemoteIdentityRequestSchema)` to create a new message.
 */ const GenerateRemoteIdentityRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 6);
/**
 * Describes the message session.v1.GenerateRemoteIdentityResponse.
 * Use `create(GenerateRemoteIdentityResponseSchema)` to create a new message.
 */ const GenerateRemoteIdentityResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 7)));
/**
 * Describes the message session.v1.ListRemotesRequest.
 * Use `create(ListRemotesRequestSchema)` to create a new message.
 */ const ListRemotesRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 8);
/**
 * Describes the message session.v1.ListRemotesResponse.
 * Use `create(ListRemotesResponseSchema)` to create a new message.
 */ const ListRemotesResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 9)));
/**
 * Describes the message session.v1.CreateRemoteRequest.
 * Use `create(CreateRemoteRequestSchema)` to create a new message.
 */ const CreateRemoteRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 10);
/**
 * Describes the message session.v1.CreateRemoteResponse.
 * Use `create(CreateRemoteResponseSchema)` to create a new message.
 */ const CreateRemoteResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 11)));
/**
 * Describes the message session.v1.DeleteRemoteRequest.
 * Use `create(DeleteRemoteRequestSchema)` to create a new message.
 */ const DeleteRemoteRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 12);
/**
 * Describes the message session.v1.DeleteRemoteResponse.
 * Use `create(DeleteRemoteResponseSchema)` to create a new message.
 */ const DeleteRemoteResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 13)));
/**
 * RemoteConnectionState mirrors session/sshremote.RemoteConnectionState for
 * wire transport (Epic 6.2's RemoteHealthChangedEvent). Kept as a proto enum
 * rather than a bare string so frontend switch/exhaustiveness checks catch a
 * state the client doesn't yet handle, instead of silently falling through
 * on an unrecognized string value.
 *
 * @generated from enum session.v1.RemoteConnectionState
 */ var RemoteConnectionState = /*#__PURE__*/ function(RemoteConnectionState) {
    /**
   * @generated from enum value: REMOTE_CONNECTION_STATE_UNSPECIFIED = 0;
   */ RemoteConnectionState[RemoteConnectionState["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: REMOTE_CONNECTION_STATE_CONNECTED = 1;
   */ RemoteConnectionState[RemoteConnectionState["CONNECTED"] = 1] = "CONNECTED";
    /**
   * @generated from enum value: REMOTE_CONNECTION_STATE_RECONNECTING = 2;
   */ RemoteConnectionState[RemoteConnectionState["RECONNECTING"] = 2] = "RECONNECTING";
    /**
   * @generated from enum value: REMOTE_CONNECTION_STATE_DISCONNECTED = 3;
   */ RemoteConnectionState[RemoteConnectionState["DISCONNECTED"] = 3] = "DISCONNECTED";
    return RemoteConnectionState;
}({});
/**
 * Describes the enum session.v1.RemoteConnectionState.
 */ const RemoteConnectionStateSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_remote, 0)));
/**
 * RemoteService validates SSH connectivity to a configured remote host and
 * drives the app's TOFU (trust-on-first-use) host-key confirmation flow
 * (ssh-remote-workspaces Epic 3.3). Kept as its own proto file, separate
 * from session.proto: a remote is a distinct resource, not an extension of
 * CreateSessionRequest.
 *
 * @generated from service session.v1.RemoteService
 */ const RemoteService = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .serviceDesc */ .Z)(file_session_v1_remote, 0);


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

/***/ 99749:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 3728));
;
Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 54763));


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
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,7924,2099,8165,9691,3516,7358], () => (__webpack_exec__(99749)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-bc7ba3c1a8b1697b.js.map