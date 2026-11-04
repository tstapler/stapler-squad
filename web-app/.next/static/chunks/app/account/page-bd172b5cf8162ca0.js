(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[1298],{

/***/ 15681:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ds: () => (/* binding */ generateInvite),
/* harmony export */   Ic: () => (/* binding */ revokeCredential),
/* harmony export */   Jc: () => (/* binding */ listCredentials),
/* harmony export */   Pq: () => (/* binding */ getAuthStatus),
/* harmony export */   eA: () => (/* binding */ registerPasskey),
/* harmony export */   ip: () => (/* binding */ loginWithPasskey),
/* harmony export */   ri: () => (/* binding */ logout)
/* harmony export */ });
/* harmony import */ var _simplewebauthn_browser__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(84000);
/* provided dependency */ var process = __webpack_require__(36843);
/**
 * Passkey authentication client utilities.
 * Uses @simplewebauthn/browser to handle the browser WebAuthn ceremony.
 */ 
/** Returns the /auth base URL using the current origin. */ function authBase() {
    // Explicit override always wins, even in the browser (e.g. `next dev`
    // running on its own port against a separately-ported backend).
    const apiUrl = process.env.NEXT_PUBLIC_API_URL;
    if (apiUrl) {
        const origin = apiUrl.replace(/\/api\/?$/, "");
        return origin + "/auth";
    }
    if (true) {
        return window.location.origin + "/auth";
    }
    return "http://localhost:8543/auth";
}
/** Fetch the current auth status from the server. */ async function getAuthStatus() {
    const resp = await fetch("".concat(authBase(), "/status"), {
        credentials: "include"
    });
    if (!resp.ok) {
        throw new Error("auth status failed: ".concat(resp.statusText));
    }
    return resp.json();
}
/**
 * Register a new passkey.
 * If a setupToken is provided it is passed as a query param (first-device bootstrap).
 */ async function registerPasskey(setupToken) {
    const base = authBase();
    // 1. Begin registration – server returns ceremony key + WebAuthn options
    const beginParams = setupToken ? "?setup_token=".concat(setupToken) : "";
    const beginResp = await fetch("".concat(base, "/register/begin").concat(beginParams), {
        method: "POST",
        credentials: "include"
    });
    if (!beginResp.ok) {
        const text = await beginResp.text();
        throw new Error("begin registration failed: ".concat(text));
    }
    const { ceremony_key, options } = await beginResp.json();
    // go-webauthn wraps options as { publicKey: {...} } per the W3C spec.
    // @simplewebauthn/browser expects the flat inner PublicKeyCredentialCreationOptionsJSON.
    const credential = await (0,_simplewebauthn_browser__WEBPACK_IMPORTED_MODULE_0__/* .startRegistration */ .Ct)({
        optionsJSON: options.publicKey
    });
    // 3. Finish registration – send credential back to server
    const finishResp = await fetch("".concat(base, "/register/finish?ceremony_key=").concat(ceremony_key).concat(setupToken ? "&setup_token=".concat(setupToken) : ""), {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify(credential),
        credentials: "include"
    });
    if (!finishResp.ok) {
        const text = await finishResp.text();
        throw new Error("finish registration failed: ".concat(text));
    }
}
/** Authenticate with an existing passkey. */ async function loginWithPasskey() {
    const base = authBase();
    // 1. Begin login
    const beginResp = await fetch("".concat(base, "/login/begin"), {
        method: "POST",
        credentials: "include"
    });
    if (!beginResp.ok) {
        const text = await beginResp.text();
        throw new Error("begin login failed: ".concat(text));
    }
    const { ceremony_key, options } = await beginResp.json();
    // go-webauthn wraps options as { publicKey: {...} } per the W3C spec.
    // @simplewebauthn/browser expects the flat inner PublicKeyCredentialRequestOptionsJSON.
    const credential = await (0,_simplewebauthn_browser__WEBPACK_IMPORTED_MODULE_0__/* .startAuthentication */ .HS)({
        optionsJSON: options.publicKey
    });
    // 3. Finish login
    const finishResp = await fetch("".concat(base, "/login/finish?ceremony_key=").concat(ceremony_key), {
        method: "POST",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify(credential),
        credentials: "include"
    });
    if (!finishResp.ok) {
        const text = await finishResp.text();
        throw new Error("finish login failed: ".concat(text));
    }
}
/** Log out (revoke the current session cookie). */ async function logout() {
    await fetch("".concat(authBase(), "/logout"), {
        method: "POST",
        credentials: "include"
    });
}
/** Generate a new one-time invite (requires authenticated session). */ async function generateInvite(label) {
    const resp = await fetch("".concat(authBase(), "/invite/generate"), {
        method: "POST",
        credentials: "include",
        headers: {
            "Content-Type": "application/json"
        },
        body: JSON.stringify({
            label
        })
    });
    if (!resp.ok) {
        const text = await resp.text();
        throw new Error("generate invite failed: ".concat(text));
    }
    return resp.json();
}
/** List all registered passkeys (requires authenticated session). */ async function listCredentials() {
    const resp = await fetch("".concat(authBase(), "/credentials"), {
        credentials: "include"
    });
    if (!resp.ok) {
        const text = await resp.text();
        throw new Error("list credentials failed: ".concat(text));
    }
    const data = await resp.json();
    var _data_credentials;
    return (_data_credentials = data.credentials) !== null && _data_credentials !== void 0 ? _data_credentials : [];
}
/** Revoke a passkey by its hex ID (requires authenticated session). */ async function revokeCredential(id) {
    const resp = await fetch("".concat(authBase(), "/credentials/").concat(id, "/revoke"), {
        method: "POST",
        credentials: "include"
    });
    if (!resp.ok) {
        const text = await resp.text();
        throw new Error("revoke credential failed: ".concat(text));
    }
    return resp.json();
}


