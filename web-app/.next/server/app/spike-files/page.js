(() => {
var exports = {};
exports.id = 4853;
exports.ids = [4853];
exports.modules = {

/***/ 1708:
/***/ ((module) => {

"use strict";
module.exports = require("node:process");

/***/ }),

/***/ 3295:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/after-task-async-storage.external.js");

/***/ }),

/***/ 8086:
/***/ ((module) => {

"use strict";
module.exports = require("module");

/***/ }),

/***/ 10846:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/compiled/next-server/app-page.runtime.prod.js");

/***/ }),

/***/ 19121:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/action-async-storage.external.js");

/***/ }),

/***/ 21820:
/***/ ((module) => {

"use strict";
module.exports = require("os");

/***/ }),

/***/ 27910:
/***/ ((module) => {

"use strict";
module.exports = require("stream");

/***/ }),

/***/ 28354:
/***/ ((module) => {

"use strict";
module.exports = require("util");

/***/ }),

/***/ 29021:
/***/ ((module) => {

"use strict";
module.exports = require("fs");

/***/ }),

/***/ 29222:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 50455));


/***/ }),

/***/ 29294:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-async-storage.external.js");

/***/ }),

/***/ 33873:
/***/ ((module) => {

"use strict";
module.exports = require("path");

/***/ }),

/***/ 37752:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (__WEBPACK_DEFAULT_EXPORT__)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = ((0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call the default export of \"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/spike-files/page.tsx\" from the server, but it's on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/spike-files/page.tsx",
"default",
));


/***/ }),

/***/ 46526:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 37752));


/***/ }),

/***/ 50455:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ SpikeFilesPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+state@6.6.0/node_modules/@codemirror/state/dist/index.js + 1 modules
var dist = __webpack_require__(64614);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+view@6.41.0/node_modules/@codemirror/view/dist/index.js + 1 modules
var view_dist = __webpack_require__(87748);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+commands@6.10.3/node_modules/@codemirror/commands/dist/index.js
var commands_dist = __webpack_require__(19154);
;// ./src/app/spike-files/cm.tsx
/* __next_internal_client_entry_do_not_use__ hostileAttributes,sharedExtensions,genDoc,spikeRegistry,CmView auto */ 
// THROWAWAY (files-viewer-revamp Phase 0 spike): minimal CodeMirror 6 host shared by the spike variants.




/** Autocorrect-hostile content attributes (Story 0.1.5; applied for real in Task 1.2.1a). */ const hostileAttributes = view_dist/* EditorView */.Lz.contentAttributes.of({
    autocorrect: "off",
    autocapitalize: "off",
    spellcheck: "false"
});
/** view.setState replaces every extension, so each per-tab state must carry the shared ones itself. */ const sharedExtensions = [
    (0,view_dist/* lineNumbers */.$K)(),
    (0,commands_dist/* history */.b6)(),
    view_dist/* EditorView */.Lz.theme({
        "&": {
            height: "100%"
        },
        ".cm-scroller": {
            overflow: "auto"
        }
    })
];
function genDoc(lines) {
    return Array.from({
        length: lines
    }, (_, i)=>`L${i + 1} text`).join("\n");
}
function spikeRegistry() {
    const w = window;
    w.__spike ??= {
        views: {}
    };
    return w.__spike;
}
function CmView(props) {
    const host = (0,react.useRef)(null);
    const { testId, doc, readOnly, hostile, onReady } = props;
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!host.current) return;
        const extensions = [
            ...sharedExtensions
        ];
        if (readOnly) extensions.push(dist/* EditorState */.$t.readOnly.of(true), view_dist/* EditorView */.Lz.editable.of(false));
        if (hostile) extensions.push(hostileAttributes);
        const view = new view_dist/* EditorView */.Lz({
            state: dist/* EditorState */.$t.create({
                doc,
                extensions
            }),
            parent: host.current
        });
        spikeRegistry().views[testId] = view;
        onReady?.(view);
        return ()=>{
            view.destroy();
            delete spikeRegistry().views[testId];
        };
    }, [
        testId,
        doc,
        readOnly,
        hostile,
        onReady
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        ref: host,
        "data-testid": testId,
        style: {
            height: props.height,
            border: "1px solid #888",
            overflow: "hidden"
        }
    });
}

