(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[286,852],{

/***/ 801:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 1442:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 2417:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 2928:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 3423:
/***/ ((__unused_webpack_module, exports, __webpack_require__) => {

"use strict";
// Format function modified from nodejs
// Copyright Joyent, Inc. and other Node contributors.
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to permit
// persons to whom the Software is furnished to do so, subject to the
// following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN
// NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
// DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
// OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE
// USE OR OTHER DEALINGS IN THE SOFTWARE.

Object.defineProperty(exports, "__esModule", ({
    value: true
}));
0 && (0);
function _export(target, all) {
    for(var name in all)Object.defineProperty(target, name, {
        enumerable: true,
        get: all[name]
    });
}
_export(exports, {
    formatUrl: function() {
        return formatUrl;
    },
    formatWithValidation: function() {
        return formatWithValidation;
    },
    urlObjectKeys: function() {
        return urlObjectKeys;
    }
});
const _interop_require_wildcard = __webpack_require__(15755);
const _querystring = /*#__PURE__*/ _interop_require_wildcard._(__webpack_require__(56829));
const slashedProtocols = /https?|ftp|gopher|file/;
function formatUrl(urlObj) {
    let { auth, hostname } = urlObj;
    let protocol = urlObj.protocol || '';
    let pathname = urlObj.pathname || '';
    let hash = urlObj.hash || '';
    let query = urlObj.query || '';
    let host = false;
    auth = auth ? encodeURIComponent(auth).replace(/%3A/i, ':') + '@' : '';
    if (urlObj.host) {
        host = auth + urlObj.host;
    } else if (hostname) {
        host = auth + (~hostname.indexOf(':') ? "[" + hostname + "]" : hostname);
        if (urlObj.port) {
            host += ':' + urlObj.port;
        }
    }
    if (query && typeof query === 'object') {
        query = String(_querystring.urlQueryToSearchParams(query));
    }
    let search = urlObj.search || query && "?" + query || '';
    if (protocol && !protocol.endsWith(':')) protocol += ':';
    if (urlObj.slashes || (!protocol || slashedProtocols.test(protocol)) && host !== false) {
        host = '//' + (host || '');
        if (pathname && pathname[0] !== '/') pathname = '/' + pathname;
    } else if (!host) {
        host = '';
    }
    if (hash && hash[0] !== '#') hash = '#' + hash;
    if (search && search[0] !== '?') search = '?' + search;
    pathname = pathname.replace(/[?#]/g, encodeURIComponent);
    search = search.replace('#', '%23');
    return "" + protocol + host + pathname + search + hash;
}
const urlObjectKeys = [
    'auth',
    'hash',
    'host',
    'hostname',
    'href',
    'path',
    'pathname',
    'port',
    'protocol',
    'query',
    'search',
    'slashes'
];
function formatWithValidation(url) {
    if (false) {}
    return formatUrl(url);
} //# sourceMappingURL=format-url.js.map


/***/ }),

/***/ 5365:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 5816:
/***/ ((module, exports, __webpack_require__) => {

"use strict";

Object.defineProperty(exports, "__esModule", ({
    value: true
}));
Object.defineProperty(exports, "useMergedRef", ({
    enumerable: true,
    get: function() {
        return useMergedRef;
    }
}));
const _react = __webpack_require__(55494);
function useMergedRef(refA, refB) {
    const cleanupA = (0, _react.useRef)(null);
    const cleanupB = (0, _react.useRef)(null);
    // NOTE: In theory, we could skip the wrapping if only one of the refs is non-null.
    // (this happens often if the user doesn't pass a ref to Link/Form/Image)
    // But this can cause us to leak a cleanup-ref into user code (e.g. via `<Link legacyBehavior>`),
    // and the user might pass that ref into ref-merging library that doesn't support cleanup refs
    // (because it hasn't been updated for React 19)
    // which can then cause things to blow up, because a cleanup-returning ref gets called with `null`.
    // So in practice, it's safer to be defensive and always wrap the ref, even on React 19.
    return (0, _react.useCallback)((current)=>{
        if (current === null) {
            const cleanupFnA = cleanupA.current;
            if (cleanupFnA) {
                cleanupA.current = null;
                cleanupFnA();
            }
            const cleanupFnB = cleanupB.current;
            if (cleanupFnB) {
                cleanupB.current = null;
                cleanupFnB();
            }
        } else {
            if (refA) {
                cleanupA.current = applyRef(refA, current);
            }
            if (refB) {
                cleanupB.current = applyRef(refB, current);
            }
        }
    }, [
        refA,
        refB
    ]);
}
function applyRef(refA, current) {
    if (typeof refA === 'function') {
        const cleanup = refA(current);
        if (typeof cleanup === 'function') {
            return cleanup;
        } else {
            return ()=>refA(null);
        }
    } else {
        refA.current = current;
        return ()=>{
            refA.current = null;
        };
    }
}
if ((typeof exports.default === 'function' || typeof exports.default === 'object' && exports.default !== null) && typeof exports.default.__esModule === 'undefined') {
    Object.defineProperty(exports.default, '__esModule', {
        value: true
    });
    Object.assign(exports.default, exports);
    module.exports = exports.default;
} //# sourceMappingURL=use-merged-ref.js.map


/***/ }),

/***/ 6024:
/***/ ((module, exports, __webpack_require__) => {

"use strict";
/* __next_internal_client_entry_do_not_use__  cjs */ 
Object.defineProperty(exports, "__esModule", ({
    value: true
}));
0 && (0);
function _export(target, all) {
    for(var name in all)Object.defineProperty(target, name, {
        enumerable: true,
        get: all[name]
    });
}
_export(exports, {
    /**
 * A React component that extends the HTML `<a>` element to provide
 * [prefetching](https://nextjs.org/docs/app/building-your-application/routing/linking-and-navigating#2-prefetching)
 * and client-side navigation. This is the primary way to navigate between routes in Next.js.
 *
 * @remarks
 * - Prefetching is only enabled in production.
 *
 * @see https://nextjs.org/docs/app/api-reference/components/link
 */ default: function() {
        return LinkComponent;
    },
    useLinkStatus: function() {
        return useLinkStatus;
    }
});
const _interop_require_wildcard = __webpack_require__(15755);
const _jsxruntime = __webpack_require__(61574);
const _react = /*#__PURE__*/ _interop_require_wildcard._(__webpack_require__(55494));
const _formaturl = __webpack_require__(3423);
const _approutercontextsharedruntime = __webpack_require__(30897);
const _routerreducertypes = __webpack_require__(93956);
const _usemergedref = __webpack_require__(5816);
const _utils = __webpack_require__(82901);
const _addbasepath = __webpack_require__(69331);
const _warnonce = __webpack_require__(82456);
const _links = __webpack_require__(66248);
const _islocalurl = __webpack_require__(30834);
const _approuterinstance = __webpack_require__(30968);
const _erroronce = __webpack_require__(93550);
function isModifiedEvent(event) {
    const eventTarget = event.currentTarget;
    const target = eventTarget.getAttribute('target');
    return target && target !== '_self' || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || // triggers resource download
    event.nativeEvent && event.nativeEvent.which === 2;
}
function linkClicked(e, href, as, linkInstanceRef, replace, scroll, onNavigate) {
    const { nodeName } = e.currentTarget;
    // anchors inside an svg have a lowercase nodeName
    const isAnchorNodeName = nodeName.toUpperCase() === 'A';
    if (isAnchorNodeName && isModifiedEvent(e) || e.currentTarget.hasAttribute('download')) {
        // ignore click for browser’s default behavior
        return;
    }
    if (!(0, _islocalurl.isLocalURL)(href)) {
        if (replace) {
            // browser default behavior does not replace the history state
            // so we need to do it manually
            e.preventDefault();
            location.replace(href);
        }
        // ignore click for browser’s default behavior
        return;
    }
    e.preventDefault();
    const navigate = ()=>{
        if (onNavigate) {
            let isDefaultPrevented = false;
            onNavigate({
                preventDefault: ()=>{
                    isDefaultPrevented = true;
                }
            });
            if (isDefaultPrevented) {
                return;
            }
        }
        (0, _approuterinstance.dispatchNavigateAction)(as || href, replace ? 'replace' : 'push', scroll != null ? scroll : true, linkInstanceRef.current);
    };
    _react.default.startTransition(navigate);
}
function formatStringOrUrl(urlObjOrString) {
    if (typeof urlObjOrString === 'string') {
        return urlObjOrString;
    }
    return (0, _formaturl.formatUrl)(urlObjOrString);
}
function LinkComponent(props) {
    const [linkStatus, setOptimisticLinkStatus] = (0, _react.useOptimistic)(_links.IDLE_LINK_STATUS);
    let children;
    const linkInstanceRef = (0, _react.useRef)(null);
    const { href: hrefProp, as: asProp, children: childrenProp, prefetch: prefetchProp = null, passHref, replace, shallow, scroll, onClick, onMouseEnter: onMouseEnterProp, onTouchStart: onTouchStartProp, legacyBehavior = false, onNavigate, ref: forwardedRef, unstable_dynamicOnHover, ...restProps } = props;
    children = childrenProp;
    if (legacyBehavior && (typeof children === 'string' || typeof children === 'number')) {
        children = /*#__PURE__*/ (0, _jsxruntime.jsx)("a", {
            children: children
        });
    }
    const router = _react.default.useContext(_approutercontextsharedruntime.AppRouterContext);
    const prefetchEnabled = prefetchProp !== false;
    /**
   * The possible states for prefetch are:
   * - null: this is the default "auto" mode, where we will prefetch partially if the link is in the viewport
   * - true: we will prefetch if the link is visible and prefetch the full page, not just partially
   * - false: we will not prefetch if in the viewport at all
   * - 'unstable_dynamicOnHover': this starts in "auto" mode, but switches to "full" when the link is hovered
   */ const appPrefetchKind = prefetchProp === null ? _routerreducertypes.PrefetchKind.AUTO : _routerreducertypes.PrefetchKind.FULL;
    if (false) {}
    if (false) {}
    const { href, as } = _react.default.useMemo(()=>{
        const resolvedHref = formatStringOrUrl(hrefProp);
        return {
            href: resolvedHref,
            as: asProp ? formatStringOrUrl(asProp) : resolvedHref
        };
    }, [
        hrefProp,
        asProp
    ]);
    // This will return the first child, if multiple are provided it will throw an error
    let child;
    if (legacyBehavior) {
        if (false) {} else {
            child = _react.default.Children.only(children);
        }
    } else {
        if (false) {}
    }
    const childRef = legacyBehavior ? child && typeof child === 'object' && child.ref : forwardedRef;
    // Use a callback ref to attach an IntersectionObserver to the anchor tag on
    // mount. In the future we will also use this to keep track of all the
    // currently mounted <Link> instances, e.g. so we can re-prefetch them after
    // a revalidation or refresh.
    const observeLinkVisibilityOnMount = _react.default.useCallback((element)=>{
        if (router !== null) {
            linkInstanceRef.current = (0, _links.mountLinkInstance)(element, href, router, appPrefetchKind, prefetchEnabled, setOptimisticLinkStatus);
        }
        return ()=>{
            if (linkInstanceRef.current) {
                (0, _links.unmountLinkForCurrentNavigation)(linkInstanceRef.current);
                linkInstanceRef.current = null;
            }
            (0, _links.unmountPrefetchableInstance)(element);
        };
    }, [
        prefetchEnabled,
        href,
        router,
        appPrefetchKind,
        setOptimisticLinkStatus
    ]);
    const mergedRef = (0, _usemergedref.useMergedRef)(observeLinkVisibilityOnMount, childRef);
    const childProps = {
        ref: mergedRef,
        onClick (e) {
            if (false) {}
            if (!legacyBehavior && typeof onClick === 'function') {
                onClick(e);
            }
            if (legacyBehavior && child.props && typeof child.props.onClick === 'function') {
                child.props.onClick(e);
            }
            if (!router) {
                return;
            }
            if (e.defaultPrevented) {
                return;
            }
            linkClicked(e, href, as, linkInstanceRef, replace, scroll, onNavigate);
        },
        onMouseEnter (e) {
            if (!legacyBehavior && typeof onMouseEnterProp === 'function') {
                onMouseEnterProp(e);
            }
            if (legacyBehavior && child.props && typeof child.props.onMouseEnter === 'function') {
                child.props.onMouseEnter(e);
            }
            if (!router) {
                return;
            }
            if (!prefetchEnabled || "production" === 'development') {
                return;
            }
            const upgradeToDynamicPrefetch = unstable_dynamicOnHover === true;
            (0, _links.onNavigationIntent)(e.currentTarget, upgradeToDynamicPrefetch);
        },
        onTouchStart:  false ? 0 : function onTouchStart(e) {
            if (!legacyBehavior && typeof onTouchStartProp === 'function') {
                onTouchStartProp(e);
            }
            if (legacyBehavior && child.props && typeof child.props.onTouchStart === 'function') {
                child.props.onTouchStart(e);
            }
            if (!router) {
                return;
            }
            if (!prefetchEnabled) {
                return;
            }
            const upgradeToDynamicPrefetch = unstable_dynamicOnHover === true;
            (0, _links.onNavigationIntent)(e.currentTarget, upgradeToDynamicPrefetch);
        }
    };
    // If child is an <a> tag and doesn't have a href attribute, or if the 'passHref' property is
    // defined, we specify the current 'href', so that repetition is not needed by the user.
    // If the url is absolute, we can bypass the logic to prepend the basePath.
    if ((0, _utils.isAbsoluteUrl)(as)) {
        childProps.href = as;
    } else if (!legacyBehavior || passHref || child.type === 'a' && !('href' in child.props)) {
        childProps.href = (0, _addbasepath.addBasePath)(as);
    }
    let link;
    if (legacyBehavior) {
        if (false) {}
        link = /*#__PURE__*/ _react.default.cloneElement(child, childProps);
    } else {
        link = /*#__PURE__*/ (0, _jsxruntime.jsx)("a", {
            ...restProps,
            ...childProps,
            children: children
        });
    }
    return /*#__PURE__*/ (0, _jsxruntime.jsx)(LinkStatusContext.Provider, {
        value: linkStatus,
        children: link
    });
}
const LinkStatusContext = /*#__PURE__*/ (0, _react.createContext)(_links.IDLE_LINK_STATUS);
const useLinkStatus = ()=>{
    return (0, _react.useContext)(LinkStatusContext);
};
if ((typeof exports.default === 'function' || typeof exports.default === 'object' && exports.default !== null) && typeof exports.default.__esModule === 'undefined') {
    Object.defineProperty(exports.default, '__esModule', {
        value: true
    });
    Object.assign(exports.default, exports);
    module.exports = exports.default;
} //# sourceMappingURL=link.js.map


/***/ }),

/***/ 10301:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 10307:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 10431:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 10782:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 13744:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 14871:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 17070:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 20982:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 23401:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 24058:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 24589:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 25418:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 26628:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 27122:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 30375:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 30834:
/***/ ((__unused_webpack_module, exports, __webpack_require__) => {

"use strict";

Object.defineProperty(exports, "__esModule", ({
    value: true
}));
Object.defineProperty(exports, "isLocalURL", ({
    enumerable: true,
    get: function() {
        return isLocalURL;
    }
}));
const _utils = __webpack_require__(82901);
const _hasbasepath = __webpack_require__(49540);
function isLocalURL(url) {
    // prevent a hydration mismatch on href for url with anchor refs
    if (!(0, _utils.isAbsoluteUrl)(url)) return true;
    try {
        // absolute urls can be local if they are on the same origin
        const locationOrigin = (0, _utils.getLocationOrigin)();
        const resolved = new URL(url, locationOrigin);
        return resolved.origin === locationOrigin && (0, _hasbasepath.hasBasePath)(resolved.pathname);
    } catch (_) {
        return false;
    }
} //# sourceMappingURL=is-local-url.js.map


/***/ }),

/***/ 34819:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 37341:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 40679:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 79426));


/***/ }),

/***/ 43702:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 45853:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _client_components_navigation__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(7113);
/* harmony import */ var _client_components_navigation__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_client_components_navigation__WEBPACK_IMPORTED_MODULE_0__);
/* harmony reexport (checked) */ if(__webpack_require__.o(_client_components_navigation__WEBPACK_IMPORTED_MODULE_0__, "redirect")) __webpack_require__.d(__webpack_exports__, { redirect: function() { return _client_components_navigation__WEBPACK_IMPORTED_MODULE_0__.redirect; } });
/* harmony reexport (checked) */ if(__webpack_require__.o(_client_components_navigation__WEBPACK_IMPORTED_MODULE_0__, "usePathname")) __webpack_require__.d(__webpack_exports__, { usePathname: function() { return _client_components_navigation__WEBPACK_IMPORTED_MODULE_0__.usePathname; } });
/* harmony reexport (checked) */ if(__webpack_require__.o(_client_components_navigation__WEBPACK_IMPORTED_MODULE_0__, "useRouter")) __webpack_require__.d(__webpack_exports__, { useRouter: function() { return _client_components_navigation__WEBPACK_IMPORTED_MODULE_0__.useRouter; } });
/* harmony reexport (checked) */ if(__webpack_require__.o(_client_components_navigation__WEBPACK_IMPORTED_MODULE_0__, "useSearchParams")) __webpack_require__.d(__webpack_exports__, { useSearchParams: function() { return _client_components_navigation__WEBPACK_IMPORTED_MODULE_0__.useSearchParams; } });


//# sourceMappingURL=navigation.js.map

/***/ }),

/***/ 45916:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 46377:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 49413:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 50707:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 51213:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   EN: () => (/* binding */ selectAllButton),
/* harmony export */   OR: () => (/* binding */ danger),
/* harmony export */   U9: () => (/* binding */ count),
/* harmony export */   hP: () => (/* binding */ actionButton),
/* harmony export */   kL: () => (/* binding */ container),
/* harmony export */   mp: () => (/* binding */ feedback),
/* harmony export */   mt: () => (/* binding */ clearButton),
/* harmony export */   o1: () => (/* binding */ actions),
/* harmony export */   r1: () => (/* binding */ selection)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FBulkActions_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_WVzY7bIBDH7_sUc6kUS4uF8_Fk6aWv0VNFzMSmtQEBiZ2t9t0rY2eLs86mW6mnSr6Y_eA_PwYmPbQ7WXsKPx8AhHSm5mcGhxq7zw8AvJalItJj4xgUqDzafvn70Xl5OJNCK4_KM3CGF0j26FtE1XsYLoRUJYPMYgNZurHY9Ot7XvworT4qweDE7YKQb9kTPi_t8zYJdm0FWmK5kEfHYGe6YbUjruJCtwwoZKaDlemAgi33fEEfYfzSLHkc7ctZO83DJg23pVRkr73XzSCxXy65ufy8PKQDl_xDXOYyLEOGg1aetCjLyjPIKe29C11re42BBoXB3clnZEDTp9V2M825CjlfGdOA97WOgSELHJyupbjaAudI5yPp6Hy85coZblH5m3Kzt3J328thT4rejEUfrevTGC0v1MI_0kutGPC6BpouXVwsq_QJbSj5dvsUcVGzUqskSrr_nwiu_wXBzfzNCFeAptcdm1_zzpcD8XS5mUB_g3M8CqUV3iIyf1s_QOo9LE0SHV1bSY9xWQNZprRfMCEd39cokjugTfKq5KBtM7ZJzT1_XZDMdMnbJ29tuh4I9OYbr17_4q1vuYSHL1LNCy9POCd7Xhadhl9CQoQ2vJD_3LNfx9SV9oTXtW5RRMHbO3CyyVbbv_GbreIUu_D9Z1dval7t56YFTS_t_aVBITm4wiIq4ErAouEdaaXwFYNtvjPdIDWNZyyEq0KEtFgMfVjo_tiEqXk1WJy36ItqsEwmar_08jtzNma_P5ejoM1dOZFzPjqPxWWUfho8Xn4B_UM6wkEIAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(83643);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FBulkActions_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_WVzY7bIBDH7_sUc6kUS4uF8_Fk6aWv0VNFzMSmtQEBiZ2t9t0rY2eLs86mW6mnSr6Y_eA_PwYmPbQ7WXsKPx8AhHSm5mcGhxq7zw8AvJalItJj4xgUqDzafvn70Xl5OJNCK4_KM3CGF0j26FtE1XsYLoRUJYPMYgNZurHY9Ot7XvworT4qweDE7YKQb9kTPi_t8zYJdm0FWmK5kEfHYGe6YbUjruJCtwwoZKaDlemAgi33fEEfYfzSLHkc7ctZO83DJg23pVRkr73XzSCxXy65ufy8PKQDl_xDXOYyLEOGg1aetCjLyjPIKe29C11re42BBoXB3clnZEDTp9V2M825CjlfGdOA97WOgSELHJyupbjaAudI5yPp6Hy85coZblH5m3Kzt3J328thT4rejEUfrevTGC0v1MI_0kutGPC6BpouXVwsq_QJbSj5dvsUcVGzUqskSrr_nwiu_wXBzfzNCFeAptcdm1_zzpcD8XS5mUB_g3M8CqUV3iIyf1s_QOo9LE0SHV1bSY9xWQNZprRfMCEd39cokjugTfKq5KBtM7ZJzT1_XZDMdMnbJ29tuh4I9OYbr17_4q1vuYSHL1LNCy9POCd7Xhadhl9CQoQ2vJD_3LNfx9SV9oTXtW5RRMHbO3CyyVbbv_GbreIUu_D9Z1dval7t56YFTS_t_aVBITm4wiIq4ErAouEdaaXwFYNtvjPdIDWNZyyEq0KEtFgMfVjo_tiEqXk1WJy36ItqsEwmar_08jtzNma_P5ejoM1dOZFzPjqPxWWUfho8Xn4B_UM6wkEIAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FBulkActions_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_WVzY7bIBDH7_sUc6kUS4uF8_Fk6aWv0VNFzMSmtQEBiZ2t9t0rY2eLs86mW6mnSr6Y_eA_PwYmPbQ7WXsKPx8AhHSm5mcGhxq7zw8AvJalItJj4xgUqDzafvn70Xl5OJNCK4_KM3CGF0j26FtE1XsYLoRUJYPMYgNZurHY9Ot7XvworT4qweDE7YKQb9kTPi_t8zYJdm0FWmK5kEfHYGe6YbUjruJCtwwoZKaDlemAgi33fEEfYfzSLHkc7ctZO83DJg23pVRkr73XzSCxXy65ufy8PKQDl_xDXOYyLEOGg1aetCjLyjPIKe29C11re42BBoXB3clnZEDTp9V2M825CjlfGdOA97WOgSELHJyupbjaAudI5yPp6Hy85coZblH5m3Kzt3J328thT4rejEUfrevTGC0v1MI_0kutGPC6BpouXVwsq_QJbSj5dvsUcVGzUqskSrr_nwiu_wXBzfzNCFeAptcdm1_zzpcD8XS5mUB_g3M8CqUV3iIyf1s_QOo9LE0SHV1bSY9xWQNZprRfMCEd39cokjugTfKq5KBtM7ZJzT1_XZDMdMnbJ29tuh4I9OYbr17_4q1vuYSHL1LNCy9POCd7Xhadhl9CQoQ2vJD_3LNfx9SV9oTXtW5RRMHbO3CyyVbbv_GbreIUu_D9Z1dval7t56YFTS_t_aVBITm4wiIq4ErAouEdaaXwFYNtvjPdIDWNZyyEq0KEtFgMfVjo_tiEqXk1WJy36ItqsEwmar_08jtzNma_P5ejoM1dOZFzPjqPxWWUfho8Xn4B_UM6wkEIAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var actionButton = 'fw8ilt6';
var actions = 'fw8ilt5';
var clearButton = 'fw8ilt4';
var container = 'fw8ilt0';
var count = 'fw8ilt2';
var danger = 'fw8ilt7';
var feedback = 'fw8ilt8';
var selectAllButton = 'fw8ilt3';
var selection = 'fw8ilt1';