/***/ }),

/***/ 20932:
/***/ ((module) => {

var __dirname = "/";
(function(){var e={229:function(e){var t=e.exports={};var r;var n;function defaultSetTimout(){throw new Error("setTimeout has not been defined")}function defaultClearTimeout(){throw new Error("clearTimeout has not been defined")}(function(){try{if(typeof setTimeout==="function"){r=setTimeout}else{r=defaultSetTimout}}catch(e){r=defaultSetTimout}try{if(typeof clearTimeout==="function"){n=clearTimeout}else{n=defaultClearTimeout}}catch(e){n=defaultClearTimeout}})();function runTimeout(e){if(r===setTimeout){return setTimeout(e,0)}if((r===defaultSetTimout||!r)&&setTimeout){r=setTimeout;return setTimeout(e,0)}try{return r(e,0)}catch(t){try{return r.call(null,e,0)}catch(t){return r.call(this,e,0)}}}function runClearTimeout(e){if(n===clearTimeout){return clearTimeout(e)}if((n===defaultClearTimeout||!n)&&clearTimeout){n=clearTimeout;return clearTimeout(e)}try{return n(e)}catch(t){try{return n.call(null,e)}catch(t){return n.call(this,e)}}}var i=[];var o=false;var u;var a=-1;function cleanUpNextTick(){if(!o||!u){return}o=false;if(u.length){i=u.concat(i)}else{a=-1}if(i.length){drainQueue()}}function drainQueue(){if(o){return}var e=runTimeout(cleanUpNextTick);o=true;var t=i.length;while(t){u=i;i=[];while(++a<t){if(u){u[a].run()}}a=-1;t=i.length}u=null;o=false;runClearTimeout(e)}t.nextTick=function(e){var t=new Array(arguments.length-1);if(arguments.length>1){for(var r=1;r<arguments.length;r++){t[r-1]=arguments[r]}}i.push(new Item(e,t));if(i.length===1&&!o){runTimeout(drainQueue)}};function Item(e,t){this.fun=e;this.array=t}Item.prototype.run=function(){this.fun.apply(null,this.array)};t.title="browser";t.browser=true;t.env={};t.argv=[];t.version="";t.versions={};function noop(){}t.on=noop;t.addListener=noop;t.once=noop;t.off=noop;t.removeListener=noop;t.removeAllListeners=noop;t.emit=noop;t.prependListener=noop;t.prependOnceListener=noop;t.listeners=function(e){return[]};t.binding=function(e){throw new Error("process.binding is not supported")};t.cwd=function(){return"/"};t.chdir=function(e){throw new Error("process.chdir is not supported")};t.umask=function(){return 0}}};var t={};function __nccwpck_require__(r){var n=t[r];if(n!==undefined){return n.exports}var i=t[r]={exports:{}};var o=true;try{e[r](i,i.exports,__nccwpck_require__);o=false}finally{if(o)delete t[r]}return i.exports}if(typeof __nccwpck_require__!=="undefined")__nccwpck_require__.ab=__dirname+"/";var r=__nccwpck_require__(229);module.exports=r})();

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

/***/ 29944:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 36843:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";

var _global_process, _global_process1;
module.exports = ((_global_process = __webpack_require__.g.process) == null ? void 0 : _global_process.env) && typeof ((_global_process1 = __webpack_require__.g.process) == null ? void 0 : _global_process1.env) === 'object' ? __webpack_require__.g.process : __webpack_require__(20932);

//# sourceMappingURL=process.js.map

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

/***/ 42910:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ AccountPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/lib/contexts/AuthContext.tsx
var AuthContext = __webpack_require__(64792);
// EXTERNAL MODULE: ./src/lib/routes.ts + 1 modules
var routes = __webpack_require__(76514);
// EXTERNAL MODULE: ./src/lib/auth/passkey.ts
var passkey = __webpack_require__(15681);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Faccount%2Faccount.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71XyXLjIBS85yu4TFV8kEso8UY-JoUFskkQaADHy9T8-5SQZAvJaPEyJ1WxPB7d_R6t6SdMYh3NcQj-vACQ4kOwZ8RsEViEYXb4sGNqwwQCIcA7I_ORDBPCxAaBH6xeg-ATrugpUqe3t0lzBE7y9YTpjOMjAgmnNmT-DQhTNDZMCgRiyXepyGc2OLsS9uPl78u0yhTaTO-MCd2YkY2ZSGECzU60tZzYe9j5PWWbrUFgHob5WCy5VM314cQBroZZsJbGyLS5ITrZHWupCFXnNTA7AC05I43V1E3-7QGA2ARqMd-vx8ScbUTADE01AjEVhqp8-GunDUuOQSyFocIgoDMc02BNzZ5S0SWZ0COZNY6_N0ruBGluWdSQ6obojKfChO106-wi0gB1zB4B8LHQBBNVgYXOGfMeBcZtBc56FSh_qEq43COwZYQUTBh6MMFlgnLOMs10PrXfMkMDyx0CQu4VzpwcFz054ok3n8hFdGkjeUQRnUaLIvWfPHP0IqSgQ5Xhv-ja3w52Suc5ZJJVxWEUFpoVEpEZjpk5gnAKZ9pFBG1zUpCQ5hURpvGaUzKxMJWbEAiny1ljU7WyufB9Vs9GSBNgzuWeEmf_aiQNYYsGe7kMKyqMlwEYDipZZ9mtzGB_mXQzgzm_wsrKz4pfjLDRPlZ3soT_D0sP7qujqucejnDBkQWpoxf6-Vq6dK0LuGV1asIOOSEAMKGpKZ9zf7RCgaeACUIPCMCouNrNT-llyveGNuQW98hz2AP6NYDo5aQrrcjOlo8dDMNfH67BfF-WBvOxVo482crVjqLXfcG1JIswLXLtTalw6z15gsHdjE3Vkac9WhusjBN0W0BtJ7eKie-yOCrOpzNF03xgWwJ9GekW1ure537UA7F4UIXWkGFnZBCA92u86c6_ejTe4U_Gavz7Ps8HAGeCBjUB-I_iwzUKuzVawpsbWARaNjYdW18eIfhcfu0oYY-q6iEqG94ZjWqguxrmY17mzrZfy0zey6v9j7DIXK2AbDCZ5b_vVZBrAX83S-qm3OGlMhKcMn5sZbN72J-T37Mdm56t-v_3e0o8SATfA17tuSsE9WQhaL8xKx1_u0_Vtpvb0zv0YHocBOlhIKTjHXq7M9buvRvU4kfS0mXWegzEz-08wD4DDuGTMK7lv7f5F01GU540TM0_TSdpPAgVAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Faccount_2Faccount_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71XyXLjIBS85yu4TFV8kEso8UY_JoUFskkQaADHy9T8_5SQZAvJaPEyJ1WxPB7d_R6t6SdMYh3NcQj_vACQ4kOwZ8RsEViEYXb4sGNqwwQCIcA7I_ORDBPCxAaBH6xeg_ATrugpUqe3t0lzBE7y9YTpjOMjAgmnNmT_DQhTNDZMCgRiyXepyGc2OLsS9uPl78u0yhTaTO_MCd2YkY2ZSGECzU60tZzYe9j5PWWbrUFgHob5WCy5VM314cQBroZZsJbGyLS5ITrZHWupCFXnNTA7AC05I43V1E3_7QGA2ARqMd_vx8ScbUTADE01AjEVhqp8_GunDUuOQSyFocIgoDMc02BNzZ5S0SWZ0COZNY6_N0ruBGluWdSQ6obojKfChO106_wi0gB1zB4B8LHQBBNVgYXOGfMeBcZtBc56FSh_qEq43COwZYQUTBh6MMFlgnLOMs10PrXfMkMDyx0CQu4VzpwcFz054ok3n8hFdGkjeUQRnUaLIvWfPHP0IqSgQ5Xhv_ja3w52Suc5ZJJVxWEUFpoVEpEZjpk5gnAKZ9pFBG1zUpCQ5hURpvGaUzKxMJWbEAiny1ljU7WyufB9Vs9GSBNgzuWeEmf_aiQNYYsGe7kMKyqMlwEYDipZZ9mtzGB_mXQzgzm_wsrKz4pfjLDRPlZ3soT_D0sP7qujqucejnDBkQWpoxf6_Vq6dK0LuGV1asIOOSEAMKGpKZ9zf7RCgaeACUIPCMCouNrNT_llyveGNuQW98hz2AP6NYDo5aQrrcjOlo8dDMNfH67BfF_WBvOxVo482crVjqLXfcG1JIswLXLtTalw6z15gsHdjE3Vkac9WhusjBN0W0BtJ7eKie_yOCrOpzNF03xgWwJ9GekW1ure537UA7F4UIXWkGFnZBCA92u86c6_ejTe4U_Gavz7Ps8HAGeCBjUB_I_iwzUKuzVawpsbWARaNjYdW18eIfhcfu0oYY_q6iEqG94ZjWqguxrmY17mzrZfy0zey6v9j7DIXK2AbDCZ5b_vVZBrAX83S_qm3OGlMhKcMn5sZbN72J_T37Mdm56t_v_3e0o8SATfA17tuSsE9WQhaL8xKx1_u0_Vtpvb0zv0YHocBOlhIKTjHXq7M9buvRvU4kfS0mXWegzEz_08wD4DDuGTMK7lv7f5F01GU540TM0_TSdpPAgVAAA_22_7D = __webpack_require__(29944);
;// ./src/app/account/account.css.ts


var countdown = '_1fcs26ar';
var countdownExpired = '_1fcs26as';
var credentialInfo = '_1fcs26a5';
var credentialList = '_1fcs26a3';
var credentialMeta = '_1fcs26a7';
var credentialName = '_1fcs26a6';
var credentialRow = '_1fcs26a4';
var dangerButton = '_1fcs26a9';
var emptyState = '_1fcs26au';
var errorText = '_1fcs26av';
var ghostButton = '_1fcs26aa';
var modal = '_1fcs26ac';
var modalActions = '_1fcs26ae';
var modalOverlay = '_1fcs26ab';
var modalTitle = '_1fcs26ad';
var page = '_1fcs26a0';
var primaryButton = '_1fcs26a8';
var qrBlock = '_1fcs26am';
var qrImage = '_1fcs26an';
var qrLabel = '_1fcs26ao';
var qrRow = '_1fcs26al';
var section = '_1fcs26a1';
var sectionTitle = '_1fcs26a2';
var step = '_1fcs26ag';
var stepContent = '_1fcs26ai';
var stepDesc = '_1fcs26ak';
var stepList = '_1fcs26af';
var stepNumber = '_1fcs26ah';
var stepTitle = '_1fcs26aj';
var urlRow = '_1fcs26ap';
var urlShowButton = '_1fcs26aw';
var urlText = '_1fcs26aq';
var warningText = '_1fcs26at';
;// ./src/app/account/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 







// ─── Countdown ────────────────────────────────────────────────────────────────
function Countdown(param) {
    let { ttlSeconds } = param;
    const [left, setLeft] = (0,react.useState)(ttlSeconds);
    (0,react.useEffect)(()=>{
        if (left <= 0) return;
        const id = setInterval(()=>{
            setLeft((n)=>{
                if (n <= 1) {
                    clearInterval(id);
                    return 0;
                }
                return n - 1;
            });
        }, 1000);
        return ()=>clearInterval(id);
    }, []); // eslint-disable-line react-hooks/exhaustive-deps
    if (left <= 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
            className: "".concat(countdown, " ").concat(countdownExpired),
            children: "Invite expired — generate a new one"
        });
    }
    const m = Math.floor(left / 60);
    const sec = left % 60;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
        className: countdown,
        children: [
            "Expires in ",
            m,
            ":",
            String(sec).padStart(2, "0")
        ]
    });
}
// ─── Add Device Modal ─────────────────────────────────────────────────────────
function AddDeviceModal(param) {
    let { onClose, onSuccess } = param;
    const [label, setLabel] = (0,react.useState)("");
    const [invite, setInvite] = (0,react.useState)(null);
    const [showUrl, setShowUrl] = (0,react.useState)(false);
    const [loading, setLoading] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)("");
    const [copied, setCopied] = (0,react.useState)(false);
    const generate = async ()=>{
        setLoading(true);
        setError("");
        try {
            const data = await (0,passkey/* generateInvite */.Ds)(label || "New Device");
            setInvite(data);
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally{
            setLoading(false);
        }
    };
    const copyUrl = async ()=>{
        if (!invite) return;
        await navigator.clipboard.writeText(invite.registration_url);
        setCopied(true);
        setTimeout(()=>setCopied(false), 2000);
    };
    if (!invite) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            className: modalOverlay,
            onClick: (e)=>e.target === e.currentTarget && onClose(),
            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: modal,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: modalTitle,
                        children: "Add New Device"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: stepDesc,
                        children: "Give this device a name so you can identify it later (e.g. “iPhone 15”)."
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "text",
                        placeholder: "Device name (optional)",
                        "aria-label": "Device name (optional)",
                        value: label,
                        onChange: (e)=>setLabel(e.target.value),
                        onKeyDown: (e)=>e.key === "Enter" && generate(),
                        style: {
                            width: "100%",
                            padding: "0.5rem 0.75rem",
                            borderRadius: "6px",
                            border: "1px solid var(--border-color)",
                            background: "var(--input-background)",
                            color: "var(--input-text)",
                            fontSize: "0.875rem",
                            boxSizing: "border-box"
                        }
                    }),
                    error && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: errorText,
                        children: error
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: modalActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: ghostButton,
                                onClick: onClose,
                                children: "Cancel"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: primaryButton,
                                onClick: generate,
                                disabled: loading,
                                children: loading ? "Generating…" : "Generate Invite"
                            })
                        ]
                    })
                ]
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: modalOverlay,
        onClick: (e)=>e.target === e.currentTarget && onClose(),
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: modal,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: modalTitle,
                    children: "Add New Device"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("ol", {
                    className: stepList,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                            className: step,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: stepNumber,
                                    children: "1"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: stepContent,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: stepTitle,
                                            children: "Install the CA certificate on the new device"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: stepDesc,
                                            children: "Scan the left QR code to download the certificate, then install it:"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                            className: stepDesc,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                                    children: "iOS"
                                                }),
                                                ": Download → Settings → General → VPN & Device Management → [cert] → Install. Then Settings → General → About → Certificate Trust Settings → enable."
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                            className: stepDesc,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                                    children: "Android"
                                                }),
                                                ": Download → Settings → Security → Encryption & credentials → Install a certificate → CA Certificate."
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                            className: stepDesc,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                                    children: "macOS / Windows"
                                                }),
                                                ": Download and double-click the .pem file to add it to the trust store."
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: qrRow,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: qrBlock,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("img", {
                                                        src: invite.ca_qr_data_url,
                                                        alt: "CA cert QR code",
                                                        className: qrImage
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: qrLabel,
                                                        children: "CA Certificate"
                                                    })
                                                ]
                                            })
                                        })
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                            className: step,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: stepNumber,
                                    children: "2"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: stepContent,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: stepTitle,
                                            children: "Register a passkey on the new device"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: stepDesc,
                                            children: "Scan the QR code with the new device’s camera, then follow the browser prompt."
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: qrRow,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: qrBlock,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("img", {
                                                        src: invite.reg_qr_data_url,
                                                        alt: "Registration QR code",
                                                        className: qrImage
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: qrLabel,
                                                        children: "Registration"
                                                    })
                                                ]
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)(Countdown, {
                                            ttlSeconds: invite.ttl_seconds
                                        }),
                                        showUrl ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: urlRow,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: urlText,
                                                    title: invite.registration_url,
                                                    children: invite.registration_url
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: ghostButton,
                                                    onClick: copyUrl,
                                                    children: copied ? "Copied!" : "Copy"
                                                })
                                            ]
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            className: "".concat(ghostButton, " ").concat(urlShowButton),
                                            onClick: ()=>setShowUrl(true),
                                            children: "Can’t scan? Show URL"
                                        })
                                    ]
                                })
                            ]
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: modalActions,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: ghostButton,
                            onClick: ()=>{
                                setInvite(null);
                                setLabel("");
                            },
                            children: "Regenerate"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: primaryButton,
                            onClick: ()=>{
                                onSuccess();
                                onClose();
                            },
                            children: "Done"
                        })
                    ]
                })
            ]
        })
    });
}
// ─── Revoke Confirm Dialog ────────────────────────────────────────────────────
function RevokeConfirmDialog(param) {
    let { credential, isLast, onConfirm, onCancel } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: modalOverlay,
        onClick: (e)=>e.target === e.currentTarget && onCancel(),
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: modal,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: modalTitle,
                    children: "Revoke passkey?"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    className: stepDesc,
                    children: [
                        "Remove ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                            children: credential.display_name || "this passkey"
                        }),
                        "? The device will no longer be able to sign in."
                    ]
                }),
                isLast && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: warningText,
                    children: "This is your last passkey. Revoking it will sign out all active sessions and lock you out of remote access until you use the CLI to register a new passkey."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: modalActions,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: ghostButton,
                            onClick: onCancel,
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: dangerButton,
                            onClick: onConfirm,
                            children: isLast ? "Revoke and lock out" : "Revoke"
                        })
                    ]
                })
            ]
        })
    });
}
// ─── Credential Row ───────────────────────────────────────────────────────────
function CredentialRow(param) {
    let { cred, isLast, onRevoke } = param;
    const createdAt = cred.created_at ? new Date(cred.created_at).toLocaleDateString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric"
    }) : "Unknown";
    const lastUsed = cred.last_used_at ? new Date(cred.last_used_at).toLocaleDateString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric"
    }) : "Never";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: credentialRow,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: credentialInfo,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: credentialName,
                        children: cred.display_name || "Passkey"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: credentialMeta,
                        children: [
                            "Registered ",
                            createdAt,
                            " \xb7 Last used ",
                            lastUsed
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: dangerButton,
                onClick: ()=>onRevoke(cred),
                title: "Revoke this passkey",
                children: "Revoke"
            })
        ]
    });
}
// ─── Account Page ─────────────────────────────────────────────────────────────
function AccountPage() {
    (0,usePageView/* usePageView */.O)();
    const router = (0,nav/* useRouter */.KA)();
    const { authEnabled, authenticated, loading: authLoading } = (0,AuthContext/* useAuth */.A)();
    const [credentials, setCredentials] = (0,react.useState)([]);
    const [credsLoading, setCredsLoading] = (0,react.useState)(true);
    const [credsError, setCredsError] = (0,react.useState)("");
    const [showAddModal, setShowAddModal] = (0,react.useState)(false);
    const [revoking, setRevoking] = (0,react.useState)(null);
    (0,react.useEffect)(()=>{
        if (!authLoading && authEnabled && !authenticated) {
            router.replace(routes/* routes */.J.login);
        }
    }, [
        authLoading,
        authEnabled,
        authenticated,
        router
    ]);
    const loadCredentials = (0,react.useCallback)(async ()=>{
        setCredsLoading(true);
        setCredsError("");
        try {
            const data = await (0,passkey/* listCredentials */.Jc)();
            setCredentials(data);
        } catch (e) {
            setCredsError(e instanceof Error ? e.message : String(e));
        } finally{
            setCredsLoading(false);
        }
    }, []);
    (0,react.useEffect)(()=>{
        if (!authLoading && authEnabled && authenticated) {
            loadCredentials();
        }
    }, [
        authLoading,
        authEnabled,
        authenticated,
        loadCredentials
    ]);
    const handleRevoke = async (cred)=>{
        try {
            const result = await (0,passkey/* revokeCredential */.Ic)(cred.id);
            setRevoking(null);
            if (result.last_credential) {
                // Session cookie was invalidated server-side; redirect to login.
                router.replace(routes/* routes */.J.login);
                return;
            }
            await loadCredentials();
        } catch (e) {
            setCredsError(e instanceof Error ? e.message : String(e));
            setRevoking(null);
        }
    };
    if (authLoading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("main", {
            id: "main-content",
            className: page,
            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: emptyState,
                children: "Loading…"
            })
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
        id: "main-content",
        className: page,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                className: section,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: sectionTitle,
                        children: "Your Passkeys"
                    }),
                    credsLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: emptyState,
                        children: "Loading passkeys…"
                    }) : credsError ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: errorText,
                        children: credsError
                    }) : credentials.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: emptyState,
                        children: "No passkeys registered."
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: credentialList,
                        children: credentials.map((cred)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(CredentialRow, {
                                cred: cred,
                                isLast: credentials.length === 1,
                                onRevoke: (c)=>setRevoking(c)
                            }, cred.id))
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: primaryButton,
                            onClick: ()=>setShowAddModal(true),
                            children: "Add New Device"
                        })
                    })
                ]
            }),
            showAddModal && /*#__PURE__*/ (0,jsx_runtime.jsx)(AddDeviceModal, {
                onClose: ()=>setShowAddModal(false),
                onSuccess: loadCredentials
            }),
            revoking && /*#__PURE__*/ (0,jsx_runtime.jsx)(RevokeConfirmDialog, {
                credential: revoking,
                isLast: credentials.length === 1,
                onConfirm: ()=>handleRevoke(revoking),
                onCancel: ()=>setRevoking(null)
            })
        ]
    });
}