;// ./src/app/spike-files/bufferSwitcher.ts
// THROWAWAY (files-viewer-revamp Phase 0 spike). Seed of the per-tab EditorState swap (ADR-032).
/** Snapshot the leaving buffer, swap states on one view, and defer while an IME composes. */ function createSwitcher(view) {
    let current = null;
    let pending = null;
    const apply = (next)=>{
        if (current) {
            current.state = view.state;
            current.scroll = view.scrollSnapshot();
        }
        current = next;
        view.setState(next.state);
        if (next.scroll) view.dispatch({
            effects: next.scroll
        });
    };
    view.contentDOM.addEventListener("compositionend", ()=>{
        if (!pending) return;
        const next = pending;
        pending = null;
        apply(next);
    });
    return {
        attach (b) {
            current = b;
            view.setState(b.state);
        },
        switchTo (next) {
            if (view.composing) {
                pending = next;
                return "deferred";
            }
            apply(next);
            return "applied";
        },
        current: ()=>current
    };
}

;// ./src/app/spike-files/StateSwap.tsx
/* __next_internal_client_entry_do_not_use__ StateSwap auto */ 
// THROWAWAY (Task 0.1.4a): two 2,000-line buffers on one EditorView, switched with setState.






const LINES = 2000;
const makeBuffer = ()=>({
        state: dist/* EditorState */.$t.create({
            doc: genDoc(LINES),
            extensions: sharedExtensions
        }),
        scroll: null
    });
function StateSwap() {
    const buffers = (0,react.useRef)({
        a: makeBuffer(),
        b: makeBuffer()
    });
    const switcher = (0,react.useRef)(null);
    const [active, setActive] = (0,react.useState)("a");
    const onReady = (0,react.useCallback)((view)=>{
        switcher.current = createSwitcher(view);
        switcher.current.attach(buffers.current.a);
        spikeRegistry().api = {
            lineHeight: ()=>view.defaultLineHeight,
            scrollTop: ()=>view.scrollDOM.scrollTop,
            undoDepth: ()=>(0,commands_dist/* undoDepth */.mk)(view.state),
            selection: ()=>view.state.selection.ranges.map((r)=>[
                        r.anchor,
                        r.head
                    ]),
            goto (line, from, to) {
                const l = view.state.doc.line(line);
                view.dispatch({
                    selection: {
                        anchor: l.from + from,
                        head: l.from + to
                    }
                });
                view.dispatch({
                    effects: view_dist/* EditorView */.Lz.scrollIntoView(l.from, {
                        y: "start"
                    })
                });
            },
            type (text) {
                const at = view.state.selection.main.to;
                view.dispatch({
                    changes: {
                        from: at,
                        insert: text
                    },
                    selection: {
                        anchor: at + text.length
                    }
                });
            }
        };
    }, []);
    const select = (which)=>{
        switcher.current?.switchTo(buffers.current[which]);
        setActive(which);
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                role: "tablist",
                "aria-label": "Spike buffers",
                children: [
                    "a",
                    "b"
                ].map((k)=>/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                        role: "tab",
                        "aria-selected": active === k,
                        "data-testid": `spike-tab-${k}`,
                        onClick: ()=>select(k),
                        style: {
                            minHeight: 44,
                            minWidth: 44
                        },
                        children: [
                            "Buffer ",
                            k.toUpperCase()
                        ]
                    }, k))
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CmView, {
                testId: "spike-editor",
                doc: "",
                height: "400px",
                onReady: onReady
            })
        ]
    });
}

;// ./src/app/spike-files/AccessibleViewSpike.tsx
/* __next_internal_client_entry_do_not_use__ ACCESSIBLE_PAGE_SIZE,pageLabel,AccessibleViewSpike auto */ 
// THROWAWAY (Task 0.1.6a): prototype of the non-virtual Accessible view (ux.md 8.10); seed of Task 1.5.4a.