/***/ }),

/***/ 51372:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 52571:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 53214:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 55943:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 56829:
/***/ ((__unused_webpack_module, exports) => {

"use strict";

Object.defineProperty(exports, "__esModule", ({
    value: true
}));
0 && (0);
function _export(target, all) {
    for(var name in all)Object.defineProperty(target, name, {
        enumerable: true,
        get: all[name]
    });
}
_export(exports, {
    assign: function() {
        return assign;
    },
    searchParamsToUrlQuery: function() {
        return searchParamsToUrlQuery;
    },
    urlQueryToSearchParams: function() {
        return urlQueryToSearchParams;
    }
});
function searchParamsToUrlQuery(searchParams) {
    const query = {};
    for (const [key, value] of searchParams.entries()){
        const existing = query[key];
        if (typeof existing === 'undefined') {
            query[key] = value;
        } else if (Array.isArray(existing)) {
            existing.push(value);
        } else {
            query[key] = [
                existing,
                value
            ];
        }
    }
    return query;
}
function stringifyUrlQueryParam(param) {
    if (typeof param === 'string') {
        return param;
    }
    if (typeof param === 'number' && !isNaN(param) || typeof param === 'boolean') {
        return String(param);
    } else {
        return '';
    }
}
function urlQueryToSearchParams(query) {
    const searchParams = new URLSearchParams();
    for (const [key, value] of Object.entries(query)){
        if (Array.isArray(value)) {
            for (const item of value){
                searchParams.append(key, stringifyUrlQueryParam(item));
            }
        } else {
            searchParams.set(key, stringifyUrlQueryParam(value));
        }
    }
    return searchParams;
}
function assign(target) {
    for(var _len = arguments.length, searchParamsList = new Array(_len > 1 ? _len - 1 : 0), _key = 1; _key < _len; _key++){
        searchParamsList[_key - 1] = arguments[_key];
    }
    for (const searchParams of searchParamsList){
        for (const key of searchParams.keys()){
            target.delete(key);
        }
        for (const [key, value] of searchParams.entries()){
            target.append(key, value);
        }
    }
    return target;
} //# sourceMappingURL=querystring.js.map


/***/ }),

/***/ 58970:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 60160:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 60621:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 61201:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 65248:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   A: () => (/* binding */ CircleDot)
/* harmony export */ });
/* unused harmony export __iconNode */
/* harmony import */ var _createLucideIcon_mjs__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(17799);
/**
 * @license lucide-react v1.14.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */ 
const __iconNode = [
    [
        "circle",
        {
            cx: "12",
            cy: "12",
            r: "10",
            key: "1mglay"
        }
    ],
    [
        "circle",
        {
            cx: "12",
            cy: "12",
            r: "1",
            key: "41hilf"
        }
    ]
];
const CircleDot = (0,_createLucideIcon_mjs__WEBPACK_IMPORTED_MODULE_0__/* ["default"] */ .A)("circle-dot", __iconNode);
 //# sourceMappingURL=circle-dot.mjs.map


/***/ }),

/***/ 67000:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 67880:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 68271:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 68540:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 71575:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 71732:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   E: () => (/* reexport safe */ _lib_nav__WEBPACK_IMPORTED_MODULE_0__.N_)
/* harmony export */ });
/* harmony import */ var _lib_nav__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(41078);



/***/ }),

/***/ 73709:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 79423:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 79426:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ BacklogPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/styles/pane/resizeHandle.css.ts
var resizeHandle_css = __webpack_require__(84265);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/lib/analytics/index.ts
var analytics = __webpack_require__(10671);
// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
// EXTERNAL MODULE: ./src/components/ui/AppLink.tsx
var AppLink = __webpack_require__(71732);
// EXTERNAL MODULE: ./src/components/backlog/BacklogItemDetail.tsx + 89 modules
var BacklogItemDetail = __webpack_require__(41062);
// EXTERNAL MODULE: ./src/components/backlog/BacklogItemForm.tsx + 1 modules
var BacklogItemForm = __webpack_require__(91618);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogEmptyState.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81VwXKbMBC9-yt0DAdlANPaIZf-SUagNWwLEpFETOj43zvIcYowsjKTTKcXGNjV27e7b1f3T0n_bHbaxOT3hhCOumvYa04ODQyPG2LflKOC0qAUOSll07disrAGK0HRQKtzUoIwoKbfP3tt8PBKSykMCDM3dYxzFFVOXpi6o_QpeYAxVeM2iyZrxborS2otBgZDbbi_aKfN_YV5YpkfpDBU4whXINyCWPsRsKrNlUdlPUrZSLW0JZETKw3EKvxI51xaNtAjclPnJIvjbnDQt-s98FR6pWDpGL037agm-_QMtKVlqkJBjVyHm_HLvkgjlnnaDY83a8nc4N9s8NXStsEO1y7Wdz9W6nrubnuG-BPSa1BUQwOlyYmQAhz4vVtRFA0KoIHGe-YoHaMlhcRSKFj5q1KyF3y9cKvZPZ-PSsVBXYhfvqliHHt9lfEuUJHio5PYKz0x6iS-ixQFrd_OZNlibvZ5LV9A2Vr6kxVuZx--SMy-rbb1brU4tCQCC499YAnN3It_rTFXLDlJuoFo2SBfHA2s3f9VbcVcbWeGqznU_vRit0OlxfIJKfFq4lM30Rt1u_Z9HTpYnj9a4MjI3fzu2sfdEFnai9vr1gSdNqc_XBMzQnAIAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogEmptyState_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81VwXKbMBC9_yt0DAdlANPaIZf_SUagNWwLEpFETOj43zvIcYowsjKTTKcXGNjV27e7b1f3T0n_bHbaxOT3hhCOumvYa04ODQyPG2LflKOC0qAUOSll07disrAGK0HRQKtzUoIwoKbfP3tt8PBKSykMCDM3dYxzFFVOXpi6o_QpeYAxVeM2iyZrxborS2otBgZDbbi_aKfN_YV5YpkfpDBU4whXINyCWPsRsKrNlUdlPUrZSLW0JZETKw3EKvxI51xaNtAjclPnJIvjbnDQt_s98FR6pWDpGL037agm_QMtKVlqkJBjVyHm_HLvkgjlnnaDY83a8nc4N9s8NXStsEO1y7Wdz9W6nrubnuG_BPSa1BUQwOlyYmQAhz4vVtRFA0KoIHGe_YoHaMlhcRSKFj5q1KyF3y9cKvZPZ_PSsVBXYhfvqliHHt9lfEuUJHio5PYKz0x6iS_ixQFrd_OZNlibvZ5LV9A2Vr6kxVuZx_SMy_rbb1brU4tCQCC499YAnN3It_rTFXLDlJuoFo2SBfHA2s3f9VbcVcbWeGqznU_vRit0OlxfIJKfFq4lM30Rt1u_Z9HTpYnj9a4MjI3fzu2sfdEFnai9vr1gSdNqc_XBMzQnAIAAA_22_7D = __webpack_require__(45916);
;// ./src/components/backlog/BacklogEmptyState.css.ts


var clearFiltersButton = '_1uqt7stb';
var ctaButton = '_1uqt7st8';
var filterZeroText = '_1uqt7sta';
var filterZeroWrapper = '_1uqt7st9';
var footerNudge = '_1uqt7stc';
var headline = '_1uqt7st1';
var lifecycleArrow = '_1uqt7st7';
var lifecycleDiagram = '_1uqt7st3';
var lifecycleNode = '_1uqt7st4';
var lifecycleNodeActive = '_1uqt7st5';
var lifecycleNodeInactive = '_1uqt7st6';
var subline = '_1uqt7st2';
var wrapper = '_1uqt7st0';
;// ./src/components/backlog/BacklogEmptyState.tsx
/* __next_internal_client_entry_do_not_use__ LifecycleDiagram,BacklogEmptyState,FilterZeroState,FooterNudge auto */ 
// +feature: backlog:empty-state

// ---------------------------------------------------------------------------
// Lifecycle diagram data
// ---------------------------------------------------------------------------
const LIFECYCLE_NODES = [
    {
        label: "idea",
        active: true
    },
    {
        label: "ready",
        active: false
    },
    {
        label: "in progress",
        active: false
    },
    {
        label: "review",
        active: false
    },
    {
        label: "done",
        active: false
    }
];
function LifecycleDiagram() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: lifecycleDiagram,
        "aria-hidden": "true",
        "data-testid": "backlog-lifecycle-diagram",
        children: LIFECYCLE_NODES.map((node, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                style: {
                    display: "contents"
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: "".concat(lifecycleNode, " ").concat(node.active ? lifecycleNodeActive : lifecycleNodeInactive),
                        "data-testid": "backlog-lifecycle-node-".concat(node.label),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: node.active ? "◉" : "○"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: node.label
                            }),
                            node.active && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                style: {
                                    fontSize: "10px",
                                    opacity: 0.75
                                },
                                children: "(you start here)"
                            })
                        ]
                    }),
                    i < LIFECYCLE_NODES.length - 1 && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: lifecycleArrow,
                        children: "──►"
                    })
                ]
            }, node.label))
    });
}
// ---------------------------------------------------------------------------
// BacklogEmptyState
// ---------------------------------------------------------------------------
function BacklogEmptyState(param) {
    let { onCreateItem } = param;
    // ponytail: onCreateItem is typed as () => void, but nothing stops a caller from handing
    // us a callback that returns a rejected promise (e.g. a future async create-item RPC) —
    // guard so that isn't an unhandled rejection.
    const handleCreateClick = ()=>{
        Promise.resolve().then(onCreateItem).catch((err)=>{
            console.error("[BacklogEmptyState] onCreateItem failed:", err);
        });
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        role: "region",
        "aria-label": "Backlog — empty",
        className: wrapper,
        "data-testid": "backlog-empty-state",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                className: headline,
                "data-testid": "backlog-empty-headline",
                children: "Your backlog is empty."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: subline,
                children: "Create a work item, define what “done” looks like, spawn an agent — the system reviews output automatically."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(LifecycleDiagram, {}),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: ctaButton,
                autoFocus: true,
                onClick: handleCreateClick,
                "data-testid": "backlog-empty-cta-button",
                children: "+ Create First Item"
            })
        ]
    });
}
// ---------------------------------------------------------------------------
// FilterZeroState
// ---------------------------------------------------------------------------
function FilterZeroState(param) {
    let { onClearFilters } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        role: "status",
        "aria-live": "polite",
        "aria-label": "No results",
        className: filterZeroWrapper,
        "data-testid": "backlog-filter-zero-state",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: filterZeroText,
                children: "No items match your filters."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: clearFiltersButton,
                onClick: onClearFilters,
                "data-testid": "backlog-clear-filters-button",
                children: "Clear filters"
            })
        ]
    });
}
// ---------------------------------------------------------------------------
// FooterNudge
// ---------------------------------------------------------------------------
function FooterNudge(_) {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        role: "status",
        "aria-live": "polite",
        className: footerNudge,
        children: "No items are currently in progress. Mark an item ready and spawn a session to start working."
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
// EXTERNAL MODULE: ./src/lib/hooks/useFocusTrap.ts
var useFocusTrap = __webpack_require__(63991);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FmodalChrome.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VVTW-jMBC951fMpVKQ1pGhbZqQS__GnlYONuAWbNZ2UpJV__sKG1JwgGSl3cNKETH-ePPmzfPw-s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH-UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve-FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec-UPAgaw5GoJUI_wi07R-pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3-oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd-mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO-nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U-cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9-xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FmodalChrome_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VVTW_jMBC951fMpVKQ1pGhbZqQS_GnlYONuAWbNZ2UpJV_sKG1JwgGSl3cNKETH_ePPmzfPw_s5OqSIl01BvxbqkGH4tAFIlSzsAkBVJuDnFgHcLgM8FgJH_UuiWPhevPlzowxlFhE6lKmM3LIhh35cRrupgdyve_FnsH7ywWTkOkT1eSc0NlyKGlNeM7ixw1YYpWGraoeJZ3o330hhZdi8kec_UPAgaw5GoJUI_wi07R_pMLAPKdVWQUwxpwepmghQ8E4gbVuoYEiYMU83020Ebnp5QIoVhwvSXzogLyuoYQoxtTCJ4SRzrrjx4FWlgRLPdV4aPNsNpgtvAZaMoU0gRyg86hjCqLM2KUMpF1mZZkhp9cGryGJ6bqjRz7fsWP3Q7cuZU2uBjPpp7848oVyxx7BNZHErhWNRI54TKjxgwNCFg3TxUtidL_A3a3_oxGBUgbJauBHhyJe4yiZ6q2j3CdTvqCdBVNaxq0LLg1FPrLeghP1vkkqiMi4YwhgbWSpVKYZDmZxZDp5Sd_mjFWbsaJrKQyq8I7sdYD2N42GHLfhQn7OO8TBmdC83MlY3RKOK_M_OlPMOAj4McNp6Vx0n2DT1fyCvbe8E3gW9Kb0MZzHGPAu_SPG2GlyYhRbIMMX4ABJFi5bi609clI9W8YFvXYb_84m2mwZUxvR15MGvTgTMvYYkNO_nJOUaJDwpQcMEuTSVcPfcC7W2gK8XGdHFsryxpxWWCfkVFtvMPT0fnvqrJXTYsp5X7OXCokILd9uLLnNWiczDmg9tCz5U_cwkclG4yqCTvLq_9vrZtxDsjBjrFuTwydZda3sFUJgeNjlzzfcEsgDyYxggxRBMX2undbkMyTW1bs9_xCzCd4GIzqohiwsx209tthd3RVv7DUtI_KOWmL8HoDj6E_svF_g3PZClKtwoAAA_22_7D = __webpack_require__(6579);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FVaguenessPromptModal.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xcmpqanM4MCB7CiAgcG9zaXRpb246IGZpeGVkOwogIGluc2V0OiAwOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6YSk7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIHotaW5kZXg6IDEwMDA7Cn0KLl8xcmpqanM4MSB7CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycno5KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemopOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwogIGJveC1zaGFkb3c6IHZhcigtLV8xOWV6MnJ6M20pOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzMpOwogIG1heC13aWR0aDogNDgwcHg7CiAgd2lkdGg6IGNhbGMoMTAwJSAtIDJyZW0pOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MzEpOwp9Ci5fMXJqampzODIgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozZCk7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIG1hcmdpbjogMDsKfQouXzFyampqczgzIHsKICBmb250LXN0eWxlOiBpdGFsaWM7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNjKTsKICBtYXJnaW46IDA7Cn0KLl8xcmpqanM4NCB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNjKTsKICBtYXJnaW46IDA7CiAgbGluZS1oZWlnaHQ6IDEuNTsKfQouXzFyampqczg1IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKICBmb250LXdlaWdodDogdmFyKC0tXzE5ZXoycnozZyk7CiAgbWFyZ2luOiAwOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FVaguenessPromptModal_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xcmpqanM4MCB7CiAgcG9zaXRpb246IGZpeGVkOwogIGluc2V0OiAwOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6YSk7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIHotaW5kZXg6IDEwMDA7Cn0KLl8xcmpqanM4MSB7CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycno5KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemopOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzgpOwogIGJveC1zaGFkb3c6IHZhcigtLV8xOWV6MnJ6M20pOwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzMpOwogIG1heC13aWR0aDogNDgwcHg7CiAgd2lkdGg6IGNhbGMoMTAwJSAtIDJyZW0pOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MzEpOwp9Ci5fMXJqampzODIgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozZCk7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwogIG1hcmdpbjogMDsKfQouXzFyampqczgzIHsKICBmb250LXN0eWxlOiBpdGFsaWM7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNjKTsKICBtYXJnaW46IDA7Cn0KLl8xcmpqanM4NCB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNjKTsKICBtYXJnaW46IDA7CiAgbGluZS1oZWlnaHQ6IDEuNTsKfQouXzFyampqczg1IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKICBmb250LXdlaWdodDogdmFyKC0tXzE5ZXoycnozZyk7CiAgbWFyZ2luOiAwOwp9_22_7D = __webpack_require__(43702);
;// ./src/components/backlog/VaguenessPromptModal.css.ts



var actions = 'x9n6mdb';
var body = '_1rjjjs84';
var dialog = '_1rjjjs81';
var heading = '_1rjjjs82';
var VaguenessPromptModal_css_itemTitle = '_1rjjjs83';
var overlay = '_1rjjjs80';
var primaryButton = 'x9n6mdc';
var VaguenessPromptModal_css_prompt = '_1rjjjs85';
var secondaryButton = 'x9n6mdd';
;// ./src/components/backlog/VaguenessPromptModal.tsx
/* __next_internal_client_entry_do_not_use__ VaguenessPromptModal auto */ 




/**
 * VaguenessPromptModal — shown after creating an item with a short description
 * and no acceptance criteria. Forces user to choose between refining the item
 * or proceeding with triage on the thin description.
 *
 * No escape-key dismissal: the user must choose one of the two explicit options.
 */ function VaguenessPromptModal(param) {
    let { itemTitle, onRefine, onProceed } = param;
    const headingId = (0,react.useId)();
    const dialogRef = (0,react.useRef)(null);
    // Traps Tab/Shift+Tab between the two buttons and moves focus onto the
    // primary button on open. No escape-key dismissal — user must choose
    // explicitly, and useFocusTrap has no Escape handling of its own.
    (0,useFocusTrap/* useFocusTrap */.r)(dialogRef, true);
    const content = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        "data-testid": "vagueness-prompt-modal",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: dialogRef,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": headingId,
            className: dialog,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    id: headingId,
                    className: heading,
                    children: "Item created."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                    className: VaguenessPromptModal_css_itemTitle,
                    children: [
                        "“",
                        itemTitle,
                        "”"
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: body,
                    children: "The description is brief and has no acceptance criteria. Triage works best with more context."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: VaguenessPromptModal_css_prompt,
                    children: "What would you like to do?"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: actions,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: primaryButton,
                            onClick: onRefine,
                            "data-testid": "vagueness-refine-button",
                            children: "Add more detail"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: secondaryButton,
                            onClick: onProceed,
                            "data-testid": "vagueness-proceed-button",
                            children: "Run triage anyway"
                        })
                    ]
                })
            ]
        })
    });
    // createPortal ensures the modal renders at document.body, escaping any
    // CSS transform/filter ancestor that would break position:fixed (ADR-009).
    if (typeof document === "undefined") return null;
    return /*#__PURE__*/ (0,react_dom.createPortal)(content, document.body);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-dialog@1.1.15+b721ee6ccd61acaa/node_modules/@radix-ui/react-dialog/dist/index.mjs + 25 modules
var dist = __webpack_require__(54069);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FModalTour.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81W226jMBB9z1fMy0qNtESQbG70pX-yMjCAW2OztgmBVf99hSGUcAlUXa32pcKe8VzOnDnpyxsWoSQJKoiP2vaYDb9XAKEUifkAECnxqS5csJ9XAO8rAC36Jqc2va9e-uGcx-EAtCRchUImLiifMHyyN-fjeibVyDNnfathU2fempepUFRTwV0I6RWDyodyhbrJ7xH_LZIi44ELFyKfLOunc8ZyK0ti4gVUpYwULoQMr9UFYTTiFtWYKBd85Bpldf2aKU3DwvIF18h111RalAd4dcGxbZOTcJqQuqYb5Pepd2INoZA5kYF6_mhoN9WQFqkLe_tb9c0w1O2hA5L5ZETjk7W3v32H6u_6MQLn2i5kgNIFJ72CEowGPa_XjpclSUAz1Q-0OxmflAQB5dHAujXWhFytnAY6dmG_tVMDdnM-25f85hIjjWLtwml_ias7cUEZMpFbhQsk02IZ4s4yxH8YxJdyICJpv7dt0fQmI8otT2gtkkH_FXPblHuTsmn8UMNw67k5Psb6PDNVXLfMaHjUC8C75RxMOdPRkq7zcYFzVwA2-87jU4_dxFOCZRpbgo-gBiBrZEZtoeDaUrTEgd0zdl8wIQcTG8DHBcfuItzOfiZV9T4V9MaACYZvi3X_plywNYdPTerkxtUuGBRHO7NniHHqRjvXsj2JIH4gnOPoDOg0xvb8UjQANYBa42PedSsmMxU_mHlNF0Y5tvLibA6fXFzvXisoN_E8Jvy3O2pUOjoqEtOlkw97SBLKigFa2RzY02M_LlJ5XMzXPKYaLZUSH6tdySVJOyj5X1bUcqGijixoW0XwVa5Mh8ZPNTj4t8HgZnmoc0S-CIIpbWwrCv8K5G24qNbpCaUr-0q3UIsfKUk8PY1fM-RO7sh9r-RTND6OT3i5EkcdJZ4urfeE-JpecOaN6L6J__kgov9BZb48nnjReE7dUkZbrn7g3v8AdZCtfz8NAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FModalTour_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81W226jMBB9z1fMy0qNtESQbG70pX_yMjCAW2OztgmBVf99hSGUcAlUXa32pcKe8VzOnDnpyxsWoSQJKoiP2vaYDb9XAKEUifkAECnxqS5csJ9XAO8rAC36Jqc2va9e_uGcx_EAtCRchUImLiifMHyyN_fjeibVyDNnfathU2fempepUFRTwV0I6RWDyodyhbrJ7xH_LZIi44ELFyKfLOunc8ZyK0ti4gVUpYwULoQMr9UFYTTiFtWYKBd85Bpldf2aKU3DwvIF18h111RalAd4dcGxbZOTcJqQuqYb5Pepd2INoZA5kYF6_mhoN9WQFqkLe_tb9c0w1O2hA5L5ZETjk7W3v32H6u_6MQLn2i5kgNIFJ72CEowGPa_XjpclSUAz1Q_0OxmflAQB5dHAujXWhFytnAY6dmG_tVMDdnM_25f85hIjjWLtwml_ias7cUEZMpFbhQsk02IZ4s4yxH8YxJdyICJpv7dt0fQmI8otT2gtkkH_FXPblHuTsmn8UMNw67k5Psb6PDNVXLfMaHjUC8C75RxMOdPRkq7zcYFzVwA2_87jU4_dxFOCZRpbgo_gBiBrZEZtoeDaUrTEgd0zdl8wIQcTG8DHBcfuItzOfiZV9T4V9MaACYZvi3X_plywNYdPTerkxtUuGBRHO7NniHHqRjvXsj2JIH4gnOPoDOg0xvb8UjQANYBa42PedSsmMxU_mHlNF0Y5tvLibA6fXFzvXisoN_E8Jvy3O2pUOjoqEtOlkw97SBLKigFa2RzY02M_LlJ5XMzXPKYaLZUSH6tdySVJOyj5X1bUcqGijixoW0XwVa5Mh8ZPNTj4t8HgZnmoc0S_CIIpbWwrCv8K5G24qNbpCaUr_0q3UIsfKUk8PY1fM_RO7sh9r_RTND6OT3i5EkcdJZ4urfeE_JpecOaN6L6J_kgov9BZb48nnjReE7dUkZbrn7g3v8AdZCtfz8NAAA_22_7D = __webpack_require__(60160);
;// ./src/components/ui/ModalTour.css.ts


