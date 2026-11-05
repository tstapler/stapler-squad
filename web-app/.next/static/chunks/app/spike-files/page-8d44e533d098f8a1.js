(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[4853],{

/***/ 33686:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ SpikeFilesPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+state@6.6.0/node_modules/@codemirror/state/dist/index.js + 1 modules
var dist = __webpack_require__(24183);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+view@6.41.0/node_modules/@codemirror/view/dist/index.js
var view_dist = __webpack_require__(19421);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@codemirror+commands@6.10.3/node_modules/@codemirror/commands/dist/index.js
var commands_dist = __webpack_require__(52198);
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
    }, (_, i)=>"L".concat(i + 1, " text")).join("\n");
}
function spikeRegistry() {
    var _w;
    const w = window;
    var ___spike;
    (___spike = (_w = w).__spike) !== null && ___spike !== void 0 ? ___spike : _w.__spike = {
        views: {}
    };
    return w.__spike;
}
function CmView(props) {
    const host = (0,react.useRef)(null);
    const { testId, doc, readOnly, hostile, onReady } = props;
    (0,react.useEffect)(()=>{
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
        onReady === null || onReady === void 0 ? void 0 : onReady(view);
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
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
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
        var _switcher_current;
        (_switcher_current = switcher.current) === null || _switcher_current === void 0 ? void 0 : _switcher_current.switchTo(buffers.current[which]);
        setActive(which);
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "tablist",
                "aria-label": "Spike buffers",
                children: [
                    "a",
                    "b"
                ].map((k)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        role: "tab",
                        "aria-selected": active === k,
                        "data-testid": "spike-tab-".concat(k),
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
            /*#__PURE__*/ (0,jsx_runtime.jsx)(CmView, {
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
function pageLabel(page, total) {
    let size = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : ACCESSIBLE_PAGE_SIZE;
    const first = page * size + 1;
    const last = Math.min(total, (page + 1) * size);
    return "Lines ".concat(first, "-").concat(last, " of ").concat(total);
}
const btn = {
    minHeight: 44,
    minWidth: 44
};
function AccessibleViewSpike(param) {
    let { lines } = param;
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
            setAnnounce("Line ".concat(n));
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        "aria-label": "Accessible view",
        "data-testid": "spike-accessible-view",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("nav", {
                "aria-label": "Pager",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        style: btn,
                        disabled: page === 0,
                        onClick: ()=>setPage(page - 1),
                        children: "Previous page"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "data-testid": "spike-page-label",
                        "aria-live": "polite",
                        children: label
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        style: btn,
                        disabled: page >= pages - 1,
                        onClick: ()=>setPage(page + 1),
                        children: "Next page"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        children: [
                            "Go to line ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
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
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        style: btn,
                        onClick: go,
                        children: "Go"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                role: "status",
                "data-testid": "spike-announce",
                children: announce
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                tabIndex: 0,
                "aria-label": label,
                "data-testid": "spike-accessible-page",
                style: {
                    maxHeight: 300,
                    overflow: "auto"
                },
                children: lines.slice(start, start + ACCESSIBLE_PAGE_SIZE).map((text, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        "data-testid": "spike-accessible-line",
                        style: {
                            display: "block"
                        },
                        children: "".concat(start + i + 1, " ").concat(text)
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
    (0,react.useEffect)(()=>{
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
        } catch (e) {
            setCopied("Clipboard blocked: long-press the JSON box below and copy");
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        "aria-label": "Key event capture",
        "data-testid": "spike-key-panel",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                children: "Type below or in the editor. Each keydown is listed here and stays on this screen."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                "data-testid": "spike-plain-input",
                "aria-label": "Plain text input",
                style: {
                    width: "100%",
                    minHeight: 44
                }
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                style: {
                    minHeight: 44
                },
                "data-testid": "spike-copy-json",
                onClick: copy,
                children: "Copy as JSON"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                style: {
                    minHeight: 44
                },
                onClick: ()=>setEvents([]),
                children: "Clear"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                role: "status",
                children: copied
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                "data-testid": "spike-key-count",
                children: [
                    events.length,
                    " keydown events"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("ol", {
                style: {
                    fontFamily: "monospace",
                    fontSize: 12,
                    maxHeight: 160,
                    overflow: "auto"
                },
                children: events.map((e, i)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("li", {
                        children: "key=".concat(JSON.stringify(e.key), " code=").concat(e.code, " keyCode=").concat(e.keyCode, " isComposing=").concat(e.isComposing, " isTrusted=").concat(e.isTrusted, " ctrl=").concat(e.ctrlKey, " meta=").concat(e.metaKey)
                    }, i))
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
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
    (0,react.useEffect)(()=>{
        const vv = window.visualViewport;
        const read = ()=>{
            var _vv_height;
            const vvHeight = (_vv_height = vv === null || vv === void 0 ? void 0 : vv.height) !== null && _vv_height !== void 0 ? _vv_height : window.innerHeight;
            var _vv_offsetTop;
            const vvOffsetTop = (_vv_offsetTop = vv === null || vv === void 0 ? void 0 : vv.offsetTop) !== null && _vv_offsetTop !== void 0 ? _vv_offsetTop : 0;
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
        vv === null || vv === void 0 ? void 0 : vv.addEventListener("resize", read);
        vv === null || vv === void 0 ? void 0 : vv.addEventListener("scroll", read);
        return ()=>{
            vv === null || vv === void 0 ? void 0 : vv.removeEventListener("resize", read);
            vv === null || vv === void 0 ? void 0 : vv.removeEventListener("scroll", read);
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
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                "data-testid": "spike-vv-readout",
                style: {
                    fontFamily: "monospace",
                    fontSize: 12
                },
                children: "innerHeight=".concat(m.inner, " vv.height=").concat(m.vvHeight, " vv.offsetTop=").concat(m.vvOffsetTop, " keyboard=").concat(m.keyboard, " maxKeyboard=").concat(m.maxKeyboard, " size=").concat( false ? 0 : "".concat(window.innerWidth, "x").concat(window.innerHeight))
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                "data-testid": "spike-bar-fixed",
                style: {
                    ...bar,
                    bottom: 0,
                    background: "#a00"
                },
                children: "RED: fixed bottom:0 (layout viewport)"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
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
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        role: "region",
        "aria-label": "Editor",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                href: "#spike-accessible-view",
                "data-testid": "spike-skip-link",
                children: "Switch to accessible view"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                children: "CodeMirror 6 read view"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(CmView, {
                testId: "spike-cm-read",
                doc: text,
                readOnly: true,
                height: "300px"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                children: "Accessible view"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(AccessibleViewSpike, {
                lines: lines
            })
        ]
    });
}
function InputVariant() {
    const doc = (0,react.useMemo)(()=>genDoc(5000), []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        style: {
            paddingBottom: 100
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                children: "Input and viewport spike"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                children: "Tap in the editor so the soft keyboard opens. Check that the GREEN bar stays above the keyboard and the caret stays visible. Type `--` and a double-quoted word, then check for em dash or curly quotes."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(ViewportReadout, {}),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(CmView, {
                testId: "spike-input-editor",
                doc: doc,
                hostile: true,
                height: "260px"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)(KeyCapturePanel, {})
        ]
    });
}
function SpikeFilesPage() {
    const [variant, setVariant] = (0,react.useState)(null);
    (0,react.useEffect)(()=>{
        var _get;
        setVariant((_get = new URLSearchParams(window.location.search).get("v")) !== null && _get !== void 0 ? _get : "input");
    }, []);
    if (!variant) return null;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        "data-testid": "spike-variant-".concat(variant),
        children: [
            variant === "stateswap" && /*#__PURE__*/ (0,jsx_runtime.jsx)(StateSwap, {}),
            variant === "a11y" && /*#__PURE__*/ (0,jsx_runtime.jsx)(A11yVariant, {}),
            variant === "input" && /*#__PURE__*/ (0,jsx_runtime.jsx)(InputVariant, {})
        ]
    });
}


/***/ }),

/***/ 89650:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 33686));


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [4952,6489,9691,3516,7358], () => (__webpack_exec__(89650)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-8d44e533d098f8a1.js.map