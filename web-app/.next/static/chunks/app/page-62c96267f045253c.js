(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[8974],{

/***/ 4942:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   T: () => (/* binding */ commandBarController)
/* harmony export */ });
/**
 * Imperative handle to the mounted command bar, so non-React callers (the
 * Alt+W alias, the Settings "Open command bar" button) can open it without
 * prop-drilling. `open` returns false when no bar is mounted or a modal dialog
 * is already open, letting callers fall back to their own behaviour.
 */ let opener = null;
const commandBarController = {
    attach (fn) {
        opener = fn;
        return ()=>{
            if (opener === fn) opener = null;
        };
    },
    open () {
        let path = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : [];
        return opener ? opener(path) : false;
    }
};


/***/ }),

/***/ 20621:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   J: () => (/* binding */ GitHubBadge)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(74512);
/* __next_internal_client_entry_do_not_use__ GitHubBadge auto */ 

/** Map PRPriority string → CSS module class name for the badge variant. */ function priorityClass(priority) {
    switch(priority){
        case "blocking":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeBlocking */ .B2;
        case "ready":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeReady */ .VG;
        case "pending":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgePending */ .ho;
        case "draft":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeDraft */ .hC;
        case "complete":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeComplete */ .L7;
        case "auth_error":
        case "error":
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeError */ .sv;
        default:
            return _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadgeUnknown */ .o5; // gray: not yet loaded
    }
}
/** Human-readable label for each priority state. */ function priorityLabel(priority, isDraft) {
    if (isDraft) return "Draft";
    switch(priority){
        case "blocking":
            return "Blocked";
        case "ready":
            return "Ready";
        case "pending":
            return "Pending";
        case "draft":
            return "Draft";
        case "complete":
            return "Merged";
        case "auth_error":
            return "Auth Error";
        case "error":
            return "Error";
        default:
            return "";
    }
}
/**
 * Badge component that displays GitHub PR or repository information.
 *
 * Shows:
 * - PR badge with number, link, and priority status (when prNumber > 0)
 * - Repository badge with owner/repo (when no PR but has repo info)
 * - Nothing if no GitHub information is available
 */ function GitHubBadge(param) {
    let { prNumber, prUrl, owner, repo, sourceRef, host, prPriority, prState, isDraft, approvedCount, changesRequestedCount, checkConclusion, compact = false } = param;
    // Don't render if no GitHub information
    const hasPR = prNumber && prNumber > 0;
    const hasRepo = owner && repo;
    if (!hasPR && !hasRepo) {
        return null;
    }
    // PR Badge
    if (hasPR) {
        // Construct URL from owner/repo when prUrl is missing (older sessions may not have it stored)
        const resolvedPrUrl = prUrl || (owner && repo ? "https://".concat(host || "github.com", "/").concat(owner, "/").concat(repo, "/pull/").concat(prNumber) : undefined);
        const handleClick = (e)=>{
            e.stopPropagation();
            e.preventDefault(); // the anchor's own href+target would otherwise open a second tab
            if (resolvedPrUrl) {
                window.open(resolvedPrUrl, "_blank", "noopener,noreferrer");
            }
        };
        const priorityVariant = priorityClass(prPriority);
        const statusLabel = priorityLabel(prPriority, isDraft);
        // Build tooltip with full status detail
        const tooltipParts = [
            "GitHub PR #".concat(prNumber)
        ];
        if (prState) tooltipParts.push("State: ".concat(prState));
        if (approvedCount && approvedCount > 0) tooltipParts.push("".concat(approvedCount, " approval").concat(approvedCount > 1 ? "s" : ""));
        if (changesRequestedCount && changesRequestedCount > 0) tooltipParts.push("".concat(changesRequestedCount, " change request").concat(changesRequestedCount > 1 ? "s" : ""));
        if (checkConclusion) tooltipParts.push("CI: ".concat(checkConclusion));
        const tooltip = tooltipParts.join(" · ");
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("a", {
            href: resolvedPrUrl,
            target: "_blank",
            rel: "noopener noreferrer",
            className: "".concat(_GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .badge */ .qS, " ").concat(_GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .prBadge */ .Eg, " ").concat(priorityVariant, " ").concat(compact ? _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .compact */ .oE : ""),
            onClick: handleClick,
            title: tooltip,
            "aria-label": "View GitHub Pull Request #".concat(prNumber).concat(statusLabel ? ", status: ".concat(statusLabel) : ""),
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("svg", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .icon */ .Kk,
                    viewBox: "0 0 16 16",
                    fill: "currentColor",
                    "aria-hidden": "true",
                    children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("path", {
                        d: "M1.5 3.25a2.25 2.25 0 1 1 3 2.122v5.256a2.251 2.251 0 1 1-1.5 0V5.372A2.25 2.25 0 0 1 1.5 3.25Zm5.677-.177L9.573.677A.25.25 0 0 1 10 .854V2.5h1A2.5 2.5 0 0 1 13.5 5v5.628a2.251 2.251 0 1 1-1.5 0V5a1 1 0 0 0-1-1h-1v1.646a.25.25 0 0 1-.427.177L7.177 3.427a.25.25 0 0 1 0-.354ZM3.75 2.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm0 9.5a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Zm8.25.75a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Z"
                    })
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("span", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .text */ .Qq,
                    children: [
                        "#",
                        prNumber
                    ]
                }),
                statusLabel && /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .priorityLabel */ .BS,
                    children: statusLabel
                })
            ]
        });
    }
    // Repository Badge (for non-PR GitHub sessions)
    if (hasRepo) {
        const repoUrl = "https://".concat(host || "github.com", "/").concat(owner, "/").concat(repo);
        const handleClick = (e)=>{
            e.stopPropagation();
            e.preventDefault(); // the anchor's own href+target would otherwise open a second tab
            window.open(repoUrl, "_blank", "noopener,noreferrer");
        };
        return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsxs)("a", {
            href: repoUrl,
            target: "_blank",
            rel: "noopener noreferrer",
            className: "".concat(_GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .badge */ .qS, " ").concat(_GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .repoBadge */ .m2, " ").concat(compact ? _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .compact */ .oE : ""),
            onClick: handleClick,
            title: "GitHub Repository: ".concat(owner, "/").concat(repo).concat(sourceRef ? " (".concat(sourceRef, ")") : ""),
            "aria-label": "View GitHub Repository ".concat(owner, "/").concat(repo),
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("svg", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .icon */ .Kk,
                    viewBox: "0 0 16 16",
                    fill: "currentColor",
                    "aria-hidden": "true",
                    children: /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("path", {
                        d: "M8 0c4.42 0 8 3.58 8 8a8.013 8.013 0 0 1-5.45 7.59c-.4.08-.55-.17-.55-.38 0-.27.01-1.13.01-2.2 0-.75-.25-1.23-.54-1.48 1.78-.2 3.65-.88 3.65-3.95 0-.88-.31-1.59-.82-2.15.08-.2.36-1.02-.08-2.12 0 0-.67-.22-2.2.82-.64-.18-1.32-.27-2-.27-.68 0-1.36.09-2 .27-1.53-1.03-2.2-.82-2.2-.82-.44 1.1-.16 1.92-.08 2.12-.51.56-.82 1.28-.82 2.15 0 3.06 1.86 3.75 3.64 3.95-.23.2-.44.55-.51 1.07-.46.21-1.61.55-2.33-.66-.15-.24-.6-.83-1.23-.82-.67.01-.27.38.01.53.34.19.73.9.82 1.13.16.45.68 1.31 2.69.94 0 .67.01 1.3.01 1.49 0 .21-.15.45-.55.38A7.995 7.995 0 0 1 0 8c0-4.42 3.58-8 8-8Z"
                    })
                }),
                /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("span", {
                    className: _GitHubBadge_css__WEBPACK_IMPORTED_MODULE_1__/* .text */ .Qq,
                    children: compact ? repo : "".concat(owner, "/").concat(repo)
                })
            ]
        });
    }
    return null;
}


/***/ }),

/***/ 25706:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
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

/***/ 35007:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  W: () => (/* binding */ ErrorState)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FErrorState.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-VVy27bMBC8-ysIAwUsIBQoJ_KDuRToOf2FghZXFhuKFEg6fhT-98KURFOO7PRe-GJJy9mZ2eVuCsvT1uYE_ZkgxIVtJDtSVEo4vE4QYlJsFRYOaktRAcqBubz-vbNOlEdcaOVAufhTLRSuQGwrR9ELIY2HaRjnQm0pmhuoXyfnSdpmzXzWmh3wXnBXUbToT3TPGSHfLo8ODg57MtdcAWXuUQottaHog5kZxr-yNZzm5pSRxJNiZisU3mjndE1Rluaex4jgu8pCtmefrdTKYStOEKP5l_tO_IKQa2qKyOXnI5F_P0r3wjYkenkgK7mFngfgmFnHSwoFoShZmkdJcp8k1IekS0_xqmnDivet0TvFb0nUSaRjXwkHPl4bDoYipVX0jA3jYmcpWrTlHSE5MC9vzSt2xl7QGy36_nKGKSuc0IpG3BBJ5_ap_VhqUyOSZjaWSSv9AcaLva-oSQZHWOHEB_gzAZgiWzAJM5KuV3H0ouvktiLXasSNK6F0jy0tktjCrDkgq6XgN1GQjBi7ao29qCyl3lNUCc5BRRSXw1L3vo94PFaKnQWDLUgo3LW4D4oRJ_4n83ls52qca6fZ6eaxOwFnHZXlev2HQ2hNJbMOF5WQfCycDPCsM1pth9NyI3XxPjJnSLhFX9929oU9GU_uAmVtR9yzq2-Rl2GL4ANFbOf0zX0k6WqZ3x0c0dQJ50pWC3mkaPoGSurpE5q-acUK_--H3hkBBv2E_fQJ1Vpp27ACIuGb_0x4u-967Od-4QVyx57cefK9Bi4YsoUBUIgpjmbRrlwuVs0h8e6l8RIf7uCQYMSm8_Xoc3d0sNfm-efAl8-BsXFRZN5FRstlMe-2S0B-ABT0zxoDJRiLfQ9gW1RQA0WcmfeB_EWX8Kv5GvpjtKfK5EZHPL4egX86uOqPhKn1OOP5L9VYAxuMCQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FErrorState_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_VVy27bMBC8_ysIAwUsIBQoJ_KDuRToOf2FghZXFhuKFEg6fhT_98KURFOO7PRe_GJJy9mZ2eVuCsvT1uYE_ZkgxIVtJDtSVEo4vE4QYlJsFRYOaktRAcqBubz_vbNOlEdcaOVAufhTLRSuQGwrR9ELIY2HaRjnQm0pmhuoXyfnSdpmzXzWmh3wXnBXUbToT3TPGSHfLo8ODg57MtdcAWXuUQottaHog5kZxr_yNZzm5pSRxJNiZisU3mjndE1Rluaex4jgu8pCtmefrdTKYStOEKP5l_tO_IKQa2qKyOXnI5F_P0r3wjYkenkgK7mFngfgmFnHSwoFoShZmkdJcp8k1IekS0_xqmnDivet0TvFb0nUSaRjXwkHPl4bDoYipVX0jA3jYmcpWrTlHSE5MC9vzSt2xl7QGy36_nKGKSuc0IpG3BBJ5_ap_VhqUyOSZjaWSSv9AcaLva_oSQZHWOHEB_gzAZgiWzAJM5KuV3H0ouvktiLXasSNK6F0jy0tktjCrDkgq6XgN1GQjBi7ao29qCyl3lNUCc5BRRSXw1L3vo94PFaKnQWDLUgo3LW4D4oRJ_4n83ls52qca6fZ6eaxOwFnHZXlev2HQ2hNJbMOF5WQfCycDPCsM1pth9NyI3XxPjJnSLhFX9929oU9GU_uAmVtR9yzq2_Rl2GL4ANFbOf0zX0k6WqZ3x0c0dQJ50pWC3mkaPoGSurpE5q_acUK_H3hkBBv2E_fQJ1Vpp27ACIuGb_0x4u_967Od_4QVyx57cefK9Bi4YsoUBUIgpjmbRrlwuVs0h8e6l8RIf7uCQYMSm8_Xoc3d0sNfm_efAl8_BsXFRZN5FRstlMe_2S0B_ABT0zxoDJRiLfQ9gW1RQA0WcmfeB_EWX8Kv5GvpjtKfK5EZHPL4egX86uOqPhKn1OOP5L9VYAxuMCQAA_22_7D = __webpack_require__(88491);
;// ./src/components/ui/ErrorState.css.ts


var container = 'e7zgs50';
var content = 'e7zgs51';
var details = 'e7zgs56';
var detailsContent = 'e7zgs58';
var detailsSummary = 'e7zgs57';
var errorBlock = 'e7zgs59';
var errorText = 'e7zgs5a';
var icon = 'e7zgs52';
var message = 'e7zgs54';
var retryButton = 'e7zgs55';
var stackTrace = 'e7zgs5b';
var title = 'e7zgs53';
;// ./src/components/ui/ErrorState.tsx


function ErrorState(param) {
    let { error, title = "Error", message = "An error occurred", onRetry, showDetails = false, errorInfo, actionLabel = "Try Again" } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: container,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: content,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: icon,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("svg", {
                        width: "64",
                        height: "64",
                        viewBox: "0 0 24 24",
                        fill: "none",
                        xmlns: "http://www.w3.org/2000/svg",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("circle", {
                                cx: "12",
                                cy: "12",
                                r: "10",
                                stroke: "currentColor",
                                strokeWidth: "2"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("path", {
                                d: "M12 8V12",
                                stroke: "currentColor",
                                strokeWidth: "2",
                                strokeLinecap: "round"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("circle", {
                                cx: "12",
                                cy: "16",
                                r: "1",
                                fill: "currentColor"
                            })
                        ]
                    })
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: title,
                    children: title
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: message,
                    children: message
                }),
                showDetails && error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("details", {
                    className: details,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("summary", {
                            className: detailsSummary,
                            children: "Error Details"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: detailsContent,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: errorBlock,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                            children: "Error:"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                            className: errorText,
                                            children: error.message
                                        })
                                    ]
                                }),
                                error.stack && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: errorBlock,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                            children: "Stack Trace:"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                            className: stackTrace,
                                            children: error.stack
                                        })
                                    ]
                                }),
                                (errorInfo === null || errorInfo === void 0 ? void 0 : errorInfo.componentStack) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: errorBlock,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                            children: "Component Stack:"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                            className: stackTrace,
                                            children: errorInfo.componentStack
                                        })
                                    ]
                                })
                            ]
                        })
                    ]
                }),
                onRetry && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: retryButton,
                    onClick: onRetry,
                    children: actionLabel
                })
            ]
        })
    });
}


/***/ }),

/***/ 48843:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $g: () => (/* binding */ invalidateTaggingRuleNamesCache),
/* harmony export */   Lk: () => (/* binding */ useTaggingRuleNames)
/* harmony export */ });
/* unused harmony export _resetTaggingRuleNamesCacheForTesting */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(23575);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95444);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* __next_internal_client_entry_do_not_use__ _resetTaggingRuleNamesCacheForTesting,invalidateTaggingRuleNamesCache,useTaggingRuleNames auto */ 





/**
 * Module-level shared cache: SessionCard renders one instance of
 * useTaggingRuleNames per card, potentially dozens at once in a session
 * list. Sharing a single in-flight/resolved fetch (instead of one RPC per
 * card) is a cheap dedup that costs nothing in correctness — rule names
 * change rarely, and this hook is a tooltip nicety, not a source of truth
 * any save path depends on.
 */ let cachedNamesPromise = null;
/**
 * Mounted `useTaggingRuleNames` instances register a refetch callback here so
 * `invalidateTaggingRuleNamesCache` can push fresh data to them directly —
 * without this, invalidation would only affect the *next* mount (empty-deps
 * `useEffect`), leaving already-mounted SessionCards stuck on stale names.
 */ const cacheListeners = new Set();
async function fetchRuleNames() {
    try {
        const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .SessionService */ .Qow, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)());
        // This fetch is a module-level cache shared across every mounted SessionCard
        // (see cachedNamesPromise above) — aborting it on any one card's unmount
        // would break the fetch for every other still-mounted consumer.
        // abort-signal-exempt
        const resp = await client.listTaggingRules((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_session_pb__WEBPACK_IMPORTED_MODULE_1__/* .ListTaggingRulesRequestSchema */ .XyZ, {}));
        const names = {};
        var _resp_rules;
        for (const rule of (_resp_rules = resp.rules) !== null && _resp_rules !== void 0 ? _resp_rules : []){
            names[rule.id] = rule.name;
        }
        return names;
    } catch (err) {
        console.error("Failed to fetch tagging rule names:", err);
        // Don't cache a failed fetch: a transient RPC error would otherwise leave
        // every tooltip blank for the rest of the page's life with no retry.
        cachedNamesPromise = null;
        return {};
    }
}
/** Resets the module-level cache — for tests only. */ function _resetTaggingRuleNamesCacheForTesting() {
    cachedNamesPromise = null;
}
/**
 * Invalidates the module-level rule-names cache so the next render refetches
 * fresh data. Call after a tagging rule mutation (upsert/delete) so already-
 * mounted SessionCards' provenance tooltips pick up the new name/removal
 * instead of showing stale data until a full page reload.
 */ function invalidateTaggingRuleNamesCache() {
    cachedNamesPromise = null;
    for (const listener of cacheListeners)listener();
}
/**
 * Returns a map of TaggingRule.ID -> rule name, for resolving a tag's
 * provenance (`RuleTagProvenance[tag]`, a rule ID or the "llm" sentinel) into
 * human-readable tooltip text (ux.md Surface 1). Falls back to an empty map
 * on any fetch failure — callers should treat an unresolved ID the same as a
 * deleted rule (ux.md's "originating rule no longer exists" edge case).
 */ function useTaggingRuleNames() {
    const [names, setNames] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)({});
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        let cancelled = false;
        const applyResolved = (resolved)=>{
            if (cancelled) return;
            setNames(resolved);
        };
        const load = ()=>{
            if (!cachedNamesPromise) {
                cachedNamesPromise = fetchRuleNames();
            }
            cachedNamesPromise.then(applyResolved);
        };
        load();
        cacheListeners.add(load);
        return ()=>{
            cancelled = true;
            cacheListeners.delete(load);
        };
    }, []);
    return names;
}


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

/***/ 53064:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ae: () => (/* binding */ useSystemMemory),
/* harmony export */   Wp: () => (/* binding */ SystemMemoryProvider)
/* harmony export */ });
/* unused harmony export MEMORY_PRESSURE_THRESHOLD */
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* harmony import */ var _lib_contexts_SessionServiceContext__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(87963);
/* __next_internal_client_entry_do_not_use__ MEMORY_PRESSURE_THRESHOLD,SystemMemoryProvider,useSystemMemory auto */ 
// +feature: ui:memory-pressure-indicator


/** Default memory pressure threshold (mirrors backend HibernationConfig default). */ const MEMORY_PRESSURE_THRESHOLD = 85;
const SystemMemoryContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)({
    systemMemoryPct: 0,
    isUnderPressure: false
});
function SystemMemoryProvider(param) {
    let { children } = param;
    const { systemMemoryPct } = (0,_lib_contexts_SessionServiceContext__WEBPACK_IMPORTED_MODULE_2__/* .useSessionServiceContext */ .MS)();
    const isUnderPressure = systemMemoryPct >= MEMORY_PRESSURE_THRESHOLD;
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(SystemMemoryContext.Provider, {
        value: {
            systemMemoryPct,
            isUnderPressure
        },
        children: children
    });
}
function useSystemMemory() {
    return (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(SystemMemoryContext);
}


/***/ }),

/***/ 53407:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ Home)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionListSkeleton.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81Uy26rMBDd5ytmEylI15FxCiHupn9SuXggo4CxjGlJqvx7BQQViNR7r7rJyprnOXM89ssJz5lTJdbwGjYfAt8TDp8rAL7uD4A3lZ5yVzVGM1vV5KkyEgTna-DPK4DrCiDkP2ezafp1tR2Rwr5IU20LdZaQO9JdTncyj6UtlEeWVkVTmlpCYlsIMweq8RUIwW0LO2Hb3u7KVEG5YeSxrCWkaDy6vpuyEt6V2zD2Gh7wItxFXIIuYpXWZHIJHEJh2851RMqPXsIu6ewJVdFT_SDtjz2TafLNfKucRsec0tTUS8jdoYfMCmxZfXRkTnIQ5FuwZUkSzBjsegYjZsj_HfTGOhRjzQSyIIPKsbxrgMZvDlxj_mfRZx-AiNZLbxJAxO-8-wD20TqY47CaLnjbmm5ZZoM9TaUNn-bajvbPc8bB36W8F3_CIZqJ-x8XOtKOb9pWVqXkzxL4Nn5AreNfzimSuzGjBxxzP1upeHjLLyVqUrCxDjN0NXOomxQ1K6vhlzIVG0JoUgz6DovHB6AMlapLZ0aVKL__zOdFWDdODW3DbVQvo-RxCLO0aoyXQCYjQx6XiZ5KMjnLGpMO3QZxx493sb-Pyy9-RH7XL5KiVwL9BgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionListSkeleton_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81Uy26rMBDd5ytmEylI15FxCiHupn9SuXggo4CxjGlJqvx7BQQViNR7r7rJyprnOXM89ssJz5lTJdbwGjYfAt8TDp8rAL7uD4A3lZ5yVzVGM1vV5KkyEgTna_DPK4DrCiDkP2ezafp1tR2Rwr5IU20LdZaQO9JdTncyj6UtlEeWVkVTmlpCYlsIMweq8RUIwW0LO2Hb3u7KVEG5YeSxrCWkaDy6vpuyEt6V2zD2Gh7wItxFXIIuYpXWZHIJHEJh2851RMqPXsIu6ewJVdFT_SDtjz2TafLNfKucRsec0tTUS8jdoYfMCmxZfXRkTnIQ5FuwZUkSzBjsegYjZsj_HfTGOhRjzQSyIIPKsbxrgMZvDlxj_mfRZx_AiNZLbxJAxO_8_wD20TqY47CaLnjbmm5ZZoM9TaUNn_bajvbPc8bB36W8F3_CIZqJ_x8XOtKOb9pWVqXkzxL4Nn5AreNfzimSuzGjBxxzP1upeHjLLyVqUrCxDjN0NXOomxQ1K6vhlzIVG0JoUgz6DovHB6AMlapLZ0aVKL_zOdFWDdODW3DbVQvo_RxCLO0aoyXQCYjQx6XiZ5KMjnLGpMO3QZxx493sb_Pyy9_RH7XL5KiVwL9BgAA_22_7D = __webpack_require__(95981);
;// ./src/components/sessions/SessionListSkeleton.css.ts


var actionsSpacer = '_1uw2ev87';
var agentPlaceholder = '_1uw2ev84';
var dot = '_1uw2ev82';
var nameBar = '_1uw2ev83';
var pathBar = '_1uw2ev85';
var skeletonRow = '_1uw2ev81';
var timeBar = '_1uw2ev86';
;// ./src/components/sessions/SessionListSkeleton.tsx
/* __next_internal_client_entry_do_not_use__ SessionListSkeleton auto */ 


function SkeletonRow(param) {
    let { index } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: skeletonRow,
        "aria-hidden": "true",
        "data-testid": "skeleton-row-".concat(index),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: dot
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: nameBar
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: agentPlaceholder
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: pathBar
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: timeBar
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: actionsSpacer
            })
        ]
    });
}
function SessionListSkeleton(param) {
    let { count = 6 } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        role: "status",
        "aria-label": "Loading sessions…",
        "aria-busy": "true",
        "data-testid": "session-list-skeleton",
        children: Array.from({
            length: count
        }, (_, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(SkeletonRow, {
                index: i
            }, i))
    });
}

// EXTERNAL MODULE: ./src/components/sessions/ResumeSessionModal.tsx + 2 modules
var ResumeSessionModal = __webpack_require__(61053);
// EXTERNAL MODULE: ./src/lib/contexts/SessionServiceContext.tsx + 1 modules
var SessionServiceContext = __webpack_require__(87963);
// EXTERNAL MODULE: ./src/lib/hooks/useKeyboard.ts
var useKeyboard = __webpack_require__(76793);
// EXTERNAL MODULE: ./src/lib/hooks/useFocusTrap.ts
var useFocusTrap = __webpack_require__(63991);
// EXTERNAL MODULE: ./src/lib/contexts/OmnibarContext.tsx + 43 modules
var OmnibarContext = __webpack_require__(40218);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/columns-2.mjs
var columns_2 = __webpack_require__(68728);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/rows-2.mjs
var rows_2 = __webpack_require__(11192);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/layout-list.mjs
var layout_list = __webpack_require__(66863);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/maximize-2.mjs
var maximize_2 = __webpack_require__(14365);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/x.mjs
var x = __webpack_require__(91208);
// EXTERNAL MODULE: ./src/lib/shortcuts/useShortcut.ts
var useShortcut = __webpack_require__(21777);
// EXTERNAL MODULE: ./src/lib/pane/paneUtils.ts
var paneUtils = __webpack_require__(25706);
;// ./src/lib/pane/usePaneShortcuts.ts
/* __next_internal_client_entry_do_not_use__ usePaneShortcuts auto */ 


/**
 * usePaneShortcuts — registers all 12 pane keyboard shortcuts.
 *
 * All shortcuts use context: "cockpit" so they fire even when a terminal
 * pane is focused (see the cockpit context guard in shortcutRegistry.ts).
 *
 * Note on Ctrl+-: browser zoom-out on Windows/Linux. event.preventDefault()
 * in ShortcutRegistry's dispatch loop reliably blocks zoom in Chrome/Firefox.
 * On macOS, zoom is Cmd+-, so Ctrl+- is safe. If Safari proves unreliable,
 * use Ctrl+Shift+H as a fallback.
 *
 * Note on Ctrl+W: browser close-tab. preventDefault() blocks it on Chrome/Firefox
 * when the page's keydown handler fires first. The close button (✕) in each pane
 * header is the primary reliable close path.
 */ function usePaneShortcuts(state, dispatch, containerRef) {
    // ── Split ──────────────────────────────────────────────────────────────────
    const splitVertical = (0,react.useCallback)(()=>{
        dispatch({
            type: "SPLIT_PANE",
            paneId: state.focusedPaneId,
            direction: "vertical"
        });
    }, [
        dispatch,
        state.focusedPaneId
    ]);
    const splitHorizontal = (0,react.useCallback)(()=>{
        dispatch({
            type: "SPLIT_PANE",
            paneId: state.focusedPaneId,
            direction: "horizontal"
        });
    }, [
        dispatch,
        state.focusedPaneId
    ]);
    // ── Close ──────────────────────────────────────────────────────────────────
    const closePane = (0,react.useCallback)(()=>{
        dispatch({
            type: "CLOSE_PANE",
            paneId: state.focusedPaneId
        });
    }, [
        dispatch,
        state.focusedPaneId
    ]);
    // ── Focus navigation ───────────────────────────────────────────────────────
    const focusRight = (0,react.useCallback)(()=>{
        const adj = (0,paneUtils/* getAdjacentLeaf */.fJ)(state.root, state.focusedPaneId, "ArrowRight");
        if (adj) dispatch({
            type: "FOCUS_PANE",
            paneId: adj.id
        });
    }, [
        dispatch,
        state.root,
        state.focusedPaneId
    ]);
    const focusLeft = (0,react.useCallback)(()=>{
        const adj = (0,paneUtils/* getAdjacentLeaf */.fJ)(state.root, state.focusedPaneId, "ArrowLeft");
        if (adj) dispatch({
            type: "FOCUS_PANE",
            paneId: adj.id
        });
    }, [
        dispatch,
        state.root,
        state.focusedPaneId
    ]);
    const focusUp = (0,react.useCallback)(()=>{
        const adj = (0,paneUtils/* getAdjacentLeaf */.fJ)(state.root, state.focusedPaneId, "ArrowUp");
        if (adj) dispatch({
            type: "FOCUS_PANE",
            paneId: adj.id
        });
    }, [
        dispatch,
        state.root,
        state.focusedPaneId
    ]);
    const focusDown = (0,react.useCallback)(()=>{
        const adj = (0,paneUtils/* getAdjacentLeaf */.fJ)(state.root, state.focusedPaneId, "ArrowDown");
        if (adj) dispatch({
            type: "FOCUS_PANE",
            paneId: adj.id
        });
    }, [
        dispatch,
        state.root,
        state.focusedPaneId
    ]);
    // ── Resize nudge ───────────────────────────────────────────────────────────
    const nudgeRight = (0,react.useCallback)(()=>{
        var _containerRef_current;
        var _containerRef_current_getBoundingClientRect_width;
        const containerSizePx = (_containerRef_current_getBoundingClientRect_width = (_containerRef_current = containerRef.current) === null || _containerRef_current === void 0 ? void 0 : _containerRef_current.getBoundingClientRect().width) !== null && _containerRef_current_getBoundingClientRect_width !== void 0 ? _containerRef_current_getBoundingClientRect_width : 400;
        dispatch({
            type: "NUDGE_RESIZE",
            paneId: state.focusedPaneId,
            direction: "ArrowRight",
            amountPx: 20,
            containerSizePx
        });
    }, [
        dispatch,
        state.focusedPaneId,
        containerRef
    ]);
    const nudgeLeft = (0,react.useCallback)(()=>{
        var _containerRef_current;
        var _containerRef_current_getBoundingClientRect_width;
        const containerSizePx = (_containerRef_current_getBoundingClientRect_width = (_containerRef_current = containerRef.current) === null || _containerRef_current === void 0 ? void 0 : _containerRef_current.getBoundingClientRect().width) !== null && _containerRef_current_getBoundingClientRect_width !== void 0 ? _containerRef_current_getBoundingClientRect_width : 400;
        dispatch({
            type: "NUDGE_RESIZE",
            paneId: state.focusedPaneId,
            direction: "ArrowLeft",
            amountPx: 20,
            containerSizePx
        });
    }, [
        dispatch,
        state.focusedPaneId,
        containerRef
    ]);
    const nudgeUp = (0,react.useCallback)(()=>{
        var _containerRef_current;
        var _containerRef_current_getBoundingClientRect_height;
        const containerSizePx = (_containerRef_current_getBoundingClientRect_height = (_containerRef_current = containerRef.current) === null || _containerRef_current === void 0 ? void 0 : _containerRef_current.getBoundingClientRect().height) !== null && _containerRef_current_getBoundingClientRect_height !== void 0 ? _containerRef_current_getBoundingClientRect_height : 400;
        dispatch({
            type: "NUDGE_RESIZE",
            paneId: state.focusedPaneId,
            direction: "ArrowUp",
            amountPx: 20,
            containerSizePx
        });
    }, [
        dispatch,
        state.focusedPaneId,
        containerRef
    ]);
    const nudgeDown = (0,react.useCallback)(()=>{
        var _containerRef_current;
        var _containerRef_current_getBoundingClientRect_height;
        const containerSizePx = (_containerRef_current_getBoundingClientRect_height = (_containerRef_current = containerRef.current) === null || _containerRef_current === void 0 ? void 0 : _containerRef_current.getBoundingClientRect().height) !== null && _containerRef_current_getBoundingClientRect_height !== void 0 ? _containerRef_current_getBoundingClientRect_height : 400;
        dispatch({
            type: "NUDGE_RESIZE",
            paneId: state.focusedPaneId,
            direction: "ArrowDown",
            amountPx: 20,
            containerSizePx
        });
    }, [
        dispatch,
        state.focusedPaneId,
        containerRef
    ]);
    // ── Swap ───────────────────────────────────────────────────────────────────
    const swapRight = (0,react.useCallback)(()=>{
        const adj = (0,paneUtils/* getAdjacentLeaf */.fJ)(state.root, state.focusedPaneId, "ArrowRight");
        if (adj) dispatch({
            type: "SWAP_PANES",
            paneId: state.focusedPaneId,
            targetPaneId: adj.id
        });
    }, [
        dispatch,
        state.root,
        state.focusedPaneId
    ]);
    const swapLeft = (0,react.useCallback)(()=>{
        const adj = (0,paneUtils/* getAdjacentLeaf */.fJ)(state.root, state.focusedPaneId, "ArrowLeft");
        if (adj) dispatch({
            type: "SWAP_PANES",
            paneId: state.focusedPaneId,
            targetPaneId: adj.id
        });
    }, [
        dispatch,
        state.root,
        state.focusedPaneId
    ]);
    const swapUp = (0,react.useCallback)(()=>{
        const adj = (0,paneUtils/* getAdjacentLeaf */.fJ)(state.root, state.focusedPaneId, "ArrowUp");
        if (adj) dispatch({
            type: "SWAP_PANES",
            paneId: state.focusedPaneId,
            targetPaneId: adj.id
        });
    }, [
        dispatch,
        state.root,
        state.focusedPaneId
    ]);
    const swapDown = (0,react.useCallback)(()=>{
        const adj = (0,paneUtils/* getAdjacentLeaf */.fJ)(state.root, state.focusedPaneId, "ArrowDown");
        if (adj) dispatch({
            type: "SWAP_PANES",
            paneId: state.focusedPaneId,
            targetPaneId: adj.id
        });
    }, [
        dispatch,
        state.root,
        state.focusedPaneId
    ]);
    // ── Zoom ───────────────────────────────────────────────────────────────────
    const zoomPane = (0,react.useCallback)(()=>{
        dispatch({
            type: "ZOOM_PANE",
            paneId: state.focusedPaneId
        });
    }, [
        dispatch,
        state.focusedPaneId
    ]);
    // ── Register all shortcuts ─────────────────────────────────────────────────
    (0,useShortcut/* useShortcut */.w)("cockpit.split-vertical", {
        key: "\\",
        modifiers: {
            ctrl: true
        },
        label: "Split pane vertically",
        context: "cockpit",
        action: splitVertical
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.split-horizontal", {
        key: "-",
        modifiers: {
            ctrl: true
        },
        label: "Split pane horizontally",
        context: "cockpit",
        action: splitHorizontal
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.close-pane", {
        key: "w",
        modifiers: {
            ctrl: true
        },
        label: "Close focused pane",
        context: "cockpit",
        action: closePane
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.focus-right", {
        key: "ArrowRight",
        modifiers: {
            ctrl: true
        },
        label: "Focus pane right",
        context: "cockpit",
        action: focusRight
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.focus-left", {
        key: "ArrowLeft",
        modifiers: {
            ctrl: true
        },
        label: "Focus pane left",
        context: "cockpit",
        action: focusLeft
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.focus-up", {
        key: "ArrowUp",
        modifiers: {
            ctrl: true
        },
        label: "Focus pane up",
        context: "cockpit",
        action: focusUp
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.focus-down", {
        key: "ArrowDown",
        modifiers: {
            ctrl: true
        },
        label: "Focus pane down",
        context: "cockpit",
        action: focusDown
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.resize-right", {
        key: "ArrowRight",
        modifiers: {
            ctrl: true,
            alt: true
        },
        label: "Resize right",
        context: "cockpit",
        action: nudgeRight
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.resize-left", {
        key: "ArrowLeft",
        modifiers: {
            ctrl: true,
            alt: true
        },
        label: "Resize left",
        context: "cockpit",
        action: nudgeLeft
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.resize-up", {
        key: "ArrowUp",
        modifiers: {
            ctrl: true,
            alt: true
        },
        label: "Resize up",
        context: "cockpit",
        action: nudgeUp
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.resize-down", {
        key: "ArrowDown",
        modifiers: {
            ctrl: true,
            alt: true
        },
        label: "Resize down",
        context: "cockpit",
        action: nudgeDown
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.zoom-pane", {
        key: "z",
        modifiers: {
            ctrl: true
        },
        label: "Zoom/unzoom focused pane",
        context: "cockpit",
        action: zoomPane
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.swap-right", {
        key: "ArrowRight",
        modifiers: {
            ctrl: true,
            shift: true
        },
        label: "Swap pane right",
        context: "cockpit",
        action: swapRight
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.swap-left", {
        key: "ArrowLeft",
        modifiers: {
            ctrl: true,
            shift: true
        },
        label: "Swap pane left",
        context: "cockpit",
        action: swapLeft
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.swap-up", {
        key: "ArrowUp",
        modifiers: {
            ctrl: true,
            shift: true
        },
        label: "Swap pane up",
        context: "cockpit",
        action: swapUp
    });
    (0,useShortcut/* useShortcut */.w)("cockpit.swap-down", {
        key: "ArrowDown",
        modifiers: {
            ctrl: true,
            shift: true
        },
        label: "Swap pane down",
        context: "cockpit",
        action: swapDown
    });
}

// EXTERNAL MODULE: ./src/components/sessions/SessionDetail.tsx + 7 modules
var SessionDetail = __webpack_require__(36313);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/chevron-right.mjs
var chevron_right = __webpack_require__(18776);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/chevron-down.mjs
var chevron_down = __webpack_require__(14784);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@tanstack+react-virtual@3.13.25+498059a1009c1789/node_modules/@tanstack/react-virtual/dist/esm/index.js + 3 modules
var esm = __webpack_require__(28396);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-virtuoso@4.18.7+498059a1009c1789/node_modules/react-virtuoso/dist/index.mjs
var dist = __webpack_require__(58362);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
// EXTERNAL MODULE: ./src/components/ui/Tooltip.tsx + 1 modules
var Tooltip = __webpack_require__(90646);
// EXTERNAL MODULE: ./src/components/sessions/ReviewQueueBadge.tsx + 1 modules
var ReviewQueueBadge = __webpack_require__(68152);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FRetryBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xbGxjdG1rMCB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDRweDsKICBwYWRkaW5nOiA0cHggMTJweDsKICBib3JkZXItcmFkaXVzOiAxMnB4OwogIGZvbnQtc2l6ZTogMTJweDsKICBmb250LXdlaWdodDogNjAwOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0KLl8xbGxjdG1rMSB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoybik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mm8pOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MnApOwp9Ci5fMWxsY3RtazIgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MmspOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejJsKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyejJtKTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FRetryBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xbGxjdG1rMCB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDRweDsKICBwYWRkaW5nOiA0cHggMTJweDsKICBib3JkZXItcmFkaXVzOiAxMnB4OwogIGZvbnQtc2l6ZTogMTJweDsKICBmb250LXdlaWdodDogNjAwOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7Cn0KLl8xbGxjdG1rMSB7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnoybik7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mm8pOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6MnApOwp9Ci5fMWxsY3RtazIgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MmspOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejJsKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyejJtKTsKfQ_22_7D = __webpack_require__(42597);
;// ./src/components/sessions/RetryBadge.css.ts


var badge = '_1llctmk0';
var neutral = '_1llctmk1';
var warning = '_1llctmk2';
;// ./src/components/sessions/RetryBadge.tsx
/* __next_internal_client_entry_do_not_use__ RetryBadge auto */ 

/**
 * Compact "🔁 N/max" badge shown once a session has consumed at least one
 * automated retry attempt (session-retry-backoff, AC4). Renders nothing at
 * attempt 0 — the majority case (healthy sessions) should see zero visual
 * noise, per research/ux.md.
 */ function RetryBadge(param) {
    let { retryAttempt, retryMaxAttempts, compact = false } = param;
    if (retryAttempt <= 0) {
        return null;
    }
    const isFinalAttempt = retryMaxAttempts > 0 && retryAttempt >= retryMaxAttempts;
    const toneClass = isFinalAttempt ? warning : neutral;
    const label = "Retry attempt ".concat(retryAttempt, " of ").concat(retryMaxAttempts);
    if (compact) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
            className: "".concat(badge, " ").concat(toneClass),
            role: "img",
            "aria-label": label,
            title: label,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    "aria-hidden": "true",
                    children: "\uD83D\uDD01"
                }),
                retryAttempt,
                "/",
                retryMaxAttempts
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: "".concat(badge, " ").concat(toneClass),
        role: "img",
        "aria-label": label,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                "aria-hidden": "true",
                children: "\uD83D\uDD01"
            }),
            label
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FRevivedContextBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xYzVpZ3hkMCB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDRweDsKICBwYWRkaW5nOiAzcHggMTBweDsKICBib3JkZXItcmFkaXVzOiAxMnB4OwogIGZvbnQtc2l6ZTogMC43NXJlbTsKICBmb250LXdlaWdodDogNjAwOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnp5KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyengpOwp9Ci5fMWM1aWd4ZDEgewogIGZvbnQtc2l6ZTogMC44MTI1cmVtOwogIGxpbmUtaGVpZ2h0OiAxOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FRevivedContextBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xYzVpZ3hkMCB7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDRweDsKICBwYWRkaW5nOiAzcHggMTBweDsKICBib3JkZXItcmFkaXVzOiAxMnB4OwogIGZvbnQtc2l6ZTogMC43NXJlbTsKICBmb250LXdlaWdodDogNjAwOwogIHdoaXRlLXNwYWNlOiBub3dyYXA7CiAgYmFja2dyb3VuZDogdmFyKC0tXzE5ZXoycnp5KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnp6KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyengpOwp9Ci5fMWM1aWd4ZDEgewogIGZvbnQtc2l6ZTogMC44MTI1cmVtOwogIGxpbmUtaGVpZ2h0OiAxOwp9_22_7D = __webpack_require__(64874);
;// ./src/components/sessions/RevivedContextBadge.css.ts


var icon = '_1c5igxd1';
var wrapper = '_1c5igxd0';
;// ./src/components/sessions/RevivedContextBadge.tsx
/* __next_internal_client_entry_do_not_use__ hasLostContext,RevivedContextBadge auto */ 


// hasLostContext is the single source of truth for "this session's last
// restart lost its conversation history" — shared with SessionRow's aria-label
// extension so the two surfaces can never drift on the condition.
function hasLostContext(session) {
    return session.reviveOutcome === types_pb/* ReviveOutcome */.eS.FRESH_LOST_HISTORY;
}
// RevivedContextBadge renders only when the session's tmux pane restarted and
// its previous conversation history could not be recovered from disk
// (session-revive-uuid-loss AC3) — a durable, at-a-glance signal that survives
// the toast notification auto-closing.
function RevivedContextBadge(param) {
    let { session } = param;
    if (!hasLostContext(session)) {
        return null;
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
        className: wrapper,
        role: "status",
        "aria-live": "polite",
        "aria-label": "This session lost its previous conversation and started fresh",
        title: "This session lost its previous conversation and started fresh",
        "data-testid": "revived-context-badge",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: "⚠"
            }),
            "Context lost"
        ]
    });
}

// EXTERNAL MODULE: ./src/components/sessions/StatusBadge.tsx + 1 modules
var StatusBadge = __webpack_require__(40793);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSubStatusChip.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA82VzW7iMBSF9zzF3YwEC6Mk0BbMptI8yMjYN4lLYlu2U35GffdRHChuKCSbGc0K4Xvwd45jcl53eMwtq9FBky2Xep_A7wlA8iN8AHjLlMu1rSlY7ZnHaSKwmG0mAB8TgDR5oFw8R9qPybwDpEEvpDMVO1KQqpIKSV7hoRWyShaKSI-1o8BRebTtcsEMhaUJEsOEkKqgkJkDrLq1rbYCLbFMyMZReGd2SsivdI2nzJ4W6-Ah18oTJ094M2fX-R5lUfobRRkU-1J6JM4wjhSU3ltm2uUQoDz_MJ0vw2YVHogrrVQ7CsnmGj8L8beM7wqrGyX6qGMgcV1p2x-dZteoFFJzAKcrKXqqwyyiLQZo6ct9XD0KV8e45RAuvY9Ls1G8NImBT3_7NI-DVyOP_TwP-OEP8o_ygzHtZYCWqfu4TI_iZSbItGFc-iOFZL6KDKyGDOADA_k4A0WceD0AfL_Pa0bhmpu4TxGeDeUtH-SV4_K-xXm3_-T_O3zB7x8J_08cfhoS3_bLttJ8F97hUviSQpp0xfH52k7iIum6pTPDG2tR-Z9tgKhqvDbkHCo0n2GtanQX3dTDa41CMpgaizlaRyyKhqMgtfZSq7ZuSDdCxXEWMn4JDMCUrFkrJorVSC99vukNRWNZt2UyX7n-VHrsxoTrRvn2_HKppMe-0MtaqoLkjeLdbu0xM3sp-6E43fdvc1wegdLqTI1vV_9xfLmZz2f8H6uZOCrWCAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSubStatusChip_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA82VzW7iMBSF9zzF3YwEC6Mk0BbMptI8yMjYN4lLYlu2U35GffdRHChuKCSbGc0K4Xvwd45jcl53eMwtq9FBky2Xep_A7wlA8iN8AHjLlMu1rSlY7ZnHaSKwmG0mAB8TgDR5oFw8R9qPybwDpEEvpDMVO1KQqpIKSV7hoRWyShaKSI_1o8BRebTtcsEMhaUJEsOEkKqgkJkDrLq1rbYCLbFMyMZReGd2SsivdI2nzJ4W6_Ah18oTJ094M2fX_R5lUfobRRkU_1J6JM4wjhSU3ltm2uUQoDz_MJ0vw2YVHogrrVQ7CsnmGj8L8beM7wqrGyX6qGMgcV1p2x_dZteoFFJzAKcrKXqqwyyiLQZo6ct9XD0KV8e45RAuvY9Ls1G8NImBT3_7NI_DVyOP_TwP_OEP8o_ygzHtZYCWqfu4TI_iZSbItGFc_iOFZL6KDKyGDOADA_k4A0WceD0AfL_Pa0bhmpu4TxGeDeUtH_SV4_K_xXm3_T_O3zB7x8J_08cfhoS3_bLttJ8F97hUviSQpp0xfH52k7iIum6pTPDG2tR_Z9tgKhqvDbkHCo0n2GtanQX3dTDa41CMpgaizlaRyyKhqMgtfZSq7ZuSDdCxXEWMn4JDMCUrFkrJorVSC99vukNRWNZt2UyX7n_VHrsxoTrRvn2_HKppMe_0MtaqoLkjeLdbu0xM3sp_6E43fdvc1wegdLqTI1vV_9xfLmZz2f8H6uZOCrWCAAA_22_7D = __webpack_require__(37637);
;// ./src/components/sessions/SubStatusChip.css.ts

var chip = 'u244ow1';
var chipCompacting = 'u244owc u244ow1';
var chipError = 'u244ow4 u244ow1';
var chipIdle = 'u244ow7 u244ow1';
var chipInputRequired = 'u244ow8 u244ow1';
var chipNeedsApproval = 'u244ow2 u244ow1';
var chipProcessing = 'u244ow3 u244ow1';
var chipRateLimited = 'u244ow6 u244ow1';
var chipReady = 'u244ow9 u244ow1';
var chipSuccess = 'u244owa u244ow1';
var chipTestsFailing = 'u244ow5 u244ow1';
var chipWaitingForAgent = 'u244owb u244ow1';
var spinner = 'u244owd';
;// ./src/components/sessions/SubStatusChip.tsx
/* __next_internal_client_entry_do_not_use__ SubStatusChip auto */ 


/**
 * SubStatusChip renders a small inline chip showing fine-grained session activity.
 * Returns null for UNSPECIFIED only.
 *
 * Note: SessionRow filters out READY before rendering this component — that state
 * is intentionally suppressed in the list view as low-signal noise. IDLE is no longer
 * suppressed (Epic 3.2.2, ADR-002): idle-reason items were removed from the Review
 * Queue entirely, so this chip is now the Sessions-list's only "ready for next task"
 * signal. Direct callers (e.g. detail headers) may still render READY chips.
 */ function SubStatusChip(param) {
    let { subStatus, subagentCount } = param;
    // Guard against undefined subStatus (e.g. when session proto field is not set)
    if (subStatus === undefined || subStatus === null) return null;
    switch(subStatus){
        case types_pb/* SubStatus */.BJ.WAITING_FOR_AGENT:
            {
                // Only trust a finite, positive count — undefined/0/negative/NaN all fall back to
                // the plain unnumbered chip text.
                const hasCount = typeof subagentCount === "number" && Number.isFinite(subagentCount) && subagentCount > 0;
                const isSingular = hasCount && subagentCount === 1;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                    className: chipWaitingForAgent,
                    role: "status",
                    "aria-label": "Waiting for agents",
                    title: // The count comes from three distinct WaitingForAgent sources (background
                    // agents, shells still running, monitors still running) collapsed into one
                    // int — "task" is deliberately source-neutral rather than "agent", which
                    // would be wrong when the match was actually a shell/monitor count.
                    hasCount ? "Claude is waiting for ".concat(subagentCount, " background task").concat(isSingular ? "" : "s", " to finish") : "Claude is waiting for background agents to finish",
                    children: [
                        "⏳ Waiting for ",
                        hasCount ? "".concat(subagentCount, " ").concat(isSingular ? "Task" : "Tasks") : "Agents"
                    ]
                });
            }
        case types_pb/* SubStatus */.BJ.PROCESSING:
            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: chipProcessing,
                role: "status",
                "aria-label": "Session is processing",
                title: "Claude is actively working",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: spinner,
                        "aria-hidden": "true"
                    }),
                    "Thinking…"
                ]
            });
        case types_pb/* SubStatus */.BJ.NEEDS_APPROVAL:
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: chipNeedsApproval,
                role: "status",
                "aria-label": "Needs approval",
                title: "Waiting for your approval on a tool request",
                children: "⚠ Approve Tool Use"
            });
        case types_pb/* SubStatus */.BJ.INPUT_REQUIRED:
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: chipInputRequired,
                role: "status",
                "aria-label": "Input needed",
                title: "Waiting for you to type a response or select an option",
                children: "⌨ Your Input Needed"
            });
        case types_pb/* SubStatus */.BJ.ERROR:
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: chipError,
                role: "status",
                "aria-label": "Error",
                title: "Session encountered an error",
                children: "✖ Error"
            });
        case types_pb/* SubStatus */.BJ.TESTS_FAILING:
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: chipTestsFailing,
                role: "status",
                "aria-label": "Tests failing",
                title: "Tests are currently failing",
                children: "⚠ Tests Failing"
            });
        case types_pb/* SubStatus */.BJ.RATE_LIMITED:
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: chipRateLimited,
                role: "status",
                "aria-label": "Rate limited",
                title: "Session is experiencing API rate limiting",
                children: "⏱ Rate Limited"
            });
        case types_pb/* SubStatus */.BJ.IDLE:
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: chipIdle,
                role: "status",
                "aria-label": "Session is idle",
                title: "Session is idle — waiting for your input",
                children: "● Idle"
            });
        case types_pb/* SubStatus */.BJ.READY:
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: chipReady,
                role: "status",
                "aria-label": "Ready for your next instruction",
                title: "Session is at the prompt — ready for your next message",
                children: "● Ready"
            });
        case types_pb/* SubStatus */.BJ.SUCCESS:
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: chipSuccess,
                role: "status",
                "aria-label": "Task complete",
                title: "Task completed successfully",
                children: "✓ Done"
            });
        case types_pb/* SubStatus */.BJ.COMPACTING:
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: chipCompacting,
                role: "status",
                "aria-label": "Compacting context",
                title: "Claude is summarizing older conversation history to free up context space",
                children: "⟳ Compacting context"
            });
        case types_pb/* SubStatus */.BJ.UNSPECIFIED:
            return null;
        default:
            {
                // Proto enums are forward-compatible: a newer server can send a
                // SubStatus value this deployed client bundle doesn't know about yet.
                // Render nothing rather than throwing (via assertNever), so one
                // unrecognized wire value can't crash the sessions UI. `_exhaustive:
                // never` still gives a compile error if a new case is added without
                // also being handled here.
                const _exhaustive = subStatus;
                console.warn("SubStatusChip: unrecognized SubStatus value", _exhaustive);
                return null;
            }
    }
}

// EXTERNAL MODULE: ./src/components/shared/GitHubBadge.tsx
var GitHubBadge = __webpack_require__(20621);
// EXTERNAL MODULE: ./src/components/shared/BacklogOriginBadge.tsx + 1 modules
var BacklogOriginBadge = __webpack_require__(97443);
// EXTERNAL MODULE: ./src/components/sessions/TagEditor.tsx + 1 modules
var TagEditor = __webpack_require__(95580);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/ansi-to-html@0.7.2/node_modules/ansi-to-html/lib/ansi_to_html.js
var ansi_to_html = __webpack_require__(64957);
var ansi_to_html_default = /*#__PURE__*/__webpack_require__.n(ansi_to_html);
;// ./src/lib/hooks/useTerminalSnapshot.ts
/* __next_internal_client_entry_do_not_use__ useTerminalSnapshot auto */ 




// Module-level cache shared across all card instances with the same sessionId.
// Prevents thundering-herd on mount when the session list renders 20+ cards at once.
const snapshotCache = new Map();
const CACHE_TTL_MS = 5000;
// Randomize each card's refresh cadence by up to ±20% so N cards' timers don't
// stay phase-locked and fire their refetch in the same synchronized burst forever.
const CACHE_TTL_JITTER_MS = CACHE_TTL_MS * 0.2;
const LAST_N_LINES = 20;
function getCached(sessionId) {
    const entry = snapshotCache.get(sessionId);
    if (entry && Date.now() - entry.timestamp < CACHE_TTL_MS) return entry;
    return null;
}
// Shared client, built once and reused by every card — this hook is
// instantiated per visible session card, polling every 5s.
let sharedClient = null;
function getSharedClient() {
    if (!sharedClient) {
        sharedClient = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }
    return sharedClient;
}
// Singleton converter — ansi-to-html's Convert holds no per-call state, so
// constructing a fresh instance (plus a dynamic require()) on every render
// was pure overhead at polling scale.
const ansiConverter = new (ansi_to_html_default())({
    escapeXML: true
});
/** Render ANSI escape sequences as HTML spans. Falls back to plain text on error. */ function renderAnsi(raw) {
    try {
        return ansiConverter.toHtml(raw);
    } catch (e) {
        // Plain text fallback — escape HTML entities to prevent XSS
        return raw.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }
}
/**
 * Fetches and caches the last N lines of terminal output for a session card preview.
 * - Does not block card render (async on mount)
 * - 5-second TTL shared across all cards for the same session
 * - ANSI escape codes rendered as HTML; plain text fallback on failure
 */ function useTerminalSnapshot(sessionId) {
    let enabled = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : true;
    const hit = getCached(sessionId);
    var _hit_html;
    const [html, setHtml] = (0,react.useState)((_hit_html = hit === null || hit === void 0 ? void 0 : hit.html) !== null && _hit_html !== void 0 ? _hit_html : "");
    var _hit_isEmpty;
    const [isEmpty, setIsEmpty] = (0,react.useState)((_hit_isEmpty = hit === null || hit === void 0 ? void 0 : hit.isEmpty) !== null && _hit_isEmpty !== void 0 ? _hit_isEmpty : false);
    const [loading, setLoading] = (0,react.useState)(!hit && enabled);
    const [error, setError] = (0,react.useState)(null);
    const fetch = (0,react.useCallback)(async function() {
        let skipCache = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false;
        if (!enabled) return;
        if (!skipCache) {
            const cached = getCached(sessionId);
            if (cached) {
                setHtml(cached.html);
                setIsEmpty(cached.isEmpty);
                setLoading(false);
                return;
            }
        }
        setLoading(true);
        setError(null);
        try {
            const client = getSharedClient();
            const response = await client.getTerminalSnapshot({
                sessionId,
                lastNLines: LAST_N_LINES
            });
            const renderedHtml = response.isEmpty ? "" : renderAnsi(response.content);
            const entry = {
                html: renderedHtml,
                isEmpty: response.isEmpty,
                timestamp: Date.now()
            };
            snapshotCache.set(sessionId, entry);
            setHtml(renderedHtml);
            setIsEmpty(response.isEmpty);
        } catch (err) {
            setError(err instanceof Error ? err.message : "Failed to load snapshot");
        } finally{
            setLoading(false);
        }
    }, [
        sessionId,
        enabled
    ]);
    const fetchRef = (0,react.useRef)(fetch);
    fetchRef.current = fetch;
    (0,react.useEffect)(()=>{
        if (!enabled) return;
        let timeoutId;
        const scheduleNext = ()=>{
            // Jitter each card's next tick independently so N cards mounted in the
            // same render burst don't stay phase-locked and refetch in lockstep forever.
            const jitter = (Math.random() * 2 - 1) * CACHE_TTL_JITTER_MS;
            timeoutId = setTimeout(()=>{
                fetchRef.current(true);
                scheduleNext();
            }, CACHE_TTL_MS + jitter);
        };
        fetch();
        scheduleNext();
        return ()=>clearTimeout(timeoutId);
    }, [
        fetch,
        enabled
    ]);
    return {
        html,
        isEmpty,
        loading,
        error,
        refetch: ()=>fetch(true)
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/useSessionActions.ts
var useSessionActions = __webpack_require__(21155);
;// ./src/lib/hooks/useCreationLifecycleActions.ts
/* __next_internal_client_entry_do_not_use__ useCreationLifecycleActions auto */ 

/**
 * Cancel/Retry lifecycle guards for an in-progress or failed session
 * creation (Epic 5.4, async-session-creation). Shared by SessionCard.tsx and
 * SessionRow.tsx, which previously each carried an identical copy of this
 * ref+state pattern (same as Omnibar.tsx's isSubmittingRef): the ref blocks a
 * second click before React re-renders with the disabled attribute, the
 * state actually disables/re-enables the button. Reset once the stream
 * carries the session past the in-flight status (Creating after a
 * successful Retry; anything other than Creating after a lost-race Cancel)
 * so a later failure/creation cycle on the SAME card/row gets a fresh guard
 * rather than staying disabled forever.
 */ function useCreationLifecycleActions(sessionId, isCreating) {
    const sessionActions = (0,useSessionActions/* useSessionActions */.J)(sessionId);
    const cancelInFlightRef = (0,react.useRef)(false);
    const [cancelDisabled, setCancelDisabled] = (0,react.useState)(false);
    const retryInFlightRef = (0,react.useRef)(false);
    const [retryDisabled, setRetryDisabled] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        if (!isCreating) {
            cancelInFlightRef.current = false;
            setCancelDisabled(false);
        }
    }, [
        isCreating
    ]);
    (0,react.useEffect)(()=>{
        if (isCreating) {
            retryInFlightRef.current = false;
            setRetryDisabled(false);
        }
    }, [
        isCreating
    ]);
    const handleCancelCreation = async (e)=>{
        e.stopPropagation();
        if (cancelInFlightRef.current) return;
        cancelInFlightRef.current = true;
        setCancelDisabled(true);
        const result = await sessionActions.cancelCreation();
        if (!result.success) {
            // Either a lost-race FailedPrecondition or a transport error -- the
            // card/row either already got its real status from the stream (lost
            // race) or is still Creating (transport error), so re-enable Cancel.
            cancelInFlightRef.current = false;
            setCancelDisabled(false);
        }
    // On success the instance is deleted server-side; the card/row is
    // removed from the store (dispatch(removeSession) in useSessionService)
    // and this component unmounts, so no further local state update is
    // needed.
    };
    const handleRetryCreation = async (e)=>{
        e.stopPropagation();
        if (retryInFlightRef.current) return;
        retryInFlightRef.current = true;
        setRetryDisabled(true);
        const ok = await sessionActions.retryCreation();
        if (!ok) {
            // FailedPrecondition (no longer Failed) or transport error -- let the
            // user try again rather than leaving Retry disabled indefinitely.
            retryInFlightRef.current = false;
            setRetryDisabled(false);
        }
    // On success the same instance flips Failed -> Creating server-side;
    // the effect above clears the guard once that status arrives.
    };
    return {
        cancelDisabled,
        retryDisabled,
        handleCancelCreation,
        handleRetryCreation
    };
}

;// ./src/lib/utils/sessionFailure.ts
/**
 * Canonical FailureReason taxonomy set server-side by the Background
 * Resolution Pipeline (session.Instance.FailureReason, see ADR-001) and
 * carried on the wire via Session.failure_reason (types_pb.ts). Both the
 * persistent card/row copy (getFailureMessage below) and the transient toast
 * copy (NotificationContext.tsx's getFailureReasonToastMessage) key off this
 * same set of reasons — importing it there guarantees the two switches never
 * silently drift apart on *which* reasons are handled, even though the two
 * contexts intentionally use different wording.
 */ /**
 * Reason-specific copy for the persistent Failed-state card/row message
 * (async-session-creation Epic 5.2/plan.md Story 5.2.2 / UX research §2/§4's
 * "three different messages, not one generic Failed card"). Shared by
 * SessionCard.tsx and SessionRow.tsx so the two views never show conflicting
 * copy for the same FailureReason.
 */ function getFailureMessage(failureReason) {
    switch(failureReason){
        case "GitHubResolutionError":
            return "Failed to resolve GitHub URL.";
        case "StartupError":
            return "Failed to start session.";
        case "Stale":
            return "This session creation appears to have stalled.";
        case "WorktreeResolutionFailed":
            return "Failed to create an isolated worktree.";
        case "DirectoryCollision":
            return "Another session is already active in this directory.";
        default:
            return "Session creation failed.";
    }
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
;// ./src/components/sessions/DetectionEventsPanel.tsx
/* __next_internal_client_entry_do_not_use__ DetectionEventsPanel auto */ 





const pad = (n)=>String(n).padStart(2, "0");
// Maps the DetectedStatus int value (from proto result_status) to the Go constant name.
// Mirrors the iota order in session/detection/detector.go.
const STATUS_INT_TO_GO = {
    0: "StatusUnknown",
    1: "StatusReady",
    2: "StatusProcessing",
    3: "StatusNeedsApproval",
    4: "StatusInputRequired",
    5: "StatusError",
    6: "StatusTestsFailing",
    7: "StatusIdle",
    8: "StatusExecuting",
    9: "StatusSuccess"
};
/**
 * DetectionEventsPanel renders the last 20 status-detection events for a session.
 * Only shown when ?debug=1 is present in the URL — useful for diagnosing why
 * a session isn't showing the expected status chip.
 *
 * The "Capture Test Case" button fetches the current terminal content and
 * generates a ready-to-paste testdata fixture + snapshot_test.go snippet.
 */ function DetectionEventsPanel(param) {
    let { sessionId, program = "claude" } = param;
    const clientRef = (0,react.useRef)(null);
    const [events, setEvents] = (0,react.useState)([]);
    const [error, setError] = (0,react.useState)(null);
    const [capturing, setCapturing] = (0,react.useState)(false);
    const [captureResult, setCaptureResult] = (0,react.useState)(null);
    const [copiedField, setCopiedField] = (0,react.useState)(null);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
    }, []);
    (0,react.useEffect)(()=>{
        if (!clientRef.current) return;
        let cancelled = false;
        const run = async ()=>{
            if (!clientRef.current) return;
            try {
                const response = await clientRef.current.getDetectionEvents({
                    sessionId,
                    limit: 20
                });
                if (!cancelled) {
                    setEvents(response.events);
                    setError(null);
                }
            } catch (e) {
                if (!cancelled) setError(String(e));
            }
        };
        void run();
        const id = setInterval(()=>void run(), 3000);
        return ()=>{
            cancelled = true;
            clearInterval(id);
        };
    }, [
        sessionId
    ]);
    const handleCapture = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        setCapturing(true);
        setCaptureResult(null);
        try {
            const resp = await clientRef.current.getTerminalSnapshot({
                sessionId,
                lastNLines: 120
            });
            const latestEvent = events[0];
            var _latestEvent_resultStatus;
            const statusInt = (_latestEvent_resultStatus = latestEvent === null || latestEvent === void 0 ? void 0 : latestEvent.resultStatus) !== null && _latestEvent_resultStatus !== void 0 ? _latestEvent_resultStatus : 0;
            var _latestEvent_matchedPattern;
            const patternName = (_latestEvent_matchedPattern = latestEvent === null || latestEvent === void 0 ? void 0 : latestEvent.matchedPattern) !== null && _latestEvent_matchedPattern !== void 0 ? _latestEvent_matchedPattern : "<none>";
            var _STATUS_INT_TO_GO_statusInt;
            // Suggest a filename based on program + status + timestamp
            const statusLabel = ((_STATUS_INT_TO_GO_statusInt = STATUS_INT_TO_GO[statusInt]) !== null && _STATUS_INT_TO_GO_statusInt !== void 0 ? _STATUS_INT_TO_GO_statusInt : "unknown").replace("Status", "").toLowerCase();
            const ts = new Date();
            const stamp = "".concat(ts.getFullYear()).concat(pad(ts.getMonth() + 1)).concat(pad(ts.getDate()), "_").concat(pad(ts.getHours())).concat(pad(ts.getMinutes())).concat(pad(ts.getSeconds()));
            const suggestedFilename = "".concat(program, "_").concat(statusLabel, "_").concat(stamp, ".txt");
            setCaptureResult({
                content: resp.content,
                statusInt,
                patternName,
                suggestedFilename
            });
        } catch (e) {
            setError("Capture failed: ".concat(String(e)));
        } finally{
            setCapturing(false);
        }
    }, [
        sessionId,
        events,
        program
    ]);
    const copyToClipboard = (0,react.useCallback)((text, field)=>{
        void navigator.clipboard.writeText(text).then(()=>{
            setCopiedField(field);
            setTimeout(()=>setCopiedField(null), 2000);
        });
    }, []);
    var _STATUS_INT_TO_GO_captureResult_statusInt;
    const goStatusName = captureResult ? (_STATUS_INT_TO_GO_captureResult_statusInt = STATUS_INT_TO_GO[captureResult.statusInt]) !== null && _STATUS_INT_TO_GO_captureResult_statusInt !== void 0 ? _STATUS_INT_TO_GO_captureResult_statusInt : "StatusUnknown" : "";
    const goSnippet = captureResult ? '{\n	fixture:     "'.concat(captureResult.suggestedFilename, '",\n	expected:    ').concat(goStatusName, ',\n	program:     "').concat(program, '",\n	description: "FILL IN: describe what state the session is in",\n},') : "";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        style: {
            marginTop: "16px",
            fontFamily: "monospace",
            fontSize: "11px",
            border: "1px solid var(--border-color)",
            borderRadius: "6px",
            padding: "12px",
            background: "var(--card-background)"
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                style: {
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    marginBottom: "8px"
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h4", {
                        style: {
                            fontSize: "12px",
                            fontWeight: 600,
                            opacity: 0.7,
                            margin: 0
                        },
                        children: "Detection Events (debug)"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        onClick: ()=>void handleCapture(),
                        disabled: capturing,
                        style: {
                            fontSize: "11px",
                            padding: "3px 8px",
                            borderRadius: "4px",
                            border: "1px solid var(--border-color)",
                            background: "var(--card-background)",
                            color: "var(--text-primary)",
                            cursor: capturing ? "not-allowed" : "pointer",
                            opacity: capturing ? 0.5 : 1
                        },
                        children: capturing ? "Capturing…" : "📸 Capture test case"
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                style: {
                    color: "var(--error)",
                    padding: "4px 0"
                },
                children: [
                    "Error: ",
                    error
                ]
            }),
            events.length === 0 && !error ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                style: {
                    color: "var(--text-secondary)",
                    opacity: 0.6
                },
                children: "No events yet — detection runs on a polling interval."
            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("table", {
                style: {
                    width: "100%",
                    borderCollapse: "collapse"
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("thead", {
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                            style: {
                                opacity: 0.6
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    style: {
                                        textAlign: "left",
                                        padding: "3px 6px",
                                        fontWeight: 500
                                    },
                                    children: "Time"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    style: {
                                        textAlign: "left",
                                        padding: "3px 6px",
                                        fontWeight: 500
                                    },
                                    children: "Pattern"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    style: {
                                        textAlign: "left",
                                        padding: "3px 6px",
                                        fontWeight: 500
                                    },
                                    children: "Category"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    style: {
                                        textAlign: "left",
                                        padding: "3px 6px",
                                        fontWeight: 500
                                    },
                                    children: "Status"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("th", {
                                    style: {
                                        textAlign: "left",
                                        padding: "3px 6px",
                                        fontWeight: 500
                                    },
                                    children: "Snippet"
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("tbody", {
                        children: events.map((e, i)=>{
                            var _STATUS_INT_TO_GO_e_resultStatus;
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("tr", {
                                style: {
                                    borderTop: "1px solid var(--border-color)"
                                },
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        style: {
                                            padding: "3px 6px",
                                            whiteSpace: "nowrap",
                                            opacity: 0.7
                                        },
                                        children: e.timestamp ? new Date(Number(e.timestamp.seconds) * 1000).toLocaleTimeString() : "—"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        style: {
                                            padding: "3px 6px"
                                        },
                                        children: e.matchedPattern
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        style: {
                                            padding: "3px 6px",
                                            opacity: 0.8
                                        },
                                        children: e.matchedCategory
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        style: {
                                            padding: "3px 6px"
                                        },
                                        children: (_STATUS_INT_TO_GO_e_resultStatus = STATUS_INT_TO_GO[e.resultStatus]) !== null && _STATUS_INT_TO_GO_e_resultStatus !== void 0 ? _STATUS_INT_TO_GO_e_resultStatus : e.resultStatus
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("td", {
                                        style: {
                                            padding: "3px 6px",
                                            maxWidth: "260px",
                                            overflow: "hidden",
                                            textOverflow: "ellipsis",
                                            whiteSpace: "nowrap",
                                            opacity: 0.6
                                        },
                                        title: e.tailSnippet,
                                        children: e.tailSnippet.slice(0, 80)
                                    })
                                ]
                            }, i);
                        })
                    })
                ]
            }),
            captureResult && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                style: {
                    marginTop: "12px",
                    borderTop: "1px solid var(--border-color)",
                    paddingTop: "12px"
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        style: {
                            marginBottom: "8px",
                            opacity: 0.8
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                children: "Captured:"
                            }),
                            " ",
                            goStatusName,
                            " (pattern: ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                children: captureResult.patternName
                            }),
                            ")"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        style: {
                            marginBottom: "10px"
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                style: {
                                    display: "flex",
                                    alignItems: "center",
                                    justifyContent: "space-between",
                                    marginBottom: "4px"
                                },
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        style: {
                                            opacity: 0.7
                                        },
                                        children: [
                                            "1. Save as",
                                            " ",
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                                                style: {
                                                    background: "var(--hover-background)",
                                                    padding: "1px 4px",
                                                    borderRadius: "3px"
                                                },
                                                children: [
                                                    "session/detection/testdata/",
                                                    captureResult.suggestedFilename
                                                ]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: ()=>copyToClipboard(captureResult.content, "content"),
                                        style: copyBtnStyle,
                                        children: copiedField === "content" ? "Copied!" : "Copy"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                style: {
                                    margin: 0,
                                    padding: "8px",
                                    background: "var(--terminal-background, #1a1a1a)",
                                    color: "var(--terminal-foreground, #d4d4d4)",
                                    borderRadius: "4px",
                                    fontSize: "10px",
                                    maxHeight: "180px",
                                    overflowY: "auto",
                                    whiteSpace: "pre-wrap",
                                    wordBreak: "break-all"
                                },
                                children: captureResult.content
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                style: {
                                    display: "flex",
                                    alignItems: "center",
                                    justifyContent: "space-between",
                                    marginBottom: "4px"
                                },
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        style: {
                                            opacity: 0.7
                                        },
                                        children: [
                                            "2. Add to",
                                            " ",
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                style: {
                                                    background: "var(--hover-background)",
                                                    padding: "1px 4px",
                                                    borderRadius: "3px"
                                                },
                                                children: "session/detection/snapshot_test.go"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: ()=>copyToClipboard(goSnippet, "snippet"),
                                        style: copyBtnStyle,
                                        children: copiedField === "snippet" ? "Copied!" : "Copy"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                style: {
                                    margin: 0,
                                    padding: "8px",
                                    background: "var(--hover-background)",
                                    borderRadius: "4px",
                                    fontSize: "10px",
                                    whiteSpace: "pre"
                                },
                                children: goSnippet
                            })
                        ]
                    })
                ]
            })
        ]
    });
}
const copyBtnStyle = {
    fontSize: "10px",
    padding: "2px 6px",
    borderRadius: "3px",
    border: "1px solid var(--border-color)",
    background: "var(--card-background)",
    color: "var(--text-primary)",
    cursor: "pointer",
    flexShrink: 0
};

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/ellipsis.mjs
var ellipsis = __webpack_require__(97963);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/pin-off.mjs
var pin_off = __webpack_require__(21613);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/pin.mjs
var pin = __webpack_require__(27341);
// EXTERNAL MODULE: ./src/components/sessions/CreatePullRequestModal.tsx + 1 modules
var CreatePullRequestModal = __webpack_require__(14653);
// EXTERNAL MODULE: ./src/components/sessions/ExportBundleModal.tsx + 3 modules
var ExportBundleModal = __webpack_require__(34892);
// EXTERNAL MODULE: ./src/lib/contexts/AnalyticsContext.tsx
var AnalyticsContext = __webpack_require__(24044);
// EXTERNAL MODULE: ./src/lib/hooks/useAvailablePrograms.ts
var useAvailablePrograms = __webpack_require__(56230);
// EXTERNAL MODULE: ./src/lib/sessions/autoApprove.ts
var autoApprove = __webpack_require__(88242);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fanimations.css.ts.vanilla.css%22%2C%22source%22%3A%22QGtleWZyYW1lcyBfMXZvMmxuYjAgewogIDAlLCAxMDAlIHsKICAgIGJveC1zaGFkb3c6IDAgMCAwIDAgdHJhbnNwYXJlbnQ7CiAgfQogIDUwJSB7CiAgICBib3gtc2hhZG93OiAwIDAgOHB4IDJweCB2YXIoLS1fMTllejJyejI0KSwgMCAwIDE2cHggNHB4IHZhcigtLV8xOWV6MnJ6MjUpOwogIH0KfQpAa2V5ZnJhbWVzIF8xdm8ybG5iMSB7CiAgMCUsIDEwMCUgewogICAgdGV4dC1zaGFkb3c6IDAgMCA0cHggdmFyKC0tXzE5ZXoycnoyNCk7CiAgfQogIDUwJSB7CiAgICB0ZXh0LXNoYWRvdzogMCAwIDEycHggdmFyKC0tXzE5ZXoycnoyNCksIDAgMCAyNHB4IHZhcigtLV8xOWV6MnJ6MjUpOwogIH0KfQpAa2V5ZnJhbWVzIF8xdm8ybG5iMiB7CiAgZnJvbSB7CiAgICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVkoMTZweCk7CiAgICBvcGFjaXR5OiAwOwogIH0KICB0byB7CiAgICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVkoMCk7CiAgICBvcGFjaXR5OiAxOwogIH0KfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fanimations_css_ts_vanilla_css_22_2C_22source_22_3A_22QGtleWZyYW1lcyBfMXZvMmxuYjAgewogIDAlLCAxMDAlIHsKICAgIGJveC1zaGFkb3c6IDAgMCAwIDAgdHJhbnNwYXJlbnQ7CiAgfQogIDUwJSB7CiAgICBib3gtc2hhZG93OiAwIDAgOHB4IDJweCB2YXIoLS1fMTllejJyejI0KSwgMCAwIDE2cHggNHB4IHZhcigtLV8xOWV6MnJ6MjUpOwogIH0KfQpAa2V5ZnJhbWVzIF8xdm8ybG5iMSB7CiAgMCUsIDEwMCUgewogICAgdGV4dC1zaGFkb3c6IDAgMCA0cHggdmFyKC0tXzE5ZXoycnoyNCk7CiAgfQogIDUwJSB7CiAgICB0ZXh0LXNoYWRvdzogMCAwIDEycHggdmFyKC0tXzE5ZXoycnoyNCksIDAgMCAyNHB4IHZhcigtLV8xOWV6MnJ6MjUpOwogIH0KfQpAa2V5ZnJhbWVzIF8xdm8ybG5iMiB7CiAgZnJvbSB7CiAgICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVkoMTZweCk7CiAgICBvcGFjaXR5OiAwOwogIH0KICB0byB7CiAgICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVkoMCk7CiAgICBvcGFjaXR5OiAxOwogIH0KfQ_22_7D = __webpack_require__(61793);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionCard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-0ca2_bOPJ7fwWvwGLjg-UT6WdUHFBgP-99PxwOC1qibTYSqVJUbOfQ_34QqQcpUQ87abYFFk0TWyKHw3lzZqTPT-R6EDghGYj2K7zPffC_DwAcBE_UBwB4ikMqrwHwP6nvUmCWHbhIAv0xxpL8-2GXXmbF_W8fAJC8PRUOTfXLid8-fG4jA4eRGVjMCQ61wbkRQn65mcH1hjYzDRs2gI7gEkvy4Efk2KVrd9xyY4z89mGhV1iq8XscPh0Fz1kUgGcsHjzvD_hIXpB42aoJey4iIgIA0wvIeEyj1ihijPIEjmietQEtd2pMiqOIsmPnLlR3EyyOlHl7LiVPOmM04cJcZFwEIOWUSSKKS2qzVFLOggqJkMdcAH-BMkBwRuZgzy9edsIRP5tXSx4Af7GsLtWUMy8m-OKdCD2eZHNVbYdX6woSY0mf1VXvTPZPVHoSp96JHk9xMVGjVEpCigVhUm2HM4kpI8KT15QEgLKYMuJl9IXYtxlOSAAykmWUs9-wiIrbmNEEFwiUt0sFtW9FucAaSX-xXGf2TUkTyo7eIWehHlNtrRlyoHHsJTwiBXXlqQWcxPgagBDH4UNC2YPmWYhF5FEWkcsc-LM5WM_A38HGT7LZJ0P2ghN_JkJLoMG2NuNpKV0VAwPgA5RewC69AHHc4wd_DsqfBZzNga_-FbJqw0Frc_WVWrfR3sWqFiWtN1mIY_LgLx5NHXBjCH0Hin6JpkIRLR_nYLPT__2FQgVUMuyRZ8JkFgDGGTFQXCsUS5XxYnKQAVij9GIM2YxSL9Go9Wo43JpU2Zrw9IqrHqVvA_Zogo8kAIX8YuEdCztAmHyQHIhCAebdhduXtjMTl12bQ9u1cfdRE6dWQLzPeJxLJbsacSWTHTsDPFB5IsnTAKz9X1qMN2y1t_Z_se3WriC_wwoZeFGW5vI_hTr_89fwRMKnPb_8-l-F7plG8hSAwn0UULRJab73Q8Vq-rh9rCfs1YSIZqnSz0NM1Apf8kzSw9Ur7AphMgBZikPi7Yk8E8KUasf0yDwqSZIFICSVjR1cGr2YS4cGrqVbPHAmlU0LAFxAtBYkqS-fSyJsfF9bPIcQW1uLvg_8ISXZTXKDyQQ3uLHFqTAQG818nstCdSorUAsL9AsJrXdP3IztYdsRp8PMOtjgSvdzB9TrkH9H1xmApZD3U1nTz8mgrxNI-zhriULr_n7WJxNjjE1Nih1_IIqF_RSDfyrFiEmx0_tT7EfYNh2JrUeYV8P5MgLn2g_nMhq6XMyVnu7HGJlwYn1saQjsLza1UTbB69hovZ6D5pe_WHZtpFMQplja0m_piMABpcY5MT2sigwcZrNf-F5GPIiFscZm2SNEl8mb63iB4q8XUUHKYD7kcZ6wWp02VuzIulzaNb5zwKjoKfIaF-cViWMaGlC5GbQ6dRR9Xx2V5CI9I5zL05SIUJ1qaiTTEUFHpwFJp6bYfB2DRAYgHUxIYgzS0wCk2ISU3W8wXiYFOpbRkGN4iwG8rQNhPgIJwgGLaRmg5zGc2ABO3IR0fi1O49SEVnR7eVfmXU2rJ7QerSwr8aJG9K5Xj4O-08vvYx4-jXjtG03pK4Oeze2WZV1Ylman8LUheJnKGnU2ypyfRQGg-G3igNw4dGY4LT9c3s2rKsKynftu6zxyrcuo6X7rOM1NWAnAerEm0Wdt3cw69av1zpLtlZEW0ZoV4exE2qp17EhuJ9s3FjrB9VT_OYkDmy4HpqAEJ1mSKefdx76krZ03NzmI49ido4XrDjsrbtrUM1PrxtADD_PMO1N5omzKDLgupzzTjO5jMjhyilSNHHAbcJtb0z1wO2APpgaEcOcG0pdEaAePpQg28B77fQfssbIAJJR5TerDRhD3A9QmtWDCIS4SsCcaRTqjpQLC5gaJY5pmVOXBzycqiadyYEXepW1k9_3LaVYq0BEJeZVhr3I3ro2ZMt6qT1iLGqLkXDp1L52ziIjCjpvAwskcneyTip0dcELjawASzrii3gRhiPp3lPcE881kMiBJ_ujsw-0Oe6WNrEmOTcfuOiOH8QM9PDYnrynZSdQw3H2uKTLesigkFLxQbsNf-GvDHTjRxRZOp2FdbQbSN84sV_UNh8gtfdPDqAGTMi_wy13WcCg9AJ_ap-V7mR-_bQIXJq1CSFOJbMYwO6jYpJdWqux1td3tSMS-7aeN3_YjcAFdgcy21I9uINEW_TI8UhFtVWCBlWOpLqwq_W6cTXXplmCSTXL7dizJW_w60AuJ2o5vN5r6fevKvF28LGqraOUqsOrK5Yuu7ioX7VtHhpKKN-heU1wva3bQ959PwANwUxbqKuftXQOAc8lNcqZvok52ha-Vo2rc-q1qMCXW7mhA4zxLo6_2EqiqZo8OWKWiARF2RNAwvUOG0yCiGd7HJGrXaNcmhowXyMf8XAh4M3sgtrHTOFBMO6zZBjbrGrtOFfhHtHwm313x40jni3VssuihiRgwLh9qvs0m8Hu8LcNcxJKICteIHHAeS_vAt9iYM6XNrpXhm9y56WbqqxOGE9o6jNXuoSNcji-znHoszG0if9-NWxzbTVDr5z7HpgI3_1PdmKE-lsnGsqymT7gj2SLsLnp0Is_G3Pd4gY73cvdRwU-9fVRoWhtVQ5yxtO57NdqhWeVyq7DHL5Wt_P6oXYgdDhQKCVa-Ix7Q8Nz0Q_30W95Ov9PS7vsAftesg7EWDyvQ7NiTC0itNTqnkQH4sN-cNwtcB4-x4G80SbmQWEcLLh_gsojWtGatF7PjqIoMarFQHG2HOW8QRb65JxxvPJrSAGoRRmf1dOTSbrqZ0ErX6fJbGnoBIZoDtFrqZkRjWdSqj9TJ9tGkxsBx0y2o6EUJarN0T8FiWhaoak7rmFkV0RNm-gCEBrPXL7Ma2hRhmhIbbe6JjSantl-dyUXonqCBjXpo1lrkFeE4Wv4sPLvfttzUt4OW7xAwo9VPT_XSRo32602tfKPV9wywzWVeoy1rK0IokgU-8B18qro6HQWJ26Rxc1OCYzwN6zxpTQ1MKzc5noOwRHI7IJB35rUHgrA8K6aQmISy01OPNkZKwTzgrO1BjsJfFSqgQZ9RDvP44ZARGQDPbthHW0dvXKWv7ZxlM0tX5upblW3oJsZ6srxW1ebj74TF_OMcfPydMxyqT7_xXFAiwL_I-eN8oK6zRU5cteoMaGk0cCDV4aVVi0sF8aoGhjMXkbcXBD8FQP0p1NIkzqObOHerTHPL3vqE1rgxPRpsnUP4Z9lIg_L-nVBGgyiP9e9v7TOxX-uFmV-tlm4216qeTmtc7uvLGCjtqSnPWFDMpMfyhAgaBkAWnbRYFBcyE6uBQuplrBSKbiykbu3Zh-7jQUNdrM3E92ua3405NDgQLb5F0zx-m-5x9H5d884z9ZQmutcTeSzncBOZyxyJs6nj5tYQI0VWWrWGNfRPZs1rm0PfX7Qv7jawhqRf_pL2e6S9IeDTXwS8kYCtEcduFNrpCEPxz0zlN2kdv88-jJA1GWmuwWPiMfkxH8SHW58rk7-yn9CtvlcUGj4DNg1LPU_9t_i41k94fk5IRDF4SAU5EJF5gkR5SCIv4VVnT_FdJ0Ss9zf0VVl8mChf5nhg39d3vk1YlnFP3yKstfrOfNtEmUDqvFLhU_luCushj27J6A_4zFHM9n4H4XpDqLMZZ9nIo8zjuWyPpZJoQF7I8-KoQdmBMirbCCLeg2H1To5-BP3FbhxF_Xz8jdg1XDLiku2meJLdZEhYom6Wucrjh7HDaNKoso_C3TfVriQ5rdXg8a7_yTP3qczGrZKio6CRp1uJAgDBP4AH2wTTOUWVB7JoVXZ-O1_EUr92w3zhhkV7tPFbtIeVNvR3OpWuoKy6mfvB5dyWnRQJjvXEOtOjH-7A7Ho-EUEMejVhbBjTtKPcOuQqsMEiI7P5iCTBrwPc__bh2_8BkD8AsRZIAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionCard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_0ca2_bOPJ7fwWvwGLjg_UT6WdUHFBgP_99PxwOC1qibTYSqVJUbOfQ_34QqQcpUQ87abYFFk0TWyKHw3lzZqTPT_R6EDghGYj2K7zPffC_DwAcBE_UBwB4ikMqrwHwP6nvUmCWHbhIAv0xxpL8_2GXXmbF_W8fAJC8PRUOTfXLid8_fG4jA4eRGVjMCQ61wbkRQn65mcH1hjYzDRs2gI7gEkvy4Efk2KVrd9xyY4z89mGhV1iq8XscPh0Fz1kUgGcsHjzvD_hIXpB42aoJey4iIgIA0wvIeEyj1ihijPIEjmietQEtd2pMiqOIsmPnLlR3EyyOlHl7LiVPOmM04cJcZFwEIOWUSSKKS2qzVFLOggqJkMdcAH_BMkBwRuZgzy9edsIRP5tXSx4Af7GsLtWUMy8m_OKdCD2eZHNVbYdX6woSY0mf1VXvTPZPVHoSp96JHk9xMVGjVEpCigVhUm2HM4kpI8KT15QEgLKYMuJl9IXYtxlOSAAykmWUs9_wiIrbmNEEFwiUt0sFtW9FucAaSX_xXGf2TUkTyo7eIWehHlNtrRlyoHHsJTwiBXXlqQWcxPgagBDH4UNC2YPmWYhF5FEWkcsc_LM5WM_A38HGT7LZJ0P2ghN_JkJLoMG2NuNpKV0VAwPgA5RewC69AHHc4wd_DsqfBZzNga_FbJqw0Frc_WVWrfR3sWqFiWtN1mIY_LgLx5NHXBjCH0Hin6JpkIRLR_nYLPT_2FQgVUMuyRZ8JkFgDGGTFQXCsUS5XxYnKQAVij9GIM2YxSL9Go9Wo43JpU2Zrw9IqrHqVvA_Zogo8kAIX8YuEdCztAmHyQHIhCAebdhduXtjMTl12bQ9u1cfdRE6dWQLzPeJxLJbsacSWTHTsDPFB5IsnTAKz9X1qMN2y1t_Z_se3WriC_wwoZeFGW5vI_hTr_89fwRMKnPb_8_l_F7plG8hSAwn0UULRJab73Q8Vq_rh9rCfs1YSIZqnSz0NM1Apf8kzSw9Ur7AphMgBZikPi7Yk8E8KUasf0yDwqSZIFICSVjR1cGr2YS4cGrqVbPHAmlU0LAFxAtBYkqS_fSyJsfF9bPIcQW1uLvg_8ISXZTXKDyQQ3uLHFqTAQG818nstCdSorUAsL9AsJrXdP3IztYdsRp8PMOtjgSvdzB9TrkH9H1xmApZD3U1nTz8mgrxNI_zhriULr_n7WJxNjjE1Nih1_IIqF_RSDfyrFiEmx0_tT7EfYNh2JrUeYV8P5MgLn2g_nMhq6XMyVnu7HGJlwYn1saQjsLza1UTbB69hovZ6D5pe_WHZtpFMQplja0m_piMABpcY5MT2sigwcZrNf_F5GPIiFscZm2SNEl8mb63iB4q8XUUHKYD7kcZ6wWp02VuzIulzaNb5zwKjoKfIaF_cViWMaGlC5GbQ6dRR9Xx2V5CI9I5zL05SIUJ1qaiTTEUFHpwFJp6bYfB2DRAYgHUxIYgzS0wCk2ISU3W8wXiYFOpbRkGN4iwG8rQNhPgIJwgGLaRmg5zGc2ABO3IR0fi1O49SEVnR7eVfmXU2rJ7QerSwr8aJG9K5Xj4O_08vvYx4_jXjtG03pK4Oeze2WZV1Ylman8LUheJnKGnU2ypyfRQGg_G3igNw4dGY4LT9c3s2rKsKynftu6zxyrcuo6X7rOM1NWAnAerEm0Wdt3cw69av1zpLtlZEW0ZoV4exE2qp17EhuJ9s3FjrB9VT_OYkDmy4HpqAEJ1mSKefdx76krZ03NzmI49ido4XrDjsrbtrUM1PrxtADD_PMO1N5omzKDLgupzzTjO5jMjhyilSNHHAbcJtb0z1wO2APpgaEcOcG0pdEaAePpQg28B77fQfssbIAJJR5TerDRhD3A9QmtWDCIS4SsCcaRTqjpQLC5gaJY5pmVOXBzycqiadyYEXepW1k9_3LaVYq0BEJeZVhr3I3ro2ZMt6qT1iLGqLkXDp1L52ziIjCjpvAwskcneyTip0dcELjawASzrii3gRhiPp3lPcE881kMiBJ_ujsw_0Oe6WNrEmOTcfuOiOH8QM9PDYnrynZSdQw3H2uKTLesigkFLxQbsNf_GvDHTjRxRZOp2FdbQbSN84sV_UNh8gtfdPDqAGTMi_wy13WcCg9AJ_ap_V7mR_bQIXJq1CSFOJbMYwO6jYpJdWqux1td3tSMS_7aeN3_YjcAFdgcy21I9uINEW_TI8UhFtVWCBlWOpLqwq_W6cTXXplmCSTXL7dizJW_w60AuJ2o5vN5r6fevKvF28LGqraOUqsOrK5Yuu7ioX7VtHhpKKN_heU1wva3bQ959PwANwUxbqKuftXQOAc8lNcqZvok52ha_Vo2rc_q1qMCXW7mhA4zxLo6_2EqiqZo8OWKWiARF2RNAwvUOG0yCiGd7HJGrXaNcmhowXyMf8XAh4M3sgtrHTOFBMO6zZBjbrGrtOFfhHtHwm313x40jni3VssuihiRgwLh9qvs0m8Hu8LcNcxJKICteIHHAeS_vAt9iYM6XNrpXhm9y56WbqqxOGE9o6jNXuoSNcji_znHoszG0if9_NWxzbTVDr5z7HpgI3_1PdmKE_lsnGsqymT7gj2SLsLnp0Is_G3Pd4gY73cvdRwU_9fVRoWhtVQ5yxtO57NdqhWeVyq7DHL5Wt_P6oXYgdDhQKCVa_Ix7Q8Nz0Q_30W95Ov9PS7vsAftesg7EWDyvQ7NiTC0itNTqnkQH4sN_cNwtcB4_x4G80SbmQWEcLLh_gsojWtGatF7PjqIoMarFQHG2HOW8QRb65JxxvPJrSAGoRRmf1dOTSbrqZ0ErX6fJbGnoBIZoDtFrqZkRjWdSqj9TJ9tGkxsBx0y2o6EUJarN0T8FiWhaoak7rmFkV0RNm_gCEBrPXL7Ma2hRhmhIbbe6JjSantl_dyUXonqCBjXpo1lrkFeE4Wv4sPLvfttzUt4OW7xAwo9VPT_XSRo32602tfKPV9wywzWVeoy1rK0IokgU_8B18qro6HQWJ26Rxc1OCYzwN6zxpTQ1MKzc5noOwRHI7IJB35rUHgrA8K6aQmISy01OPNkZKwTzgrO1BjsJfFSqgQZ9RDvP44ZARGQDPbthHW0dvXKWv7ZxlM0tX5upblW3oJsZ6srxW1ebj74TF_OMcfPydMxyqT7_xXFAiwL_I_eN8oK6zRU5cteoMaGk0cCDV4aVVi0sF8aoGhjMXkbcXBD8FQP0p1NIkzqObOHerTHPL3vqE1rgxPRpsnUP4Z9lIg_L_nVBGgyiP9e9v7TOxX_uFmV_tlm4216qeTmtc7uvLGCjtqSnPWFDMpMfyhAgaBkAWnbRYFBcyE6uBQuplrBSKbiykbu3Zh_7jQUNdrM3E92ua3405NDgQLb5F0zx_m_5x9H5d884z9ZQmutcTeSzncBOZyxyJs6nj5tYQI0VWWrWGNfRPZs1rm0PfX7Qv7jawhqRf_pL2e6S9IeDTXwS8kYCtEcduFNrpCEPxz0zlN2kdv88_jJA1GWmuwWPiMfkxH8SHW58rk7_yn9CtvlcUGj4DNg1LPU_9t_i41k94fk5IRDF4SAU5EJF5gkR5SCIv4VVnT_FdJ0Ss9zf0VVl8mChf5nhg39d3vk1YlnFP3yKstfrOfNtEmUDqvFLhU_luCushj27J6A_4zFHM9n4H4XpDqLMZZ9nIo8zjuWyPpZJoQF7I8_KoQdmBMirbCCLeg2H1To5_BP3FbhxF_Xz8jdg1XDLiku2meJLdZEhYom6Wucrjh7HDaNKoso_C3TfVriQ5rdXg8a7_yTP3qczGrZKio6CRp1uJAgDBP4AH2wTTOUWVB7JoVXZ_O1_EUr92w3zhhkV7tPFbtIeVNvR3OpWuoKy6mfvB5dyWnRQJjvXEOtOjH_7A7Ho_EUEMejVhbBjTtKPcOuQqsMEiI7P5iCTBrwPc_bh2_8BkD8AsRZIAAA_22_7D = __webpack_require__(51531);
;// ./src/components/sessions/SessionActionsOverflow.css.ts



var actionButton = 'db4abu1s';
var cancelButton = 'db4abu23';
var confirmDialog = 'db4abu1v';
var dangerButton = 'db4abu24';
var desktopActions = 'db4abu1l';
var dialogActions = 'db4abu21';
var dialogContent = 'db4abu1x';
var errorMessage = 'db4abu20';
var overflowButton = 'db4abu1n';
var overflowContainer = 'db4abu1m';
var overflowMenu = 'db4abu1o';
var overflowMenuItem = 'db4abu1p';
var overflowMenuItemDanger = 'db4abu1r';
var overflowMenuItemTouch = 'db4abu1q';
var renameDialog = 'db4abu1v';
var renameInput = 'db4abu1z';
var submitButton = 'db4abu22';
var warningText = 'db4abu1y';
;// ./src/components/sessions/SessionActionsOverflow.tsx
/* __next_internal_client_entry_do_not_use__ fitMenuTop,menuActionLabel,SessionActionsOverflow auto */ 
// +feature: session-change-program session-pin-toggle













const VIEWPORT_MARGIN = 8;
/** Top for a fixed menu: below the anchor if it fits, else flipped above it, clamped on-screen. */ function fitMenuTop(top, anchorTop, height, viewportHeight) {
    if (top + height <= viewportHeight - VIEWPORT_MARGIN) return top;
    return Math.max(VIEWPORT_MARGIN, anchorTop - height);
}
/** Collapsible group inside the overflow menu; one section open at a time keeps the menu short. */ function MenuSection(param) {
    let { id, label, openId, onToggle, children } = param;
    const open = openId === id;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                role: "menuitem",
                className: overflowMenuItem,
                "aria-expanded": open,
                "aria-controls": "overflow-section-".concat(id),
                onClick: (e)=>{
                    e.stopPropagation();
                    onToggle(id);
                },
                style: {
                    justifyContent: "space-between"
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: label
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "aria-hidden": "true",
                        children: open ? "▾" : "▸"
                    })
                ]
            }),
            open && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: "overflow-section-".concat(id),
                role: "group",
                "aria-label": label,
                style: {
                    paddingLeft: 8
                },
                children: children
            })
        ]
    });
}
/** Stable action label from a menu item's visible text: "🔀 Create PR" -> "create-pr", "View PR #12" -> "view-pr". */ function menuActionLabel(text) {
    return text.replace(/#\d+/g, "").replace(RegExp("[^\\p{L}\\p{N}\\s-]", "gu"), "").trim().toLowerCase().replace(/\s+/g, "-");
}
const menuSeparator = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
    role: "separator",
    style: {
        height: 1,
        background: "var(--border-color)",
        margin: "4px 0"
    }
});
const SessionActionsOverflow = /*#__PURE__*/ (0,react.forwardRef)(function SessionActionsOverflow(param, ref) {
    let { session, showPrimaryAction = false, buttonClassName, onResume, onPause, onHibernate, onResumeFromHibernation, onDelete, onRestart, onRetryNow, onClone, onOpenInNewPane, onNewWorkspace, onCreateCheckpoint, onSetRateLimitEnabled, onToggleAutonomousMode, onTogglePinned, onToggleAutoApprove, onSteerAutonomousSession, onClearConversationState, onUpdateTags, onRenameRequest, onWorkspaceSwitchRequest, onChangeProgram } = param;
    var _useContext;
    const isPaused = session.status === types_pb/* SessionStatus */.Ph.PAUSED;
    const isReady = session.status === types_pb/* SessionStatus */.Ph.NEEDS_APPROVAL;
    const isRunning = session.status === types_pb/* SessionStatus */.Ph.ACTIVE; // ACTIVE covers legacy RUNNING (same wire value via allow_alias)
    const isHibernated = session.status === types_pb/* SessionStatus */.Ph.HIBERNATED;
    const isCreating = session.status === types_pb/* SessionStatus */.Ph.CREATING;
    const isStopped = session.status === types_pb/* SessionStatus */.Ph.STOPPED;
    // session-retry-backoff: a terminal give-up state distinct from STOPPED —
    // never eligible for Resume/Restart, only "Retry now" (AC6).
    const isPermanentlyFailed = session.status === types_pb/* SessionStatus */.Ph.PERMANENTLY_FAILED;
    // A session mid-automated-backoff-wait (scheduled retry pending, not yet
    // exhausted) — "Retry now" here skips the wait rather than reviving a
    // terminal state.
    const isMidBackoffWait = !isPermanentlyFailed && !!session.nextRetryAt;
    // Optional (not useAnalytics) so the menu still renders outside an AnalyticsContextProvider.
    const track = (_useContext = (0,react.useContext)(AnalyticsContext/* AnalyticsContext */.Ig)) === null || _useContext === void 0 ? void 0 : _useContext.track;
    const trackMenu = (0,react.useCallback)((name, action)=>{
        track === null || track === void 0 ? void 0 : track({
            name,
            category: "user_action",
            component: "SessionActionsOverflow",
            sessionId: session.id,
            ...action ? {
                labels: {
                    action
                }
            } : {}
        });
    }, [
        track,
        session.id
    ]);
    const [showOverflow, setShowOverflow] = (0,react.useState)(false);
    const [menuPos, setMenuPos] = (0,react.useState)({
        top: 0,
        right: 0,
        anchorTop: 0
    });
    const [openSection, setOpenSection] = (0,react.useState)(null);
    const toggleSection = (0,react.useCallback)((id)=>setOpenSection((cur)=>cur === id ? null : id), []);
    const [fitTop, setFitTop] = (0,react.useState)(null);
    const [isRestartConfirmOpen, setIsRestartConfirmOpen] = (0,react.useState)(false);
    const [isRestarting, setIsRestarting] = (0,react.useState)(false);
    const [restartError, setRestartError] = (0,react.useState)("");
    const [isRetryConfirmOpen, setIsRetryConfirmOpen] = (0,react.useState)(false);
    const [isRetrying, setIsRetrying] = (0,react.useState)(false);
    const [retryError, setRetryError] = (0,react.useState)("");
    const [isDeleteConfirmOpen, setIsDeleteConfirmOpen] = (0,react.useState)(false);
    const [isDeleting, setIsDeleting] = (0,react.useState)(false);
    const [deleteError, setDeleteError] = (0,react.useState)("");
    const [isCheckpointOpen, setIsCheckpointOpen] = (0,react.useState)(false);
    const [isExportBundleOpen, setIsExportBundleOpen] = (0,react.useState)(false);
    const [checkpointLabel, setCheckpointLabel] = (0,react.useState)("");
    const [isCreatingCheckpoint, setIsCreatingCheckpoint] = (0,react.useState)(false);
    const [checkpointError, setCheckpointError] = (0,react.useState)("");
    const [isTagEditorOpen, setIsTagEditorOpen] = (0,react.useState)(false);
    const [isCreatePrOpen, setIsCreatePrOpen] = (0,react.useState)(false);
    const [isAutonomousConfirmOpen, setIsAutonomousConfirmOpen] = (0,react.useState)(false);
    // Auto-approve toggle restart notice. SetAutoApprove (session/instance_actor_setters.go)
    // restarts an Active session unconditionally in EITHER direction (the flag is baked into
    // the launch command at spawn time), so unlike autonomous mode's enable-only friction,
    // this dialog is shown for both enabling and disabling on an Active session.
    const [isAutoApproveConfirmOpen, setIsAutoApproveConfirmOpen] = (0,react.useState)(false);
    const [pendingAutoApproveValue, setPendingAutoApproveValue] = (0,react.useState)(false);
    const [isSteerOpen, setIsSteerOpen] = (0,react.useState)(false);
    const [steerMessage, setSteerMessage] = (0,react.useState)("");
    const [isSteering, setIsSteering] = (0,react.useState)(false);
    const [isClearConversationConfirmOpen, setIsClearConversationConfirmOpen] = (0,react.useState)(false);
    const [isProgramPickerOpen, setIsProgramPickerOpen] = (0,react.useState)(false);
    const [programPickerValue, setProgramPickerValue] = (0,react.useState)(session.program || "");
    const [isSavingProgram, setIsSavingProgram] = (0,react.useState)(false);
    const [programError, setProgramError] = (0,react.useState)("");
    const [isProgramRestartConfirmOpen, setIsProgramRestartConfirmOpen] = (0,react.useState)(false);
    const [pendingProgramValue, setPendingProgramValue] = (0,react.useState)("");
    const availablePrograms = (0,useAvailablePrograms/* useAvailablePrograms */.U)();
    const { draftPullRequest, createPullRequest } = (0,SessionServiceContext/* useSessionServiceContext */.MS)();
    // Keep the picker's selected value in sync with the session while the dialog is
    // open — otherwise a concurrent server-side change (e.g. the capacity-monitor
    // auto-fallback switching programs) leaves a stale dialog whose Save would
    // silently clobber that change back to the value it had when opened.
    (0,react.useEffect)(()=>{
        if (isProgramPickerOpen) setProgramPickerValue(session.program || "");
    }, [
        session.program,
        isProgramPickerOpen
    ]);
    const overflowContainerRef = (0,react.useRef)(null);
    const overflowButtonRef = (0,react.useRef)(null);
    const overflowMenuRef = (0,react.useRef)(null);
    const restartDialogRef = (0,react.useRef)(null);
    const retryDialogRef = (0,react.useRef)(null);
    const deleteDialogRef = (0,react.useRef)(null);
    const checkpointDialogRef = (0,react.useRef)(null);
    const autonomousConfirmDialogRef = (0,react.useRef)(null);
    const autoApproveConfirmDialogRef = (0,react.useRef)(null);
    const steerDialogRef = (0,react.useRef)(null);
    // Focus target while a steer RPC is in flight — see the onKeyDown/onClick
    // handlers below for why focus is moved here explicitly rather than left
    // to the browser's implicit auto-blur when the input becomes disabled.
    const steerCancelButtonRef = (0,react.useRef)(null);
    const clearConversationDialogRef = (0,react.useRef)(null);
    const programPickerDialogRef = (0,react.useRef)(null);
    const programConfirmDialogRef = (0,react.useRef)(null);
    const createPrTriggerRef = (0,react.useRef)(null);
    // All dialogs/menus spawned from this component return focus to overflowButtonRef
    // ("···") rather than the menu-item button that opened them — menu items unmount
    // when the overflow menu closes (close() flips showOverflow before the dialog's
    // state flips in the same batch), so a ref on the menu item itself is nulled by
    // React before useFocusTrap's cleanup can read it. overflowButtonRef stays mounted
    // for the component's whole lifetime.
    (0,useFocusTrap/* useFocusTrap */.r)(overflowMenuRef, showOverflow, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(restartDialogRef, isRestartConfirmOpen, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(retryDialogRef, isRetryConfirmOpen, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(deleteDialogRef, isDeleteConfirmOpen, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(checkpointDialogRef, isCheckpointOpen, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(autonomousConfirmDialogRef, isAutonomousConfirmOpen, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(autoApproveConfirmDialogRef, isAutoApproveConfirmOpen, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(steerDialogRef, isSteerOpen, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(clearConversationDialogRef, isClearConversationConfirmOpen, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(programPickerDialogRef, isProgramPickerOpen, overflowButtonRef);
    (0,useFocusTrap/* useFocusTrap */.r)(programConfirmDialogRef, isProgramRestartConfirmOpen, overflowButtonRef);
    (0,react.useLayoutEffect)(()=>{
        if (!showOverflow || !overflowMenuRef.current) return;
        const { height } = overflowMenuRef.current.getBoundingClientRect();
        setFitTop(fitMenuTop(menuPos.top, menuPos.anchorTop, height, window.innerHeight));
    }, [
        showOverflow,
        menuPos,
        openSection
    ]);
    (0,react.useEffect)(()=>{
        if (showOverflow && overflowMenuRef.current) {
            const first = overflowMenuRef.current.querySelector('[role="menuitem"],[role="menuitemcheckbox"]');
            first === null || first === void 0 ? void 0 : first.focus();
        }
    }, [
        showOverflow
    ]);
    (0,react.useEffect)(()=>{
        if (!showOverflow) return;
        const handler = (e)=>{
            var _overflowContainerRef_current, _overflowMenuRef_current;
            const target = e.target;
            const inContainer = (_overflowContainerRef_current = overflowContainerRef.current) === null || _overflowContainerRef_current === void 0 ? void 0 : _overflowContainerRef_current.contains(target);
            const inMenu = (_overflowMenuRef_current = overflowMenuRef.current) === null || _overflowMenuRef_current === void 0 ? void 0 : _overflowMenuRef_current.contains(target);
            if (!inContainer && !inMenu) {
                setShowOverflow(false);
            }
        };
        document.addEventListener("mousedown", handler);
        return ()=>document.removeEventListener("mousedown", handler);
    }, [
        showOverflow
    ]);
    (0,react.useImperativeHandle)(ref, ()=>({
            openAt (x, y) {
                setOpenSection(null);
                trackMenu("session_menu_open");
                setMenuPos({
                    top: y,
                    right: window.innerWidth - x,
                    anchorTop: y
                });
                setShowOverflow(true);
            }
        }), [
        trackMenu
    ]);
    const openMenu = (0,react.useCallback)((e)=>{
        e.stopPropagation();
        if (!overflowButtonRef.current) return;
        const rect = overflowButtonRef.current.getBoundingClientRect();
        setOpenSection(null);
        trackMenu("session_menu_open");
        setMenuPos({
            top: rect.bottom + 4,
            right: window.innerWidth - rect.right,
            anchorTop: rect.top - 4
        });
        setShowOverflow((o)=>!o);
    }, [
        trackMenu
    ]);
    // sendSteerMessage is the steer dialog's single submit path, shared by the
    // input's Enter-key handler and the Send button's onClick — both need the
    // identical sequence (disable input, move focus off it before the browser
    // auto-blurs it, deliver, then close/clear only on success). The try/catch
    // (in addition to the existing finally) treats a rejected promise the same
    // as an onSteerAutonomousSession that resolves to false — without it, a
    // caller that rejects rather than resolving false would surface as an
    // unhandled promise rejection from inside a DOM event handler.
    const sendSteerMessage = (0,react.useCallback)(async ()=>{
        var // Move focus to Cancel *before* the input's disabled prop takes effect,
        // so focus stays inside the dialog instead of the browser auto-blurring
        // it to document.body — that auto-blur would break both the Escape
        // handler and useFocusTrap's Tab-cycling (its focusable-elements
        // snapshot isn't re-evaluated when the input drops out of the tab
        // order).
        _steerCancelButtonRef_current;
        const message = steerMessage.trim();
        if (!message || isSteering) return;
        setIsSteering(true);
        (_steerCancelButtonRef_current = steerCancelButtonRef.current) === null || _steerCancelButtonRef_current === void 0 ? void 0 : _steerCancelButtonRef_current.focus();
        try {
            const ok = await (onSteerAutonomousSession === null || onSteerAutonomousSession === void 0 ? void 0 : onSteerAutonomousSession(session.id, message));
            if (ok !== false) {
                setIsSteerOpen(false);
                setSteerMessage("");
            }
        } catch (e) {
        // Treat a rejected promise the same as a resolved `false` — the steer
        // failed, so leave the dialog open with the message intact.
        } finally{
            setIsSteering(false);
        }
    }, [
        steerMessage,
        isSteering,
        onSteerAutonomousSession,
        session.id
    ]);
    const close = ()=>setShowOverflow(false);
    const handleRestartConfirm = async (e)=>{
        e.stopPropagation();
        setIsRestarting(true);
        setRestartError("");
        try {
            await (onRestart === null || onRestart === void 0 ? void 0 : onRestart(session.id));
            setIsRestartConfirmOpen(false);
        } catch (err) {
            setRestartError(err instanceof Error ? err.message : "Failed to restart session.");
        } finally{
            setIsRestarting(false);
        }
    };
    const handleRetryConfirm = async (e)=>{
        e.stopPropagation();
        setIsRetrying(true);
        setRetryError("");
        try {
            const success = await (onRetryNow === null || onRetryNow === void 0 ? void 0 : onRetryNow(session.id));
            // onRetryNow (useSessionService's retrySession) catches all errors
            // internally and resolves to false on failure rather than throwing, so
            // the result must be checked explicitly — only close the dialog on
            // success, otherwise surface an error instead of silently dismissing it.
            if (success === false) {
                setRetryError("Failed to retry session.");
            } else {
                setIsRetryConfirmOpen(false);
            }
        } catch (err) {
            // A concurrent retry already in flight (backend CAS guard) is not a
            // failure — the automated path already has this covered.
            if (err instanceof Error && /already in progress|failed_precondition/i.test(err.message)) {
                setIsRetryConfirmOpen(false);
            } else {
                setRetryError(err instanceof Error ? err.message : "Failed to retry session.");
            }
        } finally{
            setIsRetrying(false);
        }
    };
    const commitProgramChange = async (program)=>{
        setIsSavingProgram(true);
        setProgramError("");
        try {
            await (onChangeProgram === null || onChangeProgram === void 0 ? void 0 : onChangeProgram(session.id, program));
            return true;
        } catch (err) {
            setProgramError(err instanceof Error ? err.message : "Failed to change program.");
            return false;
        } finally{
            setIsSavingProgram(false);
        }
    };
    const handleCheckpointSubmit = async (e)=>{
        e.stopPropagation();
        if (!checkpointLabel.trim()) return;
        setIsCreatingCheckpoint(true);
        setCheckpointError("");
        try {
            const success = await (onCreateCheckpoint === null || onCreateCheckpoint === void 0 ? void 0 : onCreateCheckpoint(session.id, checkpointLabel.trim()));
            if (success) {
                setIsCheckpointOpen(false);
            } else {
                setCheckpointError("Failed to create checkpoint");
            }
        } catch (err) {
            setCheckpointError(err instanceof Error ? err.message : "Failed to create checkpoint");
        } finally{
            setIsCreatingCheckpoint(false);
        }
    };
    // Accordion section visibility. Resume/Pause/Hibernate, Change Program and Delete stay top-level.
    // Create/View PR item is now always rendered (no longer gated on a
    // caller-supplied callback prop — see Task 2.3.1a).
    const hasGroup2 = true;
    const hasOrganize = !!(onRenameRequest || onClone || onOpenInNewPane || onUpdateTags || onNewWorkspace || onWorkspaceSwitchRequest);
    const hasGroup4 = !!(onSetRateLimitEnabled || onToggleAutonomousMode || onToggleAutoApprove || onSteerAutonomousSession && session.autonomousMode);
    const hasMore = !!(onClearConversationState || onRestart && !isCreating || onRetryNow && (isPermanentlyFailed || isMidBackoffWait));
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            isTagEditorOpen && onUpdateTags && /*#__PURE__*/ (0,jsx_runtime.jsx)(TagEditor/* TagEditor */.y, {
                tags: session.tags || [],
                onSave: (newTags)=>{
                    onUpdateTags(session.id, newTags);
                    setIsTagEditorOpen(false);
                },
                onCancel: ()=>setIsTagEditorOpen(false),
                sessionTitle: session.title,
                triggerRef: overflowButtonRef
            }),
            isRestartConfirmOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: confirmDialog,
                onClick: (e)=>{
                    e.stopPropagation();
                    setIsRestartConfirmOpen(false);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: restartDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "restartDialogTitle",
                    className: dialogContent,
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        if (e.key === "Escape") setIsRestartConfirmOpen(false);
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "restartDialogTitle",
                            children: "Restart Session"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                'Are you sure you want to restart "',
                                session.title,
                                '"?'
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: warningText,
                            children: "This will terminate the current process and start a new one."
                        }),
                        restartError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: errorMessage,
                            children: restartError
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: handleRestartConfirm,
                                    disabled: isRestarting,
                                    className: dangerButton,
                                    children: isRestarting ? "Restarting..." : "Restart"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        setIsRestartConfirmOpen(false);
                                        setRestartError("");
                                    },
                                    disabled: isRestarting,
                                    className: cancelButton,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body),
            isRetryConfirmOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: confirmDialog,
                onClick: (e)=>{
                    e.stopPropagation();
                    setIsRetryConfirmOpen(false);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: retryDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "retryDialogTitle",
                    className: dialogContent,
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        if (e.key === "Escape") setIsRetryConfirmOpen(false);
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "retryDialogTitle",
                            children: "Retry Session"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            children: isPermanentlyFailed ? "This session gave up after ".concat(session.retryMaxAttempts, " attempt").concat(session.retryMaxAttempts === 1 ? "" : "s", " — retry anyway?") : "Skip the wait and retry now?"
                        }),
                        retryError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: errorMessage,
                            children: retryError
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: handleRetryConfirm,
                                    disabled: isRetrying,
                                    className: submitButton,
                                    children: isRetrying ? "Retrying..." : "Retry now"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        setIsRetryConfirmOpen(false);
                                        setRetryError("");
                                    },
                                    disabled: isRetrying,
                                    className: cancelButton,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body),
            isDeleteConfirmOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: confirmDialog,
                onClick: (e)=>{
                    e.stopPropagation();
                    setIsDeleteConfirmOpen(false);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: deleteDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "deleteDialogTitle",
                    className: dialogContent,
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        if (e.key === "Escape") setIsDeleteConfirmOpen(false);
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "deleteDialogTitle",
                            children: "Delete Session"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                'Are you sure you want to delete "',
                                session.title,
                                '"?'
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: warningText,
                            children: "This action cannot be undone."
                        }),
                        deleteError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: errorMessage,
                            children: deleteError
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: async (e)=>{
                                        e.stopPropagation();
                                        setIsDeleting(true);
                                        setDeleteError("");
                                        try {
                                            await (onDelete === null || onDelete === void 0 ? void 0 : onDelete());
                                            setIsDeleteConfirmOpen(false);
                                        } catch (err) {
                                            setDeleteError(err instanceof Error ? err.message : "Failed to delete session.");
                                        } finally{
                                            setIsDeleting(false);
                                        }
                                    },
                                    disabled: isDeleting,
                                    className: dangerButton,
                                    children: isDeleting ? "Deleting..." : "Delete"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        setIsDeleteConfirmOpen(false);
                                        setDeleteError("");
                                    },
                                    disabled: isDeleting,
                                    className: cancelButton,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body),
            isExportBundleOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)(ExportBundleModal/* ExportBundleModal */.s, {
                open: isExportBundleOpen,
                onOpenChange: setIsExportBundleOpen,
                sessionId: session.id,
                sessionTitle: session.title,
                repoPath: session.path
            }),
            isCheckpointOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: renameDialog,
                onClick: (e)=>{
                    e.stopPropagation();
                    setIsCheckpointOpen(false);
                    setCheckpointError("");
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: checkpointDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "checkpointDialogTitle",
                    className: dialogContent,
                    onClick: (e)=>e.stopPropagation(),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "checkpointDialogTitle",
                            children: "Create Checkpoint"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                'Enter a label for this checkpoint of "',
                                session.title,
                                '":'
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            type: "text",
                            value: checkpointLabel,
                            onChange: (e)=>setCheckpointLabel(e.target.value),
                            onKeyDown: (e)=>{
                                if (e.key === "Enter") handleCheckpointSubmit(e);
                                if (e.key === "Escape") {
                                    setIsCheckpointOpen(false);
                                    setCheckpointError("");
                                }
                            },
                            placeholder: "e.g. before refactor, working state",
                            className: renameInput,
                            autoFocus: true
                        }),
                        checkpointError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: errorMessage,
                            children: checkpointError
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: handleCheckpointSubmit,
                                    disabled: isCreatingCheckpoint || !checkpointLabel.trim(),
                                    className: submitButton,
                                    children: isCreatingCheckpoint ? "Saving..." : "📍 Save Checkpoint"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        setIsCheckpointOpen(false);
                                        setCheckpointError("");
                                    },
                                    disabled: isCreatingCheckpoint,
                                    className: cancelButton,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body),
            isAutonomousConfirmOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: confirmDialog,
                onClick: (e)=>{
                    e.stopPropagation();
                    setIsAutonomousConfirmOpen(false);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: autonomousConfirmDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "autonomousDialogTitle",
                    className: dialogContent,
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        if (e.key === "Escape") setIsAutonomousConfirmOpen(false);
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "autonomousDialogTitle",
                            children: "Run Autonomously"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                'Autonomous mode will inject up to 20 AI-generated prompts into "',
                                session.title,
                                '" automatically.'
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: warningText,
                            children: "The session will run without user input until it completes or gets stuck. You can disable it at any time from this menu."
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        onToggleAutonomousMode === null || onToggleAutonomousMode === void 0 ? void 0 : onToggleAutonomousMode(session.id, true);
                                        setIsAutonomousConfirmOpen(false);
                                    },
                                    className: submitButton,
                                    children: "Run Autonomously"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        setIsAutonomousConfirmOpen(false);
                                    },
                                    className: cancelButton,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body),
            isAutoApproveConfirmOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: confirmDialog,
                onClick: (e)=>{
                    e.stopPropagation();
                    setIsAutoApproveConfirmOpen(false);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: autoApproveConfirmDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "autoApproveDialogTitle",
                    className: dialogContent,
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        if (e.key === "Escape") setIsAutoApproveConfirmOpen(false);
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "autoApproveDialogTitle",
                            children: pendingAutoApproveValue ? "Enable Auto-Approve" : "Disable Auto-Approve"
                        }),
                        pendingAutoApproveValue ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                    children: [
                                        '"',
                                        session.title,
                                        '" will skip ALL permission/approval prompts for its agent (e.g. file edits, shell commands).'
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                    className: warningText,
                                    children: [
                                        "This is genuinely unsafe outside a disposable/sandboxed workspace — unintended file modifications or data loss are possible.",
                                        isRunning ? " The session will restart to apply this." : "",
                                        " You can disable it at any time from this menu."
                                    ]
                                })
                            ]
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            className: warningText,
                            children: [
                                '"',
                                session.title,
                                '" will restart to apply this change.'
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        onToggleAutoApprove === null || onToggleAutoApprove === void 0 ? void 0 : onToggleAutoApprove(session.id, pendingAutoApproveValue);
                                        setIsAutoApproveConfirmOpen(false);
                                    },
                                    className: submitButton,
                                    children: pendingAutoApproveValue ? "Enable Auto-Approve" : "Disable & Restart"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        setIsAutoApproveConfirmOpen(false);
                                    },
                                    className: cancelButton,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body),
            isSteerOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: renameDialog,
                onClick: (e)=>{
                    e.stopPropagation();
                    setIsSteerOpen(false);
                    setSteerMessage("");
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: steerDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "steerDialogTitle",
                    className: dialogContent,
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        // Escape lives on the dialog wrapper (not just the input) so it
                        // keeps working while isSteering disables the input — a disabled
                        // element stops receiving keydown in most browsers, which would
                        // otherwise make the dialog unclosable by keyboard mid-flight.
                        if (e.key === "Escape") {
                            setIsSteerOpen(false);
                            setSteerMessage("");
                        }
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "steerDialogTitle",
                            children: "Give Direction"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                'Send a steering instruction to "',
                                session.title,
                                '":'
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            type: "text",
                            value: steerMessage,
                            onChange: (e)=>setSteerMessage(e.target.value),
                            onKeyDown: (e)=>{
                                if (e.key === "Enter" && steerMessage.trim() && !isSteering) {
                                    // Without preventDefault + sendSteerMessage's own focus
                                    // move to Cancel, the browser's default Enter-key handling
                                    // re-targets the now-focused Cancel button and synthesizes
                                    // a click on it once this handler returns — closing the
                                    // dialog immediately instead of sending. Confirmed via a
                                    // real-browser Playwright repro: Cancel's onClick fired
                                    // right after setIsSteering(true), before
                                    // onSteerAutonomousSession had even resolved.
                                    e.preventDefault();
                                    void sendSteerMessage();
                                }
                            },
                            placeholder: "e.g. Focus on the UI tests first",
                            className: renameInput,
                            autoFocus: true,
                            disabled: isSteering
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        void sendSteerMessage();
                                    },
                                    disabled: !steerMessage.trim() || isSteering,
                                    className: submitButton,
                                    children: isSteering ? "Sending…" : "Send"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    ref: steerCancelButtonRef,
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        setIsSteerOpen(false);
                                        setSteerMessage("");
                                    },
                                    className: cancelButton,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body),
            isClearConversationConfirmOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: confirmDialog,
                onClick: (e)=>{
                    e.stopPropagation();
                    setIsClearConversationConfirmOpen(false);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: clearConversationDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "clearConversationDialogTitle",
                    className: dialogContent,
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        if (e.key === "Escape") setIsClearConversationConfirmOpen(false);
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "clearConversationDialogTitle",
                            children: "Clear Conversation"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: warningText,
                            children: "Clear conversation history? This will erase the AI's memory of this session and cannot be undone."
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        void (onClearConversationState === null || onClearConversationState === void 0 ? void 0 : onClearConversationState(session.id));
                                        setIsClearConversationConfirmOpen(false);
                                    },
                                    className: dangerButton,
                                    children: "Clear Conversation"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    autoFocus: true,
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        setIsClearConversationConfirmOpen(false);
                                    },
                                    className: cancelButton,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(CreatePullRequestModal/* CreatePullRequestModal */.m, {
                session: session,
                isOpen: isCreatePrOpen,
                onClose: ()=>setIsCreatePrOpen(false),
                draftPullRequest: draftPullRequest,
                createPullRequest: createPullRequest,
                triggerRef: createPrTriggerRef
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: desktopActions,
                children: [
                    showPrimaryAction && (isPaused || isReady) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: actionButton,
                        onClick: (e)=>{
                            e.stopPropagation();
                            onResume === null || onResume === void 0 ? void 0 : onResume();
                        },
                        "aria-label": "Resume session ".concat(session.title),
                        title: "Resume this session",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "aria-hidden": "true",
                                children: "▶️"
                            }),
                            " Resume"
                        ]
                    }),
                    showPrimaryAction && isRunning && !isCreating && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: actionButton,
                        onClick: (e)=>{
                            e.stopPropagation();
                            onPause === null || onPause === void 0 ? void 0 : onPause();
                        },
                        "aria-label": "Pause session ".concat(session.title),
                        title: "Pause this session",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "aria-hidden": "true",
                                children: "⏸️"
                            }),
                            " Pause"
                        ]
                    }),
                    showPrimaryAction && isStopped && onRestart && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: actionButton,
                        onClick: (e)=>{
                            e.stopPropagation();
                            setIsRestartConfirmOpen(true);
                        },
                        "aria-label": "Restart stopped session ".concat(session.title),
                        title: "Session stopped — restart to resume working",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "aria-hidden": "true",
                                children: "\uD83D\uDD04"
                            }),
                            " Restart"
                        ]
                    }),
                    showPrimaryAction && isPermanentlyFailed && onRetryNow && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        className: actionButton,
                        onClick: (e)=>{
                            e.stopPropagation();
                            setIsRetryConfirmOpen(true);
                        },
                        "aria-label": "Retry permanently-failed session ".concat(session.title),
                        title: "Session gave up after repeated failures — retry now",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "aria-hidden": "true",
                                children: "\uD83D\uDD01"
                            }),
                            " Retry now"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        ref: overflowContainerRef,
                        className: overflowContainer,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                ref: overflowButtonRef,
                                id: "overflow-btn-".concat(session.id),
                                className: buttonClassName !== null && buttonClassName !== void 0 ? buttonClassName : overflowButton,
                                onClick: openMenu,
                                "aria-label": session.autonomousMode ? "More session actions (autonomous mode active)" : "More session actions",
                                "aria-expanded": showOverflow,
                                "aria-haspopup": "menu",
                                "aria-controls": "overflow-menu-".concat(session.id),
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ellipsis/* default */.A, {
                                    size: 16
                                })
                            }),
                            showOverflow && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                ref: overflowMenuRef,
                                id: "overflow-menu-".concat(session.id),
                                className: overflowMenu,
                                style: {
                                    top: fitTop !== null && fitTop !== void 0 ? fitTop : menuPos.top,
                                    right: menuPos.right
                                },
                                role: "menu",
                                "aria-labelledby": "overflow-btn-".concat(session.id),
                                // Capture phase: each item's own onClick calls stopPropagation, so a bubbling handler never sees the click.
                                onClickCapture: (e)=>{
                                    const item = e.target.closest('[role^="menuitem"]');
                                    var _item_textContent;
                                    if (item) trackMenu("session_menu_click", menuActionLabel((_item_textContent = item.textContent) !== null && _item_textContent !== void 0 ? _item_textContent : ""));
                                },
                                onClick: (e)=>e.stopPropagation(),
                                onKeyDown: (e)=>{
                                    if (e.key === "Escape") setShowOverflow(false);
                                },
                                children: [
                                    !(isPaused || isReady) && !isStopped && !isPermanentlyFailed && onResume && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        role: "menuitem",
                                        className: overflowMenuItem,
                                        onClick: (e)=>{
                                            e.stopPropagation();
                                            close();
                                            onResume();
                                        },
                                        "aria-label": "Resume session ".concat(session.title),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                "aria-hidden": "true",
                                                children: "▶️"
                                            }),
                                            " Resume"
                                        ]
                                    }),
                                    isRunning && !isCreating && onPause && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        role: "menuitem",
                                        className: overflowMenuItem,
                                        onClick: (e)=>{
                                            e.stopPropagation();
                                            close();
                                            onPause();
                                        },
                                        "aria-label": "Pause session ".concat(session.title),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                "aria-hidden": "true",
                                                children: "⏸️"
                                            }),
                                            " Pause"
                                        ]
                                    }),
                                    isRunning && onHibernate && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        role: "menuitem",
                                        className: overflowMenuItem,
                                        onClick: (e)=>{
                                            e.stopPropagation();
                                            close();
                                            onHibernate();
                                        },
                                        "aria-label": "Hibernate session ".concat(session.title),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                "aria-hidden": "true",
                                                children: "❄️"
                                            }),
                                            " Hibernate"
                                        ]
                                    }),
                                    isHibernated && onResumeFromHibernation && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        role: "menuitem",
                                        className: overflowMenuItem,
                                        onClick: (e)=>{
                                            e.stopPropagation();
                                            close();
                                            onResumeFromHibernation();
                                        },
                                        "aria-label": "Resume hibernated session ".concat(session.title),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                "aria-hidden": "true",
                                                children: "▶️"
                                            }),
                                            " Resume"
                                        ]
                                    }),
                                    onTogglePinned && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        role: "menuitemcheckbox",
                                        "aria-checked": session.pinned,
                                        className: "".concat(overflowMenuItem, " ").concat(overflowMenuItemTouch),
                                        "data-testid": "session-pin-toggle",
                                        "aria-label": session.pinned ? "Unpin ".concat(session.title) : "Pin ".concat(session.title),
                                        onClick: (e)=>{
                                            e.stopPropagation();
                                            close();
                                            onTogglePinned(session.id, !session.pinned);
                                        },
                                        children: [
                                            session.pinned ? /*#__PURE__*/ (0,jsx_runtime.jsx)(pin_off/* default */.A, {
                                                "aria-hidden": "true",
                                                size: 16
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(pin/* default */.A, {
                                                "aria-hidden": "true",
                                                size: 16
                                            }),
                                            " ",
                                            session.pinned ? "Unpin" : "Pin"
                                        ]
                                    }),
                                    onChangeProgram && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        role: "menuitem",
                                        className: overflowMenuItem,
                                        onClick: (e)=>{
                                            e.stopPropagation();
                                            setProgramPickerValue(session.program || "");
                                            setProgramError("");
                                            setIsProgramPickerOpen(true);
                                        },
                                        "aria-label": "Change program for session ".concat(session.title),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                "aria-hidden": "true",
                                                children: "⚙️"
                                            }),
                                            " Change Program"
                                        ]
                                    }),
                                    onDelete && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        role: "menuitem",
                                        className: "".concat(overflowMenuItem, " ").concat(overflowMenuItemDanger),
                                        onClick: (e)=>{
                                            e.stopPropagation();
                                            close();
                                            setIsDeleteConfirmOpen(true);
                                        },
                                        disabled: isDeleting,
                                        "aria-label": "Delete session ".concat(session.title),
                                        children: isDeleting ? "Deleting..." : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    "aria-hidden": "true",
                                                    children: "\uD83D\uDDD1️"
                                                }),
                                                " Delete"
                                            ]
                                        })
                                    }),
                                    (hasGroup2 || hasOrganize || hasGroup4 || hasMore) && menuSeparator,
                                    hasGroup2 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(MenuSection, {
                                        id: "workflow",
                                        label: "Workflow",
                                        openId: openSection,
                                        onToggle: toggleSection,
                                        children: [
                                            session.githubPrUrl ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
                                                href: session.githubPrUrl,
                                                target: "_blank",
                                                rel: "noopener noreferrer",
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                "aria-label": "PR #".concat(session.githubPrNumber, ": ").concat(session.title),
                                                "data-testid": "github-pr-link",
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                },
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "✅"
                                                    }),
                                                    " View PR #",
                                                    session.githubPrNumber
                                                ]
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                ref: createPrTriggerRef,
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                disabled: !session.hasCommitsAhead,
                                                title: session.hasCommitsAhead ? undefined : "No commits ahead of main yet",
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    setIsCreatePrOpen(true);
                                                },
                                                "aria-label": "Create PR for session ".concat(session.title),
                                                "data-testid": "create-pr-trigger-".concat(session.id),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83D\uDD00"
                                                    }),
                                                    " Create PR"
                                                ]
                                            }),
                                            onCreateCheckpoint && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    setCheckpointLabel("");
                                                    setIsCheckpointOpen(true);
                                                },
                                                "aria-label": "Create checkpoint for session ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83D\uDCCD"
                                                    }),
                                                    " Checkpoint"
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    setIsExportBundleOpen(true);
                                                },
                                                "aria-label": "Export diagnostics for session ".concat(session.title),
                                                "aria-haspopup": "dialog",
                                                "data-testid": "export-diagnostics-menu-item",
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83D\uDCE6"
                                                    }),
                                                    " Export Diagnostics"
                                                ]
                                            })
                                        ]
                                    }),
                                    hasOrganize && /*#__PURE__*/ (0,jsx_runtime.jsxs)(MenuSection, {
                                        id: "organize",
                                        label: "Organize",
                                        openId: openSection,
                                        onToggle: toggleSection,
                                        children: [
                                            onRenameRequest && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    onRenameRequest();
                                                },
                                                "aria-label": "Rename session ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "✏️"
                                                    }),
                                                    " Rename"
                                                ]
                                            }),
                                            onClone && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    onClone();
                                                },
                                                "aria-label": "Clone session ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "⊕"
                                                    }),
                                                    " Clone"
                                                ]
                                            }),
                                            onOpenInNewPane && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    onOpenInNewPane();
                                                },
                                                "aria-label": "Open ".concat(session.title, " in new pane"),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "⊞"
                                                    }),
                                                    " Open in new pane"
                                                ]
                                            }),
                                            onUpdateTags && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    setIsTagEditorOpen(true);
                                                },
                                                "aria-label": "Edit tags for session ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83C\uDFF7️"
                                                    }),
                                                    " Edit Tags"
                                                ]
                                            }),
                                            onNewWorkspace && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    onNewWorkspace();
                                                },
                                                "aria-label": "New workspace from ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "➕"
                                                    }),
                                                    " New Workspace"
                                                ]
                                            }),
                                            onWorkspaceSwitchRequest && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    onWorkspaceSwitchRequest();
                                                },
                                                "aria-label": "Switch workspace for ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "⎇"
                                                    }),
                                                    " Switch Workspace"
                                                ]
                                            })
                                        ]
                                    }),
                                    hasGroup4 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(MenuSection, {
                                        id: "modes",
                                        label: "Modes",
                                        openId: openSection,
                                        onToggle: toggleSection,
                                        children: [
                                            onSetRateLimitEnabled && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    onSetRateLimitEnabled(session.id, !session.rateLimitEnabled);
                                                },
                                                "aria-label": session.rateLimitEnabled ? "Disable auto-resume for ".concat(session.title) : "Enable auto-resume for ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: session.rateLimitEnabled ? "⏸" : "▶"
                                                    }),
                                                    " ",
                                                    session.rateLimitEnabled ? "Disable auto-resume" : "Enable auto-resume"
                                                ]
                                            }),
                                            onToggleAutonomousMode && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitemcheckbox",
                                                "aria-checked": session.autonomousMode,
                                                className: overflowMenuItem,
                                                title: "AI will inject prompts automatically until the task is complete (up to 20 turns).",
                                                "aria-label": session.autonomousMode ? "Stop running ".concat(session.title, " autonomously") : "Run ".concat(session.title, " autonomously"),
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    if (!session.autonomousMode) {
                                                        setIsAutonomousConfirmOpen(true);
                                                    } else {
                                                        onToggleAutonomousMode(session.id, false);
                                                    }
                                                },
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: session.autonomousMode ? "⏹" : "🤖"
                                                    }),
                                                    " ",
                                                    session.autonomousMode ? "Stop running autonomously" : "Run autonomously"
                                                ]
                                            }),
                                            onSteerAutonomousSession && session.autonomousMode && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    setSteerMessage("");
                                                    setIsSteerOpen(true);
                                                },
                                                "aria-label": "Give direction to ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83E\uDDED"
                                                    }),
                                                    " Give direction"
                                                ]
                                            }),
                                            onToggleAutoApprove && (session.autoApprove || (0,autoApprove/* isAutoApproveSupported */.$)(session.program)) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitemcheckbox",
                                                "aria-checked": session.autoApprove,
                                                className: overflowMenuItem,
                                                title: "Skip all permission/approval prompts for this agent.",
                                                "aria-label": session.autoApprove ? "Disable auto-approve for ".concat(session.title) : "Enable auto-approve for ".concat(session.title),
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    const next = !session.autoApprove;
                                                    // Enabling always confirms (the danger warning applies regardless of
                                                    // run state -- matches the original asymmetric-friction design).
                                                    // Disabling only confirms when Active, where it also restarts
                                                    // (AC6); disabling a non-Active session is the safe direction and
                                                    // never restarts, so it applies immediately.
                                                    if (next || isRunning) {
                                                        setPendingAutoApproveValue(next);
                                                        setIsAutoApproveConfirmOpen(true);
                                                    } else {
                                                        onToggleAutoApprove(session.id, next);
                                                    }
                                                },
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: session.autoApprove ? "⏹" : "⚡"
                                                    }),
                                                    " ",
                                                    session.autoApprove ? "Disable auto-approve" : "Enable auto-approve"
                                                ]
                                            })
                                        ]
                                    }),
                                    hasMore && /*#__PURE__*/ (0,jsx_runtime.jsxs)(MenuSection, {
                                        id: "more",
                                        label: "More",
                                        openId: openSection,
                                        onToggle: toggleSection,
                                        children: [
                                            onClearConversationState && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: "".concat(overflowMenuItem, " ").concat(overflowMenuItemDanger),
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    void onClearConversationState(session.id);
                                                },
                                                "aria-label": "Clear conversation state for session ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83D\uDDD1️"
                                                    }),
                                                    " Clear Conversation"
                                                ]
                                            }),
                                            onRestart && !isCreating && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    setIsRestartConfirmOpen(true);
                                                },
                                                "aria-label": "Restart session ".concat(session.title),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83D\uDD04"
                                                    }),
                                                    " Restart"
                                                ]
                                            }),
                                            onRetryNow && (isPermanentlyFailed || isMidBackoffWait) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                                role: "menuitem",
                                                className: overflowMenuItem,
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    close();
                                                    setIsRetryConfirmOpen(true);
                                                },
                                                "aria-label": "Retry session ".concat(session.title, " now"),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83D\uDD01"
                                                    }),
                                                    " Retry now"
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            }), document.body)
                        ]
                    })
                ]
            }),
            isProgramPickerOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                ref: programPickerDialogRef,
                role: "dialog",
                "aria-modal": "true",
                "aria-label": "Change program",
                style: {
                    position: "fixed",
                    inset: 0,
                    zIndex: 9999,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    background: "rgba(0,0,0,0.5)"
                },
                onClick: (e)=>{
                    if (e.target === e.currentTarget) setIsProgramPickerOpen(false);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: confirmDialog,
                    style: {
                        minWidth: 300
                    },
                    onClick: (e)=>e.stopPropagation(),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogContent,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                    children: "Change Program"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                    style: {
                                        fontSize: 13,
                                        margin: "8px 0 4px",
                                        color: "var(--text-secondary)"
                                    },
                                    children: [
                                        "Select a program for ",
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("em", {
                                            children: session.title
                                        }),
                                        ".",
                                        isRunning && " The session will restart."
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                    value: programPickerValue,
                                    onChange: (e)=>setProgramPickerValue(e.target.value),
                                    style: {
                                        width: "100%",
                                        padding: "6px 8px",
                                        fontSize: 13,
                                        background: "var(--card-background)",
                                        color: "var(--text-primary)",
                                        border: "1px solid var(--border-color)",
                                        borderRadius: 6
                                    },
                                    autoFocus: true,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                            value: "",
                                            children: "System default"
                                        }),
                                        availablePrograms.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: p.value,
                                                children: p.label
                                            }, p.value)),
                                        programPickerValue && !availablePrograms.some((p)=>p.value === programPickerValue) && /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                            value: programPickerValue,
                                            children: programPickerValue
                                        })
                                    ]
                                }),
                                programError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: errorMessage,
                                    children: programError
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: submitButton,
                                    disabled: isSavingProgram,
                                    onClick: async ()=>{
                                        if (isRunning) {
                                            // Active sessions restart on a program change — require an
                                            // explicit second confirmation, matching Restart/Delete.
                                            setPendingProgramValue(programPickerValue);
                                            setProgramError("");
                                            setIsProgramPickerOpen(false);
                                            setIsProgramRestartConfirmOpen(true);
                                            return;
                                        }
                                        const ok = await commitProgramChange(programPickerValue);
                                        if (ok) setIsProgramPickerOpen(false);
                                    },
                                    children: isSavingProgram ? "Saving…" : "Save"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: cancelButton,
                                    onClick: ()=>{
                                        setIsProgramPickerOpen(false);
                                        setProgramError("");
                                    },
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body),
            isProgramRestartConfirmOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: confirmDialog,
                onClick: (e)=>{
                    e.stopPropagation();
                    setIsProgramRestartConfirmOpen(false);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    ref: programConfirmDialogRef,
                    role: "dialog",
                    "aria-modal": "true",
                    "aria-labelledby": "programConfirmDialogTitle",
                    className: dialogContent,
                    onClick: (e)=>e.stopPropagation(),
                    onKeyDown: (e)=>{
                        if (e.key === "Escape") setIsProgramRestartConfirmOpen(false);
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "programConfirmDialogTitle",
                            children: "Change Program"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                'Switch "',
                                session.title,
                                '" to a different program?'
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: warningText,
                            children: "This will terminate the current process and start a new one with the new program."
                        }),
                        programError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            className: errorMessage,
                            children: programError
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: dialogActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: async (e)=>{
                                        e.stopPropagation();
                                        const ok = await commitProgramChange(pendingProgramValue);
                                        if (ok) setIsProgramRestartConfirmOpen(false);
                                    },
                                    disabled: isSavingProgram,
                                    className: submitButton,
                                    children: isSavingProgram ? "Saving…" : "Change & Restart"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    onClick: (e)=>{
                                        e.stopPropagation();
                                        setIsProgramRestartConfirmOpen(false);
                                        setProgramError("");
                                    },
                                    disabled: isSavingProgram,
                                    className: cancelButton,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            }), document.body)
        ]
    });
});

// EXTERNAL MODULE: ./src/lib/sessions/formatPauseReason.ts
var formatPauseReason = __webpack_require__(31205);
// EXTERNAL MODULE: ./src/lib/session-staleness.ts
var session_staleness = __webpack_require__(76987);
// EXTERNAL MODULE: ./src/lib/store/index.ts
var store = __webpack_require__(93602);
// EXTERNAL MODULE: ./src/lib/store/remotesSlice.ts
var remotesSlice = __webpack_require__(81921);
// EXTERNAL MODULE: ./src/gen/session/v1/remote_pb.ts
var remote_pb = __webpack_require__(88149);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FRemoteConnectionIndicator.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7WUwY6bMBCG73mKuVRKpLIC0k0T59I3WRk8wGyMbY2dkKTad6_w0iTQpOmlEgdgfv7fn8fDjx2eKpYtenjLPNvcvK_h5wygYtvGG4DA0vjKciuAbZAB56nCerGdAXzMAIJ9qFuubpQfs5ffEWn8QpF3Wp4EkNFkMKk0Hnup1FSbhAK2XkCJJiD3r2vpBBwkz5PkLdvgOedzforeTipFpr5ThSx10bOwrJATlor2fipcbqJNZU1IPJ3xj3pxrXdIdRMErNJ0e4uURaRClrua7d6oqcchWpRWW56WusXIKX_idHrsdF5cWQVk7gjealIT1XGct3ySl2WPA7P8nxKzdBz5LUZ2pEIjYP3ZoWbY2PXdhr2mX2ILNB4T3zCZnYB0e-cQFdqWu1HY6xO-_Xhtqyfyye59f7Z7E_TNf0e_tiP_ywG4xATrkqG3cYCdZDQhzqGhVgayRtz8G9KXtYc-TjKQqchQwBGgjIDOevr8VBbe6n0vumBnY-zh8TLFEa6VXJMRkAxVe0CutO0ENKQUmngkNTkBjGWYp19huCJb11DAxDtZogBjO5buBnlYRj_AvwDN4nb0AAUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FRemoteConnectionIndicator_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7WUwY6bMBCG73mKuVRKpLIC0k0T59I3WRk8wGyMbY2dkKTad6_w0iTQpOmlEgdgfv7fn8fDjx2eKpYtenjLPNvcvK_h5wygYtvGG4DA0vjKciuAbZAB56nCerGdAXzMAIJ9qFuubpQfs5ffEWn8QpF3Wp4EkNFkMKk0Hnup1FSbhAK2XkCJJiD3r2vpBBwkz5PkLdvgOedzforeTipFpr5ThSx10bOwrJATlor2fipcbqJNZU1IPJ3xj3pxrXdIdRMErNJ0e4uURaRClrua7d6oqcchWpRWW56WusXIKX_idHrsdF5cWQVk7gjealIT1XGct3ySl2WPA7P8nxKzdBz5LUZ2pEIjYP3ZoWbY2PXdhr2mX2ILNB4T3zCZnYB0e_cQFdqWu1HY6xO_Xhtqyfyye59f7Z7E_TNf0e_tiP_ywG4xATrkqG3cYCdZDQhzqGhVgayRtz8G9KXtYc_TjKQqchQwBGgjIDOevr8VBbe6n0vumBnY_zh8TLFEa6VXJMRkAxVe0CutO0ENKQUmngkNTkBjGWYp19huCJb11DAxDtZogBjO5buBnlYRj_AvwDN4nb0AAUAAA_22_7D = __webpack_require__(17685);
;// ./src/components/sessions/RemoteConnectionIndicator.css.ts


var ariaLiveRegion = '_1sro2nja';
var RemoteConnectionIndicator_css_badge = '_1sro2nj0';
var dots = {connected:'_1sro2nj5 _1sro2nj4',reconnecting:'_1sro2nj6 _1sro2nj4',disconnected:'_1sro2nj7 _1sro2nj4'};
var RemoteConnectionIndicator_css_spinner = '_1sro2nj9';
var statusConnected = '_1sro2nj1';
var statusDisconnected = '_1sro2nj3';
var statusReconnecting = '_1sro2nj2';
;// ./src/components/sessions/RemoteConnectionIndicator.tsx
/* __next_internal_client_entry_do_not_use__ RemoteConnectionIndicator auto */ 
// +feature: remote-connection-indicator





// RemoteConnectionIndicator shows a remote session's live SSH connection
// health (Story 6.2.2, requirements.md AC5) -- distinct from a session's own
// lifecycle status badge (SessionCard.tsx's getStatusText/Active-Paused-etc,
// which reflects the tmux session, not the SSH transport underneath it).
// State comes exclusively from remotesSlice (fed by RemoteHealthChangedEvent
// arriving over the WatchSessions stream useSessionService.ts already
// subscribes to) -- this component issues no RPC or fetch of its own, ever.
const STATE_LABEL = {
    [remote_pb/* RemoteConnectionState */.CO.UNSPECIFIED]: "",
    [remote_pb/* RemoteConnectionState */.CO.CONNECTED]: "Connected",
    [remote_pb/* RemoteConnectionState */.CO.RECONNECTING]: "Reconnecting…",
    [remote_pb/* RemoteConnectionState */.CO.DISCONNECTED]: "Disconnected"
};
// Announcement text for the persistent aria-live="polite" region. DISCONNECTED
// is deliberately absent here -- it's announced through the separate
// role="alert" element below instead (assertive), matching this repo's
// inlineEditError convention (SessionCard.tsx's inline-rename error span) for
// a terminal failure state that needs the user's attention, not a polite
// background update.
const POLITE_ANNOUNCE = {
    [remote_pb/* RemoteConnectionState */.CO.CONNECTED]: "Remote connection restored",
    [remote_pb/* RemoteConnectionState */.CO.RECONNECTING]: "Remote reconnecting…"
};
const ALERT_ANNOUNCE = "Remote disconnected — action may be required";
const STATE_CLASS = {
    [remote_pb/* RemoteConnectionState */.CO.UNSPECIFIED]: "",
    [remote_pb/* RemoteConnectionState */.CO.CONNECTED]: statusConnected,
    [remote_pb/* RemoteConnectionState */.CO.RECONNECTING]: statusReconnecting,
    [remote_pb/* RemoteConnectionState */.CO.DISCONNECTED]: statusDisconnected
};
const DOT_CLASS = {
    [remote_pb/* RemoteConnectionState */.CO.UNSPECIFIED]: undefined,
    [remote_pb/* RemoteConnectionState */.CO.CONNECTED]: dots.connected,
    [remote_pb/* RemoteConnectionState */.CO.RECONNECTING]: dots.reconnecting,
    [remote_pb/* RemoteConnectionState */.CO.DISCONNECTED]: dots.disconnected
};
function RemoteConnectionIndicator(param) {
    let { remoteName } = param;
    const entry = (0,store/* useAppSelector */.GV)((0,remotesSlice/* selectRemoteHealthEntry */.lj)(remoteName));
    var _entry_state;
    const state = (_entry_state = entry === null || entry === void 0 ? void 0 : entry.state) !== null && _entry_state !== void 0 ? _entry_state : remote_pb/* RemoteConnectionState */.CO.UNSPECIFIED;
    const prevStateRef = (0,react.useRef)(state);
    const [politeAnnouncement, setPoliteAnnouncement] = (0,react.useState)("");
    // Announce on state transitions -- mirrors layout/ConnectionIndicator.tsx's
    // identical prevStateRef + effect pattern for the session-stream indicator.
    (0,react.useEffect)(()=>{
        if (prevStateRef.current !== state) {
            const text = POLITE_ANNOUNCE[state];
            if (text) setPoliteAnnouncement(text);
            prevStateRef.current = state;
        }
    }, [
        state
    ]);
    // No health-change event observed yet -- render nothing rather than a
    // misleading default. Story 6.2.2's data comes from Redux/the push stream
    // only, and there is no synchronous initial value to show.
    if (state === remote_pb/* RemoteConnectionState */.CO.UNSPECIFIED) {
        return null;
    }
    const label = STATE_LABEL[state];
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ariaLiveRegion,
                role: "status",
                "aria-live": "polite",
                "aria-atomic": "true",
                children: politeAnnouncement
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: "".concat(RemoteConnectionIndicator_css_badge, " ").concat(STATE_CLASS[state]),
                role: "img",
                title: "Remote ".concat(remoteName, ": ").concat(label),
                "aria-label": "Remote connection: ".concat(label),
                "data-testid": "remote-connection-indicator",
                children: [
                    state === remote_pb/* RemoteConnectionState */.CO.RECONNECTING ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: RemoteConnectionIndicator_css_spinner,
                        "aria-hidden": "true"
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: DOT_CLASS[state],
                        "aria-hidden": "true"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: label
                    })
                ]
            }),
            state === remote_pb/* RemoteConnectionState */.CO.DISCONNECTED && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                role: "alert",
                className: ariaLiveRegion,
                children: ALERT_ANNOUNCE
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/constants/programs.ts
var programs = __webpack_require__(49051);
// EXTERNAL MODULE: ./src/lib/hooks/useTaggingRuleNames.ts
var useTaggingRuleNames = __webpack_require__(48843);
// EXTERNAL MODULE: ./src/lib/sessions/tagProvenance.ts
var tagProvenance = __webpack_require__(96077);
// EXTERNAL MODULE: ./src/lib/utils/truncateMiddle.ts
var truncateMiddle = __webpack_require__(72301);
;// ./src/lib/utils/truncateWorkspacePath.ts

const HEX_HASH_RE = /^[0-9a-f]{7,40}$/i;
// Copied verbatim from a canonical UUID regex reference, per build-vs-buy.md's
// "don't hand-roll a UUID pattern" guidance.
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const TRAILING_OPAQUE_RE = /[-_][0-9a-f]{8,}$/i;
const HOME_DIR_RE = /^\/(?:home|Users)\/[^/]+/;
/**
 * Classifies a whole path segment as "opaque" (a bare hex hash/UUID, or one
 * with a trailing opaque hex suffix) or "semantic" (a human-readable name).
 */ function classifySegment(segment) {
    if (HEX_HASH_RE.test(segment) || UUID_RE.test(segment) || TRAILING_OPAQUE_RE.test(segment)) {
        return "opaque";
    }
    return "semantic";
}
function isWholeOpaque(segment) {
    return HEX_HASH_RE.test(segment) || UUID_RE.test(segment);
}
/** Replaces a segment's trailing opaque hex suffix (if any) with "…". */ function collapseTrailingSuffix(segment) {
    const trailingMatch = segment.match(TRAILING_OPAQUE_RE);
    if (trailingMatch && trailingMatch.index !== undefined) {
        return segment.slice(0, trailingMatch.index) + "…";
    }
    return segment;
}
/**
 * Collapses each run of one-or-more consecutive whole-opaque segments to a
 * single "…" segment, and collapses any remaining segment's trailing opaque
 * hex suffix (if present) to "…" while keeping its semantic prefix.
 */ function collapseOpaqueSegments(segments) {
    const result = [];
    for(let i = 0; i < segments.length;){
        if (isWholeOpaque(segments[i])) {
            result.push("…");
            do {
                i++;
            }while (i < segments.length && isWholeOpaque(segments[i]));
            continue;
        }
        result.push(collapseTrailingSuffix(segments[i]));
        i++;
    }
    return result;
}
/**
 * Collapses opaque path segments (workspace hashes, worktree UUID suffixes)
 * to a single "…" while preserving semantic segments, so long
 * workspace/worktree paths stay scannable. Falls back to `truncateMiddle`'s
 * char-budget truncation when no opaque segment is found, or when segment
 * collapsing alone doesn't bring the path under `maxLen`.
 */ function truncateWorkspacePath(path, maxLen) {
    if (!path) return path;
    // Step 0: collapse the home-dir prefix before length-checking or
    // classifying segments — splitting first would put the username as an
    // interior segment that classifySegment would never flag as opaque.
    const homeCollapsed = path.replace(HOME_DIR_RE, "~");
    if (homeCollapsed.length <= maxLen) return homeCollapsed;
    const rejoined = collapseOpaqueSegments(homeCollapsed.split("/")).join("/");
    return rejoined.length > maxLen ? (0,truncateMiddle/* truncateMiddle */.k)(rejoined, maxLen) : rejoined;
}

;// ./src/components/sessions/session-columns.ts
const COLUMN_DEFS = [
    {
        key: "agent",
        label: "Agent",
        gridWidth: "20px",
        defaultVisible: false
    },
    {
        key: "memory",
        label: "Memory",
        gridWidth: "auto",
        defaultVisible: false
    },
    {
        key: "elapsed",
        label: "Last active",
        gridWidth: "auto",
        defaultVisible: true
    },
    {
        key: "diff",
        label: "Diff",
        gridWidth: "auto",
        defaultVisible: false
    },
    {
        key: "branch",
        label: "Branch",
        gridWidth: "auto",
        defaultVisible: false
    }
];
const DEFAULT_VISIBLE_COLUMNS = COLUMN_DEFS.filter((c)=>c.defaultVisible).map((c)=>c.key);
// Card/Board views have no ColumnPicker UI (it only renders for row view — see
// SessionList.tsx), so they can't opt back into a hidden column. Pre-PR, Card/Board
// always rendered the Program row and memory badge unconditionally; this default
// preserves that behavior as SessionCard's fallback when no `visibleColumns` prop is
// passed, independent of row view's user-configurable DEFAULT_VISIBLE_COLUMNS.
const CARD_DEFAULT_VISIBLE_COLUMNS = [
    "agent",
    "memory",
    ...DEFAULT_VISIBLE_COLUMNS
];
/** Build a CSS gridTemplateColumns value from the current visible set. */ function buildRowGridTemplate(visible, options) {
    const cols = [];
    if (options === null || options === void 0 ? void 0 : options.reserveCheckbox) cols.push("24px"); // checkbox column
    cols.push("8px", "1fr"); // dot + name always present
    for (const def of COLUMN_DEFS){
        if (def.key === "elapsed") continue;
        if (visible.includes(def.key)) cols.push(def.gridWidth);
    }
    cols.push("auto"); // actions always present
    return cols.join(" ");
}

;// ./src/components/sessions/SessionCard.css.ts



var SessionCard_css_actionButton = 'db4abu1s';
var actionButtonCompact = 'db4abu1t';
var autoApproveBadge = 'db4abu2i';
var autoApprovePendingBadge = 'db4abu2j';
var autonomousBadge = 'db4abu2g';
var badges = 'db4abue';
var body = 'db4abu16';
var SessionCard_css_cancelButton = 'db4abu23';
var card = 'db4abu3';
var cardDeleting = 'db4abu4';
var cardExternal = 'db4abu7';
var cardMemoryPressure = 'db4abu2f';
var cardPaused = 'db4abu8';
var cardSelectMode = 'db4abu5';
var cardSelected = 'db4abu6';
var category = 'db4abu10';
var SessionCard_css_checkbox = 'db4abu9';
var SessionCard_css_confirmDialog = 'db4abu1v';
var creationSpinner = 'db4abu2o';
var SessionCard_css_dangerButton = 'db4abu24';
var deleteButton = 'db4abu1u';
var SessionCard_css_desktopActions = 'db4abu1l';
var SessionCard_css_dialogActions = 'db4abu21';
var SessionCard_css_dialogContent = 'db4abu1x';
var diffAdded = 'db4abu1d';
var diffRemoved = 'db4abu1e';
var diffStats = 'db4abu1c';
var editTagsButton = 'db4abu15';
var SessionCard_css_errorMessage = 'db4abu20';
var externalBadge = 'db4abuf';
var failureMessageIcon = 'db4abuz';
var footer = 'db4abu1i';
var githubLink = 'db4abu1b';
var header = 'db4abua';
var hostBadge = 'db4abug';
var info = 'db4abu17';
var infoRow = 'db4abu18';
var inlineTitleInput = 'db4abud';
var label = 'db4abu19';
var lastActivityLabel = 'db4abu1g';
var lastActivityRow = 'db4abu1f';
var lastActivityTime = 'db4abu1h';
var memoryBadge = 'db4abu2c';
var memoryBadgeHigh = 'db4abu2e';
var memoryBadgeWarning = 'db4abu2d';
var muxIndicator = 'db4abul';
var noteBadge = 'db4abu2k';
var SessionCard_css_overflowButton = 'db4abu1n';
var SessionCard_css_overflowContainer = 'db4abu1m';
var SessionCard_css_overflowMenu = 'db4abu1o';
var SessionCard_css_overflowMenuItem = 'db4abu1p';
var SessionCard_css_overflowMenuItemDanger = 'db4abu1r';
var SessionCard_css_overflowMenuItemTouch = 'db4abu1q';
var piHealthBadge = 'db4abuh';
var piHealthBadgeFailed = 'db4abuj';
var piHealthBadgeLoaded = 'db4abui';
var piHealthBadgeUnknown = 'db4abuk';
var SessionCard_css_renameDialog = 'db4abu1v';
var SessionCard_css_renameInput = 'db4abu1z';
var reviewContext = 'db4abun';
var reviewInfo = 'db4abum';
var snapshotEmpty = 'db4abu29';
var snapshotError = {base:'db4abu2b'};
var snapshotLoading = 'db4abu2a';
var snapshotPane = 'db4abu28';
var snapshotSection = 'db4abu25';
var snapshotToggle = 'db4abu26';
var snapshotToggleIcon = 'db4abu27';
var staleBadge = 'db4abu2l';
var SessionCard_css_status = 'db4abuo';
var statusCrashed = 'db4abuw';
var statusCreationFailed = 'db4abux';
var statusGlyphIcon = 'db4abuy';
var statusLoading = 'db4abut';
var statusNeedsApproval = 'db4abuu';
var statusPaused = 'db4abur';
var statusPausedDistinct = 'db4abus';
var statusReady = 'db4abuq';
var statusRunning = 'db4abup';
var statusUnknown = 'db4abuv';
var SessionCard_css_submitButton = 'db4abu22';
var tag = 'db4abu13';
var tagUnclassified = 'db4abu14';
var tags = 'db4abu12';
var tagsContainer = 'db4abu11';
var taskFraction = 'db4abu2m';
var timestamp = 'db4abu1k';
var timestamps = 'db4abu1j';
var title = 'db4abuc';
var titleRow = 'db4abub';
var value = 'db4abu1a';
var SessionCard_css_warningText = 'db4abu1y';
var workflowBadge = 'db4abu2h';
;// ./src/lib/utils/string.ts
/**
 * Truncates a goal text to a maximum length, appending "…" if truncated.
 * Returns the original string unchanged when it is already within the limit.
 */ function truncateGoal(text, max) {
    if (!text) return text;
    if (text.length <= max) return text;
    return text.slice(0, max - 1) + "…";
}

;// ./src/components/sessions/SessionCard.tsx
/* __next_internal_client_entry_do_not_use__ hasPendingProgramChange,isRedundantWithTitle,basenameOf,piHealthBadgeInfo,hasPendingAutoApproveChange,SessionCard auto */ 
// +feature: remote-host-badge



























// The launch command always starts with the program string it was last launched
// with (see Instance.buildLaunchCommand, session/instance_tmux.go). If it no longer
// starts with the current program, the program was changed since the last launch
// and won't take effect until the session is next resumed/restarted. Exported as a
// standalone predicate so it's unit-testable without rendering the full card.
function hasPendingProgramChange(session) {
    const isPausedOrStopped = session.status === types_pb/* SessionStatus */.Ph.PAUSED || session.status === types_pb/* SessionStatus */.Ph.STOPPED;
    return isPausedOrStopped && !!session.program && !!session.launchCommand && !session.launchCommand.startsWith(session.program);
}
// A secondary info-row value is redundant with the primary title when it is
// the exact same text (surrounding whitespace aside) — repeating it below the
// title adds visual noise with no new information. Deliberately NOT
// case-insensitive (a user who capitalizes a branch/title differently likely
// meant it) and NOT substring/basename-aware here — callers that need
// basename comparison (Path/Working Dir/Cloned To) pre-normalize via
// `basenameOf` before calling this.
function isRedundantWithTitle(value, title) {
    if (!value) return false;
    return value.trim() === title.trim();
}
// Last "/"-separated segment of a trimmed path string. Mirrors the
// `p.split("/").pop() || p` idiom already used in SessionsTable.tsx,
// page.tsx, RecentFilesSection.tsx, and useAvailablePrograms.ts (not
// `path.basename` — no Node `path` polyfill in this "use client" component).
function basenameOf(pathValue) {
    const trimmed = pathValue.trim();
    return trimmed.split("/").pop() || trimmed;
}
// Path-shaped info rows (Path, Working Dir, Cloned To) compare the value's
// basename against the title, unlike Branch/Goal which compare raw text —
// wrapping that in its own named function keeps the two comparison shapes
// structurally distinct instead of relying on callers to remember which rows
// need basenameOf() and which don't.
function isPathRedundantWithTitle(pathValue, title) {
    return isRedundantWithTitle(basenameOf(pathValue), title);
}
// piHealthBadgeInfo (pi-support Epic 4.2, Story 4.2.2) maps the three-state
// PiExtensionHealth enum to the exact icon/label/aria-label design/ux.md §
// "Badge component design" pins for each state. Never render Loaded's text
// before a real ping arrives — UNSPECIFIED/UNKNOWN both fall through to the
// "unknown" branch below (UNSPECIFIED is the pre-any-signal wire default and
// must read identically to an explicit Unknown, never as Loaded).
function piHealthBadgeInfo(healthValue) {
    switch(healthValue){
        case types_pb/* PiExtensionHealth */.T7.LOADED:
            return {
                icon: "\uD83D\uDEE1️",
                label: "pi",
                ariaLabel: "pi approval extension: loaded — tool calls are enforced",
                className: piHealthBadgeLoaded
            };
        case types_pb/* PiExtensionHealth */.T7.FAILED:
            return {
                icon: "⚠️",
                label: "pi",
                ariaLabel: "pi approval extension: not loaded — tool calls are unenforced",
                className: piHealthBadgeFailed
            };
        default:
            return {
                icon: "◌",
                label: "pi",
                ariaLabel: "pi approval extension: status unknown",
                className: piHealthBadgeUnknown
            };
    }
}
const AUTO_APPROVE_FLAG_LITERALS = [
    "--dangerously-skip-permissions",
    "--yes-always"
];
// Mirrors hasPendingProgramChange's shape: true when the persisted autoApprove value
// disagrees with whether a known yolo flag is actually present in the last-launched
// command (i.e. the toggle changed but the process hasn't restarted with it yet).
function hasPendingAutoApproveChange(session) {
    const isPausedOrStopped = session.status === types_pb/* SessionStatus */.Ph.PAUSED || session.status === types_pb/* SessionStatus */.Ph.STOPPED;
    if (!isPausedOrStopped || !session.launchCommand) return false;
    const flagPresent = AUTO_APPROVE_FLAG_LITERALS.some((f)=>session.launchCommand.includes(f));
    return session.autoApprove !== flagPresent;
}


const IS_DEBUG_MODE =  true && new URLSearchParams(window.location.search).get("debug") === "1";
// Names/paths are never length-truncated — full text always renders. See
// SessionRow's identically-named constant: `Infinity` keeps
// `truncateWorkspacePath`'s home-dir "~" collapsing (not lossy) while
// skipping its length-based truncation entirely.
const CARD_PATH_MAX_LEN = Infinity;
function SessionCardInner(param) {
    let { session, onClick, onOpenInNewPane, onDelete, onPause, onResume, onClone, onNewWorkspace, onRename, onRestart, onRetryNow, onUpdateTags, onCreateCheckpoint, onListCheckpoints, onForkFromCheckpoint, onSetRateLimitEnabled, onToggleAutonomousMode, onTogglePinned, onToggleAutoApprove, onSteerAutonomousSession, onClearConversationState, onHibernate, onResumeFromHibernation, selectMode = false, isSelected = false, onToggleSelect, reviewItem, detectedStatus, detectedContext, suppressApprovalSubStatus = false, staleThresholdMinutes = 30, backlogEntry, visibleColumns } = param;
    var _session_note, _session_externalMetadata, _session_externalMetadata1, _session_goal;
    const effectiveColumns = visibleColumns !== null && visibleColumns !== void 0 ? visibleColumns : CARD_DEFAULT_VISIBLE_COLUMNS;
    const sessionActions = (0,useSessionActions/* useSessionActions */.J)(session.id);
    const tagRuleNames = (0,useTaggingRuleNames/* useTaggingRuleNames */.Lk)();
    const [isTagEditorOpen, setIsTagEditorOpen] = (0,react.useState)(false);
    const [isDeleting, setIsDeleting] = (0,react.useState)(false);
    const [isInlineEditing, setIsInlineEditing] = (0,react.useState)(false);
    const [inlineEditValue, setInlineEditValue] = (0,react.useState)("");
    const [inlineEditError, setInlineEditError] = (0,react.useState)(null);
    const inlineSavingRef = (0,react.useRef)(false);
    const keyboardCommitRef = (0,react.useRef)(false);
    const cardRef = (0,react.useRef)(null);
    const snapshotToggleRef = (0,react.useRef)(null);
    const tagEditorTriggerRef = (0,react.useRef)(null);
    const [isSnapshotOpen, setIsSnapshotOpen] = (0,react.useState)(false);
    // Only fetch snapshot for active sessions (creating/paused/loading sessions have stale output).
    // SessionStatus.ACTIVE covers both ACTIVE and legacy RUNNING (same wire value = 1).
    const isSnapshotEnabled = session.status === types_pb/* SessionStatus */.Ph.ACTIVE && isSnapshotOpen;
    const isCreating = session.status === types_pb/* SessionStatus */.Ph.CREATING;
    const isFailed = session.status === types_pb/* SessionStatus */.Ph.FAILED;
    const isPaused = session.status === types_pb/* SessionStatus */.Ph.PAUSED;
    // Cancel/Retry guards (Epic 5.4, async-session-creation) -- shared with
    // SessionRow.tsx via useCreationLifecycleActions.
    const { cancelDisabled, retryDisabled, handleCancelCreation, handleRetryCreation } = useCreationLifecycleActions(session.id, isCreating);
    const pendingProgramChange = hasPendingProgramChange(session);
    const pendingAutoApproveChange = hasPendingAutoApproveChange(session);
    // Gated on AutoApproveSupported-equivalent so the badge can never claim a session is
    // unguarded when the agent doesn't actually support the injected flag (AC4 / pre-mortem
    // #4) -- e.g. autoApprove=true persisted for a since-unsupported program.
    const autoApproveFlagInjectable = session.autoApprove && (0,autoApprove/* isAutoApproveSupported */.$)(session.program);
    // pi approval-extension health badge (pi-support Epic 4.2, Story 4.2.2): shown only when
    // pi-support is on AND the session's program is one the extension actually covers.
    // session.piExtensionHealth is UNSPECIFIED for every other session (server never populates
    // it) as well as for a pi session before InstanceToProto's resolver is wired -- both cases
    // correctly fall through to piHealthBadgeInfo's "unknown" default if this guard is somehow
    // bypassed, but the guard itself is what keeps the badge from appearing on non-pi cards at all.
    const piSupportEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)(programs/* PI_SUPPORT_FLAG_NAME */.HN);
    const showPiHealthBadge = piSupportEnabled && (0,autoApprove/* isApprovalExtensionSupported */.H)(session.program);
    // AC11, explicit product decision (scoped out, not a silent gap): backlog automation's
    // headless review sessions (session/backlog_review.go, PermissionMode:
    // PermissionModeBypassPermissions) and any auto_yes-driven preset also bypass prompts,
    // without ever setting auto_approve -- but Session.permission_mode is not currently
    // exposed on the proto message (only Instance.PermissionMode, server-side), so badging
    // that population would require new proto+adapter plumbing beyond this feature's scope
    // (plan.md deliberately does not touch session/backlog_review.go). Deferred as a named
    // follow-up rather than silently missed; the badge's contract for now is strictly
    // "auto_approve is true and the agent actually supports the injected flag."
    const showAutoApproveBadge = !pendingAutoApproveChange && autoApproveFlagInjectable;
    const trimmedNote = (_session_note = session.note) === null || _session_note === void 0 ? void 0 : _session_note.trim();
    const noteTooltip = trimmedNote ? truncateGoal(trimmedNote, 120) : undefined;
    const { html: snapshotHtml, isEmpty: snapshotIsEmpty, loading: snapshotLoadingState, error: snapshotErrorMsg } = useTerminalSnapshot(session.id, isSnapshotEnabled);
    const getStatusColor = (sessionStatus)=>{
        switch(sessionStatus){
            case types_pb/* SessionStatus */.Ph.ACTIVE:
                return statusRunning;
            case types_pb/* SessionStatus */.Ph.READY:
                return statusReady;
            case types_pb/* SessionStatus */.Ph.PAUSED:
                return statusPausedDistinct; // distinct from STOPPED/HIBERNATED which use statusPaused
            case types_pb/* SessionStatus */.Ph.LOADING:
                return statusLoading;
            case types_pb/* SessionStatus */.Ph.CREATING:
                return statusLoading;
            case types_pb/* SessionStatus */.Ph.NEEDS_APPROVAL:
                return statusNeedsApproval;
            case types_pb/* SessionStatus */.Ph.STOPPED:
                return statusPaused;
            case types_pb/* SessionStatus */.Ph.HIBERNATED:
                return statusPaused; // no distinct style yet; reuses paused (session is idle/stopped)
            case types_pb/* SessionStatus */.Ph.CRASHED:
                return statusCrashed;
            case types_pb/* SessionStatus */.Ph.PERMANENTLY_FAILED:
                // Reuses CRASHED's error palette — never the same slot/style as a
                // routine NeedsAttention reason (ReviewQueueBadge), which is the
                // ambiguity this status exists to eliminate (research/ux.md).
                return statusCrashed;
            case types_pb/* SessionStatus */.Ph.FAILED:
                return statusCreationFailed;
            default:
                return statusUnknown;
        }
    };
    // WCAG 1.4.1 (icon + color, not color alone): CRASHED and FAILED are the
    // only two statuses that could otherwise be mistaken for one another by a
    // colorblind user (both render as a red/amber pill), so both get a distinct
    // glyph. Every other status is unambiguous from its label text alone.
    const getStatusIcon = (sessionStatus)=>{
        switch(sessionStatus){
            case types_pb/* SessionStatus */.Ph.CRASHED:
                return "✕";
            case types_pb/* SessionStatus */.Ph.FAILED:
                return "⚠";
            default:
                return null;
        }
    };
    const getStatusText = (sessionStatus)=>{
        switch(sessionStatus){
            case types_pb/* SessionStatus */.Ph.ACTIVE:
                return "Active";
            case types_pb/* SessionStatus */.Ph.READY:
                return "Ready";
            case types_pb/* SessionStatus */.Ph.PAUSED:
                return "Paused";
            case types_pb/* SessionStatus */.Ph.LOADING:
                return "Loading";
            case types_pb/* SessionStatus */.Ph.NEEDS_APPROVAL:
                return "Needs Approval";
            case types_pb/* SessionStatus */.Ph.CREATING:
                return "Starting…";
            case types_pb/* SessionStatus */.Ph.STOPPED:
                return "Stopped";
            case types_pb/* SessionStatus */.Ph.HIBERNATED:
                return "Hibernated";
            case types_pb/* SessionStatus */.Ph.CRASHED:
                return "Crashed";
            case types_pb/* SessionStatus */.Ph.PERMANENTLY_FAILED:
            case types_pb/* SessionStatus */.Ph.FAILED:
                return "Failed";
            default:
                return "Unknown";
        }
    };
    // Failure-reason categories are set verbatim by the async creation pipeline
    // (server/services/session_creation_pipeline.go, stale_creation_sweeper.go)
    // as session.instance_state.go's FailureReason() -- "GitHubResolutionError",
    // "StartupError", "Stale". Three distinct messages per plan.md Story 5.2.2 /
    // UX research §2/§4 (a stalled creation must never read as a user error).
    // session.failureReason is on the wire (types_pb.ts), so read it directly;
    // falls back to creation_progress, which the pipeline already sets to a
    // detailed message for GitHubResolutionError/StartupError (setPhase calls
    // in session_creation_pipeline.go) before the terminal write -- Stale does
    // not get a fresh setPhase call, so a Stale session without failureReason
    // falls back to the generic message instead of whatever stale progress
    // text it was last showing.
    const failureMessage = isFailed ? session.failureReason ? getFailureMessage(session.failureReason) : session.creationProgress || getFailureMessage("") : "";
    const formatResetTime = (ts)=>{
        if (!ts || ts.seconds === BigInt(0)) return "";
        const date = new Date(Number(ts.seconds) * 1000);
        return "until " + date.toLocaleTimeString([], {
            hour: "2-digit",
            minute: "2-digit"
        });
    };
    const getRateLimitStateText = (state)=>{
        switch(state){
            case types_pb/* RateLimitState */.nC.NONE:
                return "";
            case types_pb/* RateLimitState */.nC.WAITING:
                {
                    const resetStr = formatResetTime(session.rateLimitResetTime);
                    return resetStr ? "Rate limited ".concat(resetStr) : "Rate Limited";
                }
            case types_pb/* RateLimitState */.nC.RECOVERING:
                return "Recovering...";
            case types_pb/* RateLimitState */.nC.RECOVERED:
                return "Recovered";
            case types_pb/* RateLimitState */.nC.FAILED:
                return "Recovery Failed";
            default:
                return "";
        }
    };
    const getRateLimitStateColor = (state)=>{
        switch(state){
            case types_pb/* RateLimitState */.nC.NONE:
                return "";
            case types_pb/* RateLimitState */.nC.WAITING:
                return statusNeedsApproval;
            case types_pb/* RateLimitState */.nC.RECOVERING:
                return statusLoading;
            case types_pb/* RateLimitState */.nC.RECOVERED:
                return statusReady;
            case types_pb/* RateLimitState */.nC.FAILED:
                return statusPaused;
            default:
                return "";
        }
    };
    const formatDate = (ts)=>{
        if (!ts) return "N/A";
        const date = new Date(Number(ts.seconds) * 1000);
        return date.toLocaleString();
    };
    const formatTimeAgo = (ts)=>{
        if (!ts || ts.seconds === BigInt(0)) return "Never";
        const now = Date.now();
        const date = new Date(Number(ts.seconds) * 1000);
        const seconds = Math.floor((now - date.getTime()) / 1000);
        if (seconds < 60) return "".concat(seconds, "s ago");
        if (seconds < 3600) return "".concat(Math.floor(seconds / 60), "m ago");
        if (seconds < 86400) return "".concat(Math.floor(seconds / 3600), "h ago");
        return "".concat(Math.floor(seconds / 86400), "d ago");
    };
    const isExternal = session.instanceType === types_pb/* InstanceType */.y9.EXTERNAL;
    const sourceTerminal = ((_session_externalMetadata = session.externalMetadata) === null || _session_externalMetadata === void 0 ? void 0 : _session_externalMetadata.sourceTerminal) || "External";
    const muxEnabled = ((_session_externalMetadata1 = session.externalMetadata) === null || _session_externalMetadata1 === void 0 ? void 0 : _session_externalMetadata1.muxEnabled) || false;
    const handleCardClick = (e)=>{
        if (selectMode && onToggleSelect) {
            e.stopPropagation();
            onToggleSelect();
        } else if (e.altKey && onOpenInNewPane) {
            e.stopPropagation();
            onOpenInNewPane();
        } else if (onClick) {
            e.stopPropagation();
            onClick();
        }
    };
    const handleCardKeyDown = (e)=>{
        if ((e.key === "Enter" || e.key === " ") && !(e.target instanceof HTMLInputElement) && !(e.target instanceof HTMLTextAreaElement) && !(e.target instanceof HTMLButtonElement) && !(e.target instanceof HTMLAnchorElement) && !(e.target instanceof HTMLSelectElement)) {
            e.preventDefault();
            if (selectMode && onToggleSelect) {
                onToggleSelect();
            } else if (onClick) {
                onClick();
            }
        } else if (e.key === "F2" && !selectMode) {
            e.preventDefault();
            setInlineEditValue(session.title);
            setInlineEditError(null);
            setIsInlineEditing(true);
        }
    };
    const handleCheckboxClick = (e)=>{
        e.stopPropagation();
        onToggleSelect === null || onToggleSelect === void 0 ? void 0 : onToggleSelect();
    };
    const handleEditTags = (e)=>{
        e.stopPropagation();
        tagEditorTriggerRef.current = e.currentTarget;
        setIsTagEditorOpen(true);
    };
    const handleTitleClick = (e)=>{
        if (selectMode) return;
        e.stopPropagation();
        setInlineEditValue(session.title);
        setInlineEditError(null);
        setIsInlineEditing(true);
    };
    const handleInlineSave = async ()=>{
        if (inlineSavingRef.current) return;
        inlineSavingRef.current = true;
        const wasKeyboard = keyboardCommitRef.current;
        keyboardCommitRef.current = false;
        const trimmed = inlineEditValue.trim();
        try {
            if (!trimmed || trimmed === session.title) {
                setIsInlineEditing(false);
                setInlineEditError(null);
                if (wasKeyboard) setTimeout(()=>{
                    var _cardRef_current;
                    return (_cardRef_current = cardRef.current) === null || _cardRef_current === void 0 ? void 0 : _cardRef_current.focus();
                }, 0);
                return;
            }
            setIsInlineEditing(false);
            const success = await (onRename === null || onRename === void 0 ? void 0 : onRename(session.id, trimmed));
            if (!success) {
                // Re-open inline edit on failure so the user can correct
                setInlineEditValue(trimmed);
                setInlineEditError("Failed to save — try again");
                setIsInlineEditing(true);
            } else {
                setInlineEditError(null);
                if (wasKeyboard) setTimeout(()=>{
                    var _cardRef_current;
                    return (_cardRef_current = cardRef.current) === null || _cardRef_current === void 0 ? void 0 : _cardRef_current.focus();
                }, 0);
            }
        } catch (e) {
            setInlineEditValue(trimmed);
            setInlineEditError("Failed to save — try again");
            setIsInlineEditing(true);
        } finally{
            inlineSavingRef.current = false;
        }
    };
    const handleInlineKeyDown = (e)=>{
        if (e.key === "Enter") {
            keyboardCommitRef.current = true;
            handleInlineSave();
        } else if (e.key === "Escape") {
            setInlineEditValue(session.title);
            setInlineEditError(null);
            setIsInlineEditing(false);
            setTimeout(()=>{
                var _cardRef_current;
                return (_cardRef_current = cardRef.current) === null || _cardRef_current === void 0 ? void 0 : _cardRef_current.focus();
            }, 0);
        }
    };
    var _session_memoryRssMb, _session_goal_tasksTotal;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            isTagEditorOpen && onUpdateTags && /*#__PURE__*/ (0,jsx_runtime.jsx)(TagEditor/* TagEditor */.y, {
                tags: session.tags || [],
                tagProvenance: session.ruleTagProvenance,
                tagRuleNames: tagRuleNames,
                onSave: (newTags)=>{
                    onUpdateTags(session.id, newTags);
                    setIsTagEditorOpen(false);
                },
                onCancel: ()=>setIsTagEditorOpen(false),
                triggerRef: tagEditorTriggerRef,
                sessionTitle: session.title,
                sessionId: session.id,
                onReclassified: (appliedTags)=>{
                    onUpdateTags(session.id, appliedTags);
                }
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: [
                    card,
                    selectMode ? cardSelectMode : "",
                    isSelected ? cardSelected : "",
                    isExternal ? cardExternal : "",
                    isDeleting ? cardDeleting : "",
                    Number((_session_memoryRssMb = session.memoryRssMb) !== null && _session_memoryRssMb !== void 0 ? _session_memoryRssMb : 0n) > 500 ? cardMemoryPressure : "",
                    isPaused ? cardPaused : ""
                ].filter(Boolean).join(" "),
                ref: cardRef,
                "data-testid": "session-card",
                "data-paused": isPaused ? "true" : undefined,
                onClick: handleCardClick,
                onKeyDown: handleCardKeyDown,
                role: "group",
                "aria-roledescription": "session",
                tabIndex: 0,
                "aria-label": selectMode ? "".concat(isSelected ? "Selected" : "Not selected", ": ").concat(session.title) : !isInlineEditing ? "".concat(session.title, ", press F2 to rename") : session.title,
                "aria-keyshortcuts": !selectMode && !isInlineEditing ? "F2" : undefined,
                children: [
                    selectMode && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: SessionCard_css_checkbox,
                        "aria-hidden": "true",
                        onClick: handleCheckboxClick,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                            type: "checkbox",
                            checked: isSelected,
                            tabIndex: -1,
                            onClick: (e)=>e.stopPropagation(),
                            onChange: (e)=>{
                                e.stopPropagation();
                                onToggleSelect === null || onToggleSelect === void 0 ? void 0 : onToggleSelect();
                            }
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: header,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: titleRow,
                                children: [
                                    isInlineEditing ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        style: {
                                            position: 'relative',
                                            display: 'inline-block'
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                className: inlineTitleInput,
                                                value: inlineEditValue,
                                                autoFocus: true,
                                                onChange: (e)=>setInlineEditValue(e.target.value),
                                                onBlur: handleInlineSave,
                                                onKeyDown: handleInlineKeyDown,
                                                onClick: (e)=>e.stopPropagation(),
                                                "aria-label": "Edit session title",
                                                "aria-describedby": inlineEditError ? "inline-error-".concat(session.id) : undefined
                                            }),
                                            inlineEditError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                id: "inline-error-".concat(session.id),
                                                role: "alert",
                                                style: {
                                                    color: 'var(--error)',
                                                    fontSize: '0.75rem',
                                                    position: 'absolute',
                                                    top: '100%',
                                                    left: 0,
                                                    whiteSpace: 'nowrap',
                                                    zIndex: 1
                                                },
                                                children: inlineEditError
                                            })
                                        ]
                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: title,
                                            onClick: handleTitleClick,
                                            title: selectMode ? undefined : "Click to rename",
                                            style: selectMode ? undefined : {
                                                cursor: "text"
                                            },
                                            children: session.title
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: badges,
                                        children: [
                                            isExternal && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: externalBadge,
                                                role: "img",
                                                title: "External session from ".concat(sourceTerminal).concat(muxEnabled ? " (mux-enabled)" : ""),
                                                "aria-label": "External session from ".concat(sourceTerminal).concat(muxEnabled ? ", mux enabled" : ""),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83D\uDD17"
                                                    }),
                                                    " ",
                                                    sourceTerminal,
                                                    muxEnabled && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: muxIndicator,
                                                        "aria-hidden": "true",
                                                        children: "✓"
                                                    })
                                                ]
                                            }),
                                            session.remoteName && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: hostBadge,
                                                role: "img",
                                                title: "Running on ".concat(session.remoteName),
                                                "aria-label": "Running on ".concat(session.remoteName),
                                                "data-testid": "host-badge",
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83D\uDDA5️"
                                                    }),
                                                    " ",
                                                    session.remoteName
                                                ]
                                            }),
                                            session.remoteName && /*#__PURE__*/ (0,jsx_runtime.jsx)(RemoteConnectionIndicator, {
                                                remoteName: session.remoteName
                                            }),
                                            showPiHealthBadge && (()=>{
                                                const { icon, label, ariaLabel, className } = piHealthBadgeInfo(session.piExtensionHealth);
                                                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: "".concat(piHealthBadge, " ").concat(className),
                                                    role: "img",
                                                    title: ariaLabel,
                                                    "aria-label": ariaLabel,
                                                    "data-testid": "pi-health-badge",
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            "aria-hidden": "true",
                                                            children: icon
                                                        }),
                                                        " ",
                                                        label
                                                    ]
                                                });
                                            })(),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)(GitHubBadge/* GitHubBadge */.J, {
                                                prNumber: session.githubPrNumber,
                                                prUrl: session.githubPrUrl,
                                                owner: session.githubOwner,
                                                repo: session.githubRepo,
                                                sourceRef: session.githubSourceRef,
                                                host: session.githubHost,
                                                prPriority: session.githubPrPriority,
                                                prState: session.githubPrState,
                                                isDraft: session.githubPrIsDraft,
                                                approvedCount: session.githubApprovedCount,
                                                changesRequestedCount: session.githubChangesReqCount,
                                                checkConclusion: session.githubCheckConclusion,
                                                compact: true
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogOriginBadge/* BacklogOriginBadge */.J, {
                                                entry: backlogEntry,
                                                compact: true
                                            }),
                                            reviewItem && /*#__PURE__*/ (0,jsx_runtime.jsx)(ReviewQueueBadge/* ReviewQueueBadge */.b, {
                                                priority: reviewItem.priority,
                                                reason: reviewItem.reason,
                                                compact: true
                                            }),
                                            session.retryAttempt > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)(RetryBadge, {
                                                retryAttempt: session.retryAttempt,
                                                retryMaxAttempts: session.retryMaxAttempts,
                                                compact: true
                                            }),
                                            isPaused && session.pauseReason ? /*#__PURE__*/ (0,jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                                                label: (0,formatPauseReason/* formatPauseReason */.w)(session.pauseReason),
                                                side: "top",
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: "".concat(SessionCard_css_status, " ").concat(getStatusColor(session.status)),
                                                    role: "img",
                                                    "aria-label": "Session status: ".concat(getStatusText(session.status)),
                                                    "data-testid": "status-pill",
                                                    children: [
                                                        getStatusIcon(session.status) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: statusGlyphIcon,
                                                            "aria-hidden": "true",
                                                            children: getStatusIcon(session.status)
                                                        }),
                                                        getStatusText(session.status)
                                                    ]
                                                })
                                            }) : session.status === types_pb/* SessionStatus */.Ph.PERMANENTLY_FAILED ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: "".concat(SessionCard_css_status, " ").concat(getStatusColor(session.status)),
                                                role: "img",
                                                "aria-label": "Session status: Failed — gave up after ".concat(session.retryMaxAttempts, " attempt").concat(session.retryMaxAttempts === 1 ? "" : "s"),
                                                children: getStatusText(session.status)
                                            }) : session.status === types_pb/* SessionStatus */.Ph.STOPPED && session.creationProgress ? // ponytail: reuses creationProgress — the field is only cleared on a
                                            // successful start, so a startup/reconnect failure written here (see
                                            // instance.SetCreationProgress in health.go / connectrpc_websocket.go)
                                            // survives past the Creating phase and doubles as a "why stopped" reason.
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                                                label: session.creationProgress,
                                                side: "top",
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: "".concat(SessionCard_css_status, " ").concat(getStatusColor(session.status)),
                                                    role: "img",
                                                    "aria-label": "Session status: ".concat(getStatusText(session.status), " — ").concat(session.creationProgress),
                                                    "data-testid": "status-pill",
                                                    children: [
                                                        getStatusIcon(session.status) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: statusGlyphIcon,
                                                            "aria-hidden": "true",
                                                            children: getStatusIcon(session.status)
                                                        }),
                                                        getStatusText(session.status)
                                                    ]
                                                })
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: "".concat(SessionCard_css_status, " ").concat(getStatusColor(session.status)),
                                                role: "img",
                                                "aria-label": "Session status: ".concat(getStatusText(session.status)),
                                                "data-testid": "status-pill",
                                                children: [
                                                    getStatusIcon(session.status) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: statusGlyphIcon,
                                                        "aria-hidden": "true",
                                                        children: getStatusIcon(session.status)
                                                    }),
                                                    getStatusText(session.status)
                                                ]
                                            }),
                                            session.rateLimitState && session.rateLimitState !== types_pb/* RateLimitState */.nC.NONE && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: "".concat(SessionCard_css_status, " ").concat(getRateLimitStateColor(session.rateLimitState)),
                                                role: "img",
                                                "aria-label": "Rate limit: ".concat(getRateLimitStateText(session.rateLimitState)),
                                                children: getRateLimitStateText(session.rateLimitState)
                                            }),
                                            detectedStatus !== undefined && !(suppressApprovalSubStatus && (detectedStatus === types_pb/* DetectedStatus */.nB.NEEDS_APPROVAL || detectedStatus === types_pb/* DetectedStatus */.nB.INPUT_REQUIRED)) && (session.subStatus === types_pb/* SubStatus */.BJ.UNSPECIFIED || session.subStatus === types_pb/* SubStatus */.BJ.IDLE) && /*#__PURE__*/ (0,jsx_runtime.jsx)(StatusBadge/* StatusBadge */.Wh, {
                                                detectedStatus: detectedStatus,
                                                context: detectedContext
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)(RevivedContextBadge, {
                                                session: session
                                            }),
                                            session.status === types_pb/* SessionStatus */.Ph.ACTIVE && session.subStatus !== types_pb/* SubStatus */.BJ.UNSPECIFIED && session.subStatus !== types_pb/* SubStatus */.BJ.IDLE && !(suppressApprovalSubStatus && (session.subStatus === types_pb/* SubStatus */.BJ.NEEDS_APPROVAL || session.subStatus === types_pb/* SubStatus */.BJ.INPUT_REQUIRED)) && /*#__PURE__*/ (0,jsx_runtime.jsx)(SubStatusChip, {
                                                subStatus: session.subStatus,
                                                subagentCount: session.subagentCount
                                            }),
                                            (0,session_staleness/* isSessionStale */.G)(session, staleThresholdMinutes) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                role: "img",
                                                "aria-label": "Stale — no output for over ".concat(staleThresholdMinutes, " minutes"),
                                                className: "".concat(staleBadge),
                                                children: "\uD83D\uDFE0 Stale"
                                            }),
                                            (()=>{
                                                var _session_memoryRssMb;
                                                const mb = Number((_session_memoryRssMb = session.memoryRssMb) !== null && _session_memoryRssMb !== void 0 ? _session_memoryRssMb : 0n);
                                                if (mb <= 0) return null;
                                                if (!effectiveColumns.includes("memory")) return null;
                                                const severityClass = mb > 500 ? memoryBadgeHigh : mb > 300 ? memoryBadgeWarning : "";
                                                const label = mb >= 1024 ? "".concat((mb / 1024).toFixed(1), " GB RAM") : "".concat(mb, " MB RAM");
                                                return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: [
                                                        memoryBadge,
                                                        severityClass
                                                    ].filter(Boolean).join(" "),
                                                    role: "img",
                                                    title: "Process RSS: ".concat(mb, " MB"),
                                                    "aria-label": label,
                                                    children: label
                                                });
                                            })(),
                                            session.autonomousMode && (onToggleAutonomousMode ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: autonomousBadge,
                                                title: "Running under LLM orchestration — click to disable",
                                                "aria-label": "Auto-pilot active".concat(session.autonomousMaxTurns > 0 ? " (turn ".concat(session.autonomousTurn, "/").concat(session.autonomousMaxTurns, ")") : "", " — click to disable"),
                                                "data-testid": "badge-autonomous",
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    onToggleAutonomousMode(session.id, false);
                                                },
                                                children: session.autonomousMaxTurns > 0 ? "Auto-pilot ".concat(session.autonomousTurn, "/").concat(session.autonomousMaxTurns) : "Auto-pilot"
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: autonomousBadge,
                                                role: "img",
                                                title: "Running under LLM orchestration — injects prompts automatically",
                                                "aria-label": "Autonomous mode: session is controlled by LLM orchestration",
                                                "data-testid": "badge-autonomous",
                                                children: session.autonomousMaxTurns > 0 ? "Auto-pilot ".concat(session.autonomousTurn, "/").concat(session.autonomousMaxTurns) : "Auto-pilot"
                                            })),
                                            session.autonomousOutcome === "done" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: autonomousBadge,
                                                role: "img",
                                                style: {
                                                    background: "var(--success-bg)",
                                                    color: "var(--success)"
                                                },
                                                "data-testid": "badge-autonomous-done",
                                                "aria-label": "Autonomous run completed",
                                                children: [
                                                    "Done ",
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "✓"
                                                    })
                                                ]
                                            }),
                                            session.autonomousOutcome === "stuck" && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: autonomousBadge,
                                                role: "img",
                                                style: {
                                                    background: "var(--warning-bg)",
                                                    color: "var(--warning)"
                                                },
                                                "data-testid": "badge-autonomous-stuck",
                                                title: "Autonomous run stopped — open session to review and give next instruction",
                                                "aria-label": "Autonomous run stopped — needs attention",
                                                children: "Stuck"
                                            }),
                                            showAutoApproveBadge && // Direct-disable-on-click only for a non-Active session: SetAutoApprove
                                            // restarts an Active session unconditionally on disable too (AC6), so an
                                            // Active session's badge is deliberately non-interactive here -- disabling
                                            // it goes through the overflow menu's confirm dialog (restart notice),
                                            // not a silent one-click badge action that would restart the session with
                                            // no warning.
                                            (onToggleAutoApprove && session.status !== types_pb/* SessionStatus */.Ph.ACTIVE ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                className: autoApproveBadge,
                                                title: "Skipping all permission prompts — click to disable",
                                                "aria-label": "Auto-approve enabled — this session skips permission prompts; click to disable",
                                                "data-testid": "badge-auto-approve",
                                                onClick: (e)=>{
                                                    e.stopPropagation();
                                                    onToggleAutoApprove(session.id, false);
                                                },
                                                children: "⚡ Auto"
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: autoApproveBadge,
                                                role: "img",
                                                title: session.status === types_pb/* SessionStatus */.Ph.ACTIVE ? "Skipping all permission prompts — use the ⋯ menu to disable (restarts the session)" : "Skipping all permission prompts",
                                                "aria-label": "Auto-approve enabled: this session skips permission prompts",
                                                "data-testid": "badge-auto-approve",
                                                children: "⚡ Auto"
                                            })),
                                            session.workflowId && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: workflowBadge,
                                                role: "img",
                                                title: session.workflowName || session.workflowId,
                                                "aria-label": "Workflow: ".concat(session.workflowName || session.workflowId),
                                                "data-testid": "workflow-badge",
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "⚙"
                                                    }),
                                                    " ",
                                                    session.workflowName || "Workflow"
                                                ]
                                            }),
                                            pendingAutoApproveChange && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: autoApprovePendingBadge,
                                                role: "img",
                                                "data-testid": "badge-pending-auto-approve",
                                                title: "Auto-approve setting changed since this session last launched — takes effect on resume/restart",
                                                "aria-label": "Auto-approve change pending: takes effect on resume or restart",
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "⏳"
                                                    }),
                                                    " Auto-approve pending"
                                                ]
                                            }),
                                            pendingProgramChange && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: workflowBadge,
                                                role: "img",
                                                "data-testid": "badge-pending-program",
                                                title: "Program was changed since this session last launched — takes effect on resume/restart",
                                                "aria-label": "Program change pending: takes effect on resume or restart",
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "⏳"
                                                    }),
                                                    " Pending program change"
                                                ]
                                            }),
                                            noteTooltip && /*#__PURE__*/ (0,jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                                                label: noteTooltip,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: noteBadge,
                                                    role: "img",
                                                    "aria-label": "Has a note",
                                                    "data-testid": "badge-has-note",
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            "aria-hidden": "true",
                                                            children: "\uD83D\uDCDD"
                                                        }),
                                                        " Note"
                                                    ]
                                                })
                                            })
                                        ]
                                    })
                                ]
                            }),
                            session.category && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: category,
                                "aria-label": "Category: ".concat(session.category),
                                children: session.category
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: tagsContainer,
                                children: [
                                    session.tags && session.tags.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: tags,
                                        role: "list",
                                        "aria-label": "Session tags",
                                        children: session.tags.map((sessionTag)=>{
                                            const isUnclassified = sessionTag === tagProvenance/* UNCLASSIFIED_TAG */.Uw;
                                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: "".concat(tag, " ").concat(isUnclassified ? tagUnclassified : ""),
                                                role: "listitem",
                                                tabIndex: 0,
                                                title: (0,tagProvenance/* tagProvenanceTitle */.en)(sessionTag, session.ruleTagProvenance, tagRuleNames),
                                                "aria-label": (0,tagProvenance/* tagProvenanceAriaLabel */.bT)(sessionTag, session.ruleTagProvenance),
                                                children: [
                                                    isUnclassified && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "? "
                                                    }),
                                                    sessionTag
                                                ]
                                            }, sessionTag);
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: editTagsButton,
                                        onClick: handleEditTags,
                                        title: "Edit tags",
                                        "aria-label": "".concat(session.tags && session.tags.length > 0 ? "Edit" : "Add", " tags for ").concat(session.title),
                                        tabIndex: selectMode ? -1 : undefined,
                                        "aria-hidden": selectMode ? "true" : undefined,
                                        inert: selectMode || undefined,
                                        children: session.tags && session.tags.length > 0 ? "Edit Tags" : "Add Tags"
                                    })
                                ]
                            }),
                            reviewItem && !selectMode && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: reviewInfo,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ReviewQueueBadge/* ReviewQueueBadge */.b, {
                                        priority: reviewItem.priority,
                                        reason: reviewItem.reason,
                                        compact: false
                                    }),
                                    reviewItem.context && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: reviewContext,
                                        children: reviewItem.context
                                    })
                                ]
                            }),
                            (()=>{
                                const lastActivity = (0,session_staleness/* getLastActivityTimestamp */.w)(session);
                                return lastActivity ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: lastActivityRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: lastActivityLabel,
                                            children: "Active"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("time", {
                                            dateTime: new Date(Number(lastActivity.seconds) * 1000).toISOString(),
                                            title: new Date(Number(lastActivity.seconds) * 1000).toISOString(),
                                            className: lastActivityTime,
                                            children: formatTimeAgo(lastActivity)
                                        })
                                    ]
                                }) : null;
                            })()
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: body,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: info,
                                children: [
                                    effectiveColumns.includes("agent") && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: infoRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: label,
                                                children: "Program:"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: value,
                                                children: session.program
                                            })
                                        ]
                                    }),
                                    session.branch && !isRedundantWithTitle(session.branch, session.title) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: infoRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: label,
                                                children: "Branch:"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: value,
                                                children: session.branch
                                            })
                                        ]
                                    }),
                                    session.existingDir && !isPathRedundantWithTitle(session.existingDir, session.title) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: infoRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: label,
                                                children: "Path:"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: value,
                                                title: session.existingDir,
                                                children: truncateWorkspacePath(session.existingDir, CARD_PATH_MAX_LEN)
                                            })
                                        ]
                                    }),
                                    session.activeDir && !isPathRedundantWithTitle(session.activeDir, session.title) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: infoRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: label,
                                                children: "Working Dir:"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: value,
                                                title: session.activeDir,
                                                children: truncateWorkspacePath(session.activeDir, CARD_PATH_MAX_LEN)
                                            })
                                        ]
                                    }),
                                    session.githubOwner && session.githubRepo && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: infoRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: label,
                                                children: "Repository:"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: value,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
                                                    href: "https://".concat(session.githubHost || "github.com", "/").concat(session.githubOwner, "/").concat(session.githubRepo),
                                                    target: "_blank",
                                                    rel: "noopener noreferrer",
                                                    onClick: (e)=>e.stopPropagation(),
                                                    className: githubLink,
                                                    "aria-label": "GitHub repository ".concat(session.githubOwner, "/").concat(session.githubRepo, ", opens in new tab"),
                                                    children: [
                                                        session.githubOwner,
                                                        "/",
                                                        session.githubRepo
                                                    ]
                                                })
                                            })
                                        ]
                                    }),
                                    session.githubPrNumber > 0 && session.githubPrUrl && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: infoRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: label,
                                                children: "Pull Request:"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: value,
                                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("a", {
                                                    href: session.githubPrUrl,
                                                    target: "_blank",
                                                    rel: "noopener noreferrer",
                                                    onClick: (e)=>e.stopPropagation(),
                                                    className: githubLink,
                                                    "aria-label": "Pull request #".concat(session.githubPrNumber, " on ").concat(session.githubOwner, "/").concat(session.githubRepo, ", opens in new tab"),
                                                    children: [
                                                        "#",
                                                        session.githubPrNumber
                                                    ]
                                                })
                                            })
                                        ]
                                    }),
                                    session.clonedRepoPath && !isPathRedundantWithTitle(session.clonedRepoPath, session.title) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: infoRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: label,
                                                children: "Cloned To:"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: value,
                                                title: session.clonedRepoPath,
                                                children: truncateWorkspacePath(session.clonedRepoPath, CARD_PATH_MAX_LEN)
                                            })
                                        ]
                                    }),
                                    ((_session_goal = session.goal) === null || _session_goal === void 0 ? void 0 : _session_goal.goalText) && !isRedundantWithTitle(session.goal.goalText, session.title) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: infoRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: label,
                                                children: "Goal"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: value,
                                                children: [
                                                    truncateGoal(session.goal.goalText, 61),
                                                    ((_session_goal_tasksTotal = session.goal.tasksTotal) !== null && _session_goal_tasksTotal !== void 0 ? _session_goal_tasksTotal : 0) > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: taskFraction,
                                                        children: " \xb7 ".concat(session.goal.tasksDone, "/").concat(session.goal.tasksTotal, " done")
                                                    })
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            }),
                            session.diffStats && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: diffStats,
                                role: "img",
                                "aria-label": "Diff: +".concat(session.diffStats.added, " additions, -").concat(session.diffStats.removed, " deletions"),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: diffAdded,
                                        "aria-hidden": "true",
                                        children: [
                                            "+",
                                            session.diffStats.added
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: diffRemoved,
                                        "aria-hidden": "true",
                                        children: [
                                            "-",
                                            session.diffStats.removed
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                role: "status",
                                "aria-live": isFailed ? "assertive" : "polite",
                                id: "creation-status-".concat(session.id),
                                "data-testid": "creation-live-region",
                                style: {
                                    position: "absolute",
                                    width: 1,
                                    height: 1,
                                    overflow: "hidden",
                                    clipPath: "inset(50%)",
                                    whiteSpace: "nowrap"
                                },
                                children: isFailed ? failureMessage : isCreating ? session.creationProgress || "Starting session..." : ""
                            }),
                            isCreating && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                style: {
                                    display: "flex",
                                    alignItems: "center",
                                    justifyContent: "space-between",
                                    gap: "8px",
                                    padding: "8px 0",
                                    color: "var(--text-secondary)",
                                    fontSize: "0.875rem"
                                },
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        style: {
                                            display: "flex",
                                            alignItems: "center",
                                            gap: "8px"
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: creationSpinner,
                                                "aria-hidden": "true"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                "aria-hidden": "true",
                                                "data-testid": "session-progress-text",
                                                children: session.creationProgress || "Starting session..."
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        onClick: handleCancelCreation,
                                        disabled: cancelDisabled,
                                        "aria-label": "Cancel session creation",
                                        "data-testid": "cancel-creation-button",
                                        className: "".concat(SessionCard_css_actionButton, " ").concat(actionButtonCompact),
                                        children: "Cancel"
                                    })
                                ]
                            }),
                            isFailed && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                style: {
                                    display: "flex",
                                    alignItems: "center",
                                    justifyContent: "space-between",
                                    gap: "8px",
                                    padding: "8px 0",
                                    color: "var(--text-secondary)",
                                    fontSize: "0.875rem"
                                },
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        style: {
                                            display: "flex",
                                            alignItems: "center",
                                            gap: "8px"
                                        },
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: failureMessageIcon,
                                                "aria-hidden": "true",
                                                children: "⚠"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                "data-testid": "failure-message",
                                                children: failureMessage
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        onClick: handleRetryCreation,
                                        disabled: retryDisabled,
                                        "aria-label": "Retry creating session",
                                        "data-testid": "retry-creation-button",
                                        className: "".concat(SessionCard_css_actionButton, " ").concat(actionButtonCompact),
                                        children: "Retry"
                                    })
                                ]
                            }),
                            session.status === types_pb/* SessionStatus */.Ph.ACTIVE && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: snapshotSection,
                                onClick: (e)=>e.stopPropagation(),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        ref: snapshotToggleRef,
                                        className: snapshotToggle,
                                        onClick: ()=>setIsSnapshotOpen((prev)=>!prev),
                                        "aria-expanded": isSnapshotOpen,
                                        "aria-label": isSnapshotOpen ? "Collapse terminal preview" : "Expand terminal preview",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                children: "Terminal Preview"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: snapshotToggleIcon,
                                                "aria-hidden": "true",
                                                children: isSnapshotOpen ? "▲" : "▼"
                                            })
                                        ]
                                    }),
                                    isSnapshotOpen && (snapshotLoadingState ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: snapshotLoading,
                                        role: "status",
                                        children: "Loading…"
                                    }) : snapshotErrorMsg ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: snapshotError.base,
                                        role: "alert",
                                        children: "Failed to load preview"
                                    }) : snapshotIsEmpty ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: snapshotEmpty,
                                        role: "status",
                                        children: "No recent output"
                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: snapshotPane,
                                        // Safe: content is rendered by ansi-to-html with escapeXML enabled,
                                        // or escaped manually in the plain-text fallback path.
                                        role: "region",
                                        dangerouslySetInnerHTML: {
                                            __html: snapshotHtml
                                        },
                                        "aria-label": "Terminal output preview",
                                        tabIndex: 0,
                                        onKeyDown: (e)=>{
                                            if (e.key === "Escape") {
                                                var _snapshotToggleRef_current;
                                                e.currentTarget.blur();
                                                (_snapshotToggleRef_current = snapshotToggleRef.current) === null || _snapshotToggleRef_current === void 0 ? void 0 : _snapshotToggleRef_current.focus();
                                            }
                                        }
                                    }))
                                ]
                            })
                        ]
                    }),
                    IS_DEBUG_MODE && /*#__PURE__*/ (0,jsx_runtime.jsx)(DetectionEventsPanel, {
                        sessionId: session.id,
                        program: session.program
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: footer,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: timestamps,
                                children: session.updatedAt && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: timestamp,
                                    title: "Created: ".concat(formatDate(session.createdAt), "\nUpdated: ").concat(formatDate(session.updatedAt)),
                                    children: [
                                        "Updated ",
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("time", {
                                            dateTime: new Date(Number(session.updatedAt.seconds) * 1000).toISOString(),
                                            children: formatTimeAgo(session.updatedAt)
                                        })
                                    ]
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionActionsOverflow, {
                                session: session,
                                showPrimaryAction: true,
                                onResume: onResume,
                                onPause: onPause,
                                onHibernate: onHibernate,
                                onResumeFromHibernation: onResumeFromHibernation,
                                onDelete: async ()=>{
                                    setIsDeleting(true);
                                    try {
                                        await (onDelete === null || onDelete === void 0 ? void 0 : onDelete());
                                    } finally{
                                        setIsDeleting(false);
                                    }
                                },
                                onRestart: onRestart,
                                onRetryNow: onRetryNow,
                                onClone: onClone,
                                onOpenInNewPane: onOpenInNewPane,
                                onNewWorkspace: onNewWorkspace,
                                onCreateCheckpoint: onCreateCheckpoint,
                                onSetRateLimitEnabled: onSetRateLimitEnabled,
                                onToggleAutonomousMode: onToggleAutonomousMode,
                                onTogglePinned: onTogglePinned,
                                onToggleAutoApprove: onToggleAutoApprove,
                                onSteerAutonomousSession: onSteerAutonomousSession,
                                onClearConversationState: onClearConversationState,
                                onUpdateTags: onUpdateTags,
                                onChangeProgram: async (_id, program)=>{
                                    const result = await sessionActions.update({
                                        program
                                    });
                                    if (!result) throw new Error("Failed to change program.");
                                }
                            })
                        ]
                    })
                ]
            })
        ]
    });
}
const SessionCard = /*#__PURE__*/ (0,react.memo)(SessionCardInner);

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionRow.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VZW27jNhT99yqIAAPEwDCVbMeTKBgga-jvoBjQ0pXFiUSqJOVHiqyiv11dV1KQomyRkizF8-hPAvNx7ou89_Dq-QWOqSAFSPQ1XPIS9i8B-muGUPDB_EOIlySm6hih8GmG0NsMofvuXHC3ambDYHDr2-y5K4674jZcJCBwDqnCMc-5iNCOiFuMv4aP8LoQr8W8q8eETet1j4KThb3N7hp9Q7M3obLMyTFCW0ETvUb_xwqKMicKNFhVMBmhxao8oIfygMJUoEVQHhCpFDd_9C6S0y3DVEEhI5TmcMBSEaEMICl9bRavRp2SJAll2witNe6iPOjBgjKcAd1mKkLLh3osroTUNpWcMgVCD1mLBUloJX385drg51QqLNUxhwgxzsDI5JIqylmEBORE0Z0ZjTlThDIQWB1LiBBlOWWAJX31phkpIEISpKSc_c73T45Ho4zvQBi_bkj8shW8Yomv3MPc2bQwy7XVe5qoLELBUzss2pd6wPg0oQLiWvs6MHrmWyUVTY9YKwlMRSiGxknG9cavLYFLN-6NACeCHsTqHJqzki3IVT-k0XkvNIL-e5UUhFLOlIlEJ8rE9eS90cJutSenOUn25-VT8zg_aS0zQdmL1aAgYksZVrw0R9UV-iUhiujTrir5-UZUjFG2vflj5BAsvFPgwZSkkpCMozxeRKFJDuMYvhddjJyTZJJBl2EYQCIxKUvBdyQfRSsmeAfbWzgK9noRLKMbEIyoKe6-bGMsiMwmwITBRZiU0HwCysEFWZvlw3dlMz_dpb29E96KbF6nup7qEZgpndvSnO_tlSbsuM9AgKPGp2lqdK7Y1HTUvoor7yo-nGWnpKD5sRO_aj6eUAZcsJj35aYpLnn0XRKGtqoNyzGrd0RQwhRmVQGCxhFSZFPlROgB6WmztNVTwUFh47sICR1mRxVyder_Hp-1QubXok2_Qn2U4TgfVLUFGPcDnvldb8H2trfpnrM65XEl8Z6qjLKpm-rLTUzZlnhHJd3k8PlGiUrn5mkgiZk80aVFTcXOBS1CYXlAkuc08dwG86lkaTjTxGOJYexwXMo62_kAwdtnVAGWJYkNe2sIhOFlTVUP7-5dP00mYCef9JpF3dwKHcCGTTbub3732HHhZjgBvZ_CUBpe2_bBe9LnIFdsW9ty4oWgTya46a_w3mq698aK08_1bvqDvbt1E559uIwo-h3ZfFpl6njCVHxgiaN7NuyFQzdxrAM3edPh3WHQ3f7J2_7tpzKVlqCXayI0XAEnJdsfFKD8ZzKq0CGVEcpokgA7kZjzBOQ5LSWVF6pCQQ4NEwp1c8KxougkIKMPLujhljIkxXbz0dPtOEer4MNHpARhsiQCmHKvHasx63TTdAt0x6NOR32l2HuGlMMIywEE71X0Z905qOmVRQBt3rL0SM_d6t4L02Ob1jUdElGQ3JFQp6kmFy5skn0Xf7zU-DH9qtPbvBk0RPFhGu288j1TpzJzzEyEUy6KCFVlCSIm0tSiHJQCYU6aKTLBXXAPRX9XqeUx-S6GfaF1U--QkKcRkkqAirOBEmkoJc1NpM93yC7AsAOmZI-mHe5bq95Gs2S1D67u-73N7NMVcogVLnjSw2yvwj3tVu2mTrh2uzrN70n8dhpTHmPBXf-feMi7HrJTuIL6otM4jjOIXyA5eXa8dXKZ3XpppF9KFJFUnXiK1fHm33_-vmndKpONvXsaBnVMfBrVklgNmGAbOvY4QYI3W1fV3a99v_64CvU2ey4goQTdlgJSEBILSKoYElzwup_LOK6ngMUwN3Z6nXpUVyPbvfasYKePCdM6kwgRRguisWxP-_TV5MmbTipBapkL6c9RBfUkjnmlTwhlKWX2ULQXKlro3J5WzPavgUjAlGFeqTHNe3uHVxkQ3v0qE-KrQsbGd_HurnLEIfx_iujb7Pn04aT1yQTdttjaIgjKg3fc19aesTs7TcJy1ZWwsRLMt676S0qEQvQbwmFt5gV6bOXa62xqaF1dPRnNCeh-dvQq42BtHKqOrgZ2jSa1REhfi8SKOBUpnZ5RaLOi-7VtZUcdHcEC9C10OpDnQUdWB1BZwM62rhpv_wED2SGa1R0AAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionRow_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VZW27jNhT99yqIAAPEwDCVbMeTKBgga_jvoBjQ0pXFiUSqJOVHiqyiv11dV1KQomyRkizF8_hPAvNx7ou89_Dq_QWOqSAFSPQ1XPIS9i8B_muGUPDB_EOIlySm6hih8GmG0NsMofvuXHC3ambDYHDr2_y5K4674jZcJCBwDqnCMc_5iNCOiFuMv4aP8LoQr8W8q8eETet1j4KThb3N7hp9Q7M3obLMyTFCW0ETvUb_xwqKMicKNFhVMBmhxao8oIfygMJUoEVQHhCpFDd_9C6S0y3DVEEhI5TmcMBSEaEMICl9bRavRp2SJAll2witNe6iPOjBgjKcAd1mKkLLh3osroTUNpWcMgVCD1mLBUloJX385drg51QqLNUxhwgxzsDI5JIqylmEBORE0Z0ZjTlThDIQWB1LiBBlOWWAJX31phkpIEISpKSc_c73T45Ho4zvQBi_bkj8shW8Yomv3MPc2bQwy7XVe5qoLELBUzss2pd6wPg0oQLiWvs6MHrmWyUVTY9YKwlMRSiGxknG9cavLYFLN_6NACeCHsTqHJqzki3IVT_k0XkvNIL_e5UUhFLOlIlEJ8rE9eS90cJutSenOUn25_VT8zg_aS0zQdmL1aAgYksZVrw0R9UV_iUhiujTrir5_UZUjFG2vflj5BAsvFPgwZSkkpCMozxeRKFJDuMYvhddjJyTZJJBl2EYQCIxKUvBdyQfRSsmeAfbWzgK9noRLKMbEIyoKe6_bGMsiMwmwITBRZiU0HwCysEFWZvlw3dlMz_dpb29E96KbF6nup7qEZgpndvSnO_tlSbsuM9AgKPGp2lqdK7Y1HTUvoor7yo_nGWnpKD5sRO_aj6eUAZcsJj35aYpLnn0XRKGtqoNyzGrd0RQwhRmVQGCxhFSZFPlROgB6WmztNVTwUFh47sICR1mRxVyder_Hp_1QubXok2_Qn2U4TgfVLUFGPcDnvldb8H2trfpnrM65XEl8Z6qjLKpm_rLTUzZlnhHJd3k8PlGiUrn5mkgiZk80aVFTcXOBS1CYXlAkuc08dwG86lkaTjTxGOJYexwXMo62_kAwdtnVAGWJYkNe2sIhOFlTVUP7_5dP00mYCef9JpF3dwKHcCGTTbub3732HHhZjgBvZ_CUBpe2_bBe9LnIFdsW9ty4oWgTya46a_w3mq698aK08_1bvqDvbt1E559uIwo_h3ZfFpl6njCVHxgiaN7NuyFQzdxrAM3edPh3WHQ3f7J2_7tpzKVlqCXayI0XAEnJdsfFKD8ZzKq0CGVEcpokgA7kZjzBOQ5LSWVF6pCQQ4NEwp1c8KxougkIKMPLujhljIkxXbz0dPtOEer4MNHpARhsiQCmHKvHasx63TTdAt0x6NOR32l2HuGlMMIywEE71X0Z905qOmVRQBt3rL0SM_d6t4L02Ob1jUdElGQ3JFQp6kmFy5skn0Xf7zU_DH9qtPbvBk0RPFhGu288j1TpzJzzEyEUy6KCFVlCSIm0tSiHJQCYU6aKTLBXXAPRX9XqeUx_S6GfaF1U_QkKcRkkqAirOBEmkoJc1NpM93yC7AsAOmZI_mHe5bq95Gs2S1D67u_73N7NMVcogVLnjSw2yvwj3tVu2mTrh2uzrN70n8dhpTHmPBXf_feMi7HrJTuIL6otM4jjOIXyA5eXa8dXKZ3XpppF9KFJFUnXiK1fHm33_vmndKpONvXsaBnVMfBrVklgNmGAbOvY4QYI3W1fV3a99v_64CvU2ey4goQTdlgJSEBILSKoYElzwup_LOK6ngMUwN3Z6nXpUVyPbvfasYKePCdM6kwgRRguisWxP_TV5MmbTipBapkL6c9RBfUkjnmlTwhlKWX2ULQXKlro3J5WzPavgUjAlGFeqTHNe3uHVxkQ3v0qE_KrQsbGd_HurnLEIfx_iujb7Pn04aT1yQTdttjaIgjKg3fc19aesTs7TcJy1ZWwsRLMt676S0qEQvQbwmFt5gV6bOXa62xqaF1dPRnNCeh_dvQq42BtHKqOrgZ2jSa1REhfi8SKOBUpnZ5RaLOi_7VtZUcdHcEC9C10OpDnQUdWB1BZwM62rhpv_wED2SGa1R0AAA_22_7D = __webpack_require__(57486);
;// ./src/components/sessions/SessionRow.css.ts

var SessionRow_css_actionButton = '_13opewkf';
var actions = '_13opewkb';
var agentIcon = '_13opewk7';
var branchCell = '_13opewkl';
var checkboxButton = '_13opewkt';
var checkboxCell = '_13opewks';
var chipsLine = '_13opewk4';
var diffBadge = '_13opewkk';
var elapsed = '_13opewk9';
var elapsedIcon = '_13opewkq';
var elapsedSecondLine = '_13opewka';
var failureMessageLine = '_13opewkv';
var groupHeader = '_13opewkr';
var inlineActionButton = '_13opewkd';
var SessionRow_css_memoryBadge = '_13opewkg';
var SessionRow_css_memoryBadgeHigh = '_13opewki';
var SessionRow_css_memoryBadgeWarning = '_13opewkh';
var SessionRow_css_name = '_13opewk6';
var nameCell = '_13opewk2';
var noteIndicator = '_13opewkj';
var path = '_13opewk8';
var pathLine = '_13opewk3';
var primaryActionWrapper = '_13opewkc';
var row = '_13opewk1';
var rowActive = '_13opewkp';
var rowMemoryPressure = '_13opewkm';
var rowOverflowButton = '_13opewke';
var rowPaused = '_13opewkn';
var rowSelected = '_13opewku';
var statusDot = '_13opewk5';
;// ./src/components/sessions/SessionRow.tsx
/* __next_internal_client_entry_do_not_use__ SessionRow auto */ 
// +feature: remote-host-badge



















// Module-level constant avoids repeated BigInt(0) allocations in hot render paths.
const BIGINT_ZERO = BigInt(0);
// Name/path are never length-truncated — full text always renders, wrapping
// via `overflowWrap: anywhere` (SessionRow.css.ts) as needed. `Infinity`
// keeps `truncateWorkspacePath`'s home-dir "~" collapsing (not lossy) while
// skipping its length-based truncation entirely.
const ROW_PATH_MAX_LEN = Infinity;
function getStatusDotValue(status) {
    switch(status){
        case types_pb/* SessionStatus */.Ph.ACTIVE:
            return "running";
        case types_pb/* SessionStatus */.Ph.READY:
            return "idle";
        case types_pb/* SessionStatus */.Ph.PAUSED:
            return "paused-session";
        case types_pb/* SessionStatus */.Ph.STOPPED:
            return "paused";
        case types_pb/* SessionStatus */.Ph.LOADING:
        case types_pb/* SessionStatus */.Ph.CREATING:
            return "loading";
        case types_pb/* SessionStatus */.Ph.NEEDS_APPROVAL:
            return "needs-approval";
        case types_pb/* SessionStatus */.Ph.HIBERNATED:
            return "hibernated";
        case types_pb/* SessionStatus */.Ph.CRASHED:
            return "crashed";
        case types_pb/* SessionStatus */.Ph.PERMANENTLY_FAILED:
            return "crashed";
        case types_pb/* SessionStatus */.Ph.FAILED:
            return "failed";
        default:
            return "idle";
    }
}
const STATUS_DOT_LABELS = {
    running: "Running",
    idle: "Idle",
    "paused-session": "Paused",
    paused: "Stopped",
    loading: "Loading",
    "needs-approval": "Needs Approval",
    hibernated: "Hibernated",
    crashed: "Crashed",
    failed: "Failed"
};
function getStatusDotLabel(dotValue) {
    var _STATUS_DOT_LABELS_dotValue;
    return (_STATUS_DOT_LABELS_dotValue = STATUS_DOT_LABELS[dotValue]) !== null && _STATUS_DOT_LABELS_dotValue !== void 0 ? _STATUS_DOT_LABELS_dotValue : dotValue;
}
function formatElapsed(ts) {
    if (!ts || ts.seconds === BIGINT_ZERO) return "";
    const now = Date.now();
    const date = new Date(Number(ts.seconds) * 1000);
    const seconds = Math.floor((now - date.getTime()) / 1000);
    if (seconds < 60) return "".concat(seconds, "s");
    if (seconds < 3600) return "".concat(Math.floor(seconds / 60), "m");
    if (seconds < 86400) return "".concat(Math.floor(seconds / 3600), "h");
    return "".concat(Math.floor(seconds / 86400), "d");
}
function getAgentEmoji(program) {
    const p = program.toLowerCase();
    if (p.includes("claude")) return "✦";
    if (p.includes("aider")) return "⚡";
    if (p.includes("cursor")) return "◎";
    if (p.includes("copilot")) return "◈";
    if (p.includes("gpt") || p.includes("openai")) return "◉";
    if (p.includes("gemini")) return "◆";
    if (p.includes("agy") || p.includes("antigravity")) return "◆";
    return "◇";
}
function getLastActivity(session) {
    var _session_lastMeaningfulOutput, _session_lastTerminalUpdate;
    var _session_lastMeaningfulOutput_seconds;
    const moSecs = (_session_lastMeaningfulOutput_seconds = (_session_lastMeaningfulOutput = session.lastMeaningfulOutput) === null || _session_lastMeaningfulOutput === void 0 ? void 0 : _session_lastMeaningfulOutput.seconds) !== null && _session_lastMeaningfulOutput_seconds !== void 0 ? _session_lastMeaningfulOutput_seconds : BIGINT_ZERO;
    var _session_lastTerminalUpdate_seconds;
    const tuSecs = (_session_lastTerminalUpdate_seconds = (_session_lastTerminalUpdate = session.lastTerminalUpdate) === null || _session_lastTerminalUpdate === void 0 ? void 0 : _session_lastTerminalUpdate.seconds) !== null && _session_lastTerminalUpdate_seconds !== void 0 ? _session_lastTerminalUpdate_seconds : BIGINT_ZERO;
    if (moSecs === BIGINT_ZERO && tuSecs === BIGINT_ZERO) return undefined;
    return moSecs >= tuSecs ? session.lastMeaningfulOutput : session.lastTerminalUpdate;
}
// Builds the row's full-context aria-label (status + optional path/agent/memory/context
// fields) as a single string so it stays screen-reader-accessible regardless of which
// optional columns are visible in the grid — see instance-lock-free-reads-style
// "name assumptions" note: relies on getStatusDotLabel already handling unknown statuses.
function buildSessionRowAriaLabel(session, dotStatus, memMB) {
    const parts = [
        "Session ".concat(session.title, ", status: ").concat(getStatusDotLabel(dotStatus))
    ];
    if (session.existingDir) parts.push(", path: ".concat(session.existingDir));
    if (session.program) parts.push(", agent: ".concat(session.program));
    if (memMB > 0) parts.push(", memory: ".concat(memMB, " MB"));
    if (hasLostContext(session)) parts.push(", context: lost");
    return parts.join("");
}
function SessionRowInner(param) {
    let { session, onClick, onPause, onResume, onDelete, onClone, onOpenInNewPane, onNewWorkspace, onRestart, onRetryNow, onCreateCheckpoint, onSetRateLimitEnabled, onToggleAutonomousMode, onTogglePinned, onToggleAutoApprove, onSteerAutonomousSession, onClearConversationState, onUpdateTags, onHibernate, onResumeFromHibernation, suppressApprovalSubStatus = false, staleThresholdMinutes = 30, visibleColumns = DEFAULT_VISIBLE_COLUMNS, selectMode = false, isSelected = false, onToggleSelect, backlogEntry } = param;
    var _session_note;
    const overflowRef = (0,react.useRef)(null);
    const sessionActions = (0,useSessionActions/* useSessionActions */.J)(session.id);
    const dotStatus = getStatusDotValue(session.status);
    const isPaused = session.status === types_pb/* SessionStatus */.Ph.PAUSED;
    const isNeedsApproval = session.status === types_pb/* SessionStatus */.Ph.NEEDS_APPROVAL;
    const isHibernated = session.status === types_pb/* SessionStatus */.Ph.HIBERNATED;
    const isRunning = session.status === types_pb/* SessionStatus */.Ph.ACTIVE;
    const isCreating = session.status === types_pb/* SessionStatus */.Ph.CREATING;
    const isFailed = session.status === types_pb/* SessionStatus */.Ph.FAILED;
    // Cancel/Retry guards (Epic 5.4, async-session-creation) -- shared with
    // SessionCard.tsx via useCreationLifecycleActions.
    const { cancelDisabled, retryDisabled, handleCancelCreation, handleRetryCreation } = useCreationLifecycleActions(session.id, isCreating);
    // Sessions needing user attention always show their primary action --
    // Creating/Failed included so Cancel/Retry are clickable immediately
    // without requiring a hover (Epic 5.4/Story 5.4.1's "clickable
    // immediately when creation starts, not gated on any delay").
    const actionsAlwaysVisible = isPaused || isNeedsApproval || isHibernated || isCreating || isFailed;
    // session.failureReason is on the wire (types_pb.ts) -- read it directly.
    // Falls back to creation_progress, which the pipeline already sets to a
    // detailed message for GitHubResolutionError/StartupError before the
    // terminal write; Stale doesn't get a fresh setPhase call, so a Stale
    // session without failureReason falls back to the generic message instead
    // of whatever stale progress text it was last showing (same behavior as
    // SessionCard.tsx, which shares this mapping via lib/utils/sessionFailure).
    const failureMessage = isFailed ? session.failureReason ? getFailureMessage(session.failureReason) : session.creationProgress || getFailureMessage("") : "";
    const lastActivity = getLastActivity(session);
    const elapsedText = formatElapsed(lastActivity !== null && lastActivity !== void 0 ? lastActivity : session.updatedAt);
    // Show branch separately if the branch column is visible; otherwise fold into displayName.
    const showBranchCol = visibleColumns.includes("branch");
    const displayName = showBranchCol ? session.title : session.branch || session.title;
    const trimmedNote = (_session_note = session.note) === null || _session_note === void 0 ? void 0 : _session_note.trim();
    const noteTooltip = trimmedNote ? truncateGoal(trimmedNote, 120) : undefined;
    var _session_memoryRssMb;
    const memMB = Number((_session_memoryRssMb = session.memoryRssMb) !== null && _session_memoryRssMb !== void 0 ? _session_memoryRssMb : 0n);
    const memorySeverityClass = memMB > 500 ? SessionRow_css_memoryBadgeHigh : memMB > 300 ? SessionRow_css_memoryBadgeWarning : "";
    const handleContextMenu = (e)=>{
        var _overflowRef_current;
        e.preventDefault();
        (_overflowRef_current = overflowRef.current) === null || _overflowRef_current === void 0 ? void 0 : _overflowRef_current.openAt(e.clientX, e.clientY);
    };
    const handleKeyDown = (e)=>{
        if ((e.key === "Enter" || e.key === " ") && !(e.target instanceof HTMLButtonElement) && !(e.target instanceof HTMLInputElement) && !(e.target instanceof HTMLTextAreaElement) && !(e.target instanceof HTMLAnchorElement) && !(e.target instanceof HTMLSelectElement)) {
            e.preventDefault();
            onClick === null || onClick === void 0 ? void 0 : onClick();
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: [
            row,
            memMB > 500 ? rowMemoryPressure : "",
            isPaused ? rowPaused : "",
            session.status === types_pb/* SessionStatus */.Ph.ACTIVE && (session.subStatus === types_pb/* SubStatus */.BJ.PROCESSING || session.subStatus === types_pb/* SubStatus */.BJ.WAITING_FOR_AGENT) ? rowActive : "",
            isSelected ? rowSelected : ""
        ].filter(Boolean).join(" "),
        style: {
            gridTemplateColumns: buildRowGridTemplate(visibleColumns !== null && visibleColumns !== void 0 ? visibleColumns : DEFAULT_VISIBLE_COLUMNS, {
                reserveCheckbox: true
            })
        },
        "data-testid": "session-row",
        "data-paused": isPaused ? "true" : undefined,
        "data-actions-visible": actionsAlwaysVisible ? "true" : undefined,
        onClick: onClick,
        onContextMenu: handleContextMenu,
        onKeyDown: handleKeyDown,
        tabIndex: 0,
        "aria-label": buildSessionRowAriaLabel(session, dotStatus, memMB),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: checkboxCell,
                "aria-hidden": !selectMode ? "true" : undefined,
                onClick: (e)=>{
                    e.stopPropagation();
                    onToggleSelect === null || onToggleSelect === void 0 ? void 0 : onToggleSelect(e);
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    role: "checkbox",
                    "aria-checked": isSelected,
                    "aria-label": "Select session ".concat(displayName || session.id),
                    tabIndex: selectMode ? 0 : -1,
                    "data-testid": "session-row-checkbox",
                    onClick: (e)=>{
                        e.stopPropagation();
                        onToggleSelect === null || onToggleSelect === void 0 ? void 0 : onToggleSelect(e);
                    },
                    className: checkboxButton
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                label: "Status: ".concat(getStatusDotLabel(dotStatus)),
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: statusDot,
                    "data-status": dotStatus,
                    "data-testid": "session-status-".concat(dotStatus),
                    "aria-hidden": "true"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: nameCell,
                "data-testid": "session-row-name-cell",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: SessionRow_css_name,
                        title: displayName,
                        children: displayName
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: pathLine,
                        children: session.existingDir && /*#__PURE__*/ (0,jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                            label: session.existingDir,
                            side: "bottom",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: path,
                                role: "img",
                                "aria-label": "Path: ".concat(session.existingDir),
                                "data-testid": "session-row-path",
                                children: truncateWorkspacePath(session.existingDir, ROW_PATH_MAX_LEN)
                            })
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: chipsLine,
                        "data-testid": "session-row-chips-line",
                        children: [
                            session.status === types_pb/* SessionStatus */.Ph.ACTIVE && session.subStatus !== types_pb/* SubStatus */.BJ.UNSPECIFIED && session.subStatus !== types_pb/* SubStatus */.BJ.READY && !(suppressApprovalSubStatus && (session.subStatus === types_pb/* SubStatus */.BJ.NEEDS_APPROVAL || session.subStatus === types_pb/* SubStatus */.BJ.INPUT_REQUIRED)) && /*#__PURE__*/ (0,jsx_runtime.jsx)(SubStatusChip, {
                                subStatus: session.subStatus,
                                subagentCount: session.subagentCount
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(RevivedContextBadge, {
                                session: session
                            }),
                            (0,session_staleness/* isSessionStale */.G)(session, staleThresholdMinutes) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                role: "img",
                                "aria-label": "Stale — no output for over ".concat(staleThresholdMinutes, " minutes"),
                                className: staleBadge,
                                children: "\uD83D\uDFE0 Stale"
                            }),
                            session.remoteName && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: hostBadge,
                                role: "img",
                                title: "Running on ".concat(session.remoteName),
                                "aria-label": "Running on ".concat(session.remoteName),
                                "data-testid": "host-badge",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "\uD83D\uDDA5️"
                                    }),
                                    " ",
                                    session.remoteName
                                ]
                            }),
                            session.remoteName && /*#__PURE__*/ (0,jsx_runtime.jsx)(RemoteConnectionIndicator, {
                                remoteName: session.remoteName
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(GitHubBadge/* GitHubBadge */.J, {
                                prNumber: session.githubPrNumber,
                                prUrl: session.githubPrUrl,
                                owner: session.githubOwner,
                                repo: session.githubRepo,
                                sourceRef: session.githubSourceRef,
                                host: session.githubHost,
                                prPriority: session.githubPrPriority,
                                prState: session.githubPrState,
                                isDraft: session.githubPrIsDraft,
                                approvedCount: session.githubApprovedCount,
                                changesRequestedCount: session.githubChangesReqCount,
                                checkConclusion: session.githubCheckConclusion,
                                compact: true
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogOriginBadge/* BacklogOriginBadge */.J, {
                                entry: backlogEntry,
                                compact: true
                            }),
                            noteTooltip && /*#__PURE__*/ (0,jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                                label: noteTooltip,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: noteIndicator,
                                    role: "img",
                                    "aria-label": "Has a note",
                                    "data-testid": "badge-has-note",
                                    children: "\uD83D\uDCDD"
                                })
                            })
                        ]
                    }),
                    isFailed && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: failureMessageLine,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: failureMessageIcon,
                                "aria-hidden": "true",
                                children: "⚠"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "data-testid": "failure-message",
                                children: failureMessage
                            })
                        ]
                    }),
                    visibleColumns.includes("elapsed") && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: elapsedSecondLine,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("time", {
                            className: elapsed,
                            dateTime: lastActivity ? new Date(Number(lastActivity.seconds) * 1000).toISOString() : undefined,
                            title: lastActivity ? new Date(Number(lastActivity.seconds) * 1000).toLocaleString() : undefined,
                            "aria-label": elapsedText ? "Last active: ".concat(elapsedText) : "No recent activity",
                            children: elapsedText ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: elapsedIcon,
                                        "aria-hidden": "true",
                                        children: "⏱"
                                    }),
                                    elapsedText
                                ]
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                style: {
                                    opacity: 0.3
                                },
                                children: "—"
                            })
                        })
                    })
                ]
            }),
            visibleColumns.includes("agent") && session.program && /*#__PURE__*/ (0,jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                label: session.program,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: agentIcon,
                    role: "img",
                    tabIndex: 0,
                    "aria-label": "Agent: ".concat(session.program),
                    children: getAgentEmoji(session.program)
                })
            }),
            visibleColumns.includes("diff") && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: diffBadge,
                role: session.diffStats && (session.diffStats.added > 0 || session.diffStats.removed > 0) ? "img" : undefined,
                "aria-label": session.diffStats && (session.diffStats.added > 0 || session.diffStats.removed > 0) ? "Diff: +".concat(session.diffStats.added, " -").concat(session.diffStats.removed) : undefined,
                "aria-hidden": session.diffStats && (session.diffStats.added > 0 || session.diffStats.removed > 0) ? undefined : "true",
                children: session.diffStats && (session.diffStats.added > 0 || session.diffStats.removed > 0) ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            style: {
                                color: "var(--success)"
                            },
                            children: [
                                "+",
                                session.diffStats.added
                            ]
                        }),
                        " ",
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            style: {
                                color: "var(--error)"
                            },
                            children: [
                                "-",
                                session.diffStats.removed
                            ]
                        })
                    ]
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    style: {
                        opacity: 0.3
                    },
                    children: "—"
                })
            }),
            visibleColumns.includes("memory") && memMB > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)(Tooltip/* Tooltip */.m, {
                label: "Process RSS: ".concat(memMB, " MB"),
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: [
                        SessionRow_css_memoryBadge,
                        memorySeverityClass
                    ].filter(Boolean).join(" "),
                    role: "img",
                    tabIndex: 0,
                    "aria-label": "".concat(memMB, " MB RAM"),
                    children: memMB >= 1024 ? "".concat((memMB / 1024).toFixed(1), " GB") : "".concat(memMB, " MB")
                })
            }),
            visibleColumns.includes("branch") && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: branchCell,
                role: "img",
                title: session.branch || undefined,
                "aria-label": session.branch ? "Branch: ".concat(session.branch) : "No branch",
                children: session.branch || /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    style: {
                        opacity: 0.3
                    },
                    children: "—"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                className: actions,
                role: "group",
                "aria-label": "Session actions",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: primaryActionWrapper,
                        role: "presentation",
                        children: [
                            (isPaused || isNeedsApproval) && onResume && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: inlineActionButton,
                                onClick: (e)=>{
                                    e.stopPropagation();
                                    onResume();
                                },
                                "aria-label": "Resume session ".concat(session.title),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "▶️"
                                    }),
                                    " Resume"
                                ]
                            }),
                            isHibernated && onResumeFromHibernation && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: inlineActionButton,
                                onClick: (e)=>{
                                    e.stopPropagation();
                                    onResumeFromHibernation();
                                },
                                "aria-label": "Wake session ".concat(session.title, " from hibernation"),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "▶️"
                                    }),
                                    " Resume"
                                ]
                            }),
                            isRunning && !isCreating && onPause && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: inlineActionButton,
                                onClick: (e)=>{
                                    e.stopPropagation();
                                    onPause();
                                },
                                "aria-label": "Pause session ".concat(session.title),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "⏸️"
                                    }),
                                    " Pause"
                                ]
                            }),
                            isCreating && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: inlineActionButton,
                                onClick: handleCancelCreation,
                                disabled: cancelDisabled,
                                "aria-label": "Cancel session creation",
                                "data-testid": "cancel-creation-button",
                                children: "Cancel"
                            }),
                            isFailed && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: inlineActionButton,
                                onClick: handleRetryCreation,
                                disabled: retryDisabled,
                                "aria-label": "Retry creating session",
                                "data-testid": "retry-creation-button",
                                children: "Retry"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionActionsOverflow, {
                        ref: overflowRef,
                        session: session,
                        showPrimaryAction: false,
                        buttonClassName: rowOverflowButton,
                        onPause: onPause,
                        onResume: onResume,
                        onHibernate: onHibernate,
                        onResumeFromHibernation: onResumeFromHibernation,
                        onDelete: onDelete,
                        onClone: onClone,
                        onOpenInNewPane: onOpenInNewPane,
                        onNewWorkspace: onNewWorkspace,
                        onRestart: onRestart,
                        onRetryNow: onRetryNow,
                        onCreateCheckpoint: onCreateCheckpoint,
                        onSetRateLimitEnabled: onSetRateLimitEnabled,
                        onToggleAutonomousMode: onToggleAutonomousMode,
                        onTogglePinned: onTogglePinned,
                        onToggleAutoApprove: onToggleAutoApprove,
                        onSteerAutonomousSession: onSteerAutonomousSession,
                        onClearConversationState: onClearConversationState,
                        onUpdateTags: onUpdateTags,
                        onChangeProgram: async (_id, program)=>{
                            const result = await sessionActions.update({
                                program
                            });
                            if (!result) throw new Error("Failed to change program.");
                        }
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                role: "status",
                "aria-live": isFailed ? "assertive" : "polite",
                id: "creation-status-".concat(session.id),
                "data-testid": "creation-live-region",
                style: {
                    position: "absolute",
                    width: 1,
                    height: 1,
                    overflow: "hidden",
                    clipPath: "inset(50%)",
                    whiteSpace: "nowrap"
                },
                children: isFailed ? failureMessage : isCreating ? session.creationProgress || "Starting session..." : ""
            })
        ]
    });
}
const SessionRow = /*#__PURE__*/ (0,react.memo)(SessionRowInner);

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionListEmptyState.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xa2dsMmdyMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAganVzdGlmeS1jb250ZW50OiBjZW50ZXI7CiAgZ2FwOiB2YXIoLS1fMTllejJyejMwKTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejM0KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKfQouXzFrZ2wyZ3IxIHsKICBmb250LXNpemU6IDMycHg7CiAgb3BhY2l0eTogMC40Owp9Ci5fMWtnbDJncjIgewogIG1hcmdpbjogMDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2MpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNoKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKfQouXzFrZ2wyZ3IzIHsKICBtYXJnaW46IDA7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICB0ZXh0LWFsaWduOiBjZW50ZXI7Cn0KLl8xa2dsMmdyNCB7CiAgbWFyZ2luOiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgb3BhY2l0eTogMC43Owp9Ci5fMWtnbDJncjUgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBwYWRkaW5nOiAxcHggNnB4OwogIGZvbnQtZmFtaWx5OiB2YXIoLS1fMTllejJyejJ1KTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionListEmptyState_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xa2dsMmdyMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAganVzdGlmeS1jb250ZW50OiBjZW50ZXI7CiAgZ2FwOiB2YXIoLS1fMTllejJyejMwKTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejM0KTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKfQouXzFrZ2wyZ3IxIHsKICBmb250LXNpemU6IDMycHg7CiAgb3BhY2l0eTogMC40Owp9Ci5fMWtnbDJncjIgewogIG1hcmdpbjogMDsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2MpOwogIGZvbnQtd2VpZ2h0OiB2YXIoLS1fMTllejJyejNoKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKfQouXzFrZ2wyZ3IzIHsKICBtYXJnaW46IDA7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICB0ZXh0LWFsaWduOiBjZW50ZXI7Cn0KLl8xa2dsMmdyNCB7CiAgbWFyZ2luOiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7CiAgb3BhY2l0eTogMC43Owp9Ci5fMWtnbDJncjUgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBwYWRkaW5nOiAxcHggNnB4OwogIGZvbnQtZmFtaWx5OiB2YXIoLS1fMTllejJyejJ1KTsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwp9_22_7D = __webpack_require__(24660);
;// ./src/components/sessions/SessionListEmptyState.css.ts


var SessionListEmptyState_css_body = '_1kgl2gr3';
var container = '_1kgl2gr0';
var headline = '_1kgl2gr2';
var hint = '_1kgl2gr4';
var SessionListEmptyState_css_icon = '_1kgl2gr1';
var kbd = '_1kgl2gr5';
;// ./src/components/sessions/SessionListEmptyState.tsx
/* __next_internal_client_entry_do_not_use__ SessionListEmptyState auto */ 


function SessionListEmptyState() {
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        "data-testid": "session-list-empty-state",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: SessionListEmptyState_css_icon,
                "aria-hidden": "true",
                children: "⌘"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: headline,
                children: "No sessions yet"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: SessionListEmptyState_css_body,
                children: [
                    "Press ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                        className: kbd,
                        children: "⌘K"
                    }),
                    " or ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                        className: kbd,
                        children: "Ctrl+K"
                    }),
                    " to create a session or paste a GitHub URL"
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/sessions/BulkActions.css.ts
var BulkActions_css = __webpack_require__(51213);
;// ./src/components/sessions/BulkActions.tsx
/* __next_internal_client_entry_do_not_use__ BulkActions auto */ 
// +feature: project-grouping session-bulk-select


function BulkActions(param) {
    let { selectedCount, onPauseAll, onResumeAll, onDeleteAll, onAddTagAll, onSelectAll, onClearSelection, totalCount, feedback, onGroupAs } = param;
    const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad|iPod/.test(navigator.platform);
    const [groupAsValue, setGroupAsValue] = (0,react.useState)("");
    const [groupAsLoading, setGroupAsLoading] = (0,react.useState)(false);
    const [groupAsError, setGroupAsError] = (0,react.useState)(null);
    if (selectedCount === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            role: "toolbar",
            "aria-label": "Bulk session actions",
            className: BulkActions_css/* container */.kL,
            children: [
                feedback && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: BulkActions_css/* feedback */.mp,
                    "aria-hidden": "true",
                    children: feedback
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: BulkActions_css/* count */.U9,
                    style: {
                        color: "var(--text-muted)",
                        fontStyle: "italic"
                    },
                    children: "Click sessions to select them"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    onClick: onClearSelection,
                    className: BulkActions_css/* clearButton */.mt,
                    "aria-label": "Cancel select mode",
                    children: "Cancel (Esc)"
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        role: "toolbar",
        "aria-label": "Bulk session actions",
        className: BulkActions_css/* container */.kL,
        children: [
            feedback && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: BulkActions_css/* feedback */.mp,
                "aria-hidden": "true",
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
                    selectedCount < totalCount && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        onClick: onSelectAll,
                        className: BulkActions_css/* selectAllButton */.EN,
                        "aria-label": "Select all ".concat(totalCount, " session").concat(totalCount !== 1 ? "s" : ""),
                        children: [
                            "Select All ",
                            isMac ? "(⌘A)" : "(Ctrl+A)"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        onClick: onClearSelection,
                        className: BulkActions_css/* clearButton */.mt,
                        "aria-label": "Clear selection of ".concat(selectedCount, " session").concat(selectedCount !== 1 ? "s" : ""),
                        children: "Clear Selection (Esc)"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: BulkActions_css/* actions */.o1,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        onClick: onPauseAll,
                        className: BulkActions_css/* actionButton */.hP,
                        "data-testid": "bulk-pause-button",
                        "aria-label": "Pause ".concat(selectedCount, " selected session").concat(selectedCount !== 1 ? "s" : ""),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "aria-hidden": "true",
                                children: "⏸️"
                            }),
                            " Pause Selected"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        onClick: onResumeAll,
                        className: BulkActions_css/* actionButton */.hP,
                        "aria-label": "Resume ".concat(selectedCount, " selected session").concat(selectedCount !== 1 ? "s" : ""),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "aria-hidden": "true",
                                children: "▶️"
                            }),
                            " Resume Selected"
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        onClick: onAddTagAll,
                        className: BulkActions_css/* actionButton */.hP,
                        "aria-label": "Add tag to ".concat(selectedCount, " selected session").concat(selectedCount !== 1 ? "s" : ""),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "aria-hidden": "true",
                                children: "\uD83C\uDFF7️"
                            }),
                            " Add Tag"
                        ]
                    }),
                    onGroupAs && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        role: "group",
                        "aria-label": "Group selected sessions as project",
                        style: {
                            display: "flex",
                            gap: "4px",
                            alignItems: "center"
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                type: "text",
                                value: groupAsValue,
                                onChange: (e)=>setGroupAsValue(e.target.value),
                                onKeyDown: async (e)=>{
                                    if (e.key === "Enter" && groupAsValue.trim() && !groupAsLoading) {
                                        e.preventDefault();
                                        const name = groupAsValue.trim();
                                        setGroupAsLoading(true);
                                        setGroupAsError(null);
                                        try {
                                            await onGroupAs(name);
                                            setGroupAsValue("");
                                        } catch (e) {
                                            setGroupAsError("Failed to group — try again");
                                        } finally{
                                            setGroupAsLoading(false);
                                        }
                                    }
                                },
                                placeholder: "Group as…",
                                disabled: groupAsLoading,
                                "aria-label": "Project name",
                                style: {
                                    padding: "4px 8px",
                                    border: "1px solid var(--border-color)",
                                    borderRadius: "4px",
                                    fontSize: "0.875rem",
                                    background: "var(--input-background)",
                                    color: "var(--text-primary)",
                                    width: "140px"
                                }
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: BulkActions_css/* actionButton */.hP,
                                disabled: groupAsLoading || !groupAsValue.trim(),
                                "aria-busy": groupAsLoading,
                                "aria-label": groupAsLoading ? "Grouping sessions…" : "Group selected sessions into project",
                                onClick: async ()=>{
                                    const name = groupAsValue.trim();
                                    if (!name || groupAsLoading) return;
                                    setGroupAsLoading(true);
                                    setGroupAsError(null);
                                    try {
                                        await onGroupAs(name);
                                        setGroupAsValue("");
                                    } catch (e) {
                                        setGroupAsError("Failed to group — try again");
                                    } finally{
                                        setGroupAsLoading(false);
                                    }
                                },
                                children: groupAsLoading ? "…" : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            "aria-hidden": "true",
                                            children: "\uD83D\uDCC1"
                                        }),
                                        " Group"
                                    ]
                                })
                            }),
                            groupAsError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                role: "alert",
                                style: {
                                    color: "var(--error)",
                                    fontSize: "0.75rem",
                                    whiteSpace: "nowrap"
                                },
                                children: groupAsError
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        onClick: onDeleteAll,
                        className: "".concat(BulkActions_css/* actionButton */.hP, " ").concat(BulkActions_css/* danger */.OR),
                        "data-testid": "bulk-delete-button",
                        "aria-label": "Delete ".concat(selectedCount, " selected session").concat(selectedCount !== 1 ? "s" : ""),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                "aria-hidden": "true",
                                children: "\uD83D\uDDD1️"
                            }),
                            " Delete Selected"
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/grouping/strategies.ts
var strategies = __webpack_require__(50243);
// EXTERNAL MODULE: ./src/lib/hooks/usePersistedViewState.ts
var usePersistedViewState = __webpack_require__(88253);
// EXTERNAL MODULE: ./src/lib/hooks/useAbortableRequest.ts
var useAbortableRequest = __webpack_require__(2663);
;// ./src/lib/hooks/useStaleSessionConfig.ts
/* __next_internal_client_entry_do_not_use__ useStaleSessionConfig auto */ 




// Safe default returned before the one-time fetch resolves (and if it fails):
// a 30-minute threshold with notifications on, matching the server-side default
// in config/config.go.
const DEFAULT_CONFIG = {
    thresholdMinutes: 30,
    notifyEnabled: true
};
/**
 * Fetches the resolved stale-session threshold/notify config once on mount.
 * Returns the safe default until the fetch resolves.
 */ // NOTE: consumed per-row by SessionCard/SessionRow, so a long session list
// (46+ rows observed in practice) fires this many identical
// getSessionDefaults calls on first render — a real dedup/caching gap
// (unlike useVcsStatus.ts's module-level cache), but a separate concern
// from the cancellation fixed here; not addressed by this pass.
function useStaleSessionConfig() {
    const [config, setConfig] = (0,react.useState)(DEFAULT_CONFIG);
    const startFetch = (0,useAbortableRequest/* useAbortableRequest */.X)();
    (0,react.useEffect)(()=>{
        const signal = startFetch();
        const fetchConfig = async ()=>{
            try {
                const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
                const response = await client.getSessionDefaults({}, {
                    signal
                });
                if (signal.aborted) return;
                const defaults = response.defaults;
                if (defaults) {
                    setConfig({
                        thresholdMinutes: defaults.staleSessionThresholdMinutes,
                        notifyEnabled: defaults.staleSessionNotifyEnabled
                    });
                }
            } catch (err) {
                if (signal.aborted) return;
                // Non-critical: fall back to the safe default already in state.
                console.error("Failed to fetch stale session config:", err);
            }
        };
        fetchConfig();
    }, [
        startFetch
    ]);
    return config;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FColumnPicker.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61U0XKbMBB891fcYzxTeQSx3VT5mIxAZ7gGJI0kDKbTf-8gh9jYOEnrDk_o9u50e7tavTzV5JKfHH4tAKzxFMhoAQ4rGWiPzwsARd5W8iCAdEUa2a7C7nnxe7F6y01i7jwKQFZUaEYBay8gRx3QDceFtAL20j0w9pL8wD51fXpYDhErlSJdzEQvT_qI3xkdmKceLzMe5SneIhVluEIUEZGbyrjLWBJDmcxfC2carQRooyMfmXEKnYDEduBNReoiE5cnFHNSUeOvGm-PjRvnh87W0MhLW1JA5q3McejYOmknXIvS7NFFxmevza-uPY0_Lc_LpfcVep9ytkI1afV4obAddaiGEj0jrbATsOH845bfl_-L_rFSx3wplWkFcFjbDpLUduCKTD7wb_FbJZsPRXmUYE2ataRCKSDZcjtxxzrOfY9Iy9siTWMoYBdYcFL7nXG1gMZadLn0Ua0VhoAuKioOwFd8i_VkJv651Q7npGYmBFN_toFauoL0O3im4ImkzfQJ-Ye3o__Lt-Nwj0lvbzO7vaujoRo_LAMrzMP4opzRcObur1p4G9Gj-tY2Ele-KWn8HwhlvnSkXwXwG1PJfKB43sv10PMPDonyuC0GAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FColumnPicker_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61U0XKbMBB891fcYzxTeQSx3VT5mIxAZ7gGJI0kDKbTf_8gh9jYOEnrDk_o9u50e7tavTzV5JKfHH4tAKzxFMhoAQ4rGWiPzwsARd5W8iCAdEUa2a7C7nnxe7F6y01i7jwKQFZUaEYBay8gRx3QDceFtAL20j0w9pL8wD51fXpYDhErlSJdzEQvT_qI3xkdmKceLzMe5SneIhVluEIUEZGbyrjLWBJDmcxfC2carQRooyMfmXEKnYDEduBNReoiE5cnFHNSUeOvGm_PjRvnh87W0MhLW1JA5q3McejYOmknXIvS7NFFxmevza_uPY0_Lc_LpfcVep9ytkI1afV4obAddaiGEj0jrbATsOH845bfl_L_rFSx3wplWkFcFjbDpLUduCKTD7wb_FbJZsPRXmUYE2ataRCKSDZcjtxxzrOfY9Iy9siTWMoYBdYcFL7nXG1gMZadLn0Ua0VhoAuKioOwFd8i_VkJv651Q7npGYmBFN_toFauoL0O3im4ImkzfQJ_Ye3o_Lt_Nwj0lvbzO7vaujoRo_LAMrzMP4opzRcObur1p4G9Gj_tY2Ele_KWn8HwhlvnSkXwXwG1PJfKB43sv10PMPDonyuC0GAAA_22_7D = __webpack_require__(46007);
;// ./src/components/sessions/ColumnPicker.css.ts

var ColumnPicker_css_checkbox = '_8mir1j6';
var checkboxRow = '_8mir1j5';
var dropdown = '_8mir1j3';
var dropdownTitle = '_8mir1j4';
var triggerButton = '_8mir1j1';
var triggerButtonActive = '_8mir1j2';
var ColumnPicker_css_wrapper = '_8mir1j0';
;// ./src/components/sessions/ColumnPicker.tsx
/* __next_internal_client_entry_do_not_use__ ColumnPicker auto */ 




function ColumnPicker(param) {
    let { visibleColumns, onChange, open, onOpenChange } = param;
    const wrapperRef = (0,react.useRef)(null);
    const dropdownRef = (0,react.useRef)(null);
    const [dropdownPos, setDropdownPos] = (0,react.useState)({
        top: 0,
        right: 0
    });
    // Close on click outside — checks both the trigger wrapper and the portaled dropdown,
    // since the dropdown no longer lives inside wrapperRef's DOM subtree.
    (0,react.useEffect)(()=>{
        if (!open) return;
        const handler = (e)=>{
            var _wrapperRef_current, _dropdownRef_current;
            const target = e.target;
            if (((_wrapperRef_current = wrapperRef.current) === null || _wrapperRef_current === void 0 ? void 0 : _wrapperRef_current.contains(target)) || ((_dropdownRef_current = dropdownRef.current) === null || _dropdownRef_current === void 0 ? void 0 : _dropdownRef_current.contains(target))) {
                return;
            }
            onOpenChange(false);
        };
        document.addEventListener("mousedown", handler);
        return ()=>document.removeEventListener("mousedown", handler);
    }, [
        open,
        onOpenChange
    ]);
    const toggle = (0,react.useCallback)((key)=>{
        const next = visibleColumns.includes(key) ? visibleColumns.filter((k)=>k !== key) : [
            ...visibleColumns,
            key
        ];
        onChange(next);
    }, [
        visibleColumns,
        onChange
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: wrapperRef,
        className: ColumnPicker_css_wrapper,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: "".concat(triggerButton, " ").concat(open ? triggerButtonActive : ""),
                onClick: ()=>{
                    // Compute the dropdown's position synchronously, before it opens, so the
                    // portaled dropdown never paints at the stale {top: 0, right: 0} default
                    // for a frame (matches MoveToMenu.tsx's openMenu pattern).
                    if (!open) {
                        var _wrapperRef_current;
                        const rect = (_wrapperRef_current = wrapperRef.current) === null || _wrapperRef_current === void 0 ? void 0 : _wrapperRef_current.getBoundingClientRect();
                        if (rect) {
                            setDropdownPos({
                                top: rect.bottom + 4,
                                right: window.innerWidth - rect.right
                            });
                        }
                    }
                    onOpenChange(!open);
                },
                "aria-haspopup": "listbox",
                "aria-expanded": open,
                title: "Customize visible columns",
                children: "⊞ Columns"
            }),
            open && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                ref: dropdownRef,
                className: dropdown,
                role: "listbox",
                "aria-label": "Visible columns",
                style: {
                    top: dropdownPos.top,
                    right: dropdownPos.right
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: dropdownTitle,
                        children: "Show columns"
                    }),
                    COLUMN_DEFS.map((col)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                            className: checkboxRow,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    type: "checkbox",
                                    className: ColumnPicker_css_checkbox,
                                    checked: visibleColumns.includes(col.key),
                                    onChange: ()=>toggle(col.key)
                                }),
                                col.label
                            ]
                        }, col.key))
                ]
            }), document.body)
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/contexts/ReviewQueueContext.tsx + 2 modules
var ReviewQueueContext = __webpack_require__(34313);
// EXTERNAL MODULE: ./src/lib/contexts/ApprovalsContext.tsx + 2 modules
var ApprovalsContext = __webpack_require__(38808);
// EXTERNAL MODULE: ./src/lib/contexts/NotificationContext.tsx + 12 modules
var NotificationContext = __webpack_require__(62275);
// EXTERNAL MODULE: ./src/lib/contexts/SystemMemoryContext.tsx
var SystemMemoryContext = __webpack_require__(53064);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FMemoryPressureCallout.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71TzW7jIBC-5yk4bg9Y2N00DfswFTETe7YYLMCxnarvvoKuUzumUbRS92QxM-b7mY_sJe9GpYYTI28bQiS6VomRk6OC4deGxC-VaKH0aDQnpVFdo0OnEi0nLNttLTTh3AopUVec5H8LB1G-VtZ0WnJyEvYHpS_5Hs6FPY8PsW-sBMtJ3g7EGYXyamqYTVErJHYuAE54jbAVanow3ptmQn3fZJOgPC3od-c8HkdaGu1Be05cK0qgB_A9QBQmFFaaoofGcVKC9mBnerfXOEXEKY0y9lrnOSo4Gu1pD1jVnpMnxi41h2cIVz7vVpc-xkvnDmqjYe7adL4BXHbWhV5rcBKxwF2vjmV5EYqEZcXUNK0o0Y-R6JIjr80JbGR6GcoXIz__MVQrP7b_aZlPdy3zsrj806UT2KMyPSc1SvmB7WHw9LMBSmHr0IVWX6MHGtmGVfZWtJMx1x7uvqY0rClddppMXfDd1Rb1KydsgfK8ClwKKsmCPaRyuXq3xa20LVjfpSUV7hvyUlll2X4xtL8zZBEHtFz8LL7dwMfUe_0orsxKJPQuE-eCvrbsD-jg4T81BgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FMemoryPressureCallout_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71TzW7jIBC_5yk4bg9Y2N00DfswFTETe7YYLMCxnarvvoKuUzumUbRS92QxM_b7mY_sJe9GpYYTI28bQiS6VomRk6OC4deGxC_VaKH0aDQnpVFdo0OnEi0nLNttLTTh3AopUVec5H8LB1G_VtZ0WnJyEvYHpS_5Hs6FPY8PsW_sBMtJ3g7EGYXyamqYTVErJHYuAE54jbAVanow3ptmQn3fZJOgPC3od_c8HkdaGu1Be05cK0qgB_A9QBQmFFaaoofGcVKC9mBnerfXOEXEKY0y9lrnOSo4Gu1pD1jVnpMnxi41h2cIVz7vVpc_xkvnDmqjYe7adL4BXHbWhV5rcBKxwF2vjmV5EYqEZcXUNK0o0Y_R6JIjr80JbGR6GcoXIz_MVQrP7b_aZlPdy3zsrj806UT2KMyPSc1SvmB7WHw9LMBSmHr0IVWX6MHGtmGVfZWtJMx1x7uvqY0rClddppMXfDd1Rb1KydsgfK8ClwKKsmCPaRyuXq3xa20LVjfpSUV7hvyUlll2X4xtL8zZBEHtFz8LL7dwMfUe_0orsxKJPQuE_eCvrbsD_jg4T81BgAA_22_7D = __webpack_require__(43706);
;// ./src/components/sessions/MemoryPressureCallout.css.ts

var bulkAction = '_1uyllxv9';
var callout = '_1uyllxv0';
var dismissAll = '_1uyllxv3';
var MemoryPressureCallout_css_header = '_1uyllxv1';
var hibernateAllBtn = '_1uyllxva';
var hibernateBtn = '_1uyllxv8';
var savings = '_1uyllxv7';
var sessionList = '_1uyllxv4';
var sessionName = '_1uyllxv6';
var sessionRow = '_1uyllxv5';
var MemoryPressureCallout_css_title = '_1uyllxv2';
;// ./src/components/sessions/MemoryPressureCallout.tsx
/* __next_internal_client_entry_do_not_use__ MemoryPressureCallout auto */ 
// +feature: ui:memory-pressure-callout




function getMB(session) {
    var _session_estimatedSavingsMb;
    return Number((_session_estimatedSavingsMb = session.estimatedSavingsMb) !== null && _session_estimatedSavingsMb !== void 0 ? _session_estimatedSavingsMb : 0n);
}
function isActive(session) {
    return session.status === types_pb/* SessionStatus */.Ph.ACTIVE;
}
/**
 * Shows the top-3 active sessions by estimated memory savings when the system
 * is under memory pressure. Includes a bulk "Hibernate all recommended" action.
 * Dismissible per-session via sessionStorage.
 */ function MemoryPressureCallout(param) {
    let { sessions, onHibernate } = param;
    const { isUnderPressure } = (0,SystemMemoryContext/* useSystemMemory */.Ae)();
    const [dismissed, setDismissed] = (0,react.useState)(()=>{
        try {
            const raw = sessionStorage.getItem("memory-pressure-dismissed");
            return raw ? new Set(JSON.parse(raw)) : new Set();
        } catch (e) {
            return new Set();
        }
    });
    const dismiss = (0,react.useCallback)((id)=>{
        setDismissed((prev)=>{
            const next = new Set(prev);
            next.add(id);
            try {
                sessionStorage.setItem("memory-pressure-dismissed", JSON.stringify([
                    ...next
                ]));
            } catch (e) {}
            return next;
        });
    }, []);
    if (!isUnderPressure) return null;
    const candidates = sessions.filter((s)=>isActive(s) && !dismissed.has(s.id) && getMB(s) > 0).sort((a, b)=>getMB(b) - getMB(a)).slice(0, 3);
    if (candidates.length === 0) return null;
    const handleHibernateAll = ()=>{
        for (const s of candidates){
            onHibernate(s.id);
            dismiss(s.id);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: callout,
        role: "alert",
        "aria-live": "polite",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: MemoryPressureCallout_css_header,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: MemoryPressureCallout_css_title,
                    children: "⚠ Memory pressure — idle sessions are consuming RAM"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: sessionList,
                children: candidates.map((s)=>{
                    const mb = getMB(s);
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: sessionRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: sessionName,
                                title: s.title,
                                children: s.title
                            }),
                            mb > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: savings,
                                children: [
                                    "saves ~",
                                    mb,
                                    " MB"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: hibernateBtn,
                                onClick: ()=>{
                                    onHibernate(s.id);
                                    dismiss(s.id);
                                },
                                title: "Hibernate ".concat(s.title),
                                children: "Hibernate"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: dismissAll,
                                onClick: ()=>dismiss(s.id),
                                "aria-label": "Dismiss recommendation for ".concat(s.title),
                                title: "Dismiss",
                                children: "✕"
                            })
                        ]
                    }, s.id);
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: bulkAction,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    className: hibernateAllBtn,
                    onClick: handleHibernateAll,
                    children: "Hibernate all recommended"
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/store/sessionsSlice.ts
var sessionsSlice = __webpack_require__(60795);
// EXTERNAL MODULE: ./src/components/ui/ActionBar.tsx + 1 modules
var ActionBar = __webpack_require__(93321);
;// ./src/lib/utils/rangeSelect.ts
function computeRangeIds(anchorId, targetId, flatItems) {
    const anchorIdx = flatItems.findIndex((i)=>{
        var _i_session;
        return i.kind === "session" && ((_i_session = i.session) === null || _i_session === void 0 ? void 0 : _i_session.id) === anchorId;
    });
    const targetIdx = flatItems.findIndex((i)=>{
        var _i_session;
        return i.kind === "session" && ((_i_session = i.session) === null || _i_session === void 0 ? void 0 : _i_session.id) === targetId;
    });
    if (anchorIdx === -1 || targetIdx === -1) return [
        targetId
    ];
    const lo = Math.min(anchorIdx, targetIdx);
    const hi = Math.max(anchorIdx, targetIdx);
    return flatItems.slice(lo, hi + 1).filter((i)=>i.kind === "session").map((i)=>i.session.id);
}

// EXTERNAL MODULE: ./src/lib/hooks/useInsightsService.ts
var useInsightsService = __webpack_require__(81240);
;// ./src/components/sessions/sessionCostSort.ts
/**
 * Compares two sessions by cost, always sorting sessions with no resolved
 * cost (not yet loaded, or genuinely unpriced) last — in BOTH sort
 * directions. The early return happens before the sortDir flip: a sentinel
 * value fed into a generic comparator would invert position when sortDir
 * flips, which is the specific bug this shape avoids.
 */ function compareSessionsByCost(a, b, costById, sortDir) {
    const aCost = costById.get(a.id);
    const bCost = costById.get(b.id);
    const aMissing = aCost === undefined;
    const bMissing = bCost === undefined;
    if (aMissing !== bMissing) return aMissing ? 1 : -1;
    if (aMissing && bMissing) return 0;
    const cmp = aCost - bCost;
    return sortDir === "asc" ? cmp : -cmp;
}

;// ./src/lib/hooks/useFilteredGroupedSessions.ts




const getTimestampMs = (ts)=>{
    if (!ts || ts.seconds === BigInt(0)) return 0;
    return Number(ts.seconds) * 1000;
};
/**
 * Shared filter -> sort -> group pipeline for session collections. Extracted from
 * SessionList so SessionBoard can reuse identical filtering/search/grouping behavior.
 */ function useFilteredGroupedSessions(param) {
    let { sessions, searchQuery, selectedStatus, selectedCategory, selectedTag, hidePaused, showArchived, showHidden, filterNeedsApproval, pendingDeleteIds, sortField, sortDir, costById, groupingStrategy, staleThresholdMinutes, staleRecomputeTick } = param;
    const filteredSessions = (0,react.useMemo)(()=>{
        return sessions.filter((session)=>{
            // Exclude sessions that are pending deletion (optimistic removal)
            if (pendingDeleteIds.has(session.id)) return false;
            // Search filter
            if (searchQuery) {
                const query = searchQuery.toLowerCase();
                const matchesSearch = session.title.toLowerCase().includes(query) || session.activeDir.toLowerCase().includes(query) || session.branch.toLowerCase().includes(query) || session.category && session.category.toLowerCase().includes(query) || session.tags && session.tags.some((tag)=>tag.toLowerCase().includes(query)) || session.program && session.program.toLowerCase().includes(query);
                if (!matchesSearch) return false;
            }
            // Status filter
            if (selectedStatus !== "all" && session.status !== selectedStatus) {
                return false;
            }
            // Category filter
            if (selectedCategory !== "all" && session.category !== selectedCategory) {
                return false;
            }
            // Tag filter
            if (selectedTag !== "all") {
                if (!session.tags || !session.tags.includes(selectedTag)) {
                    return false;
                }
            }
            // Hide paused filter
            if (hidePaused && session.status === types_pb/* SessionStatus */.Ph.PAUSED) {
                return false;
            }
            // Needs-approval quick filter — show only Active sessions with subStatus === NEEDS_APPROVAL
            if (filterNeedsApproval && !(session.status === types_pb/* SessionStatus */.Ph.ACTIVE && (session.subStatus === types_pb/* SubStatus */.BJ.NEEDS_APPROVAL || session.subStatus === types_pb/* SubStatus */.BJ.INPUT_REQUIRED))) {
                return false;
            }
            // Archived filter — hidden by default even if a prior includeArchived fetch
            // left archived sessions in the Redux store (e.g. toggle turned back off
            // without a fresh non-archived fetch).
            if (!showArchived && session.archivedAt) {
                return false;
            }
            // Hidden filter — same shape as the archived filter above: excluded by
            // default even if a prior includeHidden fetch left hidden sessions (e.g.
            // Diagnose & Nudge, review dispatches) in the Redux store.
            if (!showHidden && session.hidden) {
                return false;
            }
            return true;
        });
    }, [
        sessions,
        searchQuery,
        selectedStatus,
        selectedCategory,
        selectedTag,
        hidePaused,
        filterNeedsApproval,
        showArchived,
        showHidden,
        pendingDeleteIds
    ]);
    const sortedSessions = (0,react.useMemo)(()=>{
        const sorted = [
            ...filteredSessions
        ];
        sorted.sort((a, b)=>{
            if (sortField === "tokenCost") {
                // compareSessionsByCost already applies sortDir internally (to keep
                // unloaded/unpriced rows last in BOTH directions) — return directly,
                // skipping the shared sortDir flip below.
                return compareSessionsByCost(a, b, costById, sortDir);
            }
            let cmp = 0;
            switch(sortField){
                case "name":
                    cmp = a.title.localeCompare(b.title);
                    break;
                case "createdAt":
                    cmp = getTimestampMs(a.createdAt) - getTimestampMs(b.createdAt);
                    break;
                case "updatedAt":
                    cmp = getTimestampMs(a.updatedAt) - getTimestampMs(b.updatedAt);
                    break;
                case "lastActivity":
                    {
                        const act = (s)=>Math.max(getTimestampMs(s.lastMeaningfulOutput), getTimestampMs(s.lastTerminalUpdate));
                        cmp = act(a) - act(b);
                        break;
                    }
            }
            return sortDir === "asc" ? cmp : -cmp;
        });
        return sorted;
    }, [
        filteredSessions,
        sortField,
        sortDir,
        costById
    ]);
    const filteredSessionIds = (0,react.useMemo)(()=>new Set(filteredSessions.map((s)=>s.id)), [
        filteredSessions
    ]);
    // staleRecomputeTick is a dependency purely to force recomputation on the periodic
    // tick a caller may drive — a session can cross the stale threshold with no change
    // to sortedSessions/groupingStrategy, and this is the only way to pick that up.
    const groupedSessionsResult = (0,react.useMemo)(()=>{
        return (0,strategies/* groupWithPinned */.O3)(sortedSessions, (rest)=>(0,strategies/* groupSessions */.Ln)(rest, groupingStrategy, {
                thresholdMinutes: staleThresholdMinutes
            }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sortedSessions,
        groupingStrategy,
        staleThresholdMinutes,
        staleRecomputeTick
    ]);
    return {
        filteredSessions,
        sortedSessions,
        groupedSessions: groupedSessionsResult,
        filteredSessionIds
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionList.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VY246bOhR9z1f4pWojlQiTO9GRzn8cVZVjnOAJ2BxjJpdq_r3CQLCDHUiah-nLaIK9t73XXvvmyU_4lmB29sGvEQBHGsk4BND3v2xGAGQoiijbh-AdiW-e9xOuySUQlykcl6u7hJxCAMt_U8q8mNB9LEPglx_4OxG7hB-9cwhQIflm9DGa1EdBdVSKxJ4yb8ul5GnnhGCsSwRKIqJ5lqBzqA5uLuAdBcpCUP4tP70VuaS7s4c5k4TJEOQZwsTbEnkkhJU7UEL3zKOSpHkIMGGSiE3vdSqD9-VJ5kpwMS461UyrkdhxJr2cXkgI4GQuSHr9eKwBW_pqI-YJF7f6_SvSXh4Lyg5Ka3vgzI6Mw0iLAVPfMGCu9F39Dv3sBAI_U0q3XEREeAJFtMg7epbjG2v9yWpptXdR2btF-LAXvGDRra5KVXVcCGB2AjlPaHSzi4x7UMOFyMu1jNMGACkQy6mknIUAJQnwJ0EOCMpJuXiMqSSeIkwIGK8opUETxiWrFUDuu6-0u3vWy6UG4IsefalupbrhY_qXd0InooLgCgzMkyJlw0i-sqt0CTrZaCSbVv36cUovKoJeabvITgAGOmvdNDqM-7ldO9XpJIj6uGjGBQysgTGvE0GXt1ZqmnTWKWHy2p091jql71AqMfyPevenbjhMdm5NX1OWUEa8bcLxQWPIqvJkU2Dqn30xc9-l657Eis2bMc6ILW4EPzoqkTscHLyPbLxvl4lavqm5tSp_00na1gL2GWPhWiOGkFmDI9xxXOQKFF7Ikjetl4ZzeWepd38fdD2Vrh_N3WvQ3FvQhLNPmodhDZ5KOU1ugX8ApzNNu7PM_qkEHP9p01cnok9G-y7qLrq7_fsY7eMn4acsK-R_8pyRf77imODDlp--_lBqOla0cvQFrdjNYPT2CpXmBHDojjCOMTC4jK9dliWubD3_Hc9lvPFbLik-nJU3eVZf4eJRFqnS1zM7YJ2QCdnJEMwc3B3SJizqPWX8oogfyx4lJ1Lp9IF_i8jcxeQiJ8LLSUKwbBOrQdarRcCfwHmXq4fB40crkzyUKDqzc6dVD2ZmJ9b8vhLEt6W7NovcVJT7mGs2KqAyJAiTThbBITOfFeXvoEkRNtyTB8Y-J71bdenrI5Y9qvJx_18dPFuVDwK12yU5SU8p0_c6vaPfODOzTCeSpnDcfT5RiaXVwm1vFQO7DfKCurV8slsbOPPdeavQMHjRowQ1_JN1a4Bhy3p6vxab3v7_xVloGphZaBo89T4FJ_YpfFZ5pNsWDnykMZfmRhvVJEBbr9PJmvcS13fAM4SpPFc_DbgHUYKpe9VKlE9NHQhL-k56lHDDz9WR_e-dDoA_Rv-mJKIIfEvRqZlsl4tVdhorxc0x1Uu1-3G6ir0PTYLUEtrEXJRVfKO-aiO1LrSrhW6exM237lmdbnTB_bOC8bOCiSnYbGkjpBX6-A1uYvU58BcAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionList_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VY246bOhR9z1f4pWojlQiTO9GRzn8cVZVjnOAJ2BxjJpdq_r3CQLCDHUiah_nLaIK9t73XXvvmyU_4lmB29sGvEQBHGsk4BND3v2xGAGQoiijbh_AdiW_e9xOuySUQlykcl6u7hJxCAMt_U8q8mNB9LEPglx_4OxG7hB_9cwhQIflm9DGa1EdBdVSKxJ4yb8ul5GnnhGCsSwRKIqJ5lqBzqA5uLuAdBcpCUP4tP70VuaS7s4c5k4TJEOQZwsTbEnkkhJU7UEL3zKOSpHkIMGGSiE3vdSqD9_VJ5kpwMS461UyrkdhxJr2cXkgI4GQuSHr9eKwBW_pqI_YJF7f6_SvSXh4Lyg5Ka3vgzI6Mw0iLAVPfMGCu9F39Dv3sBAI_U0q3XEREeAJFtMg7epbjG2v9yWpptXdR2btF_LAXvGDRra5KVXVcCGB2AjlPaHSzi4x7UMOFyMu1jNMGACkQy6mknIUAJQnwJ0EOCMpJuXiMqSSeIkwIGK8opUETxiWrFUDuu6_0u3vWy6UG4IsefalupbrhY_qXd0InooLgCgzMkyJlw0i_sqt0CTrZaCSbVv36cUovKoJeabvITgAGOmvdNDqM_7ldO9XpJIj6uGjGBQysgTGvE0GXt1ZqmnTWKWHy2p091jql71AqMfyPevenbjhMdm5NX1OWUEa8bcLxQWPIqvJkU2Dqn30xc9_l657Eis2bMc6ILW4EPzoqkTscHLyPbLxvl4lavqm5tSp_00na1gL2GWPhWiOGkFmDI9xxXOQKFF7Ikjetl4ZzeWepd38fdD2Vrh_N3WvQ3FvQhLNPmodhDZ5KOU1ugX8ApzNNu7PM_qkEHP9p01cnok9G_y7qLrq7_fsY7eMn4acsK_R_8pyRf77imODDlp_lBqOla0cvQFrdjNYPT2CpXmBHDojjCOMTC4jK9dliWubD3_Hc9lvPFbLik_nJU3eVZf4eJRFqnS1zM7YJ2QCdnJEMwc3B3SJizqPWX8oogfyx4lJ1Lp9IF_i8jcxeQiJ8LLSUKwbBOrQdarRcCfwHmXq4fB40crkzyUKDqzc6dVD2ZmJ9b8vhLEt6W7NovcVJT7mGs2KqAyJAiTThbBITOfFeXvoEkRNtyTB8Y_J71bdenrI5Y9qvJx_18dPFuVDwK12yU5SU8p0_c6vaPfODOzTCeSpnDcfT5RiaXVwm1vFQO7DfKCurV8slsbOPPdeavQMHjRowQ1_JN1a4Bhy3p6vxab3v7_xVloGphZaBo89T4FJ_YpfFZ5pNsWDnykMZfmRhvVJEBbr9PJmvcS13fAM4SpPFc_DbgHUYKpe9VKlE9NHQhL_k56lHDDz9WR_e_dDoA_Rv_mJKIIfEvRqZlsl4tVdhorxc0x1Uu1_3G6ir0PTYLUEtrEXJRVfKO_aiO1LrSrhW6exM237lmdbnTB_bOC8bOCiSnYbGkjpBX6_A1uYvU58BcAAA_22_7D = __webpack_require__(11163);
;// ./src/components/sessions/SessionList.css.ts


var categoryContent = '_1jlcnym';
var categoryGroup = '_1jlcnyj';
var categoryTitle = '_1jlcnyk';
var checkboxLabel = '_1jlcnyh';
var clearButton = '_1jlcnyo';
var collapseToggle = '_1jlcnyl';
var SessionList_css_container = '_1jlcny0';
var empty = '_1jlcnyn';
var emptyHint = '_1jlcnyp';
var filterActiveDot = '_1jlcnyb';
var filterControls = '_1jlcnyc';
var filterControlsOpen = '_1jlcnyd';
var filterToggle = '_1jlcny9';
var filterToggleActive = '_1jlcnya';
var filterTopRow = '_1jlcny8';
var filters = '_1jlcny7';
var SessionList_css_header = '_1jlcny1';
var headerActions = '_1jlcny4';
var headerTop = '_1jlcny2';
var newSessionHeaderButton = '_1jlcnyq';
var newSessionIcon = '_1jlcnyr';
var searchInput = '_1jlcnye';
var SessionList_css_select = '_1jlcnyf';
var selectModeButton = '_1jlcny5';
var selectModeButtonActive = '_1jlcny6';
var SessionList_css_sessionList = '_1jlcnyi';
var sortDirButton = '_1jlcnyg';
var SessionList_css_title = '_1jlcny3';
;// ./src/components/sessions/SessionList.tsx
/* __next_internal_client_entry_do_not_use__ SessionList auto */ 
// +feature: session-pinned-section






























// Memoized wrapper: turns stable per-action handlers into per-session closures
// only once per session identity change. The outer SessionList can pass one
// useCallback per action (e.g. onDeleteSession accepts an id), and this wrapper
// creates the () => onDeleteSession(session.id) closure just once.
const SessionRowWrapper = /*#__PURE__*/ react.memo(function SessionRowWrapper(param) {
    let { session, visibleColumns, selectMode, isSelected, suppressApprovalSubStatus, staleThresholdMinutes, onSessionClick, onSessionOpenInNewPane, onDeleteSession, onPauseSession, onResumeSession, onCloneSession, onNewWorkspaceSession, onRestartSession, onRetryNowSession, onCreateCheckpoint, onSetRateLimitEnabled, onToggleAutonomousMode, onTogglePinned, onToggleAutoApprove, onSteerAutonomousSession, onClearConversationState, onHibernateSession, onResumeHibernatedSession, onUpdateTags, onToggleSession, backlogEntry } = param;
    const id = session.id;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionRow, {
        session: session,
        onClick: onSessionClick ? ()=>onSessionClick(session) : undefined,
        onPause: onPauseSession ? ()=>onPauseSession(id) : undefined,
        onResume: onResumeSession ? ()=>onResumeSession(session) : undefined,
        onDelete: onDeleteSession ? ()=>onDeleteSession(id) : undefined,
        onClone: onCloneSession ? ()=>onCloneSession(id) : undefined,
        onOpenInNewPane: onSessionOpenInNewPane ? ()=>onSessionOpenInNewPane(session) : undefined,
        onNewWorkspace: onNewWorkspaceSession ? ()=>onNewWorkspaceSession(id) : undefined,
        onRestart: onRestartSession,
        onRetryNow: onRetryNowSession,
        onCreateCheckpoint: onCreateCheckpoint,
        onSetRateLimitEnabled: onSetRateLimitEnabled,
        onToggleAutonomousMode: onToggleAutonomousMode,
        onTogglePinned: onTogglePinned,
        onToggleAutoApprove: onToggleAutoApprove,
        onSteerAutonomousSession: onSteerAutonomousSession,
        onClearConversationState: onClearConversationState,
        onHibernate: onHibernateSession ? ()=>onHibernateSession(id) : undefined,
        onResumeFromHibernation: onResumeHibernatedSession ? ()=>onResumeHibernatedSession(id) : undefined,
        onUpdateTags: onUpdateTags,
        suppressApprovalSubStatus: suppressApprovalSubStatus,
        staleThresholdMinutes: staleThresholdMinutes,
        visibleColumns: visibleColumns,
        selectMode: selectMode,
        isSelected: isSelected,
        onToggleSelect: onToggleSession ? (e)=>onToggleSession(id, e) : undefined,
        backlogEntry: backlogEntry
    });
});
// Toggle button rendered inside a group header to collapse/expand its sessions.
// Shared between row-mode and card-mode header render sites.
const CategoryCollapseToggle = /*#__PURE__*/ react.memo(function CategoryCollapseToggle(param) {
    let { groupKey, displayName, collapsed, onToggle } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        type: "button",
        "data-testid": "category-collapse-toggle",
        className: collapseToggle,
        "aria-expanded": !collapsed,
        "aria-label": "".concat(collapsed ? "Expand" : "Collapse", " ").concat(displayName, " category"),
        onClick: (e)=>{
            e.stopPropagation();
            onToggle(groupKey);
        },
        children: collapsed ? /*#__PURE__*/ (0,jsx_runtime.jsx)(chevron_right/* default */.A, {
            size: 16,
            "aria-hidden": "true"
        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(chevron_down/* default */.A, {
            size: 16,
            "aria-hidden": "true"
        })
    });
});
const BASE_STORAGE_KEYS = {
    SEARCH_QUERY: 'stapler-squad-search-query',
    SELECTED_STATUS: 'stapler-squad-selected-status',
    SELECTED_CATEGORY: 'stapler-squad-selected-category',
    SELECTED_TAG: 'stapler-squad-selected-tag',
    HIDE_PAUSED: 'stapler-squad-hide-paused',
    SHOW_ARCHIVED: 'stapler-squad-show-archived',
    SHOW_HIDDEN: 'stapler-squad-show-hidden',
    FILTER_NEEDS_APPROVAL: 'stapler-squad-filter-needs-approval',
    GROUPING_STRATEGY: 'stapler-squad-grouping-strategy',
    COLLAPSED_GROUPS: 'stapler-squad-collapsed-groups',
    SORT_FIELD: 'stapler-squad-sort-field',
    SORT_DIR: 'stapler-squad-sort-dir',
    VISIBLE_COLUMNS: 'stapler-squad-visible-columns'
};
const SORT_FIELDS = [
    'lastActivity',
    'name',
    'createdAt',
    'updatedAt',
    'tokenCost'
];
const SORT_DIRS = [
    'asc',
    'desc'
];
const GROUPING_STRATEGY_VALUES = Object.values(strategies/* GroupingStrategy */.j$);
// Stable empty fallback so callers that don't pass backlogIndex (e.g. tests, SessionBoard's
// shared prop surface) don't trigger a new Map() identity on every render.
const EMPTY_BACKLOG_INDEX = new Map();
// Builds a PersistedFieldsConfig keyed off BASE_STORAGE_KEYS, prefixed per-instance
// (e.g. split-pane view) so multiple SessionList instances don't collide in localStorage.
function buildPersistedFieldsConfig() {
    let prefix = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : '';
    const k = (base)=>"".concat(prefix).concat(base);
    return {
        searchQuery: {
            key: k(BASE_STORAGE_KEYS.SEARCH_QUERY),
            defaultValue: ""
        },
        selectedStatus: {
            key: k(BASE_STORAGE_KEYS.SELECTED_STATUS),
            defaultValue: "all",
            isValid: (v)=>v === "all" || typeof v === "number"
        },
        selectedCategory: {
            key: k(BASE_STORAGE_KEYS.SELECTED_CATEGORY),
            defaultValue: "all"
        },
        selectedTag: {
            key: k(BASE_STORAGE_KEYS.SELECTED_TAG),
            defaultValue: "all"
        },
        hidePaused: {
            key: k(BASE_STORAGE_KEYS.HIDE_PAUSED),
            defaultValue: false,
            isValid: (v)=>typeof v === "boolean"
        },
        // showArchived: when true, re-fetches sessions with includeArchived=true (server-side
        // default excludes archived sessions) and stops client-side filtering them out below.
        showArchived: {
            key: k(BASE_STORAGE_KEYS.SHOW_ARCHIVED),
            defaultValue: false,
            isValid: (v)=>typeof v === "boolean"
        },
        // showHidden: same shape as showArchived — when true, re-fetches with
        // includeHidden=true (server-side default excludes Hidden sessions, e.g.
        // Diagnose & Nudge/review one-shot dispatches) and stops client-side
        // filtering them out below.
        showHidden: {
            key: k(BASE_STORAGE_KEYS.SHOW_HIDDEN),
            defaultValue: false,
            isValid: (v)=>typeof v === "boolean"
        },
        // filterNeedsApproval: when true, show only Active sessions with subStatus === NEEDS_APPROVAL.
        filterNeedsApproval: {
            key: k(BASE_STORAGE_KEYS.FILTER_NEEDS_APPROVAL),
            defaultValue: false,
            isValid: (v)=>typeof v === "boolean"
        },
        groupingStrategy: {
            key: k(BASE_STORAGE_KEYS.GROUPING_STRATEGY),
            defaultValue: strategies/* GroupingStrategy */.j$.Category,
            isValid: (v)=>GROUPING_STRATEGY_VALUES.includes(v)
        },
        // Collapsed group keys — flat set shared across grouping strategies (a key that
        // recurs after switching strategies, e.g. "Backlog", stays collapsed).
        collapsedGroups: {
            key: k(BASE_STORAGE_KEYS.COLLAPSED_GROUPS),
            defaultValue: new Set(),
            serialize: (value)=>Array.from(value),
            deserialize: (raw)=>new Set(raw),
            isValid: (v)=>v instanceof Set
        },
        sortField: {
            key: k(BASE_STORAGE_KEYS.SORT_FIELD),
            defaultValue: 'lastActivity',
            isValid: (v)=>SORT_FIELDS.includes(v)
        },
        sortDir: {
            key: k(BASE_STORAGE_KEYS.SORT_DIR),
            defaultValue: 'desc',
            isValid: (v)=>SORT_DIRS.includes(v)
        },
        visibleColumns: {
            key: k(BASE_STORAGE_KEYS.VISIBLE_COLUMNS),
            defaultValue: DEFAULT_VISIBLE_COLUMNS,
            isValid: (v)=>Array.isArray(v)
        }
    };
}
function SessionList(param) {
    let { sessions, onSessionClick, onSessionOpenInNewPane, onDeleteSession, onPauseSession, onResumeSession, onDirectResumeSession, onCloneSession, onNewWorkspaceSession, onRenameSession, onRestartSession, onRetryNowSession, onUpdateTags, onNewSession, onCreateCheckpoint, onListCheckpoints, onForkFromCheckpoint, onSetRateLimitEnabled, onToggleAutonomousMode, onTogglePinned, onToggleAutoApprove, onSteerAutonomousSession, onClearConversationState, onHibernateSession, onResumeHibernatedSession, onFetchArchivedSessions, onFetchHiddenSessions, isLoading = false, storageKeyPrefix, extraHeaderActions, viewMode = "row", backlogIndex = EMPTY_BACKLOG_INDEX } = param;
    // Review queue items indexed by session ID for badge display on session cards
    const { items: reviewItems } = (0,ReviewQueueContext/* useReviewQueueContext */.O)();
    const reviewItemBySessionId = (0,react.useMemo)(()=>{
        const map = new Map(reviewItems.map((item)=>[
                item.sessionId,
                item
            ]));
        return map;
    }, [
        reviewItems
    ]);
    // Terminal-detected status data from Redux store
    const detectedStatusMap = (0,store/* useAppSelector */.GV)(sessionsSlice/* selectDetectedStatusMap */.Hx);
    // Resolved stale-session threshold/notify config, fetched once on mount.
    const staleSessionConfig = useStaleSessionConfig();
    // Stale-session re-render tick: a session can cross the stale threshold purely by
    // clock time passing, with no new session data arriving. Force a re-render every
    // 60s so groupedSessions (below) recomputes and reclassifies it without a page
    // refresh. The setter's argument is discarded — only the re-render matters.
    const [staleRecomputeTick, forceStaleRecompute] = (0,react.useState)(0);
    (0,react.useEffect)(()=>{
        const interval = setInterval(()=>forceStaleRecompute((n)=>n + 1), 60000);
        return ()=>clearInterval(interval);
    }, []);
    // clearedSessions: optimistic approval suppression per session (card mode only; row mode uses SubStatusChip suppression)
    const { clearedSessions } = (0,ApprovalsContext/* useApprovalsContext */.B)();
    // Filters, sort, grouping, and visible columns — persisted across page loads,
    // namespaced per-instance via storageKeyPrefix (e.g. split-pane view).
    const persistedFields = (0,react.useMemo)(()=>buildPersistedFieldsConfig(storageKeyPrefix), [
        storageKeyPrefix
    ]);
    const sessionListViewState = (0,usePersistedViewState/* usePersistedViewState */.P)(persistedFields);
    const { searchQuery, selectedStatus, selectedCategory, selectedTag, hidePaused, showArchived, showHidden, filterNeedsApproval, groupingStrategy, collapsedGroups, sortField, sortDir, visibleColumns } = sessionListViewState.state;
    const { searchQuery: setSearchQuery, selectedStatus: setSelectedStatus, selectedCategory: setSelectedCategory, selectedTag: setSelectedTag, hidePaused: setHidePaused, showArchived: setShowArchived, showHidden: setShowHidden, filterNeedsApproval: setFilterNeedsApproval, groupingStrategy: setGroupingStrategy, collapsedGroups: setCollapsedGroups, sortField: setSortField, sortDir: setSortDir, visibleColumns: setVisibleColumns } = sessionListViewState.setters;
    const [columnPickerOpen, setColumnPickerOpen] = (0,react.useState)(false);
    // Multi-select state for bulk actions
    const [selectMode, setSelectMode] = (0,react.useState)(false);
    const lastAnchorRef = (0,react.useRef)(null);
    const [selectedSessions, setSelectedSessions] = (0,react.useState)(new Set());
    const [bulkFeedback, setBulkFeedback] = (0,react.useState)(null);
    const [isBulkTagEditing, setIsBulkTagEditing] = (0,react.useState)(false);
    const bulkTagEditorTriggerRef = (0,react.useRef)(null);
    // Notification hook for undo toasts
    const { showUndoToast, removeNotification, addNotification } = (0,NotificationContext/* useNotifications */.E$)();
    // Pending-delete state: tracks sessions optimistically removed from the list while undo window is open
    const pendingDeleteRef = (0,react.useRef)(null);
    const [pendingDeleteIds, setPendingDeleteIds] = (0,react.useState)(new Set());
    // Mobile filter panel toggle
    const [filtersOpen, setFiltersOpen] = (0,react.useState)(false);
    // S4: Project data for grouping headers and "Group as..." functionality
    const [projects, setProjects] = (0,react.useState)([]);
    const containerRef = (0,react.useRef)(null);
    const selectButtonRef = (0,react.useRef)(null);
    const projectClientRef = (0,react.useRef)((0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)()));
    // Fetch projects from API (called on mount and after mutations)
    const fetchProjects = (0,react.useCallback)(async ()=>{
        try {
            const response = await projectClientRef.current.listProjects({});
            var _response_projects;
            setProjects((_response_projects = response.projects) !== null && _response_projects !== void 0 ? _response_projects : []);
        } catch (e) {
        // Projects are non-critical; ignore fetch errors
        }
    }, []);
    (0,react.useEffect)(()=>{
        fetchProjects();
    }, [
        fetchProjects
    ]);
    // S4-4: Group selected sessions into a project
    const handleGroupAs = (0,react.useCallback)(async (projectName)=>{
        const sessionIds = Array.from(selectedSessions);
        if (sessionIds.length === 0) return;
        let projectId;
        const existing = projects.find((p)=>p.name.toLowerCase() === projectName.toLowerCase());
        if (existing) {
            projectId = existing.id;
        } else {
            var _created_project;
            const created = await projectClientRef.current.createProject({
                name: projectName,
                description: ""
            });
            var _created_project_id;
            projectId = (_created_project_id = (_created_project = created.project) === null || _created_project === void 0 ? void 0 : _created_project.id) !== null && _created_project_id !== void 0 ? _created_project_id : "";
            if (!projectId) return;
        }
        await projectClientRef.current.assignSessionsToProject({
            projectId,
            sessionIds
        });
        await fetchProjects();
        showFeedback("".concat(sessionIds.length, " session").concat(sessionIds.length !== 1 ? "s" : "", ' grouped as "').concat(projectName, '"'));
        setSelectedSessions(new Set());
        setSelectMode(false);
    }, [
        selectedSessions,
        projects,
        fetchProjects
    ]);
    // S4-5: Inline rename/delete state for project group headers
    const [renamingProjectId, setRenamingProjectId] = (0,react.useState)(null);
    const [renameValue, setRenameValue] = (0,react.useState)("");
    const [renameError, setRenameError] = (0,react.useState)(null);
    const [deletingProjectId, setDeletingProjectId] = (0,react.useState)(null);
    const handleProjectRename = (0,react.useCallback)(async (projectId, newName)=>{
        const trimmed = newName.trim();
        if (!trimmed) return;
        await projectClientRef.current.updateProject({
            id: projectId,
            name: trimmed
        });
        setRenamingProjectId(null);
        setTimeout(()=>{
            var _document_querySelector;
            (_document_querySelector = document.querySelector('[data-rename-btn="'.concat(projectId, '"]'))) === null || _document_querySelector === void 0 ? void 0 : _document_querySelector.focus();
        }, 0);
        await fetchProjects();
    }, [
        fetchProjects
    ]);
    const handleProjectDelete = (0,react.useCallback)(async (projectId)=>{
        await projectClientRef.current.deleteProject({
            id: projectId
        });
        setDeletingProjectId(null);
        await fetchProjects();
    }, [
        fetchProjects
    ]);
    // Re-fetch with includeArchived whenever the toggle changes (including on mount, so a
    // persisted "on" preference re-fetches archived sessions rather than showing a stale
    // client-only filtered view). The server excludes archived sessions by default, so
    // turning the toggle off does not need a re-fetch — filteredSessions below hides them.
    (0,react.useEffect)(()=>{
        if (showArchived) {
            onFetchArchivedSessions === null || onFetchArchivedSessions === void 0 ? void 0 : onFetchArchivedSessions(true);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        showArchived
    ]);
    // Re-fetch with includeHidden whenever the toggle changes — same shape as the
    // includeArchived effect above.
    (0,react.useEffect)(()=>{
        if (showHidden) {
            onFetchHiddenSessions === null || onFetchHiddenSessions === void 0 ? void 0 : onFetchHiddenSessions(true);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        showHidden
    ]);
    // Extract unique categories from sessions
    const categories = (0,react.useMemo)(()=>{
        const categorySet = new Set();
        sessions.forEach((session)=>{
            if (session.category) {
                categorySet.add(session.category);
            }
        });
        return Array.from(categorySet).sort();
    }, [
        sessions
    ]);
    // Extract unique tags from sessions
    const tags = (0,react.useMemo)(()=>{
        const tagSet = new Set();
        sessions.forEach((session)=>{
            if (session.tags) {
                session.tags.forEach((tag)=>tagSet.add(tag));
            }
        });
        return Array.from(tagSet).sort();
    }, [
        sessions
    ]);
    // AC-2: per-session cost data, joined by session_id, for the "Sort: Cost" option.
    const { summary: insightsSummary } = (0,useInsightsService/* useInsightsSummary */.VT)({
        includeOrphans: true
    });
    const costById = (0,react.useMemo)(()=>{
        const m = new Map();
        var _insightsSummary_sessions;
        for (const s of (_insightsSummary_sessions = insightsSummary === null || insightsSummary === void 0 ? void 0 : insightsSummary.sessions) !== null && _insightsSummary_sessions !== void 0 ? _insightsSummary_sessions : []){
            if (s.sessionId) m.set(s.sessionId, s.estimatedCostUsd);
        }
        return m;
    }, [
        insightsSummary
    ]);
    const { filteredSessions, sortedSessions, groupedSessions, filteredSessionIds } = useFilteredGroupedSessions({
        sessions,
        searchQuery,
        selectedStatus,
        selectedCategory,
        selectedTag,
        hidePaused,
        showArchived,
        showHidden,
        filterNeedsApproval,
        pendingDeleteIds,
        sortField,
        sortDir,
        costById,
        groupingStrategy,
        staleThresholdMinutes: staleSessionConfig.thresholdMinutes,
        staleRecomputeTick
    });
    // Epic 4.1: activeSelection — intersection of selectedSessions with currently filtered sessions
    const activeSelection = (0,react.useMemo)(()=>new Set([
            ...selectedSessions
        ].filter((id)=>filteredSessionIds.has(id))), [
        selectedSessions,
        filteredSessionIds
    ]);
    // Derived: whether any filter is active (used for empty-state messaging)
    const hasActiveFilters = !!(searchQuery || selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused || filterNeedsApproval);
    const flatItems = (0,react.useMemo)(()=>{
        if (viewMode !== "row") return [];
        const items = [];
        const isProjectGrouping = groupingStrategy === strategies/* GroupingStrategy */.j$.Project;
        for (const { groupKey, displayName, sessions: grpSessions } of groupedSessions){
            const projectData = isProjectGrouping ? projects.find((p)=>p.id === groupKey || p.name === displayName) : undefined;
            const isUngrouped = groupKey === "No Project";
            items.push({
                kind: "header",
                groupKey,
                displayName,
                groupSessions: grpSessions,
                projectData,
                isProjectGrouping,
                isUngrouped
            });
            if (!collapsedGroups.has(groupKey)) {
                for (const s of grpSessions){
                    items.push({
                        kind: "session",
                        session: s
                    });
                }
            }
        }
        return items;
    }, [
        groupedSessions,
        groupingStrategy,
        projects,
        viewMode,
        collapsedGroups
    ]);
    const flatItemsRef = (0,react.useRef)(flatItems);
    flatItemsRef.current = flatItems;
    // Card-mode virtualization data: flat session array and per-group counts for GroupedVirtuoso.
    const { cardFlatSessions, cardGroupCounts } = (0,react.useMemo)(()=>{
        if (viewMode !== "card") return {
            cardFlatSessions: [],
            cardGroupCounts: []
        };
        const flat = [];
        const counts = [];
        // Collapsed groups keep their slot in cardGroupCounts (count 0) rather than being
        // removed — this preserves the groupIndex → groupedSessions[groupIndex] mapping
        // GroupedVirtuoso's groupContent callback relies on, with no index remap needed.
        for (const { groupKey, sessions: grpSessions } of groupedSessions){
            if (collapsedGroups.has(groupKey)) {
                counts.push(0);
                continue;
            }
            counts.push(grpSessions.length);
            for (const s of grpSessions)flat.push(s);
        }
        return {
            cardFlatSessions: flat,
            cardGroupCounts: counts
        };
    }, [
        groupedSessions,
        viewMode,
        collapsedGroups
    ]);
    const rowVirtualizer = (0,esm/* useVirtualizer */.Te)({
        count: viewMode === "row" ? flatItems.length : 0,
        getScrollElement: ()=>containerRef.current,
        // 64, up from 50 (Epic 2.1 Story 2.1.3): reflects the new typical 2-line
        // wrapped-row height (Story 2.1.1's wrap + Epic 1.2's elapsed second
        // line); measureElement still corrects the real height post-render.
        estimateSize: (i)=>{
            var _flatItems_i;
            return ((_flatItems_i = flatItems[i]) === null || _flatItems_i === void 0 ? void 0 : _flatItems_i.kind) === "header" ? 40 : 64;
        },
        overscan: 8,
        measureElement: (el)=>el.getBoundingClientRect().height
    });
    // Handler for cycling grouping strategy (keyboard shortcut 'G')
    const handleCycleGrouping = ()=>{
        setGroupingStrategy((0,strategies/* cycleGroupingStrategy */.nd)(groupingStrategy));
    };
    const toggleGroupCollapsed = (0,react.useCallback)((groupKey)=>{
        setCollapsedGroups((prev)=>{
            const next = new Set(prev);
            if (next.has(groupKey)) {
                next.delete(groupKey);
            } else {
                next.add(groupKey);
            }
            return next;
        });
    }, []);
    // Bulk actions handlers
    const handleToggleSelectMode = ()=>{
        setSelectMode(!selectMode);
        if (selectMode) {
            // Clear selections when exiting select mode
            setSelectedSessions(new Set());
        }
    };
    const feedbackTimerRef = (0,react.useRef)(null);
    const showFeedback = function(msg) {
        let isError = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : false;
        if (feedbackTimerRef.current !== null) clearTimeout(feedbackTimerRef.current);
        setBulkFeedback(msg);
        feedbackTimerRef.current = setTimeout(()=>{
            setBulkFeedback(null);
            feedbackTimerRef.current = null;
        }, isError ? 5000 : 3000);
    };
    // Entering selectMode automatically when hovering a card and clicking its checkbox.
    const handleToggleSession = (0,react.useCallback)((sessionId, e)=>{
        if ((e === null || e === void 0 ? void 0 : e.shiftKey) && lastAnchorRef.current !== null) {
            const rangeIds = computeRangeIds(lastAnchorRef.current, sessionId, flatItemsRef.current);
            setSelectedSessions(new Set(rangeIds));
        } else {
            setSelectMode(true);
            setSelectedSessions((prev)=>{
                const next = new Set(prev);
                if (next.has(sessionId)) {
                    next.delete(sessionId);
                } else {
                    next.add(sessionId);
                }
                return next;
            });
            lastAnchorRef.current = sessionId;
        }
    }, []);
    const handleSelectAll = (0,react.useCallback)(()=>{
        const allSessionIds = new Set(filteredSessions.map((s)=>s.id));
        setSelectedSessions(allSessionIds);
    }, [
        filteredSessions
    ]);
    // Stable row-handler callbacks passed to SessionRowWrapper.
    // These accept (session) or (id) so SessionRowWrapper can be memoized —
    // the per-session closures are only recreated inside the wrapper when session identity changes.
    const stableOnSessionClick = (0,react.useCallback)((session)=>onSessionClick === null || onSessionClick === void 0 ? void 0 : onSessionClick(session), [
        onSessionClick
    ]);
    const stableOnSessionOpenInNewPane = (0,react.useCallback)((session)=>onSessionOpenInNewPane === null || onSessionOpenInNewPane === void 0 ? void 0 : onSessionOpenInNewPane(session), [
        onSessionOpenInNewPane
    ]);
    const stableOnDeleteSession = (0,react.useCallback)((id)=>onDeleteSession === null || onDeleteSession === void 0 ? void 0 : onDeleteSession(id), [
        onDeleteSession
    ]);
    const stableOnPauseSession = (0,react.useCallback)((id)=>onPauseSession === null || onPauseSession === void 0 ? void 0 : onPauseSession(id), [
        onPauseSession
    ]);
    const stableOnResumeSession = (0,react.useCallback)((session)=>onResumeSession === null || onResumeSession === void 0 ? void 0 : onResumeSession(session), [
        onResumeSession
    ]);
    const stableOnCloneSession = (0,react.useCallback)((id)=>onCloneSession === null || onCloneSession === void 0 ? void 0 : onCloneSession(id), [
        onCloneSession
    ]);
    const stableOnNewWorkspaceSession = (0,react.useCallback)((id)=>onNewWorkspaceSession === null || onNewWorkspaceSession === void 0 ? void 0 : onNewWorkspaceSession(id), [
        onNewWorkspaceSession
    ]);
    const stableOnHibernateSession = (0,react.useCallback)((id)=>onHibernateSession === null || onHibernateSession === void 0 ? void 0 : onHibernateSession(id), [
        onHibernateSession
    ]);
    const stableOnResumeHibernatedSession = (0,react.useCallback)((id)=>onResumeHibernatedSession === null || onResumeHibernatedSession === void 0 ? void 0 : onResumeHibernatedSession(id), [
        onResumeHibernatedSession
    ]);
    const handleClearSelection = (0,react.useCallback)(()=>{
        setSelectedSessions(new Set());
        setSelectMode(false);
        lastAnchorRef.current = null;
        showFeedback("Selection cleared");
        setTimeout(()=>{
            var _selectButtonRef_current;
            return (_selectButtonRef_current = selectButtonRef.current) === null || _selectButtonRef_current === void 0 ? void 0 : _selectButtonRef_current.focus();
        }, 0);
    }, []);
    (0,react.useEffect)(()=>{
        if (!selectMode) return;
        const handler = (e)=>{
            const target = e.target;
            const inInput = target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable;
            if ((e.metaKey || e.ctrlKey) && e.key === "a") {
                if (inInput) return;
                e.preventDefault();
                handleSelectAll();
            }
            if (e.key === "Escape") {
                e.preventDefault();
                handleClearSelection();
                e.stopImmediatePropagation();
            }
        };
        document.addEventListener("keydown", handler);
        return ()=>document.removeEventListener("keydown", handler);
    }, [
        selectMode,
        handleSelectAll,
        handleClearSelection
    ]);
    // Epic 3.3: Flush pending deletes on unmount — fire RPCs immediately rather than losing them.
    // Note: async in cleanup is fire-and-forget; tab-close data loss is a known limitation.
    (0,react.useEffect)(()=>{
        return ()=>{
            void flushPendingDeletes();
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []); // intentionally empty — runs only on unmount
    const handlePauseSelected = ()=>{
        if (!onPauseSession) return;
        const ids = Array.from(activeSelection);
        ids.forEach((id)=>onPauseSession(id));
        showFeedback("".concat(ids.length, " session").concat(ids.length !== 1 ? 's' : '', " paused"));
        setSelectedSessions(new Set());
        setSelectMode(false);
    };
    const handleResumeSelected = ()=>{
        if (!onDirectResumeSession && !onResumeSession) return;
        const ids = Array.from(activeSelection);
        // Bulk resume bypasses the confirmation modal to avoid opening N modals
        ids.forEach((id)=>{
            const session = sessions.find((s)=>s.id === id);
            if (session) {
                if (onDirectResumeSession) {
                    onDirectResumeSession(session);
                } else {
                    onResumeSession === null || onResumeSession === void 0 ? void 0 : onResumeSession(session);
                }
            }
        });
        showFeedback("".concat(ids.length, " session").concat(ids.length !== 1 ? 's' : '', " resumed"));
        setSelectedSessions(new Set());
        setSelectMode(false);
    };
    const handleStopSelected = handlePauseSelected;
    const flushPendingDeletes = (0,react.useCallback)(async ()=>{
        if (!pendingDeleteRef.current) return;
        var _pendingDeleteRef_current_timer;
        clearTimeout((_pendingDeleteRef_current_timer = pendingDeleteRef.current.timer) !== null && _pendingDeleteRef_current_timer !== void 0 ? _pendingDeleteRef_current_timer : undefined);
        const ids = [
            ...pendingDeleteRef.current.ids
        ];
        const toastId = pendingDeleteRef.current.toastId;
        removeNotification(toastId);
        pendingDeleteRef.current = null;
        if (!onDeleteSession) {
            setPendingDeleteIds(new Set());
            return;
        }
        const results = await Promise.allSettled(ids.map((id)=>Promise.resolve(onDeleteSession(id))));
        const failed = [];
        results.forEach((result, i)=>{
            if (result.status === "rejected") failed.push(ids[i]);
        });
        // Clear pendingDeleteIds unconditionally — failed sessions reappear via server state
        setPendingDeleteIds(new Set());
        if (failed.length > 0 && failed.length < ids.length) {
            const succeeded = ids.length - failed.length;
            addNotification({
                message: "".concat(succeeded, " deleted, ").concat(failed.length, " failed — failed sessions are back in the list"),
                notificationType: "error",
                sessionId: "",
                sessionName: ""
            });
        } else if (failed.length === ids.length) {
            addNotification({
                message: "All ".concat(failed.length, " delete").concat(failed.length === 1 ? "" : "s", " failed — sessions are back in the list"),
                notificationType: "error",
                sessionId: "",
                sessionName: ""
            });
        }
    }, [
        onDeleteSession,
        removeNotification,
        addNotification
    ]);
    const handleDeleteSelected = (0,react.useCallback)(()=>{
        if (!onDeleteSession) return;
        // Step 1: Synchronously flush any existing pending batch to avoid async race.
        // Calling flushPendingDeletes() asynchronously would let its terminal
        // setPendingDeleteIds(new Set()) overwrite the new batch we're about to set.
        if (pendingDeleteRef.current) {
            var _pendingDeleteRef_current_timer;
            clearTimeout((_pendingDeleteRef_current_timer = pendingDeleteRef.current.timer) !== null && _pendingDeleteRef_current_timer !== void 0 ? _pendingDeleteRef_current_timer : undefined);
            removeNotification(pendingDeleteRef.current.toastId);
            const prevIds = [
                ...pendingDeleteRef.current.ids
            ];
            pendingDeleteRef.current = null;
            void Promise.allSettled(prevIds.map((id)=>Promise.resolve(onDeleteSession(id))));
        }
        // Step 2: Capture session IDs for delete
        const ids = Array.from(activeSelection);
        // Step 3: Optimistic removal
        setPendingDeleteIds(new Set(ids));
        // Step 4: Exit select mode
        setSelectedSessions(new Set());
        setSelectMode(false);
        lastAnchorRef.current = null;
        setTimeout(()=>{
            var _selectButtonRef_current;
            return (_selectButtonRef_current = selectButtonRef.current) === null || _selectButtonRef_current === void 0 ? void 0 : _selectButtonRef_current.focus();
        }, 0);
        // Step 5: Start pending delete timer
        let toastId = "";
        toastId = showUndoToast("Deleted ".concat(ids.length, " session").concat(ids.length !== 1 ? "s" : ""), ()=>{
            // Guard: if the flush timer fired first, pendingDeleteRef is already null.
            if (!pendingDeleteRef.current) return;
            var _pendingDeleteRef_current_timer;
            clearTimeout((_pendingDeleteRef_current_timer = pendingDeleteRef.current.timer) !== null && _pendingDeleteRef_current_timer !== void 0 ? _pendingDeleteRef_current_timer : undefined);
            removeNotification(toastId);
            setPendingDeleteIds(new Set());
            pendingDeleteRef.current = null;
        }, 5000);
        const timer = setTimeout(()=>{
            void flushPendingDeletes();
        }, 5000);
        pendingDeleteRef.current = {
            ids: new Set(ids),
            timer,
            toastId
        };
    }, [
        onDeleteSession,
        activeSelection,
        flushPendingDeletes,
        showUndoToast,
        removeNotification
    ]);
    const handleBulkAddTag = (triggerEl)=>{
        bulkTagEditorTriggerRef.current = triggerEl;
        setIsBulkTagEditing(true);
    };
    const handleBulkTagSave = (newTags)=>{
        if (newTags.length > 0 && onUpdateTags) {
            const sessionMap = new Map(sessions.map((s)=>[
                    s.id,
                    s
                ]));
            activeSelection.forEach((id)=>{
                const session = sessionMap.get(id);
                var _session_tags;
                const merged = Array.from(new Set([
                    ...(_session_tags = session === null || session === void 0 ? void 0 : session.tags) !== null && _session_tags !== void 0 ? _session_tags : [],
                    ...newTags
                ]));
                onUpdateTags(id, merged);
            });
            showFeedback("Added ".concat(newTags.length, " tag").concat(newTags.length !== 1 ? 's' : '', " to ").concat(activeSelection.size, " session").concat(activeSelection.size !== 1 ? 's' : ''));
        }
        setIsBulkTagEditing(false);
    };
    var _containerRef_current;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: containerRef,
        className: SessionList_css_container,
        "data-context": "session-list",
        "data-select-mode": selectMode ? "true" : "false",
        "aria-multiselectable": selectMode ? "true" : undefined,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionList_css_header,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: headerTop,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                                className: SessionList_css_title,
                                "aria-live": "polite",
                                "aria-atomic": "true",
                                children: [
                                    "Sessions (",
                                    filteredSessions.length !== sessions.length ? "".concat(filteredSessions.length, " of ").concat(sessions.length) : filteredSessions.length,
                                    ")"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: headerActions,
                                children: [
                                    extraHeaderActions,
                                    viewMode === "row" && /*#__PURE__*/ (0,jsx_runtime.jsx)(ColumnPicker, {
                                        visibleColumns: visibleColumns,
                                        onChange: setVisibleColumns,
                                        open: columnPickerOpen,
                                        onOpenChange: setColumnPickerOpen
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: ()=>onNewSession === null || onNewSession === void 0 ? void 0 : onNewSession(),
                                        className: newSessionHeaderButton,
                                        "aria-label": "Create new session",
                                        "aria-keyshortcuts": "Control+K",
                                        title: "New session (Ctrl+K)",
                                        children: "+"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        ref: selectButtonRef,
                                        onClick: handleToggleSelectMode,
                                        className: "".concat(selectModeButton, " ").concat(selectMode ? selectModeButtonActive : ""),
                                        "aria-label": selectMode ? "Exit select mode" : "Enter select mode",
                                        "aria-pressed": selectMode,
                                        children: selectMode ? "Cancel" : "Select"
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: filters,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: filterTopRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "text",
                                        placeholder: "Search sessions...",
                                        value: searchQuery,
                                        onChange: (e)=>setSearchQuery(e.target.value),
                                        className: searchInput,
                                        "aria-label": "Search sessions"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        className: "".concat(filterToggle, " ").concat(selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused || filterNeedsApproval ? filterToggleActive : ""),
                                        "aria-expanded": filtersOpen,
                                        "aria-controls": "session-filter-controls",
                                        "aria-label": (()=>{
                                            const activeCount = [
                                                selectedStatus !== "all",
                                                selectedCategory !== "all",
                                                selectedTag !== "all",
                                                hidePaused,
                                                filterNeedsApproval
                                            ].filter(Boolean).length;
                                            return activeCount > 0 ? "Filters (".concat(activeCount, " active)") : "Filters";
                                        })(),
                                        onClick: ()=>setFiltersOpen((prev)=>!prev),
                                        children: [
                                            "Filters",
                                            (selectedStatus !== "all" || selectedCategory !== "all" || selectedTag !== "all" || hidePaused || filterNeedsApproval) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: filterActiveDot,
                                                "aria-hidden": "true"
                                            })
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)(ActionBar/* ActionBar */.E, {
                                scroll: true,
                                compact: true,
                                gap: "sm",
                                id: "session-filter-controls",
                                className: "".concat(filterControls, " ").concat(filtersOpen ? filterControlsOpen : ""),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                        value: selectedStatus,
                                        onChange: (e)=>setSelectedStatus(e.target.value === "all" ? "all" : Number(e.target.value)),
                                        className: SessionList_css_select,
                                        "aria-label": "Filter by status",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "all",
                                                children: "All Statuses"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: types_pb/* SessionStatus */.Ph.ACTIVE,
                                                children: "Active"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: types_pb/* SessionStatus */.Ph.PAUSED,
                                                children: "Paused"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: types_pb/* SessionStatus */.Ph.STOPPED,
                                                children: "Stopped"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: types_pb/* SessionStatus */.Ph.CREATING,
                                                children: "Creating"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                        value: selectedCategory,
                                        onChange: (e)=>setSelectedCategory(e.target.value),
                                        className: SessionList_css_select,
                                        "aria-label": "Filter by category",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "all",
                                                children: "All Categories"
                                            }),
                                            categories.map((cat)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: cat,
                                                    children: cat
                                                }, cat))
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                        value: selectedTag,
                                        onChange: (e)=>setSelectedTag(e.target.value),
                                        className: SessionList_css_select,
                                        "aria-label": "Filter by tag",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "all",
                                                children: "All Tags"
                                            }),
                                            tags.map((t)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: t,
                                                    children: t
                                                }, t))
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: checkboxLabel,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "checkbox",
                                                checked: hidePaused,
                                                onChange: (e)=>setHidePaused(e.target.checked),
                                                "aria-label": "Hide paused sessions"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                children: "Hide Paused"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: checkboxLabel,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "checkbox",
                                                checked: showArchived,
                                                onChange: (e)=>setShowArchived(e.target.checked),
                                                "aria-label": "Show archived sessions",
                                                "data-testid": "show-archived-toggle"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                children: "Show Archived"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: checkboxLabel,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "checkbox",
                                                checked: showHidden,
                                                onChange: (e)=>setShowHidden(e.target.checked),
                                                "aria-label": "Show hidden sessions",
                                                "data-testid": "show-hidden-toggle"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                children: "Show Hidden"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: checkboxLabel,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "checkbox",
                                                checked: filterNeedsApproval,
                                                onChange: (e)=>setFilterNeedsApproval(e.target.checked),
                                                "aria-label": "Show only sessions needing approval"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                children: "Needs Approval"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                        value: groupingStrategy,
                                        onChange: (e)=>setGroupingStrategy(e.target.value),
                                        className: SessionList_css_select,
                                        title: "Group by (Keyboard: G)",
                                        "aria-label": "Group sessions by (keyboard: G)",
                                        children: Object.entries(strategies/* GroupingStrategyLabels */.YK).map((param)=>{
                                            let [value, label] = param;
                                            return /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: value,
                                                children: label
                                            }, value);
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                        value: sortField,
                                        onChange: (e)=>setSortField(e.target.value),
                                        className: SessionList_css_select,
                                        "aria-label": "Sort sessions by",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "lastActivity",
                                                children: "Sort: Last Activity"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "name",
                                                children: "Sort: Name"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "createdAt",
                                                children: "Sort: Created"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "updatedAt",
                                                children: "Sort: Updated"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "tokenCost",
                                                children: "Sort: Cost"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: ()=>setSortDir((d)=>d === 'asc' ? 'desc' : 'asc'),
                                        className: sortDirButton,
                                        title: sortDir === 'asc' ? 'Ascending — click to sort descending' : 'Descending — click to sort ascending',
                                        "aria-label": sortDir === 'asc' ? 'Switch to descending sort' : 'Switch to ascending sort',
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            "aria-hidden": "true",
                                            children: sortDir === 'asc' ? '↑' : '↓'
                                        })
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            onHibernateSession && /*#__PURE__*/ (0,jsx_runtime.jsx)(MemoryPressureCallout, {
                sessions: filteredSessions,
                onHibernate: onHibernateSession
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: "bulk-feedback-live",
                role: "status",
                "aria-live": "polite",
                "aria-atomic": "true",
                "aria-label": "Action feedback",
                style: {
                    position: "absolute",
                    width: 1,
                    height: 1,
                    overflow: "hidden",
                    clipPath: "inset(50%)",
                    whiteSpace: "nowrap"
                },
                children: bulkFeedback !== null && bulkFeedback !== void 0 ? bulkFeedback : ""
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: "empty-state-live",
                role: "status",
                "aria-live": "polite",
                "aria-atomic": "true",
                style: {
                    position: "absolute",
                    width: 1,
                    height: 1,
                    overflow: "hidden",
                    clipPath: "inset(50%)",
                    whiteSpace: "nowrap"
                },
                children: filteredSessions.length === 0 && hasActiveFilters ? "No sessions found" : ""
            }),
            selectMode && /*#__PURE__*/ (0,jsx_runtime.jsx)(BulkActions, {
                selectedCount: activeSelection.size,
                totalCount: filteredSessions.length,
                onPauseAll: handlePauseSelected,
                onResumeAll: handleResumeSelected,
                onDeleteAll: handleDeleteSelected,
                onAddTagAll: (e)=>handleBulkAddTag(e.currentTarget),
                onSelectAll: handleSelectAll,
                onClearSelection: handleClearSelection,
                feedback: bulkFeedback,
                onGroupAs: handleGroupAs
            }),
            isBulkTagEditing && /*#__PURE__*/ (0,jsx_runtime.jsx)(TagEditor/* TagEditor */.y, {
                tags: [],
                onSave: handleBulkTagSave,
                onCancel: ()=>setIsBulkTagEditing(false),
                sessionTitle: "".concat(selectedSessions.size, " selected session").concat(selectedSessions.size !== 1 ? 's' : ''),
                triggerRef: bulkTagEditorTriggerRef
            }),
            isLoading ? /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionListSkeleton, {}) : filteredSessions.length === 0 ? (()=>{
                return hasActiveFilters ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: empty,
                    role: "region",
                    "aria-label": "No results",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            id: "no-sessions-msg",
                            children: "No sessions found"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: clearButton,
                            "aria-describedby": "no-sessions-msg",
                            onClick: ()=>{
                                setSearchQuery("");
                                setSelectedStatus("all");
                                setSelectedCategory("all");
                                setSelectedTag("all");
                                setHidePaused(false);
                                setFilterNeedsApproval(false);
                            },
                            children: "Clear filters"
                        })
                    ]
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionListEmptyState, {});
            })() : viewMode === "row" ? // Row mode: virtualized — only renders visible items (~20 rows at a time).
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "list",
                "aria-label": "Sessions, ".concat(flatItems.filter((i)=>i && i.kind === "session").length, " items"),
                style: {
                    height: rowVirtualizer.getTotalSize(),
                    width: "100%",
                    position: "relative"
                },
                children: rowVirtualizer.getVirtualItems().map((virtualItem)=>{
                    const item = flatItems[virtualItem.index];
                    if (!item) return null;
                    const isSessionItem = item.kind === "session";
                    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        role: "presentation",
                        ref: rowVirtualizer.measureElement,
                        "data-index": virtualItem.index,
                        style: {
                            position: "absolute",
                            top: 0,
                            left: 0,
                            width: "100%",
                            transform: "translateY(".concat(virtualItem.start, "px)")
                        },
                        children: item.kind === "header" ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            role: "listitem",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                role: "heading",
                                "aria-level": 3,
                                "data-testid": item.groupKey === strategies/* PINNED_GROUP_KEY */.Bv ? "pinned-section-header" : undefined,
                                className: categoryTitle,
                                style: {
                                    display: "flex",
                                    alignItems: "center",
                                    gap: "0.5rem",
                                    flexWrap: "wrap"
                                },
                                onClick: (e)=>{
                                    if (groupingStrategy === strategies/* GroupingStrategy */.j$.None) return;
                                    if (e.target !== e.currentTarget) return;
                                    toggleGroupCollapsed(item.groupKey);
                                },
                                children: item.isProjectGrouping && item.projectData && renamingProjectId === item.projectData.id ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
                                    "aria-label": "Rename project ".concat(item.displayName),
                                    style: {
                                        display: "flex",
                                        gap: "6px",
                                        alignItems: "center",
                                        flexWrap: "wrap"
                                    },
                                    onSubmit: async (e)=>{
                                        e.preventDefault();
                                        setRenameError(null);
                                        try {
                                            await handleProjectRename(item.projectData.id, renameValue);
                                        } catch (e) {
                                            setRenameError("Failed to rename — try again");
                                        }
                                    },
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            autoFocus: true,
                                            type: "text",
                                            value: renameValue,
                                            onChange: (e)=>setRenameValue(e.target.value),
                                            onKeyDown: (e)=>{
                                                if (e.key === "Escape") {
                                                    const id = item.projectData.id;
                                                    setRenamingProjectId(null);
                                                    setRenameError(null);
                                                    setTimeout(()=>{
                                                        var _document_querySelector;
                                                        (_document_querySelector = document.querySelector('[data-rename-btn="'.concat(id, '"]'))) === null || _document_querySelector === void 0 ? void 0 : _document_querySelector.focus();
                                                    }, 0);
                                                }
                                            },
                                            "aria-label": "Rename project ".concat(item.displayName),
                                            "aria-describedby": renameError ? "row-rename-error-".concat(item.projectData.id) : undefined,
                                            style: {
                                                padding: "2px 6px",
                                                border: "1px solid var(--input-focus-border)",
                                                borderRadius: "4px",
                                                fontSize: "inherit",
                                                fontWeight: "inherit",
                                                background: "var(--input-background)",
                                                color: "var(--text-primary)"
                                            }
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "submit",
                                            style: {
                                                background: "none",
                                                border: "none",
                                                cursor: "pointer",
                                                color: "var(--success)",
                                                fontSize: "1rem"
                                            },
                                            title: "Save",
                                            "aria-label": "Save project name",
                                            children: "✓"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "button",
                                            onClick: ()=>{
                                                const id = item.projectData.id;
                                                setRenamingProjectId(null);
                                                setRenameError(null);
                                                setTimeout(()=>{
                                                    var _document_querySelector;
                                                    (_document_querySelector = document.querySelector('[data-rename-btn="'.concat(id, '"]'))) === null || _document_querySelector === void 0 ? void 0 : _document_querySelector.focus();
                                                }, 0);
                                            },
                                            style: {
                                                background: "none",
                                                border: "none",
                                                cursor: "pointer",
                                                color: "var(--text-secondary)",
                                                fontSize: "1rem"
                                            },
                                            title: "Cancel",
                                            "aria-label": "Cancel rename",
                                            children: "✕"
                                        }),
                                        renameError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            id: "row-rename-error-".concat(item.projectData.id),
                                            role: "alert",
                                            style: {
                                                color: "var(--error)",
                                                fontSize: "0.75rem",
                                                width: "100%"
                                            },
                                            children: renameError
                                        })
                                    ]
                                }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        groupingStrategy !== strategies/* GroupingStrategy */.j$.None && /*#__PURE__*/ (0,jsx_runtime.jsx)(CategoryCollapseToggle, {
                                            groupKey: item.groupKey,
                                            displayName: item.displayName,
                                            collapsed: collapsedGroups.has(item.groupKey),
                                            onToggle: toggleGroupCollapsed
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            onClick: ()=>{
                                                if (groupingStrategy === strategies/* GroupingStrategy */.j$.None) return;
                                                toggleGroupCollapsed(item.groupKey);
                                            },
                                            children: [
                                                item.displayName,
                                                " (",
                                                item.groupSessions.length,
                                                ")"
                                            ]
                                        }),
                                        item.isProjectGrouping && item.projectData && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                            children: [
                                                item.projectData.runningCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    role: "img",
                                                    "aria-label": "".concat(item.projectData.runningCount, " running"),
                                                    style: {
                                                        fontSize: "0.75rem",
                                                        padding: "1px 6px",
                                                        background: "var(--success-bg)",
                                                        color: "var(--success)",
                                                        borderRadius: "10px"
                                                    },
                                                    children: [
                                                        item.projectData.runningCount,
                                                        " Running"
                                                    ]
                                                }),
                                                item.projectData.completeCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    role: "img",
                                                    "aria-label": "".concat(item.projectData.completeCount, " complete"),
                                                    style: {
                                                        fontSize: "0.75rem",
                                                        padding: "1px 6px",
                                                        background: "var(--primary)",
                                                        color: "white",
                                                        borderRadius: "10px",
                                                        opacity: 0.85
                                                    },
                                                    children: [
                                                        item.projectData.completeCount,
                                                        " Complete"
                                                    ]
                                                }),
                                                item.projectData.reviewReadyCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    role: "img",
                                                    "aria-label": "".concat(item.projectData.reviewReadyCount, " ready for review"),
                                                    style: {
                                                        fontSize: "0.75rem",
                                                        padding: "1px 6px",
                                                        background: "var(--warning-bg)",
                                                        color: "var(--warning)",
                                                        borderRadius: "10px"
                                                    },
                                                    children: [
                                                        item.projectData.reviewReadyCount,
                                                        " Review"
                                                    ]
                                                })
                                            ]
                                        }),
                                        item.isProjectGrouping && item.projectData && !item.isUngrouped && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            style: {
                                                marginLeft: "auto",
                                                display: "flex",
                                                gap: "4px",
                                                flexShrink: 0
                                            },
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    type: "button",
                                                    "data-rename-btn": item.projectData.id,
                                                    onClick: ()=>{
                                                        setRenamingProjectId(item.projectData.id);
                                                        setRenameValue(item.projectData.name);
                                                        setRenameError(null);
                                                    },
                                                    title: "Rename project",
                                                    "aria-label": "Rename project ".concat(item.displayName),
                                                    "aria-expanded": false,
                                                    style: {
                                                        background: "none",
                                                        border: "none",
                                                        cursor: "pointer",
                                                        color: "var(--text-secondary)",
                                                        fontSize: "0.875rem",
                                                        padding: "2px"
                                                    },
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "✏️"
                                                    })
                                                }),
                                                deletingProjectId === item.projectData.id ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    role: "group",
                                                    "aria-label": "Confirm project deletion",
                                                    style: {
                                                        display: "flex",
                                                        gap: "4px",
                                                        alignItems: "center",
                                                        fontSize: "0.75rem",
                                                        color: "var(--text-secondary)"
                                                    },
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            role: "alert",
                                                            "aria-atomic": "true",
                                                            children: [
                                                                "Remove project? ",
                                                                item.groupSessions.length,
                                                                " session",
                                                                item.groupSessions.length !== 1 ? "s" : "",
                                                                " will become ungrouped."
                                                            ]
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                            type: "button",
                                                            onClick: ()=>handleProjectDelete(item.projectData.id),
                                                            "aria-label": "Confirm delete project ".concat(item.displayName),
                                                            style: {
                                                                background: "var(--error)",
                                                                color: "white",
                                                                border: "none",
                                                                borderRadius: "4px",
                                                                cursor: "pointer",
                                                                padding: "2px 6px",
                                                                fontSize: "0.75rem"
                                                            },
                                                            children: "Delete"
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                            type: "button",
                                                            autoFocus: true,
                                                            "aria-label": "Cancel delete project ".concat(item.displayName),
                                                            onClick: ()=>setDeletingProjectId(null),
                                                            style: {
                                                                background: "none",
                                                                border: "1px solid var(--border-color)",
                                                                borderRadius: "4px",
                                                                cursor: "pointer",
                                                                padding: "2px 6px",
                                                                fontSize: "0.75rem",
                                                                color: "var(--text-secondary)"
                                                            },
                                                            children: "Cancel"
                                                        })
                                                    ]
                                                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    type: "button",
                                                    onClick: ()=>setDeletingProjectId(item.projectData.id),
                                                    title: "Delete project",
                                                    "aria-label": "Delete project ".concat(item.displayName),
                                                    style: {
                                                        background: "none",
                                                        border: "none",
                                                        cursor: "pointer",
                                                        color: "var(--text-secondary)",
                                                        fontSize: "0.875rem",
                                                        padding: "2px"
                                                    },
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        "aria-hidden": "true",
                                                        children: "\uD83D\uDDD1️"
                                                    })
                                                })
                                            ]
                                        })
                                    ]
                                })
                            })
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            role: "listitem",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionRowWrapper, {
                                session: item.session,
                                onSessionClick: stableOnSessionClick,
                                onSessionOpenInNewPane: stableOnSessionOpenInNewPane,
                                onDeleteSession: stableOnDeleteSession,
                                onPauseSession: stableOnPauseSession,
                                onResumeSession: stableOnResumeSession,
                                onCloneSession: stableOnCloneSession,
                                onNewWorkspaceSession: stableOnNewWorkspaceSession,
                                onRestartSession: onRestartSession,
                                onRetryNowSession: onRetryNowSession,
                                onCreateCheckpoint: onCreateCheckpoint,
                                onSetRateLimitEnabled: onSetRateLimitEnabled,
                                onToggleAutonomousMode: onToggleAutonomousMode,
                                onTogglePinned: onTogglePinned,
                                onToggleAutoApprove: onToggleAutoApprove,
                                onSteerAutonomousSession: onSteerAutonomousSession,
                                onClearConversationState: onClearConversationState,
                                onHibernateSession: stableOnHibernateSession,
                                onResumeHibernatedSession: stableOnResumeHibernatedSession,
                                onUpdateTags: onUpdateTags,
                                suppressApprovalSubStatus: clearedSessions.has(item.session.id),
                                staleThresholdMinutes: staleSessionConfig.thresholdMinutes,
                                visibleColumns: visibleColumns,
                                selectMode: selectMode,
                                isSelected: selectedSessions.has(item.session.id),
                                onToggleSession: handleToggleSession,
                                backlogEntry: backlogIndex.get(item.session.id)
                            })
                        })
                    }, virtualItem.key);
                })
            }) : // Card mode: virtualized with GroupedVirtuoso — only visible cards are rendered.
            /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* GroupedVirtuoso */.kN, {
                customScrollParent: (_containerRef_current = containerRef.current) !== null && _containerRef_current !== void 0 ? _containerRef_current : undefined,
                groupCounts: cardGroupCounts,
                groupContent: (groupIndex)=>{
                    const grp = groupedSessions[groupIndex];
                    if (!grp) return null;
                    const { groupKey, displayName, sessions: grpSessions } = grp;
                    const isProjectGrouping = groupingStrategy === strategies/* GroupingStrategy */.j$.Project;
                    const projectData = isProjectGrouping ? projects.find((p)=>p.id === groupKey || p.name === displayName) : undefined;
                    const isUngrouped = groupKey === "No Project";
                    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        role: "heading",
                        "aria-level": 3,
                        "data-testid": groupKey === strategies/* PINNED_GROUP_KEY */.Bv ? "pinned-section-header" : undefined,
                        className: categoryTitle,
                        style: {
                            display: "flex",
                            alignItems: "center",
                            gap: "0.5rem",
                            flexWrap: "wrap"
                        },
                        onClick: (e)=>{
                            if (groupingStrategy === strategies/* GroupingStrategy */.j$.None) return;
                            if (e.target !== e.currentTarget) return;
                            toggleGroupCollapsed(groupKey);
                        },
                        children: isProjectGrouping && projectData && renamingProjectId === projectData.id ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
                            "aria-label": "Rename project ".concat(displayName),
                            style: {
                                display: "flex",
                                gap: "6px",
                                alignItems: "center",
                                flexWrap: "wrap"
                            },
                            onSubmit: async (e)=>{
                                e.preventDefault();
                                setRenameError(null);
                                try {
                                    await handleProjectRename(projectData.id, renameValue);
                                } catch (e) {
                                    setRenameError("Failed to rename — try again");
                                }
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                    autoFocus: true,
                                    type: "text",
                                    value: renameValue,
                                    onChange: (e)=>setRenameValue(e.target.value),
                                    onKeyDown: (e)=>{
                                        if (e.key === "Escape") {
                                            const id = projectData.id;
                                            setRenamingProjectId(null);
                                            setRenameError(null);
                                            setTimeout(()=>{
                                                var _document_querySelector;
                                                (_document_querySelector = document.querySelector('[data-rename-btn="'.concat(id, '"]'))) === null || _document_querySelector === void 0 ? void 0 : _document_querySelector.focus();
                                            }, 0);
                                        }
                                    },
                                    "aria-label": "Rename project ".concat(displayName),
                                    "aria-describedby": renameError ? "card-rename-error-".concat(projectData.id) : undefined,
                                    style: {
                                        padding: "2px 6px",
                                        border: "1px solid var(--input-focus-border)",
                                        borderRadius: "4px",
                                        fontSize: "inherit",
                                        fontWeight: "inherit",
                                        background: "var(--input-background)",
                                        color: "var(--text-primary)"
                                    }
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "submit",
                                    style: {
                                        background: "none",
                                        border: "none",
                                        cursor: "pointer",
                                        color: "var(--success)",
                                        fontSize: "1rem"
                                    },
                                    title: "Save",
                                    "aria-label": "Save project name",
                                    children: "✓"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    onClick: ()=>{
                                        const id = projectData.id;
                                        setRenamingProjectId(null);
                                        setRenameError(null);
                                        setTimeout(()=>{
                                            var _document_querySelector;
                                            (_document_querySelector = document.querySelector('[data-rename-btn="'.concat(id, '"]'))) === null || _document_querySelector === void 0 ? void 0 : _document_querySelector.focus();
                                        }, 0);
                                    },
                                    style: {
                                        background: "none",
                                        border: "none",
                                        cursor: "pointer",
                                        color: "var(--text-secondary)",
                                        fontSize: "1rem"
                                    },
                                    title: "Cancel",
                                    "aria-label": "Cancel rename",
                                    children: "✕"
                                }),
                                renameError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    id: "card-rename-error-".concat(projectData.id),
                                    role: "alert",
                                    style: {
                                        color: "var(--error)",
                                        fontSize: "0.75rem",
                                        width: "100%"
                                    },
                                    children: renameError
                                })
                            ]
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                groupingStrategy !== strategies/* GroupingStrategy */.j$.None && /*#__PURE__*/ (0,jsx_runtime.jsx)(CategoryCollapseToggle, {
                                    groupKey: groupKey,
                                    displayName: displayName,
                                    collapsed: collapsedGroups.has(groupKey),
                                    onToggle: toggleGroupCollapsed
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    onClick: ()=>{
                                        if (groupingStrategy === strategies/* GroupingStrategy */.j$.None) return;
                                        toggleGroupCollapsed(groupKey);
                                    },
                                    children: [
                                        displayName,
                                        " (",
                                        grpSessions.length,
                                        ")"
                                    ]
                                }),
                                isProjectGrouping && projectData && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        projectData.runningCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            role: "img",
                                            "aria-label": "".concat(projectData.runningCount, " running"),
                                            style: {
                                                fontSize: "0.75rem",
                                                padding: "1px 6px",
                                                background: "var(--success-bg)",
                                                color: "var(--success)",
                                                borderRadius: "10px"
                                            },
                                            children: [
                                                projectData.runningCount,
                                                " Running"
                                            ]
                                        }),
                                        projectData.completeCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            role: "img",
                                            "aria-label": "".concat(projectData.completeCount, " complete"),
                                            style: {
                                                fontSize: "0.75rem",
                                                padding: "1px 6px",
                                                background: "var(--primary)",
                                                color: "white",
                                                borderRadius: "10px",
                                                opacity: 0.85
                                            },
                                            children: [
                                                projectData.completeCount,
                                                " Complete"
                                            ]
                                        }),
                                        projectData.reviewReadyCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            role: "img",
                                            "aria-label": "".concat(projectData.reviewReadyCount, " ready for review"),
                                            style: {
                                                fontSize: "0.75rem",
                                                padding: "1px 6px",
                                                background: "var(--warning-bg)",
                                                color: "var(--warning)",
                                                borderRadius: "10px"
                                            },
                                            children: [
                                                projectData.reviewReadyCount,
                                                " Review"
                                            ]
                                        })
                                    ]
                                }),
                                isProjectGrouping && projectData && !isUngrouped && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    style: {
                                        marginLeft: "auto",
                                        display: "flex",
                                        gap: "4px"
                                    },
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "button",
                                            "data-rename-btn": projectData.id,
                                            onClick: ()=>{
                                                setRenamingProjectId(projectData.id);
                                                setRenameValue(projectData.name);
                                                setRenameError(null);
                                            },
                                            title: "Rename project",
                                            "aria-label": "Rename project ".concat(displayName),
                                            "aria-expanded": false,
                                            style: {
                                                background: "none",
                                                border: "none",
                                                cursor: "pointer",
                                                color: "var(--text-secondary)",
                                                fontSize: "0.875rem",
                                                padding: "2px"
                                            },
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                "aria-hidden": "true",
                                                children: "✏️"
                                            })
                                        }),
                                        deletingProjectId === projectData.id ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                            role: "group",
                                            "aria-label": "Confirm project deletion",
                                            style: {
                                                display: "flex",
                                                gap: "4px",
                                                alignItems: "center",
                                                fontSize: "0.75rem",
                                                color: "var(--text-secondary)"
                                            },
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    role: "alert",
                                                    "aria-atomic": "true",
                                                    children: [
                                                        "Remove project? ",
                                                        grpSessions.length,
                                                        " session",
                                                        grpSessions.length !== 1 ? "s" : "",
                                                        " will become ungrouped."
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    type: "button",
                                                    onClick: ()=>handleProjectDelete(projectData.id),
                                                    "aria-label": "Confirm delete project ".concat(displayName),
                                                    style: {
                                                        background: "var(--error)",
                                                        color: "white",
                                                        border: "none",
                                                        borderRadius: "4px",
                                                        cursor: "pointer",
                                                        padding: "2px 6px",
                                                        fontSize: "0.75rem"
                                                    },
                                                    children: "Delete"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    type: "button",
                                                    autoFocus: true,
                                                    "aria-label": "Cancel delete project ".concat(displayName),
                                                    onClick: ()=>setDeletingProjectId(null),
                                                    style: {
                                                        background: "none",
                                                        border: "1px solid var(--border-color)",
                                                        borderRadius: "4px",
                                                        cursor: "pointer",
                                                        padding: "2px 6px",
                                                        fontSize: "0.75rem",
                                                        color: "var(--text-secondary)"
                                                    },
                                                    children: "Cancel"
                                                })
                                            ]
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            type: "button",
                                            onClick: ()=>setDeletingProjectId(projectData.id),
                                            title: "Delete project",
                                            "aria-label": "Delete project ".concat(displayName),
                                            style: {
                                                background: "none",
                                                border: "none",
                                                cursor: "pointer",
                                                color: "var(--text-secondary)",
                                                fontSize: "0.875rem",
                                                padding: "2px"
                                            },
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                "aria-hidden": "true",
                                                children: "\uD83D\uDDD1️"
                                            })
                                        })
                                    ]
                                })
                            ]
                        })
                    });
                },
                itemContent: (index)=>{
                    var _detectedStatusMap_session_id, _detectedStatusMap_session_id1;
                    const session = cardFlatSessions[index];
                    if (!session) return null;
                    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        role: "listitem",
                        style: {
                            '--card-index': index
                        },
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionCard, {
                            session: session,
                            onClick: ()=>onSessionClick === null || onSessionClick === void 0 ? void 0 : onSessionClick(session),
                            onOpenInNewPane: onSessionOpenInNewPane ? ()=>onSessionOpenInNewPane(session) : undefined,
                            onDelete: ()=>onDeleteSession === null || onDeleteSession === void 0 ? void 0 : onDeleteSession(session.id),
                            onPause: ()=>onPauseSession === null || onPauseSession === void 0 ? void 0 : onPauseSession(session.id),
                            onResume: ()=>onResumeSession === null || onResumeSession === void 0 ? void 0 : onResumeSession(session),
                            onClone: ()=>onCloneSession === null || onCloneSession === void 0 ? void 0 : onCloneSession(session.id),
                            onNewWorkspace: ()=>onNewWorkspaceSession === null || onNewWorkspaceSession === void 0 ? void 0 : onNewWorkspaceSession(session.id),
                            onRename: onRenameSession,
                            onRestart: onRestartSession,
                            onRetryNow: onRetryNowSession,
                            onUpdateTags: onUpdateTags,
                            onCreateCheckpoint: onCreateCheckpoint,
                            onListCheckpoints: onListCheckpoints,
                            onForkFromCheckpoint: onForkFromCheckpoint,
                            onSetRateLimitEnabled: onSetRateLimitEnabled,
                            onToggleAutonomousMode: onToggleAutonomousMode,
                            onTogglePinned: onTogglePinned,
                            onToggleAutoApprove: onToggleAutoApprove,
                            onSteerAutonomousSession: onSteerAutonomousSession,
                            onClearConversationState: onClearConversationState,
                            onHibernate: onHibernateSession ? ()=>onHibernateSession(session.id) : undefined,
                            onResumeFromHibernation: onResumeHibernatedSession ? ()=>onResumeHibernatedSession(session.id) : undefined,
                            selectMode: selectMode,
                            isSelected: selectedSessions.has(session.id),
                            onToggleSelect: (e)=>handleToggleSession(session.id, e),
                            reviewItem: reviewItemBySessionId.get(session.id),
                            backlogEntry: backlogIndex.get(session.id),
                            staleThresholdMinutes: staleSessionConfig.thresholdMinutes,
                            detectedStatus: (_detectedStatusMap_session_id = detectedStatusMap[session.id]) === null || _detectedStatusMap_session_id === void 0 ? void 0 : _detectedStatusMap_session_id.detectedStatus,
                            detectedContext: (_detectedStatusMap_session_id1 = detectedStatusMap[session.id]) === null || _detectedStatusMap_session_id1 === void 0 ? void 0 : _detectedStatusMap_session_id1.detectedContext,
                            suppressApprovalSubStatus: clearedSessions.has(session.id)
                        })
                    });
                }
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@dnd-kit+core@6.3.1+498059a1009c1789/node_modules/@dnd-kit/core/dist/core.esm.js + 1 modules
var core_esm = __webpack_require__(59621);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
// EXTERNAL MODULE: ./src/lib/hooks/useSessionService.ts + 1 modules
var useSessionService = __webpack_require__(73117);
// EXTERNAL MODULE: ./src/lib/store/store.ts + 1 modules
var store_store = __webpack_require__(25337);
;// ./src/lib/board/columns.ts

const BOARD_COLUMNS = [
    {
        key: "running",
        label: "Running"
    },
    {
        key: "needs_review",
        label: "Needs Review"
    },
    {
        key: "paused",
        label: "Paused"
    },
    {
        key: "complete",
        label: "Complete"
    }
];
/**
 * Maps a session's backend status/subStatus to the board column it renders in.
 * UNSPECIFIED falls back to "running" defensively — it should not occur in practice.
 */ function getBoardColumnKey(session) {
    if (session.status === types_pb/* SessionStatus */.Ph.ACTIVE && (session.subStatus === types_pb/* SubStatus */.BJ.NEEDS_APPROVAL || session.subStatus === types_pb/* SubStatus */.BJ.INPUT_REQUIRED)) {
        return "needs_review";
    }
    if (session.status === types_pb/* SessionStatus */.Ph.ACTIVE || session.status === types_pb/* SessionStatus */.Ph.CREATING || session.status === types_pb/* SessionStatus */.Ph.RESTORING) {
        return "running";
    }
    if (session.status === types_pb/* SessionStatus */.Ph.PAUSED || session.status === types_pb/* SessionStatus */.Ph.HIBERNATED) {
        return "paused";
    }
    if (session.status === types_pb/* SessionStatus */.Ph.STOPPED) {
        return "complete";
    }
    return "running";
}

;// ./src/lib/board/transitions.ts

const legalBoardTransitions = {
    running: [
        "paused",
        "complete"
    ],
    paused: [
        "running",
        "complete"
    ],
    needs_review: [],
    complete: []
};
function isLegalBoardDrag(from, to) {
    var _legalBoardTransitions_from;
    var _legalBoardTransitions_from_includes;
    return (_legalBoardTransitions_from_includes = (_legalBoardTransitions_from = legalBoardTransitions[from]) === null || _legalBoardTransitions_from === void 0 ? void 0 : _legalBoardTransitions_from.includes(to)) !== null && _legalBoardTransitions_from_includes !== void 0 ? _legalBoardTransitions_from_includes : false;
}
/**
 * Status-aware wrapper around isLegalBoardDrag: CREATING/RESTORING sessions have narrower
 * real backend edges than the "running" column they render in, so any drag out of that
 * column is illegal while transient regardless of column-level legality.
 */ function isLegalBoardDragForSession(session, fromColumn, toColumn) {
    if (session.status === types_pb/* SessionStatus */.Ph.CREATING || session.status === types_pb/* SessionStatus */.Ph.RESTORING) {
        return false;
    }
    return isLegalBoardDrag(fromColumn, toColumn);
}

;// ./src/lib/board/statusForColumnMove.ts

/**
 * Maps a drop target column to the concrete SessionStatus to send via updateSession, branching
 * on the session's *current* status (not just its column) — "paused" column members can be
 * either PAUSED or HIBERNATED and need different handling on the way out.
 *
 * Returns null when the caller must use a different RPC entirely (resumeHibernatedSession for
 * a HIBERNATED session moving to "running" — hibernation has its own dedicated RPC, it isn't a
 * plain status write).
 */ function statusForColumnMove(session, targetColumn) {
    switch(targetColumn){
        case "paused":
            return types_pb/* SessionStatus */.Ph.PAUSED;
        case "complete":
            return types_pb/* SessionStatus */.Ph.STOPPED;
        case "running":
            if (session.status === types_pb/* SessionStatus */.Ph.HIBERNATED) {
                return null;
            }
            return types_pb/* SessionStatus */.Ph.ACTIVE;
        case "needs_review":
            // No board-drag ever targets "needs_review" directly (legalBoardTransitions has no
            // inbound edges to it) — callers should never reach this branch via a legal drag.
            return null;
        default:
            return null;
    }
}

;// ./src/lib/board/compositeId.ts
/**
 * dnd-kit requires unique ids for every draggable/droppable within one DndContext. Once
 * multiple swimlane rows each render the same 4 column keys (and Tag grouping renders one
 * session's card in multiple rows simultaneously), a raw `column.key`/`session.id` id scheme
 * collides. Every BoardColumn/BoardCard id is scoped by rowKey as `${rowKey}:${entityId}` —
 * `"__default__"` until real swimlane keys land — so the scheme is uniform from Phase 3
 * onward, not retrofitted later.
 */ function makeCompositeId(rowKey, entityId) {
    return "".concat(rowKey, ":").concat(entityId);
}
/**
 * Splits a composite `${rowKey}:${entityId}` id on the LAST `:` — session IDs and
 * BoardColumnKey values never contain `:`, but rowKey does once Phase 6 wires real
 * grouping-strategy values through (a tag/category/path string can legitimately contain a
 * colon, e.g. a Windows-style path or a "type:bug" tag). Splitting on the last occurrence
 * keeps parsing correct regardless of what the rowKey contains.
 */ function parseCompositeId(id) {
    const idx = id.lastIndexOf(":");
    if (idx === -1) {
        return {
            rowKey: "",
            entityId: id
        };
    }
    return {
        rowKey: id.slice(0, idx),
        entityId: id.slice(idx + 1)
    };
}

;// ./src/lib/utils/mergeRefs.ts
/**
 * Composes multiple refs (callback refs or ref objects) into a single ref callback, so one
 * DOM node can be attached to several independent ref sources — e.g. a virtualizer's scroll
 * ref and dnd-kit's `useDroppable` `setNodeRef` on the same scrollable container.
 */ function mergeRefs() {
    for(var _len = arguments.length, refs = new Array(_len), _key = 0; _key < _len; _key++){
        refs[_key] = arguments[_key];
    }
    return (node)=>{
        for (const ref of refs){
            if (ref == null) continue;
            if (typeof ref === "function") {
                ref(node);
            } else {
                ref.current = node;
            }
        }
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/grip-vertical.mjs
var grip_vertical = __webpack_require__(19531);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@dnd-kit+utilities@3.2.2+55f3e2d4ca346cd1/node_modules/@dnd-kit/utilities/dist/utilities.esm.js
var utilities_esm = __webpack_require__(64504);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/move-right.mjs
var move_right = __webpack_require__(34100);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FMoveToMenu.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62T0XLCIBBF3_0KXjpTH3BI1NbixzgYNsnWuGSAaEzHf--EaDVax-r0Ee5hl71cRosoVmomVoJ9DRjT6MpC7SRLC6jnA8ZUgRlx9LB2kiVAHmy7_Vk5j-mOJ4Y8kD-X2pPc5RZpJZlod7aofS7ZZFKGkjlglvvTulRaI2UHeGmsBitZVNbMmQI12yj7yvki-oAmtg0MTxS3SmPl5AUyfu8YlawyayrSl0CnJ6Yw9lISnVRZ12qlweNY3ipy6NGQPKvMxCiaOgbKwXywH4x-3JS52YANnt6-x2zYOxQFvDTHNinWoNveayR-MDF6E51r96b7Dx9nB6Z9T6XN9gooAtBwJA21ZFMheg_ap-NdoK8iFgKj0ULSjZ2YolpTz5n4oXDe6t8ML-8velaRIXgmXCEapbJA_k6uUkOeO2zgquoy6B5qz8NUkhWQ-htZPGZBiJfHoxk_E81xwNfKZkiHj_qoy7_aEv_Blv03ej1FvKQEAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FMoveToMenu_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62T0XLCIBBF3_0KXjpTH3BI1NbixzgYNsnWuGSAaEzHf_EaDVax_r0Ee5hl71cRosoVmomVoJ9DRjT6MpC7SRLC6jnA8ZUgRlx9LB2kiVAHmy7_Vk5j_mOJ4Y8kD_X2pPc5RZpJZlod7aofS7ZZFKGkjlglvvTulRaI2UHeGmsBitZVNbMmQI12yj7yvki_oAmtg0MTxS3SmPl5AUyfu8YlawyayrSl0CnJ6Yw9lISnVRZ12qlweNY3ipy6NGQPKvMxCiaOgbKwXywH4x_3JS52YANnt6_x2zYOxQFvDTHNinWoNveayR_MDF6E51r96b7Dx9nB6Z9T6XN9gooAtBwJA21ZFMheg_ap_NdoK8iFgKj0ULSjZ2YolpTz5n4oXDe6t8ML_8velaRIXgmXCEapbJA_k6uUkOeO2zgquoy6B5qz8NUkhWQ_htZPGZBiJfHoxk_E81xwNfKZkiHj_qoy7_aEv_Blv03ej1FvKQEAAA_22_7D = __webpack_require__(11122);
;// ./src/components/sessions/MoveToMenu.css.ts


var menu = '_12aa80k1';
var menuEmpty = '_12aa80k3';
var menuItem = '_12aa80k2';
var menuTrigger = '_12aa80k0';
;// ./src/components/sessions/MoveToMenu.tsx
/* __next_internal_client_entry_do_not_use__ MoveToMenu auto */ 







function columnLabel(key) {
    var _BOARD_COLUMNS_find;
    var _BOARD_COLUMNS_find_label;
    return (_BOARD_COLUMNS_find_label = (_BOARD_COLUMNS_find = BOARD_COLUMNS.find((c)=>c.key === key)) === null || _BOARD_COLUMNS_find === void 0 ? void 0 : _BOARD_COLUMNS_find.label) !== null && _BOARD_COLUMNS_find_label !== void 0 ? _BOARD_COLUMNS_find_label : key;
}
/**
 * Target columns offered for `currentColumn`. Mirrors legalBoardTransitions, plus the
 * needs_review -> running special case: that move resolves the pending approval rather than
 * writing a raw status (see attemptColumnMove in SessionBoard.tsx), so it has no entry in
 * legalBoardTransitions itself and has to be added back in here.
 */ function menuTargets(currentColumn) {
    if (currentColumn === "needs_review") return [
        "running"
    ];
    var _legalBoardTransitions_currentColumn;
    return (_legalBoardTransitions_currentColumn = legalBoardTransitions[currentColumn]) !== null && _legalBoardTransitions_currentColumn !== void 0 ? _legalBoardTransitions_currentColumn : [];
}
/**
 * Non-drag "Move to..." fallback every BoardCard exposes (WCAG SC 2.5.7 -- dragging must not
 * be the only way to perform a board move). A small trigger + menu, modeled on
 * SessionActionsOverflow's portal-based overflow menu.
 */ function MoveToMenu(param) {
    let { session, currentColumn, onMove } = param;
    const [isOpen, setIsOpen] = (0,react.useState)(false);
    const [menuPos, setMenuPos] = (0,react.useState)({
        top: 0,
        left: 0
    });
    const triggerRef = (0,react.useRef)(null);
    const menuRef = (0,react.useRef)(null);
    const menuId = (0,react.useId)();
    const targets = menuTargets(currentColumn);
    (0,useFocusTrap/* useFocusTrap */.r)(menuRef, isOpen, triggerRef);
    const close = (0,react.useCallback)(()=>setIsOpen(false), []);
    const openMenu = (0,react.useCallback)(()=>{
        var _triggerRef_current;
        const rect = (_triggerRef_current = triggerRef.current) === null || _triggerRef_current === void 0 ? void 0 : _triggerRef_current.getBoundingClientRect();
        if (rect) {
            setMenuPos({
                top: rect.bottom + 4,
                left: rect.left
            });
        }
        setIsOpen(true);
    }, []);
    (0,react.useEffect)(()=>{
        if (!isOpen) return;
        const handler = (e)=>{
            var _triggerRef_current, _menuRef_current;
            const target = e.target;
            if (((_triggerRef_current = triggerRef.current) === null || _triggerRef_current === void 0 ? void 0 : _triggerRef_current.contains(target)) || ((_menuRef_current = menuRef.current) === null || _menuRef_current === void 0 ? void 0 : _menuRef_current.contains(target))) return;
            close();
        };
        document.addEventListener("mousedown", handler);
        return ()=>document.removeEventListener("mousedown", handler);
    }, [
        isOpen,
        close
    ]);
    const handleSelect = (0,react.useCallback)((toColumn)=>{
        close();
        onMove(toColumn);
    }, [
        close,
        onMove
    ]);
    const handleMenuKeyDown = (0,react.useCallback)((e)=>{
        if (e.key === "Escape") {
            close();
            return;
        }
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            var _menuRef_current, _items_nextIndex;
            e.preventDefault();
            var _menuRef_current_querySelectorAll;
            const items = Array.from((_menuRef_current_querySelectorAll = (_menuRef_current = menuRef.current) === null || _menuRef_current === void 0 ? void 0 : _menuRef_current.querySelectorAll('[role="menuitem"]')) !== null && _menuRef_current_querySelectorAll !== void 0 ? _menuRef_current_querySelectorAll : []);
            if (items.length === 0) return;
            const currentIndex = items.indexOf(document.activeElement);
            const nextIndex = e.key === "ArrowDown" ? (currentIndex + 1) % items.length : (currentIndex - 1 + items.length) % items.length;
            (_items_nextIndex = items[nextIndex]) === null || _items_nextIndex === void 0 ? void 0 : _items_nextIndex.focus();
        }
    }, [
        close
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                ref: triggerRef,
                id: "board-card-move-trigger-".concat(session.id),
                className: menuTrigger,
                "aria-haspopup": "menu",
                "aria-expanded": isOpen,
                "aria-controls": menuId,
                "aria-label": "Move ".concat(session.title, " to another column"),
                "data-testid": "move-to-menu-trigger",
                onClick: (e)=>{
                    e.stopPropagation();
                    if (isOpen) {
                        close();
                    } else {
                        openMenu();
                    }
                },
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(move_right/* default */.A, {
                    size: 16,
                    "aria-hidden": "true"
                })
            }),
            isOpen && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                ref: menuRef,
                id: menuId,
                role: "menu",
                "aria-label": "Move ".concat(session.title, " to"),
                className: menu,
                style: {
                    top: menuPos.top,
                    left: menuPos.left
                },
                "data-testid": "move-to-menu",
                onClick: (e)=>e.stopPropagation(),
                onKeyDown: handleMenuKeyDown,
                children: targets.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: menuEmpty,
                    "data-testid": "move-to-menu-empty",
                    children: "No moves available"
                }) : targets.map((target)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        role: "menuitem",
                        className: menuItem,
                        onClick: (e)=>{
                            e.stopPropagation();
                            handleSelect(target);
                        },
                        children: columnLabel(target)
                    }, target))
            }), document.body)
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FBoardCard.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xOXdxeDYwIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLl8xOXdxeDYxIHsKICBvcGFjaXR5OiAwLjU7Cn0KLl8xOXdxeDYyIHsKICBmbGV4OiAxOwogIG1pbi13aWR0aDogMDsKfQouXzE5d3F4NjMgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7CiAgZmxleC1zaHJpbms6IDA7Cn0KLl8xOXdxeDY0IHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAganVzdGlmeS1jb250ZW50OiBjZW50ZXI7CiAgZmxleC1zaHJpbms6IDA7CiAgd2lkdGg6IDQ0cHg7CiAgaGVpZ2h0OiA0NHB4OwogIHBhZGRpbmc6IDA7CiAgYm9yZGVyOiBub25lOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejIpOwogIGN1cnNvcjogZ3JhYjsKICB0b3VjaC1hY3Rpb246IG5vbmU7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FBoardCard_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xOXdxeDYwIHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBmbGV4LXN0YXJ0OwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7Cn0KLl8xOXdxeDYxIHsKICBvcGFjaXR5OiAwLjU7Cn0KLl8xOXdxeDYyIHsKICBmbGV4OiAxOwogIG1pbi13aWR0aDogMDsKfQouXzE5d3F4NjMgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeSk7CiAgZmxleC1zaHJpbms6IDA7Cn0KLl8xOXdxeDY0IHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAganVzdGlmeS1jb250ZW50OiBjZW50ZXI7CiAgZmxleC1zaHJpbms6IDA7CiAgd2lkdGg6IDQ0cHg7CiAgaGVpZ2h0OiA0NHB4OwogIHBhZGRpbmc6IDA7CiAgYm9yZGVyOiBub25lOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejIpOwogIGN1cnNvcjogZ3JhYjsKICB0b3VjaC1hY3Rpb246IG5vbmU7Cn0_22_7D = __webpack_require__(64062);
;// ./src/components/sessions/BoardCard.css.ts


var cardBody = '_19wqx62';
var cardControls = '_19wqx63';
var cardDragging = '_19wqx61';
var cardWrapper = '_19wqx60';
var dragHandle = '_19wqx64';
;// ./src/components/sessions/BoardCard.tsx
/* __next_internal_client_entry_do_not_use__ BoardCard auto */ 








/**
 * Thin wrapper around SessionCard adding a drag-handle affordance and a MoveToMenu (non-drag
 * fallback) for the board view. The handle (not the whole card) is the dnd-kit drag source, so
 * normal card clicks/scrolling are unaffected.
 */ function BoardCard(param) {
    let { rowKey, onMoveToColumn, ...sessionCardProps } = param;
    const { session } = sessionCardProps;
    const { attributes, listeners, setNodeRef, transform, isDragging } = (0,core_esm/* useDraggable */.PM)({
        id: makeCompositeId(rowKey, session.id)
    });
    const currentColumn = getBoardColumnKey(session);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: setNodeRef,
        className: isDragging ? "".concat(cardWrapper, " ").concat(cardDragging) : cardWrapper,
        "data-testid": "board-card",
        "data-session-id": session.id,
        style: transform ? {
            transform: utilities_esm/* CSS */.Ks.Translate.toString(transform)
        } : undefined,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: cardControls,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        id: "board-card-drag-handle-".concat(session.id),
                        className: dragHandle,
                        "aria-label": "Drag ".concat(session.title, " to move"),
                        ...listeners,
                        ...attributes,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(grip_vertical/* default */.A, {
                            size: 16,
                            "aria-hidden": "true"
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(MoveToMenu, {
                        session: session,
                        currentColumn: currentColumn,
                        onMove: (toColumn)=>onMoveToColumn === null || onMoveToColumn === void 0 ? void 0 : onMoveToColumn(toColumn)
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: cardBody,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionCard, {
                    ...sessionCardProps
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FBoardColumn.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA52U3Y6bMBCF73mKuakUKjkypLvpem_6JivHNsRdY1v2kECqvHuFS1Igu0nVK6Q5o_k58-H1WyGbvjh4Cr8yAKmjN7xnUBnVvWaQvkTqoARqZxkIZ9rGDspRS9wz2JTU_82M-6DtOwM6RHZcvNfBtVYyOPCwIuSteFGnMpxknnQXpAoMCt9BdEbLRVY1ySKBS93GZaHNNuU0vCN7pes9Migo_ZJi2l5j9DU7Z-vLqsXHq3Kja0s0qiYyEMqiCkO45n7ZtTylrp5LqW19MxPNH0bGCuNuO4fomvtGTOYv0_wND7W2o9WVs0iiPqmbYXb5VT-Obiwy9ilDOOPCUiuShKpDgoHbWLnQMGi9V0HwqAbRKEQVSPRcJC_omj6p5gIEg2I2-mZuvbZGW0UeXOBnG1FXPRHOorI4lYYjjyCW5R8OLze_cHmfn5f8Pqnbz80p8_vO84fO6zlGdNmgn9_9WzLvausN4gDuoEJl3JH0DHiL7g6licBJ8adU3LU4XIRB6TuQPO7VksUmzTzmEVdVUSEDUv6L2c_zls-3HP-X0RPEI_ZGMdDIjRZXdhNXU2w--3PLD__Tc_ajUVJzWA3vzIjb9vm77_K0wXr-hl6fRsGNWBWUHo5AoISvt69CGhwgiuCMIdFyf5k1Ig84qOfs_Bs0G9yOpAUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FBoardColumn_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA52U3Y6bMBCF73mKuakUKjkypLvpem_6JivHNsRdY1v2kECqvHuFS1Igu0nVK6Q5o_k58_H1WyGbvjh4Cr8yAKmjN7xnUBnVvWaQvkTqoARqZxkIZ9rGDspRS9wz2JTU_82M_6DtOwM6RHZcvNfBtVYyOPCwIuSteFGnMpxknnQXpAoMCt9BdEbLRVY1ySKBS93GZaHNNuU0vCN7pes9Migo_ZJi2l5j9DU7Z_vLqsXHq3Kja0s0qiYyEMqiCkO45n7ZtTylrp5LqW19MxPNH0bGCuNuO4fomvtGTOYv0_wND7W2o9WVs0iiPqmbYXb5VT_Obiwy9ilDOOPCUiuShKpDgoHbWLnQMGi9V0HwqAbRKEQVSPRcJC_omj6p5gIEg2I2_mZuvbZGW0UeXOBnG1FXPRHOorI4lYYjjyCW5R8OLze_cHmfn5f8Pqnbz80p8_vO84fO6zlGdNmgn9_9WzLvausN4gDuoEJl3JH0DHiL7g6licBJ8adU3LU4XIRB6TuQPO7VksUmzTzmEVdVUSEDUv6L2c_zls_3HP_X0RPEI_ZGMdDIjRZXdhNXU2w_3PLD_Tc_ajUVJzWA3vzIjb9vm77_K0wXr_hl6fRsGNWBWUHo5AoISvt69CGhwgiuCMIdFyf5k1Ig84qOfs_Bs0G9yOpAUAAA_22_7D = __webpack_require__(60551);
;// ./src/components/sessions/BoardColumn.css.ts


var column = '_1dmy1vp0';
var columnCards = '_1dmy1vp4';
var columnCount = '_1dmy1vp3';
var columnDropOver = '_1dmy1vp5';
var columnHeader = '_1dmy1vp1';
var columnTitle = '_1dmy1vp2';
var emptyColumn = '_1dmy1vp6';
;// ./src/components/sessions/BoardColumn.tsx
/* __next_internal_client_entry_do_not_use__ BoardColumn auto */ 







// Rough card height used only to seed the virtualizer's initial range before
// measureElement reports real sizes — SessionCard's actual height varies with content.
const ESTIMATED_CARD_HEIGHT = 220;
/**
 * One board column: header with a count badge, plus a virtualized, independently-scrolling
 * card list. Each BoardColumn owns its own @tanstack/react-virtual instance — never a single
 * board-wide virtualizer — so columns scroll and window their DOM nodes independently.
 */ function BoardColumn(param) {
    let { column: col, sessions, rowKey, getCardProps } = param;
    // The scrollable card-list container below is the same DOM node useDroppable's setNodeRef
    // attaches to via mergeRefs, alongside the virtualizer's own scroll ref.
    const scrollRef = (0,react.useRef)(null);
    const { setNodeRef, isOver } = (0,core_esm/* useDroppable */.zM)({
        id: makeCompositeId(rowKey, col.key)
    });
    const virtualizer = (0,esm/* useVirtualizer */.Te)({
        count: sessions.length,
        getScrollElement: ()=>scrollRef.current,
        estimateSize: ()=>ESTIMATED_CARD_HEIGHT,
        overscan: 4,
        measureElement: (el)=>el.getBoundingClientRect().height
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: column,
        "aria-label": "".concat(col.label, " column"),
        "data-testid": "board-column-".concat(col.key),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: columnHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: columnTitle,
                        children: col.label
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: columnCount,
                        "aria-label": "".concat(sessions.length, " sessions"),
                        children: sessions.length
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                ref: mergeRefs(scrollRef, setNodeRef),
                className: isOver ? "".concat(columnCards, " ").concat(columnDropOver) : columnCards,
                role: "list",
                "aria-label": "".concat(col.label, " sessions"),
                "data-testid": "board-column-cards-".concat(col.key),
                children: sessions.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: emptyColumn,
                    "data-testid": "board-column-empty-".concat(col.key),
                    children: "No sessions"
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    style: {
                        height: virtualizer.getTotalSize(),
                        width: "100%",
                        position: "relative"
                    },
                    children: virtualizer.getVirtualItems().map((virtualItem)=>{
                        const session = sessions[virtualItem.index];
                        if (!session) return null;
                        return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            role: "listitem",
                            ref: virtualizer.measureElement,
                            "data-index": virtualItem.index,
                            style: {
                                position: "absolute",
                                top: 0,
                                left: 0,
                                width: "100%",
                                transform: "translateY(".concat(virtualItem.start, "px)")
                            },
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(BoardCard, {
                                session: session,
                                rowKey: rowKey,
                                ...getCardProps(session)
                            })
                        }, session.id);
                    })
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FBoardSwimlane.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xNXViNDZ6MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7Cn0KLl8xNXViNDZ6MSB7CiAgbWFyZ2luOiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwp9Ci5fMTV1YjQ2ejIgewogIGRpc3BsYXk6IGZsZXg7CiAgZ2FwOiB2YXIoLS1fMTllejJyejMxKTsKICBvdmVyZmxvdy14OiBhdXRvOwogIGFsaWduLWl0ZW1zOiBzdHJldGNoOwp9CkBtZWRpYSAobWF4LXdpZHRoOiA3NjhweCkgewogIC5fMTV1YjQ2ejIgewogICAgZ2FwOiB2YXIoLS1fMTllejJyejMwKTsKICAgIHNjcm9sbC1zbmFwLXR5cGU6IHggbWFuZGF0b3J5OwogIH0KfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FBoardSwimlane_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xNXViNDZ6MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnoyeik7Cn0KLl8xNXViNDZ6MSB7CiAgbWFyZ2luOiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2gpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwp9Ci5fMTV1YjQ2ejIgewogIGRpc3BsYXk6IGZsZXg7CiAgZ2FwOiB2YXIoLS1fMTllejJyejMxKTsKICBvdmVyZmxvdy14OiBhdXRvOwogIGFsaWduLWl0ZW1zOiBzdHJldGNoOwp9CkBtZWRpYSAobWF4LXdpZHRoOiA3NjhweCkgewogIC5fMTV1YjQ2ejIgewogICAgZ2FwOiB2YXIoLS1fMTllejJyejMwKTsKICAgIHNjcm9sbC1zbmFwLXR5cGU6IHggbWFuZGF0b3J5OwogIH0KfQ_22_7D = __webpack_require__(75397);
;// ./src/components/sessions/BoardSwimlane.css.ts


var swimlane = '_15ub46z0';
var swimlaneLabel = '_15ub46z1';
var swimlaneRow = '_15ub46z2';
;// ./src/components/sessions/BoardSwimlane.tsx
/* __next_internal_client_entry_do_not_use__ BoardSwimlane auto */ 



/**
 * One swimlane row: a row label plus one BoardColumn per BoardColumnKey. Reuses BoardColumn
 * unmodified -- it doesn't need to know it's inside a swimlane, since `rowKey` already scopes
 * its dnd-kit ids (`${rowKey}:${column.key}`) uniquely across every row in the DndContext.
 */ function BoardSwimlane(param) {
    let { rowKey, displayName, buckets, getCardProps } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: swimlane,
        "aria-label": "".concat(displayName, " swimlane"),
        "data-testid": "board-swimlane-".concat(rowKey),
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                className: swimlaneLabel,
                children: displayName
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: swimlaneRow,
                children: BOARD_COLUMNS.map((col)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(BoardColumn, {
                        column: col,
                        rowKey: rowKey,
                        sessions: buckets[col.key],
                        getCardProps: getCardProps
                    }, col.key))
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FBoardCompleteConfirmDialog.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VUTW-jMBC951f4slI5OMJO023cH1M59gDTBRvZJqGs-t8rDFJIIB_b254Azxu_mTdvWL_z3cbWZUr-rgiprceA1giSYQv6bUUIGg9BkLR_30v1J3e2MZoqW1onyEG6J0rf2Q467jqZ9CiNvi7lpyBZCW1_IEvMDcUAlRdEgQng-uOPxgfMPqmyJoAJ01BH0WhoBWFpGplrqTWa_JJww5K31ddqPTbBYhP3qtzFKvfWaXCCsLol3paoL1AfExR1UmPjZ-yvI6alvpDaHmeAKrlVPI_RSrb0iDoUgjzztI6Kjd9KluqJpekvQgl3UC3L2z-pRgdqGJ2yZVOZPpLL-o5iPCqWWROoxw5maB0pY_wImBdhhigiYlHodOzP5Wiig068m8i7mMVOlIslqctbCSnRAC3G-th6O2V6jkwz0ZakGeqd2TLqC0afaGmws2zenQm7vWLF4KTxtXRgwlXd2EMGhQcM-vuW_3iXLJnjvva37JAPdmic7_uqLQ4rPRFGFPYA7qFNfZ22uIjAc9FFZlXj6QE97kuIHLYJvT0E4Vd0HLZqhFGbZfF_x_tFPN388lC9LL2-DNuzoRpr4L8c38s_jI9tkvPUH0xnlHR5PN99hrhrvQYAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FBoardCompleteConfirmDialog_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VUTW_jMBC951f4slI5OMJO023cH1M59gDTBRvZJqGs_t8rDFJIIB_b254Azxu_mTdvWL_z3cbWZUr_rgiprceA1giSYQv6bUUIGg9BkLR_30v1J3e2MZoqW1onyEG6J0rf2Q467jqZ9CiNvi7lpyBZCW1_IEvMDcUAlRdEgQng_uOPxgfMPqmyJoAJ01BH0WhoBWFpGplrqTWa_JJww5K31ddqPTbBYhP3qtzFKvfWaXCCsLol3paoL1AfExR1UmPjZ_yvI6alvpDaHmeAKrlVPI_RSrb0iDoUgjzztI6Kjd9KluqJpekvQgl3UC3L2z_pRgdqGJ2yZVOZPpLL_o5iPCqWWROoxw5maB0pY_wImBdhhigiYlHodOzP5Wiig068m8i7mMVOlIslqctbCSnRAC3G_th6O2V6jkwz0ZakGeqd2TLqC0afaGmws2zenQm7vWLF4KTxtXRgwlXd2EMGhQcM_vuW_3iXLJnjvva37JAPdmic7_uqLQ4rPRFGFPYA7qFNfZ22uIjAc9FFZlXj6QE97kuIHLYJvT0E4Vd0HLZqhFGbZfF_x_tFPN388lC9LL2_DNuzoRpr4L8c38s_jI9tkvPUH0xnlHR5PN99hrhrvQYAAA_22_7D = __webpack_require__(40087);
;// ./src/components/sessions/BoardCompleteConfirmDialog.css.ts


var BoardCompleteConfirmDialog_css_actions = '_293opl4';
var BoardCompleteConfirmDialog_css_body = '_293opl3';
var BoardCompleteConfirmDialog_css_dangerButton = '_293opl6';
var dialog = '_293opl1';
var heading = '_293opl2';
var overlay = '_293opl0';
var secondaryButton = '_293opl5';
;// ./src/components/sessions/BoardCompleteConfirmDialog.tsx
/* __next_internal_client_entry_do_not_use__ BoardCompleteConfirmDialog auto */ 



const FOCUSABLE_SELECTOR = 'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';
/**
 * Gates a drop/move into the "Complete" board column (Task 3.1.0, AC12) — that move calls
 * StopByUser, which kills the session's tmux pane and can remove its worktree, and
 * legalBoardTransitions["complete"] = [] means there is no in-board drag-based undo. No
 * existing modal primitive in web-app/src/components/ui/ implements both a full Tab-cycling
 * focus trap and focus-return-to-trigger-on-close (confirmed via
 * `grep -rl "ConfirmDialog\|useConfirm" web-app/src/components/ui/` — none found; the closest
 * analog, BackwardSyncConfirmDialog, lives in components/settings/ for a different feature) —
 * this mirrors that component's pattern rather than partially reusing it.
 */ function BoardCompleteConfirmDialog(param) {
    let { sessionTitle, onConfirm, onCancel } = param;
    const headingId = (0,react.useId)();
    const dialogRef = (0,react.useRef)(null);
    const confirmButtonRef = (0,react.useRef)(null);
    const previouslyFocusedRef = (0,react.useRef)(typeof document !== "undefined" ? document.activeElement : null);
    (0,react.useEffect)(()=>{
        var _confirmButtonRef_current;
        (_confirmButtonRef_current = confirmButtonRef.current) === null || _confirmButtonRef_current === void 0 ? void 0 : _confirmButtonRef_current.focus();
        return ()=>{
            var _previouslyFocusedRef_current;
            (_previouslyFocusedRef_current = previouslyFocusedRef.current) === null || _previouslyFocusedRef_current === void 0 ? void 0 : _previouslyFocusedRef_current.focus();
        };
    }, []);
    (0,react.useEffect)(()=>{
        const handleKeyDown = (e)=>{
            if (e.key === "Escape") {
                e.preventDefault();
                onCancel();
                return;
            }
            if (e.key === "Tab" && dialogRef.current) {
                const focusable = Array.from(dialogRef.current.querySelectorAll(FOCUSABLE_SELECTOR));
                if (focusable.length === 0) return;
                const first = focusable[0];
                const last = focusable[focusable.length - 1];
                if (e.shiftKey && document.activeElement === first) {
                    e.preventDefault();
                    last.focus();
                } else if (!e.shiftKey && document.activeElement === last) {
                    e.preventDefault();
                    first.focus();
                }
            }
        };
        document.addEventListener("keydown", handleKeyDown);
        return ()=>document.removeEventListener("keydown", handleKeyDown);
    }, [
        onCancel
    ]);
    const content = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        "data-testid": "board-complete-confirm-overlay",
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: dialogRef,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": headingId,
            className: dialog,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                    id: headingId,
                    className: heading,
                    children: [
                        "Stop “",
                        sessionTitle,
                        "”?"
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: BoardCompleteConfirmDialog_css_body,
                    children: "This ends its session."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: BoardCompleteConfirmDialog_css_actions,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: secondaryButton,
                            onClick: onCancel,
                            "data-testid": "board-complete-confirm-cancel",
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            ref: confirmButtonRef,
                            type: "button",
                            className: BoardCompleteConfirmDialog_css_dangerButton,
                            onClick: onConfirm,
                            "data-testid": "board-complete-confirm-confirm",
                            children: "Confirm"
                        })
                    ]
                })
            ]
        })
    });
    if (typeof document === "undefined") return null;
    return /*#__PURE__*/ (0,react_dom.createPortal)(content, document.body);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionBoard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6WUz46bMBDG7zzFXFYKB6-Abf7Ue9nH2Fvl4CFYNbZlmwSo8u6VTYiyIaStegHbM_P5N58Nr9Wx0txl8CsB4MIZyXoKlcTuPYH4JlxYLL3QikKpZduoKUIhD8NGKFKjONSeQvaenJPXUTN_rHlghsKR2RUhP_LvOBR2eMvTENFHtJXUJ9JRYK3XYc0wzoU6LFQsUwAwKQ6KCI-No-C8RV_WN3TFv3b8F9z9_3Nf-d6e8J1sYAnPWaMlKo92gbcY0mdsWTqn_cL0LTJd4CGHoshMNzVxEtzXFPJNXLvWrGON0U6MhrK907L1GMqmklFkcuEynVylUAvOMZ5BKYUhhoUioRz61Tp7iS2dauGROMNKpKD0aM2VYXPHUIkOeSjba-91s3BIEitPYZ29hIm3TLlK24aOQ8k8fq7ItP9AhOLRlmyXPfG4GB54PJJYjpZYxkXrZkDb8dZo5YkTA87i-4tGR1zNePDsLkGmN3Zsox17Vv48WN0qfp-dj0illtrOYsWt0u4PSv2y0BB1PhrkgsGqYd10gbabnenSKPzlR_L4C8ziDs_u9Bh3pdVSEqeYIb43SKGDhinOvLZ9SDkn598lL0nMDAUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionBoard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6WUz46bMBDG7zzFXFYKB6_Abf7Ue9nH2Fvl4CFYNbZlmwSo8u6VTYiyIaStegHbM_P5N58Nr9Wx0txl8CsB4MIZyXoKlcTuPYH4JlxYLL3QikKpZduoKUIhD8NGKFKjONSeQvaenJPXUTN_rHlghsKR2RUhP_LvOBR2eMvTENFHtJXUJ9JRYK3XYc0wzoU6LFQsUwAwKQ6KCI_No_C8RV_WN3TFv3b8F9z9_3Nf_d6e8J1sYAnPWaMlKo92gbcY0mdsWTqn_cL0LTJd4CGHoshMNzVxEtzXFPJNXLvWrGON0U6MhrK907L1GMqmklFkcuEynVylUAvOMZ5BKYUhhoUioRz61Tp7iS2dauGROMNKpKD0aM2VYXPHUIkOeSjba_91s3BIEitPYZ29hIm3TLlK24aOQ8k8fq7ItP9AhOLRlmyXPfG4GB54PJJYjpZYxkXrZkDb8dZo5YkTA87i_4tGR1zNePDsLkGmN3Zsox17Vv48WN0qfp_dj0illtrOYsWt0u4PSv2y0BB1PhrkgsGqYd10gbabnenSKPzlR_L4C8ziDs_u9Bh3pdVSEqeYIb43SKGDhinOvLZ9SDkn598lL0nMDAUAAA_22_7D = __webpack_require__(25099);
;// ./src/components/sessions/SessionBoard.css.ts


var board = 'fvfods1';
var boardHeader = 'fvfods3';
var boardHeaderSearch = 'fvfods4';
var boardRows = 'fvfods2';
var SessionBoard_css_container = 'fvfods0';
var liveRegion = 'fvfods5';
var toastError = 'fvfods7 fvfods6';
var toastWarning = 'fvfods8 fvfods6';
;// ./src/components/sessions/SessionBoard.tsx
/* __next_internal_client_entry_do_not_use__ attemptColumnMove,SessionBoard auto */ 
// +feature: session-board-view






















// Sole swimlane row when GroupingStrategy.None is active -- kept as a named constant so the
// composite id scheme (`${rowKey}:${entityId}`) is uniform whether or not swimlanes render.
const DEFAULT_ROW_KEY = "__default__";
// Stable empty collections so useFilteredGroupedSessions's memoized pipeline doesn't
// recompute every render purely because a fresh `new Set()`/`new Map()` literal changed
// identity — the board has no delete-undo or cost-sort UI yet, so these stay empty.
const EMPTY_ID_SET = new Set();
const EMPTY_COST_MAP = new Map();
// No stale-session config UI on the board yet; matches SessionCard's own internal default.
const STALE_THRESHOLD_MINUTES = 30;
// ConnectRPC codes that indicate a transport-level failure (offline, timeout, server crash)
// rather than a business-rule rejection -- distinguishes DragOutcome's "network_error" from
// "rejected_by_server" per Task 3.2.1c.
const NETWORKISH_ERROR_CODES = [
    code/* Code */.C.Unavailable,
    code/* Code */.C.DeadlineExceeded,
    code/* Code */.C.Unknown,
    code/* Code */.C.Internal
];
const SessionBoard_GROUPING_STRATEGY_VALUES = Object.values(strategies/* GroupingStrategy */.j$);
// Same localStorage KEY NAMES SessionList.tsx's BASE_STORAGE_KEYS uses for these two fields
// (intentionally duplicated, not imported -- SessionList doesn't export them) so that, per
// Phase 5's "toggling List/Board preserves search/grouping, no reset" AC, both views read and
// write the exact same persisted value once the user has touched either one's control.
const BOARD_SEARCH_QUERY_KEY = "stapler-squad-search-query";
const BOARD_GROUPING_STRATEGY_KEY = "stapler-squad-grouping-strategy";
function buildBoardViewFields() {
    let prefix = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : "";
    return {
        searchQuery: {
            key: "".concat(prefix).concat(BOARD_SEARCH_QUERY_KEY),
            defaultValue: ""
        },
        groupingStrategy: {
            key: "".concat(prefix).concat(BOARD_GROUPING_STRATEGY_KEY),
            // Deliberately GroupingStrategy.None (flat board), NOT SessionList's own Category
            // default -- since the storage key is shared, this fallback only governs the very
            // first time a workspace visits *either* view before anyone has touched the selector.
            defaultValue: strategies/* GroupingStrategy */.j$.None,
            isValid: (v)=>SessionBoard_GROUPING_STRATEGY_VALUES.includes(v)
        }
    };
}
/**
 * Decides and executes one column-move attempt. Shared by both call sites (Task 4.1.1a):
 * `handleDragEnd` calls it directly (once per selected session for a multi-select fan-out,
 * Task 6.3.1c), and `attemptMoveViaMenu` wraps it for MoveToMenu -- so a drag and an
 * equivalent menu selection always converge on identical legality checks, confirmation
 * prompts, and DragOutcome results.
 */ async function attemptColumnMove(session, fromColumn, toColumn, deps) {
    var _deps_onOptimisticMove;
    // "Needs Review" -> "Running" is a special case: it resolves the pending approval rather
    // than writing a status directly (no SessionStatus corresponds to "leave Needs Review").
    if (fromColumn === "needs_review" && toColumn === "running") {
        const approved = await deps.approveNeedsReview(session);
        if (!approved) {
            return {
                type: "rejected_by_server",
                reason: "Failed to resolve the pending approval"
            };
        }
        return {
            type: "moved"
        };
    }
    if (!isLegalBoardDragForSession(session, fromColumn, toColumn)) {
        return {
            type: "rejected_illegal",
            from: fromColumn,
            to: toColumn
        };
    }
    // AC12: dropping into "Complete" calls StopByUser, which kills the session's tmux pane and
    // can remove its worktree -- legalBoardTransitions["complete"] has no outbound edges, so
    // there's no in-board undo. Require explicit confirmation before the mutation fires.
    if (toColumn === "complete") {
        const confirmed = await deps.confirmComplete(session);
        if (!confirmed) {
            return {
                type: "cancelled"
            };
        }
    }
    const status = statusForColumnMove(session, toColumn);
    (_deps_onOptimisticMove = deps.onOptimisticMove) === null || _deps_onOptimisticMove === void 0 ? void 0 : _deps_onOptimisticMove.call(deps, toColumn);
    let result;
    if (status === null) {
        if (toColumn === "running" && session.status === types_pb/* SessionStatus */.Ph.HIBERNATED) {
            result = await deps.resumeHibernatedSession(session.id);
        } else {
            // Unreachable via any drag that passed isLegalBoardDragForSession above -- every legal
            // column pair maps to a concrete status except hibernated->running, handled above.
            return {
                type: "rejected_illegal",
                from: fromColumn,
                to: toColumn
            };
        }
    } else {
        result = await deps.updateSession(session.id, {
            status
        });
    }
    if (!result) {
        const { error, errorCode } = deps.getSessionsErrorState();
        const message = error !== null && error !== void 0 ? error : "Failed to update session";
        const isNetworkish = errorCode !== undefined && NETWORKISH_ERROR_CODES.includes(errorCode);
        return isNetworkish ? {
            type: "network_error"
        } : {
            type: "rejected_by_server",
            reason: message
        };
    }
    return {
        type: "moved"
    };
}
/**
 * Kanban-style alternative to SessionList: the same sessions, filtered/sorted/grouped through
 * the shared pipeline, bucketed into the 4 board columns (crossed with swimlane rows when a
 * grouping strategy is active, Task 6.1.1a), with drag-and-drop (dnd-kit) mutating status via
 * `attemptColumnMove`.
 */ function SessionBoard(param) {
    let { sessions, onSessionClick, onSessionOpenInNewPane, onDeleteSession, onPauseSession, onResumeSession, onCloneSession, onNewWorkspaceSession, onRenameSession, onRestartSession, onUpdateTags, onCreateCheckpoint, onListCheckpoints, onForkFromCheckpoint, onSetRateLimitEnabled, onToggleAutonomousMode, onToggleAutoApprove, onSteerAutonomousSession, onClearConversationState, onHibernateSession, onResumeHibernatedSession, storageKeyPrefix } = param;
    const { updateSession, resumeHibernatedSession } = (0,useSessionService/* useSessionService */.D)();
    const { track } = (0,AnalyticsContext/* useAnalytics */.st)();
    const { approvals, approve } = (0,ApprovalsContext/* useApprovalsContext */.B)();
    // Search + swimlane grouping-strategy state (Task 6.1.1a, 6.2.1a). Persisted under the same
    // localStorage keys SessionList.tsx uses (namespaced by the same `storageKeyPrefix`), so
    // switching List<->Board carries the value over rather than resetting it (Phase 5 AC).
    const boardViewFields = (0,react.useMemo)(()=>buildBoardViewFields(storageKeyPrefix), [
        storageKeyPrefix
    ]);
    const { state: boardViewState, setters: boardViewSetters } = (0,usePersistedViewState/* usePersistedViewState */.P)(boardViewFields);
    const { searchQuery, groupingStrategy } = boardViewState;
    const { searchQuery: setSearchQuery, groupingStrategy: setGroupingStrategy } = boardViewSetters;
    const groupingActive = groupingStrategy !== strategies/* GroupingStrategy */.j$.None;
    const { filteredSessions, groupedSessions, filteredSessionIds } = useFilteredGroupedSessions({
        sessions,
        searchQuery,
        selectedStatus: "all",
        selectedCategory: "all",
        selectedTag: "all",
        hidePaused: false,
        showArchived: false,
        showHidden: false,
        filterNeedsApproval: false,
        pendingDeleteIds: EMPTY_ID_SET,
        sortField: "lastActivity",
        sortDir: "desc",
        costById: EMPTY_COST_MAP,
        groupingStrategy,
        staleThresholdMinutes: STALE_THRESHOLD_MINUTES,
        staleRecomputeTick: 0
    });
    const sessionById = (0,react.useMemo)(()=>{
        const map = new Map();
        for (const session of sessions)map.set(session.id, session);
        return map;
    }, [
        sessions
    ]);
    // --- Cross-column bulk-select state (Task 6.3.1a) --------------------------------------
    // Mirrors SessionList.tsx's selectMode/selectedSessions shape and semantics exactly
    // (including intersecting against filteredSessionIds so a selection survives filter/search
    // changes without silently including sessions the user can no longer see).
    const [selectMode, setSelectMode] = (0,react.useState)(false);
    const [selectedSessions, setSelectedSessions] = (0,react.useState)(new Set());
    const [isBulkTagEditing, setIsBulkTagEditing] = (0,react.useState)(false);
    const bulkTagEditorTriggerRef = (0,react.useRef)(null);
    const activeSelection = (0,react.useMemo)(()=>new Set([
            ...selectedSessions
        ].filter((id)=>filteredSessionIds.has(id))), [
        selectedSessions,
        filteredSessionIds
    ]);
    // Moved above their first use (handlePauseAll/handleResumeAll below) -- declared again
    // lower in the file previously, which is unreachable dead code now that it's hoisted here.
    const [liveMessage, setLiveMessage] = (0,react.useState)("");
    const announceLive = (0,react.useCallback)((message)=>setLiveMessage(message), []);
    const [toast, setToast] = (0,react.useState)(null);
    const toastIdRef = (0,react.useRef)(0);
    const showToast = (0,react.useCallback)((message, kind)=>{
        toastIdRef.current += 1;
        const id = toastIdRef.current;
        setToast({
            id,
            message,
            kind
        });
    }, []);
    const handleToggleSelectMode = (0,react.useCallback)(()=>{
        setSelectMode((prev)=>{
            if (prev) setSelectedSessions(new Set());
            return !prev;
        });
    }, []);
    const handleToggleSession = (0,react.useCallback)((sessionId)=>{
        setSelectMode(true);
        setSelectedSessions((prev)=>{
            const next = new Set(prev);
            if (next.has(sessionId)) {
                next.delete(sessionId);
            } else {
                next.add(sessionId);
            }
            return next;
        });
    }, []);
    const handleSelectAll = (0,react.useCallback)(()=>{
        setSelectedSessions(new Set(filteredSessions.map((s)=>s.id)));
    }, [
        filteredSessions
    ]);
    const handleClearSelection = (0,react.useCallback)(()=>{
        setSelectedSessions(new Set());
        setSelectMode(false);
    }, []);
    // Bulk Pause/Resume call updateSession/resumeHibernatedSession directly (the same RPCs
    // attemptColumnMove's drag path already uses for these exact transitions) rather than the
    // void-returning onPauseSession/onResumeSession props -- those props report no result at
    // all, which made real partial-failure tracking impossible even though AC8 explicitly
    // requires it. Promise.all + a per-session ok flag gives genuine per-session feedback.
    const handlePauseAll = (0,react.useCallback)(()=>{
        const ids = Array.from(activeSelection);
        if (ids.length === 0) return;
        track({
            name: "board_bulk_pause",
            category: "user_action",
            component: "SessionBoard",
            labels: {
                count: String(ids.length)
            }
        });
        setSelectedSessions(new Set());
        setSelectMode(false);
        void (async ()=>{
            const results = await Promise.all(ids.map(async (id)=>{
                var _sessionById_get;
                var _sessionById_get_title;
                const title = (_sessionById_get_title = (_sessionById_get = sessionById.get(id)) === null || _sessionById_get === void 0 ? void 0 : _sessionById_get.title) !== null && _sessionById_get_title !== void 0 ? _sessionById_get_title : id;
                const result = await updateSession(id, {
                    status: types_pb/* SessionStatus */.Ph.PAUSED
                });
                return {
                    title,
                    ok: result !== null
                };
            }));
            const failed = results.filter((r)=>!r.ok);
            if (failed.length === 0) {
                announceLive("".concat(results.length, " session").concat(results.length !== 1 ? "s" : "", " paused"));
            } else if (failed.length === results.length) {
                const message = "Couldn't pause ".concat(failed.length, " selected session").concat(failed.length !== 1 ? "s" : "", ": ").concat(failed.map((f)=>f.title).join(", "), ".");
                showToast(message, "error");
                announceLive(message);
            } else {
                const message = "Paused ".concat(results.length - failed.length, " of ").concat(results.length, " selected sessions — couldn't pause: ").concat(failed.map((f)=>f.title).join(", "), ".");
                showToast(message, "warning");
                announceLive(message);
            }
        })();
    }, [
        activeSelection,
        sessionById,
        updateSession,
        announceLive,
        showToast,
        track
    ]);
    const handleResumeAll = (0,react.useCallback)(()=>{
        const ids = Array.from(activeSelection);
        if (ids.length === 0) return;
        track({
            name: "board_bulk_resume",
            category: "user_action",
            component: "SessionBoard",
            labels: {
                count: String(ids.length)
            }
        });
        setSelectedSessions(new Set());
        setSelectMode(false);
        void (async ()=>{
            const results = await Promise.all(ids.map(async (id)=>{
                const session = sessionById.get(id);
                var _session_title;
                const title = (_session_title = session === null || session === void 0 ? void 0 : session.title) !== null && _session_title !== void 0 ? _session_title : id;
                if (!session) return {
                    title,
                    ok: false
                };
                const result = session.status === types_pb/* SessionStatus */.Ph.HIBERNATED ? await resumeHibernatedSession(id) : await updateSession(id, {
                    status: types_pb/* SessionStatus */.Ph.ACTIVE
                });
                return {
                    title,
                    ok: result !== null
                };
            }));
            const failed = results.filter((r)=>!r.ok);
            if (failed.length === 0) {
                announceLive("".concat(results.length, " session").concat(results.length !== 1 ? "s" : "", " resumed"));
            } else if (failed.length === results.length) {
                const message = "Couldn't resume ".concat(failed.length, " selected session").concat(failed.length !== 1 ? "s" : "", ": ").concat(failed.map((f)=>f.title).join(", "), ".");
                showToast(message, "error");
                announceLive(message);
            } else {
                const message = "Resumed ".concat(results.length - failed.length, " of ").concat(results.length, " selected sessions — couldn't resume: ").concat(failed.map((f)=>f.title).join(", "), ".");
                showToast(message, "warning");
                announceLive(message);
            }
        })();
    }, [
        activeSelection,
        sessionById,
        updateSession,
        resumeHibernatedSession,
        announceLive,
        showToast,
        track
    ]);
    const handleDeleteAll = (0,react.useCallback)(()=>{
        if (!onDeleteSession) return;
        activeSelection.forEach((id)=>{
            void onDeleteSession(id);
        });
        setSelectedSessions(new Set());
        setSelectMode(false);
    }, [
        onDeleteSession,
        activeSelection
    ]);
    const handleBulkAddTag = (0,react.useCallback)((triggerEl)=>{
        bulkTagEditorTriggerRef.current = triggerEl;
        setIsBulkTagEditing(true);
    }, []);
    const handleBulkTagSave = (0,react.useCallback)((newTags)=>{
        if (newTags.length > 0 && onUpdateTags) {
            activeSelection.forEach((id)=>{
                const session = sessionById.get(id);
                var _session_tags;
                const merged = Array.from(new Set([
                    ...(_session_tags = session === null || session === void 0 ? void 0 : session.tags) !== null && _session_tags !== void 0 ? _session_tags : [],
                    ...newTags
                ]));
                onUpdateTags(id, merged);
            });
        }
        setIsBulkTagEditing(false);
    }, [
        onUpdateTags,
        activeSelection,
        sessionById
    ]);
    // --- Drag lifecycle state -------------------------------------------------------------
    // inFlightDragSessionIds: session IDs currently mid-drag or mid-mutation -- suppresses
    // watchSessions-driven column reassignment for those IDs until the drag/mutation settles
    // or is cancelled (Story 3.2.2). A multi-select drag (Task 6.3.1c) adds every selected
    // session's ID, not just the one the pointer grabbed. dragStartColumnRef snapshots each
    // such session's column at pickup time so the frozen render has something to show.
    // optimisticColumnOverrides holds the *destination* column once a legal move starts
    // mutating, for the "moves immediately, pending visual state until the RPC resolves"
    // behavior (Story 3.1.1).
    const [inFlightDragSessionIds, setInFlightDragSessionIds] = (0,react.useState)(new Set());
    const dragStartColumnRef = (0,react.useRef)(new Map());
    const [optimisticColumnOverrides, setOptimisticColumnOverrides] = (0,react.useState)(new Map());
    // Set right after a "moved" outcome resolves (drag or MoveToMenu); a no-dependency effect
    // below re-checks it on every render and focuses the target once it's back in the DOM
    // (Task 4.2.1b -- focus must land on the card's new location, not fall back to <body>).
    const pendingFocusRef = (0,react.useRef)(null);
    // Only clears the in-flight/pickup-column tracking -- NOT the optimistic column override.
    // A "moved" outcome must keep its override in place after the RPC settles (see
    // clearOptimisticOverride below for why); only a rejected/cancelled/network-error outcome
    // should call clearOptimisticOverride to snap the card back to its real column.
    const releaseInFlight = (0,react.useCallback)((sessionId)=>{
        dragStartColumnRef.current.delete(sessionId);
        setInFlightDragSessionIds((prev)=>{
            if (!prev.has(sessionId)) return prev;
            const next = new Set(prev);
            next.delete(sessionId);
            return next;
        });
    }, []);
    const clearOptimisticOverride = (0,react.useCallback)((sessionId)=>{
        setOptimisticColumnOverrides((prev)=>{
            if (!prev.has(sessionId)) return prev;
            const next = new Map(prev);
            next.delete(sessionId);
            return next;
        });
    }, []);
    // Shared by both the drag path (handleDragStart) and the MoveToMenu path (handleMenuMove,
    // Task 4.1.1a) so a menu-triggered move freezes watchSessions-driven reassignment exactly
    // the same way a drag does.
    const beginInFlight = (0,react.useCallback)((sessionId, fromColumn)=>{
        dragStartColumnRef.current.set(sessionId, fromColumn);
        setInFlightDragSessionIds((prev)=>new Set(prev).add(sessionId));
    }, []);
    // Once the real `sessions` prop (via a subsequent watchSessions push) reports a session in
    // the column its optimistic override already predicted -- or the session is gone entirely --
    // the override is redundant and is dropped so a later, genuinely different server-driven
    // change isn't incorrectly pinned to the stale override. Deliberately does NOT key off
    // `filteredSessions` (which excludes sessions the current search/filter hides) -- an
    // optimistically-moved session that's about to be filtered out must still reconcile against
    // its real status via `sessionById`, not linger forever because it dropped out of the
    // filtered set.
    (0,react.useEffect)(()=>{
        setOptimisticColumnOverrides((prev)=>{
            if (prev.size === 0) return prev;
            let changed = false;
            const next = new Map(prev);
            for (const [sessionId, overrideColumn] of prev){
                const session = sessionById.get(sessionId);
                if (!session || getBoardColumnKey(session) === overrideColumn) {
                    next.delete(sessionId);
                    changed = true;
                }
            }
            return changed ? next : prev;
        });
    }, [
        sessionById
    ]);
    // Resolves each visible session's current column exactly once, so a session rendered in
    // multiple swimlane rows simultaneously (Tag grouping's multi-membership) always shows --
    // and drags from -- the same column in every row it appears in.
    const columnForSession = (0,react.useMemo)(()=>{
        const map = new Map();
        for (const session of filteredSessions){
            const override = optimisticColumnOverrides.get(session.id);
            let col;
            if (override) {
                col = override;
            } else if (inFlightDragSessionIds.has(session.id)) {
                var _dragStartColumnRef_current_get;
                col = (_dragStartColumnRef_current_get = dragStartColumnRef.current.get(session.id)) !== null && _dragStartColumnRef_current_get !== void 0 ? _dragStartColumnRef_current_get : getBoardColumnKey(session);
            } else {
                col = getBoardColumnKey(session);
            }
            map.set(session.id, col);
        }
        return map;
    }, [
        filteredSessions,
        optimisticColumnOverrides,
        inFlightDragSessionIds
    ]);
    const bucketRow = (0,react.useCallback)((sessionsForRow)=>{
        const map = {
            running: [],
            needs_review: [],
            paused: [],
            complete: []
        };
        for (const session of sessionsForRow){
            var _columnForSession_get;
            const col = (_columnForSession_get = columnForSession.get(session.id)) !== null && _columnForSession_get !== void 0 ? _columnForSession_get : getBoardColumnKey(session);
            map[col].push(session);
        }
        return map;
    }, [
        columnForSession
    ]);
    // Flat (non-swimlane) bucketing -- used when groupingStrategy === GroupingStrategy.None.
    const buckets = (0,react.useMemo)(()=>bucketRow(filteredSessions), [
        bucketRow,
        filteredSessions
    ]);
    // --- Toast / announcement surface -----------------------------------------------------
    // (toast/showToast/toastIdRef are declared earlier in this component, before their first
    // use in handlePauseAll/handleResumeAll.)
    (0,react.useEffect)(()=>{
        if (!toast) return;
        const timer = setTimeout(()=>{
            setToast((current)=>(current === null || current === void 0 ? void 0 : current.id) === toast.id ? null : current);
        }, 4000);
        return ()=>clearTimeout(timer);
    }, [
        toast
    ]);
    // --- Screen-reader-only live region (Task 4.2.1a) --------------------------------------
    // Kept independent of `toast` (the visible bubble, error/warning outcomes only) -- a
    // "moved" outcome has no visible toast (the card's new column position is the sighted-user
    // feedback) but still needs its own distinct announcement for screen-reader users.
    // (liveMessage/announceLive are declared earlier in this component, before their first use
    // in handlePauseAll/handleResumeAll.)
    const columnLabel = (0,react.useCallback)((key)=>{
        var _BOARD_COLUMNS_find;
        var _BOARD_COLUMNS_find_label, _ref;
        return (_ref = (_BOARD_COLUMNS_find_label = (_BOARD_COLUMNS_find = BOARD_COLUMNS.find((c)=>c.key === key)) === null || _BOARD_COLUMNS_find === void 0 ? void 0 : _BOARD_COLUMNS_find.label) !== null && _BOARD_COLUMNS_find_label !== void 0 ? _BOARD_COLUMNS_find_label : key) !== null && _ref !== void 0 ? _ref : "";
    }, []);
    const announceOutcome = (0,react.useCallback)((outcome, session, toColumn)=>{
        switch(outcome.type){
            case "moved":
                var _session_title;
                announceLive("".concat((_session_title = session === null || session === void 0 ? void 0 : session.title) !== null && _session_title !== void 0 ? _session_title : "Session", " moved to ").concat(columnLabel(toColumn), "."));
                return;
            case "rejected_illegal":
                {
                    const fromLabel = columnLabel(outcome.from);
                    const toLabel = columnLabel(outcome.to);
                    const message = "Can't move a ".concat(fromLabel, " session to ").concat(toLabel, ".");
                    showToast(message, "error");
                    announceLive(message);
                    return;
                }
            case "rejected_by_server":
                {
                    var _session_title1;
                    const message = "Couldn't move \"".concat((_session_title1 = session === null || session === void 0 ? void 0 : session.title) !== null && _session_title1 !== void 0 ? _session_title1 : "session", '": ').concat(outcome.reason);
                    showToast(message, "error");
                    announceLive(message);
                    return;
                }
            case "network_error":
                {
                    var _session_title2;
                    const message = "Network error — couldn't move \"".concat((_session_title2 = session === null || session === void 0 ? void 0 : session.title) !== null && _session_title2 !== void 0 ? _session_title2 : "session", '". Check your connection and try again.');
                    showToast(message, "warning");
                    announceLive(message);
                    return;
                }
            case "cancelled":
                announceLive("Move cancelled.");
                return;
        }
    }, [
        showToast,
        announceLive,
        columnLabel
    ]);
    // Task 6.3.1c/d: combined outcome announcement for a multi-select drag fan-out -- reports
    // which sessions (if any) failed to move, rather than only the single dragged session's
    // outcome, per AC8's "surface which failed" partial-bulk-failure requirement.
    const announceMultiOutcome = (0,react.useCallback)((results, toColumn)=>{
        if (results.length === 0) return;
        const succeeded = results.filter((r)=>r.outcome.type === "moved");
        const failed = results.filter((r)=>r.outcome.type !== "moved");
        const toLabel = columnLabel(toColumn);
        if (failed.length === 0) {
            announceLive("".concat(succeeded.length, " session").concat(succeeded.length !== 1 ? "s" : "", " moved to ").concat(toLabel, "."));
            return;
        }
        const failedTitles = failed.map((f)=>f.session.title).join(", ");
        if (succeeded.length === 0) {
            const message = "Couldn't move ".concat(failed.length, " selected session").concat(failed.length !== 1 ? "s" : "", " to ").concat(toLabel, ": ").concat(failedTitles, ".");
            showToast(message, "error");
            announceLive(message);
            return;
        }
        const message = "Moved ".concat(succeeded.length, " of ").concat(results.length, " selected sessions to ").concat(toLabel, " — couldn't move: ").concat(failedTitles, ".");
        showToast(message, "warning");
        announceLive(message);
    }, [
        columnLabel,
        showToast,
        announceLive
    ]);
    // --- Complete-column confirmation (Task 3.1.0 / AC12) ----------------------------------
    const [pendingComplete, setPendingComplete] = (0,react.useState)(null);
    const confirmComplete = (0,react.useCallback)((session)=>{
        return new Promise((resolve)=>{
            setPendingComplete({
                session,
                resolve
            });
        });
    }, []);
    const handleCompleteConfirm = (0,react.useCallback)(()=>{
        pendingComplete === null || pendingComplete === void 0 ? void 0 : pendingComplete.resolve(true);
        setPendingComplete(null);
    }, [
        pendingComplete
    ]);
    const handleCompleteCancel = (0,react.useCallback)(()=>{
        pendingComplete === null || pendingComplete === void 0 ? void 0 : pendingComplete.resolve(false);
        setPendingComplete(null);
    }, [
        pendingComplete
    ]);
    // --- Needs Review resolution (Story 3.3.1) ----------------------------------------------
    const approveNeedsReview = (0,react.useCallback)(async (session)=>{
        const approval = approvals.find((a)=>a.sessionId === session.id);
        if (!approval) return false;
        try {
            await approve(approval.id);
            return true;
        } catch (e) {
            return false;
        }
    }, [
        approvals,
        approve
    ]);
    const getSessionsErrorState = (0,react.useCallback)(()=>{
        const state = store_store/* store */.M_.getState().sessions;
        return {
            error: state.error,
            errorCode: state.errorCode
        };
    }, []);
    // --- dnd-kit wiring ----------------------------------------------------------------------
    const sensors = (0,core_esm/* useSensors */.FR)((0,core_esm/* useSensor */.MS)(core_esm/* PointerSensor */.AN), (0,core_esm/* useSensor */.MS)(core_esm/* TouchSensor */.IG, {
        activationConstraint: {
            delay: 200,
            tolerance: 8
        }
    }));
    const handleDragStart = (0,react.useCallback)((event)=>{
        const { entityId: sessionId } = parseCompositeId(String(event.active.id));
        const session = sessionById.get(sessionId);
        if (!session) return;
        // Multi-select fan-out (Task 6.3.1c): if the picked-up card is part of the current
        // selection AND that selection has more than one member, the whole selection moves --
        // freeze watchSessions reassignment for every selected ID up front, not just the one
        // the pointer grabbed.
        const isMultiSelectDrag = activeSelection.has(sessionId) && activeSelection.size > 1;
        if (isMultiSelectDrag) {
            activeSelection.forEach((id)=>{
                const s = sessionById.get(id);
                if (s) beginInFlight(id, getBoardColumnKey(s));
            });
            announceLive("".concat(activeSelection.size, " sessions picked up."));
        } else {
            beginInFlight(sessionId, getBoardColumnKey(session));
            announceLive("".concat(session.title, " picked up."));
        }
    }, [
        sessionById,
        activeSelection,
        announceLive,
        beginInFlight
    ]);
    const handleDragCancel = (0,react.useCallback)((event)=>{
        const { entityId: sessionId } = parseCompositeId(String(event.active.id));
        const isMultiSelectDrag = activeSelection.has(sessionId) && activeSelection.size > 1;
        const idsToRelease = isMultiSelectDrag ? Array.from(activeSelection) : [
            sessionId
        ];
        idsToRelease.forEach((id)=>releaseInFlight(id));
        announceOutcome({
            type: "cancelled"
        });
    }, [
        activeSelection,
        releaseInFlight,
        announceOutcome
    ]);
    const handleDragEnd = (0,react.useCallback)(async (event)=>{
        const { entityId: sessionId } = parseCompositeId(String(event.active.id));
        const session = sessionById.get(sessionId);
        const fromColumn = dragStartColumnRef.current.get(sessionId);
        if (!session || !fromColumn) {
            releaseInFlight(sessionId);
            return;
        }
        const isMultiSelectDrag = activeSelection.has(sessionId) && activeSelection.size > 1;
        const idsToMove = isMultiSelectDrag ? Array.from(activeSelection) : [
            sessionId
        ];
        if (!event.over) {
            idsToMove.forEach((id)=>releaseInFlight(id));
            announceOutcome({
                type: "cancelled"
            });
            return;
        }
        const { entityId: toColumnId } = parseCompositeId(String(event.over.id));
        const toColumn = toColumnId;
        if (!isMultiSelectDrag) {
            if (fromColumn === toColumn) {
                releaseInFlight(sessionId);
                return;
            }
            const outcome = await attemptColumnMove(session, fromColumn, toColumn, {
                updateSession,
                resumeHibernatedSession,
                approveNeedsReview,
                confirmComplete,
                getSessionsErrorState,
                onOptimisticMove: (target)=>{
                    setOptimisticColumnOverrides((prev)=>new Map(prev).set(sessionId, target));
                }
            });
            releaseInFlight(sessionId);
            if (outcome.type === "moved") {
                // Keep the optimistic override in place -- clearing it here would fall back to the
                // still-stale `sessions` prop (the real watchSessions push hasn't landed yet) and
                // bounce the card back to its old column for one render, which also yanks focus off
                // the element the effect below is about to target. clearOptimisticOverride's own
                // reconciliation effect drops the override once real props catch up.
                pendingFocusRef.current = {
                    sessionId,
                    via: "drag"
                };
            } else {
                clearOptimisticOverride(sessionId);
            }
            announceOutcome(outcome, session, toColumn);
            return;
        }
        // Multi-select fan-out (Task 6.3.1c): one attemptColumnMove call per selected session,
        // targeting the same toColumn. Sequential (not Promise.all) is deliberate -- a drop into
        // "Complete" opens a single-slot confirmation dialog (confirmComplete/pendingComplete)
        // per session, and awaiting each call in turn keeps those prompts from racing each other.
        const results = [];
        for (const id of idsToMove){
            const s = sessionById.get(id);
            const from = dragStartColumnRef.current.get(id);
            if (!s || !from) {
                releaseInFlight(id);
                continue;
            }
            if (from === toColumn) {
                releaseInFlight(id);
                continue;
            }
            const outcome = await attemptColumnMove(s, from, toColumn, {
                updateSession,
                resumeHibernatedSession,
                approveNeedsReview,
                confirmComplete,
                getSessionsErrorState,
                onOptimisticMove: (target)=>{
                    setOptimisticColumnOverrides((prev)=>new Map(prev).set(id, target));
                }
            });
            releaseInFlight(id);
            if (outcome.type !== "moved") {
                clearOptimisticOverride(id);
            }
            results.push({
                id,
                session: s,
                outcome
            });
        }
        const draggedResult = results.find((r)=>r.id === sessionId);
        if ((draggedResult === null || draggedResult === void 0 ? void 0 : draggedResult.outcome.type) === "moved") {
            pendingFocusRef.current = {
                sessionId,
                via: "drag"
            };
        }
        announceMultiOutcome(results, toColumn);
    }, [
        sessionById,
        activeSelection,
        releaseInFlight,
        clearOptimisticOverride,
        announceOutcome,
        announceMultiOutcome,
        updateSession,
        resumeHibernatedSession,
        approveNeedsReview,
        confirmComplete,
        getSessionsErrorState
    ]);
    // MoveToMenu's non-drag counterpart to handleDragEnd: same attemptColumnMove call, same
    // in-flight freeze (Story 3.2.2) so a mid-request watchSessions push can't flicker the card
    // back before the RPC settles, same outcome announcement/focus handling -- only the trigger
    // differs (a discrete menu selection instead of a dnd-kit drop event). MoveToMenu always
    // targets a single session -- it has no concept of "act on the current selection".
    const attemptMoveViaMenu = (0,react.useCallback)(async (session, fromColumn, toColumn)=>{
        const sessionId = session.id;
        beginInFlight(sessionId, fromColumn);
        const outcome = await attemptColumnMove(session, fromColumn, toColumn, {
            updateSession,
            resumeHibernatedSession,
            approveNeedsReview,
            confirmComplete,
            getSessionsErrorState,
            onOptimisticMove: (target)=>{
                setOptimisticColumnOverrides((prev)=>new Map(prev).set(sessionId, target));
            }
        });
        releaseInFlight(sessionId);
        if (outcome.type === "moved") {
            pendingFocusRef.current = {
                sessionId,
                via: "menu"
            };
        } else {
            clearOptimisticOverride(sessionId);
        }
        announceOutcome(outcome, session, toColumn);
        return outcome;
    }, [
        beginInFlight,
        releaseInFlight,
        clearOptimisticOverride,
        announceOutcome,
        updateSession,
        resumeHibernatedSession,
        approveNeedsReview,
        confirmComplete,
        getSessionsErrorState
    ]);
    // Task 4.2.1b: after a successful move, focus the card's control (drag handle or menu
    // trigger) in its new column rather than letting focus fall back to <body>. No dependency
    // array -- runs after every render, but only acts (and only once) while a focus target is
    // pending, so the cost on unrelated renders is a no-op ref check.
    (0,react.useEffect)(()=>{
        const pending = pendingFocusRef.current;
        if (!pending) return;
        const elementId = pending.via === "menu" ? "board-card-move-trigger-".concat(pending.sessionId) : "board-card-drag-handle-".concat(pending.sessionId);
        const el = document.getElementById(elementId);
        if (el) {
            el.focus();
            pendingFocusRef.current = null;
        }
    });
    // Builds the SessionCard-shaped callback props for one session — SessionBoard receives the
    // id/session-keyed handler surface (matching SessionListProps) and adapts it per card here,
    // the same translation SessionList's own card-mode itemContent performs.
    const getCardProps = (0,react.useCallback)((session)=>({
            onClick: onSessionClick ? ()=>onSessionClick(session) : undefined,
            onOpenInNewPane: onSessionOpenInNewPane ? ()=>onSessionOpenInNewPane(session) : undefined,
            onDelete: onDeleteSession ? ()=>onDeleteSession(session.id) : undefined,
            onPause: onPauseSession ? ()=>onPauseSession(session.id) : undefined,
            onResume: onResumeSession ? ()=>onResumeSession(session) : undefined,
            onClone: onCloneSession ? ()=>onCloneSession(session.id) : undefined,
            onNewWorkspace: onNewWorkspaceSession ? ()=>onNewWorkspaceSession(session.id) : undefined,
            onRename: onRenameSession,
            onRestart: onRestartSession,
            onUpdateTags: onUpdateTags,
            onCreateCheckpoint: onCreateCheckpoint,
            onListCheckpoints: onListCheckpoints,
            onForkFromCheckpoint: onForkFromCheckpoint,
            onSetRateLimitEnabled: onSetRateLimitEnabled,
            onToggleAutonomousMode: onToggleAutonomousMode,
            onToggleAutoApprove: onToggleAutoApprove,
            onSteerAutonomousSession: onSteerAutonomousSession,
            onClearConversationState: onClearConversationState,
            onHibernate: onHibernateSession ? ()=>onHibernateSession(session.id) : undefined,
            onResumeFromHibernation: onResumeHibernatedSession ? ()=>onResumeHibernatedSession(session.id) : undefined,
            onMoveToColumn: (toColumn)=>{
                void attemptMoveViaMenu(session, getBoardColumnKey(session), toColumn);
            },
            selectMode,
            isSelected: activeSelection.has(session.id),
            onToggleSelect: ()=>handleToggleSession(session.id)
        }), [
        onSessionClick,
        onSessionOpenInNewPane,
        onDeleteSession,
        onPauseSession,
        onResumeSession,
        onCloneSession,
        onNewWorkspaceSession,
        onRenameSession,
        onRestartSession,
        onUpdateTags,
        onCreateCheckpoint,
        onListCheckpoints,
        onForkFromCheckpoint,
        onSetRateLimitEnabled,
        onToggleAutonomousMode,
        onToggleAutoApprove,
        onSteerAutonomousSession,
        onClearConversationState,
        attemptMoveViaMenu,
        onHibernateSession,
        onResumeHibernatedSession,
        selectMode,
        activeSelection,
        handleToggleSession
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: SessionBoard_css_container,
        "data-context": "session-board",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: boardHeader,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "text",
                        placeholder: "Search sessions...",
                        value: searchQuery,
                        onChange: (e)=>setSearchQuery(e.target.value),
                        className: "".concat(searchInput, " ").concat(boardHeaderSearch),
                        "aria-label": "Search sessions",
                        "data-testid": "board-search-input"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                        value: groupingStrategy,
                        onChange: (e)=>setGroupingStrategy(e.target.value),
                        className: SessionList_css_select,
                        title: "Group by",
                        "aria-label": "Group sessions by",
                        "data-testid": "board-grouping-select",
                        children: Object.entries(strategies/* GroupingStrategyLabels */.YK).map((param)=>{
                            let [value, label] = param;
                            return /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                value: value,
                                children: label
                            }, value);
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        onClick: handleToggleSelectMode,
                        className: "".concat(selectModeButton, " ").concat(selectMode ? selectModeButtonActive : ""),
                        "aria-label": selectMode ? "Exit select mode" : "Enter select mode",
                        "aria-pressed": selectMode,
                        "data-testid": "board-select-mode-toggle",
                        children: selectMode ? "Cancel" : "Select"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(core_esm/* DndContext */.Mp, {
                sensors: sensors,
                onDragStart: handleDragStart,
                onDragEnd: handleDragEnd,
                onDragCancel: handleDragCancel,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: groupingActive ? boardRows : board,
                    role: "region",
                    "aria-label": "Session board",
                    children: groupingActive ? groupedSessions.map((group)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(BoardSwimlane, {
                            rowKey: group.groupKey,
                            displayName: group.displayName,
                            buckets: bucketRow(group.sessions),
                            getCardProps: getCardProps
                        }, group.groupKey)) : BOARD_COLUMNS.map((col)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(BoardColumn, {
                            column: col,
                            rowKey: DEFAULT_ROW_KEY,
                            sessions: buckets[col.key],
                            getCardProps: getCardProps
                        }, col.key))
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                className: liveRegion,
                "data-testid": "board-live-region",
                children: liveMessage
            }),
            toast && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: toast.kind === "warning" ? toastWarning : toastError,
                "data-testid": "board-toast",
                children: toast.message
            }),
            pendingComplete && /*#__PURE__*/ (0,jsx_runtime.jsx)(BoardCompleteConfirmDialog, {
                sessionTitle: pendingComplete.session.title,
                onConfirm: handleCompleteConfirm,
                onCancel: handleCompleteCancel
            }),
            selectMode && /*#__PURE__*/ (0,jsx_runtime.jsx)(BulkActions, {
                selectedCount: activeSelection.size,
                totalCount: filteredSessions.length,
                onPauseAll: handlePauseAll,
                onResumeAll: handleResumeAll,
                onDeleteAll: handleDeleteAll,
                onAddTagAll: (e)=>handleBulkAddTag(e.currentTarget),
                onSelectAll: handleSelectAll,
                onClearSelection: handleClearSelection
            }),
            isBulkTagEditing && /*#__PURE__*/ (0,jsx_runtime.jsx)(TagEditor/* TagEditor */.y, {
                tags: [],
                onSave: handleBulkTagSave,
                onCancel: ()=>setIsBulkTagEditing(false),
                sessionTitle: "".concat(activeSelection.size, " selected session").concat(activeSelection.size !== 1 ? "s" : ""),
                triggerRef: bulkTagEditorTriggerRef
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/ui/ErrorState.tsx + 1 modules
var ErrorState = __webpack_require__(35007);
;// ./src/lib/contexts/SessionViewModeContext.tsx
/* __next_internal_client_entry_do_not_use__ useSessionViewModeContext,SessionViewModeProvider auto */ 
const SessionViewModeContext = /*#__PURE__*/ (0,react.createContext)(null);
function useSessionViewModeContext() {
    const ctx = (0,react.useContext)(SessionViewModeContext);
    if (!ctx) throw new Error("useSessionViewModeContext must be inside SessionViewModeProvider");
    return ctx;
}
const SessionViewModeProvider = SessionViewModeContext.Provider;

;// ./src/lib/contexts/CockpitActionsContext.ts
/* __next_internal_client_entry_do_not_use__ useCockpitActions,CockpitActionsProvider auto */ 
const CockpitActionsContext = (0,react.createContext)(null);
function useCockpitActions() {
    const ctx = (0,react.useContext)(CockpitActionsContext);
    if (!ctx) throw new Error("useCockpitActions must be inside CockpitActionsProvider");
    return ctx;
}
const CockpitActionsProvider = CockpitActionsContext.Provider;

;// ./src/components/pane/PaneContext.ts
/* __next_internal_client_entry_do_not_use__ PaneContext,usePaneContext auto */ 
const PaneContext = (0,react.createContext)(null);
function usePaneContext() {
    const ctx = (0,react.useContext)(PaneContext);
    if (!ctx) throw new Error("usePaneContext must be used inside PaneContext.Provider");
    return ctx;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/lucide-react@1.14.0+55f3e2d4ca346cd1/node_modules/lucide-react/dist/esm/icons/layout-dashboard.mjs
var layout_dashboard = __webpack_require__(19251);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fpane%2FpaneHeader.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA5VT0W6jMBB8z1f4sUh1hUmvvXM-pnLxAns1tmsvTUjVf68MCeGg0bUvIM8su-PZ4e6196-_RM7eN4y1aHkDWDck2bbwh92GMY3RG9VLVhkYgPTm-6C8ZOmZIGWwthwJ2ihZCZYgJLhONW8q3HD-JP7AsQjH4pglxiut0dYrts--rn9W5UsdXGf18pPHkXdBQ-DPjsi1kgl_YNEZ1ItiyKYLxCagfZEsHxBniVeqRdOvJHXZVBHxCEt-qwa-ixB4BAMlSWadhQSWXYguSKahUp2h3eZjczfaLQa7kxDJxO7k_B41NSdF7g1CZdxesga1BpswggPxCwHGoI8YE7VvkIBHr0pI4897KZ1J8_9VLL5xoUloMQhdZeDHC196Pg3Y_mjA3y4SVj0vnSWwNKdO7hX5GNtzjM_nKXH5Mk8UlI1eBbB0idJliadoBaWxiyuzHrKrPhfZPAPe4Vnp_7K0zucgEQmdlTPpTOR5G2_H6eNhbqxsUlYGe6__Pr-v68_nObj_VjchrrcTQ79PxSPOJ28EAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fpane_2FpaneHeader_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA5VT0W6jMBB8z1f4sUh1hUmvvXM_pnLxAns1tmsvTUjVf68MCeGg0bUvIM8su_PZ4e6196_RM7eN4y1aHkDWDck2bbwh92GMY3RG9VLVhkYgPTm_6C8ZOmZIGWwthwJ2ihZCZYgJLhONW8q3HD_JP7AsQjH4pglxiut0dYrts_rn9W5UsdXGf18pPHkXdBQ_DPjsi1kgl_YNEZ1ItiyKYLxCagfZEsHxBniVeqRdOvJHXZVBHxCEt_qwa_ixB4BAMlSWadhQSWXYguSKahUp2h3eZjczfaLQa7kxDJxO7k_B41NSdF7g1CZdxesga1BpswggPxCwHGoI8YE7VvkIBHr0pI4897KZ1J8_9VLL5xoUloMQhdZeDHC196Pg3Y_mjA3y4SVj0vnSWwNKdO7hX5GNtzjM_nKXH5Mk8UlI1eBbB0idJliadoBaWxiyuzHrKrPhfZPAPe4Vnp_7K0zucgEQmdlTPpTOR5G2_H6eNhbqxsUlYGe6_Pr_v68_nObj_VjchrrcTQ79PxSPOJ28EAAA_22_7D = __webpack_require__(43111);
;// ./src/styles/pane/paneHeader.css.ts


var paneCloseButton = 'qypq514 qypq513';
var paneHeader = 'qypq510';
var paneHeaderActions = 'qypq512';
var paneHeaderButton = 'qypq513';
var paneTitle = 'qypq511';
;// ./src/components/pane/PaneHeader.tsx
/* __next_internal_client_entry_do_not_use__ PaneHeader auto */ 






function PaneHeader(param) {
    let { pane, sessions, isFocused: _isFocused, onClose, onFocus, onZoom, onSetView, splitButtonVisible, onSplitVertical, onSplitHorizontal } = param;
    const cockpit = useCockpitActions();
    const { backlogIndex } = usePaneContext();
    const isListPane = pane.viewKind === "session-list";
    var _sessions_find;
    const session = !isListPane && pane.sessionId ? (_sessions_find = sessions.find((s)=>s.id === pane.sessionId)) !== null && _sessions_find !== void 0 ? _sessions_find : null : null;
    const titleText = isListPane ? "Sessions" : session ? session.title : "Empty";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: paneHeader,
        "data-testid": "pane-header-".concat(pane.id),
        onClick: onFocus,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: paneTitle,
                title: titleText,
                children: titleText
            }),
            session && /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogOriginBadge/* BacklogOriginBadge */.J, {
                entry: backlogIndex.get(session.id),
                compact: true
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: paneHeaderActions,
                children: [
                    !isListPane && session && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        onClick: (e)=>e.stopPropagation(),
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionActionsOverflow, {
                            session: session,
                            buttonClassName: paneHeaderButton,
                            onPause: ()=>cockpit.onPauseSession(session.id),
                            onResume: ()=>cockpit.onResumeSession(session),
                            onDelete: ()=>cockpit.onDeleteSession(session.id),
                            onRestart: (id)=>cockpit.onRestartSession(id),
                            onClone: ()=>cockpit.onCloneSession(session.id),
                            onNewWorkspace: ()=>cockpit.onNewWorkspaceSession(session.id),
                            onCreateCheckpoint: (id, label)=>cockpit.onCreateCheckpoint(id, label),
                            onSetRateLimitEnabled: (id, enabled)=>cockpit.onSetRateLimitEnabled(id, enabled),
                            onToggleAutonomousMode: (id, enabled)=>cockpit.onToggleAutonomousMode(id, enabled),
                            onTogglePinned: (id, pinned)=>cockpit.onTogglePinned(id, pinned),
                            onToggleAutoApprove: (id, enabled)=>cockpit.onToggleAutoApprove(id, enabled),
                            onSteerAutonomousSession: (id, msg)=>cockpit.onSteerAutonomousSession(id, msg),
                            onClearConversationState: (id)=>cockpit.onClearConversationState(id),
                            onUpdateTags: (id, tags)=>cockpit.onUpdateTags(id, tags)
                        })
                    }),
                    onSetView && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: paneHeaderButton,
                        "data-testid": "pane-view-toggle-btn",
                        onClick: (e)=>{
                            e.stopPropagation();
                            onSetView(isListPane ? "session-detail" : "session-list");
                        },
                        title: isListPane ? "Switch to session detail" : "Switch to session list",
                        "aria-label": isListPane ? "Switch to session detail" : "Switch to session list",
                        children: isListPane ? /*#__PURE__*/ (0,jsx_runtime.jsx)(layout_dashboard/* default */.A, {
                            size: 14
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(layout_list/* default */.A, {
                            size: 14
                        })
                    }),
                    splitButtonVisible && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: paneHeaderButton,
                                "data-testid": "pane-split-vertical-btn",
                                onClick: (e)=>{
                                    e.stopPropagation();
                                    onSplitVertical === null || onSplitVertical === void 0 ? void 0 : onSplitVertical();
                                },
                                title: "Split pane side by side (vertical)",
                                "aria-label": "Split pane side by side",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(columns_2/* default */.A, {
                                    size: 14
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: paneHeaderButton,
                                "data-testid": "pane-split-horizontal-btn",
                                onClick: (e)=>{
                                    e.stopPropagation();
                                    onSplitHorizontal === null || onSplitHorizontal === void 0 ? void 0 : onSplitHorizontal();
                                },
                                title: "Split pane top and bottom (horizontal)",
                                "aria-label": "Split pane top and bottom",
                                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(rows_2/* default */.A, {
                                    size: 14
                                })
                            })
                        ]
                    }),
                    !isListPane && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: paneHeaderButton,
                        onClick: (e)=>{
                            e.stopPropagation();
                            onZoom();
                        },
                        title: "Fullscreen pane",
                        "aria-label": "Fullscreen pane",
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(maximize_2/* default */.A, {
                            size: 14
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: paneCloseButton,
                        onClick: (e)=>{
                            e.stopPropagation();
                            onClose();
                        },
                        title: "Close pane",
                        "aria-label": "Close pane",
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(x/* default */.A, {
                            size: 14
                        })
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/styles/pane/resizeHandle.css.ts
var resizeHandle_css = __webpack_require__(84265);
;// ./src/components/pane/ResizeHandle.tsx
/* __next_internal_client_entry_do_not_use__ ResizeHandle auto */ 


const MIN_PX = 200;
function clampRatio(rawRatio, minPx, totalPx) {
    if (totalPx <= 0) return rawRatio;
    const minRatio = minPx / totalPx;
    return Math.max(minRatio, Math.min(1 - minRatio, rawRatio));
}
function ResizeHandle(param) {
    let { splitId, direction, onResize } = param;
    const handleRef = (0,react.useRef)(null);
    const draggingRef = (0,react.useRef)(false);
    const rafRef = (0,react.useRef)(null);
    const pendingRatioRef = (0,react.useRef)(null);
    function handlePointerDown(e) {
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        draggingRef.current = true;
    }
    function handlePointerMove(e) {
        var _handleRef_current;
        if (!draggingRef.current) return;
        const container = (_handleRef_current = handleRef.current) === null || _handleRef_current === void 0 ? void 0 : _handleRef_current.parentElement;
        if (!container) return;
        const rect = container.getBoundingClientRect();
        const rawRatio = direction === "vertical" ? (e.clientX - rect.left) / rect.width : (e.clientY - rect.top) / rect.height;
        const totalPx = direction === "vertical" ? rect.width : rect.height;
        pendingRatioRef.current = clampRatio(rawRatio, MIN_PX, totalPx);
        if (rafRef.current === null) {
            rafRef.current = requestAnimationFrame(()=>{
                if (pendingRatioRef.current !== null) {
                    onResize(splitId, pendingRatioRef.current);
                    pendingRatioRef.current = null;
                }
                rafRef.current = null;
            });
        }
    }
    function handlePointerUp(e) {
        draggingRef.current = false;
        e.currentTarget.releasePointerCapture(e.pointerId);
        if (rafRef.current !== null) {
            cancelAnimationFrame(rafRef.current);
            rafRef.current = null;
        }
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        ref: handleRef,
        "data-testid": "resize-handle",
        className: (0,resizeHandle_css/* resizeHandle */.D)({
            direction
        }),
        style: {
            touchAction: "none"
        },
        onPointerDown: handlePointerDown,
        onPointerMove: handlePointerMove,
        onPointerUp: handlePointerUp,
        onPointerCancel: handlePointerUp
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fpane%2FmobilePaneTabStrip.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8WTwW7bMAyG73kKHmcgKuykwzrlYQbFpm02siRQcpyk6LsPlpPGdW10hxU9ivwlkv9HPTwfDxnZFF5WAAV5p9VZQqnxtFsB7C0XyCJYJyFzJ_BWUwFHxT-E-JP9xsuGL5hEpcoPFdvWFHKS_xXz9ohcatuJkwTVBtvH-irC10zmICHtIzVSVQcJj6mL9ZWmyggK2HgJOZqA3Icr5aZVNudYxqmiIFNJSKf5S8z7nK3We8WioyLUEow1uFu9rh6uPkgpOtwfKIg36XtrJheyeeMWGn9ufaDyLHJrApowTt1G326G0a_93Y73waZuB1bGO8Vowh3Zp7gGsKwKav3Uy-2ALG_ZW5bgLN2aLK0JwtMFJWRPQ2O51b1o4nYyz7dRXJERGstwX4M4AAWyRo4GgyxNG78e3h8OY9tl3W9UNH959Z6SxQ7TZPTa5qsgLq7jNv3wa76J4-SKSt7ypWpInz-gbaOiqymg8E7l2P-JjpWbR_7PdNe3YeaJbz9h3Syz_jm2albRjLfhMVaa1WXvdP9lB_8Cr_LIM4EFAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fpane_2FmobilePaneTabStrip_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8WTwW7bMAyG73kKHmcgKuykwzrlYQbFpm02siRQcpyk6LsPlpPGdW10hxU9ivwlkv9HPTwfDxnZFF5WAAV5p9VZQqnxtFsB7C0XyCJYJyFzJ_BWUwFHxT_E_JP9xsuGL5hEpcoPFdvWFHKS_xXz9ohcatuJkwTVBtvH_irC10zmICHtIzVSVQcJj6mL9ZWmyggK2HgJOZqA3Icr5aZVNudYxqmiIFNJSKf5S8z7nK3We8WioyLUEow1uFu9rh6uPkgpOtwfKIg36XtrJheyeeMWGn9ufaDyLHJrApowTt1G326G0a_93Y73waZuB1bGO8Vowh3Zp7gGsKwKav3Uy_2ALG_ZW5bgLN2aLK0JwtMFJWRPQ2O51b1o4nYyz7dRXJERGstwX4M4AAWyRo4GgyxNG78e3h8OY9tl3W9UNH959Z6SxQ7TZPTa5qsgLq7jNv3wa76J4_SKSt7ypWpInz_gbaOiqymg8E7l2P_JjpWbR_7PdNe3YeaJbz9h3Syz_jm2albRjLfhMVaa1WXvdP9lB_8Cr_LIM4EFAAA_22_7D = __webpack_require__(77645);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+recipes@0.5.7+6974e6bd53375d7a/node_modules/@vanilla-extract/recipes/dist/createRuntimeFn-62c9670f.esm.js
var createRuntimeFn_62c9670f_esm = __webpack_require__(8462);
;// ./src/styles/pane/mobilePaneTabStrip.css.ts



var mobileAddPaneButton = 'jvk1io1';
var mobileTabButton = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'jvk1io2',variantClassNames:{active:{true:'jvk1io3',false:'jvk1io4'}},defaultVariants:{active:false},compoundVariants:[]});
var mobileTabStrip = 'jvk1io0';
;// ./src/components/pane/MobilePaneTabStrip.tsx
/* __next_internal_client_entry_do_not_use__ MobilePaneTabStrip auto */ 


const MobilePaneTabStrip = /*#__PURE__*/ (0,react.forwardRef)(function MobilePaneTabStrip(param, ref) {
    let { leaves, focusedPaneId, sessions, onFocus, onAddPane } = param;
    if (leaves.length <= 1) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: ref,
        className: mobileTabStrip,
        role: "tablist",
        "aria-label": "Pane switcher",
        children: [
            leaves.map((l)=>{
                var _sessions_find;
                const session = l.sessionId ? (_sessions_find = sessions.find((s)=>s.id === l.sessionId)) !== null && _sessions_find !== void 0 ? _sessions_find : null : null;
                const label = session ? session.title : l.viewKind === "session-list" ? "Sessions" : "Empty";
                const isActive = l.id === focusedPaneId;
                return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    role: "tab",
                    "aria-selected": isActive,
                    className: mobileTabButton({
                        active: isActive
                    }),
                    onClick: ()=>onFocus(l.id),
                    title: label,
                    children: label
                }, l.id);
            }),
            onAddPane && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                className: mobileAddPaneButton,
                onClick: onAddPane,
                title: "Add new pane",
                "aria-label": "Add new pane",
                children: "+"
            })
        ]
    });
});

// EXTERNAL MODULE: ./src/components/providers/ViewportProvider.tsx
var ViewportProvider = __webpack_require__(55765);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fpane%2FpaneSplit.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81WwXKbMBC9-yv20pnQiTJAGscml_5JR0aLvY2QqCQccCf_3kHYGGyRJumlJ0Zod9_btytp72yyqdVDDL8XAIJsJXmbwdaQeFoAvJBwuwySOP7SLXdI2507r_UeTSH1SwY7EgLV0-J1cdcHTHzALg5zWFaSO2S5lnWpbAY5l_nNnpsbxmwlyTHDHelbiO8eIvjqw0ewrBpICvN0FcboF3viMOClb-Gd-IbifIzLgHc_FayQ2HQI3ZcJMpg70iqDnkJYK4CSFDtKHJ_WJ43jkOCVttQHNii5oz12fzc8f94aXSuRQZ_Jj2SNh9QclpGHrp0khRmkVQNWSxLgDFe24gaVG1kwXRQWXQYsrXw63uyIeLLJtdQGkjQuLSC3OBLlmxclADdlVUYjn4c5nwnFwXzpzc868I3VsnZeB1KevFfuwEgJbDrp_i7REP1xvqoZJG8XmEvaKkYOy66rUDn0vfuzto6KluVaOVRuvLXl1SWb9OAr5jW-2vNbhVaOWddKzIAcl5Sf_9IBL53uN2evgpck26uwe29RcSFIba_8E7_rsHHMZzhOYF7Vx7GqK6_qSMS5szDp_XefrQFnPYfD2gx47XQAZ3Dm4dJflc-zQCUmmnVde6FqGw2c7c6Qej6mNdYs3OKbPo3ZivLofdizSAAbbQSaDJKZE4rR2YoZLqi2VzT6uyWvje1atdJ06orZ5h0yzD-hdeiwtG91btpGgT9xqCoDMfHP0h_-T-mnfZ-uuut9yBp91vOHuZy_k36N2QYtJpf99qOv5scujQFn96kbJ_S6vi6-lyiIw03Jm9Nj_bhcVU3kMSbjzvwA0s8x4QnkavhghbntXCYz0GuYymq9vqCSBqkch6bCXMT8A9OyqXwACgAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fpane_2FpaneSplit_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81WwXKbMBC9_yv20pnQiTJAGscml_5JR0aLvY2QqCQccCf_3kHYGGyRJumlJ0Zod9_btytp72yyqdVDDL8XAIJsJXmbwdaQeFoAvJBwuwySOP7SLXdI2507r_UeTSH1SwY7EgLV0_J1cdcHTHzALg5zWFaSO2S5lnWpbAY5l_nNnpsbxmwlyTHDHelbiO8eIvjqw0ewrBpICvN0FcboF3viMOClb_Gd_IbifIzLgHc_FayQ2HQI3ZcJMpg70iqDnkJYK4CSFDtKHJ_WJ43jkOCVttQHNii5oz12fzc8f94aXSuRQZ_Jj2SNh9QclpGHrp0khRmkVQNWSxLgDFe24gaVG1kwXRQWXQYsrXw63uyIeLLJtdQGkjQuLSC3OBLlmxclADdlVUYjn4c5nwnFwXzpzc868I3VsnZeB1KevFfuwEgJbDrp_i7REP1xvqoZJG8XmEvaKkYOy66rUDn0vfuzto6KluVaOVRuvLXl1SWb9OAr5jW_2vNbhVaOWddKzIAcl5Sf_9IBL53uN2evgpck26uwe29RcSFIba_8E7_rsHHMZzhOYF7Vx7GqK6_qSMS5szDp_XefrQFnPYfD2gx47XQAZ3Dm4dJflc_zQCUmmnVde6FqGw2c7c6Qej6mNdYs3OKbPo3ZivLofdizSAAbbQSaDJKZE4rR2YoZLqi2VzT6uyWvje1atdJ06orZ5h0yzD_hdeiwtG91btpGgT9xqCoDMfHP0h_T_mnfZ_uuut9yBp91vOHuZy_k36N2QYtJpf99qOv5scujQFn96kbJ_S6vi6_lyiIw03Jm9Nj_bhcVU3kMSbjzvwA0s8x4QnkavhghbntXCYz0GuYymq9vqCSBqkch6bCXMT8A9OyqXwACgAA_22_7D = __webpack_require__(86232);
;// ./src/styles/pane/paneSplit.css.ts



var emptyPaneSlot = 's1bun57';
var leafContainer = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'s1bun53',variantClassNames:{focused:{true:'s1bun54',false:'s1bun55'}},defaultVariants:{focused:false},compoundVariants:[]});
var leafZoomed = 's1bun56';
var paneBody = 's1bun58';
var rendererContent = 's1bun5h';
var rendererRoot = 's1bun5g';
var resetLayoutBar = 's1bun5a';
var resetLayoutButton = 's1bun5b';
var sessionListScroll = 's1bun59';
var splitContainer = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'s1bun50',variantClassNames:{direction:{vertical:'s1bun51',horizontal:'s1bun52'}},defaultVariants:{},compoundVariants:[]});
var viewModeToggleBar = 's1bun5c';
var viewModeToggleButton = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'s1bun5d',variantClassNames:{active:{true:'s1bun5e',false:'s1bun5f'}},defaultVariants:{},compoundVariants:[]});
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fpane%2FpanePickerOverlay.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71X246jOBB976_wy0pBG0c2aTI08zK_sU8jA0XiabC9tnNd9b-vYkMChEt6trWKEoFdFHVOnapyfrzDudCsAoN-0sOrNTIk6J8XhAotK3eBkFQs4_acIPL9BaGPF4Ss7G9Rv_Xx8uPRIZ12iJDVTJhC6irxlyWz8NciVqdg5oVjj5KgCWfVBBE6B0oabrkUCWKpkeXewtWQCwO2jibnRpXsnKCihNN1gZV8KzC3UJkEZSAs6Ovyr72xvDjjTAoLwra3Upa9b7XcizxBepuyBVm6zyqKXFwXzEUOpwSF7o3ZXhupE6QkbzwwwSvm47xnJSaVQcAMfG_jWk_gSqW1sqqBlVA0GDXf7prrezDRF8HfMpWgA9MLjH_SN7iE-hJeHHDF8pyL7cBub2VNg0EiKVmi5ktWcXyzyrVUuOClBZ2gtNzrBQ1r-aRS56CxlSpBVJ2QkSXPvbswipbo_kNWxHscoJ8iGg7y_-r4f5a2IW7OXW4ideqzQdpANMv53vS9rL-1bOaB0miY4EdGNs4wk-VVosM2b86kkMJiwy_wEFp63y9YxcvzAwWHYKQQXE3Xyr4HiyghlVk2jLjg_Nr1meOOW8BGsQwSJORRM9XNWLKTB9Aub_ME0Nc2-ZM8rIPOe6KuMrgouQD8-3VVcYGPPLe7BNFYOSc78JXc3N9ERNDar_RE06zOww4_paewLwFK_Zumsr6_P3SsgfSUw-fF53XvqL2x4dhiessF9l1voOJaidr0WmjBT5D358KtT1KyIdNdvuv925j3kYbcNO2MldnCB-6XsGCHGuMSbV7VKUB_1sgqmfISsGICsGUpNlZzdbMl6hQEfQxRH0OXo81E7-xawhPNKX5o8HGAiMc7MhTWNPD7w814eBjG_qTxTCd6RnE92bgtCyeLW6eOvVKgMxfINaXWgna9x5fhikRQ9SrzEedUu2_kME1_rfbGeGD6tlh6-68jq46waUaE_DGVSPLEdHeEKqZB2HbbEVLAjXQXX-LKZmRajCc-G0-rxzI1aLrkjUyPrtO4x3jCMssP8Lmn2F3N8x10EDabU_MtZ_Th1FGfS6ajnSt830auCsNmp7l4d02uhTH9lBYnpuP_J8avEl74yRZb17izHDxeP63h9Hc0nDnzmuZo0z2ANPdf8e9pZpJMi7Ia5_vvfrGEceuMMjkP5mswlVeFs1weH1z4kGq1YDiAsOaupr25DgwoIbPN4se_7fOfDpUPAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fpane_2FpanePickerOverlay_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71X246jOBB976_wy0pBG0c2aTI08zK_sU8jA0XiabC9tnNd9b_vYkMChEt6trWKEoFdFHVOnapyfrzDudCsAoN_0sOrNTIk6J8XhAotK3eBkFQs4_acIPL9BaGPF4Ss7G9Rv_Xx8uPRIZ12iJDVTJhC6irxlyWz8NciVqdg5oVjj5KgCWfVBBE6B0oabrkUCWKpkeXewtWQCwO2jibnRpXsnKCihNN1gZV8KzC3UJkEZSAs6Ovyr72xvDjjTAoLwra3Upa9b7XcizxBepuyBVm6zyqKXFwXzEUOpwSF7o3ZXhupE6QkbzwwwSvm47xnJSaVQcAMfG_jWk_gSqW1sqqBlVA0GDXf7prrezDRF8HfMpWgA9MLjH_SN7iE_hJeHHDF8pyL7cBub2VNg0EiKVmi5ktWcXyzyrVUuOClBZ2gtNzrBQ1r_aRS56CxlSpBVJ2QkSXPvbswipbo_kNWxHscoJ8iGg7y_r4f5a2IW7OXW4ideqzQdpANMv53vS9rL_1bOaB0miY4EdGNs4wk_VVosM2b86kkMJiwy_wEFp63y9YxcvzAwWHYKQQXE3Xyr4HiyghlVk2jLjg_Nr1meOOW8BGsQwSJORRM9XNWLKTB9Aub_ME0Nc2_ZM8rIPOe6KuMrgouQD8_3VVcYGPPLe7BNFYOSc78JXc3N9ERNDar_RE06zOww4_paewLwFK_Zumsr6_P3SsgfSUw_fF53XvqL2x4dhiessF9l1voOJaidr0WmjBT5D358KtT1KyIdNdvuv925j3kYbcNO2MldnCB_6XsGCHGuMSbV7VKUB_1sgqmfISsGICsGUpNlZzdbMl6hQEfQxRH0OXo81E7_xawhPNKX5o8HGAiMc7MhTWNPD7w814eBjG_qTxTCd6RnE92bgtCyeLW6eOvVKgMxfINaXWgna9x5fhikRQ9SrzEedUu2_kME1_rfbGeGD6tlh6_68jq46waUaE_DGVSPLEdHeEKqZB2HbbEVLAjXQXX_LKZmRajCc_G0_rxzI1aLrkjUyPrtO4x3jCMssP8Lmn2F3N8x10EDabU_MtZ_Th1FGfS6ajnSt830auCsNmp7l4d02uhTH9lBYnpuP_J8avEl74yRZb17izHDxeP63h9Hc0nDnzmuZo0z2ANPdf8e9pZpJMi7Ia5_vvfrGEceuMMjkP5mswlVeFs1weH1z4kGq1YDiAsOaupr25DgwoIbPN4se_7fOfDpUPAAA_22_7D = __webpack_require__(24681);
;// ./src/styles/pane/panePickerOverlay.css.ts


var mobilePickerBackdrop = '_1v4tso26';
var mobilePickerCancelButton = '_1v4tso2b';
var mobilePickerPaneItem = '_1v4tso29';
var mobilePickerPaneLabel = '_1v4tso2a';
var mobilePickerSheet = '_1v4tso27';
var mobilePickerSheetTitle = '_1v4tso28';
var pickerActionBar = '_1v4tso23';
var pickerActionButton = '_1v4tso24';
var pickerActionKbd = '_1v4tso25';
var pickerLabel = '_1v4tso2c';
var pickerOverlay = '_1v4tso22';
;// ./src/components/pane/PaneSplitRenderer.tsx
/* __next_internal_client_entry_do_not_use__ PaneSplitRenderer auto */ 




















function getPickerLetter(root, paneId) {
    const allLeaves = (0,paneUtils/* getAllLeaves */.eH)(root);
    const eligible = allLeaves.filter((l)=>l.viewKind !== "session-list");
    const idx = eligible.findIndex((l)=>l.id === paneId);
    return idx >= 0 && idx < 26 ? "ABCDEFGHIJKLMNOPQRSTUVWXYZ"[idx] : null;
}
const BOARD_SKELETON_ROWS_PER_COLUMN = 3;
// Board-shaped loading placeholder — 4 column shells with shimmer rows, shown instead of
// the flat SessionListSkeleton while in board view, so first-load never looks identical to
// a genuinely empty board (ux.md "Loading state", AC31). Reuses SessionListSkeleton's
// shimmer row pieces rather than BoardColumn's own markup/CSS, since BoardColumn.tsx is
// owned by the concurrent Phase 3 work.
function BoardColumnsSkeleton() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        role: "status",
        "aria-busy": "true",
        "aria-label": "Loading sessions…",
        "data-testid": "board-columns-skeleton",
        style: {
            display: "flex",
            gap: 16,
            padding: 16,
            flex: 1,
            overflowX: "auto",
            minHeight: 0
        },
        children: BOARD_COLUMNS.map((col)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                "data-testid": "board-column-skeleton-".concat(col.key),
                style: {
                    width: 320,
                    flexShrink: 0
                },
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        style: {
                            fontSize: 12,
                            fontWeight: 600,
                            opacity: 0.6,
                            marginBottom: 8,
                            textTransform: "uppercase"
                        },
                        children: col.label
                    }),
                    Array.from({
                        length: BOARD_SKELETON_ROWS_PER_COLUMN
                    }, (_, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: skeletonRow,
                            "aria-hidden": "true",
                            style: {
                                marginBottom: 8
                            },
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: dot
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: nameBar
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: agentPlaceholder
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: pathBar
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: timeBar
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: actionsSpacer
                                })
                            ]
                        }, i))
                ]
            }, col.key))
    });
}
function PaneNodeComponent(param) {
    let { node, state, dispatch, sessions, isMobile, hasSplits } = param;
    if (node.type === "leaf") {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(PaneLeafComponent, {
            pane: node,
            state: state,
            dispatch: dispatch,
            sessions: sessions,
            isMobile: isMobile,
            hasSplits: hasSplits
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(PaneSplitComponent, {
        pane: node,
        state: state,
        dispatch: dispatch,
        sessions: sessions,
        isMobile: isMobile,
        hasSplits: hasSplits
    });
}
function PaneSplitComponent(param) {
    let { pane, state, dispatch, sessions, isMobile, hasSplits } = param;
    const containerRef = (0,react.useRef)(null);
    // On mobile: vertical (side-by-side) splits collapse to the focused pane only.
    // Horizontal (top/bottom) splits are fine on mobile — show both panes stacked.
    if (isMobile && pane.direction === "vertical") {
        const focusedInFirst = (0,paneUtils/* containsPaneId */.$o)(pane.first, state.focusedPaneId);
        const visibleNode = focusedInFirst ? pane.first : pane.second;
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(PaneNodeComponent, {
            node: visibleNode,
            state: state,
            dispatch: dispatch,
            sessions: sessions,
            isMobile: isMobile,
            hasSplits: hasSplits
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: containerRef,
        className: splitContainer({
            direction: pane.direction
        }),
        style: {
            "--split-ratio": String(pane.ratio)
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(PaneNodeComponent, {
                node: pane.first,
                state: state,
                dispatch: dispatch,
                sessions: sessions,
                isMobile: isMobile,
                hasSplits: hasSplits
            }),
            !isMobile && /*#__PURE__*/ (0,jsx_runtime.jsx)(ResizeHandle, {
                splitId: pane.id,
                direction: pane.direction,
                onResize: (splitId, ratio)=>dispatch({
                        type: "RESIZE_PANE",
                        splitId,
                        ratio
                    })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(PaneNodeComponent, {
                node: pane.second,
                state: state,
                dispatch: dispatch,
                sessions: sessions,
                isMobile: isMobile,
                hasSplits: hasSplits
            })
        ]
    });
}
function SessionListPaneBody(param) {
    let { pane, dispatch } = param;
    const actions = useCockpitActions();
    const { sessions, loading, error, listSessions, hibernateSession, resumeHibernatedSession } = (0,SessionServiceContext/* useSessionServiceContext */.MS)();
    const { triggerPicker, triggerPickerForceNew, backlogIndex } = usePaneContext();
    const { viewMode, setViewMode } = useSessionViewModeContext();
    if (loading) return viewMode === "board" ? /*#__PURE__*/ (0,jsx_runtime.jsx)(BoardColumnsSkeleton, {}) : /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionListSkeleton, {
        count: 4
    });
    // `error` is useSessionServiceContext's single shared Redux field -- ANY
    // session-mutating RPC (createSession, updateSession, cancelSessionCreation,
    // etc., see useSessionService.ts's dispatch(setError(...)) call sites)
    // writes to this same slot, not just the initial list fetch. Gating the
    // full-pane "Failed to Load Sessions" fallback on `error` alone means an
    // already-successfully-loaded, non-empty list gets replaced by that
    // fallback the moment ANY later mutation fails -- e.g. a routine,
    // client-visible rejection like Omnibar's duplicate-title create error
    // (session-creation-async.spec.ts's "duplicate title keeps the omnibar
    // open with inline error" regression test), which already has its own
    // inline `omnibar-create-error` alert and needs no page-level fallback at
    // all. Only show this fallback when there is nothing else to show --
    // i.e. the list itself is empty because the fetch that would have
    // populated it is what failed.
    if (error && sessions.length === 0) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(ErrorState/* ErrorState */.W, {
            error: error,
            title: "Failed to Load Sessions",
            message: "Unable to connect to the server.",
            onRetry: listSessions
        });
    }
    const sharedProps = {
        sessions,
        onSessionClick: triggerPicker,
        onSessionOpenInNewPane: triggerPickerForceNew,
        onDeleteSession: actions.onDeleteSession,
        onPauseSession: actions.onPauseSession,
        onResumeSession: actions.onResumeSession,
        onDirectResumeSession: actions.onDirectResumeSession,
        onCloneSession: actions.onCloneSession,
        onNewWorkspaceSession: actions.onNewWorkspaceSession,
        onRenameSession: actions.onRenameSession,
        onRestartSession: actions.onRestartSession,
        onUpdateTags: actions.onUpdateTags,
        onNewSession: actions.onNewSession,
        onCreateCheckpoint: actions.onCreateCheckpoint,
        onListCheckpoints: actions.onListCheckpoints,
        onForkFromCheckpoint: actions.onForkFromCheckpoint,
        onSetRateLimitEnabled: actions.onSetRateLimitEnabled,
        onToggleAutonomousMode: actions.onToggleAutonomousMode,
        onTogglePinned: actions.onTogglePinned,
        onToggleAutoApprove: actions.onToggleAutoApprove,
        onSteerAutonomousSession: actions.onSteerAutonomousSession,
        onClearConversationState: actions.onClearConversationState,
        onHibernateSession: hibernateSession ? (id)=>void hibernateSession(id) : undefined,
        onResumeHibernatedSession: resumeHibernatedSession ? (id)=>void resumeHibernatedSession(id) : undefined,
        onFetchArchivedSessions: (includeArchived)=>/* analytics-exempt */ void listSessions({
                includeArchived
            }),
        onFetchHiddenSessions: (includeHidden)=>/* analytics-exempt */ void listSessions({
                includeHidden
            }),
        storageKeyPrefix: "pane-".concat(pane.id, "."),
        backlogIndex
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: viewModeToggleBar,
                role: "group",
                "aria-label": "Session view",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        "data-testid": "session-view-mode-list",
                        className: viewModeToggleButton({
                            active: viewMode === "list"
                        }),
                        "aria-pressed": viewMode === "list",
                        onClick: ()=>setViewMode("list"),
                        children: "List"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        "data-testid": "session-view-mode-board",
                        className: viewModeToggleButton({
                            active: viewMode === "board"
                        }),
                        "aria-pressed": viewMode === "board",
                        onClick: ()=>setViewMode("board"),
                        children: "Board"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                "aria-atomic": "true",
                style: {
                    position: "absolute",
                    width: 1,
                    height: 1,
                    overflow: "hidden",
                    clip: "rect(0,0,0,0)"
                },
                children: viewMode === "board" ? "Board view, showing ".concat(sessions.length, " sessions") : "List view, showing ".concat(sessions.length, " sessions")
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: sessionListScroll,
                "data-testid": "session-list-scroll",
                children: viewMode === "board" ? /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionBoard, {
                    ...sharedProps
                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionList, {
                    ...sharedProps
                })
            })
        ]
    });
}
function PaneLeafComponent(param) {
    let { pane, state, dispatch, sessions, isMobile, hasSplits } = param;
    const { pickerPendingSession, cancelPicker, backlogIndex } = usePaneContext();
    const isFocused = state.focusedPaneId === pane.id;
    const isZoomed = state.zoomedPaneId === pane.id;
    const pickerLetter = pickerPendingSession ? getPickerLetter(state.root, pane.id) : null;
    var _sessions_find;
    const session = pane.viewKind === "session-detail" && pane.sessionId ? (_sessions_find = sessions.find((s)=>s.id === pane.sessionId)) !== null && _sessions_find !== void 0 ? _sessions_find : null : null;
    const sessionBacklogEntry = session ? backlogIndex.get(session.id) : undefined;
    const handleFocus = ()=>dispatch({
            type: "FOCUS_PANE",
            paneId: pane.id
        });
    const handleClose = ()=>dispatch({
            type: "CLOSE_PANE",
            paneId: pane.id
        });
    const handleZoom = ()=>dispatch({
            type: "ZOOM_PANE",
            paneId: pane.id
        });
    const handleTabChange = (tab)=>dispatch({
            type: "ASSIGN_TAB",
            paneId: pane.id,
            tab
        });
    const handleSplitVertical = ()=>dispatch({
            type: "SPLIT_PANE",
            paneId: pane.id,
            direction: "vertical"
        });
    const handleSplitHorizontal = ()=>dispatch({
            type: "SPLIT_PANE",
            paneId: pane.id,
            direction: "horizontal"
        });
    const handleSetView = (viewKind)=>dispatch({
            type: "SET_PANE_VIEW",
            paneId: pane.id,
            viewKind
        });
    const handleDragStart = (e)=>{
        e.dataTransfer.setData("text/pane-id", pane.id);
        e.dataTransfer.effectAllowed = "move";
    };
    const handleDragOver = (e)=>{
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
    };
    const handleDrop = (e)=>{
        e.preventDefault();
        const sourcePaneId = e.dataTransfer.getData("text/pane-id");
        if (sourcePaneId && sourcePaneId !== pane.id) {
            dispatch({
                type: "SWAP_PANES",
                paneId: sourcePaneId,
                targetPaneId: pane.id
            });
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: "".concat(leafContainer({
            focused: isFocused && hasSplits
        })).concat(isZoomed ? " ".concat(leafZoomed) : ""),
        "data-focused": isFocused ? "true" : "false",
        "data-testid": "pane-leaf-".concat(pane.id),
        "data-context": "cockpit",
        draggable: hasSplits,
        onDragStart: handleDragStart,
        onDragOver: handleDragOver,
        onDrop: handleDrop,
        onClick: handleFocus,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(PaneHeader, {
                pane: pane,
                sessions: sessions,
                isFocused: isFocused,
                onClose: handleClose,
                onFocus: handleFocus,
                onZoom: handleZoom,
                onSetView: handleSetView,
                splitButtonVisible: !isMobile,
                onSplitVertical: handleSplitVertical,
                onSplitHorizontal: handleSplitHorizontal
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: paneBody,
                children: pane.viewKind === "session-list" ? /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionListPaneBody, {
                    pane: pane,
                    dispatch: dispatch
                }) : session ? /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionDetail/* SessionDetail */.G, {
                    session: session,
                    onClose: handleClose,
                    onFullscreenChange: ()=>{},
                    onTabChange: handleTabChange,
                    initialTab: pane.activeTab,
                    embedded: true,
                    isActive: isFocused,
                    backlogItemId: sessionBacklogEntry === null || sessionBacklogEntry === void 0 ? void 0 : sessionBacklogEntry.itemId,
                    backlogEntry: sessionBacklogEntry
                }, "".concat(pane.id, "-").concat(pane.sessionId)) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: emptyPaneSlot,
                    children: "Click a session to open it here"
                })
            }),
            pickerLetter && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: pickerOverlay,
                onClick: (e)=>{
                    e.stopPropagation();
                    dispatch({
                        type: "ASSIGN_SESSION",
                        paneId: pane.id,
                        sessionId: pickerPendingSession.id
                    });
                    dispatch({
                        type: "ASSIGN_TAB",
                        paneId: pane.id,
                        tab: "terminal"
                    });
                    cancelPicker();
                },
                "aria-label": "Open session in this pane (press ".concat(pickerLetter, ")"),
                role: "button",
                tabIndex: 0,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: pickerLabel,
                    children: pickerLetter
                })
            })
        ]
    });
}
/**
 * PaneSplitRenderer — root renderer for the pane tree.
 * Renders the full recursive pane layout.
 */ function PaneSplitRenderer(param) {
    let { state, dispatch, sessions } = param;
    const { isMobile, isFoldable } = (0,ViewportProvider/* useViewport */.k)();
    // Collapse split panes for any viewport below 900px (mobile + foldable) — BottomNav is
    // visible there and single-pane + MobilePaneTabStrip gives a better UX than cramped splits.
    const isNarrow = isMobile || isFoldable;
    const allLeaves = (0,paneUtils/* getAllLeaves */.eH)(state.root);
    const hasMultiplePanes = allLeaves.length > 1;
    // Show tab strip on narrow screens whenever there are multiple panes — vertical splits
    // collapse to one visible pane and need the strip to switch; horizontal splits keep both
    // visible but still benefit from the strip's "+" add-pane button.
    const showMobileTabStrip = isNarrow && hasMultiplePanes;
    // Publish the actual strip height via ResizeObserver so fixed-position elements
    // (e.g. notification toasts) can clear it without hard-coding the pixel value.
    const stripRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        if (!showMobileTabStrip) {
            document.documentElement.style.setProperty("--mobile-pane-tab-strip-height", "0px");
            return;
        }
        const el = stripRef.current;
        if (!el) return;
        const update = ()=>{
            document.documentElement.style.setProperty("--mobile-pane-tab-strip-height", "".concat(el.offsetHeight, "px"));
        };
        const ro = new ResizeObserver(update);
        ro.observe(el);
        update();
        return ()=>ro.disconnect();
    }, [
        showMobileTabStrip
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: rendererRoot,
        "data-context": "cockpit",
        children: [
            hasMultiplePanes && !isNarrow && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: resetLayoutBar,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                    "data-testid": "reset-layout-btn",
                    className: resetLayoutButton,
                    onClick: ()=>dispatch({
                            type: "RESET_LAYOUT"
                        }),
                    title: "Reset to single pane",
                    children: "Reset layout"
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: rendererContent,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(PaneNodeComponent, {
                    node: state.root,
                    state: state,
                    dispatch: dispatch,
                    sessions: sessions,
                    isMobile: isNarrow,
                    hasSplits: hasMultiplePanes
                })
            }),
            showMobileTabStrip && /*#__PURE__*/ (0,jsx_runtime.jsx)(MobilePaneTabStrip, {
                ref: stripRef,
                leaves: allLeaves,
                focusedPaneId: state.focusedPaneId,
                sessions: sessions,
                onFocus: (paneId)=>dispatch({
                        type: "FOCUS_PANE",
                        paneId
                    }),
                onAddPane: ()=>dispatch({
                        type: "SPLIT_PANE",
                        paneId: state.focusedPaneId,
                        direction: "horizontal"
                    })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionPeekModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VU0Y6qMBB99yvmUZLLBnTXle7L_sZ92hQ6QFdom7YKeuO_37SIsihqsuEBpjM5c-bMKZ8b3Oea1mjgK37fRulbEsG_GUCuZe0_AKSiGbd7AtHHDOA4A7BynIq71HH2eQ0Y3wcEsJoKk0tdEzAZrXAevSSroDuuqMW_87VqgwfNb8DEPzCioOf40jNbeCQlDbdcCgI5b5G5Ki4M2hO_lGabQsutYAR0kdJ59Mc_LysPeAi5YNgSSJLExVTwmnZwF0XjRVQbQGrwY9h_OdXfSkVguSvdd4W5dUHjAs2L8hKl0lpZnyvPVOIo8tQZN6qiewJ5ha07cO-QcY1Z1zCT1bYW4yl3VM_D8CtO8LDQhyToWmmGmkCsWjCy4mxU9T2oCjVlfGvGQMv1qaYNTUmZbK4Kal8gd6jzyqVLzhiKCVVjiF9vqvrqVb2anVa8ECG3WBsCGQqL2h1_b43l-T7MpLAoLAGjaIZhirbBrreijHFRjNkuDsGYfzQUoV_OlGJ5cN6IKTUXm5PfCqputPox4Vt3o6SwoeEHvNIx7aBdvsHOMaOK0ldkspJ6nIsml2CxteElgVXFleHGpZqSWwy9dASEbDRV_XT-fg7Irx6Qp9PUFsF9q2ZPWTV_wqrd3T5v3kHdWMn1-gZzvv_OhpdUw5ktCSzWygOUp4328TODPCnJQFj_41RUo7AP1pFttXE5JXnPeHq52UPVSOkc5rWbXvT6vn0HeOvObb0Pbxv76f_k8T8YFJC_MAcAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionPeekModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VU0Y6qMBB99yvmUZLLBnTXle7L_sZ92hQ6QFdom7YKeuO_37SIsihqsuEBpjM5c_bMKZ8b3Oea1mjgK37fRulbEsG_GUCuZe0_AKSiGbd7AtHHDOA4A7BynIq71HH2eQ0Y3wcEsJoKk0tdEzAZrXAevSSroDuuqMW_87VqgwfNb8DEPzCioOf40jNbeCQlDbdcCgI5b5G5Ki4M2hO_lGabQsutYAR0kdJ59Mc_LysPeAi5YNgSSJLExVTwmnZwF0XjRVQbQGrwY9h_OdXfSkVguSvdd4W5dUHjAs2L8hKl0lpZnyvPVOIo8tQZN6qiewJ5ha07cO_QcY1Z1zCT1bYW4yl3VM_D8CtO8LDQhyToWmmGmkCsWjCy4mxU9T2oCjVlfGvGQMv1qaYNTUmZbK4Kal8gd6jzyqVLzhiKCVVjiF9vqvrqVb2anVa8ECG3WBsCGQqL2h1_b43l_T7MpLAoLAGjaIZhirbBrreijHFRjNkuDsGYfzQUoV_OlGJ5cN6IKTUXm5PfCqputPox4Vt3o6SwoeEHvNIx7aBdvsHOMaOK0ldkspJ6nIsml2CxteElgVXFleHGpZqSWwy9dASEbDRV_XT_fg7Irx6Qp9PUFsF9q2ZPWTV_wqrd3T5v3kHdWMn1_gZzvv_OhpdUw5ktCSzWygOUp4328TODPCnJQFj_41RUo7AP1pFttXE5JXnPeHq52UPVSOkc5rWbXvT6vn0HeOvObb0Pbxv76f_k8T8YFJC_MAcAAA_22_7D = __webpack_require__(80320);
;// ./src/components/sessions/SessionPeekModal.css.ts

var backdrop = '_17u0b592';
var closeButton = '_17u0b597';
var modal = '_17u0b593';
var modalBody = '_17u0b598';
var modalHeader = '_17u0b594';
var modalTitle = '_17u0b595';
var peekBadge = '_17u0b596';
;// ./src/components/sessions/SessionPeekModal.tsx
/* __next_internal_client_entry_do_not_use__ SessionPeekModal auto */ 






function SessionPeekModal(param) {
    let { session, onClose } = param;
    const modalRef = (0,react.useRef)(null);
    const { backlogIndex } = usePaneContext();
    const backlogEntry = backlogIndex.get(session.id);
    (0,react.useEffect)(()=>{
        function handleKeyDown(e) {
            if (e.key === "Escape") {
                e.stopPropagation();
                onClose();
            }
        }
        window.addEventListener("keydown", handleKeyDown, {
            capture: true
        });
        return ()=>window.removeEventListener("keydown", handleKeyDown, {
                capture: true
            });
    }, [
        onClose
    ]);
    (0,react.useEffect)(()=>{
        var _modalRef_current;
        (_modalRef_current = modalRef.current) === null || _modalRef_current === void 0 ? void 0 : _modalRef_current.focus();
    }, []);
    if (typeof document === "undefined") return null;
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: backdrop,
                onClick: onClose,
                "aria-hidden": "true"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                ref: modalRef,
                className: modal,
                role: "dialog",
                "aria-modal": "true",
                "aria-labelledby": "peek-modal-title",
                tabIndex: -1,
                onClick: (e)=>e.stopPropagation(),
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: modalHeader,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                id: "peek-modal-title",
                                className: modalTitle,
                                children: session.title
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: peekBadge,
                                children: "Peek"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(BacklogOriginBadge/* BacklogOriginBadge */.J, {
                                entry: backlogEntry,
                                compact: true
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: closeButton,
                                onClick: onClose,
                                "aria-label": "Close peek modal",
                                children: "✕"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: modalBody,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionDetail/* SessionDetail */.G, {
                            session: session,
                            onClose: onClose,
                            embedded: true,
                            initialTab: "terminal",
                            backlogItemId: backlogEntry === null || backlogEntry === void 0 ? void 0 : backlogEntry.itemId,
                            backlogEntry: backlogEntry
                        })
                    })
                ]
            })
        ]
    }), document.body);
}

// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
;// ./src/components/pane/PaneTilingContainer.tsx
/* __next_internal_client_entry_do_not_use__ PaneTilingContainer auto */ 











/**
 * PaneTilingContainer — top-level tiling layout component.
 *
 * Holds the pane reducer state, wires keyboard shortcuts, persists layout,
 * and renders the recursive pane tree via PaneSplitRenderer.
 */ function PaneTilingContainer(param) {
    let { sessions, paneState, dispatch, externalSessionAssign } = param;
    const state = paneState;
    const containerRef = (0,react.useRef)(null);
    const prevVersionRef = (0,react.useRef)(null);
    const [pickerPendingSession, setPickerPendingSession] = (0,react.useState)(null);
    const pickerBarRef = (0,react.useRef)(null);
    const [peekSession, setPeekSession] = (0,react.useState)(null);
    // Mobile-only: bottom sheet picker state (replaces overlay when on narrow screens)
    const [mobilePickerSession, setMobilePickerSession] = (0,react.useState)(null);
    const [mobilePickerPanes, setMobilePickerPanes] = (0,react.useState)([]);
    const mobilePickerSheetRef = (0,react.useRef)(null);
    const { isMobile, isFoldable } = (0,ViewportProvider/* useViewport */.k)();
    const isNarrow = isMobile || isFoldable;
    const { index: backlogIndex } = (0,useBacklogService/* useBacklogSessionIndex */.Z3)();
    const cancelPicker = (0,react.useCallback)(()=>{
        setPickerPendingSession(null);
        setMobilePickerSession(null);
        setMobilePickerPanes([]);
    }, []);
    const triggerPickerForceNew = (0,react.useCallback)((session, tab)=>{
        var _containerRef_current;
        const resolvedTab = tab !== null && tab !== void 0 ? tab : "terminal";
        var _containerRef_current_getBoundingClientRect_width;
        // Choose split direction that keeps terminal width >= 120 columns.
        // At ~8px per character, 120 cols ≈ 960px per pane → need container >= 1920px for vertical split.
        // If the container is narrower, use horizontal split (stacked) to preserve full width.
        const containerWidth = (_containerRef_current_getBoundingClientRect_width = (_containerRef_current = containerRef.current) === null || _containerRef_current === void 0 ? void 0 : _containerRef_current.getBoundingClientRect().width) !== null && _containerRef_current_getBoundingClientRect_width !== void 0 ? _containerRef_current_getBoundingClientRect_width : 0;
        const direction = containerWidth / 2 >= 960 ? "vertical" : "horizontal";
        dispatch({
            type: "SPLIT_AND_ASSIGN_SESSION",
            paneId: state.focusedPaneId,
            sessionId: session.id,
            tab: resolvedTab,
            direction
        });
    }, [
        state.focusedPaneId,
        dispatch,
        containerRef
    ]);
    const triggerPicker = (0,react.useCallback)((session, tab)=>{
        const resolvedTab = tab !== null && tab !== void 0 ? tab : "terminal";
        const allLeaves = (0,paneUtils/* getAllLeaves */.eH)(state.root);
        const eligiblePanes = allLeaves.filter((l)=>l.viewKind !== "session-list");
        if (eligiblePanes.length === 0) {
            // Auto-split: reducer creates a new detail pane beside the session-list pane.
            dispatch({
                type: "ASSIGN_SESSION",
                paneId: state.focusedPaneId,
                sessionId: session.id
            });
        } else if (eligiblePanes.length === 1) {
            // Single detail pane: assign directly. If focused pane is the detail pane,
            // eligiblePanes[0] === focusedLeaf, so the intent shortcut still fires — just
            // via this branch rather than the removed early-return block.
            dispatch({
                type: "ASSIGN_SESSION",
                paneId: eligiblePanes[0].id,
                sessionId: session.id
            });
            dispatch({
                type: "ASSIGN_TAB",
                paneId: eligiblePanes[0].id,
                tab: resolvedTab
            });
            // On mobile, vertical splits show only the focused pane. Move focus to the detail
            // pane so the user sees the session they just opened instead of the session list.
            dispatch({
                type: "FOCUS_PANE",
                paneId: eligiblePanes[0].id
            });
        } else if (isNarrow) {
            // On mobile, only one pane is visible at a time — the desktop overlay (which
            // expects the user to click on a visible pane) doesn't work. Use a bottom sheet
            // that lists all eligible panes by name instead.
            const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
            setMobilePickerSession(session);
            setMobilePickerPanes(eligiblePanes.map((p, i)=>{
                const sess = sessions.find((s)=>s.id === p.sessionId);
                var _letters_i;
                return {
                    id: p.id,
                    label: sess ? sess.title : p.viewKind === "session-list" ? "Sessions" : "Empty pane",
                    letter: (_letters_i = letters[i]) !== null && _letters_i !== void 0 ? _letters_i : String(i + 1)
                };
            }));
        } else {
            // 2+ eligible panes on desktop: show the picker overlay so the user can click
            // on the pane they want.
            setPickerPendingSession(session);
        }
    }, [
        state.root,
        state.focusedPaneId,
        dispatch,
        isNarrow,
        sessions
    ]);
    // Register keyboard shortcuts
    usePaneShortcuts(state, dispatch, containerRef);
    // Keyboard handler for the pane picker: Escape cancels, A–Z selects the nth eligible pane.
    (0,react.useEffect)(()=>{
        if (!pickerPendingSession) return;
        const handleKeyDown = (e)=>{
            if (e.key === "Escape") {
                e.stopPropagation();
                cancelPicker();
                return;
            }
            const upper = e.key.toUpperCase();
            if (upper === "M") {
                e.stopPropagation();
                setPeekSession(pickerPendingSession);
                cancelPicker();
                return;
            }
            if (upper === "V") {
                e.stopPropagation();
                dispatch({
                    type: "SPLIT_AND_ASSIGN_SESSION",
                    paneId: state.focusedPaneId,
                    sessionId: pickerPendingSession.id,
                    tab: "terminal",
                    direction: "vertical"
                });
                cancelPicker();
                return;
            }
            if (upper === "H") {
                e.stopPropagation();
                dispatch({
                    type: "SPLIT_AND_ASSIGN_SESSION",
                    paneId: state.focusedPaneId,
                    sessionId: pickerPendingSession.id,
                    tab: "terminal",
                    direction: "horizontal"
                });
                cancelPicker();
                return;
            }
            if (upper.length === 1 && upper >= "A" && upper <= "Z") {
                const allLeaves = (0,paneUtils/* getAllLeaves */.eH)(state.root);
                const eligiblePanes = allLeaves.filter((l)=>l.viewKind !== "session-list");
                const idx = upper.charCodeAt(0) - 65;
                const target = eligiblePanes[idx];
                if (target) {
                    e.stopPropagation();
                    dispatch({
                        type: "ASSIGN_SESSION",
                        paneId: target.id,
                        sessionId: pickerPendingSession.id
                    });
                    dispatch({
                        type: "ASSIGN_TAB",
                        paneId: target.id,
                        tab: "terminal"
                    });
                    cancelPicker();
                }
            }
        };
        window.addEventListener("keydown", handleKeyDown, {
            capture: true
        });
        return ()=>window.removeEventListener("keydown", handleKeyDown, {
                capture: true
            });
    }, [
        pickerPendingSession,
        state.root,
        dispatch,
        cancelPicker
    ]);
    // Focus the mobile sheet when it opens so keyboard and screen reader users can interact.
    (0,react.useEffect)(()=>{
        if (mobilePickerSession) {
            var _mobilePickerSheetRef_current;
            (_mobilePickerSheetRef_current = mobilePickerSheetRef.current) === null || _mobilePickerSheetRef_current === void 0 ? void 0 : _mobilePickerSheetRef_current.focus();
        }
    }, [
        mobilePickerSession
    ]);
    // Focus the first button in the picker action bar when it appears, so keyboard
    // and screen reader users can immediately interact without hunting for focus.
    (0,react.useEffect)(()=>{
        if (pickerPendingSession && pickerBarRef.current) {
            const firstBtn = pickerBarRef.current.querySelector("button");
            firstBtn === null || firstBtn === void 0 ? void 0 : firstBtn.focus();
        }
    }, [
        pickerPendingSession
    ]);
    // When externalSessionAssign fires (omnibar, URL nav, keyboard), route through
    // triggerPicker so the user gets the same pane-picker UX as session-list clicks.
    (0,react.useEffect)(()=>{
        if (!externalSessionAssign) return;
        if (externalSessionAssign.version === prevVersionRef.current) return;
        prevVersionRef.current = externalSessionAssign.version;
        const session = sessions.find((s)=>s.id === externalSessionAssign.sessionId);
        if (!session) return;
        if (externalSessionAssign.forceNewPane) {
            triggerPickerForceNew(session, externalSessionAssign.tab);
        } else {
            triggerPicker(session, externalSessionAssign.tab);
        }
    }, [
        externalSessionAssign,
        sessions,
        triggerPicker,
        triggerPickerForceNew
    ]);
    const allLeaves = pickerPendingSession ? (0,paneUtils/* getAllLeaves */.eH)(state.root) : [];
    var _allLeaves_find;
    const listPane = (_allLeaves_find = allLeaves.find((l)=>l.viewKind === "session-list")) !== null && _allLeaves_find !== void 0 ? _allLeaves_find : null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(PaneContext.Provider, {
        value: {
            state,
            dispatch,
            sessions,
            pickerPendingSession,
            triggerPicker,
            triggerPickerForceNew,
            cancelPicker,
            backlogIndex
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                ref: containerRef,
                style: {
                    display: "flex",
                    flex: 1,
                    overflow: "hidden",
                    minHeight: 0,
                    position: "relative"
                },
                "data-context": "cockpit",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(PaneSplitRenderer, {
                        state: state,
                        dispatch: dispatch,
                        sessions: sessions
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        role: "status",
                        "aria-live": "assertive",
                        "aria-atomic": "true",
                        style: {
                            position: "absolute",
                            width: 1,
                            height: 1,
                            overflow: "hidden",
                            clip: "rect(0,0,0,0)"
                        },
                        children: pickerPendingSession ? 'Choose where to open "'.concat(pickerPendingSession.title, '": Side by side (V), Top/Bottom (H), Replace list (M), or Cancel (Esc)') : ""
                    }),
                    pickerPendingSession && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        ref: pickerBarRef,
                        className: pickerActionBar,
                        role: "toolbar",
                        "aria-label": "Open ".concat(pickerPendingSession.title),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: pickerActionButton,
                                onClick: ()=>{
                                    dispatch({
                                        type: "SPLIT_AND_ASSIGN_SESSION",
                                        paneId: state.focusedPaneId,
                                        sessionId: pickerPendingSession.id,
                                        tab: "terminal",
                                        direction: "vertical"
                                    });
                                    cancelPicker();
                                },
                                title: "Open in a new pane side by side",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(columns_2/* default */.A, {
                                        size: 14
                                    }),
                                    "Side by side",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: pickerActionKbd,
                                        children: "V"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: pickerActionButton,
                                onClick: ()=>{
                                    dispatch({
                                        type: "SPLIT_AND_ASSIGN_SESSION",
                                        paneId: state.focusedPaneId,
                                        sessionId: pickerPendingSession.id,
                                        tab: "terminal",
                                        direction: "horizontal"
                                    });
                                    cancelPicker();
                                },
                                title: "Open in a new pane stacked top/bottom",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(rows_2/* default */.A, {
                                        size: 14
                                    }),
                                    "Top / Bottom",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: pickerActionKbd,
                                        children: "H"
                                    })
                                ]
                            }),
                            listPane && /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: pickerActionButton,
                                onClick: ()=>{
                                    dispatch({
                                        type: "SET_PANE_VIEW",
                                        paneId: listPane.id,
                                        viewKind: "session-detail"
                                    });
                                    dispatch({
                                        type: "ASSIGN_SESSION",
                                        paneId: listPane.id,
                                        sessionId: pickerPendingSession.id
                                    });
                                    cancelPicker();
                                },
                                title: "Replace the session list pane with this session",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(layout_list/* default */.A, {
                                        size: 14
                                    }),
                                    "Replace list"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: pickerActionButton,
                                onClick: ()=>{
                                    setPeekSession(pickerPendingSession);
                                    cancelPicker();
                                },
                                title: "Open in a floating modal without changing pane layout",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(maximize_2/* default */.A, {
                                        size: 14
                                    }),
                                    "Open in Modal",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: pickerActionKbd,
                                        children: "M"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                className: pickerActionButton,
                                onClick: cancelPicker,
                                title: "Cancel (Esc)",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(x/* default */.A, {
                                        size: 14
                                    }),
                                    "Cancel",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: pickerActionKbd,
                                        children: "Esc"
                                    })
                                ]
                            })
                        ]
                    }),
                    peekSession && /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionPeekModal, {
                        session: peekSession,
                        onClose: ()=>setPeekSession(null)
                    })
                ]
            }),
            mobilePickerSession && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: mobilePickerBackdrop,
                        onClick: cancelPicker,
                        "aria-hidden": "true"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        ref: mobilePickerSheetRef,
                        className: mobilePickerSheet,
                        role: "dialog",
                        "aria-modal": "true",
                        "aria-labelledby": "mobile-pane-picker-title",
                        tabIndex: -1,
                        onKeyDown: (e)=>{
                            if (e.key === "Escape") cancelPicker();
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                id: "mobile-pane-picker-title",
                                className: mobilePickerSheetTitle,
                                children: "Open session in…"
                            }),
                            mobilePickerPanes.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                    className: mobilePickerPaneItem,
                                    onClick: ()=>{
                                        dispatch({
                                            type: "ASSIGN_SESSION",
                                            paneId: p.id,
                                            sessionId: mobilePickerSession.id
                                        });
                                        dispatch({
                                            type: "ASSIGN_TAB",
                                            paneId: p.id,
                                            tab: "terminal"
                                        });
                                        dispatch({
                                            type: "FOCUS_PANE",
                                            paneId: p.id
                                        });
                                        cancelPicker();
                                    },
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: mobilePickerPaneLabel,
                                            children: p.letter
                                        }),
                                        p.label
                                    ]
                                }, p.id)),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: mobilePickerCancelButton,
                                onClick: cancelPicker,
                                children: "Cancel"
                            })
                        ]
                    })
                ]
            }), document.body)
        ]
    });
}

;// ./src/lib/pane/usePaneLayout.ts
const STORAGE_KEY = "cockpit.paneLayout";
/**
 * Walk the pane tree and nullify any sessionId that is not in the validIds set.
 * Returns a new tree (immutable).
 */ function validateAndRepair(tree, validIds) {
    if (tree.type === "leaf") {
        var _tree_viewKind;
        const leaf = {
            ...tree,
            // Backward compat: old saved layouts pre-dating viewKind default to session-detail
            viewKind: (_tree_viewKind = tree.viewKind) !== null && _tree_viewKind !== void 0 ? _tree_viewKind : "session-detail",
            sessionId: tree.sessionId !== null && validIds.has(tree.sessionId) ? tree.sessionId : null
        };
        return leaf;
    }
    return {
        ...tree,
        first: validateAndRepair(tree.first, validIds),
        second: validateAndRepair(tree.second, validIds)
    };
}
/** Serialize and save the pane state to localStorage */ function savePaneLayout(state) {
    try {
        const layout = {
            version: 1,
            root: state.root,
            focusedPaneId: state.focusedPaneId,
            zoomedPaneId: state.zoomedPaneId
        };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(layout));
    } catch (e) {
    // localStorage may be unavailable (private mode, quota exceeded, SSR)
    }
}
/** Clear the saved pane layout from localStorage */ function clearPaneLayout() {
    try {
        localStorage.removeItem(STORAGE_KEY);
    } catch (e) {
    // ignore
    }
}
/**
 * Parse and validate a PersistedPaneLayout from localStorage.
 * Returns null on any parse/validation error.
 */ function loadPaneLayout() {
    try {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (!stored) return null;
        const parsed = JSON.parse(stored);
        if (parsed.version !== 1) return null;
        if (!parsed.root) return null;
        if (typeof parsed.focusedPaneId !== "string") return null;
        if (!("zoomedPaneId" in parsed)) return null;
        return parsed;
    } catch (e) {
        return null;
    }
}

;// ./src/lib/pane/paneReducer.ts

function paneReducer_clampRatio(ratio) {
    return Math.max(paneUtils/* MIN_RATIO */.Zn, Math.min(paneUtils/* MAX_RATIO */.zN, ratio));
}
/** Get the first leaf of a subtree (for focus fallback) */ function getFirstLeaf(node) {
    if (node.type === "leaf") return node;
    return getFirstLeaf(node.first);
}
function paneReducer(state, action) {
    switch(action.type){
        case "SPLIT_PANE":
            {
                const { paneId, direction } = action;
                const target = (0,paneUtils/* findLeaf */.iZ)(state.root, paneId);
                if (!target) return state;
                // Guard: don't exceed max depth
                if ((0,paneUtils/* wouldExceedMaxDepth */.Xt)(state.root, paneId)) return state;
                const newLeaf = (0,paneUtils/* createLeaf */.fB)();
                const splitNode = {
                    type: "split",
                    id: (0,paneUtils/* generatePaneId */.xk)(),
                    direction,
                    ratio: 0.5,
                    first: target,
                    second: newLeaf
                };
                const newRoot = (0,paneUtils/* replaceNode */.fo)(state.root, paneId, splitNode);
                return {
                    ...state,
                    root: newRoot,
                    focusedPaneId: newLeaf.id
                };
            }
        case "CLOSE_PANE":
            {
                const { paneId } = action;
                // Special case: closing the root leaf
                if (state.root.type === "leaf") {
                    if (state.root.id === paneId) {
                        return (0,paneUtils/* initialPaneState */.un)();
                    }
                    return state;
                }
                const parentSplit = (0,paneUtils/* findParentSplit */.gy)(state.root, paneId);
                if (!parentSplit) return state;
                const sibling = parentSplit.first.id === paneId ? parentSplit.second : parentSplit.first;
                const newRoot = (0,paneUtils/* replaceNode */.fo)(state.root, parentSplit.id, sibling);
                // Focus should move to the sibling — find the first leaf of sibling
                const siblingFocus = getFirstLeaf(sibling);
                return {
                    ...state,
                    root: newRoot,
                    focusedPaneId: siblingFocus.id
                };
            }
        case "RESIZE_PANE":
            {
                const { splitId, ratio } = action;
                const target = (0,paneUtils/* findSplit */.mJ)(state.root, splitId);
                if (!target) return state;
                const clamped = paneReducer_clampRatio(ratio);
                const updated = {
                    ...target,
                    ratio: clamped
                };
                const newRoot = (0,paneUtils/* replaceNode */.fo)(state.root, splitId, updated);
                return {
                    ...state,
                    root: newRoot
                };
            }
        case "FOCUS_PANE":
            {
                const { paneId } = action;
                // Only update if the pane actually exists
                const leaf = (0,paneUtils/* findLeaf */.iZ)(state.root, paneId);
                if (!leaf) return state;
                return {
                    ...state,
                    focusedPaneId: paneId
                };
            }
        case "ASSIGN_SESSION":
            {
                var _allLeaves_find;
                const { paneId, sessionId } = action;
                const allLeaves = (0,paneUtils/* getAllLeaves */.eH)(state.root);
                var _allLeaves_find_id;
                // Move-and-clear: if this session is already in another pane, clear it from there
                // first so the same session is never visible in two panes simultaneously.
                const sourcePaneId = (_allLeaves_find_id = (_allLeaves_find = allLeaves.find((l)=>l.id !== paneId && l.sessionId === sessionId)) === null || _allLeaves_find === void 0 ? void 0 : _allLeaves_find.id) !== null && _allLeaves_find_id !== void 0 ? _allLeaves_find_id : null;
                const target = (0,paneUtils/* findLeaf */.iZ)(state.root, paneId);
                if (!target) return state;
                // Step 1: clear the source pane (atomic — uses the immutable replaceNode chain)
                let newRoot = state.root;
                if (sourcePaneId) {
                    const sourceLeaf = (0,paneUtils/* findLeaf */.iZ)(newRoot, sourcePaneId);
                    newRoot = (0,paneUtils/* replaceNode */.fo)(newRoot, sourcePaneId, {
                        ...sourceLeaf,
                        sessionId: null
                    });
                }
                // Step 2: assign to the target pane.
                // If the target is a session-list pane, auto-split to create a detail pane.
                if (target.viewKind === "session-list" && !(0,paneUtils/* wouldExceedMaxDepth */.Xt)(newRoot, paneId)) {
                    const newDetail = (0,paneUtils/* createLeaf */.fB)(undefined, "terminal", "session-detail");
                    const detailWithSession = {
                        ...newDetail,
                        sessionId
                    };
                    const splitNode = {
                        type: "split",
                        id: (0,paneUtils/* generatePaneId */.xk)(),
                        direction: "vertical",
                        ratio: 0.35,
                        first: target,
                        second: detailWithSession
                    };
                    newRoot = (0,paneUtils/* replaceNode */.fo)(newRoot, paneId, splitNode);
                    return {
                        ...state,
                        root: newRoot,
                        focusedPaneId: detailWithSession.id
                    };
                }
                const updated = {
                    ...target,
                    sessionId,
                    activeTab: "terminal"
                };
                newRoot = (0,paneUtils/* replaceNode */.fo)(newRoot, paneId, updated);
                return {
                    ...state,
                    root: newRoot
                };
            }
        case "ASSIGN_TAB":
            {
                const { paneId, tab } = action;
                const target = (0,paneUtils/* findLeaf */.iZ)(state.root, paneId);
                if (!target) return state;
                const updated = {
                    ...target,
                    activeTab: tab
                };
                const newRoot = (0,paneUtils/* replaceNode */.fo)(state.root, paneId, updated);
                return {
                    ...state,
                    root: newRoot
                };
            }
        case "ZOOM_PANE":
            {
                const { paneId } = action;
                if (paneId === null) {
                    return {
                        ...state,
                        zoomedPaneId: null
                    };
                }
                // Toggle: if same pane is already zoomed, clear it
                if (state.zoomedPaneId === paneId) {
                    return {
                        ...state,
                        zoomedPaneId: null
                    };
                }
                return {
                    ...state,
                    zoomedPaneId: paneId
                };
            }
        case "NUDGE_RESIZE":
            {
                const { paneId, direction, amountPx, containerSizePx } = action;
                if (containerSizePx <= 0) return state;
                const ancestorSplit = (0,paneUtils/* findNearestAncestorSplit */.v2)(state.root, paneId, direction);
                if (!ancestorSplit) return state;
                const delta = amountPx / containerSizePx;
                // If focused pane is in the first subtree and going right/down, increase ratio
                const focusedInFirst = (0,paneUtils/* containsPaneId */.$o)(ancestorSplit.first, paneId);
                const increase = direction === "ArrowRight" || direction === "ArrowDown";
                const sign = focusedInFirst && increase || !focusedInFirst && !increase ? 1 : -1;
                const newRatio = paneReducer_clampRatio(ancestorSplit.ratio + sign * delta);
                const updated = {
                    ...ancestorSplit,
                    ratio: newRatio
                };
                const newRoot = (0,paneUtils/* replaceNode */.fo)(state.root, ancestorSplit.id, updated);
                return {
                    ...state,
                    root: newRoot
                };
            }
        case "SET_PANE_VIEW":
            {
                const { paneId, viewKind } = action;
                const target = (0,paneUtils/* findLeaf */.iZ)(state.root, paneId);
                if (!target) return state;
                const updated = {
                    ...target,
                    viewKind,
                    // session-list panes don't display a specific session
                    sessionId: viewKind === "session-list" ? null : target.sessionId
                };
                const newRoot = (0,paneUtils/* replaceNode */.fo)(state.root, paneId, updated);
                return {
                    ...state,
                    root: newRoot
                };
            }
        case "SWAP_PANES":
            {
                const { paneId, targetPaneId } = action;
                if (paneId === targetPaneId) return state;
                const newRoot = (0,paneUtils/* swapPanes */.Re)(state.root, paneId, targetPaneId);
                if (newRoot === state.root) return state;
                return {
                    ...state,
                    root: newRoot
                };
            }
        case "RESET_LAYOUT":
            {
                return (0,paneUtils/* initialPaneState */.un)();
            }
        case "RESTORE_LAYOUT":
            {
                return action.state;
            }
        case "SPLIT_AND_ASSIGN_SESSION":
            {
                const { paneId, sessionId, tab, direction = "vertical" } = action;
                const target = (0,paneUtils/* findLeaf */.iZ)(state.root, paneId);
                if (!target) return state;
                const allLeaves = (0,paneUtils/* getAllLeaves */.eH)(state.root);
                var _allLeaves_find1;
                // Move-and-clear: find the pane currently holding this session (if any).
                // Skip clearing if the source is the same pane being split (splitting the
                // pane that already holds the session is valid — the new child gets it).
                const sourceLeaf = (_allLeaves_find1 = allLeaves.find((l)=>l.sessionId === sessionId)) !== null && _allLeaves_find1 !== void 0 ? _allLeaves_find1 : null;
                var _sourceLeaf_id;
                const sourcePaneId = (_sourceLeaf_id = sourceLeaf === null || sourceLeaf === void 0 ? void 0 : sourceLeaf.id) !== null && _sourceLeaf_id !== void 0 ? _sourceLeaf_id : null;
                let newRoot = state.root;
                if (sourcePaneId && sourcePaneId !== paneId) {
                    const src = (0,paneUtils/* findLeaf */.iZ)(newRoot, sourcePaneId);
                    newRoot = (0,paneUtils/* replaceNode */.fo)(newRoot, sourcePaneId, {
                        ...src,
                        sessionId: null
                    });
                }
                // If max depth reached, fall back to assigning in place
                if ((0,paneUtils/* wouldExceedMaxDepth */.Xt)(newRoot, paneId)) {
                    const updated = {
                        ...target,
                        sessionId,
                        activeTab: tab,
                        viewKind: "session-detail"
                    };
                    newRoot = (0,paneUtils/* replaceNode */.fo)(newRoot, paneId, updated);
                    return {
                        ...state,
                        root: newRoot
                    };
                }
                const newLeaf = {
                    type: "leaf",
                    id: (0,paneUtils/* generatePaneId */.xk)(),
                    sessionId,
                    activeTab: tab,
                    viewKind: "session-detail"
                };
                const splitNode = {
                    type: "split",
                    id: (0,paneUtils/* generatePaneId */.xk)(),
                    direction,
                    ratio: 0.5,
                    first: target,
                    second: newLeaf
                };
                newRoot = (0,paneUtils/* replaceNode */.fo)(newRoot, paneId, splitNode);
                return {
                    ...state,
                    root: newRoot,
                    focusedPaneId: newLeaf.id
                };
            }
        default:
            {
                const _exhaustive = action;
                return state;
            }
    }
}
// Re-export helpers needed by tests and hooks


;// ./src/lib/window/windowReducer.ts

function createWindow(state, action) {
    const newWindow = {
        id: action.id,
        name: action.name,
        paneState: (0,paneUtils/* initialPaneState */.un)()
    };
    return {
        windows: [
            ...state.windows,
            newWindow
        ]
    };
}
function renameWindow(state, action) {
    if (action.name.trim() === "") return state;
    return {
        windows: state.windows.map((w)=>w.id === action.id ? {
                ...w,
                name: action.name
            } : w)
    };
}
function closeWindow(state, action) {
    const remaining = state.windows.filter((w)=>w.id !== action.id);
    if (remaining.length > 0) return {
        windows: remaining
    };
    // Every CLOSE_WINDOW carries a `replacement`, so an empty `windows` array
    // is unreachable — the reducer only branches on the length check above.
    const replacement = {
        id: action.replacement.id,
        name: action.replacement.name,
        paneState: (0,paneUtils/* initialPaneState */.un)()
    };
    return {
        windows: [
            replacement
        ]
    };
}
function delegatePaneAction(state, action) {
    return {
        windows: state.windows.map((w)=>w.id === action.windowId ? {
                ...w,
                paneState: paneReducer(w.paneState, action.action)
            } : w)
    };
}
function windowReducer(state, action) {
    switch(action.type){
        case "CREATE_WINDOW":
            return createWindow(state, action);
        case "RENAME_WINDOW":
            return renameWindow(state, action);
        case "CLOSE_WINDOW":
            return closeWindow(state, action);
        case "PANE_ACTION":
            return delegatePaneAction(state, action);
        case "RESTORE_WINDOWS":
            return {
                windows: action.windows
            };
        default:
            {
                const _exhaustive = action;
                throw new Error("Unhandled WindowAction: ".concat(JSON.stringify(_exhaustive)));
            }
    }
}

;// ./src/lib/window/windowUtils.ts


function generateWindowId() {
    return (0,paneUtils/* generateSecureId */.cg)().slice(0, 8);
}
function initialWindowsState() {
    const id = generateWindowId();
    return {
        windows: [
            {
                id,
                name: "Window 1",
                paneState: (0,paneUtils/* initialPaneState */.un)()
            }
        ]
    };
}

;// ./src/lib/window/windowPersistence.ts


const WINDOW_LAYOUT_KEY = "cockpit.windowLayout";
const LAST_FOCUSED_WINDOW_KEY = "cockpit.lastFocusedWindowId";
function isValidNamedWindow(value) {
    if (!value || typeof value !== "object") return false;
    const candidate = value;
    return typeof candidate.id === "string" && typeof candidate.name === "string" && !!candidate.paneState && typeof candidate.paneState === "object";
}
/**
 * Shape-validate a parsed v2 payload before trusting it — checked identically
 * on both load (loadWindowLayout) and cross-tab adoption (the `storage`
 * listener in useWindowManager.ts), so a malformed write from either source
 * can't reach the reducer/PaneTilingContainer, which assume every window has
 * a real id/name/paneState.
 */ function isValidV2Layout(value) {
    if (!value || typeof value !== "object") return false;
    const candidate = value;
    return candidate.version === 2 && typeof candidate.revision === "number" && Array.isArray(candidate.windows) && // An empty windows array is unrepresentable in this app (windowReducer.ts
    // never produces one — every CLOSE_WINDOW carries a replacement) and
    // page.tsx's `windows.find(...) ?? windows[0]` fallback assumes at least
    // one exists, so trusting one here would crash every render.
    candidate.windows.length > 0 && candidate.windows.every(isValidNamedWindow);
}
/** Plain write-through helper shared by migration and the guarded save. */ function saveWindowLayoutRaw(layout) {
    try {
        localStorage.setItem(WINDOW_LAYOUT_KEY, JSON.stringify(layout));
    } catch (e) {
    // localStorage may be unavailable (private mode, quota exceeded, SSR)
    }
}
/**
 * Wrap the existing v1 `cockpit.paneLayout` (if any) into a single "Window 1"
 * and write it through to the v2 key. Never touches/deletes `cockpit.paneLayout`
 * (ADR-003) so the migration is non-destructive even though it is irreversible.
 */ const PANE_LAYOUT_KEY = "cockpit.paneLayout";
function migrateFromV1() {
    const legacy = loadPaneLayout();
    if (!legacy) {
        // Distinguish "nothing to migrate" (fresh user, no log needed) from
        // "a v1 blob exists but failed validation" (corrupt, fail closed + log).
        if (localStorage.getItem(PANE_LAYOUT_KEY) !== null) {
            console.error("windowPersistence: cockpit.paneLayout is present but failed validation, migration fell back to null");
        }
        return null;
    }
    const migrated = {
        version: 2,
        revision: 0,
        windows: [
            {
                id: generateWindowId(),
                name: "Window 1",
                paneState: {
                    root: legacy.root,
                    focusedPaneId: legacy.focusedPaneId,
                    zoomedPaneId: legacy.zoomedPaneId
                }
            }
        ]
    };
    saveWindowLayoutRaw(migrated);
    return migrated;
}
/**
 * Resolution order (Migration Plan): a shape-valid v2 key wins outright;
 * anything else (missing, corrupt, unrecognized version) falls through to a
 * one-time v1 migration; if that also has nothing, the caller gets `null`.
 */ function loadWindowLayout() {
    let parsed;
    try {
        const stored = localStorage.getItem(WINDOW_LAYOUT_KEY);
        if (!stored) return migrateFromV1();
        parsed = JSON.parse(stored);
    } catch (e) {
        console.error("windowPersistence: failed to parse cockpit.windowLayout, falling back to migration");
        return migrateFromV1();
    }
    const version = parsed === null || parsed === void 0 ? void 0 : parsed.version;
    switch(version){
        case 2:
            if (isValidV2Layout(parsed)) {
                return parsed;
            }
            console.error("windowPersistence: cockpit.windowLayout has version 2 but invalid shape, falling back to migration");
            return migrateFromV1();
        default:
            console.error("windowPersistence: unrecognized cockpit.windowLayout version (".concat(String(version), "), falling back to migration"));
            return migrateFromV1();
    }
}
/**
 * Optimistic Offline Lock: re-reads the current stored value fresh and only
 * writes if `lastKnownRevision` still matches (ADR-002). This is a
 * read-check-write, not an atomic compare-and-swap: it detects a conflict
 * whenever the other tab's write has already landed by the time this tab
 * re-reads, but two saves whose reads both happen to land before either
 * write commits can still race past the check undetected — an accepted,
 * narrow gap given the alternative (a pessimistic lock) was ruled out of
 * scope in ADR-002's Alternatives Considered.
 */ function saveWindowLayout(windows, lastKnownRevision) {
    try {
        const stored = localStorage.getItem(WINDOW_LAYOUT_KEY);
        if (stored) {
            const parsed = JSON.parse(stored);
            if (isValidV2Layout(parsed) && parsed.revision !== lastKnownRevision) {
                console.error("windowPersistence: stale save rejected (lastKnownRevision=".concat(lastKnownRevision, ", stored=").concat(parsed.revision, ")"));
                return {
                    status: "conflict",
                    latest: parsed
                };
            }
        }
        const nextRevision = lastKnownRevision + 1;
        const layout = {
            version: 2,
            revision: nextRevision,
            windows
        };
        saveWindowLayoutRaw(layout);
        return {
            status: "ok",
            revision: nextRevision
        };
    } catch (e) {
        return {
            status: "error"
        };
    }
}
/** Single scalar, last-write-wins by construction — bypasses the revision guard entirely. */ function getLastFocusedWindowId() {
    try {
        return localStorage.getItem(LAST_FOCUSED_WINDOW_KEY);
    } catch (e) {
        return null;
    }
}
function setLastFocusedWindowId(id) {
    try {
        localStorage.setItem(LAST_FOCUSED_WINDOW_KEY, id);
    } catch (e) {
    // localStorage may be unavailable (private mode, quota exceeded, SSR)
    }
}

;// ./src/lib/window/useWindowManager.ts
/* __next_internal_client_entry_do_not_use__ useWindowManager auto */ 






const useWindowManager_WINDOW_LAYOUT_KEY = "cockpit.windowLayout";
/** The window id a tab viewing `closedId` should switch to, given the windows array *before* removal. */ function resolveReplacementViewId(windows, closedId, replacementId) {
    const currentIndex = windows.findIndex((w)=>w.id === closedId);
    const remaining = windows.filter((w)=>w.id !== closedId);
    if (remaining.length === 0) return replacementId;
    if (currentIndex >= 0 && currentIndex < windows.length - 1) {
        return windows[currentIndex + 1].id;
    }
    return remaining[remaining.length - 1].id;
}
/** True if any leaf in the window's pane tree has a sessionId not present in validIds. */ function hasStaleSessionId(window1, validIds) {
    return (0,paneUtils/* getAllLeaves */.eH)(window1.paneState.root).some((l)=>l.sessionId !== null && !validIds.has(l.sessionId));
}
/**
 * Repair a single restored window's pane tree — the per-window equivalent of
 * usePaneReducer.ts's old restore-time repair, which this restore path
 * dropped when the reducer moved up a level (only the separate
 * sessions-changed effect below re-runs validateAndRepair; nothing repairs
 * the *restore* path itself). Mirrors that logic exactly:
 * - session ids are only nulled once sessions have actually loaded, so a
 *   restore that races the initial (empty) sessions response doesn't wipe
 *   every saved assignment against an empty validIds set.
 * - a pre-tiling layout (no session-list leaf — from a v1 migration or
 *   corrupt localStorage) resets just this window to the default split
 *   rather than leaving a permanently broken pane tree.
 * - focusedPaneId/zoomedPaneId are repointed/cleared if they no longer name
 *   a leaf that exists after repair.
 */ function repairWindow(win, sessionsLoaded, validIds) {
    var _allLeaves_;
    const repairedRoot = sessionsLoaded ? validateAndRepair(win.paneState.root, validIds) : win.paneState.root;
    const allLeaves = (0,paneUtils/* getAllLeaves */.eH)(repairedRoot);
    if (!allLeaves.some((l)=>l.viewKind === "session-list")) {
        return {
            ...win,
            paneState: (0,paneUtils/* initialPaneState */.un)()
        };
    }
    var _allLeaves__id;
    const focusedPaneId = allLeaves.some((l)=>l.id === win.paneState.focusedPaneId) ? win.paneState.focusedPaneId : (_allLeaves__id = (_allLeaves_ = allLeaves[0]) === null || _allLeaves_ === void 0 ? void 0 : _allLeaves_.id) !== null && _allLeaves__id !== void 0 ? _allLeaves__id : win.paneState.focusedPaneId;
    const zoomedPaneId = win.paneState.zoomedPaneId !== null && (0,paneUtils/* findLeaf */.iZ)(repairedRoot, win.paneState.zoomedPaneId) ? win.paneState.zoomedPaneId : null;
    return {
        ...win,
        paneState: {
            root: repairedRoot,
            focusedPaneId,
            zoomedPaneId
        }
    };
}
/**
 * Re-validate every window's pane tree against the current valid session ids.
 * Only windows that actually have a stale sessionId get a new object identity
 * — validateAndRepair itself always returns a fresh object, so gating on its
 * result by reference would treat every call as "changed".
 */ function revalidateWindows(windows, validIds) {
    let changed = false;
    const nextWindows = windows.map((w)=>{
        if (!hasStaleSessionId(w, validIds)) return w;
        changed = true;
        return {
            ...w,
            paneState: {
                ...w.paneState,
                root: validateAndRepair(w.paneState.root, validIds)
            }
        };
    });
    return {
        windows: nextWindows,
        changed
    };
}
/**
 * useWindowManager — owns windowReducer state, localStorage persistence, and
 * cross-tab reconciliation, mirroring usePaneReducer.ts's restore/debounce-save/
 * re-validate shape but across an array of windows (see plan.md Epic 1.4).
 *
 * Deliberately has no notion of "the active window" — that stays in
 * useWindowUrlSync (per-tab URL binding, ADR-001).
 */ function useWindowManager(sessions) {
    const [state, dispatch] = (0,react.useReducer)(windowReducer, undefined, initialWindowsState);
    // Reactive counterpart to restoredRef below — consumers (useWindowUrlSync)
    // need to *re-render* once restoration completes, which a ref alone can't
    // trigger. `state.windows` before this flips true is a throwaway
    // placeholder from initialWindowsState(), not real persisted data.
    const [isRestored, setIsRestored] = (0,react.useState)(false);
    const restoredRef = (0,react.useRef)(false);
    const sessionsLoadedRef = (0,react.useRef)(false);
    const saveTimerRef = (0,react.useRef)(null);
    const lastKnownRevisionRef = (0,react.useRef)(0);
    // `saveWindowLayout` unconditionally bumps the revision on every call, even
    // when `windows` is byte-for-byte what's already stored — it has no "is this
    // actually a change" check. Every RESTORE_WINDOWS dispatch below (initial
    // load, cross-tab storage-event sync, conflict recovery) changes `state`'s
    // object identity, which would otherwise re-trigger the debounced-save
    // effect and vacuously re-save an echo of data that's already persisted.
    // Two tabs each doing this on mount race to bump the same revision counter,
    // and whichever save loses that race gets its *real* edit (e.g. a rename)
    // rejected as a stale write and silently discarded via RESTORE_WINDOWS.
    //
    // This holds the exact `windows` array reference passed into the next
    // RESTORE_WINDOWS dispatch, set immediately before that dispatch. It is
    // NOT a "skip the next save" boolean — a boolean is consumed by the save
    // effect's run against the *current* (pre-dispatch) state in the same
    // commit, before the dispatched state actually lands, so by the time the
    // restored state arrives the flag is already spent and the echo save
    // fires anyway. Comparing against the actual array reference instead
    // ties the skip decision to the state that needs skipping — the reducer's
    // RESTORE_WINDOWS case passes `action.windows` straight through, so
    // `state.windows` is reference-equal to this exact array once it lands,
    // regardless of how many effect passes happen in between.
    const skipWindowsRef = (0,react.useRef)(null);
    // web-app/next.config.ts sets reactStrictMode: true, so dev builds run every
    // effect twice per commit (mount → cleanup → mount again) to surface missing
    // cleanup logic. Both invocations share the exact same `state` reference —
    // guarding on `state` identity is idempotent across the duplicate
    // invocation, since `state` doesn't change between them.
    const lastHandledStateRef = (0,react.useRef)(null);
    // Track the first moment sessions arrive from the server (mirrors usePaneReducer.ts).
    (0,react.useEffect)(()=>{
        if (sessions !== null && sessions.length > 0) {
            sessionsLoadedRef.current = true;
        }
    }, [
        sessions
    ]);
    // Restore layout once sessions first become available (Task 1.4.1a).
    (0,react.useEffect)(()=>{
        if (sessions === null) return;
        if (restoredRef.current) return;
        restoredRef.current = true;
        const layout = loadWindowLayout();
        if (layout) {
            const validIds = new Set(sessions.map((s)=>s.id));
            const repairedWindows = layout.windows.map((w)=>repairWindow(w, sessionsLoadedRef.current, validIds));
            lastKnownRevisionRef.current = layout.revision;
            skipWindowsRef.current = repairedWindows;
            dispatch({
                type: "RESTORE_WINDOWS",
                windows: repairedWindows
            });
        } else {
            lastKnownRevisionRef.current = 0;
            const windows = initialWindowsState().windows;
            skipWindowsRef.current = windows;
            dispatch({
                type: "RESTORE_WINDOWS",
                windows
            });
        }
        setIsRestored(true);
    }, [
        sessions
    ]);
    // Debounced save (300ms, matches usePaneReducer.ts) through the revision guard (Task 1.4.1b).
    (0,react.useEffect)(()=>{
        if (!restoredRef.current) return;
        if (lastHandledStateRef.current === state) return;
        lastHandledStateRef.current = state;
        if (saveTimerRef.current) {
            clearTimeout(saveTimerRef.current);
        }
        if (skipWindowsRef.current === state.windows) {
            skipWindowsRef.current = null;
            return;
        }
        saveTimerRef.current = setTimeout(()=>{
            const result = saveWindowLayout(state.windows, lastKnownRevisionRef.current);
            if (result.status === "ok") {
                lastKnownRevisionRef.current = result.revision;
            } else if (result.status === "conflict") {
                lastKnownRevisionRef.current = result.latest.revision;
                skipWindowsRef.current = result.latest.windows;
                dispatch({
                    type: "RESTORE_WINDOWS",
                    windows: result.latest.windows
                });
            } else {
                console.error("useWindowManager: saveWindowLayout failed");
            }
        }, 300);
        return ()=>{
            if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
        };
    }, [
        state
    ]);
    // Cross-tab reconciliation via the storage event (Task 1.4.1c).
    (0,react.useEffect)(()=>{
        function handleStorage(event) {
            if (event.key !== useWindowManager_WINDOW_LAYOUT_KEY) return;
            if (!event.newValue) return;
            try {
                const parsed = JSON.parse(event.newValue);
                if (!isValidV2Layout(parsed)) {
                    console.error("useWindowManager: storage event newValue failed shape validation, ignoring");
                    return;
                }
                if (parsed.revision <= lastKnownRevisionRef.current) return;
                // A save this tab already scheduled (e.g. a rename not yet 300ms
                // old) is about to be superseded wholesale by another tab's write —
                // ADR-002 frames a discarded edit as "known, logged", but only the
                // saveWindowLayout conflict path actually logged it; this is the
                // other path that discards an edit (never attempts the save at all).
                if (saveTimerRef.current) {
                    console.error("useWindowManager: discarding a pending local edit — a newer cross-tab write arrived first");
                }
                lastKnownRevisionRef.current = parsed.revision;
                skipWindowsRef.current = parsed.windows;
                dispatch({
                    type: "RESTORE_WINDOWS",
                    windows: parsed.windows
                });
            } catch (e) {
                console.error("useWindowManager: failed to parse storage event newValue");
            }
        }
        window.addEventListener("storage", handleStorage);
        return ()=>window.removeEventListener("storage", handleStorage);
    }, []);
    // Re-validate every window (not just the currently-viewed one) when sessions
    // change (Task 1.4.1d) — background windows keep stale session ids around
    // otherwise (research/features.md).
    (0,react.useEffect)(()=>{
        if (sessions === null) return;
        if (!restoredRef.current) return;
        if (!sessionsLoadedRef.current) return;
        const validIds = new Set(sessions.map((s)=>s.id));
        const { windows: nextWindows, changed } = revalidateWindows(state.windows, validIds);
        if (!changed) return;
        dispatch({
            type: "RESTORE_WINDOWS",
            windows: nextWindows
        });
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessions
    ]);
    const dispatchPane = (0,react.useCallback)((windowId, action)=>{
        dispatch({
            type: "PANE_ACTION",
            windowId,
            action
        });
    }, []);
    const createWindow = (0,react.useCallback)((name)=>{
        const id = generateWindowId();
        const resolvedName = name !== null && name !== void 0 ? name : "Window ".concat(state.windows.length + 1);
        dispatch({
            type: "CREATE_WINDOW",
            id,
            name: resolvedName
        });
        return id;
    }, [
        state.windows.length
    ]);
    const closeWindow = (0,react.useCallback)((id)=>{
        const replacement = {
            id: generateWindowId(),
            name: "Window ".concat(state.windows.length + 1)
        };
        const nextViewId = resolveReplacementViewId(state.windows, id, replacement.id);
        dispatch({
            type: "CLOSE_WINDOW",
            id,
            replacement
        });
        return nextViewId;
    }, [
        state.windows
    ]);
    const renameWindow = (0,react.useCallback)((id, name)=>{
        dispatch({
            type: "RENAME_WINDOW",
            id,
            name
        });
    }, []);
    return {
        windows: state.windows,
        isRestored,
        dispatchPane,
        createWindow,
        closeWindow,
        renameWindow
    };
}

;// ./src/lib/window/useWindowUrlSync.ts



const WINDOW_PARAM = "window";
/**
 * Resolution order per ADR-001: URL `?window=` param (if it names an
 * existing window) → `lastFocusedWindowId` hint (if it names an existing
 * window) → the first window in array order.
 */ function resolveCurrentWindowId(paramWindowId, windows) {
    var _windows_;
    if (paramWindowId !== null && windows.some((w)=>w.id === paramWindowId)) {
        return paramWindowId;
    }
    const lastFocused = getLastFocusedWindowId();
    if (lastFocused !== null && windows.some((w)=>w.id === lastFocused)) {
        return lastFocused;
    }
    var _windows__id;
    return (_windows__id = (_windows_ = windows[0]) === null || _windows_ === void 0 ? void 0 : _windows_.id) !== null && _windows__id !== void 0 ? _windows__id : "";
}
/**
 * Resolves this tab's `currentWindowId` from the URL (ADR-001: the active
 * window is per-tab and derived from `?window=`, never a shared reducer
 * field), self-healing the URL when the param is missing or names a window
 * closed by another tab.
 *
 * `isRestored` must reflect `useWindowManager`'s own restore-completion flag,
 * not just "some windows array exists" — before restore, `windows` is a
 * throwaway placeholder (`initialWindowsState()`, a single window with a
 * freshly generated random id) that a real `?window=<id>` param will
 * legitimately never match. Without this guard, the self-heal effect below
 * would "correct" that placeholder mismatch by overwriting a perfectly valid
 * incoming URL param before the real windows list has even loaded — and once
 * it later loads, this tab is left resolved onto whatever `lastFocusedWindowId`
 * happens to be (e.g. another tab's most recent selection), not the window the
 * URL originally named.
 */ function useWindowUrlSync(windows, isRestored) {
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const router = (0,nav/* useRouter */.KA)();
    const paramWindowId = searchParams.get(WINDOW_PARAM);
    const currentWindowId = resolveCurrentWindowId(paramWindowId, windows);
    (0,react.useEffect)(()=>{
        if (!isRestored) return;
        if (paramWindowId === currentWindowId) return;
        const params = new URLSearchParams(searchParams.toString());
        if (currentWindowId) {
            params.set(WINDOW_PARAM, currentWindowId);
        } else {
            params.delete(WINDOW_PARAM);
        }
        const query = params.toString();
        router.replace(query ? "/?".concat(query) : "/", {
            resetScroll: false
        });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- searchParams identity changes every render; comparing paramWindowId/currentWindowId values is sufficient to avoid redundant replace calls
    }, [
        paramWindowId,
        currentWindowId,
        isRestored
    ]);
    const switchToWindow = (0,react.useCallback)((id)=>{
        setLastFocusedWindowId(id);
        const params = new URLSearchParams(searchParams.toString());
        params.set(WINDOW_PARAM, id);
        router.replace("/?".concat(params.toString()), {
            resetScroll: false
        });
    }, [
        router,
        searchParams
    ]);
    return {
        currentWindowId,
        switchToWindow
    };
}

// EXTERNAL MODULE: ./src/lib/shortcuts/shortcutRegistry.ts
var shortcutRegistry = __webpack_require__(8569);
// EXTERNAL MODULE: ./src/lib/shortcuts/commandBarController.ts
var commandBarController = __webpack_require__(4942);
;// ./src/lib/window/useWindowShortcuts.ts
/* __next_internal_client_entry_do_not_use__ useWindowShortcuts auto */ 



/**
 * useWindowShortcuts — tmux-style leader-key window switcher.
 *
 * Alt+W arms a leader mode for LEADER_TIMEOUT_MS, during which 1-9/n/p/,/Escape
 * are dynamically registered as follow-ups (Task 3.3.1a/b). Alt+W was chosen
 * over any Ctrl-based leader (Ctrl+B, tmux's actual default prefix, or Ctrl+A,
 * its common screen/remap alternative) specifically because this app displays
 * live tmux-backed terminal sessions in panes: a Ctrl-based leader risks arming
 * this app's outer leader mode whenever the user sends a real tmux prefix
 * keystroke into a pane, silently hijacking their very next keystroke before it
 * reaches the terminal (see pre-mortem.md P1 #1 / Failure Mode #1). Alt sidesteps
 * that whole collision class rather than trading one Ctrl+<letter> for another
 * that might also be someone's tmux remap.
 *
 * Task 3.3.2a — manually testing Alt+W across Chrome/Firefox/Edge/Safari x
 * Windows/macOS/Linux for Alt-as-menu-accelerator or other browser-chrome/OS
 * reservations — is a REQUIRED human follow-up step that could NOT be
 * performed here: this implementation ran in a sandboxed coding environment
 * with no real browser/OS matrix available. Alt+W is left registered as the
 * plan's chosen default below, NOT yet verified collision-free. If that manual
 * spike finds a collision on any target combination, the fallback is
 * Alt+Shift+W — the same escape-hatch pattern usePaneShortcuts.ts already uses
 * for Ctrl+- (documented fallback: Ctrl+Shift+H) — and the only change needed
 * is the `key`/`modifiers` passed to the `useShortcut("window.leader", ...)`
 * call below.
 */ const LEADER_TIMEOUT_MS = 3000;
const DIGIT_KEYS = [
    "1",
    "2",
    "3",
    "4",
    "5",
    "6",
    "7",
    "8",
    "9"
];
/** Every key the leader recognizes as a follow-up once armed. */ const FOLLOWUP_KEYS = new Set([
    ...DIGIT_KEYS,
    "n",
    "p",
    ",",
    "Escape"
]);
function disarmLeader(refs) {
    for (const cleanup of refs.followupCleanups.current)cleanup();
    refs.followupCleanups.current = [];
    if (refs.timeout.current !== null) {
        clearTimeout(refs.timeout.current);
        refs.timeout.current = null;
    }
    if (refs.catchAllListener.current !== null) {
        document.removeEventListener("keydown", refs.catchAllListener.current, true);
        refs.catchAllListener.current = null;
    }
}
function switchToOneIndexed(refs, oneIndexedPosition) {
    const target = refs.windows.current[oneIndexedPosition - 1];
    if (target) refs.switchToWindow.current(target.id);
}
function cycle(refs, delta) {
    const list = refs.windows.current;
    if (list.length === 0) return;
    const currentIndex = list.findIndex((w)=>w.id === refs.currentWindowId.current);
    const base = currentIndex === -1 ? 0 : currentIndex;
    const nextIndex = (base + delta + list.length) % list.length;
    refs.switchToWindow.current(list[nextIndex].id);
}
function registerFollowup(refs, id, key, action) {
    refs.followupCleanups.current.push(shortcutRegistry/* registry */.u5.register(id, {
        key,
        label: "Window leader: ".concat(key),
        context: "global",
        hidden: true,
        action: ()=>{
            action();
            disarmLeader(refs);
        }
    }));
}
function registerDigitFollowups(refs) {
    DIGIT_KEYS.forEach((key, index)=>{
        const oneIndexedPosition = index + 1;
        registerFollowup(refs, "window.leader.".concat(key), key, ()=>switchToOneIndexed(refs, oneIndexedPosition));
    });
}
function registerNavigationFollowups(refs) {
    registerFollowup(refs, "window.leader.next", "n", ()=>cycle(refs, 1));
    registerFollowup(refs, "window.leader.prev", "p", ()=>cycle(refs, -1));
    registerFollowup(refs, "window.leader.rename", ",", ()=>refs.onRenameRequest.current(refs.currentWindowId.current));
    // Escape disarms with no other action.
    registerFollowup(refs, "window.leader.cancel", "Escape", ()=>{});
}
function armCatchAllListener(refs) {
    // Watches every keydown while armed, purely to catch an unrecognized
    // follow-up key and disarm before the registry's own bubble-phase dispatch
    // runs (capture phase always fires first). Deregistering the follow-ups
    // here means the registry's dispatch loop then finds nothing to match, so
    // it never calls preventDefault -- the keystroke is left unconsumed for
    // whatever else (e.g. a terminal pane) would normally receive it.
    const onAnyKeyWhileArmed = (e)=>{
        // A followup-shaped keystroke (e.g. "1" or "n") typed into a focused
        // input/textarea/contenteditable never reaches a followup action —
        // ShortcutRegistry.dispatch's own isInputElement guard no-ops it — but
        // without this check it also wouldn't disarm here, leaving the leader
        // (and its registry entries for every digit/n/p/,/Escape) armed for up
        // to LEADER_TIMEOUT_MS after the user has moved on to typing elsewhere.
        const hasModifier = e.metaKey || e.ctrlKey || e.altKey;
        const typedIntoInput = !hasModifier && (0,shortcutRegistry/* isInputElement */.B6)(e.target);
        if (typedIntoInput || !FOLLOWUP_KEYS.has(e.key)) {
            disarmLeader(refs);
        }
    };
    refs.catchAllListener.current = onAnyKeyWhileArmed;
    document.addEventListener("keydown", onAnyKeyWhileArmed, true);
}
function armLeader(refs) {
    // A second Alt+W while already armed starts a fresh sequence rather than
    // stacking duplicate registrations.
    disarmLeader(refs);
    registerDigitFollowups(refs);
    registerNavigationFollowups(refs);
    armCatchAllListener(refs);
    refs.timeout.current = setTimeout(()=>disarmLeader(refs), LEADER_TIMEOUT_MS);
}
/**
 * Builds the LeaderRefs bundle. Rebuilt each render, but every property
 * points at the same persistent ref object across renders, so closures
 * capturing the returned bundle never go stale.
 */ function useLeaderRefs(windows, currentWindowId, switchToWindow, onRenameRequest) {
    const windowsRef = (0,react.useRef)(windows);
    windowsRef.current = windows;
    const currentWindowIdRef = (0,react.useRef)(currentWindowId);
    currentWindowIdRef.current = currentWindowId;
    const switchToWindowRef = (0,react.useRef)(switchToWindow);
    switchToWindowRef.current = switchToWindow;
    const onRenameRequestRef = (0,react.useRef)(onRenameRequest);
    onRenameRequestRef.current = onRenameRequest;
    const followupCleanupsRef = (0,react.useRef)([]);
    const timeoutRef = (0,react.useRef)(null);
    const catchAllListenerRef = (0,react.useRef)(null);
    return {
        windows: windowsRef,
        currentWindowId: currentWindowIdRef,
        switchToWindow: switchToWindowRef,
        onRenameRequest: onRenameRequestRef,
        followupCleanups: followupCleanupsRef,
        timeout: timeoutRef,
        catchAllListener: catchAllListenerRef
    };
}
function useWindowShortcuts(windows, currentWindowId, switchToWindow, onRenameRequest) {
    const refs = useLeaderRefs(windows, currentWindowId, switchToWindow, onRenameRequest);
    // Alt+W is an alias for the command bar's `w` branch. The legacy leader
    // below only arms when no bar is mounted (or a modal is open), which keeps
    // this hook usable standalone (and its unit tests meaningful).
    // eslint-disable-next-line react-hooks/exhaustive-deps
    const handleArmLeader = (0,react.useCallback)(()=>{
        if (!commandBarController/* commandBarController */.T.open([
            "w"
        ])) armLeader(refs);
    }, []);
    (0,react.useEffect)(()=>{
        return ()=>disarmLeader(refs);
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    // The same actions as sequence bindings, so the command bar lists them under `w`.
    (0,react.useEffect)(()=>{
        const defs = [
            ...DIGIT_KEYS.map((key, i)=>[
                    "window.seq.".concat(key),
                    "Switch to window ".concat(key),
                    [
                        "w",
                        key
                    ],
                    ()=>switchToOneIndexed(refs, i + 1)
                ]),
            [
                "window.seq.next",
                "Next window",
                [
                    "w",
                    "n"
                ],
                ()=>cycle(refs, 1)
            ],
            [
                "window.seq.prev",
                "Previous window",
                [
                    "w",
                    "p"
                ],
                ()=>cycle(refs, -1)
            ],
            [
                "window.seq.rename",
                "Rename window",
                [
                    "w",
                    ","
                ],
                ()=>refs.onRenameRequest.current(refs.currentWindowId.current)
            ]
        ];
        const cleanups = defs.map((param)=>{
            let [id, label, sequence, action] = param;
            return shortcutRegistry/* registry */.u5.register(id, {
                label,
                context: "global",
                sequence,
                group: "Windows",
                action
            });
        });
        return ()=>cleanups.forEach((c)=>c());
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    (0,useShortcut/* useShortcut */.w)("window.leader", {
        key: "w",
        code: "KeyW",
        modifiers: {
            alt: true
        },
        label: "Window leader key",
        context: "global",
        action: handleArmLeader
    });
}

// EXTERNAL MODULE: ./src/lib/window/windowGestureConstants.ts
var windowGestureConstants = __webpack_require__(2113);
;// ./src/lib/window/useWindowSwipe.ts
/* __next_internal_client_entry_do_not_use__ useWindowSwipe auto */ 

// A touch starting within this many px of either viewport edge is ignored
// entirely, so iOS Safari's native back/forward edge-swipe keeps working.
const EDGE_AVOIDANCE_PX = 30;
const SWIPE_DISTANCE_THRESHOLD_PX = 60;
const SWIPE_TIME_THRESHOLD_MS = 300;
function isInEdgeZone(clientX) {
    return clientX < EDGE_AVOIDANCE_PX || clientX > window.innerWidth - EDGE_AVOIDANCE_PX;
}
function classifyAxis(dx, dy) {
    return Math.abs(dx) > Math.abs(dy) ? "horizontal" : "vertical";
}
// Fires only when every guard agrees: horizontal axis, past the
// distance/time thresholds, and — if the strip can actually scroll — its
// scrollLeft never moved during the gesture (pre-mortem.md P2 #5).
function shouldFireSwipe(params) {
    const { axis, dx, dy, elapsedMs, isOverflowing, scrollMoved } = params;
    const isHorizontal = axis === null ? classifyAxis(dx, dy) === "horizontal" : axis === "horizontal";
    if (!isHorizontal) return false;
    if (Math.abs(dx) <= SWIPE_DISTANCE_THRESHOLD_PX) return false;
    if (elapsedMs >= SWIPE_TIME_THRESHOLD_MS) return false;
    return !(isOverflowing && scrollMoved);
}
/**
 * Detects a horizontal swipe on `containerRef` and calls `onSwipe("next" | "prev")`.
 *
 * Three things must all agree before firing, per plan.md Epic 3.2 / pre-mortem.md
 * P2 #5: the touch didn't start in the edge-avoidance zone, the gesture axis-locked
 * to horizontal (so a vertical scroll is never misclassified), and — when the
 * container itself is horizontally overflowing — its `scrollLeft` never actually
 * moved (so scrolling the strip to reveal off-screen tabs never also switches
 * the active window).
 */ function useWindowSwipe(containerRef, options) {
    const onSwipeRef = (0,react.useRef)(options.onSwipe);
    (0,react.useEffect)(()=>{
        onSwipeRef.current = options.onSwipe;
    }, [
        options.onSwipe
    ]);
    (0,react.useEffect)(()=>{
        const containerEl = containerRef.current;
        if (!containerEl) return;
        let active = false;
        let startX = 0;
        let startY = 0;
        let startTime = 0;
        let startScrollLeft = 0;
        let scrollMoved = false;
        let axisLock = null;
        const onTouchStart = (e)=>{
            const touch = e.touches[0];
            if (!touch) return;
            if (isInEdgeZone(touch.clientX)) {
                active = false;
                return;
            }
            active = true;
            axisLock = null;
            scrollMoved = false;
            startX = touch.clientX;
            startY = touch.clientY;
            startTime = Date.now();
            startScrollLeft = containerEl.scrollLeft;
        };
        const updateAxisLock = (touch)=>{
            if (axisLock !== null) return;
            const dx = touch.clientX - startX;
            const dy = touch.clientY - startY;
            if (Math.abs(dx) > windowGestureConstants/* GESTURE_MOVEMENT_THRESHOLD_PX */.j || Math.abs(dy) > windowGestureConstants/* GESTURE_MOVEMENT_THRESHOLD_PX */.j) {
                axisLock = classifyAxis(dx, dy);
            }
        };
        const onTouchMove = (e)=>{
            if (!active) return;
            const touch = e.touches[0];
            if (!touch) return;
            if (containerEl.scrollLeft !== startScrollLeft) {
                scrollMoved = true;
            }
            updateAxisLock(touch);
        };
        const onTouchEnd = (e)=>{
            if (!active) return;
            active = false;
            const touch = e.changedTouches[0];
            if (!touch) return;
            const dx = touch.clientX - startX;
            const dy = touch.clientY - startY;
            const fire = shouldFireSwipe({
                axis: axisLock,
                dx,
                dy,
                elapsedMs: Date.now() - startTime,
                isOverflowing: containerEl.scrollWidth > containerEl.clientWidth,
                scrollMoved: scrollMoved || containerEl.scrollLeft !== startScrollLeft
            });
            if (fire) onSwipeRef.current(dx < 0 ? "next" : "prev");
        };
        // passive: true — this hook never calls preventDefault, so native scroll
        // and iOS edge-swipe are left fully intact; it only observes.
        containerEl.addEventListener("touchstart", onTouchStart, {
            passive: true
        });
        containerEl.addEventListener("touchmove", onTouchMove, {
            passive: true
        });
        containerEl.addEventListener("touchend", onTouchEnd, {
            passive: true
        });
        return ()=>{
            containerEl.removeEventListener("touchstart", onTouchStart);
            containerEl.removeEventListener("touchmove", onTouchMove);
            containerEl.removeEventListener("touchend", onTouchEnd);
        };
    }, [
        containerRef
    ]);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Fwindow%2FwindowTabStrip.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VX246bMBB9z1f4pdJGqldAUpJ6X_onKwc7YbrGprbZhFT77xXmTmAhaqq-xcyMPXPOmbHz_LpnkNm9h36vEGJgUkFzgo6CX15WCB2UZlzjg7JWJQT56QUZJYChd6qfMH71v_NroK987Zxp9HbSKpOMDOyhsxebYhNrkG8EecWXmMMptgRtwtQdRwWcJAbLE0NQxKXluvh8oulwxyB3W6aUMZAngryh_bp-WX2snqvy_PHy7j6vCCTIL34mIPEZmI0Htfie96VYq3euj0Kd8YUgmllVfDORVkIcqK4jpZK8lych-MwPb2Bx49tPfRgR3FXZz8xYOOY4UtJyabumOv9gX3JRZVgvW6SHVFtNpUmp5tK2kpnVSiksTRlkZgj2Zud8okwbpQlKFdRJHpW02MCVE-RXmomUKJwGdE0ILqH6BBILfrQtK64AsKAk6RRWEJmYr-X-5aKHO4kLhh3608LfrydT9HoC3biNUlXnobmgFt75yx3kjpV7qZXmh16BV3vi9i7hDNUx2Xgb72YW_CeBDELourEfaQIiv9FM5jzOMViOTUojXnTbWdO0284ExcAYl044_GJxa-BCQGrAjCM_xs9i5X2t8ZhQ47cZHSbTOvzVhXvUI2mRO1cyOCjBGgQYj5SmZRWZZFwL6M-o0GU3urff64LwsU21e_RsrGdOw2y57E2VmXtqYXOUY36uDcIlczIokyxIwcNCKhBBxlyDS0ClNAKbjwi0sjSCHOi1B_xiHtuYvfNujve7NrqAyLK7LNV26gq_9qm4sY5PsrmJMinrz0rfdZtuyeuqxeLgsBi9yG4HzD8ew-GniOZDRCsG_gLRG6l3cHns7IjcRs21F_Qbv3rjTT8_80WVzt9Gj2NpToyfYPIj4QwoeupcaqHnpZe1w-i59xeixWy7LTH76PgEEz4NsmNB28mgzht8LHC3_LSPPyW661EJDQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Fwindow_2FwindowTabStrip_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VX246bMBB9z1f4pdJGqldAUpJ6X_onKwc7YbrGprbZhFT77xXmTmAhaqq_xcyMPXPOmbHz_LpnkNm9h36vEGJgUkFzgo6CX15WCB2UZlzjg7JWJQT56QUZJYChd6qfMH71v_NroK987Zxp9HbSKpOMDOyhsxebYhNrkG8EecWXmMMptgRtwtQdRwWcJAbLE0NQxKXluvh8oulwxyB3W6aUMZAngryh_bp_WX2snqvy_PHy7j6vCCTIL34mIPEZmI0Htfie96VYq3euj0Kd8YUgmllVfDORVkIcqK4jpZK8lych_MwPb2Bx49tPfRgR3FXZz8xYOOY4UtJyabumOv9gX3JRZVgvW6SHVFtNpUmp5tK2kpnVSiksTRlkZgj2Zud8okwbpQlKFdRJHpW02MCVE_RXmomUKJwGdE0ILqH6BBILfrQtK64AsKAk6RRWEJmYr_X_5aKHO4kLhh3608LfrydT9HoC3biNUlXnobmgFt75yx3kjpV7qZXmh16BV3vi9i7hDNUx2Xgb72YW_CeBDELourEfaQIiv9FM5jzOMViOTUojXnTbWdO0284ExcAYl044_GJxa_BCQGrAjCM_xs9i5X2t8ZhQ47cZHSbTOvzVhXvUI2mRO1cyOCjBGgQYj5SmZRWZZFwL6M_o0GU3urff64LwsU21e_RsrGdOw2y57E2VmXtqYXOUY36uDcIlczIokyxIwcNCKhBBxlyDS0ClNAKbjwi0sjSCHOi1B_xiHtuYvfNujve7NrqAyLK7LNV26gq_9qm4sY5PsrmJMinrz0rfdZtuyeuqxeLgsBi9yG4HzD8ew_GniOZDRCsG_gLRG6l3cHns7IjcRs21F_Qbv3rjTT8_80WVzt9Gj2NpToyfYPIj4QwoeupcaqHnpZe1w_i59xeixWy7LTH76PgEEz4NsmNB28mgzht8LHC3_LSPPyW661EJDQAA_22_7D = __webpack_require__(93566);
;// ./src/styles/window/windowTabStrip.css.ts



var windowAddButton = '_8diut82';
var windowOnboardingHint = '_8diut8a';
var windowOnboardingHintDismiss = '_8diut8b';
var windowTabButton = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_8diut84',variantClassNames:{active:{true:'_8diut85',false:'_8diut86'}},defaultVariants:{active:false},compoundVariants:[]});
var windowTabCloseButton = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_8diut87',variantClassNames:{active:{true:'_8diut88',false:'_8diut89'}},defaultVariants:{active:false},compoundVariants:[]});
var windowTabInput = '_8diut8c';
var windowTabList = '_8diut81';
var windowTabStrip = '_8diut80';
var windowTabWrapper = '_8diut83';
;// ./src/components/window/WindowTabStrip.tsx
/* __next_internal_client_entry_do_not_use__ WindowTabStrip auto */ 




/** Surface 10 (design/ux.md): permanently dismisses the tmux-vocabulary onboarding hint. */ const HINT_DISMISSED_KEY = "cockpit.windowHintDismissed";
function readHintDismissed() {
    try {
        return localStorage.getItem(HINT_DISMISSED_KEY) === "true";
    } catch (e) {
        return false;
    }
}
const LONG_PRESS_MS = 500;
// Shared 500ms-stationary-else-cancel long-press check: `onLongPress(id, name)`
// fires once if the touch stays within GESTURE_MOVEMENT_THRESHOLD_PX for the
// full duration; moving past the threshold cancels it so the gesture falls
// through to swipe/scroll handling instead (Epic 3.2).
function useLongPress(onLongPress) {
    const stateRef = (0,react.useRef)(null);
    const clear = ()=>{
        if (stateRef.current) {
            clearTimeout(stateRef.current.timer);
            stateRef.current = null;
        }
    };
    const onTouchStart = (e, id, name)=>{
        const touch = e.touches[0];
        if (!touch) return;
        const timer = setTimeout(()=>{
            onLongPress(id, name);
            stateRef.current = null;
        }, LONG_PRESS_MS);
        stateRef.current = {
            startX: touch.clientX,
            startY: touch.clientY,
            timer
        };
    };
    const onTouchMove = (e)=>{
        const state = stateRef.current;
        const touch = e.touches[0];
        if (!state || !touch) return;
        const dx = Math.abs(touch.clientX - state.startX);
        const dy = Math.abs(touch.clientY - state.startY);
        if (dx > windowGestureConstants/* GESTURE_MOVEMENT_THRESHOLD_PX */.j || dy > windowGestureConstants/* GESTURE_MOVEMENT_THRESHOLD_PX */.j) {
            clear();
        }
    };
    return {
        onTouchStart,
        onTouchMove,
        onTouchEnd: clear
    };
}
function WindowTabEditor(param) {
    let { id, draftName, onDraftChange, onCommit, onCancel } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: windowTabWrapper,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
            ref: (el)=>{
                if (el) {
                    el.focus();
                    el.select();
                }
            },
            className: windowTabInput,
            value: draftName,
            onChange: (e)=>onDraftChange(e.target.value),
            onBlur: ()=>onCommit(id),
            onKeyDown: (e)=>{
                if (e.key === "Enter") {
                    e.preventDefault();
                    onCommit(id);
                } else if (e.key === "Escape") {
                    e.preventDefault();
                    onCancel();
                }
            }
        })
    });
}
function WindowTabCloseButton(param) {
    let { id, name, isActive, onClose } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        className: windowTabCloseButton({
            active: isActive
        }),
        "data-testid": "window-tab-close-".concat(id),
        // Roving tabindex (WAI-ARIA APG "tabs" pattern) means the tablist is a
        // single stop in page Tab order; a native button defaults to tabIndex
        // 0 and would add a second, per-tab stop. Closing is still reachable
        // via mouse/touch here or via the tab's own Delete key (handleKeyDown).
        tabIndex: -1,
        "aria-label": "Close ".concat(name),
        title: "Close ".concat(name),
        onClick: (e)=>{
            e.stopPropagation();
            onClose(id);
        },
        children: "\xd7"
    });
}
// A <div role="tab"> rather than a native <button> — the close "×" control
// must live inside the tab's own DOM subtree (nesting it as a sibling makes
// it a direct, non-"tab"-role child of the parent role="tablist", which
// fails axe's aria-required-children rule) and a real <button> cannot be
// nested inside another <button>. `aria-label={w.name}` keeps this
// element's own accessible name pinned to just the window name rather than
// accumulating the nested close button's "Close <name>" text via
// name-from-content. The nested real <button> still trips axe's separate
// nested-interactive rule; this mirrors ShellTabLabel's identical
// tab-with-inline-actions structure (SessionDetailView.tsx's shell tab
// button), an established, pre-existing pattern in this codebase rather
// than a new tradeoff introduced here.
function WindowTabButton(param) {
    let { window: w, isActive, showClose, onSwitch, onClose, onBeginEdit, longPress, tabRef } = param;
    const handleKeyDown = (e)=>{
        // Ignore keys that bubbled up from the nested close button — only the
        // tab itself (not its child) should respond to these.
        if (e.target !== e.currentTarget) return;
        if (e.key === "F2") {
            onBeginEdit(w.id, w.name);
        } else if (e.key === "Delete") {
            onClose(w.id);
        } else if (e.key === "Enter" || e.key === " ") {
            // A native <button> auto-activates on Enter/Space; this element is a
            // <div> (see doc comment above), so that activation is wired up here.
            e.preventDefault();
            onSwitch(w.id);
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        ref: (el)=>tabRef(w.id, el),
        role: "tab",
        "aria-selected": isActive,
        "aria-label": w.name,
        tabIndex: isActive ? 0 : -1,
        "data-testid": "window-tab-".concat(w.id),
        className: windowTabButton({
            active: isActive
        }),
        title: w.name,
        onClick: ()=>onSwitch(w.id),
        onDoubleClick: ()=>onBeginEdit(w.id, w.name),
        onKeyDown: handleKeyDown,
        onTouchStart: (e)=>longPress.onTouchStart(e, w.id, w.name),
        onTouchMove: longPress.onTouchMove,
        onTouchEnd: longPress.onTouchEnd,
        children: [
            w.name,
            showClose && /*#__PURE__*/ (0,jsx_runtime.jsx)(WindowTabCloseButton, {
                id: w.id,
                name: w.name,
                isActive: isActive,
                onClose: onClose
            })
        ]
    });
}
function WindowTab(param) {
    let { window: w, isActive, isEditing, draftName, showClose, onSwitch, onClose, onBeginEdit, onDraftChange, onCommit, onCancel, longPress, tabRef } = param;
    if (isEditing) {
        return /*#__PURE__*/ (0,jsx_runtime.jsx)(WindowTabEditor, {
            id: w.id,
            draftName: draftName,
            onDraftChange: onDraftChange,
            onCommit: onCommit,
            onCancel: onCancel
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: windowTabWrapper,
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(WindowTabButton, {
            window: w,
            isActive: isActive,
            showClose: showClose,
            onSwitch: onSwitch,
            onClose: onClose,
            onBeginEdit: onBeginEdit,
            longPress: longPress,
            tabRef: tabRef
        })
    });
}
const WindowTabStripInner = /*#__PURE__*/ (0,react.forwardRef)(function WindowTabStrip(param, ref) {
    let { windows, currentWindowId, onSwitch, onCreate, onClose, onRename } = param;
    const [editingId, setEditingId] = (0,react.useState)(null);
    const [draftName, setDraftName] = (0,react.useState)("");
    const beginEdit = (id, name)=>{
        setEditingId(id);
        setDraftName(name);
    };
    const commitRename = (id)=>{
        const trimmed = draftName.trim();
        if (trimmed.length > 0) {
            onRename(id, trimmed);
        }
        setEditingId(null);
    };
    const cancelRename = ()=>setEditingId(null);
    const longPress = useLongPress(beginEdit);
    const [hintDismissed, setHintDismissed] = (0,react.useState)(readHintDismissed);
    const dismissHint = ()=>{
        setHintDismissed(true);
        try {
            localStorage.setItem(HINT_DISMISSED_KEY, "true");
        } catch (e) {
        // Degraded but acceptable (design/ux.md Surface 10): the hint may
        // reappear next session if localStorage is unavailable/full.
        }
    };
    const showHint = windows.length >= 2 && !hintDismissed;
    (0,react.useImperativeHandle)(ref, ()=>({
            beginEdit
        }), [
        beginEdit
    ]);
    // Container ref for useWindowSwipe to attach touch listeners to
    // (Task 3.1.1b) — internal only; the forwarded ref exposes `beginEdit`
    // instead of this DOM node (nothing outside this component needs the
    // node directly).
    const containerRef = (0,react.useRef)(null);
    useWindowSwipe(containerRef, {
        onSwipe: (dir)=>{
            const idx = windows.findIndex((w)=>w.id === currentWindowId);
            const nextIdx = dir === "next" ? (idx + 1) % windows.length : (idx - 1 + windows.length) % windows.length;
            onSwitch(windows[nextIdx].id);
        }
    });
    // Roving-tabindex support (WAI-ARIA APG "tabs" pattern, automatic
    // activation): tab DOM nodes are keyed by id here so ArrowLeft/Right/
    // Home/End can move both the switched-to window AND actual keyboard
    // focus in one keystroke — the nodes themselves persist across a
    // currentWindowId change (only their tabIndex/aria-selected re-render),
    // so focusing by id works without waiting on the re-render to land.
    const tabNodesRef = (0,react.useRef)(new Map());
    const setTabRef = (id, el)=>{
        if (el) tabNodesRef.current.set(id, el);
        else tabNodesRef.current.delete(id);
    };
    const handleTabListKeyDown = (e)=>{
        var _tabNodesRef_current_get;
        if (e.target !== e.currentTarget && e.target.getAttribute("role") !== "tab") return;
        const idx = windows.findIndex((w)=>w.id === currentWindowId);
        if (idx === -1) return;
        let targetIdx = null;
        if (e.key === "ArrowRight") targetIdx = (idx + 1) % windows.length;
        else if (e.key === "ArrowLeft") targetIdx = (idx - 1 + windows.length) % windows.length;
        else if (e.key === "Home") targetIdx = 0;
        else if (e.key === "End") targetIdx = windows.length - 1;
        if (targetIdx === null) return;
        e.preventDefault();
        const target = windows[targetIdx];
        onSwitch(target.id);
        (_tabNodesRef_current_get = tabNodesRef.current.get(target.id)) === null || _tabNodesRef_current_get === void 0 ? void 0 : _tabNodesRef_current_get.focus();
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                ref: containerRef,
                className: windowTabStrip,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: windowTabList,
                        role: "tablist",
                        "aria-label": "Window switcher",
                        onKeyDown: handleTabListKeyDown,
                        children: windows.map((w)=>/*#__PURE__*/ (0,jsx_runtime.jsx)(WindowTab, {
                                window: w,
                                isActive: w.id === currentWindowId,
                                isEditing: editingId === w.id,
                                draftName: draftName,
                                showClose: windows.length > 1,
                                onSwitch: onSwitch,
                                onClose: onClose,
                                onBeginEdit: beginEdit,
                                onDraftChange: setDraftName,
                                onCommit: commitRename,
                                onCancel: cancelRename,
                                longPress: longPress,
                                tabRef: setTabRef
                            }, w.id))
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: windowAddButton,
                        "data-testid": "window-add-button",
                        onClick: onCreate,
                        title: "New window",
                        "aria-label": "New window",
                        children: "+"
                    })
                ]
            }),
            showHint && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: windowOnboardingHint,
                "data-testid": "window-onboarding-hint",
                role: "note",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        children: 'ⓘ Windows group your panes into separate workspaces — like a tmux window, but the "sessions" here are your agent panes, not tmux sessions.'
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        className: windowOnboardingHintDismiss,
                        onClick: dismissHint,
                        children: "Got it ✕"
                    })
                ]
            })
        ]
    });
});
WindowTabStripInner.displayName = "WindowTabStrip";
// Compares `windows` by the derived {id, name}[] list rather than by
// reference/full deep-equality — windowReducer's PANE_ACTION case rebuilds
// the entire `windows` array (including every other window's pane tree) on
// every pane action, so comparing full objects would defeat memoization on
// every keystroke/resize in the active window (pre-mortem.md P2 #4).
// Callback props (onSwitch/onCreate/onClose/onRename) are deliberately not
// compared: page.tsx passes fresh inline arrows every render, but each one
// only forwards to the hook's already-stable functions.
function arePropsEqual(prev, next) {
    if (prev.currentWindowId !== next.currentWindowId) return false;
    if (prev.windows.length !== next.windows.length) return false;
    for(let i = 0; i < prev.windows.length; i++){
        if (prev.windows[i].id !== next.windows[i].id) return false;
        if (prev.windows[i].name !== next.windows[i].name) return false;
    }
    return true;
}
const WindowTabStrip = /*#__PURE__*/ (0,react.memo)(WindowTabStripInner, arePropsEqual);

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
;// ./src/lib/hooks/useDatabase.ts
/* __next_internal_client_entry_do_not_use__ useDatabases auto */ 






/**
 * React hook for the workspace/database switcher.
 *
 * Fetches all available workspace databases on mount and exposes a
 * `switchDatabase` action that writes the preference, waits for the
 * server to restart, then reloads the page.
 */ function useDatabases() {
    const [databases, setDatabases] = (0,react.useState)([]);
    const [currentId, setCurrentId] = (0,react.useState)("");
    const [loading, setLoading] = (0,react.useState)(false);
    const [switching, setSwitching] = (0,react.useState)(false);
    const [merging, setMerging] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    (0,react.useEffect)(()=>{
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const fetchDatabases = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        setLoading(true);
        setError(null);
        try {
            const resp = await clientRef.current.listDatabases((0,create/* create */.v)(session_pb/* ListDatabasesRequestSchema */.VWu, {}));
            var _resp_databases;
            setDatabases((_resp_databases = resp.databases) !== null && _resp_databases !== void 0 ? _resp_databases : []);
            var _resp_currentWorkspaceId;
            setCurrentId((_resp_currentWorkspaceId = resp.currentWorkspaceId) !== null && _resp_currentWorkspaceId !== void 0 ? _resp_currentWorkspaceId : "");
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Failed to fetch databases");
            setError(e.message);
            console.error("Failed to fetch databases:", e);
        } finally{
            setLoading(false);
        }
    }, []);
    const refresh = (0,react.useCallback)(async ()=>{
        await fetchDatabases();
    }, [
        fetchDatabases
    ]);
    (0,react.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    const switchDatabase = (0,react.useCallback)(async (configDir)=>{
        if (!clientRef.current) return;
        setSwitching(true);
        setError(null);
        try {
            await clientRef.current.switchDatabase((0,create/* create */.v)(session_pb/* SwitchDatabaseRequestSchema */.k0S, {
                configDir
            }));
        } catch (err) {
        // The server will restart, so a network error here is expected.
        // Proceed with polling.
        }
        // Poll until the server is back up (max 10 seconds)
        const apiBase = (0,config/* getApiBaseUrl */.h)();
        const maxAttempts = 33; // ~10s at 300ms intervals
        let attempts = 0;
        let serverBack = false;
        // Wait briefly first to give the server time to shut down
        await new Promise((resolve)=>setTimeout(resolve, 500));
        while(attempts < maxAttempts){
            try {
                const tempClient = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
                // Health-check poll after triggering a server restart — meant to
                // keep retrying until the server is back or the attempt budget
                // is spent, not tied to a mount/effect.
                // abort-signal-exempt
                await tempClient.getCurrentDatabase((0,create/* create */.v)(session_pb/* GetCurrentDatabaseRequestSchema */.haZ, {}));
                serverBack = true;
                break;
            } catch (e) {
                await new Promise((resolve)=>setTimeout(resolve, 300));
                attempts++;
            }
        }
        if (serverBack) {
            window.location.reload();
        } else {
            setSwitching(false);
            setError("Server did not restart in time. Please refresh manually.");
        }
    }, []);
    const mergeDatabase = (0,react.useCallback)(async (configDir)=>{
        if (!clientRef.current) throw new Error("Client not initialized");
        setMerging(true);
        setError(null);
        try {
            const resp = await clientRef.current.mergeDatabase((0,create/* create */.v)(session_pb/* MergeDatabaseRequestSchema */.Mpl, {
                configDir
            }));
            await refresh();
            return {
                sessionsImported: resp.sessionsImported,
                sessionsSkipped: resp.sessionsSkipped,
                message: resp.message
            };
        } catch (err) {
            const e = err instanceof Error ? err : new Error("Merge failed");
            setError(e.message);
            throw e;
        } finally{
            setMerging(false);
        }
    }, [
        refresh
    ]);
    return {
        databases,
        currentId,
        loading,
        switching,
        merging,
        error,
        switchDatabase,
        mergeDatabase,
        refresh
    };
}

;// ./src/lib/hooks/useSessionViewMode.ts


function storageKey(workspaceId) {
    return "ws-".concat(workspaceId, ".stapler-squad-session-view-mode");
}
/**
 * Persists the user's List/Board view choice, scoped per workspace so switching
 * workspaces doesn't carry over an unrelated view preference. Falls back to an
 * in-memory "list" default if localStorage throws (e.g. private-browsing quota).
 */ function useSessionViewMode() {
    const { currentId } = useDatabases();
    const [mode, setModeState] = (0,react.useState)(()=>{
        try {
            const raw = currentId ? localStorage.getItem(storageKey(currentId)) : null;
            return raw === "board" ? "board" : "list";
        } catch (e) {
            return "list";
        }
    });
    const setMode = (0,react.useCallback)((m)=>{
        setModeState(m);
        try {
            if (currentId) localStorage.setItem(storageKey(currentId), m);
        } catch (e) {
        // localStorage unavailable — in-memory state still updates
        }
    }, [
        currentId
    ]);
    return [
        mode,
        setMode
    ];
}

// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
;// ./src/app/findSessionById.ts
/**
 * Resolves a session reference that may be a full id, an id prefix, a tmux
 * session name, an external-session worktree path (or its basename), an
 * `_`-prefixed external id, a title, or a repo-dir basename — in that order
 * of preference. Extracted from page.tsx's HomeContent — same behavior,
 * isolated as a pure lookup so the component doesn't carry the fallback chain.
 */ function findSessionById_findSessionById(sessions, sessionId) {
    let session = sessions.find((s)=>s.id === sessionId);
    if (session) return session;
    session = sessions.find((s)=>{
        var _s_externalMetadata;
        if (s.id.startsWith(sessionId)) return true;
        if (((_s_externalMetadata = s.externalMetadata) === null || _s_externalMetadata === void 0 ? void 0 : _s_externalMetadata.tmuxSessionName) === sessionId) return true;
        if (sessionId.includes("/") && s.existingDir && s.existingDir.includes(sessionId)) return true;
        if (s.existingDir && s.existingDir.endsWith("/".concat(sessionId))) return true;
        return false;
    });
    if (!session && sessionId.includes("_")) {
        const withoutPrefix = sessionId.split("_").slice(1).join("_");
        session = sessions.find((s)=>s.id === withoutPrefix || s.title === withoutPrefix);
    }
    if (!session) {
        const searchLower = sessionId.toLowerCase();
        session = sessions.find((s)=>{
            var _s_existingDir_split_pop, _s_existingDir;
            if (s.title.toLowerCase() === searchLower) return true;
            const pathBasename = (_s_existingDir = s.existingDir) === null || _s_existingDir === void 0 ? void 0 : (_s_existingDir_split_pop = _s_existingDir.split("/").pop()) === null || _s_existingDir_split_pop === void 0 ? void 0 : _s_existingDir_split_pop.toLowerCase();
            if (pathBasename === searchLower) return true;
            return false;
        });
    }
    if (!session) {
        console.warn("[findSessionById] No session found for ID: ".concat(sessionId), {
            availableSessions: sessions.map((s)=>({
                    id: s.id,
                    title: s.title,
                    path: s.existingDir
                }))
        });
    }
    return session;
}

// EXTERNAL MODULE: ./src/lib/hooks/useAnnounce.ts
var useAnnounce = __webpack_require__(5647);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FSessionUnavailableCard.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61TXXOCMBB891fksTzEAbRf9Md0UhLDTSGHlyBKx__eSdBqQdTO9AVmbvf27pZlvk7Ui4zZ14wxCbYuxS5jq1Jt32YsvLkEUrkDNBnLsWwq4xFRgjYcnKpsT-fWCXIe0qLO2EbQA-fvyavqUurSLvJIJba8BemKjC1SUlVfIw1m2LBIIyYah55RCynB6BElCZofIv_UhI2RQ8JzwHMskYZQ3LciSUUZS-ots1iCHLD0GYuTkNDY0RJ-yH42DyYmwcTjQXEwEI3jFjo1apTRD94q0IUbMYoz6XQsffGwJLo-NT_TXNypGd-hyRhuFK1KbHlL_vsLs2sLReo0bnklYn2Pf94KkF-WO7xMOI567C8Dw4uDtctlHab52iGBx9JEvNIu-mvgqunArX8FzqBR90Xrtu_XEtQHOG_I-o1qBOMUnWx6CjZN3yP_4wfafwNBdda6YQQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FSessionUnavailableCard_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61TXXOCMBB891fksTzEAbRf9Md0UhLDTSGHlyBKx_eSdBqQdTO9AVmbvf27pZlvk7Ui4zZ14wxCbYuxS5jq1Jt32YsvLkEUrkDNBnLsWwq4xFRgjYcnKpsT_fWCXIe0qLO2EbQA_fvyavqUurSLvJIJba8BemKjC1SUlVfIw1m2LBIIyYah55RCynB6BElCZofIv_UhI2RQ8JzwHMskYZQ3LciSUUZS_ots1iCHLD0GYuTkNDY0RJ_yH42DyYmwcTjQXEwEI3jFjo1apTRD94q0IUbMYoz6XQsffGwJLo_NT_TXNypGd_hyRhuFK1KbHlL_vsLs2sLReo0bnklYn2Pf94KkF_WO7xMOI567C8Dw4uDtctlHab52iGBx9JEvNIu_mvgqunArX8FzqBR90Xrtu_XEtQHOG_I_o1qBOMUnWx6CjZN3yP_4wfafwNBdda6YQQAAA_22_7D = __webpack_require__(97531);
;// ./src/components/sessions/SessionUnavailableCard.css.ts


var SessionUnavailableCard_css_actions = 'q1e8d4';
var SessionUnavailableCard_css_button = 'q1e8d5';
var SessionUnavailableCard_css_card = 'q1e8d0';
var detail = 'q1e8d2';
var SessionUnavailableCard_css_heading = 'q1e8d1';
var quote = 'q1e8d3';
var SessionUnavailableCard_css_secondaryButton = 'q1e8d6 q1e8d5';
;// ./src/components/sessions/SessionUnavailableCard.tsx
/* __next_internal_client_entry_do_not_use__ SessionUnavailableCard auto */ 



function relativeAge(timestampMs, now) {
    const minutes = Math.max(0, Math.floor((now - timestampMs) / 60000));
    if (minutes < 1) return "just now";
    if (minutes < 60) return "".concat(minutes, "m ago");
    const hours = Math.floor(minutes / 60);
    return hours < 24 ? "".concat(hours, "h ago") : "".concat(Math.floor(hours / 24), "d ago");
}
/**
 * Deleted or unreachable session behind a deep link (Story 5.3, RO-3 / RO-7 / RO-10):
 * never a 404 and never a blank screen. Announced once on mount through the Announcer,
 * so the card itself carries no live role.
 */ function SessionUnavailableCard(param) {
    let { variant, sessionLabel, notification, onOpenNotifications, onGoToSessions, onRetry } = param;
    const { announce } = (0,useAnnounce/* useAnnounce */.XE)();
    const heading = variant === "failed" ? "Could not load session." : "Session no longer available";
    (0,react.useEffect)(()=>{
        announce(heading, variant === "failed" ? "assertive" : "polite", "session-unavailable");
    // Once per mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: SessionUnavailableCard_css_card,
        "data-testid": "session-unavailable-card",
        "aria-labelledby": "session-unavailable-heading",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                className: SessionUnavailableCard_css_heading,
                id: "session-unavailable-heading",
                children: heading
            }),
            variant === "unavailable" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                className: detail,
                children: [
                    sessionLabel,
                    " was removed before you opened it"
                ]
            }),
            variant === "unavailable" && notification && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                        className: quote,
                        "data-testid": "session-unavailable-notification",
                        children: [
                            "Last notification: “",
                            notification.title,
                            "”",
                            notification.message ? " - ".concat(notification.message) : ""
                        ]
                    }),
                    notification.timestampMs > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: detail,
                        children: relativeAge(notification.timestampMs, Date.now())
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SessionUnavailableCard_css_actions,
                children: [
                    variant === "failed" && onRetry && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: SessionUnavailableCard_css_button,
                        onClick: onRetry,
                        "data-testid": "session-unavailable-retry",
                        children: "Retry"
                    }),
                    variant === "unavailable" && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: SessionUnavailableCard_css_button,
                        onClick: onOpenNotifications,
                        "data-testid": "session-unavailable-open-notifications",
                        children: "Open notifications"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: SessionUnavailableCard_css_secondaryButton,
                        onClick: onGoToSessions,
                        "data-testid": "session-unavailable-go-to-sessions",
                        children: "Go to sessions"
                    })
                ]
            })
        ]
    });
}

;// ./src/lib/hooks/useNotificationRecord.ts
/* __next_internal_client_entry_do_not_use__ useNotificationRecord auto */ 




/**
 * Captured title and message for a deep link's `notification=<id>` (Story 5.3, C5): the
 * hydrated history slice first, then one `GetNotificationHistory` call scoped to the
 * session (the RPC has no id filter). `undefined` while resolving, `null` when the
 * record is gone (pruned, cleared, or the link carried no id).
 */ function useNotificationRecord(notificationHistory, notificationId, sessionId, enabled) {
    const [fetchedFor, setFetchedFor] = (0,react.useState)(null);
    const inSlice = notificationId ? (notificationHistory !== null && notificationHistory !== void 0 ? notificationHistory : []).find((n)=>n.id === notificationId) : undefined;
    (0,react.useEffect)(()=>{
        if (!enabled || !notificationId || inSlice) return;
        const controller = new AbortController();
        const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
        client.getNotificationHistory((0,create/* create */.v)(session_pb/* GetNotificationHistoryRequestSchema */.G5n, {
            sessionId: sessionId !== null && sessionId !== void 0 ? sessionId : undefined,
            limit: 100
        }), {
            signal: controller.signal
        }).then((res)=>{
            if (controller.signal.aborted) return;
            const hit = res.notifications.find((n)=>n.id === notificationId);
            setFetchedFor({
                forId: notificationId,
                value: hit ? {
                    title: hit.title,
                    message: hit.message,
                    timestampMs: hit.createdAt ? Number(hit.createdAt.seconds) * 1000 : 0
                } : null
            });
        }).catch(()=>{
            if (!controller.signal.aborted) setFetchedFor({
                forId: notificationId,
                value: null
            });
        });
        return ()=>controller.abort();
    }, [
        enabled,
        notificationId,
        sessionId,
        inSlice
    ]);
    if (!notificationId) return null;
    if (inSlice) {
        var _inSlice_title;
        return {
            title: (_inSlice_title = inSlice.title) !== null && _inSlice_title !== void 0 ? _inSlice_title : inSlice.sessionName,
            message: inSlice.message,
            timestampMs: inSlice.timestamp
        };
    }
    // A result fetched for another id must not leak onto this card while its own fetch is in flight.
    return (fetchedFor === null || fetchedFor === void 0 ? void 0 : fetchedFor.forId) === notificationId ? fetchedFor.value : undefined;
}

;// ./src/app/sessionDeepLink.ts
// Pure helpers for the `/?session=<id>&tab=terminal&notification=<id>` deep link (Story 5.3).
function parseSessionDeepLink(params) {
    return {
        sessionId: params.get("session"),
        tab: params.get("tab"),
        newPane: params.get("newPane"),
        notificationId: params.get("notification")
    };
}
/**
 * The live list does not carry hidden sessions, so a session id that is not in it is
 * resolved with `getSession`. The lookup waits until the list has settled, otherwise a
 * cold load would resolve against a list that is empty only because it has not arrived
 * (a populated list resolves in-list; an empty settled list must still fall through).
 */ function shouldResolveViaGetSession(input) {
    return Boolean(input.sessionId) && !input.foundInList && input.listSettled;
}
/** Connect code 5 is NotFound; anything else (network, timeout, auth) is a retryable failure. */ function classifyGetSessionFailure(err) {
    const code = err === null || err === void 0 ? void 0 : err.code;
    return code === 5 || code === "not_found" ? "not_found" : "failed";
}

;// ./src/app/useOmnibarQueryParamLaunch.ts

/**
 * Detects `?new=true`, `?pr=<url>`, `?duplicate=<id>`, or
 * `?worktree=<path>&branch=<branch>` query params and opens the Omnibar
 * pre-filled accordingly, clearing the param from the URL. Extracted from
 * page.tsx's HomeContent — same behavior, isolated as its own concern.
 */ function useOmnibarQueryParamLaunch(searchParams, router, openOmnibar, track, getSession) {
    (0,react.useEffect)(()=>{
        const newParam = searchParams.get("new");
        const prUrl = searchParams.get("pr");
        const duplicateId = searchParams.get("duplicate");
        const worktreePath = searchParams.get("worktree");
        const worktreeBranch = searchParams.get("branch");
        const title = searchParams.get("title");
        if (prUrl) {
            router.replace("/", {
                resetScroll: false
            });
            openOmnibar(prUrl);
        } else if (newParam === "true") {
            router.replace("/", {
                resetScroll: false
            });
            openOmnibar();
        } else if (duplicateId) {
            router.replace("/", {
                resetScroll: false
            });
            track({
                name: "session_duplicate_initiated",
                category: "user_action"
            });
            getSession(duplicateId).then((session)=>{
                openOmnibar(session === null || session === void 0 ? void 0 : session.repoRoot);
            }).catch(()=>{
                openOmnibar();
            });
        } else if (worktreePath) {
            router.replace("/", {
                resetScroll: false
            });
            // Pass path@branch so the PathWithBranch detector pre-fills both fields
            openOmnibar(worktreeBranch ? "".concat(worktreePath, "@").concat(worktreeBranch) : worktreePath, title || undefined);
        }
    }, [
        searchParams,
        getSession,
        openOmnibar,
        router,
        track
    ]);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VWwY7aMBC98xW-VCLqGjmhC2y49E9WJh6Iu47t2gYCFf9e2UnACWG7W_VSCSExMx6_eeOZx-x1WYmX7I2gXxOEtgLqHKXrCUKMWy3oKQ-2devDjBsoHFcyR4US-0p6T8UlLoHvSpcj4g0bWrztjNpLhgsllMnRgZopxq_pC5wzc14kPmrURYJLHcBshTrmqOSMgVxPLpNZizQNSLWyvMGx5TUwf8gp3d4vYNtBMTEs5Zyq7jAOIdBktH4q-E5i7qCyOSpAOjDe_GNvHd-ecKGkA-li1xlzyQKhhIQ7NWWMy12OUgNVZMABekFFMW2glEAZmJbUBH1FZPZsoEpiHrLAw-MylklTs_GZDGV8b3OUZjoUU9EaHzlzpTcR0hg7AyFf_M-upQtyKLszQ9t9oz71cjaqxrakzCcgKCO6RtmzrhH2X2a3oVPyhNrPLE2eEEGpDwpfo0HkW4-keSDpr3tpNS0Ab8Adoant1sHQkIjh7nGlukZWCc4G7YAerm93zXOGSqupAeluWXMklYTAoZIOW36G-Opib6wfIa14V8LoUGVJDzu5JhBcwrWnYewDDH7tkzKIzDLbg56XvuuhgIcjfAt_bsb1nrfu6eBTjujeqfjQon9oUM05CU9l5Hn3A-erZEAdma2W3f3BfLy-Z_KA0D8N2DtbrOvhe0-izzgVIvCNgFroERKR_hjRKh75UWC815vl_0BzSh7z_PNDPLcZPkT08kNEp_M-jznjlm4EsHBOaVpwdwqDFpcrlcNUCHX0enU7vRpK752efmKjvqecL-PLcEd1vBPulmC4DCRrNMDsuGz0qhvly-R7BYxTZAsDIBGVDE0jgVkuVrpOwt19DY9X0jr-3eQfk8ImrLe-rTPgirLxjIO3jpqwVS8RhqzFEMtaJMEHDketjGt9T14W2aFMEB4F1iL7Z4kGE9fyE8t2q9JxSfMhrd3_jDjo-XHQ5TcHEK76DgoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VWwY7aMBC98xW_VCLqGjmhC2y49E9WJh6Iu47t2gYCFf9e2UnACWG7W_VSCSExMx6_eeOZx_x1WYmX7I2gXxOEtgLqHKXrCUKMWy3oKQ_2devDjBsoHFcyR4US_0p6T8UlLoHvSpcj4g0bWrztjNpLhgsllMnRgZopxq_pC5wzc14kPmrURYJLHcBshTrmqOSMgVxPLpNZizQNSLWyvMGx5TUwf8gp3d4vYNtBMTEs5Zyq7jAOIdBktH4q_E5i7qCyOSpAOjDe_GNvHd_ecKGkA_li1xlzyQKhhIQ7NWWMy12OUgNVZMABekFFMW2glEAZmJbUBH1FZPZsoEpiHrLAw_MylklTs_GZDGV8b3OUZjoUU9EaHzlzpTcR0hg7AyFf_M_upQtyKLszQ9t9oz71cjaqxrakzCcgKCO6RtmzrhH2X2a3oVPyhNrPLE2eEEGpDwpfo0HkW4_keSDpr3tpNS0Ab8Adoant1sHQkIjh7nGlukZWCc4G7YAerm93zXOGSqupAeluWXMklYTAoZIOW36G_Opib6wfIa14V8LoUGVJDzu5JhBcwrWnYewDDH7tkzKIzDLbg56XvuuhgIcjfAt_bsb1nrfu6eBTjujeqfjQon9oUM05CU9l5Hn3A_erZEAdma2W3f3BfLy_Z_KA0D8N2DtbrOvhe0_izzgVIvCNgFroERKR_hjRKh75UWC815vl_0BzSh7z_PNDPLcZPkT08kNEp_M_jznjlm4EsHBOaVpwdwqDFpcrlcNUCHX0enU7vRpK752efmKjvqecL_PLcEd1vBPulmC4DCRrNMDsuGz0qhvly_R7BYxTZAsDIBGVDE0jgVkuVrpOwt19DY9X0jr_3eQfk8ImrLe_rTPgirLxjIO3jpqwVS8RhqzFEMtaJMEHDketjGt9T14W2aFMEB4F1iL7Z4kGE9fyE8t2q9JxSfMhrd3_jDjo_XHQ5TcHEK76DgoAAA_22_7D = __webpack_require__(16594);
;// ./src/app/page.css.ts


var page_css_cancelButton = '_7ml92k6';
var page_css_closeButton = '_7ml92k4';
var cockpitContainer = '_7ml92k8';
var page_css_dangerButton = '_7ml92k7';
var deleteConfirmActions = '_7ml92k9';
var page_css_modal = '_7ml92k1';
var page_css_modalBody = '_7ml92k5';
var modalContent = '_7ml92k2';
var page_css_modalHeader = '_7ml92k3';
var page = '_7ml92k0';
;// ./src/app/DeleteSessionConfirmModal.tsx



/**
 * Delete confirmation modal (triggered by the 'd' keyboard shortcut).
 * Extracted from page.tsx's HomeContent — same markup/behavior, isolated as
 * its own concern. `dialogRef` is still trapped by the parent's
 * useFocusTrap(deleteDialogRef, ...) call, so it must keep landing on this
 * exact dialog div.
 *
 * All three buttons below are analytics-exempt: Close/Cancel only dismiss
 * the modal (no tracked event, same as before this was its own component),
 * and Delete's track("session_deleted") call lives one level up in the
 * onConfirm callback's own handleDeleteSession (page.tsx) — the
 * analytics/require-on-click lint rule can't see across that prop boundary.
 */ function DeleteSessionConfirmModal(param) {
    let { target, dialogRef, onCancel, onConfirm } = param;
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: page_css_modal,
        onClick: onCancel,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: dialogRef,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": "deleteConfirmTitle",
            tabIndex: -1,
            className: modalContent,
            onClick: (e)=>e.stopPropagation(),
            onKeyDown: (e)=>{
                if (e.key === "Escape") onCancel();
            },
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: page_css_modalHeader,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                            id: "deleteConfirmTitle",
                            children: "Delete Session"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: page_css_closeButton,
                            onClick: onCancel,
                            "aria-label": "Close",
                            children: "✕"
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: page_css_modalBody,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                            children: [
                                'Delete "',
                                target.title,
                                '"?'
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                            style: {
                                color: "var(--error, #ef4444)",
                                fontSize: "0.875rem",
                                marginTop: "0.5rem"
                            },
                            children: "This action cannot be undone."
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: deleteConfirmActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    autoFocus: true,
                                    className: page_css_cancelButton,
                                    onClick: onCancel,
                                    children: "Cancel"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: page_css_dangerButton,
                                    onClick: async ()=>{
                                        onCancel();
                                        await onConfirm(target.id);
                                    },
                                    children: "Delete"
                                })
                            ]
                        })
                    ]
                })
            ]
        })
    }), document.body);
}

;// ./src/app/useResumeSessionFlow.ts

/**
 * "Resume session" modal flow — request/direct-resume/confirm/cancel.
 * Extracted from page.tsx's HomeContent — same behavior, isolated as its
 * own concern. `resumeTarget` is still returned so callers elsewhere in the
 * page (e.g. the Escape-key handler, j/k navigation guards) can gate on
 * whether the modal is open.
 */ function useResumeSessionFlow(resumeSession, track) {
    const [resumeTarget, setResumeTarget] = (0,react.useState)(null);
    const resumeTriggerRef = (0,react.useRef)(null);
    const handleResumeRequest = (0,react.useCallback)((session)=>{
        resumeTriggerRef.current = document.activeElement;
        setResumeTarget(session);
    }, []);
    const handleDirectResume = (0,react.useCallback)((session)=>{
        track({
            name: "session_resumed",
            category: "user_action"
        });
        resumeSession(session.id, {
            title: session.title,
            tags: [
                ...session.tags || []
            ]
        });
    }, [
        resumeSession,
        track
    ]);
    const handleResumeConfirm = (0,react.useCallback)(async (updates)=>{
        if (!resumeTarget) return;
        try {
            track({
                name: "session_resumed",
                category: "user_action"
            });
            await resumeSession(resumeTarget.id, updates);
            setResumeTarget(null);
        } catch (e) {
        // resumeSession dispatches to Redux error state; modal stays open for retry
        }
    }, [
        resumeTarget,
        resumeSession,
        track
    ]);
    const handleResumeCancel = (0,react.useCallback)(()=>{
        setResumeTarget(null);
    }, []);
    return {
        resumeTarget,
        resumeTriggerRef,
        handleResumeRequest,
        handleDirectResume,
        handleResumeConfirm,
        handleResumeCancel
    };
}

;// ./src/app/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: session-list session-search session-filter session-groupby session-list-collapse-groups



const VALID_TABS = [
    "terminal",
    "diff",
    "vcs",
    "logs",
    "info"
];
function isValidTab(tab) {
    return tab !== null && VALID_TABS.includes(tab);
}
























// A hidden session is not in the watch stream, so an open one is polled for status changes.
const FALLBACK_SESSION_REFRESH_MS = 5000;
const MAX_FALLBACK_SESSIONS = 5;
function HomeContent() {
    (0,usePageView/* usePageView */.O)();
    const { track } = (0,AnalyticsContext/* useAnalytics */.st)();
    const { addNotification, notificationHistory } = (0,NotificationContext/* useNotifications */.E$)();
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const router = (0,nav/* useRouter */.KA)();
    const { openInCreationMode, openOmnibar } = (0,OmnibarContext/* useOmnibar */.W)();
    const [selectedSession, setSelectedSession] = (0,react.useState)(null);
    const [activeTab, setActiveTab] = (0,react.useState)("info");
    const [deleteConfirmTarget, setDeleteConfirmTarget] = (0,react.useState)(null);
    const [pendingSessionId, setPendingSessionId] = (0,react.useState)(null);
    // j/k keyboard navigation index within the session list
    const [focusedSessionIndex, setFocusedSessionIndex] = (0,react.useState)(-1);
    // List/Board view mode — lifted here (rather than called again deep in the pane
    // tree) so the 'b' shortcut below and SessionListPaneBody's render both observe
    // the same state instead of two independent useSessionViewMode() instances.
    const [viewMode, setViewMode] = useSessionViewMode();
    // Tiling: tracks the most-recently-clicked session to route to the focused pane.
    // Using a counter-based key so that clicking the same session again still triggers.
    const [externalAssignCounter, setExternalAssignCounter] = (0,react.useState)(0);
    const [externalAssignSession, setExternalAssignSession] = (0,react.useState)(null);
    // Focus management: modal containers (tabIndex={-1}) and trigger element refs
    const sessionDetailRef = (0,react.useRef)(null);
    const sessionTriggerRef = (0,react.useRef)(null);
    const deleteDialogRef = (0,react.useRef)(null);
    const lastFocusBeforeDelete = (0,react.useRef)(null);
    const windowTabStripRef = (0,react.useRef)(null);
    // Tracks the last URL params that were routed to a pane. Prevents the URL-watching
    // effect from re-triggering pane assignment on every sessions stream update (which
    // changes the `sessions` dependency but not the URL itself).
    const lastUrlRoutedRef = (0,react.useRef)({
        sessionId: null,
        tab: null
    });
    // Guards the hidden-session fallback fetch (below) so it fires once per
    // sessionId instead of refiring on every `sessions` stream update.
    const hiddenSessionFallbackRef = (0,react.useRef)(null);
    // Sessions resolved through getSession because the live list does not carry them
    // (hidden sessions). They are handed to the pane tree only, never to the session list.
    const [fallbackSessions, setFallbackSessions] = (0,react.useState)({});
    // Set when the deep link's session could not be opened: gone (not_found) or a
    // retryable lookup failure (failed). Rendered as a card, never a 404 or blank screen.
    const [lookup, setLookup] = (0,react.useState)(null);
    // Focus detail panel when session opens; return focus on close
    (0,react.useEffect)(()=>{
        if (selectedSession) {
            var _sessionDetailRef_current;
            (_sessionDetailRef_current = sessionDetailRef.current) === null || _sessionDetailRef_current === void 0 ? void 0 : _sessionDetailRef_current.focus();
        } else if (sessionTriggerRef.current) {
            sessionTriggerRef.current.focus();
            sessionTriggerRef.current = null;
        }
    }, [
        selectedSession
    ]);
    // Trap focus inside delete confirmation dialog; return focus on close
    (0,useFocusTrap/* useFocusTrap */.r)(deleteDialogRef, !!deleteConfirmTarget, lastFocusBeforeDelete);
    const { sessions, loading, error, deleteSession, pauseSession, resumeSession, renameSession, restartSession, pinSession, unpinSession, retrySession, clearConversationState, createCheckpoint, listCheckpoints, forkSession, listSessions, updateSession, getSession, hasLoadedOnce, error: sessionsError } = (0,SessionServiceContext/* useSessionServiceContext */.MS)();
    const paneSessions = (0,react.useMemo)(()=>{
        const extras = Object.values(fallbackSessions).filter((f)=>!sessions.some((s)=>s.id === f.id));
        return extras.length > 0 ? [
            ...sessions,
            ...extras
        ] : sessions;
    }, [
        sessions,
        fallbackSessions
    ]);
    const { resumeTarget, resumeTriggerRef, handleResumeRequest, handleDirectResume, handleResumeConfirm, handleResumeCancel } = useResumeSessionFlow(resumeSession, track);
    // Multi-window layer (Epic 2.2): useWindowManager owns the windows array/persistence;
    // useWindowUrlSync resolves which window this tab is showing from `?window=`. Neither
    // hook has a notion of "the active window" on its own (ADR-001) — currentWindow is
    // derived here by looking up currentWindowId in the windows array.
    // createWindow/closeWindow/renameWindow have no UI to call them from yet — WindowTabStrip
    // (Task 2.2.2b) is deferred to Epic 3.1, where it doesn't exist yet.
    const { windows, isRestored: isWindowsRestored, dispatchPane, createWindow, closeWindow, renameWindow } = useWindowManager(paneSessions);
    const { currentWindowId, switchToWindow } = useWindowUrlSync(windows, isWindowsRestored);
    var _windows_find;
    const currentWindow = (_windows_find = windows.find((w)=>w.id === currentWindowId)) !== null && _windows_find !== void 0 ? _windows_find : windows[0];
    const paneDispatch = (0,react.useCallback)((action)=>dispatchPane(currentWindow.id, action), [
        dispatchPane,
        currentWindow.id
    ]);
    // Bridges useWindowShortcuts' "," leader follow-up (which only knows a
    // window id) to WindowTabStrip's imperative beginEdit(id, name) — the same
    // inline editor double-click/F2 already open, not a separate native prompt.
    const handleWindowRenameRequest = (0,react.useCallback)((id)=>{
        var _windowTabStripRef_current;
        const target = windows.find((w)=>w.id === id);
        if (target) (_windowTabStripRef_current = windowTabStripRef.current) === null || _windowTabStripRef_current === void 0 ? void 0 : _windowTabStripRef_current.beginEdit(id, target.name);
    }, [
        windows
    ]);
    useWindowShortcuts(windows, currentWindow.id, switchToWindow, handleWindowRenameRequest);
    // Helper function to find a session by ID with fuzzy matching for external sessions
    const findSessionById = (0,react.useCallback)((sessionId)=>findSessionById_findSessionById(sessions, sessionId), [
        sessions
    ]);
    // Update URL with session and tab parameters
    const updateUrl = (0,react.useCallback)((sessionId, tab)=>{
        const params = new URLSearchParams();
        if (sessionId) {
            params.set("session", sessionId);
            if (tab && tab !== "info") {
                params.set("tab", tab);
            }
        }
        const query = params.toString();
        router.replace(query ? "/?".concat(query) : "/", {
            resetScroll: false
        });
    }, [
        router
    ]);
    // Handle pending session navigation from notification click
    (0,react.useEffect)(()=>{
        if (pendingSessionId && sessions.length > 0) {
            const session = findSessionById(pendingSessionId);
            if (session) {
                setSelectedSession(session);
                setActiveTab("terminal");
                updateUrl(session.id, "terminal");
            } else {
                console.warn("[Notification] Session not found: ".concat(pendingSessionId));
            }
            setPendingSessionId(null);
        }
    }, [
        pendingSessionId,
        sessions,
        findSessionById,
        updateUrl
    ]);
    // Routes to a resolved session — shared by the in-list (findSessionById)
    // and hidden-session (getSession fallback) paths below so both apply the
    // exact same tab/pane wiring.
    const routeToResolvedSession = (0,react.useCallback)((session, sessionId, tabParam, newPaneParam)=>{
        setSelectedSession(session);
        const resolvedTab = isValidTab(tabParam) ? tabParam : "terminal";
        setActiveTab(resolvedTab);
        // Only route to pane when URL params actually changed. The `sessions` dependency
        // causes this effect to re-run on every stream update; without this guard the
        // picker would re-appear after every session status change.
        if (lastUrlRoutedRef.current.sessionId !== sessionId || lastUrlRoutedRef.current.tab !== tabParam) {
            lastUrlRoutedRef.current = {
                sessionId,
                tab: tabParam
            };
            setExternalAssignCounter((c)=>c + 1);
            setExternalAssignSession({
                sessionId: session.id,
                tab: resolvedTab,
                forceNewPane: newPaneParam === "true"
            });
        }
        // Clean up newPane param from URL after consuming it
        if (newPaneParam === "true") {
            const params = new URLSearchParams();
            params.set("session", sessionId);
            if (tabParam) params.set("tab", tabParam);
            router.replace("/?".concat(params.toString()), {
                resetScroll: false
            });
        }
    }, [
        router
    ]);
    // Not in the locally-fetched list — e.g. a Diagnose & Nudge diagnostic
    // session, dispatched with hidden=true so it doesn't clutter the main
    // session list (server/services/session_service.go's
    // SpawnDiagnosticSession). ListSessions excludes hidden sessions by
    // default and this component never passes includeHidden, so such a
    // session can never appear in `sessions` — without this fallback, its
    // "View diagnostic session" deep link (StuckItemDetail.tsx) would silently
    // no-op. GetSession, unlike ListSessions, doesn't filter on Hidden, so a
    // direct-by-UUID lookup still resolves it. Guarded by
    // hiddenSessionFallbackRef so it fires once per sessionId, not on every
    // `sessions` stream update.
    const fetchHiddenSessionFallback = (0,react.useCallback)(function(link) {
        let force = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : false;
        const { sessionId } = link;
        if (!sessionId) return;
        if (!force && hiddenSessionFallbackRef.current === sessionId) return;
        hiddenSessionFallbackRef.current = sessionId;
        let failure = "not_found";
        getSession(sessionId, {
            onFailure: (err)=>{
                failure = classifyGetSessionFailure(err);
            }
        }).then((fetched)=>{
            // A newer deep link superseded this lookup while it was in flight.
            if (hiddenSessionFallbackRef.current !== sessionId) return;
            if (!fetched) {
                setLookup({
                    sessionId,
                    notificationId: link.notificationId,
                    status: failure === "failed" ? "failed" : "not_found"
                });
                return;
            }
            setLookup(null);
            setFallbackSessions((prev)=>{
                const kept = Object.entries(prev).filter((param)=>{
                    let [id] = param;
                    return id !== fetched.id;
                }).slice(-(MAX_FALLBACK_SESSIONS - 1));
                return {
                    ...Object.fromEntries(kept),
                    [fetched.id]: fetched
                };
            });
            routeToResolvedSession(fetched, sessionId, link.tab, link.newPane);
        });
    }, [
        getSession,
        routeToResolvedSession
    ]);
    // Handle direct session selection from URL. A cold deep link arrives with an empty
    // list: the fallback runs once the list request has settled, not on `sessions.length`.
    (0,react.useEffect)(()=>{
        const link = parseSessionDeepLink(searchParams);
        if (!link.sessionId) {
            hiddenSessionFallbackRef.current = null;
            setLookup((prev)=>prev ? null : prev);
            return;
        }
        const session = findSessionById(link.sessionId);
        if (session) {
            setLookup((prev)=>prev ? null : prev);
            routeToResolvedSession(session, link.sessionId, link.tab, link.newPane);
            return;
        }
        const listSettled = hasLoadedOnce === true || sessions.length > 0 || sessionsError != null;
        if (!shouldResolveViaGetSession({
            sessionId: link.sessionId,
            foundInList: false,
            listSettled
        })) return;
        fetchHiddenSessionFallback(link);
    }, [
        searchParams,
        sessions,
        hasLoadedOnce,
        sessionsError,
        findSessionById,
        routeToResolvedSession,
        fetchHiddenSessionFallback
    ]);
    // Keep a fallback session's status current: the watch stream never carries hidden sessions.
    const openFallbackIds = (0,react.useMemo)(()=>Object.keys(fallbackSessions).join(","), [
        fallbackSessions
    ]);
    (0,react.useEffect)(()=>{
        const ids = openFallbackIds ? openFallbackIds.split(",") : [];
        if (ids.length === 0) return;
        const timer = setInterval(()=>{
            for (const id of ids){
                void getSession(id, {
                    onFailure: ()=>{}
                }).then((fresh)=>{
                    if (fresh) setFallbackSessions((prev)=>prev[id] ? {
                            ...prev,
                            [id]: fresh
                        } : prev);
                });
            }
        }, FALLBACK_SESSION_REFRESH_MS);
        return ()=>clearInterval(timer);
    }, [
        openFallbackIds,
        getSession
    ]);
    var _lookup_notificationId, _lookup_sessionId;
    const lookupNotification = useNotificationRecord(notificationHistory, (_lookup_notificationId = lookup === null || lookup === void 0 ? void 0 : lookup.notificationId) !== null && _lookup_notificationId !== void 0 ? _lookup_notificationId : null, (_lookup_sessionId = lookup === null || lookup === void 0 ? void 0 : lookup.sessionId) !== null && _lookup_sessionId !== void 0 ? _lookup_sessionId : null, (lookup === null || lookup === void 0 ? void 0 : lookup.status) === "not_found");
    // Detect ?new=true, ?pr=<url>, ?duplicate=<id>, or ?worktree=<path>&branch=<branch> query params
    useOmnibarQueryParamLaunch(searchParams, router, openOmnibar, track, getSession);
    // Close session and clear URL query parameter
    const closeSession = ()=>{
        setSelectedSession(null);
        setActiveTab("info");
        lastUrlRoutedRef.current = {
            sessionId: null,
            tab: null
        };
        updateUrl(null, null);
    };
    // Handle session deletion
    const handleDeleteSession = async (sessionId)=>{
        if ((selectedSession === null || selectedSession === void 0 ? void 0 : selectedSession.id) === sessionId) {
            closeSession();
            await new Promise((resolve)=>setTimeout(resolve, 100));
        }
        track({
            name: "session_deleted",
            category: "user_action"
        });
        await deleteSession(sessionId);
    };
    // Handle new workspace on same project
    const handleNewWorkspaceSession = (sessionId)=>{
        track({
            name: "session_new_workspace_initiated",
            category: "user_action"
        });
        getSession(sessionId).then((session)=>{
            openOmnibar(session === null || session === void 0 ? void 0 : session.repoRoot);
        }).catch(()=>{
            openOmnibar();
        });
    };
    const handleCloneSession = (0,react.useCallback)((_sessionId)=>{
        openInCreationMode();
    }, [
        openInCreationMode
    ]);
    const handleNewSession = ()=>{
        openInCreationMode();
    };
    const handleUpdateTags = async (sessionId, tags)=>{
        if (tags.length > 0) {
            track({
                name: "session_tags_updated",
                category: "user_action"
            });
            await updateSession(sessionId, {
                tags
            });
        }
    };
    const handleSetRateLimitEnabled = (0,react.useCallback)(async (sessionId, enabled)=>{
        track({
            name: "session_rate_limit_updated",
            category: "user_action"
        });
        await updateSession(sessionId, {
            rateLimitEnabled: enabled
        });
    }, [
        updateSession,
        track
    ]);
    const handleToggleAutonomousMode = (0,react.useCallback)(async (sessionId, enabled)=>{
        track({
            name: "session_autonomous_mode_updated",
            category: "user_action"
        });
        try {
            await updateSession(sessionId, {
                autonomousMode: enabled
            });
        } catch (err) {
            console.error("[page] toggleAutonomousMode failed:", err);
        }
    }, [
        updateSession,
        track
    ]);
    const handleTogglePinned = (0,react.useCallback)(async (sessionId, pinned)=>{
        track({
            name: "session_pinned_updated",
            category: "user_action"
        });
        await (pinned ? pinSession(sessionId) : unpinSession(sessionId));
    }, [
        pinSession,
        unpinSession,
        track
    ]);
    const handleToggleAutoApprove = (0,react.useCallback)(async (sessionId, enabled)=>{
        track({
            name: "session_auto_approve_updated",
            category: "user_action"
        });
        try {
            await updateSession(sessionId, {
                autoApprove: enabled
            });
        } catch (err) {
            console.error("[page] toggleAutoApprove failed:", err);
        }
    }, [
        updateSession,
        track
    ]);
    const handleSteerAutonomousSession = (0,react.useCallback)(async (sessionId, message)=>{
        track({
            name: "session_autonomous_steer",
            category: "user_action"
        });
        const result = await updateSession(sessionId, {
            steerMessage: message
        });
        if (result === null) {
            addNotification({
                message: "Failed to send steering message — the session may not be running.",
                notificationType: "error",
                sessionId,
                sessionName: ""
            });
            return false;
        }
        return true;
    }, [
        updateSession,
        track,
        addNotification
    ]);
    const handleSessionClick = (session)=>{
        sessionTriggerRef.current = document.activeElement;
        if (typeof performance !== "undefined") {
            performance.mark("session:click");
        }
        setSelectedSession(session);
        setActiveTab("info");
        // Pre-mark the URL state so the URL-watching effect skips re-routing when
        // searchParams updates in response to this updateUrl call. "info" tab is not
        // added to the URL (filtered out), so the stored tabParam is null.
        lastUrlRoutedRef.current = {
            sessionId: session.id,
            tab: null
        };
        updateUrl(session.id, "info");
        // Also route the session to the currently-focused tiling pane
        setExternalAssignCounter((c)=>c + 1);
        setExternalAssignSession({
            sessionId: session.id,
            tab: "info"
        });
    };
    const handleTabChange = (tab)=>{
        setActiveTab(tab);
        if (selectedSession) {
            // Pre-mark the URL state before the URL changes so the URL-watching effect
            // does not re-route the session to a pane (which would show the picker).
            // "info" is not written to the URL, so its tabParam is null.
            lastUrlRoutedRef.current = {
                sessionId: selectedSession.id,
                tab: tab !== "info" ? tab : null
            };
            updateUrl(selectedSession.id, tab);
        }
    };
    // Story 3.2 — j/k keyboard navigation in session list
    // When no session is open, j/k move the focus index; Enter opens the focused session.
    // When a session is open, p/r/d act on the currently-open session.
    (0,useKeyboard/* useKeyboard */.d)({
        // '?' is handled exclusively by CockpitShell's useShortcut to avoid dual-listener collision
        Escape: ()=>{
            if (deleteConfirmTarget) {
                setDeleteConfirmTarget(null);
            } else if (resumeTarget) {
                handleResumeCancel();
            } else if (selectedSession) {
                closeSession();
            }
        },
        "R": ()=>{
            if (!loading) {
                track({
                    name: "sessions_refreshed",
                    category: "user_action"
                });
                listSessions();
            }
        },
        // j/k navigation (only when no modal is open)
        "j": ()=>{
            if (deleteConfirmTarget || resumeTarget) return;
            setFocusedSessionIndex((prev)=>sessions.length === 0 ? -1 : Math.min(prev + 1, sessions.length - 1));
        },
        // List/Board view toggle. useKeyboard's default ignoreElements (INPUT/TEXTAREA/SELECT)
        // already covers the instant-search box, so this never fires while typing "b" into it.
        "b": ()=>{
            if (deleteConfirmTarget || resumeTarget) return;
            setViewMode(viewMode === "board" ? "list" : "board");
        },
        "k": ()=>{
            if (deleteConfirmTarget || resumeTarget) return;
            setFocusedSessionIndex((prev)=>sessions.length === 0 ? -1 : Math.max(prev - 1, 0));
        },
        Enter: ()=>{
            if (deleteConfirmTarget || resumeTarget) return;
            if (!selectedSession && focusedSessionIndex >= 0 && sessions[focusedSessionIndex]) {
                handleSessionClick(sessions[focusedSessionIndex]);
            }
        },
        // p/r/d act on the open session
        "p": ()=>{
            if (selectedSession && !deleteConfirmTarget) {
                track({
                    name: "session_paused",
                    category: "user_action"
                });
                pauseSession(selectedSession.id);
            }
        },
        "r": ()=>{
            if (selectedSession && !deleteConfirmTarget) {
                handleResumeRequest(selectedSession);
            }
        },
        "d": ()=>{
            if (selectedSession && !deleteConfirmTarget) {
                var _sessionTriggerRef_current;
                // document.activeElement is unreliable here: the effect above focuses
                // sessionDetailRef (the cockpit container) whenever selectedSession is
                // set, so by the time this fires, activeElement is that container, not
                // the row that was actually clicked. sessionTriggerRef still holds the
                // real opener; fall back to activeElement only for the deep-link case
                // where a session was selected without a click (no captured trigger).
                lastFocusBeforeDelete.current = (_sessionTriggerRef_current = sessionTriggerRef.current) !== null && _sessionTriggerRef_current !== void 0 ? _sessionTriggerRef_current : document.activeElement;
                setDeleteConfirmTarget(selectedSession);
            }
        },
        // t — jump to terminal tab
        "t": ()=>{
            if (selectedSession && !deleteConfirmTarget) {
                handleTabChange("terminal");
            }
        }
    });
    const cockpitActions = (0,react.useMemo)(()=>({
            onSessionClick: handleSessionClick,
            onDeleteSession: handleDeleteSession,
            onPauseSession: pauseSession,
            onResumeSession: handleResumeRequest,
            onDirectResumeSession: handleDirectResume,
            onCloneSession: handleCloneSession,
            onNewWorkspaceSession: handleNewWorkspaceSession,
            onRenameSession: renameSession,
            onRestartSession: restartSession,
            onRetryNowSession: retrySession,
            onUpdateTags: handleUpdateTags,
            onNewSession: handleNewSession,
            onCreateCheckpoint: createCheckpoint,
            onListCheckpoints: listCheckpoints,
            onForkFromCheckpoint: forkSession,
            onSetRateLimitEnabled: handleSetRateLimitEnabled,
            onToggleAutonomousMode: handleToggleAutonomousMode,
            onTogglePinned: handleTogglePinned,
            onToggleAutoApprove: handleToggleAutoApprove,
            onSteerAutonomousSession: handleSteerAutonomousSession,
            onClearConversationState: clearConversationState,
            onListSessions: listSessions
        }), [
        handleSessionClick,
        handleDeleteSession,
        pauseSession,
        handleResumeRequest,
        handleDirectResume,
        handleCloneSession,
        handleNewWorkspaceSession,
        renameSession,
        restartSession,
        retrySession,
        handleUpdateTags,
        handleNewSession,
        createCheckpoint,
        listCheckpoints,
        forkSession,
        handleSetRateLimitEnabled,
        handleToggleAutonomousMode,
        handleTogglePinned,
        handleToggleAutoApprove,
        handleSteerAutonomousSession,
        clearConversationState,
        listSessions
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: page,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(CockpitActionsProvider, {
                value: cockpitActions,
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(SessionViewModeProvider, {
                    value: {
                        viewMode,
                        setViewMode
                    },
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(WindowTabStrip, {
                            ref: windowTabStripRef,
                            windows: windows,
                            currentWindowId: currentWindow.id,
                            onSwitch: switchToWindow,
                            onCreate: ()=>switchToWindow(createWindow()),
                            onClose: (id)=>{
                                const next = closeWindow(id);
                                if (id === currentWindow.id && next) switchToWindow(next);
                            },
                            onRename: renameWindow
                        }),
                        lookup ? /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionUnavailableCard, {
                            variant: lookup.status === "failed" ? "failed" : "unavailable",
                            sessionLabel: lookup.sessionId.slice(0, 8),
                            notification: lookup.status === "not_found" ? lookupNotification : null,
                            onOpenNotifications: ()=>router.push("/notifications"),
                            onGoToSessions: ()=>{
                                setLookup(null);
                                updateUrl(null, null);
                            },
                            onRetry: ()=>{
                                const link = parseSessionDeepLink(searchParams);
                                setLookup(null);
                                fetchHiddenSessionFallback(link, true);
                            }
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            ref: sessionDetailRef,
                            className: cockpitContainer,
                            tabIndex: -1,
                            role: "region",
                            "aria-label": "Session cockpit",
                            "data-context": "cockpit",
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(PaneTilingContainer, {
                                sessions: paneSessions,
                                paneState: currentWindow.paneState,
                                dispatch: paneDispatch,
                                externalSessionAssign: externalAssignSession ? {
                                    ...externalAssignSession,
                                    version: externalAssignCounter
                                } : null
                            })
                        })
                    ]
                })
            }),
            resumeTarget && /*#__PURE__*/ (0,jsx_runtime.jsx)(ResumeSessionModal/* ResumeSessionModal */.h, {
                session: resumeTarget,
                sessions: sessions,
                onConfirm: handleResumeConfirm,
                onCancel: handleResumeCancel,
                triggerRef: resumeTriggerRef
            }, resumeTarget.id),
            deleteConfirmTarget && /*#__PURE__*/ (0,jsx_runtime.jsx)(DeleteSessionConfirmModal, {
                target: deleteConfirmTarget,
                dialogRef: deleteDialogRef,
                onCancel: ()=>setDeleteConfirmTarget(null),
                onConfirm: handleDeleteSession
            })
        ]
    });
}
function Home() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        fallback: /*#__PURE__*/ (0,jsx_runtime.jsx)(SessionListSkeleton, {
            count: 4
        }),
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(HomeContent, {})
    });
}


/***/ }),

/***/ 72301:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   k: () => (/* binding */ truncateMiddle)
/* harmony export */ });
/**
 * Truncates a filename using middle truncation, preserving both the start of
 * the name and the file extension. E.g.:
 *   truncateMiddle("very-long-filename.tsx", 18) → "very-lon…name.tsx"
 */ function truncateMiddle(name, maxLen) {
    if (!name) return name;
    if (name.length <= maxLen) return name;
    // For very small maxLen, just truncate from the right with an ellipsis.
    if (maxLen < 5) {
        return name.slice(0, maxLen - 1) + "…";
    }
    // Find the extension. Only treat a trailing dot as a real filename extension
    // when nothing after it is a further path segment — a genuine extension like
    // ".verylongext" has no "/" after the dot, no matter how long it is, but a
    // dotted path segment (e.g. "github.com/some-org/repo-name") always does.
    // Treating that whole tail as a preserved "suffix" starves `keep`'s head/tail
    // budget and forces the right-truncation fallback below, which drops the most
    // identifying trailing path segment instead of truncating the middle.
    const dotIdx = name.lastIndexOf(".");
    let suffix;
    let base;
    if (dotIdx > 0 && !name.slice(dotIdx).includes("/")) {
        suffix = name.slice(dotIdx); // e.g. ".tsx"
        base = name.slice(0, dotIdx);
    } else {
        suffix = "";
        base = name;
    }
    // How many chars of the base we can show (1 char budget for the ellipsis "…").
    let keep = maxLen - suffix.length - 1;
    // Ensure meaningful truncation: we need at least 1 char head + 1 char tail.
    if (keep < 2) {
        // Fall back to right-truncation with ellipsis.
        return name.slice(0, maxLen - 1) + "…";
    }
    let head = Math.ceil(keep * 0.6);
    let tail = keep - head;
    // Guarantee at least 1 char on each side.
    if (head < 1) head = 1;
    if (tail < 1) {
        tail = 1;
        head = keep - 1;
    }
    return base.slice(0, head) + "…" + base.slice(base.length - tail) + suffix;
}


/***/ }),

/***/ 74512:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B2: () => (/* binding */ prBadgeBlocking),
/* harmony export */   BS: () => (/* binding */ priorityLabel),
/* harmony export */   Eg: () => (/* binding */ prBadge),
/* harmony export */   Kk: () => (/* binding */ icon),
/* harmony export */   L7: () => (/* binding */ prBadgeComplete),
/* harmony export */   Qq: () => (/* binding */ text),
/* harmony export */   VG: () => (/* binding */ prBadgeReady),
/* harmony export */   hC: () => (/* binding */ prBadgeDraft),
/* harmony export */   ho: () => (/* binding */ prBadgePending),
/* harmony export */   m2: () => (/* binding */ repoBadge),
/* harmony export */   o5: () => (/* binding */ prBadgeUnknown),
/* harmony export */   oE: () => (/* binding */ compact),
/* harmony export */   qS: () => (/* binding */ badge),
/* harmony export */   sv: () => (/* binding */ prBadgeError)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FGitHubBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W227bMAx9z1fwsQFiQ3Lu6st_Y0_FYtGxUEfSJDlxO_TfB8tOYnvJ3C3FgCCIKfLwHJKxGL9QR8V65Qn8nAAI6UzB3xhIVUiFUVZg9TwB4IXcq0h6PDgGKSqPtjbvuWGwMMHFcCGk2odnoKQx7rQVaCPLhSwdA5o05kwrHzn5jgPTCeU_9wxWhNQ2j5WPBKbaci_1YqC0wnBguXKysfGiABInDpC7cJiW1mnLwGh55tnQYEBNBU4XUjQIhltU_nnyMYkvZWC5PqINxQg_mbYH1vwsuMfvTxE11bQBrSKXc6FPDAgkpgrK7X7Hn8gM2k9MpwN8nnp5xD8kIMOITKelCwG69HVbWMjWCDly_xRFL3SL74l9PwRmrVuks8yhD949SBrAeg3b_N4YOohKQtSOp697q0sl2M3cqS7q4vePfkw7w3DTw_RFJ5023M9o7vRhde4DnZMZbMgMkmReN2PezzIfwU_vKyKjinCQ61OKNqOwckzz_dlbjCSn5JEO0sHcLjqCteGp9G8MSLz9PP2kD7gcoV8_wr4c5Ppq8qsR8tUjo1YNcn01_fUI_eSRyg9zPUyeLvuIm7Gxf2jqmxfuleimn_uWGtpz2f7Xf_X2q2eD_s7lP7tO5Q_Pht9wF2A6l565MY2sm62kQK9Rxu5OnF9bZKYzPFw2VM6l3lpDNq0XUeGRNtrNyowO1_NlyI0tuuWEugny_UMrl8kXoS6HbjdS9WF6QhLg7CTFD5nQNsNLW_1nJ_r5S5yuZXqlQHpxYsmPpceg2Csd6_T5SYoOqLNirrQuRQC1aUE1wMsCmmcdA3RKmqZJIQ0RL8dUEgOLrWICrgS8NRxW682ppoGCnF_P72xodTGyyozv64yH93otI0_VyQ5R15qktyIEm1UhxpdkrPjxy_hBehOPAsAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(78554);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FGitHubBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W227bMAx9z1fwsQFiQ3Lu6st_Y0_FYtGxUEfSJDlxO_TfB8tOYnvJ3C3FgCCIKfLwHJKxGL9QR8V65Qn8nAAI6UzB3xhIVUiFUVZg9TwB4IXcq0h6PDgGKSqPtjbvuWGwMMHFcCGk2odnoKQx7rQVaCPLhSwdA5o05kwrHzn5jgPTCeU_9wxWhNQ2j5WPBKbaci_1YqC0wnBguXKysfGiABInDpC7cJiW1mnLwGh55tnQYEBNBU4XUjQIhltU_nnyMYkvZWC5PqINxQg_mbYH1vwsuMfvTxE11bQBrSKXc6FPDAgkpgrK7X7Hn8gM2k9MpwN8nnp5xD8kIMOITKelCwG69HVbWMjWCDly_xRFL3SL74l9PwRmrVuks8yhD949SBrAeg3b_N4YOohKQtSOp697q0sl2M3cqS7q4vePfkw7w3DTw_RFJ5023M9o7vRhde4DnZMZbMgMkmReN2PezzIfwU_vKyKjinCQ61OKNqOwckzz_dlbjCSn5JEO0sHcLjqCteGp9G8MSLz9PP2kD7gcoV8_wr4c5Ppq8qsR8tUjo1YNcn01_fUI_eSRyg9zPUyeLvuIm7Gxf2jqmxfuleimn_uWGtpz2f7Xf_X2q2eD_s7lP7tO5Q_Pht9wF2A6l565MY2sm62kQK9Rxu5OnF9bZKYzPFw2VM6l3lpDNq0XUeGRNtrNyowO1_NlyI0tuuWEugny_UMrl8kXoS6HbjdS9WF6QhLg7CTFD5nQNsNLW_1nJ_r5S5yuZXqlQHpxYsmPpceg2Csd6_T5SYoOqLNirrQuRQC1aUE1wMsCmmcdA3RKmqZJIQ0RL8dUEgOLrWICrgS8NRxW682ppoGCnF_P72xodTGyyozv64yH93otI0_VyQ5R15qktyIEm1UhxpdkrPjxy_hBehOPAsAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fshared_2FGitHubBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71W227bMAx9z1fwsQFiQ3Lu6st_Y0_FYtGxUEfSJDlxO_TfB8tOYnvJ3C3FgCCIKfLwHJKxGL9QR8V65Qn8nAAI6UzB3xhIVUiFUVZg9TwB4IXcq0h6PDgGKSqPtjbvuWGwMMHFcCGk2odnoKQx7rQVaCPLhSwdA5o05kwrHzn5jgPTCeU_9wxWhNQ2j5WPBKbaci_1YqC0wnBguXKysfGiABInDpC7cJiW1mnLwGh55tnQYEBNBU4XUjQIhltU_nnyMYkvZWC5PqINxQg_mbYH1vwsuMfvTxE11bQBrSKXc6FPDAgkpgrK7X7Hn8gM2k9MpwN8nnp5xD8kIMOITKelCwG69HVbWMjWCDly_xRFL3SL74l9PwRmrVuks8yhD949SBrAeg3b_N4YOohKQtSOp697q0sl2M3cqS7q4vePfkw7w3DTw_RFJ5023M9o7vRhde4DnZMZbMgMkmReN2PezzIfwU_vKyKjinCQ61OKNqOwckzz_dlbjCSn5JEO0sHcLjqCteGp9G8MSLz9PP2kD7gcoV8_wr4c5Ppq8qsR8tUjo1YNcn01_fUI_eSRyg9zPUyeLvuIm7Gxf2jqmxfuleimn_uWGtpz2f7Xf_X2q2eD_s7lP7tO5Q_Pht9wF2A6l565MY2sm62kQK9Rxu5OnF9bZKYzPFw2VM6l3lpDNq0XUeGRNtrNyowO1_NlyI0tuuWEugny_UMrl8kXoS6HbjdS9WF6QhLg7CTFD5nQNsNLW_1nJ_r5S5yuZXqlQHpxYsmPpceg2Csd6_T5SYoOqLNirrQuRQC1aUE1wMsCmmcdA3RKmqZJIQ0RL8dUEgOLrWICrgS8NRxW682ppoGCnF_P72xodTGyyozv64yH93otI0_VyQ5R15qktyIEm1UhxpdkrPjxy_hBehOPAsAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);

var badge = '_1s1d76t0';
var compact = '_1s1d76t1';
var icon = '_1s1d76tc';
var prBadge = '_1s1d76t2';
var prBadgeBlocking = '_1s1d76t4';
var prBadgeComplete = '_1s1d76t8';
var prBadgeDraft = '_1s1d76t7';
var prBadgeError = '_1s1d76t9';
var prBadgePending = '_1s1d76t6';
var prBadgeReady = '_1s1d76t5';
var prBadgeUnknown = '_1s1d76ta';
var priorityLabel = '_1s1d76tb';
var repoBadge = '_1s1d76t3';
var text = '_1s1d76td';

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

/***/ 88253:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   P: () => (/* binding */ usePersistedViewState)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ usePersistedViewState auto */ 
function readRaw(key) {
    if (false) {}
    try {
        return window.localStorage.getItem(key);
    } catch (e) {
        return null;
    }
}
function writeRaw(key, value) {
    if (false) {}
    try {
        window.localStorage.setItem(key, value);
    } catch (e) {
    // Ignore storage errors (private browsing / quota exceeded) — state
    // still lives in memory, it just won't survive a reload.
    }
}
function removeRaw(key) {
    if (false) {}
    try {
        window.localStorage.removeItem(key);
    } catch (e) {
    // Ignore storage errors
    }
}
function loadField(config) {
    const raw = readRaw(config.key);
    if (raw === null) return config.defaultValue;
    let parsed;
    try {
        parsed = JSON.parse(raw);
    } catch (e) {
        return config.defaultValue;
    }
    const value = config.deserialize ? config.deserialize(parsed) : parsed;
    if (config.isValid && !config.isValid(value)) return config.defaultValue;
    return value;
}
function saveField(config, value) {
    const serialized = config.serialize ? config.serialize(value) : value;
    writeRaw(config.key, JSON.stringify(serialized));
}
function buildDefaults(fields) {
    const out = {};
    Object.keys(fields).forEach((name)=>{
        out[name] = fields[name].defaultValue;
    });
    return out;
}
/**
 * Generic hydration-safe localStorage persistence for a page/component's view
 * state (search, filters, sort, grouping, etc). State always starts at the
 * hardcoded defaults so SSR/static-export first paint can never mismatch a
 * client-only localStorage read; a mount-only effect then loads persisted
 * values and flips `isHydrated`.
 */ function usePersistedViewState(fields) {
    const fieldsRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(fields);
    fieldsRef.current = fields;
    const defaults = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>buildDefaults(fields), [
        fields
    ]);
    const defaultsRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(defaults);
    defaultsRef.current = defaults;
    const [state, setState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(defaults);
    const [isHydrated, setIsHydrated] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    // Skips the persistence effect's next run once, so resetToDefaults's
    // removeRaw calls aren't immediately undone by re-saving the new (default)
    // state back into localStorage.
    const skipPersistRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(false);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const loaded = {};
        Object.keys(fieldsRef.current).forEach((name)=>{
            loaded[name] = loadField(fieldsRef.current[name]);
        });
        setState(loaded);
        setIsHydrated(true);
    // Mount-only: hydration must run exactly once regardless of prop identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!isHydrated) return;
        if (skipPersistRef.current) {
            skipPersistRef.current = false;
            return;
        }
        Object.keys(fieldsRef.current).forEach((name)=>{
            saveField(fieldsRef.current[name], state[name]);
        });
    }, [
        state,
        isHydrated
    ]);
    const setters = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const out = {};
        Object.keys(fields).forEach((name)=>{
            out[name] = (value)=>{
                setState((prev)=>({
                        ...prev,
                        [name]: typeof value === "function" ? value(prev[name]) : value
                    }));
            };
        });
        return out;
    }, [
        fields
    ]);
    const clearStorage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        Object.keys(fieldsRef.current).forEach((name)=>{
            removeRaw(fieldsRef.current[name].key);
        });
    }, []);
    const resetToDefaults = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>{
        skipPersistRef.current = true;
        clearStorage();
        setState(defaultsRef.current);
    }, [
        clearStorage
    ]);
    return {
        state,
        setters,
        isHydrated,
        resetToDefaults,
        clearStorage
    };
}


/***/ }),

/***/ 90646:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  m: () => (/* binding */ Tooltip)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-tooltip@1.2.8+b721ee6ccd61acaa/node_modules/@radix-ui/react-tooltip/dist/index.mjs + 9 modules
var dist = __webpack_require__(21303);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FTooltip.css.ts.vanilla.css%22%2C%22source%22%3A%22LmI4a2J1NjAgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBwYWRkaW5nOiA0cHggOHB4OwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgYm94LXNoYWRvdzogMCA0cHggMTJweCByZ2JhKDAsMCwwLDAuMyk7CiAgei1pbmRleDogOTk5OTsKICB1c2VyLXNlbGVjdDogbm9uZTsKICBhbmltYXRpb24tZHVyYXRpb246IDQwMG1zOwogIGFuaW1hdGlvbi10aW1pbmctZnVuY3Rpb246IGN1YmljLWJlemllcigwLjE2LCAxLCAwLjMsIDEpOwogIHdpbGwtY2hhbmdlOiB0cmFuc2Zvcm0sIG9wYWNpdHk7Cn0KLmI4a2J1NjBbZGF0YS1zdGF0ZT0iZGVsYXllZC1vcGVuIl1bZGF0YS1zaWRlPSJ0b3AiXSB7CiAgYW5pbWF0aW9uLW5hbWU6IHNsaWRlRG93bkFuZEZhZGU7Cn0KLmI4a2J1NjBbZGF0YS1zdGF0ZT0iZGVsYXllZC1vcGVuIl1bZGF0YS1zaWRlPSJyaWdodCJdIHsKICBhbmltYXRpb24tbmFtZTogc2xpZGVMZWZ0QW5kRmFkZTsKfQouYjhrYnU2MFtkYXRhLXN0YXRlPSJkZWxheWVkLW9wZW4iXVtkYXRhLXNpZGU9ImJvdHRvbSJdIHsKICBhbmltYXRpb24tbmFtZTogc2xpZGVVcEFuZEZhZGU7Cn0KLmI4a2J1NjBbZGF0YS1zdGF0ZT0iZGVsYXllZC1vcGVuIl1bZGF0YS1zaWRlPSJsZWZ0Il0gewogIGFuaW1hdGlvbi1uYW1lOiBzbGlkZVJpZ2h0QW5kRmFkZTsKfQouYjhrYnU2MSB7CiAgZmlsbDogdmFyKC0tXzE5ZXoycno3KTsKfQouYjhrYnU2MiB7CiAgZGlzcGxheTogYmxvY2s7CiAgbWFyZ2luLXRvcDogMnB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIHRleHQtZGVjb3JhdGlvbjogdW5kZXJsaW5lOwogIHVzZXItc2VsZWN0OiBhdXRvOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FTooltip_css_ts_vanilla_css_22_2C_22source_22_3A_22LmI4a2J1NjAgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6Nyk7CiAgYm9yZGVyOiAxcHggc29saWQgdmFyKC0tXzE5ZXoycnplKTsKICBib3JkZXItcmFkaXVzOiB2YXIoLS1fMTllejJyejM2KTsKICBwYWRkaW5nOiA0cHggOHB4OwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYSk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgYm94LXNoYWRvdzogMCA0cHggMTJweCByZ2JhKDAsMCwwLDAuMyk7CiAgei1pbmRleDogOTk5OTsKICB1c2VyLXNlbGVjdDogbm9uZTsKICBhbmltYXRpb24tZHVyYXRpb246IDQwMG1zOwogIGFuaW1hdGlvbi10aW1pbmctZnVuY3Rpb246IGN1YmljLWJlemllcigwLjE2LCAxLCAwLjMsIDEpOwogIHdpbGwtY2hhbmdlOiB0cmFuc2Zvcm0sIG9wYWNpdHk7Cn0KLmI4a2J1NjBbZGF0YS1zdGF0ZT0iZGVsYXllZC1vcGVuIl1bZGF0YS1zaWRlPSJ0b3AiXSB7CiAgYW5pbWF0aW9uLW5hbWU6IHNsaWRlRG93bkFuZEZhZGU7Cn0KLmI4a2J1NjBbZGF0YS1zdGF0ZT0iZGVsYXllZC1vcGVuIl1bZGF0YS1zaWRlPSJyaWdodCJdIHsKICBhbmltYXRpb24tbmFtZTogc2xpZGVMZWZ0QW5kRmFkZTsKfQouYjhrYnU2MFtkYXRhLXN0YXRlPSJkZWxheWVkLW9wZW4iXVtkYXRhLXNpZGU9ImJvdHRvbSJdIHsKICBhbmltYXRpb24tbmFtZTogc2xpZGVVcEFuZEZhZGU7Cn0KLmI4a2J1NjBbZGF0YS1zdGF0ZT0iZGVsYXllZC1vcGVuIl1bZGF0YS1zaWRlPSJsZWZ0Il0gewogIGFuaW1hdGlvbi1uYW1lOiBzbGlkZVJpZ2h0QW5kRmFkZTsKfQouYjhrYnU2MSB7CiAgZmlsbDogdmFyKC0tXzE5ZXoycno3KTsKfQouYjhrYnU2MiB7CiAgZGlzcGxheTogYmxvY2s7CiAgbWFyZ2luLXRvcDogMnB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIHRleHQtZGVjb3JhdGlvbjogdW5kZXJsaW5lOwogIHVzZXItc2VsZWN0OiBhdXRvOwp9_22_7D = __webpack_require__(14781);
;// ./src/components/ui/Tooltip.css.ts


var tooltipArrow = 'b8kbu61';
var tooltipContent = 'b8kbu60';
var tooltipLink = 'b8kbu62';
;// ./src/components/ui/Tooltip.tsx
/* __next_internal_client_entry_do_not_use__ Tooltip auto */ 



function Tooltip(param) {
    let { children, label, side = "top", helpSlug } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Provider */.Kq, {
        delayDuration: 400,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Root */.bL, {
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Trigger */.l9, {
                    asChild: true,
                    children: children
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Portal */.ZL, {
                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Content */.UC, {
                        className: tooltipContent,
                        sideOffset: 4,
                        side: side,
                        children: [
                            label,
                            helpSlug && /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                className: tooltipLink,
                                href: "/help?doc=".concat(encodeURIComponent(helpSlug)),
                                children: "Learn more →"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Arrow */.i3, {
                                className: tooltipArrow
                            })
                        ]
                    })
                })
            ]
        })
    });
}


/***/ }),

/***/ 99417:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 53407));


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,4202,9353,8182,7924,6000,4069,8362,820,30,3198,1020,8165,3575,8890,2513,2275,3117,4247,2805,218,8272,3313,3889,9691,3516,7358], () => (__webpack_exec__(99417)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-62c96267f045253c.js.map