var ModalTour_css_body = 'h7t0bla';
var checkboxLabel = 'h7t0bld';
var checkboxRow = 'h7t0blc';
var content = 'h7t0bl3';
var dot = 'h7t0bl5';
var dotActive = 'h7t0bl6';
var dotCompleted = 'h7t0bl7';
var footer = 'h7t0ble';
var footerRight = 'h7t0blf';
var ModalTour_css_headline = 'h7t0bl9';
var kbd = 'h7t0blb';
var ModalTour_css_overlay = 'h7t0bl2';
var ModalTour_css_primaryButton = 'h7t0blg';
var ModalTour_css_secondaryButton = 'h7t0blh';
var skipButton = 'h7t0bl8';
var stepIndicatorRow = 'h7t0bl4';
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogTourModal.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xdmwzbXZjMCB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBsaW5lLWhlaWdodDogMS42OwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzApOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MTcpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNyk7CiAgbWFyZ2luLWJvdHRvbTogdmFyKC0tXzE5ZXoycnozMSk7Cn0KLl8xdmwzbXZjMSB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBsaW5lLWhlaWdodDogMS42OwogIG1hcmdpbi1ib3R0b206IHZhcigtLV8xOWV6MnJ6MzEpOwogIHBhZGRpbmctbGVmdDogdmFyKC0tXzE5ZXoycnozMSk7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogTourModal_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xdmwzbXZjMCB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBsaW5lLWhlaWdodDogMS42OwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MzApOwogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MTcpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ZSk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNyk7CiAgbWFyZ2luLWJvdHRvbTogdmFyKC0tXzE5ZXoycnozMSk7Cn0KLl8xdmwzbXZjMSB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBsaW5lLWhlaWdodDogMS42OwogIG1hcmdpbi1ib3R0b206IHZhcigtLV8xOWV6MnJ6MzEpOwogIHBhZGRpbmctbGVmdDogdmFyKC0tXzE5ZXoycnozMSk7Cn0_22_7D = __webpack_require__(14871);
;// ./src/components/backlog/BacklogTourModal.css.ts


var calloutBox = '_1vl3mvc0';
var flagList = '_1vl3mvc1';
;// ./src/components/backlog/BacklogTourModal.tsx
// +feature: backlog:tour
/* __next_internal_client_entry_do_not_use__ BacklogTourModal auto */ 





const LAST_STEP = 4;
function StepIndicator(param) {
    let { current, total } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: stepIndicatorRow,
        children: Array.from({
            length: total
        }, (_, i)=>{
            const stepNum = i + 1;
            let dotClass = dot;
            if (stepNum < current) dotClass = "".concat(dot, " ").concat(dotCompleted);
            if (stepNum === current) dotClass = "".concat(dot, " ").concat(dotActive);
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: dotClass,
                "aria-hidden": "true"
            }, stepNum);
        })
    });
}
/**
 * First-visit walkthrough for the backlog page. Mirrors the app-wide
 * OnboardingModal (same shared modal chrome/step-indicator styles from
 * components/ui/ModalTour.css, not OnboardingModal's own CSS module), scoped
 * to explain the backlog lifecycle and the Repository Path field specifically
 * — the field that has confused first-time users into pasting a GitHub URL
 * where a local clone path was expected.
 */ function BacklogTourModal(param) {
    let { isOpen, onComplete } = param;
    const [step, setStep] = (0,react.useState)(1);
    const [dontShowAgain, setDontShowAgain] = (0,react.useState)(true);
    const handleSkip = ()=>{
        onComplete(true);
    };
    const handleNext = ()=>{
        if (step < LAST_STEP) setStep((prev)=>prev + 1);
    };
    const handleBack = ()=>{
        if (step > 1) setStep((prev)=>prev - 1);
    };
    const handleDone = ()=>{
        onComplete(dontShowAgain);
    };
    const handleOpenChange = (open)=>{
        if (open) {
            setStep(1);
            setDontShowAgain(true);
        } else {
            onComplete(dontShowAgain);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Root */.bL, {
        open: isOpen,
        onOpenChange: handleOpenChange,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Portal */.ZL, {
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Overlay */.hJ, {
                    className: ModalTour_css_overlay
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Content */.UC, {
                    className: content,
                    "aria-describedby": "backlog-tour-step-description",
                    "data-testid": "backlog-tour-modal",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Title */.hE, {
                            className: ModalTour_css_headline,
                            children: [
                                step === 1 && "How backlog items work",
                                step === 2 && "Filling out the form",
                                step === 3 && "What happens after you hit Create",
                                step === 4 && "Skip planning / Skip review gate"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(StepIndicator, {
                            current: step,
                            total: LAST_STEP
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: skipButton,
                            onClick: handleSkip,
                            "aria-label": "Skip tour",
                            children: "Skip"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            id: "backlog-tour-step-description",
                            children: [
                                step === 1 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: ModalTour_css_body,
                                            children: "Every backlog item moves through the same lifecycle, from a rough idea to a finished, reviewed change."
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)(LifecycleDiagram, {})
                                    ]
                                }),
                                step === 2 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                            className: ModalTour_css_body,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                                    children: "Repository Path"
                                                }),
                                                " is the one field people get tripped up on — it needs a local clone (like",
                                                " ",
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                    className: kbd,
                                                    children: "~/code/my-repo"
                                                }),
                                                "), not a link to a page on GitHub."
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: calloutBox,
                                            "data-testid": "backlog-tour-repo-path-callout",
                                            children: [
                                                "Don't have a local clone? Paste a GitHub URL instead (e.g.",
                                                " ",
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                    className: kbd,
                                                    children: "https://github.com/owner/repo"
                                                }),
                                                ") and we'll clone it for you automatically when you save."
                                            ]
                                        })
                                    ]
                                }),
                                step === 3 && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: ModalTour_css_body,
                                    children: "If you set a repository path, we automatically kick off triage — an agent reviews the item and suggests acceptance criteria. No repo yet? That's fine — the item just sits as an idea until you add one later."
                                }),
                                step === 4 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: ModalTour_css_body,
                                            children: "Two checkboxes on the create form skip parts of that automated workflow:"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("ul", {
                                            className: flagList,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                                            children: "Skip planning phase"
                                                        }),
                                                        " — go straight to triage without a separate planning pass."
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                                            children: "Skip review gate"
                                                        }),
                                                        " — mark work done without an automated review pass first."
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
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    children: step > 1 && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: ModalTour_css_secondaryButton,
                                        onClick: handleBack,
                                        children: "Back"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: footerRight,
                                    children: step < LAST_STEP ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: ModalTour_css_primaryButton,
                                        onClick: handleNext,
                                        children: "Next"
                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                className: checkboxRow,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                        type: "checkbox",
                                                        checked: dontShowAgain,
                                                        onChange: (e)=>setDontShowAgain(e.target.checked)
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: checkboxLabel,
                                                        children: "Don't show this again"
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: ModalTour_css_primaryButton,
                                                onClick: handleDone,
                                                children: "Got it"
                                            })
                                        ]
                                    })
                                })
                            ]
                        })
                    ]
                })
            ]
        })
    });
}

;// ./src/components/backlog/useBacklogTour.ts

const BACKLOG_ONBOARDED_KEY = "stapler-squad:backlog-onboarded";
/** First-visit walkthrough state for the backlog page. Mirrors useOnboarding.ts. */ function useBacklogTour() {
    const [showTour, setShow] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        let timerId;
        try {
            if (!localStorage.getItem(BACKLOG_ONBOARDED_KEY)) {
                timerId = setTimeout(()=>setShow(true), 500);
            }
        } catch (e) {
        // ignore storage errors (private browsing mode, etc.)
        }
        return ()=>clearTimeout(timerId);
    }, []);
    const setTourComplete = (0,react.useCallback)(()=>{
        try {
            localStorage.setItem(BACKLOG_ONBOARDED_KEY, "true");
        } catch (e) {
        // ignore storage errors
        }
        setShow(false);
    }, []);
    // Hides the modal without marking the tour as permanently seen — used when
    // the user explicitly unchecks "Don't show this again" and clicks "Got it".
    const hideTour = (0,react.useCallback)(()=>{
        setShow(false);
    }, []);
    const resetTour = (0,react.useCallback)(()=>{
        try {
            localStorage.removeItem(BACKLOG_ONBOARDED_KEY);
        } catch (e) {
        // ignore storage errors
        }
        setShow(true);
    }, []);
    return {
        showTour,
        setTourComplete,
        hideTour,
        resetTour
    };
}

// EXTERNAL MODULE: ./src/components/backlog/GitHubIssuePicker.tsx + 3 modules
var GitHubIssuePicker = __webpack_require__(85529);
// EXTERNAL MODULE: ./src/components/backlog/ClaimedElsewhereNotice.tsx + 1 modules
var ClaimedElsewhereNotice = __webpack_require__(88627);
// EXTERNAL MODULE: ./src/components/backlog/ConnectionIndicator.tsx + 1 modules
var ConnectionIndicator = __webpack_require__(52454);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/gen/session/v1/backlog_pb.ts
var backlog_pb = __webpack_require__(79870);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/lib/hooks/useWatchBacklogItems.ts
var useWatchBacklogItems = __webpack_require__(10414);
// EXTERNAL MODULE: ./src/lib/hooks/usePersistedViewState.ts
var usePersistedViewState = __webpack_require__(88253);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/backlogBulkSelectionSlice.ts
var backlogBulkSelectionSlice = __webpack_require__(24621);
// EXTERNAL MODULE: ./src/components/sessions/BulkActions.css.ts
var BulkActions_css = __webpack_require__(51213);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogBulkActions.css.ts.vanilla.css%22%2C%22source%22%3A%22Lm96bjdjajAgewogIHBvc2l0aW9uOiBmaXhlZDsKICBpbnNldDogMDsKICBiYWNrZ3JvdW5kOiByZ2JhKDAsIDAsIDAsIDAuNSk7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIHotaW5kZXg6IDEwMDA7Cn0KLm96bjdjajEgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgYm9yZGVyLXJhZGl1czogOHB4OwogIHBhZGRpbmc6IDEuNXJlbTsKICBtYXgtd2lkdGg6IDQyMHB4OwogIHdpZHRoOiA5MCU7Cn0KLm96bjdjajIgewogIGRpc3BsYXk6IGZsZXg7CiAganVzdGlmeS1jb250ZW50OiBmbGV4LWVuZDsKICBnYXA6IDAuNzVyZW07CiAgbWFyZ2luLXRvcDogMXJlbTsKfQoub3puN2NqMyB7CiAgYmFja2dyb3VuZDogcmdiYSgyMzQsIDE3OSwgOCwgMC4xMik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgcGFkZGluZzogOHB4IDEycHg7CiAgYm9yZGVyLXJhZGl1czogNnB4Owp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogBulkActions_css_ts_vanilla_css_22_2C_22source_22_3A_22Lm96bjdjajAgewogIHBvc2l0aW9uOiBmaXhlZDsKICBpbnNldDogMDsKICBiYWNrZ3JvdW5kOiByZ2JhKDAsIDAsIDAsIDAuNSk7CiAgZGlzcGxheTogZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIHotaW5kZXg6IDEwMDA7Cn0KLm96bjdjajEgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgYm9yZGVyLXJhZGl1czogOHB4OwogIHBhZGRpbmc6IDEuNXJlbTsKICBtYXgtd2lkdGg6IDQyMHB4OwogIHdpZHRoOiA5MCU7Cn0KLm96bjdjajIgewogIGRpc3BsYXk6IGZsZXg7CiAganVzdGlmeS1jb250ZW50OiBmbGV4LWVuZDsKICBnYXA6IDAuNzVyZW07CiAgbWFyZ2luLXRvcDogMXJlbTsKfQoub3puN2NqMyB7CiAgYmFja2dyb3VuZDogcmdiYSgyMzQsIDE3OSwgOCwgMC4xMik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgcGFkZGluZzogOHB4IDEycHg7CiAgYm9yZGVyLXJhZGl1czogNnB4Owp9_22_7D = __webpack_require__(52571);
;// ./src/components/backlog/BacklogBulkActions.css.ts

var BacklogBulkActions_css_dialog = 'ozn7cj1';
var dialogActions = 'ozn7cj2';
var BacklogBulkActions_css_overlay = 'ozn7cj0';
var warningNotice = 'ozn7cj3';
;// ./src/components/backlog/BacklogBulkActions.tsx
/* __next_internal_client_entry_do_not_use__ BacklogBulkActions auto */ 
// +feature: backlog-bulk-actions





function BacklogBulkActions(param) {
    let { selectedCount, totalCount, busy = false, feedback, onArchiveSelected, onDeleteSelected, onTriageSelected, onAutoImplementSelected, autoImplementEligibleIds, onSelectAll, onClear } = param;
    const { track } = (0,analytics/* useAnalytics */.st)();
    const [confirmingDelete, setConfirmingDelete] = (0,react.useState)(false);
    const [autoImplementSnapshot, setAutoImplementSnapshot] = (0,react.useState)(null);
    const autoImplementTriggerRef = (0,react.useRef)(null);
    const autoImplementDialogRef = (0,react.useRef)(null);
    // Selection can empty while the dialog is open (live removal, filter change); drop the stale snapshot.
    (0,react.useEffect)(()=>{
        if (selectedCount === 0) setAutoImplementSnapshot(null);
    }, [
        selectedCount
    ]);
    (0,useFocusTrap/* useFocusTrap */.r)(autoImplementDialogRef, autoImplementSnapshot !== null, autoImplementTriggerRef);
    if (selectedCount === 0) {
        return feedback ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            role: "status",
            className: BulkActions_css/* feedback */.mp,
            "data-testid": "backlog-bulk-feedback",
            children: feedback
        }) : null;
    }
    const noun = selectedCount === 1 ? "item" : "items";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "toolbar",
                "aria-label": "Bulk backlog actions",
                className: BulkActions_css/* container */.kL,
                "data-testid": "backlog-bulk-actions",
                children: [
                    feedback && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        role: "status",
                        className: BulkActions_css/* feedback */.mp,
                        "data-testid": "backlog-bulk-feedback",
                        children: feedback
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BulkActions_css/* selection */.r1,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: BulkActions_css/* count */.U9,
                                "aria-live": "polite",
                                "aria-atomic": "true",
                                children: [
                                    selectedCount,
                                    " of ",
                                    totalCount,
                                    " selected"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: BulkActions_css/* selectAllButton */.EN,
                                onClick: ()=>{
                                    track({
                                        name: "backlog_bulk_select_all",
                                        category: "user_action",
                                        component: "BacklogBulkActions"
                                    });
                                    onSelectAll();
                                },
                                children: "Select all"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: BulkActions_css/* clearButton */.mt,
                                onClick: ()=>{
                                    track({
                                        name: "backlog_bulk_clear",
                                        category: "user_action",
                                        component: "BacklogBulkActions"
                                    });
                                    onClear();
                                },
                                children: "Clear"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BulkActions_css/* actions */.o1,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: BulkActions_css/* actionButton */.hP,
                                disabled: busy,
                                onClick: ()=>{
                                    track({
                                        name: "backlog_bulk_triage_selected",
                                        category: "user_action",
                                        component: "BacklogBulkActions",
                                        labels: {
                                            count: String(selectedCount)
                                        }
                                    });
                                    onTriageSelected();
                                },
                                "data-testid": "backlog-bulk-triage-selected",
                                children: "Triage selected"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                ref: autoImplementTriggerRef,
                                className: BulkActions_css/* actionButton */.hP,
                                disabled: busy,
                                onClick: ()=>{
                                    track({
                                        name: "backlog_bulk_auto_implement_open",
                                        category: "user_action",
                                        component: "BacklogBulkActions",
                                        labels: {
                                            count: String(selectedCount)
                                        }
                                    });
                                    setAutoImplementSnapshot({
                                        ids: [
                                            ...autoImplementEligibleIds
                                        ],
                                        ineligible: selectedCount - autoImplementEligibleIds.length
                                    });
                                },
                                "data-testid": "backlog-bulk-auto-implement-selected",
                                children: "Auto-implement selected"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: BulkActions_css/* actionButton */.hP,
                                disabled: busy,
                                onClick: ()=>{
                                    track({
                                        name: "backlog_bulk_archive_selected",
                                        category: "user_action",
                                        component: "BacklogBulkActions",
                                        labels: {
                                            count: String(selectedCount)
                                        }
                                    });
                                    onArchiveSelected();
                                },
                                "data-testid": "backlog-bulk-archive-selected",
                                children: "Archive selected"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "".concat(BulkActions_css/* actionButton */.hP, " ").concat(BulkActions_css/* danger */.OR),
                                disabled: busy,
                                onClick: ()=>{
                                    track({
                                        name: "backlog_bulk_delete_open",
                                        category: "user_action",
                                        component: "BacklogBulkActions"
                                    });
                                    setConfirmingDelete(true);
                                },
                                "data-testid": "backlog-bulk-delete-selected",
                                children: "Delete selected"
                            })
                        ]
                    })
                ]
            }),
            confirmingDelete && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: BacklogBulkActions_css_overlay,
                onClick: ()=>setConfirmingDelete(false),
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "backlog-bulk-delete-title",
                    className: BacklogBulkActions_css_dialog,
                    "data-testid": "backlog-bulk-delete-dialog",
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        if (e.key === "Escape") setConfirmingDelete(false);
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                            id: "backlog-bulk-delete-title",
                            children: [
                                "Delete ",
                                selectedCount,
                                " ",
                                noun,
                                "?"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                "This permanently deletes the selected backlog ",
                                noun,
                                ". This cannot be undone."
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    autoFocus: true,
                                    className: BulkActions_css/* clearButton */.mt,
                                    onClick: ()=>{
                                        track({
                                            name: "backlog_bulk_delete_cancel",
                                            category: "user_action",
                                            component: "BacklogBulkActions"
                                        });
                                        setConfirmingDelete(false);
                                    },
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                    type: "button",
                                    className: "".concat(BulkActions_css/* actionButton */.hP, " ").concat(BulkActions_css/* danger */.OR),
                                    "data-testid": "backlog-bulk-delete-confirm",
                                    onClick: ()=>{
                                        track({
                                            name: "backlog_bulk_delete_confirm",
                                            category: "user_action",
                                            component: "BacklogBulkActions",
                                            labels: {
                                                count: String(selectedCount)
                                            }
                                        });
                                        setConfirmingDelete(false);
                                        onDeleteSelected();
                                    },
                                    children: [
                                        "Delete ",
                                        selectedCount,
                                        " ",
                                        noun
                                    ]
                                })
                            ]
                        })
                    ]
                })
            }),
            autoImplementSnapshot && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: BacklogBulkActions_css_overlay,
                onClick: ()=>setAutoImplementSnapshot(null),
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "backlog-bulk-auto-implement-title",
                    className: BacklogBulkActions_css_dialog,
                    "data-testid": "backlog-bulk-auto-implement-dialog",
                    ref: autoImplementDialogRef,
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        if (e.key === "Escape") setAutoImplementSnapshot(null);
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                            id: "backlog-bulk-auto-implement-title",
                            children: [
                                "Start autonomous implementation for ",
                                autoImplementSnapshot.ids.length,
                                " ",
                                autoImplementSnapshot.ids.length === 1 ? "item" : "items",
                                "?"
                            ]
                        }),
                        autoImplementSnapshot.ineligible > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            "data-testid": "backlog-bulk-auto-implement-ineligible",
                            children: [
                                autoImplementSnapshot.ineligible,
                                " selected ",
                                autoImplementSnapshot.ineligible === 1 ? "item is" : "items are",
                                " ineligible (not ready, or plan not approved) and will be skipped."
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: warningNotice,
                            children: "Sessions will run autonomously without manual steps."
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    autoFocus: true,
                                    className: BulkActions_css/* clearButton */.mt,
                                    onClick: ()=>{
                                        track({
                                            name: "backlog_bulk_auto_implement_cancel",
                                            category: "user_action",
                                            component: "BacklogBulkActions"
                                        });
                                        setAutoImplementSnapshot(null);
                                    },
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                    type: "button",
                                    className: BulkActions_css/* actionButton */.hP,
                                    disabled: busy || autoImplementSnapshot.ids.length === 0,
                                    "data-testid": "backlog-bulk-auto-implement-confirm",
                                    onClick: ()=>{
                                        track({
                                            name: "backlog_bulk_auto_implement_confirm",
                                            category: "user_action",
                                            component: "BacklogBulkActions",
                                            labels: {
                                                count: String(autoImplementSnapshot.ids.length)
                                            }
                                        });
                                        const ids = autoImplementSnapshot.ids;
                                        setAutoImplementSnapshot(null);
                                        onAutoImplementSelected(ids);
                                    },
                                    children: [
                                        "Start ",
                                        autoImplementSnapshot.ids.length,
                                        " ",
                                        autoImplementSnapshot.ids.length === 1 ? "session" : "sessions"
                                    ]
                                })
                            ]
                        })
                    ]
                })
            })
        ]
    });
}