/***/ }),

/***/ 64792:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   A: () => (/* binding */ useAuth),
/* harmony export */   AuthProvider: () => (/* binding */ AuthProvider)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_auth_passkey__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(15681);
/* __next_internal_client_entry_do_not_use__ AuthProvider,useAuth auto */ 


const AuthContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
function AuthProvider(param) {
    let { children } = param;
    const [state, setState] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)({
        authEnabled: false,
        authenticated: false,
        hasCredentials: false,
        setupActive: false,
        loading: true
    });
    const refresh = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async ()=>{
        try {
            const status = await (0,_lib_auth_passkey__WEBPACK_IMPORTED_MODULE_2__/* .getAuthStatus */ .Pq)();
            setState({
                authEnabled: status.auth_enabled,
                authenticated: status.authenticated,
                hasCredentials: status.has_credentials,
                setupActive: status.setup_active,
                loading: false
            });
        } catch (e) {
            // Server unreachable or auth not configured – treat as not-authenticated
            setState((prev)=>({
                    ...prev,
                    loading: false,
                    authEnabled: false
                }));
        }
    }, []);
    const logout = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(async ()=>{
        await (0,_lib_auth_passkey__WEBPACK_IMPORTED_MODULE_2__/* .logout */ .ri)();
        await refresh();
    }, [
        refresh
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(AuthContext.Provider, {
        value: {
            ...state,
            refresh,
            logout
        },
        children: children
    });
}
function useAuth() {
    const ctx = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(AuthContext);
    if (!ctx) {
        throw new Error("useAuth must be used within AuthProvider");
    }
    return ctx;
}


/***/ }),