const ACCESSIBLE_PAGE_SIZE = 500;
function pageLabel(page, total, size = ACCESSIBLE_PAGE_SIZE) {
    const first = page * size + 1;
    const last = Math.min(total, (page + 1) * size);
    return `Lines ${first}-${last} of ${total}`;
}
const btn = {
    minHeight: 44,
    minWidth: 44
};
function AccessibleViewSpike({ lines }) {
    const [page, setPage] = (0,react.useState)(0);
    const [goto, setGoto] = (0,react.useState)("");
    const [announce, setAnnounce] = (0,react.useState)("");
    const pages = Math.ceil(lines.length / ACCESSIBLE_PAGE_SIZE);
    const start = page * ACCESSIBLE_PAGE_SIZE;
    const label = pageLabel(page, lines.length);
    const go = ()=>{
        const n = Number.parseInt(goto, 10);
        if (Number.isFinite(n) && n >= 1 && n <= lines.length) {
            setPage(Math.floor((n - 1) / ACCESSIBLE_PAGE_SIZE));
            setAnnounce(`Line ${n}`);
        }
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
        "aria-label": "Accessible view",
        "data-testid": "spike-accessible-view",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("nav", {
                "aria-label": "Pager",
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        style: btn,
                        disabled: page === 0,
                        onClick: ()=>setPage(page - 1),
                        children: "Previous page"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        "data-testid": "spike-page-label",
                        "aria-live": "polite",
                        children: label
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        style: btn,
                        disabled: page >= pages - 1,
                        onClick: ()=>setPage(page + 1),
                        children: "Next page"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        children: [
                            "Go to line ",
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                type: "number",
                                inputMode: "numeric",
                                value: goto,
                                onChange: (e)=>setGoto(e.target.value),
                                style: {
                                    minHeight: 44
                                }
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        style: btn,
                        onClick: go,
                        children: "Go"
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                role: "status",
                "data-testid": "spike-announce",
                children: announce
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("pre", {
                tabIndex: 0,
                "aria-label": label,
                "data-testid": "spike-accessible-page",
                style: {
                    maxHeight: 300,
                    overflow: "auto"
                },
                children: lines.slice(start, start + ACCESSIBLE_PAGE_SIZE).map((text, i)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        "data-testid": "spike-accessible-line",
                        style: {
                            display: "block"
                        },
                        children: `${start + i + 1} ${text}`
                    }, start + i))
            })
        ]
    });
}

;// ./src/app/spike-files/KeyCapturePanel.tsx
/* __next_internal_client_entry_do_not_use__ KeyCapturePanel auto */ 
// THROWAWAY (Task 0.1.5c): on-screen keydown capture; never sent anywhere. Removed with the page.

function KeyCapturePanel() {
    const [events, setEvents] = (0,react.useState)([]);
    const [copied, setCopied] = (0,react.useState)("");
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const onKey = (e)=>{
            const { key, code, keyCode, isComposing, isTrusted, ctrlKey, metaKey, altKey, shiftKey } = e;
            setEvents((prev)=>[
                    ...prev,
                    {
                        key,
                        code,
                        keyCode,
                        isComposing,
                        isTrusted,
                        ctrlKey,
                        metaKey,
                        altKey,
                        shiftKey
                    }
                ].slice(-300));
        };
        document.addEventListener("keydown", onKey, true);
        return ()=>document.removeEventListener("keydown", onKey, true);
    }, []);
    const json = JSON.stringify(events, null, 1);
    const copy = async ()=>{
        try {
            await navigator.clipboard.writeText(json);
            setCopied("Copied");
        } catch  {
            setCopied("Clipboard blocked: long-press the JSON box below and copy");
        }
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
        "aria-label": "Key event capture",
        "data-testid": "spike-key-panel",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                children: "Type below or in the editor. Each keydown is listed here and stays on this screen."
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                "data-testid": "spike-plain-input",
                "aria-label": "Plain text input",
                style: {
                    width: "100%",
                    minHeight: 44
                }
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                style: {
                    minHeight: 44
                },
                "data-testid": "spike-copy-json",
                onClick: copy,
                children: "Copy as JSON"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                style: {
                    minHeight: 44
                },
                onClick: ()=>setEvents([]),
                children: "Clear"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                role: "status",
                children: copied
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("p", {
                "data-testid": "spike-key-count",
                children: [
                    events.length,
                    " keydown events"
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("ol", {
                style: {
                    fontFamily: "monospace",
                    fontSize: 12,
                    maxHeight: 160,
                    overflow: "auto"
                },
                children: events.map((e, i)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("li", {
                        children: `key=${JSON.stringify(e.key)} code=${e.code} keyCode=${e.keyCode} isComposing=${e.isComposing} isTrusted=${e.isTrusted} ctrl=${e.ctrlKey} meta=${e.metaKey}`
                    }, i))
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("textarea", {
                readOnly: true,
                "aria-label": "Captured events as JSON",
                value: json,
                rows: 4,
                style: {
                    width: "100%"
                }
            })
        ]
    });
}

;// ./src/app/spike-files/ViewportReadout.tsx
/* __next_internal_client_entry_do_not_use__ ViewportReadout auto */ 
// THROWAWAY (Task 0.1.5a): live visualViewport numbers plus two bottom bars to compare against the soft keyboard.

function ViewportReadout() {
    const [m, setM] = (0,react.useState)({
        inner: 0,
        vvHeight: 0,
        vvOffsetTop: 0,
        keyboard: 0,
        maxKeyboard: 0
    });
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const vv = window.visualViewport;
        const read = ()=>{
            const vvHeight = vv?.height ?? window.innerHeight;
            const vvOffsetTop = vv?.offsetTop ?? 0;
            const keyboard = Math.max(0, Math.round(window.innerHeight - vvHeight - vvOffsetTop));
            setM((p)=>({
                    inner: window.innerHeight,
                    vvHeight: Math.round(vvHeight),
                    vvOffsetTop: Math.round(vvOffsetTop),
                    keyboard,
                    maxKeyboard: Math.max(p.maxKeyboard, keyboard)
                }));
        };
        read();
        vv?.addEventListener("resize", read);
        vv?.addEventListener("scroll", read);
        return ()=>{
            vv?.removeEventListener("resize", read);
            vv?.removeEventListener("scroll", read);
        };
    }, []);
    const bar = {
        position: "fixed",
        left: 0,
        right: 0,
        height: 44,
        color: "#fff",
        display: "flex",
        alignItems: "center",
        zIndex: 100000
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                "data-testid": "spike-vv-readout",
                style: {
                    fontFamily: "monospace",
                    fontSize: 12
                },
                children: `innerHeight=${m.inner} vv.height=${m.vvHeight} vv.offsetTop=${m.vvOffsetTop} keyboard=${m.keyboard} maxKeyboard=${m.maxKeyboard} size=${ true ? "" : 0}`
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                "data-testid": "spike-bar-fixed",
                style: {
                    ...bar,
                    bottom: 0,
                    background: "#a00"
                },
                children: "RED: fixed bottom:0 (layout viewport)"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                "data-testid": "spike-bar-pinned",
                style: {
                    ...bar,
                    bottom: "var(--keyboard-height, 0px)",
                    background: "#070"
                },
                children: "GREEN: pinned to visualViewport (--keyboard-height)"
            })
        ]
    });
}