;// ./src/lib/backlog/bulkActions.ts
/** Runs `action` for every id in parallel; a rejection marks that id failed without stopping the rest. */ async function runBulk(ids, action) {
    const settled = await Promise.allSettled(ids.map((id)=>action(id)));
    const result = {
        succeeded: [],
        failed: []
    };
    settled.forEach((s, i)=>{
        (s.status === "fulfilled" ? result.succeeded : result.failed).push(ids[i]);
    });
    return result;
}
function formatBulkFeedback(verb, param) {
    let { succeeded, failed } = param;
    if (failed.length === 0) {
        return "".concat(verb, " ").concat(succeeded.length, " ").concat(succeeded.length === 1 ? "item" : "items");
    }
    return "".concat(verb, " ").concat(succeeded.length, " of ").concat(succeeded.length + failed.length, " (").concat(failed.length, " failed)");
}
/** Max concurrent RPCs for fan-outs that start LLM work or sessions. */ const BULK_CHUNK_SIZE = 5;
function isBulkTriageEligible(item) {
    return (item.status === "idea" || item.status === "ready") && item.triageStatus !== "running";
}
function isBulkAutoImplementEligible(item) {
    return item.status === "ready" && Boolean(item.skipPlanning || item.planApproved);
}
/** Like runBulk, but runs at most `chunkSize` actions concurrently. */ async function runBulkChunked(ids, action) {
    let chunkSize = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : BULK_CHUNK_SIZE;
    const result = {
        succeeded: [],
        failed: []
    };
    for(let i = 0; i < ids.length; i += chunkSize){
        const part = await runBulk(ids.slice(i, i + chunkSize), action);
        result.succeeded.push(...part.succeeded);
        result.failed.push(...part.failed);
    }
    return result;
}
async function runBulkSpawnChunked(ids, spawn) {
    let chunkSize = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : BULK_CHUNK_SIZE;
    const result = {
        started: [],
        queued: [],
        failed: []
    };
    for(let i = 0; i < ids.length; i += chunkSize){
        const part = ids.slice(i, i + chunkSize);
        const settled = await Promise.allSettled(part.map((id)=>spawn(id)));
        settled.forEach((s, j)=>{
            if (s.status === "rejected" || s.value === null) result.failed.push(part[j]);
            else (s.value.queued ? result.queued : result.started).push(part[j]);
        });
    }
    return result;
}
function formatSpawnBulkFeedback(param) {
    let { started, queued, failed } = param;
    const parts = [];
    if (started.length > 0 || queued.length === 0) {
        parts.push("Started ".concat(started.length).concat(queued.length === 0 && failed.length === 0 ? " autonomous session".concat(started.length === 1 ? "" : "s") : ""));
    }
    if (queued.length > 0) parts.push("queued ".concat(queued.length));
    const text = parts.join(", ");
    const capitalized = text.charAt(0).toUpperCase() + text.slice(1);
    return failed.length > 0 ? "".concat(capitalized, " (").concat(failed.length, " failed)") : capitalized;
}
/** Triage-specific feedback: `skipped` counts selected items that were ineligible. */ function formatTriageFeedback(result, skipped) {
    if (result.succeeded.length === 0 && result.failed.length === 0) {
        return skipped > 0 ? "Nothing to triage (".concat(skipped, " ineligible)") : "Nothing to triage";
    }
    const base = result.failed.length === 0 ? "Triggered triage for ".concat(result.succeeded.length, " ").concat(result.succeeded.length === 1 ? "item" : "items") : "Triggered ".concat(result.succeeded.length, " of ").concat(result.succeeded.length + result.failed.length, " (").concat(result.failed.length, " failed)");
    return skipped > 0 ? "".concat(base, ", ").concat(skipped, " skipped") : base;
}
/**
 * Runs `fn` unless another bulk action holds `busyRef`. The ref is checked synchronously
 * because React's busy state has not committed yet when a second tap arrives.
 * Returns undefined when skipped.
 */ async function runGuarded(busyRef, setBusy, fn) {
    if (busyRef.current) return undefined;
    busyRef.current = true;
    setBusy(true);
    try {
        return await fn();
    } finally{
        busyRef.current = false;
        setBusy(false);
    }
}

// EXTERNAL MODULE: ./src/lib/store/backlogItemsSlice.ts
var backlogItemsSlice = __webpack_require__(63939);
// EXTERNAL MODULE: ./src/lib/backlog/status.ts
var backlog_status = __webpack_require__(85546);
;// ./src/lib/backlog/sortGroup.ts
// Sort/group-by-repository helpers for the Backlog table view.
/** Sentinel bucket label for items with no repo_path set. Sorts last, mirrors
 * the "No Path" convention in web-app/src/lib/grouping/strategies.ts. */ const NO_REPOSITORY = "No Repository";
