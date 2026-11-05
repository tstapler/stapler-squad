"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[6262],{

/***/ 13630:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Hc: () => (/* binding */ host),
/* harmony export */   WR: () => (/* binding */ graveyard),
/* harmony export */   ZL: () => (/* binding */ dockAnchor)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Flib_2Fterminal_2FTerminalPool_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xa2ltbzVkMCB7CiAgcG9zaXRpb246IGZpeGVkOwogIHRvcDogMDsKICBsZWZ0OiAtOTk5OXB4OwogIHdpZHRoOiA4MDBweDsKICBoZWlnaHQ6IDYwMHB4OwogIHZpc2liaWxpdHk6IGhpZGRlbjsKICBwb2ludGVyLWV2ZW50czogbm9uZTsKICBvdmVyZmxvdzogaGlkZGVuOwp9Ci5fMWtpbW81ZDEgewogIHdpZHRoOiAxMDAlOwogIGhlaWdodDogMTAwJTsKfQouXzFraW1vNWQyIHsKICB3aWR0aDogMTAwJTsKICBoZWlnaHQ6IDEwMCU7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(17830);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Flib_2Fterminal_2FTerminalPool_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xa2ltbzVkMCB7CiAgcG9zaXRpb246IGZpeGVkOwogIHRvcDogMDsKICBsZWZ0OiAtOTk5OXB4OwogIHdpZHRoOiA4MDBweDsKICBoZWlnaHQ6IDYwMHB4OwogIHZpc2liaWxpdHk6IGhpZGRlbjsKICBwb2ludGVyLWV2ZW50czogbm9uZTsKICBvdmVyZmxvdzogaGlkZGVuOwp9Ci5fMWtpbW81ZDEgewogIHdpZHRoOiAxMDAlOwogIGhlaWdodDogMTAwJTsKfQouXzFraW1vNWQyIHsKICB3aWR0aDogMTAwJTsKICBoZWlnaHQ6IDEwMCU7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Flib_2Fterminal_2FTerminalPool_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xa2ltbzVkMCB7CiAgcG9zaXRpb246IGZpeGVkOwogIHRvcDogMDsKICBsZWZ0OiAtOTk5OXB4OwogIHdpZHRoOiA4MDBweDsKICBoZWlnaHQ6IDYwMHB4OwogIHZpc2liaWxpdHk6IGhpZGRlbjsKICBwb2ludGVyLWV2ZW50czogbm9uZTsKICBvdmVyZmxvdzogaGlkZGVuOwp9Ci5fMWtpbW81ZDEgewogIHdpZHRoOiAxMDAlOwogIGhlaWdodDogMTAwJTsKfQouXzFraW1vNWQyIHsKICB3aWR0aDogMTAwJTsKICBoZWlnaHQ6IDEwMCU7Cn0_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var dockAnchor = '_1kimo5d2';
var graveyard = '_1kimo5d0';
var host = '_1kimo5d1';

/***/ }),

/***/ 22479:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Y: () => (/* binding */ useDebouncedCallback),
/* harmony export */   d: () => (/* binding */ useDebounce)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);

/**
 * Hook that debounces a value by the specified delay
 * @param value - The value to debounce
 * @param delay - Delay in milliseconds
 * @returns The debounced value
 */ function useDebounce(value, delay) {
    const [debouncedValue, setDebouncedValue] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(value);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const timer = setTimeout(()=>{
            setDebouncedValue(value);
        }, delay);
        return ()=>{
            clearTimeout(timer);
        };
    }, [
        value,
        delay
    ]);
    return debouncedValue;
}
/**
 * Hook that returns a debounced callback function
 * @param callback - The callback to debounce
 * @param delay - Delay in milliseconds
 * @returns Debounced callback function
 */ function useDebouncedCallback(callback, delay) {
    const timeoutIdRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const debouncedCallback = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(function() {
        for(var _len = arguments.length, args = new Array(_len), _key = 0; _key < _len; _key++){
            args[_key] = arguments[_key];
        }
        if (timeoutIdRef.current) {
            clearTimeout(timeoutIdRef.current);
        }
        timeoutIdRef.current = setTimeout(()=>{
            timeoutIdRef.current = null;
            callback(...args);
        }, delay);
    }, [
        callback,
        delay
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            if (timeoutIdRef.current) {
                clearTimeout(timeoutIdRef.current);
            }
        };
    }, []);
    return debouncedCallback;
}


/***/ }),

/***/ 25706:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $o: () => (/* binding */ containsPaneId),
/* harmony export */   Re: () => (/* binding */ swapPanes),
/* harmony export */   Xt: () => (/* binding */ wouldExceedMaxDepth),
/* harmony export */   Zn: () => (/* binding */ MIN_RATIO),
/* harmony export */   cg: () => (/* binding */ generateSecureId),
/* harmony export */   eH: () => (/* binding */ getAllLeaves),
/* harmony export */   fB: () => (/* binding */ createLeaf),
/* harmony export */   fJ: () => (/* binding */ getAdjacentLeaf),
/* harmony export */   fo: () => (/* binding */ replaceNode),
/* harmony export */   gy: () => (/* binding */ findParentSplit),
/* harmony export */   iZ: () => (/* binding */ findLeaf),
/* harmony export */   mJ: () => (/* binding */ findSplit),
/* harmony export */   un: () => (/* binding */ initialPaneState),
/* harmony export */   v2: () => (/* binding */ findNearestAncestorSplit),
/* harmony export */   xk: () => (/* binding */ generatePaneId),
/* harmony export */   zN: () => (/* binding */ MAX_RATIO)
/* harmony export */ });
/* unused harmony exports MAX_DEPTH, hasVerticalSplit */
const MIN_RATIO = 0.1;
const MAX_RATIO = 0.9;
const MAX_DEPTH = 8;
/**
 * Generate an opaque, sufficiently-unique ID string using crypto.randomUUID()
 * where available, falling back to Math.random() outside secure contexts
 * (e.g. non-HTTPS dev servers). Not cryptographically significant — used for
 * client-side correlation (pane IDs, resync correlation IDs), never for
 * security-sensitive purposes.
 */ function generateSecureId() {
    if (typeof crypto !== "undefined" && crypto.randomUUID) {
        return crypto.randomUUID();
    }
    return Math.random().toString(36).slice(2, 10);
}
/** Generate a unique pane ID */ function generatePaneId() {
    return generateSecureId().slice(0, 8);
}
/** Create a new empty leaf pane */ function createLeaf(id) {
    let activeTab = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : "terminal", viewKind = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : "session-detail";
    return {
        type: "leaf",
        id: id !== null && id !== void 0 ? id : generatePaneId(),
        sessionId: null,
        activeTab,
        viewKind
    };
}
/** Return the initial pane state: a vertical split with session-list on the left and session-detail on the right */ function initialPaneState() {
    const listLeaf = createLeaf(undefined, "terminal", "session-list");
    const detailLeaf = createLeaf(undefined, "terminal", "session-detail");
    const splitNode = {
        type: "split",
        id: generatePaneId(),
        direction: "vertical",
        ratio: 0.28,
        first: listLeaf,
        second: detailLeaf
    };
    return {
        root: splitNode,
        focusedPaneId: detailLeaf.id,
        zoomedPaneId: null
    };
}
/**
 * Swap the content (viewKind, sessionId, activeTab) of two leaf panes while keeping their IDs.
 * Returns the original root unchanged if either pane is not found.
 */ function swapPanes(root, paneId, targetPaneId) {
    if (paneId === targetPaneId) return root;
    const leaf1 = findLeaf(root, paneId);
    const leaf2 = findLeaf(root, targetPaneId);
    if (!leaf1 || !leaf2) return root;
    const c1 = {
        viewKind: leaf1.viewKind,
        sessionId: leaf1.sessionId,
        activeTab: leaf1.activeTab
    };
    const c2 = {
        viewKind: leaf2.viewKind,
        sessionId: leaf2.sessionId,
        activeTab: leaf2.activeTab
    };
    let result = replaceNode(root, paneId, {
        ...leaf1,
        ...c2
    });
    result = replaceNode(result, targetPaneId, {
        ...leaf2,
        ...c1
    });
    return result;
}
/** Returns true if the tree contains at least one vertical (side-by-side) split */ function hasVerticalSplit(node) {
    if (node.type === "leaf") return false;
    if (node.direction === "vertical") return true;
    return hasVerticalSplit(node.first) || hasVerticalSplit(node.second);
}
/** Collect all leaf panes from the tree */ function getAllLeaves(root) {
    if (root.type === "leaf") return [
        root
    ];
    return [
        ...getAllLeaves(root.first),
        ...getAllLeaves(root.second)
    ];
}
/** Find a leaf by id, returns null if not found */ function findLeaf(root, id) {
    if (root.type === "leaf") {
        return root.id === id ? root : null;
    }
    var _findLeaf;
    return (_findLeaf = findLeaf(root.first, id)) !== null && _findLeaf !== void 0 ? _findLeaf : findLeaf(root.second, id);
}
/** Find the parent SplitPane of the given node id */ function findParentSplit(root, id) {
    if (root.type === "leaf") return null;
    if (root.first.id === id || root.second.id === id) return root;
    var _findParentSplit;
    return (_findParentSplit = findParentSplit(root.first, id)) !== null && _findParentSplit !== void 0 ? _findParentSplit : findParentSplit(root.second, id);
}
/** Find a SplitPane by its own id */ function findSplit(root, splitId) {
    if (root.type === "leaf") return null;
    if (root.id === splitId) return root;
    var _findSplit;
    return (_findSplit = findSplit(root.first, splitId)) !== null && _findSplit !== void 0 ? _findSplit : findSplit(root.second, splitId);
}
/** Check if a pane id exists somewhere in the subtree */ function containsPaneId(node, id) {
    if (node.type === "leaf") return node.id === id;
    return containsPaneId(node.first, id) || containsPaneId(node.second, id);
}
/** Get the current depth of nesting for a given pane id */ function getDepthOf(root, targetId) {
    let depth = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : 0;
    if (root.type === "leaf") return root.id === targetId ? depth : -1;
    const d1 = getDepthOf(root.first, targetId, depth + 1);
    if (d1 >= 0) return d1;
    return getDepthOf(root.second, targetId, depth + 1);
}
/** Check whether splitting the given leaf would exceed MAX_DEPTH */ function wouldExceedMaxDepth(root, paneId) {
    const depth = getDepthOf(root, paneId);
    return depth < 0 || depth >= MAX_DEPTH;
}
/**
 * Replace the node with the given targetId in the tree, returning a new tree.
 * If not found, returns the original root unchanged.
 */ function replaceNode(root, targetId, replacement) {
    if (root.id === targetId) return replacement;
    if (root.type === "leaf") return root;
    const newFirst = replaceNode(root.first, targetId, replacement);
    const newSecond = replaceNode(root.second, targetId, replacement);
    if (newFirst === root.first && newSecond === root.second) return root;
    return {
        ...root,
        first: newFirst,
        second: newSecond
    };
}
/**
 * Find the nearest ancestor SplitPane whose direction aligns with the given arrow direction.
 * "ArrowLeft"/"ArrowRight" → look for "vertical" splits
 * "ArrowUp"/"ArrowDown"   → look for "horizontal" splits
 */ function findNearestAncestorSplit(root, paneId, arrowDirection) {
    const neededDirection = arrowDirection === "ArrowLeft" || arrowDirection === "ArrowRight" ? "vertical" : "horizontal";
    function search(node, ancestors) {
        if (node.type === "leaf") {
            if (node.id !== paneId) return null;
            // Walk ancestors from nearest to farthest
            for(let i = ancestors.length - 1; i >= 0; i--){
                if (ancestors[i].direction === neededDirection) return ancestors[i];
            }
            return null;
        }
        const withThis = [
            ...ancestors,
            node
        ];
        var _search;
        return (_search = search(node.first, withThis)) !== null && _search !== void 0 ? _search : search(node.second, withThis);
    }
    return search(root, []);
}
/**
 * Get the adjacent leaf in a given direction from the focused pane.
 * Returns null if no adjacent pane exists in that direction.
 */ function getAdjacentLeaf(root, fromId, direction) {
    // Determine the split direction we care about
    const splitDir = direction === "ArrowLeft" || direction === "ArrowRight" ? "vertical" : "horizontal";
    const goToSecond = direction === "ArrowRight" || direction === "ArrowDown";
    function findAdjacentInNode(node) {
        if (node.type === "leaf") return null;
        if (node.direction !== splitDir) {
            var _findAdjacentInNode;
            return (_findAdjacentInNode = findAdjacentInNode(node.first)) !== null && _findAdjacentInNode !== void 0 ? _findAdjacentInNode : findAdjacentInNode(node.second);
        }
        // This split is in the right direction
        const inFirst = containsPaneId(node.first, fromId);
        const inSecond = containsPaneId(node.second, fromId);
        if (inFirst && goToSecond) {
            // Move to the first (leftmost/topmost) leaf of second subtree
            return getFirstLeaf(node.second);
        }
        if (inSecond && !goToSecond) {
            // Move to the last (rightmost/bottommost) leaf of first subtree
            return getLastLeaf(node.first);
        }
        // fromId is in one side but we're going the other direction — recurse
        if (inFirst) return findAdjacentInNode(node.first);
        if (inSecond) return findAdjacentInNode(node.second);
        return null;
    }
    return findAdjacentInNode(root);
}
function getFirstLeaf(node) {
    if (node.type === "leaf") return node;
    return getFirstLeaf(node.first);
}
function getLastLeaf(node) {
    if (node.type === "leaf") return node;
    return getLastLeaf(node.second);
}


/***/ }),

/***/ 27043:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   t: () => (/* binding */ useInputModeOverride)
/* harmony export */ });
/* unused harmony export INPUT_MODE_OVERRIDE_KEY */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ INPUT_MODE_OVERRIDE_KEY,useInputModeOverride auto */ 
const INPUT_MODE_OVERRIDE_KEY = "stapler-squad:input-mode-override";
/**
 * Reads and persists the user's override for mouse+keyboard-on-mobile detection.
 *
 * "auto" (default) lets the terminal toolbar decide based on live hardware
 * signals (matchMedia hover/pointer:fine via useViewport().hasFinePointer).
 * "desktop" and "touch" force that decision regardless of what the hardware
 * reports, for the rare case detection gets it wrong (e.g. some Android
 * keyboard cases, or a user who prefers the expanded toolbar despite a mouse).
 */ function useInputModeOverride() {
    // Read synchronously via a lazy initializer (not a mount effect) — a caller that
    // derives a one-time default from inputModeOverride on its own mount effect would
    // otherwise see the "auto" fallback before an async localStorage-read effect here
    // resolved the persisted value, and could apply the wrong default irreversibly.
    const [inputModeOverride, setInputModeOverrideState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(()=>{
        if (typeof localStorage === "undefined") return "auto";
        try {
            const stored = localStorage.getItem(INPUT_MODE_OVERRIDE_KEY);
            if (stored === "auto" || stored === "desktop" || stored === "touch") {
                return stored;
            }
        } catch (e) {}
        return "auto";
    });
    const setInputModeOverride = (next)=>{
        setInputModeOverrideState(next);
        try {
            localStorage.setItem(INPUT_MODE_OVERRIDE_KEY, next);
        } catch (e) {}
    };
    return {
        inputModeOverride,
        setInputModeOverride
    };
}


/***/ }),

/***/ 28600:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   L: () => (/* binding */ LiveRegion)
/* harmony export */ });
/* unused harmony export useLiveRegion */
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _LiveRegion_css__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(76863);
/* __next_internal_client_entry_do_not_use__ LiveRegion,useLiveRegion auto */ 


function LiveRegion(param) {
    let { message, politeness = "polite", role = "status" } = param;
    const [currentMessage, setCurrentMessage] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(message);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        if (message) {
            setCurrentMessage(message);
        }
    }, [
        message
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
        role: role,
        "aria-live": politeness,
        "aria-atomic": "true",
        className: _LiveRegion_css__WEBPACK_IMPORTED_MODULE_2__/* .srOnly */ .s,
        children: currentMessage
    });
}
// Hook to use live region announcements
function useLiveRegion() {
    const [message, setMessage] = useState("");
    const announce = (newMessage)=>{
        setMessage(newMessage);
        // Clear message after announcement
        setTimeout(()=>setMessage(""), 1000);
    };
    return {
        message,
        announce
    };
}


/***/ }),

/***/ 28990:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   k: () => (/* binding */ dimensionsEqual),
/* harmony export */   n: () => (/* binding */ isFiniteResizeDimensions)
/* harmony export */ });
/**
 * Shared terminal value types.
 *
 * Hoisted out of XtermTerminal.tsx so useTerminalFlowControl.ts can import
 * ResizeDimensions / isFiniteResizeDimensions without depending on a leaf
 * component (see project_plans/terminal-resize-fit-loop/implementation/plan.md
 * §2 Domain Glossary, architecture-review.md Concern 1).
 */ /**
 * Value type replacing raw positional (cols, rows) pairs wherever a size is
 * held as state (applied / proposed / pending / last-sent dimensions).
 */ /**
 * Canonical Number.isFinite guard on a ResizeDimensions value. Returns false
 * for undefined or for a value with a non-finite cols/rows (e.g. Infinity or
 * NaN), so callers can trust a `true` result before using the dimensions.
 */ function isFiniteResizeDimensions(d) {
    return d !== undefined && Number.isFinite(d.cols) && Number.isFinite(d.rows);
}
/**
 * Structural cols/rows equality. Replaces the hand-rolled `a.cols === b.cols
 * && a.rows === b.rows` comparisons duplicated across the resize pipeline
 * (shouldScheduleFit's two checks, useTerminalFlowControl's resize()
 * value-dedup guard).
 */ function dimensionsEqual(a, b) {
    return a.cols === b.cols && a.rows === b.rows;
}


/***/ }),

/***/ 36262:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  TerminalOutput: () => (/* binding */ TerminalOutput)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
;// ./src/lib/terminal/modifierKeys.ts
// xterm modifier key sequences (CSI parameter convention: 2=Shift, 3=Alt, 5=Ctrl).
// Keyed by the unmodified sequence the toolbar sends; callers fall back to the
// input (or an ESC prefix, for Alt) when a key has no entry.
const CTRL_KEY_MAP = {
    '\x1b[A': '\x1b[1;5A',
    '\x1b[B': '\x1b[1;5B',
    '\x1b[C': '\x1b[1;5C',
    '\x1b[D': '\x1b[1;5D',
    '\x1b[H': '\x1b[1;5H',
    '\x1b[F': '\x1b[1;5F',
    '\x1b[5~': '\x1b[5;5~',
    '\x1b[6~': '\x1b[6;5~',
    '/': '\x1f',
    '-': '\x1f'
};
const ALT_KEY_MAP = {
    '\x1b[A': '\x1b[1;3A',
    '\x1b[B': '\x1b[1;3B',
    '\x1b[C': '\x1b[1;3C',
    '\x1b[D': '\x1b[1;3D',
    '\x1b[H': '\x1b[1;3H',
    '\x1b[F': '\x1b[1;3F',
    '\x1b[5~': '\x1b[5;3~',
    '\x1b[6~': '\x1b[6;3~'
};
const SHIFT_KEY_MAP = {
    '\t': '\x1b[Z',
    '\x1b[A': '\x1b[1;2A',
    '\x1b[B': '\x1b[1;2B',
    '\x1b[C': '\x1b[1;2C',
    '\x1b[D': '\x1b[1;2D',
    '\x1b[H': '\x1b[1;2H',
    '\x1b[F': '\x1b[1;2F',
    '\x1b[5~': '\x1b[5;2~',
    '\x1b[6~': '\x1b[6;2~'
};

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/gen/session/v1/events_pb.ts
var events_pb = __webpack_require__(4504);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/context-values.js
var context_values = __webpack_require__(13399);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var esm_code = __webpack_require__(16294);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/serialization.js + 1 modules
var serialization = __webpack_require__(70050);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/run-call.js + 3 modules
var run_call = __webpack_require__(9103);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/create-method-url.js
var create_method_url = __webpack_require__(69522);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/compression.js
var compression = __webpack_require__(48210);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/request-header.js
var request_header = __webpack_require__(46960);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/end-stream.js
var end_stream = __webpack_require__(60713);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/it-ws@6.1.5/node_modules/it-ws/dist/src/client.js + 33 modules
var client = __webpack_require__(92609);
;// ./src/lib/transport/websocket-transport.ts
/* __next_internal_client_entry_do_not_use__ decompressGzipPayload,createWebsocketBasedTransport auto */ 



/**
 * Check if debug logging is enabled via localStorage
 */ function isDebugEnabled() {
    if (false) {}
    return localStorage.getItem("debug-terminal") === "true";
}
/**
 * Encodes a message in ConnectRPC envelope format.
 * Format: [1 byte flags][4 bytes big-endian length][N bytes data]
 */ function encodeEnvelope(flags, data) {
    const envelope = new Uint8Array(5 + data.length);
    envelope[0] = flags;
    // Write length as big-endian uint32
    const view = new DataView(envelope.buffer, envelope.byteOffset, envelope.byteLength);
    view.setUint32(1, data.length, false); // false = big-endian
    envelope.set(data, 5);
    return envelope;
}
/**
 * Decompresses a gzip-compressed envelope payload (terminal-resync-reliability Epic 5.1,
 * Task 5.1.1.0/5.1.1.2). The server sets the envelope's CompressedFlag (see
 * server/protocol/envelope.go) when a payload exceeds its size threshold; this mirrors that
 * on the client so parseResponseBody can decode the frame instead of hard-throwing.
 *
 * Built on ReadableStream + DecompressionStream directly (rather than Response/Blob.stream())
 * so it also runs under Jest's jsdom environment, which polyfills those two from
 * node:stream/web (see jest.setup.js) but does not provide a global Response.
 */ async function decompressGzipPayload(data) {
    // Typed as Uint8Array<ArrayBuffer> (rather than the bare Uint8Array default of
    // Uint8Array<ArrayBufferLike>) to match DecompressionStream.readable's declared type in
    // lib.dom.d.ts exactly — otherwise pipeThrough's generic inference can't reconcile the two
    // and tsc rejects the call.
    const stream = new ReadableStream({
        start (controller) {
            controller.enqueue(data);
            controller.close();
        }
    });
    const decompressed = stream.pipeThrough(new DecompressionStream("gzip"));
    const reader = decompressed.getReader();
    const chunks = [];
    let totalLength = 0;
    for(;;){
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        totalLength += value.length;
    }
    const result = new Uint8Array(totalLength);
    let offset = 0;
    for (const chunk of chunks){
        result.set(chunk, offset);
        offset += chunk.length;
    }
    return result;
}
/**
 * Creates a ConnectRPC transport that uses WebSocket for streaming calls.
 * This works around browser fetch() API limitations for bidirectional streaming.
 *
 * Based on: https://gist.github.com/Cyberax/3956c935a7971627e2ce8e2df3fafb8e
 */ function createWebsocketBasedTransport(opt) {
    return {
        // Use standard fetch for unary calls
        async unary (method, signal, timeoutMs, header, input, contextValues) {
            var _opt_useBinaryFormat;
            const useBinaryFormat = (_opt_useBinaryFormat = opt.useBinaryFormat) !== null && _opt_useBinaryFormat !== void 0 ? _opt_useBinaryFormat : true;
            const { serialize, parse } = (0,serialization/* createClientMethodSerializers */.Ef)(method, useBinaryFormat, opt.jsonOptions, opt.binaryOptions);
            return await (0,run_call/* runUnaryCall */.L)({
                signal,
                interceptors: opt.interceptors,
                req: {
                    stream: false,
                    service: method.parent,
                    method,
                    url: (0,create_method_url/* createMethodUrl */.j)(opt.baseUrl, method),
                    requestMethod: "POST",
                    header: (0,request_header/* requestHeader */.L)(method.methodKind, useBinaryFormat, timeoutMs, header, false),
                    contextValues: contextValues !== null && contextValues !== void 0 ? contextValues : (0,context_values/* createContextValues */.k)(),
                    message: input
                },
                next: async (req)=>{
                    const response = await fetch(req.url, {
                        method: "POST",
                        mode: "cors",
                        headers: req.header,
                        body: serialize(req.message),
                        signal: req.signal
                    });
                    // Check response status
                    if (!response.ok) {
                        const code = response.status === 401 ? esm_code/* Code */.C.Unauthenticated : esm_code/* Code */.C.Unknown;
                        throw new connect_error/* ConnectError */.T("HTTP ".concat(response.status, ": ").concat(response.statusText), code);
                    }
                    const bodyBytes = new Uint8Array(await response.arrayBuffer());
                    const trailer = new Headers();
                    const message = parse(bodyBytes);
                    return {
                        stream: false,
                        header: response.headers,
                        message,
                        trailer,
                        service: method.parent,
                        method
                    };
                }
            });
        },
        // Use WebSocket for streaming calls
        async stream (method, signal, timeoutMs, header, input, contextValues) {
            var _opt_useBinaryFormat;
            const useBinaryFormat = (_opt_useBinaryFormat = opt.useBinaryFormat) !== null && _opt_useBinaryFormat !== void 0 ? _opt_useBinaryFormat : true;
            const { serialize, parse } = (0,serialization/* createClientMethodSerializers */.Ef)(method, useBinaryFormat, opt.jsonOptions, opt.binaryOptions);
            // Parse envelope protocol messages from WebSocket stream
            async function* parseResponseBody(body, trailerTarget, headerRef, getCloseCode) {
                const reader = createEnvelopeReadableStreamForWS(body).getReader();
                let endStreamReceived = false;
                for(;;){
                    const result = await reader.read();
                    if (result.done) {
                        break;
                    }
                    const { flags, data } = result.value;
                    // The end-stream frame carries JSON trailer/error metadata (see endStreamFromJson
                    // below) and is never compressed by the server, so it must be checked ahead of the
                    // compressed-flag branch.
                    if ((flags & end_stream/* endStreamFlag */.Cu) === end_stream/* endStreamFlag */.Cu) {
                        endStreamReceived = true;
                        const endStream = (0,end_stream/* endStreamFromJson */.mo)(data);
                        if (endStream.error) {
                            const error = endStream.error;
                            headerRef.forEach((value, key)=>{
                                error.metadata.append(key, value);
                            });
                            throw error;
                        }
                        endStream.metadata.forEach((value, key)=>trailerTarget.set(key, value));
                        continue;
                    }
                    let payload = data;
                    if ((flags & compression/* compressedFlag */.y) === compression/* compressedFlag */.y) {
                        try {
                            payload = await decompressGzipPayload(data);
                        } catch (err) {
                            throw new connect_error/* ConnectError */.T("protocol error: failed to decompress gzip payload: ".concat(err instanceof Error ? err.message : String(err)), esm_code/* Code */.C.DataLoss);
                        }
                    }
                    yield parse(payload);
                }
                if (!endStreamReceived) {
                    const code = getCloseCode();
                    if (code !== null && code !== 1000 && !(signal === null || signal === void 0 ? void 0 : signal.aborted)) {
                        throw new connect_error/* ConnectError */.T("WebSocket closed", esm_code/* Code */.C.Unavailable, new Headers({
                            "ws-close-code": String(code)
                        }));
                    }
                    throw new connect_error/* ConnectError */.T("stream ended without end-stream message", esm_code/* Code */.C.Internal);
                }
            }
            // Create request body for streaming call
            // For server streaming: send single request message
            // For bidirectional streaming: send initial request, then stream via WebSocket
            async function createRequestBody(input) {
                const r = await input[Symbol.asyncIterator]().next();
                if (r.done === true) {
                    throw new Error("missing request message");
                }
                return encodeEnvelope(0, serialize(r.value));
            }
            timeoutMs = timeoutMs === undefined ? opt.defaultTimeoutMs : timeoutMs <= 0 ? undefined : timeoutMs;
            return await (0,run_call/* runStreamingCall */.u)({
                interceptors: opt.interceptors,
                timeoutMs,
                signal,
                req: {
                    stream: true,
                    service: method.parent,
                    method,
                    url: (0,create_method_url/* createMethodUrl */.j)(opt.baseUrl, method),
                    requestMethod: "POST",
                    header: (0,request_header/* requestHeader */.L)(method.methodKind, useBinaryFormat, timeoutMs, header, false),
                    contextValues: contextValues !== null && contextValues !== void 0 ? contextValues : (0,context_values/* createContextValues */.k)(),
                    message: input
                },
                next: async (req)=>{
                    // Convert HTTP URL to WebSocket URL
                    const wsUrl = req.url.replace(/^http/, "ws");
                    // Connect to WebSocket using it-ws
                    const stream = (0,client/* connect */.N)(wsUrl);
                    // Capture the WebSocket close code so we can propagate it to the hook
                    // via a ConnectError metadata field. The it-ws source generator calls
                    // EventIterator `stop` on close (which ends the async generator without
                    // an error), so we capture it externally here.
                    let wsCloseCode = null;
                    stream.socket.addEventListener("close", (ev)=>{
                        wsCloseCode = ev.code;
                    });
                    if (signal !== undefined) {
                        if (signal.aborted) stream.destroy();
                        else signal.addEventListener("abort", ()=>stream.destroy(), {
                            once: true
                        });
                    }
                    // Wait for connection
                    await stream.connected();
                    // Send headers as text message
                    let headerText = "";
                    req.header.forEach((value, key)=>{
                        headerText += "".concat(key, ": ").concat(value, "\r\n");
                    });
                    stream.socket.send(headerText + "\r\n");
                    // Send all messages from input iterator (including initial handshake)
                    (async ()=>{
                        try {
                            for await (const msg of req.message){
                                const serialized = serialize(msg);
                                if (isDebugEnabled()) {
                                    var _msg_data;
                                    console.log("[WebSocket] Sending message:", {
                                        sessionId: msg.sessionId,
                                        dataCase: (_msg_data = msg.data) === null || _msg_data === void 0 ? void 0 : _msg_data.case,
                                        serializedLength: serialized.length,
                                        envelopeLength: 5 + serialized.length,
                                        readyState: stream.socket.readyState,
                                        bufferedAmount: stream.socket.bufferedAmount
                                    });
                                }
                                const msgBytes = encodeEnvelope(0, serialized);
                                stream.socket.send(msgBytes);
                            }
                            if (isDebugEnabled()) {
                                console.log("[WebSocket] Message iterator completed, sending EndStream");
                            }
                            // CRITICAL: Send EndStream envelope to signal graceful close
                            // Without this, the server detects an abnormal WebSocket closure (code 1005)
                            // and cannot send EndStreamResponse, causing "missing EndStreamResponse" error
                            const endStreamEnvelope = encodeEnvelope(end_stream/* endStreamFlag */.Cu, new Uint8Array(0));
                            stream.socket.send(endStreamEnvelope);
                            if (isDebugEnabled()) {
                                console.log("[WebSocket] EndStream sent successfully");
                            }
                        } catch (err) {
                            console.error("[WebSocket] Error sending input messages:", err);
                        }
                    })();
                    // Parse response headers from first message
                    const headerMsg = await stream.source.next();
                    const connectHeaders = parseHeaders(new TextDecoder().decode(headerMsg.value));
                    var _connectHeaders_get;
                    // Validate response
                    const status = (_connectHeaders_get = connectHeaders.get("Status-Code")) !== null && _connectHeaders_get !== void 0 ? _connectHeaders_get : "-1";
                    const statusCode = parseInt(status);
                    if (statusCode !== 200) {
                        const code = statusCode === 401 ? esm_code/* Code */.C.Unauthenticated : esm_code/* Code */.C.Unknown;
                        throw new connect_error/* ConnectError */.T("WebSocket response status: ".concat(statusCode), code);
                    }
                    const trailer = new Headers();
                    const res = {
                        ...req,
                        header: connectHeaders,
                        trailer,
                        message: parseResponseBody(stream.source, trailer, connectHeaders, ()=>wsCloseCode)
                    };
                    return res;
                }
            });
        }
    };
}
// Parse HTTP headers from string format
function parseHeaders(allHeaders) {
    return allHeaders.trim().split(/[\r\n]+/).reduce((memo, header)=>{
        const [key, value] = header.split(": ");
        if (key && value) {
            memo.append(key, value);
        }
        return memo;
    }, new Headers());
}
// Create readable stream from async generator for envelope parsing
function createEnvelopeReadableStreamForWS(stream) {
    let reader;
    let buffer = new Uint8Array(0);
    function append(chunk) {
        const n = new Uint8Array(buffer.length + chunk.length);
        n.set(buffer);
        n.set(chunk, buffer.length);
        buffer = n;
    }
    return new ReadableStream({
        start () {
            reader = stream;
        },
        async pull (controller) {
            let header;
            for(;;){
                // Try to parse header (5 bytes)
                if (header === undefined && buffer.byteLength >= 5) {
                    let length = 0;
                    for(let i = 1; i < 5; i++){
                        length = (length << 8) + buffer[i];
                    }
                    header = {
                        flags: buffer[0],
                        length
                    };
                }
                // Check if we have full message
                if (header !== undefined && buffer.byteLength >= header.length + 5) {
                    break;
                }
                // Read more data
                const result = await reader.next();
                if (result.done) {
                    break;
                }
                append(result.value);
            }
            if (header === undefined) {
                if (buffer.byteLength === 0) {
                    controller.close();
                    return;
                }
                controller.error(new connect_error/* ConnectError */.T("premature end of stream", esm_code/* Code */.C.DataLoss));
                return;
            }
            // Extract message data
            const data = buffer.subarray(5, 5 + header.length);
            buffer = buffer.subarray(5 + header.length);
            controller.enqueue({
                flags: header.flags,
                data
            });
        }
    });
}

// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/utils/backoff.ts
var backoff = __webpack_require__(46798);
;// ./src/lib/terminal/MessageQueue.ts
/**
 * MessageQueue - Async-iterable queue for outgoing terminal messages.
 *
 * Bridges React callbacks to a ConnectRPC bidirectional stream.
 * The queue implements the async iterator protocol so it can be
 * directly passed to `client.streamTerminal(queue)`.
 *
 * Usage:
 * ```typescript
 * const queue = new MessageQueue();
 * // Producer side
 * queue.push(new TerminalData({ sessionId, data: { case: "input", value: ... } }));
 * // Consumer side (ConnectRPC stream)
 * const stream = client.streamTerminal(queue);
 * // Shutdown
 * queue.close();
 * ```
 */ 

class MessageQueue {
    push(msg) {
        if (this.closed) return;
        if (this.resolve) {
            this.resolve(msg);
            this.resolve = null;
        } else {
            this.queue.push(msg);
        }
    }
    async *[Symbol.asyncIterator]() {
        while(!this.closed){
            if (this.queue.length > 0) {
                yield this.queue.shift();
            } else {
                const msg = await new Promise((resolve)=>{
                    this.resolve = resolve;
                });
                // Don't yield sentinel messages (empty messages used to unblock iterator)
                if (msg.sessionId !== "" || msg.data.case !== undefined) {
                    yield msg;
                }
            }
        }
    }
    /**
   * Closes the queue. Any messages still buffered in `this.queue` (pushed but
   * not yet delivered to an awaiting iterator) are discarded, not drained —
   * the async iterator will yield nothing further for them. Returns the
   * number of messages dropped, so callers can surface a user-facing signal
   * (see `useTerminalStream`'s `onInputDropped`).
   */ close() {
        const droppedCount = this.queue.length;
        this.queue = [];
        this.closed = true;
        if (this.resolve) {
            // Force unblock the iterator with a sentinel message
            // This message will be filtered out by the iterator and not sent to the server
            this.resolve((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId: "",
                data: {
                    case: undefined
                }
            }));
            this.resolve = null;
        }
        return droppedCount;
    }
    isClosed() {
        return this.closed;
    }
    constructor(){
        this.queue = [];
        this.resolve = null;
        this.closed = false;
    }
}

// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
// EXTERNAL MODULE: ./src/lib/pane/paneUtils.ts
var paneUtils = __webpack_require__(25706);
// EXTERNAL MODULE: ./src/lib/terminal/types.ts
var types = __webpack_require__(28990);
// EXTERNAL MODULE: ./src/lib/terminal/mobileDebug.ts
var mobileDebug = __webpack_require__(99050);
;// ./src/lib/hooks/useResizeSettling.ts
/* __next_internal_client_entry_do_not_use__ useResizeSettling auto */ 


const THROTTLE_MS = 200;
// Last BOUNCE_HISTORY_SIZE sizes sent (oldest first; the current lastSentDimsRef is
// excluded, handled by value-dedup separately). Lets resize() catch a repeat of any
// recent size, not just two sends back, since a real oscillating viewport can cycle
// through 3+ values (observed live: 10x6 -> 67x38 -> 67x22 -> 67x38).
const BOUNCE_HISTORY_SIZE = 4;
const BOUNCE_HOLD_BASE_MS = 3000;
const BOUNCE_HOLD_MAX_MS = 15000;
function isDuplicateOfLastSent(lastSent, next) {
    return lastSent !== null && (0,types/* dimensionsEqual */.k)(lastSent, next);
}
function matchesBounceHistory(history, next) {
    return history.some((d)=>(0,types/* dimensionsEqual */.k)(d, next));
}
function nextBounceHoldMs(streak) {
    return Math.min(BOUNCE_HOLD_BASE_MS * 2 ** streak, BOUNCE_HOLD_MAX_MS);
}
/**
 * useResizeSettling -- the single "resize settling" boundary for the
 * terminal resize pipeline (BUG-101). Owns debounce, oscillation (bounce)
 * detection, and escalating backoff in one place, consolidating logic that
 * was previously patched independently and repeatedly at the resize
 * send-path (see BUG-101 doc): `20da32505`, `417d370dc`/`e5a42e730`,
 * `9651edd5e`, `e8060fb1b`/`b5dba385e`, and the generalized bounce-history
 * fix. Any future oscillation shape belongs here, not as another special
 * case bolted onto a call site.
 */ function useResizeSettling(param) {
    let { onSettled } = param;
    const lastResizeTimeRef = (0,react.useRef)(0);
    const lastSentDimsRef = (0,react.useRef)(null);
    const sentHistoryRef = (0,react.useRef)([]);
    // Consecutive bounces held back-to-back with no genuinely-new size sent in
    // between. Drives an escalating hold (doubling, capped) so a viewport
    // stuck oscillating for an extended period backs off further apart instead
    // of retrying every flat hold duration -- resets to 0 the moment a resize
    // actually sends (a real new size, or a held one whose hold elapsed).
    const bounceStreakRef = (0,react.useRef)(0);
    const pendingResizeTimerRef = (0,react.useRef)(null);
    // Cancel any pending deferred resize timer on unmount so it doesn't fire
    // against a torn-down component/connection.
    (0,react.useEffect)(()=>{
        return ()=>{
            if (pendingResizeTimerRef.current) {
                clearTimeout(pendingResizeTimerRef.current);
                pendingResizeTimerRef.current = null;
            }
        };
    }, []);
    const doSend = (0,react.useCallback)((cols, rows)=>{
        const succeeded = onSettled(cols, rows);
        if (!succeeded) return;
        lastResizeTimeRef.current = Date.now();
        if (lastSentDimsRef.current !== null) {
            sentHistoryRef.current.push(lastSentDimsRef.current);
            if (sentHistoryRef.current.length > BOUNCE_HISTORY_SIZE) {
                sentHistoryRef.current.shift();
            }
        }
        lastSentDimsRef.current = {
            cols,
            rows
        };
        bounceStreakRef.current = 0;
    }, [
        onSettled
    ]);
    // Defers `doSend(dims)` by delayMs, replacing whatever deferred send (if
    // any) was already pending. Shared by the bounce-hold and trailing-edge-
    // throttle branches of resize() below -- the only difference between them
    // is how delayMs and the log line are computed.
    const scheduleDeferredSend = (0,react.useCallback)((dims, delayMs)=>{
        pendingResizeTimerRef.current = setTimeout(()=>{
            pendingResizeTimerRef.current = null;
            doSend(dims.cols, dims.rows);
        }, delayMs);
    }, [
        doSend
    ]);
    const resize = (0,react.useCallback)(function(cols, rows) {
        let force = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false, opts = arguments.length > 3 ? arguments[3] : void 0;
        const next = {
            cols,
            rows
        };
        const bypassed = (opts === null || opts === void 0 ? void 0 : opts.bypassBounceHold) === true;
        // Cancel any previously deferred resize -- we have newer dimensions now.
        // This MUST run before the value-dedup early-return below: otherwise a
        // bounce-back call whose dimensions match lastSentDimsRef (e.g. A -> B
        // deferred within the throttle window -> back to A) would dedup-return
        // without cancelling the still-pending deferred send for B, letting
        // that stale send fire later with the wrong dimensions.
        if (pendingResizeTimerRef.current) {
            clearTimeout(pendingResizeTimerRef.current);
            pendingResizeTimerRef.current = null;
        }
        // Value-dedup: skip if this exact (cols, rows) pair was the last one
        // actually sent, independent of (and checked before) the time throttle
        // below. An unchanged value must not keep the throttle window "warm" --
        // lastResizeTimeRef is deliberately left untouched here.
        if (!force && isDuplicateOfLastSent(lastSentDimsRef.current, next)) {
            console.log("[useResizeSettling] Resize skipped, value unchanged (".concat(cols, "x").concat(rows, ")"));
            mobileDebug/* mobileDebug */.Uo.log('resize', {
                cols,
                rows,
                outcome: 'deduped',
                bypassed
            });
            return;
        }
        // Bounce detection: matches ANY of the last BOUNCE_HISTORY_SIZE sizes
        // sent, not just the one two sends ago, since a real oscillating
        // viewport can wander through 3+ values on a slower cadence than
        // THROTTLE_MS catches. Held out past an escalating hold instead of
        // sent immediately, coalescing the oscillation into one settled resize.
        if (!force && !bypassed && matchesBounceHistory(sentHistoryRef.current, next)) {
            const holdMs = nextBounceHoldMs(bounceStreakRef.current);
            bounceStreakRef.current += 1;
            console.log("[useResizeSettling] Resize bounce detected (".concat(cols, "x").concat(rows, " matches recent history), holding ").concat(holdMs, "ms (streak ").concat(bounceStreakRef.current, ")"));
            mobileDebug/* mobileDebug */.Uo.log('resize', {
                cols,
                rows,
                outcome: 'bounce',
                bounce: true,
                holdMs,
                streak: bounceStreakRef.current,
                bypassed
            });
            scheduleDeferredSend(next, holdMs);
            return;
        }
        const timeSinceLastResize = Date.now() - lastResizeTimeRef.current;
        if (!force && timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
            // Defer instead of drop: schedule the trailing-edge send so the final
            // settled size always reaches the server after rapid resize sequences.
            const remaining = THROTTLE_MS - timeSinceLastResize;
            console.log("[useResizeSettling] Resize deferred ".concat(remaining, "ms (").concat(cols, "x").concat(rows, ")"));
            mobileDebug/* mobileDebug */.Uo.log('resize', {
                cols,
                rows,
                outcome: 'deferred',
                bounce: false,
                holdMs: remaining,
                bypassed
            });
            scheduleDeferredSend(next, remaining + 1);
            return;
        }
        mobileDebug/* mobileDebug */.Uo.log('resize', {
            cols,
            rows,
            outcome: 'sent',
            bounce: false,
            bypassed
        });
        doSend(cols, rows);
    }, [
        doSend,
        scheduleDeferredSend
    ]);
    return {
        resize
    };
}

;// ./src/lib/hooks/useTerminalFlowControl.ts
/* __next_internal_client_entry_do_not_use__ RESYNC_CORRELATION_ID_FLAG,useTerminalFlowControl auto */ 





// Epic 3.1 (AC2) — client-generated correlation ID echoed back on the
// resulting TerminalOutput.resync_id. Flag off preserves pre-project wire
// behavior: CurrentPaneRequest.resync_id is left empty and
// requestFullResync()'s return value is always undefined.
const RESYNC_CORRELATION_ID_FLAG = 'terminal:resync-correlation-id';
/**
 * useTerminalFlowControl - Resync logic, resize throttling, and message dispatch
 * for terminal input/resize/scrollback/flow-control.
 */ function useTerminalFlowControl(param) {
    let { sessionId, getTerminal, pushMessageRef, isConnectedRef, onError, outstandingResyncIdsRef, readOnly = false } = param;
    const correlationIdEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)(RESYNC_CORRELATION_ID_FLAG);
    const readOnlyRef = (0,react.useRef)(readOnly);
    readOnlyRef.current = readOnly;
    // Resync state machine refs. Hold the pending resync's generated tracking
    // ID (null when no resync is pending) rather than a plain boolean — Epic
    // 3.1, Task 3.1.1.2. Truthiness is preserved for existing consumers
    // (useTerminalStream.ts's disconnect()): a non-empty string is truthy,
    // null is falsy, so the existing `if (isResyncingRef.current)` checks keep
    // working unchanged. An internal tracking ID is always generated
    // (independent of the correlation-ID flag) so this pending-state machinery
    // behaves identically whether or not the flag is on; only the wire
    // encoding (CurrentPaneRequest.resync_id) and requestFullResync's return
    // value are gated on the flag.
    const isResyncingRef = (0,react.useRef)(null);
    const waitingForPaneResponseRef = (0,react.useRef)(null);
    const lastResyncTimeRef = (0,react.useRef)(0);
    // Ref, not state: the gesture rAF callback must read the live value.
    // One token per in-flight chunked paste, so an overlapping or aborted paste never clears another's flag.
    const activePastesRef = (0,react.useRef)(new Set());
    const sessionIdRef = (0,react.useRef)(sessionId);
    sessionIdRef.current = sessionId;
    const paneRequestTimerRef = (0,react.useRef)(null);
    const dimensionSyncRef = (0,react.useRef)({});
    // Epic 3.1, Task 3.1.1.4a — last dimensions a resync response was actually
    // applied for (set in markPaneResponseReceived). Compared against the
    // terminal's CURRENT dimensions in requestFullResync to compute
    // stale_dimensions: null until the first resync response lands, so the
    // flag defaults to false on a cold start rather than an unconditional true
    // (pre-mortem P1 risk — must not reintroduce the corruption bug an
    // always-true stale_dimensions would cause).
    const lastSyncedDimensionsRef = (0,react.useRef)(null);
    // Cancel any pending deferred resize/pane-request timers when the component
    // unmounts to prevent the timer callback from firing against a torn-down
    // component/connection.
    (0,react.useEffect)(()=>{
        return ()=>{
            if (paneRequestTimerRef.current) {
                clearTimeout(paneRequestTimerRef.current);
                paneRequestTimerRef.current = null;
            }
        };
    }, []);
    // Helper to set error state
    const handleError = (0,react.useCallback)((err)=>{
        const error = err instanceof Error ? err : new Error(String(err));
        onError === null || onError === void 0 ? void 0 : onError(error);
    }, [
        onError
    ]);
    // Helper to push a message (reads from ref to avoid stale closures)
    const pushMessage = (0,react.useCallback)((msg)=>{
        var _pushMessageRef_current;
        (_pushMessageRef_current = pushMessageRef.current) === null || _pushMessageRef_current === void 0 ? void 0 : _pushMessageRef_current.call(pushMessageRef, msg);
    }, [
        pushMessageRef
    ]);
    // Shared connection gate for every public dispatch function below — a copy-pasted
    // per-callsite check is how sendInput's copy silently lacked a console.warn. Internal
    // per-chunk/per-tick continuation checks stay inline (a background retry after the
    // initial call already logged once shouldn't log again).
    const ensureConnected = (0,react.useCallback)((action)=>{
        if (!pushMessageRef.current || !isConnectedRef.current) {
            console.warn("[useTerminalFlowControl] Cannot ".concat(action, ": stream not connected"));
            return false;
        }
        return true;
    }, [
        pushMessageRef,
        isConnectedRef
    ]);
    // ---- Resync ----
    const requestFullResync = (0,react.useCallback)(function() {
        let urgent = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false, isVisibilityTriggered = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : false;
        if (!ensureConnected("request resync")) {
            return undefined;
        }
        const currentTerminal = getTerminal();
        if (!currentTerminal) {
            console.warn("[useTerminalFlowControl] Cannot request resync: terminal not available");
            return undefined;
        }
        const now = Date.now();
        const timeSinceLastResync = now - lastResyncTimeRef.current;
        const RESYNC_THROTTLE_MS = 2000;
        if (!urgent && timeSinceLastResync < RESYNC_THROTTLE_MS && lastResyncTimeRef.current !== 0) {
            console.log("[useTerminalFlowControl] Resync throttled (".concat(timeSinceLastResync, "ms since last, need ").concat(RESYNC_THROTTLE_MS, "ms)"));
            return undefined;
        }
        if (urgent) {
            console.log("[useTerminalFlowControl] Urgent resync bypassing throttle (".concat(timeSinceLastResync, "ms since last)"));
        }
        try {
            console.log("[useTerminalFlowControl] Requesting full resync with current dimensions: ".concat(currentTerminal.cols, "x").concat(currentTerminal.rows));
            lastResyncTimeRef.current = now;
            const trackingId = (0,paneUtils/* generateSecureId */.cg)();
            isResyncingRef.current = trackingId;
            waitingForPaneResponseRef.current = trackingId;
            dimensionSyncRef.current = {
                cols: currentTerminal.cols,
                rows: currentTerminal.rows
            };
            // Epic 3.1, Task 3.1.1.4b — only a visibility/focus-triggered resync
            // can be "stale" in this sense (a resize-triggered one always carries
            // just-measured dimensions). Defaults to false until a resync response
            // has actually been applied at least once (lastSyncedDimensionsRef
            // starts null), and correctly stays false when the terminal's own
            // dimensions genuinely changed while backgrounded (currentTerminal.cols/
            // rows would then differ from lastSyncedDimensionsRef).
            const staleDimensions = isVisibilityTriggered && lastSyncedDimensionsRef.current !== null && currentTerminal.cols === lastSyncedDimensionsRef.current.cols && currentTerminal.rows === lastSyncedDimensionsRef.current.rows;
            const resyncId = correlationIdEnabled ? trackingId : "";
            const currentPaneReq = (0,create/* create */.v)(events_pb/* CurrentPaneRequestSchema */.kq, {
                lines: 50,
                includeEscapes: true,
                targetCols: currentTerminal.cols,
                targetRows: currentTerminal.rows,
                resyncId,
                staleDimensions
            });
            if (resyncId && outstandingResyncIdsRef) {
                outstandingResyncIdsRef.current.set(resyncId, Date.now());
            }
            pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId,
                data: {
                    case: "currentPaneRequest",
                    value: currentPaneReq
                }
            }));
            return resyncId || undefined;
        } catch (err) {
            handleError(err);
            return undefined;
        }
    }, [
        sessionId,
        getTerminal,
        pushMessage,
        ensureConnected,
        handleError,
        correlationIdEnabled,
        outstandingResyncIdsRef
    ]);
    // ---- Message dispatch functions ----
    const PASTE_CHUNK_SIZE = 512; // bytes — fits within tmux's pty write buffer
    const CHUNK_DELAY_MS = 10; // ms between chunks — yields event loop without stalling input
    const sendInput = (0,react.useCallback)((input)=>{
        if (readOnlyRef.current) return;
        if (!ensureConnected("send input")) return;
        const encoder = new TextEncoder();
        const inputBytes = encoder.encode(input);
        if (inputBytes.length <= PASTE_CHUNK_SIZE) {
            try {
                pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                    sessionId,
                    data: {
                        case: "input",
                        value: (0,create/* create */.v)(events_pb/* TerminalInputSchema */.Qn, {
                            data: inputBytes
                        })
                    }
                }));
            } catch (err) {
                handleError(err);
            }
            return;
        }
        // Large input: send in chunks to avoid tmux/WebSocket buffer limits.
        // Capture sessionId at call-time — if the session changes mid-paste the
        // pending chunks are aborted (sessionIdAtStart !== current sessionId).
        const sessionIdAtStart = sessionId;
        let offset = 0;
        const pasteToken = Symbol("paste");
        const activePastes = activePastesRef.current;
        const finishPaste = ()=>{
            activePastes.delete(pasteToken);
        };
        const sendChunk = ()=>{
            // sessionIdRef, not the closed-over sessionId, which is always sessionIdAtStart.
            if (!pushMessageRef.current || !isConnectedRef.current || sessionIdRef.current !== sessionIdAtStart || // session changed; abort
            offset >= inputBytes.length) {
                finishPaste();
                return;
            }
            const chunk = inputBytes.slice(offset, offset + PASTE_CHUNK_SIZE);
            offset += PASTE_CHUNK_SIZE;
            try {
                pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                    sessionId: sessionIdAtStart,
                    data: {
                        case: "input",
                        value: (0,create/* create */.v)(events_pb/* TerminalInputSchema */.Qn, {
                            data: chunk
                        })
                    }
                }));
            } catch (err) {
                finishPaste();
                handleError(err);
                return;
            }
            if (offset < inputBytes.length) {
                activePastes.add(pasteToken);
                setTimeout(sendChunk, CHUNK_DELAY_MS);
            } else {
                finishPaste();
            }
        };
        sendChunk();
    }, [
        sessionId,
        pushMessage,
        pushMessageRef,
        isConnectedRef,
        handleError,
        ensureConnected
    ]);
    // True from the first chunk of a >512 B paste until the last chunk is sent (or it aborts).
    const isInputChunking = (0,react.useCallback)(()=>activePastesRef.current.size > 0, []);
    // A session switch aborts the pending chunks; clear eagerly rather than waiting for the next timer.
    (0,react.useEffect)(()=>{
        activePastesRef.current.clear();
    }, [
        sessionId
    ]);
    // The actual send, invoked by useResizeSettling only once it has judged a
    // (cols, rows) value settled (BUG-101) — debounce, throttle, and
    // oscillation/bounce detection all live in that hook now, not here. Stamps
    // the timestamp, sends the resize RPC, then requests a fresh pane capture
    // so xterm.js and tmux are guaranteed to agree on content. Returns whether
    // the send succeeded so useResizeSettling knows whether to record this
    // value as the last one actually sent.
    const sendSettledResize = (0,react.useCallback)((cols, rows)=>{
        if (!pushMessageRef.current || !isConnectedRef.current) return false;
        if (!readOnlyRef.current) {
            try {
                console.log("[useTerminalFlowControl] Sending resize to server: ".concat(cols, "x").concat(rows));
                pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                    sessionId,
                    data: {
                        case: "resize",
                        value: (0,create/* create */.v)(events_pb/* TerminalResizeSchema */.GF, {
                            cols,
                            rows
                        })
                    }
                }));
            } catch (err) {
                handleError(err);
                return false;
            }
        }
        // After resizing, request fresh terminal content
        paneRequestTimerRef.current = setTimeout(()=>{
            paneRequestTimerRef.current = null;
            if (!pushMessageRef.current || !isConnectedRef.current) return;
            try {
                console.log("[useTerminalFlowControl] Requesting fresh pane content after resize");
                // Epic 3.1, Task 3.1.1.3 — a resize-triggered pane request always
                // carries just-measured dimensions, so stale_dimensions is always
                // explicitly false here (never the isVisibilityTriggered formula
                // used in requestFullResync).
                const resyncId = correlationIdEnabled ? (0,paneUtils/* generateSecureId */.cg)() : "";
                if (resyncId && outstandingResyncIdsRef) {
                    outstandingResyncIdsRef.current.set(resyncId, Date.now());
                }
                pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                    sessionId,
                    data: {
                        case: "currentPaneRequest",
                        value: (0,create/* create */.v)(events_pb/* CurrentPaneRequestSchema */.kq, {
                            lines: 50,
                            includeEscapes: true,
                            targetCols: cols,
                            targetRows: rows,
                            resyncId,
                            staleDimensions: false
                        })
                    }
                }));
            } catch (err) {
                handleError(err);
            }
        }, 100);
        return true;
    }, [
        sessionId,
        pushMessage,
        pushMessageRef,
        isConnectedRef,
        handleError,
        correlationIdEnabled,
        outstandingResyncIdsRef
    ]);
    const { resize: settledResize } = useResizeSettling({
        onSettled: sendSettledResize
    });
    const resize = (0,react.useCallback)(function(cols, rows) {
        let force = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false, opts = arguments.length > 3 ? arguments[3] : void 0;
        if (!ensureConnected("resize terminal")) return;
        settledResize(cols, rows, force, opts);
    }, [
        ensureConnected,
        settledResize
    ]);
    const requestScrollback = (0,react.useCallback)((fromSequence, limit)=>{
        if (!ensureConnected("request scrollback")) return;
        try {
            console.log("[useTerminalFlowControl] Requesting scrollback: fromSeq=".concat(fromSequence, ", limit=").concat(limit));
            pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId,
                data: {
                    case: "scrollbackRequest",
                    value: (0,create/* create */.v)(events_pb/* ScrollbackRequestSchema */.pF, {
                        fromSequence: BigInt(fromSequence),
                        limit
                    })
                }
            }));
        } catch (err) {
            handleError(err);
        }
    }, [
        sessionId,
        pushMessage,
        ensureConnected,
        handleError
    ]);
    const sendFlowControl = (0,react.useCallback)((paused, watermark)=>{
        if (!ensureConnected("send flow control")) return;
        try {
            console.log("[useTerminalFlowControl] Sending flow control: paused=".concat(paused, ", watermark=").concat(watermark || 'N/A'));
            pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId,
                data: {
                    case: "flowControl",
                    value: (0,create/* create */.v)(events_pb/* FlowControlSchema */.bx, {
                        paused,
                        watermark: watermark !== undefined ? BigInt(watermark) : undefined
                    })
                }
            }));
        } catch (err) {
            handleError(err);
        }
    }, [
        sessionId,
        pushMessage,
        ensureConnected,
        handleError
    ]);
    const markResyncComplete = (0,react.useCallback)(()=>{
        isResyncingRef.current = null;
    }, []);
    const markPaneResponseReceived = (0,react.useCallback)(()=>{
        waitingForPaneResponseRef.current = null;
        // Epic 3.1, Task 3.1.1.4a — this is the single existing "resync response
        // applied" signal (invoked, via useVisibilityResync.ts's
        // notifyResyncOutputReceived, whenever a visibility/focus-triggered
        // resync completes), so it's the correct integration point to record
        // which dimensions the client believes are now in sync with the server.
        if (dimensionSyncRef.current.cols !== undefined && dimensionSyncRef.current.rows !== undefined) {
            lastSyncedDimensionsRef.current = {
                cols: dimensionSyncRef.current.cols,
                rows: dimensionSyncRef.current.rows
            };
        }
    }, []);
    const getIsResyncingRef = (0,react.useCallback)(()=>isResyncingRef, []);
    const getWaitingForPaneResponseRef = (0,react.useCallback)(()=>waitingForPaneResponseRef, []);
    return {
        sendInput,
        isInputChunking,
        resize,
        requestScrollback,
        sendFlowControl,
        requestFullResync,
        markResyncComplete,
        markPaneResponseReceived,
        getIsResyncingRef,
        getWaitingForPaneResponseRef
    };
}

;// ./src/lib/hooks/useTerminalMetrics.ts
/* __next_internal_client_entry_do_not_use__ useTerminalMetrics auto */ 
/**
 * useTerminalMetrics - RAF-based output batching and debug recording.
 *
 * Owns the adaptive requestAnimationFrame batching of terminal output
 * and the WebSocket message recording feature for debugging.
 *
 * Output batching strategy:
 * - Uses requestAnimationFrame for display-synchronized updates (60-144fps)
 * - Flushes immediately if buffer exceeds 4KB (prevents lag on large bursts)
 * - Automatically adapts to high refresh rate displays
 */ function useTerminalMetrics(param) {
    let { onOutput } = param;
    const [output, setOutput] = (0,react.useState)("");
    // RAF-based output batching refs
    const outputBufferRef = (0,react.useRef)([]);
    const pendingUpdateRef = (0,react.useRef)(null);
    const textDecoderRef = (0,react.useRef)(new TextDecoder());
    const bufferSizeRef = (0,react.useRef)(0);
    // Recording refs
    const recordedMessagesRef = (0,react.useRef)([]);
    const isRecordingRef = (0,react.useRef)(false);
    const [isRecording, setIsRecording] = (0,react.useState)(false);
    // Flush buffered output to state
    const flushOutputBuffer = (0,react.useCallback)(()=>{
        if (outputBufferRef.current.length > 0) {
            const bufferedText = outputBufferRef.current.join("");
            outputBufferRef.current = [];
            bufferSizeRef.current = 0;
            setOutput((prev)=>prev + bufferedText);
        }
        pendingUpdateRef.current = null;
    }, []);
    // Schedule output update with adaptive batching
    const scheduleOutputUpdate = (0,react.useCallback)((text)=>{
        // If onOutput callback provided, use it directly (better performance)
        if (onOutput) {
            onOutput(text);
            return;
        }
        // Fallback: batch via RAF into React state
        outputBufferRef.current.push(text);
        bufferSizeRef.current += text.length;
        // Immediate flush for large buffers (>4KB)
        if (bufferSizeRef.current > 4096) {
            if (pendingUpdateRef.current !== null) {
                cancelAnimationFrame(pendingUpdateRef.current);
                pendingUpdateRef.current = null;
            }
            flushOutputBuffer();
            return;
        }
        // Otherwise, use RAF for display-synchronized batching
        if (pendingUpdateRef.current === null) {
            pendingUpdateRef.current = requestAnimationFrame(flushOutputBuffer);
        }
    }, [
        flushOutputBuffer,
        onOutput
    ]);
    // Recording functions
    const startRecording = (0,react.useCallback)(()=>{
        recordedMessagesRef.current = [];
        isRecordingRef.current = true;
        setIsRecording(true);
        console.log('[Recording] Started terminal output recording');
    }, []);
    const stopRecording = (0,react.useCallback)(()=>{
        isRecordingRef.current = false;
        setIsRecording(false);
        const blob = new Blob([
            JSON.stringify(recordedMessagesRef.current, null, 2)
        ], {
            type: 'application/json'
        });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = "terminal-recording-".concat(Date.now(), ".json");
        a.click();
        console.log('[Recording] Saved recording with', recordedMessagesRef.current.length, 'messages');
    }, []);
    const recordMessage = (0,react.useCallback)((msg)=>{
        if (isRecordingRef.current) {
            recordedMessagesRef.current.push(msg);
        }
    }, []);
    // Cleanup on unmount: cancel pending RAF, flush remaining output
    (0,react.useEffect)(()=>{
        return ()=>{
            if (pendingUpdateRef.current !== null) {
                cancelAnimationFrame(pendingUpdateRef.current);
                pendingUpdateRef.current = null;
            }
            // Flush any remaining buffered output
            if (outputBufferRef.current.length > 0) {
                const bufferedText = outputBufferRef.current.join("");
                outputBufferRef.current = [];
                bufferSizeRef.current = 0;
                setOutput((prev)=>prev + bufferedText);
            }
        };
    }, []);
    return {
        output,
        scheduleOutputUpdate,
        flushOutputBuffer,
        startRecording,
        stopRecording,
        recordMessage,
        isRecording
    };
}

// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
;// ./src/lib/hooks/useTerminalStream.ts
/* provided dependency */ var process = __webpack_require__(36843);
/* __next_internal_client_entry_do_not_use__ useTerminalStream auto */ 











function useTerminalStream(param) {
    let { baseUrl, sessionId, shellId, onShellStatusChange, getTerminal, scrollbackLines = 1000, onError, onScrollbackReceived, onOutput, onAppScrollback, onAltScreenActiveHint, autoConnect = true, initialCols, initialRows, onInputDropped, foreground = false, outstandingResyncIdsRef, readOnly = false } = param;
    // ---- Connection state ----
    const [isConnected, setIsConnected] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [scrollbackLoaded, setScrollbackLoaded] = (0,react.useState)(false);
    // Task 4.1.1 — Terminal state machine (R1.4)
    const [terminalState, setTerminalState] = (0,react.useState)('DISCONNECTED');
    const [isHardFailed, setIsHardFailed] = (0,react.useState)(false);
    // Epic 4.2, Story 4.2.1 — undefined until a PathHubOwned session's first
    // connection_count-carrying message arrives; never fabricated for
    // PathLegacyPerConnection sessions (proto field is absent there).
    const [connectionCount, setConnectionCount] = (0,react.useState)(undefined);
    const messageQueueRef = (0,react.useRef)(null);
    const abortControllerRef = (0,react.useRef)(null);
    const isDisconnectingRef = (0,react.useRef)(false);
    const isConnectedRef = (0,react.useRef)(false);
    // Guards against two independent visibility/focus-triggered reconnect paths
    // (this hook's own Story 3.1.3 listener and useVisibilityResync, composed
    // together in TerminalOutput.tsx) both calling connect() for the same
    // disconnected session before either handshake completes.
    const isConnectingRef = (0,react.useRef)(false);
    const shouldReconnectRef = (0,react.useRef)(false);
    const terminalBackoffRef = (0,react.useRef)(new backoff/* BackoffState */.Et(1000, 30000));
    const isHardFailedRef = (0,react.useRef)(false);
    const terminalDebounceTimerRef = (0,react.useRef)(null);
    const reconnectTimerRef = (0,react.useRef)(null);
    const connectTimeoutRef = (0,react.useRef)(null);
    const foregroundRef = (0,react.useRef)(foreground);
    const foregroundConnectAttemptRef = (0,react.useRef)(0);
    // Shared across the hook's whole lifetime (not per-connect() call) because the
    // connectTimeoutRef callback below is declared outside the async message-processing
    // IIFE and needs to read it. Correctness relies on connect()'s own re-entrancy guard
    // (isConnectedRef/isConnectingRef) preventing two attempts from touching this ref at
    // once — do not relax those guards without re-checking this invariant.
    const firstMessageRef = (0,react.useRef)(true);
    // Set immediately before attemptController.abort() fires from a connect-timeout,
    // so the resulting stream error can be routed past onError (see the catch block
    // below) instead of surfacing as a visible connection failure — a connect-timeout
    // is a deliberate, internal fast-retry optimization, not a real failure the caller
    // should count toward its own attempt/error UI.
    const connectTimeoutAbortedRef = (0,react.useRef)(false);
    const connectRef = (0,react.useRef)(async ()=>{});
    const textDecoderRef = (0,react.useRef)(new TextDecoder());
    const scrollbackDecoderRef = (0,react.useRef)(new TextDecoder());
    // Task 2.2.1 — Connection-generation fence (mirrors usePathCompletions.ts's
    // generationRef idiom). Bumped once per connect() call; a message-processing
    // loop whose captured generation no longer matches the current value treats
    // itself as superseded and stops mutating shared state.
    const connectionGenerationRef = (0,react.useRef)(0);
    const clientRef = (0,react.useRef)((0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, createWebsocketBasedTransport({
        baseUrl,
        useBinaryFormat: true,
        interceptors: [
            (0,config/* createAuthInterceptor */._)()
        ]
    })));
    // Clears the pending per-attempt connect-timeout timer, if any. Shared across every
    // exit path (first-message success, stream error/end, disconnect(), unmount, and a
    // synchronous throw before the async message loop starts) so none of them can miss it.
    const clearConnectTimeout = (0,react.useCallback)(()=>{
        if (connectTimeoutRef.current) {
            clearTimeout(connectTimeoutRef.current);
            connectTimeoutRef.current = null;
        }
    }, []);
    // Sync ref with state
    (0,react.useEffect)(()=>{
        isConnectedRef.current = isConnected;
    }, [
        isConnected
    ]);
    // Detect the foreground false→true transition (AC3): reset both the
    // existing backoff-delay counter and the new fast-connect-timeout
    // attempt counter so a just-selected terminal gets the full fast
    // window immediately, not whatever was left over from before it was
    // foreground (or from background attempts that happened while it wasn't selected).
    //
    // Also clear any pending reconnectTimerRef and reconnect immediately (pre-mortem
    // Failure #1, P1): without this, a terminal that was mid-backoff-delay while
    // backgrounded would still sit out that stale, potentially-30s wait after being
    // selected, before the fast connect-timeout ever got a chance to apply.
    (0,react.useEffect)(()=>{
        const wasForeground = foregroundRef.current;
        foregroundRef.current = foreground;
        if (process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") return; // same gate as the rest of this file's auto-reconnect logic
        if (!wasForeground && foreground) {
            terminalBackoffRef.current.reset();
            foregroundConnectAttemptRef.current = 0;
            if (reconnectTimerRef.current) {
                clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = null;
                // !isDisconnectingRef.current is unreachable via any path today — disconnect()
                // always clears reconnectTimerRef before setting isDisconnectingRef, so this
                // branch can never observe both truthy at once — but it's cheap, matches
                // handleVisibilityOrOnline's analogous guard below, and protects a future
                // refactor of disconnect()'s ordering from silently reintroducing a race.
                // Not independently unit-tested for that reason (see sdd:6-verify PR review).
                if (shouldReconnectRef.current && !isConnectingRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
                    var _connectRef_current;
                    (_connectRef_current = connectRef.current) === null || _connectRef_current === void 0 ? void 0 : _connectRef_current.call(connectRef);
                }
            }
        }
    }, [
        foreground
    ]);
    // ---- Compose sub-hooks ----
    // pushMessageRef bridges the connection's messageQueue to flow control dispatch.
    // Bug Risk 3 mitigation: flow control reads pushMessageRef.current (not a stale closure).
    const pushMessageRef = (0,react.useRef)(null);
    // Keep pushMessageRef in sync with current messageQueue
    (0,react.useEffect)(()=>{
        pushMessageRef.current = (msg)=>{
            var _messageQueueRef_current;
            (_messageQueueRef_current = messageQueueRef.current) === null || _messageQueueRef_current === void 0 ? void 0 : _messageQueueRef_current.push(msg);
        };
        return ()=>{
            pushMessageRef.current = null;
        };
    }, []);
    const flowControl = useTerminalFlowControl({
        sessionId,
        getTerminal: getTerminal !== null && getTerminal !== void 0 ? getTerminal : ()=>null,
        pushMessageRef,
        isConnectedRef,
        onError,
        outstandingResyncIdsRef,
        readOnly
    });
    const metrics = useTerminalMetrics({
        onOutput
    });
    // ---- Error helper ----
    const handleError = (0,react.useCallback)((err)=>{
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError === null || onError === void 0 ? void 0 : onError(error);
    }, [
        onError
    ]);
    // ---- Connect ----
    const connect = (0,react.useCallback)(async (overrideCols, overrideRows, options)=>{
        // Refuse to reconnect through a hard failure. The only sanctioned way back
        // in is handleManualReconnect below, which clears isHardFailedRef.current
        // to false *before* calling connect() — so this check never blocks Retry,
        // only callers (resize handlers, visibility/focus fallbacks, stale mount
        // effects) that invoke connect() directly without going through Retry.
        if (isHardFailedRef.current) return;
        if (isConnectedRef.current || isConnectingRef.current || !sessionId) return;
        isConnectingRef.current = true;
        shouldReconnectRef.current = true;
        // Only the finally block's own self-scheduled automatic retry (below) passes
        // isAutoRetry: true. Every other caller — mount, manual reconnect, the
        // visibility/online listener and foreground-transition effect in this file,
        // and useVisibilityResync's two direct connect() calls — omits it and gets the
        // fresh backoff sequence a genuinely new attempt sequence should have. Without
        // this gate, the automatic retry itself reset terminalBackoffRef on every call,
        // so `.attempt` was always 0 when the hard-fail check below ran and the delay
        // below never escalated past uniform(0, 1000ms).
        if (!(options === null || options === void 0 ? void 0 : options.isAutoRetry)) {
            terminalBackoffRef.current.reset();
        }
        // Task 2.2.1 — bump the connection generation immediately so this call's
        // message-processing loop (started below) can identify itself as "the
        // current attempt" and detect being superseded by a later connect().
        const myGeneration = ++connectionGenerationRef.current;
        const connectStartedAt = Date.now();
        let targetCols = overrideCols !== null && overrideCols !== void 0 ? overrideCols : initialCols;
        let targetRows = overrideRows !== null && overrideRows !== void 0 ? overrideRows : initialRows;
        if (targetCols === undefined || targetRows === undefined) {
            const currentTerminal = getTerminal === null || getTerminal === void 0 ? void 0 : getTerminal();
            if (currentTerminal) {
                targetCols = currentTerminal.cols;
                targetRows = currentTerminal.rows;
                console.log("[useTerminalStream] Using current terminal dimensions: ".concat(targetCols, "x").concat(targetRows));
            }
        }
        isDisconnectingRef.current = false;
        setTerminalState('CONNECTING');
        try {
            // Task 2.2.2 — unconditionally tear down whatever generation this call
            // is about to replace, regardless of connection state. This removes the
            // previous isConnectedRef-gated skip (the root of the double-live-
            // connection risk documented in architecture.md §1) by making connect()
            // itself always close/abort what it's about to replace.
            if (messageQueueRef.current) {
                const dropped = messageQueueRef.current.close();
                if (dropped > 0) {
                    onInputDropped === null || onInputDropped === void 0 ? void 0 : onInputDropped(dropped);
                }
            }
            if (abortControllerRef.current) {
                abortControllerRef.current.abort();
            }
            abortControllerRef.current = new AbortController();
            const attemptController = abortControllerRef.current;
            firstMessageRef.current = true;
            connectTimeoutAbortedRef.current = false;
            const foregroundAtSchedule = foregroundRef.current;
            const timeoutMs = (0,backoff/* connectTimeoutMs */.Ir)(foregroundAtSchedule, foregroundConnectAttemptRef.current);
            // Counted unconditionally (even with the flag off) since it's cheap and inert
            // when nothing schedules a connect-timeout to consume it.
            foregroundConnectAttemptRef.current += 1;
            const attemptNumber = foregroundConnectAttemptRef.current;
            if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true") {
                connectTimeoutRef.current = setTimeout(()=>{
                    connectTimeoutRef.current = null;
                    // Race guard (pre-mortem Failure #3, P2): re-check firstMessageRef
                    // immediately before aborting, so a message that already landed and
                    // was processed is not retroactively aborted.
                    if (!firstMessageRef.current) return;
                    console.warn("[reconnect] stream=terminal trigger=connect-timeout foreground=".concat(foregroundAtSchedule, " attempt=").concat(attemptNumber, " timeoutMs=").concat(timeoutMs));
                    connectTimeoutAbortedRef.current = true;
                    attemptController.abort();
                }, timeoutMs);
            }
            messageQueueRef.current = new MessageQueue();
            // Send initial handshake with dimensions
            const currentPaneReq = (0,create/* create */.v)(events_pb/* CurrentPaneRequestSchema */.kq, {
                lines: 50,
                includeEscapes: true
            });
            if (targetCols !== undefined && targetRows !== undefined) {
                currentPaneReq.targetCols = targetCols;
                currentPaneReq.targetRows = targetRows;
                console.log("[useTerminalStream] Sending handshake with dimensions: ".concat(targetCols, "x").concat(targetRows));
            } else {
                console.warn("[useTerminalStream] No terminal dimensions available for handshake");
            }
            messageQueueRef.current.push((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId,
                shellId: shellId !== null && shellId !== void 0 ? shellId : "",
                data: {
                    case: "currentPaneRequest",
                    value: currentPaneReq
                }
            }));
            const stream = clientRef.current.streamTerminal(messageQueueRef.current, {
                signal: abortControllerRef.current.signal
            });
            setError(null);
            // Message processing loop
            (async ()=>{
                try {
                    for await (const msg of stream){
                        // Task 2.2.3 — a superseded generation's loop must not mutate
                        // shared state (or, transitively, deliver buffered input to the
                        // wrong connection). Mirrors usePathCompletions.ts's
                        // `if (generation !== generationRef.current) return;` guard.
                        if (myGeneration !== connectionGenerationRef.current) {
                            console.warn("[useTerminalStream] Discarding message from superseded connection generation ".concat(myGeneration, " (current: ").concat(connectionGenerationRef.current, ")"));
                            break;
                        }
                        if (firstMessageRef.current) {
                            clearConnectTimeout();
                            isConnectingRef.current = false;
                            isConnectedRef.current = true; // sync ref before state setter — mirrors the disconnect-path precedent in the finally block below, closing the window where a same-tick second connect() call could read a stale isConnectedRef before React flushes setIsConnected(true)
                            setIsConnected(true);
                            setScrollbackLoaded(true);
                            setTerminalState('LOADING');
                            firstMessageRef.current = false;
                        }
                        // Task 4.1.2 — Handle ResizeQuiescence message (R1.4).
                        // Transitions: resizing=true → RESIZING, resizing=false → STABLE.
                        if (msg.data.case === "resizeQuiescence") {
                            const rq = msg.data.value;
                            if (rq.resizing) {
                                setTerminalState('RESIZING');
                            } else {
                                setTerminalState('STABLE');
                            }
                            continue; // No further processing for quiescence messages
                        }
                        // Handle ShellStatusUpdate for custom shell PTY streams.
                        // Only fire when msg.shellId matches our shellId (guards against stray updates).
                        if (msg.data.case === "shellStatusUpdate") {
                            const update = msg.data.value;
                            if (onShellStatusChange && update.shellId === shellId) {
                                let statusStr = "running";
                                if (update.newStatus === types_pb/* ShellStatus */.d5.STOPPED) statusStr = "stopped";
                                else if (update.newStatus === types_pb/* ShellStatus */.d5.ERROR) statusStr = "error";
                                onShellStatusChange(statusStr, update.exitCode);
                            }
                            continue;
                        }
                        if (msg.data.case === "output") {
                            // Task 4.2.1c (Epic 4.2) — connection_count rides on TerminalOutput,
                            // including on side-channel messages the server sends with no `data`
                            // (server/services/connectrpc_websocket.go's sendConnectionCountUpdates).
                            // Only ever present for PathHubOwned sessions; undefined otherwise —
                            // never fabricated (plan.md Story 4.2.1 AC2).
                            if (msg.data.value.connectionCount !== undefined) {
                                setConnectionCount(msg.data.value.connectionCount);
                            }
                            // See onAltScreenActiveHint's doc comment — only ever present
                            // on an initial-connect/post-resize snapshot.
                            if (msg.data.value.altScreenActive !== undefined) {
                                onAltScreenActiveHint === null || onAltScreenActiveHint === void 0 ? void 0 : onAltScreenActiveHint(msg.data.value.altScreenActive);
                            }
                            // Handle raw output
                            const decodedData = msg.data.value.data;
                            if (decodedData.length === 0) {
                                continue;
                            }
                            const text = textDecoderRef.current.decode(decodedData, {
                                stream: true
                            });
                            // Record message if recording is active
                            metrics.recordMessage({
                                timestamp: Date.now(),
                                type: 'raw',
                                data: decodedData,
                                decoded: text
                            });
                            if ( true && localStorage.getItem("debug-terminal") === "true") {
                                console.debug("[useTerminalStream] Received output: ".concat(text.length, " bytes"));
                            }
                            // Use callback if provided, otherwise batch via RAF
                            if (onOutput) {
                                onOutput(text, msg.data.value.resyncId);
                            } else {
                                metrics.scheduleOutputUpdate(text);
                            }
                            // First raw output → terminal is stable (not resizing)
                            setTerminalState((prev)=>prev === 'LOADING' || prev === 'CONNECTING' ? 'STABLE' : prev);
                        } else if (msg.data.case === "scrollbackResponse") {
                            // Use a per-response decoder so chunks within one scrollbackResponse are streamed
                            // correctly, but separate responses don't share stateful decoder state.
                            const responseDecoder = new TextDecoder();
                            const chunks = [];
                            for (const chunk of msg.data.value.chunks){
                                const text = responseDecoder.decode(chunk.data, {
                                    stream: true
                                });
                                chunks.push(text);
                            }
                            // Flush any trailing bytes buffered by the streaming decoder.
                            const trailing = responseDecoder.decode();
                            if (trailing) chunks.push(trailing);
                            const scrollbackText = chunks.join("");
                            const metadata = {
                                hasMore: msg.data.value.hasMore,
                                oldestSequence: Number(msg.data.value.oldestSequence),
                                newestSequence: Number(msg.data.value.newestSequence),
                                totalLines: Number(msg.data.value.totalLines)
                            };
                            console.log("[useTerminalStream] Scrollback metadata:", metadata);
                            if (onScrollbackReceived) {
                                onScrollbackReceived(scrollbackText, metadata);
                            }
                            // Scrollback response received — return to STABLE
                            setTerminalState((prev)=>prev === 'FETCHING_SCROLLBACK' ? 'STABLE' : prev);
                        } else if (msg.data.case === "appScrollbackResponse") {
                            // Story 1.4.1 — a captured redraw from Instance.ForwardScroll, sent
                            // instead of scrollbackResponse when AppScrollGate passes. Decoded
                            // and dispatched through its own callback so it can never be
                            // confused with either the raw output or tmux-native scrollback path.
                            const resp = msg.data.value;
                            const text = resp.content.length > 0 ? new TextDecoder().decode(resp.content) : "";
                            onAppScrollback === null || onAppScrollback === void 0 ? void 0 : onAppScrollback(text, resp.outcome, resp.program, resp.forwardId, resp.blockedReason);
                            setTerminalState((prev)=>prev === 'FETCHING_SCROLLBACK' ? 'STABLE' : prev);
                        } else if (msg.data.case === "error") {
                            const err = new Error(msg.data.value.message);
                            setError(err);
                            onError === null || onError === void 0 ? void 0 : onError(err);
                            // design/ux.md Surface 2 — HUB_START_FAILED means the hub-owned
                            // path failed to start AND its server-side legacy fallback also
                            // failed (server/services/connectrpc_websocket.go's streamViaHub),
                            // so this connection has no working path at all. Reconnecting
                            // would hit the same failure, so skip the usual 5-attempt
                            // backoff-exhaustion path and surface TerminalOutput.tsx's
                            // existing hardFailedBanner immediately — the same treatment
                            // already given to non-retriable ws-close-codes below.
                            if (msg.data.value.code === "HUB_START_FAILED") {
                                shouldReconnectRef.current = false;
                                isHardFailedRef.current = true;
                                setIsHardFailed(true);
                                console.warn("[reconnect] stream=terminal trigger=hub-start-failed, giving up");
                            }
                        }
                    }
                } catch (err) {
                    // Task 2.2.3 — a superseded generation's error/teardown must not
                    // stomp the newer generation's state (shared backoff/hard-fail
                    // refs included — a stale, aborted generation's close should not
                    // affect the currently-live generation's reconnect fate).
                    if (myGeneration === connectionGenerationRef.current) {
                        // isWorktreeMissingError is checked here (terminal-stream-specific),
                        // not folded into the shared isNonRetriableConnectError — see that
                        // function's doc comment for why.
                        if ((0,backoff/* isNonRetriableConnectError */.Qi)(err) || (0,backoff/* isWorktreeMissingError */.GN)(err)) {
                            shouldReconnectRef.current = false;
                            isHardFailedRef.current = true;
                            setIsHardFailed(true);
                            const code = err instanceof connect_error/* ConnectError */.T ? err.code : "unknown";
                            console.warn("[reconnect] stream=terminal non-retriable error code=".concat(code, ", giving up"));
                        }
                        // A connect-timeout abort is our own deliberate fast-retry optimization, not
                        // a real failure — don't surface it via onError/setError, or callers that
                        // count onError calls toward a user-visible "connection failed" UI (e.g.
                        // TerminalOutput.tsx's connectionAttempts banner) would show churn/false
                        // "Terminal unavailable" states for what's meant to be an invisible retry.
                        // The internal backoff/reconnect scheduling below is unaffected either way.
                        if (!connectTimeoutAbortedRef.current) {
                            handleError(err);
                        }
                    }
                } finally{
                    if (myGeneration === connectionGenerationRef.current) {
                        // A superseded generation is a deliberate reconnect; reaching here means this stream ended on its own.
                        console.warn("[stream-disconnect] session=".concat(sessionId, " shell=").concat(shellId !== null && shellId !== void 0 ? shellId : "", " livedMs=").concat(Date.now() - connectStartedAt, " ") + "foreground=".concat(foregroundRef.current, " visibility=").concat(document.visibilityState, " willReconnect=").concat(shouldReconnectRef.current, " ") + "hardFailed=".concat(isHardFailedRef.current, " connectTimeoutAbort=").concat(connectTimeoutAbortedRef.current));
                        isConnectedRef.current = false; // sync ref before state setter to prevent reconnect guard race
                        isConnectingRef.current = false;
                        setIsConnected(false);
                        setTerminalState('DISCONNECTED');
                        // A stale count from the just-closed connection must not linger
                        // and imply this session still has extra viewers attached.
                        setConnectionCount(undefined);
                        // Reset decoders so stale {stream:true} buffered state from a server-closed
                        // connection does not corrupt the next connect() call.
                        textDecoderRef.current = new TextDecoder();
                        scrollbackDecoderRef.current = new TextDecoder();
                        // Task 2.2.3 — guarded by the same myGeneration check as the rest of this
                        // block: an already-superseded generation's teardown must not clear the
                        // CURRENT generation's pending connect-timeout timer (connectTimeoutRef is
                        // shared across the hook's lifetime, not per-generation).
                        clearConnectTimeout();
                        if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true" && shouldReconnectRef.current && !isDisconnectingRef.current) {
                            if (reconnectTimerRef.current) {
                                clearTimeout(reconnectTimerRef.current);
                                reconnectTimerRef.current = null;
                            }
                            if (connectTimeoutAbortedRef.current) {
                                // A connect-timeout abort is our own deliberate fast-retry optimization
                                // (see the catch block above), not a real stream failure — it must not
                                // consume the 5-attempt hard-fail budget or escalate the backoff delay,
                                // or a connection whose genuine first-message latency exceeds the
                                // connect-timeout (e.g. large scrollback replay over a high-RTT VPN link)
                                // would eventually hard-fail instead of getting the unlimited patience it
                                // had before the per-attempt connect-timeout existed. Retry immediately —
                                // each cycle is already bounded by the connect-timeout itself, so this
                                // can't thundering-herd the way an unthrottled real-failure retry could.
                                console.info("[reconnect] stream=terminal trigger=connect-timeout attempt=".concat(terminalBackoffRef.current.attempt, " delay=0ms (excluded from backoff)"));
                                reconnectTimerRef.current = setTimeout(()=>{
                                    reconnectTimerRef.current = null;
                                    if (shouldReconnectRef.current && !isDisconnectingRef.current) {
                                        var _connectRef_current;
                                        (_connectRef_current = connectRef.current) === null || _connectRef_current === void 0 ? void 0 : _connectRef_current.call(connectRef, undefined, undefined, {
                                            isAutoRetry: true
                                        });
                                    }
                                }, 0);
                            } else if (terminalBackoffRef.current.attempt >= 5) {
                                shouldReconnectRef.current = false;
                                isHardFailedRef.current = true;
                                setIsHardFailed(true);
                            } else {
                                const delay = terminalBackoffRef.current.next();
                                console.info("[reconnect] stream=terminal trigger=close attempt=".concat(terminalBackoffRef.current.attempt, " delay=").concat(delay, "ms"));
                                reconnectTimerRef.current = setTimeout(()=>{
                                    reconnectTimerRef.current = null;
                                    if (shouldReconnectRef.current && !isDisconnectingRef.current) {
                                        var _connectRef_current;
                                        (_connectRef_current = connectRef.current) === null || _connectRef_current === void 0 ? void 0 : _connectRef_current.call(connectRef, undefined, undefined, {
                                            isAutoRetry: true
                                        });
                                    }
                                }, delay);
                            }
                        }
                    }
                }
            })();
        } catch (err) {
            // Covers a synchronous throw before the async message loop starts (e.g. proto
            // schema validation, MessageQueue construction) — the connect-timeout scheduled
            // above must not outlive this now-dead attempt.
            isConnectingRef.current = false;
            clearConnectTimeout();
            handleError(err);
            setIsConnected(false);
        }
    }, [
        sessionId,
        shellId,
        onShellStatusChange,
        getTerminal,
        onError,
        onScrollbackReceived,
        onOutput,
        onAppScrollback,
        onAltScreenActiveHint,
        flowControl,
        metrics,
        handleError,
        initialCols,
        initialRows,
        onInputDropped,
        clearConnectTimeout
    ]);
    // Keep connectRef in sync so visibility/online listeners always call the current closure
    connectRef.current = connect;
    // ---- Disconnect ----
    // Use stable method reference to avoid disconnect being recreated on every render.
    // flowControl returns a new object literal each render, but getIsResyncingRef is a
    // stable useCallback(() => isResyncingRef, []) so depending on it keeps disconnect stable.
    const getIsResyncingRef = flowControl.getIsResyncingRef;
    const disconnect = (0,react.useCallback)(async ()=>{
        shouldReconnectRef.current = false;
        if (reconnectTimerRef.current) {
            clearTimeout(reconnectTimerRef.current);
            reconnectTimerRef.current = null;
        }
        clearConnectTimeout();
        const isResyncingRef = getIsResyncingRef();
        if (isDisconnectingRef.current || isResyncingRef.current) {
            if (isResyncingRef.current) {
                console.log("[useTerminalStream] Delaying disconnect - resync in progress");
                setTimeout(()=>disconnect(), 500);
            }
            return;
        }
        isDisconnectingRef.current = true;
        // Captured so the delayed callback below can tell whether a newer connect()
        // has since taken over before it mutates shared abortControllerRef/isConnected
        // state — otherwise a stale disconnect() racing a fresh connect() can abort
        // or clobber the newer generation's connection (see connection-generation
        // guard on the read side in connect(), Story 2.2).
        const myGeneration = connectionGenerationRef.current;
        if (messageQueueRef.current) {
            const dropped = messageQueueRef.current.close();
            if (dropped > 0) {
                onInputDropped === null || onInputDropped === void 0 ? void 0 : onInputDropped(dropped);
            }
            messageQueueRef.current = null;
        }
        await new Promise((resolve)=>{
            const timeout = setTimeout(()=>{
                if (myGeneration === connectionGenerationRef.current && abortControllerRef.current) {
                    console.debug("[useTerminalStream] Timeout waiting for graceful close, forcing abort");
                    abortControllerRef.current.abort();
                    abortControllerRef.current = null;
                }
                resolve();
            }, 1000);
            if (!isConnectedRef.current) {
                clearTimeout(timeout);
                resolve();
                return;
            }
        });
        if (myGeneration === connectionGenerationRef.current) {
            setIsConnected(false);
        }
        isDisconnectingRef.current = false;
        textDecoderRef.current = new TextDecoder();
        scrollbackDecoderRef.current = new TextDecoder();
    }, [
        getIsResyncingRef,
        onInputDropped,
        clearConnectTimeout
    ]);
    // ---- Auto-connect / cleanup ----
    (0,react.useEffect)(()=>{
        if (autoConnect) {
            connect();
        }
        return ()=>{
            shouldReconnectRef.current = false;
            if (reconnectTimerRef.current) {
                clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = null;
            }
            clearConnectTimeout();
            metrics.flushOutputBuffer();
            disconnect();
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionId,
        autoConnect
    ]);
    // Story 3.1.3 — reconnect on tab visibility or network online event
    (0,react.useEffect)(()=>{
        if (process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") return;
        const handleVisibilityOrOnline = (ev)=>{
            if (document.visibilityState !== "visible" && ev.type !== "online") return;
            if (terminalDebounceTimerRef.current) clearTimeout(terminalDebounceTimerRef.current);
            terminalDebounceTimerRef.current = setTimeout(()=>{
                terminalDebounceTimerRef.current = null;
                if (shouldReconnectRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
                    terminalBackoffRef.current.reset();
                    console.info("[reconnect] stream=terminal trigger=visibility delay=0ms");
                    connectRef.current();
                }
            }, 200);
        };
        document.addEventListener("visibilitychange", handleVisibilityOrOnline);
        window.addEventListener("online", handleVisibilityOrOnline);
        return ()=>{
            if (terminalDebounceTimerRef.current) clearTimeout(terminalDebounceTimerRef.current);
            document.removeEventListener("visibilitychange", handleVisibilityOrOnline);
            window.removeEventListener("online", handleVisibilityOrOnline);
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    // Story 3.1.2 — manual reconnect after hard failure
    const handleManualReconnect = (0,react.useCallback)(()=>{
        isHardFailedRef.current = false;
        setIsHardFailed(false);
        shouldReconnectRef.current = true;
        terminalBackoffRef.current.reset();
        connectRef.current();
    }, []);
    return {
        output: metrics.output,
        isConnected,
        error,
        sendInput: flowControl.sendInput,
        isInputChunking: flowControl.isInputChunking,
        resize: flowControl.resize,
        connect,
        disconnect,
        scrollbackLoaded,
        requestScrollback: flowControl.requestScrollback,
        sendFlowControl: flowControl.sendFlowControl,
        startRecording: metrics.startRecording,
        stopRecording: metrics.stopRecording,
        terminalState,
        isHardFailed,
        handleManualReconnect,
        requestFullResync: flowControl.requestFullResync,
        markResyncComplete: flowControl.markResyncComplete,
        markPaneResponseReceived: flowControl.markPaneResponseReceived,
        connectionCount
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/useAnnounce.ts
var useAnnounce = __webpack_require__(5647);
;// ./src/lib/hooks/useFileDrop.ts

const isFileDrag = (e)=>e.dataTransfer.types.includes("Files");
/**
 * Drag-and-drop of OS files onto an element. Ignores non-file drags (e.g. pane reordering)
 * and, when `enabled` is false, swallows file drags (so the browser doesn't navigate to the
 * dropped file) without showing an overlay or uploading. dragCounterRef balances enter/leave events
 * fired by child elements so the overlay doesn't flicker.
 */ // kibitzer:ignore flag-argument -- `enabled` gates the whole hook (read-only/touch), not a mode switch
function useFileDrop(enabled, onFiles) {
    const [isDragOver, setIsDragOver] = (0,react.useState)(false);
    const dragCounterRef = (0,react.useRef)(0);
    const onDragEnter = (0,react.useCallback)((e)=>{
        if (!isFileDrag(e)) return;
        e.preventDefault();
        if (!enabled) return;
        dragCounterRef.current++;
        if (dragCounterRef.current === 1) setIsDragOver(true);
    }, [
        enabled
    ]);
    const onDragOver = (0,react.useCallback)((e)=>{
        if (!isFileDrag(e)) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = enabled ? "copy" : "none";
    }, [
        enabled
    ]);
    const onDragLeave = (0,react.useCallback)((e)=>{
        if (!isFileDrag(e)) return;
        e.preventDefault();
        if (!enabled) return;
        dragCounterRef.current = Math.max(0, dragCounterRef.current - 1);
        if (dragCounterRef.current === 0) setIsDragOver(false);
    }, [
        enabled
    ]);
    const onDrop = (0,react.useCallback)((e)=>{
        dragCounterRef.current = 0;
        setIsDragOver(false);
        if (!isFileDrag(e)) return;
        e.preventDefault();
        if (!enabled) return;
        const files = Array.from(e.dataTransfer.files);
        if (files.length > 0) onFiles(files);
    }, [
        enabled,
        onFiles
    ]);
    return {
        isDragOver: isDragOver && enabled,
        dropHandlers: {
            onDragEnter,
            onDragOver,
            onDragLeave,
            onDrop
        }
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/usePublishStackTopOffset.ts
var usePublishStackTopOffset = __webpack_require__(23275);
// EXTERNAL MODULE: ./src/lib/hooks/useDebounce.ts
var useDebounce = __webpack_require__(22479);
// EXTERNAL MODULE: ./src/lib/contexts/AnalyticsContext.tsx
var AnalyticsContext = __webpack_require__(24044);
;// ./src/components/sessions/useVisibilityResync.ts




const RESYNC_DEBOUNCE_MS = 300;
const RESYNC_STALL_TIMEOUT_MS = 4000;
// Task 3.1.2.4b — hard ceiling on how long a single resync_id may stay
// outstanding, even while sibling resync traffic keeps proving the
// connection alive and resetting this hook's own 4s stall watchdog. Without
// this, a resync whose own server-side handling silently hangs/fails would
// never surface: every reset from an unrelated ID's response looks
// identical to progress. 2x RESYNC_STALL_TIMEOUT_MS gives sibling traffic
// real room to legitimately postpone the watchdog before treating it as
// evidence of an actual (not just slow) failure.
const RESYNC_STALL_ESCALATION_CEILING_MS = RESYNC_STALL_TIMEOUT_MS * 2;
// Delay before surfacing the existing reconnecting-banner UI for a pending
// resync (Story 2.1.8) — shorter than RESYNC_STALL_TIMEOUT_MS so a slow
// resync gets a visible affordance well before the 4s forced-reconnect path.
const RESYNC_BANNER_DELAY_MS = 2000;
// Epic 2.1 (AC1) — scopes visibility/focus-triggered resync to only the
// instance the user is actually looking at. Flag off preserves pre-project
// behavior (every mounted instance resyncs on every visibility/focus event).
const RESYNC_VISIBILITY_SCOPE_FLAG = 'terminal:resync-visibility-scope';
function useVisibilityResync(params) {
    const { sessionId, isVisible, isConnected, terminalState, connect, disconnect, requestFullResync, markResyncComplete, markPaneResponseReceived, setShowReconnectButton, setShowReconnectBanner, scheduleResync, outstandingResyncIdsRef } = params;
    const resyncVisibilityScopeEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)(RESYNC_VISIBILITY_SCOPE_FLAG);
    const resyncStallTimerRef = (0,react.useRef)(null);
    // Distinct ref/timer from resyncStallTimerRef and useDebouncedCallback's own
    // internal timer (pitfalls.md guardrail: never share a ref between timers).
    // Fires 2s into a pending resync to surface the existing reconnecting-banner
    // UI (Story 2.1.8) — separate from the 4s stall watchdog above.
    const resyncBannerTimerRef = (0,react.useRef)(null);
    // No-correlation-ID heuristic: cleared by the *next* output of any kind (see
    // notifyResyncOutputReceived below), not specifically the CurrentPaneRequest
    // response — same imprecise heuristic the pre-existing resize->resync path
    // already relies on (isResyncingRef/waitingForPaneResponseRef). Accepted
    // trade-off, locked in by Task 2.1.3d's test; worst case bounded by the
    // RESYNC_STALL_TIMEOUT_MS watchdog below.
    const pendingResyncCompletionRef = (0,react.useRef)(false);
    // Sibling ref to pendingResyncCompletionRef (Epic 3.1, Task 3.1.1.4c) —
    // kept separate rather than folding the ID into pendingResyncCompletionRef
    // itself, since requestFullResync returns undefined when the correlation-ID
    // flag is off, which would break the boolean pending/re-entrancy-guard
    // semantics above if it were the sole "is pending" indicator. Holds the
    // specific resync_id this hook is currently waiting on, or undefined when
    // there's no pending resync or the flag is off.
    const pendingResyncIdRef = (0,react.useRef)(undefined);
    // Tracks whether the reconnecting-banner was actually shown (i.e. the 2s
    // resyncBannerTimerRef fired) for the *current* pending resync — distinct
    // from pendingResyncCompletionRef so notifyResyncOutputReceived only calls
    // setShowReconnectBanner(false) when there is actually a shown banner to
    // hide (Story 2.1.8 AC: "If the resync completes before 2000ms elapses...
    // setShowReconnectBanner is never called").
    const bannerShownRef = (0,react.useRef)(false);
    // Task 7.1.1.4 (Epic 7.1 observability) — timestamp the current pending
    // resync started, so notifyResyncOutputReceived's success path can log how
    // long it actually took (as a companion metric to the 4s stall-watchdog
    // ceiling above, not a replacement for it).
    const resyncStartTimeRef = (0,react.useRef)(null);
    const { track } = (0,AnalyticsContext/* useAnalytics */.st)();
    const isVisibleRef = (0,react.useRef)(isVisible);
    const resyncVisibilityScopeEnabledRef = (0,react.useRef)(resyncVisibilityScopeEnabled);
    const isConnectedRef = (0,react.useRef)(isConnected);
    const terminalStateRef = (0,react.useRef)(terminalState);
    const connectRef = (0,react.useRef)(connect);
    const disconnectRef = (0,react.useRef)(disconnect);
    const requestFullResyncRef = (0,react.useRef)(requestFullResync);
    const markResyncCompleteRef = (0,react.useRef)(markResyncComplete);
    const markPaneResponseReceivedRef = (0,react.useRef)(markPaneResponseReceived);
    const setShowReconnectButtonRef = (0,react.useRef)(setShowReconnectButton);
    const setShowReconnectBannerRef = (0,react.useRef)(setShowReconnectBanner);
    const scheduleResyncRef = (0,react.useRef)(scheduleResync);
    const trackRef = (0,react.useRef)(track);
    // Mirrors the *latest* sessionId on every render, independent of which
    // sessionId a given `handleVisibilityOrFocusResyncInner` closure was created
    // for. Lets a stale, already-scheduled debounced/watchdog callback detect
    // mid-flight that the session has since switched and abort instead of
    // firing against the new session's connect()/disconnect() (2nd-review
    // architecture/adversarial finding: session-switch cleanup, Story 2.1.6,
    // only tears down state for a resync/watchdog that had already started —
    // it doesn't cancel a debounce timer that's still pending when the switch
    // happens, since useDebouncedCallback exposes no cancel handle).
    const sessionIdRef = (0,react.useRef)(sessionId);
    (0,react.useEffect)(()=>{
        isVisibleRef.current = isVisible;
        resyncVisibilityScopeEnabledRef.current = resyncVisibilityScopeEnabled;
        isConnectedRef.current = isConnected;
        terminalStateRef.current = terminalState;
        connectRef.current = connect;
        disconnectRef.current = disconnect;
        requestFullResyncRef.current = requestFullResync;
        markResyncCompleteRef.current = markResyncComplete;
        markPaneResponseReceivedRef.current = markPaneResponseReceived;
        setShowReconnectButtonRef.current = setShowReconnectButton;
        setShowReconnectBannerRef.current = setShowReconnectBanner;
        scheduleResyncRef.current = scheduleResync;
        trackRef.current = track;
        sessionIdRef.current = sessionId;
    });
    const clearStallTimer = (0,react.useCallback)(()=>{
        if (resyncStallTimerRef.current) {
            clearTimeout(resyncStallTimerRef.current);
            resyncStallTimerRef.current = null;
        }
    }, []);
    const clearBannerTimer = (0,react.useCallback)(()=>{
        if (resyncBannerTimerRef.current) {
            clearTimeout(resyncBannerTimerRef.current);
            resyncBannerTimerRef.current = null;
        }
    }, []);
    // Shared terminal-recovery path (Epic 3.1, Tasks 3.1.2.2/3.1.2.4b) for both
    // ways a pending resync can be declared lost: the ordinary 4s stall
    // watchdog firing with no sibling traffic to reset it, or resetStallWatchdog
    // below discovering this hook's own resync_id has been outstanding past the
    // 2x escalation ceiling despite repeated resets. Identical recovery
    // (disconnect+reconnect) either way — only the log/analytics label differs,
    // so a stall dashboard can distinguish "no reset ever happened" fires from
    // "resets kept happening but the real response never came" ones.
    const forceResyncRecovery = (0,react.useCallback)((reason)=>{
        if (!pendingResyncCompletionRef.current) return;
        const stalledResyncId = pendingResyncIdRef.current;
        // Actual wall-clock time since this resync's request was sent, not just the
        // nominal watchdog constant — an escalation-triggered fire can have been
        // outstanding far longer than RESYNC_STALL_ESCALATION_CEILING_MS if sibling
        // traffic kept resetting the per-fire timer (see this function's own doc
        // comment). Falls back to the nominal constant if the start time was
        // somehow never recorded, so the log line never shows an undefined/NaN
        // duration.
        const actualElapsedMs = resyncStartTimeRef.current !== null ? Date.now() - resyncStartTimeRef.current : undefined;
        pendingResyncCompletionRef.current = false;
        pendingResyncIdRef.current = undefined;
        bannerShownRef.current = false;
        resyncStartTimeRef.current = null;
        markResyncCompleteRef.current();
        markPaneResponseReceivedRef.current();
        clearStallTimer();
        clearBannerTimer();
        const elapsedMs = reason === 'escalation' ? RESYNC_STALL_ESCALATION_CEILING_MS : RESYNC_STALL_TIMEOUT_MS;
        const label = reason === 'escalation' ? 'escalation ceiling' : 'stall watchdog';
        // resyncId is included so a future occurrence can be grepped directly out of
        // the server's structured log (matches the resync_id field logged wherever
        // EchoResyncID/terminal:resync-correlation-id echoes it back) without first
        // having to correlate on session name + rough timestamp, which is how the
        // 2026-08-25 investigation into this exact symptom (see
        // session/tmux/tmux.go's resyncFastLaneTimeout doc comment) had to be done.
        console.warn("[resync] sessionId=".concat(sessionIdRef.current, " resyncId=").concat(stalledResyncId !== null && stalledResyncId !== void 0 ? stalledResyncId : '(none)', " ").concat(label, " fired after ").concat(elapsedMs, "ms nominal (").concat(actualElapsedMs !== null && actualElapsedMs !== void 0 ? actualElapsedMs : 'unknown', "ms actual), forcing disconnect+reconnect"));
        // Task 7.1.1.5 (Epic 7.1 observability) — structured analytics event
        // alongside the console.warn above, so stall-watchdog/escalation fires
        // are queryable (e.g. correlated with visibility_state) rather than only
        // discoverable by grepping console output.
        trackRef.current({
            name: reason === 'escalation' ? 'resync_stall_escalation_fired' : 'resync_stall_watchdog_fired',
            category: 'performance',
            durationMs: elapsedMs,
            labels: {
                resync_id: stalledResyncId !== null && stalledResyncId !== void 0 ? stalledResyncId : '',
                visibility_state: document.visibilityState
            }
        });
        disconnectRef.current().then(()=>connectRef.current());
    }, [
        clearStallTimer,
        clearBannerTimer
    ]);
    // Extracted (Epic 3.1, Task 3.1.2.2) so both the initial resync trigger and
    // the shared resetStallWatchdog() below can (re)arm the same 4s watchdog
    // without duplicating its body.
    const armStallTimer = (0,react.useCallback)(()=>{
        clearStallTimer();
        resyncStallTimerRef.current = setTimeout(()=>{
            resyncStallTimerRef.current = null;
            forceResyncRecovery('watchdog');
        }, RESYNC_STALL_TIMEOUT_MS);
    }, [
        clearStallTimer,
        forceResyncRecovery
    ]);
    const handleVisibilityOrFocusResyncInner = (0,react.useCallback)(function() {
        let preempt = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false;
        if (document.visibilityState !== 'visible') return;
        // Epic 2.1 (AC1) — with the flag on, only the instance actually in the
        // foreground resyncs; a backgrounded instance is a no-op here and will
        // resync on its own once the user switches to it (its own isVisible flips
        // true and fires this same handler). Flag off preserves pre-project
        // behavior: every mounted instance resyncs on every visibility/focus event
        // regardless of isVisible.
        if (resyncVisibilityScopeEnabledRef.current && !isVisibleRef.current) return;
        // Abort if the session changed since this debounced call was scheduled —
        // e.g. visibilitychange fires while viewing session A, the debounce timer
        // is armed, then the user switches to session B before the 300ms elapses.
        // `sessionId` here is this closure's frozen value (a real useCallback dep);
        // `sessionIdRef.current` is the latest live value. A mismatch means this is
        // a stale callback that must not act on the new session's connect/disconnect
        // (2nd-review finding: Story 2.1.6's cleanup only handles a resync/watchdog
        // that already started, not one still pending in the debounce window).
        if (sessionId !== sessionIdRef.current) return;
        if (isConnectedRef.current) {
            // Rapid-flap re-entrancy guard — see Story 2.1.3.
            if (pendingResyncCompletionRef.current) return;
            pendingResyncCompletionRef.current = true;
            // Epic 6.1 (terminal:resync-stagger) — `fire` is exactly today's
            // pre-Epic-6.1 synchronous resync logic, unchanged. When no scheduler
            // is wired (flag off, or no SessionDetailView-level stagger queue),
            // it's invoked immediately below, preserving byte-for-byte behavior.
            const fire = ()=>{
                try {
                    console.info("[resync] sessionId=".concat(sessionId, " trigger=visibility-or-focus delay=0ms"));
                    resyncStartTimeRef.current = Date.now();
                    pendingResyncIdRef.current = requestFullResyncRef.current(true, true);
                } catch (err) {
                    console.warn("[resync] sessionId=".concat(sessionId, " requestFullResync threw synchronously"), err);
                } finally{
                    // Arm unconditionally — even on a synchronous throw — so the watchdog
                    // remains the single recovery path. See Story 2.1.3.
                    armStallTimer();
                    // Surface the existing reconnecting-banner UI once a resync has been
                    // pending ≥2s — see Story 2.1.8. Independent of, and shorter than, the
                    // 4s stall watchdog above; reuses `TerminalOutput.tsx`'s already-shipped
                    // banner rather than adding a new resync-specific indicator (Pattern
                    // Decisions: "Transient-state UI during the 0-4s resync/watchdog window").
                    clearBannerTimer();
                    bannerShownRef.current = false;
                    resyncBannerTimerRef.current = setTimeout(()=>{
                        resyncBannerTimerRef.current = null;
                        if (pendingResyncCompletionRef.current) {
                            var _setShowReconnectBannerRef_current;
                            bannerShownRef.current = true;
                            (_setShowReconnectBannerRef_current = setShowReconnectBannerRef.current) === null || _setShowReconnectBannerRef_current === void 0 ? void 0 : _setShowReconnectBannerRef_current.call(setShowReconnectBannerRef, true);
                        }
                    }, RESYNC_BANNER_DELAY_MS);
                }
            };
            if (scheduleResyncRef.current) {
                scheduleResyncRef.current(fire, {
                    preempt
                });
            } else {
                fire();
            }
        } else {
            // Don't take the disconnected fallback mid-handshake. See Story 2.1.2.
            if (terminalStateRef.current === 'CONNECTING' || terminalStateRef.current === 'LOADING') return;
            console.info("[resync] sessionId=".concat(sessionId, " trigger=visibility-or-focus fallback=connect"));
            connectRef.current();
            setShowReconnectButtonRef.current(true);
        }
    }, [
        sessionId,
        clearBannerTimer,
        armStallTimer
    ]);
    // Zero-arg wrapper: `debouncedResync` is registered directly as a DOM event
    // listener below, so useDebouncedCallback forwards whatever argument the
    // browser passes it (the Event object) straight through to the wrapped
    // callback. Without this indirection that Event would land in
    // handleVisibilityOrFocusResyncInner's `preempt` parameter (added for Task
    // 6.1.1.3) and every ordinary visibility/focus resync would be
    // misidentified as a preempting one.
    const handleVisibilityOrFocusResync = (0,react.useCallback)(()=>{
        handleVisibilityOrFocusResyncInner(false);
    }, [
        handleVisibilityOrFocusResyncInner
    ]);
    // Epic 1.2's useDebouncedCallback (ref-backed timer, memoized return) IS the
    // debounce mechanism here — not a hand-rolled setTimeout/ref pair — making
    // Epic 1.2's "first real consumer" framing true of the code (resolves
    // architecture review Concern "Epic 1.2 vs Task 2.1.1d mismatch").
    const debouncedResync = (0,useDebounce/* useDebouncedCallback */.Y)(handleVisibilityOrFocusResync, RESYNC_DEBOUNCE_MS);
    (0,react.useEffect)(()=>{
        document.addEventListener('visibilitychange', debouncedResync);
        window.addEventListener('focus', debouncedResync);
        return ()=>{
            document.removeEventListener('visibilitychange', debouncedResync);
            window.removeEventListener('focus', debouncedResync);
        };
    }, [
        debouncedResync
    ]);
    // Task 6.1.1.3 ("newly-focused preempts queued") — today, nothing reacts to
    // an `isVisible` prop transition at all; only the global visibilitychange/
    // focus listeners above trigger a resync. Gated strictly on
    // `scheduleResync` being provided (i.e. `terminal:resync-stagger` is on and
    // SessionDetailView has wired its stagger queue in) so this new trigger
    // point is entirely absent when the flag is off, preserving AC7 flag-off
    // byte-for-byte parity — without a scheduler there is nothing for
    // "preempt" to mean, since there's no queue to jump ahead of.
    const wasVisibleRef = (0,react.useRef)(isVisible);
    (0,react.useEffect)(()=>{
        const becameVisible = isVisible && !wasVisibleRef.current;
        wasVisibleRef.current = isVisible;
        if (becameVisible && scheduleResyncRef.current) {
            handleVisibilityOrFocusResyncInner(true);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        isVisible
    ]);
    // sessionId-keyed cleanup: a watchdog/resync armed for the previous session
    // must never fire against the next one's connect()/disconnect() (adversarial
    // review Blocker 1 / research features.md race #4).
    (0,react.useEffect)(()=>{
        return ()=>{
            var _setShowReconnectBannerRef_current;
            clearStallTimer();
            clearBannerTimer();
            pendingResyncCompletionRef.current = false;
            pendingResyncIdRef.current = undefined;
            bannerShownRef.current = false;
            resyncStartTimeRef.current = null;
            (_setShowReconnectBannerRef_current = setShowReconnectBannerRef.current) === null || _setShowReconnectBannerRef_current === void 0 ? void 0 : _setShowReconnectBannerRef_current.call(setShowReconnectBannerRef, false);
            markResyncCompleteRef.current();
            markPaneResponseReceivedRef.current();
        };
    }, [
        sessionId,
        clearStallTimer,
        clearBannerTimer
    ]);
    const notifyResyncOutputReceived = (0,react.useCallback)((resyncId)=>{
        if (!pendingResyncCompletionRef.current) return;
        // Only clear pending state when the IDs match, or when either side lacks
        // an ID (correlation-ID flag off, or this output isn't a resync reply at
        // all) — preserves the pre-Epic-3.1 any-output-clears heuristic in that
        // case while preventing a *different* resync's stray output from
        // clearing this one's pending state when IDs are present and mismatch
        // (Epic 3.1, Task 3.1.1.4c / validation.md
        // notifyResyncOutputReceived_should_NotClearPendingResync_When_ResyncIdMismatch).
        if (resyncId && pendingResyncIdRef.current && resyncId !== pendingResyncIdRef.current) {
            // Task 7.1.1.3 (Epic 7.1 observability) — client-side correlation-ID
            // mismatch, mirrored by the server-side log in handleCurrentPaneRequest
            // (connectrpc_websocket.go) for the case where the mismatch instead
            // stems from the server never having echoed an ID at all.
            console.debug("[resync] sessionId=".concat(sessionId, " resync_id mismatch: expected=").concat(pendingResyncIdRef.current, " received=").concat(resyncId));
            return;
        }
        const resyncDurationMs = resyncStartTimeRef.current !== null ? Date.now() - resyncStartTimeRef.current : undefined;
        resyncStartTimeRef.current = null;
        pendingResyncCompletionRef.current = false;
        pendingResyncIdRef.current = undefined;
        clearStallTimer();
        clearBannerTimer();
        // The success path never flips isConnected false, so the pre-existing
        // isConnected-driven banner effect (TerminalOutput.tsx:736-761) won't
        // hide a banner shown by the 2s timer above — hide it explicitly here,
        // but only if the banner was actually shown (Story 2.1.8 AC: a resync
        // that completes before the 2s banner delay must never touch
        // setShowReconnectBanner at all).
        if (bannerShownRef.current) {
            var _setShowReconnectBannerRef_current;
            (_setShowReconnectBannerRef_current = setShowReconnectBannerRef.current) === null || _setShowReconnectBannerRef_current === void 0 ? void 0 : _setShowReconnectBannerRef_current.call(setShowReconnectBannerRef, false);
        }
        bannerShownRef.current = false;
        markResyncCompleteRef.current();
        markPaneResponseReceivedRef.current();
        console.log("[resync] sessionId=".concat(sessionId, " pane response received, resync complete"));
        // Task 7.1.1.4 (Epic 7.1 observability) — success-path duration, alongside
        // the existing stall-watchdog warning (armStallTimer above), so a normal
        // resync's actual latency is visible without waiting for one to stall.
        if (resyncDurationMs !== undefined) {
            console.debug("[resync] sessionId=".concat(sessionId, " resync completed in ").concat(resyncDurationMs, "ms"));
        }
    }, [
        sessionId,
        clearStallTimer,
        clearBannerTimer
    ]);
    // Epic 3.1, Task 3.1.2.1/3.1.2.2/3.1.2.4b — lets TerminalOutput.tsx reset
    // this hook's stall watchdog when a matching resync_id arrives via the
    // sibling useTerminalFlowControl/useTerminalStream resize-resync path,
    // without merging the two hooks. No-op if no resync is pending here.
    //
    // Task 3.1.2.4b: a reset must not be able to mask a genuinely-stalled
    // resync forever just because sibling traffic keeps proving the
    // connection alive. Before re-arming, check this hook's own pending
    // resync_id's recorded start time in the shared outstandingResyncIdsRef
    // map (Task 3.1.2.4a) — if it's been outstanding at or past the 2x
    // escalation ceiling, escalate (force disconnect+reconnect) instead of
    // resetting again. `outstandingResyncIdsRef` is optional (pre-3.1.2.4
    // callers/tests) — resetStallWatchdog always just re-arms when it's absent
    // or the ID's start time isn't recorded, preserving prior behavior.
    const resetStallWatchdog = (0,react.useCallback)(()=>{
        if (!pendingResyncCompletionRef.current) return;
        const ownResyncId = pendingResyncIdRef.current;
        const startedAt = ownResyncId ? outstandingResyncIdsRef === null || outstandingResyncIdsRef === void 0 ? void 0 : outstandingResyncIdsRef.current.get(ownResyncId) : undefined;
        if (startedAt !== undefined && Date.now() - startedAt >= RESYNC_STALL_ESCALATION_CEILING_MS) {
            forceResyncRecovery('escalation');
            return;
        }
        armStallTimer();
    }, [
        armStallTimer,
        forceResyncRecovery,
        outstandingResyncIdsRef
    ]);
    return {
        notifyResyncOutputReceived,
        resetStallWatchdog
    };
}

// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
;// ./src/lib/hooks/useBrowserLogStream.ts
/* __next_internal_client_entry_do_not_use__ useBrowserLogStream auto */ 




const MAX_BUFFER = 50;
const MAX_MSG_LEN = 200;
const FLUSH_INTERVAL_MS = 5000;
// NOTE: If TerminalOutput is ever rendered more than once simultaneously
// (session pool scenario), multiple instances of this hook will each patch
// console.*. Since each install captures the then-current console.log as
// its originals.log, the interceptors chain automatically. Cleanup unwinds
// in reverse React effect order. A module-level reference counter should be
// added if multi-mount becomes a real scenario.
function useBrowserLogStream(options) {
    const enabledRef = (0,react.useRef)(options.enabled);
    const sessionRef = (0,react.useRef)(options.sessionId);
    const baseUrlRef = (0,react.useRef)(options.baseUrl);
    // Keep refs current without triggering re-effects
    (0,react.useEffect)(()=>{
        enabledRef.current = options.enabled;
    }, [
        options.enabled
    ]);
    (0,react.useEffect)(()=>{
        sessionRef.current = options.sessionId;
    }, [
        options.sessionId
    ]);
    (0,react.useEffect)(()=>{
        baseUrlRef.current = options.baseUrl;
    }, [
        options.baseUrl
    ]);
    (0,react.useEffect)(()=>{
        // SSR guard
        if (false) {}
        var _baseUrlRef_current;
        // Always install interceptors — console.error and uncaught errors stream to the
        // server unconditionally (see enqueue()) as a crash-diagnostics safety net;
        // log/warn/debug are gated behind options.enabled, since those are the noisy,
        // opt-in-only levels.
        const apiBase = (_baseUrlRef_current = baseUrlRef.current) !== null && _baseUrlRef_current !== void 0 ? _baseUrlRef_current : (0,config/* getApiBaseUrl */.h)();
        const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
        const buffer = [];
        let flushTimer = null;
        let intercepting = false;
        // Capture originals BEFORE patching
        const originals = {
            log: console.log.bind(console),
            warn: console.warn.bind(console),
            error: console.error.bind(console),
            debug: console.debug.bind(console)
        };
        function truncate(s, max) {
            if (s.length <= max) return s;
            return s.slice(0, max) + "…";
        }
        function argsToMessage(args) {
            try {
                return args.slice(0, 5).map((a)=>typeof a === "object" ? JSON.stringify(a) : String(a)).join(" ");
            } catch (e) {
                return args.slice(0, 5).map(String).join(" ");
            }
        }
        function enqueue(level, args) {
            // Only "debug" is opt-in (noisy, per-frame tracing); log/warn/error
            // stream unconditionally, matching the server's info-and-up default.
            if (level === "debug" && !enabledRef.current) return;
            if (intercepting) return; // reentrancy guard
            intercepting = true;
            try {
                const entry = {
                    level,
                    message: truncate(argsToMessage(args), MAX_MSG_LEN),
                    timestamp: new Date().toISOString(),
                    url: window.location.href,
                    userAgent: navigator.userAgent,
                    sessionId: sessionRef.current
                };
                buffer.push(entry);
                if (buffer.length >= MAX_BUFFER) {
                    flush();
                } else if (!flushTimer) {
                    flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS);
                }
            } finally{
                intercepting = false;
            }
        }
        function flush() {
            if (flushTimer) {
                clearTimeout(flushTimer);
                flushTimer = null;
            }
            if (buffer.length === 0) return;
            const entries = buffer.splice(0);
            // Use ConnectRPC client — fire-and-forget; never recurse into console.
            // Deliberately not cancelled: buffered log entries should still ship
            // even if the effect that scheduled this flush re-runs first.
            // abort-signal-exempt
            client.logClientEvents({
                entries: entries.map((e)=>{
                    var _e_sessionId;
                    return {
                        level: e.level,
                        message: e.message,
                        timestamp: e.timestamp,
                        url: e.url,
                        userAgent: e.userAgent,
                        sessionId: (_e_sessionId = e.sessionId) !== null && _e_sessionId !== void 0 ? _e_sessionId : ""
                    };
                })
            }).catch(()=>{});
        }
        // Install interceptors
        console.log = function() {
            for(var _len = arguments.length, a = new Array(_len), _key = 0; _key < _len; _key++){
                a[_key] = arguments[_key];
            }
            originals.log(...a);
            enqueue("log", a);
        };
        console.warn = function() {
            for(var _len = arguments.length, a = new Array(_len), _key = 0; _key < _len; _key++){
                a[_key] = arguments[_key];
            }
            originals.warn(...a);
            enqueue("warn", a);
        };
        console.error = function() {
            for(var _len = arguments.length, a = new Array(_len), _key = 0; _key < _len; _key++){
                a[_key] = arguments[_key];
            }
            originals.error(...a);
            enqueue("error", a);
        };
        console.debug = function() {
            for(var _len = arguments.length, a = new Array(_len), _key = 0; _key < _len; _key++){
                a[_key] = arguments[_key];
            }
            originals.debug(...a);
            enqueue("debug", a);
        };
        // window.onerror
        const prevOnError = window.onerror;
        window.onerror = (msg, src, line, col, err)=>{
            originals.error("[onerror]", msg, src, line, col, err === null || err === void 0 ? void 0 : err.stack);
            var _err_stack;
            enqueue("error", [
                String(msg),
                "".concat(src, ":").concat(line, ":").concat(col),
                (_err_stack = err === null || err === void 0 ? void 0 : err.stack) !== null && _err_stack !== void 0 ? _err_stack : ""
            ]);
            var _prevOnError;
            return (_prevOnError = prevOnError === null || prevOnError === void 0 ? void 0 : prevOnError(msg, src, line, col, err)) !== null && _prevOnError !== void 0 ? _prevOnError : false;
        };
        // unhandledrejection
        function onUnhandled(e) {
            originals.error("[unhandledrejection]", e.reason);
            enqueue("error", [
                "UnhandledRejection",
                String(e.reason)
            ]);
        }
        window.addEventListener("unhandledrejection", onUnhandled);
        // Page-unload beacon — flush remaining entries synchronously
        function onBeforeUnload() {
            if (buffer.length === 0) return;
            const entries = buffer.splice(0);
            // Use sendBeacon with JSON blob for keepalive semantics on unload.
            // ConnectRPC binary POST is not suitable here; fall back to plain JSON
            // matched by the same route (the server accepts both via HTTP/1.1 fallback).
            const body = JSON.stringify({
                entries: entries.map((e)=>{
                    var _e_sessionId;
                    return {
                        level: e.level,
                        message: e.message,
                        timestamp: e.timestamp,
                        url: e.url,
                        user_agent: e.userAgent,
                        session_id: (_e_sessionId = e.sessionId) !== null && _e_sessionId !== void 0 ? _e_sessionId : ""
                    };
                })
            });
            const blob = new Blob([
                body
            ], {
                type: "application/json"
            });
            if (!navigator.sendBeacon(apiBase + "/session.v1.SessionService/LogClientEvents", blob)) {
                fetch(apiBase + "/session.v1.SessionService/LogClientEvents", {
                    method: "POST",
                    headers: {
                        "Content-Type": "application/json"
                    },
                    body,
                    keepalive: true
                }).catch(()=>{});
            }
        }
        window.addEventListener("beforeunload", onBeforeUnload);
        // Start periodic flush
        flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS);
        // Cleanup: restore originals, cancel timer, clear buffer
        return ()=>{
            console.log = originals.log;
            console.warn = originals.warn;
            console.error = originals.error;
            console.debug = originals.debug;
            window.onerror = prevOnError;
            window.removeEventListener("unhandledrejection", onUnhandled);
            window.removeEventListener("beforeunload", onBeforeUnload);
            if (flushTimer) {
                clearTimeout(flushTimer);
                flushTimer = null;
            }
            // Discard pending buffer on cleanup — the beforeunload beacon handles
            // data preservation on actual page unload.
            buffer.length = 0;
        };
    // Install once per mount — options.enabled is read live via enabledRef inside
    // enqueue(), so toggling it doesn't need to reinstall the console interceptors.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
}

// EXTERNAL MODULE: ./src/lib/hooks/useHandedness.ts
var useHandedness = __webpack_require__(39823);
;// ./src/lib/hooks/useSplitContainerSize.ts
/* __next_internal_client_entry_do_not_use__ useSplitContainerSize auto */ 
/**
 * useSplitContainerSize — tracks the rendered size of a container element
 * via ResizeObserver. Used by the pane keyboard-nudge resize to compute
 * `containerSizePx` for the NUDGE_RESIZE action.
 */ function useSplitContainerSize(ref) {
    const [size, setSize] = (0,react.useState)({
        width: 0,
        height: 0
    });
    (0,react.useEffect)(()=>{
        const el = ref.current;
        if (!el) return;
        const ro = new ResizeObserver((param)=>{
            let [entry] = param;
            const { width, height } = entry.contentRect;
            setSize({
                width,
                height
            });
        });
        ro.observe(el);
        return ()=>ro.disconnect();
    }, [
        ref
    ]);
    return size;
}

// EXTERNAL MODULE: ./src/lib/terminal/TerminalPool.tsx
var TerminalPool = __webpack_require__(48399);
// EXTERNAL MODULE: ./src/lib/terminal/TerminalPool.css.ts
var TerminalPool_css = __webpack_require__(13630);
// EXTERNAL MODULE: ./src/components/ui/LiveRegion.tsx
var LiveRegion = __webpack_require__(28600);
// EXTERNAL MODULE: ./src/lib/notification-policy.ts
var notification_policy = __webpack_require__(35172);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FInputDropBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22QGtleWZyYW1lcyBxZGx6ZW0wIHsKICAwJSB7CiAgICBvcGFjaXR5OiAwOwogICAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpIHRyYW5zbGF0ZVkoNHB4KTsKICB9CiAgMTAwJSB7CiAgICBvcGFjaXR5OiAxOwogICAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpIHRyYW5zbGF0ZVkoMCk7CiAgfQp9Ci5xZGx6ZW0xIHsKICBwb3NpdGlvbjogZml4ZWQ7CiAgYm90dG9tOiA4MHB4OwogIGxlZnQ6IDUwJTsKICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVgoLTUwJSk7CiAgei1pbmRleDogMTA4NTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ5KSB2YXIoLS1fMTllejJyejMwKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenopOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzkpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwogIGJveC1zaGFkb3c6IDAgMnB4IDhweCByZ2JhKDAsMCwwLDAuMyk7CiAgcG9pbnRlci1ldmVudHM6IG5vbmU7CiAgYW5pbWF0aW9uOiBxZGx6ZW0wIDE1MG1zIGVhc2Utb3V0Owp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FInputDropBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22QGtleWZyYW1lcyBxZGx6ZW0wIHsKICAwJSB7CiAgICBvcGFjaXR5OiAwOwogICAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpIHRyYW5zbGF0ZVkoNHB4KTsKICB9CiAgMTAwJSB7CiAgICBvcGFjaXR5OiAxOwogICAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpIHRyYW5zbGF0ZVkoMCk7CiAgfQp9Ci5xZGx6ZW0xIHsKICBwb3NpdGlvbjogZml4ZWQ7CiAgYm90dG9tOiA4MHB4OwogIGxlZnQ6IDUwJTsKICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVgoLTUwJSk7CiAgei1pbmRleDogMTA4NTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ5KSB2YXIoLS1fMTllejJyejMwKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenopOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzkpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwogIGJveC1zaGFkb3c6IDAgMnB4IDhweCByZ2JhKDAsMCwwLDAuMyk7CiAgcG9pbnRlci1ldmVudHM6IG5vbmU7CiAgYW5pbWF0aW9uOiBxZGx6ZW0wIDE1MG1zIGVhc2Utb3V0Owp9_22_7D = __webpack_require__(46849);
;// ./src/components/sessions/InputDropBadge.css.ts


var badge = 'qdlzem1';
;// ./src/components/sessions/InputDropBadge.tsx
/* __next_internal_client_entry_do_not_use__ InputDropBadge auto */ 
// +feature: input-drop-badge





function formatBadgeText(count) {
    return "".concat(count, " keystroke").concat(count === 1 ? "" : "s", " dropped — reconnecting");
}
/**
 * Portal-rendered, auto-dismissing badge that visibly and audibly announces
 * a `DropEpisode` (Story 2.3). Modeled on `XtermTerminal.tsx`'s `copiedToast`
 * pattern — see `docs/reference/css-architecture.md` for the vanilla-extract
 * convention used by `InputDropBadge.css.ts`.
 *
 * The nested `LiveRegion` is unconditionally rendered (never gated on the
 * visual badge's visibility) — see design/ux.md §2.4: a stable, always-
 * present live-region DOM node is what assistive technology reliably detects
 * content mutations on.
 */ function InputDropBadge(param) {
    let { count, episodeSeq } = param;
    const [visible, setVisible] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        // design/ux.md §3.3 — defensive no-op on count <= 0 (should not happen
        // given MessageQueue.close()'s contract, but a normal user-initiated
        // disconnect with an empty queue should never surface a badge).
        if (count <= 0) {
            return;
        }
        // A new episode always (re)shows the badge and restarts its own
        // auto-dismiss hold from this episode's close time — it does not
        // inherit/extend a prior episode's remaining hold time
        // (design/ux.md §2.3 Case C).
        //
        // No manual dedup ref here: the `[episodeSeq, count]` dependency array
        // already guarantees this effect only re-fires when a genuinely new
        // episode arrives, and this effect's own cleanup (returned below) is
        // what clears the previous episode's timer — including under React
        // StrictMode's dev-only setup→cleanup→setup replay on mount, where a
        // separate cleanup-only effect would otherwise clear the timer armed by
        // setup #1 with no corresponding rearm in setup #2.
        setVisible(true);
        const dismissTimer = setTimeout(()=>{
            setVisible(false);
        }, notification_policy/* DEFAULT_TOAST_MS */.tx);
        return ()=>{
            clearTimeout(dismissTimer);
        };
    }, [
        episodeSeq,
        count
    ]);
    if (typeof document === "undefined") {
        return null;
    }
    const badgeText = count > 0 ? formatBadgeText(count) : "";
    // Task 2.3.2 / adversarial-review.md dedup fix: append an invisible
    // (zero-width-space) nonce derived from episodeSeq so two consecutive
    // episodes with an identical human-readable count still produce a
    // distinct underlying text node — `aria-live` only fires on a genuine DOM
    // text mutation, and React bails on a redundant identical-string update.
    const announcementText = badgeText ? "".concat(badgeText).concat("​".repeat(episodeSeq % 2)) : "";
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            visible && count > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: badge,
                "data-testid": "input-drop-badge",
                // Decorative only — the LiveRegion below is the sole announcement
                // channel, so this element must not be independently read by AT.
                "aria-hidden": "true",
                children: [
                    "⚠ ",
                    badgeText
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(LiveRegion/* LiveRegion */.L, {
                role: "alert",
                politeness: "assertive",
                message: announcementText
            })
        ]
    }), document.body);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FConnectionCountIndicator.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81UQU7kMBC8zyv6gjRIGDmZXRbMhZ-gTtxJWji2ZTtMJoi_r-KEhQ2IFeKy8iXuqkpZrm7fPdCpCdhTBBfHK74u4WkHIM8uoJDyLG8AKjeK2KF2RwVyXaGtcC8v8jq_3QE87wB-fiI5-BEeMeyFuC9uaCrDNK66593lYi6z2LvIiZ1VEMhg4keaaZqjN3hSwNawJdEYGuc6Gm6t4ER9VFCTTRTmcotebezKU_bzqDXb9gN0W5kyv8L6oQ1usHor0RmvnXFhCxWL1AVNQUHhR4jOsN6w2jcsEVDzELc_OtxkTuNsEpEneodXr_iRuO3SO8biUg8hzufU1OBg0lzK99itouL2NYci5_C55x_yj01oWEVnhpRDS86r3EjzJixGcv7uMbRsRca_lNJ_kcmvb2fSZMaRdeoUlKX0uZePHScS0WNNCqwLPZp8izQmkftcgaEmZzcJtppGBaVczvt23Eo_wrUf_5rRyzKHdteTZoS9D9RQiCKQHmrSondLetaJBSJb03mOds35sE42Wu5xIb88GVdS9hEIIwm2wg3pZa7_5bbsP7T50gPyGwhAT1jHBAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FConnectionCountIndicator_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81UQU7kMBC8zyv6gjRIGDmZXRbMhZ_gTtxJWji2ZTtMJoi_r_KEhQ2IFeKy8iXuqkpZrm7fPdCpCdhTBBfHK74u4WkHIM8uoJDyLG8AKjeK2KF2RwVyXaGtcC8v8jq_3QE87wB_fiI5_BEeMeyFuC9uaCrDNK66593lYi6z2LvIiZ1VEMhg4keaaZqjN3hSwNawJdEYGuc6Gm6t4ER9VFCTTRTmcotebezKU_bzqDXb9gN0W5kyv8L6oQ1usHor0RmvnXFhCxWL1AVNQUHhR4jOsN6w2jcsEVDzELc_OtxkTuNsEpEneodXr_iRuO3SO8biUg8hzufU1OBg0lzK99itouL2NYci5_C55x_yj01oWEVnhpRDS86r3EjzJixGcv7uMbRsRca_lNJ_kcmvb2fSZMaRdeoUlKX0uZePHScS0WNNCqwLPZp8izQmkftcgaEmZzcJtppGBaVczvt23Eo_wrUf_5rRyzKHdteTZoS9D9RQiCKQHmrSondLetaJBSJb03mOds35sE42Wu5xIb88GVdS9hEIIwm2wg3pZa7_5bbsP7T50gPyGwhAT1jHBAAA_22_7D = __webpack_require__(34897);
;// ./src/components/sessions/ConnectionCountIndicator.css.ts


var ConnectionCountIndicator_css_badge = 'osx6i80';
var icon = 'osx6i81';
var ConnectionCountIndicator_css_pulse = 'osx6i83';
var tooltip = 'osx6i84';
;// ./src/components/sessions/ConnectionCountIndicator.tsx
/* __next_internal_client_entry_do_not_use__ ConnectionCountIndicator auto */ 
// +feature: connection-count-indicator


// design/ux.md's "Rapid count oscillation" edge case (UX-AC-11): debounce the
// value that drives mount/unmount so a flapping reconnect doesn't produce a
// burst of separate mount/announce cycles — mirrors InputDropBadge.tsx's
// episode-coalescing precedent (research/ux.md §3).
const COALESCE_MS = 500;
// design/ux.md's interaction-flow Step 5: the departure ("2 -> 1") must be
// announced, not just silently unmounted — most screen readers only reliably
// announce content *mutations within a still-present* live region, not the
// disappearance of the whole node in the same commit. Hold the departure
// text in the DOM for one announcement cycle before actually unmounting.
const DEPARTURE_HOLD_MS = 1000;
function formatCount(count) {
    return "".concat(count, " connection").concat(count === 1 ? "" : "s", " active");
}
/**
 * Small, non-alarming indicator that another connection (tab, ssq-mux
 * attach, etc.) is attached to the same session (Epic 4.2, Story 4.2.2).
 * Renders only when count > 1 — the overwhelmingly common single-tab case
 * gets zero added visual surface (design/ux.md Surface 1, UX-AC-1).
 *
 * Follows TerminalOutput.tsx's existing reconnecting-banner convention
 * exactly: a single conditionally-mounted `role="status"`/`aria-live="polite"`
 * element, never `role="alert"`. Mounting only on the 1→2 transition (not
 * already true at initial page paint) is what gives "changes-only"
 * announcement semantics for free — a live region's initial-page-load
 * content is not narrated by assistive tech, only content that mutates
 * after the region already exists (Story 4.2.2 AC1).
 *
 * The reverse transition (settled count drops back to <= 1) does NOT unmount
 * immediately: design/ux.md's interaction-flow Step 5 requires an equivalent
 * departure signal ("must not go silent without any signal"), and most
 * screen readers only reliably announce content mutations within a
 * still-present live region, not a node's removal in the same commit. So the
 * departure updates this region's text first, holds it for
 * `DEPARTURE_HOLD_MS`, then unmounts.
 */ function ConnectionCountIndicator(param) {
    let { count, sizeMismatch, pulse } = param;
    const [expanded, setExpanded] = (0,react.useState)(false);
    const [settledCount, setSettledCount] = (0,react.useState)(count);
    const [departureLabel, setDepartureLabel] = (0,react.useState)(null);
    const timerRef = (0,react.useRef)(null);
    const departureTimerRef = (0,react.useRef)(null);
    const prevSettledRef = (0,react.useRef)(settledCount);
    (0,react.useEffect)(()=>{
        if (timerRef.current) {
            clearTimeout(timerRef.current);
        }
        timerRef.current = setTimeout(()=>setSettledCount(count), COALESCE_MS);
        return ()=>{
            if (timerRef.current) {
                clearTimeout(timerRef.current);
            }
        };
    }, [
        count
    ]);
    // Detect the settled-count transition back down to <= 1 and hold a
    // departure announcement in the same live region before unmounting
    // (design/ux.md Step 5).
    (0,react.useEffect)(()=>{
        const prev = prevSettledRef.current;
        prevSettledRef.current = settledCount;
        const isVisible = settledCount !== undefined && settledCount > 1;
        if (isVisible) {
            // Back to (or still at) a real multi-connection count — cancel any
            // in-flight departure hold so the normal indicator takes over.
            if (departureTimerRef.current) {
                clearTimeout(departureTimerRef.current);
                departureTimerRef.current = null;
            }
            setDepartureLabel(null);
            return;
        }
        const wasVisible = prev !== undefined && prev > 1;
        if (wasVisible) {
            setDepartureLabel(formatCount(1));
            departureTimerRef.current = setTimeout(()=>{
                setDepartureLabel(null);
                departureTimerRef.current = null;
            }, DEPARTURE_HOLD_MS);
        }
    }, [
        settledCount
    ]);
    (0,react.useEffect)(()=>{
        return ()=>{
            if (departureTimerRef.current) {
                clearTimeout(departureTimerRef.current);
            }
        };
    }, []);
    if (departureLabel !== null) {
        // Minimal departure render: same live region, no icon/tooltip — just the
        // text mutation a screen reader can pick up before the node disappears.
        return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
            className: ConnectionCountIndicator_css_badge,
            role: "status",
            "aria-live": "polite",
            "aria-label": departureLabel,
            "data-testid": "connection-count-indicator",
            children: departureLabel
        });
    }
    if (settledCount === undefined || settledCount <= 1) {
        return null;
    }
    const label = formatCount(settledCount);
    const tooltipText = sizeMismatch ? "".concat(label, ". Another connection has this session open at a different size.") : label;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: pulse ? "".concat(ConnectionCountIndicator_css_badge, " ").concat(ConnectionCountIndicator_css_pulse) : ConnectionCountIndicator_css_badge,
        role: "status",
        "aria-live": "polite",
        "aria-label": label,
        "data-testid": "connection-count-indicator",
        "data-pulse": pulse ? "true" : undefined,
        tabIndex: 0,
        onMouseEnter: ()=>setExpanded(true),
        onMouseLeave: ()=>setExpanded(false),
        onFocus: ()=>setExpanded(true),
        onBlur: ()=>setExpanded(false),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: "\uD83D\uDC65"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: settledCount
            }),
            expanded && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: tooltip,
                role: "tooltip",
                "data-testid": "connection-count-tooltip",
                children: tooltipText
            })
        ]
    });
}

;// ./src/components/sessions/useDropEpisodeCoalescer.ts
/* __next_internal_client_entry_do_not_use__ useDropEpisodeCoalescer auto */ 
/**
 * Coalesces rapid-fire `report(count)` calls into a single `onFlush(total)`
 * call after `windowMs` of inactivity, so a burst of N dropped keystrokes
 * (Story 2.3 — InputDropBadge) produces exactly one coalesced-episode
 * announcement rather than one per dropped keystroke.
 *
 * Internally: a ref-held running total plus a `setTimeout` that (re)schedules
 * on each `report()` call within the window. Once the window elapses without
 * a new `report()`, `onFlush(total)` fires once with the summed count and the
 * total resets to 0 — a `report()` call after that point starts a brand-new,
 * independent episode (design/ux.md §2.3 Case C: "replace, don't merge
 * across episodes").
 *
 * Extracted (Task 2.3.3a) into its own testable unit rather than left inline
 * in `TerminalOutput.tsx` (1500+ lines) — mirrors this plan's existing
 * precedent of pulling coalescing/latch logic out of large files
 * (`answerDialogOnce`, `runInputReadLoop`).
 */ function useDropEpisodeCoalescer(onFlush, windowMs) {
    const totalRef = (0,react.useRef)(0);
    const timerRef = (0,react.useRef)(null);
    // Keep the latest onFlush without forcing report() to be recreated on
    // every render (mirrors pushMessageRef's "read the ref, not a stale
    // closure" idiom in useTerminalStream.ts).
    const onFlushRef = (0,react.useRef)(onFlush);
    (0,react.useEffect)(()=>{
        onFlushRef.current = onFlush;
    }, [
        onFlush
    ]);
    (0,react.useEffect)(()=>{
        return ()=>{
            if (timerRef.current) {
                clearTimeout(timerRef.current);
                timerRef.current = null;
            }
        };
    }, []);
    const report = (0,react.useCallback)((count)=>{
        totalRef.current += count;
        if (timerRef.current) {
            clearTimeout(timerRef.current);
        }
        timerRef.current = setTimeout(()=>{
            const total = totalRef.current;
            totalRef.current = 0;
            timerRef.current = null;
            onFlushRef.current(total);
        }, windowMs);
    }, [
        windowMs
    ]);
    return report;
}

;// ./src/lib/terminal/EscapeSequenceParser.ts
/**
 * EscapeSequenceParser - Ensures ANSI escape sequences are not split across writes
 *
 * Terminal control codes (ANSI escape sequences) must be written atomically to xterm.js.
 * Splitting them mid-sequence causes terminal corruption. This parser detects partial
 * escape sequences at the end of data chunks and buffers them for the next write.
 *
 * Supported sequences:
 * - CSI: \x1b[ ... (letter)  - Cursor positioning, colors, attributes
 * - OSC: \x1b] ... \x07|\x1b\\ - Operating system commands
 * - Simple: \x1b(letter) - Single-char escapes
 *
 * References:
 * - xterm.js Flow Control Guide: https://xtermjs.org/docs/guides/flowcontrol/
 * - xterm.js Terminal API: https://xtermjs.org/docs/api/terminal/classes/terminal/
 * - ANSI Escape Sequences: https://en.wikipedia.org/wiki/ANSI_escape_code
 * - VT100 Control Sequences: https://vt100.net/docs/vt100-ug/chapter3.html
 */ class EscapeSequenceParser {
    /**
   * Process data chunk and ensure escape sequences are not split.
   * Returns the complete data that can be safely written, buffering any
   * partial escape sequence for the next call.
   *
   * @param data - New data chunk to process
   * @returns Complete data safe to write (includes buffered partial from previous call)
   */ processChunk(data) {
        // Prepend any buffered partial sequence from previous call
        const fullData = this.partialSequence + data;
        this.partialSequence = "";
        const filtered = fullData; // No sequence stripping - xterm.js v6 handles ED2+ED3 correctly
        // Check if data ends with a partial escape sequence
        const partial = this.findPartialEscapeAtEnd(filtered);
        if (partial.length > 0) {
            // Buffer the partial sequence for next call
            this.partialSequence = partial;
            // Return data up to (but not including) the partial sequence
            return filtered.substring(0, filtered.length - partial.length);
        }
        // No partial sequence - safe to write all data
        return filtered;
    }
    /**
   * Get any buffered partial escape sequence (for debugging/testing)
   */ getBuffered() {
        return this.partialSequence;
    }
    /**
   * Clear any buffered data (call on disconnect/error)
   */ reset() {
        this.partialSequence = "";
    }
    /**
   * Detect partial escape sequence at end of string.
   * Returns the partial sequence string if found, empty string otherwise.
   *
   * Detection strategy:
   * 1. Scan backwards from end for ESC character (\x1b)
   * 2. Validate if this starts a complete or partial sequence
   * 3. CSI/OSC sequences need terminator, simple escapes need one more char
   */ findPartialEscapeAtEnd(data) {
        if (data.length === 0) return "";
        // Maximum length to scan backward - OSC title strings and DCS payloads can exceed 20 bytes
        const scanLength = Math.min(256, data.length);
        const startIndex = data.length - scanLength;
        // Scan backward for last ESC character
        let lastEscIndex = -1;
        for(let i = data.length - 1; i >= startIndex; i--){
            if (data.charCodeAt(i) === 0x1b) {
                lastEscIndex = i;
                break;
            }
        }
        if (lastEscIndex === -1) {
            return ""; // No escape sequence at end
        }
        // Extract potential escape sequence from last ESC to end
        const potentialSeq = data.substring(lastEscIndex);
        // Check if this is a complete sequence or partial
        if (this.isCompleteEscapeSequence(potentialSeq)) {
            return ""; // Complete sequence - no buffering needed
        }
        // Partial sequence - buffer it
        return potentialSeq;
    }
    /**
   * Check if escape sequence is complete (has terminator).
   *
   * ANSI escape sequence patterns:
   * - CSI: ESC [ ... (letter A-Z, a-z)
   * - OSC: ESC ] ... BEL(\x07) or ESC\
   * - Simple: ESC (letter)
   * - C1: ESC (special char in 0x40-0x5F range)
   */ isCompleteEscapeSequence(seq) {
        if (seq.length === 0 || seq.charCodeAt(0) !== 0x1b) {
            return true; // Not an escape sequence
        }
        if (seq.length === 1) {
            return false; // Just ESC - definitely partial
        }
        const secondChar = seq[1];
        // CSI sequence: ESC [
        if (secondChar === '[') {
            // CSI ends with a letter (A-Z, a-z) after optional parameters
            // Examples: ESC[31m (red), ESC[2K (clear line), ESC[10;20H (cursor position)
            return this.hasCSITerminator(seq);
        }
        // OSC sequence: ESC ]
        if (secondChar === ']') {
            // OSC ends with BEL (\x07) or ESC\ (\x1b\x5c)
            return this.hasOSCTerminator(seq);
        }
        // DCS (P), PM (^), APC (_), SOS (X) — all terminated by ST only (ESC \)
        if (EscapeSequenceParser.STRING_MODE_CHARS.has(secondChar)) {
            return seq.includes('\x1b\\');
        }
        // Simple escape: ESC + single char
        // If we have 2+ characters and second is not [ or ], it's likely complete
        // Examples: ESC 7 (save cursor), ESC 8 (restore cursor), ESC M (reverse index)
        if (seq.length === 2) {
            return true; // Simple 2-char escape is complete
        }
        // C1 control codes: ESC + char in 0x40-0x5F range
        const secondCharCode = seq.charCodeAt(1);
        if (secondCharCode >= 0x40 && secondCharCode <= 0x5F) {
            return true; // C1 sequence is complete
        }
        // Unknown sequence type - assume complete to avoid infinite buffering
        return true;
    }
    /**
   * Check if CSI sequence has terminator (letter A-Z, a-z).
   * CSI format: ESC [ (params) (letter)
   */ hasCSITerminator(seq) {
        if (seq.length < 3) return false; // Need at least ESC[X
        // Scan for terminator (letter)
        for(let i = 2; i < seq.length; i++){
            const code = seq.charCodeAt(i);
            // Terminators are letters A-Z (0x41-0x5A) or a-z (0x61-0x7A)
            if (code >= 0x41 && code <= 0x5A || code >= 0x61 && code <= 0x7A) {
                return true; // Found terminator
            }
            // Valid intermediate characters: digits, semicolon, ? , etc.
            // If we hit an invalid character, sequence is malformed
            const isValidIntermediate = code >= 0x30 && code <= 0x3F || // 0-9, :, ;, <, =, >, ?
            code >= 0x20 && code <= 0x2F; // space, !, ", #, $, %, &, ', (, ), *, +, comma, -, ., /
            if (!isValidIntermediate) {
                // Unexpected character - treat as complete to avoid blocking
                return true;
            }
        }
        // No terminator found yet - partial sequence
        return false;
    }
    /**
   * Check if OSC sequence has terminator (BEL or ESC\).
   * OSC format: ESC ] (params) BEL or ESC ]  (params) ESC \
   */ hasOSCTerminator(seq) {
        if (seq.length < 3) return false; // Need at least ESC]X
        // Check for BEL terminator (\x07)
        if (seq.indexOf('\x07') !== -1) {
            return true;
        }
        // Check for ESC\ terminator (\x1b\x5c or \x1b\\)
        if (seq.indexOf('\x1b\\') !== -1 || seq.indexOf('\x1b\x5c') !== -1) {
            return true;
        }
        // No terminator found - partial sequence
        return false;
    }
    constructor(){
        this.partialSequence = "";
    }
} /**
 * Example usage:
 *
 * const parser = new EscapeSequenceParser();
 *
 * // Chunk 1: Ends with partial CSI sequence
 * const chunk1 = "Hello \x1b[31";
 * const toWrite1 = parser.processChunk(chunk1); // Returns "Hello " (buffers "\x1b[31")
 *
 * // Chunk 2: Completes the CSI sequence
 * const chunk2 = "mWorld";
 * const toWrite2 = parser.processChunk(chunk2); // Returns "\x1b[31mWorld" (prepends buffered)
 */ 
// Allocated once at class definition; avoids a per-call object allocation in the hot path.
EscapeSequenceParser.STRING_MODE_CHARS = new Set([
    'P',
    '^',
    '_',
    'X'
]);

;// ./src/lib/terminal/RedrawThrottler.ts
/**
 * RedrawThrottler - Coalesces rapid full-screen redraws to max 30 FPS.
 *
 * Claude performs complete screen redraws at 12-25 FPS, causing visible flicker.
 * This throttler holds rapid redraws and flushes the latest one at a capped rate.
 *
 * Split out of TerminalStreamManager.ts (which still owns the one instance per
 * manager) — it has no dependency on TerminalStreamManager's internals, only
 * the onFlush callback passed to its constructor.
 */ class RedrawThrottler {
    process(chunk) {
        // Detect genuine full-screen redraws: cursor-up followed immediately by an erase sequence.
        // NOTE: \x1b[H (cursor-home) is intentionally excluded - it is also emitted during
        // incremental Ink-style renders and must not be classified as a full redraw.
        // Scoping the check to the first 32 bytes avoids false positives in large output chunks.
        const match = /^\x1b\[(\d+)A(?:\x1b\[2K|\x1b\[J)/.exec(chunk.substring(0, 32));
        if (!match) {
            this.flushPending();
            return chunk;
        }
        // \x1b[NA\x1b[2K (cursor-up N, erase ONE line) is also exactly how a multi-line
        // Ink-style TUI redraws a SINGLE targeted line (e.g. a spinner or token counter),
        // not just how it repaints the whole viewport — the leading bytes alone can't tell
        // the two apart. Coalescing is only safe when the new candidate targets the SAME
        // cursor-up region as the one already pending (repeated repaints of one region,
        // which is the flicker this throttler exists to suppress). A different region
        // arriving mid-window is flushed immediately instead of silently overwriting the
        // pending one — otherwise the pending region's update is dropped forever, since
        // nothing else resends it.
        const upCount = Number(match[1]);
        if (this.pendingRedraw !== null && upCount !== this.pendingRedrawUpCount) {
            this.flushPending();
        }
        this.pendingRedraw = chunk;
        this.pendingRedrawUpCount = upCount;
        if (!this.throttleTimer) {
            this.throttleTimer = setTimeout(()=>{
                this.flushPending();
            }, this.throttleMs);
        }
        return null; // Don't output yet
    }
    flushPending() {
        if (this.pendingRedraw) {
            this.onFlush(this.pendingRedraw);
            this.pendingRedraw = null;
            this.pendingRedrawUpCount = null;
        }
        if (this.throttleTimer) {
            clearTimeout(this.throttleTimer);
            this.throttleTimer = null;
        }
    }
    cleanup() {
        this.flushPending();
    }
    constructor(onFlush){
        this.pendingRedraw = null;
        // Cursor-up count (the \d+ in \x1b[\d+A) of the currently pending redraw —
        // identifies which screen region it repaints, so a same-region redraw can
        // be coalesced onto it while a different-region one cannot (see below).
        this.pendingRedrawUpCount = null;
        this.throttleTimer = null;
        this.throttleMs = 33 // ~30fps; coalesces burst full-screen redraws
        ;
        this.onFlush = onFlush;
    }
}

;// ./src/lib/terminal/TerminalDebugMonitor.ts
/** True when verbose terminal debug logging is enabled via localStorage. */ function isDebugTerminalEnabled() {
    return  true && localStorage.getItem("debug-terminal") === "true";
}
/**
 * TerminalDebugMonitor - optional write/refresh monkey-patching that logs
 * timing and call-site info when the `debug-terminal` localStorage flag is
 * set. Debug-only instrumentation, not on the hot write path.
 *
 * Split out of TerminalStreamManager.ts, which owns one instance per manager
 * and drives it purely through install()/restore().
 */ class TerminalDebugMonitor {
    /** Wrap terminal.write/refresh to log timing/call-site info. Idempotent. */ install(terminal) {
        if (this.installed) return;
        this.originalWrite = terminal.write.bind(terminal);
        this.originalRefresh = terminal.refresh.bind(terminal);
        terminal.write = this.wrapWrite(terminal);
        terminal.refresh = this.wrapRefresh(terminal);
        this.installed = true;
        if (isDebugTerminalEnabled()) {
            console.log('[TerminalStreamManager] Refresh and write monitoring installed');
        }
    }
    /** Restore the original terminal.write/refresh captured by install(). Idempotent. */ restore(terminal) {
        if (!this.installed || !this.originalWrite || !this.originalRefresh) return;
        terminal.write = this.originalWrite;
        terminal.refresh = this.originalRefresh;
        this.installed = false;
        // Null out refs to terminal internals to prevent GC retention (Pitfall #7):
        // when the terminal is disposed while the manager is still alive, these refs
        // hold closed terminal objects preventing garbage collection.
        this.originalWrite = null;
        this.originalRefresh = null;
    }
    wrapWrite(terminal) {
        return (data, callback)=>{
            var _this_originalWrite, _this;
            const now = performance.now();
            const timeSinceLastWrite = now - this.lastWriteTime;
            this.lastWriteTime = now;
            this.writeCount++;
            if (isDebugTerminalEnabled()) {
                console.log('[XtermWrite]', {
                    writeCount: this.writeCount,
                    dataLength: typeof data === 'string' ? data.length : data.byteLength,
                    timeSinceLastWrite: "".concat(timeSinceLastWrite.toFixed(2), "ms"),
                    cursorY: terminal.buffer.active.cursorY,
                    timestamp: new Date().toISOString()
                });
            }
            return (_this_originalWrite = (_this = this).originalWrite) === null || _this_originalWrite === void 0 ? void 0 : _this_originalWrite.call(_this, data, callback);
        };
    }
    wrapRefresh(terminal) {
        return (start, end)=>{
            var _this_originalRefresh, _this;
            if (isDebugTerminalEnabled()) {
                var _stackTrace_split_;
                const stackTrace = new Error().stack;
                const caller = (stackTrace === null || stackTrace === void 0 ? void 0 : (_stackTrace_split_ = stackTrace.split('\n')[2]) === null || _stackTrace_split_ === void 0 ? void 0 : _stackTrace_split_.trim()) || 'unknown';
                const timeSinceLastWrite = performance.now() - this.lastWriteTime;
                console.log('[XtermRefresh] Refresh called', {
                    start,
                    end,
                    rows: terminal.rows,
                    timeSinceLastWrite: "".concat(timeSinceLastWrite.toFixed(2), "ms"),
                    recentWrites: this.writeCount,
                    caller: caller.replace(/^at /, ''),
                    possibleRaceCondition: timeSinceLastWrite < 50,
                    timestamp: new Date().toISOString()
                });
                this.writeCount = 0;
            }
            return (_this_originalRefresh = (_this = this).originalRefresh) === null || _this_originalRefresh === void 0 ? void 0 : _this_originalRefresh.call(_this, start, end);
        };
    }
    constructor(){
        this.originalWrite = null;
        this.originalRefresh = null;
        this.lastWriteTime = 0;
        this.writeCount = 0;
        this.installed = false;
    }
}

;// ./src/lib/terminal/TerminalStreamManager.ts
/**
 * TerminalStreamManager - Write buffering, watermark flow control, chunked writes,
 * redraw throttling, and escape sequence safety for xterm.js.
 *
 * This is a pure imperative class (no React dependency). It encapsulates the
 * terminal write pipeline that was previously scattered across TerminalOutput.tsx.
 *
 * Lifecycle: Create an instance in a ref, call methods from effects, call cleanup() on unmount.
 */ 


// ---- Constants ----
const HIGH_WATERMARK = 100000; // 100KB - pause when buffer exceeds this
const LOW_WATERMARK = 10000; // 10KB - resume when buffer drops below this
const CHUNK_SIZE = 16384; // 16KB chunks
const CHUNK_DELAY_MS = 0; // Yield to event loop between chunks
/**
 * Mirrors connectrpc_websocket.go's `ansiSnapshotPrefix` (DECSTR + erase-screen +
 * cursor-home). The server prepends this exact byte sequence to every full-pane
 * replacement snapshot it sends — the post-resize capture, visibility/focus resync,
 * and reconnect snapshot all use it — so its presence at the start of a chunk is a
 * reliable, trigger-independent signal that this chunk replaces the whole screen
 * rather than appending to it.
 *
 * Checked once, here, inside write() — the single funnel every output path (live
 * streaming, resize snapshots, resync snapshots, and the RESIZING-queue flush in
 * TerminalOutput.tsx) already goes through — rather than at each call site, so a
 * future call site can't reintroduce the stale-buffer-overlap bug by forgetting to
 * check it.
 */ const ANSI_SNAPSHOT_PREFIX = "\x1b[!p\x1b[2J\x1b[H";
class TerminalStreamManager {
    /** Set a callback invoked once on first output received. */ setOnFirstOutput(cb) {
        this.onFirstOutput = cb;
    }
    /** Set a callback invoked whenever a full-pane replacement snapshot is detected in write(). */ setOnFullSnapshot(cb) {
        this.onFullSnapshot = cb;
    }
    /**
   * Set a callback invoked whenever altScreenActive changes value (Task 1.4.0a) —
   * called only on an actual transition, mirroring the server-side
   * `AltScreenTracker.Observe`'s `changed` return convention.
   */ setOnAltScreenChange(cb) {
        this.onAltScreenChange = cb;
    }
    /**
   * Marks altScreenActive true from the server-authoritative
   * TerminalOutput.alt_screen_active hint (only ever present on an
   * initial-connect/post-resize snapshot — see useTerminalStream.ts's
   * onAltScreenActiveHint doc comment) rather than inferring it from
   * scanning content bytes, which a capture-pane-derived snapshot can never
   * carry the DECSET 1049h marker for. Fires onAltScreenChange only on an
   * actual transition, mirroring the server-side `AltScreenTracker.Observe`'s
   * `changed` return convention.
   */ setAltScreenActive() {
        if (!this.altScreenActive) {
            var _this_onAltScreenChange, _this;
            this.altScreenActive = true;
            (_this_onAltScreenChange = (_this = this).onAltScreenChange) === null || _this_onAltScreenChange === void 0 ? void 0 : _this_onAltScreenChange.call(_this, true);
        }
    }
    /** See setAltScreenActive()'s doc comment — the inactive counterpart. */ setAltScreenInactive() {
        if (this.altScreenActive) {
            var _this_onAltScreenChange, _this;
            this.altScreenActive = false;
            (_this_onAltScreenChange = (_this = this).onAltScreenChange) === null || _this_onAltScreenChange === void 0 ? void 0 : _this_onAltScreenChange.call(_this, false);
        }
    }
    /** Set a callback invoked when handleAppScrollback() dispatches a captured frame (Task 1.4.1a). */ setOnAppScrollback(cb) {
        this.onAppScrollback = cb;
    }
    /**
   * Dispatch a captured AppScrollbackResponse frame to the registered
   * onAppScrollback callback. Never routes through write()/onFullSnapshot —
   * see this.onAppScrollback's doc comment.
   */ handleAppScrollback(frame) {
        var _this_onAppScrollback, _this;
        (_this_onAppScrollback = (_this = this).onAppScrollback) === null || _this_onAppScrollback === void 0 ? void 0 : _this_onAppScrollback.call(_this, frame);
    }
    /** Inject a SerializeAddon for prependScrollbackBatch (serialize-clear-rewrite pattern). */ setSerializeAddon(addon) {
        this.serializeAddon = addon;
    }
    /** Update the sendFlowControl callback (e.g., after reconnection). */ updateSendFlowControl(fn) {
        this.sendFlowControl = fn;
    }
    /** Install debug instrumentation (write/refresh monkey-patching). See TerminalDebugMonitor. */ installDebugMonitor() {
        this.debugMonitor.install(this.terminal);
    }
    /** True when verbose terminal debug logging is enabled via localStorage. */ isDebugTerminalEnabled() {
        return  true && localStorage.getItem("debug-terminal") === "true";
    }
    /**
   * Write output to the terminal with throttling, escape sequence safety, and flow control.
   * This is the primary entry point for streaming output.
   */ write(output) {
        if (this.disposed) return;
        // Write-lock: if writeInitialContent or prependScrollbackBatch is in progress,
        // queue live output to avoid interleaving history with live data (Pitfall #1 and #8).
        if (this.isWritingInitialContent) {
            this.pendingLiveWrites.push(output);
            return;
        }
        // A full-pane replacement snapshot must replace the screen, not append to it —
        // see ANSI_SNAPSHOT_PREFIX's doc comment. Without this, stale rows from before
        // the snapshot (reflowed under the terminal's previous wrap state, or content
        // the user had scrolled past) sit underneath the fresh content instead of being
        // replaced by it.
        if (output.startsWith(ANSI_SNAPSHOT_PREFIX)) {
            var _this_onFullSnapshot, _this;
            this.terminal.clear();
            (_this_onFullSnapshot = (_this = this).onFullSnapshot) === null || _this_onFullSnapshot === void 0 ? void 0 : _this_onFullSnapshot.call(_this);
        }
        // Track first output
        if (!this.firstOutputReceived) {
            var _this_onFirstOutput, _this1;
            this.firstOutputReceived = true;
            (_this_onFirstOutput = (_this1 = this).onFirstOutput) === null || _this_onFirstOutput === void 0 ? void 0 : _this_onFirstOutput.call(_this1);
        }
        // Throttle rapid full-screen redraws to prevent flickering
        const result = this.redrawThrottler.process(output);
        if (result) {
            const safeOutput = this.escapeParser.processChunk(result);
            this.handleProcessedOutput(safeOutput);
        }
    }
    /**
   * Write initial content (e.g., from currentPaneResponse).
   * Clears terminal, enqueues content via chunked write, scrolls to bottom.
   * Holds a write-lock for the duration to prevent live output from interleaving.
   *
   * @returns Promise that resolves when the write completes.
   */ async writeInitialContent(content) {
        if (this.disposed) return;
        this.isWritingInitialContent = true;
        try {
            this.terminal.clear();
            // Note: initial/history content here is a tmux capture-pane-derived
            // snapshot, which can never itself carry a DECSET 1049h marker (it's a
            // rendered text snapshot, not a replay of the raw byte stream) — so
            // scanning it for alt-screen transitions would never find anything.
            // setAltScreenActive()/setAltScreenInactive(), driven by the
            // server-authoritative TerminalOutput.alt_screen_active field, is what
            // actually establishes correct alt-screen state on connect; see their
            // doc comments.
            await this.enqueueWrite(content);
            this.terminal.scrollToBottom();
            // Delayed scrolls in case content is still rendering.
            // Large scrollback can take >100ms to fully render on slower machines.
            setTimeout(()=>this.terminal.scrollToBottom(), 10);
            setTimeout(()=>this.terminal.scrollToBottom(), 100);
            setTimeout(()=>this.terminal.scrollToBottom(), 500);
        } finally{
            this.isWritingInitialContent = false;
            // Flush any live writes that arrived during history loading (in order)
            const pending = this.pendingLiveWrites.splice(0);
            for (const chunk of pending){
                this.write(chunk);
            }
        }
    }
    /**
   * Prepend historical scrollback above the current terminal content.
   * Uses the serialize-clear-rewrite pattern (ADR-010):
   *   1. Serialize current buffer (if SerializeAddon available)
   *   2. Clear terminal
   *   3. Write older history
   *   4. Write saved current content
   *   5. Scroll to bottom
   *
   * Holds a write-lock for the duration to prevent live output from interleaving.
   *
   * @returns Promise that resolves when the write completes.
   */ async prependScrollbackBatch(content) {
        if (this.disposed) return;
        this.isWritingInitialContent = true;
        try {
            if (!this.serializeAddon) {
                // Degraded mode: SerializeAddon not available — cannot serialize current buffer before
                // clearing, so we skip the clear and simply append the history content. The history
                // will appear after the current content (wrong order) but current content is preserved.
                // This is preferable to clearing and losing the existing terminal output (Bug 6 fix).
                console.warn('[TerminalStreamManager] SerializeAddon not available — appending history without clear (degraded mode)');
                await this.enqueueWrite(content);
                return;
            }
            // Serialize current buffer state before clearing
            const serialized = this.serializeAddon.serialize();
            this.terminal.clear();
            await this.enqueueWrite(content);
            if (serialized) {
                await this.enqueueWrite(serialized);
            }
            this.terminal.scrollToBottom();
        } finally{
            this.isWritingInitialContent = false;
            // Flush any live writes that arrived during history loading (in order)
            const pending = this.pendingLiveWrites.splice(0);
            for (const chunk of pending){
                this.write(chunk);
            }
        }
    }
    /**
   * Task 1.4.0a — track altScreenActive off the same enter/exit markers the
   * needsRefresh scan in handleProcessedOutput already watches for exit.
   * lastIndexOf + compare (rather than two independent .includes() checks)
   * resolves the rare case where a single throttled chunk contains both an
   * exit and a re-entry by picking whichever marker occurs later in the chunk.
   */ updateAltScreenActive(safeOutput) {
        const altEnterIdx = Math.max(safeOutput.lastIndexOf('\x1b[?1049h'), safeOutput.lastIndexOf('\x1b[?47h'));
        const altExitIdx = Math.max(safeOutput.lastIndexOf('\x1b[?1049l'), safeOutput.lastIndexOf('\x1b[?47l'));
        if (altEnterIdx < 0 && altExitIdx < 0) return;
        if (altEnterIdx > altExitIdx) {
            this.setAltScreenActive();
        } else {
            this.setAltScreenInactive();
        }
    }
    /**
   * Handle processed output by routing through the appropriate write path
   * (raw direct/chunked or state batching).
   */ handleProcessedOutput(safeOutput) {
        if (safeOutput.length === 0) return;
        this.updateAltScreenActive(safeOutput);
        // Detect terminal mode transitions that may need a refresh
        const needsRefresh = safeOutput.includes('\x1b[?1049l') || safeOutput.includes('\x1b[?47l') || safeOutput.includes('\x1b[?2026l') || safeOutput.includes('\x1b[?25h');
        if (needsRefresh) {
            requestAnimationFrame(()=>{
                this.terminal.refresh(0, this.terminal.rows - 1);
                if ( true && localStorage.getItem("debug-terminal") === "true") {
                    console.log('[TerminalStreamManager] Forced refresh after mode transition', {
                        alternateScreenExit: safeOutput.includes('\x1b[?1049l') || safeOutput.includes('\x1b[?47l'),
                        syncUpdateEnd: safeOutput.includes('\x1b[?2026l'),
                        cursorRestore: safeOutput.includes('\x1b[?25h')
                    });
                }
            });
        }
        // For small writes, write directly for lowest latency with flow control tracking
        if (safeOutput.length <= CHUNK_SIZE) {
            this.writeDirectWithFlowControl(safeOutput);
        } else {
            // Large write - use chunked queue to prevent UI freezing
            this.enqueueWrite(safeOutput);
        }
    }
    /** Direct write with watermark tracking (fast path for small data). */ writeDirectWithFlowControl(data) {
        this.pendingWrites++;
        this.totalBytesWritten += data.length;
        this.watermark += data.length;
        // Check if we should pause upstream
        if (this.watermark > HIGH_WATERMARK && !this.isPaused) {
            this.isPaused = true;
            console.warn("[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: ".concat(this.watermark, " bytes)"));
            this.sendFlowControl(true, this.watermark);
        }
        this.terminal.write(data, ()=>{
            this.pendingWrites = Math.max(0, this.pendingWrites - 1);
            this.totalBytesCompleted += data.length;
            this.watermark = Math.max(0, this.watermark - data.length);
            if (this.watermark < LOW_WATERMARK && this.isPaused) {
                this.isPaused = false;
                console.log("[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: ".concat(this.watermark, " bytes)"));
                this.sendFlowControl(false, this.watermark);
            }
            if ( true && localStorage.getItem("debug-terminal") === "true") {
                console.log('[FlowControl] Write completed', {
                    bytes: data.length,
                    watermark: this.watermark,
                    paused: this.isPaused,
                    pending: this.pendingWrites,
                    totalWritten: this.totalBytesWritten,
                    totalCompleted: this.totalBytesCompleted,
                    backlog: this.totalBytesWritten - this.totalBytesCompleted
                });
            }
        });
    }
    /** Enqueue data for chunked writing (large data path). */ enqueueWrite(data) {
        return new Promise((resolve)=>{
            this.writeQueue.push({
                data,
                resolve
            });
            this.processWriteQueue();
        });
    }
    /** Process the write queue sequentially with chunking. */ async processWriteQueue() {
        if (this.isProcessingQueue || this.writeQueue.length === 0) {
            return;
        }
        this.isProcessingQueue = true;
        while(this.writeQueue.length > 0){
            if (this.disposed) {
                // Drop remaining queued items without writing — cleanup() already
                // cleared writeQueue, but a chunked item mid-flight re-checks this
                // flag between chunks (writeChunkedWithFlowControl), so this outer
                // guard only matters if cleanup() ran between two top-level queue
                // items.
                this.writeQueue.length = 0;
                break;
            }
            const item = this.writeQueue[0];
            const data = item.data;
            if (data.length <= CHUNK_SIZE) {
                await this.writeWithFlowControl(data);
            } else {
                await this.writeChunkedWithFlowControl(data);
            }
            this.writeQueue.shift();
            item.resolve();
        }
        this.isProcessingQueue = false;
    }
    /**
   * Write large data in CHUNK_SIZE pieces via writeWithFlowControl(), yielding
   * to the UI event loop between chunks. Re-checks `disposed` before every
   * chunk (Task 3.6 / Bug 3) — dispose() can land between any two chunks of a
   * large writeInitialContent()/prependScrollbackBatch() write, and the whole
   * point is to stop mid-item, not just between queue items.
   */ async writeChunkedWithFlowControl(data) {
        const totalChunks = Math.ceil(data.length / CHUNK_SIZE);
        if (this.isDebugTerminalEnabled()) {
            console.log("[FlowControl] Chunking large write: ".concat(data.length, " bytes into ").concat(totalChunks, " chunks"));
        }
        for(let i = 0; i < data.length; i += CHUNK_SIZE){
            if (this.disposed) break;
            const chunk = data.slice(i, Math.min(i + CHUNK_SIZE, data.length));
            const chunkIndex = Math.floor(i / CHUNK_SIZE) + 1;
            await this.writeWithFlowControl(chunk, "chunk ".concat(chunkIndex, "/").concat(totalChunks));
            // Yield to UI between chunks
            if (i + CHUNK_SIZE < data.length) {
                await this.yieldToUI();
            }
        }
        if (this.isDebugTerminalEnabled()) {
            console.log("[FlowControl] Completed chunked write: ".concat(data.length, " bytes"));
        }
    }
    /** Yield to the UI event loop between chunks (next rAF, or a fixed delay if configured). */ yieldToUI() {
        return new Promise((resolve)=>{
            if (CHUNK_DELAY_MS > 0) {
                setTimeout(resolve, CHUNK_DELAY_MS);
            } else {
                requestAnimationFrame(()=>resolve());
            }
        });
    }
    /**
   * Write one piece of data to the terminal with watermark-based flow-control
   * pause/resume around it. Shared by processWriteQueue()'s small-direct-write
   * path and writeChunkedWithFlowControl()'s per-chunk path — both need the
   * identical pause-before/resume-after shape, previously duplicated inline.
   *
   * @param logContext optional context (e.g. "chunk 2/5") interpolated into
   *   the watermark pause/resume debug log lines.
   */ writeWithFlowControl(piece, logContext) {
        return new Promise((resolve)=>{
            this.watermark += piece.length;
            if (this.watermark > HIGH_WATERMARK && !this.isPaused) {
                this.isPaused = true;
                console.warn("[FlowControl] HIGH WATERMARK EXCEEDED".concat(logContext ? " during ".concat(logContext) : '', " - Pausing stream (watermark: ").concat(this.watermark, " bytes)"));
                this.sendFlowControl(true, this.watermark);
            }
            this.terminal.write(piece, ()=>{
                this.watermark = Math.max(0, this.watermark - piece.length);
                if (this.watermark < LOW_WATERMARK && this.isPaused) {
                    this.isPaused = false;
                    console.log("[FlowControl] LOW WATERMARK REACHED".concat(logContext ? " after ".concat(logContext) : '', " - Resuming stream (watermark: ").concat(this.watermark, " bytes)"));
                    this.sendFlowControl(false, this.watermark);
                }
                resolve();
            });
        });
    }
    /** Clean up all pending state on unmount. */ cleanup() {
        // Reset escape sequence parser
        this.escapeParser.reset();
        // Cleanup redraw throttler
        this.redrawThrottler.cleanup();
        // Restore original terminal methods if debug monitoring was installed
        // (also nulls out the captured refs — see TerminalDebugMonitor.restore's
        // Pitfall #7 doc comment on GC retention of a disposed terminal).
        this.debugMonitor.restore(this.terminal);
        // Clear any pending live writes
        this.pendingLiveWrites.length = 0;
        // Task 3.6 / Bug 3 — stop issuing further terminal.write() calls after
        // cleanup(). Pooled terminals (TerminalPool.tsx) reuse the underlying
        // xterm.js Terminal across sessions, so a manager whose owning
        // TerminalOutput has already unmounted mid-chunked-write must not keep
        // writing into a Terminal that may now belong to a different session.
        // Checked in write()/writeInitialContent()/prependScrollbackBatch() and
        // between chunks in writeChunkedWithFlowControl() — never mid-chunk (an
        // in-flight terminal.write() call always completes; only the *next* one
        // is skipped).
        this.disposed = true;
        this.writeQueue.length = 0;
    }
    // ---- Test/debug accessors ----
    /** Get current watermark level (for testing). */ getWatermark() {
        return this.watermark;
    }
    /** Check if flow control is paused (for testing). */ getIsPaused() {
        return this.isPaused;
    }
    constructor(terminal, sendFlowControl){
        // Pending write operations tracking
        this.pendingWrites = 0;
        this.totalBytesWritten = 0;
        this.totalBytesCompleted = 0;
        // Watermark flow control
        this.watermark = 0;
        this.isPaused = false;
        // Chunked write queue
        this.writeQueue = [];
        this.isProcessingQueue = false;
        // Escape sequence parser
        this.escapeParser = new EscapeSequenceParser();
        // Debug instrumentation (write/refresh monkey-patching), gated on the
        // `debug-terminal` localStorage flag — see installDebugMonitor().
        this.debugMonitor = new TerminalDebugMonitor();
        // Write-lock: prevents live output from being written while initial/paged history is loading.
        // Live writes received during the lock are queued and flushed once the history write completes.
        this.isWritingInitialContent = false;
        this.pendingLiveWrites = [];
        // First output tracking
        this.firstOutputReceived = false;
        this.onFirstOutput = null;
        // Invoked whenever write() detects a full-pane replacement snapshot (see
        // ANSI_SNAPSHOT_PREFIX), after this.terminal.clear() has already run — lets the
        // caller reset state it owns that the manager doesn't (e.g. TerminalOutput.tsx's
        // scrollback-paging refs).
        this.onFullSnapshot = null;
        // Story 1.4.0 — client-local mirror of the server's AltScreenTracker (Task 1.1.1a),
        // observed independently off the same escape sequences already flowing through
        // write() rather than round-tripped over the wire. Purely a local scroll-up-trigger
        // decision (see TerminalOutput.tsx's wheel listener / useTerminalGestures wiring);
        // the server keeps its own copy for the safety-gate role, and the two are not meant
        // to be reconciled — they observe the same bytes independently by design.
        this.altScreenActive = false;
        this.onAltScreenChange = null;
        // Story 1.4.1 — registered handler for a captured AppScrollbackResponse frame.
        // Deliberately dispatched through handleAppScrollback(), a distinct entry point
        // from write()'s ANSI_SNAPSHOT_PREFIX sniff, so a concurrent resize resync's
        // onFullSnapshot can never fire for this message type and vice versa
        // (research/architecture.md §6).
        this.onAppScrollback = null;
        // Story 3 (Task 3.6 / Bug 3) — set by cleanup(). See cleanup()'s doc comment.
        this.disposed = false;
        this.terminal = terminal;
        this.sendFlowControl = sendFlowControl;
        this.redrawThrottler = new RedrawThrottler((data)=>{
            const safeOutput = this.escapeParser.processChunk(data);
            this.handleProcessedOutput(safeOutput);
        });
    }
}
// Export constants for testing


;// ./src/lib/terminal/ForwardEchoGuard.ts
/**
 * Recognizes the redraw echo of a scroll-forward (DELIVERED/AT_TOP) in the live
 * output stream, so TerminalOutput doesn't mistake it for a genuine live resume
 * (Ctrl+L) and drop the "viewing history" banner.
 *
 * ForwardScroll's PageUp send (session/instance_scroll_forward.go) causes a real
 * redraw that echoes back as a normal frame. Content comparison, not a time
 * window or frame count, is what's invariant: the echo's arrival timing isn't
 * guaranteed (some redraws never echo at all), but it always redraws the same
 * pane content that was just captured.
 */ // Generous headroom over any real single-pane capture.
const MAX_ECHO_BUFFER = 20000;
/**
 * Strips ANSI CSI/OSC sequences (full ECMA-48 grammar, not just digits/semicolons
 * -- narrower patterns miss e.g. DECSTR's `\x1b[!p`) and all whitespace, so two
 * redraws of the same pane compare equal despite differing line-wrap columns.
 * Deliberately not truncated: with no alt-screen scrollback, the server's capture
 * can start at a different offset into the same content than the live echo.
 */ function contentSignature(raw) {
    return raw.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "").replace(/\x1b\][^\x07]*(\x07|\x1b\\)/g, "").replace(/\s+/g, "");
}
/**
 * Longest N such that signature.slice(0, N) is a contiguous substring of
 * haystack. Monotonic in N, so a binary search needs O(log N) includes() calls
 * rather than an O(N^2) character scan.
 */ function longestSignaturePrefixContained(signature, haystack) {
    let lo = 0;
    let hi = signature.length;
    while(lo < hi){
        const mid = Math.ceil((lo + hi) / 2);
        if (haystack.includes(signature.slice(0, mid))) {
            lo = mid;
        } else {
            hi = mid - 1;
        }
    }
    return lo;
}
class ForwardEchoGuard {
    /** Records the content of a just-delivered forward; empty content disarms the guard. */ record(content) {
        this.signature = content ? contentSignature(content) : null;
        this.buffer = "";
        this.matchedPrefixLen = 0;
    }
    /** Disarms the guard (e.g. after a full-pane snapshot makes the forward stale). */ clear() {
        this.signature = null;
    }
    /** Whether `output` is recognizable as the echo of the last recorded forward. */ isSelfEcho(output) {
        const signature = this.signature;
        if (!signature) return false;
        const outSig = contentSignature(output);
        if (outSig.length === 0) return true; // no content to judge either way; assume still mid-echo
        const buffered = (this.buffer + outSig).slice(-MAX_ECHO_BUFFER);
        const matchedLen = longestSignaturePrefixContained(signature, buffered);
        if (matchedLen >= signature.length) {
            // Whole signature seen: resolved, so the next frame is judged fresh.
            this.buffer = "";
            this.matchedPrefixLen = 0;
            return true;
        }
        if (matchedLen > this.matchedPrefixLen) {
            // Genuine progress toward completing the same multi-frame echo.
            this.buffer = buffered;
            this.matchedPrefixLen = matchedLen;
            return true;
        }
        // No progress: judge this frame on its own content, not the stale buffer.
        const isEchoAlone = outSig.includes(signature) || signature.includes(outSig);
        this.buffer = isEchoAlone ? outSig : "";
        this.matchedPrefixLen = isEchoAlone ? longestSignaturePrefixContained(signature, outSig) : 0;
        return isEchoAlone;
    }
    constructor(){
        this.signature = null;
        // A redraw echo can legally arrive split across several frames, where no
        // single frame is a substring match but their concatenation is. Progress must
        // strictly increase each frame to keep accumulating; a frame that doesn't
        // extend the match is judged alone and the buffer resets, so a stale partial
        // match can't "stick" and misclassify a later genuine live resume.
        this.buffer = "";
        this.matchedPrefixLen = 0;
    }
}

;// ./src/components/sessions/useScrollLoadingPill.ts
/* __next_internal_client_entry_do_not_use__ useScrollLoadingPill auto */ 
const SHOW_DELAY_MS = 150;
const STALL_AFTER_MS = 8000;
/**
 * Story 1.4.5 — scroll-forward loading pill state.
 *
 * Shared by BOTH the tmux-native paged-scrollback trigger and Story 1.4.0's
 * app-forwarded alt-screen triggers (design/ux.md Surface 1), so the pill can't
 * drift out of sync with which trigger fired. Owns its timers and clears them on
 * unmount.
 */ function useScrollLoadingPill() {
    const [pillState, setPillState] = (0,react.useState)({
        visible: false,
        stalled: false
    });
    const showTimerRef = (0,react.useRef)(null);
    const stallTimerRef = (0,react.useRef)(null);
    const clearScrollLoadingTimers = (0,react.useCallback)(()=>{
        if (showTimerRef.current) {
            clearTimeout(showTimerRef.current);
            showTimerRef.current = null;
        }
        if (stallTimerRef.current) {
            clearTimeout(stallTimerRef.current);
            stallTimerRef.current = null;
        }
    }, []);
    // Called from every response handler (tmux-native ScrollbackResponse and every
    // AppScrollbackResponse outcome) so the pill never persists past outcome
    // delivery (Task 1.4.5b).
    const clearScrollLoadingState = (0,react.useCallback)(()=>{
        clearScrollLoadingTimers();
        setPillState({
            visible: false,
            stalled: false
        });
    }, [
        clearScrollLoadingTimers
    ]);
    // Starts the show-delay / stalled timers; the caller then sends the request.
    // `program`, if known, drives the pill's app-forwarded copy; omitted entirely
    // for the tmux-native path.
    const startScrollLoadingTimers = (0,react.useCallback)((program)=>{
        clearScrollLoadingTimers();
        showTimerRef.current = setTimeout(()=>{
            setPillState({
                visible: true,
                stalled: false,
                program
            });
        }, SHOW_DELAY_MS);
        stallTimerRef.current = setTimeout(()=>{
            setPillState({
                visible: true,
                stalled: true,
                program
            });
        }, STALL_AFTER_MS);
    }, [
        clearScrollLoadingTimers
    ]);
    (0,react.useEffect)(()=>clearScrollLoadingTimers, [
        clearScrollLoadingTimers
    ]);
    return {
        pillState,
        clearScrollLoadingState,
        startScrollLoadingTimers
    };
}

;// ./src/lib/terminal/TerminalDimensionCache.ts
/**
 * Constants for minimum and default terminal dimensions.
 *
 * Dimensions smaller than MIN_COLS/MIN_ROWS are transient (e.g., from xterm.js before
 * the CSS container has finished laying out). Caching or connecting at those dimensions
 * produces a garbled terminal on the next view. The first resize event often fires at
 * e.g., 10x6 before layout is complete.
 *
 * We also reject xterm.js default dimensions (80x24) to prevent cache corruption — the
 * logic in saveDimensions will not overwrite a larger existing cache with smaller
 * dimensions from the xterm.js default mount behavior.
 */ const MIN_COLS = 30;
const MIN_ROWS = 10;
/** xterm.js's default terminal dimensions — rejected to prevent cache corruption. */ const XTERM_DEFAULT_COLS = 80;
const XTERM_DEFAULT_ROWS = 24;
/**
 * Retrieve cached terminal dimensions for a given session.
 *
 * @param sessionId - The session identifier used as the cache key
 * @returns The cached dimensions, or null if not found or on error
 */ function getCachedDimensions(sessionId) {
    if (false) {}
    try {
        const key = "terminal-dimensions-".concat(sessionId);
        const cached = localStorage.getItem(key);
        if (cached) {
            var _dims_cellWidth, _dims_cellHeight;
            const dims = JSON.parse(cached);
            console.log("[TerminalDimensionCache] Loaded cached dimensions for ".concat(sessionId, ": ").concat(dims.cols, "x").concat(dims.rows, " (cell: ").concat((_dims_cellWidth = dims.cellWidth) === null || _dims_cellWidth === void 0 ? void 0 : _dims_cellWidth.toFixed(2), "x").concat((_dims_cellHeight = dims.cellHeight) === null || _dims_cellHeight === void 0 ? void 0 : _dims_cellHeight.toFixed(2), ")"));
            return dims;
        }
    } catch (err) {
        console.warn('[TerminalDimensionCache] Failed to load cached dimensions:', err);
        return null;
    }
    return null;
}
/**
 * Get the current window dimensions with validation against minimum values.
 *
 * @returns Object with cols, rows, cellWidth, cellHeight, or null if dimensions invalid
 */ function getCurrentWindowDimensions() {
    if (false) {}
    // Get window inner dimensions
    const cols = Math.max(MIN_COLS, Math.min(window.innerWidth / 8, XTERM_DEFAULT_COLS));
    const rows = Math.max(MIN_ROWS, Math.min(window.innerHeight / 18, XTERM_DEFAULT_ROWS));
    console.log("[TerminalDimensionCache] Calculated window dimensions: ".concat(cols, "x").concat(rows));
    return {
        cols,
        rows
    };
}
/**
 * Validate cached terminal dimensions against current font configuration.
 *
 * @param cached - Cached dimensions from localStorage (must include cols, rows, optionally cellWidth/cellHeight, fontSize/fontFamily)
 * @param fontSize - Current font size in px (from DEFAULT_TERMINAL_CONFIG)
 * @param fontFamily - Current font family (from DEFAULT_TERMINAL_CONFIG)
 * @returns Validated dimensions, or null if cache is stale/corrupt
 */ function validateCellDimensions(cached, fontSize, fontFamily) {
    // Create a copy to avoid mutating the cached value
    const validated = {
        cols: cached.cols,
        rows: cached.rows
    };
    // Font configuration changed — invalidate cell dimensions
    if (cached.fontSize != null && cached.fontFamily != null && (cached.fontSize !== fontSize || cached.fontFamily !== fontFamily)) {
        console.log("[TerminalDimensionCache] Invalidating stale cache due to font config change: " + "".concat(cached.fontSize, "px/").concat(cached.fontFamily, " → ").concat(fontSize, "px/").concat(fontFamily));
        return null;
    }
    // Preserve cell dimensions when font unchanged, but reset font-related fields
    if (cached.cellWidth != null && cached.cellHeight != null) {
        validated.cellWidth = cached.cellWidth;
        validated.cellHeight = cached.cellHeight;
        validated.fontSize = fontSize;
        validated.fontFamily = fontFamily;
    }
    console.log("[TerminalDimensionCache] Validated cached dimensions: ".concat(validated.cols, "x").concat(validated.rows));
    return validated;
}
/** Rejects xterm.js defaults (80x24) and sub-minimum dimensions so they never corrupt the cache. */ function isValidToCache(cols, rows) {
    if (cols === XTERM_DEFAULT_COLS && rows === XTERM_DEFAULT_ROWS) {
        console.log("[TerminalDimensionCache] Rejecting xterm.js default dimensions (".concat(cols, "x").concat(rows, ") for caching"));
        return false;
    }
    if (cols < MIN_COLS || rows < MIN_ROWS) {
        console.log("[TerminalDimensionCache] Rejecting sub-minimum dimensions (".concat(cols, "x").concat(rows, ") for caching"));
        return false;
    }
    return true;
}
/**
 * Save terminal dimensions to localStorage for a given session.
 *
 * @param sessionId - The session identifier used as the cache key
 * @param cols - Number of terminal columns
 * @param rows - Number of terminal rows
 * @param options - Optional configuration for cell dimensions and font metadata
 */ function saveDimensions(sessionId, cols, rows, options) {
    if (false) {}
    try {
        const key = "terminal-dimensions-".concat(sessionId);
        const payload = {
            cols,
            rows
        };
        // Validate dimensions before caching
        if (!isValidToCache(cols, rows)) {
            return;
        }
        // Check if these dimensions are better than existing cache
        if (!shouldOverwriteCache(key, cols, rows, options)) {
            return;
        }
        if ((options === null || options === void 0 ? void 0 : options.cellWidth) != null && (options === null || options === void 0 ? void 0 : options.cellHeight) != null) {
            payload.cellWidth = options.cellWidth;
            payload.cellHeight = options.cellHeight;
            payload.fontSize = options.fontSize;
            payload.fontFamily = options.fontFamily;
        }
        localStorage.setItem(key, JSON.stringify(payload));
        console.log("[TerminalDimensionCache] Saved dimensions for ".concat(sessionId, ": ").concat(cols, "x").concat(rows).concat((options === null || options === void 0 ? void 0 : options.cellWidth) != null ? " (cell: ".concat(options.cellWidth.toFixed(2), "x").concat(options.cellHeight.toFixed(2), ")") : ''));
    } catch (err) {
        console.warn('[TerminalDimensionCache] Failed to save dimensions:', err);
    }
}
/**
 * Check if we should overwrite the existing cache entry.
 *
 * @param key - The localStorage key to check
 * @param cols - Number of terminal columns
 * @param rows - Number of terminal rows
 * @param options - Optional configuration for cell dimensions
 * @returns true if we should overwrite the cache, false otherwise
 */ function shouldOverwriteCache(key, cols, rows, options) {
    const existing = localStorage.getItem(key);
    if (!existing) return true;
    try {
        const existingDims = JSON.parse(existing);
        // Only overwrite if new dimensions are larger (better)
        if (existingDims.cols >= cols && existingDims.rows >= rows) {
            console.log("[TerminalDimensionCache] Skipping smaller dimensions ".concat(cols, "x").concat(rows, " (existing: ").concat(existingDims.cols, "x").concat(existingDims.rows, ")"));
            return false;
        }
        // If existing has cell dimensions and new doesn't, preserve existing
        if (existingDims.cellWidth != null && existingDims.cellHeight != null && ((options === null || options === void 0 ? void 0 : options.cellWidth) == null || (options === null || options === void 0 ? void 0 : options.cellHeight) == null)) {
            console.log("[TerminalDimensionCache] Skipping dimensions without cell data (existing has cell: ".concat(existingDims.cellWidth, "x").concat(existingDims.cellHeight, ")"));
            return false;
        }
    } catch (parseErr) {
        console.warn('[TerminalDimensionCache] Failed to parse existing dimensions:', parseErr);
        return false;
    }
    return true;
}

;// ./src/lib/terminal/preSizing.ts
/**
 * Pure helpers for TerminalOutput's pre-sizing: reading xterm's rendered cell
 * metrics and deriving cols/rows from a container's pixel size, so a connection
 * can start before xterm fires its first onResize.
 */ 
/**
 * Cell pixel metrics from xterm.js's private render service, or null when
 * unavailable (API changed, not rendered yet). isFinite() guards against
 * NaN/Infinity a corrupted render-service state could return.
 */ function readRenderedCellMetrics(terminal) {
    var _terminal__core__renderService_dimensions_css, _terminal__core__renderService_dimensions, _terminal__core__renderService, _terminal__core;
    const cell = terminal === null || terminal === void 0 ? void 0 : (_terminal__core = terminal._core) === null || _terminal__core === void 0 ? void 0 : (_terminal__core__renderService = _terminal__core._renderService) === null || _terminal__core__renderService === void 0 ? void 0 : (_terminal__core__renderService_dimensions = _terminal__core__renderService.dimensions) === null || _terminal__core__renderService_dimensions === void 0 ? void 0 : (_terminal__core__renderService_dimensions_css = _terminal__core__renderService_dimensions.css) === null || _terminal__core__renderService_dimensions_css === void 0 ? void 0 : _terminal__core__renderService_dimensions_css.cell;
    if ((cell === null || cell === void 0 ? void 0 : cell.width) && (cell === null || cell === void 0 ? void 0 : cell.height) && isFinite(cell.width) && isFinite(cell.height)) {
        return {
            cellWidth: cell.width,
            cellHeight: cell.height
        };
    }
    return null;
}
/**
 * cols/rows that fit in a container of the given pixel size. Floors, so the
 * result never exceeds what actually fits.
 */ function computePreSize(container, cell) {
    if (!(container.width > 0 && container.height > 0)) {
        return {
            ok: false,
            reason: "zero-size"
        };
    }
    const cols = Math.floor(container.width / cell.cellWidth);
    const rows = Math.floor(container.height / cell.cellHeight);
    if (cols < MIN_COLS || rows < MIN_ROWS) {
        return {
            ok: false,
            reason: "below-minimum",
            cols,
            rows
        };
    }
    return {
        ok: true,
        cols,
        rows
    };
}

// EXTERNAL MODULE: ./src/lib/config/terminalConfig.ts
var terminalConfig = __webpack_require__(81073);
;// ./src/components/sessions/useTerminalSizing.ts
/* __next_internal_client_entry_do_not_use__ useTerminalResizeHandler,useSessionConnectionBootstrap auto */ /**
 * Size-stability and initial-connection logic for TerminalOutput.
 *
 * The refs are owned by the component (not this module) because useTerminalStream
 * is called with `lastResizeRef.current` as an input while its outputs
 * (`connect`, `isConnected`, `resize`) are what this logic consumes. Passing
 * them in keeps that cycle out of the hooks without changing useTerminalStream.
 */ 



/** Handles xterm's onResize: caches dimensions, drives size-stability connect, forwards resizes. */ function useTerminalResizeHandler(param) {
    let { sessionId, xtermRef, isConnected, error, connect, resize, clearBufferBeforeResize, setIsWaitingForStableSize, onTerminalResized, refs } = param;
    const { lastResizeRef, hasInitiatedConnectionRef, hasCachedDimensionsRef, sizeStabilityTimeoutRef, isMountedRef, metricsRef, settleBypassPendingRef } = refs;
    return (0,react.useCallback)((cols, rows)=>{
        console.log("[TerminalOutput] Terminal resized to ".concat(cols, "x").concat(rows));
        onTerminalResized === null || onTerminalResized === void 0 ? void 0 : onTerminalResized(cols, rows);
        const lastResize = lastResizeRef.current;
        const sizeChanged = !lastResize || lastResize.cols !== cols || lastResize.rows !== rows;
        if (sizeChanged) {
            lastResizeRef.current = {
                cols,
                rows
            };
            console.log("[TerminalOutput] Saved resize dimensions: ".concat(cols, "x").concat(rows));
            // Only persist dimensions that are plausibly real — transient tiny values
            // (e.g. 10x6) fired before the CSS container finishes layout would otherwise
            // corrupt the cache and cause the next session view to connect at the wrong size.
            if (cols >= MIN_COLS && rows >= MIN_ROWS) {
                var _xtermRef_current, _terminal_options, _terminal_options1;
                // Also capture cell pixel dimensions so TerminalOutput can pre-calculate
                // cols/rows from the container size on the next mount, enabling an immediate
                // connection before xterm fires its first onResize event.
                const terminal = (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal;
                var _terminal_options_fontSize, _terminal_options_fontFamily;
                saveDimensions(sessionId, cols, rows, {
                    ...readRenderedCellMetrics(terminal),
                    fontSize: (_terminal_options_fontSize = terminal === null || terminal === void 0 ? void 0 : (_terminal_options = terminal.options) === null || _terminal_options === void 0 ? void 0 : _terminal_options.fontSize) !== null && _terminal_options_fontSize !== void 0 ? _terminal_options_fontSize : 14,
                    fontFamily: (_terminal_options_fontFamily = terminal === null || terminal === void 0 ? void 0 : (_terminal_options1 = terminal.options) === null || _terminal_options1 === void 0 ? void 0 : _terminal_options1.fontFamily) !== null && _terminal_options_fontFamily !== void 0 ? _terminal_options_fontFamily : 'Menlo, Monaco, "Courier New", monospace'
                });
            } else {
                console.log("[TerminalOutput] Skipping cache write for tiny dimensions ".concat(cols, "x").concat(rows, " (below ").concat(MIN_COLS, "x").concat(MIN_ROWS, ")"));
            }
            if (metricsRef.current.firstResizeTime === null) {
                metricsRef.current.firstResizeTime = performance.now();
            }
            metricsRef.current.resizeCount++;
            // Skip size stability wait if we have cached dimensions, but only when
            // the cached size (lastResize) is itself reasonable — a stale tiny cache
            // entry would otherwise bypass the stability wait and connect at the wrong size.
            if (hasCachedDimensionsRef.current && !hasInitiatedConnectionRef.current && !isConnected && !error && isMountedRef.current) {
                const initDims = {
                    cols,
                    rows
                };
                if (initDims.cols >= MIN_COLS && initDims.rows >= MIN_ROWS) {
                    console.log("[TerminalOutput] Using cached dimensions, skipping stability wait (".concat(initDims.cols, "x").concat(initDims.rows, ")"));
                    metricsRef.current.sizeStableTime = performance.now();
                    metricsRef.current.connectionInitTime = performance.now();
                    hasInitiatedConnectionRef.current = true;
                    setIsWaitingForStableSize(false);
                    connect(initDims.cols, initDims.rows);
                    return;
                } else {
                    // Cached value is too small — treat as no cache and wait for stable size.
                    console.log("[TerminalOutput] Cached dimensions ".concat(initDims.cols, "x").concat(initDims.rows, " too small, falling through to stability wait"));
                    hasCachedDimensionsRef.current = false; // prevents future onResize events from fast-connecting on stale cache
                }
            }
            // Event-driven size stability detection for initial connection
            if (!hasInitiatedConnectionRef.current && !isConnected && !error && isMountedRef.current) {
                if (sizeStabilityTimeoutRef.current) {
                    clearTimeout(sizeStabilityTimeoutRef.current);
                }
                console.log("[TerminalOutput] Size changed, waiting for layout to stabilize...");
                setIsWaitingForStableSize(true);
                sizeStabilityTimeoutRef.current = setTimeout(()=>{
                    requestAnimationFrame(()=>{
                        requestAnimationFrame(()=>{
                            if (!hasInitiatedConnectionRef.current && !isConnected && isMountedRef.current) {
                                const stableSize = lastResizeRef.current;
                                if (stableSize) {
                                    metricsRef.current.sizeStableTime = performance.now();
                                    metricsRef.current.connectionInitTime = performance.now();
                                    console.log("[TerminalOutput] Layout stable at ".concat(stableSize.cols, "x").concat(stableSize.rows, ", initiating connection"));
                                    hasInitiatedConnectionRef.current = true;
                                    setIsWaitingForStableSize(false);
                                    connect(stableSize.cols, stableSize.rows);
                                }
                            }
                        });
                    });
                    sizeStabilityTimeoutRef.current = null;
                }, 50);
            }
        }
        if (!isConnected) {
            console.log("[TerminalOutput] Resize blocked - not connected (".concat(cols, "x").concat(rows, ")"));
            return;
        }
        if (!sizeChanged) {
            console.log("[TerminalOutput] Resize blocked - unchanged (".concat(cols, "x").concat(rows, ")"));
            return;
        }
        console.log("[TerminalOutput] Sending resize: ".concat(cols, "x").concat(rows, " (prev: ").concat((lastResize === null || lastResize === void 0 ? void 0 : lastResize.cols) || 'none', "x").concat((lastResize === null || lastResize === void 0 ? void 0 : lastResize.rows) || 'none', ")"));
        clearBufferBeforeResize();
        if (settleBypassPendingRef === null || settleBypassPendingRef === void 0 ? void 0 : settleBypassPendingRef.current) {
            settleBypassPendingRef.current = false;
            resize(cols, rows, false, {
                bypassBounceHold: true
            });
        } else {
            resize(cols, rows);
        }
    }, [
        isConnected,
        resize,
        connect,
        error,
        sessionId,
        clearBufferBeforeResize,
        onTerminalResized,
        xtermRef
    ]);
}
/**
 * The three effects that start a connection for a session, in a fixed order:
 * pre-size from cache, session-switch connect, then post-disconnect connect.
 */ function useSessionConnectionBootstrap(param) {
    let { sessionId, containerWidth, terminalContainerRef, xtermRef, isConnected, connect, setIsWaitingForStableSize, setIsLoadingInitialContent, setConnectionAttempts, setShowReconnectButton, reconnectTimeoutRef, previousConnectionStateRef, streamManagerRef, refs } = param;
    const { lastResizeRef, hasInitiatedConnectionRef, hasCachedDimensionsRef, pendingConnectAfterDisconnectRef, isMountedRef, metricsRef } = refs;
    // Initialize with cached dimensions on mount.
    // When cell pixel metrics are also cached, pre-calculate cols/rows from the
    // container's current pixel size so the session-switch effect can connect
    // immediately — before xterm.js fires its first onResize event.
    //
    // ORDERING INVARIANT: This effect MUST remain defined before the session-switch
    // effect below (both share the [sessionId] dependency). React runs same-dependency
    // effects in definition order, so this effect runs first and populates
    // lastResizeRef before the session-switch effect reads it to trigger connect().
    // Moving this effect below the session-switch effect will silently break pre-sizing.
    (0,react.useEffect)(()=>{
        const rawCached = getCachedDimensions(sessionId);
        // Validate cell dims against current font config (R1.6): stale dims from a different
        // font configuration produce an incorrect initial fit() and wrong initial resize.
        // loadTerminalConfig() is what the pooled XtermTerminal renders with (useConfig), so
        // validating against it keeps cached cell dims in sync with the actual font (Bug 4 fix).
        const { fontSize: currentFontSize, fontFamily: currentFontFamily } = (0,terminalConfig/* loadTerminalConfig */.X6)();
        const cached = rawCached ? validateCellDimensions(rawCached, currentFontSize, currentFontFamily) : null;
        if (cached && cached.cols >= MIN_COLS && cached.rows >= MIN_ROWS) {
            hasCachedDimensionsRef.current = true;
            console.log("[TerminalOutput] Initialized with cached dimensions: ".concat(cached.cols, "x").concat(cached.rows));
            if (cached.cellWidth && cached.cellHeight && terminalContainerRef.current) {
                // Guard: only use getBoundingClientRect after the ResizeObserver has confirmed
                // the container has a real constrained width (not the full pre-layout browser width).
                // containerWidth > 0 means layout is complete and the pane has its actual dimensions.
                if (containerWidth > 0) {
                    const rect = terminalContainerRef.current.getBoundingClientRect();
                    const preSize = computePreSize(rect, {
                        cellWidth: cached.cellWidth,
                        cellHeight: cached.cellHeight
                    });
                    if (preSize.ok) {
                        const { cols: preCols, rows: preRows } = preSize;
                        console.log("[TerminalOutput] Pre-sizing: ".concat(rect.width, "\xd7").concat(rect.height, "px / ") + "".concat(cached.cellWidth.toFixed(2), "\xd7").concat(cached.cellHeight.toFixed(2), "px/cell → ").concat(preCols, "\xd7").concat(preRows));
                        lastResizeRef.current = {
                            cols: preCols,
                            rows: preRows
                        };
                        // If this effect fired because containerWidth became non-zero (i.e. ResizeObserver
                        // fired after the initial render), the session-switch effect won't re-run because
                        // sessionId didn't change. We must initiate the connection here in that case.
                        // On a sessionId-triggered render, session-switch runs after this and connects,
                        // so we only act when no connection has been initiated yet.
                        const isXtermDefault = preCols === XTERM_DEFAULT_COLS && preRows === XTERM_DEFAULT_ROWS;
                        if (!hasInitiatedConnectionRef.current && !isConnected && isMountedRef.current && !isXtermDefault) {
                            var // Grow the xterm buffer to preCols/preRows BEFORE connecting — otherwise the
                            // terminal is still at its 80x24 constructor default and the capture-pane
                            // snapshot's cursor-positioning sequences for rows beyond 24 are silently
                            // dropped, leaving them unpainted until a later resize forces a full repaint.
                            _xtermRef_current;
                            hasInitiatedConnectionRef.current = true;
                            setIsWaitingForStableSize(false);
                            (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.resize(preCols, preRows);
                            connect(preCols, preRows);
                        }
                    } else if (preSize.reason === "below-minimum") {
                        console.log("[TerminalOutput] Pre-sizing skipped: calculated ".concat(preSize.cols, "x").concat(preSize.rows, " below minimum"));
                    } else {
                        console.log("[TerminalOutput] Pre-sizing skipped: container has zero size");
                    }
                } else {
                    console.log("[TerminalOutput] Pre-sizing deferred: container layout not yet resolved (containerSize.width=0)");
                }
            }
        } else if (cached) {
            console.log("[TerminalOutput] Ignoring stale cached dimensions ".concat(cached.cols, "x").concat(cached.rows, " (below ").concat(MIN_COLS, "x").concat(MIN_ROWS, ")"));
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionId,
        containerWidth
    ]);
    // Reset loading state when switching sessions and trigger reconnect
    (0,react.useEffect)(()=>{
        setIsLoadingInitialContent(true);
        hasInitiatedConnectionRef.current = false;
        metricsRef.current.mountTime = performance.now();
        metricsRef.current.firstOutputTime = null;
        // Reset connection tracking so the new session doesn't inherit stale state
        previousConnectionStateRef.current = false;
        setConnectionAttempts(0);
        setShowReconnectButton(false);
        if (reconnectTimeoutRef.current) {
            clearTimeout(reconnectTimeoutRef.current);
            reconnectTimeoutRef.current = null;
        }
        // Reset stream manager for new session
        if (streamManagerRef.current) {
            streamManagerRef.current.cleanup();
            streamManagerRef.current = null;
        }
        // Connect immediately if already disconnected (e.g. first load, or was already disconnected)
        // Otherwise set the pending flag so we connect once the in-progress disconnect resolves
        if (!isConnected) {
            const dims = lastResizeRef.current;
            // Guard: don't fast-connect with xterm default dims (80×24). Those are the
            // terminal's initial values before fitAddon.fit() measures the container —
            // if Bug 1 corrupted the cache to 80×24, using them here would cause a thin
            // PTY. The resize handler will fire with the actual dims and connect normally.
            const isXtermDefault = (dims === null || dims === void 0 ? void 0 : dims.cols) === XTERM_DEFAULT_COLS && (dims === null || dims === void 0 ? void 0 : dims.rows) === XTERM_DEFAULT_ROWS;
            if (dims && isMountedRef.current && !isXtermDefault) {
                hasInitiatedConnectionRef.current = true;
                setIsWaitingForStableSize(false);
                connect(dims.cols, dims.rows);
            }
        // If no dims yet (or dims are xterm defaults), resize handler will fire and trigger connect normally
        } else {
            // Was connected to previous session — disconnect() is in-flight (async).
            // Mark pending so the isConnected→false transition triggers connect below.
            pendingConnectAfterDisconnectRef.current = true;
        }
        // Safety net: if the container is hidden at mount (display:none, 0×0), the
        // ResizeObserver zero-size guard prevents fitAddon.fit(), so no resize event
        // fires and the stability timer never starts. After 5s, attempt to connect
        // with whatever valid cached dims are available, or skip silently.
        const safetyTimeout = setTimeout(()=>{
            if (!hasInitiatedConnectionRef.current && isMountedRef.current) {
                const dims = lastResizeRef.current;
                if (dims && dims.cols >= MIN_COLS && dims.rows >= MIN_ROWS) {
                    console.log("[TerminalOutput] Safety timeout: connecting with cached dims ".concat(dims.cols, "x").concat(dims.rows, " (container may have been hidden at mount)"));
                    hasInitiatedConnectionRef.current = true;
                    setIsWaitingForStableSize(false);
                    connect(dims.cols, dims.rows);
                } else {
                    console.log("[TerminalOutput] Safety timeout: no valid dims available, container still not visible");
                }
            }
        }, 5000);
        return ()=>{
            clearTimeout(safetyTimeout);
            setIsLoadingInitialContent(false);
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionId
    ]);
    // When a session switch happened while connected, the previous disconnect() is async (up to 1s).
    // This effect fires once isConnected transitions to false, completing the switch.
    (0,react.useEffect)(()=>{
        if (!isConnected && pendingConnectAfterDisconnectRef.current && !hasInitiatedConnectionRef.current && isMountedRef.current) {
            pendingConnectAfterDisconnectRef.current = false;
            const dims = lastResizeRef.current;
            if (dims) {
                console.log("[TerminalOutput] Post-disconnect connect for new session: ".concat(dims.cols, "x").concat(dims.rows));
                hasInitiatedConnectionRef.current = true;
                setIsWaitingForStableSize(false);
                connect(dims.cols, dims.rows);
            }
        // If no dims, the resize handler will fire and connect normally
        }
    }, [
        isConnected,
        connect
    ]);
}

;// ./src/components/sessions/useBlockedToast.ts
/* __next_internal_client_entry_do_not_use__ useBlockedToast auto */ 


const CONNECTION_PULSE_MS = 600;
/**
 * Story 1.4.3 — blocked-outcome toast plus the ConnectionCountIndicator pulse.
 * Owns its timers and clears them on unmount.
 */ function useBlockedToast() {
    const [blockedToast, setBlockedToast] = (0,react.useState)(null);
    const seqRef = (0,react.useRef)(0);
    const toastTimerRef = (0,react.useRef)(null);
    const [connectionPulse, setConnectionPulse] = (0,react.useState)(false);
    const pulseTimerRef = (0,react.useRef)(null);
    const dismissBlockedToast = (0,react.useCallback)(()=>{
        if (toastTimerRef.current) {
            clearTimeout(toastTimerRef.current);
            toastTimerRef.current = null;
        }
        setBlockedToast(null);
    }, []);
    // Keyed on blocked_reason. UNSPECIFIED (a gate failure the client should never
    // actually attempt a request for, per Epic 1.1's eligibility check) falls back
    // to the MULTIPLE_VIEWERS copy as the least-wrong default rather than showing no
    // message — a deliberate P2 tradeoff (pre-mortem.md Failure #2), accepted since
    // UNSPECIFIED should be unreachable in practice.
    const showBlockedOutcome = (0,react.useCallback)((frame)=>{
        seqRef.current += 1;
        setBlockedToast({
            reason: frame.blockedReason,
            program: frame.program,
            seq: seqRef.current
        });
        if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
        toastTimerRef.current = setTimeout(()=>setBlockedToast(null), notification_policy/* DEFAULT_TOAST_MS */.tx);
        // Pulse ConnectionCountIndicator only for the reason that actually has
        // connected-viewer state to point to (and its UNSPECIFIED fallback, which
        // renders the same copy).
        if (frame.blockedReason === events_pb/* ScrollBlockedReason */.ec.MULTIPLE_VIEWERS || frame.blockedReason === events_pb/* ScrollBlockedReason */.ec.SCROLL_BLOCKED_REASON_UNSPECIFIED) {
            if (pulseTimerRef.current) clearTimeout(pulseTimerRef.current);
            setConnectionPulse(true);
            pulseTimerRef.current = setTimeout(()=>setConnectionPulse(false), CONNECTION_PULSE_MS);
        }
    }, []);
    (0,react.useEffect)(()=>{
        return ()=>{
            if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
            if (pulseTimerRef.current) clearTimeout(pulseTimerRef.current);
        };
    }, []);
    return {
        blockedToast,
        connectionPulse,
        showBlockedOutcome,
        dismissBlockedToast
    };
}

;// ./src/lib/terminal/bufferFilter.ts
/**
 * Display-only filtering of terminal output. Works on lines read out of the xterm
 * buffer, never on the PTY byte stream, so filtering can't corrupt rendering.
 */ const MAX_FILTER_RESULTS = 500;
/** Lines of the active xterm buffer (scrollback + viewport) as plain text. */ function readBufferLines(terminal) {
    if (!terminal) return [];
    const buf = terminal.buffer.active;
    const lines = [];
    for(let i = 0; i < buf.length; i++){
        var _buf_getLine;
        var _buf_getLine_translateToString;
        lines.push((_buf_getLine_translateToString = (_buf_getLine = buf.getLine(i)) === null || _buf_getLine === void 0 ? void 0 : _buf_getLine.translateToString(true)) !== null && _buf_getLine_translateToString !== void 0 ? _buf_getLine_translateToString : "");
    }
    return lines;
}
function buildMatcher(query, param) {
    let { regex, caseSensitive } = param;
    if (regex) {
        try {
            const re = new RegExp(query, caseSensitive ? "" : "i");
            return (s)=>re.test(s);
        } catch (err) {
            return {
                error: err instanceof Error ? err.message : "Invalid pattern"
            };
        }
    }
    const needle = caseSensitive ? query : query.toLowerCase();
    return (s)=>(caseSensitive ? s : s.toLowerCase()).includes(needle);
}
/** Matching lines for `query`; an empty query matches nothing (the panel shows a hint instead). */ function filterLines(lines, query) {
    let options = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : {};
    if (query === "") return {
        matches: [],
        truncated: false
    };
    const matcher = buildMatcher(query, options);
    if ("error" in matcher) return {
        matches: [],
        truncated: false,
        error: matcher.error
    };
    const all = [];
    lines.forEach((text, i)=>{
        if (matcher(text)) all.push({
            line: i + 1,
            text
        });
    });
    const truncated = all.length > MAX_FILTER_RESULTS;
    return {
        matches: truncated ? all.slice(-MAX_FILTER_RESULTS) : all,
        truncated
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FTerminalOutputFilter.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Ty27jMAy85yt4KbA5qIjdTYuqH7OQLdomIouCpNSPov--kJwGgZMCLdqTaFLmcIaj-zBWs2938LYBcBwoElsJqgpsjhFfNgCRnYRdigw28RR6aruPeBZkNY4Sysf0qSk4oyYJjcExJdIpNHmsl-Y1m2NvU6VXo-hwabXf3aVUpepD6_lotYRX5f8I8a94xrn089M219lr9KLiGLmXULgRAhvSq8u4fdm8b-4XdkVmdzWXMtRaQRH7IKFGG9GndKvcGrqcM7ZTWpNtb1bPYGUGSxgSikySrBhIx-4k12ddpu1t1IZtFIFmXP_xUF3iPvyY5PQFOEjbY7-uFbk0dBRRBKdqlGB58MpdDPh38dgH-901218e6Iy8z8i98i3Z9RIWY1OIIsTJ5Llttj2_om8MD2KSoI6Rz6M0qiczSejZcub6jSU93l7SFyx3Q64ryZ1HsYgOMLDXovKoDhLyIZQxF5M8nW0qQufJHk5SXLh1X3f5_eMYRbaRXJ79p6KXiep_d1o0FlIEAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTerminalOutputFilter_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Ty27jMAy85yt4KbA5qIjdTYuqH7OQLdomIouCpNSPov_kJwGgZMCLdqTaFLmcIaj_zBWs2938LYBcBwoElsJqgpsjhFfNgCRnYRdigw28RR6aruPeBZkNY4Sysf0qSk4oyYJjcExJdIpNHmsl_Y1m2NvU6VXo_hwabXf3aVUpepD6_lotYRX5f8I8a94xrn089M219lr9KLiGLmXULgRAhvSq8u4fdm8b_4XdkVmdzWXMtRaQRH7IKFGG9GndKvcGrqcM7ZTWpNtb1bPYGUGSxgSikySrBhIx_4k12ddpu1t1IZtFIFmXP_xUF3iPvyY5PQFOEjbY7_uFbk0dBRRBKdqlGB58MpdDPh38dgH_901218e6Iy8z8i98i3Z9RIWY1OIIsTJ5Llttj2_om8MD2KSoI6Rz6M0qiczSejZcub6jSU93l7SFyx3Q64ryZ1HsYgOMLDXovKoDhLyIZQxF5M8nW0qQufJHk5SXLh1X3f5_eMYRbaRXJ79p6KXiep_d1o0FlIEAAA_22_7D = __webpack_require__(58471);
;// ./src/components/sessions/TerminalOutputFilter.css.ts


var header = 'sxbzrg1';
var input = 'sxbzrg2';
var lineNumber = 'sxbzrg7';
var TerminalOutputFilter_css_option = 'sxbzrg3';
var panel = 'sxbzrg0';
var results = 'sxbzrg5';
var row = 'sxbzrg6';
var TerminalOutputFilter_css_status = 'sxbzrg4';
;// ./src/components/sessions/TerminalOutputFilter.tsx
/* __next_internal_client_entry_do_not_use__ TerminalOutputFilter auto */ 
// +feature: terminal-output-filter



const REFRESH_MS = 1000;
/**
 * Opt-in, display-only filter over the terminal's buffer. It renders its own list
 * and never touches the PTY stream or xterm's buffer.
 */ function TerminalOutputFilter(param) {
    let { getLines, onClose } = param;
    const [query, setQuery] = (0,react.useState)("");
    const [regex, setRegex] = (0,react.useState)(false);
    const [caseSensitive, setCaseSensitive] = (0,react.useState)(false);
    const [result, setResult] = (0,react.useState)({
        matches: [],
        truncated: false
    });
    const inputRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        var _inputRef_current;
        (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.focus();
    }, []);
    (0,react.useEffect)(()=>{
        const run = ()=>setResult(filterLines(getLines(), query, {
                regex,
                caseSensitive
            }));
        run();
        const id = setInterval(run, REFRESH_MS);
        return ()=>clearInterval(id);
    }, [
        getLines,
        query,
        regex,
        caseSensitive
    ]);
    const hasQuery = query !== "";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: panel,
        role: "search",
        "aria-label": "Filter terminal output",
        "data-testid": "terminal-output-filter",
        onKeyDown: (e)=>{
            if (e.key === "Escape") onClose();
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        ref: inputRef,
                        className: input,
                        type: "text",
                        value: query,
                        onChange: (e)=>setQuery(e.target.value),
                        placeholder: "Filter lines…",
                        "aria-label": "Filter text",
                        "data-testid": "terminal-filter-input"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        className: TerminalOutputFilter_css_option,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "checkbox",
                                checked: regex,
                                onChange: (e)=>setRegex(e.target.checked),
                                "data-testid": "terminal-filter-regex"
                            }),
                            "Regex"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        className: TerminalOutputFilter_css_option,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "checkbox",
                                checked: caseSensitive,
                                onChange: (e)=>setCaseSensitive(e.target.checked),
                                "data-testid": "terminal-filter-case"
                            }),
                            "Case"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        onClick: onClose,
                        "aria-label": "Close filter",
                        "data-testid": "terminal-filter-close",
                        children: "✕"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: TerminalOutputFilter_css_status,
                role: "status",
                "data-testid": "terminal-filter-status",
                children: result.error ? "Invalid pattern: ".concat(result.error) : !hasQuery ? "Type to filter the terminal's scrollback. Display only — the terminal is unchanged." : result.truncated ? "Showing the last ".concat(MAX_FILTER_RESULTS, " matching lines") : "".concat(result.matches.length, " matching ").concat(result.matches.length === 1 ? "line" : "lines")
            }),
            result.matches.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                className: results,
                "data-testid": "terminal-filter-results",
                children: result.matches.map((m)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        className: row,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: lineNumber,
                                children: m.line
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                children: m.text
                            })
                        ]
                    }, m.line))
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FTerminalOutput.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-0aW4-cuvk9v8JVFWVGGkbA3DlSFanPeazUqqqOPGBmvAs2MWZuVf57ZWPABgPDbpJzUZVks9gfn7_7DX9-RfeYwRTl4FcfHvwQueC_HwBwPy6A57of5QMANIMh5vcAeL98AODbBwA23T13uSl3v3343EHrKbTqJc4gyWPK0gAwyiFHMzdCp3mFXTu6C7naarDWs7Y_7LClOsKXL0Q4zxJ4D0CcoJsAEf87EWYo5JiSAIQ0KVIids4In848kIeJ5xQTp1pzq1eVfI8wfD0xWpAoABfIZo7zq3dAD589vEhSEtKEss4eknsxJdyJYYqTewA-faEEhvTTAnz6gkgif_nHsSC8AF8okY9_pySnCczF7zktWIickEbIyZjYTimheQZD9IvG_MrOPEzwiTiYozQPQIgIR0wsvxQ5x_HdCSnhiPAASHzOEfErQlI2GYwiTE7CgnYbhlLgMZQOC2IneT1SFiHmHCnnNA2Al91AThMctYCVYIRu8jPD5FWKvOFnPYmfE8ykrUsaGyQbieSKI34OwD676UpXj4pcBiNc5IFwIXkMwSkszaV2QT8HmMSYYG4IvrTrfqkUSiqCTxjRawBc4IrTLXAN1t0IVs99Eq0CrBlyCExRAAglBhf7kfNuTx53m_eJz13u7QI8yKOli-T4gYQe97uNsja7Vxmigu33Pb9UrVy6KnVvXbcXXzo3LN7PbrV19CvAsPbKfNa6UfUbf2wwcJwugFKpKWQnTJwExdxm_aHdhXp8JZLQmttLr1fePyyL_fwZptGAwKxcuk3stMilYLl4JaO4CgIyUeDS7Bpigbv080V1rjxILhm8B2d6QWzEB446A1aK8dzECkOOL2gE7c546WQqTQVog1ps09TvRUNDChL5VQXjEo9Y4LQIzw6H7IT4AqzXWRlC9Fw8CvzmtNds9Wci_EOM4-UJ51RUXZlYFD91BMnbE2RtPK3QUdNfLg-ZSUNH-j0SNZkUqqgJ3c5l2aiqwF9wmlHGIeHjWjOAm1O-2okIGKX83xHkUErROUMSoeg_wHytXZMyejVx_w0cC84paaA12wTgesYcObJoE0e3bYOZtGGSYIKcQdU0L-djpcduivzSNrAV6muflPnviZjCDLzjFbHnTSmJVVYfyvjWZNgQeKnNRXUtnY6mLBnUQ5NC3t3hiAgZJ6IuPOMoUi0ErbIxQwkUmbAbekpkOYyRAxkqfWYBXBXXK1im54EGWC5X0I0UrqWa6tPhMadJweXpmOSoksXDwSQSovLd75REGhFW9sdORzhzF0D9Xa43JWNlUnTQRST3bgi7SRYUjr_GcdxSfmV6thK31qqozr2ttc3ZmnWqqHgjmJ9RVBLsbzYL0Pxwl6rY1SykzdhKQhQ5Yk6OEhTyLlP3Ab1wmimtqJQkfmWa4VbOM2Kq0F4P9M8ApitZJqVKAbUNeTVhEaOZE-OEC8kek4LN_JaBPvS2dN3qS9d7UzfrvmgR67FFyq8PtLDVdb2drge8HIiEAZm1X_Pc6f2KNgaprHXjGlWW5_2JrKMpqAz70Lj12zL0h-TnG-bjrTod71i3bIsUGsK1RaOeP6jSFkmbJ2PueAj5DmHYHl01ZWwM2ne6O6580x2r58odV8-7Yx9oOs0dt-XoZMgj91Mnn8pK_VY_INbWZdvf7hQqD0vhbdbOw-VemYgXCkUnYbTEhTvi6i2L4pFRoXcY7B5WipNeg9EwwXYBpQtGDURqoWntrPtbtKwD0n19qt83bbbS8HMWXtvtwHjANv3G5IwY5t1i1lbRbNTQrjtKsBQaADhXdHzF3LFulnqAyhVSSHBWJNLV5C66cUeqQJewtc0aGDS5-2bSVIqzNJBqq_6UUS6YpvfctMhQrXGI1HNNXvnBRP6aQI7-1SlCvOPYSd6U1spzn-qtPL8NZih856p0bs6ZXWnClo5IYyccqx9a-esxb-oJFXxtgvvnzNm4H-eWfqEODS3E93lrZeXZHOdwOBzGhsyH_r7L1g82KXuSOY81I170s0U7aZpkPfwnKMdb9WtnM6idkemNF-nDH50AKboMMqT7Y39830yd5256P4RYQrCdOwsbPfPTutOs_rnLVlGJ_m93v1FQ0JQQv9cK3alW6L7ZCusutB3SYMGpwdXpj2Ra9z-TaT2Rb84t5cT4hiK9zW_x679dOZ6737xTPavqY-ytqsNSTGYH93JdgLXfGmVaDOsp7bVguvM4E-Der8jHoCLlh4L6Xsxy3a3GlDtW3bv80w7cuOezxTuiyGNqFHm8N4po_Lz8vGjhbfpr55eBz61v9LULWN44YqlzwegqqvIgqLqoPGQ0SY6Q6eMRrzUeKZ-n4XM4g-HrWPcRzafjPRfpaFdj7WIt83BrC_xWop76hCxbu4nYQ8rIOOIesssWGVsKTctdF8v7Z5RkYnqDblzMgTpTyW12a31J-5yiCEOQhwwhAiCJwEyLm7vtPrvNJRrzWtvgvZRvGvhBgZs-YgEMFWB7ENZ8yXJuVdkgVivJ17ulCiRJcrJQgvV4IQCNxhSv1VgCACfNNbT8nqBmVyc56opCzeq2NfHWkcrEydS0exo6hSdFYeujuAn08seQvEjo-auDU3hCgRq-OicRLxDhM06B-vh4TEQsC2ESzuSdVUdOjOcLPb_JC6Zzk50fgV0X8-szukieASLPANE2UFVH9VZSg6PNaTY42cB7jEVn6Gs_Q5bLGw5DF8RypAHot3bs9q458kqLacqvS5ix27YT79vaijKda6a4_kkB49IJaYZknrojMO2WgLqePpiG1nu3nYZ6Ym-bn7jfVepDZ6okE20NQeYx54H3_wdbXvJJFjAAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTerminalOutput_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_0aW4_cuvk9v8JVFWVGGkbA3DlSFanPeazUqqqOPGBmvAs2MWZuVf57ZWPABgPDbpJzUZVks9gfn7_7DX9_RfeYwRTl4FcfHvwQueC_HwBwPy6A57of5QMANIMh5vcAeL98AODbBwA23T13uSl3v3343EHrKbTqJc4gyWPK0gAwyiFHMzdCp3mFXTu6C7naarDWs7Y_7LClOsKXL0Q4zxJ4D0CcoJsAEf87EWYo5JiSAIQ0KVIids4In848kIeJ5xQTp1pzq1eVfI8wfD0xWpAoABfIZo7zq3dAD589vEhSEtKEss4eknsxJdyJYYqTewA_faEEhvTTAnz6gkgif_nHsSC8AF8okY9_pySnCczF7zktWIickEbIyZjYTimheQZD9IvG_MrOPEzwiTiYozQPQIgIR0wsvxQ5x_HdCSnhiPAASHzOEfErQlI2GYwiTE7CgnYbhlLgMZQOC2IneT1SFiHmHCnnNA2Al91AThMctYCVYIRu8jPD5FWKvOFnPYmfE8ykrUsaGyQbieSKI34OwD676UpXj4pcBiNc5IFwIXkMwSkszaV2QT8HmMSYYG4IvrTrfqkUSiqCTxjRawBc4IrTLXAN1t0IVs99Eq0CrBlyCExRAAglBhf7kfNuTx53m_eJz13u7QI8yKOli_T4gYQe97uNsja7Vxmigu33Pb9UrVy6KnVvXbcXXzo3LN7PbrV19CvAsPbKfNa6UfUbf2wwcJwugFKpKWQnTJwExdxm_aHdhXp8JZLQmttLr1fePyyL_fwZptGAwKxcuk3stMilYLl4JaO4CgIyUeDS7Bpigbv080V1rjxILhm8B2d6QWzEB446A1aK8dzECkOOL2gE7c546WQqTQVog1ps09TvRUNDChL5VQXjEo9Y4LQIzw6H7IT4AqzXWRlC9Fw8CvzmtNds9Wci_EOM4_UJ51RUXZlYFD91BMnbE2RtPK3QUdNfLg_ZSUNH_j0SNZkUqqgJ3c5l2aiqwF9wmlHGIeHjWjOAm1O_2okIGKX83xHkUErROUMSoeg_wHytXZMyejVx_w0cC84paaA12wTgesYcObJoE0e3bYOZtGGSYIKcQdU0L_djpcduivzSNrAV6muflPnviZjCDLzjFbHnTSmJVVYfyvjWZNgQeKnNRXUtnY6mLBnUQ5NC3t3hiAgZJ6IuPOMoUi0ErbIxQwkUmbAbekpkOYyRAxkqfWYBXBXXK1im54EGWC5X0I0UrqWa6tPhMadJweXpmOSoksXDwSQSovLd75REGhFW9sdORzhzF0D9Xa43JWNlUnTQRST3bgi7SRYUjr_GcdxSfmV6thK31qqozr2ttc3ZmnWqqHgjmJ9RVBLsbzYL0Pxwl6rY1SykzdhKQhQ5Yk6OEhTyLlP3Ab1wmimtqJQkfmWa4VbOM2Kq0F4P9M8ApitZJqVKAbUNeTVhEaOZE_OEC8kek4LN_JaBPvS2dN3qS9d7UzfrvmgR67FFyq8PtLDVdb2drge8HIiEAZm1X_Pc6f2KNgaprHXjGlWW5_2JrKMpqAz70Lj12zL0h_TnG_bjrTod71i3bIsUGsK1RaOeP6jSFkmbJ2PueAj5DmHYHl01ZWwM2ne6O6580x2r58odV8_7Yx9oOs0dt_XoZMgj91Mnn8pK_VY_INbWZdvf7hQqD0vhbdbOw_VemYgXCkUnYbTEhTvi6i2L4pFRoXcY7B5WipNeg9EwwXYBpQtGDURqoWntrPtbtKwD0n19qt83bbbS8HMWXtvtwHjANv3G5IwY5t1i1lbRbNTQrjtKsBQaADhXdHzF3LFulnqAyhVSSHBWJNLV5C66cUeqQJewtc0aGDS5_2bSVIqzNJBqq_6UUS6YpvfctMhQrXGI1HNNXvnBRP6aQI7_1SlCvOPYSd6U1spzn_qtPL8NZih856p0bs6ZXWnClo5IYyccqx9a_esxb_oJFXxtgvvnzNm4H_eWfqEODS3E93lrZeXZHOdwOBzGhsyH_r7L1g82KXuSOY81I170s0U7aZpkPfwnKMdb9WtnM6idkemNF_nDH50AKboMMqT7Y39830yd5256P4RYQrCdOwsbPfPTutOs_rnLVlGJ_m93v1FQ0JQQv9cK3alW6L7ZCusutB3SYMGpwdXpj2Ra9z_TaT2Rb84t5cT4hiK9zW_x679dOZ6737xTPavqY_ytqsNSTGYH93JdgLXfGmVaDOsp7bVguvM4E_Der8jHoCLlh4L6Xsxy3a3GlDtW3bv80w7cuOezxTuiyGNqFHm8N4po_Lz8vGjhbfpr55eBz61v9LULWN44YqlzwegqqvIgqLqoPGQ0SY6Q6eMRrzUeKZ_n4XM4g_HrWPcRzafjPRfpaFdj7WIt83BrC_xWop76hCxbu4nYQ8rIOOIesssWGVsKTctdF8v7Z5RkYnqDblzMgTpTyW12a31J_5yiCEOQhwwhAiCJwEyLm7vtPrvNJRrzWtvgvZRvGvhBgZs_YgEMFWB7ENZ8yXJuVdkgVivJ17ulCiRJcrJQgvV4IQCNxhSv1VgCACfNNbT8nqBmVyc56opCzeq2NfHWkcrEydS0exo6hSdFYeujuAn08seQvEjo_auDU3hCgRq_OicRLxDhM06B_vh4TEQsC2ESzuSdVUdOjOcLPb_JC6Zzk50fgV0X8_szukieASLPANE2UFVH9VZSg6PNaTY42cB7jEVn6Gs_Q5bLGw5DF8RypAHot3bs9q458kqLacqvS5ix27YT79vaijKda6a4_kkB49IJaYZknrojMO2WgLqePpiG1nu3nYZ6Ym_bn7jfVepDZ6okE20NQeYx54H3_wdbXvJJFjAAAA_22_7D = __webpack_require__(45238);
;// ./src/components/sessions/TerminalOutput.css.ts


var actions = '_2a92cec';
var TerminalOutput_css_blockedToast = '_2a92ce1h';
var blockedToastButton = '_2a92ce1i';
var connected = '_2a92ce6';
var container = '_2a92ce2';
var debugActive = '_2a92cee';
var desktopOnlyUploads = '_2a92ceg';
var devGroup = '_2a92cem';
var devGroupPanel = '_2a92cel';
var devOnly = '_2a92cek';
var disconnected = '_2a92ce7';
var dropZoneLabel = '_2a92cex';
var dropZoneOverlay = '_2a92cew';
var error = '_2a92ceu';
var errorText = '_2a92ceb';
var externalLabel = '_2a92cea';
var hardFailedBanner = '_2a92ce1d';
var loadingOverlay = '_2a92cey';
var loadingSpinner = '_2a92cez';
var loadingText = '_2a92ce10';
var mobileKey = '_2a92ce1a';
var mobileKeyActive = '_2a92ces';
var mobileKeyCtrlC = '_2a92ce1b';
var mobileKeyRow = '_2a92ce19';
var mobileKeyboard = '_2a92ce18';
var mobileKeyboardToggle = '_2a92cer';
var mobileMoreActive = '_2a92cep';
var mobileMoreButton = '_2a92ceo';
var mobileOnlyUpload = '_2a92ceh';
var mobileOverflowRow = '_2a92ceq';
var mouseModeActive = '_2a92cet';
var noMoreHistoryLine = '_2a92ce1j';
var reconnectingBanner = '_2a92ce1c';
var resizingOverlay = '_2a92ce15';
var resizingSpinner = '_2a92ce17';
var scrollLoadingPill = '_2a92ce1e';
var scrollLoadingPillCancel = '_2a92ce1f';
var scrollSourceIndicator = '_2a92ce1g';
var secondaryGroup = '_2a92cen';
var stabilizing = '_2a92ce8';
var TerminalOutput_css_status = '_2a92ce4';
var statusIndicator = '_2a92ce5';
var statusText = '_2a92ce9';
var terminal = '_2a92cev';
var toolbar = '_2a92ce3';
var toolbarActions = '_2a92cej';
var toolbarButton = '_2a92ced';
var toolbarButtonLabel = '_2a92cef';
var toolbarToggle = '_2a92cei';
var unavailableIcon = '_2a92ce12';
var unavailableOverlay = '_2a92ce11';
var unavailableSubtext = '_2a92ce14';
var unavailableText = '_2a92ce13';
;// ./src/components/sessions/ScrollLoadingPill.tsx
/* __next_internal_client_entry_do_not_use__ ScrollLoadingPill auto */ 
// +feature: scroll-forward-loading-pill

/**
 * Loading affordance for a scroll-up-triggered scrollback fetch (Story 1.4.5,
 * design/ux.md Surface 1) — shared by both the pre-existing tmux-native
 * paged-scrollback path and Story 1.4.0's app-forwarded (alt-screen) path.
 * Reuses `reconnectingBanner`'s exact visual shape. TerminalOutput.tsx owns
 * the 150ms show-delay and 8s stalled timer; this component only renders
 * the state it's given.
 */ function ScrollLoadingPill(param) {
    let { visible, stalled, program, onCancel } = param;
    if (!visible) return null;
    const label = stalled ? program ? "Still trying to load ".concat(program, "'s history…") : "Still trying to load more…" : program ? "Loading ".concat(program, "'s history…") : "Loading more…";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: scrollLoadingPill,
        role: "status",
        "aria-live": "polite",
        "data-testid": "scroll-loading-pill",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                children: label
            }),
            stalled && // A transient loading-state escape hatch, not a user-initiated
            // feature action — mirrors the untracked reconnect/dismiss controls
            // elsewhere in this file.
            // analytics-exempt
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: scrollLoadingPillCancel,
                onClick: onCancel,
                "data-testid": "scroll-loading-pill-cancel",
                children: "Cancel"
            })
        ]
    });
}

;// ./src/components/sessions/ScrollSourceIndicator.tsx
/* __next_internal_client_entry_do_not_use__ ScrollSourceIndicator auto */ 
// +feature: scroll-source-indicator

/**
 * Persistent banner shown while the viewport is displaying an app-forwarded
 * page of a program's own history instead of the live tail (Story 1.4.2,
 * design/ux.md Surface 2). Unlike ScrollLoadingPill this does not auto-dismiss
 * on a timer — TerminalOutput.tsx unmounts it once a non-app-scrollback frame
 * arrives (return to live) or the session disconnects.
 */ function ScrollSourceIndicator(param) {
    let { program, visible } = param;
    if (!visible) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: scrollSourceIndicator,
        role: "status",
        "aria-live": "polite",
        "data-testid": "scroll-source-indicator",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                "aria-hidden": "true",
                children: "\uD83D\uDCDC"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                children: [
                    "Viewing ",
                    program,
                    "'s own history"
                ]
            })
        ]
    });
}

;// ./src/lib/terminal/viewportSettle.ts
/**
 * ViewportSettled signal (Story 2.1.4): fires once both visualViewport height
 * and offsetTop are unchanged for `stableFrames` consecutive frames, or after
 * `maxWaitMs` from the arming event, whichever comes first. Armed by vv
 * `resize` and `scroll`. Pure: scheduler and window are injected; imports
 * nothing from lib/hooks.
 */ const DEFAULT_STABLE_FRAMES = 3;
const DEFAULT_MAX_WAIT_MS = 600;
function createRafScheduler() {
    return {
        request: (cb)=>requestAnimationFrame(()=>cb()),
        cancel: (h)=>cancelAnimationFrame(h),
        now: ()=>performance.now()
    };
}
function windowFallbackSource(win) {
    return {
        get height () {
            return win.innerHeight;
        },
        offsetTop: 0,
        addEventListener: (type, listener)=>{
            if (type === "resize") win.addEventListener("resize", listener);
        },
        removeEventListener: (type, listener)=>{
            if (type === "resize") win.removeEventListener("resize", listener);
        }
    };
}
function resolveFallbackWindow(win) {
    if (win) return win;
    if (true) return window;
    throw new Error("createViewportSettle: no visualViewport and no window available; pass options.win");
}
/** Returns a dispose function that unsubscribes and cancels any pending frame. */ function createViewportSettle(vv, scheduler, options) {
    const source = vv !== null && vv !== void 0 ? vv : windowFallbackSource(resolveFallbackWindow(options.win));
    const { stableFrames, maxWaitMs, onSettled } = options;
    let frameHandle = null;
    let armedAt = 0;
    let lastHeight = NaN;
    let lastOffsetTop = NaN;
    let stableCount = 0;
    let disposed = false;
    const settle = (height, offsetTop)=>{
        frameHandle = null;
        onSettled({
            height,
            offsetTop
        });
    };
    const onFrame = ()=>{
        frameHandle = null;
        if (disposed) return;
        const { height, offsetTop } = source;
        if (height === lastHeight && offsetTop === lastOffsetTop) {
            stableCount++;
        } else {
            lastHeight = height;
            lastOffsetTop = offsetTop;
            stableCount = 1;
        }
        if (stableCount >= stableFrames || scheduler.now() - armedAt >= maxWaitMs) {
            settle(height, offsetTop);
            return;
        }
        frameHandle = scheduler.request(onFrame);
    };
    const onEvent = ()=>{
        if (disposed || frameHandle !== null) return;
        armedAt = scheduler.now();
        lastHeight = NaN;
        lastOffsetTop = NaN;
        stableCount = 0;
        frameHandle = scheduler.request(onFrame);
    };
    source.addEventListener("resize", onEvent);
    source.addEventListener("scroll", onEvent);
    return ()=>{
        disposed = true;
        source.removeEventListener("resize", onEvent);
        source.removeEventListener("scroll", onEvent);
        if (frameHandle !== null) {
            scheduler.cancel(frameHandle);
            frameHandle = null;
        }
    };
}

// EXTERNAL MODULE: ./src/lib/terminal/scrollRouting.ts
var scrollRouting = __webpack_require__(40230);
;// ./src/lib/terminal/scrollPosition.ts
/**
 * Away-from-live detection and the TUI `netPagesUp` estimate (design/ux.md S7).
 * Pure: no DOM, no React. The estimate counts page keys sent up minus down
 * (drag and toolbar); the app owns its real position, so the estimate is only
 * trusted while nothing has happened that could have moved it.
 */ /** Overlays (chip, jump button) are hidden when fewer terminal rows are visible. */ const MIN_ROWS_FOR_OVERLAYS = 5;
/** Beyond this the estimate is too uncertain to send keys from. */ const NET_PAGES_UP_LIMIT = 5;
/** Local buffer only: true when the viewport sits above the live bottom. */ function isAwayFromLive(param) {
    let { viewportY, baseY } = param;
    return viewportY < baseY;
}
const LIVE = {
    pages: 0,
    valid: true
};
function createNetPagesUpTracker() {
    let limit = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : NET_PAGES_UP_LIMIT;
    let state = LIVE;
    const listeners = new Set();
    const set = (next)=>{
        if (next.pages === state.pages && next.valid === state.valid && next.reason === state.reason) return;
        state = next;
        listeners.forEach((l)=>l());
    };
    const invalidate = (reason)=>set({
            pages: 0,
            valid: false,
            reason
        });
    return {
        getState: ()=>state,
        pageUp () {
            if (!state.valid && state.reason === "overflow") return;
            const pages = (state.valid ? state.pages : 0) + 1;
            if (pages > limit) invalidate("overflow");
            else set({
                pages,
                valid: true
            });
        },
        pageDown () {
            if (!state.valid) return;
            set({
                pages: Math.max(0, state.pages - 1),
                valid: true
            });
        },
        invalidate,
        markLive: ()=>set(LIVE),
        subscribe (listener) {
            listeners.add(listener);
            return ()=>{
                listeners.delete(listener);
            };
        }
    };
}

;// ./src/lib/terminal/scrollOverride.ts
/**
 * Persisted scroll settings (design/ux.md S6): the drag-routing override and the
 * Gesture scrolling switch. Per device, not per session. Storage can throw
 * (private mode, quota), so every access falls back to defaults and the
 * in-memory choice keeps applying for the rest of the session.
 */ 
const SCROLL_OVERRIDE_KEY = "terminal-scroll-override";
const GESTURE_SCROLL_KEY = "terminal-gesture-scroll";
const DEFAULT_SCROLL_OVERRIDE = "auto";
const DEFAULT_GESTURE_SCROLL = true;
function defaultStorage() {
    return typeof localStorage === "undefined" ? null : localStorage;
}
function parseOverride(raw) {
    return raw === "local" || raw === "tui" ? raw : DEFAULT_SCROLL_OVERRIDE;
}
function createScrollSettings() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    var _options_getStorage;
    const getStorage = (_options_getStorage = options.getStorage) !== null && _options_getStorage !== void 0 ? _options_getStorage : defaultStorage;
    var _options_onOverrideChange;
    const onOverrideChange = (_options_onOverrideChange = options.onOverrideChange) !== null && _options_onOverrideChange !== void 0 ? _options_onOverrideChange : (from, to)=>mobileDebug/* mobileDebug */.Uo.overrideChange(from, to);
    const listeners = new Set();
    // Set once the user changes a value, so it wins over a storage that rejected the write.
    let memoryOverride = null;
    let memoryGesture = null;
    const readRaw = (key)=>{
        try {
            var _getStorage;
            var _getStorage_getItem;
            return (_getStorage_getItem = (_getStorage = getStorage()) === null || _getStorage === void 0 ? void 0 : _getStorage.getItem(key)) !== null && _getStorage_getItem !== void 0 ? _getStorage_getItem : null;
        } catch (e) {
            return null;
        }
    };
    const write = (apply)=>{
        try {
            const storage = getStorage();
            if (storage) apply(storage);
        } catch (e) {
        // keep the in-memory choice
        }
    };
    const notify = ()=>listeners.forEach((l)=>l());
    const getOverride = ()=>memoryOverride !== null && memoryOverride !== void 0 ? memoryOverride : parseOverride(readRaw(SCROLL_OVERRIDE_KEY));
    const getGestureScroll = ()=>memoryGesture !== null && memoryGesture !== void 0 ? memoryGesture : readRaw(GESTURE_SCROLL_KEY) === "off" ? false : DEFAULT_GESTURE_SCROLL;
    return {
        getOverride,
        getGestureScroll,
        setOverride (value) {
            const previous = getOverride();
            memoryOverride = value;
            write((s)=>value === "auto" ? s.removeItem(SCROLL_OVERRIDE_KEY) : s.setItem(SCROLL_OVERRIDE_KEY, value));
            if (previous === value) return;
            onOverrideChange(previous, value);
            notify();
        },
        setGestureScroll (on) {
            const previous = getGestureScroll();
            memoryGesture = on;
            write((s)=>on ? s.removeItem(GESTURE_SCROLL_KEY) : s.setItem(GESTURE_SCROLL_KEY, "off"));
            if (previous !== on) notify();
        },
        subscribe (listener) {
            listeners.add(listener);
            return ()=>{
                listeners.delete(listener);
            };
        }
    };
}
/** Shared store for app code. */ const scrollSettings = createScrollSettings();

// EXTERNAL MODULE: ./src/components/ui/LiveRegion.css.ts
var LiveRegion_css = __webpack_require__(76863);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FJumpToLatestButton.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA42Tb4ucMBDG3_sphoPCHjSLurf0zlK471HKEc2o0zV_msSuWvzuxbiyt7oHhy9knmcyM2R-eT1hX1ou0cFbcjzlvfkWw78IIP4SfgDa8IJ8n0H8PQIYI4Ak3prJbI7RfimThBSjHXnSKgOeO920HqdES1XtM_jL7Y6xt-QFh9QO6fA4eQMjJbDLID1OoSBnGt5nQKohhaxssJt03lClGHmULoMClUc7yb9b56nsWaGVR-XfWxU3m5Z9aClJsRrnmZ6eTLdoZxK-vkqXMEn2R4tyUgwXglR1p-xKOcShUc6LU2V1q8T6iAx-oRtt19af-ai2Am0GSiu8xsxyQa1bHzm8hDOlVp45GnDj51f_jPeWcaguXTvmai70OYMYUtPBs-nAVjnfxV_Dtz_Mk7fWTaMbTct9c0WSz7u_opWksXSA3CHTrZ_SvOVqYeSC0ybrPVY_BfecFdoqtD8ecu29liwA9fArIDdLd-n6uI7X5qaI11tYVhXSkHiB4nlGZKHoEn5mSR8zMS9-Ap652pI6hTc4Rq8SBXHYGYslWscsirZAwaSeb3GOH8N4q-d4e90LSze7WsQxGv8D5NvxhR0EAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FJumpToLatestButton_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA42Tb4ucMBDG3_sphoPCHjSLurf0zlK471HKEc2o0zV_msSuWvzuxbiyt7oHhy9knmcyM2R_eT1hX1ou0cFbcjzlvfkWw78IIP4SfgDa8IJ8n0H8PQIYI4Ak3prJbI7RfimThBSjHXnSKgOeO920HqdES1XtM_jL7Y6xt_QFh9QO6fA4eQMjJbDLID1OoSBnGt5nQKohhaxssJt03lClGHmULoMClUc7yb9b56nsWaGVR_XfWxU3m5Z9aClJsRrnmZ6eTLdoZxK_vkqXMEn2R4tyUgwXglR1p_xKOcShUc6LU2V1q8T6iAx_oRtt19af_ai2Am0GSiu8xsxyQa1bHzm8hDOlVp45GnDj51f_jPeWcaguXTvmai70OYMYUtPBs_nAVjnfxV_Dtz_Mk7fWTaMbTct9c0WSz7u_opWksXSA3CHTrZ_SvOVqYeSC0ybrPVY_BfecFdoqtD8ecu29liwA9fArIDdLd_n6uI7X5qaI11tYVhXSkHiB4nlGZKHoEn5mSR8zMS9_Ap652pI6hTc4Rq8SBXHYGYslWscsirZAwaSeb3GOH8N4q_d4e90LSze7WsQxGv8D5NvxhR0EAAA_22_7D = __webpack_require__(14376);
;// ./src/components/sessions/JumpToLatestButton.css.ts


var JumpToLatestButton_css_button = '_15kbyp71';
var newOutputDot = '_15kbyp72';
;// ./src/components/sessions/JumpToLatestButton.tsx
/* __next_internal_client_entry_do_not_use__ JumpToLatestButton auto */ 
// +feature: terminal-jump-to-latest





const NEW_OUTPUT_ANNOUNCE_DELAY_MS = 2000;
const LOCAL_LABEL = "Jump to latest";
const TUI_LABEL = "Page down to latest";
function readSnapshot(terminal) {
    if (!terminal) return {
        rows: 0,
        away: false
    };
    const { viewportY, baseY, length } = terminal.buffer.active;
    return {
        rows: terminal.rows,
        away: length > 0 && isAwayFromLive({
            viewportY,
            baseY
        })
    };
}
function overlaps(a, b) {
    return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
}
/**
 * Affordance back to live output (design/ux.md S7). Local buffer: shown while the
 * viewport is above the bottom. TUI: shown only while the page-key estimate is valid
 * and positive. Never focuses the terminal, so the keyboard stays as it was.
 */ function JumpToLatestButton(param) {
    let { terminal, route, netPagesUp, connectionEpoch, sendData, isInputBusy, gestureActive = false, outputTick = 0, getCursorRowRect } = param;
    const [snapshot, setSnapshot] = (0,react.useState)(()=>readSnapshot(terminal));
    const estimate = (0,react.useSyncExternalStore)(netPagesUp.subscribe, netPagesUp.getState, netPagesUp.getState);
    const isLocal = route === "xterm-local";
    const refresh = (0,react.useCallback)(()=>{
        setSnapshot((prev)=>{
            const next = readSnapshot(terminal);
            return next.rows === prev.rows && next.away === prev.away ? prev : next;
        });
    }, [
        terminal
    ]);
    (0,react.useEffect)(()=>{
        var _terminal_onScroll, _terminal_onWriteParsed;
        refresh();
        const subscriptions = [
            terminal === null || terminal === void 0 ? void 0 : (_terminal_onScroll = terminal.onScroll) === null || _terminal_onScroll === void 0 ? void 0 : _terminal_onScroll.call(terminal, refresh),
            terminal === null || terminal === void 0 ? void 0 : (_terminal_onWriteParsed = terminal.onWriteParsed) === null || _terminal_onWriteParsed === void 0 ? void 0 : _terminal_onWriteParsed.call(terminal, refresh)
        ];
        return ()=>subscriptions.forEach((s)=>s === null || s === void 0 ? void 0 : s.dispose());
    }, [
        terminal,
        refresh
    ]);
    const visible = snapshot.rows >= MIN_ROWS_FOR_OVERLAYS && (isLocal ? snapshot.away : estimate.valid && estimate.pages > 0);
    // ---- Reconnect: the estimate no longer describes the app's position ----
    const lastEpochRef = (0,react.useRef)(connectionEpoch);
    (0,react.useEffect)(()=>{
        if (lastEpochRef.current === connectionEpoch) return;
        lastEpochRef.current = connectionEpoch;
        netPagesUp.invalidate("reconnect");
    }, [
        connectionEpoch,
        netPagesUp
    ]);
    // ---- Placement: evaluated when shown and when a gesture ends, never mid-gesture ----
    const buttonRef = (0,react.useRef)(null);
    const [corner, setCorner] = (0,react.useState)("bottom-right");
    const evaluatePlacement = (0,react.useCallback)(()=>{
        var _buttonRef_current;
        const cursor = getCursorRowRect === null || getCursorRowRect === void 0 ? void 0 : getCursorRowRect();
        const own = (_buttonRef_current = buttonRef.current) === null || _buttonRef_current === void 0 ? void 0 : _buttonRef_current.getBoundingClientRect();
        if (!cursor || !own) return;
        const next = overlaps(own, cursor) ? "top-right" : "bottom-right";
        setCorner((c)=>c === next ? c : next);
    }, [
        getCursorRowRect
    ]);
    (0,react.useLayoutEffect)(()=>{
        if (visible) evaluatePlacement();
        else setCorner("bottom-right");
    // Deliberately keyed on `visible` only: the corner is kept until the button hides.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        visible
    ]);
    const wasGestureRef = (0,react.useRef)(gestureActive);
    (0,react.useEffect)(()=>{
        if (wasGestureRef.current && !gestureActive && visible) evaluatePlacement();
        wasGestureRef.current = gestureActive;
    }, [
        gestureActive,
        visible,
        evaluatePlacement
    ]);
    // ---- Output while scrolled: dot plus one debounced announcement per episode ----
    const [hasNewOutput, setHasNewOutput] = (0,react.useState)(false);
    const [announcement, setAnnouncement] = (0,react.useState)("");
    const announcedRef = (0,react.useRef)(false);
    const announceTimerRef = (0,react.useRef)(null);
    const lastTickRef = (0,react.useRef)(outputTick);
    const clearAnnounceTimer = (0,react.useCallback)(()=>{
        if (announceTimerRef.current) clearTimeout(announceTimerRef.current);
        announceTimerRef.current = null;
    }, []);
    (0,react.useEffect)(()=>{
        if (lastTickRef.current === outputTick) return;
        lastTickRef.current = outputTick;
        if (!visible) return;
        setHasNewOutput(true);
        if (announcedRef.current || announceTimerRef.current) return;
        announceTimerRef.current = setTimeout(()=>{
            announceTimerRef.current = null;
            announcedRef.current = true;
            setAnnouncement("New output below");
        }, NEW_OUTPUT_ANNOUNCE_DELAY_MS);
    }, [
        outputTick,
        visible
    ]);
    (0,react.useEffect)(()=>{
        if (visible) return;
        clearAnnounceTimer();
        announcedRef.current = false;
        setHasNewOutput(false);
        setAnnouncement("");
    }, [
        visible,
        clearAnnounceTimer
    ]);
    // ---- Tap ----
    const pageKeyTimersRef = (0,react.useRef)(new Set());
    const cancelPageKeyTimers = (0,react.useCallback)(()=>{
        pageKeyTimersRef.current.forEach(clearTimeout);
        pageKeyTimersRef.current.clear();
    }, []);
    (0,react.useEffect)(()=>()=>{
            cancelPageKeyTimers();
            clearAnnounceTimer();
        }, [
        cancelPageKeyTimers,
        clearAnnounceTimer
    ]);
    // Queued extra PgDn keys target a position that no longer holds once the estimate is invalidated
    // (keystroke, resize, mode change, reconnect), the epoch bumps, or the route flips.
    (0,react.useEffect)(()=>{
        if (!estimate.valid) cancelPageKeyTimers();
    }, [
        estimate.valid,
        cancelPageKeyTimers
    ]);
    (0,react.useEffect)(()=>cancelPageKeyTimers, [
        route,
        connectionEpoch,
        cancelPageKeyTimers
    ]);
    const handleClick = ()=>{
        if (isLocal) {
            terminal === null || terminal === void 0 ? void 0 : terminal.scrollToBottom();
            return;
        }
        if (isInputBusy === null || isInputBusy === void 0 ? void 0 : isInputBusy()) return;
        const count = Math.min(estimate.pages, NET_PAGES_UP_LIMIT);
        netPagesUp.markLive();
        for(let i = 0; i < count; i++){
            const send = ()=>{
                if (!(isInputBusy === null || isInputBusy === void 0 ? void 0 : isInputBusy())) sendData((0,scrollRouting/* encodePageKeys */.qC)("newer"));
            };
            if (i === 0) {
                send();
                continue;
            }
            const timers = pageKeyTimersRef.current;
            const id = setTimeout(()=>{
                timers.delete(id);
                send();
            }, scrollRouting/* PAGE_RATE_LIMIT_MS */.Mu * i);
            timers.add(id);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                className: LiveRegion_css/* srOnly */.s,
                children: announcement
            }),
            visible && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                ref: buttonRef,
                type: "button",
                className: JumpToLatestButton_css_button,
                "data-corner": corner,
                onMouseDown: (e)=>e.preventDefault(),
                onClick: handleClick,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "aria-hidden": "true",
                        children: "↓"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: isLocal ? LOCAL_LABEL : TUI_LABEL
                    }),
                    hasNewOutput && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: newOutputDot,
                                "data-testid": "jump-new-output-dot",
                                "aria-hidden": "true"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: LiveRegion_css/* srOnly */.s,
                                children: "new output"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

;// ./src/components/sessions/JumpToLatestMount.tsx
/* __next_internal_client_entry_do_not_use__ JumpToLatestMount auto */ 
// +feature: terminal-jump-to-latest


/**
 * Owns the per-frame output tick so streaming output re-renders only this subtree,
 * not the whole TerminalOutput.
 */ function JumpToLatestMount(param) {
    let { terminal, getContainer, ...rest } = param;
    const [outputTick, setOutputTick] = (0,react.useState)(0);
    (0,react.useEffect)(()=>{
        if (!(terminal === null || terminal === void 0 ? void 0 : terminal.onWriteParsed)) return;
        let frame = 0;
        const subscription = terminal.onWriteParsed(()=>{
            if (frame) return;
            frame = requestAnimationFrame(()=>{
                frame = 0;
                setOutputTick((t)=>t + 1);
            });
        });
        return ()=>{
            subscription.dispose();
            if (frame) cancelAnimationFrame(frame);
        };
    }, [
        terminal
    ]);
    const getCursorRowRect = (0,react.useCallback)(()=>{
        var _terminal_buffer_active;
        const container = getContainer();
        const cursorY = terminal === null || terminal === void 0 ? void 0 : (_terminal_buffer_active = terminal.buffer.active) === null || _terminal_buffer_active === void 0 ? void 0 : _terminal_buffer_active.cursorY;
        if (!container || !terminal || cursorY === undefined || terminal.rows <= 0) return null;
        const rect = container.getBoundingClientRect();
        const cellHeight = rect.height / terminal.rows;
        const top = rect.top + cursorY * cellHeight;
        return {
            left: rect.left,
            right: rect.right,
            top,
            bottom: top + cellHeight
        };
    }, [
        terminal,
        getContainer
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(JumpToLatestButton, {
        terminal: terminal,
        outputTick: outputTick,
        getCursorRowRect: getCursorRowRect,
        ...rest
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FScrollingPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VW246bMBB9z1dY-9Q8eBVIttsQ9UuqamXwANM1NrKHJKTqv1cYaEIuu3STtMpDsGeYOTPneMzjS1gLMsGM_ZwwJtGVStQRSxVsVxPm_7lECwmh0RFLjKoK3VgyUUZsLewnzl-CJexCu5vPpo2lFFKizi5YY5G8ZtZUWh47PHt7YpSxx6buVWMl2IgF5ZY5o1AeecGBF7dCYuVOQLRJUqOJO9zBiT2eria_Jo99V75JQYKbNVgl6q8PZCt4-O5bVRqHbU9E7IyqCJrAClKK2Kx5tJjl_fOOo5awjVj41CybeKkyG15HTFRkDlMGPnpfqjbaxy2EzVB30f402K-u4yzcDQoOfXbfng20BRw1KJueQmjR8dgQmeKdDPPzQhMKM82RoHDtJnckLL2ttAI1zzuYi0W57fc2KCnfb10QpAfGWFJZ1yiuNKgJ7CHWRdsNjya3qF-7erv4YRe_h9Cve7KCWblls-Y3CBqlJqkcX6PDWIFPYSpSqCFi4QVpFx5q58ZNmjog730Y-ekGZzisB2x9HqWHvf-z9z97hoOB45c7y-DDnC89sMF5G1GOaN-6To_19P2RSVZoVwoLmq6clvmNpuWb3RR3E3s8FBBq73xWRwm0sBj7UTnCtOaJ0QSaDk3_gLsbX3djCFx-jEDGNjkScFeKBJpbaGNFuTozCw_4uB_T7SWcY5arhh-Qg4u4a0DH07ylafzESkYMor1K_vqrpxs4_1kGo756fgPI4WKeCwoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FScrollingPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VW246bMBB9z1dY_9Q8eBVIttsQ9UuqamXwANM1NrKHJKTqv1cYaEIuu3STtMpDsGeYOTPneMzjS1gLMsGM_ZwwJtGVStQRSxVsVxPm_7lECwmh0RFLjKoK3VgyUUZsLewnzl_CJexCu5vPpo2lFFKizi5YY5G8ZtZUWh47PHt7YpSxx6buVWMl2IgF5ZY5o1AeecGBF7dCYuVOQLRJUqOJO9zBiT2eria_Jo99V75JQYKbNVgl6q8PZCt4_O5bVRqHbU9E7IyqCJrAClKK2Kx5tJjl_fOOo5awjVj41CybeKkyG15HTFRkDlMGPnpfqjbaxy2EzVB30f402K_u4yzcDQoOfXbfng20BRw1KJueQmjR8dgQmeKdDPPzQhMKM82RoHDtJnckLL2ttAI1zzuYi0W57fc2KCnfb10QpAfGWFJZ1yiuNKgJ7CHWRdsNjya3qF_7erv4YRe_h9Cve7KCWblls_Y3CBqlJqkcX6PDWIFPYSpSqCFi4QVpFx5q58ZNmjog730Y_ekGZzisB2x9HqWHvf_z9z97hoOB45c7y_DDnC89sMF5G1GOaN_6To_19P2RSVZoVwoLmq6clvmNpuWb3RR3E3s8FBBq73xWRwm0sBj7UTnCtOaJ0QSaDk3_gLsbX3djCFx_jEDGNjkScFeKBJpbaGNFuTozCw_4uB_T7SWcY5arhh_Qg4u4a0DH07ylafzESkYMor1K_vqrpxs4_1kGo756fgPI4WKeCwoAAA_22_7D = __webpack_require__(80370);
;// ./src/components/sessions/ScrollingPanel.css.ts


var ScrollingPanel_css_button = '_2yato1a';
var chip = '_2yato1b';
var description = '_2yato17';
var fieldset = '_2yato11';
var hint = '_2yato1c';
var legend = '_2yato12';
var note = '_2yato19';
var ScrollingPanel_css_option = '_2yato13';
var optionInput = '_2yato14';
var optionLabel = '_2yato16';
var optionText = '_2yato15';
var ScrollingPanel_css_panel = '_2yato10';
var switchRow = '_2yato18';
;// ./src/components/sessions/ScrollHint.tsx
/* __next_internal_client_entry_do_not_use__ SCROLL_HINT_SEEN_KEY,HINT_TEXT_LOCAL,HINT_TEXT_TUI,useScrollHint,ScrollHint auto */ 
// +feature: terminal-scroll-settings


const SCROLL_HINT_SEEN_KEY = "terminal-scroll-hint-seen";
const HINT_TEXT_LOCAL = "Dragging scrolls the terminal's history. Tap the chip to change.";
const HINT_TEXT_TUI = "Dragging sends Page Up/Down to the app. Tap the chip to change.";
function ScrollHint_defaultStorage() {
    return typeof localStorage === "undefined" ? null : localStorage;
}
/**
 * First-use hint state. Shown on the first scroll start while the seen flag is unset; stays
 * (no timeout) until dismissed or the picker/panel opens. The flag is written then, not on display.
 */ function useScrollHint() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    var _options_storage;
    const getStorage = (_options_storage = options.storage) !== null && _options_storage !== void 0 ? _options_storage : ScrollHint_defaultStorage;
    const memorySeen = (0,react.useRef)(false);
    const [visible, setVisible] = (0,react.useState)(false);
    const [route, setRoute] = (0,react.useState)("xterm-local");
    const isSeen = (0,react.useCallback)(()=>{
        if (memorySeen.current) return true;
        try {
            var _getStorage;
            return ((_getStorage = getStorage()) === null || _getStorage === void 0 ? void 0 : _getStorage.getItem(SCROLL_HINT_SEEN_KEY)) === "1";
        } catch (e) {
            return false;
        }
    }, [
        getStorage
    ]);
    const markSeen = (0,react.useCallback)(()=>{
        memorySeen.current = true;
        try {
            var _getStorage;
            (_getStorage = getStorage()) === null || _getStorage === void 0 ? void 0 : _getStorage.setItem(SCROLL_HINT_SEEN_KEY, "1");
        } catch (e) {
        // in-memory flag still applies for this page load
        }
    }, [
        getStorage
    ]);
    const notifyScrollStart = (0,react.useCallback)((next)=>{
        if (isSeen()) return;
        setRoute(next);
        setVisible(true);
    }, [
        isSeen
    ]);
    const dismiss = (0,react.useCallback)(()=>{
        markSeen();
        setVisible(false);
    }, [
        markSeen
    ]);
    return {
        visible,
        route,
        notifyScrollStart,
        dismiss,
        markSeenOnPanelOpen: dismiss
    };
}
/** The status region stays mounted so the hint's appearance is announced once. */ function ScrollHint(param) {
    let { visible, route, onDismiss } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        role: "status",
        "aria-live": "polite",
        children: visible && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: hint,
            "data-testid": "scroll-hint",
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    children: route === "xterm-local" ? HINT_TEXT_LOCAL : HINT_TEXT_TUI
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    type: "button",
                    className: ScrollingPanel_css_button,
                    onClick: onDismiss,
                    children: "Got it"
                })
            ]
        })
    });
}

;// ./src/components/sessions/ScrollModeChip.tsx
/* __next_internal_client_entry_do_not_use__ MISROUTE_HIGHLIGHT_MS,MISROUTE_ANNOUNCE_INTERVAL_MS,MISROUTE_ANNOUNCEMENT,routeLabel,useTouchCapable,useMisrouteCue,ScrollModeChip auto */ 
// +feature: terminal-scroll-settings




const MISROUTE_HIGHLIGHT_MS = 5000;
const MISROUTE_ANNOUNCE_INTERVAL_MS = 30000;
const MISROUTE_ANNOUNCEMENT = "The drag did not scroll. Scroll mode is Terminal history. Tap the chip to change.";
function routeLabel(target) {
    return target === "xterm-local" ? "Terminal history" : "Page keys";
}
/**
 * True when the device has a coarse pointer or a touchstart has been seen on the surface.
 * `getSurface` is read on mount; without a surface the document is watched instead.
 */ function useTouchCapable(getSurface) {
    const [touchSeen, setTouchSeen] = (0,react.useState)(false);
    const coarse =  true && typeof window.matchMedia === "function" ? window.matchMedia("(any-pointer: coarse)").matches : false;
    (0,react.useEffect)(()=>{
        var _getSurface;
        const target = (_getSurface = getSurface === null || getSurface === void 0 ? void 0 : getSurface()) !== null && _getSurface !== void 0 ? _getSurface : document;
        const onTouch = ()=>setTouchSeen(true);
        target.addEventListener("touchstart", onTouch, {
            passive: true,
            once: true
        });
        return ()=>target.removeEventListener("touchstart", onTouch);
    }, [
        getSurface
    ]);
    return coarse || touchSeen;
}
/** Misroute cue (ux.md S6): a local drag that moved nothing highlights the chip and announces at most once per interval. */ function useMisrouteCue() {
    let options = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const { now = Date.now, highlightMs = MISROUTE_HIGHLIGHT_MS, announceIntervalMs = MISROUTE_ANNOUNCE_INTERVAL_MS } = options;
    const [highlighted, setHighlighted] = (0,react.useState)(false);
    const [announcement, setAnnouncement] = (0,react.useState)("");
    const timer = (0,react.useRef)(null);
    const lastAnnounceAt = (0,react.useRef)(null);
    (0,react.useEffect)(()=>()=>{
            if (timer.current) clearTimeout(timer.current);
        }, []);
    const report = (0,react.useCallback)((param)=>{
        let { route, postSlopLines, viewportYChanged } = param;
        if (route !== "xterm-local" || postSlopLines < 1 || viewportYChanged) return;
        setHighlighted(true);
        if (timer.current) clearTimeout(timer.current);
        timer.current = setTimeout(()=>{
            setHighlighted(false);
            setAnnouncement("");
            timer.current = null;
        }, highlightMs);
        const t = now();
        if (lastAnnounceAt.current === null || t - lastAnnounceAt.current >= announceIntervalMs) {
            lastAnnounceAt.current = t;
            setAnnouncement(MISROUTE_ANNOUNCEMENT);
        }
    }, [
        now,
        highlightMs,
        announceIntervalMs
    ]);
    return {
        highlighted,
        announcement,
        report
    };
}
function ScrollModeChip(props) {
    const touchCapable = useTouchCapable(props.getTouchSurface);
    if (!touchCapable || props.visibleRows < MIN_ROWS_FOR_OVERLAYS) return null;
    const label = routeLabel(props.effectiveTarget);
    const gestures = props.gestureScrollEnabled ? "on" : "off";
    const text = "".concat(props.highlighted ? "! " : "").concat(label).concat(props.gestureScrollEnabled ? "" : ", gestures off");
    var _props_announcement;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                ref: props.buttonRef,
                className: chip,
                "data-highlighted": props.highlighted ? "true" : "false",
                "aria-label": "Scroll mode: ".concat(label, ". Touch gestures ").concat(gestures, ". Opens scroll settings"),
                onClick: props.onClick,
                children: text
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                className: LiveRegion_css/* srOnly */.s,
                "data-testid": "scroll-chip-announcer",
                children: props.suppressAnnouncements ? "" : (_props_announcement = props.announcement) !== null && _props_announcement !== void 0 ? _props_announcement : ""
            })
        ]
    });
}

;// ./src/components/sessions/ScrollingPanel.tsx
/* __next_internal_client_entry_do_not_use__ ScrollModeChip,useTouchCapable,useMisrouteCue,routeLabel,shouldRenderPanelAsOverlay,SCROLL_OPTIONS,GESTURE_DESCRIPTION,WIDE_OUTPUT_NOTE,TMUX_NOTE,ScrollingPanel auto */ 
// +feature: terminal-scroll-settings





/** True when opening the panel inline would leave too few terminal rows, so it must overlay instead. */ function shouldRenderPanelAsOverlay(rowsLeftIfInline) {
    return rowsLeftIfInline < MIN_ROWS_FOR_OVERLAYS;
}
const SCROLL_OPTIONS = [
    {
        value: "auto",
        label: "Auto (recommended)",
        announce: "Auto",
        description: "Picks for you based on what is running."
    },
    {
        value: "local",
        label: "Terminal history",
        announce: "Terminal history",
        description: "Dragging moves the terminal's own scrollback, following your finger. Use in a plain shell."
    },
    {
        value: "tui",
        label: "Page keys",
        announce: "Page keys",
        description: "Dragging sends Page Up/Down to the app, one page per half-screen. Use for Claude Code or tmux history."
    }
];
const GESTURE_DESCRIPTION = "Turn off if you use a screen reader (TalkBack). Off disables all touch gestures on the terminal: drag scrolling, tap to focus, double-tap and long-press selection. Use the PgUp/PgDn keys to scroll.";
const WIDE_OUTPUT_NOTE = "Long lines wrap; wide output cannot be panned sideways.";
const TMUX_NOTE = "In tmux, Page keys scroll tmux history. If typing seems ignored, tap PgDn until you reach the bottom. Esc is not offered because it interrupts a running Claude Code turn.";
function ScrollingPanel(props) {
    const { variant, override, effectiveTarget, gestureScrollEnabled, openerRef, renderAsOverlay, maxHeight } = props;
    const uid = (0,react.useId)();
    const [announcement, setAnnouncement] = (0,react.useState)("");
    const returnFocus = (0,react.useCallback)(()=>{
        var _openerRef_current;
        return openerRef === null || openerRef === void 0 ? void 0 : (_openerRef_current = openerRef.current) === null || _openerRef_current === void 0 ? void 0 : _openerRef_current.focus();
    }, [
        openerRef
    ]);
    const close = ()=>{
        props.onClose();
        returnFocus();
    };
    const select = (opt)=>{
        props.onOverrideChange(opt.value);
        setAnnouncement("Scroll mode: ".concat(opt.announce));
        if (variant === "picker") close();
    };
    const nowText = effectiveTarget === "xterm-local" ? "Terminal history" : "Page keys";
    const isFull = variant === "full";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ScrollingPanel_css_panel,
        "data-testid": "scrolling-".concat(variant),
        "data-overlay": renderAsOverlay,
        style: renderAsOverlay && maxHeight !== undefined ? {
            maxHeight
        } : undefined,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("fieldset", {
                className: fieldset,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("legend", {
                        className: legend,
                        children: "How dragging scrolls"
                    }),
                    SCROLL_OPTIONS.map((opt)=>{
                        const id = "".concat(uid, "-").concat(opt.value);
                        const descId = "".concat(id, "-desc");
                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                            className: ScrollingPanel_css_option,
                            htmlFor: id,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    id: id,
                                    className: optionInput,
                                    type: "radio",
                                    name: "".concat(uid, "-scroll-mode"),
                                    value: opt.value,
                                    checked: override === opt.value,
                                    onChange: ()=>select(opt),
                                    "aria-describedby": descId
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: optionText,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: optionLabel,
                                            children: opt.label
                                        }),
                                        opt.value === "auto" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            children: [
                                                "now: ",
                                                nowText
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            id: descId,
                                            className: description,
                                            children: opt.description
                                        })
                                    ]
                                })
                            ]
                        }, opt.value);
                    })
                ]
            }),
            isFull && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        className: switchRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "checkbox",
                                role: "switch",
                                checked: gestureScrollEnabled,
                                onChange: (e)=>{
                                    props.onGestureScrollChange(e.target.checked);
                                    setAnnouncement("Touch gestures: ".concat(e.target.checked ? "on" : "off"));
                                },
                                "aria-describedby": "".concat(uid, "-gesture-desc"),
                                className: optionInput
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: optionText,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: optionLabel,
                                        children: "Touch gestures (scroll, select)"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        id: "".concat(uid, "-gesture-desc"),
                                        className: description,
                                        children: GESTURE_DESCRIPTION
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: note,
                        children: WIDE_OUTPUT_NOTE
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: note,
                        children: TMUX_NOTE
                    })
                ]
            }),
            !isFull && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: ScrollingPanel_css_button,
                onClick: props.onOpenFull,
                children: "More scrolling settings"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: ScrollingPanel_css_button,
                onClick: close,
                children: "Close"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                className: LiveRegion_css/* srOnly */.s,
                "data-testid": "scrolling-announcer",
                children: announcement
            })
        ]
    });
}

;// ./src/lib/hooks/useEffectiveScrollMode.ts



/**
 * Single source for the route the toolbar PgUp/PgDn, chip and jump button follow.
 * The drag hook does not use it: it re-reads the terminal every frame.
 * Turning gestures off never changes the route, only `gesturesOn`.
 */ function useEffectiveScrollMode(scrollMode, override, gestureOn, tuiPolicy) {
    const { bufferType, mouseTrackingMode } = scrollMode;
    return (0,react.useMemo)(()=>({
            target: (0,scrollRouting/* decideScrollTarget */.H$)({
                bufferType,
                mouseTrackingMode
            }, undefined, tuiPolicy, override),
            source: override === "auto" ? "auto" : "override",
            gesturesOn: gestureOn
        }), [
        bufferType,
        mouseTrackingMode,
        override,
        gestureOn,
        tuiPolicy
    ]);
}
/** Live view of the persisted override and Gesture scrolling switch. */ function useScrollSettings() {
    let store = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : scrollSettings;
    const override = (0,react.useSyncExternalStore)(store.subscribe, store.getOverride, ()=>DEFAULT_SCROLL_OVERRIDE);
    const gestureScrollEnabled = (0,react.useSyncExternalStore)(store.subscribe, store.getGestureScroll, ()=>DEFAULT_GESTURE_SCROLL);
    return (0,react.useMemo)(()=>({
            override,
            gestureScrollEnabled
        }), [
        override,
        gestureScrollEnabled
    ]);
}

// EXTERNAL MODULE: ./src/lib/contexts/ApprovalsContext.tsx + 2 modules
var ApprovalsContext = __webpack_require__(38808);
// EXTERNAL MODULE: ./src/components/providers/ViewportProvider.tsx
var ViewportProvider = __webpack_require__(55765);
// EXTERNAL MODULE: ./src/lib/hooks/useInputModeOverride.ts
var useInputModeOverride = __webpack_require__(27043);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
;// ./src/components/sessions/TerminalOutput.tsx
/* provided dependency */ var TerminalOutput_process = __webpack_require__(36843);
/* __next_internal_client_entry_do_not_use__ TerminalOutput auto */ 
// +feature: terminal-pre-sizing terminal-dimension-cache terminal-image-upload













































// Minimum dimensions considered "real" — anything smaller is a transient value
// from xterm.js before the CSS container has finished laying out. The first
// resize event often fires at e.g. 10x6 before layout is complete; caching or
// connecting at those dimensions produces a garbled terminal on the next view.
// Read-only view: a keypress announces "Read-only session" at most once per this window (design/ux.md Surface 12).
const READ_ONLY_HINT_INTERVAL_MS = 10000;
// Story 2.3 — coalescing window for InputDropBadge drop episodes (design/ux.md §2.2).
const DROP_EPISODE_COALESCE_WINDOW_MS = 400;
// Matches XtermTerminal's assumed first scroll mode, so only real changes are reported.
const INITIAL_SCROLL_MODE = {
    bufferType: "normal",
    mouseTrackingMode: "none"
};
const SCROLL_PICKER_PANEL_ROWS = 10;
const SCROLL_FULL_PANEL_ROWS = 20;
/**
 * Story 1.4.3 — the exact toast copy for a BLOCKED outcome, keyed on
 * ScrollBlockedReason. Three distinct strings, not one generic "blocked"
 * catch-all (adversarial-review BLOCKER: a solo user on
 * PathLegacyPerConnection, which is unconditionally BLOCKED regardless of
 * actual viewer count, must never see the "another viewer is connected"
 * claim). Copy matches design/ux.md Surface 3 / plan.md Story 1.4.3 verbatim.
 */ function blockedToastCopy(reason, program) {
    const label = program || "this session";
    switch(reason){
        case events_pb/* ScrollBlockedReason */.ec.UNSUPPORTED_STREAMING_PATH:
            return "Scroll-forwarding isn't available for this session yet";
        case events_pb/* ScrollBlockedReason */.ec.LEASE_CONTENTION:
            return "Still loading — try again in a moment";
        case events_pb/* ScrollBlockedReason */.ec.MULTIPLE_VIEWERS:
        case events_pb/* ScrollBlockedReason */.ec.SCROLL_BLOCKED_REASON_UNSPECIFIED:
        default:
            return "Can't browse ".concat(label, "'s history right now — another viewer is connected. Close other tabs or sessions viewing this session to enable it.");
    }
}
function TerminalOutput(param) {
    let { sessionId, baseUrl, isExternal = false, tmuxSessionName, isVisible, shellId, onShellStatusChange, scheduleResync, readOnly = false } = param;
    var _lastResizeRef_current, _lastResizeRef_current1, _xtermRef_current, _jumpTerminal_buffer;
    const { track } = (0,AnalyticsContext/* useAnalytics */.st)();
    const { announce } = (0,useAnnounce/* useAnnounce */.XE)();
    const { clearForSession, refresh: refreshApprovals, pendingCount } = (0,ApprovalsContext/* useApprovalsContext */.B)();
    const { leftHanded, toggleHandedness } = (0,useHandedness/* useHandedness */.V)();
    const terminalContainerRef = (0,react.useRef)(null);
    const toolbarRef = (0,react.useRef)(null);
    const overflowRowRef = (0,react.useRef)(null);
    // Only the visible terminal anchors the phone deck; a background pane must not move it.
    (0,usePublishStackTopOffset/* usePublishDeckTopOffset */.K)(toolbarRef, isVisible !== false, overflowRowRef);
    // Story 3 (Task 3.4) — anchor <div> the pooled terminal docks into. A
    // plain, otherwise-childless node (see TerminalPool.tsx's module doc
    // comment for why it must never receive JSX children of its own): the
    // pool imperatively appends/removes its host node here, which would race
    // with React's own reconciliation if this div had other React-managed
    // children.
    const dockRef = (0,react.useRef)(null);
    // Computed early (needs only props) so it's available to usePooledTerminal
    // below, which must itself run early -- see that hook's call site comment.
    const effectiveSessionId = isExternal && tmuxSessionName ? tmuxSessionName : sessionId;
    // Story 3 (Task 3.4/3.5) — replaces a locally-owned
    // `useRef<XtermTerminalHandle | null>(null)` + directly-rendered
    // `<XtermTerminal>`. Returns a stable ref into a pool-owned, possibly
    // already-warm Terminal instance that survives this component's own
    // remounts (e.g. a pane's assigned session changing -- see
    // TerminalPool.tsx's module doc comment for why that matters here).
    // isVisible defaults to visible (`!== false`) to match this file's other
    // isVisible-optional call sites (e.g. the loading overlay below).
    const { xtermRef, warmRef } = (0,TerminalPool/* usePooledTerminal */.L)(effectiveSessionId, dockRef, isVisible !== false);
    // Track actual rendered pane dimensions via ResizeObserver. Fires after CSS layout
    // has constrained the container to its real pane width (not the full browser window).
    // Used to guard connect() and pre-sizing so we never send wrong dimensions in the handshake.
    const containerSize = useSplitContainerSize(terminalContainerRef);
    const [connectionAttempts, setConnectionAttempts] = (0,react.useState)(0);
    const [showReconnectButton, setShowReconnectButton] = (0,react.useState)(false);
    const [isWaitingForStableSize, setIsWaitingForStableSize] = (0,react.useState)(true);
    // Story 3 (Task 3.4) — a warm pool entry (already showing content from a
    // prior mount) skips the loading overlay entirely; only a genuinely cold
    // entry starts in the loading state. See PoolEntry.warmRef's doc comment.
    const [isLoadingInitialContent, setIsLoadingInitialContent] = (0,react.useState)(()=>!warmRef.current);
    const reconnectTimeoutRef = (0,react.useRef)(null);
    const previousConnectionStateRef = (0,react.useRef)(false);
    const lastResizeRef = (0,react.useRef)(null);
    const refreshCountRef = (0,react.useRef)(0);
    const isMountedRef = (0,react.useRef)(true);
    // One-shot: set when a viewport settle triggers refit(); the single resize that fit produces bypasses the
    // server bounce hold (Story 2.1.5) and consumes it. Any later resize, e.g. an A->B->A oscillation, is held.
    const settleBypassPendingRef = (0,react.useRef)(false);
    const clearedAtRef = (0,react.useRef)(null);
    const sizeStabilityTimeoutRef = (0,react.useRef)(null);
    const hasInitiatedConnectionRef = (0,react.useRef)(false);
    const hasCachedDimensionsRef = (0,react.useRef)(false);
    // Set to true when we switch sessions while connected; triggers connect() once disconnect completes
    const pendingConnectAfterDisconnectRef = (0,react.useRef)(false);
    // Story 3.2.1 — reconnecting banner state
    const hasEverConnectedRef = (0,react.useRef)(false);
    const bannerTimerRef = (0,react.useRef)(null);
    const [showReconnectBanner, setShowReconnectBanner] = (0,react.useState)(false);
    // Debounce timer for the approval re-fetch triggered by any keystroke (ADR-4)
    const refreshTimerRef = (0,react.useRef)(null);
    // TerminalStreamManager ref -- lazily initialized when terminal is available
    const streamManagerRef = (0,react.useRef)(null);
    // Ref to hold sendFlowControl function (allows use in callbacks defined before useTerminalStream)
    const sendFlowControlRef = (0,react.useRef)(null);
    // Task 4.2.2 — Queue output during RESIZING state (Pitfall #2 / Race 3).
    // A snapshot arriving while the client is mid-resize writes bytes at the old column width.
    // Queuing output during resize prevents stale bytes from being written before the post-resize snapshot.
    const pendingOutputDuringResizeRef = (0,react.useRef)([]);
    // Terminal loading metrics
    const metricsRef = (0,react.useRef)({
        mountTime: performance.now(),
        firstResizeTime: null,
        sizeStableTime: null,
        connectionInitTime: null,
        connectedTime: null,
        firstOutputTime: null,
        resizeCount: 0
    });
    // xterm.js preload moved to TerminalPoolProvider (Story 3) -- it's now the
    // sole mount point for <XtermTerminal>, so the preload only needs to run
    // once app-wide rather than per TerminalOutput mount.
    const logTerminalMetrics = (0,react.useCallback)(()=>{
        var _metrics_sizeStabilizationDuration, _metrics_connectionDuration, _window_performance;
        const m = metricsRef.current;
        if (!m.firstOutputTime) return;
        const metrics = {
            totalLoadTime: m.firstOutputTime - m.mountTime,
            timeToFirstResize: m.firstResizeTime ? m.firstResizeTime - m.mountTime : null,
            timeToSizeStable: m.sizeStableTime ? m.sizeStableTime - m.mountTime : null,
            sizeStabilizationDuration: m.sizeStableTime && m.firstResizeTime ? m.sizeStableTime - m.firstResizeTime : null,
            timeToConnectionInit: m.connectionInitTime ? m.connectionInitTime - m.mountTime : null,
            timeToConnected: m.connectedTime ? m.connectedTime - m.mountTime : null,
            connectionDuration: m.connectedTime && m.connectionInitTime ? m.connectedTime - m.connectionInitTime : null,
            timeToFirstOutput: m.firstOutputTime - m.mountTime,
            resizeEventsBeforeStable: m.resizeCount,
            sessionId: isExternal ? tmuxSessionName : sessionId,
            isExternal
        };
        console.log("[TerminalMetrics] Terminal loaded in ".concat(metrics.totalLoadTime.toFixed(0), "ms"), {
            breakdown: {
                sizeStabilization: "".concat(((_metrics_sizeStabilizationDuration = metrics.sizeStabilizationDuration) === null || _metrics_sizeStabilizationDuration === void 0 ? void 0 : _metrics_sizeStabilizationDuration.toFixed(0)) || 'N/A', "ms (").concat(metrics.resizeEventsBeforeStable, " resizes)"),
                wsConnection: "".concat(((_metrics_connectionDuration = metrics.connectionDuration) === null || _metrics_connectionDuration === void 0 ? void 0 : _metrics_connectionDuration.toFixed(0)) || 'N/A', "ms"),
                totalLoad: "".concat(metrics.totalLoadTime.toFixed(0), "ms")
            },
            detailed: metrics
        });
        if ( true && typeof ((_window_performance = window.performance) === null || _window_performance === void 0 ? void 0 : _window_performance.mark) === 'function') {
            try {
                performance.mark('terminal-loaded');
                performance.measure('terminal-load-time', {
                    start: m.mountTime,
                    end: m.firstOutputTime
                });
            } catch (e) {
            // Ignore if performance API not fully supported
            }
        }
    }, [
        sessionId,
        tmuxSessionName,
        isExternal
    ]);
    // Debug mode state
    const [debugMode, setDebugMode] = (0,react.useState)(()=>{
        if (true) {
            return localStorage.getItem("debug-terminal") === "true";
        }
        return false;
    });
    // Remote log streaming state — persisted in localStorage
    const [logStreamEnabled, setLogStreamEnabled] = (0,react.useState)(()=>{
        if (true) {
            return localStorage.getItem("stapler-squad-remote-debug") === "true";
        }
        return false;
    });
    // Sticky modifier keys — CTRL, ALT, SHIFT arm on first tap, fire+clear on the next key.
    // Mutually exclusive: arming one disarms the others.
    const [ctrlActive, setCtrlActive] = (0,react.useState)(false);
    const [altActive, setAltActive] = (0,react.useState)(false);
    const [shiftActive, setShiftActive] = (0,react.useState)(false);
    // Transient paste error shown briefly when clipboard access is denied.
    const [pasteError, setPasteError] = (0,react.useState)(null);
    const pasteErrorTimerRef = (0,react.useRef)(null);
    // Multi-file upload state for the three-button toolbar picker.
    const cameraInputRef = (0,react.useRef)(null);
    const galleryInputRef = (0,react.useRef)(null);
    const filesInputRef = (0,react.useRef)(null);
    const [uploadingCount, setUploadingCount] = (0,react.useState)(0);
    const [uploadError, setUploadError] = (0,react.useState)(null);
    const [uploadSuccess, setUploadSuccess] = (0,react.useState)(null);
    const uploadErrorTimerRef = (0,react.useRef)(null);
    const uploadSuccessTimerRef = (0,react.useRef)(null);
    // Mobile keyboard visibility — persisted in localStorage, scoped per session
    const keyboardStorageKey = "stapler-squad-mobile-keyboard-visible-".concat(sessionId);
    const [isKeyboardVisible, setIsKeyboardVisible] = (0,react.useState)(()=>{
        if (false) {}
        try {
            const stored = localStorage.getItem(keyboardStorageKey);
            return stored === null ? true : stored === 'true';
        } catch (e) {
            return true;
        }
    });
    const toggleMobileKeyboard = (0,react.useCallback)(()=>{
        setIsKeyboardVisible((prev)=>{
            const next = !prev;
            try {
                localStorage.setItem(keyboardStorageKey, String(next));
            } catch (e) {
            // localStorage full or disabled — continue without persistence
            }
            return next;
        });
    }, [
        keyboardStorageKey
    ]);
    // Mobile detection — use shared ViewportProvider hook for consistency
    const { isMobile, hasFinePointer } = (0,ViewportProvider/* useViewport */.k)();
    // User-overridable detection for a real mouse+keyboard attached to a phone/tablet —
    // see Settings > Appearance > Terminal Input Mode.
    const { inputModeOverride } = (0,useInputModeOverride/* useInputModeOverride */.t)();
    const compactToolbar = inputModeOverride === 'desktop' || inputModeOverride === 'auto' && isMobile && hasFinePointer;
    // Toolbar collapsed/expanded state — persisted in localStorage; collapsed by default
    const [toolbarExpanded, setToolbarExpanded] = (0,react.useState)(()=>{
        if (false) {}
        try {
            const s = localStorage.getItem('stapler-squad-toolbar-expanded');
            return s === null ? false : s === 'true';
        } catch (e) {
            return false;
        }
    });
    const [mobileOverflowOpen, setMobileOverflowOpen] = (0,react.useState)(false);
    // The first time a mouse+physical keyboard is detected on this mobile session,
    // collapse the toolbar and hide the on-screen keyboard row by default so they
    // don't eat screen space — but only if the user hasn't already made an explicit
    // choice for either. Runs once per session mount; manual toggles afterward are
    // fully respected.
    const appliedCompactDefaultsRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        if (!compactToolbar || appliedCompactDefaultsRef.current) return;
        appliedCompactDefaultsRef.current = true;
        setToolbarExpanded(false);
        try {
            if (localStorage.getItem(keyboardStorageKey) === null) {
                setIsKeyboardVisible(false);
            }
        } catch (e) {
        // localStorage unavailable — leave the on-screen keyboard row visible
        }
    }, [
        compactToolbar,
        keyboardStorageKey
    ]);
    // Dev tools panel — persisted in localStorage; collapsed by default
    const [devGroupOpen, setDevGroupOpen] = (0,react.useState)(()=>{
        if (false) {}
        try {
            return localStorage.getItem('stapler-squad-dev-toolbar') === 'true';
        } catch (e) {
            return false;
        }
    });
    const devToggleRef = (0,react.useRef)(null);
    const handleDevGroupToggle = ()=>{
        const next = !devGroupOpen;
        track({
            name: "toolbar_button_click",
            category: "user_action",
            sessionId,
            component: "TerminalOutput",
            labels: {
                button: "dev-panel",
                state: next ? "open" : "closed"
            }
        });
        setDevGroupOpen(next);
        try {
            localStorage.setItem('stapler-squad-dev-toolbar', next ? 'true' : 'false');
        } catch (e) {
        // localStorage unavailable
        }
        if (!next) {
            setTimeout(()=>{
                var _devToggleRef_current;
                return (_devToggleRef_current = devToggleRef.current) === null || _devToggleRef_current === void 0 ? void 0 : _devToggleRef_current.focus();
            }, 0);
        }
    };
    (0,react.useEffect)(()=>{
        try {
            localStorage.setItem('stapler-squad-toolbar-expanded', String(toolbarExpanded));
        } catch (e) {
        // localStorage unavailable — continue without persistence
        }
    }, [
        toolbarExpanded
    ]);
    // Mouse tracking mode: "none" on mobile (enables xterm selection + touch scroll),
    // "any" on desktop (forwards mouse events to terminal app for vim/tmux mouse support).
    // User can toggle this via the ⌨️ toolbar to switch modes.
    const [mouseMode, setMouseMode] = (0,react.useState)('any');
    (0,react.useEffect)(()=>{
        setMouseMode(isMobile ? 'none' : 'any');
    }, [
        isMobile
    ]);
    const toggleMouseMode = (0,react.useCallback)(()=>{
        setMouseMode((prev)=>prev === 'none' ? 'any' : 'none');
    }, []);
    // Recording state
    const [isRecording, setIsRecording] = (0,react.useState)(false);
    // Opt-in, display-only output filter: reads lines out of xterm's buffer and
    // renders its own list, so it never touches the stream or the buffer itself.
    const [filterOpen, setFilterOpen] = (0,react.useState)(false);
    const getFilterLines = (0,react.useCallback)(()=>{
        var _xtermRef_current;
        return readBufferLines((_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal);
    }, [
        xtermRef
    ]);
    // The terminal chrome (tabs, header) always uses the dark VS Code-style
    // palette defined in terminalTokens (styles/theme.css.ts), independent of
    // the app's selectable UI theme. xterm.js must match that fixed palette —
    // deriving it from prefers-color-scheme instead caused the terminal canvas
    // to render solid white whenever the OS/browser preference was light while
    // the surrounding chrome stayed dark. (Story 3: the pooled <XtermTerminal>
    // itself now hardcodes theme="dark" directly -- see TerminalPool.tsx --
    // for the same reason; no local `theme` binding is needed here anymore.)
    // Paging state for on-demand scrollback loading (Task 2.3.1 / 2.3.2)
    const isFetchingScrollbackRef = (0,react.useRef)(false);
    const hasMoreScrollbackRef = (0,react.useRef)(false);
    const oldestSequenceReceivedRef = (0,react.useRef)(0);
    // Story 1.4.0 — client-local mirror of TerminalStreamManager's altScreenActive,
    // read by the wheel listener and passed to useTerminalGestures as
    // isAltScreenActive. A ref (not state) since it's read from event-handler
    // closures, not rendered directly.
    const altScreenActiveRef = (0,react.useRef)(false);
    // Story 1.4.4 (Task 1.4.4b) — "no more app-forwarded history" flag, distinct
    // from hasMoreScrollbackRef above: the tmux-native path owns that ref
    // exclusively and this feature never reads or writes it, so a plain-shell /
    // flag-off / non-Claude session's exhaustion behavior can't be affected even
    // indirectly through shared state (plan.md Story 1.4.4 Scope correction).
    const hasMoreAppScrollbackRef = (0,react.useRef)(true);
    // Story 1.4.2 — cached from the last AppScrollbackResponse.program this
    // session has seen (any outcome), so the loading pill can show a
    // program-specific label for a *subsequent* app-forwarded request within
    // the same session even though the program name itself only ever arrives
    // in the response, never known ahead of the request that triggers it.
    const lastKnownAppProgramRef = (0,react.useRef)(undefined);
    // ---- Story 1.4.5 — scroll-forward loading pill state ----
    // Shared by BOTH the tmux-native paged-scrollback trigger and Story 1.4.0's
    // app-forwarded alt-screen triggers (design/ux.md Surface 1) — every
    // requestScrollback call site funnels through requestScrollbackWithPill
    // below so the pill can't drift out of sync with which trigger fired.
    const { pillState, clearScrollLoadingState, startScrollLoadingTimers } = useScrollLoadingPill();
    // Task 1.4.5b — Cancel resets local fetch-in-flight state and clears the
    // pill without cancelling the in-flight server request (design/ux.md: a
    // late response is simply ignored once this state has been reset).
    const handleCancelScrollLoading = (0,react.useCallback)(()=>{
        clearScrollLoadingState();
        isFetchingScrollbackRef.current = false;
    }, [
        clearScrollLoadingState
    ]);
    // ---- Story 1.4.2 — ScrollSourceIndicator banner state ----
    const [appScrollbackState, setAppScrollbackState] = (0,react.useState)({
        active: false,
        program: ""
    });
    // Ref mirror read by handleOutput (Task 1.4.2b) — kept out of handleOutput's
    // own dependency array so a banner mount/unmount can't recreate that
    // callback's identity, which useTerminalStream's connection effect depends
    // on (recreating it there would spuriously re-run connection setup).
    const appScrollbackActiveRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        appScrollbackActiveRef.current = appScrollbackState.active;
    }, [
        appScrollbackState.active
    ]);
    // Distinguishes the redraw echo of our own scroll-forward from a genuine live
    // resume -- see ForwardEchoGuard's doc comment for why content, not timing.
    const [echoGuard] = (0,react.useState)(()=>new ForwardEchoGuard());
    // ---- Story 1.4.3 — Blocked outcome toast state ----
    const { blockedToast, connectionPulse, showBlockedOutcome, dismissBlockedToast } = useBlockedToast();
    // ---- Story 1.4.4 — "No more history available" (AT_TOP) affordance state ----
    const [atTopVisible, setAtTopVisible] = (0,react.useState)(false);
    // Resets scrollback paging so a subsequent scroll-up re-fetches history fresh from
    // the server (which re-derives it width-agnostically via `tmux capture-pane -J`,
    // see handleScrollbackRequest in connectrpc_websocket.go) instead of assuming
    // whatever paging cursor was left over from before the terminal was last cleared.
    // Registered as TerminalStreamManager's onFullSnapshot callback below — the manager
    // itself owns detecting a full-pane replacement snapshot and clearing the xterm
    // buffer for it (ANSI_SNAPSHOT_PREFIX in TerminalStreamManager.ts); this only resets
    // the paging refs the manager doesn't know about.
    // Gesture-cancel / netPagesUp-reset trigger (ux.md S7/S9): bumped on every reconnect
    // and on every full-snapshot write.
    const [connectionEpoch, setConnectionEpoch] = (0,react.useState)(0);
    const bumpConnectionEpoch = (0,react.useCallback)(()=>setConnectionEpoch((e)=>e + 1), []);
    const resetScrollbackPaging = (0,react.useCallback)(()=>{
        hasMoreScrollbackRef.current = true;
        hasMoreAppScrollbackRef.current = true;
        oldestSequenceReceivedRef.current = 0;
        isFetchingScrollbackRef.current = false;
        // A full-pane replacement snapshot (reconnect, resize resync) means any
        // forwarded-history state from before it is now stale — design/ux.md
        // Surface 2's edge case: "reconnection resets to the live tail... a stale
        // 'Viewing history' banner never survives a reconnect."
        clearScrollLoadingState();
        setAppScrollbackState({
            active: false,
            program: ""
        });
        setAtTopVisible(false);
        dismissBlockedToast();
        echoGuard.clear();
    }, [
        clearScrollLoadingState,
        dismissBlockedToast,
        echoGuard
    ]);
    // Ref-mirror for handleAppScrollbackFrame (Story 1.4.1-1.4.4, defined further
    // below) — same temporal-dead-zone reason as notifyResyncOutputReceivedRef:
    // getOrCreateStreamManager registers it with the manager before the real
    // handler (which needs pill/banner/toast state declared later) exists.
    const handleAppScrollbackFrameRef = (0,react.useRef)(()=>{});
    // Proactively clears the terminal and resets scrollback paging before a resize RPC
    // is sent — used only by the resize triggers below (auto-fit and the manual Resize
    // button), so the screen goes blank immediately rather than showing stale,
    // differently-wrapped content for the ~100-400ms round trip until the server's
    // post-resize snapshot arrives and TerminalStreamManager clears it again anyway.
    const clearBufferBeforeResize = (0,react.useCallback)(()=>{
        var _xtermRef_current;
        clearedAtRef.current = Date.now();
        mobileDebug/* mobileDebug */.Uo.log("clear-before-resize", {});
        (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.clear();
        resetScrollbackPaging();
    }, [
        resetScrollbackPaging,
        xtermRef
    ]);
    // Lazily create or get the TerminalStreamManager
    const getOrCreateStreamManager = (0,react.useCallback)(()=>{
        var _xtermRef_current, _xtermRef_current1;
        if (streamManagerRef.current) return streamManagerRef.current;
        const terminal = (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal;
        if (!terminal) return null;
        const manager = new TerminalStreamManager(terminal, (paused, watermark)=>{
            var _sendFlowControlRef_current;
            return (_sendFlowControlRef_current = sendFlowControlRef.current) === null || _sendFlowControlRef_current === void 0 ? void 0 : _sendFlowControlRef_current.call(sendFlowControlRef, paused, watermark);
        });
        // Detect + clear on a full-pane replacement snapshot (ANSI_SNAPSHOT_PREFIX) — the
        // single spot this is handled; every write() call site (live output, the RESIZING
        // queue flush) benefits without needing its own check.
        manager.setOnFullSnapshot(()=>{
            resetScrollbackPaging();
            bumpConnectionEpoch();
            if (clearedAtRef.current !== null) {
                mobileDebug/* mobileDebug */.Uo.log("snapshot-after-clear", {
                    elapsedMs: Date.now() - clearedAtRef.current
                });
                clearedAtRef.current = null;
            }
        });
        // Story 1.4.0 (Task 1.4.0a) — mirror the manager's altScreenActive locally
        // so the wheel listener / useTerminalGestures can read it synchronously.
        manager.setOnAltScreenChange((active)=>{
            altScreenActiveRef.current = active;
        });
        // Story 1.4.1 (Task 1.4.1a) — route a captured AppScrollbackResponse frame
        // to this component's outcome-rendering logic, defined further below.
        // Indirected through a ref (same temporal-dead-zone reason as
        // notifyResyncOutputReceivedRef above handleOutput) since
        // handleAppScrollbackFrame is defined after getOrCreateStreamManager.
        manager.setOnAppScrollback((frame)=>handleAppScrollbackFrameRef.current(frame));
        // Inject SerializeAddon so prependScrollbackBatch can serialize the current buffer
        // before clearing it (enables correct history order without losing live content).
        const serializeAddon = (_xtermRef_current1 = xtermRef.current) === null || _xtermRef_current1 === void 0 ? void 0 : _xtermRef_current1.serializeAddon;
        if (serializeAddon) {
            manager.setSerializeAddon(serializeAddon);
        }
        // Track first output for metrics and loading overlay
        manager.setOnFirstOutput(()=>{
            if (metricsRef.current.firstOutputTime === null) {
                metricsRef.current.firstOutputTime = performance.now();
                logTerminalMetrics();
                const totalLoadTime = metricsRef.current.firstOutputTime - metricsRef.current.mountTime;
                track({
                    name: "session_attach",
                    category: "performance",
                    durationMs: totalLoadTime,
                    labels: {
                        phase: "attach"
                    },
                    sessionId
                });
                const connectionDuration = metricsRef.current.connectedTime && metricsRef.current.connectionInitTime ? metricsRef.current.connectedTime - metricsRef.current.connectionInitTime : totalLoadTime;
                track({
                    name: "stream_terminal_first_byte",
                    category: "performance",
                    durationMs: connectionDuration,
                    sessionId
                });
                setIsLoadingInitialContent(false);
                // Story 3 (Task 3.4) — mark this pool entry warm so a future
                // TerminalOutput remount for the same session skips the loading
                // overlay (see isLoadingInitialContent's lazy initializer above).
                warmRef.current = true;
            }
        });
        // Install debug monitoring (respects debug-terminal localStorage flag)
        manager.installDebugMonitor();
        streamManagerRef.current = manager;
        return manager;
    }, [
        logTerminalMetrics,
        sessionId,
        track,
        resetScrollbackPaging,
        bumpConnectionEpoch,
        xtermRef,
        warmRef
    ]);
    const handleAppScrollbackFrame = (0,react.useCallback)((frame)=>{
        // Task 1.4.5b — the pill never persists past outcome delivery.
        clearScrollLoadingState();
        isFetchingScrollbackRef.current = false;
        if (frame.program) {
            lastKnownAppProgramRef.current = frame.program;
        }
        switch(frame.outcome){
            case events_pb/* ScrollForwardOutcome */.tB.DELIVERED:
                {
                    const manager = getOrCreateStreamManager();
                    if (manager && frame.content) {
                        void manager.prependScrollbackBatch(frame.content);
                    }
                    appScrollbackActiveRef.current = true;
                    setAppScrollbackState({
                        active: true,
                        program: frame.program
                    });
                    dismissBlockedToast();
                    echoGuard.record(frame.content);
                    break;
                }
            case events_pb/* ScrollForwardOutcome */.tB.AT_TOP:
                {
                    // Task 1.4.4b — a distinct flag from hasMoreScrollbackRef, which
                    // this feature never reads or writes (plan.md Story 1.4.4's Scope
                    // correction). Does NOT touch appScrollbackState — a banner already
                    // showing stays up alongside this affordance (design/ux.md Surface
                    // 2's "banner does not flicker off between pages" edge case).
                    hasMoreAppScrollbackRef.current = false;
                    setAtTopVisible(true);
                    echoGuard.record(frame.content);
                    break;
                }
            case events_pb/* ScrollForwardOutcome */.tB.NO_CAPABILITY:
                break;
            case events_pb/* ScrollForwardOutcome */.tB.BLOCKED:
            default:
                showBlockedOutcome(frame);
                break;
        }
    }, [
        clearScrollLoadingState,
        getOrCreateStreamManager,
        dismissBlockedToast,
        showBlockedOutcome,
        echoGuard
    ]);
    (0,react.useEffect)(()=>{
        handleAppScrollbackFrameRef.current = handleAppScrollbackFrame;
    }, [
        handleAppScrollbackFrame
    ]);
    // Story 1.4.1 (Task 1.4.1a) — decode useTerminalStream's raw
    // AppScrollbackResponse fields and dispatch through TerminalStreamManager,
    // mirroring how handleOutput routes through manager.write().
    const handleAppScrollbackReceived = (0,react.useCallback)((content, outcome, program, forwardId, blockedReason)=>{
        const manager = getOrCreateStreamManager();
        manager === null || manager === void 0 ? void 0 : manager.handleAppScrollback({
            content,
            outcome,
            program,
            forwardId,
            blockedReason
        });
    }, [
        getOrCreateStreamManager
    ]);
    // Story 1.4.0 bug fix — see TerminalStreamManager.setAltScreenActive()'s
    // doc comment: a capture-pane-derived initial/resize snapshot can never
    // carry the DECSET 1049h marker, so this server-authoritative hint (not
    // content-scanning) is what lets a client connecting to an already
    // alt-screen-active session learn that state at all.
    const handleAltScreenActiveHint = (0,react.useCallback)((active)=>{
        const manager = getOrCreateStreamManager();
        if (active) {
            manager === null || manager === void 0 ? void 0 : manager.setAltScreenActive();
        } else {
            manager === null || manager === void 0 ? void 0 : manager.setAltScreenInactive();
        }
    }, [
        getOrCreateStreamManager
    ]);
    // Ref to track whether the initial scrollback has been written (Task 2.3.2)
    const isInitialScrollbackDoneRef = (0,react.useRef)(false);
    // Callback to write initial pane content to terminal.
    // Metadata guard removed (R2.7): all scrollback — both initial and historical —
    // is now allowed to proceed. Initial load calls writeInitialContent(); paged
    // loads (subsequent calls with metadata) call prependScrollbackBatch().
    const handleScrollbackReceived = (0,react.useCallback)(async (scrollback, metadata)=>{
        var _xtermRef_current;
        // Task 1.4.5b — the pill never persists past outcome delivery, for any
        // outcome including the unchanged tmux-native path.
        clearScrollLoadingState();
        if (!((_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal)) return;
        const manager = getOrCreateStreamManager();
        if (!manager) return;
        if (!isInitialScrollbackDoneRef.current) {
            // Initial load
            console.log("[TerminalOutput] Writing initial scrollback: ".concat(scrollback.length, " bytes"));
            isInitialScrollbackDoneRef.current = true;
            await manager.writeInitialContent(scrollback);
            if (metadata) {
                hasMoreScrollbackRef.current = metadata.hasMore;
                oldestSequenceReceivedRef.current = metadata.oldestSequence;
            }
        } else {
            // Paged history load
            console.log("[TerminalOutput] Writing paged scrollback: ".concat(scrollback.length, " bytes"));
            try {
                await manager.prependScrollbackBatch(scrollback);
                if (metadata) {
                    hasMoreScrollbackRef.current = metadata.hasMore;
                    oldestSequenceReceivedRef.current = metadata.oldestSequence;
                }
            } catch (err) {
                console.error('[TerminalOutput] prependScrollbackBatch failed:', err);
            } finally{
                isFetchingScrollbackRef.current = false;
            }
        }
        setIsLoadingInitialContent(false);
    }, [
        getOrCreateStreamManager,
        clearScrollLoadingState,
        xtermRef
    ]);
    // Ref to access current terminalState inside the handleOutput callback
    // (useCallback can't take terminalState as a dep without recreating on every state change)
    const terminalStateRef = (0,react.useRef)('DISCONNECTED');
    // Ref-mirror for notifyResyncOutputReceived (Story 2.1.5, Task 2.1.5b). This
    // must be a ref rather than a direct useCallback dependency: handleOutput is
    // defined here and passed into useTerminalStream as `onOutput`, but
    // notifyResyncOutputReceived isn't available until useVisibilityResync is
    // called further below (which itself depends on useTerminalStream's return
    // values) — referencing it directly in this dependency array would be a
    // temporal-dead-zone error. Same ref-mirror idiom used throughout this file
    // (e.g. sendFlowControlRef) and in useTerminalStream.ts (connectRef).
    const notifyResyncOutputReceivedRef = (0,react.useRef)(()=>{});
    // Epic 3.1, Task 3.1.2.1 — shared between useTerminalStream (which forwards
    // it to useTerminalFlowControl, where both the visibility- and
    // resize-triggered resync paths register their resync_id) and this
    // component's own handleOutput below (which checks/clears membership on
    // each incoming TerminalOutput.resync_id). Lets either resync flow's stall
    // watchdog be reset when a match arrives on ANY tracked request, without
    // merging useVisibilityResync and useTerminalFlowControl into one hook.
    //
    // Task 3.1.2.4a — a plain `Set<string>` only records *membership*, not
    // *when* an ID was added, so a resync whose own response silently hangs
    // has no way to notice it's been outstanding too long if sibling resync
    // traffic keeps resetting the shared stall watchdog. Map<resyncId, addedAtMs>
    // keeps the same membership semantics (`.has`/`.delete` used by
    // handleOutput below are unchanged) while letting useVisibilityResync's
    // resetStallWatchdog (Task 3.1.2.4b) compute a specific ID's elapsed
    // outstanding time and escalate past a 2x-timeout ceiling instead of
    // resetting indefinitely.
    const outstandingResyncIdsRef = (0,react.useRef)(new Map());
    // Ref-mirror for resetStallWatchdog (Epic 3.1, Task 3.1.2.2) — same
    // temporal-dead-zone reason as notifyResyncOutputReceivedRef above:
    // useVisibilityResync isn't called until after this handleOutput
    // definition and the useTerminalStream call that consumes it.
    const resetStallWatchdogRef = (0,react.useRef)(()=>{});
    // Callback to write output directly to terminal via TerminalStreamManager.
    // Task 4.2.2: Output is queued during RESIZING to prevent bytes at the old column width
    // from being written before the post-resize snapshot.
    const handleOutput = (0,react.useCallback)((output, resyncId)=>{
        if (!xtermRef.current) return;
        // Epic 3.1 (AC2) — if this output is the reply to a correlation-ID-
        // tagged resync request tracked via EITHER hook's resync flow (resize or
        // visibility/focus), clear it here and reconcile the visibility hook's
        // stall watchdog too, so a resize-triggered resync response doesn't
        // leave a concurrently-pending visibility resync looking stalled.
        if (resyncId && outstandingResyncIdsRef.current.has(resyncId)) {
            outstandingResyncIdsRef.current.delete(resyncId);
            resetStallWatchdogRef.current();
        }
        // Signal resync receipt regardless of whether this output is written
        // immediately or queued for post-resize flush — a concurrent resize
        // (plausible on backgrounded-tab wakeup) must not make a pending
        // visibility/focus resync look stalled just because its response
        // arrived while RESIZING.
        notifyResyncOutputReceivedRef.current(resyncId);
        // Task 1.4.2b — a normal (non-app-scrollback) output frame means the
        // server has resumed live streaming, i.e. the scroll lease was released —
        // the banner is cleared on such a frame, unless it's recognizable as the
        // redraw echo of our own last DELIVERED/AT_TOP forward -- see
        // ForwardEchoGuard's doc comment.
        if (appScrollbackActiveRef.current && !echoGuard.isSelfEcho(output)) {
            appScrollbackActiveRef.current = false;
            setAppScrollbackState({
                active: false,
                program: ""
            });
        }
        if (terminalStateRef.current === 'RESIZING') {
            pendingOutputDuringResizeRef.current.push(output);
            return;
        }
        // TerminalStreamManager.write() itself detects a full-pane replacement snapshot
        // (ANSI_SNAPSHOT_PREFIX) and clears the buffer for it — see setOnFullSnapshot in
        // getOrCreateStreamManager above.
        const manager = getOrCreateStreamManager();
        if (manager) {
            manager.write(output);
        }
    }, [
        getOrCreateStreamManager,
        echoGuard,
        xtermRef
    ]);
    // Unified WebSocket streaming (effectiveSessionId computed near the top of
    // this component -- see usePooledTerminal's call site comment for why).
    // Stable callbacks
    const getTerminal = (0,react.useCallback)(()=>{
        var _xtermRef_current;
        return ((_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal) || null;
    }, [
        xtermRef
    ]);
    const handleStreamError = (0,react.useCallback)((err)=>{
        console.error("Terminal stream error (".concat(isExternal ? 'external' : 'managed', "):"), err);
        setConnectionAttempts((prev)=>prev + 1);
        // A stream error (e.g. a malformed/aborted response while the initial
        // content fetch was in flight) doesn't necessarily flip isConnected —
        // Connect-Web's underlying WebSocket transport can stay nominally
        // connected even though this specific RPC stream errored out. Without
        // this, neither the wasConnected-&&-!isConnected disconnect handler nor
        // the connectionAttempts>=5 fallback (both further down this file) ever
        // fires for a lone error like this, leaving the user staring at an
        // infinite "Loading terminal content..." spinner with no path to the
        // reconnect banner/retry button. Clear it here too, same as the
        // disconnect branch does, so a single early error still surfaces a
        // recoverable state instead of a permanent stall.
        setIsLoadingInitialContent(false);
    }, [
        isExternal
    ]);
    // Story 2.3 — InputDropBadge: dropped-keystroke count + a monotonic
    // per-episode sequence number (so two consecutive episodes with an
    // identical count still produce a distinct announcement, see
    // InputDropBadge.tsx's `episodeSeq` doc comment).
    const [dropEpisode, setDropEpisode] = (0,react.useState)({
        count: 0,
        seq: 0
    });
    const handleDropEpisodeFlush = (0,react.useCallback)((count)=>{
        if (count <= 0) return; // design/ux.md §3.3 — defensive no-op
        setDropEpisode((prev)=>({
                count,
                seq: prev.seq + 1
            }));
    }, []);
    const reportDroppedInput = useDropEpisodeCoalescer(handleDropEpisodeFlush, DROP_EPISODE_COALESCE_WINDOW_MS);
    // Task 2.3.5 — e2e test-only trigger. Reproducing a genuine WebSocket
    // reconnect race (the real trigger for onInputDropped) inside Playwright
    // proved impractical for this pass (it would require deterministically
    // racing a server-side connection teardown against a client keystroke),
    // so tests/e2e/input-drop-badge.spec.ts exercises the badge's rendering/
    // announcement/dismiss behavior via this harmless, additive test seam
    // instead of the full reconnect path (Stories 2.1/2.2 already have direct
    // Jest coverage of the drop mechanism itself). Gated on NODE_ENV (matching
    // the existing dev-only-hook convention in WebVitalsReporter.tsx /
    // rpcTiming.ts) so this unauthenticated, script-callable surface is never
    // attached in a production bundle — Next.js statically inlines
    // `process.env.NODE_ENV` at build time, so the production build tree-shakes
    // this block out entirely rather than merely no-op'ing it at runtime.
    (0,react.useEffect)(()=>{
        if (true) return;
        window.__e2eTriggerInputDropped = reportDroppedInput;
        return ()=>{
            delete window.__e2eTriggerInputDropped;
        };
    }, [
        reportDroppedInput
    ]);
    const { isConnected, error, sendInput, isInputChunking, resize, connect, disconnect, scrollbackLoaded, requestScrollback, sendFlowControl, startRecording, stopRecording, terminalState, isHardFailed, handleManualReconnect: handleHookReconnect, requestFullResync, markResyncComplete, markPaneResponseReceived, connectionCount } = useTerminalStream({
        baseUrl,
        sessionId: effectiveSessionId,
        shellId,
        onShellStatusChange,
        getTerminal,
        scrollbackLines: 1000,
        autoConnect: false,
        onError: handleStreamError,
        onScrollbackReceived: handleScrollbackReceived,
        onOutput: handleOutput,
        onAppScrollback: handleAppScrollbackReceived,
        onAltScreenActiveHint: handleAltScreenActiveHint,
        initialCols: (_lastResizeRef_current = lastResizeRef.current) === null || _lastResizeRef_current === void 0 ? void 0 : _lastResizeRef_current.cols,
        initialRows: (_lastResizeRef_current1 = lastResizeRef.current) === null || _lastResizeRef_current1 === void 0 ? void 0 : _lastResizeRef_current1.rows,
        isExternal: isExternal,
        onInputDropped: reportDroppedInput,
        foreground: isVisible,
        outstandingResyncIdsRef,
        readOnly
    });
    // Lightweight, single-purpose RPC client for the worktree-missing hard-fail
    // banner's "Retry now" button — mirrors the pattern used throughout this
    // codebase (e.g. useLogViewer.ts) for a one-off unary call, rather than
    // pulling in the full useSessionService hook (which sets up its own
    // session-list watch stream and Redux wiring — too heavy to instantiate
    // per open terminal pane).
    const retrySessionClientRef = (0,react.useRef)(null);
    if (!retrySessionClientRef.current) {
        retrySessionClientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,connect_transport/* createConnectTransport */.w)({
            baseUrl
        }));
    }
    const reconnectViaHook = (0,react.useCallback)(()=>{
        bumpConnectionEpoch();
        handleHookReconnect();
    }, [
        bumpConnectionEpoch,
        handleHookReconnect
    ]);
    const [isRetryingSession, setIsRetryingSession] = (0,react.useState)(false);
    const handleRetryNow = (0,react.useCallback)(async ()=>{
        setIsRetryingSession(true);
        try {
            var _retrySessionClientRef_current;
            // Errors are surfaced to the user via the banner remaining visible
            // (a fresh connect attempt below will just fail again) — no separate
            // error UI needed for this action itself.
            await ((_retrySessionClientRef_current = retrySessionClientRef.current) === null || _retrySessionClientRef_current === void 0 ? void 0 : _retrySessionClientRef_current.retrySession({
                id: effectiveSessionId
            }));
        } finally{
            setIsRetryingSession(false);
            reconnectViaHook();
        }
    }, [
        effectiveSessionId,
        reconnectViaHook
    ]);
    const { notifyResyncOutputReceived, resetStallWatchdog } = useVisibilityResync({
        sessionId: effectiveSessionId,
        // TerminalOutputProps.isVisible is optional (external callers may omit
        // it entirely); useVisibilityResync's isVisible is required since its
        // visibility-scoped resync gating (Task 6.1.1.3) needs a concrete
        // boolean, not undefined. Default to false — matching the "external
        // caller opted out of visibility awareness" behavior other isVisible
        // call sites in this component already fall back to (e.g. line ~1785's
        // `isVisible !== false` treats undefined as visible for rendering, but
        // for resync-gating purposes treating "unknown" as "not visible" is the
        // safer default: it defers resync-on-focus rather than risking one
        // firing for a caller that never intended to opt in).
        isVisible: isVisible !== null && isVisible !== void 0 ? isVisible : false,
        isConnected,
        terminalState,
        connect,
        disconnect,
        requestFullResync,
        markResyncComplete,
        markPaneResponseReceived,
        setShowReconnectButton,
        setShowReconnectBanner,
        scheduleResync,
        outstandingResyncIdsRef
    });
    (0,react.useEffect)(()=>{
        notifyResyncOutputReceivedRef.current = notifyResyncOutputReceived;
    }, [
        notifyResyncOutputReceived
    ]);
    (0,react.useEffect)(()=>{
        resetStallWatchdogRef.current = resetStallWatchdog;
    }, [
        resetStallWatchdog
    ]);
    // Sync terminalState into a ref so handleOutput can read it without recreating the callback.
    // Also flushes queued output when transitioning from RESIZING to STABLE (Task 4.2.2).
    (0,react.useEffect)(()=>{
        const prevState = terminalStateRef.current;
        terminalStateRef.current = terminalState;
        if (prevState === 'RESIZING' && terminalState === 'STABLE') {
            const manager = streamManagerRef.current;
            if (manager) {
                const pending = pendingOutputDuringResizeRef.current.splice(0);
                // manager.write() itself detects and clears for a full-pane snapshot — see
                // setOnFullSnapshot in getOrCreateStreamManager above.
                for (const chunk of pending){
                    manager.write(chunk);
                }
                console.log("[TerminalOutput] Flushed ".concat(pending.length, " pending output chunks after resize quiescence"));
            }
        }
    }, [
        terminalState
    ]);
    // Remote browser log streaming for mobile debugging
    useBrowserLogStream({
        enabled: logStreamEnabled,
        sessionId,
        baseUrl
    });
    // Update sendFlowControl ref when available + sync it into TerminalStreamManager
    (0,react.useEffect)(()=>{
        sendFlowControlRef.current = sendFlowControl;
        if (streamManagerRef.current) {
            streamManagerRef.current.updateSendFlowControl((paused, watermark)=>{
                var _sendFlowControlRef_current;
                return (_sendFlowControlRef_current = sendFlowControlRef.current) === null || _sendFlowControlRef_current === void 0 ? void 0 : _sendFlowControlRef_current.call(sendFlowControlRef, paused, watermark);
            });
        }
    }, [
        sendFlowControl
    ]);
    // Cleanup on unmount
    (0,react.useEffect)(()=>{
        return ()=>{
            isMountedRef.current = false;
            if (pasteErrorTimerRef.current) {
                clearTimeout(pasteErrorTimerRef.current);
                pasteErrorTimerRef.current = null;
            }
            if (uploadErrorTimerRef.current) {
                clearTimeout(uploadErrorTimerRef.current);
                uploadErrorTimerRef.current = null;
            }
            if (uploadSuccessTimerRef.current) {
                clearTimeout(uploadSuccessTimerRef.current);
                uploadSuccessTimerRef.current = null;
            }
            if (sizeStabilityTimeoutRef.current) {
                clearTimeout(sizeStabilityTimeoutRef.current);
                sizeStabilityTimeoutRef.current = null;
            }
            if (bannerTimerRef.current) {
                clearTimeout(bannerTimerRef.current);
                bannerTimerRef.current = null;
            }
            // Cleanup TerminalStreamManager
            if (streamManagerRef.current) {
                streamManagerRef.current.cleanup();
                streamManagerRef.current = null;
            }
            disconnect();
        };
    }, [
        disconnect
    ]);
    // Cancel the debounced approval refresh timer on unmount to prevent post-unmount calls
    (0,react.useEffect)(()=>{
        return ()=>{
            if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
        };
    }, []);
    // TUI page-key estimate (declared early: every input path below can invalidate it).
    const [netPagesUp] = (0,react.useState)(createNetPagesUpTracker);
    // Delivers bytes to the session without touching the page-key estimate. Programmatic page
    // keys (drag, momentum, toolbar PgUp/PgDn, the jump button) use this directly; user input
    // goes through handleTerminalData, which also invalidates the estimate.
    const deliverTerminalData = (0,react.useCallback)((data)=>{
        sendInput(data);
        // Optimistic clear on Enter only — reduces false-positive flicker
        if (data === "\r") {
            clearForSession(sessionId);
        // clearForSession fires an eager refetch — skip the debounce timer for Enter
        // to avoid a second refetch 300ms later that would cause a flicker window
        } else {
            // Only debounce-refetch when there are pending approvals to update —
            // prevents RPC storm when holding keys across N open terminals
            if (pendingCount > 0) {
                if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
                refreshTimerRef.current = setTimeout(()=>void refreshApprovals(), 300);
            }
        }
    }, [
        sendInput,
        clearForSession,
        sessionId,
        refreshApprovals,
        pendingCount
    ]);
    // User keystrokes and pastes move the app's own position, so the estimate no longer holds.
    const lastReadOnlyHintRef = (0,react.useRef)(0);
    const handleTerminalData = (0,react.useCallback)((data)=>{
        if (readOnly) {
            const now = Date.now();
            if (now - lastReadOnlyHintRef.current >= READ_ONLY_HINT_INTERVAL_MS) {
                lastReadOnlyHintRef.current = now;
                announce("Read-only session", "polite", "terminal-read-only-hint");
            }
            return;
        }
        netPagesUp.invalidate("keystroke");
        deliverTerminalData(data);
    }, [
        netPagesUp,
        deliverTerminalData,
        readOnly,
        announce
    ]);
    // Scroll route shared by the toolbar PgUp/PgDn (and later the chip and jump button).
    // XtermTerminal reports the live mode upward only when it changes.
    const { override: scrollOverride, gestureScrollEnabled } = useScrollSettings();
    const [scrollMode, setScrollMode] = (0,react.useState)(INITIAL_SCROLL_MODE);
    const handleScrollModeChange = (0,react.useCallback)((next)=>{
        setScrollMode((prev)=>prev.bufferType === next.bufferType && prev.mouseTrackingMode === next.mouseTrackingMode ? prev : next);
    }, []);
    const effectiveScroll = useEffectiveScrollMode(scrollMode, scrollOverride, gestureScrollEnabled);
    // Scrolling chip / picker / full panel (ux.md S6). Rows come from the terminal's own resize report.
    const [scrollPanel, setScrollPanel] = (0,react.useState)(null);
    const [terminalRows, setTerminalRows] = (0,react.useState)(0);
    // The xterm instance is created inside XtermTerminal's effect; the first resize report reveals it.
    const [jumpTerminal, setJumpTerminal] = (0,react.useState)(null);
    const [gestureActive, setGestureActive] = (0,react.useState)(false);
    // Host-owned live region: the picker unmounts on select, so its own announcement would be lost.
    const [scrollAnnouncement, setScrollAnnouncement] = (0,react.useState)("");
    const scrollChipRef = (0,react.useRef)(null);
    const misrouteCue = useMisrouteCue();
    const scrollHint = useScrollHint();
    const notifyScrollStart = scrollHint.notifyScrollStart;
    const markScrollHintSeen = scrollHint.markSeenOnPanelOpen;
    const getTouchSurface = (0,react.useCallback)(()=>terminalContainerRef.current, []);
    const reportMisroute = misrouteCue.report;
    // Drag/momentum page keys keep the estimate current instead of invalidating it.
    const handlePageKeysSent = (0,react.useCallback)((direction, pages)=>{
        for(let i = 0; i < pages; i++){
            if (direction === "up") netPagesUp.pageUp();
            else netPagesUp.pageDown();
        }
    }, [
        netPagesUp
    ]);
    const scrollGestureProps = (0,react.useMemo)(()=>({
            scrollOverride,
            gestureScrollEnabled,
            connectionEpoch,
            onScrollGesture: reportMisroute,
            onScrollStart: notifyScrollStart,
            onPageKeysSent: handlePageKeysSent,
            onGestureActiveChange: setGestureActive,
            onProgrammaticData: deliverTerminalData,
            isInputBusy: isInputChunking,
            mouseModeOverride: mouseMode
        }), [
        mouseMode,
        scrollOverride,
        gestureScrollEnabled,
        connectionEpoch,
        reportMisroute,
        notifyScrollStart,
        handlePageKeysSent,
        deliverTerminalData,
        isInputChunking
    ]);
    const handleScrollOverrideChange = (0,react.useCallback)((value)=>{
        scrollSettings.setOverride(value);
        if (scrollPanel === "picker") {
            var _SCROLL_OPTIONS_find;
            var _SCROLL_OPTIONS_find_announce;
            const label = (_SCROLL_OPTIONS_find_announce = (_SCROLL_OPTIONS_find = SCROLL_OPTIONS.find((o)=>o.value === value)) === null || _SCROLL_OPTIONS_find === void 0 ? void 0 : _SCROLL_OPTIONS_find.announce) !== null && _SCROLL_OPTIONS_find_announce !== void 0 ? _SCROLL_OPTIONS_find_announce : value;
            setScrollAnnouncement("Scroll mode: ".concat(label));
        }
    }, [
        scrollPanel
    ]);
    const closeScrollPanel = (0,react.useCallback)(()=>setScrollPanel(null), []);
    const openScrollPicker = (0,react.useCallback)(()=>{
        markScrollHintSeen();
        setScrollPanel((p)=>p === "picker" ? null : "picker");
    }, [
        markScrollHintSeen
    ]);
    const toggleScrollFullPanel = (0,react.useCallback)(()=>{
        markScrollHintSeen();
        setScrollPanel((p)=>p === "full" ? null : "full");
    }, [
        markScrollHintSeen
    ]);
    const openScrollFullPanel = (0,react.useCallback)(()=>setScrollPanel("full"), []);
    // Approximate panel heights in terminal rows (picker: 3 radios + 2 buttons; full: adds switch and notes).
    // Only used to decide inline vs overlay, so a rough constant is enough.
    // terminalRows is re-measured after an inline panel shrinks the terminal, so the choice is latched when the
    // panel opens (reset on close); re-deciding from the shrunken count flip-flops. The latch only upgrades
    // inline -> overlay (e.g. the soft keyboard leaves too few rows); it never downgrades while open.
    const panelOverlayLatchRef = (0,react.useRef)(null);
    const panelRowsEstimate = scrollPanel === "full" ? SCROLL_FULL_PANEL_ROWS : SCROLL_PICKER_PANEL_ROWS;
    if (scrollPanel === null) panelOverlayLatchRef.current = null;
    else if (panelOverlayLatchRef.current === null) {
        panelOverlayLatchRef.current = shouldRenderPanelAsOverlay(terminalRows - panelRowsEstimate);
    } else if (!panelOverlayLatchRef.current && shouldRenderPanelAsOverlay(terminalRows)) {
        // An inline panel has already taken its rows out of terminalRows, so no estimate is subtracted.
        panelOverlayLatchRef.current = true;
    }
    var _panelOverlayLatchRef_current;
    const panelAsOverlay = (_panelOverlayLatchRef_current = panelOverlayLatchRef.current) !== null && _panelOverlayLatchRef_current !== void 0 ? _panelOverlayLatchRef_current : false;
    const panelMaxHeight = panelAsOverlay && terminalRows > MIN_ROWS_FOR_OVERLAYS && containerSize.height > 0 ? Math.round(containerSize.height * (terminalRows - MIN_ROWS_FOR_OVERLAYS) / terminalRows) : undefined;
    const lastEpochRef = (0,react.useRef)(connectionEpoch);
    (0,react.useEffect)(()=>{
        if (lastEpochRef.current === connectionEpoch) return;
        lastEpochRef.current = connectionEpoch;
        netPagesUp.invalidate("reconnect");
    }, [
        connectionEpoch,
        netPagesUp
    ]);
    // A buffer switch, mouse-mode change or override change reroutes scrolling; skip the initial mount.
    const modeKey = "".concat(scrollMode.bufferType, "|").concat(scrollMode.mouseTrackingMode, "|").concat(scrollOverride);
    const lastModeKeyRef = (0,react.useRef)(modeKey);
    (0,react.useEffect)(()=>{
        if (lastModeKeyRef.current === modeKey) return;
        lastModeKeyRef.current = modeKey;
        netPagesUp.invalidate("mode-change");
    }, [
        modeKey,
        netPagesUp
    ]);
    // Send a key sequence, applying any active sticky modifier (CTRL or ALT) first.
    // Modifier sequences follow xterm's parameter convention:
    //   modifier 3 = Alt (escape prefix or CSI param ;3)
    //   modifier 5 = Ctrl (CSI param ;5)
    const sendKey = (0,react.useCallback)(function(keyData) {
        let programmatic = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : false;
        let data = keyData;
        if (ctrlActive) {
            var _CTRL_KEY_MAP_keyData;
            data = (_CTRL_KEY_MAP_keyData = CTRL_KEY_MAP[keyData]) !== null && _CTRL_KEY_MAP_keyData !== void 0 ? _CTRL_KEY_MAP_keyData : keyData;
            setCtrlActive(false);
        } else if (altActive) {
            var _ALT_KEY_MAP_keyData;
            data = (_ALT_KEY_MAP_keyData = ALT_KEY_MAP[keyData]) !== null && _ALT_KEY_MAP_keyData !== void 0 ? _ALT_KEY_MAP_keyData : '\x1b' + keyData;
            setAltActive(false);
        } else if (shiftActive) {
            var _SHIFT_KEY_MAP_keyData;
            data = (_SHIFT_KEY_MAP_keyData = SHIFT_KEY_MAP[keyData]) !== null && _SHIFT_KEY_MAP_keyData !== void 0 ? _SHIFT_KEY_MAP_keyData : keyData;
            setShiftActive(false);
        }
        (programmatic ? deliverTerminalData : handleTerminalData)(data);
    }, [
        ctrlActive,
        altActive,
        shiftActive,
        handleTerminalData,
        deliverTerminalData
    ]);
    // Route-aware PgUp/PgDn (ux.md S4): the same route as the drag, so the keys are an
    // equivalent single-pointer alternative. TUI-route bytes are unchanged from before.
    const sendToolbarPageKey = (0,react.useCallback)((direction)=>{
        var _xtermRef_current, _terminal_buffer;
        mobileDebug/* mobileDebug */.Uo.toolbarKey(direction === "up" ? "PageUp" : "PageDown");
        var _xtermRef_current_terminal;
        const terminal = (_xtermRef_current_terminal = (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal) !== null && _xtermRef_current_terminal !== void 0 ? _xtermRef_current_terminal : null;
        const buffer = terminal === null || terminal === void 0 ? void 0 : (_terminal_buffer = terminal.buffer) === null || _terminal_buffer === void 0 ? void 0 : _terminal_buffer.active;
        const canScroll = !!buffer && (direction === "up" ? buffer.viewportY > 0 : buffer.viewportY < buffer.baseY);
        const action = (0,scrollRouting/* toolbarPageAction */.d4)({
            route: effectiveScroll.target,
            direction,
            modifiers: {
                ctrl: ctrlActive,
                alt: altActive,
                shift: shiftActive
            },
            canScroll,
            override: scrollOverride
        });
        if (action.type === "scroll-pages" && terminal) {
            terminal.scrollPages(action.pages);
            return;
        }
        // A chunked paste is still streaming: page-key bytes would splice into it, so drop the tap (as the drag does).
        if (isInputChunking()) return;
        const bytes = action.type === "send-keys" ? action.bytes : direction === "up" ? scrollRouting/* PAGE_UP_BYTES */.UU : scrollRouting/* PAGE_DOWN_BYTES */.Nl;
        const countsTowardNetPages = action.type === "send-keys" && action.countsTowardNetPages;
        if (countsTowardNetPages) {
            if (direction === "up") netPagesUp.pageUp();
            else netPagesUp.pageDown();
        }
        sendKey(bytes, countsTowardNetPages);
    }, [
        effectiveScroll.target,
        scrollOverride,
        ctrlActive,
        altActive,
        shiftActive,
        sendKey,
        netPagesUp,
        isInputChunking
    ]);
    // Handle terminal resize with size stability detection
    const sizingRefs = {
        lastResizeRef,
        hasInitiatedConnectionRef,
        hasCachedDimensionsRef,
        sizeStabilityTimeoutRef,
        pendingConnectAfterDisconnectRef,
        isMountedRef,
        metricsRef,
        settleBypassPendingRef
    };
    const onTerminalResized = (0,react.useCallback)((_cols, rows)=>{
        var _xtermRef_current;
        setTerminalRows(rows);
        var _xtermRef_current_terminal;
        setJumpTerminal((_xtermRef_current_terminal = (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal) !== null && _xtermRef_current_terminal !== void 0 ? _xtermRef_current_terminal : null);
        netPagesUp.invalidate("resize");
    }, [
        netPagesUp,
        xtermRef
    ]);
    const handleTerminalResize = useTerminalResizeHandler({
        sessionId,
        xtermRef,
        isConnected,
        error,
        connect,
        resize,
        clearBufferBeforeResize,
        setIsWaitingForStableSize,
        onTerminalResized,
        refs: sizingRefs
    });
    // Monitor connection state changes
    (0,react.useEffect)(()=>{
        const wasConnected = previousConnectionStateRef.current;
        previousConnectionStateRef.current = isConnected;
        let postConnectionResizeTimer = null;
        if (!wasConnected && isConnected) {
            if (metricsRef.current.connectedTime === null) {
                metricsRef.current.connectedTime = performance.now();
            }
            console.log("[TerminalOutput] Connection established");
            setShowReconnectButton(false);
            setConnectionAttempts(0);
            // On reconnect (not first connect), append a separator so the user can see where
            // the reconnection happened. showReconnectBanner being true means we disconnected.
            if (hasEverConnectedRef.current && showReconnectBanner) {
                var _xtermRef_current;
                const terminal = (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal;
                if (terminal && terminal.buffer.active.length > 0) {
                    const manager = streamManagerRef.current;
                    if (manager) {
                        manager.write("\r\n\x1b[2m--- reconnected ---\x1b[0m\r\n");
                    }
                }
            }
            if (reconnectTimeoutRef.current) {
                clearTimeout(reconnectTimeoutRef.current);
                reconnectTimeoutRef.current = null;
            }
            // Delay the post-connection resize sync until after layout settles.
            // Firing immediately would start the 200ms throttle clock at T+0, causing
            // the ResizeObserver's settled-layout resize (arriving at T+150ms via its
            // own debounce) to be dropped. Waiting 250ms lets the container stabilise
            // first; by then the ResizeObserver has already sent the correct dims (or
            // nothing changed and we send here as a safety net).
            //
            // force=true (AC4): reconnect must re-sync the server's pane content even
            // when the settled dims exactly match the last value resize()'s RPC-level
            // dedup would otherwise skip the send — a dimension-unchanged reconnect
            // still needs the follow-up currentPaneRequest that only resize() sends.
            postConnectionResizeTimer = setTimeout(()=>{
                const settledSize = lastResizeRef.current;
                if (settledSize) {
                    console.log("[TerminalOutput] Post-connection resize sync (delayed, force=true): ".concat(settledSize.cols, "x").concat(settledSize.rows));
                    resize(settledSize.cols, settledSize.rows, true);
                }
            }, 250);
        } else if (wasConnected && !isConnected) {
            console.log("[TerminalOutput] Connection lost, will attempt reconnection");
            // New session: don't drop the spinner before first content arrives.
            if (isInitialScrollbackDoneRef.current) {
                setIsLoadingInitialContent(false);
            }
            if (TerminalOutput_process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") {
                reconnectTimeoutRef.current = setTimeout(()=>{
                    if (!isConnected) {
                        setShowReconnectButton(true);
                    }
                }, 5000);
            }
        }
        return ()=>{
            if (postConnectionResizeTimer) {
                clearTimeout(postConnectionResizeTimer);
            }
            if (reconnectTimeoutRef.current) {
                clearTimeout(reconnectTimeoutRef.current);
            }
        };
    // showReconnectBanner is a pre-existing missing dependency: adding it
    // would re-run this effect on every banner toggle, and its cleanup
    // clears reconnectTimeoutRef.current without nulling the ref -- that
    // would silently cancel the "show reconnect button after 5s" timer
    // mid-countdown. Left alone rather than fixed as a drive-by.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        isConnected,
        resize,
        xtermRef
    ]);
    // Story 3.2.1 — reconnecting banner: show after 2s of disconnection (if was ever connected)
    (0,react.useEffect)(()=>{
        if (isConnected) {
            hasEverConnectedRef.current = true;
            if (bannerTimerRef.current) {
                clearTimeout(bannerTimerRef.current);
                bannerTimerRef.current = null;
            }
            setShowReconnectBanner(false);
            return;
        }
        if (!hasEverConnectedRef.current) return; // never connected — don't show banner
        bannerTimerRef.current = setTimeout(()=>{
            if (!isConnected) {
                setShowReconnectBanner(true);
            }
        }, 2000);
        return ()=>{
            if (bannerTimerRef.current) {
                clearTimeout(bannerTimerRef.current);
                bannerTimerRef.current = null;
            }
        };
    }, [
        isConnected
    ]);
    // Clear loading overlay when max reconnect attempts reached
    (0,react.useEffect)(()=>{
        if (connectionAttempts >= 5) {
            setIsLoadingInitialContent(false);
        }
    }, [
        connectionAttempts
    ]);
    // Scroll to bottom whenever loading finishes (session open or switch)
    (0,react.useEffect)(()=>{
        var _xtermRef_current;
        if (!isLoadingInitialContent && ((_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal)) {
            const terminal = xtermRef.current.terminal;
            requestAnimationFrame(()=>{
                terminal.scrollToBottom();
                setTimeout(()=>terminal.scrollToBottom(), 100);
            });
        }
    }, [
        isLoadingInitialContent,
        xtermRef
    ]);
    // Task 1.4.5b — every requestScrollback call site (the tmux-native trigger
    // below and Story 1.4.0's alt-screen wheel/touch triggers) funnels through
    // this one wrapper so the loading pill can't drift out of sync with which
    // trigger actually fired.
    const requestScrollbackWithPill = (0,react.useCallback)((fromSequence, limit)=>{
        startScrollLoadingTimers(altScreenActiveRef.current ? lastKnownAppProgramRef.current : undefined);
        requestScrollback(fromSequence, limit);
    }, [
        requestScrollback,
        startScrollLoadingTimers
    ]);
    // Story 1.4.0 (Task 1.4.0c) — shared guard-and-call body for the alt-screen
    // wheel (Task 1.4.0b) and touch (via useTerminalGestures) triggers, so the
    // two paths can't drift apart. For an alt-screen session the "more
    // available" guard is hasMoreAppScrollbackRef (Task 1.4.4b) — this feature's
    // own flag — never hasMoreScrollbackRef, which a pure alt-screen session's
    // tmux-native path never touches and so would never flip false.
    const triggerAltScreenScrollUp = (0,react.useCallback)(()=>{
        if (isFetchingScrollbackRef.current || !hasMoreAppScrollbackRef.current || !isConnected) {
            return;
        }
        isFetchingScrollbackRef.current = true;
        requestScrollbackWithPill(oldestSequenceReceivedRef.current, 500);
    }, [
        isConnected,
        requestScrollbackWithPill
    ]);
    // Stable accessor for useTerminalGestures's isAltScreenActive option
    // (Task 1.4.0c) — reads the same ref the wheel listener above holds.
    const isAltScreenActive = (0,react.useCallback)(()=>altScreenActiveRef.current, []);
    // useTerminalGestures calls this with a line count (Task 1.4.0c); the
    // trigger itself doesn't scale by drag magnitude, matching the wheel
    // trigger's deltaY-direction-only behavior.
    const onAltScreenScrollUp = (0,react.useCallback)(()=>triggerAltScreenScrollUp(), [
        triggerAltScreenScrollUp
    ]);
    // Story 3 (Task 3.4) — part 2 of usePooledTerminal (see that hook's call
    // site near the top of this component and its own doc comment for why
    // this is a separate call, made here once all four callbacks it needs
    // exist).
    (0,TerminalPool/* usePooledTerminalCallbacks */.RW)(effectiveSessionId, isVisible !== false, {
        onData: handleTerminalData,
        onResize: handleTerminalResize,
        isAltScreenActive,
        onAltScreenScrollUp,
        scrollGesture: scrollGestureProps,
        onScrollModeChange: handleScrollModeChange
    });
    // Read-only view: xterm itself stops emitting keystrokes. handleTerminalData and the flow-control
    // hook drop anything that still gets through, and the server drops it again (Story 5.1).
    // The pooled xterm mounts lazily, so this re-checks on every render (a cheap property compare).
    (0,react.useEffect)(()=>{
        var _xtermRef_current_terminal, _xtermRef_current;
        const options = (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : (_xtermRef_current_terminal = _xtermRef_current.terminal) === null || _xtermRef_current_terminal === void 0 ? void 0 : _xtermRef_current_terminal.options;
        if (options && options.disableStdin !== readOnly) options.disableStdin = readOnly;
    });
    // Task 2.3.1 — DOM scroll listener to detect near-top-of-buffer and trigger paged history load.
    // Uses DOM 'scroll' event on terminal.element (NOT terminal.onScroll, which only fires on buffer
    // writes, not user scroll gestures — see xterm.js issues #3201, #3864).
    // Task 1.4.0b — a `wheel` listener lives in this same effect: alt-screen
    // sessions (the case this whole feature exists for) never move viewportY,
    // so the 'scroll' listener above never fires for them (research/architecture.md §3).
    (0,react.useEffect)(()=>{
        var _xtermRef_current, _terminal_element;
        if (isLoadingInitialContent) return; // Don't attach until initial content is loaded
        const terminal = (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal;
        if (!(terminal === null || terminal === void 0 ? void 0 : terminal.element)) return;
        var _terminal_element_querySelector;
        // xterm.js scrolls the .xterm-viewport child element, not the root terminal.element —
        // attaching the listener to terminal.element would never fire on scroll (Bug 5 fix).
        const scrollEl = (_terminal_element_querySelector = (_terminal_element = terminal.element) === null || _terminal_element === void 0 ? void 0 : _terminal_element.querySelector('.xterm-viewport')) !== null && _terminal_element_querySelector !== void 0 ? _terminal_element_querySelector : terminal.element;
        const onScroll = ()=>{
            const viewportY = terminal.buffer.active.viewportY;
            if (viewportY < 200 && !isFetchingScrollbackRef.current && hasMoreScrollbackRef.current && isConnected) {
                isFetchingScrollbackRef.current = true;
                console.log("[TerminalOutput] Near top of buffer (viewportY=".concat(viewportY, "), requesting older scrollback from seq ").concat(oldestSequenceReceivedRef.current));
                requestScrollbackWithPill(oldestSequenceReceivedRef.current, 500);
            }
        };
        // Task 1.4.0b — when altScreenActive, viewportY-gated 'scroll' never fires
        // (or is meaningless), so a wheel scroll-up gesture is the trigger instead.
        // No conflict with existing handlers: nothing else registers a 'wheel'
        // listener on this element (confirmed by grep across XtermTerminal.tsx).
        const onWheel = (e)=>{
            const deltaY = e.deltaY;
            if (altScreenActiveRef.current && deltaY < 0) {
                triggerAltScreenScrollUp();
            }
        };
        scrollEl === null || scrollEl === void 0 ? void 0 : scrollEl.addEventListener('scroll', onScroll, {
            passive: true
        });
        scrollEl === null || scrollEl === void 0 ? void 0 : scrollEl.addEventListener('wheel', onWheel, {
            passive: true
        });
        return ()=>{
            scrollEl === null || scrollEl === void 0 ? void 0 : scrollEl.removeEventListener('scroll', onScroll);
            scrollEl === null || scrollEl === void 0 ? void 0 : scrollEl.removeEventListener('wheel', onWheel);
        };
    }, [
        isLoadingInitialContent,
        isConnected,
        requestScrollbackWithPill,
        triggerAltScreenScrollUp,
        xtermRef
    ]);
    // Auto-reconnect with exponential backoff
    (0,react.useEffect)(()=>{
        if (TerminalOutput_process.env.NEXT_PUBLIC_RECONNECT_V2 === "true") return; // hook-level reconnect handles it
        // Must not fire once the hook has hard-failed (e.g. a non-retriable WS
        // close code): the hook already set shouldReconnectRef.current = false
        // in that case, but calling connect() below unconditionally resets it
        // back to true (see useTerminalStream.ts's connect()), silently
        // reconnecting right through the hard-fail and hiding the Retry banner.
        if (isHardFailed) return;
        if (!isConnected && error && connectionAttempts > 0 && connectionAttempts < 5) {
            const backoffDelay = Math.min(1000 * Math.pow(2, connectionAttempts - 1), 10000);
            console.log("[TerminalOutput] Auto-reconnecting in ".concat(backoffDelay, "ms (attempt ").concat(connectionAttempts, ")"));
            const timeout = setTimeout(()=>{
                console.log("[TerminalOutput] Attempting reconnection...");
                bumpConnectionEpoch();
                connect();
            }, backoffDelay);
            return ()=>clearTimeout(timeout);
        }
    }, [
        isConnected,
        error,
        connectionAttempts,
        connect,
        isHardFailed,
        bumpConnectionEpoch
    ]);
    // Starts the connection for a session: pre-size from cache, then session-switch
    // connect, then post-disconnect connect. The effects inside run in that fixed
    // order -- see the ORDERING INVARIANT in useSessionConnectionBootstrap.
    useSessionConnectionBootstrap({
        sessionId,
        containerWidth: containerSize.width,
        terminalContainerRef,
        xtermRef,
        isConnected,
        connect,
        setIsWaitingForStableSize,
        setIsLoadingInitialContent,
        setConnectionAttempts,
        setShowReconnectButton,
        reconnectTimeoutRef,
        previousConnectionStateRef,
        streamManagerRef,
        refs: sizingRefs
    });
    // When terminal becomes visible (e.g. session switch in pool), refit through the sampler + focus.
    // Story 3 (Task 3.5) — usePooledTerminal's docking effect also fits+focuses on dock; this is a
    // harmless second attempt (both idempotent).
    (0,react.useEffect)(()=>{
        if (isVisible && xtermRef.current) {
            var _xtermRef_current_terminal;
            xtermRef.current.refit({
                reason: 'visibility'
            });
            (_xtermRef_current_terminal = xtermRef.current.terminal) === null || _xtermRef_current_terminal === void 0 ? void 0 : _xtermRef_current_terminal.focus();
        }
    }, [
        isVisible,
        xtermRef
    ]);
    // On-screen keyboard / URL bar: refit once visualViewport height and offsetTop stop changing.
    // Sole viewport-driven fit path; the sampler in XtermTerminal owns the actual fit
    // (ADR-002: a direct fit() off visualViewport raced that sampler, so we only call refit()).
    (0,react.useEffect)(()=>{
        return createViewportSettle(window.visualViewport, createRafScheduler(), {
            stableFrames: DEFAULT_STABLE_FRAMES,
            maxWaitMs: DEFAULT_MAX_WAIT_MS,
            onSettled: (snapshot)=>{
                mobileDebug/* mobileDebug */.Uo.log("vp-settle", snapshot);
                netPagesUp.invalidate("resize");
                const xterm = xtermRef.current;
                if (!xterm) return; // no refit issued, so no bypass to arm
                settleBypassPendingRef.current = true;
                xterm.refit({
                    reason: 'viewport-settle',
                    // The fit ended without a size change (or exhausted retries): drop the unused bypass.
                    onFitted: ()=>{
                        settleBypassPendingRef.current = false;
                    }
                });
            }
        });
    }, [
        netPagesUp
    ]);
    const handleManualReconnect = (0,react.useCallback)(()=>{
        console.log("[TerminalOutput] Manual reconnect requested");
        setConnectionAttempts(0);
        setShowReconnectButton(false);
        bumpConnectionEpoch();
        connect();
    }, [
        connect,
        bumpConnectionEpoch
    ]);
    const handleToggleDebug = (0,react.useCallback)(()=>{
        const newDebugMode = !debugMode;
        setDebugMode(newDebugMode);
        if (true) {
            if (newDebugMode) {
                localStorage.setItem("debug-terminal", "true");
                console.log("%c[TerminalOutput] Debug mode ENABLED", "color: #00ff00; font-weight: bold");
                console.log("All terminal refresh and write operations will be logged");
            } else {
                localStorage.removeItem("debug-terminal");
                console.log("%c[TerminalOutput] Debug mode DISABLED", "color: #ff0000; font-weight: bold");
            }
        }
    }, [
        debugMode
    ]);
    const handleToggleLogStream = (0,react.useCallback)(()=>{
        const next = !logStreamEnabled;
        setLogStreamEnabled(next);
        if (true) {
            if (next) {
                localStorage.setItem("stapler-squad-remote-debug", "true");
            } else {
                localStorage.removeItem("stapler-squad-remote-debug");
            }
        }
    }, [
        logStreamEnabled
    ]);
    const handleCopyOutput = ()=>{
        var _xtermRef_current_terminal, _xtermRef_current;
        const selectedText = (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : (_xtermRef_current_terminal = _xtermRef_current.terminal) === null || _xtermRef_current_terminal === void 0 ? void 0 : _xtermRef_current_terminal.getSelection();
        if (selectedText) {
            navigator.clipboard.writeText(selectedText).catch(()=>{
                document.execCommand('copy');
            });
        }
    };
    // Read from clipboard and inject into the terminal.
    // Text is sent directly; images are uploaded to the server and the resulting
    // file path is inserted so the terminal process (e.g. Claude Code) can read
    // the image from disk via normal file path conventions.
    const handlePaste = (0,react.useCallback)(async ()=>{
        try {
            var _navigator_clipboard;
            // navigator.clipboard.read() supports both text and image items
            if ((_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.read) {
                const items = await navigator.clipboard.read();
                for (const item of items){
                    const imageType = item.types.find((t)=>t.startsWith('image/'));
                    if (imageType) {
                        const blob = await item.getType(imageType);
                        // Convert to base64 (strip the "data:image/...;base64," prefix)
                        const base64 = await new Promise((resolve, reject)=>{
                            const reader = new FileReader();
                            reader.onload = ()=>resolve(reader.result.split(',')[1]);
                            reader.onerror = reject;
                            reader.readAsDataURL(blob);
                        });
                        const uploadUrl = "".concat(baseUrl, "/upload/file");
                        const resp = await fetch(uploadUrl, {
                            method: 'POST',
                            headers: {
                                'Content-Type': 'application/json'
                            },
                            body: JSON.stringify({
                                data: base64,
                                contentType: imageType
                            })
                        });
                        if (resp.ok) {
                            const { path } = await resp.json();
                            handleTerminalData(path);
                        } else {
                            const msg = await resp.text().catch(()=>"HTTP ".concat(resp.status));
                            console.warn('[TerminalOutput] Image upload failed:', msg);
                            if (pasteErrorTimerRef.current) clearTimeout(pasteErrorTimerRef.current);
                            setPasteError('Upload failed');
                            pasteErrorTimerRef.current = setTimeout(()=>setPasteError(null), 2500);
                        }
                        return;
                    }
                    if (item.types.includes('text/plain')) {
                        const blob = await item.getType('text/plain');
                        const text = await blob.text();
                        if (text) handleTerminalData(text);
                        return;
                    }
                }
            } else {
                // Fallback: text-only clipboard API
                const text = await navigator.clipboard.readText();
                if (text) handleTerminalData(text);
            }
        } catch (err) {
            console.warn('[TerminalOutput] Clipboard access failed:', err);
            if (pasteErrorTimerRef.current) clearTimeout(pasteErrorTimerRef.current);
            setPasteError('Clipboard access denied');
            pasteErrorTimerRef.current = setTimeout(()=>setPasteError(null), 2500);
        }
    }, [
        handleTerminalData,
        baseUrl
    ]);
    const handleScrollToBottom = ()=>{
        var _xtermRef_current;
        if ((_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal) {
            xtermRef.current.terminal.scrollToBottom();
        }
    };
    // Upload a batch of files sequentially, inserting each path into the terminal.
    // Shared by the file-picker inputs and drag-and-drop.
    const uploadFileList = (0,react.useCallback)(async (fileArray)=>{
        const total = fileArray.length;
        setUploadingCount(total);
        setUploadError(null);
        setUploadSuccess(null);
        let successCount = 0;
        let lastUploadedPath = "";
        for(let i = 0; i < total; i++){
            const file = fileArray[i];
            // Update count as we progress through the batch.
            setUploadingCount(total - i);
            try {
                const formData = new FormData();
                formData.append("session_id", sessionId);
                formData.append("file", file);
                // Do NOT set Content-Type header — browser sets it with multipart boundary.
                const resp = await fetch("".concat(baseUrl, "/v1/upload-image"), {
                    method: "POST",
                    body: formData
                });
                if (resp.ok) {
                    const data = await resp.json();
                    // Always attempt to insert — sendInput silently no-ops when disconnected.
                    handleTerminalData(data.path + " ");
                    successCount++;
                    lastUploadedPath = data.path;
                } else {
                    let msg = "Upload failed";
                    if (resp.status === 413) msg = "File too large (max 10 MB)";
                    else if (resp.status === 404) msg = "Session not found";
                    setUploadError(msg);
                    if (uploadErrorTimerRef.current) clearTimeout(uploadErrorTimerRef.current);
                    uploadErrorTimerRef.current = setTimeout(()=>setUploadError(null), 3000);
                    break;
                }
            } catch (e) {
                setUploadError("Network error");
                if (uploadErrorTimerRef.current) clearTimeout(uploadErrorTimerRef.current);
                uploadErrorTimerRef.current = setTimeout(()=>setUploadError(null), 3000);
                break;
            }
        }
        setUploadingCount(0);
        if (successCount > 0) {
            // If terminal is disconnected, the path silently dropped — copy it to clipboard
            // and show the filename so the user can see what was uploaded.
            if (!isConnected && lastUploadedPath) {
                var _navigator_clipboard;
                (_navigator_clipboard = navigator.clipboard) === null || _navigator_clipboard === void 0 ? void 0 : _navigator_clipboard.writeText(lastUploadedPath).catch(()=>{});
            }
            var _lastUploadedPath_split_pop;
            const label = successCount === 1 && lastUploadedPath ? (_lastUploadedPath_split_pop = lastUploadedPath.split("/").pop()) !== null && _lastUploadedPath_split_pop !== void 0 ? _lastUploadedPath_split_pop : "file" : "".concat(successCount, " files");
            const msg = isConnected ? "✓ ".concat(label, " inserted") : "✓ ".concat(label, " saved — path copied");
            setUploadSuccess(msg);
            if (uploadSuccessTimerRef.current) clearTimeout(uploadSuccessTimerRef.current);
            uploadSuccessTimerRef.current = setTimeout(()=>setUploadSuccess(null), 4000);
        }
    }, [
        sessionId,
        baseUrl,
        handleTerminalData,
        isConnected
    ]);
    const handleFilesUpload = (0,react.useCallback)((e)=>{
        const fileList = e.target.files;
        if (!fileList || fileList.length === 0) return;
        // Extract File objects before resetting the input — iOS Safari invalidates FileList
        // items after e.target.value = "" because it uses lazy references.
        const fileArray = Array.from(fileList);
        // Reset input so the same file(s) can trigger onChange again.
        e.target.value = "";
        void uploadFileList(fileArray);
    }, [
        uploadFileList
    ]);
    // Drag-and-drop of OS files onto the terminal. Desktop-only and not for read-only sessions.
    const canAcceptDrop = !readOnly && hasFinePointer;
    const { isDragOver, dropHandlers } = useFileDrop(canAcceptDrop, uploadFileList);
    // Synchronous click handlers — MUST NOT have await before .click() (iOS Safari requirement).
    const handleCameraButtonClick = (0,react.useCallback)(()=>{
        var _cameraInputRef_current;
        (_cameraInputRef_current = cameraInputRef.current) === null || _cameraInputRef_current === void 0 ? void 0 : _cameraInputRef_current.click();
    }, []);
    const handleGalleryButtonClick = (0,react.useCallback)(()=>{
        var _galleryInputRef_current;
        (_galleryInputRef_current = galleryInputRef.current) === null || _galleryInputRef_current === void 0 ? void 0 : _galleryInputRef_current.click();
    }, []);
    const handleFilesButtonClick = (0,react.useCallback)(()=>{
        var _filesInputRef_current;
        (_filesInputRef_current = filesInputRef.current) === null || _filesInputRef_current === void 0 ? void 0 : _filesInputRef_current.click();
    }, []);
    const handleClear = ()=>{
        if (xtermRef.current && xtermRef.current.terminal) {
            const terminal = xtermRef.current.terminal;
            const startTime = performance.now();
            refreshCountRef.current++;
            console.log('[TerminalOutput] Clear requested', {
                refreshCount: refreshCountRef.current,
                bufferSize: terminal.buffer.active.length,
                rows: terminal.rows,
                cols: terminal.cols,
                scrollbackSize: terminal.buffer.normal.length
            });
            xtermRef.current.clear();
            const clearTime = performance.now();
            terminal.refresh(0, terminal.rows - 1);
            const refreshTime = performance.now();
            terminal.write('\x1b[H');
            const cursorResetTime = performance.now();
            console.log('[TerminalOutput] Clear completed with forced refresh', {
                refreshCount: refreshCountRef.current,
                clearDuration: "".concat((clearTime - startTime).toFixed(2), "ms"),
                refreshDuration: "".concat((refreshTime - clearTime).toFixed(2), "ms"),
                cursorResetDuration: "".concat((cursorResetTime - refreshTime).toFixed(2), "ms"),
                totalDuration: "".concat((cursorResetTime - startTime).toFixed(2), "ms"),
                refreshedRows: "0-".concat(terminal.rows - 1),
                viewport: {
                    rows: terminal.rows,
                    cols: terminal.cols,
                    scrollTop: terminal.buffer.active.viewportY
                }
            });
        }
    };
    // The fit completes asynchronously (sampler, rAF), so the forced resize message is sent from
    // onFitted with the post-fit dims. The latest connection state is read when it fires, not when
    // the button was tapped.
    const manualResizeLatestRef = (0,react.useRef)({
        isConnected,
        resize,
        clearBufferBeforeResize
    });
    (0,react.useEffect)(()=>{
        manualResizeLatestRef.current = {
            isConnected,
            resize,
            clearBufferBeforeResize
        };
    });
    const handleManualResize = ()=>{
        var _xtermRef_current;
        console.log("[TerminalOutput] Manual resize triggered");
        (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.refit({
            reason: 'manual-resize',
            onFitted: (param)=>{
                let { cols, rows, stale } = param;
                const latest = manualResizeLatestRef.current;
                // Forcing a resize (which also clears the local buffer) with pre-hide dims would redraw at a size the container no longer has.
                if (stale) {
                    console.log("[TerminalOutput] Manual resize skipped: container never became visible (stale ".concat(cols, "x").concat(rows, ")"));
                    return;
                }
                console.log("[TerminalOutput] Terminal resized to ".concat(cols, "x").concat(rows));
                if (!isMountedRef.current || !latest.isConnected) return;
                console.log("[TerminalOutput] Forcing resize message to backend: ".concat(cols, "x").concat(rows));
                lastResizeRef.current = {
                    cols,
                    rows
                };
                latest.clearBufferBeforeResize();
                latest.resize(cols, rows, true);
            }
        });
    };
    // Epic 4.2, Story 4.2.2 (Task 4.2.2b) — best-effort resize-mismatch signal
    // for ConnectionCountIndicator's tooltip. No wire field carries "did this
    // tab's ResizeVote win the hub's negotiation" (out of this UI-only epic's
    // scope), so this compares what this tab last asked to resize to
    // (lastResizeRef, set in handleTerminalResize) against xterm's actual
    // applied dimensions — if they differ, another connection's vote is
    // constraining this tab's pane size. Never shown speculatively: both refs
    // must have resolved values before a mismatch is reported (UX-AC-10).
    const appliedTerminal = (_xtermRef_current = xtermRef.current) === null || _xtermRef_current === void 0 ? void 0 : _xtermRef_current.terminal;
    const hasResizeSizeMismatch = !!(lastResizeRef.current && appliedTerminal && (appliedTerminal.cols !== lastResizeRef.current.cols || appliedTerminal.rows !== lastResizeRef.current.rows));
    const allSecondaryActions = [
        {
            key: 'copy',
            icon: '📋',
            label: 'Copy',
            ariaLabel: 'Copy terminal output to clipboard',
            title: 'Copy selected terminal text to clipboard',
            extraClass: '',
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "copy"
                    }
                });
                handleCopyOutput();
            }
        },
        {
            key: 'paste',
            icon: '📎',
            label: 'Paste',
            ariaLabel: 'Paste from clipboard',
            title: 'Paste from clipboard',
            extraClass: undefined,
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "paste"
                    }
                });
                void handlePaste();
            }
        },
        {
            key: 'bottom',
            icon: '↓',
            label: 'Bottom',
            ariaLabel: 'Scroll to bottom',
            title: 'Scroll to bottom',
            extraClass: '',
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "bottom"
                    }
                });
                handleScrollToBottom();
            }
        },
        {
            key: 'clear',
            icon: '🗑️',
            label: 'Clear',
            ariaLabel: 'Clear terminal',
            title: 'Clear terminal',
            extraClass: '',
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "clear"
                    }
                });
                handleClear();
            }
        },
        {
            key: 'scrolling',
            icon: '↕️',
            label: 'Scrolling',
            ariaLabel: 'Scrolling settings',
            title: 'How dragging scrolls, and touch gestures',
            extraClass: '',
            handler: toggleScrollFullPanel
        },
        {
            key: 'mouse',
            icon: '🖱️',
            label: mouseMode === 'none' ? 'Mouse' : 'Mouse ON',
            ariaLabel: mouseMode === 'none' ? 'Enable mouse mode for terminal apps (vim, tmux)' : 'Disable mouse mode — enables text selection',
            title: mouseMode === 'none' ? 'Mouse OFF — tap to enable for vim/tmux' : 'Mouse ON — tap to disable, enables selection',
            extraClass: mouseMode === 'any' ? mouseModeActive : '',
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "mouse",
                        state: mouseMode === 'none' ? 'on' : 'off'
                    }
                });
                toggleMouseMode();
            }
        }
    ];
    const secondaryActions = readOnly ? allSecondaryActions.filter((a)=>a.key !== 'paste') : allSecondaryActions;
    const isConnectingState = terminalState === "CONNECTING" || terminalState === "LOADING";
    // Rendered in both the desktop toolbar and the mobile overflow row; the hidden file <input>s stay in toolbarActions. The overflow row stays open on click so upload status on the Gallery label remains visible.
    const renderUploadButtons = ()=>readOnly ? null : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: toolbarButton,
                    onClick: ()=>{
                        track({
                            name: "toolbar_button_click",
                            category: "user_action",
                            sessionId,
                            component: "TerminalOutput",
                            labels: {
                                button: "gallery"
                            }
                        });
                        handleGalleryButtonClick();
                    },
                    disabled: uploadingCount > 0,
                    title: "Attach image(s) from gallery — multi-select supported",
                    "aria-label": uploadingCount > 0 ? "Uploading ".concat(uploadingCount, " file(s)...") : "Attach images from gallery",
                    children: uploadSuccess ? "✅ ".concat(uploadSuccess) : uploadError ? "⚠️ ".concat(uploadError) : uploadingCount > 0 ? "⏳ ".concat(uploadingCount, "…") : "🖼️ Gallery"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: toolbarButton,
                    onClick: ()=>{
                        track({
                            name: "toolbar_button_click",
                            category: "user_action",
                            sessionId,
                            component: "TerminalOutput",
                            labels: {
                                button: "files"
                            }
                        });
                        handleFilesButtonClick();
                    },
                    disabled: uploadingCount > 0,
                    title: "Attach any file(s) — multi-select, all types accepted",
                    "aria-label": uploadingCount > 0 ? "Uploading ".concat(uploadingCount, " file(s)...") : "Attach files",
                    children: uploadingCount > 0 ? "⏳ ".concat(uploadingCount, "…") : "📁 Files"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: "".concat(toolbarButton, " ").concat(mobileOnlyUpload),
                    onClick: ()=>{
                        track({
                            name: "toolbar_button_click",
                            category: "user_action",
                            sessionId,
                            component: "TerminalOutput",
                            labels: {
                                button: "camera"
                            }
                        });
                        handleCameraButtonClick();
                    },
                    disabled: uploadingCount > 0,
                    title: "Take a photo — opens camera directly",
                    "aria-label": uploadingCount > 0 ? "Uploading ".concat(uploadingCount, " file(s)...") : "Take photo with camera",
                    children: uploadingCount > 0 ? "⏳ ".concat(uploadingCount, "…") : "📷"
                })
            ]
        });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: toolbar,
                ref: toolbarRef,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: TerminalOutput_css_status,
                        children: [
                            isExternal && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: externalLabel,
                                title: "External session via ssq-mux",
                                children: "\uD83D\uDD17 External"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: "".concat(statusIndicator, " ").concat(isConnected ? connected : isWaitingForStableSize || isConnectingState ? stabilizing : disconnected)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: statusText,
                                children: isConnected ? "Connected" : isWaitingForStableSize ? "Initializing..." : isConnectingState ? "Connecting..." : "Disconnected"
                            }),
                            !isConnected && connectionAttempts > 0 && connectionAttempts < 5 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: statusText,
                                children: [
                                    " ",
                                    "• Reconnecting (attempt ",
                                    connectionAttempts,
                                    "/5)..."
                                ]
                            }),
                            !isConnected && connectionAttempts >= 5 && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: errorText,
                                children: " • Terminal unavailable"
                            }),
                            isHardFailed && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: errorText,
                                children: " • Terminal unavailable"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(ConnectionCountIndicator, {
                                count: connectionCount,
                                sizeMismatch: hasResizeSizeMismatch,
                                pulse: connectionPulse
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: actions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: toolbarToggle,
                                onClick: ()=>setToolbarExpanded((v)=>!v),
                                "aria-label": "Toggle toolbar",
                                "aria-expanded": toolbarExpanded,
                                "data-testid": "toolbar-toggle",
                                children: toolbarExpanded ? '✕' : '⋯'
                            }),
                            !readOnly && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: "".concat(toolbarButton, " ").concat(mobileKeyboardToggle),
                                onClick: toggleMobileKeyboard,
                                "aria-label": isKeyboardVisible ? "Hide mobile keyboard" : "Show mobile keyboard",
                                "aria-expanded": isKeyboardVisible,
                                title: isKeyboardVisible ? "Hide mobile keyboard" : "Show mobile keyboard",
                                children: "⌨️"
                            }),
                            showReconnectButton && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: toolbarButton,
                                onClick: handleManualReconnect,
                                title: "Reconnect to terminal",
                                "aria-label": "Reconnect to terminal",
                                children: "\uD83D\uDD04 Reconnect"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: toolbarButton,
                                onClick: ()=>{
                                    track({
                                        name: "toolbar_button_click",
                                        category: "user_action",
                                        sessionId,
                                        component: "TerminalOutput",
                                        labels: {
                                            button: "resize"
                                        }
                                    });
                                    handleManualResize();
                                },
                                "aria-label": "Redraw terminal (fixes a blank screen)",
                                title: "Redraw terminal (fixes a blank screen)",
                                children: [
                                    "↔️",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: toolbarButtonLabel,
                                        children: " Redraw"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(ScrollModeChip, {
                                effectiveTarget: effectiveScroll.target,
                                gestureScrollEnabled: gestureScrollEnabled,
                                onClick: openScrollPicker,
                                visibleRows: terminalRows,
                                highlighted: misrouteCue.highlighted,
                                announcement: misrouteCue.announcement,
                                suppressAnnouncements: scrollPanel !== null,
                                getTouchSurface: getTouchSurface,
                                buttonRef: scrollChipRef
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                role: "status",
                                "aria-live": "polite",
                                className: LiveRegion_css/* srOnly */.s,
                                "data-testid": "scroll-mode-announcer",
                                children: scrollAnnouncement
                            }),
                            toolbarExpanded && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: toolbarActions,
                                "data-testid": "toolbar-actions",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: secondaryGroup,
                                        "data-testid": "toolbar-secondary",
                                        children: secondaryActions.map((action)=>// action.handler (defined in secondaryActions above) always calls track() itself
                                            // analytics-exempt
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                className: "".concat(toolbarButton).concat(action.extraClass ? " ".concat(action.extraClass) : ''),
                                                onClick: action.handler,
                                                "aria-label": action.ariaLabel,
                                                title: action.title,
                                                children: [
                                                    action.icon,
                                                    " ",
                                                    action.label
                                                ]
                                            }, action.key))
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        ref: cameraInputRef,
                                        type: "file",
                                        accept: "image/*",
                                        capture: "environment",
                                        style: {
                                            display: "none"
                                        },
                                        onChange: handleFilesUpload,
                                        "aria-hidden": "true"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        ref: galleryInputRef,
                                        type: "file",
                                        accept: "image/*",
                                        multiple: true,
                                        style: {
                                            display: "none"
                                        },
                                        onChange: handleFilesUpload,
                                        "aria-hidden": "true"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        ref: filesInputRef,
                                        type: "file",
                                        accept: "*/*",
                                        multiple: true,
                                        style: {
                                            display: "none"
                                        },
                                        onChange: handleFilesUpload,
                                        "aria-hidden": "true"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: desktopOnlyUploads,
                                        children: renderUploadButtons()
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: toolbarButton,
                                        onClick: ()=>{
                                            track({
                                                name: "toolbar_button_click",
                                                category: "user_action",
                                                sessionId,
                                                component: "TerminalOutput",
                                                labels: {
                                                    button: "output-filter",
                                                    state: filterOpen ? "closed" : "open"
                                                }
                                            });
                                            setFilterOpen((open)=>!open);
                                        },
                                        "aria-label": filterOpen ? "Close output filter" : "Filter terminal output",
                                        "aria-pressed": filterOpen,
                                        "data-testid": "toolbar-filter-toggle",
                                        title: "Filter terminal output (display only)",
                                        children: "\uD83D\uDD0E"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        ref: devToggleRef,
                                        className: "".concat(toolbarButton, " ").concat(devOnly),
                                        onClick: handleDevGroupToggle,
                                        "aria-label": devGroupOpen ? "Collapse developer tools" : "Expand developer tools",
                                        "aria-expanded": devGroupOpen,
                                        "aria-controls": "toolbar-dev-group-inner",
                                        "data-testid": "toolbar-dev-toggle",
                                        title: "Developer tools: Debug, Log Stream, Record, Raw",
                                        children: [
                                            "⚙",
                                            devGroupOpen ? ' ▴' : ' ▾'
                                        ]
                                    }),
                                    devGroupOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: devGroupPanel,
                                        "data-testid": "toolbar-dev-group",
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            id: "toolbar-dev-group-inner",
                                            role: "group",
                                            "aria-label": "Developer tools",
                                            className: devGroup,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                    className: "".concat(toolbarButton, " ").concat(devOnly, " ").concat(debugMode ? debugActive : ''),
                                                    onClick: ()=>{
                                                        track({
                                                            name: "toolbar_button_click",
                                                            category: "user_action",
                                                            sessionId,
                                                            component: "TerminalOutput",
                                                            labels: {
                                                                button: "debug",
                                                                state: debugMode ? "off" : "on"
                                                            }
                                                        });
                                                        handleToggleDebug();
                                                    },
                                                    title: debugMode ? "Disable debug logging" : "Enable debug logging",
                                                    "aria-label": debugMode ? "Disable debug mode" : "Enable debug mode",
                                                    style: debugMode ? {
                                                        backgroundColor: '#2a4',
                                                        color: 'white',
                                                        fontWeight: 'bold'
                                                    } : {},
                                                    children: [
                                                        "\uD83D\uDEE0️ ",
                                                        debugMode ? 'Debug ON' : 'Debug'
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                    className: "".concat(toolbarButton, " ").concat(devOnly, " ").concat(logStreamEnabled ? debugActive : ''),
                                                    onClick: ()=>{
                                                        track({
                                                            name: "toolbar_button_click",
                                                            category: "user_action",
                                                            sessionId,
                                                            component: "TerminalOutput",
                                                            labels: {
                                                                button: "log-stream",
                                                                state: logStreamEnabled ? "off" : "on"
                                                            }
                                                        });
                                                        handleToggleLogStream();
                                                    },
                                                    title: logStreamEnabled ? "Stop forwarding verbose per-frame debug traces to server (info/warn/error always stream)" : "Also forward verbose per-frame debug traces to server (info/warn/error always stream automatically)",
                                                    "aria-label": logStreamEnabled ? "Disable verbose debug trace streaming" : "Enable verbose debug trace streaming",
                                                    style: logStreamEnabled ? {
                                                        backgroundColor: '#2a4',
                                                        color: 'white',
                                                        fontWeight: 'bold'
                                                    } : {},
                                                    children: [
                                                        "\uD83D\uDCE1 ",
                                                        logStreamEnabled ? 'Debug Traces ON' : 'Debug Traces'
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: "".concat(toolbarButton, " ").concat(devOnly),
                                                    onClick: ()=>{
                                                        track({
                                                            name: "toolbar_button_click",
                                                            category: "user_action",
                                                            sessionId,
                                                            component: "TerminalOutput",
                                                            labels: {
                                                                button: "record",
                                                                state: isRecording ? "off" : "on"
                                                            }
                                                        });
                                                        if (isRecording) {
                                                            stopRecording();
                                                            setIsRecording(false);
                                                        } else {
                                                            startRecording();
                                                            setIsRecording(true);
                                                        }
                                                    },
                                                    title: isRecording ? "Stop recording" : "Start recording terminal output",
                                                    "aria-label": isRecording ? "Stop recording terminal output" : "Start recording terminal output",
                                                    style: isRecording ? {
                                                        backgroundColor: '#ff4444',
                                                        color: 'white'
                                                    } : {},
                                                    children: isRecording ? '⏹️ Stop Rec' : '⏺️ Record'
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: "".concat(toolbarButton, " ").concat(devOnly),
                                                    onClick: ()=>{
                                                        track({
                                                            name: "toolbar_button_click",
                                                            category: "user_action",
                                                            sessionId,
                                                            component: "TerminalOutput",
                                                            labels: {
                                                                button: "handedness",
                                                                state: leftHanded ? "right" : "left"
                                                            }
                                                        });
                                                        toggleHandedness();
                                                    },
                                                    "aria-label": leftHanded ? "Switch to right-handed layout" : "Switch to left-handed layout",
                                                    "aria-pressed": leftHanded,
                                                    title: "Toggle left/right-handed mobile layout",
                                                    children: leftHanded ? "✋ Left" : "🤚 Right"
                                                })
                                            ]
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: "".concat(toolbarButton, " ").concat(mobileMoreButton, " ").concat(mobileOverflowOpen ? mobileMoreActive : ''),
                                        onClick: ()=>setMobileOverflowOpen((o)=>!o),
                                        "aria-label": mobileOverflowOpen ? 'Close more tools' : 'More tools',
                                        "aria-expanded": mobileOverflowOpen,
                                        "data-testid": "toolbar-more-button",
                                        children: mobileOverflowOpen ? '✕ Less' : 'More ▾'
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            mobileOverflowOpen && toolbarExpanded && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: mobileOverflowRow,
                "data-testid": "toolbar-overflow-row",
                ref: overflowRowRef,
                children: [
                    secondaryActions.map((action)=>// action.handler (defined in secondaryActions above) always calls track() itself
                        // analytics-exempt
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                            className: "".concat(toolbarButton).concat(action.extraClass ? " ".concat(action.extraClass) : ''),
                            onClick: ()=>{
                                action.handler();
                                setMobileOverflowOpen(false);
                            },
                            "aria-label": action.ariaLabel,
                            title: action.title,
                            children: [
                                action.icon,
                                " ",
                                action.label
                            ]
                        }, action.key)),
                    renderUploadButtons()
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(ScrollHint, {
                visible: scrollHint.visible,
                route: scrollHint.route,
                onDismiss: scrollHint.dismiss
            }),
            scrollPanel && /*#__PURE__*/ (0,jsx_runtime.jsx)(ScrollingPanel, {
                variant: scrollPanel,
                override: scrollOverride,
                onOverrideChange: handleScrollOverrideChange,
                effectiveTarget: effectiveScroll.target,
                gestureScrollEnabled: gestureScrollEnabled,
                onGestureScrollChange: scrollSettings.setGestureScroll,
                onClose: closeScrollPanel,
                onOpenFull: openScrollFullPanel,
                openerRef: scrollChipRef,
                renderAsOverlay: panelAsOverlay,
                maxHeight: panelMaxHeight
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: terminal,
                ref: terminalContainerRef,
                ...dropHandlers,
                "data-testid": "terminal-drop-zone",
                children: [
                    isDragOver && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: dropZoneOverlay,
                        "aria-hidden": "true",
                        "data-testid": "drop-zone-overlay",
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: dropZoneLabel,
                            children: "Drop to attach"
                        })
                    }),
                    filterOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)(TerminalOutputFilter, {
                        getLines: getFilterLines,
                        onClose: ()=>setFilterOpen(false)
                    }),
                    showReconnectBanner && !isHardFailed && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: reconnectingBanner,
                        role: "status",
                        "aria-live": "polite",
                        "aria-label": "Reconnecting",
                        children: "Reconnecting terminal…"
                    }),
                    showReconnectBanner && isHardFailed && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: hardFailedBanner,
                        role: "alert",
                        children: (0,backoff/* isWorktreeMissingError */.GN)(error) ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                "This session's working directory no longer exists (its git worktree may have been deleted).",
                                " ",
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: handleRetryNow,
                                    disabled: isRetryingSession,
                                    children: isRetryingSession ? "Retrying…" : "Retry now"
                                })
                            ]
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                "Connection lost — ",
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: reconnectViaHook,
                                    children: "Retry"
                                })
                            ]
                        })
                    }),
                    isVisible !== false && isLoadingInitialContent && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: loadingOverlay,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: loadingSpinner
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: loadingText,
                                children: isWaitingForStableSize ? "Initializing terminal..." : !isConnected && !isInitialScrollbackDoneRef.current ? "Starting session..." : "Loading terminal content..."
                            })
                        ]
                    }),
                    !isLoadingInitialContent && connectionAttempts >= 5 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: unavailableOverlay,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: unavailableIcon,
                                children: "⚠"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: unavailableText,
                                children: "Terminal unavailable"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: unavailableSubtext,
                                children: "Could not connect to terminal session"
                            })
                        ]
                    }),
                    terminalState === 'RESIZING' && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: resizingOverlay,
                        role: "status",
                        "aria-label": "Terminal resizing",
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: resizingSpinner
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ScrollLoadingPill, {
                        visible: pillState.visible,
                        stalled: pillState.stalled,
                        program: pillState.program,
                        onCancel: handleCancelScrollLoading
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ScrollSourceIndicator, {
                        program: appScrollbackState.program,
                        visible: appScrollbackState.active
                    }),
                    atTopVisible && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: noMoreHistoryLine,
                        role: "status",
                        "aria-live": "polite",
                        "data-testid": "no-more-app-history",
                        children: "No more history available"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        ref: dockRef,
                        className: TerminalPool_css/* dockAnchor */.ZL
                    }),
                    (jumpTerminal === null || jumpTerminal === void 0 ? void 0 : (_jumpTerminal_buffer = jumpTerminal.buffer) === null || _jumpTerminal_buffer === void 0 ? void 0 : _jumpTerminal_buffer.active) && /*#__PURE__*/ (0,jsx_runtime.jsx)(JumpToLatestMount, {
                        terminal: jumpTerminal,
                        route: effectiveScroll.target,
                        netPagesUp: netPagesUp,
                        connectionEpoch: connectionEpoch,
                        sendData: deliverTerminalData,
                        isInputBusy: isInputChunking,
                        gestureActive: gestureActive,
                        getContainer: getTouchSurface
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(InputDropBadge, {
                count: dropEpisode.count,
                episodeSeq: dropEpisode.seq
            }),
            blockedToast && typeof document !== "undefined" && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: TerminalOutput_css_blockedToast,
                role: "status",
                "aria-live": "polite",
                "data-testid": "scroll-blocked-toast",
                "data-blocked-reason": events_pb/* ScrollBlockedReason */.ec[blockedToast.reason],
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        children: [
                            "⚠ ",
                            blockedToastCopy(blockedToast.reason, blockedToast.program)
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: blockedToastButton,
                        onClick: dismissBlockedToast,
                        "data-testid": "scroll-blocked-toast-dismiss",
                        children: "Got it"
                    })
                ]
            }, blockedToast.seq), document.body),
            isKeyboardVisible && !readOnly && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: mobileKeyboard,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: mobileKeyRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b');
                                },
                                "aria-label": "Escape",
                                "data-testid": "mobile-key",
                                children: "Esc"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('/');
                                },
                                "aria-label": "Forward slash",
                                "data-testid": "mobile-key",
                                children: "/"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('-');
                                },
                                "aria-label": "Hyphen",
                                "data-testid": "mobile-key",
                                children: "-"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[H');
                                },
                                "aria-label": "Home",
                                "data-testid": "mobile-key",
                                children: "Home"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[A');
                                },
                                "aria-label": "Up arrow",
                                "data-testid": "mobile-key",
                                children: "↑"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[F');
                                },
                                "aria-label": "End",
                                "data-testid": "mobile-key",
                                children: "End"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendToolbarPageKey("up");
                                },
                                "aria-label": "Page up",
                                "data-testid": "mobile-key",
                                children: "PgUp"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: mobileKeyRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\t');
                                },
                                "aria-label": "Tab",
                                "data-testid": "mobile-key",
                                children: "Tab"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: "".concat(mobileKey, " ").concat(shiftActive ? mobileKeyActive : ''),
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setShiftActive((p)=>!p);
                                    setCtrlActive(false);
                                    setAltActive(false);
                                },
                                "aria-label": shiftActive ? 'Shift active — press next key' : 'Shift modifier',
                                "aria-pressed": shiftActive,
                                "data-testid": "mobile-key",
                                children: "Shift"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: "".concat(mobileKey, " ").concat(ctrlActive ? mobileKeyActive : ''),
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive((p)=>!p);
                                    setAltActive(false);
                                    setShiftActive(false);
                                },
                                "aria-label": ctrlActive ? 'Ctrl active — press next key' : 'Control modifier',
                                "aria-pressed": ctrlActive,
                                "data-testid": "mobile-key",
                                children: "Ctrl"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: "".concat(mobileKey, " ").concat(altActive ? mobileKeyActive : ''),
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setAltActive((p)=>!p);
                                    setCtrlActive(false);
                                    setShiftActive(false);
                                },
                                "aria-label": altActive ? 'Alt active — press next key' : 'Alt modifier',
                                "aria-pressed": altActive,
                                "data-testid": "mobile-key",
                                children: "Alt"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[D');
                                },
                                "aria-label": "Left arrow",
                                "data-testid": "mobile-key",
                                children: "←"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[B');
                                },
                                "aria-label": "Down arrow",
                                "data-testid": "mobile-key",
                                children: "↓"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[C');
                                },
                                "aria-label": "Right arrow",
                                "data-testid": "mobile-key",
                                children: "→"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendToolbarPageKey("down");
                                },
                                "aria-label": "Page down",
                                "data-testid": "mobile-key",
                                children: "PgDn"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: mobileKeyRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: "".concat(mobileKey, " ").concat(mobileKeyCtrlC),
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x03');
                                },
                                "aria-label": "Ctrl+C (interrupt)",
                                title: "Interrupt (Ctrl+C)",
                                "data-testid": "mobile-key",
                                children: "^C"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x1a');
                                },
                                "aria-label": "Ctrl+Z (suspend)",
                                title: "Suspend (Ctrl+Z)",
                                "data-testid": "mobile-key",
                                children: "^Z"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x0c');
                                },
                                "aria-label": "Ctrl+L (clear screen)",
                                title: "Clear screen (Ctrl+L)",
                                "data-testid": "mobile-key",
                                children: "^L"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x12');
                                },
                                "aria-label": "Ctrl+R (reverse search)",
                                title: "Reverse search (Ctrl+R)",
                                "data-testid": "mobile-key",
                                children: "^R"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x17');
                                },
                                "aria-label": "Ctrl+W (delete word)",
                                title: "Delete word (Ctrl+W)",
                                "data-testid": "mobile-key",
                                children: "^W"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x15');
                                },
                                "aria-label": "Ctrl+U (delete line)",
                                title: "Delete line (Ctrl+U)",
                                "data-testid": "mobile-key",
                                children: "^U"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x04');
                                },
                                "aria-label": "Ctrl+D (EOF)",
                                title: "EOF / logout (Ctrl+D)",
                                "data-testid": "mobile-key",
                                children: "^D"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 40230:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   H$: () => (/* binding */ decideScrollTarget),
/* harmony export */   Md: () => (/* binding */ PAGE_FLING_CAP),
/* harmony export */   Mu: () => (/* binding */ PAGE_RATE_LIMIT_MS),
/* harmony export */   Nl: () => (/* binding */ PAGE_DOWN_BYTES),
/* harmony export */   Ql: () => (/* binding */ decideAndLogScrollTarget),
/* harmony export */   UU: () => (/* binding */ PAGE_UP_BYTES),
/* harmony export */   YA: () => (/* binding */ ROUTING_VERIFIED),
/* harmony export */   d4: () => (/* binding */ toolbarPageAction),
/* harmony export */   gU: () => (/* binding */ encodeWheel),
/* harmony export */   qC: () => (/* binding */ encodePageKeys),
/* harmony export */   yC: () => (/* binding */ PageAccumulator)
/* harmony export */ });
/* unused harmony exports TUI_SCROLL_POLICY, DEFAULT_ROUTING_POLICY, MAX_WHEEL_REPORTS_PER_FRAME, encodeWheelX10, tuiPageStepLines */
/**
 * Scroll routing for touch drags: decides where a scroll step is delivered
 * (xterm local scrollback vs. TUI via wheel reports or PgUp/PgDn) and encodes
 * the bytes. Pure and deterministic; clocks and loggers are injected.
 */ /** Flips to true once the Phase 0 spike confirms the default rows. */ const ROUTING_VERIFIED = false;
const TUI_SCROLL_POLICY = "pgkeys";
const TRACKING_ON = [
    "x10",
    "vt200",
    "drag",
    "any"
];
const DEFAULT_ROUTING_POLICY = {
    rules: [
        {
            when: {
                bufferType: "alternate"
            },
            target: "tui-pgkeys"
        },
        ...TRACKING_ON.map((mouseTrackingMode)=>({
                when: {
                    mouseTrackingMode
                },
                target: "tui-pgkeys"
            }))
    ],
    default: "xterm-local"
};
function ruleMatches(when, mode) {
    return (when.bufferType === undefined || when.bufferType === mode.bufferType) && (when.mouseTrackingMode === undefined || when.mouseTrackingMode === mode.mouseTrackingMode);
}
/** First matching row wins; `tui-wheel` is only reachable when tuiPolicy is 'wheel'. */ function decideScrollTarget(mode) {
    let policy = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : DEFAULT_ROUTING_POLICY, tuiPolicy = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : TUI_SCROLL_POLICY, override = arguments.length > 3 && arguments[3] !== void 0 ? arguments[3] : "auto";
    const tuiTarget = tuiPolicy === "wheel" ? "tui-wheel" : "tui-pgkeys";
    if (override === "local") return "xterm-local";
    if (override === "tui") return tuiTarget;
    const row = policy.rules.find((r)=>ruleMatches(r.when, mode));
    const target = row ? row.target : policy.default;
    return target === "xterm-local" ? target : tuiTarget;
}
/** decideScrollTarget plus a log entry carrying `unverified:true` while routing is unverified. */ function decideAndLogScrollTarget(mode) {
    let opts = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : {};
    var _opts_log;
    var _opts_override;
    const override = (_opts_override = opts.override) !== null && _opts_override !== void 0 ? _opts_override : "auto";
    const target = decideScrollTarget(mode, opts.policy, opts.tuiPolicy, override);
    var _opts_routingVerified;
    const verified = (_opts_routingVerified = opts.routingVerified) !== null && _opts_routingVerified !== void 0 ? _opts_routingVerified : ROUTING_VERIFIED;
    const entry = {
        target,
        source: override === "auto" ? "auto" : "override",
        mode,
        ...verified ? {} : {
            unverified: true
        }
    };
    (_opts_log = opts.log) === null || _opts_log === void 0 ? void 0 : _opts_log.call(opts, entry);
    return target;
}
// ---- Encoders ----
const MAX_WHEEL_REPORTS_PER_FRAME = 3;
function clamp(v, lo, hi) {
    return Math.min(Math.max(v, lo), hi);
}
/**
 * SGR wheel reports. lineDelta < 0 = toward older output (wheel up, button 64),
 * > 0 = toward newer (button 65). Emits at most MAX_WHEEL_REPORTS_PER_FRAME reports.
 */ function encodeWheel(lineDelta, cell) {
    const count = Math.min(Math.abs(Math.trunc(lineDelta)), MAX_WHEEL_REPORTS_PER_FRAME);
    if (count === 0) return "";
    const col = clamp(Math.trunc(cell.col), 1, Math.max(1, cell.cols));
    const row = clamp(Math.trunc(cell.row), 1, Math.max(1, cell.rows));
    const button = lineDelta < 0 ? 64 : 65;
    return "\x1b[<".concat(button, ";").concat(col, ";").concat(row, "M").repeat(count);
}
/** X10 fallback; retained for the spike but deliberately not selectable via decideScrollTarget. */ function encodeWheelX10(lineDelta, cell) {
    const count = Math.min(Math.abs(Math.trunc(lineDelta)), MAX_WHEEL_REPORTS_PER_FRAME);
    if (count === 0) return "";
    const col = clamp(Math.trunc(cell.col), 1, Math.max(1, cell.cols));
    const row = clamp(Math.trunc(cell.row), 1, Math.max(1, cell.rows));
    const button = lineDelta < 0 ? 64 : 65;
    // X10 encodes each value as a single byte offset by 32 (limited to 223 cells).
    const ch = (n)=>String.fromCharCode(clamp(n, 0, 223) + 32);
    return "\x1b[M".concat(ch(button)).concat(ch(col)).concat(ch(row)).repeat(count);
}
const PAGE_UP_BYTES = "\x1b[5~";
const PAGE_DOWN_BYTES = "\x1b[6~";
/** direction 'older' = PgUp, 'newer' = PgDn (same bytes as the toolbar keys). */ function encodePageKeys(direction) {
    let count = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : 1;
    return (direction === "older" ? PAGE_UP_BYTES : PAGE_DOWN_BYTES).repeat(Math.max(0, count));
}
// ---- Page accumulator ----
const PAGE_RATE_LIMIT_MS = 100;
const PAGE_FLING_CAP = 5;
/** Post-slop finger travel (lines) that emits one page key: half a page. */ function tuiPageStepLines(rows) {
    return Math.max(1, Math.floor((rows - 1) / 2));
}
/**
 * Accumulates line travel into page keys. Positive lines = newer (PgDn),
 * negative = older (PgUp). Emits at most one key per call (frame), carries the
 * remainder, rate-limits to one per rateLimitMs and caps keys per fling.
 */ class PageAccumulator {
    setRows(rows) {
        this.rows = rows;
    }
    /** True once the per-fling cap is hit; callers cancel momentum. */ get capped() {
        return this.emitted >= this.flingCap;
    }
    /** Returns the page-key bytes for this frame ("" when none). */ push(lines) {
        this.carry += lines;
        const step = tuiPageStepLines(this.rows);
        if (Math.abs(this.carry) < step) return "";
        if (this.capped) {
            this.carry = 0;
            return "";
        }
        const t = this.now();
        if (t - this.lastEmitAt < this.rateLimitMs) return "";
        const older = this.carry < 0;
        this.carry += older ? step : -step;
        this.emitted += 1;
        this.lastEmitAt = t;
        return encodePageKeys(older ? "older" : "newer");
    }
    /** Reset at gesture start / mode flip. */ reset() {
        this.carry = 0;
        this.emitted = 0;
        this.lastEmitAt = Number.NEGATIVE_INFINITY;
    }
    constructor(opts){
        this.carry = 0;
        this.emitted = 0;
        this.lastEmitAt = Number.NEGATIVE_INFINITY;
        this.rows = opts.rows;
        this.now = opts.now;
        var _opts_rateLimitMs;
        this.rateLimitMs = (_opts_rateLimitMs = opts.rateLimitMs) !== null && _opts_rateLimitMs !== void 0 ? _opts_rateLimitMs : PAGE_RATE_LIMIT_MS;
        var _opts_flingCap;
        this.flingCap = (_opts_flingCap = opts.flingCap) !== null && _opts_flingCap !== void 0 ? _opts_flingCap : PAGE_FLING_CAP;
    }
}
/**
 * Same route as the drag, so the toolbar is an equivalent single-pointer
 * alternative. A modifier always sends bytes; in `auto`, a local route at its
 * edge falls through to bytes (today's behaviour for a normal-buffer TUI the
 * table misclassifies as local); an explicit `local` override never falls through.
 */ function toolbarPageAction(input) {
    const { route, direction, modifiers, canScroll, override } = input;
    const bytes = direction === "up" ? PAGE_UP_BYTES : PAGE_DOWN_BYTES;
    const modified = modifiers.ctrl || modifiers.alt || modifiers.shift;
    const isTui = route !== "xterm-local";
    if (modified) return {
        type: "send-keys",
        bytes,
        countsTowardNetPages: false
    };
    if (isTui) return {
        type: "send-keys",
        bytes,
        countsTowardNetPages: true
    };
    if (!canScroll && override !== "local") return {
        type: "send-keys",
        bytes,
        countsTowardNetPages: false
    };
    return {
        type: "scroll-pages",
        pages: direction === "up" ? -1 : 1
    };
}


/***/ }),

/***/ 48399:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   JQ: () => (/* binding */ DEFAULT_TERMINAL_POOL_MAX_SIZE),
/* harmony export */   L: () => (/* binding */ usePooledTerminal),
/* harmony export */   RW: () => (/* binding */ usePooledTerminalCallbacks),
/* harmony export */   oo: () => (/* binding */ TerminalPoolProvider)
/* harmony export */ });
/* unused harmony exports useTerminalPoolDockedIds, useTerminalPool */
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var react_dom__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(80883);
/* harmony import */ var _TerminalPool_css__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(13630);
/* __next_internal_client_entry_do_not_use__ DEFAULT_TERMINAL_POOL_MAX_SIZE,useTerminalPoolDockedIds,TerminalPoolProvider,useTerminalPool,usePooledTerminal,usePooledTerminalCallbacks auto */ 
/**
 * Terminal Jank Elimination, Story 3 (Task 3.1) — Terminal Instance Pool.
 *
 * Keeps up to `maxSize` xterm.js `Terminal` instances (via `XtermTerminal`)
 * alive across a `TerminalOutput` remount, so switching which session a pane
 * shows no longer tears down and recreates the terminal's DOM/WebGL context.
 *
 * ## Why this exists (the actual jank source)
 *
 * `PaneSplitRenderer.tsx` keys its `<SessionDetail>` on the pane id AND the
 * assigned session id (see that file's `SessionDetail` render call) --
 * switching which session a pane displays fully unmounts and remounts the
 * whole `SessionDetail` -> `SessionDetailView` -> `TerminalOutput` subtree,
 * including `TerminalOutput`'s own local `<XtermTerminal>` instance.
 * `SessionDetailView`'s `pooledSessionIds` (kept-alive `TerminalOutput`
 * instances via CSS `visibility`) only helps for *within-view* tab switches;
 * it can't help across a pane's session reassignment because the whole view
 * — pool state included — is destroyed with it. `TerminalPoolProvider` lives
 * above that remount boundary (in `app/Providers.tsx`), so it survives it.
 *
 * ## Scope decision (see PR body for the full writeup)
 *
 * The doc's Task 3.1 sketch has each pool entry own a `useTerminalStream`
 * connection (WebSocket) in addition to the xterm.js `Terminal`. This
 * implementation deliberately does NOT move the WebSocket connection,
 * `TerminalStreamManager`, or `TerminalOutput`'s resync/flow-control/
 * scrollback-paging state into the pool — `TerminalOutput.tsx` is ~2500
 * lines of tightly-coupled state built on those living in one component
 * instance's lifetime, and re-plumbing all of it was judged too high-risk
 * to do safely in one pass. Instead, the pool owns exactly the thing whose
 * teardown/recreation is actually visible and expensive: the xterm.js
 * `Terminal` + its WebGL context + its DOM. `TerminalOutput` still owns a
 * fresh `useTerminalStream` connection per mount (fast, thanks to Story 2's
 * quiescence detector + snapshot cache) and simply reconnects it against the
 * SAME, already-populated Terminal instance -- old content stays visible
 * instantly instead of a blank terminal, and `TerminalStreamManager.write()`
 * already knows how to clear+replace on a full-pane snapshot (see
 * `ANSI_SNAPSHOT_PREFIX` in TerminalStreamManager.ts), which is exactly what
 * a fresh connect's initial snapshot is.
 *
 * ## Docking mechanism
 *
 * Each entry gets one persistent, detached host `<div>` created once
 * (`entry.hostEl`) holding a `createPortal(<XtermTerminal/>, entry.hostEl)`.
 * "Docking" a session to a consumer's anchor element is a plain
 * `anchor.appendChild(entry.hostEl)` -- moving an already-attached DOM node
 * via appendChild does not destroy/recreate it (no WebGL context loss, no
 * xterm.js re-init), unlike removing then reinserting. Undocked entries live
 * in a small always-attached "graveyard" container (`TerminalPool.css.ts`)
 * so xterm.js always has a real, laid-out container to measure. This avoids
 * both `position: fixed` + bounding-rect-tracking (what the doc's "portal
 * into document.body" sketch implies) and React's `createPortal` container
 * prop changing across renders (an unreliable way to reparent state --
 * `react-reverse-portal` exists specifically because of this gotcha).
 */ 


const XtermTerminal = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.lazy)(()=>Promise.all(/* import() */[__webpack_require__.e(1869), __webpack_require__.e(3576), __webpack_require__.e(151), __webpack_require__.e(3517)]).then(__webpack_require__.bind(__webpack_require__, 83517)).then((m)=>({
            default: m.XtermTerminal
        })));
const DEFAULT_TERMINAL_POOL_MAX_SIZE = 8;
// Pooled terminals keep their buffer alive across switches, so scrollback is
// meaningful -- always 5000 regardless of what any individual consumer might
// otherwise request (Task 3.3). Never overridden per-entry.
const POOLED_SCROLLBACK = 5000;
// Fit-on-show retry cap (Bug 2, see usePooledTerminal's docking effect) --
// bounds the poll to ~5s at 60fps before giving up and logging a warning,
// rather than polling forever if a pane genuinely never lays out.
const FIT_ON_SHOW_MAX_ATTEMPTS = 300;
const TerminalPoolContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
// dockedIds lives in its OWN context, separate from TerminalPoolContextValue
// above. Every dock()/undock() call changes it, and TerminalPoolContextValue
// is a dependency of usePooledTerminal's docking effect -- folding dockedIds
// into that same object previously meant every dock/undock changed `pool`'s
// identity, re-triggering that effect, which called dock/undock again: an
// infinite update loop (caught by this file's own tests -- "Maximum update
// depth exceeded"). Splitting it out keeps TerminalPoolContextValue's
// function identities stable across dock/undock, while still letting a
// caller that genuinely wants introspection (tests, debugging) subscribe to
// dockedIds without destabilizing anything.
const DockedIdsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(new Set());
/** sessionIds currently docked (visible) -- for tests/introspection; not needed for normal pool usage (see DockedIdsContext's doc comment for why it's separate from useTerminalPool()). */ function useTerminalPoolDockedIds() {
    return useContext(DockedIdsContext);
}
function createEntry(sessionId) {
    const hostEl = document.createElement("div");
    hostEl.className = _TerminalPool_css__WEBPACK_IMPORTED_MODULE_3__/* .host */ .Hc;
    hostEl.dataset.pooledSessionId = sessionId;
    const onDataRef = {
        current: null
    };
    const onResizeRef = {
        current: null
    };
    const isAltScreenActiveRef = {
        current: null
    };
    const onAltScreenScrollUpRef = {
        current: null
    };
    const scrollGestureSourceRef = {
        current: null
    };
    const onScrollModeChangeRef = {
        current: null
    };
    const src = ()=>scrollGestureSourceRef.current;
    const scrollGesture = {
        onScrollStart: (route)=>{
            var _src_onScrollStart, _src;
            return (_src = src()) === null || _src === void 0 ? void 0 : (_src_onScrollStart = _src.onScrollStart) === null || _src_onScrollStart === void 0 ? void 0 : _src_onScrollStart.call(_src, route);
        },
        onScrollGesture: (info)=>{
            var _src_onScrollGesture, _src;
            return (_src = src()) === null || _src === void 0 ? void 0 : (_src_onScrollGesture = _src.onScrollGesture) === null || _src_onScrollGesture === void 0 ? void 0 : _src_onScrollGesture.call(_src, info);
        },
        onPageKeysSent: (direction, pages)=>{
            var _src_onPageKeysSent, _src;
            return (_src = src()) === null || _src === void 0 ? void 0 : (_src_onPageKeysSent = _src.onPageKeysSent) === null || _src_onPageKeysSent === void 0 ? void 0 : _src_onPageKeysSent.call(_src, direction, pages);
        },
        onGestureActiveChange: (active)=>{
            var _src_onGestureActiveChange, _src;
            return (_src = src()) === null || _src === void 0 ? void 0 : (_src_onGestureActiveChange = _src.onGestureActiveChange) === null || _src_onGestureActiveChange === void 0 ? void 0 : _src_onGestureActiveChange.call(_src, active);
        },
        isInputBusy: ()=>{
            var _src_isInputBusy, _src;
            var _src_isInputBusy1;
            return (_src_isInputBusy1 = (_src = src()) === null || _src === void 0 ? void 0 : (_src_isInputBusy = _src.isInputBusy) === null || _src_isInputBusy === void 0 ? void 0 : _src_isInputBusy.call(_src)) !== null && _src_isInputBusy1 !== void 0 ? _src_isInputBusy1 : false;
        },
        onProgrammaticData: (data)=>{
            var _this, _src;
            var _src_onProgrammaticData;
            return (_this = (_src_onProgrammaticData = (_src = src()) === null || _src === void 0 ? void 0 : _src.onProgrammaticData) !== null && _src_onProgrammaticData !== void 0 ? _src_onProgrammaticData : onDataRef.current) === null || _this === void 0 ? void 0 : _this(data);
        }
    };
    return {
        sessionId,
        hostEl,
        handleRef: {
            current: null
        },
        warmRef: {
            current: false
        },
        pinCount: 0,
        docked: false,
        lastAccessed: Date.now(),
        onDataRef,
        onResizeRef,
        isAltScreenActiveRef,
        onAltScreenScrollUpRef,
        stableOnData: (data)=>{
            var _onDataRef_current;
            return (_onDataRef_current = onDataRef.current) === null || _onDataRef_current === void 0 ? void 0 : _onDataRef_current.call(onDataRef, data);
        },
        // Task 3.5 / Bug 7 — onResizeRef is only ever populated (non-null) while
        // the entry is docked for a foreground consumer, see usePooledTerminal;
        // a resize firing on a backgrounded/parked entry is a plain no-op here.
        stableOnResize: (cols, rows)=>{
            var _onResizeRef_current;
            return (_onResizeRef_current = onResizeRef.current) === null || _onResizeRef_current === void 0 ? void 0 : _onResizeRef_current.call(onResizeRef, cols, rows);
        },
        stableIsAltScreenActive: ()=>{
            var _isAltScreenActiveRef_current;
            var _isAltScreenActiveRef_current1;
            return (_isAltScreenActiveRef_current1 = (_isAltScreenActiveRef_current = isAltScreenActiveRef.current) === null || _isAltScreenActiveRef_current === void 0 ? void 0 : _isAltScreenActiveRef_current.call(isAltScreenActiveRef)) !== null && _isAltScreenActiveRef_current1 !== void 0 ? _isAltScreenActiveRef_current1 : false;
        },
        stableOnAltScreenScrollUp: (lines)=>{
            var _onAltScreenScrollUpRef_current;
            return (_onAltScreenScrollUpRef_current = onAltScreenScrollUpRef.current) === null || _onAltScreenScrollUpRef_current === void 0 ? void 0 : _onAltScreenScrollUpRef_current.call(onAltScreenScrollUpRef, lines);
        },
        scrollGestureSourceRef,
        onScrollModeChangeRef,
        scrollGesture,
        stableOnScrollModeChange: (mode)=>{
            var _onScrollModeChangeRef_current;
            return (_onScrollModeChangeRef_current = onScrollModeChangeRef.current) === null || _onScrollModeChangeRef_current === void 0 ? void 0 : _onScrollModeChangeRef_current.call(onScrollModeChangeRef, mode);
        }
    };
}
/**
 * Entry creation/eviction, factored out of usePoolManager purely to keep
 * function bodies short.
 *
 * `getOrCreateEntry` is called synchronously during a *consumer's* render
 * (usePooledTerminal needs the entry's stable `handleRef` available on the
 * very first render it returns). It therefore touches only `entriesRef` (a
 * plain ref, safe to mutate during any component's render) and never calls
 * `setEntryVersion` directly -- doing so triggered React's "Cannot update a
 * component while rendering a different component" warning (updating
 * TerminalPoolProvider's state from inside a child's render is unsupported,
 * unlike a component updating its own state during its own render).
 * `registerEntry`, called from `usePooledTerminal`'s effect (i.e. after
 * render commits, a fully supported time to update an ancestor's state), is
 * what actually bumps `entryVersion` so the portal list picks up the new
 * entry. A brand-new entry that gets docked before that effect fires simply
 * renders its `<XtermTerminal>` a tick later -- both effects flush in the
 * same pass, so there's no visible gap.
 */ function useEntryLifecycle(maxSize) {
    const entriesRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(new Map());
    // Pure re-render trigger -- entriesRef.current (a Map) is the source of
    // truth; this only exists so the portal list re-renders when entries are
    // added or evicted. Do not read from this for entry data.
    const [entryVersion, setEntryVersion] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(0);
    const evictLRU = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(()=>{
        let victim = null;
        for (const entry of entriesRef.current.values()){
            if (entry.pinCount > 0 || entry.docked) continue; // never evict a live consumer (Bug: "evicted while still displayed")
            if (!victim || entry.lastAccessed < victim.lastAccessed) victim = entry;
        }
        if (!victim) return false; // pool is "full" of pinned/docked entries -- allowed to exceed maxSize rather than evict something on-screen
        entriesRef.current.delete(victim.sessionId);
        victim.hostEl.remove();
        return true;
    }, []);
    const getOrCreateEntry = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)((sessionId)=>{
        const existing = entriesRef.current.get(sessionId);
        if (existing) {
            existing.lastAccessed = Date.now();
            return existing;
        }
        if (entriesRef.current.size >= maxSize) {
            evictLRU();
        }
        const entry = createEntry(sessionId);
        entriesRef.current.set(sessionId, entry);
        return entry;
    }, [
        maxSize,
        evictLRU
    ]);
    // See this function's doc comment -- call only from an effect.
    const registerEntry = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)(()=>{
        setEntryVersion((v)=>v + 1);
    }, []);
    return {
        entriesRef,
        entryVersion,
        getOrCreateEntry,
        registerEntry
    };
}
/** pin/unpin/dock/undock, factored out of usePoolManager purely to keep function bodies short. */ function useDockingActions(entriesRef, graveyardRef) {
    const [dockedIds, setDockedIds] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(new Set());
    const pin = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)((sessionId)=>{
        const entry = entriesRef.current.get(sessionId);
        if (entry) entry.pinCount += 1;
    }, [
        entriesRef
    ]);
    const unpin = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)((sessionId)=>{
        const entry = entriesRef.current.get(sessionId);
        if (entry) entry.pinCount = Math.max(0, entry.pinCount - 1);
    }, [
        entriesRef
    ]);
    const dock = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)((sessionId, anchor)=>{
        const entry = entriesRef.current.get(sessionId);
        if (!entry) return;
        entry.lastAccessed = Date.now();
        entry.docked = true;
        // appendChild on an already-attached node moves it -- no DOM
        // destroy/recreate, no WebGL context loss (module doc comment).
        if (entry.hostEl.parentElement !== anchor) {
            anchor.appendChild(entry.hostEl);
        }
        setDockedIds((prev)=>prev.has(sessionId) ? prev : new Set(prev).add(sessionId));
    }, [
        entriesRef
    ]);
    const undock = (0,react__WEBPACK_IMPORTED_MODULE_1__.useCallback)((sessionId)=>{
        const entry = entriesRef.current.get(sessionId);
        if (!entry) return;
        entry.docked = false;
        if (graveyardRef.current && entry.hostEl.parentElement !== graveyardRef.current) {
            graveyardRef.current.appendChild(entry.hostEl);
        }
        setDockedIds((prev)=>{
            if (!prev.has(sessionId)) return prev;
            const next = new Set(prev);
            next.delete(sessionId);
            return next;
        });
    }, [
        entriesRef,
        graveyardRef
    ]);
    return {
        pin,
        unpin,
        dock,
        undock,
        dockedIds
    };
}
/** Composes entry lifecycle + docking into the full pool API, factored out of TerminalPoolProvider purely to keep that component's own body short. */ function usePoolManager(maxSize, graveyardRef) {
    const { entriesRef, entryVersion, getOrCreateEntry, registerEntry } = useEntryLifecycle(maxSize);
    const { pin, unpin, dock, undock, dockedIds } = useDockingActions(entriesRef, graveyardRef);
    // dockedIds is deliberately NOT a dependency here -- see DockedIdsContext's
    // doc comment for why folding it into this memo caused an infinite loop.
    const contextValue = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>({
            maxSize,
            getOrCreateEntry,
            registerEntry,
            pin,
            unpin,
            dock,
            undock
        }), [
        maxSize,
        getOrCreateEntry,
        registerEntry,
        pin,
        unpin,
        dock,
        undock
    ]);
    return {
        entriesRef,
        entryVersion,
        contextValue,
        dockedIds
    };
}
function TerminalPoolProvider(param) {
    let { children, maxSize = DEFAULT_TERMINAL_POOL_MAX_SIZE } = param;
    const graveyardRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(null);
    const { entriesRef, entryVersion, contextValue, dockedIds } = usePoolManager(maxSize, graveyardRef);
    // Preload xterm.js eagerly (mirrors TerminalOutput.tsx's prior per-mount
    // preload, now hoisted to the pool since it's the sole mount point).
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        void Promise.all(/* import() */[__webpack_require__.e(1869), __webpack_require__.e(3576), __webpack_require__.e(151), __webpack_require__.e(3517)]).then(__webpack_require__.bind(__webpack_require__, 83517));
    }, []);
    const portals = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>{
        return Array.from(entriesRef.current.values()).map((entry)=>/*#__PURE__*/ (0,react_dom__WEBPACK_IMPORTED_MODULE_2__.createPortal)(/*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(react__WEBPACK_IMPORTED_MODULE_1__.Suspense, {
                fallback: null,
                children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(XtermTerminal, {
                    ref: entry.handleRef,
                    onData: entry.stableOnData,
                    onResize: entry.stableOnResize,
                    theme: "dark",
                    useConfig: true,
                    scrollback: POOLED_SCROLLBACK,
                    scrollGesture: entry.scrollGesture,
                    onScrollModeChange: entry.stableOnScrollModeChange,
                    isAltScreenActive: entry.stableIsAltScreenActive,
                    onAltScreenScrollUp: entry.stableOnAltScreenScrollUp
                })
            }, entry.sessionId), entry.hostEl));
    // entryVersion (not entriesRef, a ref) is the real dependency -- see its declaration.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        entryVersion
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(TerminalPoolContext.Provider, {
        value: contextValue,
        children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)(DockedIdsContext.Provider, {
            value: dockedIds,
            children: [
                children,
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
                    ref: graveyardRef,
                    className: _TerminalPool_css__WEBPACK_IMPORTED_MODULE_3__/* .graveyard */ .WR,
                    "data-testid": "terminal-pool-graveyard",
                    "aria-hidden": "true"
                }),
                portals
            ]
        })
    });
}
function useTerminalPoolContext() {
    const ctx = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(TerminalPoolContext);
    if (!ctx) {
        throw new Error("useTerminalPool/usePooledTerminal must be used within a TerminalPoolProvider");
    }
    return ctx;
}
/** Low-level pool API (Task 3.1's PoolEntry/TerminalPoolAPI sketch), for callers that need direct control rather than the usePooledTerminal convenience hook below. */ function useTerminalPool() {
    return useTerminalPoolContext();
}
/**
 * Retries `fit()`+`focus()` on `entry` until `anchor` reports a real,
 * non-zero size AND `entry.handleRef` is populated (see usePooledTerminal's
 * docking effect for why neither is guaranteed on the frame docking
 * happens). Returns a cleanup function that cancels any pending retry.
 */ function fitOnShow(entry, anchor, sessionId) {
    let settled = false;
    let rafId = null;
    let attempts = 0;
    const tryFit = ()=>{
        rafId = null;
        if (settled) return;
        const handle = entry.handleRef.current;
        const { width, height } = anchor.getBoundingClientRect();
        if (handle && width > 0 && height > 0) {
            settled = true;
            handle.fit();
            handle.focus();
            observer.disconnect();
            return;
        }
        attempts += 1;
        if (attempts >= FIT_ON_SHOW_MAX_ATTEMPTS) {
            console.warn("[TerminalPool] Gave up waiting to fit ".concat(sessionId, " after ").concat(attempts, " frames (handle ready: ").concat(!!handle, ", size: ").concat(width, "x").concat(height, ")"));
            return;
        }
        rafId = requestAnimationFrame(tryFit);
    };
    // Anchor size changing (including its first real layout, which fires once
    // even if it never changes again) is exactly the signal that a
    // previously-failed fit attempt might now succeed.
    const observer = new ResizeObserver(()=>{
        if (rafId === null) rafId = requestAnimationFrame(tryFit);
    });
    observer.observe(anchor);
    rafId = requestAnimationFrame(tryFit);
    return ()=>{
        settled = true;
        if (rafId !== null) cancelAnimationFrame(rafId);
        observer.disconnect();
    };
}
/**
 * Consumer-facing hook (Task 3.4), part 1 of 2 -- see `usePooledTerminalCallbacks`
 * below for the other half. Split in two because a caller like
 * `TerminalOutput.tsx` needs `xtermRef` available before its own
 * onData/onResize/etc. callbacks exist (they're defined later in that
 * component and close over state declared in between) -- exactly the
 * temporal-dead-zone problem TerminalOutput.tsx's own ref-mirror callbacks
 * (e.g. `handleAppScrollbackFrameRef`) already solve the same way.
 *
 * Gets-or-creates this session's pool entry, pins it for the caller's mount
 * lifetime, and docks/undocks it against `dockRef` as `isVisible` changes,
 * including fit-on-show + focus (Task 3.5). Returns a stable `xtermRef`
 * usable exactly like the `useRef<XtermTerminalHandle | null>(null)` it
 * replaces, plus `warmRef` for the caller's own loading-state decision.
 */ function usePooledTerminal(sessionId, dockRef, isVisible) {
    const pool = useTerminalPoolContext();
    const entryRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(null);
    if (!entryRef.current || entryRef.current.sessionId !== sessionId) {
        entryRef.current = pool.getOrCreateEntry(sessionId);
    }
    const entry = entryRef.current;
    // Ensures the provider mounts (or keeps mounted) this entry's
    // <XtermTerminal> portal -- deferred to an effect rather than done inside
    // getOrCreateEntry's synchronous render-time call; see useEntryLifecycle's
    // doc comment for why.
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        pool.registerEntry();
    }, [
        pool,
        sessionId
    ]);
    // Pin for this consumer's mount lifetime (keeps the entry out of LRU
    // eviction while something is actually displaying it, even backgrounded --
    // SessionDetailView's own inner tab pool can background this consumer
    // without unmounting it).
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        pool.pin(sessionId);
        return ()=>pool.unpin(sessionId);
    }, [
        pool,
        sessionId
    ]);
    // Dock/undock + fit-on-show + focus (Task 3.5).
    //
    // Bug 2 (docs/tasks/terminal-jank.md "FitAddon on visibility:hidden
    // Terminals") — this used to fit() inside a single requestAnimationFrame,
    // assuming both (a) the anchor's CSS layout had already settled to its
    // final size and (b) the lazily-loaded <XtermTerminal> (see the `lazy(...)`
    // import above) had already mounted and populated `entry.handleRef`.
    // Neither is guaranteed on every dock: a window-switch's freshly-mounted
    // pane can still be mid-layout a frame later, and a cold pool entry's
    // XtermTerminal chunk may not have resolved yet. When the guess was wrong,
    // fit() ran against a stale/zero size (or was a no-op on a null ref) and
    // nothing re-fit it afterward, leaving a blank terminal until the user
    // clicked the manual "Resize" button. fitOnShow() polls (bounded) instead
    // of guessing one frame ahead.
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        if (!isVisible) {
            pool.undock(sessionId);
            return;
        }
        const anchor = dockRef.current;
        if (!anchor) return;
        pool.dock(sessionId, anchor);
        const cancelFit = fitOnShow(entry, anchor, sessionId);
        return ()=>{
            cancelFit();
            // Also fires on unmount (not just isVisible->false) -- without this an
            // unmounted consumer leaves its entry stuck `docked: true`, permanently
            // exempt from evictLRU.
            pool.undock(sessionId);
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- entry/dockRef are stable identities; re-run only on session/visibility change
    }, [
        pool,
        sessionId,
        isVisible
    ]);
    return {
        xtermRef: entry.handleRef,
        warmRef: entry.warmRef
    };
}
/**
 * Consumer-facing hook (Task 3.4), part 2 of 2 -- call after
 * `usePooledTerminal` and after the callbacks in `callbacks` are defined
 * (see that hook's doc comment for why this is split out). Registers this
 * render's callback closures onto the entry's ref-mirrors so
 * `<XtermTerminal>` (mounted once, inside TerminalPoolProvider, never
 * re-rendered for this) always dispatches to the current consumer.
 */ function usePooledTerminalCallbacks(sessionId, isVisible, param) {
    let { onData, onResize, isAltScreenActive, onAltScreenScrollUp, scrollGesture, onScrollModeChange } = param;
    const pool = useTerminalPoolContext();
    // Entry must already exist -- usePooledTerminal (called earlier in the
    // same render, per its own doc comment) created it. getOrCreateEntry is a
    // cheap idempotent lookup here, not a real creation.
    const entry = pool.getOrCreateEntry(sessionId);
    // Task 3.6 rapid-switch guard: always point the entry's callback refs at
    // THIS render's closures. A write/resize that arrives after this consumer
    // unmounts can only reach a ref the cleanup below has already nulled out
    // (guarded so it only clears its OWN callback, never a still-live
    // successor's that took over first).
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        entry.onDataRef.current = onData;
        // Bug 7 / Task 3.5 — never forward resizes to a backgrounded consumer's
        // backend resize() call, even if a stray ResizeObserver tick fires on a
        // parked (graveyard) or momentarily-hidden entry.
        entry.onResizeRef.current = isVisible ? onResize : null;
        entry.isAltScreenActiveRef.current = isAltScreenActive;
        entry.onAltScreenScrollUpRef.current = onAltScreenScrollUp;
        return ()=>{
            if (entry.onDataRef.current === onData) entry.onDataRef.current = null;
            if (entry.onResizeRef.current === onResize) entry.onResizeRef.current = null;
            if (entry.isAltScreenActiveRef.current === isAltScreenActive) entry.isAltScreenActiveRef.current = null;
            if (entry.onAltScreenScrollUpRef.current === onAltScreenScrollUp) entry.onAltScreenScrollUpRef.current = null;
        };
    }, [
        entry,
        isVisible,
        onData,
        onResize,
        isAltScreenActive,
        onAltScreenScrollUp
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        entry.onScrollModeChangeRef.current = onScrollModeChange !== null && onScrollModeChange !== void 0 ? onScrollModeChange : null;
        return ()=>{
            if (entry.onScrollModeChangeRef.current === onScrollModeChange) entry.onScrollModeChangeRef.current = null;
        };
    }, [
        entry,
        onScrollModeChange
    ]);
    // Callbacks flow through the source ref; only the primitives need a re-render of <XtermTerminal>.
    const scrollOverride = scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.scrollOverride;
    const gestureScrollEnabled = scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.gestureScrollEnabled;
    const tuiScrollPolicy = scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.tuiScrollPolicy;
    const connectionEpoch = scrollGesture === null || scrollGesture === void 0 ? void 0 : scrollGesture.connectionEpoch;
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        entry.scrollGestureSourceRef.current = scrollGesture !== null && scrollGesture !== void 0 ? scrollGesture : null;
        return ()=>{
            if (entry.scrollGestureSourceRef.current === scrollGesture) entry.scrollGestureSourceRef.current = null;
        };
    }, [
        entry,
        scrollGesture
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        const cur = entry.scrollGesture;
        if (cur.scrollOverride === scrollOverride && cur.gestureScrollEnabled === gestureScrollEnabled && cur.tuiScrollPolicy === tuiScrollPolicy && cur.connectionEpoch === connectionEpoch) return;
        entry.scrollGesture = {
            ...cur,
            scrollOverride,
            gestureScrollEnabled,
            tuiScrollPolicy,
            connectionEpoch
        };
        pool.registerEntry();
    }, [
        entry,
        pool,
        scrollOverride,
        gestureScrollEnabled,
        tuiScrollPolicy,
        connectionEpoch
    ]);
}


/***/ }),

/***/ 81073:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ DEFAULT_TERMINAL_CONFIG),
/* harmony export */   Ez: () => (/* binding */ lightTerminalTheme),
/* harmony export */   Gd: () => (/* binding */ darkTerminalTheme),
/* harmony export */   X6: () => (/* binding */ loadTerminalConfig),
/* harmony export */   wo: () => (/* binding */ saveTerminalConfig)
/* harmony export */ });
/* unused harmony export TERMINAL_CONFIG_PRESETS */
/**
 * Terminal configuration system with localStorage persistence
 *
 * Provides user-configurable settings for terminal behavior, appearance,
 * and performance. All settings persist across sessions.
 */ /**
 * Default terminal configuration
 * These values provide good performance and UX for most users
 */ const DEFAULT_TERMINAL_CONFIG = {
    scrollbackLines: 0,
    theme: "dark",
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    enableBell: false,
    cursorStyle: "block",
    cursorBlink: true,
    mouseTracking: "none"
};
/**
 * Configuration presets for different use cases
 */ const TERMINAL_CONFIG_PRESETS = {
    "default": DEFAULT_TERMINAL_CONFIG,
    "high-performance": {
        scrollbackLines: 0,
        cursorBlink: false
    },
    "accessibility": {
        fontSize: 16,
        cursorStyle: "block",
        cursorBlink: false,
        theme: "light"
    }
};
/**
 * Named dark terminal theme for xterm.js
 */ const darkTerminalTheme = {
    background: '#1e1e1e',
    foreground: '#cccccc',
    cursor: '#cccccc',
    cursorAccent: '#1e1e1e',
    selectionBackground: 'rgba(255, 255, 255, 0.3)',
    black: '#000000',
    red: '#cd3131',
    green: '#0dbc79',
    yellow: '#e5e510',
    blue: '#2472c8',
    magenta: '#bc3fbc',
    cyan: '#11a8cd',
    white: '#e5e5e5',
    brightBlack: '#666666',
    brightRed: '#f14c4c',
    brightGreen: '#23d18b',
    brightYellow: '#f5f543',
    brightBlue: '#3b8eea',
    brightMagenta: '#d670d6',
    brightCyan: '#29b8db',
    brightWhite: '#ffffff'
};
/**
 * Named light terminal theme for xterm.js
 */ const lightTerminalTheme = {
    background: '#ffffff',
    foreground: '#1a1a1a',
    cursor: '#333333',
    selectionBackground: 'rgba(0, 112, 243, 0.3)',
    black: '#000000',
    red: '#c0392b',
    green: '#27ae60',
    yellow: '#e67e22',
    blue: '#2980b9',
    magenta: '#8e44ad',
    cyan: '#16a085',
    white: '#bdc3c7',
    brightBlack: '#7f8c8d',
    brightRed: '#e74c3c',
    brightGreen: '#2ecc71',
    brightYellow: '#f39c12',
    brightBlue: '#3498db',
    brightMagenta: '#9b59b6',
    brightCyan: '#1abc9c',
    brightWhite: '#ecf0f1'
};
const STORAGE_KEY = "stapler-squad-terminal-config";
/**
 * Load terminal configuration from localStorage
 * Falls back to default config if not found or invalid
 */ function loadTerminalConfig() {
    if (false) {}
    try {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (!stored) {
            return DEFAULT_TERMINAL_CONFIG;
        }
        const config = JSON.parse(stored);
        var _config_scrollbackLines, _config_fontSize;
        // Validate and merge with defaults
        return {
            ...DEFAULT_TERMINAL_CONFIG,
            ...config,
            // Ensure scrollbackLines is within reasonable bounds (0 = disabled, tmux handles it)
            scrollbackLines: Math.max(0, Math.min((_config_scrollbackLines = config.scrollbackLines) !== null && _config_scrollbackLines !== void 0 ? _config_scrollbackLines : DEFAULT_TERMINAL_CONFIG.scrollbackLines, 100000)),
            // Ensure fontSize is within reasonable bounds
            fontSize: Math.max(8, Math.min((_config_fontSize = config.fontSize) !== null && _config_fontSize !== void 0 ? _config_fontSize : DEFAULT_TERMINAL_CONFIG.fontSize, 32))
        };
    } catch (err) {
        console.error("[terminalConfig] Failed to load config:", err);
        return DEFAULT_TERMINAL_CONFIG;
    }
}
/**
 * Save terminal configuration to localStorage
 */ function saveTerminalConfig(config) {
    if (false) {}
    try {
        const current = loadTerminalConfig();
        const updated = {
            ...current,
            ...config
        };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
        // Dispatch custom event for other components to react
        window.dispatchEvent(new CustomEvent("terminal-config-changed", {
            detail: updated
        }));
    } catch (err) {
        console.error("[terminalConfig] Failed to save config:", err);
    }
}


/***/ })

}]);
//# sourceMappingURL=6262.4becdfaf3ad5468c.js.map