;// ./src/app/spike-files/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 
// analytics-exempt
// THROWAWAY (files-viewer-revamp Phase 0 spikes 0.1.4a / 0.1.5 / 0.1.6). Delete after the owner sitting.
// Variants via ?v= : input (default, phone page), stateswap, a11y.






const A11Y_LINES = 3000;
function A11yVariant() {
    const text = (0,react.useMemo)(()=>genDoc(A11Y_LINES), []);
    const lines = (0,react.useMemo)(()=>text.split("\n"), [
        text
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("section", {
        role: "region",
        "aria-label": "Editor",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("a", {
                href: "#spike-accessible-view",
                "data-testid": "spike-skip-link",
                children: "Switch to accessible view"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                children: "CodeMirror 6 read view"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CmView, {
                testId: "spike-cm-read",
                doc: text,
                readOnly: true,
                height: "300px"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                children: "Accessible view"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(AccessibleViewSpike, {
                lines: lines
            })
        ]
    });
}
function InputVariant() {
    const doc = (0,react.useMemo)(()=>genDoc(5000), []);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        style: {
            paddingBottom: 100
        },
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h2", {
                children: "Input and viewport spike"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                children: "Tap in the editor so the soft keyboard opens. Check that the GREEN bar stays above the keyboard and the caret stays visible. Type `--` and a double-quoted word, then check for em dash or curly quotes."
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ViewportReadout, {}),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(CmView, {
                testId: "spike-input-editor",
                doc: doc,
                hostile: true,
                height: "260px"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(KeyCapturePanel, {})
        ]
    });
}
function SpikeFilesPage() {
    const [variant, setVariant] = (0,react.useState)(null);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        setVariant(new URLSearchParams(window.location.search).get("v") ?? "input");
    }, []);
    if (!variant) return null;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        "data-testid": `spike-variant-${variant}`,
        children: [
            variant === "stateswap" && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(StateSwap, {}),
            variant === "a11y" && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(A11yVariant, {}),
            variant === "input" && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(InputVariant, {})
        ]
    });
}


/***/ }),

/***/ 55591:
/***/ ((module) => {

"use strict";
module.exports = require("https");

/***/ }),

/***/ 63033:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/work-unit-async-storage.external.js");

/***/ }),