function compareByRepoPath(a, b) {
    var _a_repoPath, _b_repoPath;
    return ((_a_repoPath = a.repoPath) !== null && _a_repoPath !== void 0 ? _a_repoPath : "").localeCompare((_b_repoPath = b.repoPath) !== null && _b_repoPath !== void 0 ? _b_repoPath : "");
}
/** Groups items by repo_path. Items retain the relative order they arrived in
 * (callers should sort first), so sort-then-group composes as expected. */ function groupByRepoPath(items) {
    const grouped = new Map();
    for (const item of items){
        var _item_repoPath;
        const key = ((_item_repoPath = item.repoPath) === null || _item_repoPath === void 0 ? void 0 : _item_repoPath.trim()) || NO_REPOSITORY;
        const bucket = grouped.get(key);
        if (bucket) {
            bucket.push(item);
        } else {
            grouped.set(key, [
                item
            ]);
        }
    }
    const keys = Array.from(grouped.keys()).sort((a, b)=>{
        if (a === NO_REPOSITORY) return 1;
        if (b === NO_REPOSITORY) return -1;
        return a.localeCompare(b, undefined, {
            sensitivity: "base"
        });
    });
    return keys.map((key)=>{
        var _grouped_get;
        return {
            groupKey: key,
            displayName: key,
            items: (_grouped_get = grouped.get(key)) !== null && _grouped_get !== void 0 ? _grouped_get : []
        };
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useBacklogFilters.ts
var useBacklogFilters = __webpack_require__(3598);
// EXTERNAL MODULE: ./src/components/backlog/BacklogFilterBar.tsx + 1 modules
var BacklogFilterBar = __webpack_require__(23522);
// EXTERNAL MODULE: ./src/lib/contexts/OmnibarContext.tsx + 43 modules
var OmnibarContext = __webpack_require__(40218);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fpane%2FdetailPaneShell.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8yanFmdmwwIHsKICBtaW4td2lkdGg6IDI0MHB4OwogIG1heC13aWR0aDogODAwcHg7CiAgYm9yZGVyLWxlZnQ6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGZsZXgtc2hyaW5rOiAwOwogIG92ZXJmbG93OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwp9CkBtZWRpYSAobWF4LXdpZHRoOiA3NjhweCkgewogIC5fMmpxZnZsMCB7CiAgICBwb3NpdGlvbjogZml4ZWQ7CiAgICB0b3A6IDA7CiAgICBsZWZ0OiAwOwogICAgcmlnaHQ6IDA7CiAgICBib3R0b206IHZhcigtLWJvdHRvbS1uYXYtaGVpZ2h0LCA3MnB4KTsKICAgIHdpZHRoOiAxMDAlICFpbXBvcnRhbnQ7CiAgICB6LWluZGV4OiA1MDA7CiAgICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejkpOwogIH0KfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fpane_2FdetailPaneShell_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8yanFmdmwwIHsKICBtaW4td2lkdGg6IDI0MHB4OwogIG1heC13aWR0aDogODAwcHg7CiAgYm9yZGVyLWxlZnQ6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGZsZXgtc2hyaW5rOiAwOwogIG92ZXJmbG93OiBoaWRkZW47CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwp9CkBtZWRpYSAobWF4LXdpZHRoOiA3NjhweCkgewogIC5fMmpxZnZsMCB7CiAgICBwb3NpdGlvbjogZml4ZWQ7CiAgICB0b3A6IDA7CiAgICBsZWZ0OiAwOwogICAgcmlnaHQ6IDA7CiAgICBib3R0b206IHZhcigtLWJvdHRvbS1uYXYtaGVpZ2h0LCA3MnB4KTsKICAgIHdpZHRoOiAxMDAlICFpbXBvcnRhbnQ7CiAgICB6LWluZGV4OiA1MDA7CiAgICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejkpOwogIH0KfQ_22_7D = __webpack_require__(37341);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FstatusBadgeVariants.css.ts.vanilla.css%22%2C%22source%22%3A%22LmVwejgyZzAgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKfQouZXB6ODJnMSB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoyZSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MmYpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MmcpOwp9Ci5lcHo4MmcyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejJrKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoybCk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnoybSk7Cn0KLmVwejgyZzMgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MmIpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejJjKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyejJkKTsKfQouZXB6ODJnNCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoyaCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MmkpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MmopOwp9Ci5lcHo4Mmc1IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemQpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejMpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6Zyk7Cn0KLmVwejgyZzYgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6eSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnp4KTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FstatusBadgeVariants_css_ts_vanilla_css_22_2C_22source_22_3A_22LmVwejgyZzAgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6ZCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnpnKTsKfQouZXB6ODJnMSB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoyZSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MmYpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MmcpOwp9Ci5lcHo4MmcyIHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejJrKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoybCk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnoybSk7Cn0KLmVwejgyZzMgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MmIpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejJjKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyejJkKTsKfQouZXB6ODJnNCB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoyaCk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MmkpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MmopOwp9Ci5lcHo4Mmc1IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyemQpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejMpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6Zyk7Cn0KLmVwejgyZzYgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6eSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6eik7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnp4KTsKfQ_22_7D = __webpack_require__(23401);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fbacklog%2Fbacklog.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VZ247cKBB9n6_gZaW0tERuJ5OZ8bzkTyLalNtMY3AA922Vf18Z223A155JpF2NNN1tigLOqSoO-PsBLpkiBWj0Y_v1dPipHwv0zwNC0V_2A6EdSQ97JStBcSq5VAk6EvUJ4x_bF7jG6rp93rw-IPTrAaFtNNPLKCJ0SRQI09j_evg-HFz6g8uSpMxcErQdGePWGHUOP3duImtCmS45uSQo43CubepPTJmC1DApEpRKXhWibsmB7XOTWPf1b3kElXF5SlDOKAVr068pxODb5tUdfTs-OuFsLzAzUOgEpSAMqPrxW6UNyy44lcKAMAnSJUkB78CcoBm4JJQysQ9H_bLdhE9iy8VOKgoK76QxskjQtjwjLTmjgTVY4z0pB46jzQ0tnSsmDi3G9slJ1T3q_96iY7voTAqDNbvCwGczmm0_tWAHFsxajAZZ5AP85S6AR1YYX32HX32HTHAmAL_D72UzQ1h8HRDWID0dWcU0Jj8drhMkpACHe0Uoq_QA4qeehFGSdosk7ZsJVUrXMyol69Cw-c2avOrXg6LPW42AaPDxTvI6wyzq04sXPkmP7yFpkF1904lRkycofi7Pbgnoft9IjEKKglI2XhY9duZTcJ6zl_8IZ4-rOHt2VzSKTO6z-m08l21yRXdXs5608dIY1jRnIk92IutTd_u7eBkDqZmtgbPBFFKpSENSl-UucbZ_z9nfHvgul6Mh4DLp15BBTekYiG8MeIngQumEypp6_jxtWYztaPFEDBS-25dpFdAqiuE273QnzY42Zk0qI-d3Zs_Tznpq600nMXqaOCk1WCLtt-WwclynTdTKLhq0YenhYmNEliO1ayhcELpiJmi7Ssc1vTMhoj5mbTFOEIfMzK-GLCZJvipJbCRmUhUJqsoSVNpGPAdjQOFaUjVl4XP0CMVtzIwUjF8Gfqv7ZdQpZwbsOFAnzkAegQ-mX3oy20g4T1AlNJhm9HMNmbW-zcTG7y2gd1ymh9dhZN1L2UhZ8NBhIgfFQibDpx1_zvOWNedJSJbTFFIV9gpiygP8ZuuCmmQyrTQ-Ms12vMFfVqYWDfMV5GaGZZZpMAnCcS0JHN_7pmCNlNN1MZNt7t5-96u3X6_Tn8Mgd4val9gTTVg1seCHOWvG945tAwxHljzlYDu61hMzORMrO7EkzSE9AJ0zeuu_zZkdglJMdlryyoCbob7UbH-OHTZTzsoE1SfVTxGyf6vKDF-Ij-2THyBlO2cLPoYjCKO7jd8x-_murWBy60foCMqwlPAuqwtGKffHbEK9IGd8E-rRNF62RPQNwDkrNdOTmC0rM2cquj_bzu4YS_vczC62RKwZoPH1t6Ixc4j56LqdVVTviqP_qXyYTEJ6v7xoja2mm7N0sD4G5ShjZ6C1LyZsQV-QhiSQhlEUvX7kVqtvWimaTwul7GXVCfttxQn7ee7Spj04BhLLScSnuE3E-llX2V-iY-4mJ770hwar7HJC68wMxgoOMOeFOzX60Tu1etJqz8QtCucpubznBmYe-28-9rUmGbmq-x014A8lugPPdSFiYzFTjOSqcI7L4Ko5eg8lM8l5F1vhJUt7_fkRrtjKLadgot8Jm_xbUD0rLhimcW8EwfcCKCPok5WljUzaWPwDsTl8cdH3LRVkoDRWQKsUKC5kd7WDmyYQaehVtF6JYIW9CcKCFJD0r2xeg2ZadTdG8WNU6LDZsKIW6Vkl2hch9WkDy8qEhhnjHBeSQoIyqU5EUf3avogJJOT05OTM5KI_MrllsJvfEyjf8cZrAMKA938BXF2j6WEbAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fbacklog_2Fbacklog_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VZ247cKBB9n6_gZaW0tERuJ5OZ8bzkTyLalNtMY3AA922Vf18Z223A155JpF2NNN1tigLOqSoO_PsBLpkiBWj0Y_v1dPipHwv0zwNC0V_2A6EdSQ97JStBcSq5VAk6EvUJ4x_bF7jG6rp93rw_IPTrAaFtNNPLKCJ0SRQI09j_evg_HFz6g8uSpMxcErQdGePWGHUOP3duImtCmS45uSQo43CubepPTJmC1DApEpRKXhWibsmB7XOTWPf1b3kElXF5SlDOKAVr068pxODb5tUdfTs_OuFsLzAzUOgEpSAMqPrxW6UNyy44lcKAMAnSJUkB78CcoBm4JJQysQ9H_bLdhE9iy8VOKgoK76QxskjQtjwjLTmjgTVY4z0pB46jzQ0tnSsmDi3G9slJ1T3q_96iY7voTAqDNbvCwGczmm0_tWAHFsxajAZZ5AP85S6AR1YYX32HX32HTHAmAL_D72UzQ1h8HRDWID0dWcU0Jj8drhMkpACHe0Uoq_QA4qeehFGSdosk7ZsJVUrXMyol69Cw_c2avOrXg6LPW42AaPDxTvI6wyzq04sXPkmP7yFpkF1904lRkycofi7Pbgnoft9IjEKKglI2XhY9duZTcJ6zl_8IZ4_rOHt2VzSKTO6z_m08l21yRXdXs5608dIY1jRnIk92IutTd_u7eBkDqZmtgbPBFFKpSENSl_UucbZ_z9nfHvgul6Mh4DLp15BBTekYiG8MeIngQumEypp6_jxtWYztaPFEDBS_25dpFdAqiuE273QnzY42Zk0qI_d3Zs_Tznpq600nMXqaOCk1WCLtt_WwclynTdTKLhq0YenhYmNEliO1ayhcELpiJmi7Ssc1vTMhoj5mbTFOEIfMzK_GLCZJvipJbCRmUhUJqsoSVNpGPAdjQOFaUjVl4XP0CMVtzIwUjF8Gfqv7ZdQpZwbsOFAnzkAegQ_mX3oy20g4T1AlNJhm9HMNmbW_zcTG7y2gd1ymh9dhZN1L2UhZ8NBhIgfFQibDpx1_zvOWNedJSJbTFFIV9gpiygP8ZuuCmmQyrTQ_Ms12vMFfVqYWDfMV5GaGZZZpMAnCcS0JHN_7pmCNlNN1MZNt7t5_96u3X6_Tn8Mgd4val9gTTVg1seCHOWvG945tAwxHljzlYDu61hMzORMrO7EkzSE9AJ0zeuu_zZkdglJMdlryyoCbob7UbH_OHTZTzsoE1SfVTxGyf6vKDF_Ij_2THyBlO2cLPoYjCKO7jd8x_murWBy60foCMqwlPAuqwtGKffHbEK9IGd8E_rRNF62RPQNwDkrNdOTmC0rM2cquj_bzu4YS_vczC62RKwZoPH1t6Ixc4j56LqdVVTviqP_qXyYTEJ6v7xoja2mm7N0sD4G5ShjZ6C1LyZsQV_QhiSQhlEUvX7kVqtvWimaTwul7GXVCfttxQn7ee7Spj04BhLLScSnuE3E_llX2V_iY_4mJ770hwar7HJC68wMxgoOMOeFOzX60Tu1etJqz8QtCucpubznBmYe_28_9rUmGbmq_x014A8lugPPdSFiYzFTjOSqcI7L4Ko5eg8lM8l5F1vhJUt7_fkRrtjKLadgot8Jm_xbUD0rLhimcW8EwfcCKCPok5WljUzaWPwDsTl8cdH3LRVkoDRWQKsUKC5kd7WDmyYQaehVtF6JYIW9CcKCFJD0r2xeg2ZadTdG8WNU6LDZsKIW6Vkl2hch9WkDy8qEhhnjHBeSQoIyqU5EUf3avogJJOT05OTM5KI_MrllsJvfEyjf8cZrAMKA938BXF2j6WEbAAA_22_7D = __webpack_require__(60621);
;// ./src/app/backlog/backlog.css.ts




var acProgressCell = '_14wkqs5s';
var checkboxCell = '_14wkqs5h';
var contentArea = '_14wkqs59';
var detailPane = '_2jqfvl0';
var groupHeaderCell = '_14wkqs5u';
var headerActions = '_14wkqs53';
var helpButton = '_14wkqs55';
var listPane = '_14wkqs5a';
var modalBox = '_14wkqs5w';
var modalOverlay = '_14wkqs5v';
var modalTitle = '_14wkqs5x';
var newItemButton = '_14wkqs54';
var pageHeader = '_14wkqs51';
var pageTitle = '_14wkqs52';
var pageWrapper = '_14wkqs50';
var priorityBadge = '_14wkqs510';
var repoPathCell = '_14wkqs5t';
var rowCheckbox = '_14wkqs5i';
var rowCheckboxVisible = '_14wkqs5j';
var srOnly = '_14wkqs5k';
var statusArchived = 'epz82g5';
var statusBadge = '_14wkqs5y';
var statusDone = 'epz82g4';
var statusIdea = 'epz82g0';
var statusInProgress = 'epz82g2';
var statusQueued = '_14wkqs5z';
var statusReady = 'epz82g1';
var statusRefining = 'epz82g6';
var statusReview = 'epz82g3';
var tab = '_14wkqs57';
var tabActive = '_14wkqs58';
var tabBar = '_14wkqs56';
var table = '_14wkqs5b';
var tableCell = '_14wkqs5q';
var tableHead = '_14wkqs5c';
var tableHeaderCell = '_14wkqs5d';
var tableHeaderCellSortable = '_14wkqs5e';
var tableHeaderSortButton = '_14wkqs5f';
var tableRow = '_14wkqs5g';
var tableRowActive = '_14wkqs5l';
var tableRowExiting = '_14wkqs5p';
var tableRowJustChanged = '_14wkqs5n';
var titleCell = '_14wkqs5r';
;// ./src/app/backlog/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: backlog:list-page

































const SORT_COLUMNS = [
    "title",
    "status",
    "priority",
    "updatedAt",
    "repoPath"
];
const GROUP_BY_VALUES = [
    "none",
    "repoPath"
];
// Persisted under stapler-squad-backlog-* keys (see usePersistedViewState) —
// namespaced separately from SessionList's stapler-squad-* keys. The
// search/statusFilter/priorityFilter/showArchived fields are spread in from
// BACKLOG_FILTER_FIELDS (useBacklogFilters.ts) — the single source of truth
// shared with the board view — so the two views can't silently diverge on
// keys/validators. sortCol/sortAsc/groupBy are list-view-only extras.
const BACKLOG_VIEW_FIELDS = {
    ...useBacklogFilters/* BACKLOG_FILTER_FIELDS */.py,
    sortCol: {
        key: "stapler-squad-backlog-sort-col",
        defaultValue: "updatedAt",
        isValid: (v)=>SORT_COLUMNS.includes(v)
    },
    sortAsc: {
        key: "stapler-squad-backlog-sort-asc",
        defaultValue: false,
        isValid: (v)=>typeof v === "boolean"
    },
    groupBy: {
        key: "stapler-squad-backlog-group-by",
        defaultValue: "none",
        isValid: (v)=>GROUP_BY_VALUES.includes(v)
    }
};
const STATUS_CSS = {
    idea: statusIdea,
    refining: statusRefining,
    ready: statusReady,
    queued: statusQueued,
    in_progress: statusInProgress,
    review: statusReview,
    pr_pending: statusReview,
    done: statusDone,
    archived: statusArchived
};
const getStatusClass = (s)=>{
    var _STATUS_CSS_s;
    return (_STATUS_CSS_s = STATUS_CSS[s]) !== null && _STATUS_CSS_s !== void 0 ? _STATUS_CSS_s : statusArchived;
};
// Epic 6.3 (backlog-event-driven-updates): how long a row's fade-out plays
// before it's removed from the DOM after dropping out of the active filter
// (ux.md §7 — "~200ms"). Under `prefers-reduced-motion: reduce` this is
// bypassed to 0ms (instant removal) at the call site.
const EXIT_TRANSITION_MS = 200;
const PRIORITY_LABELS = {
    1: "P1",
    2: "P2",
    3: "P3",
    4: "P4",
    5: "P5"
};
function formatDateShort(iso) {
    if (!iso) return "—";
    return new Date(iso).toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        year: "numeric"
    });
}
// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------
// Sweep fix (backlog-event-driven-updates Phase 5 compliance sweep,
// 2026-07-22): ux.md UX AC #6 requires the list row itself to play "a
// background flash that fades within ~1 second" on a genuine live update —
// the page previously only tracked `item.liveVersion` for the Epic 6.3 exit
// transition, never applying the Epic 6.1 in-place flash BacklogItemCard.tsx
// (used by the Kanban board) already has. Extracted into its own component,
// mirroring BacklogItemCard's exact flash-tracking hook, since a plain row
// function has no hook of its own to track the previous liveVersion.
function BacklogTableRow(param) {
    let { item, isActive, isExiting, isSelected, isSelectMode, onRowClick, onToggleSelect } = param;
    const acDone = item.acCriteria.filter((c)=>c.status === "done").length;
    const prevLiveVersionRef = (0,react.useRef)(item.liveVersion);
    const [justChanged, setJustChanged] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        const prev = prevLiveVersionRef.current;
        const next = item.liveVersion;
        prevLiveVersionRef.current = next;
        if (prev === undefined || next === undefined || next === prev) return;
        setJustChanged(true);
        const timer = setTimeout(()=>setJustChanged(false), 250);
        return ()=>clearTimeout(timer);
    }, [
        item.liveVersion
    ]);
    var _PRIORITY_LABELS_item_priority, _PRIORITY_LABELS_item_priority1;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
        className: "".concat(tableRow, " ").concat(isActive ? tableRowActive : "", " ").concat(isExiting ? tableRowExiting : "", " ").concat(justChanged ? tableRowJustChanged : ""),
        tabIndex: isExiting ? -1 : 0,
        role: "row",
        "aria-selected": isActive,
        "aria-hidden": isExiting || undefined,
        "data-testid": "backlog-table-row",
        "data-item-id": item.id,
        "data-exiting": isExiting ? "true" : undefined,
        onClick: ()=>{
            if (isExiting) return;
            if (isSelectMode) {
                onToggleSelect(item.id);
                return;
            }
            onRowClick(item.id);
        },
        onKeyDown: (e)=>{
            if (isExiting || e.target !== e.currentTarget) return;
            if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                if (isSelectMode) onToggleSelect(item.id);
                else onRowClick(item.id);
            }
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: "".concat(tableCell, " ").concat(checkboxCell),
                onClick: (e)=>e.stopPropagation(),
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                    type: "checkbox",
                    className: "".concat(rowCheckbox, " ").concat(isSelectMode ? rowCheckboxVisible : ""),
                    checked: isSelected,
                    onChange: ()=>onToggleSelect(item.id),
                    "aria-label": "Select ".concat(item.title),
                    "data-testid": "backlog-row-checkbox"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: "".concat(tableCell, " ").concat(titleCell),
                children: item.title
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: tableCell,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: "".concat(statusBadge, " ").concat(getStatusClass(item.status)),
                    "aria-label": "Status: ".concat((0,backlog_status/* getStatusLabel */.p)(item.status)),
                    "data-testid": item.status === "queued" ? "backlog-status-queued" : undefined,
                    children: (0,backlog_status/* getStatusLabel */.p)(item.status)
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: tableCell,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: priorityBadge,
                    "data-testid": "priority-badge",
                    "aria-label": "Priority: ".concat((_PRIORITY_LABELS_item_priority = PRIORITY_LABELS[item.priority]) !== null && _PRIORITY_LABELS_item_priority !== void 0 ? _PRIORITY_LABELS_item_priority : "Unknown"),
                    children: (_PRIORITY_LABELS_item_priority1 = PRIORITY_LABELS[item.priority]) !== null && _PRIORITY_LABELS_item_priority1 !== void 0 ? _PRIORITY_LABELS_item_priority1 : "P?"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: "".concat(tableCell, " ").concat(acProgressCell),
                children: item.acCriteria.length > 0 ? "".concat(acDone, "/").concat(item.acCriteria.length) : "—"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: tableCell,
                style: {
                    whiteSpace: "nowrap"
                },
                children: formatDateShort(item.updatedAt)
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                className: "".concat(tableCell, " ").concat(repoPathCell),
                "data-testid": "backlog-repo-path-cell",
                children: item.repoPath || "—"
            })
        ]
    });
}
// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------
function BacklogPageInner() {
    (0,usePageView/* usePageView */.O)();
    const { track } = (0,analytics/* useAnalytics */.st)();
    const { createBacklogItem, importGitHubIssue, triggerTriage, spawnSessionFromItem, archiveBacklogItem, deleteBacklogItem } = (0,useBacklogService/* useBacklogService */.gG)();
    const { openOmnibar } = (0,OmnibarContext/* useOmnibar */.W)();
    const router = (0,nav/* useRouter */.KA)();
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const dispatch = (0,store/* useAppDispatch */.jL)();
    const selectedItemId = searchParams.get("item");
    // Epic 5.1 (backlog-event-driven-updates): live-updating list, replacing
    // the former fetch-once-on-mount pattern. useWatchBacklogItems streams
    // every backlog item unfiltered — per design/ux.md Surface 1's interaction
    // flow ("subscribes ... unfiltered ... then filters client-side same as
    // today") — so status/priority/search/archived filtering below is purely
    // client-side and never tears down/reconnects the stream when a filter
    // chip changes. The hook already returns domain-shaped BacklogItem[]
    // (mapped internally, see useWatchBacklogItems.ts's file-header note 3),
    // so no further proto->domain mapping is needed here.
    const { items, connectionState } = (0,useWatchBacklogItems/* useWatchBacklogItems */.E)();
    // Only the very first connect (before any items have ever loaded) shows a
    // loading state — a reconnect must keep showing last-known data, not blank
    // or spinner out (design/ux.md Surface 1 "Error / edge cases").
    const loading = connectionState === "connecting" && items.length === 0;
    // Filters, sort, and grouping — persisted across page loads under
    // stapler-squad-backlog-* keys (see usePersistedViewState).
    const backlogViewState = (0,usePersistedViewState/* usePersistedViewState */.P)(BACKLOG_VIEW_FIELDS);
    const { search, statusFilter, priorityFilter, sourceFilter, // showArchived: archived items are excluded client-side by default (Epic
    // 5.1 — see filteredItems below); enabling this reveals them from the
    // already-loaded live store. Mirrors SessionList's "Show Archived" toggle.
    showArchived, sortCol, sortAsc, groupBy } = backlogViewState.state;
    const { search: setSearch, statusFilter: setStatusFilter, priorityFilter: setPriorityFilter, sourceFilter: setSourceFilter, showArchived: setShowArchived, sortCol: setSortCol, sortAsc: setSortAsc, groupBy: setGroupBy } = backlogViewState.setters;
    const resetViewState = backlogViewState.resetToDefaults;
    // Detail pane resize
    const [detailWidth, setDetailWidth] = (0,react.useState)(420);
    const dragRef = (0,react.useRef)({
        active: false,
        startX: 0,
        startWidth: 0
    });
    const handleResizePointerDown = (0,react.useCallback)((e)=>{
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        dragRef.current = {
            active: true,
            startX: e.clientX,
            startWidth: detailWidth
        };
    }, [
        detailWidth
    ]);
    const handleResizePointerMove = (0,react.useCallback)((e)=>{
        if (!dragRef.current.active) return;
        const delta = dragRef.current.startX - e.clientX;
        setDetailWidth(Math.max(240, Math.min(800, dragRef.current.startWidth + delta)));
    }, []);
    const handleResizePointerUp = (0,react.useCallback)((e)=>{
        dragRef.current.active = false;
        e.currentTarget.releasePointerCapture(e.pointerId);
    }, []);
    // New-item modal
    const [showForm, setShowForm] = (0,react.useState)(false);
    const [formMode, setFormMode] = (0,react.useState)("manual");
    const [githubIssueUrl, setGithubIssueUrl] = (0,react.useState)("");
    const [githubImporting, setGithubImporting] = (0,react.useState)(false);
    const [githubImportError, setGithubImportError] = (0,react.useState)(null);
    const [githubImportProgress, setGithubImportProgress] = (0,react.useState)(null);
    // Issues another host already claimed (cross_host_claim_dedup) — a third
    // import outcome, distinct from "already imported" and "failed".
    const [claimedElsewhere, setClaimedElsewhere] = (0,react.useState)([]);
    // First-visit walkthrough
    const { showTour, setTourComplete, hideTour, resetTour } = useBacklogTour();
    // Vagueness prompt modal state
    const [vaguenessItem, setVaguenessItem] = (0,react.useState)(null);
    // Raw (unmapped) ConnectRPC client — used only to hydrate a just-created
    // item into the shared backlogItemsSlice store immediately. The
    // WatchBacklogItems event stream (Phases 1-3) has no "item created" oneof
    // variant (see plan.md's BacklogItemEvent oneof: status_changed /
    // verdict_recorded / session_attached / item_updated / item_archived /
    // item_removed only), so a freshly created/imported item would not
    // otherwise appear in this live list until some other event touches it.
    // This is a known gap in the event proto's scope, not something Epic 5.1
    // can fix on its own — flagged here rather than silently worked around by
    // re-adding a full listBacklogItems fetch-on-every-mutation path.
    const rawClientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)(),
            interceptors: [
                (0,config/* createAuthInterceptor */._)()
            ]
        });
        rawClientRef.current = (0,promise_client/* createClient */.UU)(backlog_pb/* BacklogService */.jSM, transport);
    }, []);
    const hydrateItemIntoStore = (0,react.useCallback)(async (itemId)=>{
        if (!rawClientRef.current) return;
        try {
            const resp = await rawClientRef.current.getBacklogItem({
                itemId
            });
            if (resp.item) dispatch((0,backlogItemsSlice/* upsertItem */.vH)(resp.item));
        } catch (err) {
            console.error("[BacklogPage] failed to hydrate newly created item into the live store:", err);
        }
    }, [
        dispatch
    ]);
    // Epic 5.1: client-side filtering (search/status/priority/archived),
    // mirroring what the server-side listBacklogItems filter used to do before
    // this page moved to the shared live stream (design/ux.md Surface 1).
    const filteredItems = (0,react.useMemo)(()=>(0,useBacklogFilters/* filterBacklogItems */.ub)(items, {
            search,
            statusFilter,
            priorityFilter,
            sourceFilter,
            showArchived
        }), [
        items,
        search,
        statusFilter,
        priorityFilter,
        sourceFilter,
        showArchived
    ]);
    // Epic 6.3 (backlog-event-driven-updates): when an item's fields change
    // such that it no longer matches the active filter, keep rendering it
    // briefly with a fade-out instead of letting it vanish in the same render
    // the filter re-evaluates (ux.md §7 "reads as moved, not vanished"). Only
    // genuinely live, one-at-a-time departures animate — gated on
    // `item.liveVersion` advancing (the same signal BacklogItemCard's flash
    // uses), so a bulk resnapshot on reconnect (liveVersion never advances for
    // a snapshot-flagged event, per backlogItemsSlice.ts) or the user simply
    // toggling a filter chip (no liveVersion change at all) both fall through
    // to an ordinary instant removal, matching ux.md's edge cases.
    const [exitingItems, setExitingItems] = (0,react.useState)(new Map());
    const exitingMapRef = (0,react.useRef)(new Map());
    const prevMatchedIdsRef = (0,react.useRef)(null);
    const prevLiveVersionsRef = (0,react.useRef)(new Map());
    const exitTimersRef = (0,react.useRef)(new Map());
    const reducedMotionRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if ( false || !window.matchMedia) return;
        const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
        reducedMotionRef.current = mq.matches;
        const onChange = ()=>{
            reducedMotionRef.current = mq.matches;
        };
        mq.addEventListener("change", onChange);
        return ()=>mq.removeEventListener("change", onChange);
    }, []);
    // useLayoutEffect (not useEffect): runs before the browser paints, so a
    // departing row is re-added to the exiting set within the same commit it
    // was excluded from `filteredItems` — no visible blank frame in between.
    (0,react.useLayoutEffect)(()=>{
        const currentMatchedIds = new Set(filteredItems.map((i)=>i.id));
        const prevMatchedIds = prevMatchedIdsRef.current;
        const itemsById = new Map(items.map((i)=>[
                i.id,
                i
            ]));
        const exitingMap = exitingMapRef.current;
        let changed = false;
        if (prevMatchedIds) {
            // Flap protection: if a pending exit's item re-matches the filter
            // before its timer fires, cancel the exit and let it settle back to a
            // normal in-place row (ux.md §7 "Error / edge cases").
            for (const id of Array.from(exitingMap.keys())){
                if (currentMatchedIds.has(id)) {
                    const timer = exitTimersRef.current.get(id);
                    if (timer) clearTimeout(timer);
                    exitTimersRef.current.delete(id);
                    exitingMap.delete(id);
                    changed = true;
                }
            }
            for (const id of prevMatchedIds){
                if (currentMatchedIds.has(id) || exitingMap.has(id)) continue;
                const fullItem = itemsById.get(id);
                if (!fullItem) continue; // item removed entirely -- not a filter departure
                const prevVersion = prevLiveVersionsRef.current.get(id);
                const isGenuineLiveChange = fullItem.liveVersion !== undefined && fullItem.liveVersion !== prevVersion;
                if (!isGenuineLiveChange) continue; // bulk resnapshot / manual filter change -> instant
                exitingMap.set(id, fullItem);
                changed = true;
                const duration = reducedMotionRef.current ? 0 : EXIT_TRANSITION_MS;
                const timer = setTimeout(()=>{
                    if (exitingMapRef.current.delete(id)) {
                        setExitingItems(new Map(exitingMapRef.current));
                    }
                    exitTimersRef.current.delete(id);
                }, duration);
                exitTimersRef.current.set(id, timer);
            }
        }
        prevMatchedIdsRef.current = currentMatchedIds;
        for (const item of items){
            prevLiveVersionsRef.current.set(item.id, item.liveVersion);
        }
        if (changed) setExitingItems(new Map(exitingMap));
    }, [
        filteredItems,
        items
    ]);
    // Clear any in-flight exit timers on unmount.
    (0,react.useEffect)(()=>{
        return ()=>{
            for (const timer of exitTimersRef.current.values())clearTimeout(timer);
        };
    }, []);
    // Re-merge still-fading items into the visible set so they keep sorting
    // into a natural position instead of jumping to the end of the list.
    const visibleItems = (0,react.useMemo)(()=>{
        if (exitingItems.size === 0) return filteredItems;
        const presentIds = new Set(filteredItems.map((i)=>i.id));
        const extra = Array.from(exitingItems.values()).filter((i)=>!presentIds.has(i.id));
        return extra.length === 0 ? filteredItems : [
            ...filteredItems,
            ...extra
        ];
    }, [
        filteredItems,
        exitingItems
    ]);
    // Sort items client-side
    const sortedItems = [
        ...visibleItems
    ].sort((a, b)=>{
        let cmp = 0;
        if (sortCol === "title") {
            cmp = a.title.localeCompare(b.title);
        } else if (sortCol === "status") {
            cmp = useBacklogFilters/* ALL_STATUSES */.nY.indexOf(a.status) - useBacklogFilters/* ALL_STATUSES */.nY.indexOf(b.status);
        } else if (sortCol === "priority") {
            cmp = a.priority - b.priority;
        } else if (sortCol === "updatedAt") {
            var _a_updatedAt, _b_updatedAt;
            cmp = ((_a_updatedAt = a.updatedAt) !== null && _a_updatedAt !== void 0 ? _a_updatedAt : "").localeCompare((_b_updatedAt = b.updatedAt) !== null && _b_updatedAt !== void 0 ? _b_updatedAt : "");
        } else if (sortCol === "repoPath") {
            cmp = compareByRepoPath(a, b);
        }
        return sortAsc ? cmp : -cmp;
    });
    // Grouping is applied after sorting so items within each group keep the
    // currently selected sort order (mirrors SessionList's sort-then-group).
    const groups = groupBy === "repoPath" ? groupByRepoPath(sortedItems) : null;
    const handleSortClick = (col)=>{
        if (sortCol === col) {
            setSortAsc((prev)=>!prev);
        } else {
            setSortCol(col);
            setSortAsc(false);
        }
    };
    const handleHeaderKeyDown = (e, col)=>{
        if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            handleSortClick(col);
        }
    };
    const handleRowClick = (itemId)=>{
        const params = new URLSearchParams(searchParams.toString());
        params.set("item", itemId);
        router.push("/backlog?".concat(params.toString()));
    };
    const handleDetailClose = ()=>{
        const params = new URLSearchParams(searchParams.toString());
        params.delete("item");
        const qs = params.toString();
        router.push(qs ? "/backlog?".concat(qs) : "/backlog");
    };
    const handleCreateItem = (0,react.useCallback)(async (data)=>{
        const result = await createBacklogItem(data);
        setShowForm(false);
        if (!result) return;
        await hydrateItemIntoStore(result.item.id);
        // Show vagueness prompt if item was created with skip_triage=true
        if (data.skipTriage) {
            setVaguenessItem(result.item);
            // Navigate to the new item
            const params = new URLSearchParams(searchParams.toString());
            params.set("item", result.item.id);
            router.push("/backlog?".concat(params.toString()));
        } else {
            const params = new URLSearchParams(searchParams.toString());
            params.set("item", result.item.id);
            router.push("/backlog?".concat(params.toString()));
        }
    }, [
        createBacklogItem,
        hydrateItemIntoStore,
        router,
        searchParams
    ]);
    const handleImportGitHubIssue = (0,react.useCallback)(async (e)=>{
        e.preventDefault();
        if (!githubIssueUrl.trim()) return;
        setGithubImportError(null);
        setGithubImporting(true);
        try {
            const result = await importGitHubIssue(githubIssueUrl.trim());
            if (result === null || result === void 0 ? void 0 : result.alreadyClaimedElsewhere) {
                setGithubImportError("Another host already claimed this issue.");
            } else if (result) {
                setShowForm(false);
                setGithubIssueUrl("");
                await hydrateItemIntoStore(result.item.id);
                const params = new URLSearchParams(searchParams.toString());
                params.set("item", result.item.id);
                router.push("/backlog?".concat(params.toString()));
            } else {
                setGithubImportError("Import failed. Check the URL and try again.");
            }
        } catch (err) {
            setGithubImportError(err instanceof Error ? err.message : "Import failed.");
        } finally{
            setGithubImporting(false);
        }
    }, [
        githubIssueUrl,
        importGitHubIssue,
        hydrateItemIntoStore,
        router,
        searchParams
    ]);
    const handlePickerSelect = (0,react.useCallback)(async (owner, repo, issues)=>{
        setGithubImportError(null);
        setGithubImporting(true);
        setGithubImportProgress({
            done: 0,
            total: issues.length
        });
        const createdIds = [];
        const claimed = [];
        let failures = 0;
        let duplicates = 0;
        try {
            for (const issue of issues){
                const url = issue.url || "https://".concat(issue.host || "github.com", "/").concat(owner, "/").concat(repo, "/issues/").concat(issue.number);
                const result = await importGitHubIssue(url.trim());
                if (result === null || result === void 0 ? void 0 : result.alreadyClaimedElsewhere) {
                    claimed.push({
                        issueUrl: url.trim(),
                        claim: result.alreadyClaimedElsewhere
                    });
                } else if (result) {
                    await hydrateItemIntoStore(result.item.id);
                    createdIds.push(result.item.id);
                    if (result.alreadyExisted) duplicates++;
                } else {
                    failures++;
                }
                setGithubImportProgress((prev)=>prev ? {
                        ...prev,
                        done: prev.done + 1
                    } : prev);
            }
        } finally{
            setGithubImporting(false);
            setGithubImportProgress(null);
        }
        setClaimedElsewhere(claimed);
        if (failures > 0) {
            // Leave the modal open (don't setShowForm(false) below) so this error
            // is actually visible — closing the form first would unmount it
            // before the message could ever render.
            setGithubImportError(issues.length === 1 ? "Import failed. Check that this is a real issue, not a pull request, and try again." : "Imported ".concat(createdIds.length, " of ").concat(issues.length, " — ").concat(failures, " failed. Pull requests can't be imported as backlog items."));
            return;
        }
        if (duplicates > 0) {
            // Same reasoning as the failures branch above: leave the modal open
            // so this message is actually visible.
            setGithubImportError(duplicates === issues.length ? "Already imported — no new items created." : "Imported ".concat(createdIds.length - duplicates, " new item").concat(createdIds.length - duplicates === 1 ? "" : "s", "; ").concat(duplicates, " already imported."));
            return;
        }
        // Leave the modal open so the claimed-elsewhere notice (Copy link /
        // Import anyway) stays reachable.
        if (claimed.length > 0) return;
        setShowForm(false);
        // Only navigate to the item detail when a single issue was imported —
        // with multiple, there's no single item to land on.
        if (createdIds.length === 1) {
            const params = new URLSearchParams(searchParams.toString());
            params.set("item", createdIds[0]);
            router.push("/backlog?".concat(params.toString()));
        }
    }, [
        importGitHubIssue,
        hydrateItemIntoStore,
        router,
        searchParams
    ]);
    const handleImportAnyway = (0,react.useCallback)(async (entry, reason)=>{
        const result = await importGitHubIssue(entry.issueUrl, {
            overrideReason: reason
        });
        if (!result || result.alreadyClaimedElsewhere) {
            throw new Error("Import failed. Check the reason and try again.");
        }
        await hydrateItemIntoStore(result.item.id);
        const remaining = claimedElsewhere.filter((e)=>e.issueUrl !== entry.issueUrl);
        setClaimedElsewhere(remaining);
        if (remaining.length === 0) {
            setShowForm(false);
            const params = new URLSearchParams(searchParams.toString());
            params.set("item", result.item.id);
            router.push("/backlog?".concat(params.toString()));
        }
    }, [
        importGitHubIssue,
        hydrateItemIntoStore,
        claimedElsewhere,
        router,
        searchParams
    ]);
    const sortIndicator = (col)=>{
        if (sortCol !== col) return null;
        return sortAsc ? " ↑" : " ↓";
    };
    // Bulk selection (backlogBulkSelectionSlice) and fan-out actions.
    const selectedIds = (0,store/* useAppSelector */.GV)((st)=>st.backlogBulkSelection.selectedIds);
    const selectMode = (0,store/* useAppSelector */.GV)((st)=>st.backlogBulkSelection.selectMode);
    const selectedSet = (0,react.useMemo)(()=>new Set(selectedIds), [
        selectedIds
    ]);
    // Selection can outlive filter changes or live removals; act only on rows still visible.
    const visibleSelectedIds = (0,react.useMemo)(()=>{
        const visible = new Set(sortedItems.map((i)=>i.id));
        return selectedIds.filter((id)=>visible.has(id));
    }, [
        selectedIds,
        sortedItems
    ]);
    const [bulkBusy, setBulkBusy] = (0,react.useState)(false);
    const [bulkFeedback, setBulkFeedback] = (0,react.useState)(null);
    const busyRef = (0,react.useRef)(false);
    const mountedRef = (0,react.useRef)(true);
    const feedbackTimerRef = (0,react.useRef)(null);
    const handleToggleSelect = (0,react.useCallback)((id)=>dispatch((0,backlogBulkSelectionSlice/* toggleSelection */.N0)(id)), [
        dispatch
    ]);
    const showBulkFeedback = (0,react.useCallback)((msg)=>{
        if (!mountedRef.current) return;
        setBulkFeedback(msg);
        if (feedbackTimerRef.current) clearTimeout(feedbackTimerRef.current);
        feedbackTimerRef.current = setTimeout(()=>setBulkFeedback(null), 4000);
    }, []);
    (0,react.useEffect)(()=>{
        mountedRef.current = true;
        return ()=>{
            mountedRef.current = false;
            if (feedbackTimerRef.current) clearTimeout(feedbackTimerRef.current);
            dispatch((0,backlogBulkSelectionSlice/* clearSelection */.wr)());
        };
    }, [
        dispatch
    ]);
    const doneItemIds = (0,react.useMemo)(()=>filteredItems.filter((i)=>i.status === "done").map((i)=>i.id), [
        filteredItems
    ]);
    const runBulkAction = (0,react.useCallback)(async (verb, ids, action)=>{
        if (ids.length === 0 || busyRef.current) return;
        busyRef.current = true;
        setBulkBusy(true);
        try {
            const result = await runBulk(ids, action);
            showBulkFeedback(formatBulkFeedback(verb, result));
            return result;
        } finally{
            busyRef.current = false;
            if (mountedRef.current) setBulkBusy(false);
        }
    }, [
        showBulkFeedback
    ]);
    const handleArchiveDone = (0,react.useCallback)(()=>runBulkAction("Archived", doneItemIds, archiveBacklogItem), [
        runBulkAction,
        doneItemIds,
        archiveBacklogItem
    ]);
    const handleArchiveSelected = (0,react.useCallback)(async ()=>{
        const byId = new Map(items.map((i)=>[
                i.id,
                i
            ]));
        const ids = visibleSelectedIds.filter((id)=>{
            const it = byId.get(id);
            return it !== undefined && it.status !== "archived";
        });
        if (ids.length === 0) {
            showBulkFeedback("Nothing to archive");
            dispatch((0,backlogBulkSelectionSlice/* clearSelection */.wr)());
            return;
        }
        const result = await runBulkAction("Archived", ids, archiveBacklogItem);
        if (!result || !mountedRef.current) return;
        if (result.failed.length === 0) dispatch((0,backlogBulkSelectionSlice/* clearSelection */.wr)());
        else dispatch((0,backlogBulkSelectionSlice/* selectAll */.Ub)(result.failed));
    }, [
        items,
        visibleSelectedIds,
        runBulkAction,
        showBulkFeedback,
        archiveBacklogItem,
        dispatch
    ]);
    const setBulkBusyIfMounted = (0,react.useCallback)((busy)=>{
        if (busy || mountedRef.current) setBulkBusy(busy);
    }, []);
    const handleTriageSelected = (0,react.useCallback)(async ()=>{
        if (busyRef.current) return;
        const byId = new Map(items.map((i)=>[
                i.id,
                i
            ]));
        const eligible = visibleSelectedIds.filter((id)=>{
            const it = byId.get(id);
            return it !== undefined && isBulkTriageEligible(it);
        });
        const skipped = visibleSelectedIds.length - eligible.length;
        if (eligible.length === 0) {
            showBulkFeedback(formatTriageFeedback({
                succeeded: [],
                failed: []
            }, skipped));
            return;
        }
        await runGuarded(busyRef, setBulkBusyIfMounted, async ()=>{
            const result = await runBulkChunked(eligible, (id)=>triggerTriage(id));
            if (!mountedRef.current) return;
            showBulkFeedback(formatTriageFeedback(result, skipped));
            dispatch((0,backlogBulkSelectionSlice/* selectionAfterBulk */.xc)(result.failed));
        });
    }, [
        items,
        visibleSelectedIds,
        triggerTriage,
        setBulkBusyIfMounted,
        showBulkFeedback,
        dispatch
    ]);
    const autoImplementEligibleIds = (0,react.useMemo)(()=>{
        const byId = new Map(items.map((i)=>[
                i.id,
                i
            ]));
        return visibleSelectedIds.filter((id)=>{
            const it = byId.get(id);
            return it !== undefined && isBulkAutoImplementEligible(it);
        });
    }, [
        items,
        visibleSelectedIds
    ]);
    // frozenIds is the snapshot taken when the confirmation dialog opened, not the live selection.
    const handleAutoImplementSelected = (0,react.useCallback)(async (frozenIds)=>{
        // The frozen set is the maximum; drop items that stopped being eligible since the dialog opened.
        const byId = new Map(items.map((i)=>[
                i.id,
                i
            ]));
        const ids = frozenIds.filter((id)=>{
            const it = byId.get(id);
            return it !== undefined && isBulkAutoImplementEligible(it);
        });
        if (ids.length === 0) {
            showBulkFeedback("Nothing eligible for auto-implement");
            return;
        }
        await runGuarded(busyRef, setBulkBusyIfMounted, async ()=>{
            const result = await runBulkSpawnChunked(ids, (id)=>spawnSessionFromItem(id, {
                    autonomous: true
                }));
            if (!mountedRef.current) return;
            const dropped = frozenIds.length - ids.length;
            showBulkFeedback(formatSpawnBulkFeedback(result) + (dropped > 0 ? ", ".concat(dropped, " no longer eligible") : ""));
            dispatch((0,backlogBulkSelectionSlice/* selectionAfterBulk */.xc)(result.failed));
        });
    }, [
        items,
        spawnSessionFromItem,
        setBulkBusyIfMounted,
        showBulkFeedback,
        dispatch
    ]);
    const handleDeleteSelected = (0,react.useCallback)(async ()=>{
        const result = await runBulkAction("Deleted", visibleSelectedIds, deleteBacklogItem);
        if (!result || !mountedRef.current) return;
        if (result.failed.length > 0) dispatch((0,backlogBulkSelectionSlice/* selectAll */.Ub)(result.failed));
        else dispatch((0,backlogBulkSelectionSlice/* clearSelection */.wr)());
    }, [
        visibleSelectedIds,
        runBulkAction,
        deleteBacklogItem,
        dispatch
    ]);
    const renderItemRow = (item)=>{
        const isActive = selectedItemId === item.id;
        const isExiting = exitingItems.has(item.id);
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogTableRow, {
            item: item,
            isActive: isActive,
            isExiting: isExiting,
            isSelected: selectedSet.has(item.id),
            isSelectMode: selectMode,
            onRowClick: handleRowClick,
            onToggleSelect: handleToggleSelect
        }, item.id);
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: pageWrapper,
        "data-testid": "backlog-page",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: pageHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                        className: pageTitle,
                        children: "Backlog"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: headerActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(ConnectionIndicator/* ConnectionIndicator */._, {
                                connectionState: connectionState
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: helpButton,
                                onClick: ()=>{
                                    track({
                                        name: "backlog_open_tour",
                                        category: "user_action",
                                        component: "BacklogPage"
                                    });
                                    resetTour();
                                },
                                "aria-label": "How this page works",
                                "data-testid": "backlog-tour-button",
                                children: "?"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: newItemButton,
                                onClick: ()=>{
                                    track({
                                        name: "backlog_ai_intake",
                                        category: "user_action",
                                        component: "BacklogPage"
                                    });
                                    openOmnibar("backlog: ");
                                },
                                "aria-label": "Create backlog item via AI intake",
                                "data-testid": "backlog-ai-intake-button",
                                style: {
                                    marginRight: "8px",
                                    background: "var(--accent, #6366f1)"
                                },
                                children: "✨ AI Intake"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: newItemButton,
                                onClick: ()=>{
                                    track({
                                        name: "backlog_new_item",
                                        category: "user_action",
                                        component: "BacklogPage"
                                    });
                                    setFormMode("manual");
                                    setShowForm(true);
                                },
                                "aria-label": "Create new backlog item",
                                "data-testid": "backlog-new-item-button",
                                children: "+ New Item"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("nav", {
                className: tabBar,
                "aria-label": "Backlog views",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: "".concat(tab, " ").concat(tabActive),
                        "aria-current": "page",
                        "data-testid": "backlog-tab-list",
                        children: "List"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(AppLink/* AppLink */.E, {
                        href: "/backlog/board",
                        className: tab,
                        "data-testid": "backlog-tab-board",
                        children: "Board"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogFilterBar/* BacklogFilterBar */.l, {
                search: search,
                onSearchChange: setSearch,
                statusFilter: statusFilter,
                onStatusFilterChange: setStatusFilter,
                priorityFilter: priorityFilter,
                onPriorityFilterChange: setPriorityFilter,
                sourceFilter: sourceFilter,
                onSourceFilterChange: setSourceFilter,
                showArchived: showArchived,
                onShowArchivedChange: setShowArchived,
                onResetView: resetViewState,
                showSortGroupControls: true,
                groupBy: groupBy,
                onGroupByChange: setGroupBy,
                doneCount: doneItemIds.length,
                onArchiveDone: handleArchiveDone,
                archiveDoneBusy: bulkBusy
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogBulkActions, {
                selectedCount: visibleSelectedIds.length,
                totalCount: sortedItems.length,
                busy: bulkBusy,
                feedback: bulkFeedback,
                onArchiveSelected: handleArchiveSelected,
                onDeleteSelected: handleDeleteSelected,
                onTriageSelected: handleTriageSelected,
                onAutoImplementSelected: handleAutoImplementSelected,
                autoImplementEligibleIds: autoImplementEligibleIds,
                onSelectAll: ()=>dispatch((0,backlogBulkSelectionSlice/* selectAll */.Ub)(sortedItems.map((i)=>i.id))),
                onClear: ()=>dispatch((0,backlogBulkSelectionSlice/* clearSelection */.wr)())
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: contentArea,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: listPane,
                        children: [
                            loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                role: "status",
                                "aria-label": "Loading backlog items",
                                style: {
                                    padding: "32px",
                                    textAlign: "center",
                                    color: "inherit",
                                    opacity: 0.6
                                },
                                children: "Loading…"
                            }) : sortedItems.length === 0 && items.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogEmptyState, {
                                onCreateItem: ()=>{
                                    setFormMode("manual");
                                    setShowForm(true);
                                }
                            }) : sortedItems.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)(FilterZeroState, {
                                onClearFilters: resetViewState
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                                className: table,
                                "aria-label": "Backlog items",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                                        className: tableHead,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                    scope: "col",
                                                    className: tableHeaderCell,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: srOnly,
                                                        children: "Select"
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                    scope: "col",
                                                    className: "".concat(tableHeaderCell, " ").concat(tableHeaderCellSortable),
                                                    "aria-sort": sortCol === "title" ? sortAsc ? "ascending" : "descending" : "none",
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                        type: "button",
                                                        className: tableHeaderSortButton,
                                                        onClick: ()=>handleSortClick("title"),
                                                        onKeyDown: (e)=>handleHeaderKeyDown(e, "title"),
                                                        children: [
                                                            "Title",
                                                            sortIndicator("title")
                                                        ]
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                    scope: "col",
                                                    className: "".concat(tableHeaderCell, " ").concat(tableHeaderCellSortable),
                                                    "aria-sort": sortCol === "status" ? sortAsc ? "ascending" : "descending" : "none",
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                        type: "button",
                                                        className: tableHeaderSortButton,
                                                        onClick: ()=>handleSortClick("status"),
                                                        onKeyDown: (e)=>handleHeaderKeyDown(e, "status"),
                                                        children: [
                                                            "Status",
                                                            sortIndicator("status")
                                                        ]
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                    scope: "col",
                                                    className: "".concat(tableHeaderCell, " ").concat(tableHeaderCellSortable),
                                                    "aria-sort": sortCol === "priority" ? sortAsc ? "ascending" : "descending" : "none",
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                        type: "button",
                                                        className: tableHeaderSortButton,
                                                        onClick: ()=>handleSortClick("priority"),
                                                        onKeyDown: (e)=>handleHeaderKeyDown(e, "priority"),
                                                        children: [
                                                            "Priority",
                                                            sortIndicator("priority")
                                                        ]
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                    scope: "col",
                                                    className: tableHeaderCell,
                                                    children: "AC"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                    scope: "col",
                                                    className: "".concat(tableHeaderCell, " ").concat(tableHeaderCellSortable),
                                                    "aria-sort": sortCol === "updatedAt" ? sortAsc ? "ascending" : "descending" : "none",
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                        type: "button",
                                                        className: tableHeaderSortButton,
                                                        onClick: ()=>handleSortClick("updatedAt"),
                                                        onKeyDown: (e)=>handleHeaderKeyDown(e, "updatedAt"),
                                                        children: [
                                                            "Updated",
                                                            sortIndicator("updatedAt")
                                                        ]
                                                    })
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                                    scope: "col",
                                                    className: "".concat(tableHeaderCell, " ").concat(tableHeaderCellSortable),
                                                    "aria-sort": sortCol === "repoPath" ? sortAsc ? "ascending" : "descending" : "none",
                                                    "data-testid": "backlog-col-repo-path",
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                        type: "button",
                                                        className: tableHeaderSortButton,
                                                        onClick: ()=>handleSortClick("repoPath"),
                                                        onKeyDown: (e)=>handleHeaderKeyDown(e, "repoPath"),
                                                        children: [
                                                            "Repository",
                                                            sortIndicator("repoPath")
                                                        ]
                                                    })
                                                })
                                            ]
                                        })
                                    }),
                                    groups ? groups.map((group)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("tbody", {
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("tr", {
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("td", {
                                                        colSpan: 7,
                                                        className: groupHeaderCell,
                                                        "data-testid": "backlog-group-header",
                                                        children: [
                                                            group.displayName,
                                                            " (",
                                                            group.items.length,
                                                            ")"
                                                        ]
                                                    })
                                                }),
                                                group.items.map((item)=>renderItemRow(item))
                                            ]
                                        }, group.groupKey)) : /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                                        children: sortedItems.map((item)=>renderItemRow(item))
                                    })
                                ]
                            }),
                            filteredItems.length > 0 && !filteredItems.some((i)=>i.status === "in_progress") && /*#__PURE__*/ (0,jsx_runtime.jsx)(FooterNudge, {})
                        ]
                    }),
                    selectedItemId && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: (0,resizeHandle_css/* resizeHandle */.D)({
                                    direction: "vertical"
                                }),
                                style: {
                                    touchAction: "none"
                                },
                                "aria-label": "Resize detail panel",
                                onPointerDown: handleResizePointerDown,
                                onPointerMove: handleResizePointerMove,
                                onPointerUp: handleResizePointerUp,
                                onPointerCancel: handleResizePointerUp
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("aside", {
                                className: detailPane,
                                style: {
                                    width: detailWidth
                                },
                                "aria-label": "Item detail",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogItemDetail/* BacklogItemDetail */.Uw, {
                                    itemId: selectedItemId,
                                    onClose: handleDetailClose
                                }, selectedItemId)
                            })
                        ]
                    })
                ]
            }),
            showForm && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: modalOverlay,
                role: "dialog",
                "aria-modal": "true",
                "aria-label": "Create new backlog item",
                onClick: (e)=>{
                    if (e.target === e.currentTarget) {
                        setShowForm(false);
                        setGithubIssueUrl("");
                        setGithubImportError(null);
                    }
                },
                "data-testid": "backlog-form-modal",
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: modalBox,
                    onClick: (e)=>e.stopPropagation(),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                            className: modalTitle,
                            children: "New Backlog Item"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            style: {
                                display: "flex",
                                gap: "8px",
                                marginBottom: "16px"
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    onClick: ()=>setFormMode("manual"),
                                    style: {
                                        padding: "6px 16px",
                                        borderRadius: "6px",
                                        border: "1px solid",
                                        cursor: "pointer",
                                        fontSize: "13px",
                                        fontWeight: formMode === "manual" ? 600 : 400,
                                        background: formMode === "manual" ? "var(--primary)" : "transparent",
                                        color: formMode === "manual" ? "var(--primary-text)" : "var(--text-secondary)",
                                        borderColor: formMode === "manual" ? "var(--primary)" : "var(--border-color)"
                                    },
                                    children: "Manual"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    onClick: ()=>setFormMode("github"),
                                    style: {
                                        padding: "6px 16px",
                                        borderRadius: "6px",
                                        border: "1px solid",
                                        cursor: "pointer",
                                        fontSize: "13px",
                                        fontWeight: formMode === "github" ? 600 : 400,
                                        background: formMode === "github" ? "var(--primary)" : "transparent",
                                        color: formMode === "github" ? "var(--primary-text)" : "var(--text-secondary)",
                                        borderColor: formMode === "github" ? "var(--primary)" : "var(--border-color)"
                                    },
                                    children: "Import from GitHub Issue"
                                })
                            ]
                        }),
                        formMode === "manual" ? /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogItemForm/* BacklogItemForm */.T, {
                            onSubmit: handleCreateItem,
                            onCancel: ()=>setShowForm(false)
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(GitHubIssuePicker/* GitHubIssuePicker */.n, {
                                    onSelect: handlePickerSelect,
                                    onCancel: ()=>{
                                        setShowForm(false);
                                        setGithubIssueUrl("");
                                        setGithubImportError(null);
                                        setClaimedElsewhere([]);
                                    },
                                    importing: githubImporting,
                                    importProgress: githubImportProgress
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(ClaimedElsewhereNotice/* ClaimedElsewhereNotice */.E, {
                                    entries: claimedElsewhere,
                                    onImportAnyway: handleImportAnyway
                                }),
                                githubImportError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    style: {
                                        fontSize: "12px",
                                        color: "var(--error)",
                                        margin: "8px 0 0"
                                    },
                                    children: githubImportError
                                })
                            ]
                        }),
                         false && /*#__PURE__*/ 0
                    ]
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogTourModal, {
                isOpen: showTour,
                onComplete: (persist)=>persist ? setTourComplete() : hideTour()
            }),
            vaguenessItem && /*#__PURE__*/ (0,jsx_runtime.jsx)(VaguenessPromptModal, {
                itemTitle: vaguenessItem.title,
                onRefine: ()=>{
                    setVaguenessItem(null);
                    setFormMode("manual");
                    setShowForm(true);
                },
                onProceed: ()=>{
                    const item = vaguenessItem;
                    setVaguenessItem(null);
                    void triggerTriage(item.id);
                }
            })
        ]
    });
}
function BacklogPage() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogPageInner, {})
    });
}