/***/ 65916:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 76514:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  J: () => (/* reexport */ routes)
});

;// ./src/lib/nav/paths.ts
/**
 * Centralized route definitions for type-safe navigation (re-exported from `@/lib/routes`)
 */ const routes = {
    home: "/",
    sessionCreate: "/?new=true",
    reviewQueue: "/review-queue",
    unfinished: "/unfinished",
    rules: "/rules",
    history: "/history",
    logs: "/logs",
    errors: "/errors",
    config: "/config",
    notifications: "/notifications",
    settings: "/settings",
    settingsDefaults: "/settings/defaults",
    help: "/help",
    changelog: "/changelog",
    settingsUnfinished: "/settings/unfinished",
    insights: "/insights",
    settingsFeatures: "/settings/features",
    settingsBacklogSources: "/settings/backlog-sources",
    settingsBacklogStages: "/settings/backlog-stages",
    settingsPipelineModes: "/settings/pipeline-modes",
    settingsRemotes: "/settings/remotes",
    settingsJules: "/settings/jules",
    settingsTaggingClassifier: "/settings/tagging-classifier",
    settingsLlmBackends: "/settings/llm-backends",
    settingsBackgroundModels: "/settings/background-models",
    backlog: "/backlog",
    backlogBoard: "/backlog/board",
    sessionsImport: "/sessions/import",
    workflows: "/workflows",
    triggers: "/triggers",
    login: "/login",
    account: "/account",
    escapeAnalytics: "/analytics/escape",
    files: "/files",
    sessionDetail: (id)=>"/?session=".concat(id),
    newSessionFromWorktree: (worktreePath, branch, title)=>{
        const params = new URLSearchParams({
            worktree: worktreePath,
            branch
        });
        if (title) params.set("title", title);
        return "/?".concat(params.toString());
    },
    unfinishedItem: (itemId)=>"/unfinished?item=".concat(encodeURIComponent(itemId)),
    unfinishedTab: (tab)=>"/unfinished?tab=".concat(encodeURIComponent(tab))
};
/** Every `page.tsx` under src/app, one literal each; `route-meta.test.ts` fails when this list drifts from the directory. */ const ROUTE_PATHS = (/* unused pure expression or super */ null && ([
    "/",
    "/account",
    "/analytics/escape",
    "/backlog",
    "/backlog/board",
    "/changelog",
    "/config",
    "/debug/escape-codes",
    "/errors",
    "/fanout",
    "/files",
    "/help",
    "/history",
    "/insights",
    "/insights/session-detail",
    "/login",
    "/logs",
    "/notifications",
    "/resolve",
    "/review-queue",
    "/rules",
    "/sessions/import",
    "/sessions/new",
    "/sessions/summary",
    "/settings",
    "/settings/background-models",
    "/settings/backlog-sources",
    "/settings/backlog-stages",
    "/settings/defaults",
    "/settings/features",
    "/settings/jules",
    "/settings/llm-backends",
    "/settings/pipeline-modes",
    "/settings/remotes",
    "/settings/tagging-classifier",
    "/settings/unfinished",
    "/spike-files",
    "/spike-lazydoc",
    "/test/escape-codes",
    "/test/layout-overlap",
    "/test/terminal-stress",
    "/triggers",
    "/unfinished",
    "/workflows"
]));

;// ./src/lib/routes.ts
/**
 * Centralized route definitions for type-safe navigation.
 * The data lives in `@/lib/nav/paths`; this path stays for existing importers.
 */ 


/***/ }),

/***/ 78279:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 42910));


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
/******/ __webpack_require__.O(0, [1869,852,4000,9691,3516,7358], () => (__webpack_exec__(78279)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-bd172b5cf8162ca0.js.map