/***/ 73136:
/***/ ((module) => {

"use strict";
module.exports = require("node:url");

/***/ }),

/***/ 73566:
/***/ ((module) => {

"use strict";
module.exports = require("worker_threads");

/***/ }),

/***/ 74075:
/***/ ((module) => {

"use strict";
module.exports = require("zlib");

/***/ }),

/***/ 76760:
/***/ ((module) => {

"use strict";
module.exports = require("node:path");

/***/ }),

/***/ 79551:
/***/ ((module) => {

"use strict";
module.exports = require("url");

/***/ }),

/***/ 81630:
/***/ ((module) => {

"use strict";
module.exports = require("http");

/***/ }),

/***/ 83997:
/***/ ((module) => {

"use strict";
module.exports = require("tty");

/***/ }),

/***/ 95616:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   GlobalError: () => (/* reexport default from dynamic */ next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2___default.a),
/* harmony export */   __next_app__: () => (/* binding */ __next_app__),
/* harmony export */   pages: () => (/* binding */ pages),
/* harmony export */   routeModule: () => (/* binding */ routeModule),
/* harmony export */   tree: () => (/* binding */ tree)
/* harmony export */ });
/* harmony import */ var next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(54315);
/* harmony import */ var next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var next_dist_server_route_kind__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(34788);
/* harmony import */ var next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(92838);
/* harmony import */ var next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2___default = /*#__PURE__*/__webpack_require__.n(next_dist_client_components_error_boundary__WEBPACK_IMPORTED_MODULE_2__);
/* harmony import */ var next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(79793);
/* harmony import */ var next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3___default = /*#__PURE__*/__webpack_require__.n(next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__);
/* harmony reexport (unknown) */ var __WEBPACK_REEXPORT_OBJECT__ = {};
/* harmony reexport (unknown) */ for(const __WEBPACK_IMPORT_KEY__ in next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__) if(["default","tree","pages","GlobalError","__next_app__","routeModule"].indexOf(__WEBPACK_IMPORT_KEY__) < 0) __WEBPACK_REEXPORT_OBJECT__[__WEBPACK_IMPORT_KEY__] = () => next_dist_server_app_render_entry_base__WEBPACK_IMPORTED_MODULE_3__[__WEBPACK_IMPORT_KEY__]
/* harmony reexport (unknown) */ __webpack_require__.d(__webpack_exports__, __WEBPACK_REEXPORT_OBJECT__);
const module0 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 36776));
const module1 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.t.bind(__webpack_require__, 93250, 23));
const module2 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.t.bind(__webpack_require__, 76963, 23));
const module3 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.t.bind(__webpack_require__, 54552, 23));
const page4 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 37752));


// We inject the tree and pages here so that we can use them in the route
// module.
const tree = {
        children: [
        '',
        {
        children: [
        'spike-files',
        {
        children: ['__PAGE__', {}, {
          page: [page4, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/spike-files/page.tsx"],
          
        }]
      },
        {
        
        
      }
      ]
      },
        {
        'layout': [module0, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/layout.tsx"],
'not-found': [module1, "next/dist/client/components/not-found-error"],
'forbidden': [module2, "next/dist/client/components/forbidden-error"],
'unauthorized': [module3, "next/dist/client/components/unauthorized-error"],
        
      }
      ]
      }.children;
const pages = ["/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/spike-files/page.tsx"];


const __next_app_require__ = __webpack_require__
const __next_app_load_chunk__ = () => Promise.resolve()
const __next_app__ = {
    require: __next_app_require__,
    loadChunk: __next_app_load_chunk__
};

// Create and export the route module that will be consumed.
const routeModule = new next_dist_server_route_modules_app_page_module_compiled__WEBPACK_IMPORTED_MODULE_0__.AppPageRouteModule({
    definition: {
        kind: next_dist_server_route_kind__WEBPACK_IMPORTED_MODULE_1__.RouteKind.APP_PAGE,
        page: "/spike-files/page",
        pathname: "/spike-files",
        // The following aren't used in production.
        bundlePath: '',
        filename: '',
        appPaths: []
    },
    userland: {
        loaderTree: tree
    }
});

//# sourceMappingURL=app-page.js.map

/***/ })

};
;

// load runtime
var __webpack_require__ = require("../../webpack-runtime.js");
__webpack_require__.C(exports);
var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
var __webpack_exports__ = __webpack_require__.X(0, [4655,9154,4164], () => (__webpack_exec__(95616)));
module.exports = __webpack_exports__;

})();