/***/ }),

/***/ 80520:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 81458:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 82362:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 82901:
/***/ ((__unused_webpack_module, exports) => {

"use strict";

Object.defineProperty(exports, "__esModule", ({
    value: true
}));
0 && (0);
function _export(target, all) {
    for(var name in all)Object.defineProperty(target, name, {
        enumerable: true,
        get: all[name]
    });
}
_export(exports, {
    DecodeError: function() {
        return DecodeError;
    },
    MiddlewareNotFoundError: function() {
        return MiddlewareNotFoundError;
    },
    MissingStaticPage: function() {
        return MissingStaticPage;
    },
    NormalizeError: function() {
        return NormalizeError;
    },
    PageNotFoundError: function() {
        return PageNotFoundError;
    },
    SP: function() {
        return SP;
    },
    ST: function() {
        return ST;
    },
    WEB_VITALS: function() {
        return WEB_VITALS;
    },
    execOnce: function() {
        return execOnce;
    },
    getDisplayName: function() {
        return getDisplayName;
    },
    getLocationOrigin: function() {
        return getLocationOrigin;
    },
    getURL: function() {
        return getURL;
    },
    isAbsoluteUrl: function() {
        return isAbsoluteUrl;
    },
    isResSent: function() {
        return isResSent;
    },
    loadGetInitialProps: function() {
        return loadGetInitialProps;
    },
    normalizeRepeatedSlashes: function() {
        return normalizeRepeatedSlashes;
    },
    stringifyError: function() {
        return stringifyError;
    }
});
const WEB_VITALS = [
    'CLS',
    'FCP',
    'FID',
    'INP',
    'LCP',
    'TTFB'
];
function execOnce(fn) {
    let used = false;
    let result;
    return function() {
        for(var _len = arguments.length, args = new Array(_len), _key = 0; _key < _len; _key++){
            args[_key] = arguments[_key];
        }
        if (!used) {
            used = true;
            result = fn(...args);
        }
        return result;
    };
}
// Scheme: https://tools.ietf.org/html/rfc3986#section-3.1
// Absolute URL: https://tools.ietf.org/html/rfc3986#section-4.3
const ABSOLUTE_URL_REGEX = /^[a-zA-Z][a-zA-Z\d+\-.]*?:/;
const isAbsoluteUrl = (url)=>ABSOLUTE_URL_REGEX.test(url);
function getLocationOrigin() {
    const { protocol, hostname, port } = window.location;
    return protocol + "//" + hostname + (port ? ':' + port : '');
}
function getURL() {
    const { href } = window.location;
    const origin = getLocationOrigin();
    return href.substring(origin.length);
}
function getDisplayName(Component) {
    return typeof Component === 'string' ? Component : Component.displayName || Component.name || 'Unknown';
}
function isResSent(res) {
    return res.finished || res.headersSent;
}
function normalizeRepeatedSlashes(url) {
    const urlParts = url.split('?');
    const urlNoQuery = urlParts[0];
    return urlNoQuery // first we replace any non-encoded backslashes with forward
    // then normalize repeated forward slashes
    .replace(/\\/g, '/').replace(/\/\/+/g, '/') + (urlParts[1] ? "?" + urlParts.slice(1).join('?') : '');
}
async function loadGetInitialProps(App, ctx) {
    if (false) { var _App_prototype; }
    // when called from _app `ctx` is nested in `ctx`
    const res = ctx.res || ctx.ctx && ctx.ctx.res;
    if (!App.getInitialProps) {
        if (ctx.ctx && ctx.Component) {
            // @ts-ignore pageProps default
            return {
                pageProps: await loadGetInitialProps(ctx.Component, ctx.ctx)
            };
        }
        return {};
    }
    const props = await App.getInitialProps(ctx);
    if (res && isResSent(res)) {
        return props;
    }
    if (!props) {
        const message = '"' + getDisplayName(App) + '.getInitialProps()" should resolve to an object. But found "' + props + '" instead.';
        throw Object.defineProperty(new Error(message), "__NEXT_ERROR_CODE", {
            value: "E394",
            enumerable: false,
            configurable: true
        });
    }
    if (false) {}
    return props;
}
const SP = typeof performance !== 'undefined';
const ST = SP && [
    'mark',
    'measure',
    'getEntriesByName'
].every((method)=>typeof performance[method] === 'function');
class DecodeError extends Error {
}
class NormalizeError extends Error {
}
class PageNotFoundError extends Error {
    constructor(page){
        super();
        this.code = 'ENOENT';
        this.name = 'PageNotFoundError';
        this.message = "Cannot find module for page: " + page;
    }
}
class MissingStaticPage extends Error {
    constructor(page, message){
        super();
        this.message = "Failed to load static file for page: " + page + " " + message;
    }
}
class MiddlewareNotFoundError extends Error {
    constructor(){
        super();
        this.code = 'ENOENT';
        this.message = "Cannot find the middleware module";
    }
}
function stringifyError(error) {
    return JSON.stringify({
        message: error.message,
        stack: error.stack
    });
} //# sourceMappingURL=utils.js.map


/***/ }),

/***/ 83643:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 83665:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 84265:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   D: () => (/* binding */ resizeHandle)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fpane_2FresizeHandle_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xbjJyNzRmMCB7CiAgcG9zaXRpb246IHJlbGF0aXZlOwogIGZsZXgtc2hyaW5rOiAwOwogIHotaW5kZXg6IDEwOwogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IGNlbnRlcjsKICB1c2VyLXNlbGVjdDogbm9uZTsKICB0cmFuc2l0aW9uOiBiYWNrZ3JvdW5kIDEyMG1zIGVhc2U7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKfQouXzFuMnI3NGYwOjphZnRlciB7CiAgY29udGVudDogJyc7CiAgZGlzcGxheTogYmxvY2s7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozOSk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnplKTsKICBvcGFjaXR5OiAwLjY7CiAgdHJhbnNpdGlvbjogb3BhY2l0eSAxMjBtcyBlYXNlLCBiYWNrZ3JvdW5kIDEyMG1zIGVhc2U7Cn0KLl8xbjJyNzRmMDpob3Zlcjo6YWZ0ZXIgewogIG9wYWNpdHk6IDE7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnptKTsKfQouXzFuMnI3NGYwOmFjdGl2ZTo6YWZ0ZXIgewogIG9wYWNpdHk6IDE7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpuKTsKfQouXzFuMnI3NGYxIHsKICB3aWR0aDogNnB4OwogIGN1cnNvcjogY29sLXJlc2l6ZTsKICBtYXJnaW4tbGVmdDogLTdweDsKICBtYXJnaW4tcmlnaHQ6IC03cHg7CiAgcGFkZGluZy1sZWZ0OiA3cHg7CiAgcGFkZGluZy1yaWdodDogN3B4Owp9Ci5fMW4ycjc0ZjE6OmFmdGVyIHsKICB3aWR0aDogMnB4OwogIGhlaWdodDogMjRweDsKfQouXzFuMnI3NGYyIHsKICBoZWlnaHQ6IDZweDsKICBjdXJzb3I6IHJvdy1yZXNpemU7CiAgbWFyZ2luLXRvcDogLTdweDsKICBtYXJnaW4tYm90dG9tOiAtN3B4OwogIHBhZGRpbmctdG9wOiA3cHg7CiAgcGFkZGluZy1ib3R0b206IDdweDsKfQouXzFuMnI3NGYyOjphZnRlciB7CiAgd2lkdGg6IDI0cHg7CiAgaGVpZ2h0OiAycHg7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(58970);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fpane_2FresizeHandle_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xbjJyNzRmMCB7CiAgcG9zaXRpb246IHJlbGF0aXZlOwogIGZsZXgtc2hyaW5rOiAwOwogIHotaW5kZXg6IDEwOwogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IGNlbnRlcjsKICB1c2VyLXNlbGVjdDogbm9uZTsKICB0cmFuc2l0aW9uOiBiYWNrZ3JvdW5kIDEyMG1zIGVhc2U7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKfQouXzFuMnI3NGYwOjphZnRlciB7CiAgY29udGVudDogJyc7CiAgZGlzcGxheTogYmxvY2s7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozOSk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnplKTsKICBvcGFjaXR5OiAwLjY7CiAgdHJhbnNpdGlvbjogb3BhY2l0eSAxMjBtcyBlYXNlLCBiYWNrZ3JvdW5kIDEyMG1zIGVhc2U7Cn0KLl8xbjJyNzRmMDpob3Zlcjo6YWZ0ZXIgewogIG9wYWNpdHk6IDE7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnptKTsKfQouXzFuMnI3NGYwOmFjdGl2ZTo6YWZ0ZXIgewogIG9wYWNpdHk6IDE7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpuKTsKfQouXzFuMnI3NGYxIHsKICB3aWR0aDogNnB4OwogIGN1cnNvcjogY29sLXJlc2l6ZTsKICBtYXJnaW4tbGVmdDogLTdweDsKICBtYXJnaW4tcmlnaHQ6IC03cHg7CiAgcGFkZGluZy1sZWZ0OiA3cHg7CiAgcGFkZGluZy1yaWdodDogN3B4Owp9Ci5fMW4ycjc0ZjE6OmFmdGVyIHsKICB3aWR0aDogMnB4OwogIGhlaWdodDogMjRweDsKfQouXzFuMnI3NGYyIHsKICBoZWlnaHQ6IDZweDsKICBjdXJzb3I6IHJvdy1yZXNpemU7CiAgbWFyZ2luLXRvcDogLTdweDsKICBtYXJnaW4tYm90dG9tOiAtN3B4OwogIHBhZGRpbmctdG9wOiA3cHg7CiAgcGFkZGluZy1ib3R0b206IDdweDsKfQouXzFuMnI3NGYyOjphZnRlciB7CiAgd2lkdGg6IDI0cHg7CiAgaGVpZ2h0OiAycHg7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Fpane_2FresizeHandle_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xbjJyNzRmMCB7CiAgcG9zaXRpb246IHJlbGF0aXZlOwogIGZsZXgtc2hyaW5rOiAwOwogIHotaW5kZXg6IDEwOwogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IGNlbnRlcjsKICB1c2VyLXNlbGVjdDogbm9uZTsKICB0cmFuc2l0aW9uOiBiYWNrZ3JvdW5kIDEyMG1zIGVhc2U7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycno3KTsKfQouXzFuMnI3NGYwOjphZnRlciB7CiAgY29udGVudDogJyc7CiAgZGlzcGxheTogYmxvY2s7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozOSk7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnplKTsKICBvcGFjaXR5OiAwLjY7CiAgdHJhbnNpdGlvbjogb3BhY2l0eSAxMjBtcyBlYXNlLCBiYWNrZ3JvdW5kIDEyMG1zIGVhc2U7Cn0KLl8xbjJyNzRmMDpob3Zlcjo6YWZ0ZXIgewogIG9wYWNpdHk6IDE7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnptKTsKfQouXzFuMnI3NGYwOmFjdGl2ZTo6YWZ0ZXIgewogIG9wYWNpdHk6IDE7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnpuKTsKfQouXzFuMnI3NGYxIHsKICB3aWR0aDogNnB4OwogIGN1cnNvcjogY29sLXJlc2l6ZTsKICBtYXJnaW4tbGVmdDogLTdweDsKICBtYXJnaW4tcmlnaHQ6IC03cHg7CiAgcGFkZGluZy1sZWZ0OiA3cHg7CiAgcGFkZGluZy1yaWdodDogN3B4Owp9Ci5fMW4ycjc0ZjE6OmFmdGVyIHsKICB3aWR0aDogMnB4OwogIGhlaWdodDogMjRweDsKfQouXzFuMnI3NGYyIHsKICBoZWlnaHQ6IDZweDsKICBjdXJzb3I6IHJvdy1yZXNpemU7CiAgbWFyZ2luLXRvcDogLTdweDsKICBtYXJnaW4tYm90dG9tOiAtN3B4OwogIHBhZGRpbmctdG9wOiA3cHg7CiAgcGFkZGluZy1ib3R0b206IDdweDsKfQouXzFuMnI3NGYyOjphZnRlciB7CiAgd2lkdGg6IDI0cHg7CiAgaGVpZ2h0OiAycHg7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _vanilla_extract_recipes_createRuntimeFn__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(8462);



var resizeHandle = (0,_vanilla_extract_recipes_createRuntimeFn__WEBPACK_IMPORTED_MODULE_2__.c)({defaultClassName:'_1n2r74f0',variantClassNames:{direction:{vertical:'_1n2r74f1',horizontal:'_1n2r74f2'}},defaultVariants:{},compoundVariants:[]});

/***/ }),

/***/ 85529:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  n: () => (/* binding */ GitHubIssuePicker)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-redux@9.2.0+5436edeee5f9f735/node_modules/react-redux/dist/react-redux.mjs
var react_redux = __webpack_require__(55324);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/reselect@5.1.1/node_modules/reselect/dist/reselect.mjs
var reselect = __webpack_require__(13009);
// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
;// ./src/lib/utils/issuePickerCache.ts
// TTL cache for the GitHub issue picker — keyed under the page's origin so
// multiple instances (dev / prod) never share stale data.
const TTL_MS = 5 * 60 * 1000; // 5 minutes
function cacheKey(suffix) {
    if (false) {}
    return "ssq:".concat(window.location.origin, ":").concat(suffix);
}
function readEntry(key) {
    try {
        const raw = localStorage.getItem(key);
        if (!raw) return null;
        const entry = JSON.parse(raw);
        if (Date.now() > entry.expiry) {
            localStorage.removeItem(key);
            return null;
        }
        return entry.data;
    } catch (e) {
        return null;
    }
}
function writeEntry(key, data) {
    try {
        const entry = {
            data,
            expiry: Date.now() + TTL_MS
        };
        localStorage.setItem(key, JSON.stringify(entry));
    } catch (e) {
    // localStorage full or unavailable — silently skip
    }
}
// ─── Repo list cache ────────────────────────────────────────────────────────
const REPOS_KEY = ()=>cacheKey("github:repos");
function getCachedRepos() {
    return readEntry(REPOS_KEY());
}
function setCachedRepos(repos) {
    writeEntry(REPOS_KEY(), repos);
}
// ─── Issue list cache ───────────────────────────────────────────────────────
function issuesKey(owner, repo, state) {
    return cacheKey("github:issues:".concat(owner, "/").concat(repo, ":").concat(state));
}
function getCachedIssues(owner, repo, state) {
    return readEntry(issuesKey(owner, repo, state));
}
function setCachedIssues(owner, repo, state, issues) {
    writeEntry(issuesKey(owner, repo, state), issues);
}
// ─── Recent repos (multi-entry history, up to 5) ───────────────────────────
const RECENT_REPOS_KEY = ()=>cacheKey("github:recentRepos");
const RECENT_REPOS_MAX = 5;
function getRecentRepos() {
    if (false) {}
    try {
        const raw = localStorage.getItem(RECENT_REPOS_KEY());
        if (!raw) return [];
        return JSON.parse(raw);
    } catch (e) {
        return [];
    }
}
function addRecentRepo(owner, repo) {
    if (false) {}
    try {
        const existing = getRecentRepos();
        const key = "".concat(owner, "/").concat(repo);
        const filtered = existing.filter((r)=>"".concat(r.owner, "/").concat(r.repo) !== key);
        const updated = [
            {
                owner,
                repo
            },
            ...filtered
        ].slice(0, RECENT_REPOS_MAX);
        localStorage.setItem(RECENT_REPOS_KEY(), JSON.stringify(updated));
    } catch (e) {
    // ignore
    }
}

;// ./src/lib/hooks/useGitHubIssuePicker.ts
/* __next_internal_client_entry_do_not_use__ useGitHubIssuePicker auto */ 





// ─── Hook ────────────────────────────────────────────────────────────────────
const ISSUE_DEBOUNCE_MS = 150;
// Derives local-path repos from the session list. Memoized with createSelector
// (module-level, not inline in the hook — an inline selector would rebuild the
// memoization cache every render, defeating the point): selectAllSessions's
// result only changes reference when the sessions slice actually changes, so
// this only recomputes then, returning the SAME array reference on every other
// render instead of a fresh one. An inline `useSelector((state) => { ... build
// a new array ... })` — the previous shape — returns a new array reference on
// every invocation regardless of whether the input changed, which is the
// classic react-redux anti-pattern that produces the "The result of
// getSnapshot should be cached" class of bug: useSyncExternalStore treats each
// new reference as a genuine store change and can loop re-rendering trying to
// reach a stable snapshot, intermittently manifesting as a crash under CPU
// contention (jsdom + React 19's stricter consistency checks) without ever
// reproducing reliably in isolation — see useGitHubIssuePicker.multiselect.test.ts.
const selectLocalRepos = (0,reselect/* createSelector */.Mz)([
    sessionsSlice/* selectAllSessions */.rt
], (sessions)=>{
    const seen = new Set();
    const results = [];
    for (const s of sessions){
        if (!s.repoRoot) continue;
        const parts = s.repoRoot.split("/");
        var _parts_;
        const repo = (_parts_ = parts[parts.length - 1]) !== null && _parts_ !== void 0 ? _parts_ : "";
        var _parts_1;
        const owner = (_parts_1 = parts[parts.length - 2]) !== null && _parts_1 !== void 0 ? _parts_1 : "";
        const key = "".concat(owner, "/").concat(repo);
        if (seen.has(key)) continue;
        seen.add(key);
        var _s_title;
        results.push({
            owner,
            repo,
            isLocal: true,
            localPath: s.repoRoot,
            description: (_s_title = s.title) !== null && _s_title !== void 0 ? _s_title : "",
            host: ""
        });
    }
    return results;
});
function useGitHubIssuePicker(param) {
    let { searchGitHubRepos, listGitHubIssues, onSelect } = param;
    const [phase, setPhase] = (0,react.useState)("repo");
    const [repoQuery, setRepoQuery] = (0,react.useState)("");
    const [repos, setRepos] = (0,react.useState)([]);
    const [reposLoading, setReposLoading] = (0,react.useState)(false);
    const [selectedRepo, setSelectedRepo] = (0,react.useState)(null);
    const [issueSearch, setIssueSearch] = (0,react.useState)("");
    const [issueState, setIssueState] = (0,react.useState)("open");
    const [issues, setIssues] = (0,react.useState)([]);
    const [issuesLoading, setIssuesLoading] = (0,react.useState)(false);
    const [authError, setAuthError] = (0,react.useState)(false);
    // In-memory cache of ALL fetched repos — filtered client-side as user types.
    const allReposRef = (0,react.useRef)([]);
    const issueGenRef = (0,react.useRef)(0);
    // Pull local-path repos from Redux session list for the "local repos" tier.
    const localRepos = (0,react_redux/* useSelector */.d4)(selectLocalRepos);
    // ─── Repo phase: one-time eager fetch, then filter client-side ─────────────
    // Step 1: fetch full repo list once on mount (or from localStorage cache).
    (0,react.useEffect)(()=>{
        // Seed from localStorage cache immediately so the list appears before the
        // network response arrives.
        const cached = getCachedRepos();
        if (cached && cached.length > 0) {
            const mapped = cached.map((r)=>{
                var _r_host;
                return {
                    ...r,
                    isLocal: false,
                    localPath: "",
                    host: (_r_host = r.host) !== null && _r_host !== void 0 ? _r_host : ""
                };
            });
            allReposRef.current = mapped;
            setRepos(mapped);
        }
        let cancelled = false;
        setReposLoading(true);
        searchGitHubRepos("", 100).then((results)=>{
            if (cancelled) return;
            allReposRef.current = results;
            setRepos(results);
            setCachedRepos(results.map((r)=>({
                    owner: r.owner,
                    repo: r.repo,
                    description: r.description,
                    host: r.host
                })));
            setAuthError(false);
        }).catch((err)=>{
            if (cancelled) return;
            if (err instanceof useBacklogService/* GitHubAuthError */.Kz) setAuthError(true);
        }).finally(()=>{
            if (!cancelled) setReposLoading(false);
        });
        return ()=>{
            cancelled = true;
        };
    // Run once on mount — searchGitHubRepos identity is stable from the service hook.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    // ─── Issue phase: fetch on search/state/repo change ────────────────────────
    (0,react.useEffect)(()=>{
        if (phase !== "issue" || !selectedRepo) return;
        const gen = ++issueGenRef.current;
        const { owner, repo, host } = selectedRepo;
        // Check cache (only for no-search case).
        if (issueSearch === "") {
            const cached = getCachedIssues(owner, repo, issueState);
            if (cached) {
                if (gen === issueGenRef.current) {
                    setIssues(cached.map((c)=>{
                        var _c_isPR, _c_host;
                        return {
                            ...c,
                            isPR: (_c_isPR = c.isPR) !== null && _c_isPR !== void 0 ? _c_isPR : false,
                            host: (_c_host = c.host) !== null && _c_host !== void 0 ? _c_host : ""
                        };
                    }));
                }
                return;
            }
        }
        let timer = null;
        timer = setTimeout(async ()=>{
            if (gen !== issueGenRef.current) return;
            setIssuesLoading(true);
            try {
                const results = await listGitHubIssues(owner, repo, {
                    state: issueState,
                    search: issueSearch,
                    limit: 50,
                    host
                });
                if (gen !== issueGenRef.current) return;
                setIssues(results);
                if (issueSearch === "") {
                    setCachedIssues(owner, repo, issueState, results);
                }
                setAuthError(false);
            } catch (err) {
                if (gen !== issueGenRef.current) return;
                if (err instanceof useBacklogService/* GitHubAuthError */.Kz) {
                    setAuthError(true);
                }
            } finally{
                if (gen === issueGenRef.current) setIssuesLoading(false);
            }
        }, ISSUE_DEBOUNCE_MS);
        return ()=>{
            if (timer !== null) clearTimeout(timer);
        };
    }, [
        phase,
        selectedRepo,
        issueSearch,
        issueState,
        listGitHubIssues
    ]);
    // ─── Actions ────────────────────────────────────────────────────────────────
    const selectRepo = (0,react.useCallback)((repo)=>{
        setSelectedRepo(repo);
        addRecentRepo(repo.owner, repo.repo);
        setIssues([]);
        setIssueSearch("");
        setPhase("issue");
    }, []);
    const selectIssues = (0,react.useCallback)((issues)=>{
        if (!selectedRepo || issues.length === 0) return;
        onSelect(selectedRepo.owner, selectedRepo.repo, issues);
    }, [
        selectedRepo,
        onSelect
    ]);
    const goBack = (0,react.useCallback)(()=>{
        setPhase("repo");
        setSelectedRepo(null);
        setIssues([]);
        setIssueSearch("");
    }, []);
    const reset = (0,react.useCallback)(()=>{
        setPhase("repo");
        setRepoQuery("");
        setRepos(allReposRef.current);
        setSelectedRepo(null);
        setIssues([]);
        setIssueSearch("");
        setIssueState("open");
        setAuthError(false);
        issueGenRef.current = 0;
    }, []);
    // Build the full merged + filtered + history-tiered repo list.
    // Returns [repos, historyCount] — historyCount is the number of leading history entries.
    const mergedRepos = (0,react.useCallback)((fetched)=>{
        // Enrich fetched repos with local path info from session list.
        const localMap = new Map(localRepos.map((r)=>[
                "".concat(r.owner, "/").concat(r.repo),
                r.localPath
            ]));
        const enrichedFetched = fetched.map((r)=>{
            const localPath = localMap.get("".concat(r.owner, "/").concat(r.repo));
            return localPath ? {
                ...r,
                isLocal: true,
                localPath
            } : r;
        });
        // Local-only: session repos not returned by the GitHub API.
        const fetchedKeys = new Set(fetched.map((r)=>"".concat(r.owner, "/").concat(r.repo)));
        const localOnly = localRepos.filter((r)=>!fetchedKeys.has("".concat(r.owner, "/").concat(r.repo)));
        // Full candidate list.
        const all = [
            ...enrichedFetched,
            ...localOnly
        ];
        // Apply query filter.
        const q = repoQuery.trim().toLowerCase();
        const filtered = q === "" ? all : all.filter((r)=>{
            var _r_description;
            return "".concat(r.owner, "/").concat(r.repo).toLowerCase().includes(q) || ((_r_description = r.description) === null || _r_description === void 0 ? void 0 : _r_description.toLowerCase().includes(q));
        });
        // History tier: recent repos (in recency order) that appear in filtered results.
        const recentList = getRecentRepos();
        const historyRepos = recentList.map((recent)=>filtered.find((f)=>f.owner === recent.owner && f.repo === recent.repo)).filter((r)=>r !== undefined);
        const historyKeySet = new Set(historyRepos.map((r)=>"".concat(r.owner, "/").concat(r.repo)));
        const otherRepos = filtered.filter((r)=>!historyKeySet.has("".concat(r.owner, "/").concat(r.repo)));
        return [
            [
                ...historyRepos,
                ...otherRepos
            ],
            historyRepos.length
        ];
    }, [
        localRepos,
        repoQuery
    ]);
    const [computedRepos, repoHistoryCount] = mergedRepos(repos);
    return {
        phase,
        repoQuery,
        repos: computedRepos,
        repoHistoryCount,
        reposLoading,
        selectedRepo,
        setRepoQuery,
        selectRepo,
        issueSearch,
        issueState,
        issues,
        issuesLoading,
        setIssueSearch,
        setIssueState,
        selectIssues,
        goBack,
        authError,
        reset
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FGitHubIssuePicker.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VZ3W6jOhC-71P4XBxpq5UrSLZJ6-o8yWp1ZGCSeGNsakzzc7TvfoQJ4IBNIMl2ryJlhvH8fjNjP2kQ4p0H6L8HhBKWZ5weCFpx2L89IPOLE6Yg1kwKgmLJi1SUlDXNCPqg6gvG_4avcJyp4zx4fHv49fBUSQzdEilna4GZhjQnKAahQXnEzY62uNnN4g6PJSWjScLE2kF1HY9QROPtWslCJARpRUWeUQVCl6RYcqm6gsLqK6kSUASF2R7lkrOkwwUWF1Y0YUXe8-bS8Kyk0DhnR-jRI0OPC5WXWmSSVca3HiMb-QHK-M224lzMy6PXlLNwzs_9zwRnAvAfCcM5w_KTHW7oO2DrjSboOQjGue-bcd-OJXpDUBgEfw854dh1gqmsISeE1B_GMBrloe3dPCQLXSYHQUIKqITuS35j6umASO5t75CVjIu8StWKwWkKt136PBWyUrrHm1PgZt-CzLCXJbLicocPBNFCy_sm03DaNqYs7oKV0xOqBx4j8MaX6qgNrdYyHfZeaznhNNc43jCe2OGvhVQpZLGPxbT2k-9UMYpz4BBrSP7RqoAfUwQsDbOr7Ms4ERSisMmcOpsI2rAkAZN2GvYatwTgnGU5y0vSbsM04DyjsamWnaKZdfBLe7AzIEM1_3gvbWorAxQ0VpaFdIKy5xLJGo1fr9d41kLLRxkwobEoUlAsJkjTqOBUlX_kLo2a86mzSUVcxtuz8ihT09mWhmt60YW_8AQiruS4quVOmR6sKLx0wOwu6Re5QcmPPk64asTF48VdGhE-cyBo9K_g6XPHx7aBTkpLd9V5B0W42bJzhtSf3--fZ16nKhdlVTYmr4ZMnldBMIVjctpuvpfAy9s6m7PXg2f7Jj1QSiocrfu-rSiltneb4HzFY46ybdl0Ab9GxBFQew0-zkaj9Lj693YSNiJId06Qmq4PHAhimnIWWxr9vDizoL9YmkmlaYkvzXdb853MaMz0gaDg6dkulwRWtOBn_J4R6wy9Gm5u-OqpOqyC71ey7lxqzYQLaZqxCucbxcSWILtq0162nQ7sjyhde80g0KhpyRT96W7Zb-BjoDvtxVBIlVJuHSavWaJ_FrlmqwOOpdAgtE2yXPHqmEROhnDQGpTp8yaZg6dgBun9BqJ-uDPFUqoO_To-EVq0cgyXTNRjzewEJqbQTAhWUqUEFVkGKqa5vRlkv8-1TZY5fPu7PDgRC6finMPNjSffbx7iHQtDWfCYw0q3W_aYWb9RSk1d9q9ZRtW0ZbS6r-hb65-09FW3rDMb5Oxt04qhDZPF1MH9hli7No7hteLDaDcw_Po3bUef3nliMADe3elzwLor7kcaQAhccNDqbXXZy7eHe4-Vl5QxwJlALBWtMqu22WqPywla7y2tGxF2Nz3c4Rmhu_51VZoHjo0ExZTHX_qMXw26oa_9L0YAb2PWccCsMr0Jqi9LfPa0zyLVS0s9AgVX119VLGe1linAtSLNvWalX6SAbvFOquStcxMaLrw3oa3S0x5zem3V6Icj0DsAb9jPnnvC2W1dqBU0n7bdXrxqv_N2e8Wjw-L06OBtM-GcJCynEYdkcPgXstxfuNxB8vbw63_kBboIChwAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FGitHubIssuePicker_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VZ3W6jOhC_71P4XBxpq5UrSLZJ6_o8yWp1ZGCSeGNsakzzc7TvfoQJ4IBNIMl2ryJlhvH8fjNjP2kQ4p0H6L8HhBKWZ5weCFpx2L89IPOLE6Yg1kwKgmLJi1SUlDXNCPqg6gvG_4avcJyp4zx4fHv49fBUSQzdEilna4GZhjQnKAahQXnEzY62uNnN4g6PJSWjScLE2kF1HY9QROPtWslCJARpRUWeUQVCl6RYcqm6gsLqK6kSUASF2R7lkrOkwwUWF1Y0YUXe8_bS8Kyk0DhnR_jRI0OPC5WXWmSSVca3HiMb_QHK_M224lzMy6PXlLNwzs_9zwRnAvAfCcM5w_KTHW7oO2DrjSboOQjGue_bcd_OJXpDUBgEfw854dh1gqmsISeE1B_GMBrloe3dPCQLXSYHQUIKqITuS35j6umASO5t75CVjIu8StWKwWkKt136PBWyUrrHm1PgZt_CzLCXJbLicocPBNFCy_sm03DaNqYs7oKV0xOqBx4j8MaX6qgNrdYyHfZeaznhNNc43jCe2OGvhVQpZLGPxbT2k_9UMYpz4BBrSP7RqoAfUwQsDbOr7Ms4ERSisMmcOpsI2rAkAZN2GvYatwTgnGU5y0vSbsM04DyjsamWnaKZdfBLe7AzIEM1_3gvbWorAxQ0VpaFdIKy5xLJGo1fr9d41kLLRxkwobEoUlAsJkjTqOBUlX_kLo2a86mzSUVcxtuz8ihT09mWhmt60YW_8AQiruS4quVOmR6sKLx0wOwu6Re5QcmPPk64asTF48VdGhE_cyBo9K_g6XPHx7aBTkpLd9V5B0W42bJzhtSf3_fZ16nKhdlVTYmr4ZMnldBMIVjctpuvpfAy9s6m7PXg2f7Jj1QSiocrfu_rSiltneb4HzFY46ybdl0Ab9GxBFQew0_zkaj9Lj693YSNiJId06Qmq4PHAhimnIWWxr9vDizoL9YmkmlaYkvzXdb853MaMz0gaDg6dkulwRWtOBn_J4R6wy9Gm5u_OqpOqyC71ey7lxqzYQLaZqxCucbxcSWILtq0162nQ7sjyhde80g0KhpyRT96W7Zb_BjoDvtxVBIlVJuHSavWaJ_FrlmqwOOpdAgtE2yXPHqmEROhnDQGpTp8yaZg6dgBun9BqJ_uDPFUqoO_To_EVq0cgyXTNRjzewEJqbQTAhWUqUEFVkGKqa5vRlkv8_1TZY5fPu7PDgRC6finMPNjSffbx7iHQtDWfCYw0q3W_aYWb9RSk1d9q9ZRtW0ZbS6r_hb65_09FW3rDMb5Oxt04qhDZPF1MH9hli7No7hteLDaDcw_Po3bUef3nliMADe3elzwLor7kcaQAhccNDqbXXZy7eHe4_Vl5QxwJlALBWtMqu22WqPywla7y2tGxF2Nz3c4Rmhu_51VZoHjo0ExZTHX_qMXw26oa_9L0YAb2PWccCsMr0Jqi9LfPa0zyLVS0s9AgVX119VLGe1linAtSLNvWalX6SAbvFOquStcxMaLrw3oa3S0x5zem3V6Icj0DsAb9jPnnvC2W1dqBU0n7bdXrxqv_N2e8Wjw_L06OBtM_GcJCynEYdkcPgXstxfuNxB8vbw63_kBboIChwAAA_22_7D = __webpack_require__(10301);
;// ./src/components/backlog/GitHubIssuePicker.css.ts


var authErrorBox = 'tennqlg';
var backButton = 'tennql2';
var container = 'tennql0';
var emptyState = 'tennqlf';
var expandToggle = 'tennqlw';
var filterBar = 'tennqlb';
var historyDivider = 'tennqll';
var historyIcon = 'tennqlm';
var importBar = 'tennql11';
var importBarHint = 'tennql12';
var importButton = 'tennql13';
var issueAuthor = 'tennqlv';
var issueCheckbox = 'tennqls';
var issueExpandedBody = 'tennql10';
var issueExpandedLabels = 'tennqlz';
var issueExpandedPanel = 'tennqly';
var issueMainCol = 'tennqlt';
var issueNumber = 'tennql9';
var issueRowWrapper = 'tennqlr';
var issueSubMeta = 'tennqlu';
var issueTypeBadge = 'tennqlp';
var labelBadge = 'tennqla';
var listContainer = 'tennql5';
var listItem = 'tennql6';
var listItemDisabled = 'tennqlk';
var listItemMeta = 'tennql8';
var listItemName = 'tennql7';
var listItemSelected = 'tennqlj';
var loadingText = 'tennqli';
var localBadge = 'tennqlh';
var matchHighlight = 'tennqln';
var openLink = 'tennqlx';
var phaseHeader = 'tennql1';
var prTypeBadge = 'tennqlo';
var relativeDate = 'tennqlq';
var repoChip = 'tennql3';
var searchInput = 'tennql4';
var stateButton = {inactive:'tennqld',active:'tennqle'};
var stateToggle = 'tennqlc';
;// ./src/components/backlog/GitHubIssuePicker.tsx
// +feature: backlog:github-issue-picker
/* __next_internal_client_entry_do_not_use__ GitHubIssuePicker auto */ 




// ─── Helpers ─────────────────────────────────────────────────────────────────
function highlightMatch(text, query) {
    const q = query.trim().toLowerCase();
    if (!q) return text;
    const idx = text.toLowerCase().indexOf(q);
    if (idx === -1) return text;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            text.slice(0, idx),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("mark", {
                className: matchHighlight,
                children: text.slice(idx, idx + q.length)
            }),
            text.slice(idx + q.length)
        ]
    });
}
function relativeTime(iso) {
    if (!iso) return "";
    const diff = Date.now() - new Date(iso).getTime();
    const s = Math.floor(diff / 1000);
    if (s < 60) return "".concat(s, "s ago");
    const m = Math.floor(s / 60);
    if (m < 60) return "".concat(m, "m ago");
    const h = Math.floor(m / 60);
    if (h < 24) return "".concat(h, "h ago");
    const d = Math.floor(h / 24);
    if (d < 30) return "".concat(d, "d ago");
    const mo = Math.floor(d / 30);
    if (mo < 12) return "".concat(mo, "mo ago");
    return "".concat(Math.floor(mo / 12), "y ago");
}
// ─── Component ────────────────────────────────────────────────────────────────
function GitHubIssuePicker(param) {
    let { onSelect, onCancel, importing, importProgress } = param;
    const { searchGitHubRepos, listGitHubIssues } = (0,useBacklogService/* useBacklogService */.gG)();
    const picker = useGitHubIssuePicker({
        searchGitHubRepos,
        listGitHubIssues,
        onSelect
    });
    const searchRef = (0,react.useRef)(null);
    // Focus input when phase changes.
    (0,react.useEffect)(()=>{
        var _searchRef_current;
        (_searchRef_current = searchRef.current) === null || _searchRef_current === void 0 ? void 0 : _searchRef_current.focus();
    }, [
        picker.phase
    ]);
    // Two-level Escape: issue → repo → onCancel.
    const handleKeyDown = (0,react.useCallback)((e)=>{
        if (e.key !== "Escape") return;
        e.preventDefault();
        if (picker.phase === "issue") {
            picker.goBack();
        } else {
            onCancel();
        }
    }, [
        picker,
        onCancel
    ]);
    if (picker.authError) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: authErrorBox,
                    children: [
                        "No GitHub token configured. Set ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: "GITHUB_TOKEN"
                        }),
                        " or ",
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                            children: "GH_TOKEN"
                        }),
                        " to enable the GitHub issue picker."
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    style: {
                        display: "flex",
                        justifyContent: "flex-end"
                    },
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        onClick: onCancel,
                        className: backButton,
                        children: "Close"
                    })
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        onKeyDown: handleKeyDown,
        children: [
            picker.phase === "repo" ? /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPhase, {
                picker: picker,
                searchRef: searchRef
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(IssuePhase, {
                picker: picker,
                searchRef: searchRef,
                importing: importing,
                importProgress: importProgress
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                style: {
                    display: "flex",
                    justifyContent: "flex-end"
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    onClick: onCancel,
                    className: backButton,
                    disabled: importing,
                    children: "Cancel"
                })
            })
        ]
    });
}
// ─── Repo phase ───────────────────────────────────────────────────────────────
function RepoPhase(param) {
    let { picker, searchRef } = param;
    const [selectedIndex, setSelectedIndex] = (0,react.useState)(-1);
    const listRef = (0,react.useRef)(null);
    const repos = picker.repos;
    // Reset selection when the repo list changes (query update or initial load).
    (0,react.useEffect)(()=>{
        setSelectedIndex(-1);
    }, [
        repos
    ]);
    // Scroll keyboard-selected item into view.
    (0,react.useEffect)(()=>{
        var _items_selectedIndex;
        if (selectedIndex < 0 || !listRef.current) return;
        const items = listRef.current.querySelectorAll('[role="option"]');
        (_items_selectedIndex = items[selectedIndex]) === null || _items_selectedIndex === void 0 ? void 0 : _items_selectedIndex.scrollIntoView({
            block: "nearest"
        });
    }, [
        selectedIndex
    ]);
    const handleKeyDown = (e)=>{
        if (repos.length === 0) return;
        if (e.key === "ArrowDown") {
            e.preventDefault();
            setSelectedIndex((i)=>Math.min(i + 1, repos.length - 1));
        } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setSelectedIndex((i)=>Math.max(i - 1, 0));
        } else if (e.key === "Enter") {
            e.preventDefault();
            const idx = selectedIndex >= 0 ? selectedIndex : 0;
            const repo = repos[idx];
            if (repo) picker.selectRepo(repo);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: phaseHeader,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    style: {
                        fontSize: "13px",
                        fontWeight: 600
                    },
                    children: "Select a repository"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                ref: searchRef,
                className: searchInput,
                type: "text",
                placeholder: "Search repos…",
                value: picker.repoQuery,
                onChange: (e)=>picker.setRepoQuery(e.target.value),
                onKeyDown: handleKeyDown,
                "aria-label": "Search GitHub repositories",
                "aria-autocomplete": "list",
                "aria-controls": "repo-list",
                autoComplete: "off",
                autoFocus: true
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: "repo-list",
                ref: listRef,
                role: "listbox",
                "aria-label": "GitHub repositories",
                className: listContainer,
                children: picker.reposLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: loadingText,
                    children: "Loading…"
                }) : repos.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: emptyState,
                    children: picker.repoQuery ? "No repos found." : "No repos available."
                }) : repos.map((repo, idx)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)(react.Fragment, {
                        children: [
                            idx === picker.repoHistoryCount && picker.repoHistoryCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: historyDivider,
                                role: "separator"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                role: "option",
                                "aria-selected": idx === selectedIndex,
                                className: idx === selectedIndex ? "".concat(listItem, " ").concat(listItemSelected) : listItem,
                                onMouseDown: (e)=>{
                                    e.preventDefault();
                                    picker.selectRepo(repo);
                                },
                                onMouseEnter: ()=>setSelectedIndex(idx),
                                children: [
                                    idx < picker.repoHistoryCount && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: historyIcon,
                                        children: "\uD83D\uDD52"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: listItemName,
                                        children: highlightMatch("".concat(repo.owner, "/").concat(repo.repo), picker.repoQuery)
                                    }),
                                    repo.isLocal && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: localBadge,
                                        children: "local"
                                    }),
                                    repo.host && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: localBadge,
                                        children: repo.host
                                    }),
                                    repo.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: listItemMeta,
                                        children: highlightMatch(repo.description, picker.repoQuery)
                                    })
                                ]
                            })
                        ]
                    }, "".concat(repo.owner, "/").concat(repo.repo)))
            })
        ]
    });
}
// ─── Issue phase ──────────────────────────────────────────────────────────────
function IssuePhase(param) {
    let { picker, searchRef, importing, importProgress } = param;
    var _picker_selectedRepo, _picker_selectedRepo1, _picker_selectedRepo2;
    const states = [
        "open",
        "closed",
        "all"
    ];
    const [selected, setSelected] = (0,react.useState)(new Set());
    const [expanded, setExpanded] = (0,react.useState)(new Set());
    // Selections/expansions don't carry across a repo or filter change.
    (0,react.useEffect)(()=>{
        setSelected(new Set());
        setExpanded(new Set());
    }, [
        picker.selectedRepo,
        picker.issueState,
        picker.issueSearch
    ]);
    const toggleSelected = (0,react.useCallback)((number)=>{
        setSelected((prev)=>{
            const next = new Set(prev);
            if (next.has(number)) next.delete(number);
            else next.add(number);
            return next;
        });
    }, []);
    const toggleExpanded = (0,react.useCallback)((number)=>{
        setExpanded((prev)=>{
            const next = new Set(prev);
            if (next.has(number)) next.delete(number);
            else next.add(number);
            return next;
        });
    }, []);
    const selectedIssues = picker.issues.filter((i)=>selected.has(i.number));
    const handleImport = (0,react.useCallback)(()=>{
        if (selectedIssues.length > 0) picker.selectIssues(selectedIssues);
    }, [
        picker,
        selectedIssues
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: phaseHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: backButton,
                        onClick: picker.goBack,
                        "aria-label": "Back to repository selection",
                        children: "← Back"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: repoChip,
                        "aria-label": "Selected repository",
                        children: [
                            (_picker_selectedRepo = picker.selectedRepo) === null || _picker_selectedRepo === void 0 ? void 0 : _picker_selectedRepo.owner,
                            "/",
                            (_picker_selectedRepo1 = picker.selectedRepo) === null || _picker_selectedRepo1 === void 0 ? void 0 : _picker_selectedRepo1.repo,
                            ((_picker_selectedRepo2 = picker.selectedRepo) === null || _picker_selectedRepo2 === void 0 ? void 0 : _picker_selectedRepo2.host) ? " (".concat(picker.selectedRepo.host, ")") : ""
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: filterBar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        ref: searchRef,
                        className: searchInput,
                        type: "search",
                        placeholder: "Search issues…",
                        value: picker.issueSearch,
                        onChange: (e)=>picker.setIssueSearch(e.target.value),
                        "aria-label": "Search issues",
                        autoComplete: "off",
                        style: {
                            flex: 1
                        },
                        autoFocus: true
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: stateToggle,
                        role: "group",
                        "aria-label": "Issue state filter",
                        children: states.map((s)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: picker.issueState === s ? stateButton.active : stateButton.inactive,
                                onMouseDown: (e)=>{
                                    e.preventDefault();
                                    picker.setIssueState(s);
                                },
                                children: s
                            }, s))
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "listbox",
                "aria-label": "GitHub issues",
                "aria-multiselectable": "true",
                className: listContainer,
                children: picker.issuesLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: loadingText,
                    children: "Loading…"
                }) : picker.issues.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: emptyState,
                    children: picker.issueSearch ? "No issues match your search." : "No issues found."
                }) : picker.issues.map((issue)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(IssueRow, {
                        issue: issue,
                        selected: selected.has(issue.number),
                        expanded: expanded.has(issue.number),
                        onToggleSelected: ()=>toggleSelected(issue.number),
                        onToggleExpanded: ()=>toggleExpanded(issue.number)
                    }, issue.number))
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: importBar,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: importBarHint,
                        children: importing && importProgress ? "Importing ".concat(importProgress.done, " of ").concat(importProgress.total, "…") : selectedIssues.length > 0 ? "".concat(selectedIssues.length, " selected") : "Check issues to import, then Import."
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: importButton,
                        disabled: selectedIssues.length === 0 || importing,
                        onClick: handleImport,
                        children: importing ? "Importing…" : "Import".concat(selectedIssues.length > 0 ? " (".concat(selectedIssues.length, ")") : "")
                    })
                ]
            })
        ]
    });
}
// ─── Issue row ────────────────────────────────────────────────────────────────
function IssueRow(param) {
    let { issue, selected, expanded, onToggleSelected, onToggleExpanded } = param;
    var _issue_body;
    const age = relativeTime(issue.updatedAt || issue.createdAt);
    const bodyPreview = (_issue_body = issue.body) === null || _issue_body === void 0 ? void 0 : _issue_body.trim();
    // ImportGitHubIssue only accepts real issue URLs (ParseGitHubRef categorizes
    // /pull/N URLs as RefTypePR, not RefTypeIssue) — a PR can never be imported
    // as a backlog item, so don't offer a selection that's guaranteed to fail.
    const importable = !issue.isPR;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: issueRowWrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                role: "option",
                "aria-selected": selected,
                "aria-disabled": !importable,
                className: (selected ? "".concat(listItem, " ").concat(listItemSelected) : listItem) + (importable ? "" : " ".concat(listItemDisabled)),
                title: importable ? undefined : "Pull requests can't be imported as backlog items — open it on GitHub instead.",
                onClick: importable ? onToggleSelected : undefined,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: selected,
                        disabled: !importable,
                        onChange: onToggleSelected,
                        onClick: (e)=>e.stopPropagation(),
                        "aria-label": importable ? "Select issue #".concat(issue.number, ": ").concat(issue.title) : "Issue #".concat(issue.number, " is a pull request and can't be imported: ").concat(issue.title),
                        className: issueCheckbox
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: issue.isPR ? prTypeBadge : issueTypeBadge,
                        children: issue.isPR ? "PR" : "#"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: issueNumber,
                        children: issue.number
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: issueMainCol,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: listItemName,
                                children: issue.title
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: issueSubMeta,
                                children: [
                                    issue.author && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: issueAuthor,
                                        children: [
                                            "@",
                                            issue.author
                                        ]
                                    }),
                                    age && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        children: age
                                    }),
                                    issue.labels.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        children: [
                                            issue.labels.length,
                                            " label",
                                            issue.labels.length > 1 ? "s" : ""
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    bodyPreview && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: expandToggle,
                        "aria-label": expanded ? "Collapse details" : "Expand details",
                        "aria-expanded": expanded,
                        onClick: (e)=>{
                            e.stopPropagation();
                            onToggleExpanded();
                        },
                        children: expanded ? "▾" : "▸"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                        href: issue.url,
                        target: "_blank",
                        rel: "noopener noreferrer",
                        className: openLink,
                        "aria-label": "Open issue #".concat(issue.number, " on GitHub"),
                        title: "Open on GitHub",
                        onClick: (e)=>e.stopPropagation(),
                        children: "\uD83D\uDC41"
                    })
                ]
            }),
            expanded && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: issueExpandedPanel,
                children: [
                    issue.labels.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: issueExpandedLabels,
                        children: issue.labels.map((label)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: labelBadge,
                                title: label,
                                children: label
                            }, label))
                    }),
                    bodyPreview && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: issueExpandedBody,
                        children: bodyPreview
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 93550:
/***/ ((__unused_webpack_module, exports) => {

"use strict";

Object.defineProperty(exports, "__esModule", ({
    value: true
}));
Object.defineProperty(exports, "errorOnce", ({
    enumerable: true,
    get: function() {
        return errorOnce;
    }
}));
let errorOnce = (_)=>{};
if (false) {} //# sourceMappingURL=error-once.js.map


/***/ }),

/***/ 94564:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 96907:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 97510:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 97516:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,4202,9353,8182,7924,6000,4069,935,2265,820,2921,8165,3575,8890,2513,2275,3117,4247,4298,2805,218,3793,6925,1618,3467,6833,9691,3516,7358], () => (__webpack_exec__(40679)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-d121b1a0929b24b7.js.map